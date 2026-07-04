// internal/v2/crypto/mac.go
// HMAC-SHA1-80 是 ENCV v4 Segment 的完整性保护原语。
//
// 参考规范：WinZip AES (AE-2) 规范明确指出：
//   "HMAC-SHA1-80 is used because it is a mature and widely respected
//    authentication algorithm, with a correspondingly modest implementation
//    footprint, and because HMAC-SHA1-80 was already selected for use in
//    the ZipArchive specification for the same reasons."
//
// 关键设计点：
//   1. SHA-1 输出 20 字节；MAC 截断到 10 字节（80 比特），符合 WinZip AE-2 规范
//   2. 校验必须使用 crypto/subtle.ConstantTimeCompare 防侧信道
//   3. expected 长度必须严格等于 10 字节（HMACSize_v4 = 10），否则视为不匹配
//   4. MAC 长度为常量数组 [10]byte，栈分配，无堆压力
package crypto

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"errors"
)

// HMACSize_v4 是 ENCV v4 Segment 中存储的 HMAC 截断长度（WinZip 规范固定 10 字节）。
const HMACSize_v4 = 10

// ErrMACMismatch 在 HMAC 校验失败时返回。
// 解密端在验 MAC 失败时必须立即返回此错误，不得继续 CTR 解密/解压。
var ErrMACMismatch = errors.New("HMAC verification failed")

// HMACSHA1_80 使用 key 对 message 计算 HMAC-SHA1，并截断到前 10 字节（80 比特）。
//
// 算法流程（参考 RFC 2104 + WinZip AE-2 规范）：
//   1. 构造 HMAC-SHA1 实例：mac = hmac.New(sha1.New, key)
//   2. mac.Write(message) → 内部产出 20 字节 SHA-1 输出
//   3. 取 sum[:10] 作为 80 比特截断 MAC
//
// 为什么截断到 80 比特：
//   - WinZip AE-2 规范要求 HMAC-SHA1-80（即 10 字节截断）
//   - 80 比特对密文比特翻转攻击的防御足够：单 bit 翻转的未检出概率为 1 - 2^(-80) ≈ 0
//   - 10 字节相对 20 字节节省 50% 存储（每 Segment 省 10 字节）
//
// 注意：本函数不做任何参数校验，调用方需自行保证 key 非空、message 可为 nil。
func HMACSHA1_80(key []byte, message []byte) [HMACSize_v4]byte {
	// 固定大小数组返回：[10]byte 由编译器在栈上分配，无堆分配
	var mac [HMACSize_v4]byte

	// hmac.New + mac.Sum 等价于 hmac.New().Write().Sum()
	// 显式分两步更清晰：先构造，再 Write，最后 Sum 到固定数组
	h := hmac.New(sha1.New, key)
	h.Write(message)
	sum := h.Sum(nil) // sum 长度 = sha1.Size = 20

	// 截断到 10 字节（必须 copy，避免 sum 内部 buffer 持有引用）
	copy(mac[:], sum[:HMACSize_v4])
	return mac
}

// VerifyHMACSHA1_80 校验 expected MAC 是否等于 HMAC-SHA1-80(key, message)。
//
// **关键安全约束：使用 crypto/subtle.ConstantTimeCompare 防侧信道攻击。**
// 早退式比较（bytes.Equal）在不同位置上不同步返回，攻击者可通过测量响应时间
// 推断出哪几个字节匹配，从而逐步恢复 MAC。ConstantTimeCompare 保证总比较时间
// 仅依赖于输入长度，不依赖于内容。
//
// 返回值：
//   - true:  校验通过
//   - false: expected 长度 != HMACSize_v4 (10) 或 MAC 不匹配
//
// 调用方应将 false 翻译为 ErrMACMismatch 向上抛出，禁止继续 CTR 解密。
func VerifyHMACSHA1_80(expected []byte, message []byte, key []byte) bool {
	// 长度必须严格匹配：长度不同的 expected 在语义上根本不是 HMACSize_v4 字节
	if len(expected) != HMACSize_v4 {
		return false
	}

	// 计算 expected MAC
	computed := HMACSHA1_80(key, message)

	// ConstantTimeCompare 返回 1 表示相等，0 表示不等
	// 注意：subtle.ConstantTimeCompare 的输入必须是等长切片（上面已保证）
	return subtle.ConstantTimeCompare(computed[:], expected) == 1
}
