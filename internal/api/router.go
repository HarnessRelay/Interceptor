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

	"github.com/harnessrelay/interceptor/internal/config"
	"github.com/harnessrelay/interceptor/internal/events"
	"github.com/harnessrelay/interceptor/internal/harness"
	"github.com/harnessrelay/interceptor/internal/logging"
	"github.com/harnessrelay/interceptor/internal/security"
	"github.com/harnessrelay/interceptor/internal/session"
	"github.com/harnessrelay/interceptor/internal/storage"
	"github.com/harnessrelay/interceptor/internal/tunnel"
)

type Options struct {
	Logger     *slog.Logger
	Version    string
	StaticFS   fs.FS
	Sessions   *session.Manager
	Events     *events.Bus
	Auth       *security.Authenticator
	Audit      *storage.AuditLog
	DB         *storage.DB
	Harnesses  []harness.Detected
	AllowedIPs []config.AllowedIP
	Identity   *security.DeviceIdentity
	Pairing    *security.PairingManager
	Devices    *security.PairedDeviceStore
	Tunnel     *tunnel.Manager

	// IPFilter is the runtime allow/ban list; when nil no network gating
	// beyond class policy applies.
	IPFilter *security.IPFilter
	// RemoteSettings backs the remote-access toggle.
	RemoteSettings *security.RemoteSettingsStore
	// KnownClients stores user-assigned network client names.
	KnownClients *security.KnownClientStore

	// TunnelDownloadAPI overrides the cloudflared releases endpoint (tests).
	TunnelDownloadAPI string

	// loginLimiter throttles token login attempts per client IP; created in
	// NewRouter so every router instance gets its own limiter.
	loginLimiter *rateLimiter
	// pairingLimiter throttles unauthenticated pairing submissions.
	pairingLimiter *rateLimiter
	// clients tracks observed network clients for the settings view.
	clients *clientTracker
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
	Authenticated     bool   `json:"authenticated"`
	CSRFToken         string `json:"csrf_token,omitempty"`
	ClientClass       string `json:"client_class"`
	TokenLoginAllowed bool   `json:"token_login_allowed"`
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
	Name          string            `json:"name"`
	HarnessType   string            `json:"harness_type"`
	Command       string            `json:"command"`
	Args          []string          `json:"args"`
	CWD           string            `json:"cwd"`
	WorkDir       string            `json:"work_dir"`
	Env           map[string]string `json:"env"`
	Terminal      terminalDTO       `json:"terminal"`
	Origin        string            `json:"origin"`
	OriginBackend string            `json:"origin_backend"`
	ShimName      string            `json:"shim_name"`
	RealBinary    string            `json:"real_binary"`
	Attachable    bool              `json:"attachable"`
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

type commandRequest struct {
	Arguments string `json:"arguments"`
}

type commandsResponse struct {
	Supported bool                        `json:"supported"`
	Commands  []harness.CommandDescriptor `json:"commands"`
	Fallback  string                      `json:"fallback,omitempty"`
}

type commandResultResponse struct {
	Accepted bool                      `json:"accepted"`
	Command  harness.CommandDescriptor `json:"command"`
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
	Origin              string      `json:"origin,omitempty"`
	OriginBackend       string      `json:"origin_backend,omitempty"`
	ShimName            string      `json:"shim_name,omitempty"`
	RealBinary          string      `json:"real_binary,omitempty"`
	Attachable          bool        `json:"attachable"`
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

type identityResponse struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	PublicKey  string `json:"public_key"`
}

type pairingRequestPayload struct {
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	Platform   string `json:"platform"`
	PublicKey  string `json:"public_key"`
}

type pairingStatusResponse struct {
	Status string `json:"status"`
}

type pairingRequestsResponse struct {
	Requests []security.PairingRequest `json:"requests"`
}

type pairingAcceptRejectRequest struct {
	DeviceID string `json:"device_id"`
}

type pairedDevicesResponse struct {
	Devices []security.PairedDevice `json:"devices"`
}

type tunnelResponse struct {
	Status string `json:"status"`
	URL    string `json:"url,omitempty"`
	Error  string `json:"error,omitempty"`
}

type tunnelAvailableResponse struct {
	Available bool   `json:"available"`
	Binary    string `json:"binary"`
}

type tunnelConfigRequest struct {
	Mode     string `json:"mode"`
	Token    string `json:"token"`
	Hostname string `json:"hostname"`
}

type tunnelConfigResponse struct {
	Mode     string `json:"mode"`
	Hostname string `json:"hostname,omitempty"`
	TokenSet bool   `json:"token_set"`
}

type tunnelBinaryResponse struct {
	Path        string `json:"path,omitempty"`
	Source      string `json:"source,omitempty"`
	Version     string `json:"version,omitempty"`
	ManagedPath string `json:"managed_path"`
}

type tunnelDownloadResponse struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

type tunnelLogsResponse struct {
	Lines []string `json:"lines"`
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
	opts.loginLimiter = newRateLimiter(loginRateWindow, loginRateMax)
	opts.pairingLimiter = newRateLimiter(pairingRateWindow, pairingRateMax)
	opts.clients = newClientTracker()

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
	mux.HandleFunc("GET /api/v1/archive/sessions", opts.requireAuth(opts.handleListArchive))
	mux.HandleFunc("POST /api/v1/sessions", opts.requireAuth(opts.handleCreateSession))
	mux.HandleFunc("GET /api/v1/sessions/{id}", opts.requireAuth(opts.handleGetSession))
	mux.HandleFunc("DELETE /api/v1/sessions/{id}", opts.requireAuth(opts.handleDeleteSession))
	mux.HandleFunc("POST /api/v1/sessions/{id}/input", opts.requireAuth(opts.handleSessionInput))
	mux.HandleFunc("POST /api/v1/sessions/{id}/prompt", opts.requireAuth(opts.handleSessionPrompt))
	mux.HandleFunc("GET /api/v1/sessions/{id}/commands", opts.requireAuth(opts.handleSessionCommands))
	mux.HandleFunc("POST /api/v1/sessions/{id}/commands/{command_id}", opts.requireAuth(opts.handleSessionCommand))
	mux.HandleFunc("POST /api/v1/sessions/{id}/resize", opts.requireAuth(opts.handleSessionResize))
	mux.HandleFunc("POST /api/v1/sessions/{id}/interrupt", opts.requireAuth(opts.handleSessionInterrupt))
	mux.HandleFunc("POST /api/v1/sessions/{id}/terminate", opts.requireAuth(opts.handleSessionTerminate))
	mux.HandleFunc("POST /api/v1/sessions/{id}/kill", opts.requireAuth(opts.handleSessionKill))
	mux.HandleFunc("POST /api/v1/sessions/{id}/cleanup", opts.requireAuth(opts.handleSessionCleanup))
	mux.HandleFunc("GET /api/v1/sessions/{id}/snapshot", opts.requireAuth(opts.handleSessionSnapshot))
	mux.HandleFunc("GET /api/v1/sessions/{id}/events", opts.requireAuth(opts.handleSessionEvents))
	mux.HandleFunc("POST /api/v1/sessions/{id}/actions/{action_id}", opts.requireAuth(opts.handleSessionAction))
	mux.HandleFunc("GET /api/v1/ws", opts.handleWebSocket)
	mux.HandleFunc("GET /api/v1/identity", opts.handleIdentity)
	mux.HandleFunc("POST /api/v1/pairing/request", opts.handlePairingRequest)
	mux.HandleFunc("GET /api/v1/pairing/status", opts.handlePairingStatus)
	mux.HandleFunc("GET /api/v1/pairing/requests", opts.requireAuth(opts.handlePairingRequests))
	mux.HandleFunc("POST /api/v1/pairing/accept", opts.requireAuth(opts.handlePairingAccept))
	mux.HandleFunc("POST /api/v1/pairing/reject", opts.requireAuth(opts.handlePairingReject))
	mux.HandleFunc("GET /api/v1/pairing/devices", opts.requireAuth(opts.handlePairingDevices))
	mux.HandleFunc("DELETE /api/v1/pairing/devices/{id}", opts.requireAuth(opts.handlePairingDeviceRemove))
	mux.HandleFunc("GET /api/v1/tunnel/available", opts.requireAuth(opts.handleTunnelAvailable))
	mux.HandleFunc("GET /api/v1/tunnel", opts.requireAuth(opts.handleTunnelStatus))
	mux.HandleFunc("POST /api/v1/tunnel/start", opts.requireAuth(opts.handleTunnelStart))
	mux.HandleFunc("POST /api/v1/tunnel/stop", opts.requireAuth(opts.handleTunnelStop))
	mux.HandleFunc("GET /api/v1/tunnel/config", opts.requireAuth(opts.handleTunnelConfigGet))
	mux.HandleFunc("PUT /api/v1/tunnel/config", opts.requireAuth(opts.handleTunnelConfigPut))
	mux.HandleFunc("GET /api/v1/tunnel/binary", opts.requireAuth(opts.handleTunnelBinary))
	mux.HandleFunc("POST /api/v1/tunnel/download", opts.requireAuth(opts.handleTunnelDownload))
	mux.HandleFunc("GET /api/v1/tunnel/logs", opts.requireAuth(opts.handleTunnelLogs))
	mux.HandleFunc("POST /api/v1/auth/device-session", opts.requireAuth(opts.handleDeviceSession))
	mux.HandleFunc("POST /api/v1/pairing/web", opts.handlePairingWebSubmit)
	mux.HandleFunc("GET /api/v1/pairing/web/{id}", opts.handlePairingWebPoll)
	mux.HandleFunc("PUT /api/v1/pairing/devices/{id}/name", opts.requireAuth(opts.handlePairingDeviceRename))
	mux.HandleFunc("GET /api/v1/network/settings", opts.requireAuth(opts.handleNetworkSettingsGet))
	mux.HandleFunc("PUT /api/v1/network/settings", opts.requireAuth(opts.handleNetworkSettingsPut))
	mux.HandleFunc("POST /api/v1/network/allow", opts.requireAuth(opts.handleNetworkAllowAdd))
	mux.HandleFunc("DELETE /api/v1/network/allow", opts.requireAuth(opts.handleNetworkAllowRemove))
	mux.HandleFunc("POST /api/v1/network/ban", opts.requireAuth(opts.handleNetworkBanAdd))
	mux.HandleFunc("DELETE /api/v1/network/ban", opts.requireAuth(opts.handleNetworkBanRemove))
	mux.HandleFunc("GET /api/v1/network/clients", opts.requireAuth(opts.handleNetworkClients))
	mux.HandleFunc("PUT /api/v1/network/clients/{key}/name", opts.requireAuth(opts.handleNetworkClientRename))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	mux.Handle("/", http.FileServerFS(opts.StaticFS))

	handler := requestLogMiddleware(opts.Logger, mux)
	handler = opts.accessGateMiddleware(handler)
	return handler
}

func (opts Options) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	info := security.ClassifyClient(r)
	// GET request, so AuthorizeRequest applies class policy without CSRF.
	principal, err := opts.Auth.AuthorizeRequest(r)
	if err != nil {
		writeJSON(w, http.StatusOK, authStatusResponse{
			Authenticated:     false,
			ClientClass:       string(info.Class),
			TokenLoginAllowed: info.Class == security.ClientClassHost,
		})
		return
	}
	writeJSON(w, http.StatusOK, authStatusResponse{
		Authenticated:     true,
		CSRFToken:         principal.CSRFToken,
		ClientClass:       string(info.Class),
		TokenLoginAllowed: info.Class == security.ClientClassHost,
	})
}

