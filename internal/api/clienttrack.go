package api

import (
	"bufio"
	"context"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/harnessrelay/interceptor/internal/security"
)

// clientEntry is one observed network client.
type clientEntry struct {
	IP        string
	Class     security.ClientClass
	FirstSeen time.Time
	LastSeen  time.Time
	Conns     int
}

// clientTracker records every client that hits the daemon so the settings
// Network tab can show who is (or was recently) connected.
type clientTracker struct {
	mu          sync.Mutex
	clients     map[string]*clientEntry // keyed by real client IP
	hostnames   map[string]string
	arpPath     string
	resolver    *net.Resolver
	lastARPLoad time.Time
	arpCache    map[string]string
}

func newClientTracker() *clientTracker {
	return &clientTracker{
		clients:   make(map[string]*clientEntry),
		hostnames: make(map[string]string),
		arpPath:   "/proc/net/arp",
		resolver:  net.DefaultResolver,
		arpCache:  make(map[string]string),
	}
}

func (t *clientTracker) record(info security.ClientInfo) {
	if info.RemoteIP == nil {
		return
	}
	key := info.Key()
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	entry, ok := t.clients[key]
	if !ok {
		t.clients[key] = &clientEntry{IP: key, Class: info.Class, FirstSeen: now, LastSeen: now}
		return
	}
	entry.Class = info.Class
	entry.LastSeen = now
}

func (t *clientTracker) connOpen(key string)  { t.adjustConns(key, 1) }
func (t *clientTracker) connClose(key string) { t.adjustConns(key, -1) }

func (t *clientTracker) adjustConns(key string, delta int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if entry, ok := t.clients[key]; ok {
		entry.Conns += delta
		if entry.Conns < 0 {
			entry.Conns = 0
		}
	}
}

// list returns clients seen in the last 15 minutes, most recent first.
func (t *clientTracker) list() []clientEntry {
	cutoff := time.Now().Add(-15 * time.Minute)
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]clientEntry, 0, len(t.clients))
	for _, entry := range t.clients {
		if entry.LastSeen.Before(cutoff) && entry.Conns == 0 {
			continue
		}
		out = append(out, *entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	return out
}

// parseARPTable parses /proc/net/arp content into an IP → MAC map.
func parseARPTable(data string) map[string]string {
	out := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(data))
	scanner.Scan() // header
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		ip, hwType, mac := fields[0], fields[1], fields[3]
		if hwType != "0x1" || !strings.Contains(mac, ":") {
			continue
		}
		out[ip] = strings.ToUpper(mac)
	}
	return out
}

func (t *clientTracker) loadARP() map[string]string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if time.Since(t.lastARPLoad) < 10*time.Second {
		return t.arpCache
	}
	data, err := os.ReadFile(t.arpPath)
	if err != nil {
		return t.arpCache
	}
	t.arpCache = parseARPTable(string(data))
	t.lastARPLoad = time.Now()
	return t.arpCache
}

// hostname does a best-effort reverse DNS lookup with a short timeout and
// caches results for the tracker's lifetime.
func (t *clientTracker) hostname(ip string) string {
	t.mu.Lock()
	if name, ok := t.hostnames[ip]; ok {
		t.mu.Unlock()
		return name
	}
	t.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	names, err := t.resolver.LookupAddr(ctx, ip)
	name := ""
	if err == nil && len(names) > 0 {
		name = strings.TrimSuffix(names[0], ".")
	}
	t.mu.Lock()
	t.hostnames[ip] = name
	t.mu.Unlock()
	return name
}

// LANAddresses lists this machine's non-loopback IPv4/IPv6 addresses.
func LANAddresses() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			out = append(out, ip.String())
		}
	}
	sort.Strings(out)
	return out
}
