package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/harnessrelay/interceptor/internal/config"
	"github.com/harnessrelay/interceptor/internal/service"
	"github.com/harnessrelay/interceptor/internal/shims"
	"github.com/harnessrelay/interceptor/internal/terminalcleanup"
)

const version = "dev"

type client struct {
	baseURL     string
	token       string
	tokenSource string
	http        *http.Client
}

type sessionDTO struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	CWD      string   `json:"cwd"`
	Status   string   `json:"status"`
	ExitCode *int     `json:"exit_code"`
}

type snapshotResponse struct {
	LatestSequence uint64 `json:"latest_seq"`
	Chunks         []struct {
		Encoding string `json:"encoding"`
		Bytes    string `json:"bytes"`
	} `json:"chunks"`
}

type eventEnvelope struct {
	Type      string          `json:"type"`
	SessionID string          `json:"session_id"`
	Sequence  uint64          `json:"seq"`
	Data      json.RawMessage `json:"data"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		var exitErr processExitError
		if errors.As(err, &exitErr) {
			code := exitErr.code
			if code < 0 {
				code = 1
			}
			os.Exit(code)
		}
		fmt.Fprintf(os.Stderr, "harnessctl: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		printUsage(stdout)
		return nil
	}
	c := newClient()
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, version)
	case "status":
		return c.status(stdout)
	case "sessions", "list":
		return c.sessions(stdout)
	case "shims":
		return runShims(args[1:], stdout, stderr)
	case "services":
		return runServices(args[1:], stdout, stderr)
	case "shim":
		return c.runShim(args[1:], stdout, stderr)
	case "run":
		return c.runSession(args[1:], stdout)
	case "interrupt":
		if len(args) != 2 {
			return errors.New("interrupt requires a session id")
		}
		return c.control(args[1], "interrupt", map[string]any{"strategy": "ctrl_c"}, stdout)
	case "terminate", "stop":
		if len(args) != 2 {
			return errors.New("terminate requires a session id")
		}
		return c.control(args[1], "terminate", map[string]any{"grace_ms": 5000}, stdout)
	case "attach":
		if len(args) != 2 {
			return errors.New("attach requires a session id")
		}
		return c.attach(args[1], os.Stdin, stdout)
	default:
		printUsage(stderr)
		return fmt.Errorf("unknown command: %s", args[0])
	}
	return nil
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, `harnessctl controls a local harnessd daemon.

Usage:
  harnessctl --help
  harnessctl version
  harnessctl status
  harnessctl sessions
  harnessctl run [--name NAME] [--cwd DIR] [--backend pty|direct] -- <command> [args...]
  harnessctl interrupt <session-id>
  harnessctl terminate <session-id>
  harnessctl attach <session-id>       detach with Ctrl-]
  harnessctl shims <install|uninstall|uninstall-all|list|status|doctor|reshim|path>
  harnessctl services <install|uninstall|start|stop|restart|enable|disable|status|logs>

Environment:
  HARNESSRELAY_ADDR   default http://127.0.0.1:8765
  HARNESSRELAY_TOKEN  overrides the installed user-local token
  HARNESSRELAY_BYPASS set to 1 to execute a shim's real binary directly
`)
}

func runServices(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(stdout, `Manage the rootless HarnessRelay systemd user service.

Usage:
  harnessctl services install
  harnessctl services uninstall
  harnessctl services start
  harnessctl services stop
  harnessctl services restart
  harnessctl services enable
  harnessctl services disable
  harnessctl services status
  harnessctl services logs
