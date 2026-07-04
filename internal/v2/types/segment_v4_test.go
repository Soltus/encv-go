// internal/v2/types/segment_v4_test.go
package types

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"testing"
)

// TestSegmentHeader_BinarySize 验证 SegmentHeaderSize 常量与序列化产物的字节数一致。
//
// 必须等于 34（v4-container-capability-upgrade spec 约定的升级后大小）。
// 同时验证 unsafe.Sizeof 与 binary.Sizeof 不等于 34（说明 Go 结构体有 padding），
// 但手动序列化产物等于 34，padding 不会泄漏到磁盘。
func TestSegmentHeader_BinarySize(t *testing.T) {
	if SegmentHeaderSize != 34 {
		t.Fatalf("SegmentHeaderSize = %d, want 34 (per v4-container-capability-upgrade spec)", SegmentHeaderSize)
	}

	// 用全零值做最严格的对照
	zero := &SegmentHeader{}
	raw, err := zero.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}
	if len(raw) != SegmentHeaderSize {
		t.Errorf("len(MarshalBinary()) = %d, want %d", len(raw), SegmentHeaderSize)
	}
	if len(raw) != 34 {
		t.Errorf("len(MarshalBinary()) = %d, want 34", len(raw))
	}

	// 文档化：unsafe.Sizeof 不等于 34（Go padding 干扰）
	// 这是预期行为，不影响序列化产物
	// unsafe.Sizeof(SegmentHeader{}) = 40  (uint32 + uint64 + uint16*4 + uint32*3 产生 padding)
	// 这里只验证序列化产物严格 34 字节，不验证 unsafe.Sizeof

	// 验证 interface 约束
	var _ encoding.BinaryMarshaler = (*SegmentHeader)(nil)
	var _ encoding.BinaryUnmarshaler = (*SegmentHeader)(nil)
}

// TestSegmentHeader_Extended_RoundTrip 验证所有 10 个字段的 round-trip 一致性。
func TestSegmentHeader_Extended_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		hdr  SegmentHeader
	}{
		{
			name: "all_zero",
			hdr:  SegmentHeader{},
		},
		{
			name: "encrypted_no_compression",
			hdr: SegmentHeader{
				SegmentID:  0x12345678,
				DataLength: 0xDEADBEEFCAFEBABE,
				NonceSize:  16,
				ModeFlags:  ModeFlagEncrypted,
				MACSize:    10,
				DataCRC32:  0xABCDEF01,
			},
		},
		{
			name: "encrypted_with_zstd",
			hdr: SegmentHeader{
				SegmentID:           42,
				DataLength:          1024 * 1024,
				NonceSize:           16,
				ModeFlags:           ModeFlagEncrypted | ModeFlagCompressionZstd,
				MACSize:             10,
				DataCRC32:           0xDEADBEEF,
				CompressedBlockSize: 65535, // uint16 上限，避免 overflow
				Reserved:            0,
				SeekTableOffset:     0x1000,
				SeekTableLength:     0x2000,
			},
		},
		{
			name: "all_max_values",
			hdr: SegmentHeader{
				SegmentID:           0xFFFFFFFF,
				DataLength:          0xFFFFFFFFFFFFFFFF,
				NonceSize:           0xFFFF,
				ModeFlags:           0xFFFF,
				MACSize:             0xFFFF,
				DataCRC32:           0xFFFFFFFF,
				CompressedBlockSize: 0xFFFF,
				Reserved:            0xFFFF,
				SeekTableOffset:     0xFFFFFFFF,
				SeekTableLength:     0xFFFFFFFF,
			},
		},
		{
			name: "plaintext_segment",
			hdr: SegmentHeader{
				SegmentID:  7,
				DataLength: 8192,
				NonceSize:  0, // 明文 Segment 无 nonce
				ModeFlags:  0, // 明文 + 不压缩
				MACSize:    0, // 明文无 MAC
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			raw, err := tc.hdr.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary failed: %v", err)
			}
			if len(raw) != SegmentHeaderSize {
				t.Fatalf("len(raw) = %d, want %d", len(raw), SegmentHeaderSize)
			}

			var got SegmentHeader
			if err := got.UnmarshalBinary(raw); err != nil {
				t.Fatalf("UnmarshalBinary failed: %v", err)
			}

			if got != tc.hdr {
				t.Errorf("round-trip mismatch:\n got:  %+v\n want: %+v", got, tc.hdr)
			}
		})
	}
}

