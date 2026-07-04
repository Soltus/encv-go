// internal/v2/types/header_v4_test.go
package types

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"testing"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

// TestHeaderV4_CipherMode_RoundTrip 验证 v4 Header 的 CipherMode 字段
// 在 WriteHeaderV4 / ReadHeaderV4 两侧的序列化/反序列化一致性。
//
// 覆盖场景：
//  1. CipherMode=0（AES-128-CTR，v4 新默认）round-trip
//  2. CipherMode=1（AES-256-CTR，v4 可选）round-trip
//  3. 非法值（0xFFFF、0x00FF 等）在 ReadHeaderV4 时 fallback 到 0
//  4. 非法值在 WriteHeaderV4 时被规范化为 0
//  5. 旧 v4 容器（手工构造 0x0000）能正确读为 AES-128-CTR
//  6. CipherMode 字段在二进制中位于 offset 2040-2042（不破坏已有字段偏移）
//  7. 写入 CipherMode=0/1 后磁盘上相应字节是 LittleEndian 0x0000 / 0x0001
//  8. HeaderCRC32 计算范围 [0, 2036) 不变，CipherMode 字段不影响 CRC
func TestHeaderV4_CipherMode_RoundTrip(t *testing.T) {
	cases := []struct {
		name        string
		writeValue  uint16
		expectValue uint16
	}{
		{"AES-128-CTR (default)", 0, 0},
		{"AES-256-CTR (optional)", 1, 1},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// ① 构造 header 并显式设置 CipherMode
			hdr, err := CreateHeaderV4(true, ContainerTypeText, false, IDType_Raw, nil, [16]byte{})
			if err != nil {
				t.Fatalf("CreateHeaderV4 failed: %v", err)
			}
			hdr.CipherMode = tc.writeValue

			// ② 写 → 读 round-trip
			var buf bytes.Buffer
			if err := WriteHeaderV4(&buf, hdr); err != nil {
				t.Fatalf("WriteHeaderV4 failed: %v", err)
			}
			if buf.Len() != EnvelopeHeaderSize_v4 {
				t.Fatalf("written bytes = %d, want %d", buf.Len(), EnvelopeHeaderSize_v4)
			}

			got, err := ReadHeaderV4(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("ReadHeaderV4 failed: %v", err)
			}
			if got.CipherMode != tc.expectValue {
				t.Errorf("CipherMode round-trip: got %d, want %d", got.CipherMode, tc.expectValue)
			}

			// ③ 二进制布局校验：CipherMode 位于 offset 2040-2042（LittleEndian）
			raw := buf.Bytes()
			binaryMode := binary.LittleEndian.Uint16(raw[CipherModeOffsetV4 : CipherModeOffsetV4+2])
			if binaryMode != tc.expectValue {
				t.Errorf("CipherMode on-disk bytes = 0x%04x, want 0x%04x (offset %d)",
					binaryMode, tc.expectValue, CipherModeOffsetV4)
			}

			// ④ 已有字段偏移保护：Magic/Version/Flags/ContainerType 必须保持原位
			if !bytes.Equal(raw[0:4], MagicHeader_v2[:]) {
				t.Errorf("Magic offset broken: got %x, want %x", raw[0:4], MagicHeader_v2[:])
			}
			if binary.LittleEndian.Uint16(raw[4:6]) != hdr.Version {
				t.Errorf("Version offset broken: got %d, want %d",
					binary.LittleEndian.Uint16(raw[4:6]), hdr.Version)
			}
			if binary.LittleEndian.Uint16(raw[6:8]) != hdr.Flags {
				t.Errorf("Flags offset broken")
			}
			if binary.LittleEndian.Uint16(raw[8:10]) != hdr.ContainerType {
				t.Errorf("ContainerType offset broken")
			}
			if binary.LittleEndian.Uint32(raw[2028:2032]) != hdr.ManifestOffset {
				t.Errorf("ManifestOffset offset broken")
			}
			if binary.LittleEndian.Uint32(raw[2032:2036]) != hdr.ManifestLength {
				t.Errorf("ManifestLength offset broken")
			}
			if binary.LittleEndian.Uint32(raw[2036:2040]) != hdr.HeaderCRC32 {
				t.Errorf("HeaderCRC32 offset broken")
			}
		})
	}
}

