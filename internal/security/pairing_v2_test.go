package security

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestStoreAndManager(t *testing.T) (*PairedDeviceStore, *PairingManager) {
	t.Helper()
	store, err := NewPairedDeviceStore(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	manager := NewPairingManager(store)
	t.Cleanup(manager.Stop)
	return store, manager
}

func TestSubmitRequestGeneratesSixDigitCode(t *testing.T) {
	_, manager := newTestStoreAndManager(t)

	msg, ok := manager.SubmitRequest(PairingRequest{DeviceID: "dev-1", DeviceName: "Pixel", PublicKey: "a2V5"})
	if !ok || msg != "" {
		t.Fatalf("submit: %q %v", msg, ok)
	}

	code := manager.PendingCode("dev-1")
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		t.Errorf("code = %q, want 6 digits", code)
	}

	pending := manager.PendingRequests()
	if len(pending) != 1 || pending[0].Code != code || pending[0].Type != DeviceTypeMobile {
		t.Errorf("pending = %+v", pending)
	}

	// Resubmission keeps the same code so both screens stay in sync.
	_, ok = manager.SubmitRequest(PairingRequest{DeviceID: "dev-1", DeviceName: "Pixel", PublicKey: "a2V5"})
	if !ok {
		t.Fatal("resubmit rejected")
	}
	if manager.PendingCode("dev-1") != code {
		t.Errorf("code changed on resubmit: %q vs %q", manager.PendingCode("dev-1"), code)
	}
}

func TestGenerateCodeDistribution(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code := GenerateCode()
		if len(code) != 6 {
			t.Fatalf("code %q is not 6 digits", code)
		}
		seen[code] = true
	}
	if len(seen) < 50 {
		t.Errorf("codes lack entropy: %d unique in 100", len(seen))
	}
}

func TestWebPairingFlow(t *testing.T) {
	store, manager := newTestStoreAndManager(t)

	requestID, code, secret, errMsg := manager.SubmitWebRequest("Laptop Browser")
	if errMsg != "" || requestID == "" || len(code) != 6 || secret == "" {
		t.Fatalf("web submit: %q %q %q %q", requestID, code, secret, errMsg)
	}

	// Pending request is visible to the dashboard with its code and type.
	pending := manager.PendingRequests()
	found := false
	for _, req := range pending {
		if req.DeviceID != "" && req.Type == DeviceTypeWeb && req.Code == code && req.DeviceName == "Laptop Browser" {
			found = true
		}
	}
	if !found {
		t.Fatalf("web request missing from pending list: %+v", pending)
	}

	// Polling before acceptance returns pending without a token.
	status, token := manager.PollWebRequest(requestID, secret)
	if status != PairingStatusPending || token != "" {
		t.Fatalf("poll before accept = %q %q", status, token)
	}

	// Wrong secret cannot poll.
	if status, _ := manager.PollWebRequest(requestID, "wrong"); status != PairingStatusUnknown {
		t.Fatalf("poll with wrong secret = %q, want unknown", status)
	}

	// Accept mints a device token, claimable exactly once.
	deviceID := ""
	for _, req := range pending {
		if req.Type == DeviceTypeWeb {
			deviceID = req.DeviceID
		}
	}
	if !manager.Accept(deviceID) {
		t.Fatal("accept failed")
	}

	status, token = manager.PollWebRequest(requestID, secret)
	if status != PairingStatusAccepted || !strings.HasPrefix(token, DeviceTokenPrefix) {
		t.Fatalf("claim = %q %q", status, token)
	}
	status, token2 := manager.PollWebRequest(requestID, secret)
	if status != PairingStatusAccepted || token2 != "" {
		t.Fatalf("second claim = %q %q, want accepted with no token", status, token2)
	}

	// The device authenticates with the token hash.
	dev, ok := store.FindByTokenHash(HashDeviceToken(token))
	if !ok || dev.Type != DeviceTypeWeb || dev.DeviceName != "Laptop Browser" {
		t.Fatalf("stored device = %+v ok=%v", dev, ok)
	}
}

