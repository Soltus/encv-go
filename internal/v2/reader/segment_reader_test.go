// internal/v2/reader/segment_reader_test.go
//
// Task 12: segment_reader 集成 MAC 校验前置 + 压缩解压的测试套件。
//
// 覆盖 SubTask 12.4 + 12.5 + 若干 reader 端独立测试用例：
//   - TestReaderV4_DetectTamperedMAC_ReturnsErrMACMismatch
//   - TestReaderV4_DecompressZstd_OnTheFly
//   - TestReaderV4_CompressionMode_Skipped_If_Small
//   - TestReaderV4_MixedSegments_EncryptedAndPlain
//   - TestReaderV4_NoHMAC_BackwardCompat
//   - TestReaderV4_WrongPassword_ReturnsErrMACMismatch
//   - TestReaderV4_CipherMode_AES128
//   - TestReaderV4_CipherMode_AES256
//
// 所有测试通过 writer.WriteV4Container 写出 v4 容器，然后通过 reader
// 路径（NewSegmentSeekableReader / NewSegmentSequentialReader）验证：
//   - 启用 HMAC 时密文 1 bit 篡改必须返回 crypto.ErrMACMismatch
//   - 启用 zstd 时 reader 必须解压并与原文完全一致
//   - 小数据自动跳过压缩时 reader 不触发解压
//   - 混合明文/加密 Segment 正确处理 ModeFlags
//   - EnableHMAC=false 容器（向后兼容）reader 跳过 MAC 校验
//   - 错误密码触发 MAC 失败
//   - AES-128 / AES-256 两种 CipherMode 均能 roundtrip
package reader

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/pluginsext"
	"github.com/Soltus/encv-go/internal/v2/types"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
	"github.com/Soltus/encv-go/internal/v2/writer"
)

// makeTestKVI 生成测试用 KVI 字节（含 salt_base64 / iv_base64）。
func makeTestKVI(salt []byte) json.RawMessage {
	kvi := map[string]string{
		"salt_base64": base64.StdEncoding.EncodeToString(salt),
		"iv_base64":   base64.StdEncoding.EncodeToString(make([]byte, crypto.IVSize_v2)),
	}
	data, _ := json.Marshal(kvi)
	return data
}

// makeTestManifest 构造一个最小 v4 Manifest（仅必备字段）。
func makeTestManifest(salt []byte, segments []types.Segment_v4, playlistIDs []string) *types.Manifest_v4 {
	return &types.Manifest_v4{
		Version:       4,
		ContainerID:   "test-container",
		ContainerType: "video",
		IsSeekable:    true,
		Segments:      segments,
		Playlists:     map[string][]string{"default": playlistIDs},
		KVI:           makeTestKVI(salt),
	}
}

// writeV4ContainerToTemp 写出 v4 容器到临时文件，返回路径。
func writeV4ContainerToTemp(t *testing.T, params *writer.V4WriteParams) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "v4readertest-*"+pluginsext.VideoExt)
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	params.OutputPath = tmpPath
	if err := writer.WriteV4Container(params); err != nil {
		os.Remove(tmpPath)
		t.Fatalf("WriteV4Container: %v", err)
	}
	return tmpPath
}

// flipBitInFile 翻转 v4 容器文件中某 Segment 的 ciphertext 1 bit。
//
// 位置：Segment 起始 + SegmentHeaderSize + NonceSize（密文开头 1 字节）。
// 通过 XOR 0x01 翻转该字节最低位（不影响整体字节数 / 长度）。
func flipBitInFile(t *testing.T, path string, seg types.Segment_v4) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()

	// 密文起始：Header(34) + Nonce(16)
	bitOffset := int64(seg.Offset) + int64(types.SegmentHeaderSize) + int64(len(seg.Nonce))
	if len(seg.Nonce) == 0 {
		// 极端：明文 Segment 没有 nonce，但本函数只用于加密 Segment
		t.Fatalf("flipBitInFile called on plaintext segment '%s'", seg.ID)
	}

	// 解码 nonce 拿到实际字节数
	nonceBytes, err := base64.StdEncoding.DecodeString(seg.Nonce)
	if err != nil {
		t.Fatalf("decode nonce: %v", err)
	}
	bitOffset = int64(seg.Offset) + int64(types.SegmentHeaderSize) + int64(len(nonceBytes))

	// 读取首字节 → XOR 0x01 → 写回
	var b [1]byte
	if _, err := f.ReadAt(b[:], bitOffset); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	b[0] ^= 0x01
	if _, err := f.WriteAt(b[:], bitOffset); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
}

