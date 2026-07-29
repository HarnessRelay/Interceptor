package config

import (
	"net"
	"os"
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

func TestLoadSecurityEnvironment(t *testing.T) {
	t.Setenv("HARNESSRELAY_TOKEN", "local-secret")
	t.Setenv("HARNESSRELAY_ALLOW_ROOT_FOR_TESTING", "1")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Security.AuthToken != "local-secret" {
		t.Fatalf("AuthToken = %q, want local-secret", cfg.Security.AuthToken)
	}
	if cfg.Security.AuthTokenSource != "env" {
		t.Fatalf("AuthTokenSource = %q, want env", cfg.Security.AuthTokenSource)
	}
	if !cfg.Security.AllowRootForTesting {
		t.Fatal("AllowRootForTesting = false, want true")
	}
}

func TestLoadSecurityTokenFileAndEnvironmentOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HARNESSRELAY_TOKEN", "")
	tokenPath := filepath.Join(root, "harnessrelay", "token")
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenPath, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.AuthToken != "file-secret" || cfg.Security.AuthTokenSource != "config" {
		t.Fatalf("file auth = %q from %q", cfg.Security.AuthToken, cfg.Security.AuthTokenSource)
	}

	t.Setenv("HARNESSRELAY_TOKEN", "env-secret")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.AuthToken != "env-secret" || cfg.Security.AuthTokenSource != "env" {
		t.Fatalf("override auth = %q from %q", cfg.Security.AuthToken, cfg.Security.AuthTokenSource)
	}
}

func TestLoadUserConfigWithEnvironmentOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	path := filepath.Join(root, "harnessrelay", "interceptor.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("bind_address = \"localhost\"\nport = 9123\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BindAddress != "localhost" || cfg.Port != 9123 {
		t.Fatalf("file config address = %s", cfg.Address())
	}
	t.Setenv("HARNESSRELAY_PORT", "9234")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != 9234 {
		t.Fatalf("environment did not override config port: %d", cfg.Port)
	}
}

func TestResolveAuthTokenMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HARNESSRELAY_TOKEN", "")
	token, source, err := ResolveAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	if token != "" || source != "missing" {
		t.Fatalf("token = %q, source = %q", token, source)
	}
}

func TestLoadRejectsNonLocalBindWithoutExplicitAllow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HARNESSRELAY_BIND_ADDRESS", "0.0.0.0")
	if _, err := Load(); err == nil {
		t.Fatal("Load accepted non-local bind without explicit allow")
	}
}

func TestLoadAllowsExplicitNonLocalBind(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HARNESSRELAY_BIND_ADDRESS", "0.0.0.0")
	t.Setenv("HARNESSRELAY_ALLOW_NONLOCAL_BIND", "1")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BindAddress != "0.0.0.0" {
		t.Fatalf("BindAddress = %q, want 0.0.0.0", cfg.BindAddress)
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

func TestLoadAllowedIPs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)

	path, err := AllowedIPsPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	content := "# comment\n192.168.1.5\n192.168.2.0/24\n::1\n2001:db8::/32\n\ninvalid-line\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	allowed, err := loadAllowedIPs()
	if err != nil {
		t.Fatalf("loadAllowedIPs: %v", err)
	}
	if len(allowed) != 4 {
		t.Fatalf("loaded %d entries, want 4", len(allowed))
	}

	if allowed[0].IP == nil || !allowed[0].IP.Equal(net.ParseIP("192.168.1.5")) {
		t.Fatalf("first entry IP = %v", allowed[0].IP)
	}
	if allowed[0].CIDR == nil || !allowed[0].CIDR.Contains(net.ParseIP("192.168.1.5")) {
		t.Fatal("first entry missing valid CIDR")
	}

	if allowed[1].CIDR == nil || !allowed[1].CIDR.Contains(net.ParseIP("192.168.2.100")) {
		t.Fatalf("second entry CIDR = %v", allowed[1].CIDR)
	}

	if allowed[2].IP == nil || !allowed[2].IP.Equal(net.ParseIP("::1")) {
		t.Fatalf("third entry IP = %v", allowed[2].IP)
	}

	if allowed[3].CIDR == nil || !allowed[3].CIDR.Contains(net.ParseIP("2001:db8::1")) {
		t.Fatalf("fourth entry CIDR = %v", allowed[3].CIDR)
	}
}

func TestLoadAllowlistPermitsNonLocalBind(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HARNESSRELAY_BIND_ADDRESS", "")
	t.Setenv("HARNESSRELAY_ALLOW_NONLOCAL_BIND", "")

	cfgPath := filepath.Join(root, "harnessrelay", "interceptor.toml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("bind_address = \"0.0.0.0\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ipsPath := filepath.Join(root, "harnessrelay", "allowed_ips.txt")
	if err := os.WriteFile(ipsPath, []byte("192.168.1.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with default allowlist_permits_nonlocal_bind: %v", err)
	}
	if !cfg.AllowlistPermitsNonLocalBind {
		t.Fatal("AllowlistPermitsNonLocalBind should default to true")
	}

	if err := os.WriteFile(cfgPath, []byte("bind_address = \"0.0.0.0\"\nallowlist_permits_nonlocal_bind = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Load()
	if err == nil {
		t.Fatal("Load should fail when allowlist_permits_nonlocal_bind = false and no env var")
	}
}

func TestLoadAllowlistReplacesEnvVar(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HARNESSRELAY_BIND_ADDRESS", "")
	t.Setenv("HARNESSRELAY_ALLOW_NONLOCAL_BIND", "")

	cfgPath := filepath.Join(root, "harnessrelay", "interceptor.toml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("bind_address = \"0.0.0.0\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ipsPath := filepath.Join(root, "harnessrelay", "allowed_ips.txt")
	if err := os.WriteFile(ipsPath, []byte("192.168.1.0/24\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.BindAddress != "0.0.0.0" {
		t.Fatalf("BindAddress = %q, want 0.0.0.0", cfg.BindAddress)
	}
	if len(cfg.AllowedIPs) != 1 {
		t.Fatalf("AllowedIPs = %d, want 1", len(cfg.AllowedIPs))
	}
}

func TestLoadAllowlistDoesNotReplaceEnvVarWhenDisabled(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HARNESSRELAY_BIND_ADDRESS", "")
	t.Setenv("HARNESSRELAY_ALLOW_NONLOCAL_BIND", "")

	cfgPath := filepath.Join(root, "harnessrelay", "interceptor.toml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("bind_address = \"0.0.0.0\"\nallowlist_permits_nonlocal_bind = false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ipsPath := filepath.Join(root, "harnessrelay", "allowed_ips.txt")
	if err := os.WriteFile(ipsPath, []byte("192.168.1.0/24\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load should fail when allowlist_permits_nonlocal_bind = false")
	}
}
