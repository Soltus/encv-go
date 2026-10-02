package peerlink

// crypto.go —— 配对密钥派生 / proof 校验 / 报文 AEAD / SAS 短码
//
// 威胁模型：Hub（cnb 上的服务）视为**不可信中转**。
//   - 配对 psk 经 HKDF-SHA256 派生两个方向的 AEAD 密钥（A→B、B→A）
//   - Hub 只转发密文，读不到明文（R5）
//   - SAS 6 位短码由 psk 派生，供双端人工核对，防 Hub 侧 MITM

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// NewPSK 生成 32 字节配对密钥。
func NewPSK() ([]byte, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("peerlink: psk: %w", err)
	}
	return b, nil
}

// PSKHex / MustDecodePSK 用于二维码里以 hex 传输 psk。
func PSKHex(psk []byte) string { return hex.EncodeToString(psk) }

func DecodePSK(s string) ([]byte, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("peerlink: psk decode: %w", err)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("peerlink: psk length %d, want 32", len(b))
	}
	return b, nil
}

// Proof 计算配对证明：HMAC-SHA256(psk, pairingID+"|"+deviceID)。
// 目的：证明"扫码方真的看到了二维码里的 psk"，而不是 Hub 伪造的一次连接。
func Proof(psk []byte, pairingID, deviceID string) string {
	m := hmac.New(sha256.New, psk)
	_, _ = io.WriteString(m, pairingID+"|"+deviceID)
	return hex.EncodeToString(m.Sum(nil))
}

// VerifyProof 恒定时间比较，防止时序侧信道。
func VerifyProof(psk []byte, pairingID, deviceID, proof string) bool {
	want := Proof(psk, pairingID, deviceID)
	return subtle.ConstantTimeCompare([]byte(want), []byte(proof)) == 1
}

// DeriveKeys 由 psk 派生两个方向的 AEAD 密钥（各 32 字节）。
//
//	a2b: 桌面（Hub 侧）→ 手机（Edge）
//	b2a: 手机 → 桌面
func DeriveKeys(psk []byte) (a2b, b2a []byte, err error) {
	a2b = make([]byte, 32)
	b2a = make([]byte, 32)
	if _, err = io.ReadFull(hkdf.New(sha256.New, psk, nil, []byte("encv-peerlink-a2b")), a2b); err != nil {
		return nil, nil, fmt.Errorf("peerlink: derive a2b: %w", err)
	}
	if _, err = io.ReadFull(hkdf.New(sha256.New, psk, nil, []byte("encv-peerlink-b2a")), b2a); err != nil {
		return nil, nil, fmt.Errorf("peerlink: derive b2a: %w", err)
	}
	return a2b, b2a, nil
}

// SAS 返回 6 位短认证串（Short Authentication String），双端各显示供人工核对。
// 同一 psk 在两端派生结果一致。
func SAS(psk []byte) string {
	m := hmac.New(sha256.New, psk)
	_, _ = io.WriteString(m, "encv-peerlink-sas")
	sum := m.Sum(nil)
	n := uint32(sum[0])<<24 | uint32(sum[1])<<16 | uint32(sum[2])<<8 | uint32(sum[3])
	return fmt.Sprintf("%06d", n%1000000)
}

// Seal 用 AEAD(AES-GCM) 加密报文（nonce 前置）。
// Hub 侧看到的就是这段密文，无法解密（R5）。
func Seal(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("peerlink: seal cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("peerlink: seal gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("peerlink: seal nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Open 解密 Seal 产出的密文。
func Open(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("peerlink: open cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("peerlink: open gcm: %w", err)
	}
	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("peerlink: open: ciphertext too short (%d)", len(ciphertext))
	}
	nonce, body := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	out, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, fmt.Errorf("peerlink: open: %w", err)
	}
	return out, nil
}