// readAllSequential 通过 SequentialReader 流式读完整个 playlist。
func readAllSequential(t *testing.T, info *V4ContainerInfo) []byte {
	t.Helper()
	r, err := NewSegmentSequentialReader(info, "default")
	if err != nil {
		t.Fatalf("NewSegmentSequentialReader: %v", err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return data
}

// TestReaderV4_DetectTamperedMAC_ReturnsErrMACMismatch
//
// 验证 SubTask 12.4：在磁盘上翻转 ciphertext 1 bit 后，reader 必须返回
// crypto.ErrMACMismatch（不解密、不解压、不泄露明文线索）。
func TestReaderV4_DetectTamperedMAC_ReturnsErrMACMismatch(t *testing.T) {
	const (
		password = "test-tamper-pw"
		segData  = 4 * 1024 // 4KB
	)

	plaintext := make([]byte, segData)
	if _, err := rand.Read(plaintext); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	salt, err := crypto.GenerateSalt_v2(crypto.SaltSize_v2)
	if err != nil {
		t.Fatalf("GenerateSalt_v2: %v", err)
	}
	macSalt, err := crypto.GenerateMACSalt()
	if err != nil {
		t.Fatalf("GenerateMACSalt: %v", err)
	}
	key, macKey := deriveKeysForV4(t, password, salt, macSalt, crypto.KeySize_v4_128)

	encResult, err := crypto.EncryptSegment(plaintext, key, macKey, 0, crypto.CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment: %v", err)
	}

	manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "tamper-test",
		ContainerType: "video",
		IsSeekable:    true,
		Segments: []types.Segment_v4{
			{ID: "seg-0", StartTime: 0, Duration: 10},
		},
		Playlists: map[string][]string{"default": {"seg-0"}},
		KVI:       makeTestKVI(salt),
	}
	manifest.MACSaltBase64 = base64.StdEncoding.EncodeToString(macSalt)

	tmpPath := writeV4ContainerToTemp(t, &writer.V4WriteParams{
		OutputPath:     "",
		IsMain:         true,
		ContainerType:  types.ContainerTypeVideo,
		IsSeekable:     true,
		IDType:         types.IDType_Raw,
		Manifest:       manifest,
		SegmentResults: []*crypto.SegmentEncryptionResult{encResult},
		CipherMode:     0,
		CompressionMode: "none",
		EnableHMAC:     true,
	})
	defer os.Remove(tmpPath)

	// 先验证未篡改时能成功解密（baseline）
	info, err := OpenV4Container(tmpPath, password)
	if err != nil {
		t.Fatalf("OpenV4Container: %v", err)
	}
	dec, err := NewSegmentSequentialReader(info, "default")
	if err != nil {
		t.Fatalf("NewSegmentSequentialReader: %v", err)
	}
	baseline, err := io.ReadAll(dec)
	if err != nil {
		t.Fatalf("baseline ReadAll: %v", err)
	}
	dec.Close()
	if !bytes.Equal(baseline, plaintext) {
		t.Fatalf("baseline roundtrip failed (no tamper yet!)")
	}

	// 篡改 ciphertext 1 bit
	flipBitInFile(t, tmpPath, info.Manifest.Segments[0])

	// 重新打开（不要复用缓存）
	info, err = OpenV4Container(tmpPath, password)
	if err != nil {
		t.Fatalf("OpenV4Container after tamper: %v", err)
	}

	// 验证：reader 必须返回 ErrMACMismatch
	dec, err = NewSegmentSequentialReader(info, "default")
	if err != nil {
		t.Fatalf("NewSegmentSequentialReader after tamper: %v", err)
	}
	defer dec.Close()
	_, readErr := io.ReadAll(dec)
	if readErr == nil {
		t.Fatalf("expected ErrMACMismatch after bit flip, got nil error")
	}
	if !errors.Is(readErr, crypto.ErrMACMismatch) {
		t.Fatalf("expected crypto.ErrMACMismatch, got: %v", readErr)
	}

	// 同样验证 SegmentSeekableReader 路径
	sr, err := NewSegmentSeekableReader(info, "default")
	if err != nil {
		t.Fatalf("NewSegmentSeekableReader after tamper: %v", err)
	}
	defer sr.Close()
	buf := make([]byte, len(plaintext))
	_, readErr = io.ReadFull(sr, buf)
	if readErr == nil {
		t.Fatalf("SegmentSeekableReader expected ErrMACMismatch after bit flip, got nil error")
	}
	if !errors.Is(readErr, crypto.ErrMACMismatch) {
		t.Fatalf("SegmentSeekableReader expected crypto.ErrMACMismatch, got: %v", readErr)
	}
}

// TestReaderV4_DecompressZstd_OnTheFly 验证 SubTask 12.5：
// writer 写入 zstd 压缩的 v4 容器，reader 端能"读时解压"还原成原始明文。
func TestReaderV4_DecompressZstd_OnTheFly(t *testing.T) {
	const (
		password = "test-zstd-pw"
		segData  = 10 * 1024 // 10KB（≥1KB 阈值，触发 zstd 压缩）
	)

	// 构造可压缩数据（高重复度 → 压缩率高）
	plaintext := bytes.Repeat([]byte("abcdefghij"), segData/10)

	salt, err := crypto.GenerateSalt_v2(crypto.SaltSize_v2)
	if err != nil {
		t.Fatalf("GenerateSalt_v2: %v", err)
	}
	macSalt, err := crypto.GenerateMACSalt()
	if err != nil {
		t.Fatalf("GenerateMACSalt: %v", err)
	}
	key, macKey := deriveKeysForV4(t, password, salt, macSalt, crypto.KeySize_v4_128)

	encResult, err := crypto.EncryptSegment(plaintext, key, macKey, 0, crypto.CompressionModeZstd)
	if err != nil {
		t.Fatalf("EncryptSegment(zstd): %v", err)
	}
	if !encResult.Compressed {
		t.Fatalf("expected compressed result, but got non-compressed")
	}
	if len(encResult.SeekTable) == 0 {
		t.Fatalf("expected non-empty seek table for compressed segment")
	}

	manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "zstd-test",
		ContainerType: "video",
		IsSeekable:    true,
		Segments: []types.Segment_v4{
			{ID: "seg-0", StartTime: 0, Duration: 10},
		},
		Playlists: map[string][]string{"default": {"seg-0"}},
		KVI:       makeTestKVI(salt),
	}
	manifest.MACSaltBase64 = base64.StdEncoding.EncodeToString(macSalt)

	tmpPath := writeV4ContainerToTemp(t, &writer.V4WriteParams{
		OutputPath:      "",
		IsMain:          true,
		ContainerType:   types.ContainerTypeVideo,
		IsSeekable:      true,
		IDType:          types.IDType_Raw,
		Manifest:        manifest,
		SegmentResults:  []*crypto.SegmentEncryptionResult{encResult},
		CipherMode:      0,
		CompressionMode: "zstd",
		EnableHMAC:      true,
	})
	defer os.Remove(tmpPath)

	// reader 路径
	info, err := OpenV4Container(tmpPath, password)
	if err != nil {
		t.Fatalf("OpenV4Container: %v", err)
	}

	// 验证：reader 解压后必须等于原始 plaintext
	dec := readAllSequential(t, info)
	if !bytes.Equal(dec, plaintext) {
		t.Errorf("zstd decompress mismatch:\n got_len=%d, want_len=%d\n got=%x...\n want=%x...",
			len(dec), len(plaintext), dec[:32], plaintext[:32])
	}

	// 同样验证 SegmentSeekableReader 路径
	sr, err := NewSegmentSeekableReader(info, "default")
	if err != nil {
		t.Fatalf("NewSegmentSeekableReader: %v", err)
	}
	defer sr.Close()
	buf := make([]byte, len(plaintext))
	n, err := io.ReadFull(sr, buf)
	if err != nil && err != io.EOF {
		t.Fatalf("ReadFull: %v", err)
	}
	if n != len(plaintext) {
		t.Errorf("expected to read %d bytes, got %d", len(plaintext), n)
	}
	if !bytes.Equal(buf, plaintext) {
		t.Errorf("seekable zstd decompress mismatch")
	}
}

