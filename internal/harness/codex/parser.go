package codex

import (
	"regexp"
	"strings"
	"sync"
	"sync/atomic"

	xterm "github.com/gitpod-io/xterm-go"
	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/harness"
)

var (
	csiPattern      = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	oscPattern      = regexp.MustCompile(`\x1b\][^\x07]*(?:\x07|\x1b\\)`)
	modelPattern    = regexp.MustCompile(`(?i)model:\s*([a-z0-9][a-z0-9._+-]*(?:\s+(?:low|medium|high|xhigh|max|ultra))?)`)
	footerModel     = regexp.MustCompile(`(?m)^\s*([a-z0-9][a-z0-9._+-]*(?:\s+(?:low|medium|high|xhigh|max|ultra))?)\s+·\s+`)
	versionPattern  = regexp.MustCompile(`OpenAI Codex\s*\(v([^)]+)\)`)
	commandPattern  = regexp.MustCompile(`(?m)^\$\s+([^\r\n]+)`)
	keyboardPattern = regexp.MustCompile(`\x1b\[([><=])([0-9]*)u`)
)

// Parser keeps duplicate-suppression state for one Codex session.
type Parser struct {
	mu             sync.Mutex
	announcedUI    bool
	lastStatus     string
	lastMetadata   events.HarnessMetadata
	recent         string
	approvalOpen   bool
	announcedTrust bool
	kittyKeyboard  atomic.Bool
	screen         *xterm.Terminal
	screenRows     uint16
	screenCols     uint16
	pendingPrompt  string
	turn           uint64
	lastAssistant  string
	screenFailed   bool
}

func (p *Parser) Process(update harness.TerminalUpdate) []events.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.trackKeyboardProtocol(update.Chunk)
	screenFailed := p.updateScreen(update)
	snapshot := cleanTerminalText(string(update.Snapshot))
	chunk := cleanTerminalText(string(update.Chunk))
	p.recent += chunk
	if len(p.recent) > 32*1024 {
		p.recent = p.recent[len(p.recent)-32*1024:]
	}
	var out []events.Event
	if screenFailed {
		out = append(out, events.Event{
			Type: events.TypeAdapterWarning,
			Data: events.AdapterNotice{
				Description: "Codex response projection was disabled after the terminal screen model rejected an update. Raw output remains available in Terminal Mode.",
				Source:      adapterID,
			},
		})
	}

	if !p.announcedUI && (strings.Contains(snapshot, "OpenAI Codex") || strings.Contains(string(update.Snapshot), "\x1b[>7u")) {
		p.announcedUI = true
		out = append(out,
			events.Event{
				Type: events.TypeTerminalNoisyOutput,
				Data: events.TerminalNoiseSuppressed{Reason: "Codex terminal UI redraw is available in Terminal Mode"},
			},
			events.Event{
				Type: events.TypeChatSystemMessage,
				Data: events.ChatMessage{
					Role:       "system",
					Content:    "Codex is running in a terminal interface. HarnessRelay is using the Codex semantic adapter; raw output remains available in Terminal Mode.",
					Source:     adapterID,
					Confidence: 1,
				},
			},
		)
	}

	metadataText := snapshot
	if p.screen != nil {
		metadataText += "\n" + p.screen.String()
	}
	if metadata, ok := parseMetadata(metadataText, update.WorkDir); ok && metadata != p.lastMetadata {
		p.lastMetadata = metadata
		out = append(out, events.Event{Type: events.TypeHarnessMetadata, Data: metadata})
	}

	if strings.Contains(p.recent, "Do you trust the contents of this directory?") {
		if !p.announcedTrust {
			p.announcedTrust = true
			out = append(out,
				p.status("waiting_for_terminal", "Codex requires a workspace trust decision in Terminal Mode.", 0.95),
				events.Event{
					Type: events.TypeApprovalRequired,
					Data: events.ApprovalRequired{
						OperationKind:    "workspace_trust",
						WorkingDirectory: update.WorkDir,
						Prompt:           "Codex is asking whether to trust this workspace. Review and decide in Terminal Mode.",
						Confidence:       0.95,
						Actions: []events.SemanticAction{
							{
								ID:              "open_terminal",
								Label:           "Open Terminal",
								Kind:            "ui",
								Style:           "secondary",
								RequiresEventID: true,
								Version:         1,
							},
						},
					},
				},
			)
		}
		return compactEvents(out)
	}

	if strings.Contains(p.recent, "Would you like to run the following command?") {
		command := parseCommand(p.recent)
		if !p.approvalOpen {
			p.approvalOpen = true
			out = append(out,
				p.status("waiting_for_approval", "Codex is waiting for an explicit decision.", 0.95),
				events.Event{
					Type: events.TypeApprovalRequired,
					Data: events.ApprovalRequired{
						OperationKind:    "shell_command",
						Command:          command,
						WorkingDirectory: update.WorkDir,
						Prompt:           "Codex is asking whether it may run this command.",
						Confidence:       0.95,
						Actions: []events.SemanticAction{
							{
								ID:              "codex.approval_deny",
								Label:           "Deny",
								Kind:            "approval",
								Style:           "secondary",
								RequiresEventID: true,
								Version:         1,
							},
							{
								ID:              "open_terminal",
								Label:           "Open Terminal",
								Kind:            "ui",
								Style:           "secondary",
								RequiresEventID: true,
								Version:         1,
							},
						},
					},
				},
			)
		}
		return compactEvents(out)
	}

	if strings.Contains(chunk, "Working") || strings.Contains(chunk, "Starting MCP server") || strings.Contains(chunk, "Booting MCP server") {
		out = append(out, p.status("processing", "Codex is processing terminal activity.", 0.8))
	} else if p.announcedUI && p.lastStatus == "" {
		out = append(out, p.status("terminal_ui_active", "Codex terminal interface detected.", 0.9))
	}

	return compactEvents(out)
}

