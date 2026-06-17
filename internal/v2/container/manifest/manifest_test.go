package manifest

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/container/block"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/pluginsext"
	"github.com/Soltus/encv-go/internal/v2/types"
)

const testPassword = "test-password-123"

func createMinimalV3Fixture(t testing.TB, dataSize int64) string {
	t.Helper()
	tempDir := t.TempDir()
	containerPath := filepath.Join(tempDir, "fixture_v3.sccgv")

	salt, _ := crypto.GenerateSalt_v2(types.SaltSize_v2)
	iv, _ := crypto.GenerateIV_v2(types.IVSize_v2)
	key := crypto.GenerateKey(testPassword, salt, types.KeySize_v2)

	origData := make([]byte, dataSize)
	rand.Read(origData)

	var encryptedBuf bytes.Buffer
	crypto.EncryptStream_v2(bytes.NewReader(origData), &encryptedBuf, key, iv)

	kviRaw := map[string]string{
		"salt_base64": crypto.Base64Encode_v2(salt),
		"iv_base64":   crypto.Base64Encode_v2(iv),
	}
	kviBytes, _ := json.Marshal(kviRaw)

	mf := &types.Manifest{
		Kind: "text",
		KVI:  json.RawMessage(kviBytes),
		Fragments: []types.Fragment{
			{ID: "main", Type: types.FragmentType_AtomicFile, Length: uint64(encryptedBuf.Len())},
		},
	}

	header, _ := types.CreateHeaderV3(true, types.IDType_Raw, nil)

	f, _ := os.Create(containerPath)
	types.WriteHeaderV3(f, header)

	block.WriteBlock(f, types.BlockTypeKVI_v2, kviBytes)
	block.WriteBlock(f, types.BlockTypeData_v2, encryptedBuf.Bytes())

	manifestBytes, _ := json.Marshal(mf)
	manifestEncrypted, _ := EncryptManifest(manifestBytes)
	block.WriteBlock(f, types.BlockTypeManifest_v2, manifestEncrypted)

	currentPos, _ := f.Seek(0, io.SeekCurrent)
	headerSize := block.GetBlockHeader_v2_Size()
	footer := types.EnvelopeFooter_v2{
		Magic:          types.MagicFooter_v2,
		ManifestOffset: uint64(currentPos) - uint64(len(manifestEncrypted)) - uint64(headerSize),
		ManifestLength: uint64(len(manifestEncrypted)),
	}
	binary.Write(f, types.ByteOrder_v2, &footer)
	f.Close()

	return containerPath
}

func createMinimalV4Fixture(t testing.TB, dataSize int64) string {
	t.Helper()
	tempDir := t.TempDir()
	containerPath := filepath.Join(tempDir, "fixture_v4"+pluginsext.VideoExt)

	salt, _ := crypto.GenerateSalt_v2(types.SaltSize_v2)
	key := crypto.GenerateKey(testPassword, salt, types.KeySize_v2)
	macKey := crypto.DeriveMACKey(testPassword, bytes.Repeat([]byte{0xAB}, crypto.MACSaltLength))

	origData := make([]byte, dataSize)
	rand.Read(origData)

	encResult, _ := crypto.EncryptSegment(origData, key, macKey, 0, crypto.CompressionModeNone)
	encResult.SegmentID = 0

	kviRaw := map[string]string{
		"salt_base64": crypto.Base64Encode_v2(salt),
		"iv_base64":   crypto.Base64Encode_v2(make([]byte, crypto.IVSize_v2)),
	}
	kviBytes, _ := json.Marshal(kviRaw)

	v4Manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "test-v4-fixture",
		ContainerType: "text",
		IsSeekable:    false,
		Segments: []types.Segment_v4{
			{ID: "seg-0", Offset: 0, Size: uint64(len(encResult.EncryptedData))},
		},
		KVI: json.RawMessage(kviBytes),
	}
	manifestJSON, _ := v4Manifest.SerializeToJSON_v4()

	obfuscatedManifest, _ := crypto.ObfuscateManifest(manifestJSON)

	hdr, _ := types.CreateHeaderV4(true, types.ContainerTypeText, false, types.IDType_Raw, nil, [16]byte{})
	hdr.ManifestOffset = uint32(types.EnvelopeHeaderSize_v4 + len(encResult.EncryptedData))
	hdr.ManifestLength = uint32(len(obfuscatedManifest))

	f, _ := os.Create(containerPath)
	types.WriteHeaderV4(f, hdr)
	f.Write(encResult.EncryptedData)
	f.Write(obfuscatedManifest)

	footer := &types.EnvelopeFooterV4{
		Magic:       types.MagicFooter_v2,
		GlobalCRC32: 0,
	}
	types.WriteFooterV4(f, footer)
	f.Close()

	return containerPath
}

func TestExtractManifest_V3(t *testing.T) {
	path := createMinimalV3Fixture(t, 4096)

	data, err := ExtractManifest(path)
	if err != nil {
		t.Fatalf("ExtractManifest V3 failed: %v", err)
	}

	var mf types.Manifest
	if err := json.Unmarshal(data, &mf); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if len(mf.Fragments) == 0 {
		t.Error("expected at least 1 fragment")
	}
	if len(mf.KVI) == 0 {
		t.Error("KVI is empty")
	}
}

