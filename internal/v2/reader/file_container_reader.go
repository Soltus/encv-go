// internal/v2/reader/file_container_reader.go

package reader

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/Soltus/encv-go/internal/v2/container/block"
	containerhandle "github.com/Soltus/encv-go/internal/v2/container/handle"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// fileContainerReader 是 EncryptedContainerReader 接口的一个健壮、自适应的具体实现。
// 它负责从单个或多个文件中读取原始的、加密的数据块，并具备强大的错误恢复能力。
type fileContainerReader struct {
	// 核心元数据，在构造时解析并缓存
	manifest     *types.Manifest
	footer       *types.EnvelopeFooter_v2 // 可能为 nil
	kviProvider  types.KVIProvider
	containerDir string
	mainFilePath string
	// 这是为了处理 V3 容器但 Manifest 格式仍为 V2 的兼容情况
	headerVersion int

	// 添加字段持有初始化（扫描）阶段打开的主文件句柄
	// 这确保了后续的 GetFragmentReader 使用的是同一个句柄，避免读取到旧文件或截断的文件
	initMainFileHandle *os.File

	// 运行时状态，用于物理偏移映射和外部文件缓存
	mu                sync.RWMutex
	physicalOffsets   map[string]uint64   // 主文件中的偏移
	openExternalFiles map[string]*os.File // 缓存已打开的外部文件句柄
	// 【新增】缓存物理分片文件内部的偏移映射
	// Key: 物理分片文件名 (e.g., "321.4pm.0001")
	// Value: Fragment ID -> Local Offset
	chunkPhysicalOffsets map[string]map[string]uint64
}

// pooledFileHandleWrapper 是一个优化的包装器，它实现了 io.Reader, io.ReaderAt, io.Seeker 和 io.Closer。
// 它确保在 Close 时归还文件句柄到全局池，同时保留底层 SectionReader 的高性能接口。
type pooledFileHandleWrapper struct {
	io.Reader
	io.ReaderAt
	io.Seeker
	closer fileHandleCloser
}

// fileHandleCloser 负责在 Close 时归还文件句柄
type fileHandleCloser struct {
	file *os.File
}

func (c *fileHandleCloser) Close() error {
	if c.file != nil {
		return globalFileHandlePool.Put(c.file)
	}
	return nil
}

// newPooledFileHandleWrapper 创建一个新的包装器实例
func newPooledFileHandleWrapper(section *io.SectionReader, file *os.File) *pooledFileHandleWrapper {
	return &pooledFileHandleWrapper{
		Reader:   section, // SectionReader 实现了 io.Reader
		ReaderAt: section, // SectionReader 实现了 io.ReaderAt
		Seeker:   section, // SectionReader 实现了 io.Seeker
		closer:   fileHandleCloser{file: file},
	}
}

func (w *pooledFileHandleWrapper) Close() error {
	return w.closer.Close()
}

// readOnlySectionCloser 保留 SectionReader 的 ReaderAt/Seeker 能力，同时避免关闭共享句柄
type readOnlySectionCloser struct {
	io.Reader
	io.ReaderAt
	io.Seeker
}

func (r *readOnlySectionCloser) Close() error {
	return nil
}

// NewEncryptedContainerReaderFromFile 是创建 EncryptedContainerReader 的入口。
// 它会解析并缓存所有必要的元数据，后续操作将基于这些缓存数据进行，非常高效。