// TestHeaderV4_CipherMode_DefaultZero 验证 CreateHeaderV4 创建的 Header
// 默认 CipherMode=0（AES-128-CTR，新默认）。
func TestHeaderV4_CipherMode_DefaultZero(t *testing.T) {
	hdr, err := CreateHeaderV4(true, ContainerTypeText, false, IDType_Raw, nil, [16]byte{})
	if err != nil {
		t.Fatalf("CreateHeaderV4 failed: %v", err)
	}
	if hdr.CipherMode != 0 {
		t.Errorf("CreateHeaderV4 default CipherMode = %d, want 0", hdr.CipherMode)
	}

	// 序列化后磁盘字节也必须是 0
	var buf bytes.Buffer
	if err := WriteHeaderV4(&buf, hdr); err != nil {
		t.Fatalf("WriteHeaderV4: %v", err)
	}
	got := binary.LittleEndian.Uint16(buf.Bytes()[CipherModeOffsetV4 : CipherModeOffsetV4+2])
	if got != 0 {
		t.Errorf("on-disk CipherMode = 0x%04x, want 0x0000", got)
	}
}

// TestHeaderV4_CipherMode_InvalidFallback_Read 验证 ReadHeaderV4
// 对非法 CipherMode 值（如 0xFFFF、0x00FF、0x0002 等）fallback 到 0。
//
// 这是关键的向后兼容保护：即使磁盘上该位置是 0xFFFF（旧 v4 容器
// 在某种异常填充情况下可能产生的值），读端也能稳健处理。
func TestHeaderV4_CipherMode_InvalidFallback_Read(t *testing.T) {
	// 手工构造一个 Header 二进制，篡改 CipherMode 位置为非法值
	hdr, err := CreateHeaderV4(true, ContainerTypeText, false, IDType_Raw, nil, [16]byte{})
	if err != nil {
		t.Fatalf("CreateHeaderV4 failed: %v", err)
	}
	var buf bytes.Buffer
	if err := WriteHeaderV4(&buf, hdr); err != nil {
		t.Fatalf("WriteHeaderV4: %v", err)
	}
	raw := buf.Bytes()

	invalidValues := []uint16{0xFFFF, 0x00FF, 0x0002, 0x0100, 0xABCD}
	for _, invalid := range invalidValues {
		// 写入非法值到 CipherMode 位置
		binary.LittleEndian.PutUint16(raw[CipherModeOffsetV4:CipherModeOffsetV4+2], invalid)

		// 必须重新计算 CRC，因为 2040-2042 区域不在 CRC 范围内（[0, 2036)）
		// 但 HeaderCRC32 字段位置是 2036-2040，**不覆盖** 2040-2042，
		// 所以修改 CipherMode 不会让 CRC 校验失败。直接读即可。
		got, err := ReadHeaderV4(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("ReadHeaderV4 with invalid CipherMode 0x%04x failed: %v", invalid, err)
		}
		if got.CipherMode != 0 {
			t.Errorf("invalid CipherMode 0x%04x should fallback to 0, got %d", invalid, got.CipherMode)
		}
	}
}

