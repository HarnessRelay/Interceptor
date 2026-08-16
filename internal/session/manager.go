package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/harness"
	"github.com/harnessrelay/interceptor/internal/harness/codex"
	"github.com/harnessrelay/interceptor/internal/harness/fakesemantic"
	"github.com/harnessrelay/interceptor/internal/harness/generic"
	"github.com/harnessrelay/interceptor/internal/harness/opencode"
	"github.com/harnessrelay/interceptor/internal/pty"
)

const defaultOutputBufferSize = 4 * 1024 * 1024
const semanticIdleDelay = 3 * time.Second
const promptSubmitKeyDelay = 100 * time.Millisecond

var (
	ErrStaleSemanticAction = errors.New("session: stale or unknown semantic action")
	ErrUnsupportedAction   = errors.New("session: semantic action is not supported")
	ErrApprovalPending     = errors.New("session: an approval decision is pending")
	ErrHarnessNotReady     = errors.New("session: harness is not ready for prompt input")
	ErrCommandUnsupported  = errors.New("session: harness command catalog is unavailable")
	ErrCommandUnknown      = errors.New("session: unknown harness command")
)

// Status represents the lifecycle state of a session.
type Status string

const (
	StatusStarting   Status = "starting"
	StatusRunning    Status = "running"
	StatusExited     Status = "exited"
	StatusFailed     Status = "failed"
	StatusTerminated Status = "terminated"
)

// CreateOptions describes how to create a new session.
type CreateOptions struct {
	Name          string
	HarnessType   string
	Command       string
	Args          []string
	WorkDir       string
	Env           []string
	Rows          uint16
	Cols          uint16
	Origin        string
	OriginBackend string
	ShimName      string
	RealBinary    string
	Attachable    bool
}

// TerminalSize describes a session terminal's character dimensions.
type TerminalSize struct {
	Rows uint16
	Cols uint16
}

// Info is a point-in-time, race-safe session metadata snapshot.
type Info struct {
	ID            string
	Name          string
	HarnessType   string
	AdapterID     string
	AdapterName   string
	Capabilities  []harness.Capability
	Command       string
	Args          []string
	WorkDir       string
	Status        Status
	PID           int
	PGID          int
	Terminal      TerminalSize
	StartedAt     time.Time
	ExitedAt      *time.Time
	ExitCode      *int
	Origin        string
	OriginBackend string
	ShimName      string
	RealBinary    string
	Attachable    bool
}

// Session holds metadata and runtime state for one session.
type Session struct {
	ID            string
	Name          string
	HarnessType   string
	AdapterID     string
	AdapterName   string
	Capabilities  []harness.Capability
	Command       string
	Args          []string
	WorkDir       string
	Status        Status
	PID           int
	PGID          int
	Rows          uint16
	Cols          uint16
	StartedAt     time.Time
	ExitedAt      *time.Time
	ExitCode      *int
	Origin        string
	OriginBackend string
	ShimName      string
	RealBinary    string
	Attachable    bool

	runtime           *pty.Runtime
	adapter           harness.Adapter
	parser            harness.Parser
	buf               *outputBuffer
	done              chan struct{}
	mu                sync.RWMutex
	inputMu           sync.Mutex
	parserMu          sync.Mutex
	semanticStatus    string
	semanticIdleTimer *time.Timer
	pendingAction     *pendingSemanticAction
	requestedExit     string
	outputDone        chan struct{}

	publish func(typ events.Type, data any) events.Event
	onExit  func()
}

type pendingSemanticAction struct {
	eventID string
	actions map[string]struct{}
}

// OutputChunk represents a chunk of PTY output data.
type OutputChunk struct {
	Data []byte
	Done bool
}

// Subscribe returns a channel that receives PTY output chunks.
// The first chunk is the current buffer snapshot (if any).
// When the session ends, a final chunk with Done=true is sent and the channel is closed.
// Callers that subscribe after the session has ended receive the snapshot (if any) followed
// immediately by Done=true.
func (s *Session) Subscribe() <-chan OutputChunk {
	ch := make(chan OutputChunk, 64)
	s.buf.addSub(ch)
	return ch
}

