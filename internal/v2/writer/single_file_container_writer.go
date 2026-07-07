// internal/v2/writer/single_file_container_writer.go
package writer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"os"

	"github.com/Soltus/encv-go/internal/v2/container/block"
	"github.com/Soltus/encv-go/internal/v2/container/manifest"
	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/filename"
	"github.com/Soltus/encv-go/internal/v2/types"
)

type pendingFragment struct {
	id                string
	typ               types.FragmentType
	length            uint64
	globalStartOffset uint64
	physicalOffset    uint64
	crc32             uint32
	v2DataBuffer      *bytes.Buffer
}

// SingleFileContainerWriter 是 ContainerWriter_v2 的一个具体实现，专用于单文件容器
type SingleFileContainerWriter struct {
	file                    *os.File
	fragments               []types.Fragment
	manifestOffset          uint64
	manifestLength          uint64
	currentDataStreamOffset uint64
	globalHasher            hash.Hash32
	manifestBytes           []byte
	manifestCRC             uint32
	headerVersion           int
	v4Header                *types.EnvelopeHeaderV4
	currentFragment         *pendingFragment

	originalName    string
	fnPassword      string
	fnConfig        filename.FNConfig
	encryptFilename bool

	wrappedDEK *types.WrappedDEK
}

// 创建一个新的文件容器写入器，他会在关闭的时候自动写入 Footer
func NewSingleFileContainerWriter(outputPath string, header *types.EnvelopeHeaderV3) (*SingleFileContainerWriter, error) {
	file, err := os.Create(outputPath)
	if err != nil {
		return nil, err
	}

	headerSize := 0
	if header != nil {
		if err := types.WriteHeaderV3(file, header); err != nil {
			return nil, err
		}
		headerSize = types.EnvelopeHeaderSize_v3
	}

	globalHasher := crc32.NewIEEE()
	if headerSize > 0 {
		headerBytes := make([]byte, headerSize)
		if _, err := file.Seek(0, io.SeekStart); err == nil {
			if _, err := io.ReadFull(file, headerBytes); err == nil {
				globalHasher.Write(headerBytes)
			}
		}
		file.Seek(int64(headerSize), io.SeekStart)
	}

	return &SingleFileContainerWriter{file: file, globalHasher: globalHasher, manifestCRC: 0, headerVersion: 3}, nil
}

func NewSingleFileContainerWriterV4(outputPath string, header *types.EnvelopeHeaderV4) (*SingleFileContainerWriter, error) {
	file, err := os.Create(outputPath)
	if err != nil {
		return nil, err
	}

	headerSize := 0
	if header != nil {
		if err := types.WriteHeaderV4(file, header); err != nil {
			return nil, err
		}
		headerSize = types.EnvelopeHeaderSize_v4
	}

	globalHasher := crc32.NewIEEE()
	if headerSize > 0 {
		headerBytes := make([]byte, headerSize)
		if _, err := file.Seek(0, io.SeekStart); err == nil {
			if _, err := io.ReadFull(file, headerBytes); err == nil {
				globalHasher.Write(headerBytes)
			}
		}
		file.Seek(int64(headerSize), io.SeekStart)
	}

	return &SingleFileContainerWriter{file: file, globalHasher: globalHasher, manifestCRC: 0, headerVersion: 4, v4Header: header}, nil
}

func (w *SingleFileContainerWriter) SetFilenameEncoding(originalName string, password string, cfg filename.FNConfig) {
	w.originalName = originalName
	w.fnPassword = password
	w.fnConfig = cfg
	w.encryptFilename = originalName != "" && password != ""
}

func (w *SingleFileContainerWriter) SetWrappedDEK(wd *types.WrappedDEK) {
	w.wrappedDEK = wd
}

func (w *SingleFileContainerWriter) WriteKVI(kviData []byte) error {
	if w.headerVersion == 4 {
		w.globalHasher.Write(kviData)
		return nil
	}
	crcVal, err := block.WriteBlock(w.file, types.BlockTypeKVI_v2, kviData)
	if err != nil {
		return err
	}
	header := &block.BlockHeader_v2{
		Type:   types.BlockTypeKVI_v2,
		Length: uint64(len(kviData)),
		CRC32:  crcVal,
	}
	return block.WriteBlockToHasherFromHeader(w.globalHasher, header, kviData)
}

func (w *SingleFileContainerWriter) WriteFragment(frag *types.Fragment, data []byte) error {
	if err := w.BeginFragment(frag); err != nil {
		return err
	}
	if err := w.WriteFragmentData(data); err != nil {
		return err
	}
	return w.FinishFragment()
}