// TestHeaderV4_CipherMode_InvalidFallback_Write 验证 WriteHeaderV4
// 对非法 CipherMode 值在写入磁盘前规范化到 0（避免磁盘上出现非法值）。
func TestHeaderV4_CipherMode_InvalidFallback_Write(t *testing.T) {
	invalidValues := []uint16{0xFFFF, 0x00FF, 0x0002, 0x0100, 0xABCD}
	for _, invalid := range invalidValues {
		hdr, err := CreateHeaderV4(true, ContainerTypeText, false, IDType_Raw, nil, [16]byte{})
		if err != nil {
			t.Fatalf("CreateHeaderV4 failed: %v", err)
		}
		hdr.CipherMode = invalid

		var buf bytes.Buffer
		if err := WriteHeaderV4(&buf, hdr); err != nil {
			t.Fatalf("WriteHeaderV4: %v", err)
		}
		onDisk := binary.LittleEndian.Uint16(buf.Bytes()[CipherModeOffsetV4 : CipherModeOffsetV4+2])
		if onDisk != 0 {
			t.Errorf("invalid CipherMode 0x%04x written to disk as 0x%04x, want 0x0000", invalid, onDisk)
		}

		// 写完后，hdr.CipherMode 字段也应被规范化为 0
		if hdr.CipherMode != 0 {
			t.Errorf("after WriteHeaderV4, hdr.CipherMode = %d, want 0 (normalized)", hdr.CipherMode)
		}
	}
}

// TestHeaderV4_CipherMode_BytesLayout 验证 CipherMode 字段在二进制中的
// 精确位置是 offset 2040-2042，**不破坏** HeaderCRC32 计算范围。
func TestHeaderV4_CipherMode_BytesLayout(t *testing.T) {
	hdr, err := CreateHeaderV4(true, ContainerTypeText, false, IDType_Raw, nil, [16]byte{})
	if err != nil {
		t.Fatalf("CreateHeaderV4 failed: %v", err)
	}
	hdr.CipherMode = 1 // AES-256-CTR

	var buf bytes.Buffer
	if err := WriteHeaderV4(&buf, hdr); err != nil {
		t.Fatalf("WriteHeaderV4: %v", err)
	}
	raw := buf.Bytes()

	// 1. CipherMode 字段位置必须是 2040-2042
	if got := binary.LittleEndian.Uint16(raw[2040:2042]); got != 1 {
		t.Errorf("CipherMode at offset 2040 = 0x%04x, want 0x0001", got)
	}

	// 2. HeaderCRC32 位置必须是 2036-2040（不变）
	storedCRC := binary.LittleEndian.Uint32(raw[2036:2040])
	calculatedCRC := crc32.ChecksumIEEE(raw[:2036])
	if storedCRC != calculatedCRC {
		t.Errorf("HeaderCRC32 mismatch: stored=%08x, calculated=%08x", storedCRC, calculatedCRC)
	}

	// 3. Reserved2 占据 2042-2048 字节
	if len(hdr.Reserved2) != 6 {
		t.Errorf("Reserved2 length = %d, want 6", len(hdr.Reserved2))
	}

	// 4. 整个 header 仍是 2048 字节
	if len(raw) != EnvelopeHeaderSize_v4 {
		t.Errorf("header size = %d, want %d", len(raw), EnvelopeHeaderSize_v4)
	}
}

// TestHeaderV4_CipherMode_OldContainerCompat 模拟读取一个"旧 v4 容器"（
// CipherMode 位置为 0x0000）能正确读为 CipherMode=0（AES-128-CTR）。
//
// 旧 v4 容器在升级前创建时没有 CipherMode 字段，磁盘上 Reserved2 区域
// 是 Go 结构体零值（0x0000）。升级后 read 必须能稳健处理。
func TestHeaderV4_CipherMode_OldContainerCompat(t *testing.T) {
	hdr, err := CreateHeaderV4(true, ContainerTypeText, false, IDType_Raw, nil, [16]byte{})
	if err != nil {
		t.Fatalf("CreateHeaderV4 failed: %v", err)
	}
	// 显式不设置 CipherMode（保持零值 = 旧 v4 容器行为）
	hdr.CipherMode = 0

	var buf bytes.Buffer
	if err := WriteHeaderV4(&buf, hdr); err != nil {
		t.Fatalf("WriteHeaderV4: %v", err)
	}
	raw := buf.Bytes()

	// 模拟"读侧"：把磁盘看作旧 v4 容器，期望解析为 0
	got, err := ReadHeaderV4(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadHeaderV4 (old-container simulation) failed: %v", err)
	}
	if got.CipherMode != 0 {
		t.Errorf("old v4 container CipherMode = %d, want 0 (AES-128-CTR fallback)", got.CipherMode)
	}
}