// TestReaderV4_CompressionMode_Skipped_If_Small 验证：
// writer 写入 < 1KB 数据时自动跳过 zstd 压缩，reader 也能正确处理（不解压）。
func TestReaderV4_CompressionMode_Skipped_If_Small(t *testing.T) {
	const (
		password = "test-small-pw"
		segData  = 500 // < 1KB 阈值
	)

	plaintext := make([]byte, segData)
	if _, err := rand.Read(plaintext); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	salt, err := crypto.GenerateSalt_v2(crypto.SaltSize_v2)
	if err != nil {
		t.Fatalf("GenerateSalt_v2: %v", err)
	}
	macSalt, err := crypto.GenerateMACSalt()
	if err != nil {
		t.Fatalf("GenerateMACSalt: %v", err)
	}
	key, macKey := deriveKeysForV4(t, password, salt, macSalt, crypto.KeySize_v4_128)

	encResult, err := crypto.EncryptSegment(plaintext, key, macKey, 0, crypto.CompressionModeZstd)
	if err != nil {
		t.Fatalf("EncryptSegment: %v", err)
	}
	if encResult.Compressed {
		t.Fatalf("expected non-compressed result for < 1KB input, got compressed")
	}
	if encResult.ModeFlags&types.ModeFlagCompressionZstd != 0 {
		t.Errorf("ModeFlags should NOT have CompressionZstd bit: 0x%04x", encResult.ModeFlags)
	}

	manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "small-test",
		ContainerType: "video",
		IsSeekable:    true,
		Segments: []types.Segment_v4{
			{ID: "seg-0", StartTime: 0, Duration: 10},
		},
		Playlists: map[string][]string{"default": {"seg-0"}},
		KVI:       makeTestKVI(salt),
	}
	manifest.MACSaltBase64 = base64.StdEncoding.EncodeToString(macSalt)

	tmpPath := writeV4ContainerToTemp(t, &writer.V4WriteParams{
		OutputPath:      "",
		IsMain:          true,
		ContainerType:   types.ContainerTypeVideo,
		IsSeekable:      true,
		IDType:          types.IDType_Raw,
		Manifest:        manifest,
		SegmentResults:  []*crypto.SegmentEncryptionResult{encResult},
		CipherMode:      0,
		CompressionMode: "zstd", // 即使传入 zstd，小数据也自动跳过
		EnableHMAC:      true,
	})
	defer os.Remove(tmpPath)

	info, err := OpenV4Container(tmpPath, password)
	if err != nil {
		t.Fatalf("OpenV4Container: %v", err)
	}
	dec := readAllSequential(t, info)
	if !bytes.Equal(dec, plaintext) {
		t.Errorf("small data roundtrip mismatch")
	}
}

