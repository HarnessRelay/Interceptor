package events

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPublishPopulatesIDAndTimestamp(t *testing.T) {
	bus := NewBus()
	ev := bus.Publish(context.Background(), Event{Type: TypeTerminalOutput, SessionID: "ses_1"})

	if ev.ID == "" {
		t.Fatal("event ID is empty")
	}
	if !strings.HasPrefix(ev.ID, "evt_") {
		t.Fatalf("unexpected event ID prefix: %s", ev.ID)
	}
	if ev.Timestamp.IsZero() {
		t.Fatal("event timestamp is zero")
	}
	if ev.Sequence != 1 {
		t.Fatalf("Sequence = %d, want 1", ev.Sequence)
	}
}

func TestPublishIncrementsSequence(t *testing.T) {
	bus := NewBus()
	e1 := bus.Publish(context.Background(), Event{Type: TypeTerminalOutput, SessionID: "ses_1"})
	e2 := bus.Publish(context.Background(), Event{Type: TypeTerminalOutput, SessionID: "ses_1"})
	e3 := bus.Publish(context.Background(), Event{Type: TypeTerminalOutput, SessionID: "ses_2"})

	if e1.Sequence != 1 || e2.Sequence != 2 {
		t.Fatalf("session 1 sequences: %d, %d, want 1, 2", e1.Sequence, e2.Sequence)
	}
	if e3.Sequence != 1 {
		t.Fatalf("session 2 sequence: %d, want 1", e3.Sequence)
	}
}

func TestPublishDoesNotOverridePrepopulatedFields(t *testing.T) {
	bus := NewBus()
	now := time.Now()
	ev := bus.Publish(context.Background(), Event{
		ID:        "evt_custom",
		Type:      TypeSessionCreated,
		SessionID: "ses_1",
		Timestamp: now,
	})

	if ev.ID != "evt_custom" {
		t.Fatalf("ID = %q, want evt_custom", ev.ID)
	}
	if !ev.Timestamp.Equal(now) {
		t.Fatalf("timestamp was overridden")
	}
}

