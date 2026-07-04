package service

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/v2/pluginsext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock 实现 ---

type mockDirReader struct {
	entries   []fs.DirEntry
	err       error
	callCount int
	mu        sync.Mutex
}

func (m *mockDirReader) ReadDir(name string) ([]fs.DirEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	if m.err != nil {
		return nil, m.err
	}
	return m.entries, nil
}

type mockDirEntry struct {
	name    string
	isDir   bool
	size    int64
	modTime time.Time
	infoErr error
}

func (e *mockDirEntry) Name() string { return e.name }
func (e *mockDirEntry) IsDir() bool  { return e.isDir }
func (e *mockDirEntry) Type() fs.FileMode {
	if e.isDir {
		return fs.ModeDir
	}
	return 0
}
func (e *mockDirEntry) Info() (fs.FileInfo, error) {
	if e.infoErr != nil {
		return nil, e.infoErr
	}
	return &mockFileInfo{entry: e}, nil
}

type mockFileInfo struct {
	entry *mockDirEntry
}

func (f *mockFileInfo) Name() string       { return f.entry.name }
func (f *mockFileInfo) Size() int64        { return f.entry.size }
func (f *mockFileInfo) Mode() fs.FileMode  { return 0644 }
func (f *mockFileInfo) ModTime() time.Time { return f.entry.modTime }
func (f *mockFileInfo) IsDir() bool        { return f.entry.isDir }
func (f *mockFileInfo) Sys() interface{}   { return nil }

type mockContainerDetector struct {
	encryptedPaths map[string]bool
	detectCalls    []string
	mu             sync.Mutex
}

func (m *mockContainerDetector) DetectContainer(path string) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.detectCalls = append(m.detectCalls, path)
	if m.encryptedPaths[path] {
		return nil, nil
	}
	return nil, errors.New("not a container")
}

func (m *mockContainerDetector) getDetectCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	calls := make([]string, len(m.detectCalls))
	copy(calls, m.detectCalls)
	return calls
}

// ========== 测试用例 ==========

// TestListFiles_WithMockDirReader 验证使用 mock DirReader 时正常返回文件列表
func TestListFiles_WithMockDirReader(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	mockEntries := []fs.DirEntry{
		&mockDirEntry{name: "video.mp4", isDir: false, size: 1024, modTime: time.Now()},
		&mockDirEntry{name: "music", isDir: true},
		&mockDirEntry{name: ".hidden", isDir: false},
		&mockDirEntry{name: "doc.pdf", isDir: false, size: 2048, modTime: time.Now()},
	}

	svc.dirReader = &mockDirReader{entries: mockEntries}
	svc.containerDetector = &mockContainerDetector{encryptedPaths: map[string]bool{}}

	files, err := svc.ListFiles("/")
	require.NoError(t, err)
	require.Len(t, files, 3, ".hidden 应被跳过")

	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f.Name
	}
	assert.Contains(t, names, "video.mp4")
	assert.Contains(t, names, "music")
	assert.Contains(t, names, "doc.pdf")
	assert.NotContains(t, names, ".hidden")
}

// TestListFiles_MockDirReaderPermissionError 验证 mock 返回权限错误时转换为 PermissionError
func TestListFiles_MockDirReaderPermissionError(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	svc.dirReader = &mockDirReader{err: os.ErrPermission}

	files, err := svc.ListFiles("/")
	require.Error(t, err)
	require.Nil(t, files)
	var permErr *PermissionError
	require.ErrorAs(t, err, &permErr)
}

// TestListFiles_MockDirReaderNotExist 验证 mock 返回不存在错误时转换为 NotFoundError
func TestListFiles_MockDirReaderNotExist(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	svc.dirReader = &mockDirReader{err: fs.ErrNotExist}

	files, err := svc.ListFiles("/")
	require.Error(t, err)
	require.Nil(t, files)
	var notFound *NotFoundError
	require.ErrorAs(t, err, &notFound)
}

