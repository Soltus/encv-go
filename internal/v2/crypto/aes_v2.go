// internal/v2/crypto/aes_v2.go
package crypto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"github.com/Soltus/encv-go/internal/v2/crypto/keys"
	"golang.org/x/crypto/pbkdf2"
)

var (
	ErrInvalidKeyLength_v2 = errors.New("invalid key length")
	ErrInvalidIVLength_v2  = errors.New("invalid IV length")
)

const (
	// Algorithm 加密算法
	Algorithm_v2 = "aes-256-ctr"
	// KeySize 密钥长度
	KeySize_v2 = 32
	// IVSize_v2 是 AES CTR 模式 IV 的标准长度
	IVSize_v2 = aes.BlockSize
	// SaltSize 盐值长度
	SaltSize_v2 = 32
	// Iterations PBKDF2 迭代次数
	Iterations_v2 = 100000
)

// CipherMode_v4 标识 v4 容器使用的 AES 密钥长度（CTR 模式）。
//
// 字段值定义（与 v4 Header.CipherMode 直接对应）：
//   - CipherModeAES128CTR = 0：使用 16 字节密钥（AES-128-CTR），v4 新容器默认
//   - CipherModeAES256CTR = 1：使用 32 字节密钥（AES-256-CTR），v4 可选加强档
//
// 历史背景：v4 早期版本硬编码 32 字节（AES-256）。引入此枚举后
// 默认改为 AES-128，与 WinZip / RAR / 7-Zip 行业惯例对齐。
type CipherMode_v4 uint16

const (
	// CipherModeAES128CTR AES-128-CTR（16 字节密钥），v4 默认
	CipherModeAES128CTR CipherMode_v4 = 0
	// CipherModeAES256CTR AES-256-CTR（32 字节密钥），v4 可选
	CipherModeAES256CTR CipherMode_v4 = 1
)

// KeySize_v4_* 给出 v4 容器不同 CipherMode 对应的密钥长度（字节）。
//
// 关系：
//   - KeySize_v4_128 = aes.BlockSize = 16
//   - KeySize_v4_256 = 2 * aes.BlockSize = 32
//
// 与 aes.NewCipher 的合法输入一致（16/24/32 字节），故现有的
// EncryptStream_v2 / DecryptStream_v2 无需修改即可直接接收 16 字节 key。
const (
	KeySize_v4_128 = aes.BlockSize      // 16
	KeySize_v4_256 = 2 * aes.BlockSize  // 32
)

// KeySizeForCipherMode_v4 根据 CipherMode 枚举返回对应的密钥长度（字节）。
// 未知 / 越界值 fallback 到 AES-128-CTR（与 v4 Header 默认一致）。
func KeySizeForCipherMode_v4(mode CipherMode_v4) int {
	switch mode {
	case CipherModeAES256CTR:
		return KeySize_v4_256
	default:
		return KeySize_v4_128
	}
}

// GenerateKey 使用 PBKDF2 从密码和盐派生密钥
func GenerateKey(password string, salt []byte, keyLen int) []byte {
	if keyLen <= 0 {
		keyLen = KeySize_v2 // 默认 AES-256
	}
	return pbkdf2.Key([]byte(password), salt, 100000, keyLen, sha256.New)
}

// GenerateKey_v4 是 v4 容器专用的密钥派生函数。
//
// 与 GenerateKey 的关系：本函数是 GenerateKey 的 v4 命名别名，
// 签名与算法完全一致（PBKDF2-SHA256, 100000 iter），便于调用方
// 按 v4 spec 命名引用，同时保留 v2 旧 API 的可用性。
//
// 支持的 keyLen 取值（与 CipherMode_v4 一一对应）：
//   - 16 → AES-128-CTR（CipherModeAES128CTR）
//   - 32 → AES-256-CTR（CipherModeAES256CTR）
//
// keyLen <= 0 或其他值统一 fallback 到 16 字节（v4 默认 AES-128）。
func GenerateKey_v4(password string, salt []byte, keyLen int) []byte {
	if keyLen <= 0 {
		keyLen = KeySize_v4_128 // v4 默认 AES-128
	}
	return pbkdf2.Key([]byte(password), salt, 100000, keyLen, sha256.New)
}

// GenerateSalt_v2 生成一个随机盐
func GenerateSalt_v2(size int) ([]byte, error) {
	salt := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return salt, nil
}

// EncryptStream_v2 使用 AES-CTR 加密一个 io.Reader
func EncryptStream_v2(src io.Reader, dst io.Writer, key, iv []byte) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create cipher block: %w", err)
	}
	if len(key) != block.BlockSize() && len(key) != 2*block.BlockSize() && len(key) != 4*block.BlockSize() {
		return ErrInvalidKeyLength_v2
	}
	if len(iv) != block.BlockSize() {
		return ErrInvalidIVLength_v2
	}

	stream := cipher.NewCTR(block, iv)
	writer := &cipher.StreamWriter{S: stream, W: dst}

	_, err = io.Copy(writer, src)
	return err
}

// DecryptStream_v2 使用 AES-CTR 解密一个 io.Reader
func DecryptStream_v2(src io.Reader, dst io.Writer, key, iv []byte) error {
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("failed to create cipher block: %w", err)
	}
	if len(key) != block.BlockSize() && len(key) != 2*block.BlockSize() && len(key) != 4*block.BlockSize() {
		return ErrInvalidKeyLength_v2
	}
	if len(iv) != block.BlockSize() {
		return ErrInvalidIVLength_v2
	}

	stream := cipher.NewCTR(block, iv)
	reader := &cipher.StreamReader{S: stream, R: src}

	_, err = io.Copy(dst, reader)
	return err
}

