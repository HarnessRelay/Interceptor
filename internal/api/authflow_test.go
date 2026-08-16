package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/harness"
	"github.com/harnessrelay/interceptor/internal/security"
	"github.com/harnessrelay/interceptor/internal/session"
)

// newAuthFlowRouter builds a router with the full pairing/device/network
// stack against temp directories.
func newAuthFlowRouter(t *testing.T) (http.Handler, *security.PairingManager, *security.PairedDeviceStore) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dir := t.TempDir()

	store, err := security.NewPairedDeviceStore(dir)
	if err != nil {
		t.Fatalf("device store: %v", err)
	}
	pairing := security.NewPairingManager(store)
	t.Cleanup(pairing.Stop)

	filter, err := security.NewIPFilter(filepath.Join(dir, "allowed_ips.txt"), filepath.Join(dir, "banned_ips.txt"), nil)
	if err != nil {
		t.Fatalf("ip filter: %v", err)
	}
	settings, err := security.NewRemoteSettingsStore(dir)
	if err != nil {
		t.Fatalf("settings: %v", err)
	}
	known, err := security.NewKnownClientStore(dir)
	if err != nil {
		t.Fatalf("known clients: %v", err)
	}

	auth := security.NewAuthenticator(testAuthToken)
	auth.SetPairedDeviceStore(store)

	bus := events.NewBus()
	router := NewRouter(Options{
		Logger:         logger,
		Version:        "test-version",
		StaticFS:       testStaticFS(),
		Sessions:       session.NewManagerWithBus(bus),
		Events:         bus,
		Auth:           auth,
		Harnesses:      []harness.Detected{},
		Pairing:        pairing,
		Devices:        store,
		IPFilter:       filter,
		RemoteSettings: settings,
		KnownClients:   known,
	})
	return router, pairing, store
}

// lanRequest builds a request from a LAN address, optionally with a bearer
// token (device token or master token).
func lanRequest(t *testing.T, router http.Handler, method, path string, body any, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = strings.NewReader(string(data))
	}
	req := httptest.NewRequest(method, path, reader)
	req.RemoteAddr = "192.168.1.50:40000"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestRemoteLoginFlow drives the full remote web-device pairing over HTTP:
