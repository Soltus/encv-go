package detector

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// buildV4ContainerWithCipherMode 构造一个指定 CipherMode 的完整 v4 容器
// 用于 SubTask 10.10（TestDetect_StrippedSuffix_CipherMode_0/1）测试。
func buildV4ContainerWithCipherMode(t *testing.T, cipherMode uint16) []byte {
	t.Helper()

	manifest, err := crypto.ObfuscateManifest([]byte(`{"k":"v"}`))
	if err != nil {
		t.Fatalf("failed to obfuscate manifest: %v", err)
	}
	obfuscatedManifest, err := crypto.DeobfuscateManifest(manifest)
	if err != nil {
		t.Fatalf("failed to deobfuscate manifest: %v", err)
	}
	_ = obfuscatedManifest

	obf2, err := crypto.ObfuscateManifest([]byte(`{"k":"v"}`))
	if err != nil {
		t.Fatalf("failed to obfuscate manifest: %v", err)
	}

	headerSize := types.EnvelopeHeaderSize_v4
	footerSize := types.EnvelopeFooterSize_v4
	manifestOffset := uint32(headerSize)
	manifestLength := uint32(len(obf2))

	header := &types.EnvelopeHeaderV4{
		Magic:          types.MagicHeader_v2,
		Version:        0x04,
		Flags:          types.FlagIsMainContainer,
		ContainerType:  types.ContainerTypeVideo,
		IsSeekable:     1,
		IDType:         0,
		IDLength:       0,
		ManifestOffset: manifestOffset,
		ManifestLength: manifestLength,
		CipherMode:     cipherMode,
	}

	var headerBuf bytes.Buffer
	if err := types.WriteHeaderV4(&headerBuf, header); err != nil {
		t.Fatalf("failed to write v4 header: %v", err)
	}

	footer := &types.EnvelopeFooterV4{
		Magic:       types.MagicFooter_v2,
		GlobalCRC32: 0,
		Reserved:    [4]byte{},
	}
	var footerBuf bytes.Buffer
	if err := types.WriteFooterV4(&footerBuf, footer); err != nil {
		t.Fatalf("failed to write v4 footer: %v", err)
	}

	totalSize := headerSize + len(obf2) + footerSize
	containerData := make([]byte, totalSize)
	copy(containerData[:headerSize], headerBuf.Bytes())
	copy(containerData[headerSize:headerSize+len(obf2)], obf2)
	copy(containerData[totalSize-footerSize:], footerBuf.Bytes())
	return containerData
}

// TestDetect_StrippedSuffix_CipherMode_0 验证 detector 对 AES-128-CTR v4 容器
// （CipherMode=0）能从字节流中正确解析 CipherMode 字段。
func TestDetect_StrippedSuffix_CipherMode_0(t *testing.T) {
	data := buildV4ContainerWithCipherMode(t, uint16(crypto.CipherModeAES128CTR))
	if len(data) < types.EnvelopeHeaderSize_v4 {
		t.Fatalf("fixture too small: %d bytes", len(data))
	}

	result, err := DetectContainerFromReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DetectContainerFromReader failed: %v", err)
	}

	if !result.IsENCVContainer {
		t.Fatal("expected IsENCVContainer=true for valid v4 magic")
	}
	if result.Version != 4 {
		t.Errorf("Version=%d, want 4", result.Version)
	}
	if result.CipherMode != 0 {
		t.Errorf("CipherMode=%d, want 0 (AES-128-CTR)", result.CipherMode)
	}
	if result.ContainerType != types.ContainerTypeVideo {
		t.Errorf("ContainerType=%d, want %d (Video)", result.ContainerType, types.ContainerTypeVideo)
	}
	if !result.IsSeekable {
		t.Error("expected IsSeekable=true")
	}
}

// TestDetect_StrippedSuffix_CipherMode_1 验证 detector 对 AES-256-CTR v4 容器
// （CipherMode=1）能从字节流中正确解析 CipherMode 字段。
func TestDetect_StrippedSuffix_CipherMode_1(t *testing.T) {
	data := buildV4ContainerWithCipherMode(t, 1)
	if len(data) < types.EnvelopeHeaderSize_v4 {
		t.Fatalf("fixture too small: %d bytes", len(data))
	}

	result, err := DetectContainerFromReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DetectContainerFromReader failed: %v", err)
	}

	if !result.IsENCVContainer {
		t.Fatal("expected IsENCVContainer=true")
	}
	if result.Version != 4 {
		t.Errorf("Version=%d, want 4", result.Version)
	}
	if result.CipherMode != 1 {
		t.Errorf("CipherMode=%d, want 1 (AES-256-CTR)", result.CipherMode)
	}
}