// PromptBytes uses the currently active keyboard protocol, not a historical
// marker retained in terminal scrollback.
func (p *Parser) PromptBytes(text string, _ []byte) []byte {
	p.mu.Lock()
	p.beginTurn(text)
	p.mu.Unlock()
	if p.kittyKeyboard.Load() {
		return append([]byte(text), []byte(kittyEnter)...)
	}
	return append([]byte(text), '\r')
}

// PromptSequence keeps the submit key in a separate PTY write. Codex can treat
// a key sequence appended to pasted text in the same write as part of the
// composer input.
func (p *Parser) PromptSequence(text string, _ []byte) [][]byte {
	p.mu.Lock()
	p.beginTurn(text)
	p.mu.Unlock()
	key := []byte{'\r'}
	if p.kittyKeyboard.Load() {
		key = []byte(kittyEnter)
	}
	return [][]byte{[]byte(text), key}
}

// OnIdle extracts a completed response from the rendered Codex screen. The
// quiet-period callback avoids publishing every intermediate TUI redraw.
func (p *Parser) OnIdle() []events.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pendingPrompt == "" || p.screen == nil {
		return nil
	}
	content := extractAssistantResponse(p.screen, p.pendingPrompt, p.lastMetadata.Model)
	if content == "" || content == p.lastAssistant {
		return nil
	}
	p.lastAssistant = content
	return []events.Event{{
		Type: events.TypeChatAssistantMessage,
		Data: events.ChatMessage{
			MessageID:  "codex-turn-" + formatTurn(p.turn),
			Role:       "assistant",
			Content:    content,
			Source:     adapterID,
			Confidence: 0.9,
		},
	}}
}

func (p *Parser) beginTurn(text string) {
	p.pendingPrompt = strings.TrimSpace(text)
	p.turn++
	p.lastAssistant = ""
}

func (p *Parser) updateScreen(update harness.TerminalUpdate) (failed bool) {
	if p.screenFailed {
		return false
	}
	defer func() {
		if recover() != nil {
			p.screen = nil
			p.screenFailed = true
			failed = true
		}
	}()
	rows, cols := update.Rows, update.Cols
	if rows == 0 {
		rows = 24
	} else if rows > 500 {
		rows = 500
	}
	if cols == 0 {
		cols = 80
	} else if cols > 1000 {
		cols = 1000
	}
	if p.screen == nil {
		p.screen = xterm.New(
			xterm.WithRows(int(rows)),
			xterm.WithCols(int(cols)),
			xterm.WithScrollback(2000),
		)
		p.screenRows = rows
		p.screenCols = cols
	} else if rows != p.screenRows || cols != p.screenCols {
		p.screen.Resize(int(cols), int(rows))
		p.screenRows = rows
		p.screenCols = cols
	}
	_, _ = p.screen.Write(update.Chunk)
	return false
}

func (p *Parser) trackKeyboardProtocol(chunk []byte) {
	for _, match := range keyboardPattern.FindAllSubmatch(chunk, -1) {
		switch string(match[1]) {
		case ">":
			p.kittyKeyboard.Store(string(match[2]) != "" && string(match[2]) != "0")
		case "<":
			p.kittyKeyboard.Store(false)
		case "=":
			p.kittyKeyboard.Store(string(match[2]) != "" && string(match[2]) != "0")
		}
	}
}

// ActionResolved drops the captured approval overlay so an identical later
// request can produce a new event.
func (p *Parser) ActionResolved(string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.approvalOpen = false
	p.recent = ""
}

func (p *Parser) status(status, detail string, confidence float64) events.Event {
	if p.lastStatus == status {
		return events.Event{}
	}
	p.lastStatus = status
	return events.Event{
		Type: events.TypeHarnessStatus,
		Data: events.HarnessStatus{
			Status:     status,
			Detail:     detail,
			Confidence: confidence,
		},
	}
}

