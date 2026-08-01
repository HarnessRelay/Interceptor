package security

import (
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestPairedDeviceStoreAddAndGet(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPairedDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewPairedDeviceStore: %v", err)
	}

	pubKey, _, _ := ed25519.GenerateKey(nil)
	dev := PairedDevice{
		DeviceID:   "abc123",
		DeviceName: "Test Phone",
		Platform:   "android",
		PublicKey:  base64.StdEncoding.EncodeToString(pubKey),
	}
	if err := store.Add(dev); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, ok := store.Get("abc123")
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.DeviceName != "Test Phone" {
		t.Fatalf("DeviceName = %q", got.DeviceName)
	}
	if got.PairedAt.IsZero() {
		t.Fatal("PairedAt not set")
	}
}

func TestPairedDeviceStoreRemove(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPairedDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewPairedDeviceStore: %v", err)
	}

	pubKey, _, _ := ed25519.GenerateKey(nil)
	store.Add(PairedDevice{
		DeviceID:  "abc123",
		PublicKey: base64.StdEncoding.EncodeToString(pubKey),
	})

	if !store.Remove("abc123") {
		t.Fatal("Remove returned false")
	}
	if store.Remove("abc123") {
		t.Fatal("double remove returned true")
	}
	if _, ok := store.Get("abc123"); ok {
		t.Fatal("device still present after remove")
	}
}

func TestPairedDeviceStoreIsTrusted(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPairedDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewPairedDeviceStore: %v", err)
	}

	if store.IsTrusted("unknown") {
		t.Fatal("unknown device trusted")
	}

	pubKey, _, _ := ed25519.GenerateKey(nil)
	store.Add(PairedDevice{
		DeviceID:  "abc123",
		PublicKey: base64.StdEncoding.EncodeToString(pubKey),
	})

	if !store.IsTrusted("abc123") {
		t.Fatal("added device not trusted")
	}
}

func TestPairedDeviceStoreGetPublicKey(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPairedDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewPairedDeviceStore: %v", err)
	}

	pubKey, _, _ := ed25519.GenerateKey(nil)
	store.Add(PairedDevice{
		DeviceID:  "abc123",
		PublicKey: base64.StdEncoding.EncodeToString(pubKey),
	})

	got, err := store.GetPublicKey("abc123")
	if err != nil {
		t.Fatalf("GetPublicKey: %v", err)
	}
	if !got.Equal(pubKey) {
		t.Fatal("public key mismatch")
	}
}

func TestPairedDeviceStorePersistence(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPairedDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewPairedDeviceStore: %v", err)
	}

	pubKey, _, _ := ed25519.GenerateKey(nil)
	store.Add(PairedDevice{
		DeviceID:   "abc123",
		DeviceName: "Persisted",
		PublicKey:  base64.StdEncoding.EncodeToString(pubKey),
	})

	// Reload from disk
	store2, err := NewPairedDeviceStore(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	got, ok := store2.Get("abc123")
	if !ok {
		t.Fatal("device not persisted")
	}
	if got.DeviceName != "Persisted" {
		t.Fatalf("DeviceName = %q", got.DeviceName)
	}
}

func TestPairedDeviceStoreList(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPairedDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewPairedDeviceStore: %v", err)
	}

	if len(store.List()) != 0 {
		t.Fatalf("List() = %d, want 0", len(store.List()))
	}

	pubKey, _, _ := ed25519.GenerateKey(nil)
	store.Add(PairedDevice{DeviceID: "a", PublicKey: base64.StdEncoding.EncodeToString(pubKey)})
	store.Add(PairedDevice{DeviceID: "b", PublicKey: base64.StdEncoding.EncodeToString(pubKey)})

	list := store.List()
	if len(list) != 2 {
		t.Fatalf("List() = %d, want 2", len(list))
	}
}

func TestPairedDeviceStoreOnChange(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPairedDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewPairedDeviceStore: %v", err)
	}

	called := 0
	store.SetOnChange(func() { called++ })

	pubKey, _, _ := ed25519.GenerateKey(nil)
	store.Add(PairedDevice{DeviceID: "a", PublicKey: base64.StdEncoding.EncodeToString(pubKey)})
	store.Remove("a")

	if called != 2 {
		t.Fatalf("onChange called %d times, want 2", called)
	}
}

