package reader

import (
	"fmt"
	"hash/crc32"
	"io"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// crcGuardReader 边读边累计 CRC32，读到分片末尾时与 manifest 里记录的
// DataCRC32 比对；不一致就返回 ErrDataCorrupted，**绝不把损坏的数据当明文吐出去**。
//
// 为什么需要它（2026-10-02 真机实测的教训）：
//   v4 的 fragment 栈布局里数据区是**裸密文**（没有 v2 那种 BlockHeader，
//   `verifyFragmentAt` 那套按 block header 的校验用不上），
//   而写入端此前又把 DataCRC32 写死成 0 ⇒ 容器被篡改/位翻转后：
//     HTTP 返回 200、长度与明文完全一致、内容却是乱码，而且不报任何错
//   （真机表现：文件能列能下载，就是播不出来/花屏，无从下手）。
//
// 校验时机与边界（如实记录，别当全能）：
//   · 只在**从分片起点完整读到分片末尾**时校验（localOffset == 0）。
//     中途 seek 进入分片只读取了部分字节，算不出整片 CRC ⇒ 跳过。
//   · DataCRC32 == 0 的老容器一律跳过（保留"无元数据也能读"的向后兼容）。
//   · 因此：全量读 / 下载 / 从 0 开始的 Range 能发现损坏；
//     从中间拖动的流式播放只能在读到损坏处时中断（仍好过静默乱码）。
type crcGuardReader struct {
	r      io.Reader
	crc    uint32
	want   uint32
	fragID string
	size   uint64 // 分片总长
	read   uint64 // 已读字节

	// 分块校验：每读满 blockSize 就比对该块 CRC（blockCRC 为空时退化成整片校验）
	blockSize uint64
	blocks    []uint32
	blockIdx  int
	blockCRC  uint32
	blockRead uint64
}

func newCRCGuardReader(r io.Reader, want uint32, fragID string, size uint64) *crcGuardReader {
	return &crcGuardReader{r: r, crc: 0, want: want, fragID: fragID, size: size}
}

// withBlocks 装上分块 CRC 表（写入端默认会写，老容器没有 ⇒ 不装）
func (c *crcGuardReader) withBlocks(blockSize uint64, blocks []uint32) *crcGuardReader {
	if blockSize > 0 && len(blocks) > 0 {
		c.blockSize = blockSize
		c.blocks = blocks
	}
	return c
}

func (c *crcGuardReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.crc = crc32.Update(c.crc, crc32.IEEETable, p[:n])
		c.read += uint64(n)
		if blkErr := c.feedBlocks(p[:n]); blkErr != nil {
			// 【早期发现】块校验不过就立刻停：流式播放时这才有可能在
			// 损坏数据全部发出去之前中止传输（整片 CRC 做不到）。
			return n, blkErr
		}
	}

	// 判定 1：正常读到分片末尾（EOF）
	if err == io.EOF && c.want != 0 && c.crc != c.want {
		return n, c.mismatch()
	}

	// 判定 2：读满整片就判定，不等 io.EOF。
	//
	// ⚠️ 为什么必须有这条：HTTP 侧是 io.Copy(w, io.LimitReader(reader, contentLength))，
	//    当 contentLength 正好等于明文长度时，LimitReader 会在读满后直接返回 EOF，
	//    **根本不会再调底层 reader 一次** ⇒ 底层那次 (0, io.EOF) 永远不会发生
	//    ⇒ 只靠判定 1，大文件（流式分支）的校验**永远不触发**，
	//    后端还会若无其事地记一条 "Successfully served N bytes"（实测 8.6MB 样例）。
	if c.size > 0 && c.read >= c.size && c.want != 0 && c.crc != c.want {
		return n, c.mismatch()
	}

	return n, err
}

// feedBlocks 按块边界累计 CRC，读满一块就比对。
func (c *crcGuardReader) feedBlocks(data []byte) error {
	if c.blockSize == 0 || len(c.blocks) == 0 {
		return nil
	}
	for len(data) > 0 {
		room := c.blockSize - c.blockRead
		take := uint64(len(data))
		if take > room {
			take = room
		}
		c.blockCRC = crc32.Update(c.blockCRC, crc32.IEEETable, data[:take])
		c.blockRead += take
		data = data[take:]
		if c.blockRead < c.blockSize {
			continue
		}
		if c.blockIdx < len(c.blocks) && c.blocks[c.blockIdx] != 0 && c.blockCRC != c.blocks[c.blockIdx] {
			return fmt.Errorf(
				"fragment '%s' 第 %d 个数据块（起点 %d，%d 字节）完整性校验失败（CRC32 期望 %08x，实际 %08x）：%w",
				c.fragID, c.blockIdx, uint64(c.blockIdx)*c.blockSize, c.blockSize,
				c.blocks[c.blockIdx], c.blockCRC, types.ErrDataCorrupted)
		}
		c.blockIdx++
		c.blockCRC = 0
		c.blockRead = 0
	}
	return nil
}

func (c *crcGuardReader) mismatch() error {
	return fmt.Errorf(
		"fragment '%s' 数据完整性校验失败（CRC32 期望 %08x，实际 %08x，已读 %d/%d 字节）：%w",
		c.fragID, c.want, c.crc, c.read, c.size, types.ErrDataCorrupted)
}

// crcGuardFor 决定本次读取要不要套 CRC 校验。
// 返回原 reader 表示不需要（老容器无元数据 / 不是从分片起点开始读）。
func crcGuardFor(r io.Reader, frag *types.Fragment, localOffset uint64) io.Reader {
	if frag.DataCRC32 == 0 || localOffset != 0 {
		return r
	}
	return newCRCGuardReader(r, frag.DataCRC32, frag.ID, frag.Length).
		withBlocks(frag.BlockCRCSize, frag.BlockCRC32)
}
