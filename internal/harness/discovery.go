package harness

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Definition describes a known terminal harness that can be launched directly.
type Definition struct {
	ID          string
	Name        string
	Command     string
	Args        []string
	DefaultMode string
	Description string
}

// Detected describes a known harness and whether it is available to launch.
type Detected struct {
	Definition
	Installed bool
	Path      string
	Version   string
}

// KnownDefinitions returns the stable catalog of harnesses the daemon can detect.
func KnownDefinitions() []Definition {
	return []Definition{
		{
			ID:          "codex",
			Name:        "Codex",
			Command:     "codex",
			DefaultMode: "chat",
			Description: "OpenAI Codex CLI",
		},
		{
			ID:          "opencode",
			Name:        "OpenCode",
			Command:     "opencode",
			DefaultMode: "chat",
			Description: "OpenCode terminal UI",
		},
		{
			ID:          "claude",
			Name:        "Claude Code",
			Command:     "claude",
			DefaultMode: "chat",
			Description: "Claude Code CLI",
		},
		{
			ID:          "aider",
			Name:        "Aider",
			Command:     "aider",
			DefaultMode: "chat",
			Description: "Aider terminal pair programmer",
		},
		{
			ID:          "gemini",
			Name:        "Gemini CLI",
			Command:     "gemini",
			DefaultMode: "chat",
			Description: "Gemini CLI interactive REPL",
		},
	}
}

// DiscoverInstalled returns known harnesses available on PATH.
func DiscoverInstalled(ctx context.Context) []Detected {
	return Discover(ctx, true)
}

// Discover returns known harnesses with PATH and best-effort version metadata.
func Discover(ctx context.Context, installedOnly bool) []Detected {
	defs := KnownDefinitions()
	out := make([]Detected, 0, len(defs))
	for _, def := range defs {
		detected := Detected{Definition: def}
		if path, err := exec.LookPath(def.Command); err == nil {
			detected.Installed = true
			detected.Path = path
			detected.Version = probeVersion(ctx, def.Command)
		}
		if detected.Installed || !installedOnly {
			out = append(out, detected)
		}
	}
	return out
}

func probeVersion(parent context.Context, command string) string {
	ctx, cancel := context.WithTimeout(parent, 1500*time.Millisecond)
	defer cancel()

	out, err := exec.CommandContext(ctx, command, "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	return firstLine(string(out))
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if index := strings.IndexAny(value, "\r\n"); index >= 0 {
		return strings.TrimSpace(value[:index])
	}
	return value
}
