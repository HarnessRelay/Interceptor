package security

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"
)

const (
	maxPendingRequests = 5
	pairingRequestTTL  = 5 * time.Minute
	pairingCooldown    = 60 * time.Second
	cleanupInterval    = 10 * time.Second
	webClaimTTL        = 10 * time.Minute
)

type PairingRequest struct {
	DeviceID   string    `json:"device_id"`
	DeviceName string    `json:"device_name"`
	Platform   string    `json:"platform"`
	PublicKey  string    `json:"public_key"`
	Type       string    `json:"type"`
	Code       string    `json:"code"`
	ReceivedAt time.Time `json:"received_at"`
}

type PairingStatus string

const (
	PairingStatusPending  PairingStatus = "pending"
	PairingStatusAccepted PairingStatus = "accepted"
	PairingStatusRejected PairingStatus = "rejected"
	PairingStatusExpired  PairingStatus = "expired"
	PairingStatusUnknown  PairingStatus = "unknown"
)

// webResolution tracks a resolved web pairing request so the requesting
// browser can claim its device token exactly once.
type webResolution struct {
	status    PairingStatus
	secret    string
	deviceID  string
	token     string // plaintext hrk_ token; cleared on first claim
	claimed   bool
	createdAt time.Time
}

type PairingManager struct {
	mu        sync.Mutex
	store     *PairedDeviceStore
	pending   map[string]PairingRequest // keyed by device_id
	resolved  map[string]PairingStatus  // keyed by device_id, short-lived
	webClaims map[string]*webResolution // keyed by request id
	lastReq   map[string]time.Time      // cooldown tracking
	onChange  func()
	stopCh    chan struct{}
}

func NewPairingManager(store *PairedDeviceStore) *PairingManager {
	pm := &PairingManager{
		store:     store,
		pending:   make(map[string]PairingRequest),
		resolved:  make(map[string]PairingStatus),
		webClaims: make(map[string]*webResolution),
		lastReq:   make(map[string]time.Time),
		stopCh:    make(chan struct{}),
	}
	go pm.cleanupLoop()
	return pm
}

func (pm *PairingManager) SetOnChange(fn func()) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.onChange = fn
}

func (pm *PairingManager) Stop() {
	close(pm.stopCh)
}

// GenerateCode produces a random 6-digit verification code shown on both the
// requesting device and the daemon approval dialog so the user can compare
// them before accepting.
func GenerateCode() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

func randomID(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(b)
}

// SubmitRequest adds a new mobile pairing request. The returned request
// carries the verification code the caller must display.
func (pm *PairingManager) SubmitRequest(req PairingRequest) (string, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Already paired?
	if pm.store.IsTrusted(req.DeviceID) {
		return "already paired", false
	}

	// Already pending: keep the original code so both screens stay in sync;
	// the handler re-reads it via PendingCode.
	if _, ok := pm.pending[req.DeviceID]; ok {
		return "", true
	}

	// Cooldown check
	if last, ok := pm.lastReq[req.DeviceID]; ok {
		if time.Since(last) < pairingCooldown {
			return "too many requests", false
		}
	}

	// Max pending check
	if len(pm.pending) >= maxPendingRequests {
		return "too many pending requests", false
	}

	req.ReceivedAt = time.Now()
	req.Type = DeviceTypeMobile
	req.Code = GenerateCode()
	pm.pending[req.DeviceID] = req
	pm.lastReq[req.DeviceID] = time.Now()
	delete(pm.resolved, req.DeviceID)

	if pm.onChange != nil {
		pm.onChange()
	}
	return "", true
}

// PendingCode returns the verification code for a pending mobile request so
// the submit response can echo it to the requesting device.
func (pm *PairingManager) PendingCode(deviceID string) string {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if req, ok := pm.pending[deviceID]; ok {
		return req.Code
	}
	return ""
}

// SubmitWebRequest starts a web-device pairing flow. The requester receives
// a request id, the 6-digit verification code to display, and a poll secret
// that is required to claim the resulting device token.
func (pm *PairingManager) SubmitWebRequest(deviceName string) (requestID, code, secret string, err string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if len(pm.webClaims) >= maxPendingRequests {
		return "", "", "", "too many pending requests"
	}

	deviceID := randomID("web_")
	for _, ok := pm.pending[deviceID]; ok; {
		deviceID = randomID("web_")
	}

	req := PairingRequest{
		DeviceID:   deviceID,
		DeviceName: deviceName,
		Platform:   "web",
		Type:       DeviceTypeWeb,
		Code:       GenerateCode(),
		ReceivedAt: time.Now(),
	}
	pm.pending[deviceID] = req
	pm.lastReq[deviceID] = time.Now()

	requestID = randomID("pr_")
	secret = GenerateToken()
	pm.webClaims[requestID] = &webResolution{
		status:    PairingStatusPending,
		secret:    secret,
		deviceID:  deviceID,
		createdAt: time.Now(),
	}

	if pm.onChange != nil {
		pm.onChange()
	}
	return requestID, req.Code, secret, ""
}

