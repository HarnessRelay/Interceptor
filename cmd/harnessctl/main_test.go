package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/creack/pty"
	"github.com/harnessrelay/interceptor/internal/api"
	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/security"
	"github.com/harnessrelay/interceptor/internal/service"
	"github.com/harnessrelay/interceptor/internal/session"
	"github.com/harnessrelay/interceptor/internal/shims"
	"github.com/harnessrelay/interceptor/internal/terminalcleanup"
)

func TestStatusCommand(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/health" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":  "ok",
			"service": "harnessd",
			"version": "test",
		})
	}))
	defer server.Close()
	t.Setenv("HARNESSRELAY_ADDR", server.URL)

	var out bytes.Buffer
	if err := run([]string{"status"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("run status: %v", err)
	}
	if !strings.Contains(out.String(), "harnessd ok (test)") {
		t.Fatalf("output = %q", out.String())
	}
	for _, want := range []string{"daemon: reachable", "configured address:", "token source:", "active binary:", "PATH binary:", "config:", "shim path:", "user service:", "service unit:"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("status output missing %q: %q", want, out.String())
		}
	}
}

func TestStatusReportsUnreachableWithoutFailing(t *testing.T) {
	c := client{
		baseURL:     "http://127.0.0.1:1",
		tokenSource: "missing",
		http: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("daemon offline")
		})},
	}
	var out bytes.Buffer
	if err := c.status(&out); err != nil {
		t.Fatalf("status returned error: %v", err)
	}
	if !strings.Contains(out.String(), "daemon: unreachable") || !strings.Contains(out.String(), "token source: missing") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestNewClientReadsConfigTokenWithEnvironmentOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HARNESSRELAY_TOKEN", "")
	tokenPath := filepath.Join(root, "harnessrelay", "token")
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenPath, []byte("config-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := newClient()
	if c.token != "config-token" || c.tokenSource != "config" {
		t.Fatalf("client token = %q from %q", c.token, c.tokenSource)
	}
	t.Setenv("HARNESSRELAY_TOKEN", "env-token")
	c = newClient()
	if c.token != "env-token" || c.tokenSource != "env" {
		t.Fatalf("override token = %q from %q", c.token, c.tokenSource)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestReadWebSocketFrame(t *testing.T) {
	var frame bytes.Buffer
	payload := []byte(`{"type":"terminal.output"}`)
	frame.WriteByte(0x81)
	frame.WriteByte(byte(len(payload)))
	frame.Write(payload)

	got, err := readWebSocketFrame(bufio.NewReader(&frame))
	if err != nil {
		t.Fatalf("readWebSocketFrame: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload = %q, want %q", string(got), string(payload))
	}
}

func TestReadWebSocketFrameExtendedLength(t *testing.T) {
	var frame bytes.Buffer
	payload := bytes.Repeat([]byte("x"), 130)
	frame.WriteByte(0x81)
	frame.WriteByte(126)
	var size [2]byte
	binary.BigEndian.PutUint16(size[:], uint16(len(payload)))
	frame.Write(size[:])
	frame.Write(payload)

	got, err := readWebSocketFrame(bufio.NewReader(&frame))
	if err != nil {
		t.Fatalf("readWebSocketFrame: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload length = %d, want %d", len(got), len(payload))
	}
}

func TestStreamAttachInputDetach(t *testing.T) {
	c := client{}
	err := c.streamAttachInput(bytes.NewBuffer([]byte{0x1d}), "ses_test")
	if !errors.Is(err, errDetach) {
		t.Fatalf("streamAttachInput error = %v, want detach", err)
	}
}

func TestAttachReplaysFinalSnapshotForFastExitedSession(t *testing.T) {
	var snapshotCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/sessions/ses_fast/snapshot":
			snapshotCalls++
			payload := ""
			if snapshotCalls > 1 {
				payload = "ZmFzdC1vdXRwdXQNCg=="
			}
			chunks := []map[string]any{}
			if payload != "" {
				chunks = append(chunks, map[string]any{"encoding": "base64", "bytes": payload})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"latest_seq": snapshotCalls, "chunks": chunks})
		case "/api/v1/sessions/ses_fast":
			zero := 0
			_ = json.NewEncoder(w).Encode(map[string]any{"session": map[string]any{
				"id": "ses_fast", "status": "exited", "exit_code": zero,
			}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := client{baseURL: server.URL, http: server.Client()}
	var out bytes.Buffer
	if err := c.attach("ses_fast", nil, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "fast-output\r\n"+terminalcleanup.RestoreSequence {
		t.Fatalf("output = %q", out.String())
	}
}

func TestAttachReturnsManagedExitCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/sessions/ses_failed/snapshot":
			_ = json.NewEncoder(w).Encode(map[string]any{"latest_seq": 2, "chunks": []any{}})
		case "/api/v1/sessions/ses_failed":
			_ = json.NewEncoder(w).Encode(map[string]any{"session": map[string]any{
				"id": "ses_failed", "status": "exited", "exit_code": 7,
			}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := client{baseURL: server.URL, http: server.Client()}
	err := c.attach("ses_failed", nil, &bytes.Buffer{})
	var exitErr processExitError
	if !errors.As(err, &exitErr) || exitErr.code != 7 {
		t.Fatalf("error = %v, want process exit 7", err)
	}
}

func TestRawTerminalRestoreIsIdempotent(t *testing.T) {
	master, terminalFile, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer terminalFile.Close()

	before := terminalState(t, terminalFile)
	raw, err := makeRaw(terminalFile)
	if err != nil {
		t.Fatal(err)
	}
	during := terminalState(t, terminalFile)
	if during.Lflag&syscall.ICANON != 0 || during.Lflag&syscall.ECHO != 0 {
		t.Fatalf("terminal was not made raw: lflag=%#x", during.Lflag)
	}
	if err := raw.Restore(); err != nil {
		t.Fatal(err)
	}
	if err := raw.Restore(); err != nil {
		t.Fatal(err)
	}
	after := terminalState(t, terminalFile)
	if after != before {
		t.Fatalf("terminal state was not restored\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestRawTerminalSignalHelper(t *testing.T) {
	if os.Getenv("HARNESSRELAY_TEST_RAW_SIGNAL_HELPER") != "1" {
		return
	}
	terminalFile := os.NewFile(3, "signal-test-terminal")
	raw, err := makeRaw(terminalFile)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Restore()
	stop := watchTerminalSignals(&localTerminalState{raw: raw, writer: os.Stdout})
	defer stop()
	fmt.Println("READY")
	select {}
}

func TestTerminationSignalsRestoreRawTerminal(t *testing.T) {
	for _, signalTest := range []struct {
		name   string
		signal syscall.Signal
	}{
		{name: "SIGHUP", signal: syscall.SIGHUP},
		{name: "SIGINT", signal: syscall.SIGINT},
		{name: "SIGQUIT", signal: syscall.SIGQUIT},
		{name: "SIGTERM", signal: syscall.SIGTERM},
	} {
		t.Run(signalTest.name, func(t *testing.T) {
			testTerminationSignalRestoresRawTerminal(t, signalTest.signal)
		})
	}
}

func testTerminationSignalRestoresRawTerminal(t *testing.T, terminationSignal syscall.Signal) {
	master, terminalFile, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer terminalFile.Close()
	before := terminalState(t, terminalFile)

	cmd := exec.Command(os.Args[0], "-test.run=TestRawTerminalSignalHelper")
	cmd.Env = append(os.Environ(), "HARNESSRELAY_TEST_RAW_SIGNAL_HELPER=1")
	cmd.ExtraFiles = []*os.File{terminalFile}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(stdout)
	if line, err := reader.ReadString('\n'); err != nil || strings.TrimSpace(line) != "READY" {
		t.Fatalf("helper readiness = %q, %v", line, err)
	}
	during := terminalState(t, terminalFile)
	if during.Lflag&syscall.ICANON != 0 || during.Lflag&syscall.ECHO != 0 {
		t.Fatalf("helper did not enter raw mode: lflag=%#x", during.Lflag)
	}
	if err := cmd.Process.Signal(terminationSignal); err != nil {
		t.Fatal(err)
	}
	cleanupOutput, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatal(readErr)
	}
	err = cmd.Wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("helper exit = %v, want signal exit", err)
	}
	if !bytes.Contains(cleanupOutput, []byte(terminalcleanup.RestoreSequence)) {
		t.Fatalf("signal output missing protocol cleanup: %q", cleanupOutput)
	}
	after := terminalState(t, terminalFile)
	if after != before {
		t.Fatalf("%s left terminal changed\nbefore=%+v\nafter=%+v", terminationSignal, before, after)
	}
}

func TestAttachRestoresTerminalAndReportsDaemonDisconnect(t *testing.T) {
	master, terminalFile, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer terminalFile.Close()
	before := terminalState(t, terminalFile)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/sessions/ses_disconnect/snapshot":
			_ = json.NewEncoder(w).Encode(map[string]any{"latest_seq": 0, "chunks": []any{}})
		case "/api/v1/sessions/ses_disconnect":
			_ = json.NewEncoder(w).Encode(map[string]any{"session": map[string]any{
				"id": "ses_disconnect", "status": "running",
			}})
		case "/api/v1/sessions/ses_disconnect/resize":
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/ws":
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("server does not support hijacking")
			}
			conn, rw, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_, _ = fmt.Fprint(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: test\r\n\r\n")
			_ = rw.Flush()
			_ = conn.Close()
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	c := client{baseURL: server.URL, http: server.Client()}
	var out bytes.Buffer
	err = c.attach("ses_disconnect", terminalFile, &out)
	var disconnect daemonDisconnectedError
	if !errors.As(err, &disconnect) {
		t.Fatalf("attach error = %v, want daemonDisconnectedError", err)
	}
	for _, wanted := range []string{"Restored terminal.", "printf '\\033[<1u", "stty sane", "reset", "harnessctl services restart", "HARNESSRELAY_BYPASS=1"} {
		if !strings.Contains(err.Error(), wanted) {
			t.Fatalf("disconnect error missing %q: %v", wanted, err)
		}
	}
	if !bytes.Contains(out.Bytes(), []byte(terminalcleanup.RestoreSequence)) {
		t.Fatalf("daemon disconnect output missing protocol cleanup: %q", out.Bytes())
	}
	after := terminalState(t, terminalFile)
	if after != before {
		t.Fatalf("daemon disconnect left terminal changed\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestAttachDetachRestoresTerminalProtocols(t *testing.T) {
	server := newOpenAttachServer(t, "ses_detach")
	defer server.Close()

	master, terminalFile, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer terminalFile.Close()
	before := terminalState(t, terminalFile)
	if _, err := master.Write([]byte{0x1d}); err != nil {
		t.Fatal(err)
	}

	c := client{baseURL: server.URL, http: server.Client()}
	var out bytes.Buffer
	if err := c.attach("ses_detach", terminalFile, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte(terminalcleanup.RestoreSequence)) {
		t.Fatalf("detach output missing protocol cleanup: %q", out.Bytes())
	}
	after := terminalState(t, terminalFile)
	if after != before {
		t.Fatalf("detach left termios changed\nbefore=%+v\nafter=%+v", before, after)
	}
}

func newOpenAttachServer(t *testing.T, sessionID string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/sessions/" + sessionID + "/snapshot":
			_ = json.NewEncoder(w).Encode(map[string]any{"latest_seq": 0, "chunks": []any{}})
		case "/api/v1/sessions/" + sessionID:
			_ = json.NewEncoder(w).Encode(map[string]any{"session": map[string]any{
				"id": sessionID, "status": "running",
			}})
		case "/api/v1/sessions/" + sessionID + "/resize":
			w.WriteHeader(http.StatusNoContent)
		case "/api/v1/ws":
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("server does not support hijacking")
			}
			conn, rw, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_, _ = fmt.Fprint(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: test\r\n\r\n")
			_ = rw.Flush()
			_, _ = io.Copy(io.Discard, conn)
			_ = conn.Close()
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
}

func TestShimAttachRemoteStopCleansFakeKittyProtocol(t *testing.T) {
	tests := []struct {
		name string
		path string
		body map[string]any
	}{
		{name: "terminate", path: "terminate", body: map[string]any{"grace_ms": 200}},
		{name: "force_kill", path: "kill", body: map[string]any{"confirmation": "KILL"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testShimAttachRemoteStopCleansFakeKittyProtocol(t, test.path, test.body)
		})
	}
}

func testShimAttachRemoteStopCleansFakeKittyProtocol(t *testing.T, action string, body map[string]any) {
	bus := events.NewBus()
	manager := session.NewManagerWithBus(bus)
	auth := security.NewAuthenticator("test-token")
	server := httptest.NewServer(api.NewRouter(api.Options{
		Sessions: manager,
		Events:   bus,
		Auth:     auth,
	}))
	defer server.Close()

	fakeHarness, err := filepath.Abs(filepath.Join("..", "..", "testdata", "fake-harnesses", "fake-kitty-protocol-harness"))
	if err != nil {
		t.Fatal(err)
	}
	c := client{
		baseURL: server.URL,
		token:   "test-token",
		http:    server.Client(),
	}
	managed, err := c.createShimSession("fake-kitty-protocol-harness", shims.Entry{
		Harness:    "fake-kitty-protocol-harness",
		RealBinary: fakeHarness,
		Backend:    shims.BackendPTY,
	}, nil, 24, 80)
	if err != nil {
		t.Fatal(err)
	}

	master, terminalFile, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer terminalFile.Close()
	before := terminalState(t, terminalFile)

	var out lockedBuffer
	attachDone := make(chan error, 1)
	go func() {
		attachDone <- c.attach(managed.ID, terminalFile, &out)
	}()
	waitForBufferText(t, &out, "FAKE_TUI_READY", 5*time.Second)

	if err := c.request(http.MethodPost, "/api/v1/sessions/"+managed.ID+"/"+action, body, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case attachErr := <-attachDone:
		var exitErr processExitError
		if attachErr != nil && !errors.As(attachErr, &exitErr) {
			t.Fatalf("attach error = %v", attachErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("attach did not exit after remote %s", action)
	}

	output := out.String()
	if !strings.Contains(output, "\x1b[>7u") {
		t.Fatalf("fake harness did not enable Kitty protocol: %q", output)
	}
	if !strings.Contains(output, terminalcleanup.RestoreSequence) {
		t.Fatalf("remote %s did not emit protocol cleanup: %q", action, output)
	}
	if !strings.Contains(output, "HarnessRelay session was terminated remotely. Restored local terminal.") {
		t.Fatalf("remote %s notice missing: %q", action, output)
	}
	after := terminalState(t, terminalFile)
	if after != before {
		t.Fatalf("remote %s left termios changed\nbefore=%+v\nafter=%+v", action, before, after)
	}
	info, ok := manager.Get(managed.ID)
	if !ok || info.Info().Status != session.StatusTerminated {
		t.Fatalf("session status = %v, found=%v", info, ok)
	}
}

type lockedBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

func waitForBufferText(t *testing.T, buffer *lockedBuffer, wanted string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(buffer.String(), wanted) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("buffer did not contain %q: %q", wanted, buffer.String())
}

func terminalState(t *testing.T, file *os.File) syscall.Termios {
	t.Helper()
	var state syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&state)))
	if errno != 0 {
		t.Fatal(errno)
	}
	return state
}

func TestServicesCLILifecycleUsesTemporaryOwnedUnit(t *testing.T) {
	root := t.TempDir()
	daemon := filepath.Join(root, "harnessd")
	if err := os.WriteFile(daemon, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	commandLog := filepath.Join(root, "commands.log")
	systemctl := filepath.Join(root, "systemctl")
	journalctl := filepath.Join(root, "journalctl")
	script := "#!/bin/sh\nprintf '%s %s\\n' \"$(basename \"$0\")\" \"$*\" >> \"" + commandLog + "\"\nexit 0\n"
	for _, path := range []string{systemctl, journalctl} {
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	unitPath := filepath.Join(root, "config", service.UnitName)
	t.Setenv("HARNESSRELAY_DAEMON_BINARY", daemon)
	t.Setenv("HARNESSRELAY_SERVICE_UNIT_PATH", unitPath)
	t.Setenv("HARNESSRELAY_SYSTEMCTL", systemctl)
	t.Setenv("HARNESSRELAY_JOURNALCTL", journalctl)

	var out bytes.Buffer
	for _, verb := range []string{"install", "start", "status", "logs", "restart", "stop", "enable", "disable", "uninstall"} {
		if err := run([]string{"services", verb}, &out, &bytes.Buffer{}); err != nil {
			t.Fatalf("services %s: %v", verb, err)
		}
	}
	if _, err := os.Stat(unitPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("service unit remains after uninstall: %v", err)
	}
	logData, err := os.ReadFile(commandLog)
	if err != nil {
		t.Fatal(err)
	}
	logText := string(logData)
	for _, wanted := range []string{
		"systemctl --user daemon-reload",
		"systemctl --user start " + service.UnitName,
		"systemctl --user status --no-pager " + service.UnitName,
		"journalctl --user --unit " + service.UnitName + " --no-pager",
		"systemctl --user disable --now " + service.UnitName,
	} {
		if !strings.Contains(logText, wanted) {
			t.Fatalf("command log missing %q:\n%s", wanted, logText)
		}
	}
}

func TestRunCommandUsesBearerTokenAndPayload(t *testing.T) {
	var sawAuth bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sessions" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		sawAuth = r.Header.Get("Authorization") == "Bearer secret"
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["name"] != "demo" || body["cwd"] != "/tmp/project" || body["command"] != "/bin/bash" {
			t.Fatalf("unexpected body: %+v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"session": map[string]any{
				"id":      "ses_test",
				"status":  "running",
				"command": "/bin/bash",
			},
		})
	}))
	defer server.Close()
	t.Setenv("HARNESSRELAY_ADDR", server.URL)
	t.Setenv("HARNESSRELAY_TOKEN", "secret")

	var out bytes.Buffer
	err := run([]string{"run", "--name", "demo", "--cwd", "/tmp/project", "/bin/bash", "-l"}, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if !sawAuth {
		t.Fatal("missing bearer auth")
	}
	if !strings.Contains(out.String(), "ses_test") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestCreateShimSessionPreservesArgsCwdEnvAndOrigin(t *testing.T) {
	root := t.TempDir()
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldCWD) }()
	t.Setenv("SHIM_SESSION_ENV", "preserved")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Command       string            `json:"command"`
			Args          []string          `json:"args"`
			CWD           string            `json:"cwd"`
			Env           map[string]string `json:"env"`
			Origin        string            `json:"origin"`
			OriginBackend string            `json:"origin_backend"`
			ShimName      string            `json:"shim_name"`
			RealBinary    string            `json:"real_binary"`
			Attachable    bool              `json:"attachable"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Command != "/opt/fake" || strings.Join(body.Args, ",") != "one,two" ||
			body.CWD != root || body.Env["SHIM_SESSION_ENV"] != "preserved" ||
			body.Origin != "shim" || body.OriginBackend != "pty" ||
			body.ShimName != "fake" || body.RealBinary != "/opt/fake" || !body.Attachable {
			t.Fatalf("shim create body = %+v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"session": map[string]any{"id": "ses_shim", "status": "running", "command": "/opt/fake"}})
	}))
	defer server.Close()
	c := client{baseURL: server.URL, http: server.Client()}
	if _, err := c.createShimSession("fake", shims.Entry{Harness: "fake", RealBinary: "/opt/fake"}, []string{"one", "two"}, 40, 120); err != nil {
		t.Fatal(err)
	}
}

func TestShimsLifecycleCommands(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	shimDir := filepath.Join(root, "shims")
	configPath := filepath.Join(root, "config", "shims.json")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(realDir, "fakeharness")
	if err := os.WriteFile(real, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESSRELAY_SHIMS_CONFIG", configPath)
	t.Setenv("HARNESSRELAY_SHIMS_DIR", shimDir)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+realDir)
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer health.Close()
	t.Setenv("HARNESSRELAY_ADDR", health.URL)

	var out, stderr bytes.Buffer
	if err := run([]string{"shims", "install", "fakeharness"}, &out, &stderr); err != nil {
		t.Fatalf("install: %v", err)
	}
	if !strings.Contains(out.String(), "installed fakeharness") {
		t.Fatalf("install output = %q", out.String())
	}
	out.Reset()
	if err := run([]string{"shims", "list"}, &out, &stderr); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.String(), "fakeharness\tinstalled") {
		t.Fatalf("list output = %q", out.String())
	}
	out.Reset()
	if err := run([]string{"shims", "path"}, &out, &stderr); err != nil {
		t.Fatalf("path: %v", err)
	}
	if strings.TrimSpace(out.String()) != shimDir {
		t.Fatalf("path output = %q", out.String())
	}
	out.Reset()
	if err := run([]string{"shims", "status"}, &out, &stderr); err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out.String(), "fakeharness\tactive\tpty") {
		t.Fatalf("status output = %q", out.String())
	}
	out.Reset()
	if err := run([]string{"shims", "doctor"}, &out, &stderr); err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if !strings.Contains(out.String(), "[ok] fakeharness PATH order") || !strings.Contains(out.String(), "[ok] daemon reachable") {
		t.Fatalf("doctor output = %q", out.String())
	}
	out.Reset()
	if err := run([]string{"shims", "reshim"}, &out, &stderr); err != nil {
		t.Fatalf("reshim: %v", err)
	}
	if err := run([]string{"shims", "uninstall", "fakeharness"}, &out, &stderr); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(shimDir, "fakeharness")); !os.IsNotExist(err) {
		t.Fatalf("shim still exists: %v", err)
	}
	if err := run([]string{"shims", "install", "fakeharness"}, &out, &stderr); err != nil {
		t.Fatalf("reinstall: %v", err)
	}
	if err := run([]string{"shims", "uninstall-all"}, &out, &stderr); err != nil {
		t.Fatalf("uninstall-all: %v", err)
	}
	if _, err := os.Stat(filepath.Join(shimDir, "fakeharness")); !os.IsNotExist(err) {
		t.Fatalf("shim still exists after uninstall-all: %v", err)
	}
}

func TestShimsInstallAllKnownUsesOnlyDetectedTargets(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	shimDir := filepath.Join(root, "shims")
	configPath := filepath.Join(root, "shims.json")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"codex", "opencode", "grok"} {
		if err := os.WriteFile(filepath.Join(realDir, name), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HARNESSRELAY_SHIMS_CONFIG", configPath)
	t.Setenv("HARNESSRELAY_SHIMS_DIR", shimDir)
	t.Setenv("PATH", realDir)
	var out bytes.Buffer
	if err := run([]string{"shims", "install", "--all-known"}, &out, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"codex", "opencode", "grok"} {
		if _, err := os.Stat(filepath.Join(shimDir, name)); err != nil {
			t.Fatalf("%s shim: %v", name, err)
		}
	}
}

func TestShimExecDirectHelper(t *testing.T) {
	if os.Getenv("HARNESSRELAY_TEST_SHIM_EXEC") != "1" {
		return
	}
	if err := run([]string{"shim", "exec", "fakeharness", "--", "one", "two"}, os.Stdout, os.Stderr); err != nil {
		t.Fatal(err)
	}
}

func TestShimExecBypassPreservesArgsCwdEnvAndExitCode(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config", "shims.json")
	shimDir := filepath.Join(root, "shims")
	real := filepath.Join(root, "fakeharness")
	script := "#!/bin/sh\nprintf 'args=%s,%s cwd=%s env=%s\\n' \"$1\" \"$2\" \"$PWD\" \"$SHIM_TEST_ENV\"\nexit 7\n"
	if err := os.WriteFile(real, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := shims.NewConfig(shimDir)
	cfg.Entries["fakeharness"] = shims.Entry{
		Enabled: true, ShimPath: filepath.Join(shimDir, "fakeharness"),
		RealBinary: real, Harness: "fakeharness", Backend: shims.BackendPTY,
		CreatedBy: "harnessrelay",
	}
	if err := shims.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestShimExecDirectHelper")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HARNESSRELAY_TEST_SHIM_EXEC=1",
		"HARNESSRELAY_BYPASS=1",
		"HARNESSRELAY_SHIMS_CONFIG="+configPath,
		"SHIM_TEST_ENV=preserved",
	)
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("exit error = %v, output = %s", err, output)
	}
	want := "args=one,two cwd=" + root + " env=preserved"
	if !strings.Contains(string(output), want) {
		t.Fatalf("output = %q, want %q", string(output), want)
	}
}

func TestShimExecFallsBackToDirectWhenDaemonUnavailable(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "config", "shims.json")
	shimDir := filepath.Join(root, "shims")
	real := filepath.Join(root, "fakeharness")
	if err := os.WriteFile(real, []byte("#!/bin/sh\nprintf 'fallback:%s\\n' \"$1\"\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := shims.NewConfig(shimDir)
	cfg.Entries["fakeharness"] = shims.Entry{
		Enabled: true, ShimPath: filepath.Join(shimDir, "fakeharness"),
		RealBinary: real, Harness: "fakeharness", Backend: shims.BackendPTY,
		CreatedBy: "harnessrelay",
	}
	if err := shims.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestShimExecDirectHelper")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"HARNESSRELAY_TEST_SHIM_EXEC=1",
		"HARNESSRELAY_ADDR=http://127.0.0.1:1",
		"HARNESSRELAY_SHIMS_CONFIG="+configPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fallback error = %v, output = %s", err, output)
	}
	if !strings.Contains(string(output), "daemon unavailable") || !strings.Contains(string(output), "fallback:one") {
		t.Fatalf("fallback output = %q", string(output))
	}
}
