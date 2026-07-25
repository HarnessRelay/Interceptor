package pty

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartReadsCombinedOutput(t *testing.T) {
	r := startRuntime(t, StartOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	out := collectOutput(t, r)

	got := readUntil(t, out, "plain stderr", time.Second)
	if !strings.Contains(got, "plain stdout") {
		t.Fatalf("output missing stdout: %q", got)
	}
	if !strings.Contains(got, "plain stderr") {
		t.Fatalf("output missing stderr: %q", got)
	}
	if err := waitRuntime(r, time.Second); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestStartUsesWorkingDirectory(t *testing.T) {
	workDir := t.TempDir()
	r := startRuntime(t, StartOptions{
		Command: "/bin/sh",
		Args:    []string{"-c", "pwd"},
		WorkDir: workDir,
	})
	out := collectOutput(t, r)

	got := readUntil(t, out, workDir, time.Second)
	if !strings.Contains(got, workDir) {
		t.Fatalf("output missing work dir %q: %q", workDir, got)
	}
	if err := waitRuntime(r, time.Second); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestStartMergesEnvironmentOverrides(t *testing.T) {
	r := startRuntime(t, StartOptions{
		Command: "/bin/sh",
		Args:    []string{"-c", "printf '%s\\n' \"$HARNESS_RELAY_TEST\""},
		Env:     []string{"HARNESS_RELAY_TEST=override"},
	})
	out := collectOutput(t, r)

	got := readUntil(t, out, "override", time.Second)
	if !strings.Contains(got, "override") {
		t.Fatalf("output missing environment value: %q", got)
	}
	if err := waitRuntime(r, time.Second); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestWriteInputToInteractiveProcess(t *testing.T) {
	r := startRuntime(t, StartOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "interactive-echo.sh")},
	})
	out := collectOutput(t, r)
	_ = readUntil(t, out, "input>", time.Second)

	if _, err := r.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := readUntil(t, out, "echo:hello", time.Second)
	if !strings.Contains(got, "echo:hello") {
		t.Fatalf("output missing echo: %q", got)
	}
	if err := waitRuntime(r, time.Second); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestInterruptExitsLongRunningProcess(t *testing.T) {
	r := startRuntime(t, StartOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "long-running.sh")},
	})
	out := collectOutput(t, r)
	_ = readUntil(t, out, "ready", time.Second)

	if err := r.Interrupt(); err != nil {
		t.Fatalf("interrupt: %v", err)
	}
	got := readUntil(t, out, "interrupted", time.Second)
	if !strings.Contains(got, "interrupted") {
		t.Fatalf("output missing interrupt marker: %q", got)
	}
	err := waitRuntime(r, time.Second)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 130 {
		t.Fatalf("wait got %v, want exit code 130", err)
	}
}

func TestTerminateStopsLongRunningProcess(t *testing.T) {
	r := startRuntime(t, StartOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "long-running.sh")},
	})
	out := collectOutput(t, r)
	_ = readUntil(t, out, "ready", time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Terminate(ctx); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	if err := waitRuntime(r, time.Second); err == nil {
		t.Fatal("wait got nil, want signal exit")
	}
}

func TestTerminateKillsProcessThatIgnoresSIGTERM(t *testing.T) {
	r := startRuntime(t, StartOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "ignore-term.sh")},
	})
	out := collectOutput(t, r)
	_ = readUntil(t, out, "ready", time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := r.Terminate(ctx); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	err := waitRuntime(r, time.Second)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("wait got %v, want signal exit", err)
	}
}

func TestResizeNotifiesProcess(t *testing.T) {
	r := startRuntime(t, StartOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "resize-aware.sh")},
		Rows:    24,
		Cols:    80,
	})
	out := collectOutput(t, r)
	_ = readUntil(t, out, "ready", time.Second)

	if err := r.Resize(40, 100); err != nil {
		t.Fatalf("resize: %v", err)
	}
	if _, err := r.Write([]byte("size\n")); err != nil {
		t.Fatalf("write size probe: %v", err)
	}
	got := readUntil(t, out, "40 100", time.Second)
	if !strings.Contains(got, "40 100") {
		t.Fatalf("output missing resized dimensions: %q", got)
	}
}

func TestStartRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name string
		opts StartOptions
	}{
		{
			name: "empty command",
			opts: StartOptions{},
		},
		{
			name: "missing working directory",
			opts: StartOptions{Command: "/bin/sh", WorkDir: filepath.Join(t.TempDir(), "missing")},
		},
		{
			name: "one sided terminal size",
			opts: StartOptions{Command: "/bin/sh", Rows: 0, Cols: 80},
		},
		{
			name: "negative terminal size",
			opts: StartOptions{Command: "/bin/sh", Rows: -1, Cols: 80},
		},
		{
			name: "invalid env entry",
			opts: StartOptions{Command: "/bin/sh", Env: []string{"HARNESS_RELAY_TEST"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Start(context.Background(), tt.opts)
			if err == nil {
				_ = r.Close()
				t.Fatal("Start succeeded, want error")
			}
		})
	}
}

func startRuntime(t *testing.T, opts StartOptions) *Runtime {
	t.Helper()
	r, err := Start(context.Background(), opts)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if r.PID() <= 0 {
		t.Fatalf("PID = %d, want positive", r.PID())
	}
	if r.PGID() <= 0 {
		t.Fatalf("PGID = %d, want positive", r.PGID())
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_ = r.Terminate(ctx)
		_ = r.Close()
	})
	return r
}

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "testdata", "fake-harnesses", name))
	if err != nil {
		t.Fatalf("fixture path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture stat: %v", err)
	}
	return path
}

func collectOutput(t *testing.T, r *Runtime) <-chan string {
	t.Helper()
	out := make(chan string, 16)
	go func() {
		defer close(out)
		buf := make([]byte, 1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				out <- string(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	return out
}

func readUntil(t *testing.T, out <-chan string, want string, timeout time.Duration) string {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var got strings.Builder
	for {
		select {
		case chunk, ok := <-out:
			if !ok {
				return got.String()
			}
			got.WriteString(chunk)
			if strings.Contains(got.String(), want) {
				return got.String()
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %q in output %q", want, got.String())
		}
	}
}

func waitRuntime(r *Runtime, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- r.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return errors.New("timed out waiting for process")
	}
}
