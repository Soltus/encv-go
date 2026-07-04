package detector

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/pluginsext"
	"github.com/Soltus/encv-go/internal/v2/types"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

func buildV4Data(t *testing.T) []byte {
	t.Helper()
	footer := types.EnvelopeFooterV4{Magic: types.MagicFooter_v2}
	footerBuf := &bytes.Buffer{}
	if err := binary.Write(footerBuf, binary.LittleEndian, &footer); err != nil {
		t.Fatalf("failed to write v4 footer: %v", err)
	}
	data := make([]byte, 6+footerBuf.Len())
	copy(data[0:4], types.MagicHeader_v2[:])
	binary.LittleEndian.PutUint16(data[4:6], 0x0004)
	copy(data[6:], footerBuf.Bytes())
	return data
}

func buildV2V3Data(t *testing.T, version uint16) []byte {
	t.Helper()
	footer := types.EnvelopeFooter_v2{Magic: types.MagicFooter_v2}
	footerBuf := &bytes.Buffer{}
	if err := binary.Write(footerBuf, binary.LittleEndian, &footer); err != nil {
		t.Fatalf("failed to write v2 footer: %v", err)
	}
	data := make([]byte, 6+footerBuf.Len())
	copy(data[0:4], types.MagicHeader_v2[:])
	binary.LittleEndian.PutUint16(data[4:6], version)
	copy(data[6:], footerBuf.Bytes())
	return data
}

func TestIsEncvContainerFromBytes_V4Magic(t *testing.T) {
	data := buildV4Data(t)

	ok, err := IsEncvContainerFromBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true for V4 magic, got false")
	}
}

func TestIsEncvContainerFromBytes_V3Magic(t *testing.T) {
	data := buildV2V3Data(t, 0x0003)

	ok, err := IsEncvContainerFromBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true for V3 magic, got false")
	}
}

func TestIsEncvContainerFromBytes_V2Magic(t *testing.T) {
	data := buildV2V3Data(t, 0x0002)

	ok, err := IsEncvContainerFromBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected true for V2 magic, got false")
	}
}

func TestIsEncvContainerFromBytes_BadMagic(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"NOTENC", []byte("NOTENC")},
		{"RandomBytes", []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x00, 0x01}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := IsEncvContainerFromBytes(tc.data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok {
				t.Fatalf("expected false for bad magic %q, got true", string(tc.data))
			}
		})
	}
}

func TestIsEncvContainerFromBytes_ShortData(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"Empty", []byte{}},
		{"OneByte", []byte{0x45}},
		{"ThreeBytes", []byte("ENC")},
		{"FiveBytes", []byte("XXXX\x00")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := IsEncvContainerFromBytes(tc.data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok {
				t.Fatal("expected false for short data, got true")
			}
		})
	}
}

func TestIsEncvContainerFromBytes_Nil(t *testing.T) {
	ok, err := IsEncvContainerFromBytes(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected false for nil input, got false")
	}
}

func TestDetectContainerType_NonExistent(t *testing.T) {
	ct, err := DetectContainerType("/tmp/nonexistent_encv_file_xyz" + pluginsext.VideoExt)
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
	if ct != types.ContainerTypeUnknown {
		t.Fatalf("expected ContainerTypeUnknown (%d), got %d", types.ContainerTypeUnknown, ct)
	}
}

