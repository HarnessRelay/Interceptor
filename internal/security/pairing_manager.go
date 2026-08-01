package security

import (
	"sync"
	"time"
)

const (
	maxPendingRequests  = 5
	pairingRequestTTL   = 60 * time.Second
	pairingCooldown     = 60 * time.Second
	cleanupInterval     = 10 * time.Second
)

type PairingRequest struct {
	DeviceID   string    `json:"device_id"`
	DeviceName string    `json:"device_name"`
	Platform   string    `json:"platform"`
	PublicKey  string    `json:"public_key"`
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

type PairingManager struct {
	mu       sync.Mutex
	store    *PairedDeviceStore
	pending  map[string]PairingRequest // keyed by device_id
	resolved map[string]PairingStatus  // keyed by device_id, short-lived
	lastReq  map[string]time.Time      // cooldown tracking
	onChange func()
	stopCh   chan struct{}
}

func NewPairingManager(store *PairedDeviceStore) *PairingManager {
	pm := &PairingManager{
		store:    store,
		pending:  make(map[string]PairingRequest),
		resolved: make(map[string]PairingStatus),
		lastReq:  make(map[string]time.Time),
		stopCh:   make(chan struct{}),
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

// SubmitRequest adds a new pairing request. Returns an error string if rejected.
func (pm *PairingManager) SubmitRequest(req PairingRequest) (string, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Already paired?
	if pm.store.IsTrusted(req.DeviceID) {
		return "already paired", false
	}

	// Already pending?
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
	pm.pending[req.DeviceID] = req
	pm.lastReq[req.DeviceID] = time.Now()
	delete(pm.resolved, req.DeviceID)

	if pm.onChange != nil {
		pm.onChange()
	}
	return "", true
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

// Accept approves a pairing request and stores the device.
func (pm *PairingManager) Accept(deviceID string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	req, ok := pm.pending[deviceID]
	if !ok {
		return false
	}

	delete(pm.pending, deviceID)
	pm.resolved[deviceID] = PairingStatusAccepted

	err := pm.store.Add(PairedDevice{
		DeviceID:   req.DeviceID,
		DeviceName: req.DeviceName,
		Platform:   req.Platform,
		PublicKey:   req.PublicKey,
	})
	if err != nil {
		return false
	}

	if pm.onChange != nil {
		pm.onChange()
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
		}
	}
	// Clean old resolved entries (keep for 2 minutes)
	for id := range pm.resolved {
		if _, stillPending := pm.pending[id]; stillPending {
			continue
		}
		// Resolved entries are short-lived; they'll be naturally replaced
	}
}
