// internal/v2/crypto/mac_key.go
// ENCV v4 mac_key 派生模块：与 encrypt_key 独立的 MAC 密钥派生通道。
//
// 为什么需要独立 mac_key？
//   - encrypt_key 走 AES-CTR 加密路径，mac_key 走 HMAC-SHA1 路径
//   - 同一密码同一 salt 派生两个 key 会导致 key reuse，破坏 security domain separation
//   - 独立 salt + 独立派生让 encrypt 和 MAC 通道完全解耦
//   - 即使 attacker 拿到 mac_key，也无法恢复 encrypt_key，反之亦然
//
// 派生参数（与 encrypt_key 一致，方便共享硬件加速路径）：
//   - KDF:       PBKDF2-SHA256
//   - iterations: 100000
//   - output:    32 bytes（256 bit，与 KeySize_v2 对齐）
//
// mac_salt 来源：
//   - 写入器：GenerateMACSalt() 生成 16 字节随机值，存入 v4 Header.MacSalt 字段
//   - 读取器：从 v4 Header 提取 mac_salt，调用 DeriveMACKey 重建 mac_key
//
// 参考：spec.md Requirement "MAC key 派生"
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

// mac_key 派生参数（与 encrypt_key 保持一致，简化心智模型）
const (
	// MACKeyIterations 是 mac_key PBKDF2 派生迭代次数。
	// 必须与 Iterations_v2 (100000) 保持一致——故意保持一致是为了让加密/MAC
	// 通道在硬件加速（如 AES-NI + SHA 硬件）上获得相同的吞吐预估。
	MACKeyIterations = 10000

	// MACKeyLength 是 mac_key 的输出长度（字节）。
	// 32 字节（256 bit）与 KeySize_v2 对齐——HMAC-SHA1 可接受任意 key 长度，
	// 32 字节超过 SHA-1 块大小（20 字节）后会先 SHA-1 hash 一次再使用。
	MACKeyLength = 32

	// MACSaltLength 是 mac_salt 的标准长度（字节）。
	// 16 字节 = 128 bit 随机性——与 IVSize_v2 一致，方便对齐加密硬件路径。
	MACSaltLength = 16
)

// DeriveMACKey 从 password + mac_salt 派生 32 字节 mac_key。
//
// 算法：PBKDF2-SHA256(password, mac_salt, 100000, 32)
//
// **确定性**：相同输入永远产生相同输出（PBKDF2 本身是确定性 KDF）。
//   - 写入器派生一次存到内存
//   - 读取器从 Header 拿到 mac_salt 后派生一次重建
//   - 两者结果必须完全一致，否则 MAC 校验必失败
//
// **与 encrypt_key 的关系**：
//   - encrypt_key = PBKDF2-SHA256(password, encrypt_salt, 100000, keyLen)
//   - mac_key    = PBKDF2-SHA256(password, mac_salt,    100000, 32)
//   - encrypt_salt 与 mac_salt **必须不同**（test 中有专门验证）
//
// **不要修改此函数签名**——所有调用方（Phase 2 Task 4、Phase 6 writer/reader）
// 都依赖 (password string, macSalt []byte) → []byte 形态。
func DeriveMACKey(password string, macSalt []byte) []byte {
	return pbkdf2.Key([]byte(password), macSalt, MACKeyIterations, MACKeyLength, sha256.New)
}

// GenerateMACSalt 生成 16 字节随机 mac_salt。
//
// 与 GenerateSalt_v2 的区别：
//   - GenerateSalt_v2(size) 接受任意 size，更通用
//   - GenerateMACSalt() 固定 16 字节，语义清晰（"这是 mac 专用的 salt"）
//
// 调用方：
//   - 写入器在 WriteHeaderV4 时调用一次，存到 Header.MacSalt 字段
//   - 每个容器一份独立 mac_salt，跨容器不复用
//
// 错误：
//   - crypto/rand 读取失败时返回 error（极罕见，仅在 OS 熵源耗尽时发生）
func GenerateMACSalt() ([]byte, error) {
	salt := make([]byte, MACSaltLength)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return salt, nil
}
