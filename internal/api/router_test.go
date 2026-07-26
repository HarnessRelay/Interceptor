package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/harness"
	"github.com/harnessrelay/interceptor/internal/security"
	"github.com/harnessrelay/interceptor/internal/session"
	"github.com/harnessrelay/interceptor/internal/storage"
)

const testAuthToken = "test-local-token"

func TestHealthEndpoint(t *testing.T) {
	router := NewRouter(Options{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version:   "test-version",
		StaticFS:  testStaticFS(),
		Harnesses: []harness.Detected{},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	want := `{"status":"ok","service":"harnessd","version":"test-version"}`
	if strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("body = %q, want %q", strings.TrimSpace(rec.Body.String()), want)
	}
}

func TestStaticRootAndAPINotFoundAreSeparated(t *testing.T) {
	router := NewRouter(Options{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version:   "test-version",
		StaticFS:  testStaticFS(),
		Harnesses: []harness.Detected{},
	})

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRec := httptest.NewRecorder()
	router.ServeHTTP(rootRec, rootReq)
	if rootRec.Code != http.StatusOK {
		t.Fatalf("root status = %d, want %d", rootRec.Code, http.StatusOK)
	}
	if !strings.Contains(rootRec.Body.String(), "HarnessRelay Interceptor") {
		t.Fatalf("root body did not contain dashboard placeholder: %q", rootRec.Body.String())
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/api/v1/missing", nil)
	apiRec := httptest.NewRecorder()
	router.ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusNotFound {
		t.Fatalf("api miss status = %d, want %d", apiRec.Code, http.StatusNotFound)
	}
	if strings.Contains(apiRec.Body.String(), "HarnessRelay Interceptor") {
		t.Fatalf("api miss served static dashboard: %q", apiRec.Body.String())
	}
}

func TestSessionRESTCreateListGetSnapshotAndEvents(t *testing.T) {
	router, _, _ := newTestRouter()

	createBody := map[string]any{
		"name":    "plain",
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "plain-output.sh")},
		"terminal": map[string]any{
			"rows": 24,
			"cols": 80,
		},
	}
	createRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions", createBody)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	var created sessionResponse
	decodeBody(t, createRec, &created)
	if created.Session.ID == "" {
		t.Fatal("created session ID is empty")
	}
	if created.Session.AdapterID != "generic" {
		t.Fatalf("adapter = %q, want generic", created.Session.AdapterID)
	}

	waitForStatus(t, router, created.Session.ID, session.StatusExited)

	listRec := serveJSON(t, router, http.MethodGet, "/api/v1/sessions", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRec.Code)
	}
	var list sessionsResponse
	decodeBody(t, listRec, &list)
	if len(list.Sessions) != 1 || list.Sessions[0].ID != created.Session.ID {
		t.Fatalf("unexpected session list: %+v", list.Sessions)
	}

	getRec := serveJSON(t, router, http.MethodGet, "/api/v1/sessions/"+created.Session.ID, nil)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d", getRec.Code)
	}
	var got sessionResponse
	decodeBody(t, getRec, &got)
	if got.Session.Status != string(session.StatusExited) {
		t.Fatalf("status = %q, want exited", got.Session.Status)
	}

	snapshotRec := serveJSON(t, router, http.MethodGet, "/api/v1/sessions/"+created.Session.ID+"/snapshot", nil)
	if snapshotRec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d", snapshotRec.Code)
	}
	var snapshot snapshotResponse
	decodeBody(t, snapshotRec, &snapshot)
	if snapshot.SessionID != created.Session.ID {
		t.Fatalf("snapshot session = %q", snapshot.SessionID)
	}
	if snapshot.Timestamp.IsZero() {
		t.Fatal("snapshot timestamp is zero")
	}
	if len(snapshot.Chunks) == 0 {
		t.Fatal("snapshot did not include output chunks")
	}
	bytes, err := base64.StdEncoding.DecodeString(snapshot.Chunks[0].Bytes)
	if err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if !strings.Contains(string(bytes), "plain stdout") {
		t.Fatalf("snapshot missing output: %q", string(bytes))
	}

	eventsRec := serveJSON(t, router, http.MethodGet, "/api/v1/sessions/"+created.Session.ID+"/events?limit=10", nil)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("events status = %d", eventsRec.Code)
	}
	var eventList eventsResponse
	decodeBody(t, eventsRec, &eventList)
	if len(eventList.Events) == 0 {
		t.Fatal("events endpoint returned no events")
	}
}

