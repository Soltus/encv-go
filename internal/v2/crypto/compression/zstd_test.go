package compression

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"

	seekable "github.com/SaveTheRbtz/zstd-seekable-format-go/pkg"
	"github.com/klauspost/compress/zstd"
)

// TestZstdSeekable_PublicAPI_RoundTrip 验证最基本的"Hello, World!"走完压缩 →
// 解压后字节完全一致。覆盖 CompressZstdSeekable / DecompressZstdSeekable 公开
// API 的端到端路径（zstd_smoke_test.go 里的同名 TestZstdSeekable_BasicRoundTrip
// 是 Task 7 冒烟测试，用的是库原始 API；本测试专门覆盖 Task 8 公开包装函数）。
func TestZstdSeekable_PublicAPI_RoundTrip(t *testing.T) {
	original := []byte("Hello, World!")

	compressed, seekTable, err := CompressZstdSeekable(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("CompressZstdSeekable failed: %v", err)
	}
	if len(compressed) == 0 {
		t.Fatalf("expected non-empty compressed output, got 0 bytes")
	}
	if len(seekTable) == 0 {
		t.Fatalf("expected non-empty seek table, got 0 bytes")
	}

	plaintext, err := DecompressZstdSeekable(compressed, seekTable)
	if err != nil {
		t.Fatalf("DecompressZstdSeekable failed: %v", err)
	}
	if !bytes.Equal(plaintext, original) {
		t.Fatalf("round-trip mismatch:\n want=%q\n got =%q", original, plaintext)
	}
}

// TestZstdSeekable_LargeFile_CompressDecompress 用 10MB 重复文本验证大
// 块输入下分块 + seekable 索引的正确性。10MB / 64KB ≈ 160 帧，足够覆盖
// 多帧拼接、seek-table 解析、跨帧随机访问的代码路径。
func TestZstdSeekable_LargeFile_CompressDecompress(t *testing.T) {
	const totalSize = 10 * 1024 * 1024 // 10 MB
	original := bytes.Repeat([]byte("abc"), totalSize/3)

	compressed, seekTable, err := CompressZstdSeekable(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("CompressZstdSeekable failed: %v", err)
	}

	plaintext, err := DecompressZstdSeekable(compressed, seekTable)
	if err != nil {
		t.Fatalf("DecompressZstdSeekable failed: %v", err)
	}
	if !bytes.Equal(plaintext, original) {
		t.Fatalf("large-file round-trip mismatch: want_len=%d got_len=%d", len(original), len(plaintext))
	}

	// 重复文本可压缩性极强（字典压缩 + 内容冗余），应至少省 50% 空间。
	ratio := float64(len(compressed)) / float64(len(original))
	if ratio >= 0.5 {
		t.Fatalf("expected compression ratio < 0.5 for repetitive input, got %.4f (orig=%d comp=%d)",
			ratio, len(original), len(compressed))
	}
}

// TestZstdSeekable_EmptyInput 验证空数据走完整路径时不会 panic，并且
// round-trip 后仍为空。seekable 库对空输入会产 0 帧 + 空 seek table，公开
// API 必须把这种"零数据"场景当成正常值处理。
func TestZstdSeekable_EmptyInput(t *testing.T) {
	original := []byte{}

	compressed, seekTable, err := CompressZstdSeekable(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("CompressZstdSeekable on empty input failed: %v", err)
	}
	// 空输入下 compressed 与 seekTable 都应当为空或零长（库行为）。
	if len(compressed) != 0 {
		t.Logf("note: empty input produced %d compressed bytes (acceptable)", len(compressed))
	}
	if len(seekTable) != 0 {
		t.Logf("note: empty input produced %d seek-table bytes (acceptable)", len(seekTable))
	}

	plaintext, err := DecompressZstdSeekable(compressed, seekTable)
	if err != nil {
		t.Fatalf("DecompressZstdSeekable on empty input failed: %v", err)
	}
	if len(plaintext) != 0 {
		t.Fatalf("expected empty plaintext, got %d bytes: %q", len(plaintext), plaintext)
	}
}

