package opencode

import (
	"errors"
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
	versionPattern  = regexp.MustCompile(`(?m)(\d+\.\d+\.\d+)\s*$`)
	keyboardPattern = regexp.MustCompile(`\x1b\[([><=])([0-9]*)u`)

	// OpenCode-specific patterns
	opencodeBanner      = "▄"
	opencodeBannerRight = "█▀▀█ █▀▀█"
	permissionHeaderPattern = regexp.MustCompile(`(?i)[▲△⚠]\s*Permission\s*required`)
	permissionToolLine      = regexp.MustCompile(`(?m)(?:[┃╹]\s*)?#\s*(.+)$`)
	permissionCmdLine       = regexp.MustCompile(`(?m)(?:[┃╹]\s*)?\$\s*(.+)$`)
	allowOnceText           = "Allow once"
	allowAlwaysText         = "Allow always"
	rejectText              = "Reject"
	thinkingText            = "Thinking"
	thoughtText             = "Thought"
	writingText             = "Writing command"
	interruptText           = "esc interrupt"
	askAnythingPrefix       = "Ask anything..."
	modelFooterPattern      = regexp.MustCompile(`(?m)(?:[┃╹■]\s*)?(?:Build|Plan)\s*·\s*(.+)$`)
)

// Parser keeps duplicate-suppression state for one OpenCode session.
type Parser struct {
	mu            sync.Mutex
	announcedUI   bool
	lastStatus    string
	lastMetadata  events.HarnessMetadata
	recent        string
	approvalOpen  bool
	kittyKeyboard atomic.Bool
	screen        *xterm.Terminal
	screenRows    uint16
	screenCols    uint16
	pendingPrompt string
	turn          uint64
	lastAssistant string
	screenFailed  bool

	// Command discovery
	commandsInitialized bool
	discoveredCommands  []harness.CommandDescriptor
	lastDiscoveryOutput string
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
				Description: "OpenCode response projection was disabled after the terminal screen model rejected an update. Raw output remains available in Terminal Mode.",
				Source:      adapterID,
			},
		})
	}

	p.ensureCommandsInitialized()
	p.discoverCommandsFromOutput(snapshot)

	// Detect OpenCode UI from banner or version string
	screenText := ""
	if p.screen != nil {
		screenText = p.screen.String()
	}
	combined := snapshot + "\n" + screenText

	if !p.announcedUI && p.detectOpenCodeUI(combined) {
		p.announcedUI = true
		out = append(out,
			events.Event{
				Type: events.TypeTerminalNoisyOutput,
				Data: events.TerminalNoiseSuppressed{Reason: "OpenCode terminal UI redraw is available in Terminal Mode"},
			},
			events.Event{
				Type: events.TypeChatSystemMessage,
				Data: events.ChatMessage{
					Role:       "system",
					Content:    "OpenCode is running in a terminal interface. HarnessRelay is using the OpenCode semantic adapter; raw output remains available in Terminal Mode.",
					Source:     adapterID,
					Confidence: 1,
				},
			},
		)
	}

	// Parse metadata (model, version)
	if metadata, ok := p.parseMetadata(combined, update.WorkDir); ok && metadata != p.lastMetadata {
		p.lastMetadata = metadata
		out = append(out, events.Event{Type: events.TypeHarnessMetadata, Data: metadata})
	}

	// Detect permission prompts
	approvalText := p.approvalText()
	if permissionHeaderPattern.MatchString(approvalText) {
		toolName, command := p.parsePermissionPrompt(approvalText)
		if toolName == "" && command == "" {
			toolName = "Unknown operation"
		}
		if !p.approvalOpen {
			p.approvalOpen = true
			operationKind := "tool_call"
			if toolName == "Shell command" {
				operationKind = "shell_command"
			} else if toolName == "Edit" || toolName == "Write" {
				operationKind = "file_edit"
			}
			out = append(out,
				p.status("waiting_for_approval", "OpenCode is waiting for a permission decision.", 0.95),
				events.Event{
					Type: events.TypeApprovalRequired,
					Data: events.ApprovalRequired{
						OperationKind:    operationKind,
						ToolName:         toolName,
						Command:          command,
						WorkingDirectory: update.WorkDir,
						AdapterSource:    adapterID,
						Prompt:           "OpenCode is asking for permission to perform this operation.",
						Confidence:       0.95,
						BlocksPrompt:     true,
						Actions: []events.SemanticAction{
							{
								ID:              "opencode.approval_allow",
								Label:           "Allow once",
								Kind:            "approval",
								Style:           "primary",
								RequiresEventID: true,
								Version:         1,
							},
							{
								ID:              "opencode.approval_allow_always",
								Label:           "Allow always",
								Kind:            "approval",
								Style:           "secondary",
								RequiresEventID: true,
								Version:         1,
							},
							{
								ID:              "opencode.approval_deny",
								Label:           "Reject",
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

	// Detect status from activity indicators
	if strings.Contains(chunk, thinkingText) || strings.Contains(chunk, writingText) || strings.Contains(chunk, interruptText) {
		out = append(out, p.status("processing", "OpenCode is processing.", 0.8))
	} else if p.announcedUI && p.lastStatus == "" {
		out = append(out, p.status("terminal_ui_active", "OpenCode terminal interface detected.", 0.9))
	}

	return compactEvents(out)
}

// PromptBytes uses the currently active keyboard protocol.
func (p *Parser) PromptBytes(text string, _ []byte) []byte {
	p.mu.Lock()
	p.beginTurn(text)
	p.mu.Unlock()
	if p.kittyKeyboard.Load() {
		return append([]byte(text), []byte(kittyEnter)...)
	}
	return append([]byte(text), '\r')
}

// PromptSequence keeps the submit key in a separate PTY write.
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

// CommandCatalog returns the catalog of available commands.
func (p *Parser) CommandCatalog() []harness.CommandDescriptor {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureCommandsInitialized()
	return append([]harness.CommandDescriptor(nil), p.discoveredCommands...)
}

// CommandSequence builds a catalog-validated command without opening an agent turn.
func (p *Parser) CommandSequence(commandID, arguments string) ([][]byte, harness.CommandDescriptor, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureCommandsInitialized()

	var command harness.CommandDescriptor
	found := false
	for _, candidate := range p.discoveredCommands {
		if candidate.ID == commandID {
			command = candidate
			found = true
			break
		}
	}
	if !found {
		return nil, harness.CommandDescriptor{}, errors.New("unknown command")
	}

	arguments = strings.TrimSpace(arguments)
	text := command.Invocation
	if arguments != "" {
		text += " " + arguments
	}

	if command.Interaction == harness.CommandPrefillTerminal {
		return [][]byte{[]byte(text)}, command, nil
	}

	key := []byte{'\r'}
	if p.kittyKeyboard.Load() {
		key = []byte(kittyEnter)
	}
	return [][]byte{[]byte(text), key}, command, nil
}

// OnIdle extracts a completed response from the rendered OpenCode screen.
func (p *Parser) OnIdle() []events.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pendingPrompt == "" || p.screen == nil || p.approvalOpen {
		return nil
	}
	content := extractAssistantResponse(p.screen, p.pendingPrompt)
	if content == "" || content == p.lastAssistant {
		return nil
	}
	p.lastAssistant = content
	return []events.Event{{
		Type: events.TypeChatAssistantMessage,
		Data: events.ChatMessage{
			MessageID:  "opencode-turn-" + formatTurn(p.turn),
			Role:       "assistant",
			Content:    content,
			Source:     adapterID,
			Confidence: 0.85,
		},
	}}
}

func (p *Parser) beginTurn(text string) {
	p.pendingPrompt = strings.TrimSpace(text)
	p.turn++
	p.lastAssistant = ""
}

func (p *Parser) detectOpenCodeUI(text string) bool {
	return strings.Contains(text, opencodeBanner) && strings.Contains(text, opencodeBannerRight)
}

func (p *Parser) parseMetadata(text, fallbackWorkDir string) (events.HarnessMetadata, bool) {
	metadata := events.HarnessMetadata{WorkDir: fallbackWorkDir, Confidence: 0.8}

	// Extract version from footer line (e.g. "1.18.3" at end of a line)
	if matches := versionPattern.FindStringSubmatch(text); len(matches) == 2 {
		metadata.Version = strings.TrimSpace(matches[1])
	}

	// Extract model from status bar (e.g. "Build · Kimi K2.6 Canopy Wave Coding Plan")
	if matches := modelFooterPattern.FindStringSubmatch(text); len(matches) == 2 {
		metadata.Model = strings.TrimSpace(matches[1])
	}

	if metadata.Model == "" && metadata.Version == "" {
		return events.HarnessMetadata{}, false
	}
	return metadata, true
}

func (p *Parser) parsePermissionPrompt(text string) (toolName, command string) {
	// Extract tool name from "# ToolName" line
	if matches := permissionToolLine.FindStringSubmatch(text); len(matches) == 2 {
		toolName = strings.TrimSpace(matches[1])
	}
	// Extract command from "$ command" line
	if matches := permissionCmdLine.FindStringSubmatch(text); len(matches) == 2 {
		command = strings.TrimSpace(matches[1])
	}
	return toolName, command
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
			mode := string(match[2])
			// OpenCode uses >4;1m - flag 1 means CSI u encoding is active.
			parts := strings.SplitN(mode, ";", 2)
			if len(parts) == 2 {
				p.kittyKeyboard.Store(parts[1] != "" && parts[1] != "0")
			} else {
				p.kittyKeyboard.Store(mode != "" && mode != "0")
			}
		case "<":
			p.kittyKeyboard.Store(false)
		case "=":
			p.kittyKeyboard.Store(string(match[2]) != "" && string(match[2]) != "0")
		}
	}
}

// ActionResolved drops the captured approval overlay.
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

func (p *Parser) approvalText() string {
	if p.screen != nil {
		return p.recent + "\n" + p.screen.String()
	}
	return p.recent
}

// ensureCommandsInitialized initializes the command catalog with OpenCode's built-in commands.
func (p *Parser) ensureCommandsInitialized() {
	if p.commandsInitialized {
		return
	}
	p.discoveredCommands = append([]harness.CommandDescriptor(nil), opencodeCommands...)
	p.commandsInitialized = true
}

// discoverCommandsFromOutput parses terminal output for command patterns.
func (p *Parser) discoverCommandsFromOutput(snapshot string) {
	if snapshot == p.lastDiscoveryOutput {
		return
	}
	p.lastDiscoveryOutput = snapshot

	lines := strings.Split(snapshot, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if command := parseCommandFromOutput(line); command != nil {
			p.addDiscoveredCommand(command)
		}
	}

	if strings.Contains(snapshot, "Available commands") || strings.Contains(snapshot, "Commands:") {
		p.discoverCommandsFromHelp(snapshot)
	}
}

func (p *Parser) discoverCommandsFromHelp(helpText string) {
	lines := strings.Split(helpText, "\n")
	inCommandsSection := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Available commands") || strings.Contains(line, "Commands:") {
			inCommandsSection = true
			continue
		}
		if inCommandsSection {
			if line == "" {
				break
			}
			if command := parseCommandFromOutput(line); command != nil {
				p.addDiscoveredCommand(command)
			}
		}
	}
}

func (p *Parser) addDiscoveredCommand(command *harness.CommandDescriptor) {
	for i, existing := range p.discoveredCommands {
		if existing.ID == command.ID {
			if command.Description != "" {
				p.discoveredCommands[i].Description = command.Description
			}
			return
		}
	}
	p.discoveredCommands = append(p.discoveredCommands, *command)
}

func parseCommandFromOutput(line string) *harness.CommandDescriptor {
	if len(line) < 2 || line[0] != '/' {
		return nil
	}
	if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/ ") {
		return nil
	}
	parts := strings.SplitN(line, " ", 2)
	if len(parts) < 1 {
		return nil
	}
	invocation := strings.TrimSpace(parts[0])
	if !strings.HasPrefix(invocation, "/") || len(invocation) < 2 {
		return nil
	}
	name := strings.TrimPrefix(invocation, "/")
	if !isValidCommandName(name) {
		return nil
	}
	description := ""
	if len(parts) > 1 {
		desc := strings.TrimSpace(parts[1])
		desc = strings.TrimLeft(desc, "- ")
		if len(desc) > 0 {
			description = desc
		}
	}
	return &harness.CommandDescriptor{
		ID:          name,
		Invocation:  invocation,
		Label:       formatCommandLabel(name),
		Description: description,
		Group:       "Discovered",
		Interaction: harness.CommandSubmit,
	}
}