`)
		return nil
	}
	if len(args) != 1 {
		return errors.New("services commands accept exactly one verb")
	}
	manager, err := service.NewManager()
	if err != nil {
		return err
	}
	ctx := context.Background()
	switch args[0] {
	case "install":
		if err := manager.Install(ctx, stdout, stderr); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "installed %s\nStart now: harnessctl services start\nStart at login: harnessctl services enable\n", manager.UnitPath)
	case "uninstall":
		if err := manager.Uninstall(ctx, stdout, stderr); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "uninstalled %s\n", service.UnitName)
	case "start":
		return manager.Start(ctx, stdout, stderr)
	case "stop":
		return manager.Stop(ctx, stdout, stderr)
	case "restart":
		return manager.Restart(ctx, stdout, stderr)
	case "enable":
		return manager.Enable(ctx, stdout, stderr)
	case "disable":
		return manager.Disable(ctx, stdout, stderr)
	case "status":
		return manager.Status(ctx, stdout, stderr)
	case "logs":
		return manager.Logs(ctx, stdout, stderr)
	default:
		return fmt.Errorf("unknown services command: %s", args[0])
	}
	return nil
}

func (c client) attach(id string, stdin *os.File, stdout io.Writer) error {
	localTerminal := &localTerminalState{writer: localProtocolWriter(stdout)}
	defer localTerminal.Restore()

	var snapshot snapshotResponse
	if err := c.request(http.MethodGet, "/api/v1/sessions/"+id+"/snapshot", nil, &snapshot); err != nil {
		return err
	}
	initialBytes, err := decodedSnapshot(snapshot)
	if err != nil {
		return err
	}
	if _, err := stdout.Write(initialBytes); err != nil {
		return err
	}
	var current struct {
		Session sessionDTO `json:"session"`
	}
	if err := c.request(http.MethodGet, "/api/v1/sessions/"+id, nil, &current); err != nil {
		return err
	}
	if current.Session.Status == "exited" || current.Session.Status == "failed" || current.Session.Status == "terminated" {
		var finalSnapshot snapshotResponse
		if err := c.request(http.MethodGet, "/api/v1/sessions/"+id+"/snapshot", nil, &finalSnapshot); err != nil {
			return err
		}
		finalBytes, err := decodedSnapshot(finalSnapshot)
		if err != nil {
			return err
		}
		if bytes.HasPrefix(finalBytes, initialBytes) {
			if _, err := stdout.Write(finalBytes[len(initialBytes):]); err != nil {
				return err
			}
		}
		if current.Session.ExitCode != nil && *current.Session.ExitCode != 0 {
			return processExitError{code: *current.Session.ExitCode}
		}
		return nil
	}
	if err := c.resizeFromTerminal(id, stdout); err != nil {
		// Non-TTY output is fine; attach can still stream bytes.
		_, _ = fmt.Fprintf(stdout, "\r\nresize unavailable: %v\r\n", err)
	}
	stopResize := c.forwardResizeSignals(id, stdout)
	defer stopResize()

	terminal, err := makeRaw(stdin)
	if err == nil {
		localTerminal.SetRaw(terminal)
		stopSignals := watchTerminalSignals(localTerminal)
		defer stopSignals()
	}

	ws, err := c.openWebSocket("/api/v1/ws?session_id=" + url.QueryEscape(id) + "&after_seq=" + fmt.Sprint(snapshot.LatestSequence))
	if err != nil {
		return disconnected(err, localTerminal)
	}
	defer ws.Close()

	outputErrCh := make(chan error, 1)
	inputErrCh := make(chan error, 1)
	go func() {
		outputErrCh <- c.streamWebSocketOutput(ws, id, stdout)
	}()
	go func() {
		inputErrCh <- c.streamAttachInput(stdin, id)
	}()

	for {
		select {
		case err := <-outputErrCh:
			if err == nil {
				return nil
			}
			var ended sessionEndedError
			if errors.As(err, &ended) {
				if restoreErr := localTerminal.Restore(); restoreErr != nil {
					return fmt.Errorf("%w; terminal restore failed: %v", err, restoreErr)
				}
				if ended.remotelyTerminated() {
					_, _ = fmt.Fprint(stdout, "\r\nHarnessRelay session was terminated remotely. Restored local terminal.\r\n")
				}
				if ended.code != 0 {
					return processExitError{code: ended.code}
				}
				return nil
			}
			return disconnected(err, localTerminal)
		case err := <-inputErrCh:
			if errors.Is(err, errDetach) {
				return nil
			}
			if errors.Is(err, io.EOF) {
				inputErrCh = nil
				continue
			}
			_ = ws.Close()
			return disconnected(err, localTerminal)
		}
	}
}

func localProtocolWriter(stdout io.Writer) io.Writer {
	file, ok := stdout.(*os.File)
	if !ok {
		return stdout
	}
	var state syscall.Termios
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&state))); errno != 0 {
		return nil
	}
	return file
}

func decodedSnapshot(snapshot snapshotResponse) ([]byte, error) {
	var out []byte
	for _, chunk := range snapshot.Chunks {
		if chunk.Encoding != "base64" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(chunk.Bytes)
		if err != nil {
			return nil, err
		}
		out = append(out, data...)
	}
	return out, nil
}

func (c client) forwardResizeSignals(id string, stdout io.Writer) func() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
				_ = c.resizeFromTerminal(id, stdout)
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}

func (c client) streamAttachInput(stdin io.Reader, id string) error {
	buf := make([]byte, 1024)
	for {
		n, err := stdin.Read(buf)
		if n > 0 {
			data := buf[:n]
			if i := bytes.IndexByte(data, 0x1d); i >= 0 {
				if i > 0 {
					if err := c.sendRawInput(id, data[:i]); err != nil {
						return err
					}
				}
				return errDetach
			}
			if err := c.sendRawInput(id, data); err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}
}

func (c client) sendRawInput(id string, data []byte) error {
	body := map[string]any{
		"mode":     "raw",
		"encoding": "base64",
		"data":     base64.StdEncoding.EncodeToString(data),
	}
	return c.request(http.MethodPost, "/api/v1/sessions/"+id+"/input", body, nil)
}

func (c client) streamWebSocketOutput(conn net.Conn, id string, stdout io.Writer) error {
	reader := bufio.NewReader(conn)
	for {
		payload, err := readWebSocketFrame(reader)
		if err != nil {
			return err
		}
		var event eventEnvelope
		if err := json.Unmarshal(payload, &event); err != nil {
			continue
		}
		if event.SessionID != id {
			continue
		}
		if event.Type == "session.exited" {
			var data struct {
				ExitCode int    `json:"exit_code"`
				Reason   string `json:"reason"`
			}
			if err := json.Unmarshal(event.Data, &data); err != nil {
				return err
			}
			return sessionEndedError{code: data.ExitCode, reason: data.Reason}
		}
		if event.Type != "terminal.output" {
			continue
		}
		var data struct {
			Bytes string `json:"bytes"`
			Data  string `json:"data"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			continue
		}
		encoded := data.Bytes
		if encoded == "" {
			encoded = data.Data
		}
		if encoded == "" {
			continue
		}
		bytes, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		if _, err := stdout.Write(bytes); err != nil {
			return err
		}
	}
}