func TestExtractManifest_V4(t *testing.T) {
	path := createMinimalV4Fixture(t, 4096)

	data, err := ExtractManifest(path)
	if err != nil {
		t.Fatalf("ExtractManifest V4 failed: %v", err)
	}

	var mf types.Manifest
	if err := json.Unmarshal(data, &mf); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
	if len(mf.KVI) == 0 {
		t.Error("KVI is empty")
	}
}

func TestExtractManifest_V4_DeobfuscationCorrectness(t *testing.T) {
	path := createMinimalV4Fixture(t, 2048)

	data, err := ExtractManifest(path)
	if err != nil {
		t.Fatalf("ExtractManifest V4 failed: %v", err)
	}

	var mf types.Manifest
	if err := json.Unmarshal(data, &mf); err != nil {
		t.Fatalf("invalid manifest JSON: %v", err)
	}
	if len(mf.KVI) == 0 {
		t.Errorf("KVI should not be empty after deobfuscation")
	}

	var kvi map[string]string
	if err := json.Unmarshal(mf.KVI, &kvi); err != nil {
		t.Fatalf("KVI is not valid JSON: %v", err)
	}
	if kvi["salt_base64"] == "" {
		t.Error("salt_base64 should not be empty")
	}
}

func TestExtractManifest_NotAContainer(t *testing.T) {
	tmpFile := t.TempDir() + "/plain.txt"
	if err := os.WriteFile(tmpFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ExtractManifest(tmpFile)
	if err == nil {
		t.Fatal("expected error for non-container file, got nil")
	}
}

func TestExtractKVI_V3(t *testing.T) {
	path := createMinimalV3Fixture(t, 4096)

	data, err := ExtractKVI(path)
	if err != nil {
		t.Fatalf("ExtractKVI V3 failed: %v", err)
	}

	var kvi map[string]string
	if err := json.Unmarshal(data, &kvi); err != nil {
		t.Fatalf("KVI data is not valid JSON: %v", err)
	}
	if kvi["salt_base64"] == "" {
		t.Error("salt_base64 is empty")
	}
}

func TestExtractKVI_V4(t *testing.T) {
	path := createMinimalV4Fixture(t, 4096)

	data, err := ExtractKVI(path)
	if err != nil {
		t.Fatalf("ExtractKVI V4 failed: %v", err)
	}

	var kvi map[string]string
	if err := json.Unmarshal(data, &kvi); err != nil {
		t.Fatalf("KVI data is not valid JSON: %v", err)
	}
	if kvi["salt_base64"] == "" {
		t.Error("salt_base64 is empty")
	}
}

func TestReadManifestFromFile_V4(t *testing.T) {
	path := createMinimalV4Fixture(t, 4096)

	mf, footer, version, headerSize, err := ReadManifestFromFile(path)
	if err != nil {
		t.Fatalf("ReadManifestFromFile V4 failed: %v", err)
	}
	if version != 4 {
		t.Errorf("expected version 4, got %d", version)
	}
	if headerSize != types.EnvelopeHeaderSize_v4 {
		t.Errorf("expected headerSize %d, got %d", types.EnvelopeHeaderSize_v4, headerSize)
	}
	if mf == nil {
		t.Fatal("manifest is nil")
	}
	if len(mf.KVI) == 0 {
		t.Error("KVI is empty")
	}
	if footer != nil {
		t.Error("V4 container should have nil footer (no EnvelopeFooter_v2)")
	}
}

func TestScanManifestFromFile_V4(t *testing.T) {
	path := createMinimalV4Fixture(t, 4096)

	mf, version, _, err := ScanManifestFromFile(path)
	if err != nil {
		t.Fatalf("ScanManifestFromFile V4 failed: %v", err)
	}
	if version != 4 {
		t.Errorf("expected version 4, got %d", version)
	}
	if mf == nil {
		t.Fatal("manifest is nil")
	}
	if len(mf.KVI) == 0 {
		t.Error("KVI is empty")
	}
}

func TestDeserializeFromJSON_RoundTrip(t *testing.T) {
	original := &types.Manifest{
		Kind: "text",
		KVI:  json.RawMessage(`{"salt_base64":"abc123","iv_base64":"def456"}`),
		Fragments: []types.Fragment{
			{ID: "frag_0", Type: types.FragmentType_AtomicFile, Length: 100},
		},
	}
	jsonBytes, _ := json.Marshal(original)

	restore, err := DeserializeFromJSON(jsonBytes)
	if err != nil {
		t.Fatalf("DeserializeFromJSON failed: %v", err)
	}
	if restore.Kind != original.Kind {
		t.Errorf("Kind mismatch: got %q, want %q", restore.Kind, original.Kind)
	}
	if len(restore.Fragments) != len(original.Fragments) {
		t.Errorf("Fragments count mismatch: got %d, want %d", len(restore.Fragments), len(original.Fragments))
	}
}
