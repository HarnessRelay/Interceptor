package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/harness"
	"github.com/harnessrelay/interceptor/internal/security"
	"github.com/harnessrelay/interceptor/internal/session"
	"github.com/harnessrelay/interceptor/internal/tunnel"
)

func fakeCloudflared(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "testdata", "bin", "fake-cloudflared"))
	if err != nil {
		t.Fatalf("fixture path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture stat: %v", err)
	}
	return path
}

// newTunnelTestRouter builds a router whose tunnel manager and download API
// endpoint can be steered per test.
func newTunnelTestRouter(t *testing.T, downloadAPI string) (http.Handler, *tunnel.Manager) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := events.NewBus()
	mgr := session.NewManagerWithBus(bus)
	tunnelMgr := tunnel.NewManager(context.Background(), 8765, t.TempDir(), logger)
	router := NewRouter(Options{
		Logger:            logger,
		Version:           "test-version",
		StaticFS:          testStaticFS(),
		Sessions:          mgr,
		Events:            bus,
		Auth:              security.NewAuthenticator(testAuthToken),
		Harnesses:         []harness.Detected{},
		Tunnel:            tunnelMgr,
		TunnelDownloadAPI: downloadAPI,
	})
	t.Cleanup(func() { _ = tunnelMgr.Stop() })
	return router, tunnelMgr
}

func tunnelStatus(t *testing.T, router http.Handler) tunnelResponse {
	t.Helper()
	rec := serveJSON(t, router, http.MethodGet, "/api/v1/tunnel", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("tunnel status = %d: %s", rec.Code, rec.Body.String())
	}
	var resp tunnelResponse
	decodeBody(t, rec, &resp)
	return resp
}

func waitForTunnelStatus(t *testing.T, router http.Handler, want string, timeout time.Duration) tunnelResponse {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp := tunnelStatus(t, router)
		if resp.Status == want {
			return resp
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for tunnel status %q (last: %+v)", want, tunnelStatus(t, router))
	return tunnelResponse{}
}

func TestTunnelEndpointsRequireAuth(t *testing.T) {
	router, _ := newTunnelTestRouter(t, "")
	paths := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/tunnel"},
		{http.MethodGet, "/api/v1/tunnel/available"},
		{http.MethodPost, "/api/v1/tunnel/start"},
		{http.MethodPost, "/api/v1/tunnel/stop"},
		{http.MethodGet, "/api/v1/tunnel/config"},
		{http.MethodPut, "/api/v1/tunnel/config"},
		{http.MethodGet, "/api/v1/tunnel/binary"},
		{http.MethodPost, "/api/v1/tunnel/download"},
		{http.MethodGet, "/api/v1/tunnel/logs"},
	}
	for _, p := range paths {
		req := httptest.NewRequest(p.method, p.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without token = %d, want 401", p.method, p.path, rec.Code)
		}
	}
}

// TestTunnelStartSurvivesRequestLifecycle is the HTTP-level regression test
// for the "cloudflared exited: signal: killed" bug: after POST /tunnel/start
// returns, the tunnel must reach and stay in running state.
func TestTunnelStartSurvivesRequestLifecycle(t *testing.T) {
	t.Setenv("HARNESSRELAY_CLOUDFLARED_BIN", fakeCloudflared(t))
	t.Setenv("FAKE_CLOUDFLARED_MODE", "quick")
	router, _ := newTunnelTestRouter(t, "")

	rec := serveJSON(t, router, http.MethodPost, "/api/v1/tunnel/start", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("start = %d: %s", rec.Code, rec.Body.String())
	}

	resp := waitForTunnelStatus(t, router, "running", 5*time.Second)
	if resp.URL != "https://fake-tunnel-abc.trycloudflare.com" {
		t.Errorf("url = %q, want fake quick tunnel URL", resp.URL)
	}

	// The tunnel must still be running well after the start request finished.
	time.Sleep(500 * time.Millisecond)
	resp = tunnelStatus(t, router)
	if resp.Status != "running" {
		t.Fatalf("status = %q 500ms after start response, want running (error: %q)", resp.Status, resp.Error)
	}

	// Debug logs must contain the tunnel URL line.
	rec = serveJSON(t, router, http.MethodGet, "/api/v1/tunnel/logs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("logs = %d", rec.Code)
	}
	var logs tunnelLogsResponse
	decodeBody(t, rec, &logs)
	if !strings.Contains(strings.Join(logs.Lines, "\n"), "fake-tunnel-abc") {
		t.Errorf("logs missing tunnel URL: %v", logs.Lines)
	}

	rec = serveJSON(t, router, http.MethodPost, "/api/v1/tunnel/stop", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop = %d: %s", rec.Code, rec.Body.String())
	}
	if resp := tunnelStatus(t, router); resp.Status != "stopped" {
		t.Errorf("status after stop = %q, want stopped", resp.Status)
	}
}

