package harness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverFindsInstalledKnownHarnesses(t *testing.T) {
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "codex"), "#!/bin/sh\necho codex-cli test-version\n")
	writeExecutable(t, filepath.Join(dir, "opencode"), "#!/bin/sh\necho 9.8.7\n")
	t.Setenv("PATH", dir)

	detected := Discover(context.Background(), true)
	if len(detected) != 2 {
		t.Fatalf("detected count = %d, want 2: %+v", len(detected), detected)
	}
	if detected[0].ID != "codex" || !detected[0].Installed || detected[0].Path == "" {
		t.Fatalf("unexpected first detection: %+v", detected[0])
	}
	if detected[0].Version != "codex-cli test-version" {
		t.Fatalf("codex version = %q", detected[0].Version)
	}
	if detected[1].ID != "opencode" || detected[1].Version != "9.8.7" {
		t.Fatalf("unexpected second detection: %+v", detected[1])
	}
}

func TestDiscoverCanIncludeMissingKnownHarnesses(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	detected := Discover(context.Background(), false)
	if len(detected) != len(KnownDefinitions()) {
		t.Fatalf("detected count = %d, want %d", len(detected), len(KnownDefinitions()))
	}
	for _, item := range detected {
		if item.Installed {
			t.Fatalf("expected no installed harnesses with empty PATH, got %+v", item)
		}
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
}