// TestListFiles_MockEncryptedDetection 验证 mock ContainerDetector 正确标记加密文件
func TestListFiles_MockEncryptedDetection(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	absDir, _ := filepath.Abs(dir)
	mockEntries := []fs.DirEntry{
		&mockDirEntry{name: "normal.txt", isDir: false, size: 100, modTime: time.Now()},
		&mockDirEntry{name: "secret" + pluginsext.VideoExt, isDir: false, size: 9999, modTime: time.Now()},
	}

	detector := &mockContainerDetector{
		encryptedPaths: map[string]bool{
			filepath.Join(absDir, "secret"+pluginsext.VideoExt): true,
		},
	}

	svc.dirReader = &mockDirReader{entries: mockEntries}
	svc.containerDetector = detector

	files, err := svc.ListFiles("/")
	require.NoError(t, err)
	require.Len(t, files, 2)

	var normal, secret *FileInfo
	for i := range files {
		if files[i].Name == "normal.txt" {
			normal = &files[i]
		} else if files[i].Name == "secret"+pluginsext.VideoExt {
			secret = &files[i]
		}
	}
	require.NotNil(t, normal)
	require.NotNil(t, secret)

	assert.False(t, normal.IsEncrypted, "普通文件不应标记为加密")
	assert.True(t, secret.IsEncrypted, "视频容器应标记为加密")
}

// TestListFiles_EntryInfoErrorFallback 验证 entry.Info() 失败时 fallback 到默认值
func TestListFiles_EntryInfoErrorFallback(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	mockEntries := []fs.DirEntry{
		&mockDirEntry{name: "ok.txt", isDir: false, size: 100, modTime: time.Now()},
		&mockDirEntry{
			name:    "broken.bin",
			isDir:   false,
			infoErr: os.ErrPermission,
		},
	}

	svc.dirReader = &mockDirReader{entries: mockEntries}
	svc.containerDetector = &mockContainerDetector{encryptedPaths: map[string]bool{}}

	files, err := svc.ListFiles("/")
	require.NoError(t, err)
	require.Len(t, files, 2)

	var broken *FileInfo
	for i := range files {
		if files[i].Name == "broken.bin" {
			broken = &files[i]
			break
		}
	}
	require.NotNil(t, broken)
	assert.Equal(t, int64(0), broken.Size, "Info 失败时 Size 应 fallback 为 0")
	assert.Equal(t, "", broken.Modified, "Info 失败时 Modified 应为空字符串")
}

// TestListFiles_EntryInfoNotExistDuringIteration 模拟遍历过程中文件被删除（Info 返回 ErrNotExist）
func TestListFiles_EntryInfoNotExistDuringIteration(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	now := time.Now()
	mockEntries := []fs.DirEntry{
		&mockDirEntry{name: "stable.txt", isDir: false, size: 100, modTime: now},
		&mockDirEntry{
			name:    "deleted_midway.mp4",
			isDir:   false,
			infoErr: os.ErrNotExist,
		},
		&mockDirEntry{name: "also_ok.jpg", isDir: false, size: 200, modTime: now},
	}

	svc.dirReader = &mockDirReader{entries: mockEntries}
	svc.containerDetector = &mockContainerDetector{encryptedPaths: map[string]bool{}}

	files, err := svc.ListFiles("/")
	require.NoError(t, err, "遍历时部分文件被删除不应返回错误")
	require.Len(t, files, 3, "所有条目都应出现在结果中（包括已删除的）")

	var stable, deleted, alsoOk *FileInfo
	for i := range files {
		switch files[i].Name {
		case "stable.txt":
			stable = &files[i]
		case "deleted_midway.mp4":
			deleted = &files[i]
		case "also_ok.jpg":
			alsoOk = &files[i]
		}
	}
	require.NotNil(t, stable)
	require.NotNil(t, deleted)
	require.NotNil(t, alsoOk)

	assert.Equal(t, int64(100), stable.Size)
	assert.Equal(t, int64(0), deleted.Size, "已删除文件的 Size 应 fallback 为 0")
	assert.Equal(t, "", deleted.Modified, "已删除文件的 Modified 应为空")
	assert.Equal(t, int64(200), alsoOk.Size)
}

