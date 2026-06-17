package webdav

import (
	"context"
	"fmt"
	iofs "io/fs"
	"log/slog"
	"mime"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/container/manifest"
	"github.com/Soltus/encv-go/internal/v2/namer"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	"github.com/Soltus/encv-go/internal/v2/provider"
	"github.com/Soltus/encv-go/internal/v2/reader"
	"github.com/Soltus/encv-go/internal/v2/service"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/fsnotify/fsnotify"
	goWebdav "golang.org/x/net/webdav"
)

// encvWebDAVFS 是一个自定义的 webdav.FileSystem
// 它拦截文件请求，如果文件是 ENCV 容器，则提供解密后的流
type encvWebDAVFS struct {
	dir string // WebDAV 服务的本地文件系统目录（绝对路径）
	// WebDAV 的 URL 前缀 (例如 "/webdav/")
	webdavPrefix string
	// 注入 v2 架构的核心依赖
	readerService *service.ReaderService
	cfg           *config.Config

	// 【关键重构】使用互斥锁保护索引，而不是 atomic.Value
	indexes      *pathIndexes // 指向同一个索引对象
	indexesMutex sync.RWMutex
	// 【新增】索引构建完成的信号
	indexReady chan struct{}
	// 【新增】Index 对象缓存
	// key: 容器文件的完整路径
	// value: types.Index
	indexCache sync.Map // sync.Map 适合读多写少的场景
	// 【新增】定义要排除的目录
	excludeDirs map[string]bool
	// 【新增】定义已知的容器文件扩展名
	containerExtensions map[string]bool
	// 🆕 2026-06-17：注册的容器扩展名列表（manifest API 暴露给前端）
	registeredContainerExts []string
	// 【新增】文件系统监视器
	watcher *fsnotify.Watcher
	// 【新增】存储所有已知的碎片命名规则，用于过滤
	ChunkNamers []namer.ChunkNamer
}

// --- 辅助结构体 ---

// 【新增】一个结构体来持有所有索引数据
type pathIndexes struct {
	// 虚拟路径 -> 真实容器路径
	// 文件名索引
	// key: 虚拟路径 (e.g., "/output/config.user.json")
	// value: 真实路径 (e.g., "A:\\path\\to\\output\\config.user.nosj.sccgt")
	pathMap map[string]string
	// 父目录路径 -> 子项名称列表
	dirMap map[string][]string
	// 【新增】虚拟路径 -> 预计算的 FileInfo
	// 这样 ReadDir 就不需要在运行时调用 statFile 了
	fileInfoMap map[string]os.FileInfo
	// 【新增】反向映射：真实路径 -> 虚拟路径
	// 这能让我们在 O(1) 时间内判断一个物理文件是否是容器
	reversePathMap map[string]string
}

// decryptedFileInfo 实现了 os.FileInfo 接口
type decryptedFileInfo struct {
	// name 存储解密后的原始文件名
	name string
	// originalName 存储容器加密后的文件名（磁盘上的真实文件）
	originalName string
	size         int64
	mode         os.FileMode
	modTime      time.Time
	isDir        bool
	// 用于满足 WebDAV 的额外属性
	mimeType string
	etag     string
	// 【关键修复】保存原始容器文件的 FileInfo
	underlyingFileInfo os.FileInfo
}

// decryptedDir 实现了 webdav.File 接口，用于目录
// 它覆盖了 Readdir 方法，以提供解密后的文件列表
type decryptedDir struct {
	// *os.File // 嵌入原始的文件句柄
	// *os.File // 【关键修改】不再嵌入，避免 Handler 走捷径
	fullPath   string // 持有目录的绝对路径
	fs         *encvWebDAVFS
	name       string      // WebDAV 路径名，例如 "/webdav/output"
	cachedInfo os.FileInfo // 新增：缓存的 FileInfo
	infoOnce   sync.Once   // 新增：用于确保只加载一次
}

// NewENCVFS 创建一个新的 encvWebDAVFS 实例
// 【修改】构造函数现在需要接收 ReaderService 和 Config
//
// rootDir 指定 WebDAV 服务的物理根目录：
//   - "" 空字符串时兜底用 cfg.Webdav.Dir（向后兼容旧调用方）
//   - 非空时用显式 rootDir（多挂载点场景）
//
// 🆕 2026-06-17：多挂载点 webdav 适配（multi-mount-storage-refactor spec 续）
//   - server.go 现在为每个 mount 调一次 NewENCVFSForRoot(ctx, m.RootPath, ...)
//   - 每个 mount 独立 indexCache / fsnotify watcher / runIndexer goroutine
//   - URL 路径由 server.go 路由（/webdav/、/d/automation/、/d/primary/、/d/sandbox/）
func NewENCVFS(ctx context.Context, readerService *service.ReaderService, chunkNamers []namer.ChunkNamer) (goWebdav.FileSystem, IndexProvider, error) {
	return NewENCVFSForRoot(ctx, "", readerService, chunkNamers)
}

