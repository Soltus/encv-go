package reader

import (
	"bytes"
	"errors"
	"hash/crc32"
	"io"
	"os"
	"testing"

	containerhandle "github.com/Soltus/encv-go/internal/v2/container/handle"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/internal/v2/writer"

	_ "github.com/Soltus/encv-go/internal/testguard"
)

// ============================================================================
// 回归锁：v4 容器**必须带可校验的完整性元数据**，且损坏必须被**显式发现**
// ----------------------------------------------------------------------------
// 真机事故（2026-10-02，模拟器内真实后端实测）：
//   把合法容器中间 512 字节抹成 0，/stream 返回
//     **HTTP 200 + 长度与明文完全一致 + 内容从第 25496 字节起是乱码，且不报错**
//   真机表现：文件能列能下载，就是播不出来/花屏，前端与后端都没有任何错误提示。
//
// 根因（两处都是 v4 独有的洞）：
//   1. 写入端 `SingleFileContainerWriter.FinishFragment` 的 v4 分支把
//      `DataCRC32` **写死成 0**（v2/v3 分支会算真实 CRC）⇒ 容器里没有元数据。
//   2. 读取端 `GetFragmentReader` 的唯一 CRC 校验被 `if r.headerVersion != 4`
//      排除 ⇒ v4 根本不校验（何况 v4 数据区是裸密文，没有 block header 可校）。
//
// 修法（对应决策 A：写入端默认启用完整性校验）：
//   1. 写入端 v4 也累计并写入真实 CRC32
//   2. 读取端在"从分片起点整片读"时边读边校验（crcGuardReader）
//   3. DataCRC32 == 0（老容器）一律跳过 ⇒ 存量容器不受影响
// ============================================================================

// openV4Manifest 直接读容器里的（v2 适配后）manifest，用于检查元数据
func openV4Manifest(t *testing.T, path string) *types.Manifest {
	t.Helper()
	src, err := containerhandle.NewFileSource(path)
	if err != nil {
		t.Fatalf("NewFileSource: %v", err)
	}
	defer src.Close()
	h, err := containerhandle.Open(src)
	if err != nil {
		t.Fatalf("containerhandle.Open: %v", err)
	}
	defer h.Close()
	mf := h.Manifest()
	if mf == nil || len(mf.Fragments) == 0 {
		t.Fatalf("容器里没有 fragment（无法验证元数据）")
	}
	return mf
}

// TestV4Fragment_WriterRecordsDataCRC32 写入端必须把真实 CRC 写进 manifest
func TestV4Fragment_WriterRecordsDataCRC32(t *testing.T) {
	plain := bytes.Repeat([]byte("integrity-guard"), 300) // ≈4.8KB
	path, _ := createV4PluginPathContainer(t, types.ContainerTypeVideo, plain)

	mf := openV4Manifest(t, path)
	frag := mf.Fragments[0]

	if frag.DataCRC32 == 0 {
		t.Fatalf("v4 容器的 DataCRC32 = 0 —— 容器里**没有任何完整性元数据**，损坏永远无法被发现（本 bug 的根因）")
	}

	// 与"文件里那一段真实字节"的 CRC 对得上，才是可信的元数据
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	buf := make([]byte, frag.Length)
	if _, err := f.ReadAt(buf, int64(frag.PhysicalOffset)); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	want := crc32.ChecksumIEEE(buf)
	if frag.DataCRC32 != want {
		t.Errorf("DataCRC32=%08x 与数据实际 CRC %08x 不一致（元数据不可信）", frag.DataCRC32, want)
	}
}

