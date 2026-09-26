// Package simple 定义 ENCV 的「轻量信封」ENCVS1，面向前端（VuePress / Obsidian / 思源笔记插件）。
//
// # 设计目的
//
// 完整容器（v2/v3/v4，.sccg* 系列）带有 2048 字节信封头、Manifest、Segment、CRC 等结构，
// 对「一条笔记 / 一段文本 / 一个小文件」来说过重，且这些结构依赖 os 文件语义，无法在浏览器里直接复用。
//
// ENCVS1 只保留加密层本身，并且**不做任何自己的密码学实现**：
//
//   - 密钥派生：crypto.GenerateKey（PBKDF2-HMAC-SHA256, Iterations_v2=10000）
//   - 数据加解密：crypto.EncryptBytes_v2 / crypto.DecryptBytes_v2（AES-CTR，无填充）
//   - 口令校验：crypto.CalculatePasswordHint / VerifyPasswordHint（与 v4 容器同一算法）
//   - 随机数：crypto.GenerateSalt_v2 / GenerateIV_v2（crypto/rand，js/wasm 下可用）
//
// 因此 WASM 版（编译同一份 Go 代码）与 Go 原生版天然字节级对等：
// 不存在「前端再写一套」的机会，也不需要靠跨语言对齐测试来兜底。
//
// # 信封布局（ENCVS1，小端，与 types.ByteOrder_v2 一致）
//
//	Offset  Size  Field          说明
//	0       4     Magic          'E','N','C','S'（区别于容器的 'ENCV'，检测器不会误判）
//	4       1     FormatVersion  1
//	5       2     CipherMode     0=AES-128-CTR / 1=AES-256-CTR（与 crypto.CipherMode_v4 同义）
//	7       1     KDF            0=PBKDF2-HMAC-SHA256
//	8       4     Iterations     uint32
//	12      1     SaltLen
//	13      1     IVLen
//	14      1     HintLen
//	15      16    PasswordHint   口令校验值（crypto.CalculatePasswordHint）
//	31      S     Salt
//	31+S    16    IV
//	31+S+16 rest  Ciphertext     AES-CTR 密文，长度 == 明文长度
//
// 固定头部 31 字节（HeaderSize）。
//
// # 迭代次数不可配置
//
// crypto.GenerateKey 内部硬编码 Iterations_v2，ENCVS1 为与之严格对等，
// 拒绝任何 Iterations != Iterations_v2 的请求（返回 ErrUnsupportedIterations）。
// 信封里仍然记录该值，供未来扩展与排障。
package simple

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
)

// 信封标识与版本。
const (
	// Magic 是 ENCVS1 的魔数。刻意与容器的 "ENCV"（types.MagicHeader_v2）不同，
	// 保证 detector.DetectIndexKind 等容器识别逻辑不会把轻量信封误判为容器。
	Magic = "ENCS"
	// FormatVersion 是 ENCVS1 的格式版本号。
	FormatVersion = 1
	// HintSize 是口令校验值长度，与 crypto.PasswordHintSize 绑定。
	HintSize = crypto.PasswordHintSize
)

// 固定头部的字段偏移（单一事实来源，读写两侧共用）。
const (
	offFormatVersion = 4
	offCipherMode    = 5  // uint16
	offKDF           = 7  // uint8
	offIterations    = 8  // uint32
	offSaltLen       = 12 // uint8
	offIVLen         = 13 // uint8
	offHintLen       = 14 // uint8
	offHint          = 15 // [16]byte
	// HeaderSize 是固定头部长度（不含 salt/iv）。
	HeaderSize = offHint + HintSize // 31
)

// KDF 标识。
const (
	KDFPBKDF2SHA256 uint8 = 0
)

// 密码模式：值直接取自加密层的枚举，避免两套常量漂移。
const (
	CipherAES128CTR uint16 = uint16(crypto.CipherModeAES128CTR) // 0
	CipherAES256CTR uint16 = uint16(crypto.CipherModeAES256CTR) // 1
)