// Snapshot returns a copy of the current output buffer contents.
func (s *Session) Snapshot() []byte {
	return s.buf.snapshot()
}

// Info returns a point-in-time metadata snapshot for this session.
func (s *Session) Info() Info {
	s.mu.RLock()
	defer s.mu.RUnlock()
	args := make([]string, len(s.Args))
	copy(args, s.Args)
	var exitedAt *time.Time
	if s.ExitedAt != nil {
		t := *s.ExitedAt
		exitedAt = &t
	}
	var exitCode *int
	if s.ExitCode != nil {
		code := *s.ExitCode
		exitCode = &code
	}
	return Info{
		ID:            s.ID,
		Name:          s.Name,
		HarnessType:   s.HarnessType,
		AdapterID:     s.AdapterID,
		AdapterName:   s.AdapterName,
		Capabilities:  append([]harness.Capability(nil), s.Capabilities...),
		Command:       s.Command,
		Args:          args,
		WorkDir:       s.WorkDir,
		Status:        s.Status,
		PID:           s.PID,
		PGID:          s.PGID,
		Terminal:      TerminalSize{Rows: s.Rows, Cols: s.Cols},
		StartedAt:     s.StartedAt,
		ExitedAt:      exitedAt,
		ExitCode:      exitCode,
		Origin:        s.Origin,
		OriginBackend: s.OriginBackend,
		ShimName:      s.ShimName,
		RealBinary:    s.RealBinary,
		Attachable:    s.Attachable,
	}
}

// Manager manages the lifecycle of multiple sessions.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	bus      *events.Bus
	registry *harness.Registry
	store    ArchiveStore
}

// ArchiveStore is implemented by the archive DB.
type ArchiveStore interface {
	ArchiveSession(info Info, evts []events.Event) error
}

// NewManager creates a new session manager without an event bus.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		registry: defaultRegistry(),
	}
}

// NewManagerWithBus creates a new session manager that publishes lifecycle
// and terminal output events through the given event bus.
func NewManagerWithBus(bus *events.Bus) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		bus:      bus,
		registry: defaultRegistry(),
	}
}

// NewManagerWithRegistry creates a manager with an explicit adapter registry.
func NewManagerWithRegistry(bus *events.Bus, registry *harness.Registry) *Manager {
	if registry == nil {
		registry = defaultRegistry()
	}
	return &Manager{
		sessions: make(map[string]*Session),
		bus:      bus,
		registry: registry,
	}
}

// SetStore attaches an archive store to the manager.
func (m *Manager) SetStore(store ArchiveStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store = store
}

func defaultRegistry() *harness.Registry {
	if os.Getenv("HARNESSRELAY_ENABLE_FAKE_ADAPTER") == "1" {
		return generic.NewRegistry(codex.New(), opencode.New(), fakesemantic.New())
	}
	return generic.NewRegistry(codex.New(), opencode.New())
}