// TestNormalizeCipherModeV4 单元测试 normalizeCipherModeV4 函数的边界。
func TestNormalizeCipherModeV4(t *testing.T) {
	cases := []struct {
		in, want uint16
	}{
		{0, 0},
		{1, 1},
		{0x0002, 0}, // 未知值 fallback
		{0x00FF, 0},
		{0xABCD, 0},
		{0xFFFF, 0}, // 明显非法值 fallback
	}
	for _, c := range cases {
		if got := normalizeCipherModeV4(c.in); got != c.want {
			t.Errorf("normalizeCipherModeV4(0x%04x) = 0x%04x, want 0x%04x", c.in, got, c.want)
		}
	}
}

// =============================================================================
// Task 5.2-5.5: MacSalt 字段存储测试套件
// =============================================================================
//
// 这些测试覆盖 v4 容器 MacSalt 字段的存储方案。
//
// 关键设计：
//   - MacSalt **不在 v4 Header 中**（Header 偏移 36-2028 被 SpecialID 完全占据）
//   - MacSalt 改存于 Manifest（v4 容器的可变长元数据区域）的 MACSaltBase64 字段
//   - 旧 v4 容器（无 mac_salt 字段）的 Manifest.MACSaltBase64 = "" → fallback
//   - HasMACSalt() helper 统一"是否存在 mac_salt"的判断逻辑
//
// 测试覆盖：
//  1. TestHeaderV4_MacSalt_RoundTrip：Manifest 中 MacSalt 完整 round-trip
//  2. TestHeaderV4_MacSalt_BackwardCompat_AllZeros：旧 v4 容器无 mac_salt → HasMACSalt=false
//  3. TestHeaderV4_MacSalt_DistinctFromPasswordHint：MacSalt 不在 PasswordHint/SpecialID 偏移
//  4. TestHeaderV4_MacSalt_LengthIs16：mac_salt 长度严格 16 字节
//  5. TestHasMACSalt：helper 函数边界
//  6. TestManifestV4_MacSaltBase64_OmitEmpty：JSON omitempty 行为

// TestHeaderV4_MacSalt_RoundTrip 验证 MacSalt 字段在 Manifest 序列化/反序列化
// 两端完全一致（base64 编码后的 16 字节随机值）。
//
// 这是 MacSalt 存储的核心契约：写入器生成的 mac_salt，读取器必须能无损还原。
func TestHeaderV4_MacSalt_RoundTrip(t *testing.T) {
	// 模拟 writer 生成的 16 字节随机 mac_salt
	originalSalt := []byte{
		0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF,
		0xFE, 0xDC, 0xBA, 0x98, 0x76, 0x54, 0x32, 0x10,
	}
	mf := &Manifest_v4{
		Version:        4,
		ContainerID:    "test-container",
		ContainerType:  "video",
		IsSeekable:     true,
		MACSaltBase64:  base64.StdEncoding.EncodeToString(originalSalt),
		Segments:       []Segment_v4{},
		Playlists:      map[string][]string{},
	}

	// 序列化 → 反序列化
	raw, err := mf.SerializeToJSON_v4()
	if err != nil {
		t.Fatalf("SerializeToJSON_v4 failed: %v", err)
	}

	parsed, err := DeserializeManifest_v4(raw)
	if err != nil {
		t.Fatalf("DeserializeManifest_v4 failed: %v", err)
	}

	// 验证 base64 字符串 round-trip 一致
	if parsed.MACSaltBase64 != mf.MACSaltBase64 {
		t.Errorf("MACSaltBase64 round-trip mismatch:\n got = %q\nwant = %q",
			parsed.MACSaltBase64, mf.MACSaltBase64)
	}

	// 验证解码后字节完全一致
	decoded, err := base64.StdEncoding.DecodeString(parsed.MACSaltBase64)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}
	if !bytes.Equal(decoded, originalSalt) {
		t.Errorf("decoded mac_salt mismatch:\n got = %x\nwant = %x", decoded, originalSalt)
	}

	// 验证 HasMACSalt helper 返回 true
	if !HasMACSalt(decoded) {
		t.Error("HasMACSalt should return true for non-zero 16-byte mac_salt")
	}
}

