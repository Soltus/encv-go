// internal/v2/writer/chunked_container_writer.go

package writer

import (
	"encoding/binary"
	"fmt"
	"hash"
	"io"
	"os"

	"github.com/Soltus/encv-go/internal/v2/container/block"
	"github.com/Soltus/encv-go/internal/v2/container/manifest"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// ChunkedContainerWriter 提供构建物理分片容器所需的工具方法
type ChunkedContainerWriter struct {
	globalHasher    hash.Hash32
	lastManifestLen uint64
}

func NewChunkedContainerWriter(globalHasher hash.Hash32) *ChunkedContainerWriter {
	return &ChunkedContainerWriter{
		globalHasher: globalHasher,
	}
}

// WriteDataChunk 将一个数据块写入指定的目标 writer，并返回其 CRC
func (w *ChunkedContainerWriter) WriteDataChunk(targetWriter io.Writer, data []byte) (uint32, error) {
	// 1. 写入文件并获取 CRC
	crcVal, err := block.WriteBlock(targetWriter, types.BlockTypeData_v2, data)
	if err != nil {
		return 0, err
	}

	// 2. 写入全局 Hasher（复用 Header/CRC）
	header := &block.BlockHeader_v2{
		Type:   types.BlockTypeData_v2,
		Length: uint64(len(data)),
		CRC32:  crcVal,
	}
	if err := block.WriteBlockToHasherFromHeader(w.globalHasher, header, data); err != nil {
		return 0, err
	}

	return crcVal, nil
}

// WriteDataChunkFromReader streams a data block to disk and mirrors the same
// serialized block into the global CRC without keeping the fragment in heap.
func (w *ChunkedContainerWriter) WriteDataChunkFromReader(targetFile *os.File, r io.Reader, length uint64) (uint32, error) {
	blockStart, err := targetFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	crcVal, err := block.WriteBlockFromReader_v2(targetFile, types.BlockTypeData_v2, r, length)
	if err != nil {
		return 0, err
	}

	header := &block.BlockHeader_v2{
		Type:   types.BlockTypeData_v2,
		Length: length,
		CRC32:  crcVal,
	}
	if err := binary.Write(w.globalHasher, types.ByteOrder_v2, header); err != nil {
		return 0, err
	}

	end, err := targetFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	dataStart := blockStart + block.GetBlockHeader_v2_Size()
	if _, err := targetFile.Seek(dataStart, io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := io.CopyBuffer(w.globalHasher, io.LimitReader(targetFile, int64(length)), make([]byte, 256*1024)); err != nil {
		return 0, err
	}
	if _, err := targetFile.Seek(end, io.SeekStart); err != nil {
		return 0, err
	}

	return crcVal, nil
}

// WriteManifestAndFooter 将 Manifest 和 Footer 写入主容器文件的末尾
func (w *ChunkedContainerWriter) WriteManifestAndFooter(mainFile *os.File, manifestObj *types.Manifest) error {
	manifestBytes, err := manifestObj.SerializeToJSON()
	if err != nil {
		return err
	}

	// 2. 【关键新增】加密 Manifest
	encryptedManifestBytes, err := manifest.EncryptManifest(manifestBytes)
	if err != nil {
		return fmt.Errorf("failed to encrypt manifest: %w", err)
	}

	// 写入 Manifest 块并获取 CRC
	// 3. 写入加密块到文件 (计算并写入 Header + EncryptedData)
	// WriteBlock 会计算 EncryptedData 的 CRC
	crcVal, err := block.WriteBlock(mainFile, types.BlockTypeManifest_v2, encryptedManifestBytes)
	if err != nil {
		return fmt.Errorf("failed to write manifest block: %w", err)
	}

	// 4. 将加密块 Header 写入 Global Hasher（复用 CRC）
	// 注意：写入 Hasher 的 Header CRC 必须与写入文件的一致
	manifestBlockHeader := &block.BlockHeader_v2{
		Type:   types.BlockTypeManifest_v2,
		Length: uint64(len(encryptedManifestBytes)),
		CRC32:  crcVal,
	}
	if err := block.WriteBlockToHasherFromHeader(w.globalHasher, manifestBlockHeader, encryptedManifestBytes); err != nil {
		return err
	}

	// 5. 写入 Footer
	manifestOffset, err := mainFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}

	// 计算块总大小 (Header + EncryptedData)
	manifestBlockSize := block.GetBlockHeader_v2_Size() + int64(len(encryptedManifestBytes))

	footer := &types.EnvelopeFooter_v2{
		Magic:          types.MagicFooter_v2,
		ManifestOffset: uint64(manifestOffset - manifestBlockSize),
		ManifestLength: uint64(len(encryptedManifestBytes)), // 记录加密长度
		ManifestCRC32:  crcVal,                              // 记录加密数据的 CRC
		GlobalCRC32:    w.globalHasher.Sum32(),
	}
	return binary.Write(mainFile, types.ByteOrder_v2, footer)
}

func (w *ChunkedContainerWriter) WriteManifestOnly(mainFile *os.File, manifestObj *types.Manifest) error {
	manifestBytes, err := manifestObj.SerializeToJSON()
	if err != nil {
		return err
	}

	encryptedManifestBytes, err := manifest.EncryptManifest(manifestBytes)
	if err != nil {
		return fmt.Errorf("failed to encrypt manifest: %w", err)
	}

	crcVal, err := block.WriteBlock(mainFile, types.BlockTypeManifest_v2, encryptedManifestBytes)
	if err != nil {
		return fmt.Errorf("failed to write manifest block: %w", err)
	}

	manifestBlockHeader := &block.BlockHeader_v2{
		Type:   types.BlockTypeManifest_v2,
		Length: uint64(len(encryptedManifestBytes)),
		CRC32:  crcVal,
	}
	if err := block.WriteBlockToHasherFromHeader(w.globalHasher, manifestBlockHeader, encryptedManifestBytes); err != nil {
		return err
	}

	w.lastManifestLen = uint64(len(encryptedManifestBytes))
	return nil
}

func (w *ChunkedContainerWriter) LastManifestLen() uint64 {
	return w.lastManifestLen
}

func (w *ChunkedContainerWriter) GlobalCRC32() uint32 {
	return w.globalHasher.Sum32()
}