func TestCodexSessionPromptAndApprovalActionAPI(t *testing.T) {
	router, mgr, bus := newTestRouter()
	created := createSession(t, router, map[string]any{
		"name":    "semantic-codex",
		"command": fixturePath(t, "codex"),
		"cwd":     t.TempDir(),
	})
	t.Cleanup(func() {
		sess, ok := mgr.Get(created.ID)
		if !ok || sess.Info().Status != session.StatusRunning {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = mgr.Terminate(ctx, created.ID)
	})

	if created.AdapterID != "codex" || created.AdapterName != "Codex" {
		t.Fatalf("adapter = %q/%q, want codex/Codex", created.AdapterID, created.AdapterName)
	}
	if !slices.Contains(created.AdapterCapabilities, "semantic_chat") ||
		!slices.Contains(created.AdapterCapabilities, "prompt_submit") {
		t.Fatalf("adapter capabilities = %v", created.AdapterCapabilities)
	}
	waitForManagerSnapshotText(t, mgr, created.ID, "OpenAI Codex", 5*time.Second)
	waitForBusHarnessStatus(t, bus, created.ID, "idle", 5*time.Second)

	commandsRec := serveJSON(t, router, http.MethodGet, "/api/v1/sessions/"+created.ID+"/commands", nil)
	if commandsRec.Code != http.StatusOK {
		t.Fatalf("commands status = %d, body = %s", commandsRec.Code, commandsRec.Body.String())
	}
	var catalog commandsResponse
	decodeBody(t, commandsRec, &catalog)
	if !catalog.Supported || len(catalog.Commands) < 20 {
		t.Fatalf("unexpected command catalog: %+v", catalog)
	}
	commandRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+created.ID+"/commands/status", map[string]any{})
	if commandRec.Code != http.StatusOK {
		t.Fatalf("command status = %d, body = %s", commandRec.Code, commandRec.Body.String())
	}
	waitForManagerSnapshotText(t, mgr, created.ID, "RECEIVED:/status", 5*time.Second)
	time.Sleep(3200 * time.Millisecond)

	responseRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+created.ID+"/prompt", map[string]any{
		"text": "hello via API",
	})
	if responseRec.Code != http.StatusOK {
		t.Fatalf("response prompt status = %d, body = %s", responseRec.Code, responseRec.Body.String())
	}
	assistant := waitForBusEvent(t, bus, created.ID, events.TypeChatAssistantMessage, 5*time.Second)
	assistantData := assistant.Data.(events.ChatMessage)
	if assistantData.Content != "Fake Codex response to: hello via API" || assistantData.MessageID != "codex-turn-1" {
		t.Fatalf("assistant message = %+v", assistantData)
	}
	waitForBusHarnessStatus(t, bus, created.ID, "idle", 5*time.Second)

	promptRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+created.ID+"/prompt", map[string]any{
		"text": "request approval",
	})
	if promptRec.Code != http.StatusOK {
		t.Fatalf("prompt status = %d, body = %s", promptRec.Code, promptRec.Body.String())
	}
	waitForManagerSnapshotText(t, mgr, created.ID, "Would you like to run the following command?", 5*time.Second)
	approval := waitForBusEvent(t, bus, created.ID, events.TypeApprovalRequired, 5*time.Second)
	time.Sleep(20 * time.Millisecond)

	actionRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+created.ID+"/actions/codex.approval_deny", map[string]any{
		"event_id":       approval.ID,
		"action_version": 1,
	})
	if actionRec.Code != http.StatusOK {
		t.Fatalf("deny action status = %d, body = %s", actionRec.Code, actionRec.Body.String())
	}
	waitForManagerSnapshotText(t, mgr, created.ID, "DENIED", 5*time.Second)

	replayRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+created.ID+"/actions/codex.approval_deny", map[string]any{
		"event_id":       approval.ID,
		"action_version": 1,
	})
	if replayRec.Code != http.StatusConflict {
		t.Fatalf("replayed action status = %d, want %d", replayRec.Code, http.StatusConflict)
	}
}

