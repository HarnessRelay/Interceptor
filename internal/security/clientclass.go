package security

import (
	"net"
	"net/http"
)

// ClientClass describes what kind of client made a request.
type ClientClass string

const (
	// ClientClassHost is the daemon's own machine (loopback, not via tunnel).
	ClientClassHost ClientClass = "host"
	// ClientClassLAN is a directly connected non-loopback client.
	ClientClassLAN ClientClass = "lan"
	// ClientClassTunnel is a remote client arriving through the local
	// cloudflared process, whose real IP is in CF-Connecting-IP.
	ClientClassTunnel ClientClass = "tunnel"
)

// CFConnectingIPHeader is set by the Cloudflare edge on tunneled requests.
const CFConnectingIPHeader = "CF-Connecting-IP"

// ClientInfo is the classified origin of a request. RemoteIP is the real
// client IP: the socket address for host/LAN clients, or the Cloudflare
// reported address for tunnel clients.
type ClientInfo struct {
	Class    ClientClass
	RemoteIP net.IP
}

// ClassifyClient determines the request origin class. CF-Connecting-IP is
// trusted only when the direct connection comes from loopback: that is our
// own cloudflared. A non-loopback client sending a spoofed header is still
// classified LAN, so the header can never upgrade a remote client's trust.
func ClassifyClient(r *http.Request) ClientInfo {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		if forwarded := net.ParseIP(r.Header.Get(CFConnectingIPHeader)); forwarded != nil {
			return ClientInfo{Class: ClientClassTunnel, RemoteIP: forwarded}
		}
		return ClientInfo{Class: ClientClassHost, RemoteIP: ip}
	}
	return ClientInfo{Class: ClientClassLAN, RemoteIP: ip}
}

// Key returns a stable string key for the client IP.
func (c ClientInfo) Key() string {
	if c.RemoteIP == nil {
		return ""
	}
	return c.RemoteIP.String()
}
