package physical

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/Soltus/encv-go/internal/v2/container/block"
	"github.com/Soltus/encv-go/internal/v2/container/manifest"
	"github.com/Soltus/encv-go/internal/v2/namer"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/internal/v2/writer"
)

// FileChunkerPhysicalPacker 使用文件分片器进行物理打包
type FileChunkerPhysicalPacker struct {
	chunkSize int64            // 标定的物理分片大小（软限制）
	namer     namer.ChunkNamer // 【关键修改】注入命名器
}

func NewFileChunkerPhysicalPacker(chunkSize int64, namer namer.ChunkNamer) *FileChunkerPhysicalPacker {
	return &FileChunkerPhysicalPacker{
		chunkSize: chunkSize,
		namer:     namer,
	}
}

// Pack 实现 PhysicalPacker 接口
func (p *FileChunkerPhysicalPacker) Pack(manifest *types.Manifest, req *PackRequest) (string, error) {
	mainHeader, chunkHeader, mainHeaderV4, chunkHeaderV4, err := p.prepareHeaders(req.HeaderVersion, req.ContainerType, req.IsSeekable, req.SpecialIDType, req.SpecialID, req.PasswordHint)
	if err != nil {
		return "", fmt.Errorf("failed to prepare headers: %w", err)
	}

	mainFile, err := p.prepareMainFile(req, mainHeader, mainHeaderV4)
	if err != nil {
		return "", fmt.Errorf("failed to prepare main file: %w", err)
	}
	defer p.cleanup(mainFile, err)

	globalHasher := crc32.NewIEEE()
	headerSize := 0
	if req.HeaderVersion == 4 && mainHeaderV4 != nil {
		headerSize = types.EnvelopeHeaderSize_v4
	} else if mainHeader != nil {
		headerSize = types.EnvelopeHeaderSize_v3
	}
	if headerSize > 0 {
		headerBytes := make([]byte, headerSize)
		if _, err := mainFile.Seek(0, io.SeekStart); err == nil {
			if _, err := io.ReadFull(mainFile, headerBytes); err == nil {
				globalHasher.Write(headerBytes)
			}
		}
		mainFile.Seek(int64(headerSize), io.SeekStart)
	}

	chunkedWriter := writer.NewChunkedContainerWriter(globalHasher)
	chunkContext := &chunkContext{
		chunkHeader:      chunkHeader,
		chunkHeaderV4:    chunkHeaderV4,
		chunkedWriter:    chunkedWriter,
		currentPartIndex: 0,
		currentPartSize:  0,
		headerVersion:    req.HeaderVersion,
		containerType:    req.ContainerType,
		isSeekable:       req.IsSeekable,
		specialIDType:    req.SpecialIDType,
		specialID:        req.SpecialID,
		passwordHint:     req.PasswordHint,
	}

	for i := range manifest.Fragments {
		frag := &manifest.Fragments[i]
		if err := p.processFragment(mainFile, req, frag, chunkContext); err != nil {
			return "", fmt.Errorf("failed to process fragment '%s': %w", frag.ID, err)
		}
	}

	return p.finalize(mainFile, manifest, chunkContext, req)
}

type chunkContext struct {
	currentPartFile  *os.File
	currentPartIndex int
	currentPartSize  int64
	chunkHeader      *types.EnvelopeHeaderV3
	chunkHeaderV4    *types.EnvelopeHeaderV4
	chunkedWriter    *writer.ChunkedContainerWriter
	headerVersion    int
	containerType    uint16
	isSeekable       bool
	specialIDType    types.IDType
	specialID        []byte
	passwordHint     [16]byte
}

func (p *FileChunkerPhysicalPacker) prepareHeaders(v int, containerType uint16, isSeekable bool, t types.IDType, d []byte, passwordHint [16]byte) (*types.EnvelopeHeaderV3, *types.EnvelopeHeaderV3, *types.EnvelopeHeaderV4, *types.EnvelopeHeaderV4, error) {
	if v == 4 {
		mainH, err := types.CreateHeaderV4(true, containerType, isSeekable, t, d, passwordHint)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		chunkH, err := types.CreateHeaderV4(false, containerType, isSeekable, t, mainH.SpecialID[:mainH.IDLength], passwordHint)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		return nil, nil, mainH, chunkH, nil
	}
	if v == 3 {
		h, err := types.CreateHeaderV3(true, t, d)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		c, err := types.CreateHeaderV3(false, t, h.SpecialID[:h.IDLength])
		return h, c, nil, nil, err
	}
	return nil, nil, nil, nil, nil
}

