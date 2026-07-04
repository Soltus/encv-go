package text

import (
	"fmt"
	"log"
	"os"

	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// 实现 plugins.MetadataExtractor 接口
type TextMetadataExtractor struct {
	// 可以在这里注入依赖，例如配置
}

// 提取元数据
func (e *TextMetadataExtractor) ExtractMetadata(inputPath string) (types.Index, error) {
	// 1. 获取基础文件信息
	fileInfo, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	mimeType, err := utils.DetectFileMIMEType(inputPath)
	if err != nil {
		log.Printf("failed to detect MIME type: %v", err)
	}
	index := &TextIndex{
		OriginalFilename: fileInfo.Name(),
		OriginalFileSize: fileInfo.Size(),
		MimeType:         mimeType,
	}

	return index, nil
}
