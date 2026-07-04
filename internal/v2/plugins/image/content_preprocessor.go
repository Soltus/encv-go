package image

import (
	"io"
	"os"
)

// 实现 plugins.ContentPreprocessor 接口
type ImageContentPreprocessor struct {
	// 可以在这里注入依赖
}

// Preprocess 预处理图片内容
func (p *ImageContentPreprocessor) Preprocess(inputPath string) (io.ReadCloser, error) {
	return os.Open(inputPath)
}
