package security

import (
	"net/http"
	"strings"
)

func SameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	host := r.Host
	if host == "" {
		host = r.URL.Host
	}
	if host == "" {
		return false
	}
	allowedHTTP := "http://" + host
	allowedHTTPS := "https://" + host
	return strings.EqualFold(origin, allowedHTTP) || strings.EqualFold(origin, allowedHTTPS)
}

func IsLocalBind(address string) bool {
	host := address
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	switch host {
	case "", "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	default:
		return false
	}
}