// 默认值：与加密层的既有取值保持一致。
const (
	// DefaultCipherMode 默认 AES-256-CTR（crypto.GenerateKey 的默认密钥长度也是 32 字节）。
	DefaultCipherMode = CipherAES256CTR // uint16
	// DefaultIterations 与 crypto.Iterations_v2 强行绑定（见包注释）。
	DefaultIterations = uint32(crypto.Iterations_v2)
	// DefaultSaltLen 与 v2/v4 加密流水线（crypto.EncryptToTempFile_v2 / PrepareEncryptionContext）一致。
	DefaultSaltLen = uint8(types.SaltSize_v2)
	// DefaultIVLen 与 types.IVSize_v2 一致。
	DefaultIVLen = uint8(types.IVSize_v2)
)

// 错误定义。
var (
	ErrInvalidMagic      = errors.New("encvs1: invalid magic")
	ErrUnsupportedVer    = errors.New("encvs1: unsupported format version")
	ErrUnsupportedKDF    = errors.New("encvs1: unsupported kdf")
	ErrUnsupportedIter   = errors.New("encvs1: iterations must equal crypto.Iterations_v2")
	ErrUnsupportedCipher = errors.New("encvs1: unsupported cipher mode")
	ErrTruncated         = errors.New("encvs1: container truncated")
	ErrInvalidSaltLen    = errors.New("encvs1: invalid salt length")
	ErrInvalidIVLen      = errors.New("encvs1: invalid iv length")
	ErrInvalidHintLen    = errors.New("encvs1: invalid password hint length")
)

// Header 是 ENCVS1 的固定头部。
type Header struct {
	Magic         [4]byte
	FormatVersion uint8
	CipherMode    uint16
	KDF           uint8
	Iterations    uint32
	SaltLen       uint8
	IVLen         uint8
	HintLen       uint8
	Hint          [HintSize]byte
}

// Options 控制加密参数。零值/非法字段会被 Normalize 填充为默认值。
type Options struct {
	// CipherMode：CipherAES128CTR / CipherAES256CTR，其他值回退到 DefaultCipherMode。
	CipherMode uint16
	// Iterations：必须等于 DefaultIterations，否则 Encrypt 返回 ErrUnsupportedIter。
	Iterations uint32
	// SaltLen：盐长度，0 回退到 DefaultSaltLen。
	SaltLen uint8
}

// DefaultOptions 返回默认加密参数（AES-256-CTR / 10000 迭代 / 16 字节盐）。
func DefaultOptions() Options {
	return Options{
		CipherMode: DefaultCipherMode,
		Iterations: DefaultIterations,
		SaltLen:    DefaultSaltLen,
	}
}

// Normalize 规范化参数：非法/零值回退到默认，保证磁盘上不会出现非法取值。
//
// Iterations 只做「零值补默认」，不做「越界静默改写」——
// 显式传入非默认迭代次数一律报错（见 ValidateIterations）：
// 否则调用方以为用了 200000 迭代、实际拿到 10000，属安全陷阱。
func (o Options) Normalize() Options {
	switch crypto.CipherMode_v4(o.CipherMode) {
	case crypto.CipherModeAES128CTR, crypto.CipherModeAES256CTR:
	default:
		o.CipherMode = uint16(DefaultCipherMode)
	}
	if o.Iterations == 0 {
		o.Iterations = DefaultIterations
	}
	if o.SaltLen == 0 {
		o.SaltLen = DefaultSaltLen
	}
	return o
}

// ValidateIterations 校验迭代次数：等于 DefaultIterations 通过，其余一律拒绝。
func (o Options) ValidateIterations() error {
	if o.Iterations == DefaultIterations {
		return nil
	}
	return fmt.Errorf("%w: want %d, got %d", ErrUnsupportedIter, DefaultIterations, o.Iterations)
}