// NewENCVFSForRoot 创建绑定到显式 rootDir 的 webdavFS。
// 🆕 2026-06-17：多挂载点适配用。rootDir 非空时优先使用。
func NewENCVFSForRoot(ctx context.Context, rootDir string, readerService *service.ReaderService, chunkNamers []namer.ChunkNamer) (goWebdav.FileSystem, IndexProvider, error) {
	cfg := config.FromContext(ctx)
	if rootDir == "" {
		rootDir = cfg.Webdav.Dir
	}
	dir := rootDir
	var err error
	if dir == "/" {
		dir, err = os.Getwd()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get current working directory for WebDAV: %w", err)
		}
	}
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve absolute path for WebDAV directory '%s': %w", dir, err)
	}
	if _, statErr := os.Stat(dir); os.IsNotExist(statErr) {
		slog.Warn("WebDAV directory does not exist, creating it", "dir", dir)
		if mkdirErr := os.MkdirAll(dir, 0755); mkdirErr != nil {
			return nil, nil, fmt.Errorf("WebDAV directory '%s' does not exist and cannot be created: %w", dir, mkdirErr)
		}
	}
	// 规范化 WebDAV 前缀，确保它是一个以 '/' 开头且不以 '/' 结尾的路径
	webdavPrefix := strings.TrimSuffix(cfg.Webdav.Root, "/")
	if !strings.HasPrefix(webdavPrefix, "/") {
		webdavPrefix = "/" + webdavPrefix
	}

	// 1. 从插件系统获取扩展名列表
	registeredExtsSlice := plugins.GetAllRegisteredContainerExtensions()

	// 2. 将 []string 转换为 map[string]bool 以实现 O(1) 查找
	containerExtensionsMap := make(map[string]bool, len(registeredExtsSlice))
	for _, ext := range registeredExtsSlice {
		// 使用 strings.ToLower 确保匹配是大小写不敏感的
		containerExtensionsMap[strings.ToLower(ext)] = true
	}

	fs := &encvWebDAVFS{
		dir:           dir, // 使用处理过的绝对路径
		webdavPrefix:  webdavPrefix,
		readerService: readerService, // 【关键】注入依赖
		cfg:           cfg,           // 【关键】注入依赖
		// 初始化一个空的索引对象
		indexes: &pathIndexes{
			pathMap:        make(map[string]string),
			dirMap:         make(map[string][]string),
			fileInfoMap:    make(map[string]os.FileInfo),
			reversePathMap: make(map[string]string),
		},
		indexReady:          make(chan struct{}),
		containerExtensions: containerExtensionsMap,
		// 🆕 2026-06-17：保留原始顺序供 manifest 暴露
		registeredContainerExts: registeredExtsSlice,
		ChunkNamers:         chunkNamers,
		excludeDirs: map[string]bool{
			"node_modules": true,
			".git":         true,
			".idea":        true,
			// 可以根据需要添加更多
		},
	}

	// 启动后台索引构建和监视
	go fs.runIndexer(ctx)

	slog.Info("WebDAV FS initialized, index building in background", "dir", fs.dir)
	slog.Info("WebDAV registered container extensions", "extensions", registeredExtsSlice)

	return fs, fs, nil
}

// runIndexer 现在负责初始构建和增量更新
func (fs *encvWebDAVFS) runIndexer(ctx context.Context) {
	slog.Debug("Indexer started")
	// 1. 首次构建
	if err := fs.buildInitialIndex(ctx); err != nil {
		slog.Error("Initial index build failed", "error", err)
	} else {
		close(fs.indexReady) // 通知首次构建完成
		slog.Info("Initial index build complete")
	}

	// 2. 设置文件系统监视并处理增量事件
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("Failed to create fsnotify watcher", "error", err)
		return
	}
	defer watcher.Close()
	fs.watcher = watcher

	_ = filepath.Walk(fs.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	})
	slog.Debug("File system watcher active", "dir", fs.dir)

	var rebuildTimer *time.Timer
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				slog.Debug("Watcher events channel closed, exiting")
				return
			}
			// 清理缓存
			if event.Op&(fsnotify.Remove|fsnotify.Rename|fsnotify.Write) != 0 {
				fs.indexCache.Delete(event.Name)
			}

			// 防抖
			if rebuildTimer != nil {
				rebuildTimer.Stop()
			}
			rebuildTimer = time.AfterFunc(2*time.Second, func() {
				slog.Debug("Debounced timer fired, triggering incremental update", "event", event.Name)
				fs.processIncrementalEvent(ctx, event)
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			slog.Error("File system watcher error", "error", err)
		case <-ctx.Done():
			slog.Debug("Indexer shutting down")
			return
		}
	}
}

// buildInitialIndex 只在启动时执行一次
func (fs *encvWebDAVFS) buildInitialIndex(ctx context.Context) error {
	// 遍历文件系统，填充初始索引
	return filepath.WalkDir(fs.dir, func(p string, d iofs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			if fs.excludeDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(p))
		if !fs.containerExtensions[ext] {
			return nil
		}

		// 【关键】调用 addOrUpdateEntry，而不是直接修改 map
		if err := fs.addOrUpdateEntry(p); err != nil {
			slog.Warn("Failed to add entry during initial build", "path", p, "error", err)
		}

		// 在 buildInitialIndex 的末尾
		// log.Printf("[Index-DEBUG] Final dirMap state: %+v", fs.indexes.dirMap)
		return nil
	})
}

// processIncrementalEvent 处理单个文件系统事件
func (fs *encvWebDAVFS) processIncrementalEvent(ctx context.Context, event fsnotify.Event) {
	// 我们不再关心具体的事件类型和文件路径，任何变化都意味着我们需要重新同步
	slog.Debug("Received relevant FS event, triggering full sync", "op", event.Op, "path", event.Name)

	// 【关键】调用 syncWithFilesystem 来与磁盘状态进行比对和同步
	fs.syncWithFilesystem()
}

// syncWithFilesystem 是新的核心方法，它将索引与磁盘同步
func (fs *encvWebDAVFS) syncWithFilesystem() {
	slog.Debug("syncWithFilesystem started")

	// 获取当前索引的快照
	indexes := fs.getIndexes()
	// 创建一个 map 用于快速查找磁盘上的文件
	onDiskFiles := make(map[string]os.FileInfo)

	// 1. 扫描磁盘，获取所有容器文件
	fullPath := fs.dir
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		slog.Error("syncWithFilesystem: Failed to read directory", "dir", fullPath, "error", err)
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if fs.containerExtensions[ext] {
			onDiskFiles[entry.Name()] = nil // 我们只关心文件名是否存在
		}
	}

	// 2. 遍历索引，找出需要删除的条目（文件已从磁盘消失）
	for _, physicalPath := range indexes.pathMap {
		physicalFileName := filepath.Base(physicalPath)
		if _, exists := onDiskFiles[physicalFileName]; !exists {
			slog.Debug("File no longer exists on disk, removing from index", "path", physicalPath)
			fs.removeEntry(physicalPath)
		}
	}

	// 3. 遍历磁盘，找出需要添加或更新的条目（文件新增或修改）
	for diskFileName := range onDiskFiles {
		diskFilePath := filepath.Join(fs.dir, diskFileName)
		// 检查是否需要更新（可以通过比较 modTime，但为简单起见，我们直接尝试添加）
		slog.Debug("File exists on disk, attempting to add/update", "path", diskFilePath)
		if err := fs.addOrUpdateEntry(diskFilePath); err != nil {
			slog.Warn("syncWithFilesystem: Failed to add/update entry", "path", diskFilePath, "error", err)
		}
	}
	slog.Debug("syncWithFilesystem finished")
}