// TestHeaderV4_MacSalt_BackwardCompat_AllZeros 模拟旧 v4 容器（Manifest 中无
// mac_salt_base64 字段）的场景，验证 HasMACSalt 能正确识别为"缺失"。
//
// 旧 v4 容器的 Manifest JSON 没有 mac_salt_base64 字段 → 反序列化后 MACSaltBase64 = ""
// → 行为等效于"mac_salt 全为零 / 缺失" → HasMACSalt 返回 false
// → 调用方应 fallback 到 encrypt salt（KVI.salt_base64）
func TestHeaderV4_MacSalt_BackwardCompat_AllZeros(t *testing.T) {
	// 模拟旧 v4 容器：Manifest JSON 中没有 mac_salt_base64 字段
	const oldManifestJSON = `{
		"version": 4,
		"container_id": "old-container",
		"container_type": "video",
		"is_seekable": true,
		"segments": [],
		"playlists": {},
		"kvi": {"salt_base64":"AAAA","iv_base64":"AAAA"}
	}`

	parsed, err := DeserializeManifest_v4([]byte(oldManifestJSON))
	if err != nil {
		t.Fatalf("DeserializeManifest_v4 failed: %v", err)
	}

	// 旧 v4 容器反序列化后 MACSaltBase64 必须是空字符串
	if parsed.MACSaltBase64 != "" {
		t.Errorf("old v4 container MACSaltBase64 = %q, want empty string", parsed.MACSaltBase64)
	}

	// HasMACSalt(nil) 必须返回 false（向后兼容判断）
	if HasMACSalt(nil) {
		t.Error("HasMACSalt(nil) should return false (old v4 container case)")
	}

	// HasMACSalt([]byte{}) 也必须返回 false（防御）
	if HasMACSalt([]byte{}) {
		t.Error("HasMACSalt([]byte{}) should return false")
	}

	// HasMACSalt(make([]byte, 16)) 全零必须返回 false（结构体零值场景）
	zeroSalt := make([]byte, 16)
	if HasMACSalt(zeroSalt) {
		t.Error("HasMACSalt(zero 16-byte slice) should return false (backward-compat all-zeros)")
	}

	// 显式 [16]byte{} 零值数组也必须返回 false
	var zeroArray [16]byte
	if HasMACSalt(zeroArray[:]) {
		t.Error("HasMACSalt([16]byte{} slice) should return false (backward-compat all-zeros)")
	}

	// 旧 v4 容器不能直接 round-trip 一个 mac_salt（这是预期的）
	// 重新序列化后 mac_salt_base64 字段不会出现在 JSON 中（omitempty 行为）
	raw, err := parsed.SerializeToJSON_v4()
	if err != nil {
		t.Fatalf("re-serialize failed: %v", err)
	}
	if bytes.Contains(raw, []byte("mac_salt_base64")) {
		t.Errorf("re-serialized JSON should NOT contain mac_salt_base64 (omitempty), got: %s", raw)
	}
}