// TestListFiles_DetectContainerOnDeletedFile 验证对已删除文件调用 detectContainer 不 panic
func TestListFiles_DetectContainerOnDeletedFile(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	mockEntries := []fs.DirEntry{
		&mockDirEntry{name: "normal.txt", isDir: false, size: 50, modTime: time.Now()},
		&mockDirEntry{name: "gone" + pluginsext.VideoExt, isDir: false, size: 9999, modTime: time.Now()},
	}

	detector := &mockContainerDetector{
		encryptedPaths: map[string]bool{},
	}

	svc.dirReader = &mockDirReader{entries: mockEntries}
	svc.containerDetector = detector

	files, err := svc.ListFiles("/")
	require.NoError(t, err)
	require.Len(t, files, 2)

	calls := detector.getDetectCalls()
	assert.GreaterOrEqual(t, len(calls), 1, "detectContainer 应至少被调用一次（针对非目录文件）")
}

// TestListFiles_EmptyDirectory 验证空目录返回空列表
func TestListFiles_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	svc.dirReader = &mockDirReader{entries: []fs.DirEntry{}}
	svc.containerDetector = &mockContainerDetector{encryptedPaths: map[string]bool{}}

	files, err := svc.ListFiles("/")
	require.NoError(t, err)
	require.Empty(t, files, "空目录应返回空列表")
}

// TestListFiles_AllHiddenFiles 验证全部是隐藏文件时返回空列表
func TestListFiles_AllHiddenFiles(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	mockEntries := []fs.DirEntry{
		&mockDirEntry{name: ".gitignore", isDir: false, size: 100, modTime: time.Now()},
		&mockDirEntry{name: ".DS_Store", isDir: false, size: 200, modTime: time.Now()},
		&mockDirEntry{name: ".hidden_dir", isDir: true},
	}

	svc.dirReader = &mockDirReader{entries: mockEntries}
	svc.containerDetector = &mockContainerDetector{encryptedPaths: map[string]bool{}}

	files, err := svc.ListFiles("/")
	require.NoError(t, err)
	require.Empty(t, files, "全隐藏文件目录应返回空列表")
}

// TestListFiles_LongFilename 验证超长文件名正常处理
func TestListFiles_LongFilename(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	longName := strings.Repeat("a", 255) + ".txt"
	mockEntries := []fs.DirEntry{
		&mockDirEntry{name: longName, isDir: false, size: 42, modTime: time.Now()},
	}

	svc.dirReader = &mockDirReader{entries: mockEntries}
	svc.containerDetector = &mockContainerDetector{encryptedPaths: map[string]bool{}}

	files, err := svc.ListFiles("/")
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, longName, files[0].Name)
	assert.Equal(t, int64(42), files[0].Size)
}

// TestListFiles_PathTraversalStillBlocked 验证注入 mock 后路径遍历攻击仍被拦截
func TestListFiles_PathTraversalStillBlocked(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	mockEntries := []fs.DirEntry{
		&mockDirEntry{name: "etc_passwd", isDir: false, size: 1024, modTime: time.Now()},
	}

	svc.dirReader = &mockDirReader{entries: mockEntries}
	svc.containerDetector = &mockContainerDetector{encryptedPaths: map[string]bool{}}

	files, err := svc.ListFiles("../../etc")
	require.Error(t, err, "路径遍历攻击应被拦截，即使使用了 mock")
	require.Nil(t, files)
	var forbidden *ForbiddenError
	require.ErrorAs(t, err, &forbidden, "应返回 ForbiddenError")
}

// TestListFiles_DirectoryEntriesHaveCorrectPath 验证目录条目的 Path 字段格式正确
func TestListFiles_DirectoryEntriesHaveCorrectPath(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	mockEntries := []fs.DirEntry{
		&mockDirEntry{name: "subdir", isDir: true},
		&mockDirEntry{name: "file.txt", isDir: false, size: 10, modTime: time.Now()},
	}

	svc.dirReader = &mockDirReader{entries: mockEntries}
	svc.containerDetector = &mockContainerDetector{encryptedPaths: map[string]bool{}}

	files, err := svc.ListFiles("/my/path")
	require.NoError(t, err)
	require.Len(t, files, 2)

	var subdirEntry, fileEntry *FileInfo
	for i := range files {
		if files[i].IsDirectory {
			subdirEntry = &files[i]
		} else {
			fileEntry = &files[i]
		}
	}
	require.NotNil(t, subdirEntry)
	require.NotNil(t, fileEntry)
	assert.Equal(t, "/my/path/subdir", subdirEntry.Path)
	assert.Equal(t, "/my/path/file.txt", fileEntry.Path)
}