// TestZstdSeekable_BinaryRandomData 验证 1MB 随机（不可压缩）数据的
// round-trip。压缩率可能 < 1%（甚至负数——压缩头+索引可能比原数据更大），
// 但库必须正确处理、不能 panic、且解压后字节完全一致。
func TestZstdSeekable_BinaryRandomData(t *testing.T) {
	original := make([]byte, 1024*1024) // 1 MB
	if _, err := rand.Read(original); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}

	compressed, seekTable, err := CompressZstdSeekable(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("CompressZstdSeekable failed: %v", err)
	}

	plaintext, err := DecompressZstdSeekable(compressed, seekTable)
	if err != nil {
		t.Fatalf("DecompressZstdSeekable failed: %v", err)
	}
	if !bytes.Equal(plaintext, original) {
		t.Fatalf("random-data round-trip mismatch: want_len=%d got_len=%d", len(original), len(plaintext))
	}
}

// TestZstdSeekable_SeekTable_NonEmpty 验证 seek table 至少包含一帧，并且
// 公开的 NumFrames() 与 Reader 内部解析出的 NumFrames() 一致。
func TestZstdSeekable_SeekTable_NonEmpty(t *testing.T) {
	// 用 2 块 (128KB > 64KB) 强制产 >= 2 帧，确保 NumFrames 路径被覆盖。
	original := bytes.Repeat([]byte("xyz"), 128*1024/3)

	compressed, seekTable, err := CompressZstdSeekable(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("CompressZstdSeekable failed: %v", err)
	}
	if len(seekTable) == 0 {
		t.Fatalf("expected non-empty seek table, got 0 bytes")
	}

	st, err := seekable.NewSeekTable(seekTable)
	if err != nil {
		t.Fatalf("NewSeekTable failed: %v", err)
	}
	if numFrames := st.NumFrames(); numFrames < 1 {
		t.Fatalf("expected NumFrames() >= 1, got %d", numFrames)
	}

	// 进一步：把 compressed + seekTable 拼起来，Reader 内部解析出的
	// SeekTable.NumFrames() 必须与上面一致。
	combined := make([]byte, 0, len(compressed)+len(seekTable))
	combined = append(combined, compressed...)
	combined = append(combined, seekTable...)

	dec, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatalf("zstd.NewReader failed: %v", err)
	}
	defer dec.Close()

	r, err := seekable.NewReader(bytes.NewReader(combined), dec)
	if err != nil {
		t.Fatalf("seekable.NewReader failed: %v", err)
	}
	defer r.Close()

	rt, err := r.SeekTable()
	if err != nil {
		t.Fatalf("Reader.SeekTable failed: %v", err)
	}
	if rt.NumFrames() != st.NumFrames() {
		t.Fatalf("Reader.SeekTable.NumFrames()=%d != NewSeekTable.NumFrames()=%d",
			rt.NumFrames(), st.NumFrames())
	}
}

// TestZstdSeekable_SingleFrame 验证小数据（< 1KB）走单帧路径——输入只够
// 产 1 帧压缩数据，NumFrames() 应为 1。
func TestZstdSeekable_SingleFrame(t *testing.T) {
	// < 1KB 的明文，应只产生 1 帧。
	original := []byte("Small payload for single-frame test — < 1KB")

	compressed, seekTable, err := CompressZstdSeekable(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("CompressZstdSeekable failed: %v", err)
	}

	plaintext, err := DecompressZstdSeekable(compressed, seekTable)
	if err != nil {
		t.Fatalf("DecompressZstdSeekable failed: %v", err)
	}
	if !bytes.Equal(plaintext, original) {
		t.Fatalf("single-frame round-trip mismatch:\n want=%q\n got =%q", original, plaintext)
	}

	// 校验 NumFrames == 1
	st, err := seekable.NewSeekTable(seekTable)
	if err != nil {
		t.Fatalf("NewSeekTable failed: %v", err)
	}
	if n := st.NumFrames(); n != 1 {
		t.Fatalf("expected NumFrames() == 1 for small payload, got %d", n)
	}
}

