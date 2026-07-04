package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

func newTestMobileService(t *testing.T) (*MobileService, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := defaultTestConfig()
	svc := NewMobileService(dir, cfg)
	return svc, dir
}

// TestListFiles_RootPath 验证 ListFiles("/") 返回临时目录中的条目
func TestListFiles_RootPath(t *testing.T) {
	svc, dir := newTestMobileService(t)

	os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("hello"), 0644)
	os.Mkdir(filepath.Join(dir, "subdir"), 0755)
	os.WriteFile(filepath.Join(dir, ".hidden"), []byte("hidden"), 0644)

	files, err := svc.ListFiles("/")
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, f := range files {
		names[f.Name] = true
	}
	assert.Contains(t, names, "file1.txt")
	assert.Contains(t, names, "subdir")
	assert.NotContains(t, names, ".hidden", "隐藏文件（以 . 开头）不应出现在结果中")
}

// TestListFiles_NonExistent 验证不存在的目录返回 NotFoundError
func TestListFiles_NonExistent(t *testing.T) {
	svc, _ := newTestMobileService(t)

	files, err := svc.ListFiles("/no-such-dir")
	assert.Nil(t, files)
	require.Error(t, err)
	var notFound *NotFoundError
	require.ErrorAs(t, err, &notFound)
}

// TestListFiles_PathTraversal 验证路径遍历攻击返回 ForbiddenError
func TestListFiles_PathTraversal(t *testing.T) {
	svc, _ := newTestMobileService(t)

	files, err := svc.ListFiles("../../etc")
	assert.Nil(t, files)
	require.Error(t, err)
	var forbidden *ForbiddenError
	require.ErrorAs(t, err, &forbidden)
}

// TestListFiles_WithFiles 创建子目录和文件，验证返回正确的 FileInfo 列表
func TestListFiles_WithFiles(t *testing.T) {
	svc, dir := newTestMobileService(t)

	os.Mkdir(filepath.Join(dir, "movies"), 0755)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# readme"), 0644)
	os.WriteFile(filepath.Join(dir, "video.mp4"), []byte("fake video data"), 0644)

	files, err := svc.ListFiles("/")
	require.NoError(t, err)
	require.Len(t, files, 3)

	var moviesFound, readmeFound, videoFound bool
	for _, f := range files {
		switch f.Name {
		case "movies":
			moviesFound = true
			assert.True(t, f.IsDirectory)
			assert.Equal(t, "/movies", f.Path)
		case "readme.md":
			readmeFound = true
			assert.False(t, f.IsDirectory)
			assert.Equal(t, "/readme.md", f.Path)
		case "video.mp4":
			videoFound = true
			assert.False(t, f.IsDirectory)
			assert.Equal(t, "/video.mp4", f.Path)
			assert.Greater(t, f.Size, int64(0))
		}
	}
	assert.True(t, moviesFound, "应包含 movies 目录")
	assert.True(t, readmeFound, "应包含 readme.md 文件")
	assert.True(t, videoFound, "应包含 video.mp4 文件")
}

// TestGetFileInfo_EmptyPath 验证空路径返回 BadRequestError
func TestGetFileInfo_EmptyPath(t *testing.T) {
	svc, _ := newTestMobileService(t)

	info, err := svc.GetFileInfo("")
	assert.Nil(t, info)
	require.Error(t, err)
	var badReq *BadRequestError
	require.ErrorAs(t, err, &badReq)
}

// TestGetFileInfo_NonExistent 验证不存在的文件返回 NotFoundError
func TestGetFileInfo_NonExistent(t *testing.T) {
	svc, _ := newTestMobileService(t)

	info, err := svc.GetFileInfo("/missing")
	assert.Nil(t, info)
	require.Error(t, err)
	var notFound *NotFoundError
	require.ErrorAs(t, err, &notFound)
}

// TestGetFileInfo_Directory 验证查询目录时 IsDirectory=true
func TestGetFileInfo_Directory(t *testing.T) {
	svc, dir := newTestMobileService(t)

	os.Mkdir(filepath.Join(dir, "myfolder"), 0755)

	info, err := svc.GetFileInfo("/myfolder")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "myfolder", info.Name)
	assert.Equal(t, "/myfolder", info.Path)
	assert.True(t, info.IsDirectory)
	assert.False(t, info.IsEncvContainer)
}

// TestGetFileInfo_NormalFile 验证 .txt 文件的 MimeType 和 Category
func TestGetFileInfo_NormalFile(t *testing.T) {
	svc, dir := newTestMobileService(t)

	os.WriteFile(filepath.Join(dir, "note.txt"), []byte("hello world"), 0644)

	info, err := svc.GetFileInfo("/note.txt")
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "note.txt", info.Name)
	assert.False(t, info.IsDirectory)
	assert.Equal(t, "text/plain; charset=utf-8", info.MimeType)
	assert.Equal(t, "document", info.Category)
	assert.Greater(t, info.Size, int64(0))
	assert.NotEmpty(t, info.Modified)
}