func NewEncryptedContainerReaderFromFile(mainFilePath string) (EncryptedContainerReader, error) {
	globalFileHandlePool.Close(mainFilePath)

	src, err := containerhandle.NewFileSource(mainFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open container source: %w", err)
	}

	h, err := containerhandle.Open(src)
	if err != nil {
		src.Close()
		return nil, fmt.Errorf("failed to open container handle: %w", err)
	}
	defer h.Close()

	r := &fileContainerReader{
		mainFilePath:         mainFilePath,
		containerDir:         filepath.Dir(mainFilePath),
		manifest:             h.Manifest(),
		headerVersion:        h.Version(),
		physicalOffsets:      make(map[string]uint64),
		openExternalFiles:    make(map[string]*os.File),
		chunkPhysicalOffsets: make(map[string]map[string]uint64),
	}

	kvi, _ := types.NewKVIProviderFromManifest(r.manifest)
	r.kviProvider = kvi

	headerSize := block.GetBlockHeader_v2_Size()
	var headerOverhead int64
	if r.headerVersion == 4 {
		headerOverhead = 0
	} else {
		headerOverhead = headerSize
	}
	for _, frag := range r.manifest.Fragments {
		if frag.PhysicalPath == "" {
			r.physicalOffsets[frag.ID] = frag.PhysicalOffset + uint64(headerOverhead)
		}
	}

	log.Printf("INFO: [Reader] Initialized from ContainerHandle. Scan skipped.")
	return r, nil
}

// NewFileContainerReaderFromMetadata 是一个新的、轻量级的构造函数。
// 它使用预先解析好的 manifest、headerVersion 和 physicalOffsets 来创建 reader，避免了重复的文件扫描。
func NewFileContainerReaderFromMetadata(mainFilePath string, manifest *types.Manifest, headerVersion int, physicalOffsets map[string]uint64) (*fileContainerReader, error) {
	kviProvider, err := types.NewKVIProviderFromManifest(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal KVI from manifest: %w", err)
	}

	r := &fileContainerReader{
		mainFilePath:         mainFilePath,
		containerDir:         filepath.Dir(mainFilePath),
		manifest:             manifest,
		headerVersion:        headerVersion,
		kviProvider:          kviProvider,
		physicalOffsets:      physicalOffsets, // 【关键】直接使用传入的 map，不扫描
		openExternalFiles:    make(map[string]*os.File),
		chunkPhysicalOffsets: make(map[string]map[string]uint64),
	}

	return r, nil
}

// GetManifest 返回已解析的容器清单。
func (r *fileContainerReader) GetManifest() *types.Manifest {
	return r.manifest
}

// GetKVIProvider 返回解析后的 KVI provider
func (r *fileContainerReader) GetKVIProvider() (types.KVIProvider, error) {
	return types.NewKVIProviderFromManifest(r.manifest)
}

func (r *fileContainerReader) GetFragments() []types.Fragment {
	return r.manifest.Fragments
}

// GetFragmentReader 根据 Fragment ID，返回一个读取该 Fragment 原始加密数据的 io.ReadCloser。
// 此方法是线程安全的，并集成了数据校验和错误恢复机制。
// 【优化】现在返回的 Reader 实现了 io.ReaderAt，允许 VirtualSeekableDecryptReader 使用零拷贝路径。
func (r *fileContainerReader) GetFragmentReader(fragID string) (io.ReadCloser, error) {
	frag, err := r.findFragmentByID(fragID)
	if err != nil {
		return nil, err
	}

	headerSize := block.GetBlockHeader_v2_Size()

	// Case A: 主文件
	if frag.PhysicalPath == "" {
		payloadOffset, ok := r.physicalOffsets[frag.ID]
		if !ok || payloadOffset == 0 {
			return nil, fmt.Errorf("fragment '%s' offset missing or zero", fragID)
		}

		mainFile, useInit, err := r.acquireMainFile()
		if err != nil {
			return nil, err
		}

		if r.headerVersion != 4 {
			if err := r.verifyFragmentAt(mainFile, int64(payloadOffset)-headerSize, frag); err != nil {
				if !useInit {
					globalFileHandlePool.Put(mainFile)
				}
				return nil, err
			}
		}

		section := io.NewSectionReader(mainFile, int64(payloadOffset), int64(frag.Length))
		if useInit {
			return &readOnlySectionCloser{
				Reader:   section,
				ReaderAt: section,
				Seeker:   section,
			}, nil
		}
		return newPooledFileHandleWrapper(section, mainFile), nil
	}

	// --- 情况 2: 数据在外部文件中 ---
	// 外部文件暂时保留扫描机制，因为如果 Packer 没有写入 PhysicalOffset，我们必须找到它
	if err := r.ensureChunkScanned(frag.PhysicalPath); err != nil {
		return nil, fmt.Errorf("failed to scan chunk file layout: %w", err)
	}

	expectedChunkPath := filepath.Join(r.containerDir, frag.PhysicalPath)

	r.mu.RLock()
	chunkMap, ok := r.chunkPhysicalOffsets[frag.PhysicalPath]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("internal inconsistency: chunk map missing for %s", frag.PhysicalPath)
	}

	blockStartOffset, ok := chunkMap[frag.ID]
	if !ok {
		return nil, fmt.Errorf("fragment %s not found in chunk %s layout map", frag.ID, frag.PhysicalPath)
	}

	extFile, err := globalFileHandlePool.Get(expectedChunkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get external file handle from pool: %w", err)
	}

	// 验证
	if err := r.verifyFragmentAt(extFile, int64(blockStartOffset), frag); err != nil {
		globalFileHandlePool.Put(extFile)
		return nil, fmt.Errorf("fragment '%s' in chunk '%s' is corrupt: %w", fragID, frag.PhysicalPath, err)
	}

	dataStartOffset := int64(blockStartOffset) + headerSize
	section := io.NewSectionReader(extFile, dataStartOffset, int64(frag.Length))

	// 外部文件总是通过池获取的，因为 initHandle 只针对主文件
	return newPooledFileHandleWrapper(section, extFile), nil

}