func TestDetectContainerType_NotAContainer(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/garbage.bin"

	if err := os.WriteFile(path, []byte("this is not an ENCV container at all"), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	ct, err := DetectContainerType(path)
	if err == nil {
		t.Fatal("expected error for non-container file, got nil")
	}
	if ct != types.ContainerTypeUnknown {
		t.Fatalf("expected ContainerTypeUnknown (%d), got %d", types.ContainerTypeUnknown, ct)
	}
}

// =============================================================================
// Phase 5 / Task 10: detector 魔数识别边界测试套件
//
// 核心目的：
//   验证 detector 100% 基于魔数（"ENCV" + version）识别容器，
//   与文件扩展名（包括无扩展名、`.sccgv`、`.zip`、隐藏文件等）完全解耦。
//
// 本套件不修改 detector.go 任何行为，仅为现有能力补齐边界覆盖。
// =============================================================================

// buildFullV4Container 构造一个完整、可被 detector 解析的 v4 容器字节流。
// 与 handle_test.go 中 TestOpen_V4_ValidContainer 构造方式一致。
func buildFullV4Container(t *testing.T) []byte {
	t.Helper()

	v4Manifest := &types.Manifest_v4{
		Version:          4,
		ContainerID:      "detector-boundary-test",
		ContainerType:    "video",
		IsSeekable:       true,
		OriginalDuration: 12.5,
		Segments: []types.Segment_v4{
			{ID: "seg-0", Offset: 2060, Size: 100, StartTime: 0.0, Duration: 5.0, Nonce: ""},
			{ID: "seg-1", Offset: 2160, Size: 200, StartTime: 5.0, Duration: 7.5, Nonce: ""},
		},
		KVI:       []byte(`{"salt_base64":"AA==","iv_base64":"BB=="}`),
		Playlists: map[string][]string{"default": {"seg-0", "seg-1"}},
	}

	manifestJSON, err := v4Manifest.SerializeToJSON_v4()
	if err != nil {
		t.Fatalf("failed to serialize v4 manifest: %v", err)
	}

	obfuscatedManifest, err := crypto.ObfuscateManifest(manifestJSON)
	if err != nil {
		t.Fatalf("failed to obfuscate manifest: %v", err)
	}

	headerSize := types.EnvelopeHeaderSize_v4
	footerSize := types.EnvelopeFooterSize_v4
	manifestOffset := uint32(headerSize)
	manifestLength := uint32(len(obfuscatedManifest))

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

	totalSize := headerSize + len(obfuscatedManifest) + footerSize
	containerData := make([]byte, totalSize)
	copy(containerData[:headerSize], headerBuf.Bytes())
	copy(containerData[headerSize:headerSize+len(obfuscatedManifest)], obfuscatedManifest)
	copy(containerData[totalSize-footerSize:], footerBuf.Bytes())
	return containerData
}

// TestDetect_StrippedSuffix_Plain
// 验证 IsEncvContainerFromBytes 不依赖文件路径/扩展名，仅看字节内容。
func TestDetect_StrippedSuffix_Plain(t *testing.T) {
	containerData := buildFullV4Container(t)
	if len(containerData) < types.EnvelopeHeaderSize_v4 {
		t.Fatalf("test fixture too small: %d bytes", len(containerData))
	}

	// ① bytes.NewReader 包装：用 reader 模拟任意"裸字节流"
	reader := bytes.NewReader(containerData)
	buf := make([]byte, len(containerData))
	if _, err := reader.Read(buf); err != nil {
		t.Fatalf("failed to read from bytes.Reader: %v", err)
	}

	ok, err := IsEncvContainerFromBytes(buf)
	if err != nil {
		t.Fatalf("unexpected error from IsEncvContainerFromBytes: %v", err)
	}
	if !ok {
		t.Fatal("expected IsEncvContainerFromBytes to return true for valid v4 bytes, got false")
	}

	// ② 写入无扩展名文件，文件路径探测器也应识别
	dir := t.TempDir()
	path := dir + "/mydocument"
	if err := os.WriteFile(path, containerData, 0644); err != nil {
		t.Fatalf("failed to write plain-named file: %v", err)
	}

	ct, err := DetectContainerType(path)
	if err != nil {
		t.Fatalf("DetectContainerType failed for plain-named file: %v", err)
	}
	if ct != types.ContainerTypeVideo {
		t.Fatalf("expected ContainerType=Video(%d), got %d", types.ContainerTypeVideo, ct)
	}
}

// TestDetect_StrippedSuffix_Dotfile
// 验证隐藏文件（以 `.` 开头的文件名）同样被 detector 识别。
// detector 不应因为"点文件"语义而拒绝。
func TestDetect_StrippedSuffix_Dotfile(t *testing.T) {
	containerData := buildFullV4Container(t)

	dir := t.TempDir()
	path := dir + "/.sccgv" // 隐藏文件，扩展名形如 .sccgv
	if err := os.WriteFile(path, containerData, 0644); err != nil {
		t.Fatalf("failed to write dotfile: %v", err)
	}

	ct, err := DetectContainerType(path)
	if err != nil {
		t.Fatalf("DetectContainerType failed for dotfile: %v", err)
	}
	if ct != types.ContainerTypeVideo {
		t.Fatalf("expected ContainerType=Video(%d), got %d", types.ContainerTypeVideo, ct)
	}

	// 同时验证字节级 API
	ok, err := IsEncvContainerFromBytes(containerData)
	if err != nil || !ok {
		t.Fatalf("IsEncvContainerFromBytes returned ok=%v err=%v, want true/nil", ok, err)
	}
}

// TestDetect_StrippedSuffix_WrongExtension
// 验证 detector 与扩展名完全解耦：
//   - mydocument.zip 包含 ENCV 头 → 仍应识别为 ENCV 容器
//   - mydocument.zip 包含 ZIP 头 → 返回 false
func TestDetect_StrippedSuffix_WrongExtension(t *testing.T) {
	dir := t.TempDir()

	t.Run("encv_bytes_with_zip_name", func(t *testing.T) {
		containerData := buildFullV4Container(t)
		path := dir + "/mydocument.zip"
		if err := os.WriteFile(path, containerData, 0644); err != nil {
			t.Fatalf("failed to write fake-zip: %v", err)
		}

		ok, err := IsEncvContainerFromBytes(containerData)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected IsEncvContainerFromBytes to recognize ENCV magic in a .zip-named file")
		}

		ct, derr := DetectContainerType(path)
		if derr != nil {
			t.Fatalf("DetectContainerType failed: %v", derr)
		}
		if ct == types.ContainerTypeUnknown {
			t.Fatal("expected detector to identify ENCV magic regardless of .zip extension")
		}
	})

	t.Run("real_zip_bytes_with_zip_name", func(t *testing.T) {
		// 构造最小 ZIP 头 + 一些填充
		zipData := append([]byte{'P', 'K', 0x03, 0x04}, bytes.Repeat([]byte{0x00}, 64)...)
		path := dir + "/realarchive.zip"
		if err := os.WriteFile(path, zipData, 0644); err != nil {
			t.Fatalf("failed to write real zip: %v", err)
		}

		ok, err := IsEncvContainerFromBytes(zipData)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatal("expected IsEncvContainerFromBytes to return false for real ZIP bytes")
		}

		ct, derr := DetectContainerType(path)
		if derr == nil {
			t.Fatalf("expected DetectContainerType to return error for non-ENCV file, got nil (ct=%d)", ct)
		}
		if ct != types.ContainerTypeUnknown {
			t.Fatalf("expected ContainerTypeUnknown for non-ENCV file, got %d", ct)
		}
		if !strings.Contains(derr.Error(), "not an ENCV container") {
			t.Fatalf("expected error to mention 'not an ENCV container', got: %s", derr.Error())
		}
	})
}

