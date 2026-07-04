package crypto

import (
	"fmt"
	"io"
	"os"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// EncryptionResult 封装了加密操作的所有结果
type EncryptionResult struct {
	TempPath             string // 临时加密文件路径
	Salt                 []byte // 加密盐
	IV                   []byte // 初始化向量
	SaltIVHeaderSize     int64  // 显式 Header 大小 (Salt + IV)
	EncryptedPayloadSize int64  // 加密数据载荷大小 (TotalSize - SaltIVHeaderSize)
}

// EncryptToTempFile 将数据流加密并保存到临时文件
//
// 【架构职责】
// 1. 生成 Salt 和 IV。
// 2. 将 Salt 和 IV 显式写入文件开头（作为加密文件的独立头）。
// 3. 使用 CTR 模式加密数据流并紧随其后写入。
// 4. 返回包含精确尺寸信息的结构体。
//
// 【输出文件结构】
// [Salt (16B)][IV (16B)][Encrypted_Data...]
func EncryptToTempFile_v2(src io.Reader, password string, outputDir string) (*EncryptionResult, error) {
	// 1. 生成加密参数
	salt, err := GenerateSalt_v2(types.SaltSize_v2)
	if err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	iv, err := GenerateIV_v2(types.IVSize_v2)
	if err != nil {
		return nil, fmt.Errorf("failed to generate iv: %w", err)
	}
	key := GenerateKey(password, salt, types.KeySize_v2)

	// 2. 创建临时文件
	tempFile, err := os.CreateTemp(outputDir, "*.enc.tmp")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	// 3. 【关键】显式写入 Salt 和 IV Header
	// 这部分数据构成了文件的“加密头”，总大小固定为 32 字节 (16+16)
	// 任何解密此文件的操作都必须先读取并剥离这 32 字节
	bytesWritten := 0

	if _, err := tempFile.Write(salt); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return nil, fmt.Errorf("failed to write salt: %w", err)
	}
	bytesWritten += len(salt)

	if _, err := tempFile.Write(iv); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return nil, fmt.Errorf("failed to write iv: %w", err)
	}
	bytesWritten += len(iv)

	saltIVSize := int64(bytesWritten)

	// 4. 执行数据加密
	// EncryptStream_v2 将 src 加密并写入 tempFile
	// 此时文件指针位于 saltIVSize 处，加密数据将紧随其后
	if err := EncryptStream_v2(src, tempFile, key, iv); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return nil, fmt.Errorf("failed to encrypt stream: %w", err)
	}

	// 5. 【性能优化】一次性获取文件元数据
	// 避免后续 Plugin 或 Packer 重复调用 Stat
	fileInfo, err := tempFile.Stat()
	if err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return nil, fmt.Errorf("failed to stat temp file: %w", err)
	}
	totalFileSize := fileInfo.Size()

	// 6. 关闭文件
	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return nil, fmt.Errorf("failed to close temp file: %w", err)
	}

	// 7. 构造返回结果
	// EncryptedPayloadSize 即为去掉 Salt/IV 头后的纯数据量
	payloadSize := totalFileSize - saltIVSize

	return &EncryptionResult{
		TempPath:             tempPath,
		Salt:                 salt,
		IV:                   iv,
		SaltIVHeaderSize:     saltIVSize,  // 明确指明是 Salt+IV 的大小
		EncryptedPayloadSize: payloadSize, // 等于明文大小
	}, nil
}