// Create starts a new session with the given options.
func (m *Manager) Create(ctx context.Context, opts CreateOptions) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.Command == "" {
		return nil, errors.New("session: command is required")
	}
	rows, cols := normalizedTerminalSize(opts.Rows, opts.Cols)
	adapter, match, ok := m.registry.Select(harness.LaunchSpec{
		Command: opts.Command,
		Args:    opts.Args,
		WorkDir: opts.WorkDir,
		Env:     opts.Env,
	})
	if !ok {
		return nil, errors.New("session: no harness adapter available")
	}

	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("session: generate id: %w", err)
	}

	r, err := pty.Start(ctx, pty.StartOptions{
		Command: opts.Command,
		Args:    opts.Args,
		WorkDir: opts.WorkDir,
		Env:     opts.Env,
		Rows:    int(rows),
		Cols:    int(cols),
	})
	if err != nil {
		return nil, fmt.Errorf("session: start: %w", err)
	}

	if err := ctx.Err(); err != nil {
		_ = r.Close()
		return nil, err
	}

	harnessType := opts.HarnessType
	if harnessType == "" {
		harnessType = adapter.ID()
	}

	var parser harness.Parser
	if provider, ok := adapter.(harness.ParserProvider); ok {
		parser = provider.NewParser()
	}

	sess := &Session{
		ID:            id,
		Name:          opts.Name,
		HarnessType:   harnessType,
		AdapterID:     adapter.ID(),
		AdapterName:   adapter.Name(),
		Capabilities:  append([]harness.Capability(nil), adapter.Capabilities()...),
		Command:       opts.Command,
		Args:          opts.Args,
		WorkDir:       opts.WorkDir,
		Status:        StatusStarting,
		PID:           r.PID(),
		PGID:          r.PGID(),
		Rows:          rows,
		Cols:          cols,
		StartedAt:     time.Now(),
		Origin:        opts.Origin,
		OriginBackend: opts.OriginBackend,
		ShimName:      opts.ShimName,
		RealBinary:    opts.RealBinary,
		Attachable:    true,
		runtime:       r,
		adapter:       adapter,
		parser:        parser,
		buf:           newOutputBuffer(defaultOutputBufferSize),
		done:          make(chan struct{}),
		outputDone:    make(chan struct{}),
	}

	if m.bus != nil {
		sess.publish = func(typ events.Type, data any) events.Event {
			return m.bus.Publish(context.Background(), events.Event{
				Type:      typ,
				SessionID: id,
				Data:      data,
			})
		}
		capabilities := make([]string, len(sess.Capabilities))
		for index, capability := range sess.Capabilities {
			capabilities[index] = string(capability)
		}
		sess.publish(events.TypeSessionCreated, events.SessionCreated{
			ID:                  sess.ID,
			Name:                sess.Name,
			HarnessType:         sess.HarnessType,
			AdapterID:           sess.AdapterID,
			AdapterName:         sess.AdapterName,
			AdapterCapabilities: capabilities,
			Command:             sess.Command,
			Args:                append([]string(nil), sess.Args...),
			WorkDir:             sess.WorkDir,
			Status:              string(sess.Status),
			StartedAt:           sess.StartedAt,
			Origin:              sess.Origin,
			OriginBackend:       sess.OriginBackend,
			ShimName:            sess.ShimName,
			RealBinary:          sess.RealBinary,
			Attachable:          sess.Attachable,
		})
		sess.publish(events.TypeHarnessDetected, events.HarnessDetected{
			AdapterID:   adapter.ID(),
			HarnessName: adapter.Name(),
			Confidence:  match.Confidence,
			Reason:      match.Reason,
		})
	}

	sess.onExit = func() {
		m.mu.Lock()
		store := m.store
		bus := m.bus
		m.mu.Unlock()
		if store != nil {
			var evts []events.Event
			if bus != nil {
				evts = bus.History(sess.ID, 0, 10000)
			}
			_ = store.ArchiveSession(sess.Info(), evts)
		}
	}

	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()

	sess.setStatus(StatusRunning)

	go sess.readOutput()
	go sess.wait()

	return sess, nil
}

// List returns all sessions.
func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartedAt.After(sessions[j].StartedAt)
	})
	return sessions
}

// Get returns the session with the given id, or false if not found.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

// Write sends input data to the session's PTY.
func (m *Manager) Write(id string, data []byte) error {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session: unknown session %q", id)
	}
	st := s.status()
	if st == StatusExited || st == StatusFailed || st == StatusTerminated {
		return fmt.Errorf("session: session %q is %s", id, st)
	}
	if isUserIntentInput(data) {
		s.clearPendingForRawInput()
	}
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	_, err := s.runtime.Write(data)
	return err
}

