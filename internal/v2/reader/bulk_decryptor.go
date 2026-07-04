package reader

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"io"
	"os"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// BulkDecryptor 提供高性能的、顺序的全量解密功能。
type BulkDecryptor struct {
	containerPath string
	password      string
}

// NewBulkDecryptor 创建一个新的 BulkDecryptor 实例。
// 注意：这是一个“内部”构造函数，通常由 DecryptReaderFactory 调用。
func NewBulkDecryptor(containerPath, password string) *BulkDecryptor {
	return &BulkDecryptor{
		containerPath: containerPath,
		password:      password,
	}
}

// DecryptToFile 将整个容器解密到指定的目标文件，并进行极致优化。
func (bd *BulkDecryptor) DecryptToFile(ctx context.Context, outputPath string) error {
	// 1. 创建一个临时的、高性能的容器读取器
	containerReader, err := NewEncryptedContainerReaderFromFile(bd.containerPath)
	if err != nil {
		return err
	}
	defer containerReader.Close()

	// 2. 打开目标文件
	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	// 3. 获取密钥和 IV
	manifest := containerReader.GetManifest()
	kviProvider, err := types.NewKVIProviderFromManifest(manifest)
	if err != nil {
		return fmt.Errorf("failed to create KVI provider from manifest: %w", err)
	}
	key, iv, err := deriveKeyAndIV(kviProvider, bd.password)
	if err != nil {
		return err
	}

	block, _ := aes.NewCipher(key)
	stream := cipher.NewCTR(block, iv)

	// 4. 按顺序处理所有 Fragment，无抽象开销
	for _, frag := range manifest.Fragments {
		if frag.Type == types.FragmentType_Metadata {
			continue
		}

		// 直接获取原始数据流
		fragReader, err := containerReader.GetFragmentReader(frag.ID)
		if err != nil {
			return err
		}
		defer fragReader.Close()

		// 创建解密流，直接写入文件
		decryptReader := &cipher.StreamReader{S: stream, R: fragReader}

		// 【性能优化】使用更大的缓冲区进行拷贝，减少系统调用
		buf := make([]byte, 4*1024*1024) // 4MB buffer
		if _, err := io.CopyBuffer(outFile, decryptReader, buf); err != nil {
			return err
		}
	}

	return nil
}
