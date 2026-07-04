package text

import (
	"io"
	"os"
)

// 实现 plugins.ContentPreprocessor 接口
type TextContentPreprocessor struct {
	// 可以在这里注入依赖
}

// Preprocess 预处理图片内容
func (p *TextContentPreprocessor) Preprocess(inputPath string) (io.ReadCloser, error) {
	return os.Open(inputPath)
}