func isValidCommandName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func formatCommandLabel(name string) string {
	words := strings.Split(name, "-")
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

// extractAssistantResponse extracts the assistant's response from the rendered screen.
// OpenCode uses a full-screen TUI with ┃ borders framing the chat content.
func extractAssistantResponse(term *xterm.Terminal, prompt string) string {
	buffer := term.Buffer()
	lines := make([]string, 0, buffer.Lines.Length())
	for index := 0; index < buffer.Lines.Length(); index++ {
		line := buffer.Lines.Get(index)
		if line == nil {
			continue
		}
		lines = append(lines, line.TranslateToString(true, 0, -1))
	}

	// Look for the last user prompt match, then find the assistant response after it.
	promptTrimmed := firstPromptLine(prompt)
	if promptTrimmed == "" {
		return ""
	}

	response := ""
	for index, line := range lines {
		content, ok := borderedContent(line)
		if !ok {
			continue
		}
		innerText := stripBorder(content)
		innerText = stripSidebar(innerText)
		trimmed := strings.TrimSpace(innerText)
		if trimmed == "" {
			continue
		}
		isMatch := false
		if trimmed == promptTrimmed || strings.HasPrefix(trimmed, promptTrimmed) {
			isMatch = true
		} else if strings.HasPrefix(promptTrimmed, trimmed) {
			// Only allow prefix-of-prompt matching for substantial lines
			// to avoid false matches from short fragments or wrapped lines.
			if len(trimmed) >= 20 || len(promptTrimmed) < 20 {
				isMatch = true
			}
		}
		if isMatch {
			if candidate := assistantResponseAfter(lines, index); candidate != "" {
				response = candidate
			}
		}
	}
	return response
}

func assistantResponseAfter(lines []string, promptIndex int) string {
	assistantIndex := -1
	for index := promptIndex + 1; index < len(lines); index++ {
		content, ok := borderedContent(lines[index])
		if !ok {
			continue
		}
		innerText := stripBorder(content)
		innerText = stripSidebar(innerText)
		trimmed := strings.TrimSpace(innerText)
		if trimmed == "" || isSidebarContent(trimmed) {
			continue
		}
		// Skip status/permission lines
		if strings.HasPrefix(trimmed, "△") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "$") {
			continue
		}
		if strings.Contains(trimmed, allowOnceText) || strings.Contains(trimmed, allowAlwaysText) || strings.Contains(trimmed, rejectText) {
			continue
		}
		// Skip "Thought" / "Thinking" indicators (OpenCode shows + Thought: Nms)
		if strings.Contains(trimmed, thinkingText) || strings.Contains(trimmed, thoughtText) || strings.Contains(trimmed, interruptText) {
			continue
		}
		// Found a non-empty content line after the prompt - this is the response start
		assistantIndex = index
		break
	}
	if assistantIndex < 0 {
		return ""
	}

	var output []string
	for index := assistantIndex; index < len(lines); index++ {
		content, ok := borderedContent(lines[index])
		if !ok {
			break
		}
		innerText := stripBorder(content)
		innerText = stripSidebar(innerText)
		trimmed := strings.TrimSpace(innerText)
		if trimmed == "" || isSidebarContent(trimmed) {
			continue
		}
		// Stop at the next prompt, status bar, or permission prompt
		if strings.HasPrefix(trimmed, "△") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "$") {
			break
		}
		if strings.Contains(trimmed, allowOnceText) || strings.Contains(trimmed, allowAlwaysText) || strings.Contains(trimmed, rejectText) {
			break
		}
		if strings.Contains(trimmed, thinkingText) || strings.Contains(trimmed, thoughtText) || strings.Contains(trimmed, interruptText) {
			break
		}
		if strings.HasPrefix(trimmed, "Ask anything") {
			break
		}
		// Check for mode/model footer
		if modelFooterPattern.MatchString(trimmed) {
			break
		}
		output = append(output, trimmed)
	}

	content := strings.TrimSpace(strings.Join(output, "\n"))
	if isTransientAssistantText(content) {
		return ""
	}
	return content
}

