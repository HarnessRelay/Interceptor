// Package service manages HarnessRelay's rootless user service.
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	UnitName      = "harnessrelay.service"
	managedMarker = "# Managed by HarnessRelay. Do not edit."
)

// CommandRunner executes service-manager commands. It is injectable so tests
// never contact the real user service manager or journal.
type CommandRunner interface {
	Run(context.Context, string, []string, io.Writer, io.Writer) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// Manager owns exactly one HarnessRelay systemd user unit.
type Manager struct {
	UnitPath   string
	DaemonPath string
	Systemctl  string
	Journalctl string
	Runner     CommandRunner
}

// NewManager resolves the default user-unit and daemon paths.
func NewManager() (*Manager, error) {
	unitPath, err := DefaultUnitPath()
	if err != nil {
		return nil, err
	}
	daemonPath, _ := ResolveDaemonPath()
	systemctl := os.Getenv("HARNESSRELAY_SYSTEMCTL")
	if systemctl == "" {
		systemctl = "systemctl"
	}
	journalctl := os.Getenv("HARNESSRELAY_JOURNALCTL")
	if journalctl == "" {
		journalctl = "journalctl"
	}
	return &Manager{
		UnitPath: unitPath, DaemonPath: daemonPath,
		Systemctl: systemctl, Journalctl: journalctl, Runner: execRunner{},
	}, nil
}

// DefaultUnitPath returns the rootless systemd user-unit destination.
func DefaultUnitPath() (string, error) {
	if override := os.Getenv("HARNESSRELAY_SERVICE_UNIT_PATH"); override != "" {
		return filepath.Abs(override)
	}
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "systemd", "user", UnitName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("service: determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "systemd", "user", UnitName), nil
}

// ResolveDaemonPath finds the harnessd installed alongside harnessctl, with a
// test/packaging override and PATH fallback.
func ResolveDaemonPath() (string, error) {
	if override := os.Getenv("HARNESSRELAY_DAEMON_BINARY"); override != "" {
		return validateDaemonPath(override)
	}
	if current, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(current), "harnessd")
		if path, err := validateDaemonPath(sibling); err == nil {
			return path, nil
		}
	}
	path, err := exec.LookPath("harnessd")
	if err != nil {
		return "", errors.New("service: harnessd is not installed alongside harnessctl or available from PATH")
	}
	return validateDaemonPath(path)
}

func validateDaemonPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("service: resolve harnessd path: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("service: resolve harnessd %s: %w", absolute, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("service: inspect harnessd %s: %w", resolved, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("service: harnessd is not an executable regular file: %s", resolved)
	}
	return resolved, nil
}

// Unit renders the complete owned unit file.
func Unit(daemonPath string) (string, error) {
	path, err := validateDaemonPath(daemonPath)
	if err != nil {
		return "", err
	}
	return managedMarker + `
[Unit]
Description=HarnessRelay local harness daemon

[Service]
Type=exec
ExecStart=` + unitQuote(path) + ` serve
Restart=on-failure
RestartSec=2s

[Install]
WantedBy=default.target
`, nil
}

func unitQuote(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + replacer.Replace(value) + `"`
}

// Install creates the owned unit and reloads the user manager. It deliberately
// does not start or enable the service.
func (m *Manager) Install(ctx context.Context, stdout, stderr io.Writer) error {
	if m.DaemonPath == "" {
		var err error
		m.DaemonPath, err = ResolveDaemonPath()
		if err != nil {
			return err
		}
	}
	content, err := Unit(m.DaemonPath)
	if err != nil {
		return err
	}
	if existing, err := os.ReadFile(m.UnitPath); err == nil {
		if !IsManaged(existing) {
			return fmt.Errorf("service: refusing to overwrite unmanaged unit %s", m.UnitPath)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("service: inspect unit %s: %w", m.UnitPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(m.UnitPath), 0o755); err != nil {
		return fmt.Errorf("service: create user unit directory: %w", err)
	}
	if err := writeAtomic(m.UnitPath, []byte(content), 0o644); err != nil {
		return err
	}
	if err := m.systemctl(ctx, stdout, stderr, "daemon-reload"); err != nil {
		return fmt.Errorf("service: unit installed at %s but daemon-reload failed: %w", m.UnitPath, err)
	}
	return nil
}

// Uninstall stops/disables and removes only an owned HarnessRelay unit.
func (m *Manager) Uninstall(ctx context.Context, stdout, stderr io.Writer) error {
	content, err := os.ReadFile(m.UnitPath)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("service: unit is not installed: %s", m.UnitPath)
	}
	if err != nil {
		return fmt.Errorf("service: read unit %s: %w", m.UnitPath, err)
	}
	if !IsManaged(content) {
		return fmt.Errorf("service: refusing to remove unmanaged unit %s", m.UnitPath)
	}
	if err := m.systemctl(ctx, stdout, stderr, "disable", "--now", UnitName); err != nil {
		return fmt.Errorf("service: could not stop and disable %s; unit was preserved: %w", UnitName, err)
	}
	if err := os.Remove(m.UnitPath); err != nil {
		return fmt.Errorf("service: remove owned unit %s: %w", m.UnitPath, err)
	}
	if err := m.systemctl(ctx, stdout, stderr, "daemon-reload"); err != nil {
		return fmt.Errorf("service: unit removed but daemon-reload failed: %w", err)
	}
	return nil
}

// IsManaged reports whether content has HarnessRelay's exact ownership marker.
func IsManaged(content []byte) bool {
	first, _, _ := bytes.Cut(content, []byte("\n"))
	return string(first) == managedMarker
}

func (m *Manager) Start(ctx context.Context, stdout, stderr io.Writer) error {
	return m.systemctl(ctx, stdout, stderr, "start", UnitName)
}

func (m *Manager) Stop(ctx context.Context, stdout, stderr io.Writer) error {
	return m.systemctl(ctx, stdout, stderr, "stop", UnitName)
}

func (m *Manager) Restart(ctx context.Context, stdout, stderr io.Writer) error {
	return m.systemctl(ctx, stdout, stderr, "restart", UnitName)
}

func (m *Manager) Enable(ctx context.Context, stdout, stderr io.Writer) error {
	return m.systemctl(ctx, stdout, stderr, "enable", UnitName)
}

func (m *Manager) Disable(ctx context.Context, stdout, stderr io.Writer) error {
	return m.systemctl(ctx, stdout, stderr, "disable", UnitName)
}

func (m *Manager) Status(ctx context.Context, stdout, stderr io.Writer) error {
	return m.systemctl(ctx, stdout, stderr, "status", "--no-pager", UnitName)
}

func (m *Manager) Logs(ctx context.Context, stdout, stderr io.Writer) error {
	return m.run(ctx, m.Journalctl, []string{"--user", "--unit", UnitName, "--no-pager"}, stdout, stderr)
}

func (m *Manager) systemctl(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	return m.run(ctx, m.Systemctl, append([]string{"--user"}, args...), stdout, stderr)
}

func (m *Manager) run(ctx context.Context, name string, args []string, stdout, stderr io.Writer) error {
	if m.Runner == nil {
		return errors.New("service: command runner is unavailable")
	}
	if err := m.Runner.Run(ctx, name, args, stdout, stderr); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func writeAtomic(path string, content []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".harnessrelay-service-*")
	if err != nil {
		return fmt.Errorf("service: create temporary unit: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("service: set temporary unit mode: %w", err)
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("service: write temporary unit: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("service: sync temporary unit: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("service: close temporary unit: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("service: install unit %s: %w", path, err)
	}
	return nil
}
