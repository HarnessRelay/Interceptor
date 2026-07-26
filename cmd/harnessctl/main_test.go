package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harnessrelay/interceptor/internal/shims"
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
	for _, want := range []string{"daemon: reachable", "configured address:", "token source:", "active binary:", "PATH binary:", "config:", "shim path:"} {
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
	if out.String() != "fast-output\r\n" {
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
