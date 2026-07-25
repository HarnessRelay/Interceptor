package main

import (
	"bufio"
	"bytes"
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
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const version = "dev"

type client struct {
	baseURL string
	token   string
	http    *http.Client
}

type sessionDTO struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	CWD     string   `json:"cwd"`
	Status  string   `json:"status"`
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
  harnessctl run [--name NAME] [--cwd DIR] <command> [args...]
  harnessctl interrupt <session-id>
  harnessctl terminate <session-id>
  harnessctl attach <session-id>       detach with Ctrl-]

Environment:
  HARNESSRELAY_ADDR   default http://127.0.0.1:8765
  HARNESSRELAY_TOKEN  required for authenticated API calls
`)
}

func (c client) attach(id string, stdin *os.File, stdout io.Writer) error {
	var snapshot snapshotResponse
	if err := c.request(http.MethodGet, "/api/v1/sessions/"+id+"/snapshot", nil, &snapshot); err != nil {
		return err
	}
	for _, chunk := range snapshot.Chunks {
		if chunk.Encoding != "base64" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(chunk.Bytes)
		if err != nil {
			return err
		}
		if _, err := stdout.Write(data); err != nil {
			return err
		}
	}
	if err := c.resizeFromTerminal(id, stdout); err != nil {
		// Non-TTY output is fine; attach can still stream bytes.
		_, _ = fmt.Fprintf(stdout, "\r\nresize unavailable: %v\r\n", err)
	}
	stopResize := c.forwardResizeSignals(id, stdout)
	defer stopResize()

	restore, err := makeRaw(stdin)
	if err == nil {
		defer restore()
	}

	ws, err := c.openWebSocket("/api/v1/ws?session_id=" + url.QueryEscape(id) + "&after_seq=" + fmt.Sprint(snapshot.LatestSequence))
	if err != nil {
		return err
	}
	defer ws.Close()

	errCh := make(chan error, 2)
	done := make(chan struct{})
	go func() {
		defer close(done)
		errCh <- c.streamWebSocketOutput(ws, id, stdout)
	}()
	go func() {
		errCh <- c.streamAttachInput(stdin, id)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, errDetach) || errors.Is(err, io.EOF) {
			return nil
		}
		return err
	case <-done:
		return nil
	}
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
		if event.SessionID != id || event.Type != "terminal.output" {
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

func makeRaw(file *os.File) (func(), error) {
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
	return func() {
		_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&old)))
	}, nil
}

func newClient() client {
	baseURL := os.Getenv("HARNESSRELAY_ADDR")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:8765"
	}
	return client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   os.Getenv("HARNESSRELAY_TOKEN"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c client) status(stdout io.Writer) error {
	var health struct {
		Status  string `json:"status"`
		Service string `json:"service"`
		Version string `json:"version"`
	}
	if err := c.request(http.MethodGet, "/api/v1/health", nil, &health); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s %s (%s)\n", health.Service, health.Status, health.Version)
	return nil
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
		default:
			goto parsed
		}
	}
parsed:
	if len(args) == 0 {
		return errors.New("run requires a command")
	}
	body := map[string]any{
		"name":     name,
		"command":  args[0],
		"args":     args[1:],
		"cwd":      cwd,
		"terminal": map[string]any{"rows": 24, "cols": 80},
	}
	var resp struct {
		Session sessionDTO `json:"session"`
	}
	if err := c.request(http.MethodPost, "/api/v1/sessions", body, &resp); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s\t%s\t%s\n", resp.Session.ID, resp.Session.Status, resp.Session.Command)
	return nil
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