// Close 关闭容器及其打开的所有外部资源。
func (r *fileContainerReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 关闭初始化阶段打开并持有的主文件句柄
	if r.initMainFileHandle != nil {
		if err := r.initMainFileHandle.Close(); err != nil {
			// 记录错误但继续尝试清理其他资源
			log.Printf("WARN: [fileContainerReader] Failed to close initial main file handle: %v", err)
		}
		r.initMainFileHandle = nil
	}

	var combinedErr error
	for path, f := range r.openExternalFiles {
		if f == nil {
			continue
		}
		if err := f.Close(); err != nil {
			if combinedErr == nil {
				combinedErr = fmt.Errorf("failed to close external file %s: %w", path, err)
			} else {
				combinedErr = fmt.Errorf("%v; failed to close external file %s: %w", combinedErr, path, err)
			}
		}
		// 关闭时也要通过句柄池
		globalFileHandlePool.Put(f)
	}
	r.openExternalFiles = make(map[string]*os.File)
	return combinedErr
}

func (r *fileContainerReader) findFragmentByID(fragID string) (*types.Fragment, error) {
	for _, frag := range r.manifest.Fragments {
		if frag.ID == fragID {
			return &frag, nil
		}
	}
	return nil, fmt.Errorf("fragment with ID '%s' not found in manifest", fragID)
}

func (r *fileContainerReader) acquireMainFile() (*os.File, bool, error) {
	if r.initMainFileHandle != nil {
		if _, err := r.initMainFileHandle.Seek(0, io.SeekCurrent); err == nil {
			return r.initMainFileHandle, true, nil
		}
	}
	f, err := globalFileHandlePool.Get(r.mainFilePath)
	if err == nil {
		r.initMainFileHandle = f
	}
	return f, false, err
}

