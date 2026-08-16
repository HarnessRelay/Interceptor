package security

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// IPEntry is one allowlist or banlist entry, persisted one per line in CIDR
// or plain-IP form.
type IPEntry struct {
	Raw  string
	IP   net.IP
	CIDR *net.IPNet
}

func (e IPEntry) Matches(ip net.IP) bool {
	if e.CIDR != nil {
		return e.CIDR.Contains(ip)
	}
	return e.IP != nil && e.IP.Equal(ip)
}

func parseIPEntry(raw string) (IPEntry, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") {
		return IPEntry{}, errors.New("empty entry")
	}
	if _, ipnet, err := net.ParseCIDR(raw); err == nil {
		return IPEntry{Raw: raw, CIDR: ipnet}, nil
	}
	ip := net.ParseIP(raw)
	if ip == nil {
		return IPEntry{}, fmt.Errorf("invalid IP or CIDR: %s", raw)
	}
	return IPEntry{Raw: raw, IP: ip}, nil
}

// IPFilter is the runtime, hot-reloadable allowlist and banlist. The
// allowlist gates directly connected LAN clients; the banlist applies to the
// real client IP of LAN and tunnel clients. Host clients are never filtered.
type IPFilter struct {
	mu        sync.RWMutex
	allowed   []IPEntry
	banned    []IPEntry
	allowPath string
	banPath   string
}

// NewIPFilter loads persisted lists; missing files start empty. Seeds from
// any allowlist the config loader already read.
func NewIPFilter(allowPath, banPath string, seedAllowed []string) (*IPFilter, error) {
	f := &IPFilter{allowPath: allowPath, banPath: banPath}
	if len(seedAllowed) > 0 {
		entries, err := parseList(seedAllowed)
		if err != nil {
			return nil, err
		}
		f.allowed = entries
	}
	allowed, err := loadListFile(allowPath)
	if err != nil {
		return nil, err
	}
	banned, err := loadListFile(banPath)
	if err != nil {
		return nil, err
	}
	if len(allowed) > 0 {
		f.allowed = allowed // persisted file wins over seed
	}
	f.banned = banned
	return f, nil
}

func parseList(lines []string) ([]IPEntry, error) {
	var out []IPEntry
	for _, line := range lines {
		entry, err := parseIPEntry(line)
		if err != nil {
			continue // malformed lines are skipped, matching config loader
		}
		out = append(out, entry)
	}
	return out, nil
}

func loadListFile(path string) ([]IPEntry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	entries, err := parseList(strings.Split(string(data), "\n"))
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (f *IPFilter) saveList(path string, entries []IPEntry) error {
	var b strings.Builder
	for _, e := range entries {
		b.WriteString(e.Raw)
		b.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".iplist-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.WriteString(b.String()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// AllowedByAllowlist reports whether a LAN client IP passes the allowlist.
// An empty allowlist permits everything (no allowlist configured).
func (f *IPFilter) AllowedByAllowlist(ip net.IP) bool {
	if ip == nil {
		return false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	if len(f.allowed) == 0 {
		return true
	}
	for _, e := range f.allowed {
		if e.Matches(ip) {
			return true
		}
	}
	return false
}

// Banned reports whether the real client IP is on the banlist.
func (f *IPFilter) Banned(ip net.IP) bool {
	if ip == nil {
		return false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, e := range f.banned {
		if e.Matches(ip) {
			return true
		}
	}
	return false
}

// Allow adds an allowlist entry and persists it.
func (f *IPFilter) Allow(entry string) error {
	parsed, err := parseIPEntry(entry)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.allowed {
		if e.Raw == parsed.Raw {
			return nil
		}
	}
	f.allowed = append(f.allowed, parsed)
	return f.saveList(f.allowPath, f.allowed)
}

// Unallow removes an allowlist entry and persists the change.
func (f *IPFilter) Unallow(entry string) error {
	return f.removeEntry(entry, &f.allowed, f.allowPath)
}

// Ban adds a banlist entry and persists it.
func (f *IPFilter) Ban(entry string) error {
	parsed, err := parseIPEntry(entry)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, e := range f.banned {
		if e.Raw == parsed.Raw {
			return nil
		}
	}
	f.banned = append(f.banned, parsed)
	return f.saveList(f.banPath, f.banned)
}

// Unban removes a banlist entry and persists the change.
func (f *IPFilter) Unban(entry string) error {
	return f.removeEntry(entry, &f.banned, f.banPath)
}

func (f *IPFilter) removeEntry(raw string, list *[]IPEntry, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	raw = strings.TrimSpace(raw)
	kept := (*list)[:0]
	removed := false
	for _, e := range *list {
		if e.Raw == raw {
			removed = true
			continue
		}
		kept = append(kept, e)
	}
	if !removed {
		return fmt.Errorf("entry not found: %s", raw)
	}
	*list = kept
	return f.saveList(path, *list)
}

// Lists returns the raw entries of both lists.
func (f *IPFilter) Lists() (allowed, banned []string) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	for _, e := range f.allowed {
		allowed = append(allowed, e.Raw)
	}
	for _, e := range f.banned {
		banned = append(banned, e.Raw)
	}
	return allowed, banned
}