func parseMetadata(text, fallbackWorkDir string) (events.HarnessMetadata, bool) {
	metadata := events.HarnessMetadata{WorkDir: fallbackWorkDir, Confidence: 0.8}
	if match := versionPattern.FindStringSubmatch(text); len(match) == 2 {
		metadata.Version = strings.TrimSpace(match[1])
	}
	if match := modelPattern.FindStringSubmatch(text); len(match) == 2 {
		metadata.Model = strings.TrimSpace(match[1])
	}
	if metadata.Model == "" {
		if match := footerModel.FindStringSubmatch(text); len(match) == 2 {
			metadata.Model = strings.TrimSpace(match[1])
		}
	}
	if metadata.Model == "" && metadata.Version == "" {
		return events.HarnessMetadata{}, false
	}
	return metadata, true
}

func parseCommand(text string) string {
	matches := commandPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.TrimSpace(matches[len(matches)-1][1])
}

func cleanTerminalText(value string) string {
	value = oscPattern.ReplaceAllString(value, "")
	value = csiPattern.ReplaceAllString(value, "")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 0x20 {
			return r
		}
		return -1
	}, value)
	return value
}

func compactEvents(input []events.Event) []events.Event {
	out := input[:0]
	for _, event := range input {
		if event.Type != "" {
			out = append(out, event)
		}
	}
	return out
}

type renderedLine struct {
	text    string
	wrapped bool
}

func extractAssistantResponse(term *xterm.Terminal, prompt, model string) string {
	lines := logicalTerminalLines(term)
	promptLine := firstPromptLine(prompt)
	if promptLine == "" {
		return ""
	}

	response := ""
	for index, line := range lines {
		content, ok := promptContent(line)
		if ok && matchingPromptLine(content, promptLine) {
			if candidate := assistantResponseAfter(lines, index, model); candidate != "" {
				response = candidate
			}
		}
	}
	return response
}

func assistantResponseAfter(lines []string, promptIndex int, model string) string {
	assistantIndex := -1
	for index := promptIndex + 1; index < len(lines); index++ {
		trimmed := strings.TrimSpace(lines[index])
		if _, ok := promptContent(lines[index]); ok {
			break
		}
		if strings.HasPrefix(trimmed, "• ") {
			assistantIndex = index
		}
	}
	if assistantIndex < 0 {
		return ""
	}

	var output []string
	for index := assistantIndex; index < len(lines); index++ {
		line := strings.TrimRight(lines[index], " ")
		trimmed := strings.TrimSpace(line)
		if index > assistantIndex {
			if _, ok := promptContent(line); ok {
				break
			}
			if isModelFooter(trimmed, model) {
				break
			}
			if isCodexNoticeBoundary(trimmed) {
				break
			}
			if trimmed != "" && !strings.HasPrefix(line, "  ") {
				break
			}
		}
		if index == assistantIndex {
			line = strings.TrimSpace(strings.TrimPrefix(trimmed, "•"))
		} else {
			line = strings.TrimPrefix(line, "  ")
		}
		output = append(output, line)
	}

	content := strings.TrimSpace(strings.Join(output, "\n"))
	if isTransientAssistantText(content) {
		return ""
	}
	return content
}

func logicalTerminalLines(term *xterm.Terminal) []string {
	buffer := term.Buffer()
	lines := make([]renderedLine, 0, buffer.Lines.Length())
	for index := 0; index < buffer.Lines.Length(); index++ {
		line := buffer.Lines.Get(index)
		if line == nil {
			continue
		}
		lines = append(lines, renderedLine{
			text:    line.TranslateToString(true, 0, -1),
			wrapped: line.IsWrapped,
		})
	}

	logical := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.wrapped && len(logical) > 0 {
			logical[len(logical)-1] += line.text
			continue
		}
		logical = append(logical, line.text)
	}
	return logical
}

func firstPromptLine(prompt string) string {
	for _, line := range strings.Split(strings.ReplaceAll(prompt, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

func promptContent(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "›") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(trimmed, "›")), true
}

func matchingPromptLine(rendered, submitted string) bool {
	if rendered == "" || submitted == "" {
		return false
	}
	return rendered == submitted ||
		strings.HasPrefix(rendered, submitted) ||
		strings.HasPrefix(submitted, rendered)
}

func isModelFooter(line, model string) bool {
	if !strings.Contains(line, " · ") {
		return false
	}
	return model != "" && strings.HasPrefix(line, model) ||
		strings.HasPrefix(line, "gpt-")
}

func isTransientAssistantText(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	return lower == "" ||
		strings.HasPrefix(lower, "working") ||
		strings.HasPrefix(lower, "thinking") ||
		strings.Contains(lower, "esc to interrupt")
}

func isCodexNoticeBoundary(content string) bool {
	content = strings.TrimSpace(content)
	return strings.HasPrefix(content, "Approaching rate limits") ||
		strings.HasPrefix(content, "Tip:") ||
		strings.HasPrefix(content, "Heads up,") ||
		strings.Contains(content, "usage limit reset available") ||
		strings.HasPrefix(content, "MCP client for ") ||
		strings.HasPrefix(content, "MCP startup incomplete")
}

func formatTurn(turn uint64) string {
	const digits = "0123456789"
	if turn == 0 {
		return "0"
	}
	var buffer [20]byte
	index := len(buffer)
	for turn > 0 {
		index--
		buffer[index] = digits[turn%10]
		turn /= 10
	}
	return string(buffer[index:])
}
