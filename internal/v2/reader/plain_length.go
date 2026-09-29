package reader

import (
	"fmt"
	"io"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// PlainLength 求容器的明文总长，**优先不读密文**。
//
// 为什么需要它：为了拿一个长度去把整个容器解密一遍太贵 ——
// 字节来自网络（HTTP Range / 远端源）时尤其离谱：1GB/1MB 段的容器
// 1024 个段头分散在整个文件里，读完就是 1024 个 Round Trip。
//
// 策略（顺序尝试，都不行才退回"解密到底"）：
//  1. segment 栈：抽样首/末段段头，推出统一的段开销
//     (Header + Nonce + MAC + SeekTable)，按 manifest 里的 Size 算术求和；
//     开销不一致 / 声明了 zstd 压缩 / 算出的长度为负 → 不适用。
//  2. 其它情况：返回 errPlainLengthNotComputable，调用方退回读到底。
//
// ⚠️ "字节还没供给"（按需源）会以 NeedBytesError 原样上抛：
// 调用方补字节后重试即可。绝不能在函数内部把它吞掉、退回"读全文"，
// 否则每次重试只多命中一个段头 → O(n²)（实测 1GB 容器 13 万次段头读）。
func PlainLength(info *V4ContainerInfo) (int64, error) {
	if info == nil || info.Src == nil || info.Manifest == nil || len(info.Manifest.Segments) == 0 {
		return 0, errPlainLengthNotComputable
	}
	src, ok := info.Src.(io.ReaderAt)
	if !ok {
		return 0, errPlainLengthNotComputable
	}
	segs := info.Manifest.Segments

	readOverhead := func(seg types.Segment_v4) (int64, error) {
		buf := make([]byte, types.SegmentHeaderSize)
		if _, err := src.ReadAt(buf, int64(seg.Offset)); err != nil {
			return 0, err // 可能是 NeedBytesError，原样上抛
		}
		var hdr types.SegmentHeader
		if err := hdr.UnmarshalBinary(buf); err != nil {
			return 0, fmt.Errorf("解析段头 %s 失败：%w", seg.ID, err)
		}
		if hdr.ModeFlags&types.ModeFlagCompressionZstd != 0 {
			return 0, errPlainLengthNotComputable
		}
		return int64(types.SegmentHeaderSize) + int64(hdr.NonceSize) + int64(hdr.MACSize) + int64(hdr.SeekTableLength), nil
	}

	head, err := readOverhead(segs[0])
	if err != nil {
		return 0, err
	}
	if len(segs) > 1 {
		tail, err := readOverhead(segs[len(segs)-1])
		if err != nil {
			return 0, err
		}
		if tail != head {
			return 0, errPlainLengthNotComputable
		}
	}

	var total int64
	for _, seg := range segs {
		plain := int64(seg.Size) - head
		if plain < 0 {
			return 0, errPlainLengthNotComputable
		}
		total += plain
	}
	return total, nil
}

// errPlainLengthNotComputable：算不出来（不是"缺字节"），调用方应退回读到底。
var errPlainLengthNotComputable = fmt.Errorf("明文长度无法算术求出，需读到底")
