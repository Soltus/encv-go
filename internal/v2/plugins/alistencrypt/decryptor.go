package alistencrypt

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DecryptFile 解密容器文件，按插件命名范式还原原始文件名：
//   - 容器文件 = EncodeName(完整原始名) + 容器后缀（如 .bin）
//   - 通过 DecodeName 的 CRC6 校验判断密码是否正确（密码错会 CRC 失败）
//   - 还原失败时使用 orig_<baseName-no-suffix> 兜底（避免用户误以为"丢后缀名"）
func DecryptFile(containerPath, outputDir, password, encType string) (string, error) {
	baseName := filepath.Base(containerPath)
	containerExt := filepath.Ext(baseName)
	if containerExt != "" && containerExt != ".bin" && containerExt != ".alist" && containerExt != ".enc" {
		return "", &DecryptionError{Reason: "invalid format", Err: ErrInvalidFormat}
	}

	// 范式：从 baseName 去掉容器后缀后调 DecodeName 还原原始完整文件名
	encPart := TrimContainerExt(baseName)
	originalName := DecodeName(encPart, password, encType)
	if originalName == "" {
		// CRC6 校验失败 = 密码错 或 文件非本插件编码。绝不静默写出 "丢 ext" 的文件名
		return "", &DecryptionError{Reason: "password mismatch or unsupported container", Err: ErrInvalidPassword}
	}

	f, err := os.Open(containerPath)
	if err != nil {
		return "", fmt.Errorf("failed to open container file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat container file: %w", err)
	}
	fileSize := info.Size()

	dr, err := NewDecryptReader(f, password, fileSize)
	if err != nil {
		return "", fmt.Errorf("failed to create decrypt reader: %w", err)
	}

	decryptedData, err := io.ReadAll(dr)
	if err != nil {
		return "", fmt.Errorf("failed to read decrypted data: %w", err)
	}

	outputPath := filepath.Join(outputDir, originalName)

	outFile, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	if _, err := outFile.Write(decryptedData); err != nil {
		return "", fmt.Errorf("failed to write decrypted data: %w", err)
	}

	return outputPath, nil
}

// TrimContainerExt 剥去插件容器后缀（默认 .bin），用于解码失败时的兜底文件名
func TrimContainerExt(name string) string {
	for _, ext := range []string{".bin", ".alist", ".enc"} {
		if len(name) > len(ext) && name[len(name)-len(ext):] == ext {
			return name[:len(name)-len(ext)]
		}
	}
	return name
}
