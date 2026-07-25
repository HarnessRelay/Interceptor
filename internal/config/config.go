package config

import (
	"errors"
	"fmt"
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

type Config struct {
	BindAddress string
	Port        int
	Storage     StorageConfig
	Terminal    TerminalConfig
	Security    SecurityConfig
}

type StorageConfig struct {
	Path string
}

type TerminalConfig struct {
	HistoryLimitBytes int64
}

type SecurityConfig struct {
	AuthToken           string
	AllowRootForTesting bool
	AllowNonLocalBind   bool
}

func Default() Config {
	return Config{
		BindAddress: DefaultBindAddress,
		Port:        DefaultPort,
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
	cfg.Security.AuthToken = os.Getenv("HARNESSRELAY_TOKEN")
	cfg.Security.AllowRootForTesting = os.Getenv("HARNESSRELAY_ALLOW_ROOT_FOR_TESTING") == "1"
	cfg.Security.AllowNonLocalBind = os.Getenv("HARNESSRELAY_ALLOW_NONLOCAL_BIND") == "1"
	if !isLocalBind(cfg.BindAddress) && !cfg.Security.AllowNonLocalBind {
		return Config{}, errors.New("non-local bind requires HARNESSRELAY_ALLOW_NONLOCAL_BIND=1")
	}
	return cfg, nil
}

func (c Config) Address() string {
	return fmt.Sprintf("%s:%d", c.BindAddress, c.Port)
}

func SearchPaths() []string {
	var paths []string
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		paths = append(paths, filepath.Join(xdgConfig, "harnessrelay", "interceptor.toml"))
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
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
