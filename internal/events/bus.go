package events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sort"
	"sync"
	"time"
)

const defaultHistoryLimit = 1024

// SubscribeOptions controls what events a subscription receives.
type SubscribeOptions struct {
	SessionID string
	Types     []Type
	Buffer    int
}

// Subscription receives events from the bus.
type Subscription struct {
	id  string
	C   <-chan Event
	ch  chan Event
	bus *Bus
}

// Close removes the subscription from the bus and closes the channel.
func (s *Subscription) Close() {
	s.bus.remove(s.id)
}

// Bus provides in-memory publish/subscribe for events.
type Bus struct {
	mu           sync.RWMutex
	subs         map[string]*subEntry
	sequences    map[string]uint64
	history      map[string][]Event
	historyLimit int
	store        PersistStore
}

type subEntry struct {
	ch     chan Event
	sessID string
	types  map[Type]struct{}
}

// PersistStore is implemented by the archive DB to persist lightweight events.
type PersistStore interface {
	PersistEvent(Event) error
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		subs:         make(map[string]*subEntry),
		sequences:    make(map[string]uint64),
		history:      make(map[string][]Event),
		historyLimit: defaultHistoryLimit,
	}
}

// NewBusWithStore creates a new event bus that also persists lightweight events.
func NewBusWithStore(store PersistStore) *Bus {
	b := NewBus()
	b.store = store
	return b
}

// Publish sends an event to all matching subscribers.
// Missing ID, timestamp, and sequence are filled in automatically.
// Returns the event with all fields populated.
func (b *Bus) Publish(ctx context.Context, event Event) Event {
	if event.ID == "" {
		event.ID = newEventID()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.SessionID != "" {
		b.mu.Lock()
		event.Sequence = b.nextSeqLocked(event.SessionID)
		b.appendHistoryLocked(event)
		b.mu.Unlock()
		if b.store != nil {
			_ = b.store.PersistEvent(event)
		}
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, e := range b.subs {
		if !b.matchLocked(e, event) {
			continue
		}
		select {
		case e.ch <- event:
		default:
		}
	}
	return event
}

// History returns stored events for one session with optional sequence filtering.
func (b *Bus) History(sessionID string, afterSeq uint64, limit int) []Event {
	if limit <= 0 || limit > b.historyLimit {
		limit = b.historyLimit
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	events := b.history[sessionID]
	if len(events) == 0 {
		return nil
	}
	start := sort.Search(len(events), func(i int) bool {
		return events[i].Sequence > afterSeq
	})
	if start >= len(events) {
		return nil
	}
	events = events[start:]
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	out := make([]Event, len(events))
	copy(out, events)
	return out
}

// Subscribe creates a new subscription with the given options.
func (b *Bus) Subscribe(opts SubscribeOptions) *Subscription {
	if opts.Buffer <= 0 {
		opts.Buffer = 64
	}
	id := newSubID()
	ch := make(chan Event, opts.Buffer)
	entry := &subEntry{ch: ch, sessID: opts.SessionID}
	if len(opts.Types) > 0 {
		entry.types = make(map[Type]struct{}, len(opts.Types))
		for _, t := range opts.Types {
			entry.types[t] = struct{}{}
		}
	}
	b.mu.Lock()
	b.subs[id] = entry
	b.mu.Unlock()

	return &Subscription{id: id, C: ch, ch: ch, bus: b}
}

func (b *Bus) remove(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e, ok := b.subs[id]; ok {
		close(e.ch)
		delete(b.subs, id)
	}
}

// ClearHistory removes stored events for a session to free memory.
func (b *Bus) ClearHistory(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.history, sessionID)
	delete(b.sequences, sessionID)
}

func (b *Bus) nextSeqLocked(sessionID string) uint64 {
	seq := b.sequences[sessionID] + 1
	b.sequences[sessionID] = seq
	return seq
}

func (b *Bus) appendHistoryLocked(event Event) {
	if b.historyLimit <= 0 {
		return
	}
	history := append(b.history[event.SessionID], event)
	if len(history) > b.historyLimit {
		history = history[len(history)-b.historyLimit:]
	}
	b.history[event.SessionID] = history
}

func (b *Bus) matchLocked(e *subEntry, event Event) bool {
	if e.sessID != "" && e.sessID != event.SessionID {
		return false
	}
	if len(e.types) > 0 {
		_, ok := e.types[event.Type]
		return ok
	}
	return true
}

func newSubID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "sub_" + hex.EncodeToString(b)
}
