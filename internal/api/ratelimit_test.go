package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRateLimiterFixedWindow(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	limiter := newRateLimiter(time.Minute, 3)
	limiter.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if !limiter.allow("1.2.3.4") {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if limiter.allow("1.2.3.4") {
		t.Fatal("4th attempt within window should be blocked")
	}
	// Other keys are unaffected.
	if !limiter.allow("5.6.7.8") {
		t.Fatal("different key should be allowed")
	}

	// After the window passes, the key is allowed again.
	now = now.Add(61 * time.Second)
	if !limiter.allow("1.2.3.4") {
		t.Fatal("key should be allowed after window expiry")
	}
}

func TestLoginRateLimited(t *testing.T) {
	router, _, _ := newTestRouter()

	for i := 0; i < loginRateMax; i++ {
		rec := serveJSON(t, router, http.MethodPost, "/api/v1/auth/login", map[string]string{"token": "wrong"})
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d hit rate limit too early", i+1)
		}
	}

	rec := serveJSON(t, router, http.MethodPost, "/api/v1/auth/login", map[string]string{"token": testAuthToken})
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("login after max attempts = %d, want 429", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "too many") {
		t.Errorf("body = %s, want rate-limit message", rec.Body.String())
	}
}

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.10:54321"
	if got := clientIP(req); got != "192.168.1.10" {
		t.Errorf("clientIP = %q, want 192.168.1.10", got)
	}
}