func (w *SingleFileContainerWriter) BeginFragment(frag *types.Fragment) error {
	if w.file != nil {
		if pos, err := w.file.Seek(0, io.SeekCurrent); err == nil {
			frag.PhysicalOffset = uint64(pos)
		}
	}

	w.currentFragment = &pendingFragment{
		id:                frag.ID,
		typ:               frag.Type,
		length:            0,
		globalStartOffset: w.currentDataStreamOffset,
		physicalOffset:    frag.PhysicalOffset,
		crc32:             0,
	}

	if w.headerVersion != 4 {
		w.currentFragment.v2DataBuffer = bytes.NewBuffer(nil)
	}

	return nil
}

func (w *SingleFileContainerWriter) WriteFragmentData(data []byte) error {
	if w.currentFragment == nil {
		return fmt.Errorf("BeginFragment must be called before WriteFragmentData")
	}

	if w.headerVersion == 4 {
		if _, err := w.file.Write(data); err != nil {
			return fmt.Errorf("failed to write v4 fragment data: %w", err)
		}
		w.globalHasher.Write(data)
	} else {
		w.currentFragment.v2DataBuffer.Write(data)
	}

	w.currentFragment.length += uint64(len(data))
	w.currentDataStreamOffset += uint64(len(data))
	return nil
}

func (w *SingleFileContainerWriter) FinishFragment() error {
	if w.currentFragment == nil {
		return fmt.Errorf("BeginFragment must be called before FinishFragment")
	}
	defer func() { w.currentFragment = nil }()

	frag := w.currentFragment

	if w.headerVersion == 4 {
		w.fragments = append(w.fragments, types.Fragment{
			ID:                frag.id,
			Type:              frag.typ,
			Length:            frag.length,
			GlobalStartOffset: frag.globalStartOffset,
			DataCRC32:         0,
			PhysicalPath:      "",
			PhysicalOffset:    frag.physicalOffset,
		})
	} else {
		data := frag.v2DataBuffer.Bytes()
		crc, err := block.WriteBlock(w.file, types.BlockTypeData_v2, data)
		if err != nil {
			return fmt.Errorf("failed to write data block: %w", err)
		}
		header := &block.BlockHeader_v2{Type: types.BlockTypeData_v2, Length: uint64(len(data)), CRC32: crc}
		block.WriteBlockToHasherFromHeader(w.globalHasher, header, data)
		w.fragments = append(w.fragments, types.Fragment{
			ID:                frag.id,
			Type:              frag.typ,
			Length:            frag.length,
			GlobalStartOffset: frag.globalStartOffset,
			DataCRC32:         crc,
			PhysicalPath:      "",
			PhysicalOffset:    frag.physicalOffset,
		})
	}

	return nil
}

func (w *SingleFileContainerWriter) WriteManifest(manifestObj *types.Manifest) error {
	manifestObj.Fragments = w.fragments
	if w.headerVersion == 4 {
		return w.writeManifestV4(manifestObj)
	}
	return w.writeManifestV23(manifestObj)
}

// writeManifestV4 使用 V4 格式：XOR 混淆 + 无 Block header 包裹
func (w *SingleFileContainerWriter) writeManifestV4(manifestObj *types.Manifest) error {
	containerTypeStr := containerTypeToString(w.v4Header.ContainerType)

	segments := make([]types.Segment_v4, 0, len(manifestObj.Fragments))
	for _, frag := range manifestObj.Fragments {
		segments = append(segments, types.Segment_v4{
			ID:     frag.ID,
			Offset: frag.PhysicalOffset,
			Size:   frag.Length,
			Nonce:  "",
		})
	}

	mf := &types.Manifest_v4{
		Version:       4,
		ContainerID:   string(w.v4Header.SpecialID[:w.v4Header.IDLength]),
		ContainerType: containerTypeStr,
		IsSeekable:    w.v4Header.IsSeekable != 0,
		Segments:      segments,
		KVI:           manifestObj.KVI,
		WrappedDEK:    w.wrappedDEK,
	}

	if w.encryptFilename && w.originalName != "" {
		w.fnConfig.Password = []byte(w.fnPassword)
		encoded, err := w.fnConfig.Encode([]byte(w.originalName))
		if err == nil {
			mf.OriginalName = encoded
			mf.FilenameAlgorithm = "enc-fn:v1"
			w.v4Header.Flags |= types.FlagFilenameEncrypted
		} else {
			mf.OriginalName = w.originalName
		}
	} else if w.originalName != "" {
		mf.OriginalName = w.originalName
	}

	manifestJSON, err := mf.SerializeToJSON_v4()
	if err != nil {
		return fmt.Errorf("failed to serialize v4 manifest: %w", err)
	}

	obfuscatedManifest, err := crypto.ObfuscateManifest(manifestJSON)
	if err != nil {
		return fmt.Errorf("failed to obfuscate v4 manifest: %w", err)
	}

	if _, err := w.file.Write(obfuscatedManifest); err != nil {
		return fmt.Errorf("failed to write obfuscated v4 manifest: %w", err)
	}

	w.globalHasher.Write(obfuscatedManifest)

	w.manifestBytes = obfuscatedManifest
	w.manifestLength = uint64(len(obfuscatedManifest))
	return nil
}

