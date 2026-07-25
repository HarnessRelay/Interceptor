package config

import (
	"fmt"
	"os"
	"path/filepath"
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
}

type StorageConfig struct {
	Path string
}

type TerminalConfig struct {
	HistoryLimitBytes int64
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
	return Default(), nil
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
