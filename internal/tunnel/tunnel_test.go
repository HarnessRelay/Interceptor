package tunnel

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.Default()
}

// fakeCloudflaredPath returns the repo fixture and a cleanup that restores
// the fake's mode environment variable.
func fakeCloudflaredPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "testdata", "bin", "fake-cloudflared"))
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fake cloudflared missing: %v", err)
	}
	return path
}

// waitFor polls until cond passes or the deadline expires.
func waitFor(t *testing.T, timeout time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", desc)
}

// procAlive reports whether the pid is a live (non-zombie) process.
func procAlive(pid int) bool {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	s := string(data)
	if i := strings.LastIndex(s, ")"); i >= 0 && i+2 < len(s) {
		return s[i+2] != 'Z'
	}
	return false
}

func newTestManager(t *testing.T, ctx context.Context) *Manager {
	t.Helper()
	m := NewManager(ctx, 8765, t.TempDir(), testLogger())
	m.binOverride = fakeCloudflaredPath(t)
	return m
}

func TestTunnelURLPattern(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "standard cloudflared output",
			input: "https://abc-123-def.trycloudflare.com",
			want:  "https://abc-123-def.trycloudflare.com",
		},
		{
			name: "box drawing output",
			input: `+--------------------------------------------------------------------------------------------+
|  Your quick Tunnel has been created! Visit it at (it may take some time to be reachable):  |
|  https://random-name-xyz.trycloudflare.com                                                |
+--------------------------------------------------------------------------------------------+`,
			want: "https://random-name-xyz.trycloudflare.com",
		},
		{
			name:  "embedded in longer line",
			input: "INF Connection registered connIndex=0 ip=198.41.200.73 location=SFO url=https://my-tunnel-abc.trycloudflare.com",
			want:  "https://my-tunnel-abc.trycloudflare.com",
		},
		{
			name:  "no url",
			input: "INF Starting cloudflared",
			want:  "",
		},
		{
			name:  "non-trycloudflare url ignored",
			input: "INF url=https://example.com",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tunnelURLPattern.FindString(tt.input)
			if got != tt.want {
				t.Errorf("FindString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestManagerInfoInitialState(t *testing.T) {
	m := NewManager(context.Background(), 8765, t.TempDir(), testLogger())
	info := m.Info()
	if info.Status != StatusStopped {
		t.Errorf("initial status = %q, want %q", info.Status, StatusStopped)
	}
	if info.URL != "" {
		t.Errorf("initial url = %q, want empty", info.URL)
	}
	if info.Error != "" {
		t.Errorf("initial error = %q, want empty", info.Error)
	}
}

// TestManagerQuickTunnelLifecycle is the regression test for the
// "cloudflared exited: signal: killed" bug: the tunnel process must survive
// after Start returns and only die on Stop or daemon-context cancellation.
func TestManagerQuickTunnelLifecycle(t *testing.T) {
	daemonCtx, daemonCancel := context.WithCancel(context.Background())
	defer daemonCancel()
	m := newTestManager(t, daemonCtx)
	t.Cleanup(func() { _ = m.Stop() })

	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	waitFor(t, 5*time.Second, "tunnel running", func() bool {
		return m.Info().Status == StatusRunning
	})

	info := m.Info()
	if info.URL != "https://fake-tunnel-abc.trycloudflare.com" {
		t.Errorf("url = %q, want fake quick tunnel URL", info.URL)
	}

	pid := m.cmd.Process.Pid
	if !procAlive(pid) {
		t.Fatal("cloudflared process died while tunnel reports running")
	}

	// The process must still be alive well after Start returned; this is the
	// essence of the request-context bug.
	time.Sleep(500 * time.Millisecond)
	if info := m.Info(); info.Status != StatusRunning {
		t.Fatalf("status = %q after 500ms, want running (error: %q)", info.Status, info.Error)
	}
	if !procAlive(pid) {
		t.Fatal("cloudflared process died after Start returned")
	}

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := m.Info().Status; got != StatusStopped {
		t.Errorf("status after Stop = %q, want stopped", got)
	}
	waitFor(t, 5*time.Second, "process exit after Stop", func() bool {
		return !procAlive(pid)
	})

	// Logs must survive Stop for the debug console.
	logs := strings.Join(m.Logs(), "\n")
	if !strings.Contains(logs, "fake-tunnel-abc") {
		t.Errorf("logs after stop do not contain tunnel URL: %q", logs)
	}
}

func TestManagerDaemonContextCancelKillsProcess(t *testing.T) {
	daemonCtx, daemonCancel := context.WithCancel(context.Background())
	m := newTestManager(t, daemonCtx)

	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, 5*time.Second, "tunnel running", func() bool {
		return m.Info().Status == StatusRunning
	})

	pid := m.cmd.Process.Pid
	daemonCancel()
	waitFor(t, 5*time.Second, "process exit after daemon context cancel", func() bool {
		return !procAlive(pid)
	})
}

func TestManagerImmediateExitBecomesError(t *testing.T) {
	t.Setenv("FAKE_CLOUDFLARED_MODE", "fail")
	m := newTestManager(t, context.Background())
	t.Cleanup(func() { _ = m.Stop() })

	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, 5*time.Second, "error status", func() bool {
		return m.Info().Status == StatusError
	})
	if err := m.Info().Error; !strings.Contains(err, "cloudflared exited") {
		t.Errorf("error = %q, want cloudflared exited detail", err)
	}
	if m.Info().URL != "" {
		t.Errorf("url = %q, want empty on error", m.Info().URL)
	}
}

