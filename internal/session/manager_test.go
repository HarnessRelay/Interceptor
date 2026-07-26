package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/harnessrelay/interceptor/internal/events"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "testdata", "fake-harnesses", name))
	if err != nil {
		t.Fatalf("fixture path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture stat: %v", err)
	}
	return path
}

func TestManagerCreateSessionFromFakeHarness(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Name:    "test-plain",
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sess.ID == "" {
		t.Fatal("session ID is empty")
	}
	if !strings.HasPrefix(sess.ID, "ses_") {
		t.Fatalf("unexpected session ID prefix: %s", sess.ID)
	}
	if sess.PID <= 0 {
		t.Fatalf("PID = %d, want positive", sess.PID)
	}
	if sess.Status != StatusRunning {
		t.Fatalf("Status = %q, want %q", sess.Status, StatusRunning)
	}

	waitSessionDone(t, sess, 5*time.Second)

	out := sess.Snapshot()
	if !strings.Contains(string(out), "plain stdout") {
		t.Fatalf("snapshot missing stdout: %q", string(out))
	}
	if !strings.Contains(string(out), "plain stderr") {
		t.Fatalf("snapshot missing stderr: %q", string(out))
	}

	if sess.Status != StatusExited {
		t.Fatalf("Status = %q, want %q", sess.Status, StatusExited)
	}
	if sess.ExitedAt == nil {
		t.Fatal("ExitedAt is nil")
	}
	if sess.ExitCode == nil {
		t.Fatal("ExitCode is nil")
	}
	if *sess.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", *sess.ExitCode)
	}
}