// TestDetect_StrippedSuffix_Boundary_Magic
// 验证最小可识别的"魔数+版本"组合。
//   DetectHeaderVersion 只需 6 字节（4B magic + 2B version）。
//   IsEncvContainerFromBytes 在 v4 路径下还需要 12B footer，因此 6 字节会返回 false
//   （这是当前 detector 的真实行为，测试负责锁定此行为以防回归）。
func TestDetect_StrippedSuffix_Boundary_Magic(t *testing.T) {
	data := make([]byte, 6)
	copy(data[0:4], types.MagicHeader_v2[:])
	binary.LittleEndian.PutUint16(data[4:6], 0x0004)

	// 锁定当前 detector 行为：6 字节 v4 魔数 + 版本被识别为"不是 ENCV 容器"
	// 因为 v4 footer（12B）缺失。
	ok, err := IsEncvContainerFromBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected IsEncvContainerFromBytes to return false for 6-byte v4 magic (no footer), got true")
	}

	// 锁定底层版本检测行为：DetectHeaderVersion 对这 6 字节应返回 4
	ver := types.DetectHeaderVersion(data)
	if ver != 4 {
		t.Fatalf("expected DetectHeaderVersion=4 for v4 magic, got %d", ver)
	}
}

// TestDetect_StrippedSuffix_Boundary_HeaderMinus1
// 验证差 1 字节完整 Header（2047 字节）的边界。
//   - IsEncvContainerFromBytes 不抛 panic，返回 false（footer 区域是零值，魔数不匹配）
//   - DetectV4Header 返回"header truncated"语义错误
func TestDetect_StrippedSuffix_Boundary_HeaderMinus1(t *testing.T) {
	const size = types.EnvelopeHeaderSize_v4 - 1 // 2047 字节
	data := make([]byte, size)
	copy(data[0:4], types.MagicHeader_v2[:])
	binary.LittleEndian.PutUint16(data[4:6], 0x0004)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("IsEncvContainerFromBytes panicked: %v", r)
		}
	}()
	ok, err := IsEncvContainerFromBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 2047 字节大于 footerSize(12)，会尝试读最后 12 字节作为 footer。
	// 那 12 字节是零值，Magic 不匹配 → 返回 false
	if ok {
		t.Fatal("expected IsEncvContainerFromBytes to return false for header-minus-1 data, got true")
	}

	// 文件路径版本：DetectV4Header 需要 2048 字节才能完成 io.ReadFull
	dir := t.TempDir()
	path := dir + "/truncated-header.bin"
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write truncated file: %v", err)
	}
	hdr, derr := DetectV4Header(path)
	if derr == nil {
		t.Fatalf("expected DetectV4Header to return error for 2047-byte file, got nil (hdr=%+v)", hdr)
	}
	if hdr != nil {
		t.Fatal("expected nil header on error")
	}
	// 错误链应能体现"header truncated"语义（io.ErrUnexpectedEOF 被 ReadHeaderV4 包装）
	if !strings.Contains(strings.ToLower(derr.Error()), "header") {
		t.Fatalf("expected error to mention 'header', got: %s", derr.Error())
	}
}

