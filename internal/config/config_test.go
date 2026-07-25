package config

import (
	"path/filepath"
	"testing"
)

func TestDefaultNetworkConfig(t *testing.T) {
	cfg := Default()
	if cfg.BindAddress != "127.0.0.1" {
		t.Fatalf("BindAddress = %q, want 127.0.0.1", cfg.BindAddress)
	}
	if cfg.Port != 8765 {
		t.Fatalf("Port = %d, want 8765", cfg.Port)
	}
	if cfg.Address() != "127.0.0.1:8765" {
		t.Fatalf("Address() = %q, want 127.0.0.1:8765", cfg.Address())
	}
}

func TestDefaultTerminalHistoryLimit(t *testing.T) {
	cfg := Default()
	const want = int64(4 * 1024 * 1024)
	if cfg.Terminal.HistoryLimitBytes != want {
		t.Fatalf("HistoryLimitBytes = %d, want %d", cfg.Terminal.HistoryLimitBytes, want)
	}
}

func TestDefaultStoragePathUsesXDGDataHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/tmp/xdg-data")
	cfg := Default()
	want := filepath.Join("/tmp/xdg-data", "harnessrelay", "interceptor")
	if cfg.Storage.Path != want {
		t.Fatalf("Storage.Path = %q, want %q", cfg.Storage.Path, want)
	}
}

func TestSearchPathsIncludesFutureTOMLLocations(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-config")
	paths := SearchPaths()
	if len(paths) == 0 {
		t.Fatal("SearchPaths returned no paths")
	}
	if paths[0] != filepath.Join("/tmp/xdg-config", "harnessrelay", "interceptor.toml") {
		t.Fatalf("SearchPaths()[0] = %q", paths[0])
	}
	if Format != "toml" {
		t.Fatalf("Format = %q, want toml", Format)
	}
}