func (p *FileChunkerPhysicalPacker) prepareMainFile(req *PackRequest, header *types.EnvelopeHeaderV3, headerV4 *types.EnvelopeHeaderV4) (*os.File, error) {
	path := filepath.Join(req.OutputDir, req.Namer.GenerateMainChunkName(req.BaseName)) + ".tmp"
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if headerV4 != nil {
		if err := types.WriteHeaderV4(f, headerV4); err != nil {
			f.Close()
			return nil, err
		}
	} else if header != nil {
		if err := types.WriteHeaderV3(f, header); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func (p *FileChunkerPhysicalPacker) cleanup(f *os.File, err error) {
	f.Close()
	if err != nil {
		os.Remove(f.Name())
	}
}

// processFragment 处理单个分片
func (p *FileChunkerPhysicalPacker) processFragment(mainFile *os.File, req *PackRequest, frag *types.Fragment, ctx *chunkContext) error {
	// 【关键修复】跳过 Metadata Fragments (如 KVI)
	// Unpacker 在重建时也会跳过这些 Fragments，为了保持哈希一致性，Packer 也不应写入数据流
	if frag.Type == types.FragmentType_Metadata {
		log.Printf("DEBUG: Skipping metadata fragment '%s' (not part of data stream)", frag.ID)
		return nil
	}

	// 1. 获取 Writer (处理文件切换、Path 更新、Offset 记录)
	activeWriter, err := p.getActiveWriter(mainFile, req, frag, ctx)
	if err != nil {
		return err
	}

	// 2. 写入数据
	crc, err := ctx.chunkedWriter.WriteDataChunkFromReader(activeWriter, req.EncryptedDataReader, frag.Length)
	if err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}

	// 3. 更新状态
	ctx.currentPartSize += block.GetBlockHeader_v2_Size() + int64(frag.Length)
	frag.DataCRC32 = crc
	return nil
}

// getActiveWriter 获取目标 Writer 并更新元数据
func (p *FileChunkerPhysicalPacker) getActiveWriter(mainFile *os.File, req *PackRequest, frag *types.Fragment, ctx *chunkContext) (*os.File, error) {
	// 【修正】将 activeFile 重命名为 activeWriter，避免与 return 语句不匹配
	activeWriter := mainFile
	needsSwitch := false

	// 轻量主分片模式
	if ctx.currentPartIndex == 0 && req.LightMainChunkEnabled {
		needsSwitch = true
	} else {
		needsSwitch = ctx.currentPartSize > 0 && (ctx.currentPartSize+int64(frag.Length) > p.chunkSize)
	}

	if needsSwitch {
		// 关闭旧文件
		if ctx.currentPartFile != nil {
			ctx.currentPartFile.Close()
		}

		// 打开新文件
		ctx.currentPartIndex++
		ctx.currentPartSize = 0

		chunkName := req.Namer.GenerateDataChunkName(req.BaseName, ctx.currentPartIndex)
		chunkPath := filepath.Join(req.OutputDir, chunkName)
		f, err := os.Create(chunkPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create part file: %w", err)
		}

		if ctx.chunkHeaderV4 != nil {
			if err := types.WriteHeaderV4(f, ctx.chunkHeaderV4); err != nil {
				f.Close()
				return nil, err
			}
		} else if ctx.chunkHeader != nil {
			if err := types.WriteHeaderV3(f, ctx.chunkHeader); err != nil {
				f.Close()
				return nil, err
			}
		}

		ctx.currentPartFile = f
		if ctx.headerVersion == 4 {
			ctx.currentPartSize = int64(types.EnvelopeHeaderSize_v4)
		} else {
			ctx.currentPartSize = int64(binary.Size(types.EnvelopeHeaderV3{}))
		}
		activeWriter = f

		// 更新 Manifest
		frag.PhysicalPath = chunkName
	} else {
		if ctx.currentPartFile != nil {
			activeWriter = ctx.currentPartFile
			// 如果在分片中，更新 Path
			frag.PhysicalPath = req.Namer.GenerateDataChunkName(req.BaseName, ctx.currentPartIndex)
		} else {
			activeWriter = mainFile
			// 主文件，清空 Path
			frag.PhysicalPath = ""
		}
	}

	// 【关键】记录 PhysicalOffset (在 WriteDataChunk 之前)
	// activeWriter 是 *os.File，可以直接 Seek
	if pos, err := activeWriter.Seek(0, io.SeekCurrent); err == nil {
		frag.PhysicalOffset = uint64(pos)
	}

	return activeWriter, nil
}

func (p *FileChunkerPhysicalPacker) finalize(mainFile *os.File, manifest *types.Manifest, ctx *chunkContext, req *PackRequest) (string, error) {
	if ctx.currentPartFile != nil {
		ctx.currentPartFile.Close()
	}

	if ctx.headerVersion == 4 {
		if err := ctx.chunkedWriter.WriteManifestOnly(mainFile, manifest); err != nil {
			return "", err
		}

		manifestOffset, err := mainFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return "", err
		}
		manifestBlockSize := block.GetBlockHeader_v2_Size() + int64(ctx.chunkedWriter.LastManifestLen())
		actualManifestOffset := manifestOffset - manifestBlockSize

		mainHeaderV4, err := types.CreateHeaderV4(true, ctx.containerType, ctx.isSeekable, ctx.specialIDType, ctx.specialID, ctx.passwordHint)
		if err != nil {
			return "", fmt.Errorf("failed to recreate v4 header for finalize: %w", err)
		}
		mainHeaderV4.ManifestOffset = uint32(actualManifestOffset + block.GetBlockHeader_v2_Size())
		mainHeaderV4.ManifestLength = uint32(ctx.chunkedWriter.LastManifestLen())

		if _, err := mainFile.Seek(0, io.SeekStart); err != nil {
			return "", fmt.Errorf("failed to seek to header for v4 rewrite: %w", err)
		}
		if err := types.WriteHeaderV4(mainFile, mainHeaderV4); err != nil {
			return "", fmt.Errorf("failed to rewrite v4 header: %w", err)
		}

		if _, err := mainFile.Seek(0, io.SeekEnd); err != nil {
			return "", fmt.Errorf("failed to seek to end for v4 footer: %w", err)
		}

		footer := &types.EnvelopeFooterV4{
			Magic:       types.MagicFooter_v2,
			GlobalCRC32: ctx.chunkedWriter.GlobalCRC32(),
		}
		if err := types.WriteFooterV4(mainFile, footer); err != nil {
			return "", fmt.Errorf("failed to write v4 footer: %w", err)
		}
	} else {
		if err := ctx.chunkedWriter.WriteManifestAndFooter(mainFile, manifest); err != nil {
			return "", err
		}
	}

	if err := mainFile.Close(); err != nil {
		return "", err
	}

	basePath := req.Namer.GenerateMainChunkName(req.BaseName)
	finalPath := filepath.Join(req.OutputDir, basePath)
	if err := os.Rename(finalPath+".tmp", finalPath); err != nil {
		return "", err
	}

	log.Printf("✅ [FileChunker] Packed to: %s (Parts: %d)\n", finalPath, ctx.currentPartIndex)
	return finalPath, nil
}