func (c client) openWebSocket(path string) (net.Conn, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, err
	}
	if base.Scheme != "http" {
		return nil, errors.New("attach currently supports http daemon URLs")
	}
	host := base.Host
	conn, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		return nil, err
	}
	key, err := websocketKey()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: websocket\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n"
	if c.token != "" {
		req += "Authorization: Bearer " + c.token + "\r\n"
	}
	req += "\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !strings.Contains(status, " 101 ") {
		_ = conn.Close()
		return nil, fmt.Errorf("websocket upgrade failed: %s", strings.TrimSpace(status))
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	return &bufferedConn{Conn: conn, reader: reader}, nil
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func readWebSocketFrame(reader *bufio.Reader) ([]byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, err
	}
	opcode := header[0] & 0x0f
	if opcode == 0x8 {
		return nil, io.EOF
	}
	if opcode != 0x1 {
		return nil, fmt.Errorf("unsupported websocket opcode %d", opcode)
	}
	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(reader, ext[:]); err != nil {
			return nil, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(reader, ext[:]); err != nil {
			return nil, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > 1<<20 {
		return nil, errors.New("websocket frame too large")
	}
	payload := make([]byte, length)
	_, err := io.ReadFull(reader, payload)
	return payload, err
}

func websocketKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

var errDetach = errors.New("detach requested")

type processExitError struct {
	code int
}

func (e processExitError) Error() string {
	return fmt.Sprintf("managed command exited with status %d", e.code)
}

type sessionEndedError struct {
	code   int
	reason string
}

func (e sessionEndedError) Error() string {
	return fmt.Sprintf("managed session ended with status %d (%s)", e.code, e.reason)
}

func (e sessionEndedError) remotelyTerminated() bool {
	return e.reason == "terminate" || e.reason == "kill" || e.reason == "signal"
}

type daemonDisconnectedError struct {
	cause    error
	restored bool
}

func (e daemonDisconnectedError) Error() string {
	state := "Terminal connection closed."
	if e.restored {
		state = "Restored terminal."
	}
	return fmt.Sprintf(`HarnessRelay daemon disconnected (%v). %s
If your terminal still looks wrong, run:
  printf '\033[<1u\033[>4;0m\033[?1000;1002;1003;1006l\033[?2004l'
  stty sane
  reset
Restart: harnessctl services restart
Bypass: HARNESSRELAY_BYPASS=1 <command>`, e.cause, state)
}

func (e daemonDisconnectedError) Unwrap() error { return e.cause }

func disconnected(cause error, terminal *localTerminalState) error {
	if terminal == nil {
		return daemonDisconnectedError{cause: cause}
	}
	if err := terminal.EmergencyRestore(); err != nil {
		cause = fmt.Errorf("%w; terminal restore failed: %v", cause, err)
		return daemonDisconnectedError{cause: cause}
	}
	return daemonDisconnectedError{cause: cause, restored: true}
}

func (c client) resizeFromTerminal(id string, stdout io.Writer) error {
	file, ok := stdout.(*os.File)
	if !ok {
		return errors.New("stdout is not a file")
	}
	size, err := getWinsize(file)
	if err != nil {
		return err
	}
	return c.request(http.MethodPost, "/api/v1/sessions/"+id+"/resize", map[string]any{
		"rows": int(size.Rows),
		"cols": int(size.Cols),
	}, nil)
}

type winsize struct {
	Rows uint16
	Cols uint16
	X    uint16
	Y    uint16
}

func getWinsize(file *os.File) (winsize, error) {
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return ws, errno
	}
	if ws.Rows == 0 || ws.Cols == 0 {
		return ws, errors.New("terminal size unavailable")
	}
	return ws, nil
}

