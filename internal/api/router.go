package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/harness"
	"github.com/harnessrelay/interceptor/internal/logging"
	"github.com/harnessrelay/interceptor/internal/security"
	"github.com/harnessrelay/interceptor/internal/session"
	"github.com/harnessrelay/interceptor/internal/storage"
)

type Options struct {
	Logger    *slog.Logger
	Version   string
	StaticFS  fs.FS
	Sessions  *session.Manager
	Events    *events.Bus
	Auth      *security.Authenticator
	Audit     *storage.AuditLog
	Harnesses []harness.Detected
}

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

type errorResponse struct {
	Error string `json:"error"`
}

type loginRequest struct {
	Token string `json:"token"`
}

type authStatusResponse struct {
	Authenticated bool   `json:"authenticated"`
	CSRFToken     string `json:"csrf_token,omitempty"`
}

type sessionResponse struct {
	Session sessionDTO `json:"session"`
}

type sessionsResponse struct {
	Sessions []sessionDTO `json:"sessions"`
}

type harnessesResponse struct {
	Harnesses []harnessDTO `json:"harnesses"`
}

type createSessionRequest struct {
	Name        string            `json:"name"`
	HarnessType string            `json:"harness_type"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	CWD         string            `json:"cwd"`
	WorkDir     string            `json:"work_dir"`
	Env         map[string]string `json:"env"`
	Terminal    terminalDTO       `json:"terminal"`
}

type inputRequest struct {
	Mode     string `json:"mode"`
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
	Text     string `json:"text"`
	Key      string `json:"key"`
}

type promptRequest struct {
	Text string `json:"text"`
}

type resizeRequest struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

type interruptRequest struct {
	Strategy string `json:"strategy"`
}

type terminateRequest struct {
	GraceMS int `json:"grace_ms"`
}

type killRequest struct {
	Confirmation string `json:"confirmation"`
}

type actionRequest struct {
	EventID       string         `json:"event_id"`
	ActionVersion int            `json:"action_version"`
	Params        map[string]any `json:"params"`
}

type actionResultResponse struct {
	Result actionResult `json:"result"`
}

type actionResult struct {
	Status   string `json:"status"`
	EventID  string `json:"event_id"`
	ActionID string `json:"action_id"`
}

type sessionDTO struct {
	ID                  string      `json:"id"`
	Name                string      `json:"name,omitempty"`
	HarnessType         string      `json:"harness_type"`
	AdapterID           string      `json:"adapter_id"`
	AdapterName         string      `json:"adapter_name"`
	AdapterCapabilities []string    `json:"adapter_capabilities"`
	Command             string      `json:"command"`
	Args                []string    `json:"args"`
	CWD                 string      `json:"cwd"`
	Status              string      `json:"status"`
	PID                 int         `json:"pid,omitempty"`
	PGID                int         `json:"pgid,omitempty"`
	Terminal            terminalDTO `json:"terminal"`
	CreatedAt           time.Time   `json:"created_at"`
	UpdatedAt           time.Time   `json:"updated_at"`
	ExitedAt            *time.Time  `json:"exited_at,omitempty"`
	ExitCode            *int        `json:"exit_code,omitempty"`
}

type harnessDTO struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	Installed   bool     `json:"installed"`
	Path        string   `json:"path,omitempty"`
	Version     string   `json:"version,omitempty"`
	DefaultMode string   `json:"default_mode"`
	Description string   `json:"description"`
}

type terminalDTO struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

type snapshotResponse struct {
	SessionID        string          `json:"session_id"`
	Rows             uint16          `json:"rows"`
	Cols             uint16          `json:"cols"`
	LatestSequence   uint64          `json:"latest_seq"`
	Timestamp        time.Time       `json:"ts"`
	HistoryTruncated bool            `json:"history_truncated"`
	Chunks           []snapshotChunk `json:"chunks"`
}

type snapshotChunk struct {
	Sequence uint64 `json:"seq"`
	Encoding string `json:"encoding"`
	Bytes    string `json:"bytes"`
}

type eventsResponse struct {
	Events []events.Event `json:"events"`
}

var requestCounter uint64

func NewRouter(opts Options) http.Handler {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	if opts.StaticFS == nil {
		opts.StaticFS = fs.FS(osDirFallback{})
	}
	if opts.Events == nil {
		opts.Events = events.NewBus()
	}
	if opts.Sessions == nil {
		opts.Sessions = session.NewManagerWithBus(opts.Events)
	}
	if opts.Auth == nil {
		opts.Auth = security.NewAuthenticator("")
	}
	if opts.Audit == nil {
		opts.Audit = storage.NewAuditLog(0)
	}
	if opts.Harnesses == nil {
		opts.Harnesses = harness.DiscoverInstalled(context.Background())
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, healthResponse{
			Status:  "ok",
			Service: "harnessd",
			Version: opts.Version,
		})
	})
	mux.HandleFunc("GET /api/v1/auth/status", opts.handleAuthStatus)
	mux.HandleFunc("POST /api/v1/auth/login", opts.handleAuthLogin)
	mux.HandleFunc("GET /api/v1/harnesses", opts.requireAuth(opts.handleListHarnesses))
	mux.HandleFunc("GET /api/v1/sessions", opts.requireAuth(opts.handleListSessions))
	mux.HandleFunc("POST /api/v1/sessions", opts.requireAuth(opts.handleCreateSession))
	mux.HandleFunc("GET /api/v1/sessions/{id}", opts.requireAuth(opts.handleGetSession))
	mux.HandleFunc("DELETE /api/v1/sessions/{id}", opts.requireAuth(opts.handleDeleteSession))
	mux.HandleFunc("POST /api/v1/sessions/{id}/input", opts.requireAuth(opts.handleSessionInput))
	mux.HandleFunc("POST /api/v1/sessions/{id}/prompt", opts.requireAuth(opts.handleSessionPrompt))
	mux.HandleFunc("POST /api/v1/sessions/{id}/resize", opts.requireAuth(opts.handleSessionResize))
	mux.HandleFunc("POST /api/v1/sessions/{id}/interrupt", opts.requireAuth(opts.handleSessionInterrupt))
	mux.HandleFunc("POST /api/v1/sessions/{id}/terminate", opts.requireAuth(opts.handleSessionTerminate))
	mux.HandleFunc("POST /api/v1/sessions/{id}/kill", opts.requireAuth(opts.handleSessionKill))
	mux.HandleFunc("POST /api/v1/sessions/{id}/cleanup", opts.requireAuth(opts.handleSessionCleanup))
	mux.HandleFunc("GET /api/v1/sessions/{id}/snapshot", opts.requireAuth(opts.handleSessionSnapshot))
	mux.HandleFunc("GET /api/v1/sessions/{id}/events", opts.requireAuth(opts.handleSessionEvents))
	mux.HandleFunc("POST /api/v1/sessions/{id}/actions/{action_id}", opts.requireAuth(opts.handleSessionAction))
	mux.HandleFunc("GET /api/v1/ws", opts.handleWebSocket)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	mux.Handle("/", http.FileServerFS(opts.StaticFS))

	return requestLogMiddleware(opts.Logger, mux)
}

func (opts Options) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	principal, err := opts.Auth.Authenticate(r)
	if err != nil {
		writeJSON(w, http.StatusOK, authStatusResponse{Authenticated: false})
		return
	}
	writeJSON(w, http.StatusOK, authStatusResponse{Authenticated: true, CSRFToken: principal.CSRFToken})
}

func (opts Options) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if !security.SameOrigin(r) {
		writeError(w, http.StatusForbidden, "unexpected origin")
		return
	}
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	principal, ok := opts.Auth.Login(w, req.Token)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid token")
		return
	}
	writeJSON(w, http.StatusOK, authStatusResponse{Authenticated: true, CSRFToken: principal.CSRFToken})
}

func (opts Options) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isUnsafeMethod(r.Method) && !security.SameOrigin(r) {
			writeError(w, http.StatusForbidden, "unexpected origin")
			return
		}
		if _, err := opts.Auth.Authorize(r); err != nil {
			status := http.StatusUnauthorized
			if err == security.ErrForbidden {
				status = http.StatusForbidden
			}
			writeError(w, status, err.Error())
			return
		}
		next(w, r)
	}
}

func (opts Options) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions := opts.Sessions.List()
	out := make([]sessionDTO, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, sessionToDTO(sess.Info()))
	}
	writeJSON(w, http.StatusOK, sessionsResponse{Sessions: out})
}

func (opts Options) handleListHarnesses(w http.ResponseWriter, r *http.Request) {
	out := make([]harnessDTO, 0, len(opts.Harnesses))
	for _, detected := range opts.Harnesses {
		out = append(out, harnessToDTO(detected))
	}
	writeJSON(w, http.StatusOK, harnessesResponse{Harnesses: out})
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

func (opts Options) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Command == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}
	if err := validateTerminalSize(req.Terminal.Rows, req.Terminal.Cols, true); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	workDir := req.CWD
	if workDir == "" {
		workDir = req.WorkDir
	}
	sess, err := opts.Sessions.Create(r.Context(), session.CreateOptions{
		Name:        req.Name,
		HarnessType: req.HarnessType,
		Command:     req.Command,
		Args:        req.Args,
		WorkDir:     workDir,
		Env:         envMapToList(req.Env),
		Rows:        req.Terminal.Rows,
		Cols:        req.Terminal.Cols,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	opts.recordAudit("session.create", sess.ID, map[string]any{
		"command":    sess.Command,
		"cwd":        sess.WorkDir,
		"args_count": len(sess.Args),
	})
	writeJSON(w, http.StatusCreated, sessionResponse{Session: sessionToDTO(sess.Info())})
}

func (opts Options) handleGetSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := opts.Sessions.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{Session: sessionToDTO(sess.Info())})
}

func (opts Options) handleDeleteSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := opts.Sessions.Get(id); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := opts.Sessions.Terminate(ctx, id); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (opts Options) handleSessionInput(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := opts.Sessions.Get(id); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	var req inputRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	data, err := inputBytes(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(data) > 64*1024 {
		writeError(w, http.StatusRequestEntityTooLarge, "input exceeds 65536 byte limit")
		return
	}
	if err := opts.Sessions.Write(id, data); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	opts.recordAudit("session.input", id, map[string]any{"bytes": len(data)})
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "bytes": len(data)})
}

func (opts Options) handleSessionPrompt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := opts.Sessions.Get(id); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	var req promptRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "prompt text is required")
		return
	}
	if len([]byte(req.Text)) > 64*1024 {
		writeError(w, http.StatusRequestEntityTooLarge, "prompt exceeds 65536 byte limit")
		return
	}
	if err := opts.Sessions.SubmitPrompt(id, req.Text); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	opts.recordAudit("session.prompt", id, map[string]any{"bytes": len([]byte(req.Text))})
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true})
}

func (opts Options) handleSessionResize(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := opts.Sessions.Get(id); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	var req resizeRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateTerminalSize(req.Rows, req.Cols, false); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := opts.Sessions.Resize(id, req.Rows, req.Cols); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	opts.recordAudit("session.resize", id, map[string]any{"rows": req.Rows, "cols": req.Cols})
	sess, _ := opts.Sessions.Get(id)
	writeJSON(w, http.StatusOK, sessionResponse{Session: sessionToDTO(sess.Info())})
}

func (opts Options) handleSessionInterrupt(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := opts.Sessions.Get(id); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	var req interruptRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.Strategy != "" && req.Strategy != "ctrl_c" {
		writeError(w, http.StatusBadRequest, "unsupported interrupt strategy")
		return
	}
	if err := opts.Sessions.Interrupt(id); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	opts.recordAudit("session.interrupt", id, map[string]any{"strategy": "ctrl_c"})
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true, "strategy": "ctrl_c"})
}

func (opts Options) handleSessionTerminate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := opts.Sessions.Get(id); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	var req terminateRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	grace := 5 * time.Second
	if req.GraceMS > 0 {
		if req.GraceMS > 30000 {
			writeError(w, http.StatusBadRequest, "grace_ms must be <= 30000")
			return
		}
		grace = time.Duration(req.GraceMS) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(r.Context(), grace)
	defer cancel()
	if err := opts.Sessions.Terminate(ctx, id); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	opts.recordAudit("session.terminate", id, map[string]any{"grace_ms": int(grace / time.Millisecond)})
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true})
}

func (opts Options) handleSessionKill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := opts.Sessions.Get(id); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	var req killRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Confirmation != "KILL" {
		writeError(w, http.StatusBadRequest, "confirmation must be KILL")
		return
	}
	if err := opts.Sessions.Kill(id); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	opts.recordAudit("session.kill", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true})
}

func (opts Options) handleSessionCleanup(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := opts.Sessions.Cleanup(id); err != nil {
		status := http.StatusConflict
		if _, ok := opts.Sessions.Get(id); !ok {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	opts.recordAudit("session.cleanup", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"accepted": true})
}

func (opts Options) handleSessionSnapshot(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess, ok := opts.Sessions.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	info := sess.Info()
	events := opts.Events.History(id, 0, 1)
	var latest uint64
	if len(events) > 0 {
		latest = events[len(events)-1].Sequence
	}
	chunks := []snapshotChunk{}
	if data := sess.Snapshot(); len(data) > 0 {
		chunks = append(chunks, snapshotChunk{
			Sequence: latest,
			Encoding: "base64",
			Bytes:    base64.StdEncoding.EncodeToString(data),
		})
	}
	writeJSON(w, http.StatusOK, snapshotResponse{
		SessionID:        id,
		Rows:             info.Terminal.Rows,
		Cols:             info.Terminal.Cols,
		LatestSequence:   latest,
		Timestamp:        time.Now(),
		HistoryTruncated: false,
		Chunks:           chunks,
	})
}

func (opts Options) handleSessionEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := opts.Sessions.Get(id); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	after, err := parseUintQuery(r, "after_seq")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = parsed
	}
	writeJSON(w, http.StatusOK, eventsResponse{Events: opts.Events.History(id, after, limit)})
}

func (opts Options) handleSessionAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := opts.Sessions.Get(id); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	actionID := r.PathValue("action_id")
	var req actionRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.EventID == "" {
		writeError(w, http.StatusBadRequest, "event_id is required")
		return
	}
	opts.recordAudit("semantic.action", id, map[string]any{"event_id": req.EventID, "action_id": actionID})
	event, ok := findEvent(opts.Events.History(id, 0, 1024), req.EventID)
	if !ok {
		writeError(w, http.StatusConflict, "stale or unknown event")
		return
	}
	if !eventHasAction(event, actionID, req.ActionVersion) {
		writeError(w, http.StatusConflict, "stale or unknown action")
		return
	}
	if err := opts.Sessions.ExecuteAction(id, req.EventID, actionID); err != nil {
		switch {
		case errors.Is(err, session.ErrStaleSemanticAction):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, session.ErrUnsupportedAction):
			writeJSON(w, http.StatusNotImplemented, actionResultResponse{Result: actionResult{
				Status:   "unsupported",
				EventID:  req.EventID,
				ActionID: actionID,
			}})
		default:
			writeError(w, http.StatusConflict, err.Error())
		}
		return
	}
	opts.Events.Publish(r.Context(), events.Event{
		Type:      events.TypeActionCompleted,
		SessionID: id,
		Data: map[string]any{
			"event_id":  req.EventID,
			"action_id": actionID,
		},
	})
	writeJSON(w, http.StatusOK, actionResultResponse{Result: actionResult{
		Status:   "completed",
		EventID:  req.EventID,
		ActionID: actionID,
	}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (opts Options) recordAudit(typ, sessionID string, metadata map[string]any) {
	if opts.Audit == nil {
		return
	}
	opts.Audit.Record(storage.AuditRecord{
		Type:      typ,
		SessionID: sessionID,
		Actor:     "local",
		Metadata:  metadata,
	})
}

func findEvent(eventList []events.Event, eventID string) (events.Event, bool) {
	for _, event := range eventList {
		if event.ID == eventID {
			return event, true
		}
	}
	return events.Event{}, false
}

func eventHasAction(event events.Event, actionID string, version int) bool {
	data, err := json.Marshal(event.Data)
	if err != nil {
		return false
	}
	var payload struct {
		Actions []struct {
			ID      string `json:"id"`
			Version int    `json:"version"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return false
	}
	for _, action := range payload.Actions {
		if action.ID != actionID {
			continue
		}
		if action.Version != 0 && version != 0 && action.Version != version {
			return false
		}
		return true
	}
	return false
}

func readJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func sessionToDTO(info session.Info) sessionDTO {
	updatedAt := info.StartedAt
	if info.ExitedAt != nil {
		updatedAt = *info.ExitedAt
	}
	return sessionDTO{
		ID:                  info.ID,
		Name:                info.Name,
		HarnessType:         info.HarnessType,
		AdapterID:           info.AdapterID,
		AdapterName:         info.AdapterName,
		AdapterCapabilities: capabilitiesToStrings(info.Capabilities),
		Command:             info.Command,
		Args:                info.Args,
		CWD:                 info.WorkDir,
		Status:              string(info.Status),
		PID:                 info.PID,
		PGID:                info.PGID,
		Terminal: terminalDTO{
			Rows: info.Terminal.Rows,
			Cols: info.Terminal.Cols,
		},
		CreatedAt: info.StartedAt,
		UpdatedAt: updatedAt,
		ExitedAt:  info.ExitedAt,
		ExitCode:  info.ExitCode,
	}
}

func capabilitiesToStrings(capabilities []harness.Capability) []string {
	out := make([]string, len(capabilities))
	for i, capability := range capabilities {
		out[i] = string(capability)
	}
	return out
}

func harnessToDTO(detected harness.Detected) harnessDTO {
	args := make([]string, len(detected.Args))
	copy(args, detected.Args)
	return harnessDTO{
		ID:          detected.ID,
		Name:        detected.Name,
		Command:     detected.Command,
		Args:        args,
		Installed:   detected.Installed,
		Path:        detected.Path,
		Version:     detected.Version,
		DefaultMode: detected.DefaultMode,
		Description: detected.Description,
	}
}