// FileChunkerPhysicalUnpacker 使用文件分片器进行物理解包
type FileChunkerPhysicalUnpacker struct {
	namer namer.ChunkNamer // 【关键修改】注入命名器
}

func NewFileChunkerPhysicalUnpacker(namer namer.ChunkNamer) *FileChunkerPhysicalUnpacker {
	return &FileChunkerPhysicalUnpacker{namer: namer}
}

// Unpack 实现 PhysicalUnpacker 接口
// 【关键修复】此函数现在使用统一的逻辑来重建任何类型的容器
func (u *FileChunkerPhysicalUnpacker) Unpack(mainContainerPath string) (string, func(), error) {
	// 1. 准备工作：解析名称，创建临时文件
	baseName, err := u.namer.ParseFirstChunkName(mainContainerPath)
	if err != nil {
		// 如果解析失败，说明它可能不是标准分片文件，我们使用文件本身作为 baseName
		baseName = mainContainerPath
	}

	tempFile, err := os.CreateTemp(filepath.Dir(baseName), filepath.Base(baseName)+"_unified_*.tmp")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	unifiedPath := tempFile.Name()

	cleanup := func() {
		log.Printf("DEBUG: [Unpack] Cleaning up temp file: %s\n", unifiedPath)
		tempFile.Close()
		os.Remove(unifiedPath)
	}

	// 2. 发现：找到并解析 Manifest
	originalManifest, err := u.findAndParseManifest(mainContainerPath)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to find and parse manifest: %w", err)
	}

	// 3. 【核心】统一的重建逻辑：根据原始 Manifest 重建一个完整的、连续的容器
	newManifestBytes, err := u.rebuildToSingleFile(mainContainerPath, originalManifest, tempFile)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to rebuild container: %w", err)
	}

	// 4. 定稿：写入新的 Footer
	if err := u.writeFinalFooter(tempFile, newManifestBytes); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to write final footer: %w", err)
	}

	// 成功，关闭文件并返回清理函数
	tempFile.Close()
	log.Printf("DEBUG: [Unpacker] Successfully unified container to: %s\n", unifiedPath)
	return unifiedPath, cleanup, nil
}