// addOrUpdateEntry 是一个核心的、线程安全的索引修改方法
func (fs *encvWebDAVFS) addOrUpdateEntry(p string) error {
	slog.Debug("addOrUpdateEntry called", "path", p)
	// 1. 检查是否是容器
	ext := strings.ToLower(filepath.Ext(p))
	if !fs.containerExtensions[ext] {
		slog.Debug("File is not a container, skipping", "path", p)
		return nil
	}
	if !fs.validateContainerHeader(p) {
		slog.Debug("File ignored: invalid ENCV header", "path", p)
		return nil
	}

	// 2. 解析容器
	index, err := fs.getIndexFromContainerPathWithCache(p)
	if err != nil {
		slog.Warn("Failed to parse container, entry will not be added", "path", p, "error", err)
		return fmt.Errorf("could not parse container '%s': %w", p, err)
	}

	// 3. 获取文件信息
	info, err := os.Stat(p)
	if err != nil {
		return err
	}

	// 4. 【关键修复】加写锁，并智能地修改共享索引
	fs.indexesMutex.Lock()
	defer fs.indexesMutex.Unlock()

	virtualFileName := index.GetOriginalFilename()
	fullVirtualPath, err := fs.physicalPathToIndexKey(p, virtualFileName)
	if err != nil {
		return err
	}

	// 【关键修复】先检查是否需要移除旧条目
	// 只有当物理路径发生变化时，才需要移除
	if existingPhysicalPath, exists := fs.indexes.pathMap[fullVirtualPath]; exists && existingPhysicalPath != p {
		// 物理路径变了，移除旧的关联
		fs.removeEntryUnsafe(fullVirtualPath, existingPhysicalPath)
	} else if !exists {
		// 是一个全新的虚拟路径，但可能有一个旧的物理文件被映射到了它（例如文件被重命名了）
		// 这种情况比较罕见，但为了健壮性，我们还是检查一下
		if oldVirtualPath, isOldPhysical := fs.indexes.reversePathMap[p]; isOldPhysical {
			fs.removeEntryUnsafe(oldVirtualPath, p)
		}
	}

	// 添加新条目
	parentDir := path.Dir(fullVirtualPath)
	fileName := path.Base(fullVirtualPath)

	// 【关键修复】避免在 dirMap 中重复添加
	// 先检查文件名是否已存在于该目录下
	dirEntries := fs.indexes.dirMap[parentDir]
	alreadyExists := slices.Contains(dirEntries, fileName)
	if !alreadyExists {
		fs.indexes.dirMap[parentDir] = append(fs.indexes.dirMap[parentDir], fileName)
	}

	fs.indexes.pathMap[fullVirtualPath] = p
	fs.indexes.reversePathMap[p] = fullVirtualPath

	decryptedInfo := &decryptedFileInfo{
		name:               index.GetOriginalFilename(),
		originalName:       filepath.Base(p),
		size:               index.GetOriginalFileSize(),
		mode:               0444,
		modTime:            info.ModTime(),
		isDir:              false,
		mimeType:           mime.TypeByExtension(filepath.Ext(index.GetOriginalFilename())),
		etag:               utils.GenETag(info.ModTime(), index.GetOriginalFileSize()),
		underlyingFileInfo: info,
	}
	fs.indexes.fileInfoMap[fullVirtualPath] = decryptedInfo

	// 在 addOrUpdateEntry 的末尾，解锁前
	// log.Printf("[Index-DEBUG] After adding '%s', dirMap for parent '%s' is now: %v", fullVirtualPath, parentDir, fs.indexes.dirMap[parentDir])

	return nil
}

// removeEntry 从索引中安全地移除一个条目
func (fs *encvWebDAVFS) removeEntry(p string) {
	fs.indexesMutex.Lock()
	defer fs.indexesMutex.Unlock()
	fs.removeEntryUnsafe("", p) // 调用内部不安全版本
}

// removeEntryUnsafe 是 removeEntry 的内部版本，不加锁
func (fs *encvWebDAVFS) removeEntryUnsafe(virtualPath, physicalPath string) {
	// 通过物理路径查找虚拟路径
	if virtualPath == "" {
		var ok bool
		virtualPath, ok = fs.indexes.reversePathMap[physicalPath]
		if !ok {
			return // 条目不存在
		}
	}

	// 从所有 map 中移除
	delete(fs.indexes.pathMap, virtualPath)
	delete(fs.indexes.reversePathMap, physicalPath)
	delete(fs.indexes.fileInfoMap, virtualPath)

	// 从 dirMap 中移除
	parentDir := path.Dir(virtualPath)
	fileName := path.Base(virtualPath)
	if entries, ok := fs.indexes.dirMap[parentDir]; ok {
		// 创建一个新的 slice 来避免修改正在遍历的 slice
		newEntries := make([]string, 0, len(entries)-1)
		for _, entry := range entries {
			if entry != fileName {
				newEntries = append(newEntries, entry)
			}
		}
		fs.indexes.dirMap[parentDir] = newEntries
	}
	slog.Debug("Removed entry from index", "path", virtualPath)
}