// TestListFiles_RootPathSlashHandling 验证根路径 "/" 的 path 拼接正确
func TestListFiles_RootPathSlashHandling(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	mockEntries := []fs.DirEntry{
		&mockDirEntry{name: "rootfile.txt", isDir: false, size: 99, modTime: time.Now()},
	}

	svc.dirReader = &mockDirReader{entries: mockEntries}
	svc.containerDetector = &mockContainerDetector{encryptedPaths: map[string]bool{}}

	files, err := svc.ListFiles("/")
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "/rootfile.txt", files[0].Path, "根路径下不应出现双斜杠 //")
}

// TestListFiles_NilDIUsesRealFS 验证不注入依赖时回退到真实 os.ReadDir（nil DI 回退验证）
func TestListFiles_NilDIUsesRealFS(t *testing.T) {
	svc, dir := newTestMobileService(t)

	os.WriteFile(filepath.Join(dir, "real_file.txt"), []byte("real data"), 0644)

	assert.Nil(t, svc.dirReader, "默认情况下 dirReader 应为 nil")
	assert.Nil(t, svc.containerDetector, "默认情况下 containerDetector 应为 nil")

	files, err := svc.ListFiles("/")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	var found bool
	for _, f := range files {
		if f.Name == "real_file.txt" {
			found = true
			break
		}
	}
	assert.True(t, found, "nil DI 时应使用真实 os.ReadDir 并能读取到实际文件")
}

// TestListFiles_ConcurrentSafeMockReads 验证并发调用 mock DirReader 的线程安全性
func TestListFiles_ConcurrentSafeMockReads(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	mockR := &mockDirReader{
		entries: []fs.DirEntry{
			&mockDirEntry{name: "concurrent.txt", isDir: false, size: 77, modTime: time.Now()},
		},
	}
	svc.dirReader = mockR
	svc.containerDetector = &mockContainerDetector{encryptedPaths: map[string]bool{}}

	const goroutines = 20
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			files, err := svc.ListFiles("/")
			if err != nil {
				errCh <- err
				return
			}
			if len(files) != 1 || files[0].Name != "concurrent.txt" {
				errCh <- assert.AnError
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("并发测试失败: %v", err)
	}

	assert.Equal(t, goroutines, mockR.callCount, "mock ReadDir 应被调用 %d 次", goroutines)
}

// TestListFiles_MixedDirAndFileWithEncryption 验证混合目录和文件且含加密文件的场景
func TestListFiles_MixedDirAndFileWithEncryption(t *testing.T) {
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)

	absDir, _ := filepath.Abs(dir)
	now := time.Now()
	mockEntries := []fs.DirEntry{
		&mockDirEntry{name: "docs", isDir: true},
		&mockDirEntry{name: "readme.md", isDir: false, size: 256, modTime: now},
		&mockDirEntry{name: "movie.mp4", isDir: false, size: 1048576, modTime: now},
		&mockDirEntry{name: "backup" + pluginsext.VideoExt, isDir: false, size: 5000000, modTime: now},
		&mockDirEntry{name: ".env", isDir: false},
		&mockDirEntry{name: "music.flac", isDir: false, size: 30000000, modTime: now},
	}

	detector := &mockContainerDetector{
		encryptedPaths: map[string]bool{
			filepath.Join(absDir, "backup"+pluginsext.VideoExt): true,
		},
	}

	svc.dirReader = &mockDirReader{entries: mockEntries}
	svc.containerDetector = detector

	files, err := svc.ListFiles("/")
	require.NoError(t, err)
	require.Len(t, files, 5, ".env 应被跳过，其余 5 个应保留")

	nameMap := make(map[string]*FileInfo)
	for i := range files {
		nameMap[files[i].Name] = &files[i]
	}

	assert.True(t, nameMap["docs"].IsDirectory)
	assert.False(t, nameMap["readme.md"].IsEncrypted)
	assert.False(t, nameMap["movie.mp4"].IsEncrypted)
	assert.True(t, nameMap["backup"+pluginsext.VideoExt].IsEncrypted, "backup 容器应标记为加密")
	assert.Equal(t, int64(5000000), nameMap["backup"+pluginsext.VideoExt].Size)
	assert.False(t, nameMap["music.flac"].IsEncrypted)
	_, hasEnv := nameMap[".env"]
	assert.False(t, hasEnv, ".env 不应出现在结果中")
}