type rawTerminal struct {
	file *os.File
	old  syscall.Termios
	mu   sync.Mutex
	raw  bool
}

type localTerminalState struct {
	mu     sync.Mutex
	raw    *rawTerminal
	writer io.Writer
}

func (t *localTerminalState) SetRaw(raw *rawTerminal) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.raw = raw
	t.mu.Unlock()
}

func (t *localTerminalState) Restore() error {
	return t.restore(false)
}

func (t *localTerminalState) EmergencyRestore() error {
	return t.restore(true)
}

func (t *localTerminalState) restore(emergency bool) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	var termiosErr error
	if t.raw != nil {
		termiosErr = t.raw.Restore()
	}
	var protocolErr error
	if emergency {
		protocolErr = terminalcleanup.EmergencyResetLocalTerminal(t.writer)
	} else {
		protocolErr = terminalcleanup.RestoreLocalTerminal(t.writer)
	}
	return errors.Join(termiosErr, protocolErr)
}

func (t *localTerminalState) ReenterRaw() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.raw == nil {
		return nil
	}
	return t.raw.ReenterRaw()
}

func makeRaw(file *os.File) (*rawTerminal, error) {
	if file == nil {
		return nil, errors.New("stdin is not a file")
	}
	fd := file.Fd()
	var old syscall.Termios
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&old))); errno != 0 {
		return nil, errno
	}
	raw := old
	raw.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Cflag |= syscall.CS8
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&raw))); errno != 0 {
		return nil, errno
	}
	return &rawTerminal{file: file, old: old, raw: true}, nil
}

func (t *rawTerminal) Restore() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.raw {
		return nil
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, t.file.Fd(), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&t.old)))
	if errno != 0 {
		return errno
	}
	t.raw = false
	return nil
}

func (t *rawTerminal) ReenterRaw() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.raw {
		return nil
	}
	raw := t.old
	raw.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Cflag |= syscall.CS8
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, t.file.Fd(), uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&raw))); errno != 0 {
		return errno
	}
	t.raw = true
	return nil
}

func watchTerminalSignals(terminal *localTerminalState) func() {
	signals := []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGTSTP}
	ch := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(ch, signals...)
	go func() {
		for {
			select {
			case received := <-ch:
				_ = terminal.Restore()
				signal.Reset(received)
				sig, ok := received.(syscall.Signal)
				if !ok {
					continue
				}
				_ = syscall.Kill(os.Getpid(), sig)
				if sig == syscall.SIGTSTP {
					// Execution resumes here only after SIGCONT.
					signal.Notify(ch, syscall.SIGTSTP)
					_ = terminal.ReenterRaw()
				}
			case <-done:
				return
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			signal.Stop(ch)
			close(done)
		})
	}
}

func newClient() client {
	baseURL := os.Getenv("HARNESSRELAY_ADDR")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8765"
		if cfg, err := config.Load(); err == nil {
			baseURL = "http://" + cfg.Address()
		}
	}
	token, source, _ := config.ResolveAuthToken()
	return client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		token:       token,
		tokenSource: source,
		http:        &http.Client{Timeout: 10 * time.Second},
	}
}