// PollWebRequest reports a web pairing request's status. After acceptance it
// hands the device token to the original requester exactly once; the secret
// from submission is required.
func (pm *PairingManager) PollWebRequest(requestID, secret string) (PairingStatus, string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	res, ok := pm.webClaims[requestID]
	if !ok {
		return PairingStatusUnknown, ""
	}
	if secret == "" || secret != res.secret {
		return PairingStatusUnknown, ""
	}
	if res.status == PairingStatusAccepted && !res.claimed {
		res.claimed = true
		token := res.token
		res.token = ""
		return PairingStatusAccepted, token
	}
	return res.status, ""
}

// GetStatus returns the status of a pairing request.
func (pm *PairingManager) GetStatus(deviceID string) PairingStatus {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.store.IsTrusted(deviceID) {
		return PairingStatusAccepted
	}
	if status, ok := pm.resolved[deviceID]; ok {
		return status
	}
	if _, ok := pm.pending[deviceID]; ok {
		return PairingStatusPending
	}
	return PairingStatusUnknown
}

// PendingRequests returns all pending pairing requests.
func (pm *PairingManager) PendingRequests() []PairingRequest {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	out := make([]PairingRequest, 0, len(pm.pending))
	for _, req := range pm.pending {
		out = append(out, req)
	}
	return out
}

// Accept approves a pairing request and stores the device. For web requests
// a fresh device token is generated and held for one-time claim by the
// requester.
func (pm *PairingManager) Accept(deviceID string) bool {
	pm.mu.Lock()

	req, ok := pm.pending[deviceID]
	if !ok {
		pm.mu.Unlock()
		return false
	}

	delete(pm.pending, deviceID)
	pm.resolved[deviceID] = PairingStatusAccepted

	device := PairedDevice{
		DeviceID:   req.DeviceID,
		DeviceName: req.DeviceName,
		Platform:   req.Platform,
		PublicKey:  req.PublicKey,
		Type:       req.Type,
	}

	var token string
	if req.Type == DeviceTypeWeb {
		var hash string
		token, hash = GenerateDeviceToken()
		device.TokenHash = hash
	}

	store := pm.store
	onChange := pm.onChange
	var webRes *webResolution
	for _, res := range pm.webClaims {
		if res.deviceID == deviceID && res.status == PairingStatusPending {
			res.status = PairingStatusAccepted
			res.token = token
			webRes = res
		}
	}
	pm.mu.Unlock()

	if err := store.Add(device); err != nil {
		if webRes != nil {
			pm.mu.Lock()
			webRes.status = PairingStatusRejected
			webRes.token = ""
			pm.mu.Unlock()
		}
		return false
	}

	if onChange != nil {
		onChange()
	}
	return true
}

// Reject rejects a pairing request.
func (pm *PairingManager) Reject(deviceID string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.pending[deviceID]; !ok {
		return false
	}
	delete(pm.pending, deviceID)
	pm.resolved[deviceID] = PairingStatusRejected
	for _, res := range pm.webClaims {
		if res.deviceID == deviceID && res.status == PairingStatusPending {
			res.status = PairingStatusRejected
			res.token = ""
		}
	}

	if pm.onChange != nil {
		pm.onChange()
	}
	return true
}

func (pm *PairingManager) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			pm.cleanup()
		case <-pm.stopCh:
			return
		}
	}
}

func (pm *PairingManager) cleanup() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	now := time.Now()
	for id, req := range pm.pending {
		if now.Sub(req.ReceivedAt) > pairingRequestTTL {
			delete(pm.pending, id)
			pm.resolved[id] = PairingStatusExpired
			for _, res := range pm.webClaims {
				if res.deviceID == id && res.status == PairingStatusPending {
					res.status = PairingStatusExpired
				}
			}
		}
	}
	for id, res := range pm.webClaims {
		if now.Sub(res.createdAt) > webClaimTTL {
			delete(pm.webClaims, id)
		}
	}
}
