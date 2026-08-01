package security

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const pairedDevicesFile = "paired_devices.json"

type PairedDevice struct {
	DeviceID   string    `json:"device_id"`
	DeviceName string    `json:"device_name"`
	Platform   string    `json:"platform"`
	PublicKey  string    `json:"public_key"` // base64-encoded ed25519 public key
	PairedAt   time.Time `json:"paired_at"`
	LastSeen   time.Time `json:"last_seen"`
}

type pairedDevicesData struct {
	Devices []PairedDevice `json:"devices"`
}

type PairedDeviceStore struct {
	mu       sync.Mutex
	path     string
	devices  map[string]PairedDevice // keyed by device_id
	onChange func()
}

func NewPairedDeviceStore(configDir string) (*PairedDeviceStore, error) {
	s := &PairedDeviceStore{
		path:    filepath.Join(configDir, pairedDevicesFile),
		devices: make(map[string]PairedDevice),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *PairedDeviceStore) SetOnChange(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

func (s *PairedDeviceStore) Add(dev PairedDevice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if dev.PairedAt.IsZero() {
		dev.PairedAt = time.Now()
	}
	if dev.LastSeen.IsZero() {
		dev.LastSeen = dev.PairedAt
	}
	s.devices[dev.DeviceID] = dev
	if err := s.save(); err != nil {
		return err
	}
	if s.onChange != nil {
		s.onChange()
	}
	return nil
}

func (s *PairedDeviceStore) Remove(deviceID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[deviceID]; !ok {
		return false
	}
	delete(s.devices, deviceID)
	if err := s.save(); err != nil {
		return false
	}
	if s.onChange != nil {
		s.onChange()
	}
	return true
}

func (s *PairedDeviceStore) Get(deviceID string) (PairedDevice, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dev, ok := s.devices[deviceID]
	return dev, ok
}

func (s *PairedDeviceStore) IsTrusted(deviceID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.devices[deviceID]
	return ok
}

func (s *PairedDeviceStore) GetPublicKey(deviceID string) (ed25519.PublicKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dev, ok := s.devices[deviceID]
	if !ok {
		return nil, fmt.Errorf("device %s not paired", deviceID)
	}
	raw, err := base64.StdEncoding.DecodeString(dev.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("decode public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key length: %d", len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

func (s *PairedDeviceStore) Touch(deviceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dev, ok := s.devices[deviceID]
	if !ok {
		return
	}
	dev.LastSeen = time.Now()
	s.devices[deviceID] = dev
	_ = s.save() // best-effort
}

func (s *PairedDeviceStore) List() []PairedDevice {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PairedDevice, 0, len(s.devices))
	for _, dev := range s.devices {
		out = append(out, dev)
	}
	return out
}

func (s *PairedDeviceStore) load() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read paired devices: %w", err)
	}
	var store pairedDevicesData
	if err := json.Unmarshal(data, &store); err != nil {
		return fmt.Errorf("parse paired devices: %w", err)
	}
	for _, dev := range store.Devices {
		s.devices[dev.DeviceID] = dev
	}
	return nil
}

func (s *PairedDeviceStore) save() error {
	store := pairedDevicesData{
		Devices: make([]PairedDevice, 0, len(s.devices)),
	}
	for _, dev := range s.devices {
		store.Devices = append(store.Devices, dev)
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal paired devices: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0600); err != nil {
		return fmt.Errorf("write paired devices: %w", err)
	}
	return nil
}