// GenerateIV_v2 生成一个随机 IV
func GenerateIV_v2(size int) ([]byte, error) {
	iv := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	return iv, nil
}

// Base64Encode_v2 编码为 Base64 字符串
func Base64Encode_v2(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// Base64Decode_v2 从 Base64 字符串解码
func Base64Decode_v2(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// DecryptBytes_v2 解密一个完整的字节切片，使用 CTR 模式以匹配加密端
func DecryptBytes_v2(ciphertext, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher block: %w", err)
	}

	// 【关键修正】使用 CTR 模式进行解密
	// CTR 模式不需要填充，也不需要检查密文长度是否为块大小的倍数
	stream := cipher.NewCTR(block, iv)
	plaintext := make([]byte, len(ciphertext))
	stream.XORKeyStream(plaintext, ciphertext)

	return plaintext, nil
}

// EncryptBytes_v2 使用 CTR 模式加密一个完整的字节切片
func EncryptBytes_v2(plaintext, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher block: %w", err)
	}

	// CTR 模式的加密和解密操作是完全相同的
	stream := cipher.NewCTR(block, iv)
	ciphertext := make([]byte, len(plaintext))
	stream.XORKeyStream(ciphertext, plaintext)

	return ciphertext, nil
}

// DecryptReader_v2 包装一个加密的 io.Reader，返回一个解密后的 io.Reader
// 它使用 CTR 模式，非常适合流式处理
func DecryptReader_v2(src io.Reader, key, iv []byte) (io.Reader, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// 创建 CTR 流
	stream := cipher.NewCTR(block, iv)

	// 返回一个 StreamReader，它会在读取时自动解密
	return &cipher.StreamReader{S: stream, R: src}, nil
}

// EncryptReaderToBytes_v2 读取 io.Reader 的全部内容，使用 AES-CTR 加密，并返回加密后的字节切片。
// 这是一个便利函数，用于将流式加密的结果一次性读入内存。
func EncryptReaderToBytes_v2(src io.Reader, key, iv []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := EncryptStream_v2(src, &buf, key, iv); err != nil {
		return nil, fmt.Errorf("failed to encrypt stream to bytes: %w", err)
	}
	return buf.Bytes(), nil
}

// DecryptReaderToBytes_v2 读取 io.Reader 的全部内容，使用 AES-CTR 解密，并返回解密后的字节切片。
func DecryptReaderToBytes_v2(src io.Reader, key, iv []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := DecryptStream_v2(src, &buf, key, iv); err != nil {
		return nil, fmt.Errorf("[DecryptReaderToBytes_v2] failed to decrypt stream to bytes: %w", err)
	}
	return buf.Bytes(), nil
}

// EncryptWriter_v2 包装一个 io.Writer，返回一个加密后的 io.Writer
// 它使用 CTR 模式，非常适合流式处理
func EncryptWriter_v2(dst io.Writer, key, iv []byte) (io.Writer, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// 创建 CTR 流
	stream := cipher.NewCTR(block, iv)

	// 返回一个 StreamWriter，它会在写入时自动加密
	return &cipher.StreamWriter{S: stream, W: dst}, nil
}

// =========== 以下为系统内置密钥加密解密相关函数 ===========

// EncryptSystemPayload 使用系统内置密钥加密数据块（如 Manifest）。
// 算法：AES-256-CTR
// 返回：IV (16 bytes) + Ciphertext
// IV 拼接在密文头部，方便解密时提取。
func EncryptSystemPayload(plainData []byte) ([]byte, error) {
	key := keys.GetSystemKey()

	// 1. 生成随机 IV (16 bytes)
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV for system payload encryption: %w", err)
	}

	// 2. 创建 AES Block Cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// 3. 创建 CTR 流
	stream := cipher.NewCTR(block, iv)

	// 4. 执行加密 (CTR 模式支持并行，这里使用 XORKeyStream 标准接口)
	cipherText := make([]byte, len(plainData))
	stream.XORKeyStream(cipherText, plainData)

	// 5. 格式化：IV (16 bytes) + CipherText
	encrypted := make([]byte, aes.BlockSize+len(cipherText))
	copy(encrypted[:aes.BlockSize], iv)
	copy(encrypted[aes.BlockSize:], cipherText)

	return encrypted, nil
}

// DecryptSystemPayload 使用系统内置密钥解密数据块。
// 输入：IV (16 bytes) + Ciphertext
func DecryptSystemPayload(encryptedData []byte) ([]byte, error) {
	key := keys.GetSystemKey()

	// 1. 提取 IV
	if len(encryptedData) < aes.BlockSize {
		return nil, fmt.Errorf("encrypted payload too short to contain IV")
	}
	iv := encryptedData[:aes.BlockSize]
	cipherText := encryptedData[aes.BlockSize:]

	// 2. 创建 AES Block Cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// 3. 创建 CTR 流
	stream := cipher.NewCTR(block, iv)

	// 4. 执行解密
	plainData := make([]byte, len(cipherText))
	stream.XORKeyStream(plainData, cipherText)

	return plainData, nil
}