// getIndexes 现在安全地返回当前索引的深拷贝快照
func (fs *encvWebDAVFS) getIndexes() *pathIndexes {
	select {
	case <-fs.indexReady:
		// 索引已就绪
	default:
		// 索引未就绪，返回空索引
		slog.Debug("getIndexes called before index is ready, returning empty index")
		return &pathIndexes{pathMap: make(map[string]string), dirMap: make(map[string][]string), fileInfoMap: make(map[string]os.FileInfo), reversePathMap: make(map[string]string)}
	}

	fs.indexesMutex.RLock()
	defer fs.indexesMutex.RUnlock()

	// 【关键修复】创建一个深拷贝快照，确保与后台线程的数据隔离
	snapshot := &pathIndexes{
		pathMap:        make(map[string]string, len(fs.indexes.pathMap)),
		dirMap:         make(map[string][]string, len(fs.indexes.dirMap)),
		fileInfoMap:    make(map[string]os.FileInfo, len(fs.indexes.fileInfoMap)),
		reversePathMap: make(map[string]string, len(fs.indexes.reversePathMap)),
	}

	for k, v := range fs.indexes.pathMap {
		snapshot.pathMap[k] = v
	}
	for k, v := range fs.indexes.dirMap {
		// 切片需要特殊处理，创建一个新的副本
		snapshot.dirMap[k] = append([]string(nil), v...)
	}
	for k, v := range fs.indexes.fileInfoMap {
		snapshot.fileInfoMap[k] = v // os.FileInfo 是接口，拷贝指针是安全的
	}
	for k, v := range fs.indexes.reversePathMap {
		snapshot.reversePathMap[k] = v
	}

	return snapshot
}

type IndexProvider interface {
	GetIndexStats() IndexStatsResult
	SearchInIndex(keyword, queryPath string, maxResults int) []SearchEntry
	Dir() string
	IsContainerExtension(filename string) bool
	// 🆕 2026-06-17：多挂载点 webdav 适配（multi-mount-storage-refactor spec 续）
	// 返回 mount 的 manifest snapshot（virtual tree + container map + index stats）
	// 用于 GET /api/webdav/manifest API
	GetManifest() ManifestSnapshot
}

type IndexStatsResult struct {
	TotalFiles int    `json:"totalFiles"`
	TotalDirs  int    `json:"totalDirs"`
	Containers int    `json:"containers"`
	Source     string `json:"source"`
}

// 🆕 2026-06-17：ManifestSnapshot 是 GetManifest 的返回值。
// 用于 GET /api/webdav/manifest API：前端用 virtual tree 生成 manifest-driven 测试用例。
//
// 字段语义：
//   - IndexReady: indexCache 是否已构建完成（false 时 Frontend 等待后端 rebuild）
//   - IndexStats: 文件 / 目录 / 容器计数
//   - VirtualTree: 扁平化虚拟文件树（virtual_path / name / is_dir / size / container 物理路径）
//   - ContainerMap: 虚拟路径 → 物理容器路径的映射（攻防测试用：验证容器可见性）
//   - RegisteredContainerExts: webdavFS 注册的容器扩展名（攻防测试用：构造容器物理路径）
type ManifestSnapshot struct {
	IndexReady             bool                          `json:"index_ready"`
	IndexStats             IndexStatsResult              `json:"index_stats"`
	VirtualTree            []ManifestVirtualEntry        `json:"virtual_tree"`
	ContainerMap           []ManifestContainerMapping    `json:"container_map"`
	RegisteredContainerExts []string                      `json:"registered_container_exts"`
}

// ManifestVirtualEntry 描述 manifest 中的一个虚拟文件 / 目录节点。
type ManifestVirtualEntry struct {
	VirtualPath string `json:"virtual_path"` // 相对 mount root 的路径（不含前导 /）
	Name        string `json:"name"`
	IsDir       bool   `json:"is_dir"`
	Size        int64  `json:"size"`
	ModTime     int64  `json:"mod_time"` // unix epoch seconds
	Container   string `json:"container,omitempty"` // 物理容器路径（仅虚拟文件有）
}

// ManifestContainerMapping 描述虚拟文件 → 物理容器的映射。
// 攻防测试用：构造 PROPFIND <container_path> 验证容器可见性是否被 webdavFS 过滤。
type ManifestContainerMapping struct {
	VirtualPath   string `json:"virtual_path"`
	ContainerPath string `json:"container_path"`
	MountName     string `json:"mount_name"` // 🆕 用于多 mount 场景标识
}

func (fs *encvWebDAVFS) GetIndexStats() IndexStatsResult {
	idx := fs.getIndexes()
	containers := 0
	for range idx.reversePathMap {
		containers++
	}
	return IndexStatsResult{
		TotalFiles: len(idx.pathMap),
		TotalDirs:  len(idx.dirMap),
		Containers: containers,
		Source:     "webdav",
	}
}

type SearchEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size,omitempty"`
	ModTime string `json:"modTime,omitempty"`
}

func (fs *encvWebDAVFS) SearchInIndex(keyword, queryPath string, maxResults int) []SearchEntry {
	idx := fs.getIndexes()
	keyword = strings.ToLower(keyword)
	var results []SearchEntry

	for vPath, info := range idx.fileInfoMap {
		name := info.Name()
		if !strings.Contains(strings.ToLower(name), keyword) {
			continue
		}
		if queryPath != "" && !strings.HasPrefix(vPath, queryPath) {
			continue
		}
		results = append(results, SearchEntry{
			Name:    name,
			Path:    vPath,
			IsDir:   info.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		})
		if maxResults > 0 && len(results) >= maxResults {
			break
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Name < results[j].Name
	})

	return results
}

func (fs *encvWebDAVFS) Dir() string {
	return fs.dir
}

func (fs *encvWebDAVFS) IsContainerExtension(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return fs.containerExtensions[ext]
}