// TestReadFileContent_MockContainerDetected 验证 ReadFileContent 使用 mock detector 识别加密容器
func TestReadFileContent_MockContainerDetected(t *testing.T) {
	svc, dir := newTestMobileService(t)

	absPath := filepath.Join(dir, "encrypted"+pluginsext.VideoExt)
	os.WriteFile(absPath, []byte("fake container data"), 0644)

	svc.containerDetector = &mockContainerDetector{
		encryptedPaths: map[string]bool{absPath: true},
	}

	result, err := svc.ReadFileContent("/encrypted"+pluginsext.VideoExt)
	require.Error(t, err, "加密容器应返回 BadRequestError")
	require.Nil(t, result)
	var badReq *BadRequestError
	require.ErrorAs(t, err, &badReq)
	assert.Contains(t, err.Error(), "is_encv_container")
}

// TestReadFileContent_MockContainerNotDetected 验证 ReadFileContent 使用 mock detector 识别普通文件
func TestReadFileContent_MockContainerNotDetected(t *testing.T) {
	svc, dir := newTestMobileService(t)

	os.WriteFile(filepath.Join(dir, "normal.log"), []byte("log line\n"), 0644)

	svc.containerDetector = &mockContainerDetector{
		encryptedPaths: map[string]bool{},
	}

	result, err := svc.ReadFileContent("/normal.log")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "normal.log", result.Name)
	assert.Equal(t, "log line\n", result.Content)
	assert.Equal(t, "utf-8", result.Encoding)
}

// TestGetFileInfo_MockContainerDetected 验证 GetFileInfo 使用 mock detector 识别容器
func TestGetFileInfo_MockContainerDetected(t *testing.T) {
	svc, dir := newTestMobileService(t)

	absPath := filepath.Join(dir, "container"+pluginsext.VideoExt)
	os.WriteFile(absPath, []byte("not a real container but mock says yes"), 0644)

	svc.containerDetector = &mockContainerDetector{
		encryptedPaths: map[string]bool{absPath: true},
	}

	info, err := svc.GetFileInfo("/container"+pluginsext.VideoExt)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.True(t, info.IsEncvContainer, "mock 标记的容器应被识别")
	assert.True(t, info.IsEncrypted)
	assert.Equal(t, "encrypted", info.Category)
}

// TestGetFileInfo_MockContainerNotDetected 验证 GetFileInfo 使用 mock detector 识别普通文件
func TestGetFileInfo_MockContainerNotDetected(t *testing.T) {
	svc, dir := newTestMobileService(t)

	os.WriteFile(filepath.Join(dir, "image.png"), []byte("\x89PNG\r\n\x1a\nfake"), 0644)

	svc.containerDetector = &mockContainerDetector{
		encryptedPaths: map[string]bool{},
	}

	info, err := svc.GetFileInfo("/image.png")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.False(t, info.IsEncvContainer)
	assert.False(t, info.IsEncrypted)
}

// TestHelperMethod_readDirNilFallback 验证 readDir 在 dirReader 为 nil 时回退到 os.ReadDir
func TestHelperMethod_readDirNilFallback(t *testing.T) {
	svc, dir := newTestMobileService(t)

	assert.Nil(t, svc.dirReader, "前置条件：dirReader 为 nil")

	os.WriteFile(filepath.Join(dir, "fallback_test.txt"), []byte("data"), 0644)

	entries, err := svc.readDir(svc.servingDir)
	require.NoError(t, err)
	assert.NotEmpty(t, entries, "nil fallback 应成功读取 servingDir")
}

// TestHelperMethod_detectContainerNilFallback 验证 detectContainer 在 containerDetector 为 nil 时回退到真实检测
func TestHelperMethod_detectContainerNilFallback(t *testing.T) {
	svc, _ := newTestMobileService(t)

	assert.Nil(t, svc.containerDetector, "前置条件：containerDetector 为 nil")

	result := svc.detectContainer("/nonexistent_path_that_is_not_a_container")
	assert.False(t, result, "不存在的路径不应被检测为容器")
}