// findManifestBlockOffset 是一个辅助函数，用于找到 Manifest 块的起始偏移量。
// 它会从文件开头扫描，直到找到第一个 BlockTypeManifest_v2 类型的块。
// 注意：此函数不负责读取 Manifest 数据，只返回其位置。
func (r *fileContainerReader) findManifestBlockOffset() (int64, error) {
	// 【关键修改 1】从全局文件句柄池获取文件句柄
	fileHandle, err := globalFileHandlePool.Get(r.mainFilePath)
	if err != nil {
		return 0, fmt.Errorf("failed to get file handle from pool for manifest scan: %w", err)
	}
	// 【关键修改 2】使用 defer 确保文件句柄被归还，防止泄漏
	defer globalFileHandlePool.Put(fileHandle)

	version, headerSize, err := types.DetectHeaderInfoFromReaderAt(fileHandle)
	if err != nil {
		return 0, fmt.Errorf("failed to read header for version detection: %w", err)
	}
	if headerSize == 0 {
		return 0, fmt.Errorf("unknown header version detected: %d", version)
	}

	log.Printf("DEBUG: [fileContainerReader] Detected Header Version %d (Size: %d). Scanning starts at offset %d.", version, headerSize, headerSize)

	// 4. Seek 到 Header 结束位置
	if _, err := fileHandle.Seek(headerSize, io.SeekStart); err != nil {
		return 0, fmt.Errorf("failed to seek past header (size %d): %w", headerSize, err)
	}

	// 5. 扫描 Manifest 块
	for {
		// 获取当前偏移量
		currentOffset, err := fileHandle.Seek(0, io.SeekCurrent)
		if err != nil {
			return 0, fmt.Errorf("failed to get current offset during manifest scan: %w", err)
		}

		// 读取块头
		header, err := block.ReadBlockHeader(fileHandle)
		if err != nil {
			// 如果读到文件末尾还没找到，也是一种明确的错误
			if err == io.EOF {
				return 0, fmt.Errorf("reached end of file without finding a manifest block")
			}
			return 0, fmt.Errorf("failed to read block header during scan: %w", err)
		}

		// 检查是否是 Manifest 块
		if header.Type == types.BlockTypeManifest_v2 {
			// 找到了，返回其起始偏移量
			return currentOffset, nil
		}

		// 不是 Manifest 块，跳过其数据部分，继续扫描
		if _, err := fileHandle.Seek(int64(header.Length), io.SeekCurrent); err != nil {
			return 0, fmt.Errorf("failed to skip block data at offset %d: %w", currentOffset, err)
		}
	}
}

// ensureChunkScanned 确保（扫描并缓存）指定物理分片文件中所有 fragment 的偏移量
func (r *fileContainerReader) ensureChunkScanned(chunkFilename string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.chunkPhysicalOffsets[chunkFilename]; exists {
		return nil
	}

	log.Printf("INFO: Scanning chunk layout '%s'...", chunkFilename)

	var fragsInChunk []types.Fragment
	for _, frag := range r.manifest.Fragments {
		if frag.PhysicalPath == chunkFilename {
			fragsInChunk = append(fragsInChunk, frag)
		}
	}

	if len(fragsInChunk) == 0 {
		return fmt.Errorf("no fragments found for chunk file %s", chunkFilename)
	}

	chunkPath := filepath.Join(r.containerDir, chunkFilename)
	file, err := os.Open(chunkPath)
	if err != nil {
		return err
	}
	defer file.Close()

	headerSize := int64(0)
	if r.headerVersion == 3 {
		headerSize = types.EnvelopeHeaderSize_v3
	} else if r.headerVersion == 4 {
		headerSize = types.EnvelopeHeaderSize_v4
	}
	if _, err := file.Seek(headerSize, io.SeekStart); err != nil {
		return err
	}

	offsets := make(map[string]uint64)
	currentOffset := headerSize

	for i := 0; i < len(fragsInChunk); i++ {
		frag := fragsInChunk[i]

		header, err := block.ReadBlockHeader(file)
		if err != nil {
			return fmt.Errorf("failed to read block header in chunk %s at offset %d: %w", chunkFilename, currentOffset, err)
		}

		if header.Type != types.BlockTypeData_v2 {
			return fmt.Errorf("invalid header type %x at offset %d for fragment %s (expected Data)", header.Type, currentOffset, frag.ID)
		}

		if header.CRC32 != frag.DataCRC32 {
			return fmt.Errorf("corruption detected in chunk '%s' (fragment '%s', offset %d): Header CRC mismatch", chunkFilename, frag.ID, currentOffset)
		}
		if header.Length != frag.Length {
			return fmt.Errorf("length mismatch detected in chunk '%s' (fragment '%s')", chunkFilename, frag.ID)
		}

		offsets[frag.ID] = uint64(currentOffset)
		// 移除: log.Printf("DEBUG: [ensureChunkScanned] Mapped fragment '%s' to offset %d in chunk '%s'", frag.ID, currentOffset, chunkFilename)

		currentOffset += block.GetBlockHeader_v2_Size() + int64(frag.Length)
		if _, err := file.Seek(currentOffset, io.SeekStart); err != nil {
			return fmt.Errorf("failed to seek past fragment %s in chunk: %w", frag.ID, err)
		}
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat chunk file %s: %w", chunkFilename, err)
	}
	actualFileSize := fileInfo.Size()

	if currentOffset != actualFileSize {
		diff := actualFileSize - currentOffset
		if diff > 0 {
			return fmt.Errorf("ERROR: Chunk file '%s' integrity check failed: manifest ends at offset %d, but file size is %d. %d bytes of garbage data detected.", chunkFilename, currentOffset, actualFileSize, diff)
		} else {
			return fmt.Errorf("ERROR: Chunk file '%s' integrity check failed: file size is only %d, expected %d. Data MISSING.", chunkFilename, actualFileSize, currentOffset)
		}
	}

	r.chunkPhysicalOffsets[chunkFilename] = offsets
	return nil
}

