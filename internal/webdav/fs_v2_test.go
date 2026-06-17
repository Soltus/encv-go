package webdav

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/container/manifest"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/pluginsext"
	"github.com/Soltus/encv-go/internal/v2/types"
)

func createV4ContainerForWebDAV(t testing.TB) string {
	t.Helper()
	tempDir := t.TempDir()
	containerPath := filepath.Join(tempDir, "test_v4"+pluginsext.VideoExt)

	salt, _ := crypto.GenerateSalt_v2(types.SaltSize_v2)
	key := crypto.GenerateKey("test-password", salt, types.KeySize_v2)
	macKey := crypto.DeriveMACKey("test-password", bytes.Repeat([]byte{0xAB}, crypto.MACSaltLength))

	origData := make([]byte, 2048)
	for i := range origData {
		origData[i] = byte(i)
	}

	encResult, _ := crypto.EncryptSegment(origData, key, macKey, 0, crypto.CompressionModeNone)
	encResult.SegmentID = 0

	kviRaw := map[string]string{
		"salt_base64": crypto.Base64Encode_v2(salt),
		"iv_base64":   crypto.Base64Encode_v2(make([]byte, crypto.IVSize_v2)),
	}
	kviBytes, _ := json.Marshal(kviRaw)

	v4Manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "webdav-test-v4",
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

func createMinimalWebDAVFS(dir string) *encvWebDAVFS {
	return &encvWebDAVFS{
		dir: dir,
		cfg: &config.Config{
			Password: "test-password",
			Webdav: types.WebdavServer{
				Dir: dir,
			},
		},
		indexes: &pathIndexes{
			pathMap:        make(map[string]string),
			dirMap:         make(map[string][]string),
			fileInfoMap:    make(map[string]os.FileInfo),
			reversePathMap: make(map[string]string),
		},
		indexReady:          make(chan struct{}),
		containerExtensions: make(map[string]bool),
		excludeDirs: map[string]bool{
			"node_modules": true,
			".git":         true,
		},
	}
}

func TestValidateContainerHeader_V4(t *testing.T) {
	v4Path := createV4ContainerForWebDAV(t)
	fs := createMinimalWebDAVFS(filepath.Dir(v4Path))

	if !fs.validateContainerHeader(v4Path) {
		t.Error("validateContainerHeader should return true for V4 container")
	}
}

func TestValidateContainerHeader_NotAContainer(t *testing.T) {
	tmpFile := t.TempDir() + "/plain.txt"
	os.WriteFile(tmpFile, []byte("hello"), 0644)

	fs := createMinimalWebDAVFS(t.TempDir())
	if fs.validateContainerHeader(tmpFile) {
		t.Error("validateContainerHeader should return false for plain file")
	}
}

func TestGetIndexFromContainerPath_V4(t *testing.T) {
	v4Path := createV4ContainerForWebDAV(t)
	fs := createMinimalWebDAVFS(filepath.Dir(v4Path))

	idx, err := fs.getIndexFromContainerPath(v4Path)
	if err != nil {
		t.Fatalf("getIndexFromContainerPath V4 failed: %v", err)
	}
	if idx == nil {
		t.Fatal("returned index should not be nil")
	}
}

func TestExtractManifestIntegration_V4_ThroughWebDAVPath(t *testing.T) {
	v4Path := createV4ContainerForWebDAV(t)

	data, err := manifest.ExtractManifest(v4Path)
	if err != nil {
		t.Fatalf("ExtractManifest failed: %v", err)
	}

	mf, err := manifest.DeserializeFromJSON(data)
	if err != nil {
		t.Fatalf("DeserializeFromJSON failed: %v", err)
	}
	if len(mf.KVI) == 0 {
		t.Error("KVI should not be empty")
	}
	if mf.Kind == "" {
		t.Errorf("Kind should be set after V4→V2 adaptation, got empty")
	}

	provider, err := types.NewKVIProviderFromManifest(mf)
	if err != nil {
		t.Fatalf("NewKVIProviderFromManifest failed: %v", err)
	}
	if provider == nil {
		t.Fatal("provider should not be nil")
	}
	index := provider.GetIndex()
	if index == nil {
		t.Fatal("index should not be nil")
	}
}
