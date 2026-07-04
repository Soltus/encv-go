package namer

import (
	"fmt"
	"path/filepath"
	"strings"
)

// BaseNamer 负责生成加密文件的基础名（例如 "321.4pm"）
// 它是一个纯粹的、无状态的字符串转换工具
type BaseNamer interface {
	GenerateEncryptedBaseName(originalFilename string) string
}

// --- 【拆分】接口2：分片级命名 ---
// ChunkNamer 负责物理分片的命名规则
type ChunkNamer interface {
	//  生成主容器文件名（例如 "321.4pm.sccgv"）
	GenerateMainChunkName(baseName string) string
	//  从主容器文件路径中解析出基础名
	ParseFirstChunkName(firstChunkPath string) (baseName string, err error)
	//  根据基础名和索引生成数据分片文件名
	GenerateDataChunkName(baseName string, index int) string
	//  返回第一个数据分片的索引
	GetFirstDataChunkIndex() int
	// 【新增】判断一个文件名是否是由此规则生成的数据碎片
	IsDataChunk(filename string) bool
}

func generateReversedExt(ext string) string {
	if len(ext) > 0 && ext[0] == '.' {
		ext = ext[1:]
	}
	runes := []rune(ext)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// 从主容器文件路径中解析出基础名
func parseFirstChunkNameHelper(firstChunkPath string, mainChunkExt string) (string, error) {
	dir := filepath.Dir(firstChunkPath)
	filename := filepath.Base(firstChunkPath)

	if strings.HasSuffix(filename, mainChunkExt) {
		baseName := strings.TrimSuffix(filename, mainChunkExt)
		return filepath.Join(dir, baseName), nil
	}

	return "", fmt.Errorf("filename '%s' does not look like a main container file (expected to end with '%s')", filename, mainChunkExt)
}