// TestReaderV4_MixedSegments_EncryptedAndPlain 验证：
// 多 Segment 混合（明文 + 加密不压缩 + 加密 zstd）reader 正确处理每种 ModeFlags。
func TestReaderV4_MixedSegments_EncryptedAndPlain(t *testing.T) {
	const password = "test-mixed-pw"

	salt, err := crypto.GenerateSalt_v2(crypto.SaltSize_v2)
	if err != nil {
		t.Fatalf("GenerateSalt_v2: %v", err)
	}
	macSalt, err := crypto.GenerateMACSalt()
	if err != nil {
		t.Fatalf("GenerateMACSalt: %v", err)
	}
	key, macKey := deriveKeysForV4(t, password, salt, macSalt, crypto.KeySize_v4_128)

	// Segment A：明文
	plainA := []byte("PLAINTEXT-SEGMENT-A-DATA")
	segA := &crypto.SegmentEncryptionResult{
		SegmentID:     0,
		EncryptedData: plainA,
		ModeFlags:     types.ModeFlagsPlaintext,
	}

	// Segment B：加密不压缩
	plainB := []byte("encrypted-no-compression-segment-B")
	encB, err := crypto.EncryptSegment(plainB, key, macKey, 1, crypto.CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment B: %v", err)
	}

	// Segment C：加密 + zstd
	plainC := bytes.Repeat([]byte("zstd-compressed-C-payload-"), 80) // 约 2KB
	encC, err := crypto.EncryptSegment(plainC, key, macKey, 2, crypto.CompressionModeZstd)
	if err != nil {
		t.Fatalf("EncryptSegment C: %v", err)
	}

	manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "mixed-test",
		ContainerType: "video",
		IsSeekable:    true,
		Segments: []types.Segment_v4{
			{ID: "seg-a", StartTime: 0, Duration: 10},
			{ID: "seg-b", StartTime: 10, Duration: 10},
			{ID: "seg-c", StartTime: 20, Duration: 10},
		},
		Playlists: map[string][]string{"default": {"seg-a", "seg-b", "seg-c"}},
		KVI:       makeTestKVI(salt),
	}
	manifest.MACSaltBase64 = base64.StdEncoding.EncodeToString(macSalt)

	tmpPath := writeV4ContainerToTemp(t, &writer.V4WriteParams{
		OutputPath:     "",
		IsMain:         true,
		ContainerType:  types.ContainerTypeVideo,
		IsSeekable:     true,
		IDType:         types.IDType_Raw,
		Manifest:       manifest,
		SegmentResults: []*crypto.SegmentEncryptionResult{segA, encB, encC},
		CipherMode:     0,
		EnableHMAC:     true,
	})
	defer os.Remove(tmpPath)

	info, err := OpenV4Container(tmpPath, password)
	if err != nil {
		t.Fatalf("OpenV4Container: %v", err)
	}

	// 通过 SegmentSeekableReader 验证每个 Segment 内容
	sr, err := NewSegmentSeekableReader(info, "default")
	if err != nil {
		t.Fatalf("NewSegmentSeekableReader: %v", err)
	}
	defer sr.Close()

	// Segment A：明文
	bufA := make([]byte, len(plainA))
	n, err := sr.ReadAt(bufA, 0)
	if err != nil && err != io.EOF {
		t.Fatalf("ReadAt segA: %v", err)
	}
	if n != len(plainA) || !bytes.Equal(bufA, plainA) {
		t.Errorf("segA mismatch: got %x, want %x", bufA, plainA)
	}

	// Segment B：加密
	bufB := make([]byte, len(plainB))
	n, err = sr.ReadAt(bufB, int64(len(plainA)))
	if err != nil && err != io.EOF {
		t.Fatalf("ReadAt segB: %v", err)
	}
	if n != len(plainB) || !bytes.Equal(bufB, plainB) {
		t.Errorf("segB mismatch: got %x, want %x", bufB, plainB)
	}

	// Segment C：zstd 压缩
	bufC := make([]byte, len(plainC))
	n, err = sr.ReadAt(bufC, int64(len(plainA)+len(plainB)))
	if err != nil && err != io.EOF {
		t.Fatalf("ReadAt segC: %v", err)
	}
	if n != len(plainC) || !bytes.Equal(bufC, plainC) {
		t.Errorf("segC mismatch (len got=%d, want=%d)", n, len(plainC))
	}
}

