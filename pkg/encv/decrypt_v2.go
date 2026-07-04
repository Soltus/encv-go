// pkg/encv/decrypt_v2.go
package encv

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Soltus/encv-go/internal/service"
	"github.com/Soltus/encv-go/internal/v2/plugins"
)

// DecryptPathV2 解密指定目录中的所有容器。
func DecryptPathV2(ctx context.Context, inputDir, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory '%s': %w", outputDir, err)
	}

	log.Printf("-> Scanning directory '%s' for containers to decrypt...", inputDir)
	return filepath.WalkDir(inputDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		p, err := plugins.FindDecryptingPlugin(path)
		if err != nil {
			// 如果没有插件处理，直接跳过
			return nil
		}

		log.Printf("--> Found container: %s", path)
		if _, err := plugins.DecryptContainerWithPlugin(ctx, p, path, outputDir, nil); err != nil {
			log.Printf("WARN: [DecryptPathV2] Failed to decrypt '%s': %v. Continuing...", path, err)
		}
		return nil
	})
}

func Preview(ctx context.Context, inputPath string) error {
	// 将实际工作委托给 internal/service
	return service.Preview(ctx, inputPath)
}
