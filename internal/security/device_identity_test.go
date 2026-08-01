package security

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateDeviceIdentity_CreatesNew(t *testing.T) {
	dir := t.TempDir()
	identity, err := LoadOrCreateDeviceIdentity(dir, "test-host")
	if err != nil {
		t.Fatalf("LoadOrCreateDeviceIdentity: %v", err)
	}

	if identity.ID == "" {
		t.Fatal("device ID is empty")
	}
	if len(identity.ID) != 16 {
		t.Fatalf("device ID length = %d, want 16", len(identity.ID))
	}
	if identity.Name != "test-host" {
		t.Fatalf("device name = %q, want %q", identity.Name, "test-host")
	}
	if len(identity.PublicKey) != ed25519.PublicKeySize {
		t.Fatalf("public key size = %d, want %d", len(identity.PublicKey), ed25519.PublicKeySize)
	}

	// Verify files were created
	for _, name := range []string{"device_key.pem", "device_id", "device_name"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("file %s not created: %v", name, err)
		}
	}

	// Verify key file has restrictive permissions
	info, err := os.Stat(filepath.Join(dir, "device_key.pem"))
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("key file permissions = %o, want 0600", info.Mode().Perm())
	}
}

func TestLoadOrCreateDeviceIdentity_LoadsExisting(t *testing.T) {
	dir := t.TempDir()
	first, err := LoadOrCreateDeviceIdentity(dir, "test-host")
	if err != nil {
		t.Fatalf("first load: %v", err)
	}

	second, err := LoadOrCreateDeviceIdentity(dir, "renamed-host")
	if err != nil {
		t.Fatalf("second load: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("device ID changed: %s != %s", first.ID, second.ID)
	}
	if !first.PublicKey.Equal(second.PublicKey) {
		t.Fatal("public key changed across loads")
	}
	// Name should persist from first creation
	if second.Name != "test-host" {
		t.Fatalf("device name = %q, want %q", second.Name, "test-host")
	}
}

func TestDeviceIdentitySignAndVerify(t *testing.T) {
	dir := t.TempDir()
	identity, err := LoadOrCreateDeviceIdentity(dir, "test-host")
	if err != nil {
		t.Fatalf("LoadOrCreateDeviceIdentity: %v", err)
	}

	message := []byte("test message to sign")
	sig := identity.Sign(message)

	if !VerifySignature(identity.PublicKey, message, sig) {
		t.Fatal("valid signature rejected")
	}

	// Tampered message should fail
	tampered := []byte("tampered message")
	if VerifySignature(identity.PublicKey, tampered, sig) {
		t.Fatal("tampered message accepted")
	}

	// Wrong key should fail
	otherDir := t.TempDir()
	other, err := LoadOrCreateDeviceIdentity(otherDir, "other-host")
	if err != nil {
		t.Fatalf("other identity: %v", err)
	}
	if VerifySignature(other.PublicKey, message, sig) {
		t.Fatal("signature verified with wrong key")
	}
}

func TestFingerprint(t *testing.T) {
	dir := t.TempDir()
	identity, err := LoadOrCreateDeviceIdentity(dir, "test-host")
	if err != nil {
		t.Fatalf("LoadOrCreateDeviceIdentity: %v", err)
	}

	fp := Fingerprint(identity.PublicKey)
	if len(fp) != 16 {
		t.Fatalf("fingerprint length = %d, want 16", len(fp))
	}

	short := ShortFingerprint(identity.PublicKey)
	if len(short) != 8 {
		t.Fatalf("short fingerprint length = %d, want 8", len(short))
	}
	if fp[:8] != short {
		t.Fatalf("short fingerprint %q != first 8 chars of full %q", short, fp)
	}
}

func TestFingerprintDeterministic(t *testing.T) {
	dir := t.TempDir()
	identity, err := LoadOrCreateDeviceIdentity(dir, "test-host")
	if err != nil {
		t.Fatalf("LoadOrCreateDeviceIdentity: %v", err)
	}

	fp1 := Fingerprint(identity.PublicKey)
	fp2 := Fingerprint(identity.PublicKey)
	if fp1 != fp2 {
		t.Fatalf("fingerprint not deterministic: %s != %s", fp1, fp2)
	}
}
