package security

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	deviceKeyFile  = "device_key.pem"
	deviceIDFile   = "device_id"
	deviceNameFile = "device_name"
)

type DeviceIdentity struct {
	ID         string // 16-char hex fingerprint
	Name       string // human-readable device name
	PublicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

// LoadOrCreateDeviceIdentity loads an existing Ed25519 keypair from configDir,
// or generates a new one on first run.
func LoadOrCreateDeviceIdentity(configDir, hostname string) (*DeviceIdentity, error) {
	keyPath := filepath.Join(configDir, deviceKeyFile)
	idPath := filepath.Join(configDir, deviceIDFile)
	namePath := filepath.Join(configDir, deviceNameFile)

	privKey, err := loadPrivateKey(keyPath)
	if errors.Is(err, os.ErrNotExist) {
		return generateAndSave(configDir, hostname)
	}
	if err != nil {
		return nil, fmt.Errorf("load device key: %w", err)
	}

	pubKey := privKey.Public().(ed25519.PublicKey)
	deviceID, err := os.ReadFile(idPath)
	if err != nil {
		return nil, fmt.Errorf("load device id: %w", err)
	}

	name := hostname
	if data, err := os.ReadFile(namePath); err == nil {
		if n := strings.TrimSpace(string(data)); n != "" {
			name = n
		}
	}

	return &DeviceIdentity{
		ID:         strings.TrimSpace(string(deviceID)),
		Name:       name,
		PublicKey:  pubKey,
		privateKey: privKey,
	}, nil
}

// Sign signs the given message with the device's private key.
func (d *DeviceIdentity) Sign(message []byte) []byte {
	return ed25519.Sign(d.privateKey, message)
}

// Fingerprint returns the first 16 hex chars of SHA-256(public_key).
func Fingerprint(pubKey ed25519.PublicKey) string {
	hash := sha256.Sum256(pubKey)
	return hex.EncodeToString(hash[:8])
}

// ShortFingerprint returns the first 8 hex chars for display during pairing.
func ShortFingerprint(pubKey ed25519.PublicKey) string {
	return Fingerprint(pubKey)[:8]
}

// VerifySignature verifies an Ed25519 signature.
func VerifySignature(pubKey ed25519.PublicKey, message, signature []byte) bool {
	return ed25519.Verify(pubKey, message, signature)
}

func generateAndSave(configDir, hostname string) (*DeviceIdentity, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	// Save private key as PEM
	keyPath := filepath.Join(configDir, deviceKeyFile)
	pemBlock := &pem.Block{
		Type:  "ED25519 PRIVATE KEY",
		Bytes: privKey.Seed(),
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(pemBlock), 0600); err != nil {
		return nil, fmt.Errorf("write device key: %w", err)
	}

	deviceID := Fingerprint(pubKey)
	idPath := filepath.Join(configDir, deviceIDFile)
	if err := os.WriteFile(idPath, []byte(deviceID+"\n"), 0644); err != nil {
		return nil, fmt.Errorf("write device id: %w", err)
	}

	namePath := filepath.Join(configDir, deviceNameFile)
	if err := os.WriteFile(namePath, []byte(hostname+"\n"), 0644); err != nil {
		return nil, fmt.Errorf("write device name: %w", err)
	}

	return &DeviceIdentity{
		ID:         deviceID,
		Name:       hostname,
		PublicKey:  pubKey,
		privateKey: privKey,
	}, nil
}

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in %s", path)
	}
	if block.Type != "ED25519 PRIVATE KEY" {
		return nil, fmt.Errorf("unexpected PEM type %q", block.Type)
	}
	seed := block.Bytes
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid seed length: %d", len(seed))
	}
	return ed25519.NewKeyFromSeed(seed), nil
}