// TestGetFileInfo_V4Container 使用 CreateV4Fixture 创建容器，验证容器元数据
func TestGetFileInfo_V4Container(t *testing.T) {
	svc, dir := newTestMobileService(t)

	fixture := testutil.CreateV4Fixture(t, 1024, 1)

	containerName := filepath.Base(fixture.Path)
	destPath := filepath.Join(dir, containerName)
	data, err := os.ReadFile(fixture.Path)
	require.NoError(t, err)
	err = os.WriteFile(destPath, data, 0644)
	require.NoError(t, err)

	info, err := svc.GetFileInfo("/" + containerName)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.True(t, info.IsEncvContainer, "应识别为 ENCV 容器")
	assert.True(t, info.IsEncrypted)
	assert.Equal(t, "encrypted", info.Category)
	require.NotNil(t, info.Container, "Container 字段不应为 nil")

	_, hasVersion := info.Container["version"]
	assert.True(t, hasVersion, "Container 应包含 version 字段")
	_, hasType := info.Container["container_type"]
	assert.True(t, hasType, "Container 应包含 container_type 字段")
	_, hasSeekable := info.Container["is_seekable"]
	assert.True(t, hasSeekable, "Container 应包含 is_seekable 字段")

	assert.Equal(t, 4, info.Container["version"])
	assert.Equal(t, "video", info.Container["container_type"])
	assert.True(t, info.Container["is_seekable"].(bool))
}

// TestDeleteFile_Success 创建文件后删除，验证文件不存在
func TestDeleteFile_Success(t *testing.T) {
	svc, dir := newTestMobileService(t)

	filePath := filepath.Join(dir, "to_delete.txt")
	os.WriteFile(filePath, []byte("delete me"), 0644)

	assert.FileExists(t, filePath)

	err := svc.DeleteFile("/to_delete.txt")
	require.NoError(t, err)
	assert.NoFileExists(t, filePath)
}

// TestDeleteFile_NotFound 删除不存在的文件返回 NotFoundError
func TestDeleteFile_NotFound(t *testing.T) {
	svc, _ := newTestMobileService(t)

	err := svc.DeleteFile("/nonexistent.txt")
	require.Error(t, err)
	var notFound *NotFoundError
	require.ErrorAs(t, err, &notFound)
}

// TestReadFileContent_TextFile 创建 UTF-8 文本文件，验证 Content 和 Encoding
func TestReadFileContent_TextFile(t *testing.T) {
	svc, dir := newTestMobileService(t)

	content := "你好，世界！Hello World 🌍"
	os.WriteFile(filepath.Join(dir, "greeting.txt"), []byte(content), 0644)

	result, err := svc.ReadFileContent("/greeting.txt")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "greeting.txt", result.Name)
	assert.Equal(t, "/greeting.txt", result.Path)
	assert.Equal(t, content, result.Content)
	assert.Equal(t, "utf-8", result.Encoding)
	assert.Greater(t, result.Size, int64(0))
}

// TestReadFileContent_Binary 创建二进制文件，验证 Size 正确且 Encoding=binary
func TestReadFileContent_Binary(t *testing.T) {
	svc, dir := newTestMobileService(t)

	binaryData := []byte{0x00, 0x01, 0xFF, 0xFE, 0x80, 0x00}
	os.WriteFile(filepath.Join(dir, "binary.bin"), binaryData, 0644)

	result, err := svc.ReadFileContent("/binary.bin")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "binary.bin", result.Name)
	assert.Equal(t, int64(len(binaryData)), result.Size)
	assert.Equal(t, "binary", result.Encoding)
}

// TestCheckStoragePermission_HasPermission t.TempDir() 一定有权限，返回 true
func TestCheckStoragePermission_HasPermission(t *testing.T) {
	svc, _ := newTestMobileService(t)

	hasPerm := svc.CheckStoragePermission()
	assert.True(t, hasPerm, "t.TempDir() 创建的目录一定有读写权限")
}

