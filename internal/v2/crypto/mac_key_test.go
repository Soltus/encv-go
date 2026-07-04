// internal/v2/crypto/mac_key_test.go
package crypto

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

// TestDeriveMACKey_Deterministic 验证相同输入产生相同输出。
//
// PBKDF2 是确定性 KDF，因此给定 (password, mac_salt, iter, len) 输出必须一致。
// 这条性质是写入/读取一致性的基础——读端从 Header 拿 mac_salt 重新派生必须
// 复现写端当时派生的 mac_key，否则 HMAC 校验必失败。
func TestDeriveMACKey_Deterministic(t *testing.T) {
	password := "my-strong-password-123"
	macSalt := bytes.Repeat([]byte{0xAA}, MACSaltLength)

	key1 := DeriveMACKey(password, macSalt)
	key2 := DeriveMACKey(password, macSalt)

	if !bytes.Equal(key1, key2) {
		t.Error("DeriveMACKey is not deterministic: same input produced different output")
	}

	// 长度必须严格为 32 字节
	if len(key1) != MACKeyLength {
		t.Errorf("DeriveMACKey returned %d bytes, want %d", len(key1), MACKeyLength)
	}
}

// TestDeriveMACKey_Different 验证不同 password 派生不同 key。
func TestDeriveMACKey_Different(t *testing.T) {
	macSalt := bytes.Repeat([]byte{0x42}, MACSaltLength)

	key1 := DeriveMACKey("password-A", macSalt)
	key2 := DeriveMACKey("password-B", macSalt)

	if bytes.Equal(key1, key2) {
		t.Error("Different passwords produced identical mac_keys")
	}
}

// TestDeriveMACKey_SaltIndependence 验证 mac_key 与 encrypt_salt 完全独立。
//
// 关键安全性质：encrypt_key 派生用的 salt 与 mac_key 派生用的 salt 必须不同，
// 否则会触发 key reuse——attacker 拿到 mac_key 后可通过相同 KDF 推出 encrypt_key。
//
// 测试设计：
//   - encrypt_salt 和 mac_salt 取完全不同的 16 字节
//   - 各自派生 key 应当不同
//   - 同时验证：相同密码 + 不同 salt → 不同 key（PBKDF2 对 salt 敏感）
func TestDeriveMACKey_SaltIndependence(t *testing.T) {
	password := "shared-password"
	encryptSalt := bytes.Repeat([]byte{0x11}, MACSaltLength)
	macSalt := bytes.Repeat([]byte{0x22}, MACSaltLength)

	encryptKey := pbkdf2.Key([]byte(password), encryptSalt, MACKeyIterations, MACKeyLength, sha256.New)
	macKey := DeriveMACKey(password, macSalt)

	if bytes.Equal(encryptKey, macKey) {
		t.Error("mac_key should not equal encrypt_key when derived from different salts")
	}

	// 反向验证：相同 salt 派生两次结果一致（PBKDF2 确定性）
	macKey2 := DeriveMACKey(password, macSalt)
	if !bytes.Equal(macKey, macKey2) {
		t.Error("DeriveMACKey is not deterministic across same-salt calls")
	}

	// 修改 salt 任意一字节 → key 完全不同
	macSaltMod := make([]byte, len(macSalt))
	copy(macSaltMod, macSalt)
	macSaltMod[0] ^= 0x01
	macKey3 := DeriveMACKey(password, macSaltMod)
	if bytes.Equal(macKey, macKey3) {
		t.Error("Modifying 1 bit of mac_salt should produce completely different mac_key")
	}
}

// TestDeriveMACKey_EmptyPassword 验证空密码也可派生（边界场景）。
func TestDeriveMACKey_EmptyPassword(t *testing.T) {
	macSalt := bytes.Repeat([]byte{0x33}, MACSaltLength)
	key := DeriveMACKey("", macSalt)
	if len(key) != MACKeyLength {
		t.Errorf("DeriveMACKey with empty password returned %d bytes, want %d",
			len(key), MACKeyLength)
	}
}