// verifyFragmentAt 从给定的 ReaderAt 的特定偏移量处验证一个片段。
// 线程安全（不修改接收者状态）。
func (r *fileContainerReader) verifyFragmentAt(readerAt io.ReaderAt, blockStartOffset int64, frag *types.Fragment) error {
	headerSize := block.GetBlockHeader_v2_Size()
	headerReader := io.NewSectionReader(readerAt, blockStartOffset, headerSize)
	header, err := block.ReadBlockHeader(headerReader)
	if err != nil {
		return fmt.Errorf("failed to read block header at offset %d: %w", blockStartOffset, err)
	}

	// 核心验证：CRC 与 长度
	if header.CRC32 != frag.DataCRC32 {
		return fmt.Errorf("crc mismatch for fragment '%s' (expected %08x, got %08x)", frag.ID, frag.DataCRC32, header.CRC32)
	}
	if header.Length != frag.Length {
		return fmt.Errorf("length mismatch for fragment '%s' (expected %d, got %d)", frag.ID, frag.Length, header.Length)
	}
	return nil
}

// findAndOpenFragmentRecovery 扫描目录以查找匹配的文件
func (r *fileContainerReader) findAndOpenFragmentRecovery(frag *types.Fragment) (*os.File, error) {
	log.Printf("INFO: Entering recovery mode for fragment '%s' (CRC: %08x)", frag.ID, frag.DataCRC32)

	entries, err := os.ReadDir(r.containerDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read container directory '%s' for recovery: %w", r.containerDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// skip main file itself
		if entry.Name() == filepath.Base(r.mainFilePath) {
			continue
		}

		candidatePath := filepath.Join(r.containerDir, entry.Name())
		candidateFile, err := os.Open(candidatePath)
		if err != nil {
			// 忽略无法打开的条目
			continue
		}

		chunkHeaderOffset := int64(0)
		if r.headerVersion == 3 {
			chunkHeaderOffset = types.EnvelopeHeaderSize_v3
		} else if r.headerVersion == 4 {
			chunkHeaderOffset = types.EnvelopeHeaderSize_v4
		}

		// 验证候选文件：校验在正确偏移量处的块头是否匹配 frag
		if err := r.verifyFragmentAt(candidateFile, chunkHeaderOffset, frag); err == nil {
			log.Printf("INFO: Recovery successful: found fragment '%s' at '%s'", frag.ID, candidatePath)
			return candidateFile, nil
		}
		_ = candidateFile.Close()
	}

	return nil, fmt.Errorf("recovery failed: could not find a valid file for fragment '%s' (crc %08x)", frag.ID, frag.DataCRC32)
}

// scanPhysicalOffsets 统一扫描并建立所有在主文件中的分片的物理偏移映射。
// 它不再区分“单文件”或“物理分片”容器，而是只关注“哪些分片在主文件中”。
// func (r *fileContainerReader) scanPhysicalOffsetsWithSize(headerSize int64) error {
// 	// 1. 筛选出所有需要从主文件中查找的分片
// 	var fragmentsInMainFile []types.Fragment_v2
// 	for _, frag := range r.manifest.Fragments {
// 		// 只有当 PhysicalPath 为空时，才表示它在主文件中
// 		if frag.PhysicalPath == "" {
// 			fragmentsInMainFile = append(fragmentsInMainFile, frag)
// 		}
// 	}