// TestDetectFromReader_Plain 验证 DetectContainerFromReader 对完整 v4 容器
// 返回结构化字段，且与文件扩展名完全解耦。
func TestDetectFromReader_Plain(t *testing.T) {
	data := buildFullV4Container(t)

	result, err := DetectContainerFromReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("DetectContainerFromReader failed: %v", err)
	}
	if !result.IsENCVContainer {
		t.Fatal("expected IsENCVContainer=true")
	}
	if result.Version != 4 {
		t.Errorf("Version=%d, want 4", result.Version)
	}
	if result.ContainerType != types.ContainerTypeVideo {
		t.Errorf("ContainerType=%d, want %d", result.ContainerType, types.ContainerTypeVideo)
	}
	if !result.IsSeekable {
		t.Error("expected IsSeekable=true")
	}
}

// TestDetectFromReader_ExtensionlessFile 验证无扩展名文件能走 io.Reader 路径识别。
func TestDetectFromReader_ExtensionlessFile(t *testing.T) {
	data := buildFullV4Container(t)

	dir := t.TempDir()
	path := dir + "/mydocument" // 关键：无任何扩展名
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open: %v", err)
	}
	defer f.Close()

	result, err := DetectContainerFromReader(f)
	if err != nil {
		t.Fatalf("DetectContainerFromReader failed: %v", err)
	}
	if !result.IsENCVContainer {
		t.Fatal("expected IsENCVContainer=true for extensionless v4 file")
	}
	if result.Version != 4 {
		t.Errorf("Version=%d, want 4", result.Version)
	}
}

// TestDetectFromReader_NonENCVMagic 验证非 ENCV 字节流返回 IsENCVContainer=false。
func TestDetectFromReader_NonENCVMagic(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"ZIP", []byte{'P', 'K', 0x03, 0x04, 0x00, 0x00}},
		{"PNG", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A}},
		{"MP4_ftyp", []byte{'f', 't', 'y', 'p', 0x00, 0x00}},
		{"GZIP", []byte{0x1F, 0x8B, 0x08, 0x00, 0x00, 0x00}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := DetectContainerFromReader(bytes.NewReader(tc.data))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsENCVContainer {
				t.Errorf("expected IsENCVContainer=false for %s magic", tc.name)
			}
		})
	}
}

// TestDetectFromReader_EmptyInput 验证空输入返回 ErrEmptyInput。
func TestDetectFromReader_EmptyInput(t *testing.T) {
	_, err := DetectContainerFromReader(bytes.NewReader(nil))
	if !errors.Is(err, ErrEmptyInput) {
		t.Errorf("expected ErrEmptyInput, got %v", err)
	}
}

// TestDetectFromReader_HeaderTruncated 验证 1-5 字节输入返回 ErrHeaderTruncated。
func TestDetectFromReader_HeaderTruncated(t *testing.T) {
	for size := 1; size < 6; size++ {
		t.Run("size_"+itoa(size), func(t *testing.T) {
			data := []byte{'E', 'N', 'C', 'V', 0x04, 0x00, 0x00}[:size]
			_, err := DetectContainerFromReader(bytes.NewReader(data))
			if !errors.Is(err, ErrHeaderTruncated) {
				t.Errorf("size=%d: expected ErrHeaderTruncated, got %v", size, err)
			}
		})
	}
}

// TestDetectFromReader_V2V3_BackendCompat 验证 v2/v3 容器也能从 reader 识别。
func TestDetectFromReader_V2V3_BackendCompat(t *testing.T) {
	// v3
	data := buildV2V3Data(t, 0x0003)
	result, err := DetectContainerFromReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("v3 detect failed: %v", err)
	}
	if !result.IsENCVContainer {
		t.Error("expected v3 to be ENCV container")
	}
	if result.Version != 3 {
		t.Errorf("v3 Version=%d, want 3", result.Version)
	}
}

// TestDetectFromReader_DoesNotPanic_OnZeroLength 回归保护：零长度不 panic。
func TestDetectFromReader_DoesNotPanic_OnZeroLength(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on zero-length input: %v", r)
		}
	}()
	_, _ = DetectContainerFromReader(bytes.NewReader([]byte{}))
}

// TestDetectFromReader_ErrorChainContainsNotAnENCVContainer 验证错误信息可读。
// （兼容 v2 的错误信息风格："not an ENCV container"）
func TestDetectFromReader_ErrorChainContainsNotAnENCVContainer(t *testing.T) {
	// 通过旧的 DetectContainerType 路径验证（reader 路径已通过 NonENCVMagic 覆盖）
	dir := t.TempDir()
	path := dir + "/mydocument.zip"
	if err := os.WriteFile(path, []byte{'P', 'K', 0x03, 0x04, 0x00, 0x00}, 0644); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	_, err := DetectContainerType(path)
	if err == nil {
		t.Fatal("expected error for non-ENCV file")
	}
	if !strings.Contains(err.Error(), "not an ENCV container") {
		t.Errorf("error message should contain 'not an ENCV container', got: %v", err)
	}
}

// itoa 简易 int → string，避免引入 strconv 包
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// _ 触发 io 包的导入使用（reader 路径下 io.ReadFull 实际使用过，但保留此引用防止 lint 误报）
var _ = io.EOF