func (opts Options) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if !security.SameOrigin(r) {
		writeError(w, http.StatusForbidden, "unexpected origin")
		return
	}
	if !opts.loginLimiter.allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts; try again later")
		return
	}
	if info := security.ClassifyClient(r); info.Class != security.ClientClassHost {
		writeError(w, http.StatusForbidden, "token login is only available on the host machine; request device access instead")
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
	writeJSON(w, http.StatusOK, authStatusResponse{
		Authenticated:     true,
		CSRFToken:         principal.CSRFToken,
		ClientClass:       string(security.ClientClassHost),
		TokenLoginAllowed: true,
	})
}

// handleDeviceSession exchanges a device credential (web token or Ed25519
// signature) for a device cookie session so browser clients get uniform
// cookie-based auth, including WebSockets which cannot send headers.
func (opts Options) handleDeviceSession(w http.ResponseWriter, r *http.Request) {
	principal, err := opts.Auth.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, err.Error())
		return
	}
	if principal.Master {
		writeError(w, http.StatusForbidden, "device sessions require a paired device credential")
		return
	}
	if principal.DeviceID == "" {
		writeError(w, http.StatusForbidden, "no device credential on request")
		return
	}
	session := opts.Auth.LoginDevice(w, principal)
	writeJSON(w, http.StatusOK, authStatusResponse{
		Authenticated: true,
		CSRFToken:     session.CSRFToken,
	})
}