// findAndParseManifest 封装了查找和解析 Manifest 的逻辑
func (u *FileChunkerPhysicalUnpacker) findAndParseManifest(mainContainerPath string) (*types.Manifest, error) {
	// 尝试从 Footer 读取
	if footer, err := readFooter(mainContainerPath); err == nil {
		manifestBytes, err := manifest.ReadManifestAt(mainContainerPath, int64(footer.ManifestOffset), int64(footer.ManifestLength))
		if err == nil {
			var manifest types.Manifest
			if err := json.Unmarshal(manifestBytes, &manifest); err == nil {
				return &manifest, nil
			}
		}
	}
	// 备用方案：线性扫描
	manifestBytes, _, err := extractManifestWithScan(mainContainerPath)
	if err != nil {
		return nil, err
	}
	var manifest types.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// rebuildToSingleFile 【核心修复】统一的重建函数
// 它遍历原始 Manifest 的所有 fragments，将它们的数据从各自的物理位置读取出来，
// 连续地写入到目标文件，并生成一个描述新布局的、正确的 Manifest。
func (u *FileChunkerPhysicalUnpacker) rebuildToSingleFile(sourcePath string, originalManifest *types.Manifest, destFile *os.File) ([]byte, error) {
	containerDir := filepath.Dir(sourcePath)

	// --- 1. 将源文件的 Header 复制到重建文件中 ---
	// 重建的容器必须包含 Header 才能被正确识别和解析
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source file to read header: %w", err)
	}
	defer sourceFile.Close()

	version, headerSize, err := types.DetectHeaderInfoFromReaderAt(sourceFile)
	if err != nil {
		return nil, fmt.Errorf("failed to detect header info from source: %w", err)
	}

	// 如果源文件有 Header (V3 通常有 2048 字节)，则写入重建文件
	if headerSize > 0 {
		log.Printf("DEBUG: [Unpacker] Source file has V%d Header (%d bytes). Copying to unified file.", version, headerSize)
		headerBytes := make([]byte, headerSize)
		if _, err := io.ReadFull(sourceFile, headerBytes); err != nil {
			return nil, fmt.Errorf("failed to read header bytes from source: %w", err)
		}
		if _, err := destFile.Write(headerBytes); err != nil {
			return nil, fmt.Errorf("failed to write header to dest file: %w", err)
		}
	}
	// -------------------------------------------------------

	// 1. 创建一个新的 Manifest，复制 KVI 等元数据
	newManifest := &types.Manifest{
		Version:    originalManifest.Version,
		KVI:        originalManifest.KVI,
		Redundancy: originalManifest.Redundancy,
	}

	var dataStreamOffset uint64 = 0

	// 2. 遍历原始 Manifest 的 fragments
	for _, frag := range originalManifest.Fragments {
		switch frag.Type {
		case types.FragmentType_SeekableStream:
			// --- 处理 SeekableStream Fragment (原有逻辑) ---
			var physicalPath string
			if frag.PhysicalPath == "" {
				physicalPath = sourcePath // 在主文件中
			} else {
				physicalPath = filepath.Join(containerDir, frag.PhysicalPath) // 在 .part 文件中
			}

			physicalFile, err := os.Open(physicalPath)
			if err != nil {
				return nil, fmt.Errorf("failed to open physical file '%s' for fragment '%s': %w", physicalPath, frag.ID, err)
			}
			defer physicalFile.Close()

			if _, err := physicalFile.Seek(int64(frag.PhysicalOffset), io.SeekStart); err != nil {
				return nil, fmt.Errorf("failed to seek to physical offset for fragment '%s': %w", frag.ID, err)
			}

			var header block.BlockHeader_v2
			if err := binary.Read(physicalFile, binary.LittleEndian, &header); err != nil {
				return nil, fmt.Errorf("failed to read block header for fragment '%s': %w", frag.ID, err)
			}
			if header.Type != types.BlockTypeData_v2 {
				return nil, fmt.Errorf("unexpected block type %d for fragment '%s'", header.Type, frag.ID)
			}
			if header.Length != frag.Length {
				return nil, fmt.Errorf("unexpected block length for fragment '%s': manifest=%d block=%d", frag.ID, frag.Length, header.Length)
			}

			chunkData := make([]byte, header.Length)
			if _, err := io.ReadFull(physicalFile, chunkData); err != nil {
				return nil, fmt.Errorf("failed to read data for fragment '%s': %w", frag.ID, err)
			}

			// 获取写入时计算的 CRC，确保 Manifest CRC 与磁盘 Header 一致
			crcVal, err := block.WriteBlock(destFile, types.BlockTypeData_v2, chunkData)
			if err != nil {
				return nil, fmt.Errorf("failed to write data block for fragment '%s': %w", frag.ID, err)
			}

			newFrag := types.Fragment{
				ID:                frag.ID,
				Type:              frag.Type,
				Length:            header.Length,
				GlobalStartOffset: dataStreamOffset,
				DataCRC32:         crcVal, // 使用实际写入计算出的 CRC
			}
			newManifest.Fragments = append(newManifest.Fragments, newFrag)
			dataStreamOffset += header.Length

		case types.FragmentType_AtomicFile:
			// --- 处理 AtomicFile Fragment ---
			// AtomicFile 的数据是连续存储的，没有额外的块头结构。
			// 我们直接根据 manifest 中的 GlobalStartOffset 和 Length 读取数据。
			sourceFile, err := os.Open(sourcePath)
			if err != nil {
				return nil, fmt.Errorf("failed to open source file for atomic fragment '%s': %w", frag.ID, err)
			}
			defer sourceFile.Close()

			// 使用 SectionReader 精确读取 Fragment 数据
			sectionReader := io.NewSectionReader(sourceFile, int64(frag.GlobalStartOffset), int64(frag.Length))
			chunkData := make([]byte, frag.Length)
			if _, err := io.ReadFull(sectionReader, chunkData); err != nil {
				return nil, fmt.Errorf("failed to read data for atomic fragment '%s': %w", frag.ID, err)
			}

			// 将数据写入目标文件
			crcVal, err := block.WriteBlock(destFile, types.BlockTypeData_v2, chunkData)
			if err != nil {
				return nil, fmt.Errorf("failed to write data block for fragment '%s': %w", frag.ID, err)
			}

			newFrag := types.Fragment{
				ID:                frag.ID,
				Type:              frag.Type,
				Length:            frag.Length,
				GlobalStartOffset: dataStreamOffset,
				DataCRC32:         crcVal, // 复用计算结果，移除 crc32.ChecksumIEEE 调用
			}
			newManifest.Fragments = append(newManifest.Fragments, newFrag)
			dataStreamOffset += frag.Length
		case types.FragmentType_Metadata:
			// KVI 等元数据已经作为 newManifest.KVI 被包含在新的 Manifest 中了
			// 它们会在最后被统一写入，不需要作为数据流的一部分重建
			continue // 跳过此 fragment，处理下一个
		default:
			// --- 未知或不支持的 Fragment 类型 ---
			return nil, fmt.Errorf("encountered unsupported or unknown fragment type: %v for fragment ID: %s", frag.Type, frag.ID)
		}

	}

	// 3. 将新创建的 Manifest 序列化为 JSON
	newManifestBytes, err := json.Marshal(newManifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal new manifest: %w", err)
	}

	// 4. 加密 Manifest (使用 manifest.EncryptManifest)
	encryptedManifestBytes, err := manifest.EncryptManifest(newManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt manifest: %w", err)
	}

	// 5. 将新的 Manifest 块写入到临时文件末尾
	if _, err := block.WriteBlock(destFile, types.BlockTypeManifest_v2, encryptedManifestBytes); err != nil {
		return nil, fmt.Errorf("failed to write new manifest block to unified file: %w", err)
	}

	return newManifestBytes, nil
}

