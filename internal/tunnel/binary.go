package tunnel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// BinarySource describes where the resolved cloudflared binary came from.
type BinarySource string

const (
	BinarySourceEnv     BinarySource = "env"
	BinarySourceManaged BinarySource = "managed"
	BinarySourcePath    BinarySource = "path"
	BinarySourceCommon  BinarySource = "common"
)

const (
	// githubReleasesAPI is the public endpoint for the latest cloudflared release.
	githubReleasesAPI = "https://api.github.com/repos/cloudflare/cloudflared/releases/latest"
	previousSuffix    = ".previous"
)

// ManagedBinaryDir returns the directory holding the app-managed cloudflared
// binary. Default: ~/.local/share/harnessrelay/bin (XDG_DATA_HOME aware).
// HARNESSRELAY_CLOUDFLARED_BIN_DIR overrides it.
func ManagedBinaryDir() string {
	if dir := os.Getenv("HARNESSRELAY_CLOUDFLARED_BIN_DIR"); dir != "" {
		return dir
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "harnessrelay", "bin")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "harnessrelay", "bin")
}

// ManagedBinaryPath is the expected location of the app-managed cloudflared.
func ManagedBinaryPath() string {
	dir := ManagedBinaryDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "cloudflared")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// commonPaths are probed when "cloudflared" is not found in PATH. This covers
// Linuxbrew, Homebrew (Apple Silicon/Intel), Snap, and /usr/local/bin —
// locations that may be missing from the daemon's PATH under systemd.
var commonPaths = []string{
	"/home/linuxbrew/.linuxbrew/bin/cloudflared",
	"/opt/homebrew/bin/cloudflared",
	"/usr/local/bin/cloudflared",
	"/usr/bin/cloudflared",
	"/snap/bin/cloudflared",
}

// ResolveBinary finds cloudflared: an explicit env override first (which is
// authoritative even when broken, so misconfiguration is loud), then the
// app-managed copy, PATH, and common installation directories.
func ResolveBinary() (string, BinarySource) {
	if p := os.Getenv("HARNESSRELAY_CLOUDFLARED_BIN"); p != "" {
		if fileExists(p) {
			return p, BinarySourceEnv
		}
		return "", ""
	}
	if p := ManagedBinaryPath(); p != "" && fileExists(p) {
		return p, BinarySourceManaged
	}
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p, BinarySourcePath
	}
	for _, candidate := range commonPaths {
		if fileExists(candidate) {
			return candidate, BinarySourceCommon
		}
	}
	return "", ""
}

// BinaryPath returns the resolved cloudflared path or an empty string.
func BinaryPath() string {
	p, _ := ResolveBinary()
	return p
}

// IsAvailable reports whether a cloudflared binary can be found.
func IsAvailable() bool {
	return BinaryPath() != ""
}

// BinaryVersion runs `<binary> --version` and returns the trimmed output.
func BinaryVersion(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

var versionPattern = regexp.MustCompile(`\d+\.\d+\.\d+`)

// parseVersion extracts the semver from a cloudflared --version line.
func parseVersion(output string) string {
	return versionPattern.FindString(output)
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest"`
}

type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

// Downloader installs cloudflared from Cloudflare's GitHub releases into the
// managed binary directory with digest verification and an atomic swap that
// keeps the previous binary on disk as a fallback.
type Downloader struct {
	// API is the releases endpoint URL; overridable for tests.
	API string
	// Client used for both API and asset requests.
	Client *http.Client
	Logger *slog.Logger
}

func NewDownloader() *Downloader {
	return &Downloader{
		API:    githubReleasesAPI,
		Client: &http.Client{Timeout: 10 * time.Minute},
		Logger: slog.Default(),
	}
}

func (d *Downloader) fetchRelease(ctx context.Context) (releaseInfo, error) {
	api := d.API
	if api == "" {
		api = githubReleasesAPI
	}
	client := d.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return releaseInfo{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return releaseInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return releaseInfo{}, fmt.Errorf("release lookup returned %s", resp.Status)
	}
	var info releaseInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&info); err != nil {
		return releaseInfo{}, err
	}
	return info, nil
}

func (d *Downloader) assetFor(info releaseInfo) (releaseAsset, error) {
	name := fmt.Sprintf("cloudflared-linux-%s", runtime.GOARCH)
	for _, asset := range info.Assets {
		if asset.Name == name && asset.BrowserDownloadURL != "" {
			return asset, nil
		}
	}
	return releaseAsset{}, fmt.Errorf("release %s has no %s asset", info.TagName, name)
}

// InstallLatest downloads the newest cloudflared release into the managed
// directory. On any failure an existing managed binary is left untouched.
func (d *Downloader) InstallLatest(ctx context.Context) (version string, path string, err error) {
	info, err := d.fetchRelease(ctx)
	if err != nil {
		return "", "", err
	}
	asset, err := d.assetFor(info)
	if err != nil {
		return "", "", err
	}

	dir := ManagedBinaryDir()
	if dir == "" {
		return "", "", errors.New("cannot determine managed binary directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	final := filepath.Join(dir, "cloudflared")
	previous := final + previousSuffix

	tmp, err := os.CreateTemp(dir, ".cloudflared-download-*")
	if err != nil {
		return "", "", err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	if err := d.downloadAsset(ctx, asset, tmp); err != nil {
		return "", "", err
	}
	if err := tmp.Close(); err != nil {
		return "", "", err
	}
	if err := verifyDigest(asset.Digest, tmpName); err != nil {
		return "", "", err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return "", "", err
	}

	version, err = runVersionCheck(ctx, tmpName, info.TagName)
	if err != nil {
		return "", "", err
	}

	if fileExists(final) {
		_ = os.Remove(previous)
		if err := os.Rename(final, previous); err != nil {
			return "", "", fmt.Errorf("backup existing binary: %w", err)
		}
		if err := os.Rename(tmpName, final); err != nil {
			_ = os.Rename(previous, final) // best-effort rollback
			return "", "", fmt.Errorf("install downloaded binary: %w", err)
		}
	} else {
		if err := os.Rename(tmpName, final); err != nil {
			return "", "", err
		}
	}
	return version, final, nil
}

func (d *Downloader) downloadAsset(ctx context.Context, asset releaseAsset, dst io.Writer) error {
	client := d.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("asset download returned %s", resp.Status)
	}
	_, err = io.Copy(dst, resp.Body)
	return err
}

// verifyDigest checks the downloaded file against a GitHub asset digest
// ("sha256:<hex>"). An empty digest is accepted (older releases may not
// publish one); the post-download --version check still applies.
func verifyDigest(digest, path string) error {
	if digest == "" {
		return nil
	}
	wantHex, ok := strings.CutPrefix(digest, "sha256:")
	if !ok {
		return fmt.Errorf("unsupported digest format: %s", digest)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), wantHex) {
		return errors.New("downloaded binary failed digest verification")
	}
	return nil
}

func runVersionCheck(ctx context.Context, path, fallbackTag string) (string, error) {
	out, err := BinaryVersion(ctx, path)
	if err != nil {
		return "", fmt.Errorf("downloaded binary failed to run: %w", err)
	}
	if v := parseVersion(out); v != "" {
		return v, nil
	}
	if fallbackTag != "" {
		return strings.TrimPrefix(fallbackTag, "v"), nil
	}
	return "", errors.New("downloaded binary reported no version")
}