// SubmitPrompt writes one adapter-specific prompt submission sequence.
func (m *Manager) SubmitPrompt(id, text string) error {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session: unknown session %q", id)
	}
	st := s.status()
	if st == StatusExited || st == StatusFailed || st == StatusTerminated {
		return fmt.Errorf("session: session %q is %s", id, st)
	}

	s.mu.RLock()
	pending := s.pendingAction != nil
	semanticStatus := s.semanticStatus
	s.mu.RUnlock()
	if pending {
		return ErrApprovalPending
	}
	if hasCapability(s.Capabilities, harness.CapabilitySemanticChat) && semanticStatus != "idle" {
		return ErrHarnessNotReady
	}

	parts := [][]byte{append([]byte(text), '\r')}
	if sequencer, ok := s.parser.(harness.PromptSequencer); ok {
		parts = sequencer.PromptSequence(text, s.Snapshot())
	} else if submitter, ok := s.parser.(harness.PromptSubmitter); ok {
		parts = [][]byte{submitter.PromptBytes(text, s.Snapshot())}
	} else if submitter, ok := s.adapter.(harness.PromptSubmitter); ok {
		parts = [][]byte{submitter.PromptBytes(text, s.Snapshot())}
	}
	s.inputMu.Lock()
	for index, part := range parts {
		if index > 0 {
			time.Sleep(promptSubmitKeyDelay)
		}
		if _, err := s.runtime.Write(part); err != nil {
			s.inputMu.Unlock()
			return err
		}
	}
	s.inputMu.Unlock()
	s.emitSemanticEvent(events.Event{
		Type: events.TypeChatUserMessage,
		Data: events.ChatMessage{
			Role:       "user",
			Content:    text,
			Source:     s.AdapterID,
			Confidence: 1,
		},
	})
	s.emitSemanticEvent(events.Event{
		Type: events.TypeHarnessStatus,
		Data: events.HarnessStatus{
			Status:     "processing",
			Detail:     s.AdapterName + " received the prompt.",
			Confidence: 1,
		},
	})
	return nil
}

// CommandCatalog returns the active parser's version-verified command catalog.
func (m *Manager) CommandCatalog(id string) ([]harness.CommandDescriptor, error) {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session: unknown session %q", id)
	}
	provider, ok := s.parser.(harness.CommandCatalogProvider)
	if !ok {
		return nil, ErrCommandUnsupported
	}
	return provider.CommandCatalog(), nil
}

// SubmitCommand invokes one catalog command without treating it as an agent prompt.
func (m *Manager) SubmitCommand(id, commandID, arguments string) (harness.CommandDescriptor, error) {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return harness.CommandDescriptor{}, fmt.Errorf("session: unknown session %q", id)
	}
	st := s.status()
	if st == StatusExited || st == StatusFailed || st == StatusTerminated {
		return harness.CommandDescriptor{}, fmt.Errorf("session: session %q is %s", id, st)
	}
	s.mu.RLock()
	pending := s.pendingAction != nil
	semanticStatus := s.semanticStatus
	s.mu.RUnlock()
	if pending {
		return harness.CommandDescriptor{}, ErrApprovalPending
	}
	if hasCapability(s.Capabilities, harness.CapabilitySemanticChat) && semanticStatus != "idle" {
		return harness.CommandDescriptor{}, ErrHarnessNotReady
	}
	sequencer, ok := s.parser.(harness.CommandSequencer)
	if !ok {
		return harness.CommandDescriptor{}, ErrCommandUnsupported
	}
	parts, command, err := sequencer.CommandSequence(commandID, arguments)
	if err != nil {
		if strings.Contains(err.Error(), "unknown") {
			return harness.CommandDescriptor{}, ErrCommandUnknown
		}
		return harness.CommandDescriptor{}, err
	}
	marksProcessing := command.Interaction != harness.CommandPrefillTerminal
	if marksProcessing {
		s.mu.Lock()
		s.semanticStatus = "processing"
		if s.semanticIdleTimer != nil {
			s.semanticIdleTimer.Stop()
		}
		s.mu.Unlock()
	}
	s.inputMu.Lock()
	for index, part := range parts {
		if index > 0 {
			time.Sleep(promptSubmitKeyDelay)
		}
		if _, err := s.runtime.Write(part); err != nil {
			s.inputMu.Unlock()
			if marksProcessing {
				s.mu.Lock()
				s.semanticStatus = "idle"
				s.mu.Unlock()
			}
			return harness.CommandDescriptor{}, err
		}
	}
	s.inputMu.Unlock()
	if marksProcessing {
		s.emitSemanticEvent(events.Event{
			Type: events.TypeHarnessStatus,
			Data: events.HarnessStatus{
				Status:     "processing",
				Detail:     s.AdapterName + " is handling " + command.Invocation + ".",
				Confidence: 1,
			},
		})
	}
	return command, nil
}