// GetManifest 返回当前 webdavFS 实例的 manifest snapshot。
// 🆕 2026-06-17：用于 GET /api/webdav/manifest API。
//
// 实现：
//   1. 调 getIndexes() 拿 pathIndexes 快照（深拷贝，线程安全）
//   2. 遍历 fileInfoMap 构造 VirtualTree
//   3. 遍历 reversePathMap 构造 ContainerMap（虚拟路径 → 物理路径）
//   4. 检查 indexReady channel 状态
//
// 性能：fileInfoMap 1000 项 ~1ms；10000 项 ~10ms。可接受。
func (fs *encvWebDAVFS) GetManifest() ManifestSnapshot {
	snap := ManifestSnapshot{
		RegisteredContainerExts: append([]string(nil), fs.registeredContainerExts...),
	}

	// indexReady 检查（非阻塞）
	select {
	case <-fs.indexReady:
		snap.IndexReady = true
	default:
		snap.IndexReady = false
	}

	idx := fs.getIndexes()
	snap.IndexStats = IndexStatsResult{
		TotalFiles: len(idx.pathMap),
		TotalDirs:  len(idx.dirMap),
		Containers: len(idx.reversePathMap),
		Source:     "webdav",
	}

	// 构造 VirtualTree
	snap.VirtualTree = make([]ManifestVirtualEntry, 0, len(idx.fileInfoMap))
	for vPath, info := range idx.fileInfoMap {
		entry := ManifestVirtualEntry{
			VirtualPath: vPath,
			Name:        info.Name(),
			IsDir:       info.IsDir(),
			Size:        info.Size(),
			ModTime:     info.ModTime().Unix(),
		}
		// 关联物理容器路径
		if containerPath, isVirtual := idx.pathMap[vPath]; isVirtual {
			entry.Container = containerPath
		}
		snap.VirtualTree = append(snap.VirtualTree, entry)
	}

	// 构造 ContainerMap
	snap.ContainerMap = make([]ManifestContainerMapping, 0, len(idx.reversePathMap))
	for containerPath, vPath := range idx.reversePathMap {
		snap.ContainerMap = append(snap.ContainerMap, ManifestContainerMapping{
			VirtualPath:   vPath,
			ContainerPath: containerPath,
		})
	}

	return snap
}

func (fs *encvWebDAVFS) WaitForIndexReady(ctx context.Context) {
	select {
	case <-fs.indexReady:
	case <-ctx.Done():
	}
}

// validateContainerHeader 检查文件是否具有有效的 ENCV 头部（V2 或 V3）
// 它利用 types 包中的通用检测器来统一处理版本识别和 Header 大小获取。
func (fs *encvWebDAVFS) validateContainerHeader(filePath string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()
	version, headerSize, err := types.DetectHeaderInfoFromReaderAt(file)

	// 判断逻辑：
	// 1. err 必须为 nil（读取成功）
	// 2. version 必须不为 0（是已知的 V2/V3 容器）
	// 3. headerSize 必须大于 0（Header 长度合法）
	return err == nil && version != 0 && headerSize > 0
}

// 带缓存的 Index 获取
func (fs *encvWebDAVFS) getIndexFromContainerPathWithCache(fullPath string) (types.Index, error) {
	// 1. 尝试从缓存加载
	if cachedIndex, ok := fs.indexCache.Load(fullPath); ok {
		return cachedIndex.(types.Index), nil
	}

	// 2. 缓存未命中，从文件加载
	index, err := fs.getIndexFromContainerPath(fullPath)
	if err != nil {
		return nil, err
	}

	// 3. 加载成功，存入缓存
	// 可以考虑使用文件的 ModTime 作为缓存有效性判断，但为简单起见，这里不做
	fs.indexCache.Store(fullPath, index)
	return index, nil
}

// 从容器路径获取 Index，封装了新的架构逻辑
func (fs *encvWebDAVFS) getIndexFromContainerPath(fullPath string) (types.Index, error) {
	// 1. 提取 Manifest 的原始 JSON 字节
	manifestBytes, err := manifest.ExtractManifest(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract manifest bytes from %s: %w", fullPath, err)
	}

	// 2. 将字节反序列化为 Manifest 结构体
	manifestStruct, err := manifest.DeserializeFromJSON(manifestBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize manifest from %s: %w", fullPath, err)
	}

	// 3. 使用注册表动态创建 KVIProvider
	provider, err := types.NewKVIProviderFromManifest(manifestStruct)
	if err != nil {
		return nil, fmt.Errorf("failed to create KVI provider from %s: %w", fullPath, err)
	}

	// 4. 从 Provider 获取 Index
	return provider.GetIndex(), nil
}

// statFile 获取文件信息，如果是 ENCV 容器，则返回原始文件信息
// 【关键修复】这个方法现在被设计为可以安全地处理文件和目录，并且能从任何内部错误中恢复。
func (fs *encvWebDAVFS) statFile(ctx context.Context, fullPath string) (os.FileInfo, error) {
	// 步骤 1: 首先调用 os.Stat 获取路径的基本信息。
	// 这是判断一个路径是文件还是目录的最可靠方法。
	// 如果路径本身不存在或无法访问，os.Stat 会返回错误，我们直接将错误向上传递。
	info, err := os.Stat(fullPath)
	if err != nil {
		slog.Debug("os.Stat failed", "path", fullPath, "error", err)
		return nil, err
	}
	// 步骤 2: 检查它是否是一个目录。
	// 如果是目录，我们**直接返回**其信息，并且**绝不**进行任何容器检测。
	// 这是防止将目录误判为容器、从而避免 panic 的核心防线。
	if info.IsDir() {
		slog.Debug("Path is a directory, returning info directly", "path", fullPath)
		return info, nil
	}

	// 步骤 3: 从这里开始，我们 100% 确定它是一个文件。
	// 现在可以安全地尝试将其作为 ENCV 容器来处理。
	// 【关键修复】使用 recover 来捕获任何潜在的 panic，确保单个文件的处理失败不会影响整个请求。
	finalInfo := info // 默认返回原始文件信息
	func() {
		defer func() {
			if r := recover(); r != nil {
				// 日志可以简化，因为这不影响用户列表
				slog.Warn("Panic caught while processing file during background scan", "path", fullPath, "panic", r)
			}
		}()

		index, err := fs.getIndexFromContainerPathWithCache(fullPath)
		if err != nil {
			// 不是容器或 KVI 损坏，返回原始文件信息。
			// 我们将日志级别从 WARN 降为 DEBUG，因为这对于非容器文件是正常行为。
			slog.Debug("File is not a valid container, returning original file info", "path", fullPath, "error", err)
			// 不做任何事，让函数最后返回原始的 'info'
			return
		}
		// 步骤 5: 如果是有效容器，创建并返回代表解密后文件的虚拟 FileInfo。
		// 注意：这里我们不能直接返回，因为我们在一个闭包里。
		origSize := index.GetOriginalFileSize()
		decryptedInfo := &decryptedFileInfo{
			name:               index.GetOriginalFilename(),
			originalName:       filepath.Base(fullPath),
			size:               origSize,
			mode:               0444,
			modTime:            info.ModTime(),
			isDir:              false,
			mimeType:           mime.TypeByExtension(filepath.Ext(index.GetOriginalFilename())),
			etag:               utils.GenETag(info.ModTime(), index.GetOriginalFileSize()),
			underlyingFileInfo: info,
		}
		// 通过修改外部作用域的变量来返回结果
		finalInfo = decryptedInfo
	}()

	// 无论是否是容器，或者处理过程中是否出错（包括 panic），最终都返回一个 FileInfo
	// 这保证了 WebDAV Handler 总是能拿到一个结果，从而继续处理下一个文件。
	return finalInfo, nil
}