func TestManagerRestartAfterError(t *testing.T) {
	t.Setenv("FAKE_CLOUDFLARED_MODE", "fail")
	m := newTestManager(t, context.Background())
	t.Cleanup(func() { _ = m.Stop() })

	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, 5*time.Second, "error status", func() bool {
		return m.Info().Status == StatusError
	})

	// Retry with a working fake.
	t.Setenv("FAKE_CLOUDFLARED_MODE", "quick")
	if err := m.Start(); err != nil {
		t.Fatalf("Start after error: %v", err)
	}
	waitFor(t, 5*time.Second, "running after retry", func() bool {
		return m.Info().Status == StatusRunning
	})
}

func TestManagerTokenMode(t *testing.T) {
	t.Setenv("FAKE_CLOUDFLARED_MODE", "token")
	daemonCtx, daemonCancel := context.WithCancel(context.Background())
	defer daemonCancel()
	m := newTestManager(t, daemonCtx)
	t.Cleanup(func() { _ = m.Stop() })

	if err := m.UpdateConfig(Config{Mode: ModeToken, Token: "secret-token", Hostname: "https://relay.example.com"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	if got := m.tunnelArgs(); strings.Join(got, " ") != "tunnel --no-autoupdate run --token secret-token" {
		t.Errorf("token mode args = %v", got)
	}

	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, 5*time.Second, "tunnel running", func() bool {
		return m.Info().Status == StatusRunning
	})
	if got := m.Info().URL; got != "https://relay.example.com" {
		t.Errorf("token mode url = %q, want hostname label", got)
	}
}

func TestManagerStartIsIdempotent(t *testing.T) {
	m := newTestManager(t, context.Background())
	t.Cleanup(func() { _ = m.Stop() })

	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, 5*time.Second, "tunnel running", func() bool {
		return m.Info().Status == StatusRunning
	})
	pid := m.cmd.Process.Pid

	if err := m.Start(); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	if m.cmd.Process.Pid != pid {
		t.Errorf("second Start spawned a new process (pid %d, want %d)", m.cmd.Process.Pid, pid)
	}
}

func TestManagerMissingBinary(t *testing.T) {
	m := NewManager(context.Background(), 8765, t.TempDir(), testLogger())
	m.binOverride = "/nonexistent/cloudflared"
	if err := m.Start(); err == nil {
		t.Fatal("Start with missing binary should fail")
	}
	info := m.Info()
	if info.Status != StatusError || info.Error != "cloudflared not found" {
		t.Errorf("info = %+v, want error/cloudflared not found", info)
	}
}

func TestTunnelArgsQuickMode(t *testing.T) {
	m := NewManager(context.Background(), 9123, t.TempDir(), testLogger())
	want := "tunnel --no-autoupdate --url http://127.0.0.1:9123"
	if got := strings.Join(m.tunnelArgs(), " "); got != want {
		t.Errorf("quick mode args = %q, want %q", got, want)
	}
}

