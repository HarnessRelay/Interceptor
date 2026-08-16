package tunnel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeAsset is a tiny stand-in for the cloudflared binary: a script that
// answers --version.
const fakeAsset = "#!/bin/sh\n[ \"$1\" = \"--version\" ] && echo \"cloudflared version %s (fake)\" && exit 0\nexit 1\n"

// newFakeReleaseServer serves a GitHub-style /releases/latest endpoint plus
// the binary asset. digest controls the reported sha256; empty omits it.
func newFakeReleaseServer(t *testing.T, version, digest string, status int) (*httptest.Server, *int) {
	t.Helper()
	hits := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		hits++
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		body := map[string]any{
			"tag_name": "v" + version,
			"assets": []map[string]string{{
				"name":                 fmt.Sprintf("cloudflared-linux-%s", runtime.GOARCH),
				"browser_download_url": fmt.Sprintf("http://%s/asset/cloudflared", r.Host),
				"digest":               digest,
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
	mux.HandleFunc("/asset/cloudflared", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, fakeAsset, version)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &hits
}

func assetDigest(t *testing.T, version string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(fmt.Sprintf(fakeAsset, version)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func newTestDownloader(t *testing.T, api string) *Downloader {
	dir := t.TempDir()
	t.Setenv("HARNESSRELAY_CLOUDFLARED_BIN_DIR", dir)
	return &Downloader{API: api, Logger: testLogger()}
}

func TestResolveBinaryOrder(t *testing.T) {
	t.Setenv("HARNESSRELAY_CLOUDFLARED_BIN", "")
	t.Setenv("HARNESSRELAY_CLOUDFLARED_BIN_DIR", "")

	// Managed copy wins when present.
	dir := t.TempDir()
	managed := filepath.Join(dir, "cloudflared")
	if err := os.WriteFile(managed, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESSRELAY_CLOUDFLARED_BIN_DIR", dir)
	path, source := ResolveBinary()
	if path != managed || source != BinarySourceManaged {
		t.Fatalf("resolve = %q/%q, want managed copy", path, source)
	}

	// Explicit env override beats the managed copy.
	t.Setenv("HARNESSRELAY_CLOUDFLARED_BIN", managed)
	path, source = ResolveBinary()
	if source != BinarySourceEnv {
		t.Fatalf("source = %q, want env", source)
	}
	t.Setenv("HARNESSRELAY_CLOUDFLARED_BIN", "")
}

func TestInstallLatestHappyPath(t *testing.T) {
	version := "2099.1.2"
	srv, _ := newFakeReleaseServer(t, version, assetDigest(t, version), http.StatusOK)
	dl := newTestDownloader(t, srv.URL+"/releases/latest")

	gotVersion, path, err := dl.InstallLatest(context.Background())
	if err != nil {
		t.Fatalf("InstallLatest: %v", err)
	}
	if gotVersion != version {
		t.Errorf("version = %q, want %q", gotVersion, version)
	}
	if path != ManagedBinaryPath() {
		t.Errorf("path = %q, want %q", path, ManagedBinaryPath())
	}
	if !fileExists(path) {
		t.Fatal("installed binary missing")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o755 {
		t.Errorf("installed mode = %o, want 0755", perm)
	}
	if fileExists(path + previousSuffix) {
		t.Error("first install should not create a .previous file")
	}

	// Second install swaps and keeps the old binary as fallback.
	version2 := "2099.2.0"
	srv2, _ := newFakeReleaseServer(t, version2, assetDigest(t, version2), http.StatusOK)
	dl2 := &Downloader{API: srv2.URL + "/releases/latest", Logger: testLogger()}
	got2, _, err := dl2.InstallLatest(context.Background())
	if err != nil {
		t.Fatalf("second InstallLatest: %v", err)
	}
	if got2 != version2 {
		t.Errorf("second version = %q, want %q", got2, version2)
	}
	if !fileExists(path + previousSuffix) {
		t.Error("second install should keep a .previous fallback")
	}
}

func TestInstallLatestDigestMismatchKeepsOldBinary(t *testing.T) {
	version := "2099.1.2"
	srv, _ := newFakeReleaseServer(t, version, "sha256:"+strings.Repeat("0", 64), http.StatusOK)
	dl := newTestDownloader(t, srv.URL+"/releases/latest")

	// Seed an existing managed binary.
	dir := ManagedBinaryDir()
	old := filepath.Join(dir, "cloudflared")
	if err := os.WriteFile(old, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, _, err := dl.InstallLatest(context.Background())
	if err == nil {
		t.Fatal("digest mismatch should fail")
	}
	if !strings.Contains(err.Error(), "digest") {
		t.Errorf("err = %v, want digest failure", err)
	}
	data, err := os.ReadFile(old)
	if err != nil || string(data) != "#!/bin/sh\nexit 0\n" {
		t.Errorf("existing binary was modified: %q, %v", data, err)
	}
	if fileExists(old + previousSuffix) {
		t.Error("failed install must not leave a .previous file")
	}
}

func TestInstallLatestAPIFailure(t *testing.T) {
	srv, hits := newFakeReleaseServer(t, "", "", http.StatusInternalServerError)
	dl := newTestDownloader(t, srv.URL+"/releases/latest")

	if _, _, err := dl.InstallLatest(context.Background()); err == nil {
		t.Fatal("API failure should fail install")
	}
	if *hits == 0 {
		t.Error("release endpoint was not contacted")
	}
	if fileExists(ManagedBinaryPath()) {
		t.Error("nothing should be installed on API failure")
	}
}

func TestInstallLatestMissingAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v2099.1.2",
				"assets": []map[string]string{{
					"name":                 "cloudflared-darwin-amd64.tgz",
					"browser_download_url": "/asset/nope",
				}},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	dl := newTestDownloader(t, srv.URL+"/releases/latest")

	_, _, err := dl.InstallLatest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no cloudflared-linux") {
		t.Fatalf("err = %v, want missing asset failure", err)
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct{ in, want string }{
		{"cloudflared version 2025.1.0 (checksum 1234)", "2025.1.0"},
		{"cloudflared version 2099.10.20", "2099.10.20"},
		{"garbage", ""},
	}
	for _, tt := range tests {
		if got := parseVersion(tt.in); got != tt.want {
			t.Errorf("parseVersion(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
