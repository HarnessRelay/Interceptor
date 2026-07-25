package pty

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	creackpty "github.com/creack/pty"
)

const (
	defaultRows = 24
	defaultCols = 80
)

// StartOptions describes the process to start inside a PTY.
type StartOptions struct {
	Command string
	Args    []string
	WorkDir string
	Env     []string
	Rows    int
	Cols    int
}

// Runtime owns one child process and its PTY master.
type Runtime struct {
	cmd  *exec.Cmd
	file *os.File

	pid  int
	pgid int

	done      chan struct{}
	waitMu    sync.Mutex
	waitErr   error
	closeOnce sync.Once
}

// Start launches a command in a new session with a controlling terminal.
func Start(ctx context.Context, opts StartOptions) (*Runtime, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.Command == "" {
		return nil, errors.New("pty: command is required")
	}
	rows, cols, err := terminalSize(opts.Rows, opts.Cols)
	if err != nil {
		return nil, err
	}
	if opts.WorkDir != "" {
		info, err := os.Stat(opts.WorkDir)
		if err != nil {
			return nil, fmt.Errorf("pty: invalid working directory: %w", err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("pty: working directory is not a directory: %s", opts.WorkDir)
		}
	}
	env, err := mergedEnv(opts.Env)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(opts.Command, opts.Args...)
	cmd.Dir = opts.WorkDir
	cmd.Env = env

	f, err := creackpty.StartWithAttrs(cmd, &creackpty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	}, &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		_ = f.Close()
		_ = terminateProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
		return nil, err
	}

	pid := cmd.Process.Pid
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		_ = f.Close()
		_ = terminateProcessGroup(pid, syscall.SIGKILL)
		_, _ = cmd.Process.Wait()
		return nil, fmt.Errorf("pty: get process group: %w", err)
	}

	r := &Runtime{
		cmd:  cmd,
		file: f,
		pid:  pid,
		pgid: pgid,
		done: make(chan struct{}),
	}
	go r.wait()

	return r, nil
}

func (r *Runtime) Read(p []byte) (int, error) {
	return r.file.Read(p)
}

func (r *Runtime) Write(p []byte) (int, error) {
	return r.file.Write(p)
}

func (r *Runtime) Resize(rows, cols int) error {
	rows, cols, err := terminalSize(rows, cols)
	if err != nil {
		return err
	}
	return creackpty.Setsize(r.file, &creackpty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
}

func (r *Runtime) Interrupt() error {
	_, err := r.Write([]byte{0x03})
	return err
}

func (r *Runtime) Terminate(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-r.done:
		return nil
	default:
	}
	if err := terminateProcessGroup(r.pgid, syscall.SIGTERM); err != nil {
		return err
	}
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		if err := terminateProcessGroup(r.pgid, syscall.SIGKILL); err != nil {
			return err
		}
		<-r.done
		return nil
	}
}

func (r *Runtime) Kill() error {
	select {
	case <-r.done:
		return nil
	default:
	}
	if err := terminateProcessGroup(r.pgid, syscall.SIGKILL); err != nil {
		return err
	}
	<-r.done
	return nil
}

func (r *Runtime) Wait() error {
	<-r.done
	r.waitMu.Lock()
	defer r.waitMu.Unlock()
	return r.waitErr
}

func (r *Runtime) PID() int {
	return r.pid
}

func (r *Runtime) PGID() int {
	return r.pgid
}

func (r *Runtime) Close() error {
	var err error
	r.closeOnce.Do(func() {
		err = r.file.Close()
	})
	return err
}

func (r *Runtime) wait() {
	err := r.cmd.Wait()
	_ = r.Close()
	r.waitMu.Lock()
	r.waitErr = err
	r.waitMu.Unlock()
	close(r.done)
}

func terminalSize(rows, cols int) (int, int, error) {
	if rows == 0 && cols == 0 {
		return defaultRows, defaultCols, nil
	}
	if rows <= 0 || cols <= 0 {
		return 0, 0, fmt.Errorf("pty: invalid terminal size %dx%d", rows, cols)
	}
	if rows > 65535 || cols > 65535 {
		return 0, 0, fmt.Errorf("pty: terminal size exceeds uint16 range %dx%d", rows, cols)
	}
	return rows, cols, nil
}

func mergedEnv(overrides []string) ([]string, error) {
	env := os.Environ()
	index := make(map[string]int, len(env)+len(overrides))
	for i, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			index[key] = i
		}
	}
	for _, entry := range overrides {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" || strings.ContainsRune(entry, 0) {
			return nil, fmt.Errorf("pty: invalid environment entry %q", entry)
		}
		if i, ok := index[key]; ok {
			env[i] = entry
			continue
		}
		index[key] = len(env)
		env = append(env, entry)
	}
	return env, nil
}

func terminateProcessGroup(pgid int, sig syscall.Signal) error {
	if pgid <= 0 {
		return errors.New("pty: invalid process group")
	}
	if err := syscall.Kill(-pgid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

var _ io.Reader = (*Runtime)(nil)
var _ io.Writer = (*Runtime)(nil)