func TestHarnessesEndpointListsDetectedHarnesses(t *testing.T) {
	router := NewRouter(Options{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version:  "test-version",
		StaticFS: testStaticFS(),
		Auth:     security.NewAuthenticator(testAuthToken),
		Harnesses: []harness.Detected{
			{
				Definition: harness.Definition{
					ID:          "codex",
					Name:        "Codex",
					Command:     "codex",
					DefaultMode: "chat",
					Description: "OpenAI Codex CLI",
				},
				Installed: true,
				Path:      "/tmp/bin/codex",
				Version:   "codex-cli test",
			},
		},
	})

	rec := serveJSON(t, router, http.MethodGet, "/api/v1/harnesses", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("harnesses status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp harnessesResponse
	decodeBody(t, rec, &resp)
	if len(resp.Harnesses) != 1 {
		t.Fatalf("harness count = %d, want 1", len(resp.Harnesses))
	}
	got := resp.Harnesses[0]
	if got.ID != "codex" || got.Command != "codex" || got.DefaultMode != "chat" || !got.Installed {
		t.Fatalf("unexpected harness response: %+v", got)
	}
	if got.Path != "/tmp/bin/codex" || got.Version != "codex-cli test" {
		t.Fatalf("missing path/version metadata: %+v", got)
	}
}

func TestSessionRESTInputResizeInterruptTerminateAndDelete(t *testing.T) {
	router, _, _ := newTestRouter()

	interactive := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "interactive-echo.sh")},
	})
	waitForSnapshotText(t, router, interactive.ID, "input>")
	inputRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+interactive.ID+"/input", map[string]any{
		"mode":     "raw",
		"encoding": "base64",
		"data":     base64.StdEncoding.EncodeToString([]byte("hello\n")),
	})
	if inputRec.Code != http.StatusOK {
		t.Fatalf("input status = %d, body = %s", inputRec.Code, inputRec.Body.String())
	}
	waitForSnapshotText(t, router, interactive.ID, "echo:hello")

	resize := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "resize-aware.sh")},
		"terminal": map[string]any{
			"rows": 24,
			"cols": 80,
		},
	})
	waitForSnapshotText(t, router, resize.ID, "ready")
	resizeRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+resize.ID+"/resize", map[string]any{
		"rows": 40,
		"cols": 100,
	})
	if resizeRec.Code != http.StatusOK {
		t.Fatalf("resize status = %d, body = %s", resizeRec.Code, resizeRec.Body.String())
	}
	_ = serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+resize.ID+"/input", map[string]any{
		"mode": "text",
		"text": "size\n",
	})
	waitForSnapshotText(t, router, resize.ID, "40 100")

	interrupt := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "long-running.sh")},
	})
	waitForSnapshotText(t, router, interrupt.ID, "ready")
	interruptRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+interrupt.ID+"/interrupt", map[string]any{
		"strategy": "ctrl_c",
	})
	if interruptRec.Code != http.StatusOK {
		t.Fatalf("interrupt status = %d, body = %s", interruptRec.Code, interruptRec.Body.String())
	}
	waitForSnapshotText(t, router, interrupt.ID, "interrupted")

	terminate := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "long-running.sh")},
	})
	waitForSnapshotText(t, router, terminate.ID, "ready")
	terminateRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+terminate.ID+"/terminate", map[string]any{
		"grace_ms": 1000,
	})
	if terminateRec.Code != http.StatusOK {
		t.Fatalf("terminate status = %d, body = %s", terminateRec.Code, terminateRec.Body.String())
	}
	waitForStatus(t, router, terminate.ID, session.StatusTerminated)

	deleteSess := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "long-running.sh")},
	})
	waitForSnapshotText(t, router, deleteSess.ID, "ready")
	deleteRec := serveJSON(t, router, http.MethodDelete, "/api/v1/sessions/"+deleteSess.ID, nil)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", deleteRec.Code, deleteRec.Body.String())
	}
	waitForStatus(t, router, deleteSess.ID, session.StatusTerminated)

	killSess := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "long-running.sh")},
	})
	waitForSnapshotText(t, router, killSess.ID, "ready")
	badKillRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+killSess.ID+"/kill", map[string]any{
		"confirmation": "kill",
	})
	if badKillRec.Code != http.StatusBadRequest {
		t.Fatalf("bad kill status = %d, body = %s", badKillRec.Code, badKillRec.Body.String())
	}
	killRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+killSess.ID+"/kill", map[string]any{
		"confirmation": "KILL",
	})
	if killRec.Code != http.StatusOK {
		t.Fatalf("kill status = %d, body = %s", killRec.Code, killRec.Body.String())
	}
	waitForStatus(t, router, killSess.ID, session.StatusTerminated)
}