// ExecuteAction runs an action only while its originating semantic event is pending.
func (m *Manager) ExecuteAction(id, eventID, actionID string) error {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session: unknown session %q", id)
	}
	st := s.status()
	if st == StatusExited || st == StatusFailed || st == StatusTerminated {
		return fmt.Errorf("session: session %q is %s", id, st)
	}

	s.mu.Lock()
	pending := s.pendingAction
	if pending == nil || pending.eventID != eventID {
		s.mu.Unlock()
		return ErrStaleSemanticAction
	}
	if _, ok := pending.actions[actionID]; !ok {
		s.mu.Unlock()
		return ErrStaleSemanticAction
	}
	handler, ok := s.adapter.(harness.ActionHandler)
	if !ok {
		s.mu.Unlock()
		return ErrUnsupportedAction
	}
	result, ok := handler.ExecuteAction(actionID)
	if !ok {
		s.mu.Unlock()
		return ErrUnsupportedAction
	}
	if result.ClearsPending {
		s.pendingAction = nil
	}
	status := result.Status
	if status == "" {
		status = "processing"
	}
	s.semanticStatus = status
	if s.semanticIdleTimer != nil {
		s.semanticIdleTimer.Stop()
	}
	s.mu.Unlock()

	if len(result.TerminalInput) > 0 {
		s.inputMu.Lock()
		_, writeErr := s.runtime.Write(result.TerminalInput)
		s.inputMu.Unlock()
		if writeErr != nil {
			return writeErr
		}
	}
	if observer, ok := s.parser.(harness.ActionObserver); ok {
		s.parserMu.Lock()
		observer.ActionResolved(actionID)
		s.parserMu.Unlock()
	}
	resolution := result.Resolution
	if resolution == "" {
		resolution = "completed"
	}
	s.emitSemanticEvent(events.Event{
		Type: events.TypeApprovalResolved,
		Data: events.ApprovalResolved{
			ApprovalEventID: eventID,
			ActionID:        actionID,
			Resolution:      resolution,
		},
	})
	for _, event := range result.Events {
		s.emitSemanticEvent(event)
	}
	detail := result.Detail
	if detail == "" {
		detail = s.AdapterName + " completed the requested action."
	}
	s.emitSemanticEvent(events.Event{
		Type: events.TypeHarnessStatus,
		Data: events.HarnessStatus{
			Status:     status,
			Detail:     detail,
			Confidence: 0.95,
		},
	})
	if hasCapability(s.Capabilities, harness.CapabilityStatusDetection) {
		s.scheduleSemanticIdle()
	}
	return nil
}

// Resize changes the terminal dimensions of a session.
func (m *Manager) Resize(id string, rows, cols uint16) error {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session: unknown session %q", id)
	}
	st := s.status()
	s.mu.Lock()
	s.Rows = rows
	s.Cols = cols
	s.mu.Unlock()
	if st == StatusExited || st == StatusFailed || st == StatusTerminated {
		return nil
	}
	if err := s.runtime.Resize(int(rows), int(cols)); err != nil {
		return err
	}
	return nil
}

// Interrupt sends an interrupt signal (Ctrl+C) to a session.
func (m *Manager) Interrupt(id string) error {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session: unknown session %q", id)
	}
	st := s.status()
	if st == StatusExited || st == StatusFailed || st == StatusTerminated {
		return fmt.Errorf("session: session %q is %s", id, st)
	}
	return s.runtime.Interrupt()
}

// Terminate stops a session gracefully with SIGTERM, then SIGKILL after the context expires.
func (m *Manager) Terminate(ctx context.Context, id string) error {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session: unknown session %q", id)
	}
	st := s.status()
	if st == StatusExited || st == StatusFailed || st == StatusTerminated {
		return fmt.Errorf("session: session %q is %s", id, st)
	}
	s.mu.Lock()
	s.requestedExit = "terminate"
	s.mu.Unlock()
	if err := s.runtime.Terminate(ctx); err != nil {
		return err
	}
	return nil
}

// Kill forcefully stops a session process group.
func (m *Manager) Kill(id string) error {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session: unknown session %q", id)
	}
	st := s.status()
	if st == StatusExited || st == StatusFailed || st == StatusTerminated {
		return fmt.Errorf("session: session %q is %s", id, st)
	}
	s.mu.Lock()
	s.requestedExit = "kill"
	s.mu.Unlock()
	return s.runtime.Kill()
}