// 	// 2. 如果没有分片在主文件中，则无需扫描，直接返回。
// 	// 这种情况在理论上可能发生，虽然罕见。
// 	if len(fragmentsInMainFile) == 0 {
// 		log.Printf("INFO: No fragments found in the main file. Skipping physical offset scan.")
// 		return nil
// 	}

// 	// 4. 委托给专门的扫描器，传入 headerSize
// 	log.Printf("INFO: Found %d fragments in main file. Starting physical offset scan.", len(fragmentsInMainFile))
// 	return r.scanForDataBlocksInMainFile(fragmentsInMainFile, headerSize)
// }

// // scanForDataBlocksInMainFile 扫描主文件，为指定的分片列表建立 ID -> Offset 映射。
// func (r *fileContainerReader) scanForDataBlocksInMainFile(fragmentsToScan []types.Fragment_v2, headerSize int64) error {
// 	manifestBlockOffset, err := r.findManifestBlockOffset()
// 	if err != nil {
// 		return fmt.Errorf("failed to find manifest block for pre-scan: %w", err)
// 	}

// 	mainFile, err := os.Open(r.mainFilePath)
// 	if err != nil {
// 		return err
// 	}
// 	r.initMainFileHandle = mainFile

// 	fileInfo, err := mainFile.Stat()
// 	if err != nil {
// 		return fmt.Errorf("failed to stat main file for boundary check: %w", err)
// 	}
// 	fileSize := fileInfo.Size()

// 	log.Printf("INFO: Found %d fragments in main file. Starting physical offset scan.", len(fragmentsToScan))

// 	if _, err := mainFile.Seek(headerSize, io.SeekStart); err != nil {
// 		return fmt.Errorf("failed to seek past header (size %d): %w", headerSize, err)
// 	}

// 	fragIndex := 0
// 	for {
// 		blockStartOffset, err := mainFile.Seek(0, io.SeekCurrent)
// 		if err != nil {
// 			return fmt.Errorf("failed to get current offset: %w", err)
// 		}

// 		if blockStartOffset >= manifestBlockOffset {
// 			break
// 		}

// 		header, err := block.ReadBlockHeader(mainFile)
// 		if err != nil {
// 			if err == io.EOF {
// 				break
// 			}
// 			return fmt.Errorf("failed to read block header at offset %d: %w", blockStartOffset, err)
// 		}

// 		if header.Type == types.BlockTypeData_v2 {
// 			if fragIndex >= len(fragmentsToScan) {
// 				return fmt.Errorf("found more data blocks in main file than expected (found %d, expected %d)", fragIndex+1, len(fragmentsToScan))
// 			}

// 			frag := fragmentsToScan[fragIndex]

// 			dataEndOffset := blockStartOffset + int64(headerSize) + int64(header.Length)
// 			if dataEndOffset > fileSize {
// 				return fmt.Errorf("corruption detected: fragment '%s' (offset %d, len %d) would exceed file size %d by %d bytes. File truncated or manifest corrupt.",
// 					frag.ID, blockStartOffset, header.Length, fileSize, dataEndOffset-fileSize)
// 			}

// 			r.physicalOffsets[frag.ID] = uint64(blockStartOffset)
// 			fragIndex++
// 		}

// 		if _, err = mainFile.Seek(int64(header.Length), io.SeekCurrent); err != nil {
// 			return fmt.Errorf("failed to seek past block data at offset %d: %w", blockStartOffset, err)
// 		}
// 	}

// 	if fragIndex != len(fragmentsToScan) {
// 		return fmt.Errorf("scan finished, but found %d data blocks in main file, expected %d", fragIndex, len(fragmentsToScan))
// 	}

// 	log.Printf("INFO: All %d fragments in main file successfully mapped.", len(fragmentsToScan))
// 	return nil
// }