func TestSessionRESTCleanupCompletedSession(t *testing.T) {
	router, _, _ := newTestRouter()
	running := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "long-running.sh")},
	})
	t.Cleanup(func() {
		_ = serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+running.ID+"/terminate", map[string]any{"grace_ms": 100})
	})
	runningCleanup := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+running.ID+"/cleanup", nil)
	if runningCleanup.Code != http.StatusConflict {
		t.Fatalf("running cleanup status = %d, want %d", runningCleanup.Code, http.StatusConflict)
	}

	completed := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "plain-output.sh")},
	})
	waitForStatus(t, router, completed.ID, session.StatusExited)
	cleanupRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+completed.ID+"/cleanup", nil)
	if cleanupRec.Code != http.StatusOK {
		t.Fatalf("cleanup status = %d, body = %s", cleanupRec.Code, cleanupRec.Body.String())
	}
	getRec := serveJSON(t, router, http.MethodGet, "/api/v1/sessions/"+completed.ID, nil)
	if getRec.Code != http.StatusNotFound {
		t.Fatalf("get after cleanup status = %d, want %d", getRec.Code, http.StatusNotFound)
	}
}

func TestSessionRESTValidation(t *testing.T) {
	router, _, _ := newTestRouter()

	createRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions", map[string]any{
		"command": "",
	})
	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("empty command status = %d", createRec.Code)
	}

	inputRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/ses_missing/input", map[string]any{
		"mode":     "raw",
		"encoding": "base64",
		"data":     "not-base64",
	})
	if inputRec.Code != http.StatusNotFound {
		t.Fatalf("missing session input status = %d", inputRec.Code)
	}

	sess := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "long-running.sh")},
	})
	t.Cleanup(func() {
		_ = serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+sess.ID+"/terminate", map[string]any{"grace_ms": 100})
	})
	badInputRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+sess.ID+"/input", map[string]any{
		"mode":     "raw",
		"encoding": "base64",
		"data":     "not-base64",
	})
	if badInputRec.Code != http.StatusBadRequest {
		t.Fatalf("bad input status = %d", badInputRec.Code)
	}
	badResizeRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+sess.ID+"/resize", map[string]any{
		"rows": 0,
		"cols": 100,
	})
	if badResizeRec.Code != http.StatusBadRequest {
		t.Fatalf("bad resize status = %d", badResizeRec.Code)
	}
}

