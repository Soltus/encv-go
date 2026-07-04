package namer

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	ChunkNameRuleSequential = ".part"
)

// SequentialNamer 是一个顺序命名的实现，例如 .part1, .part2
type SequentialNamer struct {
	baseNamer    BaseNamer
	suffix       string // 例如 ".part"
	mainChunkExt string // 【关键】内部存储主容器后缀
}

func NewSequentialNamer(mainChunkExt string, baseNamer BaseNamer, suffix string) *SequentialNamer {
	return &SequentialNamer{
		mainChunkExt: mainChunkExt,
		baseNamer:    baseNamer,
		suffix:       suffix,
	}
}

// GenerateMainChunkName 使用内部存储的 mainChunkExt
func (n *SequentialNamer) GenerateMainChunkName(baseName string) string {
	return baseName + n.mainChunkExt
}

// ParseFirstChunkName 使用内部存储的 mainChunkExt
func (n *SequentialNamer) ParseFirstChunkName(firstChunkPath string) (string, error) {
	return parseFirstChunkNameHelper(firstChunkPath, n.mainChunkExt)
}

// GenerateDataChunkName 覆盖核心结构体的方法，实现自己的逻辑
func (n *SequentialNamer) GenerateDataChunkName(baseName string, index int) string {
	return fmt.Sprintf("%s%s%d", baseName, n.suffix, index)
}
func (n *SequentialNamer) GetFirstDataChunkIndex() int {
	return 1
}

// IsDataChunk 检查文件名是否是 .part1, .part2 格式的碎片
func (n *SequentialNamer) IsDataChunk(filename string) bool {
	base := filepath.Base(filename)

	// 检查是否以 .part 开头
	if !strings.HasPrefix(base, n.suffix) {
		return false
	}

	// 获取 .part 后面的数字部分
	numStr := base[len(n.suffix):]
	_, err := strconv.Atoi(numStr)
	return err == nil
}
