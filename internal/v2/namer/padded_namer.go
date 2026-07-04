package namer

import (
	"fmt"
	"path/filepath"
	"strconv"
)

const (
	ChunkNameRulePadded = ".padded" // 这是一个新的规则标识符
)

// PaddedNamer 是一个补零命名的实现，例如 .001, .002
type PaddedNamer struct {
	baseNamer    BaseNamer
	mainChunkExt string // 【关键】内部存储主容器后缀
	padding      int    // 补零的位数，例如 3 表示 001
}

func NewPaddedNamer(mainChunkExt string, baseNamer BaseNamer, padding int) *PaddedNamer {
	return &PaddedNamer{
		mainChunkExt: mainChunkExt,
		baseNamer:    baseNamer,
		padding:      padding,
	}
}

func (n *PaddedNamer) GenerateMainChunkName(baseName string) string {
	return baseName + n.mainChunkExt
}

func (n *PaddedNamer) ParseFirstChunkName(firstChunkPath string) (string, error) {
	return parseFirstChunkNameHelper(firstChunkPath, n.mainChunkExt)
}

func (n *PaddedNamer) GenerateDataChunkName(baseName string, index int) string {
	return fmt.Sprintf("%s.%0*d", baseName, n.padding, index)
}

func (n *PaddedNamer) GetFirstDataChunkIndex() int {
	return 1
}

// IsDataChunk 检查文件名是否是 .0001, .0002 格式的碎片
func (n *PaddedNamer) IsDataChunk(filename string) bool {
	base := filepath.Base(filename)
	ext := filepath.Ext(base) // 获取 .0001 这样的后缀

	// 检查后缀长度是否足够（例如 padding=3, 后缀至少是 .000）
	if len(ext) < n.padding+1 {
		return false
	}

	// 去掉点，尝试转换为数字
	numStr := ext[1:]
	_, err := strconv.Atoi(numStr)
	return err == nil
}