func (opts Options) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isUnsafeMethod(r.Method) && !security.SameOrigin(r) {
			writeError(w, http.StatusForbidden, "unexpected origin")
			return
		}
		if _, err := opts.Auth.AuthorizeRequest(r); err != nil {
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

// accessGateMiddleware enforces network-level access rules before routing:
// the banlist for LAN/tunnel real IPs, the remote-access toggle for non-host
// clients (health stays reachable), and the allowlist for direct LAN
// clients. Host clients are never filtered. It also records every client in
// the tracker for the settings Network tab.
func (opts Options) accessGateMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := security.ClassifyClient(r)
		opts.clients.record(info)

		if info.Class == security.ClientClassHost {
			next.ServeHTTP(w, r)
			return
		}

		if r.URL.Path == "/api/v1/health" {
			next.ServeHTTP(w, r)
			return
		}

		if opts.IPFilter != nil && opts.IPFilter.Banned(info.RemoteIP) {
			opts.Logger.Warn("client IP banned",
				logging.RequestID(r.Header.Get("X-Request-ID")),
				slog.String("client_ip", info.Key()),
				slog.String("path", r.URL.Path),
			)
			writeError(w, http.StatusForbidden, "client IP banned")
			return
		}

		if opts.RemoteSettings != nil && !opts.RemoteSettings.Get().RemoteAccessEnabled {
			writeError(w, http.StatusForbidden, "remote access is disabled on this daemon")
			return
		}

		if info.Class == security.ClientClassLAN && opts.IPFilter != nil &&
			!opts.IPFilter.AllowedByAllowlist(info.RemoteIP) {
			opts.Logger.Warn("client IP not allowed",
				logging.RequestID(r.Header.Get("X-Request-ID")),
				slog.String("client_ip", info.Key()),
				slog.String("path", r.URL.Path),
			)
			writeError(w, http.StatusForbidden, "client IP not allowed")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (opts Options) handleListSessions(w http.ResponseWriter, r *http.Request) {
	sessions := opts.Sessions.List()
	out := make([]sessionDTO, 0, len(sessions))
	for _, sess := range sessions {
		out = append(out, sessionToDTO(sess.Info()))
	}
	// Append up to 5 most recent archived sessions
	if opts.DB != nil {
		archived, err := opts.DB.ListArchivedSessions(5)
		if err == nil {
			for _, info := range archived {
				out = append(out, sessionInfoToDTO(info))
			}
		}
	}
	writeJSON(w, http.StatusOK, sessionsResponse{Sessions: out})
}

func (opts Options) handleListArchive(w http.ResponseWriter, r *http.Request) {
	if opts.DB == nil {
		writeJSON(w, http.StatusOK, sessionsResponse{Sessions: []sessionDTO{}})
		return
	}
	archived, err := opts.DB.ListArchivedSessions(0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load archive")
		return
	}
	out := make([]sessionDTO, 0, len(archived))
	for _, info := range archived {
		out = append(out, sessionInfoToDTO(info))
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
	if req.Origin != "" && req.Origin != "shim" {
		writeError(w, http.StatusBadRequest, "origin must be shim when provided")
		return
	}
	if req.Origin == "shim" {
		if req.OriginBackend != "pty" && req.OriginBackend != "tmux" {
			writeError(w, http.StatusBadRequest, "shim origin_backend must be pty or tmux")
			return
		}
		if req.ShimName == "" || req.RealBinary == "" {
			writeError(w, http.StatusBadRequest, "shim_name and real_binary are required for shim origin")
			return
		}
		if req.RealBinary != req.Command {
			writeError(w, http.StatusBadRequest, "real_binary must match command for shim origin")
			return
		}
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
		Name:          req.Name,
		HarnessType:   req.HarnessType,
		Command:       req.Command,
		Args:          req.Args,
		WorkDir:       workDir,
		Env:           envMapToList(req.Env),
		Rows:          req.Terminal.Rows,
		Cols:          req.Terminal.Cols,
		Origin:        req.Origin,
		OriginBackend: req.OriginBackend,
		ShimName:      req.ShimName,
		RealBinary:    req.RealBinary,
		Attachable:    req.Attachable,
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
	if ok {
		writeJSON(w, http.StatusOK, sessionResponse{Session: sessionToDTO(sess.Info())})
		return
	}
	if opts.DB != nil {
		info, err := opts.DB.GetArchivedSession(r.PathValue("id"))
		if err == nil && info != nil {
			writeJSON(w, http.StatusOK, sessionResponse{Session: sessionInfoToDTO(*info)})
			return
		}
	}
	writeError(w, http.StatusNotFound, "session not found")
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

func (opts Options) handleSessionCommands(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := opts.Sessions.Get(id); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	commands, err := opts.Sessions.CommandCatalog(id)
	if err != nil {
		if errors.Is(err, session.ErrCommandUnsupported) {
			writeJSON(w, http.StatusOK, commandsResponse{Supported: false, Commands: []harness.CommandDescriptor{}, Fallback: "terminal"})
			return
		}
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if commands == nil {
		commands = []harness.CommandDescriptor{}
	}
	writeJSON(w, http.StatusOK, commandsResponse{Supported: len(commands) > 0, Commands: commands, Fallback: "terminal"})
}

func (opts Options) handleSessionCommand(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, ok := opts.Sessions.Get(id); !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	var req commandRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := readJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if len([]byte(req.Arguments)) > 16*1024 {
		writeError(w, http.StatusRequestEntityTooLarge, "command arguments exceed 16384 byte limit")
		return
	}
	command, err := opts.Sessions.SubmitCommand(id, r.PathValue("command_id"), req.Arguments)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, session.ErrCommandUnknown) {
			status = http.StatusNotFound
		}
		writeError(w, status, err.Error())
		return
	}
	opts.recordAudit("session.command", id, map[string]any{
		"command_id":     command.ID,
		"argument_bytes": len([]byte(req.Arguments)),
		"interaction":    command.Interaction,
	})
	writeJSON(w, http.StatusOK, commandResultResponse{Accepted: true, Command: command})
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
	if _, ok := opts.Sessions.Get(id); ok {
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
		return
	}
	if opts.DB != nil {
		info, err := opts.DB.GetArchivedSession(id)
		if err == nil && info != nil {
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
			evts, err := opts.DB.GetArchivedEvents(id, after, limit)
			if err == nil {
				writeJSON(w, http.StatusOK, eventsResponse{Events: evts})
				return
			}
		}
	}
	writeError(w, http.StatusNotFound, "session not found")
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

func (opts Options) handleIdentity(w http.ResponseWriter, r *http.Request) {
	if opts.Identity == nil {
		writeError(w, http.StatusServiceUnavailable, "device identity not configured")
		return
	}
	writeJSON(w, http.StatusOK, identityResponse{
		DeviceID:   opts.Identity.ID,
		DeviceName: opts.Identity.Name,
		PublicKey:  base64.StdEncoding.EncodeToString(opts.Identity.PublicKey),
	})
}

func (opts Options) handlePairingRequest(w http.ResponseWriter, r *http.Request) {
	if opts.Pairing == nil {
		writeError(w, http.StatusServiceUnavailable, "pairing not enabled")
		return
	}
	if !opts.pairingLimiter.allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "too many pairing attempts; try again later")
		return
	}
	var req pairingRequestPayload
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.DeviceID == "" || req.DeviceName == "" || req.PublicKey == "" {
		writeError(w, http.StatusBadRequest, "device_id, device_name, and public_key are required")
		return
	}
	msg, ok := opts.Pairing.SubmitRequest(security.PairingRequest{
		DeviceID:   req.DeviceID,
		DeviceName: req.DeviceName,
		Platform:   req.Platform,
		PublicKey:  req.PublicKey,
	})
	if !ok {
		writeError(w, http.StatusTooManyRequests, msg)
		return
	}
	// Echo the verification code so the requesting device can display it
	// next to the same code shown in the daemon approval dialog.
	writeJSON(w, http.StatusOK, pairingSubmitResponse{
		Status: "pending",
		Code:   opts.Pairing.PendingCode(req.DeviceID),
	})
}

type pairingSubmitResponse struct {
	Status string `json:"status"`
	Code   string `json:"code,omitempty"`
}

type pairingWebRequest struct {
	DeviceName string `json:"device_name"`
}

type pairingWebResponse struct {
	RequestID string `json:"request_id"`
	Code      string `json:"code"`
	Secret    string `json:"secret"`
}

type pairingWebPollResponse struct {
	Status      string `json:"status"`
	DeviceToken string `json:"device_token,omitempty"`
}

// handlePairingWebSubmit starts a web-device pairing flow from a remote
// browser. Unauthenticated by design; the 6-digit code and the approval
// dialog provide the trust decision, and the poll secret gates token claim.
func (opts Options) handlePairingWebSubmit(w http.ResponseWriter, r *http.Request) {
	if opts.Pairing == nil {
		writeError(w, http.StatusServiceUnavailable, "pairing not enabled")
		return
	}
	if !security.SameOrigin(r) {
		writeError(w, http.StatusForbidden, "unexpected origin")
		return
	}
	if !opts.pairingLimiter.allow(clientIP(r)) {
		writeError(w, http.StatusTooManyRequests, "too many pairing attempts; try again later")
		return
	}
	var req pairingWebRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.DeviceName == "" {
		writeError(w, http.StatusBadRequest, "device_name is required")
		return
	}
	requestID, code, secret, errMsg := opts.Pairing.SubmitWebRequest(req.DeviceName)
	if errMsg != "" {
		writeError(w, http.StatusTooManyRequests, errMsg)
		return
	}
	writeJSON(w, http.StatusOK, pairingWebResponse{RequestID: requestID, Code: code, Secret: secret})
}

// handlePairingWebPoll lets the requesting browser poll its pairing status.
// The secret from submission is required; the device token is returned
// exactly once, on the first poll after acceptance.
func (opts Options) handlePairingWebPoll(w http.ResponseWriter, r *http.Request) {
	if opts.Pairing == nil {
		writeError(w, http.StatusServiceUnavailable, "pairing not enabled")
		return
	}
	requestID := r.PathValue("id")
	if requestID == "" {
		writeError(w, http.StatusBadRequest, "request id is required")
		return
	}
	status, token := opts.Pairing.PollWebRequest(requestID, r.Header.Get("X-Pairing-Secret"))
	if status == security.PairingStatusUnknown {
		writeError(w, http.StatusNotFound, "unknown pairing request")
		return
	}
	writeJSON(w, http.StatusOK, pairingWebPollResponse{Status: string(status), DeviceToken: token})
}

func (opts Options) handlePairingStatus(w http.ResponseWriter, r *http.Request) {
	if opts.Pairing == nil {
		writeError(w, http.StatusServiceUnavailable, "pairing not enabled")
		return
	}
	deviceID := r.URL.Query().Get("device_id")
	if deviceID == "" {
		writeError(w, http.StatusBadRequest, "device_id is required")
		return
	}
	status := opts.Pairing.GetStatus(deviceID)
	writeJSON(w, http.StatusOK, pairingStatusResponse{Status: string(status)})
}

func (opts Options) handlePairingRequests(w http.ResponseWriter, r *http.Request) {
	if opts.Pairing == nil {
		writeJSON(w, http.StatusOK, pairingRequestsResponse{Requests: []security.PairingRequest{}})
		return
	}
	writeJSON(w, http.StatusOK, pairingRequestsResponse{Requests: opts.Pairing.PendingRequests()})
}

func (opts Options) handlePairingAccept(w http.ResponseWriter, r *http.Request) {
	if opts.Pairing == nil {
		writeError(w, http.StatusServiceUnavailable, "pairing not enabled")
		return
	}
	var req pairingAcceptRejectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "device_id is required")
		return
	}
	if !opts.Pairing.Accept(req.DeviceID) {
		writeError(w, http.StatusNotFound, "no pending request for this device")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "accepted"})
}