func TestWebPairingReject(t *testing.T) {
	_, manager := newTestStoreAndManager(t)

	requestID, _, secret, errMsg := manager.SubmitWebRequest("Sneaky")
	if errMsg != "" {
		t.Fatalf("submit: %q", errMsg)
	}
	deviceID := manager.PendingRequests()[0].DeviceID

	if !manager.Reject(deviceID) {
		t.Fatal("reject failed")
	}
	status, token := manager.PollWebRequest(requestID, secret)
	if status != PairingStatusRejected || token != "" {
		t.Fatalf("poll after reject = %q %q", status, token)
	}
}

func TestWebPairingExpiry(t *testing.T) {
	_, manager := newTestStoreAndManager(t)

	requestID, _, secret, _ := manager.SubmitWebRequest("Old")
	req := manager.PendingRequests()[0]
	manager.mu.Lock()
	old := manager.pending[req.DeviceID]
	old.ReceivedAt = time.Now().Add(-pairingRequestTTL - time.Minute)
	manager.pending[req.DeviceID] = old
	manager.mu.Unlock()

	manager.cleanup()
	status, _ := manager.PollWebRequest(requestID, secret)
	if status != PairingStatusExpired {
		t.Fatalf("status after TTL = %q, want expired", status)
	}
}

func TestDeviceTokenAuthenticationAndPolicy(t *testing.T) {
	store, manager := newTestStoreAndManager(t)
	auth := NewAuthenticator("master-token")
	auth.SetPairedDeviceStore(store)

	requestID, _, secret, _ := manager.SubmitWebRequest("Tablet")
	deviceID := manager.PendingRequests()[0].DeviceID
	if !manager.Accept(deviceID) {
		t.Fatal("accept failed")
	}
	_, token := manager.PollWebRequest(requestID, secret)

	lanReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	lanReq.RemoteAddr = "192.168.1.77:4444"
	lanReq.Header.Set("Authorization", "Bearer "+token)
	principal, err := auth.Authenticate(lanReq)
	if err != nil {
		t.Fatalf("device token from LAN: %v", err)
	}
	if principal.DeviceID != deviceID || principal.Master {
		t.Errorf("principal = %+v", principal)
	}
	if _, err := auth.AuthorizeRequest(lanReq); err != nil {
		t.Errorf("device token from LAN should authorize: %v", err)
	}

	// Master token is rejected from LAN but allowed from the host.
	masterReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	masterReq.RemoteAddr = "192.168.1.77:4444"
	masterReq.Header.Set("Authorization", "Bearer master-token")
	if _, err := auth.AuthorizeRequest(masterReq); err != ErrForbidden {
		t.Errorf("master from LAN = %v, want ErrForbidden", err)
	}
	hostReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	hostReq.RemoteAddr = "127.0.0.1:4444"
	hostReq.Header.Set("Authorization", "Bearer master-token")
	if _, err := auth.AuthorizeRequest(hostReq); err != nil {
		t.Errorf("master from host = %v, want nil", err)
	}

	// Host login cookie is rejected off-host, device cookie is not.
	loginRec := httptest.NewRecorder()
	principal, ok := auth.Login(loginRec, "master-token")
	if !ok {
		t.Fatal("login failed")
	}
	cookie := loginRec.Result().Cookies()[0]

	hostCookieReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	hostCookieReq.RemoteAddr = "127.0.0.1:4444"
	hostCookieReq.AddCookie(cookie)
	if _, err := auth.AuthorizeRequest(hostCookieReq); err != nil {
		t.Errorf("host cookie from host = %v, want nil", err)
	}
	lanCookieReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	lanCookieReq.RemoteAddr = "192.168.1.77:4444"
	lanCookieReq.AddCookie(cookie)
	if _, err := auth.AuthorizeRequest(lanCookieReq); err != ErrForbidden {
		t.Errorf("host cookie from LAN = %v, want ErrForbidden", err)
	}

	deviceRec := httptest.NewRecorder()
	auth.LoginDevice(deviceRec, Principal{Actor: deviceID, DeviceID: deviceID})
	deviceCookie := deviceRec.Result().Cookies()[0]
	lanDeviceReq := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	lanDeviceReq.RemoteAddr = "192.168.1.77:4444"
	lanDeviceReq.AddCookie(deviceCookie)
	got, err := auth.AuthorizeRequest(lanDeviceReq)
	if err != nil {
		t.Fatalf("device cookie from LAN = %v, want nil", err)
	}
	if got.DeviceID != deviceID || got.Master {
		t.Errorf("device principal = %+v", got)
	}
}