// Cleanup removes a completed session from the manager. Running sessions must
// be interrupted, terminated, or killed before cleanup so output history is not
// lost unexpectedly.
func (m *Manager) Cleanup(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("session: unknown session %q", id)
	}
	st := s.status()
	if st != StatusExited && st != StatusFailed && st != StatusTerminated {
		m.mu.Unlock()
		return fmt.Errorf("session: session %q is %s", id, st)
	}
	delete(m.sessions, id)
	m.mu.Unlock()
	return nil
}

func (s *Session) status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status
}

func (s *Session) setStatus(st Status) {
	s.mu.Lock()
	old := s.Status
	s.Status = st
	s.mu.Unlock()
	if s.publish != nil && st != old {
		s.publish(events.TypeSessionStatusChanged, events.SessionStatusChanged{
			OldStatus: string(old),
			NewStatus: string(st),
		})
	}
}

func (s *Session) readOutput() {
	defer close(s.outputDone)
	buf := make([]byte, 4096)
	for {
		n, err := s.runtime.Read(buf)
		if n > 0 {
			s.buf.Write(buf[:n])
			if s.publish != nil {
				data := make([]byte, n)
				copy(data, buf[:n])
				s.publish(events.TypeTerminalOutput, events.TerminalOutput{Data: data})
			}
			s.processAdapterOutput(buf[:n])
		}
		if err != nil {
			s.buf.Close()
			return
		}
	}
}

func (s *Session) processAdapterOutput(chunk []byte) {
	if s.parser != nil {
		s.mu.RLock()
		rows, cols := s.Rows, s.Cols
		s.mu.RUnlock()
		update := harness.TerminalUpdate{
			Chunk:    append([]byte(nil), chunk...),
			Snapshot: s.Snapshot(),
			Command:  s.Command,
			WorkDir:  s.WorkDir,
			Rows:     rows,
			Cols:     cols,
		}
		s.parserMu.Lock()
		parsed := s.parser.Process(update)
		s.parserMu.Unlock()
		for _, event := range parsed {
			s.emitSemanticEvent(event)
		}
	}
	if hasCapability(s.Capabilities, harness.CapabilityStatusDetection) {
		s.scheduleSemanticIdle()
	}
}

func (s *Session) emitSemanticEvent(event events.Event) events.Event {
	if event.Type == "" {
		return events.Event{}
	}
	if event.Type == events.TypeHarnessStatus {
		if status, ok := event.Data.(events.HarnessStatus); ok {
			s.mu.Lock()
			s.semanticStatus = status.Status
			if status.Status == "waiting_for_approval" && s.semanticIdleTimer != nil {
				s.semanticIdleTimer.Stop()
			}
			s.mu.Unlock()
		}
	}
	if s.publish == nil {
		return events.Event{}
	}

	published := s.publish(event.Type, event.Data)
	if event.Type == events.TypeApprovalRequired {
		if approval, ok := event.Data.(events.ApprovalRequired); ok {
			actions := make(map[string]struct{}, len(approval.Actions))
			for _, action := range approval.Actions {
				if action.Kind != "ui" {
					actions[action.ID] = struct{}{}
				}
			}
			if len(actions) > 0 || approval.BlocksPrompt {
				s.mu.Lock()
				s.pendingAction = &pendingSemanticAction{eventID: published.ID, actions: actions}
				if s.semanticIdleTimer != nil {
					s.semanticIdleTimer.Stop()
				}
				s.mu.Unlock()
			}
		}
	}
	return published
}

func (s *Session) clearPendingForRawInput() {
	s.mu.Lock()
	if s.pendingAction == nil {
		s.mu.Unlock()
		return
	}
	eventID := s.pendingAction.eventID
	s.pendingAction = nil
	s.semanticStatus = "terminal_ui_active"
	s.mu.Unlock()
	if observer, ok := s.parser.(harness.ActionObserver); ok {
		s.parserMu.Lock()
		observer.ActionResolved("raw_terminal_input")
		s.parserMu.Unlock()
	}
	if s.publish != nil {
		s.publish(events.TypeHarnessStatus, events.HarnessStatus{
			Status:     "terminal_ui_active",
			Detail:     s.AdapterName + " is handling terminal input.",
			Confidence: 0.8,
		})
		s.publish(events.TypeApprovalResolved, events.ApprovalResolved{
			ApprovalEventID: eventID,
			ActionID:        "raw_terminal_input",
			Resolution:      "handled_in_terminal",
		})
	}
}