// KeySize 返回该 CipherMode 对应的密钥长度（字节），委托给加密层同一个函数。
func (o Options) KeySize() int {
	return crypto.KeySizeForCipherMode_v4(crypto.CipherMode_v4(o.CipherMode))
}

// PayloadOffset 返回密文在信封中的起始偏移。
func (h *Header) PayloadOffset() int {
	return HeaderSize + int(h.SaltLen) + int(h.IVLen)
}

// ParseHeader 解析信封头部，只做结构校验（不校验口令）。
func ParseHeader(blob []byte) (*Header, error) {
	if len(blob) < HeaderSize {
		return nil, fmt.Errorf("%w: need %d bytes, got %d", ErrTruncated, HeaderSize, len(blob))
	}
	h := &Header{}
	copy(h.Magic[:], blob[0:4])
	if string(h.Magic[:]) != Magic {
		return nil, ErrInvalidMagic
	}
	h.FormatVersion = blob[offFormatVersion]
	if h.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedVer, h.FormatVersion)
	}
	h.CipherMode = binary.LittleEndian.Uint16(blob[offCipherMode : offCipherMode+2])
	switch crypto.CipherMode_v4(h.CipherMode) {
	case crypto.CipherModeAES128CTR, crypto.CipherModeAES256CTR:
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedCipher, h.CipherMode)
	}
	h.KDF = blob[offKDF]
	if h.KDF != KDFPBKDF2SHA256 {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedKDF, h.KDF)
	}
	h.Iterations = binary.LittleEndian.Uint32(blob[offIterations : offIterations+4])
	if h.Iterations != DefaultIterations {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedIter, h.Iterations)
	}
	h.SaltLen = blob[offSaltLen]
	h.IVLen = blob[offIVLen]
	h.HintLen = blob[offHintLen]
	copy(h.Hint[:], blob[offHint:offHint+HintSize])

	if h.SaltLen == 0 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidSaltLen, h.SaltLen)
	}
	if h.IVLen != DefaultIVLen {
		return nil, fmt.Errorf("%w: %d", ErrInvalidIVLen, h.IVLen)
	}
	if h.HintLen != HintSize {
		return nil, fmt.Errorf("%w: %d", ErrInvalidHintLen, h.HintLen)
	}
	if len(blob) < h.PayloadOffset() {
		return nil, fmt.Errorf("%w: need %d bytes, got %d", ErrTruncated, h.PayloadOffset(), len(blob))
	}
	return h, nil
}

// Salt 返回信封中的盐（引用 blob，不拷贝）。
func (h *Header) Salt(blob []byte) []byte {
	return blob[HeaderSize : HeaderSize+int(h.SaltLen)]
}

// IV 返回信封中的 IV（引用 blob，不拷贝）。
func (h *Header) IV(blob []byte) []byte {
	off := HeaderSize + int(h.SaltLen)
	return blob[off : off+int(h.IVLen)]
}

// Payload 返回信封中的密文（引用 blob，不拷贝）。
func (h *Header) Payload(blob []byte) []byte {
	return blob[h.PayloadOffset():]
}

// Encrypt 使用随机盐/IV 加密明文，返回完整信封（header+salt+iv+ciphertext）。
func Encrypt(plain []byte, password string, opts Options) ([]byte, error) {
	opts = opts.Normalize()
	if err := opts.ValidateIterations(); err != nil {
		return nil, err
	}
	salt, err := crypto.GenerateSalt_v2(int(opts.SaltLen))
	if err != nil {
		return nil, fmt.Errorf("encvs1: failed to generate salt: %w", err)
	}
	iv, err := crypto.GenerateIV_v2(int(DefaultIVLen))
	if err != nil {
		return nil, fmt.Errorf("encvs1: failed to generate iv: %w", err)
	}
	return EncryptWithEntropy(plain, password, opts, salt, iv)
}

