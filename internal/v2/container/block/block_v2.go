// internal/v2/container/block_v2.go
package block

import (
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"log/slog"

	"github.com/Soltus/encv-go/internal/logger"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// blockLogger 是 block 包的日志记录器
var blockLogger = logger.WithComponent("block")

type BlockHeader_v2 struct {
	Type   uint16
	Length uint64
	CRC32  uint32
}

func GetBlockHeader_v2_Size() int64 {
	size := int64(binary.Size(BlockHeader_v2{}))
	if size == 0 {
		blockLogger.Error("critical structural definition error",
			slog.String("error", "GetBlockHeader_v2_Size() returned 0"),
			slog.String("detail", "BlockHeader_v2 struct might be malformed or zero-sized"),
		)
		panic("FATAL: GetBlockHeader_v2_Size() returned 0")
	}
	return size
}

// ReadBlockHeader 从当前位置读取一个块头
func ReadBlockHeader(r io.Reader) (*BlockHeader_v2, error) {
	var header BlockHeader_v2
	err := binary.Read(r, types.ByteOrder_v2, &header)
	if err != nil {
		return nil, fmt.Errorf("failed to read block header: %w", err)
	}
	return &header, nil
}

// ReadBlockData 读取块的数据并校验CRC
func ReadBlockData(r io.Reader, header *BlockHeader_v2) ([]byte, error) {
	data := make([]byte, header.Length)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read block data: %w", err)
	}

	if actualCRC := crc32.ChecksumIEEE(data); actualCRC != header.CRC32 {
		return nil, fmt.Errorf("block data CRC32 mismatch (expected %08x, got %08x)", header.CRC32, actualCRC)
	}

	return data, nil
}

// WriteBlock 写入一个完整的块 (头 + 数据)
// 返回计算出的数据 CRC32，避免上层重复计算
func WriteBlock(w io.Writer, blockType uint16, data []byte) (uint32, error) {
	crc := crc32.ChecksumIEEE(data) // 【只计算一次】

	header := &BlockHeader_v2{
		Type:   blockType,
		Length: uint64(len(data)),
		CRC32:  crc,
	}

	if err := binary.Write(w, types.ByteOrder_v2, header); err != nil {
		return 0, err
	}

	// 检查数据部分是否完整写入
	n, err := w.Write(data)
	if n != len(data) {
		// 即使 err 为 nil，如果字节数不对，也视为错误（Short Write）
		return 0, fmt.Errorf("short write: expected %d bytes for data, got %d", len(data), n)
	}
	if err != nil {
		return 0, err
	}
	return crc, err
}

// WriteBlockFromReader_v2 writes a block without materializing the whole payload
// in memory. The writer must be seekable so the CRC can be patched into the
// block header after streaming the payload.
func WriteBlockFromReader_v2(w io.WriteSeeker, blockType uint16, r io.Reader, length uint64) (uint32, error) {
	start, err := w.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, fmt.Errorf("failed to get block start offset: %w", err)
	}

	header := &BlockHeader_v2{
		Type:   blockType,
		Length: length,
		CRC32:  0,
	}
	if err := binary.Write(w, types.ByteOrder_v2, header); err != nil {
		return 0, err
	}

	hasher := crc32.NewIEEE()
	limited := &io.LimitedReader{R: r, N: int64(length)}
	n, err := io.CopyBuffer(io.MultiWriter(w, hasher), limited, make([]byte, 256*1024))
	if err != nil {
		return 0, err
	}
	if n != int64(length) || limited.N != 0 {
		return 0, fmt.Errorf("short read: expected %d bytes for data, got %d", length, n)
	}

	end, err := w.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, fmt.Errorf("failed to get block end offset: %w", err)
	}

	crc := hasher.Sum32()
	header.CRC32 = crc
	if _, err := w.Seek(start, io.SeekStart); err != nil {
		return 0, fmt.Errorf("failed to seek to block header: %w", err)
	}
	if err := binary.Write(w, types.ByteOrder_v2, header); err != nil {
		return 0, err
	}
	if _, err := w.Seek(end, io.SeekStart); err != nil {
		return 0, fmt.Errorf("failed to restore block end offset: %w", err)
	}

	return crc, nil
}

// WriteBlockToHasher 将 Header 和数据写入哈希器
func WriteBlockToHasherFromHeader(hasher hash.Hash32, header *BlockHeader_v2, data []byte) error {
	// 将头部字节写入哈希器
	if err := binary.Write(hasher, types.ByteOrder_v2, header); err != nil {
		return err
	}
	// 将数据字节写入哈希器
	_, err := hasher.Write(data)
	return err
}