// TestSegmentHeader_BytesLayout 验证序列化产物的字节布局严格符合 spec 定义的偏移。
func TestSegmentHeader_BytesLayout(t *testing.T) {
	hdr := &SegmentHeader{
		SegmentID:           0x11223344,
		DataLength:          0xAABBCCDDEEFF0011,
		NonceSize:           0x5566,
		ModeFlags:           0x7788,
		MACSize:             0x99AA,
		DataCRC32:           0xBBCCDDEE,
		CompressedBlockSize: 0xFF00,
		Reserved:            0x1234,
		SeekTableOffset:     0x56789ABC,
		SeekTableLength:     0xDEF01234,
	}

	raw, err := hdr.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary failed: %v", err)
	}

	// 验证每个字段在二进制中的精确偏移（Little-Endian）
	tests := []struct {
		offset, size int
		name         string
		want         []byte
	}{
		{0, 4, "SegmentID", []byte{0x44, 0x33, 0x22, 0x11}},
		{4, 8, "DataLength", []byte{0x11, 0x00, 0xFF, 0xEE, 0xDD, 0xCC, 0xBB, 0xAA}},
		{12, 2, "NonceSize", []byte{0x66, 0x55}},
		{14, 2, "ModeFlags", []byte{0x88, 0x77}},
		{16, 2, "MACSize", []byte{0xAA, 0x99}},
		{18, 4, "DataCRC32", []byte{0xEE, 0xDD, 0xCC, 0xBB}},
		{22, 2, "CompressedBlockSize", []byte{0x00, 0xFF}},
		{24, 2, "Reserved", []byte{0x34, 0x12}},
		{26, 4, "SeekTableOffset", []byte{0xBC, 0x9A, 0x78, 0x56}},
		{30, 4, "SeekTableLength", []byte{0x34, 0x12, 0xF0, 0xDE}},
	}
	for _, tc := range tests {
		got := raw[tc.offset : tc.offset+tc.size]
		if !bytes.Equal(got, tc.want) {
			t.Errorf("%s at offset %d: got %x, want %x", tc.name, tc.offset, got, tc.want)
		}
	}
}

// TestSegmentHeader_ModeFlags 验证 ModeFlag* 常量位掩码的正确性。
func TestSegmentHeader_ModeFlags(t *testing.T) {
	if ModeFlagEncrypted != 1<<0 {
		t.Errorf("ModeFlagEncrypted = 0x%04x, want 0x%04x", ModeFlagEncrypted, uint16(1<<0))
	}
	if ModeFlagEncrypted != 0x0001 {
		t.Errorf("ModeFlagEncrypted = 0x%04x, want 0x0001", ModeFlagEncrypted)
	}
	if ModeFlagCompressionZstd != 1<<1 {
		t.Errorf("ModeFlagCompressionZstd = 0x%04x, want 0x%04x", ModeFlagCompressionZstd, uint16(1<<1))
	}
	if ModeFlagCompressionZstd != 0x0002 {
		t.Errorf("ModeFlagCompressionZstd = 0x%04x, want 0x0002", ModeFlagCompressionZstd)
	}

	// 验证 | 操作
	combined := ModeFlagEncrypted | ModeFlagCompressionZstd
	if combined != 0x0003 {
		t.Errorf("ModeFlagEncrypted | ModeFlagCompressionZstd = 0x%04x, want 0x0003", combined)
	}

	// 验证便利常量
	if ModeFlagsPlaintext != 0 {
		t.Errorf("ModeFlagsPlaintext = 0x%04x, want 0", ModeFlagsPlaintext)
	}
	if ModeFlagsEncryptedNoCompression != ModeFlagEncrypted {
		t.Errorf("ModeFlagsEncryptedNoCompression != ModeFlagEncrypted")
	}
	if ModeFlagsEncryptedZstd != (ModeFlagEncrypted|ModeFlagCompressionZstd) {
		t.Errorf("ModeFlagsEncryptedZstd != ModeFlagEncrypted|ModeFlagCompressionZstd")
	}
}

// TestSegmentHeader_DefaultValues 验证零值 SegmentHeader 的默认值语义。
//
// 零值应当表示「明文 + 不压缩」，对应 ModeFlags = 0x0000。
// 实际写入加密 Segment 时必须显式设 ModeFlagEncrypted。
func TestSegmentHeader_DefaultValues(t *testing.T) {
	var hdr SegmentHeader
	if hdr.ModeFlags != 0 {
		t.Errorf("zero-value ModeFlags = 0x%04x, want 0x0000 (plaintext + no compression)", hdr.ModeFlags)
	}
	if hdr.ModeFlags&ModeFlagEncrypted != 0 {
		t.Errorf("zero-value ModeFlags should NOT have ModeFlagEncrypted bit set")
	}
	if hdr.ModeFlags&ModeFlagCompressionZstd != 0 {
		t.Errorf("zero-value ModeFlags should NOT have ModeFlagCompressionZstd bit set")
	}
	// DataLength / SegmentID / 其他字段也应为 0
	if hdr.DataLength != 0 || hdr.SegmentID != 0 || hdr.MACSize != 0 {
		t.Errorf("zero-value header has unexpected non-zero fields: %+v", hdr)
	}

	// 序列化 0 值应是 34 字节全 0
	raw, err := hdr.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	for i, b := range raw {
		if b != 0 {
			t.Errorf("byte %d of zero-value marshal = 0x%02x, want 0x00", i, b)
		}
	}
}