// --- 实现 webdav.FileSystem 接口 ---

func (fs *encvWebDAVFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	indexKey, keyErr := fs.webdavPathToIndexKey(name)
	if keyErr != nil {
		return nil, keyErr
	}
	indexes := fs.getIndexes()

	if fileInfo, ok := indexes.fileInfoMap[indexKey]; ok {
		slog.Debug("Found virtual file in index, returning cached info", "name", name)
		return fileInfo, nil
	}

	// 2. 索引中没找到，再处理物理文件/目录
	fullPath, err := fs.resolvePath(name)
	if err != nil {
		return nil, err
	}

	// 3. 调用 statFile 处理物理文件
	info, err := fs.statFile(ctx, fullPath)
	if err != nil {
		slog.Debug("Physical file not found or error", "name", name, "error", err)
		return nil, err
	}

	return info, nil
}

func (fs *encvWebDAVFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (goWebdav.File, error) {
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0 {
		return nil, os.ErrPermission
	}

	indexKey, keyErr := fs.webdavPathToIndexKey(name)
	if keyErr != nil {
		return nil, keyErr
	}

	fullPath, err := fs.resolvePath(name)
	if err != nil {
		return nil, err
	}

	if f, err := fs.openAsDirectory(fullPath, name); err == nil {
		return f, nil
	}

	// 【关键修改】优先使用索引进行路由，完全移除 ExtractKVI_v2 调用
	indexes := fs.getIndexes()

	// 情况A：请求的是虚拟文件
	if realPath, isVirtual := indexes.pathMap[indexKey]; isVirtual {
		slog.Debug("Request is a virtual file, creating lazy adapter", "name", name, "container", realPath)

		// 【最终修复】不再在这里初始化任何东西！
		// 我们只创建一个"懒加载"的适配器，它把所有初始化都推迟到 Read() 调用时。
		fileInfo := indexes.fileInfoMap[indexKey]
		if fileInfo == nil {
			return nil, fmt.Errorf("internal error: fileInfo not found in index for virtual file '%s'", indexKey)
		}

		// 创建并返回懒加载的适配器
		lazyAdapter := &lazyWebDAVFileAdapter{
			fs:            fs,
			containerPath: realPath,
			fileInfo:      fileInfo,
		}

		return lazyAdapter, nil
	}
	// 检查这个物理文件是否是容器（通过反向映射）
	if _, isContainer := indexes.reversePathMap[fullPath]; isContainer {
		slog.Debug("Request is a physical container, creating lazy adapter", "name", name, "container", fullPath)
		fileInfo := indexes.fileInfoMap[indexKey]
		if fileInfo == nil {
			return nil, fmt.Errorf("internal error: fileInfo not found in index for physical container '%s'", indexKey)
		}
		lazyAdapter := &lazyWebDAVFileAdapter{
			fs:            fs,
			containerPath: fullPath,
			fileInfo:      fileInfo,
		}
		return lazyAdapter, nil
	}

	// 情况C：请求的是真正的普通文件
	slog.Debug("Request is a standard file", "name", name)
	// 【最终修复】使用 recover 保护整个文件打开过程
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Panic caught while opening standard file", "name", name, "panic", r)
		}
	}()
	prov, err := provider.NewStandardFileProvider(fullPath)
	if err != nil {
		return nil, err
	}
	standardFileInfo, err := os.Stat(fullPath)
	if err != nil {
		prov.Close()
		return nil, err
	}
	adapter, err := newWebDAVFileAdapter(prov, standardFileInfo)
	if err != nil {
		prov.Close()
		return nil, err
	}
	return adapter, nil
}

// --- 辅助函数 ---

// openAsDirectory 尝试将路径作为目录打开，如果成功，则返回一个虚拟目录对象，避免真实打开目录
func (fs *encvWebDAVFS) openAsDirectory(fullPath string, name string) (goWebdav.File, error) {
	if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
		slog.Debug("Opening directory", "path", fullPath)
		// osFile, err := os.Open(fullPath)
		// 【关键修改】不再打开 os.File，只传递路径
		return &decryptedDir{
			fullPath: fullPath,
			fs:       fs,
			name:     name,
		}, nil
	}
	return nil, os.ErrNotExist
}

