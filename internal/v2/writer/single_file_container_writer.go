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
	// 分块 CRC：每写满 FragmentBlockCRCSize 就落一个块 CRC
	blockCRC    hash.Hash32
	blockAcc    uint64
	blockCRCs   []uint32
}

// FragmentBlockCRCSize 分块 CRC 的块大小（64KB）。
//
// 取这个值的原因：足够小（8.6MB 视频在损坏处 64KB 内就能发现，不会把整片吐完），
// 又足够大（元数据体积 = 块数 × 4B ≈ 文件/16KB，1GB 电影也才 64KB 元数据）。
const FragmentBlockCRCSize = 64 * 1024

// appendBlockCRC 按 FragmentBlockCRCSize 切块累计 CRC，写满一块就落一个值。
//
// 分块的意义：整片 CRC 要读完整片才能判定，而流式播放读到那时数据早发给客户端了
// （HTTP 已经是 200 + 全量乱码）。分块后读取端每读满一块就能校，损坏处立刻报错。
func (p *pendingFragment) appendBlockCRC(data []byte) {
	if p.blockCRC == nil {
		p.blockCRC = crc32.NewIEEE()
	}
	for len(data) > 0 {
		room := FragmentBlockCRCSize - int(p.blockAcc)
		take := len(data)
		if take > room {
			take = room
		}
		p.blockCRC.Write(data[:take])
		p.blockAcc += uint64(take)
		data = data[take:]
		if p.blockAcc >= FragmentBlockCRCSize {
			p.blockCRCs = append(p.blockCRCs, p.blockCRC.Sum32())
			p.blockCRC.Reset()
			p.blockAcc = 0
		}
	}
}

// finishBlockCRCs 收尾：不足一块的余数也要落一个 CRC
func (p *pendingFragment) finishBlockCRCs() (uint64, []uint32) {
	if len(p.blockCRCs) == 0 && p.blockAcc == 0 {
		return 0, nil
	}
	if p.blockAcc > 0 {
		p.blockCRCs = append(p.blockCRCs, p.blockCRC.Sum32())
		p.blockAcc = 0
	}
	return FragmentBlockCRCSize, p.blockCRCs
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
		blockCRC:          crc32.NewIEEE(),
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
		// 【完整性】v4 也累计分片 CRC，FinishFragment 时写进 manifest。
		// 修前 v4 分支把 DataCRC32 写死成 0 ⇒ 容器里**没有任何完整性元数据**，
		// 位翻转/截断后读取端无从校验 ⇒ 200 + 长度正确 + 内容乱码（静默损坏）。
		w.currentFragment.crc32 = crc32.Update(w.currentFragment.crc32, crc32.IEEETable, data)
		w.currentFragment.appendBlockCRC(data)
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
		// 【完整性】写入真实 CRC（修前恒为 0，见 WriteFragmentData 的注释）。
		// 0 是"老容器/无元数据"的保留值：读取端只在 DataCRC32 != 0 时校验，
		// 所以这一改动**不影响存量容器**的可读性。
		blockSize, blockCRCs := frag.finishBlockCRCs()
		w.fragments = append(w.fragments, types.Fragment{
			ID:                frag.id,
			Type:              frag.typ,
			Length:            frag.length,
			GlobalStartOffset: frag.globalStartOffset,
			DataCRC32:         frag.crc32,
			BlockCRCSize:      blockSize,
			BlockCRC32:        blockCRCs,
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
		// 【完整性】把分片 CRC / 分块 CRC 带进 v4 manifest：读取端是从 v4 manifest 反推
		// v2 fragment 的（AdaptV4ToV2），这里不带过去，读出来就还是 0。
		segments = append(segments, types.Segment_v4{
			ID:           frag.ID,
			Offset:       frag.PhysicalOffset,
			Size:         frag.Length,
			Nonce:        "",
			DataCRC32:    frag.DataCRC32,
			BlockCRCSize: frag.BlockCRCSize,
			BlockCRC32:   frag.BlockCRC32,
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