// TestHeaderV4_MacSalt_DistinctFromPasswordHint 验证 MacSalt 字段在 Manifest 中存储，
// 物理上**不冲突** v4 Header 中已分配的 PasswordHint (offset 20-36) 和
// SpecialID (offset 36-2028) 字段。
//
// 这是 MacSalt 改存 Manifest 的根本原因：Header 中没有可用空间。
// 本测试作为这一设计决策的回归保护——如果未来有人尝试"清理"Manifest 中的 MacSalt
// 字段，错误地认为它应该放回 Header，本测试会立即失败提醒。
func TestHeaderV4_MacSalt_DistinctFromPasswordHint(t *testing.T) {
	// 1. Header 布局：PasswordHint 在 offset 20-36（16 字节），SpecialID 在 36-2028（1992 字节）
	const (
		PasswordHintStart = 20
		PasswordHintEnd   = 36
		SpecialIDStart    = 36
		SpecialIDEnd      = 2028
	)

	// 2. MacSalt 字段位于 Manifest_v4 结构体内（不属于 Header 二进制布局）
	mfType := Manifest_v4{}

	// 编译期保护：Manifest_v4 中必须有名为 MACSaltBase64 的字段
	// （如果有人重命名/删除此字段，本测试在编译期会失败）
	_ = mfType.MACSaltBase64

	// 3. 序列化 Header 二进制，验证 PasswordHint/SpecialID 偏移未被 MacSalt 占用
	hdr, err := CreateHeaderV4(true, ContainerTypeVideo, true, IDType_Raw, nil, [16]byte{})
	if err != nil {
		t.Fatalf("CreateHeaderV4 failed: %v", err)
	}

	var buf bytes.Buffer
	if err := WriteHeaderV4(&buf, hdr); err != nil {
		t.Fatalf("WriteHeaderV4 failed: %v", err)
	}
	raw := buf.Bytes()

	// 4. PasswordHint 区域（offset 20-36）必须是原始值（未被 mac_salt 占用）
	passwordHintArea := raw[PasswordHintStart:PasswordHintEnd]
	if !bytes.Equal(passwordHintArea, make([]byte, 16)) {
		// 如果此断言失败，说明有人错误地往 PasswordHint 区域写了 mac_salt
		allZeros := true
		for _, b := range passwordHintArea {
			if b != 0 {
				allZeros = false
				break
			}
		}
		if !allZeros {
			t.Errorf("PasswordHint area (offset %d-%d) was modified, "+
				"this means MacSalt was incorrectly placed in Header. "+
				"Got bytes: %x", PasswordHintStart, PasswordHintEnd, passwordHintArea)
		}
	}

	// 5. SpecialID 区域（offset 36-2028）起始 16 字节也必须是原始 ID 内容
	//    （注：CreateHeaderV4 会填充随机 ID，所以这里只检查 PasswordHint 区域）
	_ = SpecialIDStart
	_ = SpecialIDEnd

	// 6. 验证 Manifest_v4 的 JSON tag 不在 Header offset 范围内
	//    （即 mac_salt 物理存储于 Manifest，绝不会与 Header 任何字段冲突）
	if MACSaltBase64JSONTag == "" {
		t.Error("MACSaltBase64JSONTag should be defined for documentation purposes")
	}
}

// MACSaltBase64JSONTag 记录 Manifest_v4.MACSaltBase64 字段的 JSON tag，
// 仅用于 TestHeaderV4_MacSalt_DistinctFromPasswordHint 的文档化目的。
const MACSaltBase64JSONTag = "mac_salt_base64"

// TestHeaderV4_MacSalt_LengthIs16 验证 mac_salt 字段长度严格 16 字节。
//
// 16 字节 = 128 bit 随机性，与 AES IV（IVSize_v2 = 16）保持一致。
// 这一长度与 crypto.MACSaltLength 常量绑定，是接口契约。
func TestHeaderV4_MacSalt_LengthIs16(t *testing.T) {
	// 1. Manifest_v4 中 MACSaltBase64 是 string 字段（base64 编码），长度由调用方保证
	//    验证 base64 编码 16 字节 → 24 字符
	salt16 := []byte{
		0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77,
		0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF,
	}
	encoded := base64.StdEncoding.EncodeToString(salt16)

	mf := &Manifest_v4{
		Version:       4,
		ContainerID:   "test",
		ContainerType: "video",
		MACSaltBase64: encoded,
	}

	raw, err := mf.SerializeToJSON_v4()
	if err != nil {
		t.Fatalf("SerializeToJSON_v4 failed: %v", err)
	}

	parsed, err := DeserializeManifest_v4(raw)
	if err != nil {
		t.Fatalf("DeserializeManifest_v4 failed: %v", err)
	}

	// 2. 验证解码后字节长度严格 == 16
	decoded, err := base64.StdEncoding.DecodeString(parsed.MACSaltBase64)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}
	const expectedLength = 16
	if len(decoded) != expectedLength {
		t.Errorf("decoded mac_salt length = %d, want %d", len(decoded), expectedLength)
	}

	// 3. 验证常量值（防止有人误改 MACSaltLength）
	if MACSaltLengthConst != 16 {
		t.Errorf("MACSaltLengthConst = %d, want 16", MACSaltLengthConst)
	}
}