// ReadDir 方法完全重写，变为高性能且无竞争
func (fs *encvWebDAVFS) ReadDir(ctx context.Context, name string) ([]os.FileInfo, error) {
	indexKey, keyErr := fs.webdavPathToIndexKey(name)
	if keyErr != nil {
		return nil, keyErr
	}
	slog.Debug("ReadDir called", "name", name, "indexKey", indexKey)

	// 1. 获取当前索引（这是一个内存快照，非常快）
	indexes := fs.getIndexes()
	// 2. 使用 map 来存储结果，键为文件/目录名，可以自动去重
	fileInfos := make(map[string]os.FileInfo)

	// 2. 从索引中获取所有虚拟文件，使用转换后的 indexKey
	virtualNames, found := indexes.dirMap[indexKey]

	// 【关键修复】如果请求的是根目录 ('.')，但 dirMap 中没有，我们需要扫描整个 pathMap
	if indexKey == "." && !found {
		slog.Debug("Requesting root dir but no entry in dirMap, scanning all pathMap", "name", name)
		for virtualPath := range indexes.pathMap {
			if path.Dir(virtualPath) == "." { // 检查这个虚拟文件是否在根目录
				fileName := path.Base(virtualPath)
				if info, ok := indexes.fileInfoMap[virtualPath]; ok {
					fileInfos[fileName] = info
				}
			}
		}
	} else if found {
		slog.Debug("Lookup in dirMap", "key", indexKey, "names", virtualNames)
		for _, virtualName := range virtualNames {
			virtualPath := path.Join(indexKey, virtualName)
			if info, ok := indexes.fileInfoMap[virtualPath]; ok {
				fileInfos[virtualName] = info
			}
		}
	}

	// 4. 添加磁盘上存在但不在索引中的物理文件/目录
	// 注意：这里 resolvePath 需要的是原始的 name，用于处理普通文件
	fullPath, err := fs.resolvePath(name)
	if err != nil {
		return nil, err
	}
	entries, _ := os.ReadDir(fullPath)
	for _, entry := range entries {
		entryName := entry.Name()
		// 如果这个条目已经被虚拟文件覆盖了，则跳过
		if _, exists := fileInfos[entryName]; exists {
			continue
		}

		// 【关键修复】使用 ChunkNamers 过滤碎片
		isChunk := false
		for _, namer := range fs.ChunkNamers {
			if namer.IsDataChunk(entryName) {
				isChunk = true
				break // 只要匹配任何一个规则，就认为是碎片
			}
		}
		if isChunk {
			continue // 跳过碎片
		}

		// 【关键修复】使用反向映射检查是否是主容器文件
		entryPath := filepath.Join(fullPath, entryName)
		if _, isContainer := indexes.reversePathMap[entryPath]; isContainer {
			// 是容器，跳过，因为它的虚拟文件已经被处理了
			continue
		}

		// 不是碎片，不是主容器，是普通文件或目录（如字幕、封面），获取其信息
		var info os.FileInfo
		info, err = entry.Info()
		if err != nil {
			// 【关键修改】entry.Info() 失败，尝试用 os.Stat 降级处理
			slog.Warn("entry.Info() failed, retrying with os.Stat", "path", entryPath, "error", err)
			info, err = os.Stat(entryPath)
			if err != nil {
				// os.Stat 也失败了，才真正跳过这个文件
				slog.Error("os.Stat also failed, skipping", "path", entryPath, "error", err)
				continue
			}
		}
		fileInfos[entryName] = info
	}

	// 5. 将 map 转换为切片并排序，以提供确定性的、有序的列表
	var result []os.FileInfo
	for _, info := range fileInfos {
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name()) < strings.ToLower(result[j].Name())
	})

	return result, nil
}

func (fs *encvWebDAVFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	return os.ErrPermission
}

func (fs *encvWebDAVFS) RemoveAll(ctx context.Context, name string) error {
	return os.ErrPermission
}

func (fs *encvWebDAVFS) Rename(ctx context.Context, oldName, newName string) error {
	return os.ErrPermission
}

func (dfi *decryptedFileInfo) Name() string {
	// log.Printf("[FileInfo-DEBUG] decryptedFileInfo.Name() called for '%s'. Returning: '%s'", dfi.originalName, dfi.name)
	return dfi.name
}

func (dfi *decryptedFileInfo) Size() int64 {
	// log.Printf("[FileInfo-DEBUG] decryptedFileInfo.Size() called for '%s'. Returning: %d", dfi.name, dfi.size)
	return dfi.size
}

func (dfi *decryptedFileInfo) Mode() os.FileMode {
	// log.Printf("[FileInfo-DEBUG] decryptedFileInfo.Mode() called for '%s'. Returning: %v", dfi.name, dfi.mode)
	return dfi.mode
}

func (dfi *decryptedFileInfo) ModTime() time.Time {
	// log.Printf("[FileInfo-DEBUG] decryptedFileInfo.ModTime() called for '%s'. Returning: %v", dfi.name, dfi.modTime)
	return dfi.modTime
}

func (dfi *decryptedFileInfo) IsDir() bool {
	// log.Printf("[FileInfo-DEBUG] decryptedFileInfo.IsDir() called for '%s'. Returning: %v", dfi.name, dfi.isDir)
	return dfi.isDir
}
func (dfi *decryptedFileInfo) Sys() interface{} {
	// log.Printf("[FileInfo-DEBUG] decryptedFileInfo.Sys() called for '%s'. Returning: %v", dfi.name, dfi.underlyingFileInfo.Sys())
	return dfi.underlyingFileInfo.Sys()
}

func (dfi *decryptedFileInfo) ContentType() string { return dfi.mimeType }
func (dfi *decryptedFileInfo) ETag() string        { return dfi.etag }

func (d *decryptedDir) Stat() (os.FileInfo, error) {
	var err error
	d.infoOnce.Do(func() {
		d.cachedInfo, err = os.Stat(d.fullPath)
	})
	return d.cachedInfo, err
}

func (d *decryptedDir) Close() error {
	// 我们没有持有打开的文件句柄，所以 Close 是空操作
	return nil
}