// TestReaderV4_NoHMAC_BackwardCompat 验证 EnableHMAC=false 时的向后兼容：
// reader 必须跳过 MAC 校验，仅解密即得 plaintext。
func TestReaderV4_NoHMAC_BackwardCompat(t *testing.T) {
	const (
		password = "test-nohmac-pw"
		segData  = 2 * 1024
	)

	plaintext := make([]byte, segData)
	if _, err := rand.Read(plaintext); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	salt, err := crypto.GenerateSalt_v2(crypto.SaltSize_v2)
	if err != nil {
		t.Fatalf("GenerateSalt_v2: %v", err)
	}
	macSalt, err := crypto.GenerateMACSalt()
	if err != nil {
		t.Fatalf("GenerateMACSalt: %v", err)
	}
	key, macKey := deriveKeysForV4(t, password, salt, macSalt, crypto.KeySize_v4_128)

	encResult, err := crypto.EncryptSegment(plaintext, key, macKey, 0, crypto.CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment: %v", err)
	}

	manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "nohmac-test",
		ContainerType: "video",
		IsSeekable:    true,
		Segments: []types.Segment_v4{
			{ID: "seg-0", StartTime: 0, Duration: 10},
		},
		Playlists: map[string][]string{"default": {"seg-0"}},
		KVI:       makeTestKVI(salt),
	}
	// 即使 writer 注入 mac_salt，EnableHMAC=false 时不会写 MAC 字节
	manifest.MACSaltBase64 = base64.StdEncoding.EncodeToString(macSalt)

	tmpPath := writeV4ContainerToTemp(t, &writer.V4WriteParams{
		OutputPath:     "",
		IsMain:         true,
		ContainerType:  types.ContainerTypeVideo,
		IsSeekable:     true,
		IDType:         types.IDType_Raw,
		Manifest:       manifest,
		SegmentResults: []*crypto.SegmentEncryptionResult{encResult},
		CipherMode:     0,
		EnableHMAC:     false, // 关键：禁用 HMAC
	})
	defer os.Remove(tmpPath)

	// 验证：reader 仍能成功解密（跳过 MAC 校验）
	info, err := OpenV4Container(tmpPath, password)
	if err != nil {
		t.Fatalf("OpenV4Container: %v", err)
	}
	dec := readAllSequential(t, info)
	if !bytes.Equal(dec, plaintext) {
		t.Errorf("NoHMAC backward-compat roundtrip mismatch")
	}
}