// request access → code shown → host accepts → token claimed once →
// device session minted → API access works from the LAN client.
func TestRemoteLoginFlow(t *testing.T) {
	router, pairing, _ := newAuthFlowRouter(t)

	// Remote client cannot log in with the static token.
	rec := lanRequest(t, router, http.MethodPost, "/api/v1/auth/login", map[string]string{"token": testAuthToken}, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("remote token login = %d, want 403 (body: %s)", rec.Code, rec.Body.String())
	}

	// auth/status tells the client which flow to use.
	rec = lanRequest(t, router, http.MethodGet, "/api/v1/auth/status", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("auth status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"client_class":"lan"`) ||
		!strings.Contains(rec.Body.String(), `"token_login_allowed":false`) {
		t.Fatalf("auth status body = %s", rec.Body.String())
	}

	// Remote client requests access; gets a code and poll secret.
	rec = lanRequest(t, router, http.MethodPost, "/api/v1/pairing/web", map[string]string{"device_name": "Kitchen Tablet"}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("web pairing submit = %d: %s", rec.Code, rec.Body.String())
	}
	var submit struct {
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
		Secret    string `json:"secret"`
	}
	decodeBody(t, rec, &submit)
	if len(submit.Code) != 6 || submit.Secret == "" || submit.RequestID == "" {
		t.Fatalf("submit = %+v", submit)
	}

	// The pending request appears on the dashboard with the same code.
	rec = serveJSON(t, router, http.MethodGet, "/api/v1/pairing/requests", nil)
	if !strings.Contains(rec.Body.String(), submit.Code) {
		t.Fatalf("dashboard pending list missing code %q: %s", submit.Code, rec.Body.String())
	}
	var list struct {
		Requests []security.PairingRequest `json:"requests"`
	}
	decodeBody(t, rec, &list)
	deviceID := ""
	for _, req := range list.Requests {
		if req.Code == submit.Code {
			deviceID = req.DeviceID
		}
	}
	if deviceID == "" {
		t.Fatal("pending web request not found")
	}

	// Polling before acceptance returns pending.
	pollRec := func(secret string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/pairing/web/"+submit.RequestID, nil)
		req.RemoteAddr = "192.168.1.50:40000"
		req.Header.Set("X-Pairing-Secret", secret)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	rec = pollRec(submit.Secret)
	if !strings.Contains(rec.Body.String(), `"status":"pending"`) {
		t.Fatalf("poll before accept = %s", rec.Body.String())
	}

	// Wrong secret gets 404.
	if rec := pollRec("wrong-secret"); rec.Code != http.StatusNotFound {
		t.Fatalf("poll wrong secret = %d, want 404", rec.Code)
	}

	// Host accepts the request.
	rec = serveJSON(t, router, http.MethodPost, "/api/v1/pairing/accept", map[string]string{"device_id": deviceID})
	if rec.Code != http.StatusOK {
		t.Fatalf("accept = %d: %s", rec.Code, rec.Body.String())
	}

	// Token delivered exactly once.
	rec = pollRec(submit.Secret)
	var claim struct {
		Status      string `json:"status"`
		DeviceToken string `json:"device_token"`
	}
	decodeBody(t, rec, &claim)
	if claim.Status != "accepted" || !strings.HasPrefix(claim.DeviceToken, "hrk_") {
		t.Fatalf("claim = %+v (%s)", claim, rec.Body.String())
	}
	rec = pollRec(submit.Secret)
	if strings.Contains(rec.Body.String(), "hrk_") {
		t.Fatalf("token delivered twice: %s", rec.Body.String())
	}

	// The device token authorizes API access from the LAN client...
	rec = lanRequest(t, router, http.MethodGet, "/api/v1/sessions", nil, claim.DeviceToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("device token access = %d: %s", rec.Code, rec.Body.String())
	}

	// ...and can mint a cookie session for WebSocket compatibility.
	rec = lanRequest(t, router, http.MethodPost, "/api/v1/auth/device-session", nil, claim.DeviceToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("device session = %d: %s", rec.Code, rec.Body.String())
	}
	var sessionResp authStatusResponse
	decodeBody(t, rec, &sessionResp)
	if !sessionResp.Authenticated || sessionResp.CSRFToken == "" {
		t.Fatalf("device session response = %+v", sessionResp)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("device session did not set a cookie")
	}

	// Cookie session works from the remote client for safe methods.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.RemoteAddr = "192.168.1.50:40000"
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusOK {
		t.Fatalf("device cookie access = %d: %s", rec2.Code, rec2.Body.String())
	}

	_ = pairing
}

func TestDeviceRenameEndpoint(t *testing.T) {
	router, _, store := newAuthFlowRouter(t)
	if err := store.Add(security.PairedDevice{DeviceID: "dev-9", DeviceName: "Phone", Platform: "android"}); err != nil {
		t.Fatal(err)
	}

	rec := serveJSON(t, router, http.MethodPut, "/api/v1/pairing/devices/dev-9/name", map[string]string{"name": "My Phone"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("rename = %d: %s", rec.Code, rec.Body.String())
	}
	dev, _ := store.Get("dev-9")
	if dev.DisplayName() != "My Phone" {
		t.Fatalf("display name = %q", dev.DisplayName())
	}

	// Reset to default.
	rec = serveJSON(t, router, http.MethodPut, "/api/v1/pairing/devices/dev-9/name", map[string]string{"name": ""})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("reset = %d", rec.Code)
	}
	dev, _ = store.Get("dev-9")
	if dev.DisplayName() != "Phone" {
		t.Fatalf("display name after reset = %q", dev.DisplayName())
	}
}

func TestNetworkSettingsAndLists(t *testing.T) {
	router, _, _ := newAuthFlowRouter(t)

	rec := serveJSON(t, router, http.MethodGet, "/api/v1/network/settings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("settings = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"remote_access_enabled":true`) {
		t.Fatalf("default settings = %s", rec.Body.String())
	}

	disable := false
	rec = serveJSON(t, router, http.MethodPut, "/api/v1/network/settings", map[string]any{"remote_access_enabled": disable})
	if rec.Code != http.StatusOK {
		t.Fatalf("toggle = %d: %s", rec.Code, rec.Body.String())
	}
	enable := true
	rec = serveJSON(t, router, http.MethodPut, "/api/v1/network/settings", map[string]any{"remote_access_enabled": enable})
	if rec.Code != http.StatusOK {
		t.Fatalf("re-enable = %d", rec.Code)
	}

	rec = serveJSON(t, router, http.MethodPost, "/api/v1/network/allow", map[string]string{"entry": "192.168.1.0/24"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "192.168.1.0/24") {
		t.Fatalf("allow add = %d: %s", rec.Code, rec.Body.String())
	}
	rec = serveJSON(t, router, http.MethodPost, "/api/v1/network/ban", map[string]string{"entry": "10.9.9.9"})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "10.9.9.9") {
		t.Fatalf("ban add = %d: %s", rec.Code, rec.Body.String())
	}
	rec = serveJSON(t, router, http.MethodPost, "/api/v1/network/allow", map[string]string{"entry": "garbage"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid entry = %d, want 400", rec.Code)
	}
	rec = serveJSON(t, router, http.MethodDelete, "/api/v1/network/ban", map[string]string{"entry": "10.9.9.9"})
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "10.9.9.9") {
		t.Fatalf("ban remove = %d: %s", rec.Code, rec.Body.String())
	}

	// Clients endpoint lists the LAN client that hit auth/status earlier.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/status", nil)
	req.RemoteAddr = "192.168.1.50:40000"
	router.ServeHTTP(httptest.NewRecorder(), req)
	rec = serveJSON(t, router, http.MethodGet, "/api/v1/network/clients", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "192.168.1.50") {
		t.Fatalf("clients = %d: %s", rec.Code, rec.Body.String())
	}

	// Rename a client using the stable key the clients endpoint exposes
	// (MAC when known, IP otherwise).
	var clients struct {
		Clients []struct {
			Key string `json:"key"`
			IP  string `json:"ip"`
		} `json:"clients"`
	}
	decodeBody(t, rec, &clients)
	clientKey := ""
	for _, c := range clients.Clients {
		if c.IP == "192.168.1.50" {
			clientKey = c.Key
		}
	}
	if clientKey == "" {
		t.Fatalf("lan client missing from list: %s", rec.Body.String())
	}
	rec = serveJSON(t, router, http.MethodPut, "/api/v1/network/clients/"+clientKey+"/name", map[string]string{"name": "Tablet"})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("client rename = %d: %s", rec.Code, rec.Body.String())
	}
	rec = serveJSON(t, router, http.MethodGet, "/api/v1/network/clients", nil)
	if !strings.Contains(rec.Body.String(), "Tablet") {
		t.Fatalf("custom name missing: %s", rec.Body.String())
	}
}

func TestParseARPTable(t *testing.T) {
	data := `IP address       HW type     Flags       HW address            Mask     Device
192.168.1.20      0x1         0x2         aa:bb:cc:dd:ee:ff     *        wlp3s0
192.168.1.21      0x1         0x2         11:22:33:44:55:66     *        eth0
10.0.0.1          0x1         0x0         (incomplete)          *        eth0
`
	got := parseARPTable(data)
	if len(got) != 2 {
		t.Fatalf("parsed %d entries, want 2: %v", len(got), got)
	}
	if got["192.168.1.20"] != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("mac = %q, want upper-cased", got["192.168.1.20"])
	}
	if got["192.168.1.21"] != "11:22:33:44:55:66" {
		t.Errorf("mac = %q", got["192.168.1.21"])
	}
}