// TestDeriveMACKey_EmptySalt 验证空 salt 也可派生（边界场景，不推荐使用）。
func TestDeriveMACKey_EmptySalt(t *testing.T) {
	password := "some-password"
	key := DeriveMACKey(password, []byte{})
	if len(key) != MACKeyLength {
		t.Errorf("DeriveMACKey with empty salt returned %d bytes, want %d",
			len(key), MACKeyLength)
	}
}

// TestGenerateMACSalt_Uniqueness 验证多次调用返回不同 salt。
func TestGenerateMACSalt_Uniqueness(t *testing.T) {
	const N = 100
	salts := make([][]byte, N)
	for i := 0; i < N; i++ {
		s, err := GenerateMACSalt()
		if err != nil {
			t.Fatalf("GenerateMACSalt() at iteration %d failed: %v", i, err)
		}
		salts[i] = s
	}

	// 两两比较：相同 salt 的概率应为 2^(-128) ≈ 0
	for i := 0; i < N; i++ {
		for j := i + 1; j < N; j++ {
			if bytes.Equal(salts[i], salts[j]) {
				t.Errorf("GenerateMACSalt() produced identical salt at iterations %d and %d", i, j)
			}
		}
	}
}

// TestGenerateMACSalt_Length 验证返回 16 字节。
func TestGenerateMACSalt_Length(t *testing.T) {
	salt, err := GenerateMACSalt()
	if err != nil {
		t.Fatalf("GenerateMACSalt() failed: %v", err)
	}
	if len(salt) != MACSaltLength {
		t.Errorf("GenerateMACSalt() returned %d bytes, want %d", len(salt), MACSaltLength)
	}
	if MACSaltLength != 16 {
		t.Errorf("MACSaltLength = %d, must be 16 (128 bit)", MACSaltLength)
	}
}

// TestMACSalt_DeriveKeyRoundTrip 端到端验证：GenerateMACSalt → DeriveMACKey → VerifyHMAC。
//
// 这是最贴近真实使用场景的集成测试：
//   1. 生成 mac_salt
//   2. 派生 mac_key
//   3. 用 mac_key 计算 HMAC
//   4. 验证 HMAC（应通过）
//   5. 修改 1 bit 密文（模拟比特翻转攻击）→ 验证应失败
func TestMACSalt_DeriveKeyRoundTrip(t *testing.T) {
	password := "round-trip-password"
	message := []byte("important ciphertext content")

	// 1. 生成 mac_salt
	macSalt, err := GenerateMACSalt()
	if err != nil {
		t.Fatalf("GenerateMACSalt() failed: %v", err)
	}

	// 2. 派生 mac_key
	macKey := DeriveMACKey(password, macSalt)

	// 3. 计算 HMAC
	mac := HMACSHA1_80(macKey, message)
	macBytes := mac[:]

	// 4. 验证应通过
	if !VerifyHMACSHA1_80(macBytes, message, macKey) {
		t.Error("VerifyHMACSHA1_80 failed for self-derived mac_key")
	}

	// 5. 模拟 attacker 翻转密文 1 bit → 验证应失败
	corrupted := make([]byte, len(message))
	copy(corrupted, message)
	corrupted[5] ^= 0x80 // 翻转第 6 字节的最高位
	if VerifyHMACSHA1_80(macBytes, corrupted, macKey) {
		t.Error("Verify should fail for 1-bit-flipped ciphertext (bit-flipping attack)")
	}

	// 6. 模拟 attacker 用错误密码派生 mac_key → 验证应失败
	wrongPassword := "wrong-password"
	wrongMacKey := DeriveMACKey(wrongPassword, macSalt)
	if VerifyHMACSHA1_80(macBytes, message, wrongMacKey) {
		t.Error("Verify should fail for mac_key derived from wrong password")
	}
}