// TestZstdSeekable_BlockSizeOverride 验证 CompressZstdSeekableWithBlockSize
// 的 blockSize 覆盖生效。同一原始数据，blockSize=1 字节和 blockSize=64KB
// 都能 round-trip，但前者会产生大量帧，后者会少很多——足以证明切分
// 逻辑按 blockSize 走。
func TestZstdSeekable_BlockSizeOverride(t *testing.T) {
	original := bytes.Repeat([]byte("xy"), 2048) // 4KB

	// 块大小=1 → 4096 帧
	smallCompressed, smallSeek, err := CompressZstdSeekableWithBlockSize(
		bytes.NewReader(original), 1)
	if err != nil {
		t.Fatalf("CompressZstdSeekableWithBlockSize(1) failed: %v", err)
	}

	// 块大小=64KB → 1 帧
	largeCompressed, largeSeek, err := CompressZstdSeekableWithBlockSize(
		bytes.NewReader(original), 64*1024)
	if err != nil {
		t.Fatalf("CompressZstdSeekableWithBlockSize(64K) failed: %v", err)
	}

	// 两边都能 round-trip
	if got, err := DecompressZstdSeekable(smallCompressed, smallSeek); err != nil {
		t.Fatalf("decompress small-block failed: %v", err)
	} else if !bytes.Equal(got, original) {
		t.Fatalf("small-block round-trip mismatch: want_len=%d got_len=%d", len(original), len(got))
	}

	if got, err := DecompressZstdSeekable(largeCompressed, largeSeek); err != nil {
		t.Fatalf("decompress large-block failed: %v", err)
	} else if !bytes.Equal(got, original) {
		t.Fatalf("large-block round-trip mismatch: want_len=%d got_len=%d", len(original), len(got))
	}

	// 帧数差异：small-block 应远多于 large-block
	smallST, _ := seekable.NewSeekTable(smallSeek)
	largeST, _ := seekable.NewSeekTable(largeSeek)
	if smallST.NumFrames() <= largeST.NumFrames() {
		t.Fatalf("expected small-block NumFrames (%d) > large-block NumFrames (%d)",
			smallST.NumFrames(), largeST.NumFrames())
	}

	// blockSize <= 0 应回退到默认 64KB，与显式 64KB 行为一致（至少都能 round-trip）
	defCompressed, defSeek, err := CompressZstdSeekableWithBlockSize(
		bytes.NewReader(original), 0)
	if err != nil {
		t.Fatalf("CompressZstdSeekableWithBlockSize(0) failed: %v", err)
	}
	if got, err := DecompressZstdSeekable(defCompressed, defSeek); err != nil {
		t.Fatalf("decompress default-block failed: %v", err)
	} else if !bytes.Equal(got, original) {
		t.Fatalf("default-block round-trip mismatch: want_len=%d got_len=%d", len(original), len(got))
	}
}

// TestZstdSeekable_RandomReadAt 验证压缩流的随机访问：通过 Reader.ReadAt
// 直接取任意偏移的明文，必须等于原始数据对应偏移的字节。
func TestZstdSeekable_RandomReadAt(t *testing.T) {
	// 1MB 高度可压缩数据 → 强制多帧
	original := make([]byte, 1*1024*1024)
	for i := range original {
		original[i] = byte(i % 251) // 周期 251 的伪随机字节
	}

	compressed, seekTable, err := CompressZstdSeekable(bytes.NewReader(original))
	if err != nil {
		t.Fatalf("CompressZstdSeekable failed: %v", err)
	}

	combined := make([]byte, 0, len(compressed)+len(seekTable))
	combined = append(combined, compressed...)
	combined = append(combined, seekTable...)

	dec, err := zstd.NewReader(nil)
	if err != nil {
		t.Fatalf("zstd.NewReader failed: %v", err)
	}
	defer dec.Close()

	r, err := seekable.NewReader(bytes.NewReader(combined), dec)
	if err != nil {
		t.Fatalf("seekable.NewReader failed: %v", err)
	}
	defer r.Close()

	// 抽 3 个偏移采样校验：开头、中间、末尾
	offsets := []int64{0, 512 * 1024, int64(len(original)) - 16}

	for _, off := range offsets {
		buf := make([]byte, 16)
		n, err := r.ReadAt(buf, off)
		if err != nil && err != io.EOF {
			t.Fatalf("ReadAt(%d) failed: %v", off, err)
		}
		if n == 0 {
			t.Fatalf("ReadAt(%d) returned 0 bytes", off)
		}
		want := original[off : off+int64(n)]
		if !bytes.Equal(buf[:n], want) {
			t.Fatalf("ReadAt(%d) mismatch:\n want=%x\n got =%x", off, want, buf[:n])
		}
	}
}