// TestReaderV4_WrongPassword_ReturnsErrMACMismatch 验证：
// 错误密码必须返回 ErrMACMismatch（不解密 / 不返回明文）。
func TestReaderV4_WrongPassword_ReturnsErrMACMismatch(t *testing.T) {
	const (
		correctPassword = "correct-pw"
		wrongPassword   = "wrong-pw"
		segData         = 2 * 1024
	)

	plaintext := make([]byte, segData)
	if _, err := rand.Read(plaintext); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	salt, err := crypto.GenerateSalt_v2(crypto.SaltSize_v2)
	if err != nil {
		t.Fatalf("GenerateSalt_v2: %v", err)
	}
	macSalt, err := crypto.GenerateMACSalt()
	if err != nil {
		t.Fatalf("GenerateMACSalt: %v", err)
	}
	key, macKey := deriveKeysForV4(t, correctPassword, salt, macSalt, crypto.KeySize_v4_128)

	encResult, err := crypto.EncryptSegment(plaintext, key, macKey, 0, crypto.CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment: %v", err)
	}

	manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "wrong-pw-test",
		ContainerType: "video",
		IsSeekable:    true,
		Segments: []types.Segment_v4{
			{ID: "seg-0", StartTime: 0, Duration: 10},
		},
		Playlists: map[string][]string{"default": {"seg-0"}},
		KVI:       makeTestKVI(salt),
	}
	manifest.MACSaltBase64 = base64.StdEncoding.EncodeToString(macSalt)

	tmpPath := writeV4ContainerToTemp(t, &writer.V4WriteParams{
		OutputPath:     "",
		IsMain:         true,
		ContainerType:  types.ContainerTypeVideo,
		IsSeekable:     true,
		IDType:         types.IDType_Raw,
		Manifest:       manifest,
		SegmentResults: []*crypto.SegmentEncryptionResult{encResult},
		CipherMode:     0,
		EnableHMAC:     true,
	})
	defer os.Remove(tmpPath)

	// 错误密码打开 → 如果 password_hint 关闭，会走到 MAC 校验失败
	info, err := OpenV4Container(tmpPath, wrongPassword)
	if err != nil {
		// 也可能在 PasswordHint 阶段就失败
		t.Logf("OpenV4Container failed (acceptable for wrong password): %v", err)
		return
	}

	// 走到这里说明 PasswordHint 未启用，必须在 reader 阶段返回 ErrMACMismatch
	dec, err := NewSegmentSequentialReader(info, "default")
	if err != nil {
		t.Fatalf("NewSegmentSequentialReader: %v", err)
	}
	defer dec.Close()
	_, readErr := io.ReadAll(dec)
	if readErr == nil {
		t.Fatalf("expected ErrMACMismatch with wrong password, got nil")
	}
	if !errors.Is(readErr, crypto.ErrMACMismatch) {
		t.Fatalf("expected crypto.ErrMACMismatch, got: %v", readErr)
	}

	// 关键：reader 不能泄露明文线索（明文不应是 plaintext）
	// 已经在 ErrMACMismatch 路径上不会返回明文 → 隐式验证
}