// TestMobileService_V4Container_ListFilesDetectsEncrypted 验证 ListFiles 能正确识别 V4 容器为已加密文件
func TestMobileService_V4Container_ListFilesDetectsEncrypted(t *testing.T) {
	svc, dir := newTestMobileService(t)

	fixture := testutil.CreateV4Fixture(t, 1024, 2)

	containerName := filepath.Base(fixture.Path)
	destPath := filepath.Join(dir, containerName)
	data, err := os.ReadFile(fixture.Path)
	require.NoError(t, err)
	err = os.WriteFile(destPath, data, 0644)
	require.NoError(t, err)

	files, err := svc.ListFiles("/")
	require.NoError(t, err)
	require.NotEmpty(t, files, "ListFiles 应返回至少一个文件")

	var v4FileFound bool
	for _, f := range files {
		if f.Name == containerName {
			v4FileFound = true
			assert.True(t, f.IsEncrypted, "V4 容器文件应被标记为加密")
			assert.False(t, f.IsDirectory, "V4 容器文件不应是目录")
			assert.Greater(t, f.Size, int64(0), "V4 容器文件大小应大于 0")
			break
		}
	}
	assert.True(t, v4FileFound, "应在文件列表中找到 V4 容器文件")
}

// TestMobileService_V4Container_GetFileInfoRecognizes 验证 GetFileInfo 对 V4 容器返回完整的容器信息
func TestMobileService_V4Container_GetFileInfoRecognizes(t *testing.T) {
	svc, dir := newTestMobileService(t)

	fixture := testutil.CreateV4Fixture(t, 2048, 3)

	containerName := filepath.Base(fixture.Path)
	destPath := filepath.Join(dir, containerName)
	data, err := os.ReadFile(fixture.Path)
	require.NoError(t, err)
	err = os.WriteFile(destPath, data, 0644)
	require.NoError(t, err)

	info, err := svc.GetFileInfo("/" + containerName)
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.True(t, info.IsEncvContainer, "应识别为 ENCV 容器")
	assert.True(t, info.IsEncrypted, "应标记为已加密")
	assert.Equal(t, "encrypted", info.Category, "分类应为 encrypted")
	require.NotNil(t, info.Container, "Container 字段不应为 nil")

	version, hasVersion := info.Container["version"]
	assert.True(t, hasVersion, "Container 应包含 version 字段")
	assert.Equal(t, 4, version, "版本号应为 4")

	isSeekable, hasSeekable := info.Container["is_seekable"]
	assert.True(t, hasSeekable, "Container 应包含 is_seekable 字段")
	assert.True(t, isSeekable.(bool), "V4 容器应为可寻址")

	segCount, hasSegCount := info.Container["segment_count"]
	assert.True(t, hasSegCount, "Container 应包含 segment_count 字段")
	assert.Greater(t, segCount.(int), 0, "segment_count 应大于 0")

	containerType, hasType := info.Container["container_type"]
	assert.True(t, hasType, "Container 应包含 container_type 字段")
	assert.Equal(t, "video", containerType, "容器类型应为 video")
}

// TestMobileService_V4Container_EncryptFlowRoundtrip 验证完整的加密→检测→识别链路
func TestMobileService_V4Container_EncryptFlowRoundtrip(t *testing.T) {
	svc, dir := newTestMobileService(t)

	fixture := testutil.CreateV4Fixture(t, 512, 1)

	containerName := filepath.Base(fixture.Path)
	destPath := filepath.Join(dir, containerName)
	data, err := os.ReadFile(fixture.Path)
	require.NoError(t, err)
	err = os.WriteFile(destPath, data, 0644)
	require.NoError(t, err)

	files, err := svc.ListFiles("/")
	require.NoError(t, err)

	var targetFile *FileInfo
	for i := range files {
		if files[i].Name == containerName {
			targetFile = &files[i]
			break
		}
	}
	require.NotNil(t, targetFile, "应在列表中找到 V4 容器文件")
	assert.True(t, targetFile.IsEncrypted, "ListFiles 检测: IsEncrypted 应为 true")

	info, err := svc.GetFileInfo("/" + containerName)
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.True(t, info.IsEncvContainer, "GetFileInfo 识别: IsEncvContainer 应为 true")
	assert.True(t, info.IsEncrypted, "GetFileInfo 识别: IsEncrypted 应为 true")
	assert.Equal(t, "encrypted", info.Category, "GetFileInfo 分类: Category 应为 'encrypted'")
	require.NotNil(t, info.Container, "Container 元数据不应为 nil")

	assert.Equal(t, 4, info.Container["version"], "容器版本应为 4")
	assert.NotNil(t, info.Container["is_seekable"], "应包含 is_seekable 字段")
	assert.Greater(t, info.Container["segment_count"].(int), 0, "segment_count 应大于 0")

	category := info.Category
	isEncrypted := info.IsEncrypted
	assert.Equal(t, "encrypted", category, "模拟前端逻辑: getFileCategory(name, isEncrypted=true) 应返回 'encrypted'")
	assert.True(t, isEncrypted, "模拟前端逻辑: isEncrypted 标志应为 true")
}