// EncryptWithEntropy 使用调用方给定的盐/IV 加密，用于生成确定性黄金测试向量。
//
// salt/iv 长度由 opts / DefaultIVLen 决定；长度不符直接报错，绝不静默截断。
func EncryptWithEntropy(plain []byte, password string, opts Options, salt, iv []byte) ([]byte, error) {
	opts = opts.Normalize()
	if err := opts.ValidateIterations(); err != nil {
		return nil, err
	}
	if len(salt) != int(opts.SaltLen) {
		return nil, fmt.Errorf("%w: want %d, got %d", ErrInvalidSaltLen, opts.SaltLen, len(salt))
	}
	if len(iv) != int(DefaultIVLen) {
		return nil, fmt.Errorf("%w: want %d, got %d", ErrInvalidIVLen, DefaultIVLen, len(iv))
	}

	// 严格复用加密层：密钥派生与 CTR 加解密都不在本包重新实现。
	key := crypto.GenerateKey(password, salt, opts.KeySize())
	ciphertext, err := crypto.EncryptBytes_v2(plain, key, iv)
	if err != nil {
		return nil, fmt.Errorf("encvs1: encrypt failed: %w", err)
	}

	hint, err := crypto.CalculatePasswordHint(password, salt)
	if err != nil {
		return nil, fmt.Errorf("encvs1: failed to calculate password hint: %w", err)
	}

	blob := make([]byte, HeaderSize+len(salt)+len(iv)+len(ciphertext))
	copy(blob[0:4], Magic)
	blob[offFormatVersion] = FormatVersion
	binary.LittleEndian.PutUint16(blob[offCipherMode:offCipherMode+2], opts.CipherMode)
	blob[offKDF] = KDFPBKDF2SHA256
	binary.LittleEndian.PutUint32(blob[offIterations:offIterations+4], opts.Iterations)
	blob[offSaltLen] = opts.SaltLen
	blob[offIVLen] = DefaultIVLen
	blob[offHintLen] = HintSize
	copy(blob[offHint:offHint+HintSize], hint[:])

	off := HeaderSize
	copy(blob[off:], salt)
	off += len(salt)
	copy(blob[off:], iv)
	off += len(iv)
	copy(blob[off:], ciphertext)

	return blob, nil
}

// Decrypt 校验口令并解密信封，返回明文。
func Decrypt(blob []byte, password string) ([]byte, error) {
	h, err := ParseHeader(blob)
	if err != nil {
		return nil, err
	}
	salt := h.Salt(blob)
	iv := h.IV(blob)
	payload := h.Payload(blob)

	// 与 v4 容器同一套口令校验：密码错误时明确报错，而不是吐出一堆乱码。
	if !crypto.VerifyPasswordHint(h.Hint, password, salt) {
		return nil, types.ErrWrongPassword
	}

	keyLen := crypto.KeySizeForCipherMode_v4(crypto.CipherMode_v4(h.CipherMode))
	key := crypto.GenerateKey(password, salt, keyLen)
	plain, err := crypto.DecryptBytes_v2(payload, key, iv)
	if err != nil {
		return nil, fmt.Errorf("encvs1: decrypt failed: %w", err)
	}
	return plain, nil
}

// EncryptString 是 string → 信封 的便捷封装（UTF-8）。
func EncryptString(plain string, password string, opts Options) ([]byte, error) {
	return Encrypt([]byte(plain), password, opts)
}

// DecryptString 解密并作为 UTF-8 字符串返回。
func DecryptString(blob []byte, password string) (string, error) {
	plain, err := Decrypt(blob, password)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// VerifyPassword 只做口令校验，不解密数据（用于「密码对不对」的快速判断）。
func VerifyPassword(blob []byte, password string) bool {
	h, err := ParseHeader(blob)
	if err != nil {
		return false
	}
	return crypto.VerifyPasswordHint(h.Hint, password, h.Salt(blob))
}
