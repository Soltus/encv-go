package reader

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/container/detector"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/internal/v2/writer"
)

// KVI providers registered in bench_test.go TestMain (text/image/video)

func createV4PluginPathContainer(t *testing.T, containerType uint16, plaintext []byte) (string, string) {
	t.Helper()
	dir := t.TempDir()
	ext := ".sccgv"
	if containerType == types.ContainerTypeImage {
		ext = ".sccgi"
	}
	path := filepath.Join(dir, "test_v4"+ext)
	password := "test-v4-e2e-password"

	salt, err := crypto.GenerateSalt_v2(16)
	if err != nil {
		t.Fatal(err)
	}
	key := crypto.GenerateKey(password, salt, types.KeySize_v2)
	iv, err := crypto.GenerateIV_v2(types.IVSize_v2)
	if err != nil {
		t.Fatal(err)
	}

	var encryptedBuf bytes.Buffer
	if err := crypto.EncryptStream_v2(bytes.NewReader(plaintext), &encryptedBuf, key, iv); err != nil {
		t.Fatal(err)
	}
	ciphertext := encryptedBuf.Bytes()

	v4Header := &types.EnvelopeHeaderV4{
		Magic:         types.MagicHeader_v2,
		Version:       4,
		Flags:         1,
		ContainerType: containerType,
		IsSeekable:    1,
		IDType:        uint32(types.IDType_Raw),
		IDLength:      0,
	}

	w, err := writer.NewSingleFileContainerWriterV4(path, v4Header)
	if err != nil {
		t.Fatal(err)
	}

	kviData, _ := json.Marshal(map[string]string{
		"salt_base64": base64.StdEncoding.EncodeToString(salt),
		"iv_base64":   base64.StdEncoding.EncodeToString(iv),
	})
	if err := w.WriteKVI(kviData); err != nil {
		t.Fatal(err)
	}

	frag := &types.Fragment{
		ID:   "main",
		Type: types.FragmentType_SeekableStream,
	}
	if err := w.WriteFragment(frag, ciphertext); err != nil {
		t.Fatal(err)
	}

	containerTypeStr := "text"
	switch containerType {
	case types.ContainerTypeImage:
		containerTypeStr = "image"
	case types.ContainerTypeVideo:
		containerTypeStr = "video"
	}

	manifestObj := &types.Manifest{
		Kind: types.IndexKind(containerTypeStr),
		KVI:  json.RawMessage(kviData),
	}
	if err := w.WriteManifest(manifestObj); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	return path, password
}

func TestV4DecryptRoundtrip_PluginPath(t *testing.T) {
	original := make([]byte, 512)
	for i := range original {
		original[i] = byte(i)
	}

	path, password := createV4PluginPathContainer(t, types.ContainerTypeText, original)

	factory, err := NewDecryptReaderFactory(path, password)
	if err != nil {
		t.Fatalf("NewDecryptReaderFactory failed: %v", err)
	}
	defer factory.Close()

	decryptReader, err := factory.NewDecryptReader()
	if err != nil {
		t.Fatalf("NewDecryptReader failed: %v", err)
	}
	defer decryptReader.Close()

	var buf bytes.Buffer
	n, err := buf.ReadFrom(decryptReader)
	if err != nil {
		t.Fatalf("ReadFrom (decrypt) failed: %v", err)
	}

	decrypted := buf.Bytes()
	if n != int64(len(original)) {
		t.Errorf("decrypted size %d != original size %d", n, len(original))
	}
	if !bytes.Equal(decrypted, original) {
		t.Error("decrypted data does not match original")
	}
}

func TestV4DecryptRoundtrip_Image(t *testing.T) {
	pngHeader := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	}
	original := make([]byte, 256)
	copy(original, pngHeader)
	for i := len(pngHeader); i < len(original); i++ {
		original[i] = byte(i)
	}

	path, password := createV4PluginPathContainer(t, types.ContainerTypeImage, original)

	factory, err := NewDecryptReaderFactory(path, password)
	if err != nil {
		t.Fatalf("NewDecryptReaderFactory failed: %v", err)
	}
	defer factory.Close()

	decryptReader, err := factory.NewDecryptReader()
	if err != nil {
		t.Fatalf("NewDecryptReader failed: %v", err)
	}
	defer decryptReader.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(decryptReader); err != nil {
		t.Fatalf("ReadFrom (decrypt image) failed: %v", err)
	}

	decrypted := buf.Bytes()
	if !bytes.Equal(decrypted, original) {
		t.Errorf("decrypted image data mismatch: got %d bytes, want %d", len(decrypted), len(original))
	}
	if !bytes.HasPrefix(decrypted, pngHeader) {
		t.Error("decrypted data missing PNG header signature")
	}
}