// TestReaderV4_CipherMode_AES128 验证 AES-128-CTR (CipherMode=0) 的 roundtrip。
func TestReaderV4_CipherMode_AES128(t *testing.T) {
	const (
		password = "test-aes128-pw"
		segData  = 3 * 1024
	)

	plaintext := make([]byte, segData)
	if _, err := rand.Read(plaintext); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	salt, err := crypto.GenerateSalt_v2(crypto.SaltSize_v2)
	if err != nil {
		t.Fatalf("GenerateSalt_v2: %v", err)
	}
	macSalt, err := crypto.GenerateMACSalt()
	if err != nil {
		t.Fatalf("GenerateMACSalt: %v", err)
	}
	// 16 字节 key（AES-128）
	key, macKey := deriveKeysForV4(t, password, salt, macSalt, crypto.KeySize_v4_128)

	encResult, err := crypto.EncryptSegment(plaintext, key, macKey, 0, crypto.CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment: %v", err)
	}

	manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "aes128-test",
		ContainerType: "video",
		IsSeekable:    true,
		Segments: []types.Segment_v4{
			{ID: "seg-0", StartTime: 0, Duration: 10},
		},
		Playlists: map[string][]string{"default": {"seg-0"}},
		KVI:       makeTestKVI(salt),
	}
	manifest.MACSaltBase64 = base64.StdEncoding.EncodeToString(macSalt)

	tmpPath := writeV4ContainerToTemp(t, &writer.V4WriteParams{
		OutputPath:     "",
		IsMain:         true,
		ContainerType:  types.ContainerTypeVideo,
		IsSeekable:     true,
		IDType:         types.IDType_Raw,
		Manifest:       manifest,
		SegmentResults: []*crypto.SegmentEncryptionResult{encResult},
		CipherMode:     0, // AES-128
		EnableHMAC:     true,
	})
	defer os.Remove(tmpPath)

	// 验证 Header.CipherMode 写入正确
	rawData, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	cipherModeBytes := binary.LittleEndian.Uint16(rawData[types.CipherModeOffsetV4 : types.CipherModeOffsetV4+2])
	if cipherModeBytes != 0 {
		t.Errorf("on-disk CipherMode = %d, want 0 (AES-128)", cipherModeBytes)
	}

	// reader 解密
	info, err := OpenV4Container(tmpPath, password)
	if err != nil {
		t.Fatalf("OpenV4Container: %v", err)
	}
	if info.Header.CipherMode != 0 {
		t.Errorf("Header.CipherMode = %d, want 0", info.Header.CipherMode)
	}
	if len(info.EncryptKey) != crypto.KeySize_v4_128 {
		t.Errorf("EncryptKey length = %d, want %d (AES-128)", len(info.EncryptKey), crypto.KeySize_v4_128)
	}

	dec := readAllSequential(t, info)
	if !bytes.Equal(dec, plaintext) {
		t.Errorf("AES-128 roundtrip mismatch")
	}
}