func TestIPFilterLifecycle(t *testing.T) {
	dir := t.TempDir()
	allowPath := dir + "/allowed_ips.txt"
	banPath := dir + "/banned_ips.txt"

	filter, err := NewIPFilter(allowPath, banPath, nil)
	if err != nil {
		t.Fatalf("filter: %v", err)
	}

	if !filter.AllowedByAllowlist(net.ParseIP("192.168.1.5")) {
		t.Error("empty allowlist must allow everything")
	}

	if err := filter.Allow("192.168.1.0/24"); err != nil {
		t.Fatalf("allow: %v", err)
	}
	if !filter.AllowedByAllowlist(net.ParseIP("192.168.1.5")) {
		t.Error("CIDR member should be allowed")
	}
	if filter.AllowedByAllowlist(net.ParseIP("10.0.0.1")) {
		t.Error("non-member should be blocked")
	}

	if err := filter.Ban("10.0.0.9"); err != nil {
		t.Fatalf("ban: %v", err)
	}
	if !filter.Banned(net.ParseIP("10.0.0.9")) {
		t.Error("banned IP should be banned")
	}
	if filter.Banned(net.ParseIP("10.0.0.8")) {
		t.Error("unrelated IP should not be banned")
	}

	// Invalid entries are rejected.
	if err := filter.Allow("not-an-ip"); err == nil {
		t.Error("invalid entry accepted")
	}

	// Removal persists and reload survives restart.
	if err := filter.Unallow("192.168.1.0/24"); err != nil {
		t.Fatalf("unallow: %v", err)
	}
	reloaded, err := NewIPFilter(allowPath, banPath, nil)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded.AllowedByAllowlist(net.ParseIP("192.168.1.5")) {
		t.Error("allowlist should be empty after removal")
	}
	if !reloaded.Banned(net.ParseIP("10.0.0.9")) {
		t.Error("banlist must survive reload")
	}
}

func TestRemoteSettingsPersistence(t *testing.T) {
	dir := t.TempDir()
	store, err := NewRemoteSettingsStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if !store.Get().RemoteAccessEnabled {
		t.Error("remote access must default to enabled")
	}
	if err := store.Set(RemoteSettings{RemoteAccessEnabled: false}); err != nil {
		t.Fatalf("set: %v", err)
	}
	reloaded, err := NewRemoteSettingsStore(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Get().RemoteAccessEnabled {
		t.Error("disabled setting must survive reload")
	}
}

func TestKnownClientRenames(t *testing.T) {
	store, err := NewKnownClientStore(t.TempDir())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := store.Rename("AA:BB:CC:DD:EE:FF", "Nethun's Laptop"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got := store.Name("AA:BB:CC:DD:EE:FF"); got != "Nethun's Laptop" {
		t.Errorf("name = %q", got)
	}
	// Reset to default.
	if err := store.Rename("AA:BB:CC:DD:EE:FF", ""); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if got := store.Name("AA:BB:CC:DD:EE:FF"); got != "" {
		t.Errorf("name after reset = %q", got)
	}
}