// Readdir 返回目录中的文件信息，使用高性能的混合索引+物理扫描策略
func (d *decryptedDir) Readdir(count int) ([]os.FileInfo, error) {
	// 1. 获取当前索引的快照
	indexes := d.fs.getIndexes()

	// 2. 将 WebDAV 路径转换为索引键
	indexKey, keyErr := d.fs.webdavPathToIndexKey(d.name)
	if keyErr != nil {
		return nil, keyErr
	}
	slog.Debug("decryptedDir Readdir called", "path", d.name, "indexKey", indexKey)

	// 3. 使用 map 来自动去重和合并，键为文件名
	mergedFiles := make(map[string]os.FileInfo)

	// 4. 【第一步】从索引中加载所有虚拟文件（容器解密后的文件）
	// 这些是最高优先级的，因为它们有最完整的信息
	virtualNames := indexes.dirMap[indexKey]
	for _, virtualName := range virtualNames {
		virtualPath := path.Join(indexKey, virtualName)
		if fileInfo, ok := indexes.fileInfoMap[virtualPath]; ok {
			mergedFiles[virtualName] = fileInfo // 优先使用索引中的完整信息
		}
	}

	// 5. 【第二步】快速扫描物理磁盘，添加未被索引覆盖的文件
	// 这里只做 os.ReadDir，不做任何 entry.Info() 或 os.Stat()，所以非常快
	fullPath, err := d.fs.resolvePath(d.name)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		entryName := entry.Name()
		// 如果这个文件已经被虚拟文件覆盖了，则跳过
		if _, exists := mergedFiles[entryName]; exists {
			continue
		}

		// 【关键过滤】
		// a. 过滤碎片
		isChunk := false
		for _, namer := range d.fs.ChunkNamers {
			if namer.IsDataChunk(entryName) {
				isChunk = true
				break
			}
		}
		if isChunk {
			continue
		}

		// b. 过滤主容器文件
		entryPath := filepath.Join(fullPath, entryName)
		if _, isContainer := indexes.reversePathMap[entryPath]; isContainer {
			continue
		}

		// c. 对于剩余的物理文件（如字幕、普通mkv），创建一个简单的 FileInfo
		// 我们不调用 entry.Info() 来避免慢速 I/O
		simpleInfo := &simpleFileInfo{
			name:    entryName,
			isDir:   entry.IsDir(),
			modTime: time.Time{}, // 使用零值时间，WebDAV 客户端通常能接受
			size:    0,           // 使用零值大小
		}
		mergedFiles[entryName] = simpleInfo
	}

	// 6. 【第三步】将 map 转换为切片并排序
	var result []os.FileInfo
	for _, info := range mergedFiles {
		result = append(result, info)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name()) < strings.ToLower(result[j].Name())
	})

	if count > 0 && len(result) > count {
		result = result[:count]
	}

	slog.Debug("decryptedDir Readdir returning files", "count", len(result))
	return result, nil
}

func (d *decryptedDir) Read(p []byte) (n int, err error)             { return 0, os.ErrInvalid }
func (d *decryptedDir) Seek(offset int64, whence int) (int64, error) { return 0, os.ErrInvalid }
func (d *decryptedDir) Write(p []byte) (n int, err error)            { return 0, os.ErrPermission }

// simpleFileInfo 是一个轻量级的 FileInfo，用于物理文件
// 它只包含从 DirEntry 能获取的最基本信息，避免昂贵的 os.Stat 调用
type simpleFileInfo struct {
	name    string
	size    int64 // 对于物理文件，我们可能无法快速获取大小，先设为0
	modTime time.Time
	isDir   bool
}

func (s *simpleFileInfo) Name() string       { return s.name }
func (s *simpleFileInfo) Size() int64        { return s.size }
func (s *simpleFileInfo) Mode() os.FileMode  { return 0444 } // 只读
func (s *simpleFileInfo) ModTime() time.Time { return s.modTime }
func (s *simpleFileInfo) IsDir() bool        { return s.isDir }
func (s *simpleFileInfo) Sys() interface{}   { return nil }

// lazyWebDAVFileAdapter 实现了 goWebdav.File 接口
// 它将所有耗时的容器初始化推迟到第一次 Read() 调用时
// --- 【修改】lazyWebDAVFileAdapter 的状态管理 ---
type lazyWebDAVFileAdapter struct {
	fs            *encvWebDAVFS
	containerPath string
	fileInfo      os.FileInfo

	// 【修改】内部状态，用于更精细的懒加载控制
	mu             sync.Mutex
	isInitialized  bool
	provider       provider.FileContentProvider // 需要关闭
	underlyingFile goWebdav.File                // 最终的、实际的文件适配器
	initError      error                        // 用于保存初始化时遇到的错误
}

// 【修改】一个内部方法，用于执行实际的初始化
func (l *lazyWebDAVFileAdapter) initialize() error {
	if l.isInitialized {
		return l.initError
	}
	slog.Debug("lazyWebDAVFileAdapter performing lazy initialization", "path", l.containerPath)

	factory, err := reader.NewDecryptReaderFactory(l.containerPath, l.fs.cfg.Password)
	if err != nil {
		l.initError = err
		l.isInitialized = true // 标记为已尝试，避免重复
		return err
	}
	decryptReader, err := factory.NewDecryptReader()
	if err != nil {
		factory.Close()
		l.initError = err
		l.isInitialized = true
		return err
	}
	prov, err := provider.NewLocalFileProvider(context.Background(), factory, decryptReader)
	if err != nil {
		l.initError = err
		l.isInitialized = true
		return err
	}

	underlyingAdapter, err := newWebDAVFileAdapter(prov, l.fileInfo)
	if err != nil {
		prov.Close()
		l.initError = err
		l.isInitialized = true
		return err
	}

	l.provider = prov
	l.underlyingFile = underlyingAdapter
	l.initError = nil
	l.isInitialized = true
	slog.Debug("lazyWebDAVFileAdapter lazy initialization successful", "path", l.containerPath)
	return nil
}

func (l *lazyWebDAVFileAdapter) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.underlyingFile != nil {
		err := l.underlyingFile.Close()
		l.underlyingFile = nil
		return err
	}
	return nil
}

func (l *lazyWebDAVFileAdapter) Stat() (os.FileInfo, error) {
	return l.fileInfo, nil
}

// 【核心修改】所有 I/O 操作都先检查初始化状态
func (l *lazyWebDAVFileAdapter) Read(p []byte) (n int, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.initialize(); err != nil {
		return 0, err
	}
	// 将 Read 调用转发给已经初始化的底层文件
	return l.underlyingFile.Read(p)
}

func (l *lazyWebDAVFileAdapter) Readdir(count int) ([]os.FileInfo, error) {
	return nil, os.ErrInvalid
}

// 【核心修改】Seek 现在只触发初始化，不读取数据
func (l *lazyWebDAVFileAdapter) Seek(offset int64, whence int) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.initialize(); err != nil {
		return 0, err
	}
	// 将 Seek 调用转发给已经初始化的底层文件
	return l.underlyingFile.Seek(offset, whence)
}

func (l *lazyWebDAVFileAdapter) Write(p []byte) (n int, err error) {
	return 0, os.ErrPermission
}