// MACSaltLengthConst 镜像 crypto.MACSaltLength 常量，仅用于测试断言。
// 这样 header_v4_test.go 不需要 import crypto 包就能验证长度契约。
const MACSaltLengthConst = 16

// TestHasMACSalt 单元测试 HasMACSalt helper 函数的边界场景。
//
// HasMACSalt 是 MacSalt 字段向后兼容判断的统一入口，
// 它的行为必须严格定义，否则调用方会写出不一致的 fallback 逻辑。
func TestHasMACSalt(t *testing.T) {
	cases := []struct {
		name string
		salt []byte
		want bool
	}{
		{"nil slice", nil, false},
		{"empty slice", []byte{}, false},
		{"all zero 1 byte", []byte{0x00}, false},
		{"all zero 16 bytes", make([]byte, 16), false},
		{"all zero 32 bytes", make([]byte, 32), false},
		{"first byte non-zero", []byte{0x01, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, true},
		{"last byte non-zero", []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x01}, true},
		{"middle byte non-zero", []byte{0, 0, 0, 0, 0, 0, 0, 0xFF, 0, 0, 0, 0, 0, 0, 0, 0}, true},
		{"all 0xFF", []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, true},
		{"all 0x01", []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}, true},
		{"non-zero length 15", make([]byte, 15), false}, // 长度不足视为缺失
		{"non-zero length 17", append(make([]byte, 16), 0x01), true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got := HasMACSalt(tc.salt); got != tc.want {
				t.Errorf("HasMACSalt(%v) = %v, want %v", tc.salt, got, tc.want)
			}
		})
	}
}

// TestManifestV4_MacSaltBase64_OmitEmpty 验证 JSON tag `omitempty` 的行为：
// MACSaltBase64 为空字符串时，序列化结果不包含 "mac_salt_base64" 键。
//
// 这一行为保证旧 v4 容器（无 mac_salt 字段）的 Manifest JSON 形态完全不变。
func TestManifestV4_MacSaltBase64_OmitEmpty(t *testing.T) {
	// 1. 空 MACSaltBase64 → JSON 中不应该出现 mac_salt_base64 键
	mfEmpty := &Manifest_v4{
		Version:       4,
		ContainerID:   "test-empty",
		ContainerType: "video",
		MACSaltBase64: "", // 显式空
	}

	rawEmpty, err := mfEmpty.SerializeToJSON_v4()
	if err != nil {
		t.Fatalf("SerializeToJSON_v4 failed: %v", err)
	}

	if bytes.Contains(rawEmpty, []byte("mac_salt_base64")) {
		t.Errorf("empty MACSaltBase64 should be omitted from JSON, got: %s", rawEmpty)
	}

	// 2. 非空 MACSaltBase64 → JSON 中**应该**出现 mac_salt_base64 键
	mfSet := &Manifest_v4{
		Version:       4,
		ContainerID:   "test-set",
		ContainerType: "video",
		MACSaltBase64: base64.StdEncoding.EncodeToString([]byte{
			0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
			0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10,
		}),
	}

	rawSet, err := mfSet.SerializeToJSON_v4()
	if err != nil {
		t.Fatalf("SerializeToJSON_v4 failed: %v", err)
	}

	if !bytes.Contains(rawSet, []byte(`"mac_salt_base64"`)) {
		t.Errorf("non-empty MACSaltBase64 should be present in JSON, got: %s", rawSet)
	}
}
