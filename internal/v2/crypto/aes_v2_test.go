// internal/v2/crypto/aes_v2_test.go
package crypto

import (
	"bytes"
	"crypto/aes"
	"strings"
	"testing"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

func TestEncryptDecrypt_v2(t *testing.T) {
	password := "my-super-secret-password"
	salt, err := GenerateSalt_v2(SaltSize_v2)
	if err != nil {
		t.Fatalf("Failed to generate salt: %v", err)
	}

	key := GenerateKey(password, salt, KeySize_v2)
	iv, err := GenerateIV_v2(IVSize_v2)
	if err != nil {
		t.Fatalf("Failed to generate IV: %v", err)
	}

	// 保存原始字符串
	originalText := "This is a secret message that needs to be encrypted."
	plaintext := strings.NewReader(originalText)
	var ciphertext bytes.Buffer

	err = EncryptStream_v2(plaintext, &ciphertext, key, iv)
	if err != nil {
		t.Fatalf("Encryption failed: %v", err)
	}

	var decrypted bytes.Buffer
	ciphertextReader := bytes.NewReader(ciphertext.Bytes())
	err = DecryptStream_v2(ciphertextReader, &decrypted, key, iv)
	if err != nil {
		t.Fatalf("Decryption failed: %v", err)
	}

	// 比较解密后的字符串与原始字符串
	if decrypted.String() != originalText {
		t.Errorf("Decrypted text does not match original. Got %s", decrypted.String())
	}
}

func TestGenerateSalt_v2_Size16(t *testing.T) {
	salt, err := GenerateSalt_v2(16)
	if err != nil {
		t.Fatalf("GenerateSalt_v2(16) returned error: %v", err)
	}
	if len(salt) != 16 {
		t.Errorf("GenerateSalt_v2(16) returned length %d, want 16", len(salt))
	}
}

func TestGenerateSalt_v2_Size32(t *testing.T) {
	salt, err := GenerateSalt_v2(32)
	if err != nil {
		t.Fatalf("GenerateSalt_v2(32) returned error: %v", err)
	}
	if len(salt) != 32 {
		t.Errorf("GenerateSalt_v2(32) returned length %d, want 32", len(salt))
	}
}

func TestGenerateSalt_v2_ZeroSize(t *testing.T) {
	salt, err := GenerateSalt_v2(0)
	if err != nil {
		t.Fatalf("GenerateSalt_v2(0) returned unexpected error: %v", err)
	}
	if len(salt) != 0 {
		t.Errorf("GenerateSalt_v2(0) returned length %d, want 0", len(salt))
	}
}

func TestGenerateSalt_v2_Deterministic(t *testing.T) {
	salt1, err := GenerateSalt_v2(16)
	if err != nil {
		t.Fatalf("first GenerateSalt_v2(16) failed: %v", err)
	}
	salt2, err := GenerateSalt_v2(16)
	if err != nil {
		t.Fatalf("second GenerateSalt_v2(16) failed: %v", err)
	}
	if bytes.Equal(salt1, salt2) {
		t.Error("GenerateSalt_v2 produced identical salts; salts should be random")
	}
}

func TestGenerateIV_v2_CorrectSize(t *testing.T) {
	iv, err := GenerateIV_v2(aes.BlockSize)
	if err != nil {
		t.Fatalf("GenerateIV_v2(%d) returned error: %v", aes.BlockSize, err)
	}
	if len(iv) != aes.BlockSize {
		t.Errorf("GenerateIV_v2(%d) returned length %d, want %d", aes.BlockSize, len(iv), aes.BlockSize)
	}
}

func TestGenerateIV_v2_ZeroSize(t *testing.T) {
	iv, err := GenerateIV_v2(0)
	if err != nil {
		t.Fatalf("GenerateIV_v2(0) returned unexpected error: %v", err)
	}
	if len(iv) != 0 {
		t.Errorf("GenerateIV_v2(0) returned length %d, want 0", len(iv))
	}
}

func TestBase64Encode_v2_Roundtrip(t *testing.T) {
	original := []byte("hello world! \x00\x01\x02\xff\xfe")
	encoded := Base64Encode_v2(original)
	decoded, err := Base64Decode_v2(encoded)
	if err != nil {
		t.Fatalf("Base64Decode_v2 failed: %v", err)
	}
	if !bytes.Equal(decoded, original) {
		t.Errorf("Roundtrip mismatch: got %x, want %x", decoded, original)
	}
}

func TestBase64Encode_v2_Empty(t *testing.T) {
	encoded := Base64Encode_v2([]byte{})
	if encoded != "" {
		t.Errorf("Base64Encode_v2(empty) returned %q, want empty string", encoded)
	}
}

func TestEncryptDecryptBytes_v2_Roundtrip(t *testing.T) {
	password := "test-password-123"
	salt, _ := GenerateSalt_v2(SaltSize_v2)
	key := GenerateKey(password, salt, KeySize_v2)
	iv, _ := GenerateIV_v2(IVSize_v2)

	plaintext := []byte("EncryptBytes_v2 round-trip test data with some binary: \x00\xff\xab\xcd")

	ciphertext, err := EncryptBytes_v2(plaintext, key, iv)
	if err != nil {
		t.Fatalf("EncryptBytes_v2 failed: %v", err)
	}

	decrypted, err := DecryptBytes_v2(ciphertext, key, iv)
	if err != nil {
		t.Fatalf("DecryptBytes_v2 failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Roundtrip mismatch:\n  plaintext : %x\n  decrypted: %x", plaintext, decrypted)
	}
}

// TestGenerateKey_VariableLength 验证 GenerateKey / GenerateKey_v4 支持
// 16 字节（AES-128）和 32 字节（AES-256）两种密钥长度，且：
//  1. 相同 (password, salt) 派生出的 16 / 32 字节 key，前 16 字节完全相同
//     （PBKDF2 派生特性：长输出是短输出的扩展）
//  2. 两次相同输入派生结果确定（无随机性）
//  3. 不同 keyLen 派生结果互不相同
//  4. keyLen <= 0 时 fallback 到 16 字节（v4 默认）
//  5. 16 字节 key 可直接用于现有 EncryptStream_v2 / DecryptStream_v2
func TestGenerateKey_VariableLength(t *testing.T) {
	password := "p4ssw0rd-variable-len"
	salt := bytes.Repeat([]byte{0x42}, SaltSize_v2) // 32 字节固定 salt

	// ① 派生 16 字节（AES-128-CTR）
	key16 := GenerateKey(password, salt, KeySize_v4_128)
	if len(key16) != KeySize_v4_128 {
		t.Fatalf("GenerateKey(_, _, 16) length = %d, want %d", len(key16), KeySize_v4_128)
	}
	if len(key16) != 16 {
		t.Fatalf("GenerateKey_v4 16-byte mode: key length = %d, want 16", len(key16))
	}

	// ② 派生 32 字节（AES-256-CTR）
	key32 := GenerateKey(password, salt, KeySize_v4_256)
	if len(key32) != KeySize_v4_256 {
		t.Fatalf("GenerateKey(_, _, 32) length = %d, want %d", len(key32), KeySize_v4_256)
	}
	if len(key32) != 32 {
		t.Fatalf("GenerateKey_v4 32-byte mode: key length = %d, want 32", len(key32))
	}

	// ③ PBKDF2 特性：长输出是短输出的扩展 → 前 16 字节必须相同
	if !bytes.Equal(key16, key32[:16]) {
		t.Errorf("PBKDF2 extension property violated:\n  key16=%x\n  key32[:16]=%x",
			key16, key32[:16])
	}

	// ④ 确定性：相同输入两次派生结果必须完全相同
	key16Again := GenerateKey(password, salt, KeySize_v4_128)
	if !bytes.Equal(key16, key16Again) {
		t.Errorf("GenerateKey is not deterministic: %x vs %x", key16, key16Again)
	}
	key32Again := GenerateKey(password, salt, KeySize_v4_256)
	if !bytes.Equal(key32, key32Again) {
		t.Errorf("GenerateKey 32-byte mode is not deterministic: %x vs %x", key32, key32Again)
	}

	// ⑤ 不同 keyLen 派生结果必须互不相同（16 字节 vs 32 字节 不能完全相等）
	if bytes.Equal(key16, key32) {
		t.Error("16-byte and 32-byte keys are identical, but should differ in last 16 bytes")
	}

	// ⑥ GenerateKey_v4 命名别名必须与 GenerateKey 行为完全一致
	key16V4 := GenerateKey_v4(password, salt, KeySize_v4_128)
	key32V4 := GenerateKey_v4(password, salt, KeySize_v4_256)
	if !bytes.Equal(key16, key16V4) {
		t.Errorf("GenerateKey_v4 16-byte mode differs from GenerateKey: %x vs %x", key16, key16V4)
	}
	if !bytes.Equal(key32, key32V4) {
		t.Errorf("GenerateKey_v4 32-byte mode differs from GenerateKey: %x vs %x", key32, key32V4)
	}

	// ⑦ fallback 行为：keyLen <= 0 必须 fallback 到 16 字节（v4 默认）
	keyFallback := GenerateKey_v4(password, salt, 0)
	if !bytes.Equal(keyFallback, key16) {
		t.Errorf("GenerateKey_v4 fallback mismatch: got %x, want %x", keyFallback, key16)
	}
	keyFallbackNeg := GenerateKey_v4(password, salt, -1)
	if !bytes.Equal(keyFallbackNeg, key16) {
		t.Errorf("GenerateKey_v4 negative keyLen fallback mismatch: got %x, want %x", keyFallbackNeg, key16)
	}

	// ⑧ 16 字节 key 走通现有 AES-CTR 流接口（端到端验证）
	plaintext := "AES-128-CTR end-to-end with variable-length KDF"
	src := strings.NewReader(plaintext)
	iv, err := GenerateIV_v2(IVSize_v2)
	if err != nil {
		t.Fatalf("GenerateIV_v2 failed: %v", err)
	}
	var ciphertext bytes.Buffer
	if err := EncryptStream_v2(src, &ciphertext, key16, iv); err != nil {
		t.Fatalf("EncryptStream_v2 with 16-byte key failed: %v", err)
	}
	dec, err := DecryptReaderToBytes_v2(&ciphertext, key16, iv)
	if err != nil {
		t.Fatalf("DecryptReaderToBytes_v2 with 16-byte key failed: %v", err)
	}
	if string(dec) != plaintext {
		t.Errorf("AES-128-CTR roundtrip mismatch:\n  want=%q\n  got =%q", plaintext, string(dec))
	}

	// ⑨ 32 字节 key 走通 AES-CTR（回归保护）
	src32 := strings.NewReader(plaintext)
	var ciphertext32 bytes.Buffer
	if err := EncryptStream_v2(src32, &ciphertext32, key32, iv); err != nil {
		t.Fatalf("EncryptStream_v2 with 32-byte key failed: %v", err)
	}
	dec32, err := DecryptReaderToBytes_v2(&ciphertext32, key32, iv)
	if err != nil {
		t.Fatalf("DecryptReaderToBytes_v2 with 32-byte key failed: %v", err)
	}
	if string(dec32) != plaintext {
		t.Errorf("AES-256-CTR roundtrip mismatch:\n  want=%q\n  got =%q", plaintext, string(dec32))
	}
}

// TestKeySizeForCipherMode_v4 验证 CipherMode → KeySize 映射正确。
func TestKeySizeForCipherMode_v4(t *testing.T) {
	if got := KeySizeForCipherMode_v4(CipherModeAES128CTR); got != KeySize_v4_128 {
		t.Errorf("CipherModeAES128CTR → %d, want %d", got, KeySize_v4_128)
	}
	if got := KeySizeForCipherMode_v4(CipherModeAES256CTR); got != KeySize_v4_256 {
		t.Errorf("CipherModeAES256CTR → %d, want %d", got, KeySize_v4_256)
	}
	// 未知枚举值 fallback 到 AES-128
	if got := KeySizeForCipherMode_v4(CipherMode_v4(99)); got != KeySize_v4_128 {
		t.Errorf("unknown CipherMode(99) → %d, want fallback %d", got, KeySize_v4_128)
	}
}

// TestEncryptDecrypt_v2_AES128CTR 16 字节 key 走通 EncryptStream_v2 端到端
// （验证 aes.NewCipher(16B key) 路径可用，且与现有 32 字节路径行为一致）。
func TestEncryptDecrypt_v2_AES128CTR(t *testing.T) {
	password := "aes-128-test-pw"
	salt, err := GenerateSalt_v2(SaltSize_v2)
	if err != nil {
		t.Fatalf("GenerateSalt_v2: %v", err)
	}
	key16 := GenerateKey_v4(password, salt, KeySize_v4_128)
	iv, err := GenerateIV_v2(IVSize_v2)
	if err != nil {
		t.Fatalf("GenerateIV_v2: %v", err)
	}

	// 验证 key 长度合法
	if len(key16) != aes.BlockSize {
		t.Fatalf("AES-128 key length = %d, want %d", len(key16), aes.BlockSize)
	}

	// 端到端 round-trip
	original := strings.NewReader("The quick brown fox jumps over the lazy dog. AES-128-CTR.")
	var enc bytes.Buffer
	if err := EncryptStream_v2(original, &enc, key16, iv); err != nil {
		t.Fatalf("EncryptStream_v2(16B key): %v", err)
	}
	dec, err := DecryptReaderToBytes_v2(&enc, key16, iv)
	if err != nil {
		t.Fatalf("DecryptReaderToBytes_v2(16B key): %v", err)
	}
	if !bytes.Equal(dec, []byte("The quick brown fox jumps over the lazy dog. AES-128-CTR.")) {
		t.Errorf("AES-128-CTR roundtrip mismatch: got %q", dec)
	}
}