// TestDetect_StrippedSuffix_TruncatedAt5Bytes
// 验证 < 6 字节的极短输入：必须返回 false 且不抛 panic。
func TestDetect_StrippedSuffix_TruncatedAt5Bytes(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"five_bytes_encv_plus_one", append([]byte(types.MagicHeader_v2[:]), 0x04)},
		{"five_bytes_random", []byte{0x00, 0x01, 0x02, 0x03, 0x04}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("IsEncvContainerFromBytes panicked: %v", r)
				}
			}()
			ok, err := IsEncvContainerFromBytes(tc.data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ok {
				t.Fatalf("expected false for %d-byte data, got true", len(tc.data))
			}
		})
	}
}

// TestDetect_StrippedSuffix_NonENCVMagic
// 验证常见文件格式的魔数均不会被误判为 ENCV。
func TestDetect_StrippedSuffix_NonENCVMagic(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"zip_local_file_header", append([]byte{'P', 'K', 0x03, 0x04}, bytes.Repeat([]byte{0x00}, 28)...)},
		{"png_signature", append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, bytes.Repeat([]byte{0x00}, 24)...)},
		{"mp4_ftyp_box", append([]byte{0x00, 0x00, 0x00, 0x20, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}, bytes.Repeat([]byte{0x00}, 20)...)},
		{"gzip_magic", append([]byte{0x1F, 0x8B, 0x08, 0x00}, bytes.Repeat([]byte{0x00}, 28)...)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := IsEncvContainerFromBytes(tc.data)
			if err != nil {
				t.Fatalf("unexpected error for %s: %v", tc.name, err)
			}
			if ok {
				t.Fatalf("expected IsEncvContainerFromBytes=false for %s magic, got true", tc.name)
			}

			// 写文件并验证 DetectContainerType 给出明确错误
			dir := t.TempDir()
			path := dir + "/" + tc.name + ".bin"
			if err := os.WriteFile(path, tc.data, 0644); err != nil {
				t.Fatalf("failed to write %s: %v", tc.name, err)
			}
			ct, derr := DetectContainerType(path)
			if derr == nil {
				t.Fatalf("expected DetectContainerType error for %s, got nil (ct=%d)", tc.name, ct)
			}
			if ct != types.ContainerTypeUnknown {
				t.Fatalf("expected ContainerTypeUnknown for %s, got %d", tc.name, ct)
			}
			if !strings.Contains(derr.Error(), "not an ENCV container") {
				t.Fatalf("expected error to mention 'not an ENCV container' for %s, got: %s", tc.name, derr.Error())
			}
		})
	}
}

