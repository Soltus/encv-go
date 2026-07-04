package keys

import (
	"crypto/sha256"
	"encoding/hex"
)

// ManifestEncryptionKey 是用于加密/解密 Manifest 块的系统级密钥。
// 它不依赖用户输入的密码，而是派生自一个固定常量。
// 这意味着任何持有 EncV 客户端的人都可以解密 Manifest 元数据。
// 这种设计权衡了元数据的隐秘性（如文件名）与加密数据的绝对安全性（内容需要密码）。
var SystemKey []byte

func init() {
	// 使用固定常量生成密钥，确保不同构建/语言移植时密钥一致
	// "encv-manifest-global-key-v2" 是固定的种子
	seed := "encv-manifest-global-key-v2"
	hash := sha256.Sum256([]byte(seed))
	SystemKey = hash[:]
}

// GetSystemKey 返回用于加密 Manifest 的密钥
func GetSystemKey() []byte {
	return SystemKey
}

// GetSystemKeyHex 返回密钥的十六进制字符串表示，方便跨语言调试
func GetSystemKeyHex() string {
	return hex.EncodeToString(SystemKey)
}