// TestReaderV4_CipherMode_AES256 验证 AES-256-CTR (CipherMode=1) 的 roundtrip。
func TestReaderV4_CipherMode_AES256(t *testing.T) {
	const (
		password = "test-aes256-pw"
		segData  = 3 * 1024
	)

	plaintext := make([]byte, segData)
	if _, err := rand.Read(plaintext); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	salt, err := crypto.GenerateSalt_v2(crypto.SaltSize_v2)
	if err != nil {
		t.Fatalf("GenerateSalt_v2: %v", err)
	}
	macSalt, err := crypto.GenerateMACSalt()
	if err != nil {
		t.Fatalf("GenerateMACSalt: %v", err)
	}
	// 32 字节 key（AES-256）
	key, macKey := deriveKeysForV4(t, password, salt, macSalt, crypto.KeySize_v4_256)

	encResult, err := crypto.EncryptSegment(plaintext, key, macKey, 0, crypto.CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment: %v", err)
	}

	manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "aes256-test",
		ContainerType: "video",
		IsSeekable:    true,
		Segments: []types.Segment_v4{
			{ID: "seg-0", StartTime: 0, Duration: 10},
		},
		Playlists: map[string][]string{"default": {"seg-0"}},
		KVI:       makeTestKVI(salt),
	}
	manifest.MACSaltBase64 = base64.StdEncoding.EncodeToString(macSalt)

	tmpPath := writeV4ContainerToTemp(t, &writer.V4WriteParams{
		OutputPath:     "",
		IsMain:         true,
		ContainerType:  types.ContainerTypeVideo,
		IsSeekable:     true,
		IDType:         types.IDType_Raw,
		Manifest:       manifest,
		SegmentResults: []*crypto.SegmentEncryptionResult{encResult},
		CipherMode:     1, // AES-256
		EnableHMAC:     true,
	})
	defer os.Remove(tmpPath)

	rawData, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	cipherModeBytes := binary.LittleEndian.Uint16(rawData[types.CipherModeOffsetV4 : types.CipherModeOffsetV4+2])
	if cipherModeBytes != 1 {
		t.Errorf("on-disk CipherMode = %d, want 1 (AES-256)", cipherModeBytes)
	}

	info, err := OpenV4Container(tmpPath, password)
	if err != nil {
		t.Fatalf("OpenV4Container: %v", err)
	}
	if info.Header.CipherMode != 1 {
		t.Errorf("Header.CipherMode = %d, want 1", info.Header.CipherMode)
	}
	if len(info.EncryptKey) != crypto.KeySize_v4_256 {
		t.Errorf("EncryptKey length = %d, want %d (AES-256)", len(info.EncryptKey), crypto.KeySize_v4_256)
	}

	dec := readAllSequential(t, info)
	if !bytes.Equal(dec, plaintext) {
		t.Errorf("AES-256 roundtrip mismatch")
	}
}

// =============================================================================
// 辅助函数
// =============================================================================

// deriveKeysForV4 根据 password + macSalt + keyLen 派生 key/macKey。
func deriveKeysForV4(t *testing.T, password string, salt, macSalt []byte, keyLen int) (key, macKey []byte) {
	t.Helper()
	key = crypto.GenerateKey_v4(password, salt, keyLen)
	macKey = crypto.DeriveMACKey(password, macSalt)
	return key, macKey
}
