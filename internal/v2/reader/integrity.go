package reader

import (
	"fmt"
	"hash/crc32"
	"io"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// MaxPreVerifySize 吐字节前整片预校验的大小上限（64MB）。
//
// 超过就不预校验：一次顺序读会让首字节明显变慢，而大文件（长视频）本来就是
// 边下边播，损坏交给**分块 CRC** 在读到时中止即可。
const MaxPreVerifySize = 64 << 20

// RawContainerFactory 本地（文件）工厂才有的能力：开一条只读密文的通道。
// 远程/alist 工厂没有这个方法 ⇒ 预校验自动跳过。
type RawContainerFactory interface {
	NewRawContainerReader() (EncryptedContainerReader, error)
}

// VerifyContainerIntegrity 顺序读一遍各数据分片的**密文**并核对 CRC。
//
// 为什么需要它（两层原因）：
//  1. 分块 CRC 只能在**读到损坏块**时中止 —— 那时 HTTP 响应头（200 + Content-Length）
//     已经发出去了，客户端拿到的是"截断的流"，真机表现为"播到一半卡住"，
//     前端和用户都不知道是文件坏了。预校验让我们能在吐第一个字节前返回 422。
//  2. CRC 是**密文**的，用解密后的流算不出同一个值，所以必须另开一条只读密文的通道。
//
// 成本：一次顺序读（不解密）。因此只在 size ≤ MaxPreVerifySize 时才真的干活，
// 且老容器（DataCRC32 == 0）直接跳过 ⇒ 存量容器行为完全不变。
//
// 调用方（server.handleStreamRequest / service.serveEncryptedExternalFile）
// 还需自己判断"请求不带 Range"（带 Range 的拖动/续传不为 1KB 读整片）。
func VerifyContainerIntegrity(factory DecryptReaderFactory, size int64) error {
	if factory == nil || size <= 0 || size > MaxPreVerifySize {
		return nil
	}
	rf, ok := factory.(RawContainerFactory)
	if !ok {
		return nil // 不是本地文件容器（远程/alist 流）⇒ 跳过
	}
	cr, err := rf.NewRawContainerReader()
	if err != nil {
		return nil // 打不开就不拦（把"环境问题"误报成"文件损坏"更糟）
	}
	defer cr.Close()

	for _, frag := range cr.GetFragments() {
		if frag.Type == types.FragmentType_Metadata {
			continue
		}
		if frag.DataCRC32 == 0 {
			return nil // 老容器没有元数据 ⇒ 与改造前行为一致
		}
		rc, err := cr.GetFragmentReader(frag.ID)
		if err != nil {
			return fmt.Errorf("读取分片 %s 失败：%w", frag.ID, err)
		}
		h := crc32.NewIEEE()
		_, copyErr := io.Copy(h, rc)
		rc.Close()
		if copyErr != nil {
			return fmt.Errorf("校验分片 %s 时读取中断：%w", frag.ID, copyErr)
		}
		if got := h.Sum32(); got != frag.DataCRC32 {
			return fmt.Errorf("分片 %s 完整性校验失败（CRC32 期望 %08x，实际 %08x）：%w",
				frag.ID, frag.DataCRC32, got, types.ErrDataCorrupted)
		}
	}
	return nil
}
