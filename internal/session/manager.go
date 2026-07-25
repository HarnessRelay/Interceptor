package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/pty"
)

const defaultOutputBufferSize = 64 * 1024

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
	Name    string
	Command string
	Args    []string
	WorkDir string
	Env     []string
	Rows    uint16
	Cols    uint16
}

// Session holds metadata and runtime state for one session.
type Session struct {
	ID        string
	Name      string
	Command   string
	Args      []string
	WorkDir   string
	Status    Status
	PID       int
	PGID      int
	StartedAt time.Time
	ExitedAt  *time.Time
	ExitCode  *int

	runtime *pty.Runtime
	buf     *outputBuffer
	done    chan struct{}
	mu      sync.RWMutex

	publish func(typ events.Type, data any)
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

// Manager manages the lifecycle of multiple sessions.
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	bus      *events.Bus
}

// NewManager creates a new session manager without an event bus.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// NewManagerWithBus creates a new session manager that publishes lifecycle
// and terminal output events through the given event bus.
func NewManagerWithBus(bus *events.Bus) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		bus:      bus,
	}
}

// Create starts a new session with the given options.
func (m *Manager) Create(ctx context.Context, opts CreateOptions) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.Command == "" {
		return nil, errors.New("session: command is required")
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
		Rows:    int(opts.Rows),
		Cols:    int(opts.Cols),
	})
	if err != nil {
		return nil, fmt.Errorf("session: start: %w", err)
	}

	if err := ctx.Err(); err != nil {
		_ = r.Close()
		return nil, err
	}

	sess := &Session{
		ID:        id,
		Name:      opts.Name,
		Command:   opts.Command,
		Args:      opts.Args,
		WorkDir:   opts.WorkDir,
		Status:    StatusStarting,
		PID:       r.PID(),
		PGID:      r.PGID(),
		StartedAt: time.Now(),
		runtime:   r,
		buf:       newOutputBuffer(defaultOutputBufferSize),
		done:      make(chan struct{}),
	}

	if m.bus != nil {
		sess.publish = func(typ events.Type, data any) {
			m.bus.Publish(context.Background(), events.Event{
				Type:      typ,
				SessionID: id,
				Data:      data,
			})
		}
		sess.publish(events.TypeSessionCreated, sess)
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
	_, err := s.runtime.Write(data)
	return err
}

// Resize changes the terminal dimensions of a session.
func (m *Manager) Resize(id string, rows, cols uint16) error {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("session: unknown session %q", id)
	}
	return s.runtime.Resize(int(rows), int(cols))
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
	if err := s.runtime.Terminate(ctx); err != nil {
		return err
	}
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
		}
		if err != nil {
			s.buf.Close()
			return
		}
	}
}

func (s *Session) wait() {
	defer close(s.done)
	err := s.runtime.Wait()
	_ = s.runtime.Close()
	s.buf.Close()

	now := time.Now()

	if err == nil {
		code := 0
		s.mu.Lock()
		s.ExitedAt = &now
		s.ExitCode = &code
		s.mu.Unlock()
		s.setStatus(StatusExited)
		if s.publish != nil {
			s.publish(events.TypeSessionExited, events.SessionExited{
				ExitCode: 0,
				Reason:   "process_exit",
			})
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
		s.mu.Lock()
		s.ExitedAt = &now
		s.ExitCode = &code
		s.mu.Unlock()
		s.setStatus(finalStatus)
		if s.publish != nil {
			s.publish(events.TypeSessionExited, events.SessionExited{
				ExitCode: code,
				Reason:   signalReason(finalStatus),
			})
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