// TestDetect_StrippedSuffix_EmptyFile
// 验证 0 字节空文件的边界行为。
func TestDetect_StrippedSuffix_EmptyFile(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("IsEncvContainerFromBytes panicked on empty input: %v", r)
		}
	}()
	ok, err := IsEncvContainerFromBytes([]byte{})
	if err != nil {
		t.Fatalf("unexpected error for empty input: %v", err)
	}
	if ok {
		t.Fatal("expected IsEncvContainerFromBytes=false for empty input, got true")
	}

	dir := t.TempDir()
	path := dir + "/empty.bin"
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("failed to write empty file: %v", err)
	}
	ct, derr := DetectContainerType(path)
	if derr == nil {
		t.Fatalf("expected DetectContainerType error for empty file, got nil (ct=%d)", ct)
	}
	if ct != types.ContainerTypeUnknown {
		t.Fatalf("expected ContainerTypeUnknown for empty file, got %d", ct)
	}
}

// TestDetect_StrippedSuffix_ValidV4_HeaderRead
// 验证 detector 对无扩展名的完整 v4 容器能完整解析 Header 字段。
func TestDetect_StrippedSuffix_ValidV4_HeaderRead(t *testing.T) {
	containerData := buildFullV4Container(t)
	if len(containerData) < types.EnvelopeHeaderSize_v4 {
		t.Fatalf("test fixture too small: %d bytes", len(containerData))
	}

	dir := t.TempDir()
	// 关键：文件名无任何扩展名
	path := dir + "/mydocument"
	if err := os.WriteFile(path, containerData, 0644); err != nil {
		t.Fatalf("failed to write container: %v", err)
	}

	hdr, err := DetectV4Header(path)
	if err != nil {
		t.Fatalf("DetectV4Header failed for extensionless v4 container: %v", err)
	}
	if hdr == nil {
		t.Fatal("expected non-nil v4 header")
	}

	if hdr.Magic != types.MagicHeader_v2 {
		t.Errorf("header magic mismatch: got %v, want %v", hdr.Magic, types.MagicHeader_v2)
	}
	if hdr.Version != 0x04 {
		t.Errorf("header Version=%d, want 4", hdr.Version)
	}
	if hdr.ContainerType != types.ContainerTypeVideo {
		t.Errorf("header ContainerType=%d, want %d (Video)", hdr.ContainerType, types.ContainerTypeVideo)
	}
	if hdr.IsSeekable != 1 {
		t.Errorf("header IsSeekable=%d, want 1", hdr.IsSeekable)
	}
	if hdr.Flags&types.FlagIsMainContainer == 0 {
		t.Errorf("expected FlagIsMainContainer to be set, got flags=%#x", hdr.Flags)
	}
	if hdr.ManifestOffset != types.EnvelopeHeaderSize_v4 {
		t.Errorf("ManifestOffset=%d, want %d", hdr.ManifestOffset, types.EnvelopeHeaderSize_v4)
	}
	if hdr.ManifestLength == 0 {
		t.Error("ManifestLength should be non-zero")
	}

	// 关联断言：DetectContainerType 与 IsSeekable 也工作
	ct, err := DetectContainerType(path)
	if err != nil {
		t.Fatalf("DetectContainerType failed: %v", err)
	}
	if ct != types.ContainerTypeVideo {
		t.Errorf("DetectContainerType=%d, want %d (Video)", ct, types.ContainerTypeVideo)
	}

	seekable, err := DetectIsSeekable(path)
	if err != nil {
		t.Fatalf("DetectIsSeekable failed: %v", err)
	}
	if !seekable {
		t.Error("expected DetectIsSeekable=true for seekable v4 container")
	}
}