func TestInputBytesSpecialKeys(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{key: "Enter", want: "\r"},
		{key: "Escape", want: "\x1b"},
		{key: "Tab", want: "\t"},
		{key: "ArrowUp", want: "\x1b[A"},
		{key: "ArrowDown", want: "\x1b[B"},
		{key: "ArrowRight", want: "\x1b[C"},
		{key: "ArrowLeft", want: "\x1b[D"},
		{key: "CtrlC", want: "\x03"},
	}
	for _, tt := range tests {
		got, err := inputBytes(inputRequest{Mode: "key", Key: tt.key})
		if err != nil {
			t.Fatalf("inputBytes(%s): %v", tt.key, err)
		}
		if string(got) != tt.want {
			t.Fatalf("inputBytes(%s) = %q, want %q", tt.key, string(got), tt.want)
		}
	}
	if _, err := inputBytes(inputRequest{Mode: "key", Key: "Nope"}); err == nil {
		t.Fatal("unsupported key returned nil error")
	}
}

func TestSessionRESTSpecialKeyInput(t *testing.T) {
	router, _, _ := newTestRouter()
	sess := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "interactive-echo.sh")},
	})
	waitForSnapshotText(t, router, sess.ID, "input>")
	textRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+sess.ID+"/input", map[string]any{
		"mode": "text",
		"text": "hello",
	})
	if textRec.Code != http.StatusOK {
		t.Fatalf("text input status = %d, body = %s", textRec.Code, textRec.Body.String())
	}
	enterRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+sess.ID+"/input", map[string]any{
		"mode": "key",
		"key":  "Enter",
	})
	if enterRec.Code != http.StatusOK {
		t.Fatalf("key input status = %d, body = %s", enterRec.Code, enterRec.Body.String())
	}
	waitForSnapshotText(t, router, sess.ID, "echo:hello")
}

func TestWebSocketRequiresUpgrade(t *testing.T) {
	router, _, _ := newTestRouter()
	rec := serveJSON(t, router, http.MethodGet, "/api/v1/ws", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestWebSocketRejectsUnknownSessionFilter(t *testing.T) {
	router, _, _ := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws?session_id=ses_missing", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", websocketTestKey("missing"))
	req.Header.Set("Authorization", "Bearer "+testAuthToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAPIRequiresAuthentication(t *testing.T) {
	router, _, _ := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestCookieAuthRequiresCSRFForUnsafeRequests(t *testing.T) {
	router, _, _ := newTestRouter()
	loginRec := serveRawJSON(t, router, http.MethodPost, "/api/v1/auth/login", map[string]any{"token": testAuthToken})
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginRec.Code, loginRec.Body.String())
	}
	var auth authStatusResponse
	decodeBody(t, loginRec, &auth)
	if !auth.Authenticated || auth.CSRFToken == "" {
		t.Fatalf("unexpected auth response: %+v", auth)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(`{"command":"/bin/sh"}`))
	req.Header.Set("Content-Type", "application/json")
	for _, cookie := range loginRec.Result().Cookies() {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing CSRF status = %d, want %d", rec.Code, http.StatusForbidden)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(`{"command":"/bin/sh","args":["`+fixturePath(t, "plain-output.sh")+`"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(security.CSRFHeaderName, auth.CSRFToken)
	for _, cookie := range loginRec.Result().Cookies() {
		req.AddCookie(cookie)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("with CSRF status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestUnexpectedOriginRejected(t *testing.T) {
	router, _, _ := newTestRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sessions", strings.NewReader(`{"command":"/bin/sh"}`))
	req.Host = "127.0.0.1:8765"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAuthToken)
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestSessionActionRejectsStaleOrUnknownActions(t *testing.T) {
	router, _, bus := newTestRouter()
	sess := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "plain-output.sh")},
	})

	missingEventRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+sess.ID+"/actions/approve_once", map[string]any{
		"event_id": "evt_missing",
	})
	if missingEventRec.Code != http.StatusConflict {
		t.Fatalf("missing event status = %d, want %d", missingEventRec.Code, http.StatusConflict)
	}

	event := bus.Publish(nil, events.Event{
		ID:        "evt_action_test",
		Type:      events.TypeApprovalRequired,
		SessionID: sess.ID,
		Data: map[string]any{
			"actions": []map[string]any{
				{"id": "approve_once", "version": 1},
			},
		},
	})
	if event.ID == "" {
		t.Fatal("published event ID is empty")
	}

	versionMismatchRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+sess.ID+"/actions/approve_once", map[string]any{
		"event_id":       "evt_action_test",
		"action_version": 2,
	})
	if versionMismatchRec.Code != http.StatusConflict {
		t.Fatalf("version mismatch status = %d, want %d", versionMismatchRec.Code, http.StatusConflict)
	}

	unsupportedRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+sess.ID+"/actions/approve_once", map[string]any{
		"event_id":       "evt_action_test",
		"action_version": 1,
	})
	if unsupportedRec.Code != http.StatusConflict {
		t.Fatalf("unregistered action status = %d, want %d", unsupportedRec.Code, http.StatusConflict)
	}
}