func TestConfigValidation(t *testing.T) {
	if err := validateConfig(Config{Mode: ModeQuick}); err != nil {
		t.Errorf("quick mode: %v", err)
	}
	if err := validateConfig(Config{Mode: ModeToken, Token: "t"}); err != nil {
		t.Errorf("token mode with token: %v", err)
	}
	if err := validateConfig(Config{Mode: ModeToken}); err == nil {
		t.Error("token mode without token should fail")
	}
	if err := validateConfig(Config{Mode: "bogus"}); err == nil {
		t.Error("unknown mode should fail")
	}
}

func TestConfigPersistenceAndReload(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(context.Background(), 8765, dir, testLogger())
	if err := m.UpdateConfig(Config{Mode: ModeToken, Token: "tok-123", Hostname: "https://h.example.com"}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}

	// View must never expose the token.
	view := m.ViewConfig()
	if view.TokenSet != true || view.Mode != ModeToken || view.Hostname != "https://h.example.com" {
		t.Errorf("view = %+v", view)
	}

	// A fresh manager must load the same config.
	m2 := NewManager(context.Background(), 8765, dir, testLogger())
	if m2.cfg.Mode != ModeToken || m2.cfg.Token != "tok-123" || m2.cfg.Hostname != "https://h.example.com" {
		t.Errorf("reloaded cfg = %+v", m2.cfg)
	}

	// File must not be world readable.
	info, err := os.Stat(filepath.Join(dir, configFileName))
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("config file mode = %o, want 0600", perm)
	}

	// Keeping the token when an update omits it.
	if err := m2.UpdateConfig(Config{Mode: ModeToken, Hostname: "https://other.example.com"}); err != nil {
		t.Fatalf("UpdateConfig keep token: %v", err)
	}
	if m2.cfg.Token != "tok-123" || m2.cfg.Hostname != "https://other.example.com" {
		t.Errorf("cfg after token-keeping update = %+v", m2.cfg)
	}
}

func TestUpdateConfigWhileActiveRejected(t *testing.T) {
	m := newTestManager(t, context.Background())
	t.Cleanup(func() { _ = m.Stop() })

	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, 5*time.Second, "tunnel running", func() bool {
		return m.Info().Status == StatusRunning
	})

	err := m.UpdateConfig(Config{Mode: ModeQuick})
	if err == nil {
		t.Fatal("UpdateConfig while running should fail")
	}
	if err != ErrTunnelActive {
		t.Errorf("err = %v, want ErrTunnelActive", err)
	}
}

func TestLogBufferRing(t *testing.T) {
	buf := newLogBuffer(3)
	for _, line := range []string{"a", "b", "c", "d", "e"} {
		buf.add(line)
	}
	got := strings.Join(buf.snapshot(), ",")
	if got != "c,d,e" {
		t.Errorf("snapshot = %q, want c,d,e", got)
	}
	buf.reset()
	if len(buf.snapshot()) != 0 {
		t.Error("reset should clear the buffer")
	}
}

func TestDrainParsesURL(t *testing.T) {
	m := NewManager(context.Background(), 8765, t.TempDir(), testLogger())
	m.status = StatusStarting

	lines := "INF Starting cloudflared\nhttps://my-test-tunnel.trycloudflare.com\nINF Some other log\n"
	m.drain(strings.NewReader(lines), ModeQuick, "")

	info := m.Info()
	if info.Status != StatusRunning {
		t.Errorf("status = %q, want %q", info.Status, StatusRunning)
	}
	if info.URL != "https://my-test-tunnel.trycloudflare.com" {
		t.Errorf("url = %q, want https://my-test-tunnel.trycloudflare.com", info.URL)
	}
	logs := m.Logs()
	if len(logs) != 3 {
		t.Errorf("logs = %v, want 3 lines", logs)
	}
}

func TestDrainTokenModeUsesHostname(t *testing.T) {
	m := NewManager(context.Background(), 8765, t.TempDir(), testLogger())
	m.status = StatusStarting

	lines := "INF Starting tunnel\nINF Registered tunnel connection connIndex=0\n"
	m.drain(strings.NewReader(lines), ModeToken, "https://relay.example.com")

	info := m.Info()
	if info.Status != StatusRunning {
		t.Errorf("status = %q, want %q", info.Status, StatusRunning)
	}
	if info.URL != "https://relay.example.com" {
		t.Errorf("url = %q, want hostname label", info.URL)
	}
}
