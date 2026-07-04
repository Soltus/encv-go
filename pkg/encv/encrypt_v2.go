package encv

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Soltus/encv-go/internal/v2/plugins"
)

// EncryptPathV2 加密指定的路径（文件或目录）到输出目录，自动识别文件类型。
func EncryptPathV2(ctx context.Context, inputPath, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory '%s': %w", outputDir, err)
	}

	info, err := os.Stat(inputPath)
	if err != nil {
		return fmt.Errorf("cannot access input path '%s': %w", inputPath, err)
	}

	if info.IsDir() {
		log.Printf("-> Scanning directory '%s' for files to encrypt...", inputPath)
		return plugins.WalkAndEncrypt(ctx, inputPath, inputPath, outputDir)
	} else {
		log.Printf("-> Encrypting single file: %s", inputPath)
		p, err := plugins.FindEncryptingPlugin(inputPath)
		if err != nil {
			return err
		}
		_, err = plugins.EncryptFileWithPlugin(ctx, p, inputPath, filepath.Dir(inputPath), outputDir, nil)
		return err
	}
}
