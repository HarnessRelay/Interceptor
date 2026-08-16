package security

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SessionCookieName = "harnessrelay_session"
	CSRFHeaderName    = "X-CSRF-Token"
)

// Cookie session kinds.
const (
	SessionKindHost   = "host"
	SessionKindDevice = "device"
)

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrForbidden       = errors.New("forbidden")
)

type Principal struct {
	Actor      string
	CSRFToken  string
	FromCookie bool
	// Master marks the static daemon token or a login it produced. Master
	// credentials are only accepted from the host machine.
	Master bool
	// DeviceID is set for paired-device principals (signature or web token).
	DeviceID string
	// SessionKind is set for cookie sessions (host or device).
	SessionKind string
}

type Authenticator struct {
	tokenHash [32]byte
	mu        sync.Mutex
	sessions  map[string]sessionRecord
	now       func() time.Time
	devices   *PairedDeviceStore
}

type sessionRecord struct {
	CSRFToken string
	ExpiresAt time.Time
	Actor     string
	Kind      string
	DeviceID  string
}

func NewAuthenticator(token string) *Authenticator {
	if token == "" {
		token = GenerateToken()
	}
	return &Authenticator{
		tokenHash: sha256.Sum256([]byte(token)),
		sessions:  make(map[string]sessionRecord),
		now:       time.Now,
	}
}

func GenerateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// GenerateDeviceToken creates a new opaque web-device credential with the
// hrk_ prefix plus its SHA-256 hex hash for storage.
func GenerateDeviceToken() (token, tokenHash string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	token = DeviceTokenPrefix + base64.RawURLEncoding.EncodeToString(b)
	tokenHash = HashDeviceToken(token)
	return token, tokenHash
}

func HashDeviceToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (a *Authenticator) SetPairedDeviceStore(devices *PairedDeviceStore) {
	a.devices = devices
}

func (a *Authenticator) CheckToken(token string) bool {
	hash := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(hash[:], a.tokenHash[:]) == 1
}

// Login exchanges the static daemon token for a host cookie session. Only
// meaningful for host clients; callers must reject it remotely.
func (a *Authenticator) Login(w http.ResponseWriter, token string) (Principal, bool) {
	if !a.CheckToken(token) {
		return Principal{}, false
	}
	return a.newSession(w, "local", SessionKindHost, true, ""), true
}

// LoginDevice mints a device cookie session for an already-authenticated
// device principal. Device sessions are valid from any client class (host,
// LAN, tunnel) so browser WebSockets work uniformly through cookies.
func (a *Authenticator) LoginDevice(w http.ResponseWriter, principal Principal) Principal {
	return a.newSession(w, principal.Actor, SessionKindDevice, false, principal.DeviceID)
}

func (a *Authenticator) newSession(w http.ResponseWriter, actor, kind string, master bool, deviceID string) Principal {
	sessionID := GenerateToken()
	csrf := GenerateToken()
	expires := a.now().Add(12 * time.Hour)
	a.mu.Lock()
	a.sessions[sessionID] = sessionRecord{CSRFToken: csrf, ExpiresAt: expires, Actor: actor, Kind: kind, DeviceID: deviceID}
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
	return Principal{Actor: actor, CSRFToken: csrf, FromCookie: true, Master: master, SessionKind: kind, DeviceID: deviceID}
}

func (a *Authenticator) Authenticate(r *http.Request) (Principal, error) {
	if token := bearerToken(r); token != "" {
		if a.CheckToken(token) {
			return Principal{Actor: "local", Master: true}, nil
		}
		if strings.HasPrefix(token, DeviceTokenPrefix) {
			if a.devices != nil {
				if dev, ok := a.devices.FindByTokenHash(HashDeviceToken(token)); ok {
					a.devices.Touch(dev.DeviceID)
					return Principal{Actor: dev.DeviceID, DeviceID: dev.DeviceID}, nil
				}
			}
			return Principal{}, ErrUnauthenticated
		}
		return Principal{}, ErrUnauthenticated
	}
	if principal, err := a.authenticateDeviceSignature(r); err == nil {
		return principal, nil
	}
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil || cookie.Value == "" {
		return Principal{}, ErrUnauthenticated
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	record, ok := a.sessions[cookie.Value]
	if !ok {
		return Principal{}, ErrUnauthenticated
	}
	if !record.ExpiresAt.After(a.now()) {
		delete(a.sessions, cookie.Value)
		return Principal{}, ErrUnauthenticated
	}
	return Principal{
		Actor:       record.Actor,
		CSRFToken:   record.CSRFToken,
		FromCookie:  true,
		Master:      record.Kind == SessionKindHost,
		SessionKind: record.Kind,
		DeviceID:    record.DeviceID,
	}, nil
}

// Authorize authenticates the request and enforces CSRF for cookie sessions.
func (a *Authenticator) Authorize(r *http.Request) (Principal, error) {
	principal, err := a.Authenticate(r)
	if err != nil {
		return Principal{}, err
	}
	if principal.FromCookie && unsafeMethod(r.Method) {
		if r.Header.Get(CSRFHeaderName) != principal.CSRFToken {
			return Principal{}, ErrForbidden
		}
	}
	return principal, nil
}

// AuthorizeRequest is Authorize plus the client-class policy: master
// credentials (static token or host login sessions) are rejected from any
// client that is not the host machine. Device credentials are valid
// everywhere. Network-level gating (allowlist/banlist/remote toggle) is
// enforced by middleware before this runs.
func (a *Authenticator) AuthorizeRequest(r *http.Request) (Principal, error) {
	principal, err := a.Authorize(r)
	if err != nil {
		return principal, err
	}
	if principal.Master && ClassifyClient(r).Class != ClientClassHost {
		return Principal{}, ErrForbidden
	}
	return principal, nil
}

func bearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if value == "" {
		return ""
	}
	prefix := "Bearer "
	if !strings.HasPrefix(value, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, prefix))
}

func unsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

const maxSignatureDrift = 30 * time.Second

func (a *Authenticator) authenticateDeviceSignature(r *http.Request) (Principal, error) {
	deviceID := r.Header.Get("X-Device-ID")
	sigB64 := r.Header.Get("X-Signature")
	tsStr := r.Header.Get("X-Timestamp")
	if deviceID == "" || sigB64 == "" || tsStr == "" {
		return Principal{}, ErrUnauthenticated
	}

	if a.devices == nil {
		return Principal{}, ErrUnauthenticated
	}

	pubKey, err := a.devices.GetPublicKey(deviceID)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	now := a.now().Unix()
	drift := now - ts
	if drift < 0 {
		drift = -drift
	}
	if drift > int64(maxSignatureDrift.Seconds()) {
		return Principal{}, ErrUnauthenticated
	}

	bodyHash := sha256Hash(r)
	message := fmt.Sprintf("%s\n%s\n%s\n%s", r.Method, r.URL.Path, tsStr, bodyHash)

	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}

	if !VerifySignature(pubKey, []byte(message), sig) {
		return Principal{}, ErrUnauthenticated
	}

	a.devices.Touch(deviceID)
	return Principal{Actor: deviceID, DeviceID: deviceID}, nil
}

func sha256Hash(r *http.Request) string {
	if r.Body == nil || r.ContentLength == 0 {
		return ""
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return ""
	}
	r.Body.Close()
	// Restore body for downstream handlers
	r.Body = io.NopCloser(strings.NewReader(string(body)))
	hash := sha256.Sum256(body)
	return base64.StdEncoding.EncodeToString(hash[:])
}
