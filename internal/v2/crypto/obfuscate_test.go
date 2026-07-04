package crypto

import (
	"bytes"
	"testing"
)

func TestObfuscateDeobfuscateRoundTrip(t *testing.T) {
	original := []byte(`{"version":4,"tracks":[{"id":1,"codec":"h264"}]}`)
	obfuscated, err := ObfuscateManifest(original)
	if err != nil {
		t.Fatalf("ObfuscateManifest failed: %v", err)
	}

	deobfuscated, err := DeobfuscateManifest(obfuscated)
	if err != nil {
		t.Fatalf("DeobfuscateManifest failed: %v", err)
	}

	if !bytes.Equal(deobfuscated, original) {
		t.Errorf("Round-trip failed: got %s, want %s", deobfuscated, original)
	}
}

func TestObfuscateDeobfuscateEmptyData(t *testing.T) {
	original := []byte{}
	obfuscated, err := ObfuscateManifest(original)
	if err != nil {
		t.Fatalf("ObfuscateManifest failed: %v", err)
	}

	if len(obfuscated) != ObfuscationSaltSize {
		t.Errorf("Obfuscated empty data should be exactly salt size: got %d, want %d", len(obfuscated), ObfuscationSaltSize)
	}

	deobfuscated, err := DeobfuscateManifest(obfuscated)
	if err != nil {
		t.Fatalf("DeobfuscateManifest failed: %v", err)
	}

	if !bytes.Equal(deobfuscated, original) {
		t.Errorf("Round-trip with empty data failed: got %v, want %v", deobfuscated, original)
	}
}

func TestObfuscateDeobfuscateSmallData(t *testing.T) {
	original := []byte("short data 12345")
	obfuscated, err := ObfuscateManifest(original)
	if err != nil {
		t.Fatalf("ObfuscateManifest failed: %v", err)
	}

	deobfuscated, err := DeobfuscateManifest(obfuscated)
	if err != nil {
		t.Fatalf("DeobfuscateManifest failed: %v", err)
	}

	if !bytes.Equal(deobfuscated, original) {
		t.Errorf("Round-trip with small data failed: got %s, want %s", deobfuscated, original)
	}
}

func TestObfuscateDeobfuscateLargeData(t *testing.T) {
	original := make([]byte, 100)
	for i := range original {
		original[i] = byte(i % 256)
	}

	obfuscated, err := ObfuscateManifest(original)
	if err != nil {
		t.Fatalf("ObfuscateManifest failed: %v", err)
	}

	deobfuscated, err := DeobfuscateManifest(obfuscated)
	if err != nil {
		t.Fatalf("DeobfuscateManifest failed: %v", err)
	}

	if !bytes.Equal(deobfuscated, original) {
		t.Errorf("Round-trip with large data failed")
	}
}

func TestObfuscatedDoesNotContainPlaintext(t *testing.T) {
	original := []byte(`{"version":4,"tracks":[{"id":1,"codec":"h264"}]}`)
	obfuscated, err := ObfuscateManifest(original)
	if err != nil {
		t.Fatalf("ObfuscateManifest failed: %v", err)
	}

	if bytes.Contains(obfuscated, original) {
		t.Error("Obfuscated data contains original plaintext as substring")
	}
}

func TestDeobfuscateTooShort(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	_, err := DeobfuscateManifest(data)
	if err != ErrObfuscatedDataTooShort {
		t.Errorf("Expected ErrObfuscatedDataTooShort, got %v", err)
	}
}

func TestObfuscationUsesDifferentSalts(t *testing.T) {
	original := []byte("same input data")
	obfuscated1, err := ObfuscateManifest(original)
	if err != nil {
		t.Fatalf("First ObfuscateManifest failed: %v", err)
	}

	obfuscated2, err := ObfuscateManifest(original)
	if err != nil {
		t.Fatalf("Second ObfuscateManifest failed: %v", err)
	}

	if bytes.Equal(obfuscated1, obfuscated2) {
		t.Error("Two obfuscations of the same data produced identical output (salt should be random)")
	}
}