func TestTunnelStartMissingBinary(t *testing.T) {
	t.Setenv("HARNESSRELAY_CLOUDFLARED_BIN", "/nonexistent/cloudflared")
	router, _ := newTunnelTestRouter(t, "")

	rec := serveJSON(t, router, http.MethodPost, "/api/v1/tunnel/start", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("start = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "cloudflared not found") {
		t.Errorf("body = %s, want not-found error", rec.Body.String())
	}
}

func TestTunnelConfigEndpoints(t *testing.T) {
	t.Setenv("HARNESSRELAY_CLOUDFLARED_BIN", fakeCloudflared(t))
	router, _ := newTunnelTestRouter(t, "")

	// Defaults.
	rec := serveJSON(t, router, http.MethodGet, "/api/v1/tunnel/config", nil)
	var cfg tunnelConfigResponse
	decodeBody(t, rec, &cfg)
	if cfg.Mode != "quick" || cfg.TokenSet {
		t.Errorf("default config = %+v, want quick/no token", cfg)
	}

	// Invalid mode.
	rec = serveJSON(t, router, http.MethodPut, "/api/v1/tunnel/config", map[string]any{"mode": "bogus"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bogus mode = %d, want 400", rec.Code)
	}

	// Token mode without a token.
	rec = serveJSON(t, router, http.MethodPut, "/api/v1/tunnel/config", map[string]any{"mode": "token"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("token mode without token = %d, want 400", rec.Code)
	}

	// Token mode with a token; the token must never be echoed back.
	rec = serveJSON(t, router, http.MethodPut, "/api/v1/tunnel/config",
		map[string]any{"mode": "token", "token": "secret-tunnel-token", "hostname": "https://relay.example.com"})
	if rec.Code != http.StatusOK {
		t.Fatalf("token config = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret-tunnel-token") {
		t.Error("config response must not contain the token")
	}
	decodeBody(t, rec, &cfg)
	if cfg.Mode != "token" || !cfg.TokenSet || cfg.Hostname != "https://relay.example.com" {
		t.Errorf("config = %+v", cfg)
	}

	// Switching back to quick keeps the stored token but stops requiring it.
	rec = serveJSON(t, router, http.MethodPut, "/api/v1/tunnel/config", map[string]any{"mode": "quick"})
	if rec.Code != http.StatusOK {
		t.Fatalf("back to quick = %d", rec.Code)
	}
	decodeBody(t, rec, &cfg)
	if cfg.Mode != "quick" || !cfg.TokenSet {
		t.Errorf("config after quick switch = %+v, want quick with token kept", cfg)
	}
}

func TestTunnelConfigRejectedWhileRunning(t *testing.T) {
	t.Setenv("HARNESSRELAY_CLOUDFLARED_BIN", fakeCloudflared(t))
	t.Setenv("FAKE_CLOUDFLARED_MODE", "quick")
	router, _ := newTunnelTestRouter(t, "")

	rec := serveJSON(t, router, http.MethodPost, "/api/v1/tunnel/start", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("start = %d", rec.Code)
	}
	waitForTunnelStatus(t, router, "running", 5*time.Second)

	rec = serveJSON(t, router, http.MethodPut, "/api/v1/tunnel/config", map[string]any{"mode": "quick"})
	if rec.Code != http.StatusConflict {
		t.Errorf("config while running = %d, want 409", rec.Code)
	}
}

func TestTunnelBinaryEndpoint(t *testing.T) {
	t.Setenv("HARNESSRELAY_CLOUDFLARED_BIN", fakeCloudflared(t))
	router, _ := newTunnelTestRouter(t, "")

	rec := serveJSON(t, router, http.MethodGet, "/api/v1/tunnel/binary", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("binary = %d", rec.Code)
	}
	var resp tunnelBinaryResponse
	decodeBody(t, rec, &resp)
	if resp.Path == "" || resp.Source != "env" {
		t.Errorf("binary = %+v, want env override", resp)
	}
	if !strings.Contains(resp.Version, "2099.9.9") {
		t.Errorf("version = %q, want fake version", resp.Version)
	}
	if resp.ManagedPath == "" {
		t.Error("managed_path must always be reported")
	}
}

func TestTunnelDownloadEndpoint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HARNESSRELAY_CLOUDFLARED_BIN_DIR", dir)

	asset := "#!/bin/sh\n[ \"$1\" = \"--version\" ] && echo \"cloudflared version 2099.3.4 (fake)\" && exit 0\nexit 1\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v2099.3.4",
				"assets": []map[string]string{{
					"name":                 fmt.Sprintf("cloudflared-linux-%s", runtime.GOARCH),
					"browser_download_url": fmt.Sprintf("http://%s/asset", r.Host),
				}},
			})
		case strings.HasSuffix(r.URL.Path, "/asset"):
			_, _ = w.Write([]byte(asset))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	// Endpoint failures must not install anything; try a dead API first.
	badRouter, _ := newTunnelTestRouter(t, "http://127.0.0.1:1/releases/latest")
	rec := serveJSON(t, badRouter, http.MethodPost, "/api/v1/tunnel/download", nil)
	if rec.Code != http.StatusBadGateway {
		t.Errorf("download from dead API = %d, want 502", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, "cloudflared")); !os.IsNotExist(err) {
		t.Error("failed download must not install a binary")
	}

	goodRouter, _ := newTunnelTestRouter(t, srv.URL+"/releases/latest")
	rec = serveJSON(t, goodRouter, http.MethodPost, "/api/v1/tunnel/download", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("download = %d: %s", rec.Code, rec.Body.String())
	}
	var resp tunnelDownloadResponse
	decodeBody(t, rec, &resp)
	if resp.Version != "2099.3.4" || resp.Path != filepath.Join(dir, "cloudflared") {
		t.Errorf("download = %+v", resp)
	}
}