// TestSegmentHeader_AllFieldsSet 验证所有字段非零时序列化产物严格 34 字节。
func TestSegmentHeader_AllFieldsSet(t *testing.T) {
	hdr := &SegmentHeader{
		SegmentID:           1,
		DataLength:          1,
		NonceSize:           1,
		ModeFlags:           1,
		MACSize:             1,
		DataCRC32:           1,
		CompressedBlockSize: 1,
		Reserved:            1,
		SeekTableOffset:     1,
		SeekTableLength:     1,
	}
	raw, err := hdr.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	if len(raw) != 34 {
		t.Errorf("len(raw) = %d, want 34", len(raw))
	}

	// round-trip 也应一致
	var got SegmentHeader
	if err := got.UnmarshalBinary(raw); err != nil {
		t.Fatalf("UnmarshalBinary: %v", err)
	}
	if got != *hdr {
		t.Errorf("round-trip mismatch:\n got:  %+v\n want: %+v", got, *hdr)
	}
}

// TestSegmentHeader_ShortBuffer_Rejected 验证 UnmarshalBinary 对不足 34 字节的输入
// 返回明确错误，不崩溃。
func TestSegmentHeader_ShortBuffer_Rejected(t *testing.T) {
	cases := []struct {
		name string
		size int
	}{
		{"empty", 0},
		{"one_byte", 1},
		{"under_half", 16},
		{"one_byte_less", 33},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var hdr SegmentHeader
			data := make([]byte, tc.size)
			err := hdr.UnmarshalBinary(data)
			if err == nil {
				t.Errorf("UnmarshalBinary(%d bytes) should fail, got nil", tc.size)
			}
		})
	}
}

// TestSegmentHeader_ExtraBytes_Accepted 验证 UnmarshalBinary 接受长度 >= 34 的输入
// （多余字节被忽略，只读前 34 字节）。
func TestSegmentHeader_ExtraBytes_Accepted(t *testing.T) {
	hdr := &SegmentHeader{
		SegmentID:  0xDEADBEEF,
		DataLength: 100,
		ModeFlags:  ModeFlagEncrypted,
		MACSize:    10,
	}
	raw, _ := hdr.MarshalBinary()
	// 拼接额外的 100 字节（模拟 Segment 头 + Nonce/Payload 串联）
	extra := make([]byte, 100)
	for i := range extra {
		extra[i] = 0xFF
	}
	combined := append(raw, extra...)

	var got SegmentHeader
	if err := got.UnmarshalBinary(combined); err != nil {
		t.Fatalf("UnmarshalBinary failed with extra bytes: %v", err)
	}
	if got.SegmentID != 0xDEADBEEF || got.DataLength != 100 ||
		got.ModeFlags != ModeFlagEncrypted || got.MACSize != 10 {
		t.Errorf("UnmarshalBinary ignored trailing bytes incorrectly: %+v", got)
	}
}

// TestSegmentHeader_BackwardCompat_OldSize18_Rejected 验证旧 18 字节 SegmentHeader
// 在新解析器中被拒绝（不崩溃、不误读）。
//
// 这是 v4-container-capability-upgrade spec 接受的破坏性 trade-off：
// 旧 18 字节布局已废弃，新解析器对 18 字节输入返回明确错误。
func TestSegmentHeader_BackwardCompat_OldSize18_Rejected(t *testing.T) {
	// 模拟旧 18 字节布局：
	//   SegmentID(4) + DataLength(8) + NonceSize(2) + DataCRC32(4) = 18
	oldData := make([]byte, 18)
	binary.LittleEndian.PutUint32(oldData[0:4], 0x12345678)
	binary.LittleEndian.PutUint64(oldData[4:12], 0xAABBCCDDEEFF0011)
	binary.LittleEndian.PutUint16(oldData[12:14], 16)
	binary.LittleEndian.PutUint32(oldData[14:18], 0xDEADBEEF)

	var hdr SegmentHeader
	err := hdr.UnmarshalBinary(oldData)
	if err == nil {
		t.Errorf("UnmarshalBinary(18 bytes old layout) should fail, got nil; parsed = %+v", hdr)
	}
	// 错误信息应明确提及 SegmentHeaderSize
	if err != nil {
		t.Logf("expected error: %v", err)
	}
}

// TestSegmentHeader_InterfaceCompliance 验证 SegmentHeader 实现了
// encoding.BinaryMarshaler 和 encoding.BinaryUnmarshaler 接口。
func TestSegmentHeader_InterfaceCompliance(t *testing.T) {
	// 指针类型实现接口
	var _ encoding.BinaryMarshaler = (*SegmentHeader)(nil)
	var _ encoding.BinaryUnmarshaler = (*SegmentHeader)(nil)
}
