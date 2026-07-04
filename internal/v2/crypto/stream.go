package crypto

import (
	"crypto/aes"
	"fmt"
	"io"
	"os"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// DeriveCTRIVForOffset_v2 根据全局 base IV 与字节偏移，计算 CTR 模式实际使用的 IV
func DeriveCTRIVForOffset_v2(baseIV []byte, byteOffset uint64) ([]byte, error) {
	if len(baseIV) != aes.BlockSize {
		return nil, fmt.Errorf("baseIV length must be %d", aes.BlockSize)
	}
	iv := make([]byte, aes.BlockSize)
	copy(iv, baseIV)

	blockIndex := byteOffset / uint64(aes.BlockSize)
	var carry uint64 = blockIndex
	for i := aes.BlockSize - 1; i >= aes.BlockSize-8; i-- {
		sum := uint64(iv[i]) + (carry & 0xff)
		iv[i] = byte(sum & 0xff)
		carry = (carry >> 8) + (sum >> 8)
		if i == aes.BlockSize-8 {
			break
		}
	}
	return iv, nil
}

// EncryptToTempFile 将一个数据流加密，并保存到一个临时文件中。
// 它返回临时文件的路径、生成的盐和初始化向量（IV）。
// 调用者负责在使用完毕后删除这个临时文件。
// Padding 无法解决 GOP Fragment 边界未对齐导致的数据丢失问题。
// 解决方案必须是在解密端正确处理 IV 偏移，而不是在加密端添加垃圾数据。
func EncryptToTempFile(src io.Reader, password string, outputDir string) (tempPath string, salt []byte, iv []byte, err error) {
	// 1. 生成加密参数
	salt, err = GenerateSalt_v2(types.SaltSize_v2)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	iv, err = GenerateIV_v2(types.IVSize_v2)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to generate iv: %w", err)
	}
	key := GenerateKey(password, salt, types.KeySize_v2)

	// 2. 创建临时文件
	tempFile, err := os.CreateTemp(outputDir, "*.enc.tmp")
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath = tempFile.Name()

	// 3. 执行加密
	err = EncryptStream_v2(src, tempFile, key, iv)
	if err != nil {
		tempFile.Close()
		os.Remove(tempPath) // 失败时清理
		return "", nil, nil, fmt.Errorf("failed to encrypt stream: %w", err)
	}

	// 4. 关闭文件并返回结果
	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath) // 失败时清理
		return "", nil, nil, fmt.Errorf("failed to close temp file: %w", err)
	}

	return tempPath, salt, iv, nil
}