// TestV4Fragment_TamperedData_MustFail 篡改后的容器必须**显式失败**，不能吐乱码
func TestV4Fragment_TamperedData_MustFail(t *testing.T) {
	plain := bytes.Repeat([]byte("tamper-detection"), 300)
	path, password := createV4PluginPathContainer(t, types.ContainerTypeVideo, plain)

	// 先确认未篡改时是好的（排除"测试本身写错"导致的假红）
	if got := readAllViaFactory(t, path, password, nil); !bytes.Equal(got, plain) {
		t.Fatalf("未篡改的容器就读不对（%d 字节）—— 测试夹具本身有问题", len(got))
	}

	// 篡改数据区中间 1 字节（不是头，避免变成"非容器"）
	mf := openV4Manifest(t, path)
	frag := mf.Fragments[0]
	off := int64(frag.PhysicalOffset) + int64(frag.Length)/2

	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var one [1]byte
	if _, err := f.ReadAt(one[:], off); err != nil {
		t.Fatalf("read: %v", err)
	}
	one[0] ^= 0x01 // 翻转 1 bit
	if _, err := f.WriteAt(one[:], off); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	// 关键断言：必须报错，且**不能**返回与明文一致的"看起来正常"的内容
	got, err := readAllViaFactoryErr(t, path, password)
	if err == nil {
		if bytes.Equal(got, plain) {
			t.Fatalf("篡改后仍然解出正确明文 —— 篡改没生效，测试无效")
		}
		t.Fatalf("篡改后的容器被静默解出 %d 字节乱码且**不报错** —— 静默损坏（真机：能下载、播不出、无任何提示）", len(got))
	}
	if !errors.Is(err, types.ErrDataCorrupted) {
		t.Errorf("期望 ErrDataCorrupted（便于上层映射 422 data_corrupted），实际：%v", err)
	}
}

// TestCRCGuard_SkipsWhenNoMetadata 老容器（DataCRC32=0）必须不受影响
func TestCRCGuard_SkipsWhenNoMetadata(t *testing.T) {
	r := bytes.NewReader([]byte("legacy"))
	if got := crcGuardFor(r, &types.Fragment{ID: "f0", DataCRC32: 0}, 0); got != io.Reader(r) {
		t.Error("DataCRC32=0 的老容器不得套 CRC 校验（否则存量容器全读不了）")
	}
	// 中途 seek 进入分片：读不全 ⇒ 也不校验
	if got := crcGuardFor(r, &types.Fragment{ID: "f0", DataCRC32: 0x1234}, 4096); got != io.Reader(r) {
		t.Error("非分片起点读取时不得套 CRC 校验（算不出整片 CRC）")
	}
}

// TestCRCGuard_DetectsMismatch 校验器本身：CRC 不符 → ErrDataCorrupted
func TestCRCGuard_DetectsMismatch(t *testing.T) {
	data := []byte("hello-integrity")
	g := newCRCGuardReader(bytes.NewReader(data), 0xDEADBEEF, "f0", uint64(len(data)))
	if _, err := io.ReadAll(g); !errors.Is(err, types.ErrDataCorrupted) {
		t.Errorf("CRC 不符必须返回 ErrDataCorrupted，实际：%v", err)
	}
	// 相符则必须一路读到 EOF（不能误报）
	g2 := newCRCGuardReader(bytes.NewReader(data), crc32.ChecksumIEEE(data), "f0", uint64(len(data)))
	if _, err := io.ReadAll(g2); err != nil {
		t.Errorf("CRC 相符却报错：%v", err)
	}
}

// TestCRCGuard_DetectsMismatchWithLimitReader 读满整片就判定，不等 io.EOF。
//
// 真机路径就是 io.Copy(w, io.LimitReader(reader, contentLength))：contentLength
// 正好等于明文长度时 LimitReader 读满即 EOF，**不会**再调底层一次 ⇒
// 只等 io.EOF 判定的话，大文件（流式分支）的校验永远不触发（实测 8.6MB 样例）。
func TestCRCGuard_DetectsMismatchWithLimitReader(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 1024)
	g := newCRCGuardReader(bytes.NewReader(data), 0xDEADBEEF, "f0", uint64(len(data)))
	// 模拟 HTTP 侧：只读满 contentLength 就停
	if _, err := io.Copy(io.Discard, io.LimitReader(g, int64(len(data)))); !errors.Is(err, types.ErrDataCorrupted) {
		t.Errorf("读满整片就必须判定（不等 io.EOF），实际：%v", err)
	}
}