// isUserIntentInput filters out terminal escape sequences that are not explicit
// user intent, such as focus reporting (ESC [ I / ESC [ O), OSC sequences,
// and mouse events, so that window blur/focus or TUI redraws do not resolve
// pending semantic actions.
func isUserIntentInput(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	// Focus reporting: ESC [ I  or  ESC [ O
	if len(data) >= 3 && data[0] == '\x1b' && data[1] == '[' && (data[2] == 'I' || data[2] == 'O') {
		return false
	}
	// OSC sequences: ESC ]
	if len(data) >= 2 && data[0] == '\x1b' && data[1] == ']' {
		return false
	}
	// DCS sequences: ESC P
	if len(data) >= 2 && data[0] == '\x1b' && data[1] == 'P' {
		return false
	}
	// Mouse tracking: ESC [ M ...
	if len(data) >= 3 && data[0] == '\x1b' && data[1] == '[' && data[2] == 'M' {
		return false
	}
	return true
}

func (s *Session) scheduleSemanticIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingAction != nil || s.semanticStatus == "waiting_for_approval" {
		return
	}
	if s.semanticIdleTimer == nil {
		s.semanticIdleTimer = time.AfterFunc(semanticIdleDelay, s.markSemanticIdle)
		return
	}
	s.semanticIdleTimer.Reset(semanticIdleDelay)
}

func (s *Session) markSemanticIdle() {
	s.mu.Lock()
	if s.pendingAction != nil || s.Status == StatusExited || s.Status == StatusFailed || s.Status == StatusTerminated {
		s.mu.Unlock()
		return
	}
	alreadyIdle := s.semanticStatus == "idle"
	s.semanticStatus = "idle"
	s.mu.Unlock()
	if provider, ok := s.parser.(harness.IdleEventProvider); ok {
		s.parserMu.Lock()
		idleEvents := provider.OnIdle()
		s.parserMu.Unlock()
		for _, event := range idleEvents {
			s.emitSemanticEvent(event)
		}
	}
	if alreadyIdle {
		return
	}
	if s.publish != nil {
		s.publish(events.TypeHarnessStatus, events.HarnessStatus{
			Status:     "idle",
			Detail:     s.AdapterName + " is waiting for input.",
			Confidence: 0.75,
		})
	}
}

func hasCapability(capabilities []harness.Capability, wanted harness.Capability) bool {
	for _, capability := range capabilities {
		if capability == wanted {
			return true
		}
	}
	return false
}

// outputDrainGrace bounds how long the output reader may keep draining PTY
// output after the child exits before the master is force-closed. Fast exits
// need a moment to drain buffered bytes; orphaned slave holders (grandchild
// processes) must not hang session exit forever.
const outputDrainGrace = 2 * time.Second