// borderedContent extracts content within OpenCode's ┃ bordered area.
func borderedContent(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	// OpenCode uses ┃ as the left border for chat content
	idx := strings.Index(trimmed, "┃")
	if idx >= 0 {
		return trimmed[idx:], true
	}
	// Also handle the corner pieces
	if strings.HasPrefix(trimmed, "┃") || strings.HasPrefix(trimmed, "╹") {
		return trimmed, true
	}
	return "", false
}

// stripBorder removes the OpenCode border character (┃, ╹) from the start of a line.
func stripBorder(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "┃") {
		return strings.TrimPrefix(trimmed, "┃")
	}
	if strings.HasPrefix(trimmed, "╹") {
		return strings.TrimPrefix(trimmed, "╹")
	}
	return trimmed
}

func firstPromptLine(prompt string) string {
	for _, line := range strings.Split(strings.ReplaceAll(prompt, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

func isTransientAssistantText(content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	return lower == "" ||
		strings.HasPrefix(lower, "thinking") ||
		strings.HasPrefix(lower, "thought") ||
		strings.HasPrefix(lower, "writing") ||
		strings.Contains(lower, "esc to interrupt")
}

// stripSidebar removes trailing sidebar content from an OpenCode TUI line.
// OpenCode renders a sidebar on the right separated from chat content by
// either a box-drawing character or a large whitespace gap.
func stripSidebar(line string) string {
	// Split on the sidebar separator character (│) if present.
	if idx := strings.Index(line, "│"); idx >= 0 {
		line = line[:idx]
	}
	// Also split on large whitespace gaps (6+ spaces) which separate
	// chat content from the sidebar.
	if match := sidebarGapPattern.FindStringIndex(line); match != nil {
		// Keep the non-space character before the gap.
		line = line[:match[0]+1]
	}
	return line
}

var sidebarGapPattern = regexp.MustCompile(`\S\s{6,}`)

// isSidebarContent reports whether text is known OpenCode sidebar chrome.
func isSidebarContent(text string) bool {
	switch text {
	case "Context", "Greeting", "LSP", "LSPs are disabled":
		return true
	}
	if sidebarTokenPattern.MatchString(text) {
		return true
	}
	if sidebarCostPattern.MatchString(text) {
		return true
	}
	if sidebarUsagePattern.MatchString(text) {
		return true
	}
	return false
}

var (
	sidebarTokenPattern = regexp.MustCompile(`^\d[\d,]*\s+tokens$`)
	sidebarCostPattern  = regexp.MustCompile(`^\$\d+\.\d+\s+spent$`)
	sidebarUsagePattern = regexp.MustCompile(`^\d+%\s+used$`)
)

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

// opencodeCommands are OpenCode's built-in slash commands.
var opencodeCommands = []harness.CommandDescriptor{
	{ID: "connect", Invocation: "/connect", Label: "Connect", Description: "Add a provider to OpenCode.", Group: "Configure", Interaction: harness.CommandSubmit},
	{ID: "compact", Invocation: "/compact", Label: "Compact", Description: "Compact the current session.", Group: "Conversation", Interaction: harness.CommandSubmit},
	{ID: "details", Invocation: "/details", Label: "Details", Description: "Toggle tool execution details.", Group: "Inspect", Interaction: harness.CommandSubmit},
	{ID: "editor", Invocation: "/editor", Label: "Editor", Description: "Open external editor for composing messages.", Group: "Conversation", Interaction: harness.CommandSubmit},
	{ID: "exit", Invocation: "/exit", Label: "Exit", Description: "Exit OpenCode.", Group: "Sensitive", Interaction: harness.CommandPrefillTerminal, Danger: true},
	{ID: "export", Invocation: "/export", Label: "Export", Description: "Export current conversation to Markdown.", Group: "Inspect", Interaction: harness.CommandSubmit},
	{ID: "help", Invocation: "/help", Label: "Help", Description: "Show the help dialog.", Group: "Inspect", Interaction: harness.CommandSubmit},
	{ID: "init", Invocation: "/init", Label: "Init", Description: "Guided setup for creating or updating AGENTS.md.", Group: "Configure", Interaction: harness.CommandSubmit},
	{ID: "models", Invocation: "/models", Label: "Models", Description: "List available models.", Group: "Configure", Interaction: harness.CommandSubmitThenTerminal},
	{ID: "new", Invocation: "/new", Label: "New", Description: "Start a new session.", Group: "Sensitive", Interaction: harness.CommandPrefillTerminal, Danger: true},
	{ID: "redo", Invocation: "/redo", Label: "Redo", Description: "Redo a previously undone message.", Group: "Conversation", Interaction: harness.CommandSubmit},
	{ID: "sessions", Invocation: "/sessions", Label: "Sessions", Description: "List and switch between sessions.", Group: "Inspect", Interaction: harness.CommandSubmitThenTerminal},
	{ID: "share", Invocation: "/share", Label: "Share", Description: "Share current session.", Group: "Inspect", Interaction: harness.CommandSubmit},
	{ID: "themes", Invocation: "/themes", Label: "Themes", Description: "List available themes.", Group: "Configure", Interaction: harness.CommandSubmitThenTerminal},
	{ID: "thinking", Invocation: "/thinking", Label: "Thinking", Description: "Toggle thinking block visibility.", Group: "Inspect", Interaction: harness.CommandSubmit},
	{ID: "undo", Invocation: "/undo", Label: "Undo", Description: "Undo last message and revert file changes.", Group: "Sensitive", Interaction: harness.CommandPrefillTerminal, Danger: true},
	{ID: "unshare", Invocation: "/unshare", Label: "Unshare", Description: "Unshare current session.", Group: "Inspect", Interaction: harness.CommandSubmit},
	{ID: "quit", Invocation: "/quit", Label: "Quit", Description: "Exit OpenCode.", Group: "Sensitive", Interaction: harness.CommandPrefillTerminal, Danger: true},
	{ID: "q", Invocation: "/q", Label: "Quit", Description: "Exit OpenCode.", Group: "Sensitive", Interaction: harness.CommandPrefillTerminal, Danger: true},
}