func TestSessionControlWritesAuditMetadataWithoutInputPayload(t *testing.T) {
	bus := events.NewBus()
	mgr := session.NewManagerWithBus(bus)
	audit := storage.NewAuditLog(10)
	router := NewRouter(Options{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version:   "test-version",
		StaticFS:  testStaticFS(),
		Sessions:  mgr,
		Events:    bus,
		Auth:      security.NewAuthenticator(testAuthToken),
		Audit:     audit,
		Harnesses: []harness.Detected{},
	})

	sess := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "interactive-echo.sh")},
	})
	waitForSnapshotText(t, router, sess.ID, "input>")
	inputRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+sess.ID+"/input", map[string]any{
		"mode": "text",
		"text": "super-secret-value\n",
	})
	if inputRec.Code != http.StatusOK {
		t.Fatalf("input status = %d, body = %s", inputRec.Code, inputRec.Body.String())
	}

	records := audit.List()
	if len(records) < 2 {
		t.Fatalf("audit record count = %d, want at least 2", len(records))
	}
	var inputAudit storage.AuditRecord
	for _, record := range records {
		if record.Type == "session.input" {
			inputAudit = record
		}
	}
	if inputAudit.ID == "" {
		t.Fatal("missing session.input audit record")
	}
	if inputAudit.Metadata["bytes"] == nil {
		t.Fatalf("input audit missing byte count: %+v", inputAudit.Metadata)
	}
	for _, value := range inputAudit.Metadata {
		if value == "super-secret-value\n" {
			t.Fatal("input audit stored raw input payload")
		}
	}

	interruptSess := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "long-running.sh")},
	})
	waitForSnapshotText(t, router, interruptSess.ID, "ready")
	interruptRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+interruptSess.ID+"/interrupt", map[string]any{"strategy": "ctrl_c"})
	if interruptRec.Code != http.StatusOK {
		t.Fatalf("interrupt status = %d, body = %s", interruptRec.Code, interruptRec.Body.String())
	}

	terminateSess := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "long-running.sh")},
	})
	waitForSnapshotText(t, router, terminateSess.ID, "ready")
	terminateRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+terminateSess.ID+"/terminate", map[string]any{"grace_ms": 1000})
	if terminateRec.Code != http.StatusOK {
		t.Fatalf("terminate status = %d, body = %s", terminateRec.Code, terminateRec.Body.String())
	}

	types := map[string]bool{}
	for _, record := range audit.List() {
		types[record.Type] = true
	}
	plain := createSession(t, router, map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "plain-output.sh")},
	})
	waitForStatus(t, router, plain.ID, session.StatusExited)
	cleanupRec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions/"+plain.ID+"/cleanup", nil)
	if cleanupRec.Code != http.StatusOK {
		t.Fatalf("cleanup status = %d, body = %s", cleanupRec.Code, cleanupRec.Body.String())
	}

	types = map[string]bool{}
	for _, record := range audit.List() {
		types[record.Type] = true
	}
	for _, typ := range []string{"session.create", "session.input", "session.interrupt", "session.terminate", "session.cleanup"} {
		if !types[typ] {
			t.Fatalf("missing audit type %s in %v", typ, types)
		}
	}
}