func (c client) status(stdout io.Writer) error {
	executable, executableErr := os.Executable()
	if executableErr != nil {
		executable = "unknown"
	} else if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	pathExecutable, pathErr := exec.LookPath("harnessctl")
	pathStatus := "ready"
	if pathErr != nil {
		pathExecutable = "not found"
		pathStatus = "harnessctl is not available from PATH"
	} else if pathResolved, err := filepath.EvalSymlinks(pathExecutable); err == nil && pathResolved != executable {
		pathStatus = "PATH resolves to a different harnessctl"
	}
	installTarget := filepath.Join(userHomeOrDot(), ".local", "bin", "harnessctl")
	if override := os.Getenv("HARNESSRELAY_BIN_DIR"); override != "" {
		installTarget = filepath.Join(override, "harnessctl")
	}
	configPath, configErr := config.ConfigPath()
	if configErr != nil {
		configPath = "unavailable: " + configErr.Error()
	}
	tokenPath, tokenPathErr := config.TokenPath()
	if tokenPathErr != nil {
		tokenPath = "unavailable: " + tokenPathErr.Error()
	}
	shimPath, shimErr := shims.DefaultShimDir()
	if shimErr != nil {
		shimPath = "unavailable: " + shimErr.Error()
	}
	servicePath, servicePathErr := service.DefaultUnitPath()
	serviceState := "not installed"
	if servicePathErr != nil {
		servicePath = "unavailable: " + servicePathErr.Error()
		serviceState = "unavailable"
	} else if content, err := os.ReadFile(servicePath); err == nil {
		if service.IsManaged(content) {
			serviceState = "installed (run: harnessctl services status)"
		} else {
			serviceState = "unmanaged file present"
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		serviceState = "unavailable: " + err.Error()
	}

	fmt.Fprintln(stdout, "HarnessRelay status")
	fmt.Fprintf(stdout, "  version: %s\n", version)
	fmt.Fprintf(stdout, "  configured address: %s\n", c.baseURL)
	var health struct {
		Status  string `json:"status"`
		Service string `json:"service"`
		Version string `json:"version"`
	}
	if err := c.request(http.MethodGet, "/api/v1/health", nil, &health); err != nil {
		fmt.Fprintf(stdout, "  daemon: unreachable (%v)\n", err)
	} else {
		fmt.Fprintf(stdout, "  daemon: reachable\n")
		fmt.Fprintf(stdout, "  %s %s (%s)\n", health.Service, health.Status, health.Version)
	}
	fmt.Fprintln(stdout, "Auth:")
	fmt.Fprintf(stdout, "  token source: %s\n", c.tokenSource)
	fmt.Fprintf(stdout, "  token file: %s\n", tokenPath)
	fmt.Fprintln(stdout, "Install:")
	fmt.Fprintf(stdout, "  active binary: %s\n", executable)
	fmt.Fprintf(stdout, "  PATH binary: %s\n", pathExecutable)
	fmt.Fprintf(stdout, "  PATH status: %s\n", pathStatus)
	fmt.Fprintf(stdout, "  default install target: %s\n", installTarget)
	fmt.Fprintf(stdout, "  config: %s\n", configPath)
	fmt.Fprintf(stdout, "  shim path: %s\n", shimPath)
	fmt.Fprintf(stdout, "  user service: %s\n", serviceState)
	fmt.Fprintf(stdout, "  service unit: %s\n", servicePath)
	return nil
}

func userHomeOrDot() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}

func (c client) sessions(stdout io.Writer) error {
	var resp struct {
		Sessions []sessionDTO `json:"sessions"`
	}
	if err := c.request(http.MethodGet, "/api/v1/sessions", nil, &resp); err != nil {
		return err
	}
	if len(resp.Sessions) == 0 {
		fmt.Fprintln(stdout, "no sessions")
		return nil
	}
	for _, session := range resp.Sessions {
		fmt.Fprintf(stdout, "%s\t%s\t%s %s\n", session.ID, session.Status, session.Command, strings.Join(session.Args, " "))
	}
	return nil
}

func (c client) runSession(args []string, stdout io.Writer) error {
	var name, cwd string
	backend := shims.BackendPTY
	for len(args) > 0 {
		switch args[0] {
		case "--name":
			if len(args) < 2 {
				return errors.New("--name requires a value")
			}
			name = args[1]
			args = args[2:]
		case "--cwd":
			if len(args) < 2 {
				return errors.New("--cwd requires a value")
			}
			cwd = args[1]
			args = args[2:]
		case "--backend":
			if len(args) < 2 {
				return errors.New("--backend requires a value")
			}
			var err error
			backend, err = shims.ParseBackend(args[1])
			if err != nil {
				return err
			}
			args = args[2:]
		case "--":
			args = args[1:]
			goto parsed
		default:
			goto parsed
		}
	}
parsed:
	if len(args) == 0 {
		return errors.New("run requires a command")
	}
	if backend == shims.BackendDirect {
		path, err := exec.LookPath(args[0])
		if err != nil {
			return err
		}
		return execDirect(path, args[1:])
	}
	if backend == shims.BackendTMUX {
		return errors.New("tmux relay backend is deferred; use --backend pty or --backend direct")
	}
	rows, cols := 24, 80
	if file, ok := stdout.(*os.File); ok {
		if size, err := getWinsize(file); err == nil {
			rows, cols = int(size.Rows), int(size.Cols)
		}
	}
	body := map[string]any{
		"name":     name,
		"command":  args[0],
		"args":     args[1:],
		"cwd":      cwd,
		"terminal": map[string]any{"rows": rows, "cols": cols},
	}
	var resp struct {
		Session sessionDTO `json:"session"`
	}
	if err := c.request(http.MethodPost, "/api/v1/sessions", body, &resp); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\n", resp.Session.ID, resp.Session.Status, resp.Session.Command)
	if file, ok := stdout.(*os.File); ok {
		if _, err := getWinsize(file); err == nil {
			return c.attach(resp.Session.ID, os.Stdin, stdout)
		}
	}
	return nil
}

