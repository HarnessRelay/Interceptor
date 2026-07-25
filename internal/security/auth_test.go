package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticatorBearer(t *testing.T) {
	auth := NewAuthenticator("secret")
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	if _, err := auth.Authenticate(req); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	if _, err := auth.Authenticate(req); err == nil {
		t.Fatal("wrong bearer token authenticated")
	}
}

func TestAuthenticatorCookieCSRF(t *testing.T) {
	auth := NewAuthenticator("secret")
	rec := httptest.NewRecorder()
	principal, ok := auth.Login(rec, "secret")
	if !ok {
		t.Fatal("Login failed")
	}

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	for _, cookie := range rec.Result().Cookies() {
		req.AddCookie(cookie)
	}
	if _, err := auth.Authorize(req); err != ErrForbidden {
		t.Fatalf("Authorize without CSRF = %v, want forbidden", err)
	}

	req.Header.Set(CSRFHeaderName, principal.CSRFToken)
	if _, err := auth.Authorize(req); err != nil {
		t.Fatalf("Authorize with CSRF: %v", err)
	}
}

func TestSameOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8765/api", nil)
	req.Host = "127.0.0.1:8765"
	req.Header.Set("Origin", "http://127.0.0.1:8765")
	if !SameOrigin(req) {
		t.Fatal("same origin rejected")
	}
	req.Header.Set("Origin", "http://evil.example")
	if SameOrigin(req) {
		t.Fatal("unexpected origin accepted")
	}
}

func TestIsLocalBind(t *testing.T) {
	if !IsLocalBind("127.0.0.1:8765") {
		t.Fatal("localhost bind rejected")
	}
	if IsLocalBind("0.0.0.0:8765") {
		t.Fatal("public bind accepted as local")
	}
}

func TestRedactSecret(t *testing.T) {
	if got := RedactSecret("OPENAI_API_KEY", "secret-value"); got != RedactedValue {
		t.Fatalf("RedactSecret API key = %q", got)
	}
	if got := RedactSecret("normal_value", "visible"); got != "visible" {
		t.Fatalf("RedactSecret normal value = %q", got)
	}
}