func envMapToList(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

func inputBytes(req inputRequest) ([]byte, error) {
	mode := req.Mode
	if mode == "" {
		mode = "raw"
	}
	switch mode {
	case "raw":
		if req.Encoding != "" && req.Encoding != "base64" {
			return nil, fmt.Errorf("unsupported input encoding")
		}
		data, err := base64.StdEncoding.DecodeString(req.Data)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 input: %w", err)
		}
		return data, nil
	case "text":
		if req.Text != "" {
			return []byte(req.Text), nil
		}
		return []byte(req.Data), nil
	case "key":
		data, ok := specialKeyBytes(req.Key)
		if !ok {
			return nil, fmt.Errorf("unsupported special key")
		}
		return data, nil
	default:
		return nil, fmt.Errorf("unsupported input mode")
	}
}

func specialKeyBytes(key string) ([]byte, bool) {
	switch key {
	case "enter", "Enter":
		return []byte("\r"), true
	case "escape", "Escape", "esc", "Esc":
		return []byte{0x1b}, true
	case "tab", "Tab":
		return []byte("\t"), true
	case "backspace", "Backspace":
		return []byte{0x7f}, true
	case "arrow_up", "ArrowUp":
		return []byte("\x1b[A"), true
	case "arrow_down", "ArrowDown":
		return []byte("\x1b[B"), true
	case "arrow_right", "ArrowRight":
		return []byte("\x1b[C"), true
	case "arrow_left", "ArrowLeft":
		return []byte("\x1b[D"), true
	case "ctrl_c", "CtrlC", "ControlC":
		return []byte{0x03}, true
	default:
		return nil, false
	}
}

func validateTerminalSize(rows, cols uint16, allowZero bool) error {
	if allowZero && rows == 0 && cols == 0 {
		return nil
	}
	if rows < 1 || rows > 500 {
		return fmt.Errorf("rows must be between 1 and 500")
	}
	if cols < 2 || cols > 1000 {
		return fmt.Errorf("cols must be between 2 and 1000")
	}
	return nil
}

func parseUintQuery(r *http.Request, key string) (uint64, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an unsigned integer", key)
	}
	return parsed, nil
}

func requestLogMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = nextRequestID()
		}
		started := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("http request",
			logging.RequestID(requestID),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Duration("duration", time.Since(started)),
		)
	})
}

func nextRequestID() string {
	return fmt.Sprintf("req-%d", atomic.AddUint64(&requestCounter, 1))
}

type osDirFallback struct{}

func (osDirFallback) Open(name string) (fs.File, error) {
	return nil, fs.ErrNotExist
}