// TestV4Fragment_BlockCRC_Written 写入端必须写分块 CRC（块大小 64KB）
func TestV4Fragment_BlockCRC_Written(t *testing.T) {
	// 3 个块：64KB * 2 + 余数
	plain := bytes.Repeat([]byte("block-crc"), 64*1024/9*3)
	path, _ := createV4PluginPathContainer(t, types.ContainerTypeVideo, plain)

	mf := openV4Manifest(t, path)
	frag := mf.Fragments[0]
	if frag.BlockCRCSize == 0 || len(frag.BlockCRC32) == 0 {
		t.Fatalf("容器没有分块 CRC（BlockCRCSize=%d 块数=%d）⇒ 只能读完整片才发现损坏",
			frag.BlockCRCSize, len(frag.BlockCRC32))
	}
	wantBlocks := (int(frag.Length) + writer.FragmentBlockCRCSize - 1) / writer.FragmentBlockCRCSize
	if len(frag.BlockCRC32) != wantBlocks {
		t.Errorf("块数 %d ≠ 期望 %d（片长 %d / 块 %d）", len(frag.BlockCRC32), wantBlocks, frag.Length, writer.FragmentBlockCRCSize)
	}
	// 逐块核对真实数据
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	data := make([]byte, frag.Length)
	if _, err := f.ReadAt(data, int64(frag.PhysicalOffset)); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	for i, want := range frag.BlockCRC32 {
		start := i * writer.FragmentBlockCRCSize
		end := start + writer.FragmentBlockCRCSize
		if end > len(data) {
			end = len(data)
		}
		if got := crc32.ChecksumIEEE(data[start:end]); got != want {
			t.Errorf("第 %d 块 CRC=%08x ≠ 实际 %08x（元数据不可信）", i, want, got)
		}
	}
}

// TestV4Fragment_BlockCRC_DetectsEarly 分块 CRC 的意义：**在损坏处就报错**
// （整片 CRC 要读完整片才知道 ⇒ 数据早发出去了，HTTP 层仍是 200 + 全量乱码）。
func TestV4Fragment_BlockCRC_DetectsEarly(t *testing.T) {
	// 8 个块左右，篡改第 1 块 ⇒ 期望在读完第 1 块（约 1/8 处）就报错
	plain := bytes.Repeat([]byte("early-detection"), 64*1024/15*8)
	path, password := createV4PluginPathContainer(t, types.ContainerTypeVideo, plain)

	mf := openV4Manifest(t, path)
	frag := mf.Fragments[0]
	// 篡改第 1 块的中间 1 字节
	off := int64(frag.PhysicalOffset) + int64(writer.FragmentBlockCRCSize)/2
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var one [1]byte
	if _, err := f.ReadAt(one[:], off); err != nil {
		t.Fatalf("read: %v", err)
	}
	one[0] ^= 0x01
	if _, err := f.WriteAt(one[:], off); err != nil {
		t.Fatalf("write: %v", err)
	}
	f.Close()

	// 分段读，记录"报错时已经读出了多少字节"
	factory, err := NewDecryptReaderFactory(path, password)
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	dr, err := factory.NewDecryptReader()
	if err != nil {
		t.Fatalf("NewDecryptReader: %v", err)
	}
	defer dr.Close()

	buf := make([]byte, 16*1024)
	var got int64
	var readErr error
	for {
		var n int
		n, readErr = dr.Read(buf)
		got += int64(n)
		if readErr != nil {
			break
		}
	}
	if readErr == nil {
		t.Fatalf("篡改后竟然一路读到 EOF 且不报错 —— 静默乱码（分块 CRC 没生效）")
	}
	if !errors.Is(readErr, types.ErrDataCorrupted) {
		t.Errorf("期望 ErrDataCorrupted，实际：%v", readErr)
	}
	// 关键断言：**早期**发现（远早于读完整片）
	limit := int64(frag.Length) / 2
	if got > limit {
		t.Errorf("发现得太晚：报错时已读 %d/%d 字节（阈值 %d）⇒ 退回到了整片校验", got, frag.Length, limit)
	}
}

// --- 小工具 ---

func readAllViaFactory(t *testing.T, path, password string, _ []byte) []byte {
	t.Helper()
	factory, err := NewDecryptReaderFactory(path, password)
	if err != nil {
		t.Fatalf("NewDecryptReaderFactory: %v", err)
	}
	defer factory.Close()
	dr, err := factory.NewDecryptReader()
	if err != nil {
		t.Fatalf("NewDecryptReader: %v", err)
	}
	defer dr.Close()
	got, err := io.ReadAll(dr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	return got
}

func readAllViaFactoryErr(t *testing.T, path, password string) ([]byte, error) {
	t.Helper()
	factory, err := NewDecryptReaderFactory(path, password)
	if err != nil {
		return nil, err
	}
	defer factory.Close()
	dr, err := factory.NewDecryptReader()
	if err != nil {
		return nil, err
	}
	defer dr.Close()
	return io.ReadAll(dr)
}