func (s *Session) wait() {
	defer close(s.done)
	err := s.runtime.Wait()
	// Let the reader drain output the child wrote just before exiting, then
	// close the master (which unblocks a reader stuck on a slave held open
	// by an orphaned grandchild).
	select {
	case <-s.outputDone:
	case <-time.After(outputDrainGrace):
	}
	_ = s.runtime.Close()
	<-s.outputDone
	s.buf.Close()
	s.mu.Lock()
	if s.semanticIdleTimer != nil {
		s.semanticIdleTimer.Stop()
	}
	s.pendingAction = nil
	s.mu.Unlock()
	s.flushSemanticOnExit()

	now := time.Now()
	s.mu.RLock()
	requestedExit := s.requestedExit
	s.mu.RUnlock()

	if err == nil {
		code := 0
		s.mu.Lock()
		s.ExitedAt = &now
		s.ExitCode = &code
		s.mu.Unlock()
		finalStatus := StatusExited
		reason := "process_exit"
		if requestedExit != "" {
			finalStatus = StatusTerminated
			reason = requestedExit
		}
		s.setStatus(finalStatus)
		if s.publish != nil {
			s.publish(events.TypeSessionExited, events.SessionExited{
				ExitCode: 0,
				Reason:   reason,
			})
		}
		if s.onExit != nil {
			s.onExit()
		}
		return
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		var finalStatus Status
		if code == -1 {
			finalStatus = StatusTerminated
		} else {
			finalStatus = StatusExited
		}
		reason := signalReason(finalStatus)
		if requestedExit != "" {
			finalStatus = StatusTerminated
			reason = requestedExit
		}
		s.mu.Lock()
		s.ExitedAt = &now
		s.ExitCode = &code
		s.mu.Unlock()
		s.setStatus(finalStatus)
		if s.publish != nil {
			s.publish(events.TypeSessionExited, events.SessionExited{
				ExitCode: code,
				Reason:   reason,
			})
		}
		if s.onExit != nil {
			s.onExit()
		}
		return
	}

	code := -1
	s.mu.Lock()
	s.ExitedAt = &now
	s.ExitCode = &code
	s.mu.Unlock()
	s.setStatus(StatusFailed)
	if s.publish != nil {
		s.publish(events.TypeSessionExited, events.SessionExited{
			ExitCode: -1,
			Reason:   "unknown_error",
		})
	}
	if s.onExit != nil {
		s.onExit()
	}
}

func (s *Session) flushSemanticOnExit() {
	provider, ok := s.parser.(harness.IdleEventProvider)
	if !ok {
		return
	}
	s.parserMu.Lock()
	finalEvents := provider.OnIdle()
	s.parserMu.Unlock()
	for _, event := range finalEvents {
		s.emitSemanticEvent(event)
	}
}

func signalReason(st Status) string {
	if st == StatusTerminated {
		return "signal"
	}
	return "process_exit"
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "ses_" + hex.EncodeToString(b), nil
}

// outputBuffer is a bounded ring buffer that supports multiple subscribers.
type outputBuffer struct {
	mu   sync.Mutex
	ring []byte
	size int
	head int
	len  int
	subs []chan<- OutputChunk
	done bool
}

func newOutputBuffer(size int) *outputBuffer {
	if size <= 0 {
		size = defaultOutputBufferSize
	}
	return &outputBuffer{
		ring: make([]byte, size),
		size: size,
	}
}

func (b *outputBuffer) Write(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.writeLocked(data)
	chunk := OutputChunk{Data: cloneBytes(data)}
	for _, ch := range b.subs {
		select {
		case ch <- chunk:
		default:
		}
	}
}

func (b *outputBuffer) writeLocked(data []byte) {
	for i := 0; i < len(data); i++ {
		b.ring[b.head] = data[i]
		b.head = (b.head + 1) % b.size
		if b.len < b.size {
			b.len++
		}
	}
}

func (b *outputBuffer) snapshot() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.snapshotLocked()
}

func (b *outputBuffer) snapshotLocked() []byte {
	if b.len == 0 {
		return nil
	}
	out := make([]byte, b.len)
	start := (b.head - b.len + b.size) % b.size
	for i := 0; i < b.len; i++ {
		out[i] = b.ring[(start+i)%b.size]
	}
	return out
}

func (b *outputBuffer) addSub(ch chan<- OutputChunk) {
	b.mu.Lock()
	if b.done {
		snap := b.snapshotLocked()
		b.mu.Unlock()
		if len(snap) > 0 {
			ch <- OutputChunk{Data: snap}
		}
		ch <- OutputChunk{Done: true}
		close(ch)
		return
	}
	snap := b.snapshotLocked()
	b.subs = append(b.subs, ch)
	b.mu.Unlock()
	if len(snap) > 0 {
		select {
		case ch <- OutputChunk{Data: snap}:
		default:
		}
	}
}

func (b *outputBuffer) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.done {
		return
	}
	b.done = true
	for _, ch := range b.subs {
		select {
		case ch <- OutputChunk{Done: true}:
		default:
		}
		close(ch)
	}
	b.subs = nil
}

func cloneBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

func normalizedTerminalSize(rows, cols uint16) (uint16, uint16) {
	if rows == 0 && cols == 0 {
		return 24, 80
	}
	return rows, cols
}