func TestAuthenticatorDeviceSignature(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPairedDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewPairedDeviceStore: %v", err)
	}

	// Generate a test device identity
	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	deviceID := Fingerprint(pubKey)
	store.Add(PairedDevice{
		DeviceID:  deviceID,
		PublicKey: base64.StdEncoding.EncodeToString(pubKey),
	})

	auth := NewAuthenticator("test-token")
	auth.SetPairedDeviceStore(store)

	// Build a signed request
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	message := fmt.Sprintf("GET\n/api/v1/sessions\n%s\n", ts)
	sig := ed25519.Sign(privKey, []byte(message))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("X-Device-ID", deviceID)
	req.Header.Set("X-Signature", base64.StdEncoding.EncodeToString(sig))
	req.Header.Set("X-Timestamp", ts)

	principal, err := auth.Authenticate(req)
	if err != nil {
		t.Fatalf("Authenticate with device sig: %v", err)
	}
	if principal.Actor != deviceID {
		t.Fatalf("Actor = %q, want %q", principal.Actor, deviceID)
	}
}

func TestAuthenticatorDeviceSignatureRejectsExpiredTimestamp(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPairedDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewPairedDeviceStore: %v", err)
	}

	pubKey, privKey, _ := ed25519.GenerateKey(nil)
	deviceID := Fingerprint(pubKey)
	store.Add(PairedDevice{
		DeviceID:  deviceID,
		PublicKey: base64.StdEncoding.EncodeToString(pubKey),
	})

	auth := NewAuthenticator("test-token")
	auth.SetPairedDeviceStore(store)

	// Use a timestamp from 60 seconds ago (outside the 30s window)
	ts := strconv.FormatInt(time.Now().Unix()-60, 10)
	message := fmt.Sprintf("GET\n/api/v1/sessions\n%s\n", ts)
	sig := ed25519.Sign(privKey, []byte(message))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("X-Device-ID", deviceID)
	req.Header.Set("X-Signature", base64.StdEncoding.EncodeToString(sig))
	req.Header.Set("X-Timestamp", ts)

	if _, err := auth.Authenticate(req); err != ErrUnauthenticated {
		t.Fatalf("expired timestamp: got %v, want ErrUnauthenticated", err)
	}
}

func TestAuthenticatorDeviceSignatureRejectsWrongKey(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPairedDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewPairedDeviceStore: %v", err)
	}

	pubKey, _, _ := ed25519.GenerateKey(nil)
	deviceID := Fingerprint(pubKey)
	store.Add(PairedDevice{
		DeviceID:  deviceID,
		PublicKey: base64.StdEncoding.EncodeToString(pubKey),
	})

	// Sign with a DIFFERENT key
	_, wrongPrivKey, _ := ed25519.GenerateKey(nil)

	auth := NewAuthenticator("test-token")
	auth.SetPairedDeviceStore(store)

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	message := fmt.Sprintf("GET\n/api/v1/sessions\n%s\n", ts)
	sig := ed25519.Sign(wrongPrivKey, []byte(message))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	req.Header.Set("X-Device-ID", deviceID)
	req.Header.Set("X-Signature", base64.StdEncoding.EncodeToString(sig))
	req.Header.Set("X-Timestamp", ts)

	if _, err := auth.Authenticate(req); err != ErrUnauthenticated {
		t.Fatalf("wrong key: got %v, want ErrUnauthenticated", err)
	}
}

func TestPairedDeviceStoreTouch(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPairedDeviceStore(dir)
	if err != nil {
		t.Fatalf("NewPairedDeviceStore: %v", err)
	}

	pubKey, _, _ := ed25519.GenerateKey(nil)
	store.Add(PairedDevice{DeviceID: "a", PublicKey: base64.StdEncoding.EncodeToString(pubKey)})

	before, _ := store.Get("a")
	store.Touch("a")
	after, _ := store.Get("a")

	if after.LastSeen.Before(before.LastSeen) {
		t.Fatal("Touch did not update LastSeen")
	}
}