// writeManifestV23 保持原有 V2/V3 逻辑：AES 加密 + Block header 包裹
func (w *SingleFileContainerWriter) writeManifestV23(manifestObj *types.Manifest) error {
	manifestBytes, err := manifestObj.SerializeToJSON()
	if err != nil {
		return err
	}
	w.manifestBytes = manifestBytes
	w.manifestLength = uint64(len(manifestBytes))

	encryptedManifestBytes, err := manifest.EncryptManifest(manifestBytes)
	if err != nil {
		return err
	}
	w.manifestBytes = encryptedManifestBytes
	w.manifestLength = uint64(len(encryptedManifestBytes))

	crcVal, err := block.WriteBlock(w.file, types.BlockTypeManifest_v2, encryptedManifestBytes)
	if err != nil {
		return err
	}

	manifestBlockHeader := &block.BlockHeader_v2{
		Type:   types.BlockTypeManifest_v2,
		Length: uint64(len(encryptedManifestBytes)),
		CRC32:  crcVal,
	}
	if err := block.WriteBlockToHasherFromHeader(w.globalHasher, manifestBlockHeader, encryptedManifestBytes); err != nil {
		return err
	}

	w.manifestCRC = crcVal
	return nil
}

// containerTypeToString 将 ContainerType uint16 转换为 Manifest_v4 使用的字符串
func containerTypeToString(ct uint16) string {
	switch ct {
	case types.ContainerTypeVideo:
		return "video"
	case types.ContainerTypeAudio:
		return "audio"
	case types.ContainerTypeImage:
		return "image"
	case types.ContainerTypeDocument:
		return "document"
	case types.ContainerTypeText:
		return "text"
	default:
		return "unknown"
	}
}

// Close 写入 Footer 并关闭文件
func (w *SingleFileContainerWriter) Close() error {
	defer w.file.Close()

	fileInfo, err := w.file.Stat()
	if err != nil {
		return err
	}

	var manifestBlockStart int64
	if w.headerVersion == 4 {
		manifestBlockStart = fileInfo.Size() - int64(len(w.manifestBytes))
	} else {
		manifestBlockSize := block.GetBlockHeader_v2_Size() + int64(len(w.manifestBytes))
		manifestBlockStart = fileInfo.Size() - manifestBlockSize
	}

	if w.headerVersion == 4 {
		// V4: manifest 直接写在当前位置，无 Block header 包裹
		w.v4Header.ManifestOffset = uint32(manifestBlockStart)
		w.v4Header.ManifestLength = uint32(w.manifestLength)

		if _, err := w.file.Seek(0, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek to header for v4 rewrite: %w", err)
		}
		if err := types.WriteHeaderV4(w.file, w.v4Header); err != nil {
			return fmt.Errorf("failed to rewrite v4 header: %w", err)
		}

		if _, err := w.file.Seek(0, io.SeekEnd); err != nil {
			return fmt.Errorf("failed to seek to end for v4 footer: %w", err)
		}

		footer := &types.EnvelopeFooterV4{
			Magic:       types.MagicFooter_v2,
			GlobalCRC32: w.globalHasher.Sum32(),
		}
		return types.WriteFooterV4(w.file, footer)
	}

	footer := &types.EnvelopeFooter_v2{
		Magic:          types.MagicFooter_v2,
		ManifestOffset: uint64(manifestBlockStart),
		ManifestLength: w.manifestLength,
		ManifestCRC32:  w.manifestCRC,
		GlobalCRC32:    w.globalHasher.Sum32(),
	}
	return binary.Write(w.file, types.ByteOrder_v2, footer)
}
