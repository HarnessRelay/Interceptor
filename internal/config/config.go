package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	Format                         = "toml"
	DefaultBindAddress             = "127.0.0.1"
	DefaultPort                    = 8765
	DefaultTerminalHistoryLimitMiB = 4
)

type AllowedIP struct {
	IP   net.IP
	CIDR *net.IPNet
}

type Config struct {
	BindAddress                  string
	Port                         int
	Storage                      StorageConfig
	Terminal                     TerminalConfig
	Security                     SecurityConfig
	AllowlistPermitsNonLocalBind bool
	AllowedIPs                   []AllowedIP
}

type StorageConfig struct {
	Path string
}

type TerminalConfig struct {
	HistoryLimitBytes int64
}

type SecurityConfig struct {
	AuthToken           string
	AuthTokenSource     string
	AllowRootForTesting bool
	AllowNonLocalBind   bool
}

func Default() Config {
	return Config{
		BindAddress:                  DefaultBindAddress,
		Port:                         DefaultPort,
		AllowlistPermitsNonLocalBind: true,
		Storage: StorageConfig{
			Path: defaultStoragePath(),
		},
		Terminal: TerminalConfig{
			HistoryLimitBytes: DefaultTerminalHistoryLimitMiB * 1024 * 1024,
		},
	}
}

func Load() (Config, error) {
	cfg := Default()
	if err := loadFile(&cfg); err != nil {
		return Config{}, err
	}
	if bindAddress := os.Getenv("HARNESSRELAY_BIND_ADDRESS"); bindAddress != "" {
		cfg.BindAddress = bindAddress
	}
	if port := os.Getenv("HARNESSRELAY_PORT"); port != "" {
		parsed, err := strconv.Atoi(port)
		if err != nil || parsed <= 0 || parsed > 65535 {
			return Config{}, fmt.Errorf("invalid HARNESSRELAY_PORT %q", port)
		}
		cfg.Port = parsed
	}
	authToken, authTokenSource, err := ResolveAuthToken()
	if err != nil {
		return Config{}, err
	}
	cfg.Security.AuthToken = authToken
	cfg.Security.AuthTokenSource = authTokenSource
	cfg.Security.AllowRootForTesting = os.Getenv("HARNESSRELAY_ALLOW_ROOT_FOR_TESTING") == "1"
	cfg.Security.AllowNonLocalBind = os.Getenv("HARNESSRELAY_ALLOW_NONLOCAL_BIND") == "1"

	allowedIPs, err := loadAllowedIPs()
	if err != nil {
		return Config{}, err
	}
	cfg.AllowedIPs = allowedIPs

	hasAllowlist := len(cfg.AllowedIPs) > 0
	allowNonLocal := cfg.Security.AllowNonLocalBind ||
		(hasAllowlist && cfg.AllowlistPermitsNonLocalBind)
	if !isLocalBind(cfg.BindAddress) && !allowNonLocal {
		return Config{}, errors.New("non-local bind requires HARNESSRELAY_ALLOW_NONLOCAL_BIND=1 or an IP allowlist")
	}
	return cfg, nil
}

func loadFile(cfg *Config) error {
	for _, path := range SearchPaths() {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read config %s: %w", path, err)
		}
		lines := strings.Split(string(data), "\n")
		for index, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
				continue
			}
			key, raw, found := strings.Cut(line, "=")
			if !found {
				return fmt.Errorf("parse config %s:%d: expected key = value", path, index+1)
			}
			key = strings.TrimSpace(key)
			raw = strings.TrimSpace(raw)
			switch key {
			case "bind_address":
				value, err := strconv.Unquote(raw)
				if err != nil || value == "" {
					return fmt.Errorf("parse config %s:%d: invalid bind_address", path, index+1)
				}
				cfg.BindAddress = value
			case "port":
				value, err := strconv.Atoi(raw)
				if err != nil || value <= 0 || value > 65535 {
					return fmt.Errorf("parse config %s:%d: invalid port", path, index+1)
				}
				cfg.Port = value
			case "allowlist_permits_nonlocal_bind":
				cfg.AllowlistPermitsNonLocalBind = strings.ToLower(raw) == "true"
			}
		}
		return nil
	}
	return nil
}

// ResolveAuthToken applies the local authentication precedence used by both
// harnessd and harnessctl. An explicit environment value always wins; the
// user-local token file is the stable fallback.
func ResolveAuthToken() (token, source string, err error) {
	if token := os.Getenv("HARNESSRELAY_TOKEN"); token != "" {
		return token, "env", nil
	}
	path, err := TokenPath()
	if err != nil {
		return "", "missing", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", "missing", nil
	}
	if err != nil {
		return "", "missing", fmt.Errorf("read auth token %s: %w", path, err)
	}
	token = strings.TrimSpace(string(data))
	if token == "" {
		return "", "missing", fmt.Errorf("auth token file %s is empty", path)
	}
	return token, "config", nil
}

func ConfigDir() (string, error) {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "harnessrelay"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "harnessrelay"), nil
}

func TokenPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "token"), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "interceptor.toml"), nil
}

func AllowedIPsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "allowed_ips.txt"), nil
}

func loadAllowedIPs() ([]AllowedIP, error) {
	path, err := AllowedIPsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read allowed IPs %s: %w", path, err)
	}

	var allowed []AllowedIP
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if ip, ipNet, err := net.ParseCIDR(line); err == nil {
			allowed = append(allowed, AllowedIP{IP: ip, CIDR: ipNet})
			continue
		}
		if ip := net.ParseIP(line); ip != nil {
			var bits int
			if ip.To4() != nil {
				bits = 32
			} else {
				bits = 128
			}
			allowed = append(allowed, AllowedIP{
				IP: ip,
				CIDR: &net.IPNet{
					IP:   ip,
					Mask: net.CIDRMask(bits, bits),
				},
			})
			continue
		}
	}
	return allowed, nil
}

func (c Config) Address() string {
	return fmt.Sprintf("%s:%d", c.BindAddress, c.Port)
}

func SearchPaths() []string {
	var paths []string
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		paths = append(paths, filepath.Join(xdgConfig, "harnessrelay", "interceptor.toml"))
	} else if home, err := os.UserHomeDir(); err == nil && home != "" {
		paths = append(paths, filepath.Join(home, ".config", "harnessrelay", "interceptor.toml"))
	}
	paths = append(paths, "harnessrelay.interceptor.toml")
	return paths
}

func defaultStoragePath() string {
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		return filepath.Join(xdgData, "harnessrelay", "interceptor")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "share", "harnessrelay", "interceptor")
	}
	return filepath.Join(".", ".harnessrelay", "interceptor")
}

func isLocalBind(bindAddress string) bool {
	host := strings.Trim(bindAddress, "[]")
	switch host {
	case "", "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