// writeFinalFooter 计算并写入最终的 Footer
func (u *FileChunkerPhysicalUnpacker) writeFinalFooter(file *os.File, manifestBytes []byte) error {
	currentEOF, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to get current file size: %w", err)
	}

	manifestBlockSize := block.GetBlockHeader_v2_Size() + int64(len(manifestBytes))
	newManifestOffset := currentEOF - manifestBlockSize

	footer := &types.EnvelopeFooter_v2{
		ManifestOffset: uint64(newManifestOffset),
		ManifestLength: uint64(len(manifestBytes)),
	}
	copy(footer.Magic[:], types.MagicFooter_v2[:])

	return binary.Write(file, binary.LittleEndian, footer)
}

// --- 以下是之前已经验证过的辅助函数 ---

// readFooter 是一个辅助函数，用于从文件末尾读取并验证 Footer
func readFooter(filePath string) (*types.EnvelopeFooter_v2, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}

	footer := &types.EnvelopeFooter_v2{}
	footerSize := int64(binary.Size(footer))
	footerReader := io.NewSectionReader(file, fileInfo.Size()-footerSize, footerSize)

	err = binary.Read(footerReader, types.ByteOrder_v2, footer)
	if err != nil {
		return nil, fmt.Errorf("failed to read footer bytes from file '%s': %w", filePath, err)
	}

	if !bytes.Equal(footer.Magic[:], types.MagicFooter_v2[:]) {
		return nil, fmt.Errorf("file '%s' is not a valid ENCV container (footer magic mismatch at end of file)", filePath)
	}

	return footer, nil
}