func TestWebSocketRequiresAuthenticationAndSameOrigin(t *testing.T) {
	router, _, _ := newTestRouter()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", websocketTestKey("unauth"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	req.Host = "127.0.0.1:8765"
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", websocketTestKey("origin"))
	req.Header.Set("Authorization", "Bearer "+testAuthToken)
	req.Header.Set("Origin", "http://evil.example")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("origin status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestWebSocketStreamsSessionEvents(t *testing.T) {
	router, _, _ := newTestRouter()
	server := httptest.NewServer(router)
	defer server.Close()

	conn, reader := openWebSocket(t, server.URL+"/api/v1/ws")
	defer conn.Close()

	resp := postServerJSON(t, server.URL+"/api/v1/sessions", map[string]any{
		"command": "/bin/sh",
		"args":    []string{fixturePath(t, "plain-output.sh")},
	})
	if resp.Session.ID == "" {
		t.Fatal("session ID is empty")
	}

	deadline := time.Now().Add(5 * time.Second)
	seenCreated := false
	seenOutput := false
	for time.Now().Before(deadline) {
		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		payload, err := readServerTextFrame(reader)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			t.Fatalf("read websocket frame: %v", err)
		}
		var event events.Event
		if err := json.Unmarshal(payload, &event); err != nil {
			t.Fatalf("unmarshal event %q: %v", string(payload), err)
		}
		if event.SessionID != resp.Session.ID {
			continue
		}
		switch event.Type {
		case events.TypeSessionCreated:
			seenCreated = true
		case events.TypeTerminalOutput:
			seenOutput = true
		}
		if seenCreated && seenOutput {
			return
		}
	}
	t.Fatalf("did not see expected websocket events: created=%v output=%v", seenCreated, seenOutput)
}

func testStaticFS() fs.FS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<!doctype html><title>HarnessRelay Interceptor</title>")},
	}
}

func newTestRouter() (http.Handler, *session.Manager, *events.Bus) {
	bus := events.NewBus()
	mgr := session.NewManagerWithBus(bus)
	router := NewRouter(Options{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version:   "test-version",
		StaticFS:  testStaticFS(),
		Sessions:  mgr,
		Events:    bus,
		Auth:      security.NewAuthenticator(testAuthToken),
		Harnesses: []harness.Detected{},
	})
	return router, mgr, bus
}

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