func runShims(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printShimsUsage(stdout)
		return nil
	}
	configPath, err := shims.DefaultConfigPath()
	if err != nil {
		return err
	}
	switch args[0] {
	case "path":
		if len(args) != 1 {
			return errors.New("shims path accepts no arguments")
		}
		cfg, err := shims.Load(configPath)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, cfg.ShimDir)
		return nil
	case "list":
		if len(args) != 1 {
			return errors.New("shims list accepts no arguments")
		}
		cfg, err := shims.Load(configPath)
		if err != nil {
			return err
		}
		seen := make(map[string]struct{})
		for _, target := range shims.KnownTargets() {
			state := "available"
			if _, ok := cfg.Entries[target.Name]; ok {
				state = "installed"
			}
			fmt.Fprintf(stdout, "%s\t%s\t%s\n", target.Name, state, target.Description)
			seen[target.Name] = struct{}{}
		}
		for _, name := range shims.SortedEntryNames(cfg) {
			if _, ok := seen[name]; ok {
				continue
			}
			fmt.Fprintf(stdout, "%s\tinstalled\tuser-configured shim target\n", name)
		}
		return nil
	case "install":
		return installShims(configPath, args[1:], stdout, stderr)
	case "uninstall":
		if len(args) < 2 {
			return errors.New("shims uninstall requires at least one shim name")
		}
		if err := shims.Uninstall(configPath, args[1:]); err != nil {
			return err
		}
		for _, name := range args[1:] {
			fmt.Fprintf(stdout, "uninstalled %s\n", name)
		}
		return nil
	case "uninstall-all":
		if len(args) != 1 {
			return errors.New("shims uninstall-all accepts no arguments")
		}
		if err := shims.UninstallAll(configPath); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "uninstalled all HarnessRelay-owned shims")
		return nil
	case "reshim":
		if len(args) != 1 {
			return errors.New("shims reshim accepts no arguments")
		}
		harnessctl, err := os.Executable()
		if err != nil {
			return err
		}
		if err := shims.Reshim(configPath, harnessctl); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "regenerated HarnessRelay shims from config")
		return nil
	case "status":
		return printShimsStatus(configPath, stdout)
	case "doctor":
		return doctorShims(configPath, stdout)
	default:
		printShimsUsage(stderr)
		return fmt.Errorf("unknown shims command: %s", args[0])
	}
}

func printShimsUsage(w io.Writer) {
	fmt.Fprint(w, `Manage user-local HarnessRelay command shims.

Usage:
  harnessctl shims install [--all-known] [--backend pty|tmux|direct] [--real-binary PATH] [--force] <name>...
  harnessctl shims uninstall <name>...
  harnessctl shims uninstall-all
  harnessctl shims list
  harnessctl shims status
  harnessctl shims doctor
  harnessctl shims reshim
  harnessctl shims path
`)
}

func installShims(configPath string, args []string, stdout, stderr io.Writer) error {
	cfg, err := shims.Load(configPath)
	if err != nil {
		return err
	}
	backend := cfg.DefaultBackend
	var realBinary string
	var force, allKnown bool
	var names []string
	for len(args) > 0 {
		switch args[0] {
		case "--backend":
			if len(args) < 2 {
				return errors.New("--backend requires a value")
			}
			var err error
			backend, err = shims.ParseBackend(args[1])
			if err != nil {
				return err
			}
			args = args[2:]
		case "--real-binary":
			if len(args) < 2 {
				return errors.New("--real-binary requires a value")
			}
			realBinary = args[1]
			args = args[2:]
		case "--force":
			force = true
			args = args[1:]
		case "--all-known":
			allKnown = true
			args = args[1:]
		case "--":
			names = append(names, args[1:]...)
			args = nil
		default:
			if strings.HasPrefix(args[0], "-") {
				return fmt.Errorf("unknown shims install option: %s", args[0])
			}
			names = append(names, args[0])
			args = args[1:]
		}
	}
	if allKnown {
		for _, target := range shims.KnownTargets() {
			names = append(names, target.Name)
		}
	}
	names = uniqueStrings(names)
	if len(names) == 0 {
		return errors.New("shims install requires a shim name or --all-known")
	}
	if realBinary != "" && len(names) != 1 {
		return errors.New("--real-binary requires exactly one shim name")
	}
	harnessctl, err := os.Executable()
	if err != nil {
		return err
	}
	var installed []shims.Entry
	for _, name := range names {
		selectedRealBinary := realBinary
		if selectedRealBinary == "" {
			candidates, resolveErr := shims.ResolveRealBinaryCandidates(name, os.Getenv("PATH"), cfg.ShimDir)
			if resolveErr != nil {
				if allKnown {
					fmt.Fprintf(stderr, "skipped %s: real binary is not installed\n", name)
					continue
				}
				return resolveErr
			}
			selectedRealBinary = candidates[0]
			if len(candidates) > 1 {
				fmt.Fprintf(stderr, "multiple real binaries found for %s; using %s\n", name, candidates[0])
				for _, candidate := range candidates[1:] {
					fmt.Fprintf(stderr, "  also found: %s\n", candidate)
				}
			}
		}
		entry, err := shims.Install(shims.InstallOptions{
			Name: name, RealBinary: selectedRealBinary, Backend: backend,
			Harnessctl: harnessctl, Force: force, ConfigPath: configPath,
		})
		if err != nil {
			return err
		}
		installed = append(installed, entry)
		fmt.Fprintf(stdout, "installed %s -> %s (%s)\n", name, entry.RealBinary, entry.Backend)
	}
	if len(installed) == 0 {
		return errors.New("no requested shim targets have an installed real binary")
	}
	pathOK := true
	for _, entry := range installed {
		if !shims.AnalyzePath(cfg.ShimDir, entry.RealBinary, os.Getenv("PATH")).Active {
			pathOK = false
		}
	}
	if !pathOK {
		fmt.Fprintf(stderr, "HarnessRelay shims are installed, but %s is not active before the real binaries in PATH.\n", cfg.ShimDir)
		fmt.Fprintf(stderr, "Add this to your shell profile:\n\n  export PATH=%q:$PATH\n\nThen restart your shell.\n", cfg.ShimDir)
	}
	return nil
}