func TestV4DecryptRoundtrip_MultiFragment(t *testing.T) {
	password := "multi-frag-v4-pw"
	dir := t.TempDir()
	path := filepath.Join(dir, "multi_v4.sccgv")

	salt, _ := crypto.GenerateSalt_v2(16)
	key := crypto.GenerateKey(password, salt, types.KeySize_v2)
	iv, _ := crypto.GenerateIV_v2(types.IVSize_v2)

	v4Header := &types.EnvelopeHeaderV4{
		Magic:         types.MagicHeader_v2,
		Version:       4,
		Flags:         1,
		ContainerType: types.ContainerTypeVideo,
		IsSeekable:    1,
		IDType:        uint32(types.IDType_Raw),
		IDLength:      0,
	}

	w, err := writer.NewSingleFileContainerWriterV4(path, v4Header)
	if err != nil {
		t.Fatal(err)
	}

	kviData, _ := json.Marshal(map[string]string{
		"salt_base64": base64.StdEncoding.EncodeToString(salt),
		"iv_base64":   base64.StdEncoding.EncodeToString(iv),
	})
	w.WriteKVI(kviData)

	var allOriginal []byte
	for i := 0; i < 3; i++ {
		data := make([]byte, 128)
		for j := range data {
			data[j] = byte(i*128 + j)
		}
		allOriginal = append(allOriginal, data...)
	}

	var encryptedBuf bytes.Buffer
	if err := crypto.EncryptStream_v2(bytes.NewReader(allOriginal), &encryptedBuf, key, iv); err != nil {
		t.Fatal(err)
	}
	ciphertext := encryptedBuf.Bytes()

	fragmentSize := len(allOriginal) / 3
	written := uint64(0)
	for i := 0; i < 3; i++ {
		end := written + uint64(fragmentSize)
		if end > uint64(len(ciphertext)) {
			end = uint64(len(ciphertext))
		}
		chunk := ciphertext[written:end]
		frag := &types.Fragment{
			ID:   string(rune('a'+byte(i))) + "_seg",
			Type: types.FragmentType_SeekableStream,
		}
		if err := w.WriteFragment(frag, chunk); err != nil {
			t.Fatal(err)
		}
		written = end
	}

	manifestObj := &types.Manifest{
		Kind: "video",
		KVI:  json.RawMessage(kviData),
	}
	w.WriteManifest(manifestObj)
	w.Close()

	factory, err := NewDecryptReaderFactory(path, password)
	if err != nil {
		t.Fatalf("NewDecryptReaderFactory multi-frag failed: %v", err)
	}
	defer factory.Close()

	decryptReader, err := factory.NewDecryptReader()
	if err != nil {
		t.Fatalf("NewDecryptReader multi-frag failed: %v", err)
	}
	defer decryptReader.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(decryptReader); err != nil {
		t.Fatalf("ReadFrom multi-fragment failed: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), allOriginal) {
		t.Errorf("multi-fragment roundtrip mismatch: got %d, want %d", buf.Len(), len(allOriginal))
	}
}

func TestV4Factory_IsSeekable_DetectCorrectly(t *testing.T) {
	original := make([]byte, 256)
	path, password := createV4PluginPathContainer(t, types.ContainerTypeText, original)

	factory, err := NewDecryptReaderFactory(path, password)
	if err != nil {
		t.Fatal(err)
	}
	defer factory.Close()

	if !factory.IsSeekable() {
		t.Error("V4 plugin-path container should be detected as seekable")
	}
}

func TestV4DetectIndexKind_Image(t *testing.T) {
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	original := make([]byte, 128)
	copy(original, pngHeader)

	path, _ := createV4PluginPathContainer(t, types.ContainerTypeImage, original)

	kind, err := detector.DetectIndexKind(path)
	if err != nil {
		t.Fatalf("DetectIndexKind failed for image V4: %v", err)
	}
	if kind != "image" {
		t.Errorf("DetectIndexKind returned %q, want 'image'", kind)
	}
}

func TestV4DetectIndexKind_Text(t *testing.T) {
	original := make([]byte, 128)
	path, _ := createV4PluginPathContainer(t, types.ContainerTypeText, original)

	kind, err := detector.DetectIndexKind(path)
	if err != nil {
		t.Fatalf("DetectIndexKind failed for text V4: %v", err)
	}
	if kind != "text" {
		t.Errorf("DetectIndexKind returned %q, want 'text'", kind)
	}
}