// extractManifestWithScan 复制 manifest-v2 的线性扫描逻辑
func extractManifestWithScan(filePath string) ([]byte, int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open file '%s': %w", filePath, err)
	}
	defer file.Close()

	version, startOffset, err := types.DetectHeaderInfoFromReaderAt(file)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read header for version detection: %w", err)
	}

	switch version {
	case 3:
		startOffset = types.EnvelopeHeaderSize_v3
	case 2:
		startOffset = types.EnvelopeHeaderSize_v2
	default:
		return nil, 0, fmt.Errorf("unsupported container version detected")
	}

	// 2. Seek 到数据流起始位置
	if _, err := file.Seek(startOffset, io.SeekStart); err != nil {
		return nil, 0, fmt.Errorf("failed to seek to start of data stream (offset %d): %w", startOffset, err)
	}

	currentOffset := startOffset
	for {
		header, err := block.ReadBlockHeader(file)
		if err != nil {
			if err == io.EOF {
				return nil, 0, fmt.Errorf("reached end of file but Manifest block was not found in '%s'", filePath)
			}
			return nil, 0, fmt.Errorf("failed to read block header: %w", err)
		}

		if header.Type == types.BlockTypeManifest_v2 {
			manifestData, err := block.ReadBlockData(file, header)
			if err != nil {
				return nil, 0, fmt.Errorf("failed to read Manifest block data: %w", err)
			}
			return manifestData, currentOffset, nil
		}

		// 跳过当前数据块
		_, err = file.Seek(int64(header.Length), io.SeekCurrent)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to seek past block data: %w", err)
		}
		currentOffset += block.GetBlockHeader_v2_Size() + int64(header.Length)
	}
}