func (opts Options) handlePairingReject(w http.ResponseWriter, r *http.Request) {
	if opts.Pairing == nil {
		writeError(w, http.StatusServiceUnavailable, "pairing not enabled")
		return
	}
	var req pairingAcceptRejectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "device_id is required")
		return
	}
	if !opts.Pairing.Reject(req.DeviceID) {
		writeError(w, http.StatusNotFound, "no pending request for this device")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "rejected"})
}

func (opts Options) handlePairingDevices(w http.ResponseWriter, r *http.Request) {
	if opts.Devices == nil {
		writeJSON(w, http.StatusOK, pairedDevicesResponse{Devices: []security.PairedDevice{}})
		return
	}
	writeJSON(w, http.StatusOK, pairedDevicesResponse{Devices: opts.Devices.List()})
}

func (opts Options) handlePairingDeviceRemove(w http.ResponseWriter, r *http.Request) {
	if opts.Devices == nil {
		writeError(w, http.StatusServiceUnavailable, "device management not available")
		return
	}
	deviceID := r.PathValue("id")
	if deviceID == "" {
		writeError(w, http.StatusBadRequest, "device id is required")
		return
	}
	if !opts.Devices.Remove(deviceID) {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (opts Options) handlePairingDeviceRename(w http.ResponseWriter, r *http.Request) {
	if opts.Devices == nil {
		writeError(w, http.StatusServiceUnavailable, "device management not available")
		return
	}
	deviceID := r.PathValue("id")
	if deviceID == "" {
		writeError(w, http.StatusBadRequest, "device id is required")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Name) > 64 {
		writeError(w, http.StatusBadRequest, "name is too long")
		return
	}
	if !opts.Devices.Rename(deviceID, req.Name) {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type networkSettingsResponse struct {
	RemoteAccessEnabled bool     `json:"remote_access_enabled"`
	LANIPs              []string `json:"lan_ips"`
	Allowlist           []string `json:"allowlist"`
	Banlist             []string `json:"banlist"`
}

type networkSettingsRequest struct {
	RemoteAccessEnabled *bool `json:"remote_access_enabled"`
}

type networkEntryRequest struct {
	Entry string `json:"entry"`
}

type networkClientDTO struct {
	Key               string `json:"key"`
	IP                string `json:"ip"`
	Class             string `json:"class"`
	MAC               string `json:"mac,omitempty"`
	Hostname          string `json:"hostname,omitempty"`
	CustomName        string `json:"custom_name,omitempty"`
	FirstSeen         int64  `json:"first_seen"`
	LastSeen          int64  `json:"last_seen"`
	ActiveConnections int    `json:"active_connections"`
}

type networkClientsResponse struct {
	Clients []networkClientDTO `json:"clients"`
}

func (opts Options) handleNetworkSettingsGet(w http.ResponseWriter, r *http.Request) {
	resp := networkSettingsResponse{
		LANIPs:    LANAddresses(),
		Allowlist: []string{},
		Banlist:   []string{},
	}
	if opts.RemoteSettings != nil {
		resp.RemoteAccessEnabled = opts.RemoteSettings.Get().RemoteAccessEnabled
	}
	if opts.IPFilter != nil {
		resp.Allowlist, resp.Banlist = opts.IPFilter.Lists()
	}
	if resp.Allowlist == nil {
		resp.Allowlist = []string{}
	}
	if resp.Banlist == nil {
		resp.Banlist = []string{}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (opts Options) handleNetworkSettingsPut(w http.ResponseWriter, r *http.Request) {
	if opts.RemoteSettings == nil {
		writeError(w, http.StatusServiceUnavailable, "settings store not available")
		return
	}
	var req networkSettingsRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.RemoteAccessEnabled == nil {
		writeError(w, http.StatusBadRequest, "remote_access_enabled is required")
		return
	}
	next := opts.RemoteSettings.Get()
	next.RemoteAccessEnabled = *req.RemoteAccessEnabled
	if err := opts.RemoteSettings.Set(next); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	opts.recordAudit("settings.updated", "", map[string]any{
		"remote_access_enabled": next.RemoteAccessEnabled,
	})
	writeJSON(w, http.StatusOK, networkSettingsResponse{
		RemoteAccessEnabled: next.RemoteAccessEnabled,
		LANIPs:              LANAddresses(),
	})
}

func (opts Options) handleNetworkAllowAdd(w http.ResponseWriter, r *http.Request) {
	opts.mutateIPList(w, r, "allow")
}

func (opts Options) handleNetworkAllowRemove(w http.ResponseWriter, r *http.Request) {
	opts.mutateIPList(w, r, "unallow")
}

func (opts Options) handleNetworkBanAdd(w http.ResponseWriter, r *http.Request) {
	opts.mutateIPList(w, r, "ban")
}

func (opts Options) handleNetworkBanRemove(w http.ResponseWriter, r *http.Request) {
	opts.mutateIPList(w, r, "unban")
}

func (opts Options) mutateIPList(w http.ResponseWriter, r *http.Request, action string) {
	if opts.IPFilter == nil {
		writeError(w, http.StatusServiceUnavailable, "ip filtering not available")
		return
	}
	var req networkEntryRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Entry == "" {
		writeError(w, http.StatusBadRequest, "entry is required")
		return
	}
	var err error
	switch action {
	case "allow":
		err = opts.IPFilter.Allow(req.Entry)
	case "unallow":
		err = opts.IPFilter.Unallow(req.Entry)
	case "ban":
		err = opts.IPFilter.Ban(req.Entry)
	case "unban":
		err = opts.IPFilter.Unban(req.Entry)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	opts.recordAudit("network."+action, "", map[string]any{"entry": req.Entry})
	allowed, banned := opts.IPFilter.Lists()
	remoteEnabled := true
	if opts.RemoteSettings != nil {
		remoteEnabled = opts.RemoteSettings.Get().RemoteAccessEnabled
	}
	writeJSON(w, http.StatusOK, networkSettingsResponse{
		RemoteAccessEnabled: remoteEnabled,
		LANIPs:              LANAddresses(),
		Allowlist:           allowed,
		Banlist:             banned,
	})
}

func (opts Options) handleNetworkClients(w http.ResponseWriter, r *http.Request) {
	arp := opts.clients.loadARP()
	out := make([]networkClientDTO, 0)
	for _, entry := range opts.clients.list() {
		dto := networkClientDTO{
			IP:                entry.IP,
			Class:             string(entry.Class),
			FirstSeen:         entry.FirstSeen.Unix(),
			LastSeen:          entry.LastSeen.Unix(),
			ActiveConnections: entry.Conns,
		}
		if mac, ok := arp[entry.IP]; ok {
			dto.MAC = mac
		}
		// The rename key prefers the MAC so names survive IP changes.
		dto.Key = clientKey(dto.MAC, dto.IP)
		if name := opts.KnownClients.Name(dto.Key); name != "" {
			dto.CustomName = name
		}
		if entry.Class == security.ClientClassLAN {
			dto.Hostname = opts.clients.hostname(entry.IP)
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, networkClientsResponse{Clients: out})
}

func (opts Options) handleNetworkClientRename(w http.ResponseWriter, r *http.Request) {
	if opts.KnownClients == nil {
		writeError(w, http.StatusServiceUnavailable, "client naming not available")
		return
	}
	key := r.PathValue("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "client key is required")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Name) > 64 {
		writeError(w, http.StatusBadRequest, "name is too long")
		return
	}
	if err := opts.KnownClients.Rename(key, req.Name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	opts.recordAudit("network.client_renamed", "", map[string]any{"key": key})
	w.WriteHeader(http.StatusNoContent)
}

// clientKey prefers the MAC (stable across DHCP changes) over the IP.
func clientKey(mac, ip string) string {
	if mac != "" {
		return mac
	}
	return ip
}

func (opts Options) handleTunnelAvailable(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, tunnelAvailableResponse{
		Available: tunnel.IsAvailable(),
		Binary:    tunnel.BinaryPath(),
	})
}

func (opts Options) handleTunnelStatus(w http.ResponseWriter, r *http.Request) {
	if opts.Tunnel == nil {
		writeJSON(w, http.StatusOK, tunnelResponse{Status: string(tunnel.StatusStopped)})
		return
	}
	info := opts.Tunnel.Info()
	writeJSON(w, http.StatusOK, tunnelResponse{
		Status: string(info.Status),
		URL:    info.URL,
		Error:  info.Error,
	})
}

func (opts Options) handleTunnelStart(w http.ResponseWriter, r *http.Request) {
	if opts.Tunnel == nil {
		writeError(w, http.StatusServiceUnavailable, "tunnel not configured")
		return
	}
	if err := opts.Tunnel.Start(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	info := opts.Tunnel.Info()
	writeJSON(w, http.StatusOK, tunnelResponse{
		Status: string(info.Status),
		URL:    info.URL,
		Error:  info.Error,
	})
}

func (opts Options) handleTunnelStop(w http.ResponseWriter, r *http.Request) {
	if opts.Tunnel == nil {
		writeJSON(w, http.StatusOK, tunnelResponse{Status: string(tunnel.StatusStopped)})
		return
	}
	if err := opts.Tunnel.Stop(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	info := opts.Tunnel.Info()
	writeJSON(w, http.StatusOK, tunnelResponse{
		Status: string(info.Status),
		URL:    info.URL,
		Error:  info.Error,
	})
}

func (opts Options) handleTunnelConfigGet(w http.ResponseWriter, r *http.Request) {
	if opts.Tunnel == nil {
		writeJSON(w, http.StatusOK, tunnelConfigResponse{Mode: string(tunnel.ModeQuick)})
		return
	}
	view := opts.Tunnel.ViewConfig()
	writeJSON(w, http.StatusOK, tunnelConfigResponse{
		Mode:     string(view.Mode),
		Hostname: view.Hostname,
		TokenSet: view.TokenSet,
	})
}

func (opts Options) handleTunnelConfigPut(w http.ResponseWriter, r *http.Request) {
	if opts.Tunnel == nil {
		writeError(w, http.StatusServiceUnavailable, "tunnel not configured")
		return
	}
	var req tunnelConfigRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	err := opts.Tunnel.UpdateConfig(tunnel.Config{
		Mode:     tunnel.Mode(req.Mode),
		Token:    req.Token,
		Hostname: req.Hostname,
	})
	switch {
	case err == nil:
	case errors.Is(err, tunnel.ErrTunnelActive):
		writeError(w, http.StatusConflict, err.Error())
		return
	default:
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	view := opts.Tunnel.ViewConfig()
	writeJSON(w, http.StatusOK, tunnelConfigResponse{
		Mode:     string(view.Mode),
		Hostname: view.Hostname,
		TokenSet: view.TokenSet,
	})
}

func (opts Options) handleTunnelBinary(w http.ResponseWriter, r *http.Request) {
	path, source := tunnel.ResolveBinary()
	resp := tunnelBinaryResponse{ManagedPath: tunnel.ManagedBinaryPath()}
	if path != "" {
		resp.Path = path
		resp.Source = string(source)
		if version, err := tunnel.BinaryVersion(r.Context(), path); err == nil {
			resp.Version = version
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (opts Options) handleTunnelDownload(w http.ResponseWriter, r *http.Request) {
	downloader := tunnel.NewDownloader()
	if opts.TunnelDownloadAPI != "" {
		downloader.API = opts.TunnelDownloadAPI
	}
	version, path, err := downloader.InstallLatest(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("cloudflared download failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, tunnelDownloadResponse{Version: version, Path: path})
}

func (opts Options) handleTunnelLogs(w http.ResponseWriter, r *http.Request) {
	if opts.Tunnel == nil {
		writeJSON(w, http.StatusOK, tunnelLogsResponse{Lines: []string{}})
		return
	}
	writeJSON(w, http.StatusOK, tunnelLogsResponse{Lines: opts.Tunnel.Logs()})
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
	return sessionInfoToDTO(info)
}

func sessionInfoToDTO(info session.Info) sessionDTO {
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
		CreatedAt:     info.StartedAt,
		UpdatedAt:     updatedAt,
		ExitedAt:      info.ExitedAt,
		ExitCode:      info.ExitCode,
		Origin:        info.Origin,
		OriginBackend: info.OriginBackend,
		ShimName:      info.ShimName,
		RealBinary:    info.RealBinary,
		Attachable:    info.Attachable,
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