func TestSingleSubscriberReceivesEvent(t *testing.T) {
	bus := NewBus()
	sub := bus.Subscribe(SubscribeOptions{})
	defer sub.Close()

	bus.Publish(context.Background(), Event{
		Type:      TypeSessionCreated,
		SessionID: "ses_1",
	})

	select {
	case ev := <-sub.C:
		if ev.Type != TypeSessionCreated {
			t.Fatalf("Type = %q, want %q", ev.Type, TypeSessionCreated)
		}
		if ev.SessionID != "ses_1" {
			t.Fatalf("SessionID = %q, want ses_1", ev.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestMultipleSubscribersReceiveSameEvent(t *testing.T) {
	bus := NewBus()
	sub1 := bus.Subscribe(SubscribeOptions{})
	defer sub1.Close()
	sub2 := bus.Subscribe(SubscribeOptions{})
	defer sub2.Close()
	sub3 := bus.Subscribe(SubscribeOptions{})
	defer sub3.Close()

	bus.Publish(context.Background(), Event{
		Type:      TypeSessionExited,
		SessionID: "ses_1",
		Data:      SessionExited{ExitCode: 0, Reason: "process_exit"},
	})

	for i, sub := range []*Subscription{sub1, sub2, sub3} {
		select {
		case ev := <-sub.C:
			if ev.Type != TypeSessionExited {
				t.Fatalf("sub %d: Type = %q, want %q", i+1, ev.Type, TypeSessionExited)
			}
		case <-time.After(time.Second):
			t.Fatalf("sub %d: timed out", i+1)
		}
	}
}

func TestSessionFiltering(t *testing.T) {
	bus := NewBus()
	sub := bus.Subscribe(SubscribeOptions{SessionID: "ses_1"})
	defer sub.Close()

	bus.Publish(context.Background(), Event{
		Type:      TypeTerminalOutput,
		SessionID: "ses_2",
	})
	bus.Publish(context.Background(), Event{
		Type:      TypeTerminalOutput,
		SessionID: "ses_1",
	})

	select {
	case ev := <-sub.C:
		if ev.SessionID != "ses_1" {
			t.Fatalf("SessionID = %q, want ses_1", ev.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for filtered event")
	}

	select {
	case <-sub.C:
		t.Fatal("received unexpected event for ses_2")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestTypeFiltering(t *testing.T) {
	bus := NewBus()
	sub := bus.Subscribe(SubscribeOptions{
		Types: []Type{TypeSessionExited, TypeError},
	})
	defer sub.Close()

	bus.Publish(context.Background(), Event{Type: TypeTerminalOutput, SessionID: "ses_1"})
	bus.Publish(context.Background(), Event{Type: TypeSessionCreated, SessionID: "ses_1"})
	bus.Publish(context.Background(), Event{Type: TypeSessionExited, SessionID: "ses_1", Data: SessionExited{ExitCode: 0}})

	select {
	case ev := <-sub.C:
		if ev.Type != TypeSessionExited {
			t.Fatalf("Type = %q, want %q", ev.Type, TypeSessionExited)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for filtered event")
	}

	select {
	case <-sub.C:
		t.Fatal("received unexpected extra event")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestCombinedFiltering(t *testing.T) {
	bus := NewBus()
	sub := bus.Subscribe(SubscribeOptions{
		SessionID: "ses_1",
		Types:     []Type{TypeTerminalOutput},
	})
	defer sub.Close()

	bus.Publish(context.Background(), Event{Type: TypeTerminalOutput, SessionID: "ses_2"})
	bus.Publish(context.Background(), Event{Type: TypeSessionCreated, SessionID: "ses_1"})
	bus.Publish(context.Background(), Event{Type: TypeTerminalOutput, SessionID: "ses_1"})

	select {
	case ev := <-sub.C:
		if ev.SessionID != "ses_1" || ev.Type != TypeTerminalOutput {
			t.Fatalf("got event %s/%s, want ses_1/terminal.output", ev.SessionID, ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}

	select {
	case <-sub.C:
		t.Fatal("received unexpected extra event")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestSlowSubscriberDoesNotBlockPublish(t *testing.T) {
	bus := NewBus()
	sub := bus.Subscribe(SubscribeOptions{Buffer: 1})

	publishDone := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			bus.Publish(context.Background(), Event{Type: TypeError, SessionID: "ses_1"})
		}
		close(publishDone)
	}()

	select {
	case <-publishDone:
	case <-time.After(time.Second):
		sub.Close()
		t.Fatal("publish blocked by slow subscriber")
	}

	count := 0
	drain := time.After(200 * time.Millisecond)
drainloop:
	for {
		select {
		case _, ok := <-sub.C:
			if !ok {
				break drainloop
			}
			count++
		case <-drain:
			break drainloop
		}
	}
	sub.Close()

	if count == 0 {
		t.Fatal("subscriber received 0 events")
	}
	if count == 100 {
		t.Fatal("subscriber received all events, buffer should have dropped some")
	}
}

func TestCloseSubscriptionStopsDelivery(t *testing.T) {
	bus := NewBus()
	sub := bus.Subscribe(SubscribeOptions{})
	sub.Close()

	select {
	case _, ok := <-sub.C:
		if ok {
			t.Fatal("channel should be closed")
		}
	default:
		t.Fatal("channel not closed")
	}

	bus.Publish(context.Background(), Event{Type: TypeError, SessionID: "ses_1"})
}

func TestPublishAfterSubscriptionCloseDoesNotPanic(t *testing.T) {
	bus := NewBus()
	sub := bus.Subscribe(SubscribeOptions{})
	sub.Close()

	for i := 0; i < 10; i++ {
		bus.Publish(context.Background(), Event{Type: TypeError, SessionID: "ses_1"})
	}
}

func TestConcurrentPublishSubscribe(t *testing.T) {
	bus := NewBus()

	subs := make([]*Subscription, 5)
	for i := 0; i < 5; i++ {
		subs[i] = bus.Subscribe(SubscribeOptions{Buffer: 128})
		defer subs[i].Close()
	}

	pubCount := 10
	goroutines := 10
	totalPublished := pubCount * goroutines
	expectedTotal := totalPublished * len(subs)

	var pubWg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		pubWg.Add(1)
		go func() {
			defer pubWg.Done()
			for j := 0; j < pubCount; j++ {
				bus.Publish(context.Background(), Event{
					Type:      TypeTerminalOutput,
					SessionID: "ses_concurrent",
				})
			}
		}()
	}
	pubWg.Wait()

	consumed := make([]int, len(subs))
	var consWg sync.WaitGroup
	for i, sub := range subs {
		consWg.Add(1)
		go func(idx int, s *Subscription) {
			defer consWg.Done()
			for range s.C {
				consumed[idx]++
			}
		}(i, sub)
	}

	for _, sub := range subs {
		sub.Close()
	}
	consWg.Wait()

	total := 0
	for _, c := range consumed {
		total += c
	}
	if total != expectedTotal {
		t.Fatalf("total consumed = %d, want %d (received %v)", total, expectedTotal, consumed)
	}
}

func TestSubscriptionWithDefaultBuffer(t *testing.T) {
	bus := NewBus()
	sub := bus.Subscribe(SubscribeOptions{})
	defer sub.Close()
	for i := 0; i < 32; i++ {
		bus.Publish(context.Background(), Event{Type: TypeError, SessionID: "ses_1"})
	}

	count := 0
	timeout := time.After(time.Second)
	for count < 32 {
		select {
		case <-sub.C:
			count++
		case <-timeout:
			t.Fatalf("received %d events, want 32", count)
		}
	}
}

func TestEventDataPersistence(t *testing.T) {
	bus := NewBus()
	sub := bus.Subscribe(SubscribeOptions{})
	defer sub.Close()

	bus.Publish(context.Background(), Event{
		Type:      TypeSessionExited,
		SessionID: "ses_1",
		Data:      SessionExited{ExitCode: 42, Reason: "crash"},
	})

	select {
	case ev := <-sub.C:
		d, ok := ev.Data.(SessionExited)
		if !ok {
			t.Fatalf("Data type = %T, want SessionExited", ev.Data)
		}
		if d.ExitCode != 42 {
			t.Fatalf("ExitCode = %d, want 42", d.ExitCode)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestPublishWithEmptyType(t *testing.T) {
	bus := NewBus()
	sub := bus.Subscribe(SubscribeOptions{})
	defer sub.Close()

	ev := bus.Publish(context.Background(), Event{SessionID: "ses_1"})
	if ev.Type != "" {
		t.Fatalf("Type = %q, want empty", ev.Type)
	}

	select {
	case received := <-sub.C:
		if received.Type != "" {
			t.Fatalf("received Type = %q, want empty", received.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func TestTypeFilterWithEmptyEventType(t *testing.T) {
	bus := NewBus()
	sub := bus.Subscribe(SubscribeOptions{
		Types: []Type{TypeTerminalOutput},
	})
	defer sub.Close()

	ev := bus.Publish(context.Background(), Event{SessionID: "ses_1"})

	select {
	case <-sub.C:
		t.Fatal("subscriber with type filter should not receive empty-type event")
	case <-time.After(100 * time.Millisecond):
	}

	_ = ev
}