func TestManagerList(t *testing.T) {
	mgr := NewManager()

	if got := len(mgr.List()); got != 0 {
		t.Fatalf("List length = %d, want 0", got)
	}

	s1, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	s2, err := mgr.Create(context.Background(), CreateOptions{
		Name:    "session-two",
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	waitSessionDone(t, s1, 5*time.Second)
	waitSessionDone(t, s2, 5*time.Second)

	list := mgr.List()
	if len(list) != 2 {
		t.Fatalf("List length = %d, want 2", len(list))
	}

	ids := make(map[string]bool)
	for _, s := range list {
		ids[s.ID] = true
	}
	if !ids[s1.ID] || !ids[s2.ID] {
		t.Fatalf("List missing expected IDs: %v", ids)
	}
	if list[0].ID != s2.ID || list[1].ID != s1.ID {
		t.Fatalf("List order = [%s, %s], want newest session first [%s, %s]", list[0].ID, list[1].ID, s2.ID, s1.ID)
	}
}

func TestManagerGet(t *testing.T) {
	mgr := NewManager()

	_, ok := mgr.Get("nonexistent")
	if ok {
		t.Fatal("Get nonexistent returned ok=true")
	}

	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	got, ok := mgr.Get(sess.ID)
	if !ok {
		t.Fatal("Get returned ok=false")
	}
	if got.ID != sess.ID {
		t.Fatalf("Get ID = %q, want %q", got.ID, sess.ID)
	}
}

func TestManagerCreateRejectsEmptyCommand(t *testing.T) {
	mgr := NewManager()
	_, err := mgr.Create(context.Background(), CreateOptions{})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestManagerWriteInput(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "interactive-echo.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sub := sess.Subscribe()
	readUntil(t, sub, "input>", 5*time.Second)

	if err := mgr.Write(sess.ID, []byte("hello\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	readUntil(t, sub, "echo:hello", 5*time.Second)
}

func TestManagerSubmitPromptUsesGenericLineInputFallback(t *testing.T) {
	bus := events.NewBus()
	mgr := NewManagerWithBus(bus)
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "interactive-echo.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sub := sess.Subscribe()
	readUntil(t, sub, "input>", 5*time.Second)
	if err := mgr.SubmitPrompt(sess.ID, "generic prompt"); err != nil {
		t.Fatalf("SubmitPrompt: %v", err)
	}
	readUntil(t, sub, "echo:generic prompt", 5*time.Second)
	waitSessionDone(t, sess, 5*time.Second)

	event := waitForHistoryEvent(t, bus, sess.ID, events.TypeChatUserMessage, time.Second)
	if data := event.Data.(events.ChatMessage); data.Content != "generic prompt" || data.Source != "generic" {
		t.Fatalf("chat.user_message = %+v", data)
	}
}

func TestManagerFakeApprovalHarness(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "approval-prompt.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sub := sess.Subscribe()
	readUntil(t, sub, "approve? [y/N]", 5*time.Second)

	if err := mgr.Write(sess.ID, []byte("y\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	readUntil(t, sub, "approved", 5*time.Second)
	waitSessionDone(t, sess, 5*time.Second)
}

func TestManagerPublishesGenericApprovalHeuristic(t *testing.T) {
	bus := events.NewBus()
	mgr := NewManagerWithBus(bus)
	sub := bus.Subscribe(events.SubscribeOptions{Types: []events.Type{events.TypeApprovalRequired}, Buffer: 4})
	defer sub.Close()

	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "approval-prompt.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Terminate(ctx, sess.ID)
	})

	deadline := time.After(5 * time.Second)
	for {
		select {
		case event := <-sub.C:
			if event.Type != events.TypeApprovalRequired {
				continue
			}
			if event.SessionID != sess.ID {
				t.Fatalf("event session = %q, want %q", event.SessionID, sess.ID)
			}
			data := event.Data.(map[string]any)
			if data["confidence"] != "heuristic" {
				t.Fatalf("confidence = %v, want heuristic", data["confidence"])
			}
			return
		case <-deadline:
			t.Fatal("timed out waiting for approval.required event")
		}
	}
}

func TestManagerFakeFullscreenHarness(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "fullscreen-redraw.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	waitSessionDone(t, sess, 5*time.Second)
	out := string(sess.Snapshot())
	if !strings.Contains(out, "\x1b[?1049h") {
		t.Fatalf("snapshot missing alternate-screen enter sequence: %q", out)
	}
	if !strings.Contains(out, "fullscreen complete") {
		t.Fatalf("snapshot missing completion text: %q", out)
	}
}

func TestManagerCodexAdapterSemanticFlow(t *testing.T) {
	bus := events.NewBus()
	mgr := NewManagerWithBus(bus)
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Name:    "fake-codex",
		Command: fixturePath(t, "codex"),
		WorkDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		if sess.status() != StatusRunning {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Terminate(ctx, sess.ID)
	})

	info := sess.Info()
	if info.AdapterID != "codex" || info.AdapterName != "Codex" {
		t.Fatalf("adapter = %q/%q, want codex/Codex", info.AdapterID, info.AdapterName)
	}
	if !hasCapability(info.Capabilities, "semantic_chat") {
		t.Fatalf("capabilities missing semantic_chat: %v", info.Capabilities)
	}

	output := sess.Subscribe()
	readUntil(t, output, "OpenAI Codex", 5*time.Second)
	waitForHistoryEvent(t, bus, sess.ID, events.TypeChatSystemMessage, 5*time.Second)
	waitForHistoryEvent(t, bus, sess.ID, events.TypeTerminalNoisyOutput, 5*time.Second)
	metadata := waitForHarnessMetadataModel(t, bus, sess.ID, "gpt-fake high", 5*time.Second)
	metadataData := metadata.Data.(events.HarnessMetadata)
	if metadataData.Model != "gpt-fake high" || metadataData.Version != "0.145.0" {
		t.Fatalf("metadata = %+v", metadataData)
	}
	waitForHarnessStatus(t, bus, sess.ID, "idle", 5*time.Second)

	if err := mgr.SubmitPrompt(sess.ID, "hello fake codex"); err != nil {
		t.Fatalf("SubmitPrompt: %v", err)
	}
	readUntil(t, output, "RECEIVED:hello fake codex", 5*time.Second)
	user := waitForHistoryEvent(t, bus, sess.ID, events.TypeChatUserMessage, 5*time.Second)
	if data := user.Data.(events.ChatMessage); data.Content != "hello fake codex" {
		t.Fatalf("user message = %+v", data)
	}
	assistant := waitForHistoryEvent(t, bus, sess.ID, events.TypeChatAssistantMessage, 5*time.Second)
	if data := assistant.Data.(events.ChatMessage); data.Content != "Fake Codex response to: hello fake codex" || data.MessageID != "codex-turn-1" {
		t.Fatalf("assistant message = %+v", data)
	}
	if !strings.Contains(string(sess.Snapshot()), "MMMMMMMM") {
		t.Fatal("raw terminal snapshot did not retain fake TUI artifact")
	}
}

func TestManagerCodexPromptSubmissionWithoutEventBus(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: fixturePath(t, "codex"),
		WorkDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		if sess.status() != StatusRunning {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Terminate(ctx, sess.ID)
	})

	output := sess.Subscribe()
	readUntil(t, output, "OpenAI Codex", 5*time.Second)
	deadline := time.Now().Add(5 * time.Second)
	for {
		err = mgr.SubmitPrompt(sess.ID, "without bus")
		if err == nil {
			break
		}
		if !errors.Is(err, ErrHarnessNotReady) || time.Now().After(deadline) {
			t.Fatalf("SubmitPrompt: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	readUntil(t, output, "RECEIVED:without bus", 5*time.Second)
}

func TestManagerCodexApprovalDenyIsEventBound(t *testing.T) {
	bus := events.NewBus()
	mgr := NewManagerWithBus(bus)
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: fixturePath(t, "codex"),
		WorkDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		if sess.status() != StatusRunning {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Terminate(ctx, sess.ID)
	})

	output := sess.Subscribe()
	readUntil(t, output, "OpenAI Codex", 5*time.Second)
	if err := mgr.SubmitPrompt(sess.ID, "too early"); !errors.Is(err, ErrHarnessNotReady) {
		t.Fatalf("early SubmitPrompt error = %v, want ErrHarnessNotReady", err)
	}
	waitForHarnessStatus(t, bus, sess.ID, "idle", 5*time.Second)
	if err := mgr.SubmitPrompt(sess.ID, "request approval"); err != nil {
		t.Fatalf("SubmitPrompt: %v", err)
	}
	readUntil(t, output, "Would you like to run the following command?", 5*time.Second)
	approval := waitForHistoryEvent(t, bus, sess.ID, events.TypeApprovalRequired, 5*time.Second)
	waitForPendingAction(t, sess, approval.ID, 5*time.Second)

	if err := mgr.SubmitPrompt(sess.ID, "must be blocked"); !errors.Is(err, ErrApprovalPending) {
		t.Fatalf("SubmitPrompt while pending error = %v, want ErrApprovalPending", err)
	}
	if err := mgr.ExecuteAction(sess.ID, "evt_stale", "codex.approval_deny"); !errors.Is(err, ErrStaleSemanticAction) {
		t.Fatalf("stale ExecuteAction error = %v", err)
	}
	if err := mgr.ExecuteAction(sess.ID, approval.ID, "codex.approval_deny"); err != nil {
		t.Fatalf("ExecuteAction deny: %v", err)
	}
	readUntil(t, output, "DENIED", 5*time.Second)
	resolved := waitForHistoryEvent(t, bus, sess.ID, events.TypeApprovalResolved, 5*time.Second)
	if data := resolved.Data.(events.ApprovalResolved); data.ApprovalEventID != approval.ID || data.Resolution != "denied" {
		t.Fatalf("approval.resolved = %+v", data)
	}
	if err := mgr.ExecuteAction(sess.ID, approval.ID, "codex.approval_deny"); !errors.Is(err, ErrStaleSemanticAction) {
		t.Fatalf("replayed ExecuteAction error = %v", err)
	}
}

func TestManagerInterrupt(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "long-running.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sub := sess.Subscribe()
	readUntil(t, sub, "ready", 5*time.Second)

	if err := mgr.Interrupt(sess.ID); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}

	readUntil(t, sub, "interrupted", 5*time.Second)
	waitSessionDone(t, sess, 5*time.Second)

	if sess.Status != StatusExited {
		t.Fatalf("Status = %q, want %q", sess.Status, StatusExited)
	}
}

func TestManagerTerminate(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "long-running.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sub := sess.Subscribe()
	readUntil(t, sub, "ready", 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.Terminate(ctx, sess.ID); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	if sess.Status != StatusTerminated {
		t.Fatalf("Status = %q, want %q", sess.Status, StatusTerminated)
	}
}

func TestManagerTerminateKillsSIGTERMIgnoringProcess(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "ignore-term.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sub := sess.Subscribe()
	readUntil(t, sub, "ready", 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := mgr.Terminate(ctx, sess.ID); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	if sess.Status != StatusTerminated {
		t.Fatalf("Status = %q, want %q", sess.Status, StatusTerminated)
	}
}

func TestManagerKill(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "long-running.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sub := sess.Subscribe()
	readUntil(t, sub, "ready", 5*time.Second)

	if err := mgr.Kill(sess.ID); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	if sess.Status != StatusTerminated {
		t.Fatalf("Status = %q, want %q", sess.Status, StatusTerminated)
	}
}

func TestManagerCleanupCompletedSession(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	if err := mgr.Cleanup(sess.ID); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if _, ok := mgr.Get(sess.ID); ok {
		t.Fatal("session still present after cleanup")
	}
}

func TestManagerCleanupRejectsRunningSession(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "long-running.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Terminate(ctx, sess.ID)
	})
	if err := mgr.Cleanup(sess.ID); err == nil {
		t.Fatal("Cleanup accepted running session")
	}
}

func TestManagerResize(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "resize-aware.sh")},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sub := sess.Subscribe()
	readUntil(t, sub, "ready", 5*time.Second)

	if err := mgr.Resize(sess.ID, 40, 100); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if _, err := sess.runtime.Write([]byte("size\n")); err != nil {
		t.Fatalf("write size probe: %v", err)
	}

	readUntil(t, sub, "40 100", 5*time.Second)
}

func TestManagerUnknownSessionErrors(t *testing.T) {
	mgr := NewManager()
	unknownID := "ses_nonexistent"

	if err := mgr.Write(unknownID, []byte("data")); err == nil {
		t.Fatal("Write on unknown session should fail")
	}
	if err := mgr.Resize(unknownID, 24, 80); err == nil {
		t.Fatal("Resize on unknown session should fail")
	}
	if err := mgr.Interrupt(unknownID); err == nil {
		t.Fatal("Interrupt on unknown session should fail")
	}
	ctx := context.Background()
	if err := mgr.Terminate(ctx, unknownID); err == nil {
		t.Fatal("Terminate on unknown session should fail")
	}
	if err := mgr.Kill(unknownID); err == nil {
		t.Fatal("Kill on unknown session should fail")
	}
	if err := mgr.Cleanup(unknownID); err == nil {
		t.Fatal("Cleanup on unknown session should fail")
	}
}

func TestManagerWriteAndInterruptOnExitedSession(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	if err := mgr.Write(sess.ID, []byte("data")); err == nil {
		t.Fatal("Write on exited session should fail")
	}
	if err := mgr.Interrupt(sess.ID); err == nil {
		t.Fatal("Interrupt on exited session should fail")
	}
	ctx := context.Background()
	if err := mgr.Terminate(ctx, sess.ID); err == nil {
		t.Fatal("Terminate on exited session should fail")
	}
	if err := mgr.Kill(sess.ID); err == nil {
		t.Fatal("Kill on exited session should fail")
	}
}

func TestManagerResizeAfterExitUpdatesMetadata(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
		Rows:    24,
		Cols:    80,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	if err := mgr.Resize(sess.ID, 40, 100); err != nil {
		t.Fatalf("Resize after exit: %v", err)
	}
	info := sess.Info()
	if info.Terminal.Rows != 40 || info.Terminal.Cols != 100 {
		t.Fatalf("terminal size = %dx%d, want 40x100", info.Terminal.Rows, info.Terminal.Cols)
	}
}

func TestSessionSubscription(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ch := sess.Subscribe()

	for chunk := range ch {
		if chunk.Done {
			break
		}
	}

	waitSessionDone(t, sess, 5*time.Second)

	if sess.Status != StatusExited {
		t.Fatalf("Status = %q, want %q", sess.Status, StatusExited)
	}
}

func TestSessionSubscriptionAfterExit(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	ch := sess.Subscribe()

	hasDone := false
	for chunk := range ch {
		if chunk.Done {
			hasDone = true
			break
		}
	}
	if !hasDone {
		t.Fatal("late subscription did not receive Done")
	}
}

func TestOutputBufferKeepsNewestBytes(t *testing.T) {
	buf := newOutputBuffer(5)
	buf.Write([]byte("hello"))
	buf.Write([]byte("world"))

	got := string(buf.snapshot())
	if got != "world" {
		t.Fatalf("snapshot = %q, want world", got)
	}
}

func TestSessionSnapshot(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	snap := sess.Snapshot()
	if len(snap) == 0 {
		t.Fatal("snapshot is empty")
	}
	if !strings.Contains(string(snap), "plain stdout") {
		t.Fatalf("snapshot missing stdout: %q", string(snap))
	}
}

func TestMultipleSubscribers(t *testing.T) {
	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "interactive-echo.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	sub1 := sess.Subscribe()
	sub2 := sess.Subscribe()

	readUntil(t, sub1, "input>", 5*time.Second)

	if err := mgr.Write(sess.ID, []byte("hello\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	readUntil(t, sub2, "echo:hello", 5*time.Second)
}

func readUntil(t *testing.T, ch <-chan OutputChunk, want string, timeout time.Duration) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var got strings.Builder
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed while waiting for %q, got: %q", want, got.String())
			}
			if chunk.Done {
				t.Fatalf("got Done while waiting for %q, accumulated: %q", want, got.String())
			}
			got.Write(chunk.Data)
			if strings.Contains(got.String(), want) {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %q in output %q", want, got.String())
		}
	}
}

func waitSessionDone(t *testing.T, s *Session, timeout time.Duration) {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for session to complete")
	}
}

