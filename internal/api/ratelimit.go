package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

const (
	loginRateWindow   = time.Minute
	loginRateMax      = 5
	pairingRateWindow = time.Minute
	pairingRateMax    = 10
)

// rateLimiter is a fixed-window per-key throttle. It exists to slow token
// brute-force attempts once the daemon is reachable through a tunnel or LAN.
type rateLimiter struct {
	mu     sync.Mutex
	window time.Duration
	max    int
	hits   map[string][]time.Time
	now    func() time.Time
}

func newRateLimiter(window time.Duration, max int) *rateLimiter {
	return &rateLimiter{
		window: window,
		max:    max,
		hits:   make(map[string][]time.Time),
		now:    time.Now,
	}
}

// allow records a hit for key and reports whether it is within the limit.
func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	cutoff := now.Add(-l.window)
	recent := l.hits[key][:0]
	for _, ts := range l.hits[key] {
		if ts.After(cutoff) {
			recent = append(recent, ts)
		}
	}
	if len(recent) >= l.max {
		l.hits[key] = recent
		return false
	}
	l.hits[key] = append(recent, now)
	if len(l.hits) > 1024 {
		l.pruneLocked(cutoff)
	}
	return true
}

// pruneLocked drops stale keys; caller holds the mutex.
func (l *rateLimiter) pruneLocked(cutoff time.Time) {
	for key, stamps := range l.hits {
		recent := stamps[:0]
		for _, ts := range stamps {
			if ts.After(cutoff) {
				recent = append(recent, ts)
			}
		}
		if len(recent) == 0 {
			delete(l.hits, key)
			continue
		}
		l.hits[key] = recent
	}
}

// clientIP extracts the remote address host for rate-limit keys.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
