package security

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClassifyClient(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		cfHeader   string
		wantClass  ClientClass
		wantRealIP string
	}{
		{
			name:       "loopback is host",
			remoteAddr: "127.0.0.1:5000",
			wantClass:  ClientClassHost,
			wantRealIP: "127.0.0.1",
		},
		{
			name:       "ipv6 loopback is host",
			remoteAddr: "[::1]:5000",
			wantClass:  ClientClassHost,
			wantRealIP: "::1",
		},
		{
			name:       "lan client",
			remoteAddr: "192.168.1.42:5000",
			wantClass:  ClientClassLAN,
			wantRealIP: "192.168.1.42",
		},
		{
			name:       "loopback with cf header is tunnel",
			remoteAddr: "127.0.0.1:5000",
			cfHeader:   "203.0.113.50",
			wantClass:  ClientClassTunnel,
			wantRealIP: "203.0.113.50",
		},
		{
			name:       "spoofed cf header from lan stays lan",
			remoteAddr: "192.168.1.42:5000",
			cfHeader:   "203.0.113.50",
			wantClass:  ClientClassLAN,
			wantRealIP: "192.168.1.42",
		},
		{
			name:       "garbage cf header ignored",
			remoteAddr: "127.0.0.1:5000",
			cfHeader:   "not-an-ip",
			wantClass:  ClientClassHost,
			wantRealIP: "127.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.cfHeader != "" {
				req.Header.Set(CFConnectingIPHeader, tt.cfHeader)
			}
			info := ClassifyClient(req)
			if info.Class != tt.wantClass {
				t.Errorf("class = %q, want %q", info.Class, tt.wantClass)
			}
			if got := info.Key(); got != tt.wantRealIP {
				t.Errorf("real ip = %q, want %q", got, tt.wantRealIP)
			}
		})
	}
}