func createSession(t *testing.T, router http.Handler, body map[string]any) sessionDTO {
	t.Helper()
	rec := serveJSON(t, router, http.MethodPost, "/api/v1/sessions", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sessionResponse
	decodeBody(t, rec, &resp)
	return resp.Session
}

func waitForManagerSnapshotText(t *testing.T, mgr *session.Manager, sessionID, wanted string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		sess, ok := mgr.Get(sessionID)
		if ok && strings.Contains(string(sess.Snapshot()), wanted) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for terminal output %q", wanted)
}

func waitForBusEvent(t *testing.T, bus *events.Bus, sessionID string, wanted events.Type, timeout time.Duration) events.Event {
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
	t.Fatalf("timed out waiting for event %q", wanted)
	return events.Event{}
}

func waitForBusHarnessStatus(t *testing.T, bus *events.Bus, sessionID, wanted string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, event := range bus.History(sessionID, 0, 1024) {
			if event.Type != events.TypeHarnessStatus {
				continue
			}
			status, ok := event.Data.(events.HarnessStatus)
			if ok && status.Status == wanted {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for harness status %q", wanted)
}

func serveJSON(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	rec := serveRawJSON(t, router, method, path, body)
	return rec
}

func serveRawJSON(t *testing.T, router http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
		reader = &buf
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if !strings.HasPrefix(path, "/api/v1/auth/") {
		req.Header.Set("Authorization", "Bearer "+testAuthToken)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(dst); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
}

func postServerJSON(t *testing.T, url string, body any) sessionResponse {
	t.Helper()
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		t.Fatalf("encode server body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		t.Fatalf("new server request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAuthToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post server json: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("post status = %d, body = %s", resp.StatusCode, string(data))
	}
	var decoded sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode server response: %v", err)
	}
	return decoded
}

func openWebSocket(t *testing.T, rawURL string) (net.Conn, *bufio.Reader) {
	t.Helper()
	addr := strings.TrimPrefix(rawURL, "http://")
	slash := strings.IndexByte(addr, '/')
	host := addr[:slash]
	path := addr[slash:]
	conn, err := net.Dial("tcp", host)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	key := websocketTestKey("stream")
	request := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + host + "\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: websocket\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n\r\n"
	request = strings.Replace(request, "\r\n\r\n", "\r\nAuthorization: Bearer "+testAuthToken+"\r\n\r\n", 1)
	if _, err := conn.Write([]byte(request)); err != nil {
		_ = conn.Close()
		t.Fatalf("write websocket handshake: %v", err)
	}
	reader := bufio.NewReader(conn)
	statusLine, err := reader.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		t.Fatalf("read websocket status: %v", err)
	}
	status, err := parseHTTPStatusLine(strings.TrimSpace(statusLine))
	if err != nil {
		_ = conn.Close()
		t.Fatalf("parse websocket status: %v", err)
	}
	if status != http.StatusSwitchingProtocols {
		_ = conn.Close()
		t.Fatalf("websocket status = %d, want %d", status, http.StatusSwitchingProtocols)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			t.Fatalf("read websocket headers: %v", err)
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	return conn, reader
}

func websocketTestKey(seed string) string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%-16.16s", seed)))
}

func parseHTTPStatusLine(line string) (int, error) {
	parts := strings.Split(line, " ")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid status line %q", line)
	}
	return strconv.Atoi(parts[1])
}

func readServerTextFrame(reader *bufio.Reader) ([]byte, error) {
	var header [2]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, err
	}
	opcode := header[0] & 0x0f
	if opcode != 0x1 {
		return nil, io.ErrUnexpectedEOF
	}
	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(reader, ext[:]); err != nil {
			return nil, err
		}
		length = uint64(ext[0])<<8 | uint64(ext[1])
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(reader, ext[:]); err != nil {
			return nil, err
		}
		length = 0
		for _, b := range ext {
			length = length<<8 | uint64(b)
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func waitForStatus(t *testing.T, router http.Handler, id string, want session.Status) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec := serveJSON(t, router, http.MethodGet, "/api/v1/sessions/"+id, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("get status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var resp sessionResponse
		decodeBody(t, rec, &resp)
		if resp.Session.Status == string(want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for session %s to become %s", id, want)
}

func waitForSnapshotText(t *testing.T, router http.Handler, id, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rec := serveJSON(t, router, http.MethodGet, "/api/v1/sessions/"+id+"/snapshot", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("snapshot status = %d, body = %s", rec.Code, rec.Body.String())
		}
		var resp snapshotResponse
		decodeBody(t, rec, &resp)
		var out strings.Builder
		for _, chunk := range resp.Chunks {
			data, err := base64.StdEncoding.DecodeString(chunk.Bytes)
			if err != nil {
				t.Fatalf("decode snapshot chunk: %v", err)
			}
			out.Write(data)
		}
		if strings.Contains(out.String(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for snapshot text %q in session %s", want, id)
}
