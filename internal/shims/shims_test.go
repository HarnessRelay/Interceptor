package shims

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func executable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigRoundTrip(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config", "shims.json")
	cfg := NewConfig(filepath.Join(root, "shims"))
	cfg.Entries["fake"] = Entry{
		Enabled: true, ShimPath: filepath.Join(root, "shims", "fake"),
		RealBinary: executable(t, root, "fake", "#!/bin/sh\n"), Harness: "fake",
		Backend: BackendPTY, CreatedBy: "harnessrelay",
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Entries["fake"].Backend != BackendPTY || got.Version != ConfigVersion {
		t.Fatalf("unexpected config: %+v", got)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %v, err = %v", info.Mode().Perm(), err)
	}
}

func TestLoadRejectsMalformedConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shims.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"shim_dir":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "malformed config") {
		t.Fatalf("error = %v", err)
	}
}

func TestInstallGeneratesAuditableShimAndResolvesOutsideShimDir(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	shimDir := filepath.Join(root, "shims")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	real := executable(t, realDir, "fake", "#!/bin/sh\nexit 0\n")
	configPath := filepath.Join(root, "config", "shims.json")
	entry, err := Install(InstallOptions{
		Name: "fake", Backend: BackendPTY, Harnessctl: "/opt/harnessctl",
		Path:       shimDir + string(os.PathListSeparator) + realDir,
		ConfigPath: configPath, ShimDir: shimDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if entry.RealBinary != real {
		t.Fatalf("real binary = %q, want %q", entry.RealBinary, real)
	}
	data, err := os.ReadFile(entry.ShimPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, OwnershipMarker) ||
		!strings.Contains(text, "'/opt/harnessctl' shim exec 'fake' -- \"$@\"") {
		t.Fatalf("generated shim = %q", text)
	}
}

func TestInstallRefusesUnmanagedFileWithoutForce(t *testing.T) {
	root := t.TempDir()
	shimDir := filepath.Join(root, "shims")
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	executable(t, shimDir, "fake", "#!/bin/sh\necho unmanaged\n")
	executable(t, realDir, "fake", "#!/bin/sh\nexit 0\n")
	_, err := Install(InstallOptions{
		Name: "fake", Backend: BackendPTY, Harnessctl: "/opt/harnessctl",
		Path: realDir, ConfigPath: filepath.Join(root, "shims.json"), ShimDir: shimDir,
	})
	if err == nil || !strings.Contains(err.Error(), "unmanaged") {
		t.Fatalf("error = %v", err)
	}
}

func TestResolveRealBinaryPreventsRecursion(t *testing.T) {
	root := t.TempDir()
	shimDir := filepath.Join(root, "shims")
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	executable(t, shimDir, "fake", GeneratedShim("fake", "/opt/harnessctl"))
	managedReal := executable(t, realDir, "fake", GeneratedShim("fake", "/opt/harnessctl"))
	_, err := ResolveRealBinary("fake", shimDir+string(os.PathListSeparator)+realDir, shimDir)
	if err == nil {
		t.Fatal("expected managed candidates to be rejected")
	}
	entry := Entry{ShimPath: filepath.Join(shimDir, "fake"), RealBinary: managedReal}
	if err := ValidateRuntimeEntry("fake", entry); err == nil {
		t.Fatal("expected runtime recursion prevention")
	}
}

func TestResolveRealBinaryCandidatesPreservesPathOrder(t *testing.T) {
	root := t.TempDir()
	firstDir := filepath.Join(root, "first")
	secondDir := filepath.Join(root, "second")
	shimDir := filepath.Join(root, "shims")
	for _, dir := range []string{firstDir, secondDir, shimDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	first := executable(t, firstDir, "fake", "#!/bin/sh\n")
	second := executable(t, secondDir, "fake", "#!/bin/sh\n")
	candidates, err := ResolveRealBinaryCandidates(
		"fake",
		shimDir+string(os.PathListSeparator)+firstDir+string(os.PathListSeparator)+secondDir,
		shimDir,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0] != first || candidates[1] != second {
		t.Fatalf("candidates = %v", candidates)
	}
}

func TestUninstallPreservesUnmanagedFile(t *testing.T) {
	root := t.TempDir()
	shimDir := filepath.Join(root, "shims")
	real := executable(t, root, "real", "#!/bin/sh\n")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatal(err)
	}
	shim := executable(t, shimDir, "fake", "#!/bin/sh\necho changed\n")
	configPath := filepath.Join(root, "shims.json")
	cfg := NewConfig(shimDir)
	cfg.Entries["fake"] = Entry{Enabled: true, ShimPath: shim, RealBinary: real, Harness: "fake", Backend: BackendPTY, CreatedBy: "harnessrelay"}
	if err := Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	if err := Uninstall(configPath, []string{"fake"}); err == nil {
		t.Fatal("expected unmanaged-file refusal")
	}
	if _, err := os.Stat(shim); err != nil {
		t.Fatalf("unmanaged shim was removed: %v", err)
	}
}

func TestUninstallAllRemovesOnlyConfiguredManagedShims(t *testing.T) {
	root := t.TempDir()
	shimDir := filepath.Join(root, "shims")
	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatal(err)
	}
	real := executable(t, root, "real", "#!/bin/sh\n")
	unrelated := executable(t, shimDir, "unrelated", "#!/bin/sh\n")
	cfg := NewConfig(shimDir)
	for _, name := range []string{"one", "two"} {
		path := executable(t, shimDir, name, GeneratedShim(name, "/opt/harnessctl"))
		cfg.Entries[name] = Entry{Enabled: true, ShimPath: path, RealBinary: real, Harness: name, Backend: BackendPTY, CreatedBy: "harnessrelay"}
	}
	configPath := filepath.Join(root, "shims.json")
	if err := Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	if err := UninstallAll(configPath); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"one", "two"} {
		if _, err := os.Stat(filepath.Join(shimDir, name)); !os.IsNotExist(err) {
			t.Fatalf("managed shim %s still exists: %v", name, err)
		}
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated file was removed: %v", err)
	}
}

func TestPathAnalysisRequiresShimBeforeRealBinary(t *testing.T) {
	state := AnalyzePath("/data/shims", "/usr/bin/fake", "/usr/bin:/data/shims")
	if state.Active {
		t.Fatal("shim after real binary must not be active")
	}
	state = AnalyzePath("/data/shims", "/usr/bin/fake", "/data/shims:/usr/bin")
	if !state.Active {
		t.Fatal("shim before real binary should be active")
	}
}

func TestBackendSelection(t *testing.T) {
	for _, value := range []string{"pty", "tmux", "direct"} {
		got, err := ParseBackend(value)
		if err != nil || string(got) != value {
			t.Fatalf("ParseBackend(%q) = %q, %v", value, got, err)
		}
	}
	if _, err := ParseBackend("magic"); err == nil {
		t.Fatal("expected unknown backend to be rejected")
	}
}
