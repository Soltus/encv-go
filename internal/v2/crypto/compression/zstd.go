// Package compression provides compression primitives used by v4 container
// Segments. This file implements the zstd-seekable wrapper that the v4
// writer/reader call before/after AES-CTR encryption (Task 8 of the v4
// container capability upgrade spec).
//
// The wrapper exposes two high-level helpers:
//
//	CompressZstdSeekable       - plaintext io.Reader -> (compressed, seekTable, err)
//	DecompressZstdSeekable     - (compressed, seekTable) -> plaintext []byte
//
// Both helpers are pure in-memory wrappers around the upstream
// github.com/SaveTheRbtz/zstd-seekable-format-go/pkg package. The seek table
// is returned as a separate skippable frame so the segment writer can store
// it inside the segment header layout (see internal/v2/types/segment_v4.go).
//
// Integration with segment encryption (which decides whether to compress
// based on v4_compression_mode and segment size) is implemented separately
// in Task 9. This file MUST stay unaware of the segment/header/container
// layout so it can be unit-tested in isolation.
package compression

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	seekable "github.com/SaveTheRbtz/zstd-seekable-format-go/pkg"
	"github.com/klauspost/compress/zstd"
)

// DefaultZstdBlockSize is the default chunk size used when splitting
// plaintext into multiple zstd frames. 64KB is a good trade-off between
// compression ratio (more redundancy to find) and seek granularity
// (smaller = more frames in the index, larger = coarser random access).
const DefaultZstdBlockSize = 64 * 1024

// CompressZstdSeekable 压缩 src 并返回压缩数据 + seek table。
//
// src 中的明文会被切分为 blockSize 字节的块（最后一块可能更短），每块作为
// 一个独立的 zstd 帧写入 compressed。调用方负责把 compressed 与 seekTable
// 按顺序拼接成完整的 seekable 流：即 compressed 帧在前，seekTable 帧在末尾。
//
// seekTable 是 zstd-seekable 格式中的 "skippable frame"——它可以被标准 zstd
// 解码器跳过，所以返回单独的字节切片让 segment 写入方按需嵌入 SegmentHeader
// 之后的任意位置。
//
// 关键不变量：必须调用 enc.EndStream() 才能得到 seek-table 帧。EndStream
// 之后 encoder 会被关闭，重复调用会返回 seekable.ErrClosed。
func CompressZstdSeekable(src io.Reader) (compressed []byte, seekTable []byte, err error) {
	return CompressZstdSeekableWithBlockSize(src, DefaultZstdBlockSize)
}

// CompressZstdSeekableWithBlockSize 与 CompressZstdSeekable 行为一致，
// 但允许调用方覆盖默认块大小。blockSize <= 0 时回退到 DefaultZstdBlockSize。
//
// blockSize 是每帧压缩前的明文大小。seekable 库内部对单帧大小有
// maxChunkSize (math.MaxUint32) 上限，正常使用不会被触发。
func CompressZstdSeekableWithBlockSize(src io.Reader, blockSize int) (compressed []byte, seekTable []byte, err error) {
	if src == nil {
		return nil, nil, fmt.Errorf("compression: src is nil")
	}
	if blockSize <= 0 {
		blockSize = DefaultZstdBlockSize
	}

	// 构造 zstd encoder。zstd.NewWriter(nil) 返回的 encoder 把压缩结果
	// 通过 EncodeAll(src, dst) 暴露为字节切片（与 seekable.ZSTDEncoder
	// 接口完全一致）。defer enc.Close() 释放内部 CGO 资源。
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, nil, fmt.Errorf("compression: zstd.NewWriter failed: %w", err)
	}
	defer enc.Close()

	// 构造 seekable 编码器。每个 Encode 调用产出一个 zstd 帧并往内部
	// seek-table 追加一条记录。
	se, err := seekable.NewEncoder(enc)
	if err != nil {
		return nil, nil, fmt.Errorf("compression: seekable.NewEncoder failed: %w", err)
	}

	// 把 src 切成 blockSize 大小的块并顺序编码。空块会被库自动跳过（不
	// 产生帧、不追加 seek-table 记录）。
	//
	// io.ReadFull 的 EOF 语义：
	//   * src 已空且未读到任何字节 → (0, io.EOF)
	//   * 读到部分字节但未填满缓冲区 → (n>0, io.ErrUnexpectedEOF)
	//   * 恰好填满缓冲区 → (blockSize, nil)
	chunk := make([]byte, blockSize)
	var out bytes.Buffer
	for {
		n, readErr := io.ReadFull(src, chunk)
		if n > 0 {
			frame, encErr := se.Encode(chunk[:n])
			if encErr != nil {
				return nil, nil, fmt.Errorf("compression: seekable.Encode failed: %w", encErr)
			}
			out.Write(frame)
		}
		// EOF / UnexpectedEOF 都是正常结束信号；其它错误才向上抛。
		if readErr != nil {
			if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
				break
			}
			return nil, nil, fmt.Errorf("compression: read from src failed: %w", readErr)
		}
	}

	// 收尾：必须调 EndStream() 拿 seek-table 帧，否则 Reader 无法重建索引。
	seekTableFrame, err := se.EndStream()
	if err != nil {
		return nil, nil, fmt.Errorf("compression: seekable.EndStream failed: %w", err)
	}

	return out.Bytes(), seekTableFrame, nil
}

// DecompressZstdSeekable 解压 compressed + seekTable 还原成明文。
//
// 调用方负责把 compressed 帧与 seekTable 帧按顺序拼成完整的 seekable 流。
// 拼装既可以是 `append(compressed, seekTable...)`，也可以是
// `bytes.NewReader(combined)`，因为 seekable Reader 只关心流的尾部 skippable
// frame 内的 footer 字段。
//
// 返回的 plaintext 总是从字节 0 开始，完整还原所有原始内容。
func DecompressZstdSeekable(compressed, seekTable []byte) (plaintext []byte, err error) {
	if seekTable == nil {
		return nil, fmt.Errorf("compression: seekTable is nil")
	}
	// 注：compressed 可以为空（明文为空）—— seekable 库会用 0 帧的 seek table
	// 与之配套，Reader 会立即返回 io.EOF。
	if compressed == nil {
		compressed = []byte{}
	}

	// 拼成完整 seekable 流：[compressed frames...][seek-table frame]
	combined := make([]byte, 0, len(compressed)+len(seekTable))
	combined = append(combined, compressed...)
	combined = append(combined, seekTable...)

	// 构造 zstd decoder。zstd.NewReader(nil) 返回的 decoder 通过
	// DecodeAll(input, dst) 暴露——与 seekable.ZSTDDecoder 接口一致。
	// 内部不使用 goroutine，所以这里不用关心并发 DecodeAll 兼容性。
	dec, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("compression: zstd.NewReader failed: %w", err)
	}
	defer dec.Close()

	// seekable.Reader 必须拿到一个 io.ReadSeeker，所以用 bytes.NewReader
	// 包装内存流。Reader 内部对 RS 的使用模式是 Seek+ReadFull 与 ReadAt
	// （当 RS 实现 io.ReaderAt 时走 ReadAt 路径，避免移动 offset）。
	r, err := seekable.NewReader(bytes.NewReader(combined), dec)
	if err != nil {
		return nil, fmt.Errorf("compression: seekable.NewReader failed: %w", err)
	}
	defer r.Close()

	// 一次性读出全部明文。io.ReadAll 通过循环 Read 推动 offset 直至 EOF。
	plaintext, err = io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("compression: seekable ReadAll failed: %w", err)
	}
	return plaintext, nil
}