func printShimsStatus(configPath string, stdout io.Writer) error {
	cfg, err := shims.Load(configPath)
	if err != nil {
		return err
	}
	if len(cfg.Entries) == 0 {
		fmt.Fprintln(stdout, "no HarnessRelay shims installed")
		return nil
	}
	for _, name := range shims.SortedEntryNames(cfg) {
		entry := cfg.Entries[name]
		state := shims.AnalyzePath(cfg.ShimDir, entry.RealBinary, os.Getenv("PATH"))
		active := "inactive"
		if state.Active {
			active = "active"
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\treal=%s\tshim=%s\n", name, active, entry.Backend, entry.RealBinary, entry.ShimPath)
	}
	return nil
}

func doctorShims(configPath string, stdout io.Writer) error {
	cfg, err := shims.Load(configPath)
	if err != nil {
		fmt.Fprintf(stdout, "[fail] config: %v\n", err)
		return nil
	}
	fmt.Fprintf(stdout, "[ok] config: %s\n", configPath)
	if len(cfg.Entries) == 0 {
		fmt.Fprintln(stdout, "[warn] no shims are installed")
	}
	for _, name := range shims.SortedEntryNames(cfg) {
		entry := cfg.Entries[name]
		if err := shims.ValidateRuntimeEntry(name, entry); err != nil {
			fmt.Fprintf(stdout, "[fail] %s: %v\n", name, err)
			continue
		}
		owned, err := shims.IsManagedShim(entry.ShimPath)
		if err != nil {
			fmt.Fprintf(stdout, "[fail] %s shim: %v\n", name, err)
		} else if !owned {
			fmt.Fprintf(stdout, "[fail] %s shim is not owned by HarnessRelay\n", name)
		} else if info, err := os.Stat(entry.ShimPath); err != nil || info.Mode()&0o111 == 0 {
			fmt.Fprintf(stdout, "[fail] %s shim is not executable\n", name)
		} else {
			fmt.Fprintf(stdout, "[ok] %s shim file\n", name)
		}
		state := shims.AnalyzePath(cfg.ShimDir, entry.RealBinary, os.Getenv("PATH"))
		if !state.Present {
			fmt.Fprintf(stdout, "[fail] %s: shim directory is missing from PATH\n", name)
		} else if !state.Active {
			fmt.Fprintf(stdout, "[fail] %s: shim directory appears after the real binary directory in PATH\n", name)
		} else {
			fmt.Fprintf(stdout, "[ok] %s PATH order\n", name)
		}
		if entry.Backend == shims.BackendTMUX {
			if _, err := exec.LookPath("tmux"); err != nil {
				fmt.Fprintln(stdout, "[fail] tmux backend requested but tmux is unavailable")
			} else {
				fmt.Fprintln(stdout, "[warn] tmux is installed, but HarnessRelay tmux registration is deferred; runtime falls back to PTY")
			}
		}
	}
	if entries, err := os.ReadDir(cfg.ShimDir); err == nil {
		for _, item := range entries {
			if item.IsDir() {
				continue
			}
			if _, configured := cfg.Entries[item.Name()]; !configured {
				fmt.Fprintf(stdout, "[warn] unmanaged file in shim directory may shadow %s: %s\n", item.Name(), filepath.Join(cfg.ShimDir, item.Name()))
			}
		}
	}
	c := newClient()
	if err := c.health(); err != nil {
		fmt.Fprintf(stdout, "[warn] daemon unavailable: %v; configured fallback is %s\n", err, cfg.DaemonUnavailableFallback)
	} else {
		fmt.Fprintln(stdout, "[ok] daemon reachable")
	}
	fmt.Fprintln(stdout, "[info] bypass any shim with HARNESSRELAY_BYPASS=1")
	return nil
}

func (c client) runShim(args []string, stdout, stderr io.Writer) error {
	if len(args) < 2 || args[0] != "exec" {
		return errors.New("usage: harnessctl shim exec <shim-name> -- <args...>")
	}
	name := args[1]
	childArgs := args[2:]
	if len(childArgs) > 0 && childArgs[0] == "--" {
		childArgs = childArgs[1:]
	}
	configPath, err := shims.DefaultConfigPath()
	if err != nil {
		return err
	}
	cfg, err := shims.Load(configPath)
	if err != nil {
		return err
	}
	entry, ok := cfg.Entries[name]
	if !ok || !entry.Enabled {
		return fmt.Errorf("shim %q is not installed or enabled", name)
	}
	if err := shims.ValidateRuntimeEntry(name, entry); err != nil {
		return err
	}
	if os.Getenv("HARNESSRELAY_BYPASS") == "1" {
		return execDirect(entry.RealBinary, childArgs)
	}
	backend := entry.Backend
	if backend == shims.BackendDirect {
		return execDirect(entry.RealBinary, childArgs)
	}
	if err := c.health(); err != nil {
		if cfg.DaemonUnavailableFallback == shims.BackendDirect {
			fmt.Fprintf(stderr, "HarnessRelay daemon unavailable (%v); running %s directly. No relay session will be created.\n", err, name)
			return execDirect(entry.RealBinary, childArgs)
		}
		return fmt.Errorf("daemon unavailable: %w", err)
	}
	if backend == shims.BackendTMUX {
		fmt.Fprintln(stderr, "HarnessRelay tmux registration is deferred; using the daemon-owned PTY backend.")
		backend = shims.BackendPTY
	}
	if backend != shims.BackendPTY {
		return fmt.Errorf("unsupported shim backend %q", backend)
	}
	rows, cols := 24, 80
	if file, ok := stdout.(*os.File); ok {
		if size, err := getWinsize(file); err == nil {
			rows, cols = int(size.Rows), int(size.Cols)
		}
	}
	session, err := c.createShimSession(name, entry, childArgs, rows, cols)
	if err != nil {
		return err
	}
	return c.attach(session.ID, os.Stdin, stdout)
}

func (c client) health() error {
	var health struct {
		Status string `json:"status"`
	}
	if err := c.request(http.MethodGet, "/api/v1/health", nil, &health); err != nil {
		return err
	}
	if health.Status != "ok" {
		return fmt.Errorf("daemon health is %q", health.Status)
	}
	return nil
}

func (c client) createShimSession(name string, entry shims.Entry, args []string, rows, cols int) (sessionDTO, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return sessionDTO{}, err
	}
	body := map[string]any{
		"name": name, "harness_type": entry.Harness, "command": entry.RealBinary,
		"args": args, "cwd": cwd, "env": currentEnvironment(),
		"terminal": map[string]any{"rows": rows, "cols": cols},
		"origin":   "shim", "origin_backend": string(shims.BackendPTY),
		"shim_name": name, "real_binary": entry.RealBinary, "attachable": true,
	}
	var resp struct {
		Session sessionDTO `json:"session"`
	}
	if err := c.request(http.MethodPost, "/api/v1/sessions", body, &resp); err != nil {
		return sessionDTO{}, err
	}
	return resp.Session, nil
}

func execDirect(path string, args []string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return syscall.Exec(absolute, append([]string{absolute}, args...), os.Environ())
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func currentEnvironment() map[string]string {
	env := make(map[string]string)
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

func (c client) control(id, action string, body map[string]any, stdout io.Writer) error {
	if err := c.request(http.MethodPost, "/api/v1/sessions/"+id+"/"+action, body, nil); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s accepted for %s\n", action, id)
	return nil
}

func (c client) request(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		if len(data) == 0 {
			return fmt.Errorf("%s %s failed: %s", method, path, resp.Status)
		}
		return fmt.Errorf("%s %s failed: %s", method, path, strings.TrimSpace(string(data)))
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
