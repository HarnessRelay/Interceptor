package events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

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
	mu        sync.RWMutex
	subs      map[string]*subEntry
	sequences map[string]uint64
}

type subEntry struct {
	ch     chan Event
	sessID string
	types  map[Type]struct{}
}

// NewBus creates a new event bus.
func NewBus() *Bus {
	return &Bus{
		subs:      make(map[string]*subEntry),
		sequences: make(map[string]uint64),
	}
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
		event.Sequence = b.nextSeq(event.SessionID)
	}
	b.mu.RLock()
	subs := make([]chan Event, 0, len(b.subs))
	for _, e := range b.subs {
		if !b.matchLocked(e, event) {
			continue
		}
		subs = append(subs, e.ch)
	}
	b.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
		}
	}
	return event
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

func (b *Bus) nextSeq(sessionID string) uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	seq := b.sequences[sessionID] + 1
	b.sequences[sessionID] = seq
	return seq
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
