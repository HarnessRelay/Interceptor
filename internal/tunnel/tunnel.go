package tunnel

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Status represents the current state of the tunnel.
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusError    Status = "error"
)

// Mode selects how cloudflared is run.
type Mode string

const (
	// ModeQuick runs a zero-config Quick Tunnel (random trycloudflare.com URL).
	ModeQuick Mode = "quick"
	// ModeToken runs a named, remotely-managed tunnel with a stable hostname.
	ModeToken Mode = "token"
)

// Config is the persisted tunnel configuration.
type Config struct {
	Mode     Mode   `json:"mode"`
	Token    string `json:"token,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}

// ConfigView is the API-facing configuration; the token is never returned.
type ConfigView struct {
	Mode     Mode   `json:"mode"`
	Hostname string `json:"hostname,omitempty"`
	TokenSet bool   `json:"token_set"`
}

// Info is the JSON-serialisable tunnel state returned by the API.
type Info struct {
	Status Status `json:"status"`
	URL    string `json:"url,omitempty"`
	Error  string `json:"error,omitempty"`
}

// ErrTunnelActive is returned when configuration is changed while the
// tunnel process is running or starting.
var ErrTunnelActive = errors.New("stop the tunnel before changing its configuration")

const (
	configFileName = "tunnel.json"
	logBufferLines = 300
	tokenRunMarker = "Registered tunnel connection"
)

// tunnelURLPattern matches the trycloudflare.com URL emitted by cloudflared.
var tunnelURLPattern = regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)

// Manager controls the lifecycle of a cloudflared tunnel process. The process
// is bound to the daemon-lifetime context captured at construction; it must
// never be tied to an HTTP request context, which is canceled as soon as the
// handler returns.
type Manager struct {
	mu  sync.Mutex
	cmd *exec.Cmd

	daemonCtx context.Context

	status  Status
	url     string
	lastErr string

	port      int
	configDir string
	cfg       Config
	logger    *slog.Logger
	logs      *logBuffer

	// binOverride forces the cloudflared binary path (test hook).
	binOverride string
}

// NewManager creates a tunnel manager for the daemon's HTTP port. The ctx is
// the daemon's shutdown context: canceling it terminates cloudflared.
func NewManager(ctx context.Context, port int, configDir string, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	m := &Manager{
		daemonCtx: ctx,
		status:    StatusStopped,
		port:      port,
		configDir: configDir,
		cfg:       Config{Mode: ModeQuick},
		logger:    logger,
		logs:      newLogBuffer(logBufferLines),
	}
	if err := m.loadConfig(); err != nil {
		logger.Warn("failed to load tunnel config; using defaults", slog.String("error", err.Error()))
	}
	return m
}

// Info returns the current tunnel state.
func (m *Manager) Info() Info {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Info{
		Status: m.status,
		URL:    m.url,
		Error:  m.lastErr,
	}
}

// Logs returns a copy of the recent cloudflared output lines.
func (m *Manager) Logs() []string {
	return m.logs.snapshot()
}

// ViewConfig returns the current configuration without the token value.
func (m *Manager) ViewConfig() ConfigView {
	m.mu.Lock()
	defer m.mu.Unlock()
	return ConfigView{
		Mode:     m.cfg.Mode,
		Hostname: m.cfg.Hostname,
		TokenSet: strings.TrimSpace(m.cfg.Token) != "",
	}
}

// UpdateConfig validates, persists, and applies a new configuration. An empty
// token keeps the previously stored token. Fails with ErrTunnelActive while
// the tunnel process is running or starting.
func (m *Manager) UpdateConfig(next Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if next.Mode == "" {
		next.Mode = ModeQuick
	}
	if strings.TrimSpace(next.Token) == "" {
		next.Token = m.cfg.Token
	}
	if err := validateConfig(next); err != nil {
		return err
	}
	if m.status == StatusRunning || m.status == StatusStarting {
		return ErrTunnelActive
	}
	if err := saveConfigFile(m.configPath(), next); err != nil {
		return err
	}
	m.cfg = next
	return nil
}

func validateConfig(c Config) error {
	switch c.Mode {
	case ModeQuick:
		return nil
	case ModeToken:
		if strings.TrimSpace(c.Token) == "" {
			return errors.New("token mode requires a tunnel token")
		}
		return nil
	default:
		return fmt.Errorf("unknown tunnel mode: %s", c.Mode)
	}
}

func (m *Manager) configPath() string {
	return filepath.Join(m.configDir, configFileName)
}

func (m *Manager) loadConfig() error {
	data, err := os.ReadFile(m.configPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	if cfg.Mode == "" {
		cfg.Mode = ModeQuick
	}
	if err := validateConfig(cfg); err != nil {
		return err
	}
	m.cfg = cfg
	return nil
}

func saveConfigFile(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tunnel.json-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// tunnelArgs builds the cloudflared argument list for the configured mode.
func (m *Manager) tunnelArgs() []string {
	if m.cfg.Mode == ModeToken {
		return []string{"tunnel", "--no-autoupdate", "run", "--token", m.cfg.Token}
	}
	return []string{"tunnel", "--no-autoupdate", "--url", fmt.Sprintf("http://127.0.0.1:%d", m.port)}
}

// resolveBinary returns the cloudflared binary to launch, preferring the
// test override, then env override, managed copy, PATH, and common paths.
// An override that does not exist is treated as "not found".
func (m *Manager) resolveBinary() string {
	if m.binOverride != "" {
		if fileExists(m.binOverride) {
			return m.binOverride
		}
		return ""
	}
	path, _ := ResolveBinary()
	return path
}

// Start launches cloudflared if it is not already running. It is safe to call
// multiple times; a second call while the tunnel is active is a no-op.
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.status {
	case StatusRunning, StatusStarting:
		return nil
	}

	binPath := m.resolveBinary()
	if binPath == "" {
		m.status = StatusError
		m.lastErr = "cloudflared not found"
		return errors.New("cloudflared not found")
	}

	cmd := exec.CommandContext(m.daemonCtx, binPath, m.tunnelArgs()...)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		m.status = StatusError
		m.lastErr = fmt.Sprintf("stderr pipe: %v", err)
		return err
	}

	if err := cmd.Start(); err != nil {
		m.status = StatusError
		m.lastErr = fmt.Sprintf("start: %v", err)
		return err
	}

	m.cmd = cmd
	m.status = StatusStarting
	m.lastErr = ""
	m.url = ""
	m.logs.reset()
	m.logs.add(fmt.Sprintf("starting cloudflared (pid %d): %s %s", cmd.Process.Pid, binPath, strings.Join(m.tunnelArgs(), " ")))

	mode := m.cfg.Mode
	hostname := m.cfg.Hostname
	pid := cmd.Process.Pid
	go m.supervise(stderr, mode, hostname)

	m.logger.Info("cloudflared tunnel starting",
		slog.String("mode", string(mode)),
		slog.Int("port", m.port),
		slog.Int("pid", pid),
	)
	return nil
}

// Stop terminates the cloudflared process if it is running. Recent logs are
// kept so the debug console still shows why a tunnel went away.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.status == StatusStopped {
		return nil
	}

	if m.cmd != nil && m.cmd.Process != nil {
		_ = m.cmd.Process.Kill()
	}

	m.status = StatusStopped
	m.url = ""
	m.lastErr = ""
	m.logger.Info("cloudflared tunnel stopped")
	return nil
}

// supervise drains cloudflared stderr (recording log lines and watching for
// the tunnel becoming active) and then reaps the process, recording why it
// exited. Draining before Wait avoids losing buffered stderr lines.
func (m *Manager) supervise(stderr io.Reader, mode Mode, hostname string) {
	m.drain(stderr, mode, hostname)

	err := m.cmd.Wait()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.status == StatusStopped {
		return
	}

	m.status = StatusError
	m.url = ""
	switch {
	case err != nil:
		m.lastErr = fmt.Sprintf("cloudflared exited: %v", err)
	default:
		m.lastErr = "cloudflared exited"
	}
	m.logs.add("tunnel process exited: " + m.lastErr)
	m.logger.Warn("cloudflared exited", slog.String("error", m.lastErr))
}

// drain reads cloudflared stderr lines into the log buffer and flips the
// manager from starting to running once the tunnel URL or a registered
// connection appears.
func (m *Manager) drain(stderr io.Reader, mode Mode, hostname string) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	announced := false
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		m.logs.add(line)

		if announced {
			continue
		}
		url := ""
		if mode != ModeToken {
			url = tunnelURLPattern.FindString(line)
		}
		if url != "" || strings.Contains(line, tokenRunMarker) {
			announced = true
			m.mu.Lock()
			if m.status == StatusStarting {
				m.status = StatusRunning
				switch {
				case mode == ModeToken:
					m.url = hostname // informational; the real URL lives in Cloudflare
				case url != "":
					m.url = url
				}
				m.logger.Info("cloudflared tunnel active", slog.String("url", m.url))
			}
			m.mu.Unlock()
		}
	}
}