func collectEventsByType(sub *events.Subscription, timeout time.Duration) map[events.Type][]events.Event {
	result := make(map[events.Type][]events.Event)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-sub.C:
			if !ok {
				return result
			}
			result[ev.Type] = append(result[ev.Type], ev)
		case <-timer.C:
			sub.Close()
			return result
		}
	}
}

func waitForHistoryEvent(t *testing.T, bus *events.Bus, sessionID string, wanted events.Type, timeout time.Duration) events.Event {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, event := range bus.History(sessionID, 0, 1024) {
			if event.Type == wanted {
				return event
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q", wanted)
	return events.Event{}
}

func waitForPendingAction(t *testing.T, sess *Session, eventID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sess.mu.RLock()
		pending := sess.pendingAction
		matched := pending != nil && pending.eventID == eventID
		sess.mu.RUnlock()
		if matched {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for pending action %q", eventID)
}

func waitForHarnessStatus(t *testing.T, bus *events.Bus, sessionID, wanted string, timeout time.Duration) events.Event {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, event := range bus.History(sessionID, 0, 1024) {
			if event.Type != events.TypeHarnessStatus {
				continue
			}
			status, ok := event.Data.(events.HarnessStatus)
			if ok && status.Status == wanted {
				return event
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for harness status %q", wanted)
	return events.Event{}
}

func waitForHarnessMetadataModel(t *testing.T, bus *events.Bus, sessionID, wanted string, timeout time.Duration) events.Event {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, event := range bus.History(sessionID, 0, 1024) {
			if event.Type != events.TypeHarnessMetadata {
				continue
			}
			metadata, ok := event.Data.(events.HarnessMetadata)
			if ok && metadata.Model == wanted {
				return event
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for harness model %q", wanted)
	return events.Event{}
}

func TestManagerWithBusPublishesSessionCreated(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(events.SubscribeOptions{Buffer: 32})
	defer sub.Close()

	mgr := NewManagerWithBus(bus)
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	byType := collectEventsByType(sub, 2*time.Second)
	created, ok := byType[events.TypeSessionCreated]
	if !ok || len(created) == 0 {
		t.Fatal("no session.created event received")
	}
	if created[0].SessionID != sess.ID {
		t.Fatalf("session.created SessionID = %q, want %q", created[0].SessionID, sess.ID)
	}
	payload, ok := created[0].Data.(events.SessionCreated)
	if !ok {
		t.Fatalf("session.created Data type = %T, want events.SessionCreated", created[0].Data)
	}
	if payload.AdapterID != "generic" || payload.Status != string(StatusStarting) {
		t.Fatalf("session.created payload = %+v", payload)
	}
}

func TestManagerWithBusPublishesTerminalOutput(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(events.SubscribeOptions{Buffer: 64})
	defer sub.Close()

	mgr := NewManagerWithBus(bus)
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	byType := collectEventsByType(sub, 2*time.Second)
	outputs, ok := byType[events.TypeTerminalOutput]
	if !ok || len(outputs) == 0 {
		t.Fatal("no terminal.output events received")
	}

	var full strings.Builder
	for _, ev := range outputs {
		if ev.SessionID != sess.ID {
			t.Fatalf("terminal.output SessionID = %q, want %q", ev.SessionID, sess.ID)
		}
		d, ok := ev.Data.(events.TerminalOutput)
		if ok {
			full.Write(d.Data)
		}
	}
	if !strings.Contains(full.String(), "plain stdout") {
		t.Fatalf("terminal.output missing stdout: %q", full.String())
	}
}

func TestManagerWithBusPublishesStatusChanged(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(events.SubscribeOptions{Buffer: 32})
	defer sub.Close()

	mgr := NewManagerWithBus(bus)
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	byType := collectEventsByType(sub, 2*time.Second)
	statuses, ok := byType[events.TypeSessionStatusChanged]
	if !ok || len(statuses) == 0 {
		t.Fatal("no session.status_changed events received")
	}

	for _, ev := range statuses {
		if ev.SessionID != sess.ID {
			t.Fatalf("session.status_changed SessionID = %q, want %q", ev.SessionID, sess.ID)
		}
	}

	foundStartingToRunning := false
	foundRunningToExited := false
	for _, ev := range statuses {
		d, ok := ev.Data.(events.SessionStatusChanged)
		if !ok {
			continue
		}
		if d.OldStatus == "starting" && d.NewStatus == "running" {
			foundStartingToRunning = true
		}
		if d.OldStatus == "running" && d.NewStatus == "exited" {
			foundRunningToExited = true
		}
	}
	if !foundStartingToRunning {
		t.Fatal("missing starting->running status change")
	}
	if !foundRunningToExited {
		t.Fatal("missing running->exited status change")
	}
}

func TestManagerWithBusPublishesSessionExited(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(events.SubscribeOptions{Buffer: 32})
	defer sub.Close()

	mgr := NewManagerWithBus(bus)
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	byType := collectEventsByType(sub, 2*time.Second)
	exited, ok := byType[events.TypeSessionExited]
	if !ok || len(exited) == 0 {
		t.Fatal("no session.exited event received")
	}

	ev := exited[len(exited)-1]
	if ev.SessionID != sess.ID {
		t.Fatalf("session.exited SessionID = %q, want %q", ev.SessionID, sess.ID)
	}
	d, ok := ev.Data.(events.SessionExited)
	if !ok {
		t.Fatalf("session.exited Data type = %T, want SessionExited", ev.Data)
	}
	if d.ExitCode != 0 {
		t.Fatalf("session.exited ExitCode = %d, want 0", d.ExitCode)
	}
	if d.Reason != "process_exit" {
		t.Fatalf("session.exited Reason = %q, want process_exit", d.Reason)
	}
}

func TestManagerWithBusTerminatePublishesSignalReason(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(events.SubscribeOptions{
		Buffer: 32,
		Types:  []events.Type{events.TypeSessionExited},
	})
	defer sub.Close()

	mgr := NewManagerWithBus(bus)
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "long-running.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.Terminate(ctx, sess.ID); err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	byType := collectEventsByType(sub, 2*time.Second)
	exited, ok := byType[events.TypeSessionExited]
	if !ok || len(exited) == 0 {
		t.Fatal("no session.exited event received")
	}

	ev := exited[len(exited)-1]
	d, ok := ev.Data.(events.SessionExited)
	if !ok {
		t.Fatalf("session.exited Data type = %T", ev.Data)
	}
	if d.Reason != "signal" {
		t.Fatalf("session.exited Reason = %q, want signal", d.Reason)
	}
}

func TestManagerWithBusInterruptPublishesExited(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(events.SubscribeOptions{
		Buffer: 32,
		Types:  []events.Type{events.TypeSessionExited},
	})
	defer sub.Close()

	mgr := NewManagerWithBus(bus)
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "long-running.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := mgr.Interrupt(sess.ID); err != nil {
		t.Fatalf("Interrupt: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	byType := collectEventsByType(sub, 2*time.Second)
	exited, ok := byType[events.TypeSessionExited]
	if !ok || len(exited) == 0 {
		t.Fatal("no session.exited event received")
	}

	ev := exited[len(exited)-1]
	d, ok := ev.Data.(events.SessionExited)
	if !ok {
		t.Fatalf("session.exited Data type = %T", ev.Data)
	}
	if d.Reason != "process_exit" && d.Reason != "signal" {
		t.Fatalf("session.exited Reason = %q, want process_exit or signal", d.Reason)
	}
}

func TestManagerWithoutBusDoesNotPublish(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(events.SubscribeOptions{Buffer: 64})
	defer sub.Close()

	mgr := NewManager()
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	sub.Close()
	count := 0
	for range sub.C {
		count++
	}
	if count > 0 {
		t.Fatalf("received %d events without bus, want 0", count)
	}
}

func TestManagerWithBusSequenceOrder(t *testing.T) {
	bus := events.NewBus()
	sub := bus.Subscribe(events.SubscribeOptions{Buffer: 64})
	defer sub.Close()

	mgr := NewManagerWithBus(bus)
	sess, err := mgr.Create(context.Background(), CreateOptions{
		Command: "/bin/sh",
		Args:    []string{fixturePath(t, "plain-output.sh")},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	waitSessionDone(t, sess, 5*time.Second)

	byType := collectEventsByType(sub, 2*time.Second)

	var all []events.Event
	all = append(all, byType[events.TypeSessionCreated]...)
	all = append(all, byType[events.TypeSessionStatusChanged]...)
	all = append(all, byType[events.TypeTerminalOutput]...)
	all = append(all, byType[events.TypeSessionExited]...)

	if len(all) == 0 {
		t.Fatal("no events received")
	}
	for _, ev := range all {
		if ev.Sequence == 0 {
			t.Fatalf("event %q has sequence 0", ev.Type)
		}
	}
}
