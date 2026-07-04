package crypto

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// =========== Encrypt-then-MAC 兼容保留测试（更新签名） ===========

func TestEncryptDecryptSegmentRoundTrip(t *testing.T) {
	key := make([]byte, KeySize_v2)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	macKey := make([]byte, MACKeyLength)
	if _, err := rand.Read(macKey); err != nil {
		t.Fatalf("Failed to generate macKey: %v", err)
	}

	plainData := []byte("This is a secret segment payload for round-trip testing.")

	result, err := EncryptSegment(plainData, key, macKey, 0, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment failed: %v", err)
	}

	if result.SegmentID != 0 {
		t.Errorf("Expected SegmentID 0, got %d", result.SegmentID)
	}
	if len(result.Nonce) != IVSize_v2 {
		t.Errorf("Expected nonce length %d, got %d", IVSize_v2, len(result.Nonce))
	}
	if len(result.EncryptedData) != len(plainData) {
		t.Errorf("Expected encrypted data length %d, got %d", len(plainData), len(result.EncryptedData))
	}
	if result.DataCRC32 == 0 {
		t.Error("Expected non-zero CRC32")
	}

	// Encrypt-then-MAC：MAC 字段必须非零（密文任意翻转 1 bit MAC 必变）
	var zeroMAC [HMACSize_v4]byte
	if result.HMAC == zeroMAC {
		t.Error("Expected non-zero HMAC for randomly generated macKey/nonce/plaintext")
	}

	// 默认压缩模式 = none，所以 Compressed/SeekTable 必须为 0
	if result.Compressed {
		t.Error("Expected Compressed=false for default mode (none)")
	}
	if len(result.SeekTable) != 0 {
		t.Error("Expected empty SeekTable for default mode (none)")
	}

	decrypted, err := DecryptSegment(result.EncryptedData, result.Nonce, key, macKey, result.HMAC[:], CompressionModeNone, nil)
	if err != nil {
		t.Fatalf("DecryptSegment failed: %v", err)
	}

	if !bytes.Equal(decrypted, plainData) {
		t.Errorf("Decrypted data does not match original.\nExpected: %s\nGot: %s", plainData, decrypted)
	}
}

func TestDifferentSegmentsDifferentCiphertext(t *testing.T) {
	key := make([]byte, KeySize_v2)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	macKey := make([]byte, MACKeyLength)
	if _, err := rand.Read(macKey); err != nil {
		t.Fatalf("Failed to generate macKey: %v", err)
	}

	plainData := []byte("Same plaintext for both segments")

	result0, err := EncryptSegment(plainData, key, macKey, 0, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment 0 failed: %v", err)
	}

	result1, err := EncryptSegment(plainData, key, macKey, 1, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment 1 failed: %v", err)
	}

	if bytes.Equal(result0.EncryptedData, result1.EncryptedData) {
		t.Error("Different segments with same plaintext and key should produce different ciphertext due to independent nonces")
	}

	if bytes.Equal(result0.Nonce, result1.Nonce) {
		t.Error("Different segments should have different nonces")
	}

	// 不同 nonce 必然产生不同 MAC（MAC 输入含 nonce）
	if result0.HMAC == result1.HMAC {
		t.Error("Different nonces should produce different HMAC values")
	}
}

func TestSegmentIndependence(t *testing.T) {
	key := make([]byte, KeySize_v2)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	macKey := make([]byte, MACKeyLength)
	if _, err := rand.Read(macKey); err != nil {
		t.Fatalf("Failed to generate macKey: %v", err)
	}

	plainData0 := []byte("Segment zero payload data")
	plainData1 := []byte("Segment one payload data")

	result0, err := EncryptSegment(plainData0, key, macKey, 0, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment 0 failed: %v", err)
	}

	result1, err := EncryptSegment(plainData1, key, macKey, 1, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment 1 failed: %v", err)
	}

	decrypted0, err := DecryptSegment(result0.EncryptedData, result0.Nonce, key, macKey, result0.HMAC[:], CompressionModeNone, nil)
	if err != nil {
		t.Fatalf("DecryptSegment 0 failed: %v", err)
	}

	decrypted1, err := DecryptSegment(result1.EncryptedData, result1.Nonce, key, macKey, result1.HMAC[:], CompressionModeNone, nil)
	if err != nil {
		t.Fatalf("DecryptSegment 1 failed: %v", err)
	}

	if !bytes.Equal(decrypted0, plainData0) {
		t.Errorf("Segment 0 decryption failed.\nExpected: %s\nGot: %s", plainData0, decrypted0)
	}

	if !bytes.Equal(decrypted1, plainData1) {
		t.Errorf("Segment 1 decryption failed.\nExpected: %s\nGot: %s", plainData1, decrypted1)
	}

	decrypted0Again, err := DecryptSegment(result0.EncryptedData, result0.Nonce, key, macKey, result0.HMAC[:], CompressionModeNone, nil)
	if err != nil {
		t.Fatalf("DecryptSegment 0 after corruption of segment 1 failed: %v", err)
	}

	if !bytes.Equal(decrypted0Again, plainData0) {
		t.Error("Corrupting segment 1 data should not affect decryption of segment 0")
	}
}

func TestEncryptStreamToSegments(t *testing.T) {
	key := make([]byte, KeySize_v2)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	macKey := make([]byte, MACKeyLength)
	if _, err := rand.Read(macKey); err != nil {
		t.Fatalf("Failed to generate macKey: %v", err)
	}

	data := make([]byte, 5000)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("Failed to generate test data: %v", err)
	}

	segmentSize := int64(2048)
	results, err := EncryptStreamToSegments(bytes.NewReader(data), key, macKey, segmentSize, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptStreamToSegments failed: %v", err)
	}

	expectedSegments := 3
	if len(results) != expectedSegments {
		t.Fatalf("Expected %d segments, got %d", expectedSegments, len(results))
	}

	if results[0].SegmentID != 0 {
		t.Errorf("Segment 0 ID expected 0, got %d", results[0].SegmentID)
	}
	if results[1].SegmentID != 1 {
		t.Errorf("Segment 1 ID expected 1, got %d", results[1].SegmentID)
	}
	if results[2].SegmentID != 2 {
		t.Errorf("Segment 2 ID expected 2, got %d", results[2].SegmentID)
	}

	if len(results[0].EncryptedData) != 2048 {
		t.Errorf("Segment 0 data length expected 2048, got %d", len(results[0].EncryptedData))
	}
	if len(results[1].EncryptedData) != 2048 {
		t.Errorf("Segment 1 data length expected 2048, got %d", len(results[1].EncryptedData))
	}
	if len(results[2].EncryptedData) != 904 {
		t.Errorf("Segment 2 data length expected 904, got %d", len(results[2].EncryptedData))
	}

	var allDecrypted []byte
	for _, result := range results {
		decrypted, err := DecryptSegment(result.EncryptedData, result.Nonce, key, macKey, result.HMAC[:], CompressionModeNone, nil)
		if err != nil {
			t.Fatalf("DecryptSegment %d failed: %v", result.SegmentID, err)
		}
		allDecrypted = append(allDecrypted, decrypted...)
	}

	if !bytes.Equal(allDecrypted, data) {
		t.Errorf("Decrypted stream does not match original data")
	}
}

func TestSegmentIndependentNonces(t *testing.T) {
	key := make([]byte, KeySize_v2)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	macKey := make([]byte, MACKeyLength)
	if _, err := rand.Read(macKey); err != nil {
		t.Fatalf("Failed to generate macKey: %v", err)
	}

	data := make([]byte, 3000)
	if _, err := rand.Read(data); err != nil {
		t.Fatalf("Failed to generate test data: %v", err)
	}

	segmentSize := int64(1000)
	results, err := EncryptStreamToSegments(bytes.NewReader(data), key, macKey, segmentSize, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptStreamToSegments failed: %v", err)
	}

	for i, result := range results {
		if len(result.Nonce) != IVSize_v2 {
			t.Errorf("Segment %d nonce length expected %d, got %d", i, IVSize_v2, len(result.Nonce))
		}
	}

	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if bytes.Equal(results[i].Nonce, results[j].Nonce) {
				t.Errorf("Segment %d and %d have identical nonces, expected independent nonces", i, j)
			}
		}
	}
}

// =========== Task 4: Encrypt-then-MAC 新增测试 ===========

// TestEncryptDecryptSegment_WithMAC 验证 Encrypt-then-MAC 模式端到端 round-trip：
//   1. 加密 "Hello, World!" → 解密 → 字节完全一致
//   2. 验证 trailer 包含 10 字节 HMAC
func TestEncryptDecryptSegment_WithMAC(t *testing.T) {
	key := make([]byte, KeySize_v2)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}
	macKey := make([]byte, MACKeyLength)
	if _, err := rand.Read(macKey); err != nil {
		t.Fatalf("Failed to generate macKey: %v", err)
	}

	plaintext := []byte("Hello, World!")

	result, err := EncryptSegment(plaintext, key, macKey, 0, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment failed: %v", err)
	}

	// 1.1 HMAC 字段长度恒为 10 字节
	if len(result.HMAC) != HMACSize_v4 {
		t.Errorf("HMAC length expected %d, got %d", HMACSize_v4, len(result.HMAC))
	}

	// 1.2 验证 HMAC 是 HMACSHA1_80(macKey, nonce || ciphertext)[:10]
	expectedMAC := HMACSHA1_80(macKey, append(append([]byte{}, result.Nonce...), result.EncryptedData...))
	if result.HMAC != expectedMAC {
		t.Error("EncryptSegment.HMAC does not match independent HMACSHA1_80 computation")
	}

	// 1.3 round-trip：解密后字节完全一致
	decrypted, err := DecryptSegment(result.EncryptedData, result.Nonce, key, macKey, result.HMAC[:], CompressionModeNone, nil)
	if err != nil {
		t.Fatalf("DecryptSegment failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Round-trip mismatch.\nExpected: %q\nGot: %q", plaintext, decrypted)
	}

	// 1.4 磁盘布局组装验证：nonce(16) + ciphertext + mac(10)
	onDisk := make([]byte, 0, len(result.Nonce)+len(result.EncryptedData)+HMACSize_v4)
	onDisk = append(onDisk, result.Nonce...)
	onDisk = append(onDisk, result.EncryptedData...)
	onDisk = append(onDisk, result.HMAC[:]...)
	if len(onDisk) != IVSize_v2+len(plaintext)+HMACSize_v4 {
		t.Errorf("On-disk size = %d, expected %d", len(onDisk), IVSize_v2+len(plaintext)+HMACSize_v4)
	}
	// 最后一字节必须是 HMAC 切片第一字节（验证 trailer 包含 10 字节 MAC）
	if onDisk[len(onDisk)-HMACSize_v4] != result.HMAC[0] {
		t.Error("Trailer does not contain HMAC bytes")
	}
}

// TestDecryptSegment_TamperedCiphertext_ReturnsErrMACMismatch 验证密文翻转 1 bit
// 必须返回 ErrMACMismatch（**不能**返回 CTR 解密后的"垃圾"明文）。
func TestDecryptSegment_TamperedCiphertext_ReturnsErrMACMismatch(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	plaintext := []byte("This is a payload that must not be revealed by tampering")
	result, err := EncryptSegment(plaintext, key, macKey, 0, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment failed: %v", err)
	}

	// 翻转密文第 1 字节的第 0 位
	tampered := make([]byte, len(result.EncryptedData))
	copy(tampered, result.EncryptedData)
	tampered[0] ^= 0x01

	// 必须返回 ErrMACMismatch，**不能**返回其他错误
	_, decErr := DecryptSegment(tampered, result.Nonce, key, macKey, result.HMAC[:], CompressionModeNone, nil)
	if decErr == nil {
		t.Fatal("DecryptSegment should fail on tampered ciphertext, got nil error")
	}
	if !errors.Is(decErr, ErrMACMismatch) {
		t.Errorf("DecryptSegment must return ErrMACMismatch on tampered ciphertext, got: %v", decErr)
	}
	// 关键断言：绝不能是 "invalid plaintext" 之类的副作用错误
	if errors.Is(decErr, ErrInvalidIVLength_v2) {
		t.Error("DecryptSegment returned ErrInvalidIVLength_v2 on tampered ciphertext - MAC check should fail first")
	}
}

// TestDecryptSegment_TamperedNonce_ReturnsErrMACMismatch 验证翻转 nonce 任意 1 bit
// 必须返回 ErrMACMismatch（因为 nonce 也参与 HMAC 计算）。
func TestDecryptSegment_TamperedNonce_ReturnsErrMACMismatch(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	plaintext := []byte("Nonce-tampering defense test payload")
	result, err := EncryptSegment(plaintext, key, macKey, 0, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment failed: %v", err)
	}

	// 翻转 nonce 第 5 字节
	tamperedNonce := make([]byte, IVSize_v2)
	copy(tamperedNonce, result.Nonce)
	tamperedNonce[5] ^= 0x80

	_, decErr := DecryptSegment(result.EncryptedData, tamperedNonce, key, macKey, result.HMAC[:], CompressionModeNone, nil)
	if decErr == nil {
		t.Fatal("DecryptSegment should fail on tampered nonce, got nil error")
	}
	if !errors.Is(decErr, ErrMACMismatch) {
		t.Errorf("DecryptSegment must return ErrMACMismatch on tampered nonce (nonce is part of HMAC input), got: %v", decErr)
	}
}

// TestDecryptSegment_TamperedMAC_ReturnsErrMACMismatch 验证翻转 MAC 任意 1 bit
// 必须返回 ErrMACMismatch。
func TestDecryptSegment_TamperedMAC_ReturnsErrMACMismatch(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	plaintext := []byte("MAC-tampering defense test payload")
	result, err := EncryptSegment(plaintext, key, macKey, 0, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment failed: %v", err)
	}

	tamperedMAC := make([]byte, HMACSize_v4)
	copy(tamperedMAC, result.HMAC[:])
	tamperedMAC[0] ^= 0xFF

	_, decErr := DecryptSegment(result.EncryptedData, result.Nonce, key, macKey, tamperedMAC, CompressionModeNone, nil)
	if decErr == nil {
		t.Fatal("DecryptSegment should fail on tampered MAC, got nil error")
	}
	if !errors.Is(decErr, ErrMACMismatch) {
		t.Errorf("DecryptSegment must return ErrMACMismatch on tampered MAC, got: %v", decErr)
	}
}

// TestDecryptSegment_WrongMACKey_ReturnsErrMACMismatch 验证用错误 macKey 解密
// 必须返回 ErrMACMismatch（不能解出任何明文）。
func TestDecryptSegment_WrongMACKey_ReturnsErrMACMismatch(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	wrongMacKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)
	rand.Read(wrongMacKey)

	plaintext := []byte("This should never be revealed to a wrong-macKey attacker")
	result, err := EncryptSegment(plaintext, key, macKey, 0, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment failed: %v", err)
	}

	// 用错误 macKey 解密
	decrypted, decErr := DecryptSegment(result.EncryptedData, result.Nonce, key, wrongMacKey, result.HMAC[:], CompressionModeNone, nil)
	if decErr == nil {
		t.Fatal("DecryptSegment should fail with wrong macKey, got nil error")
	}
	if !errors.Is(decErr, ErrMACMismatch) {
		t.Errorf("DecryptSegment must return ErrMACMismatch with wrong macKey, got: %v", decErr)
	}
	// 关键：绝不能泄露 plaintext（或部分 plaintext）
	if bytes.Equal(decrypted, plaintext) {
		t.Error("DecryptSegment with wrong macKey must NOT return correct plaintext")
	}
	if len(decrypted) > 0 {
		t.Errorf("DecryptSegment with wrong macKey must return nil/empty plaintext, got %d bytes", len(decrypted))
	}
}

// TestDecryptSegment_MissingMAC_ReturnsError 验证数据缺少 trailing 10 字节 MAC
// 必须返回明确错误（不能 panic，不能返回部分明文）。
func TestDecryptSegment_MissingMAC_ReturnsError(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	plaintext := []byte("payload for missing-MAC test")
	result, err := EncryptSegment(plaintext, key, macKey, 0, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment failed: %v", err)
	}

	// 场景 1：传 nil MAC
	_, decErr := DecryptSegment(result.EncryptedData, result.Nonce, key, macKey, nil, CompressionModeNone, nil)
	if decErr == nil {
		t.Error("DecryptSegment with nil MAC should return error")
	}
	if !errors.Is(decErr, ErrSegmentTooShort) {
		t.Errorf("DecryptSegment with nil MAC must return ErrSegmentTooShort, got: %v", decErr)
	}

	// 场景 2：传 9 字节 MAC（差 1 字节）
	shortMAC := make([]byte, HMACSize_v4-1)
	copy(shortMAC, result.HMAC[:])
	_, decErr = DecryptSegment(result.EncryptedData, result.Nonce, key, macKey, shortMAC, CompressionModeNone, nil)
	if decErr == nil {
		t.Error("DecryptSegment with short MAC should return error")
	}
	if !errors.Is(decErr, ErrSegmentTooShort) {
		t.Errorf("DecryptSegment with short MAC must return ErrSegmentTooShort, got: %v", decErr)
	}

	// 场景 3：传 11 字节 MAC（超长 1 字节）—— 长度 ≠ HMACSize_v4，语义上即非合法 MAC
	longMAC := make([]byte, HMACSize_v4+1)
	copy(longMAC, result.HMAC[:])
	longMAC[HMACSize_v4] = 0x00
	_, decErr = DecryptSegment(result.EncryptedData, result.Nonce, key, macKey, longMAC, CompressionModeNone, nil)
	if decErr == nil {
		t.Error("DecryptSegment with over-long MAC should return error")
	}
	// 非 10 字节 MAC 一律视作"长度非法"，返回 ErrSegmentTooShort（不进 HMAC verify）
	if !errors.Is(decErr, ErrSegmentTooShort) {
		t.Errorf("DecryptSegment with over-long MAC must return ErrSegmentTooShort, got: %v", decErr)
	}

	// 场景 4：空密文 + 空 MAC
	_, decErr = DecryptSegment([]byte{}, result.Nonce, key, macKey, []byte{}, CompressionModeNone, nil)
	if decErr == nil {
		t.Error("DecryptSegment with empty ciphertext+MAC should return error")
	}
	if !errors.Is(decErr, ErrSegmentTooShort) {
		t.Errorf("DecryptSegment with empty MAC must return ErrSegmentTooShort, got: %v", decErr)
	}
}

// TestDecryptSegment_InvalidNonce_ReturnsErrInvalidIVLength_v2 验证 nonce 长度非法
// 必须返回 ErrInvalidIVLength_v2（先于 MAC 校验）。
func TestDecryptSegment_InvalidNonce_ReturnsErrInvalidIVLength_v2(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	// 错误长度的 nonce（不是 16 字节）
	wrongNonce := make([]byte, 8)

	// 任意 10 字节 MAC
	mac := make([]byte, HMACSize_v4)
	rand.Read(mac)

	_, decErr := DecryptSegment([]byte("some data"), wrongNonce, key, macKey, mac, CompressionModeNone, nil)
	if !errors.Is(decErr, ErrInvalidIVLength_v2) {
		t.Errorf("DecryptSegment with wrong nonce length must return ErrInvalidIVLength_v2, got: %v", decErr)
	}
}

// TestEncryptDecryptSegment_LargeData_WithMAC 验证 1MB 数据的端到端 round-trip。
func TestEncryptDecryptSegment_LargeData_WithMAC(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	// 1MB 随机明文
	plaintext := make([]byte, 1<<20)
	if _, err := rand.Read(plaintext); err != nil {
		t.Fatalf("Failed to generate 1MB plaintext: %v", err)
	}

	result, err := EncryptSegment(plaintext, key, macKey, 0, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment 1MB failed: %v", err)
	}

	// 1.1 密文长度 = plaintext 长度（不含 MAC）
	if len(result.EncryptedData) != len(plaintext) {
		t.Errorf("EncryptedData length = %d, want %d", len(result.EncryptedData), len(plaintext))
	}

	// 1.2 round-trip
	decrypted, err := DecryptSegment(result.EncryptedData, result.Nonce, key, macKey, result.HMAC[:], CompressionModeNone, nil)
	if err != nil {
		t.Fatalf("DecryptSegment 1MB failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("1MB round-trip mismatch")
	}

	// 1.3 翻转密文中间位置 1 bit
	tampered := make([]byte, len(result.EncryptedData))
	copy(tampered, result.EncryptedData)
	tampered[1<<19] ^= 0x80
	_, decErr := DecryptSegment(tampered, result.Nonce, key, macKey, result.HMAC[:], CompressionModeNone, nil)
	if !errors.Is(decErr, ErrMACMismatch) {
		t.Errorf("1MB tampered ciphertext must return ErrMACMismatch, got: %v", decErr)
	}
}

// TestEncryptSegment_NilMACKey_StillProducesMAC 验证 macKey=nil 时仍能写出非零 MAC
// （hmac.New 接受 nil key，但语义上不安全，调用方需保证 macKey 已正确派生）。
//
// 这是"防呆"测试：保证加密端不会因为 macKey=nil 而 panic，MAC 字段仍能产出稳定值。
func TestEncryptSegment_NilMACKey_StillProducesMAC(t *testing.T) {
	key := make([]byte, KeySize_v2)
	rand.Read(key)

	plaintext := []byte("nil macKey test")
	result, err := EncryptSegment(plaintext, key, nil, 0, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment with nil macKey should not fail: %v", err)
	}

	// 重新计算 MAC（用 nil key）应得到相同结果
	expected := HMACSHA1_80(nil, append(append([]byte{}, result.Nonce...), result.EncryptedData...))
	if result.HMAC != expected {
		t.Error("EncryptSegment with nil macKey: HMAC should still be deterministic")
	}
}

// =========== Task 9: Segment 集成 zstd 压缩 测试 ===========

// errReader 是一个总在首次 Read 返回 (0, err) 的 io.Reader 实现，
// 用于模拟 zstd 压缩失败（CompressZstdSeekable 内部 read 阶段会拿到这个错误）。
type errReader struct {
	err error
}

func (r *errReader) Read(p []byte) (int, error) {
	return 0, r.err
}

// TestEncryptDecryptSegment_ZstdCompressed 验证 10KB 文本数据 + compressionMode="zstd"
// 的端到端 round-trip：压缩 → 加密 → 解密 → 解压 → 字节完全一致。
//
// 重点断言：
//   - result.Compressed == true
//   - result.SeekTable 非空（seekable 帧至少 1 帧）
//   - result.ModeFlags 含 ModeFlagCompressionZstd 位
//   - result.ModeFlags 也含 ModeFlagEncrypted 位（默认加密）
//   - 10KB 高度可压缩文本，密文长度应 < 10KB
func TestEncryptDecryptSegment_ZstdCompressed(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	// 10KB 高度可压缩的重复文本（zstd 压缩比应非常显著）
	plaintext := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 230)
	if len(plaintext) < MinimumCompressionSize {
		t.Fatalf("test setup: plaintext must be >= %d bytes, got %d", MinimumCompressionSize, len(plaintext))
	}

	result, err := EncryptSegment(plaintext, key, macKey, 0, CompressionModeZstd)
	if err != nil {
		t.Fatalf("EncryptSegment(zstd) failed: %v", err)
	}

	// 1.1 标记位正确
	if !result.Compressed {
		t.Error("expected Compressed=true for compressionMode=zstd on 10KB input")
	}
	if len(result.SeekTable) == 0 {
		t.Error("expected non-empty SeekTable after zstd compression")
	}
	if result.ModeFlags&types.ModeFlagCompressionZstd == 0 {
		t.Errorf("expected ModeFlags & ModeFlagCompressionZstd != 0, got %d (0x%04x)", result.ModeFlags, result.ModeFlags)
	}
	if result.ModeFlags&types.ModeFlagEncrypted == 0 {
		t.Errorf("expected ModeFlags & ModeFlagEncrypted != 0, got %d (0x%04x)", result.ModeFlags, result.ModeFlags)
	}

	// 1.2 压缩后密文应明显小于明文（重复文本压缩比 1-5%）
	if len(result.EncryptedData) >= len(plaintext) {
		t.Errorf("expected compressed-then-encrypted bytes (%d) < plaintext (%d) — zstd did not compress repetitive input",
			len(result.EncryptedData), len(plaintext))
	}

	// 1.3 round-trip：传 seekTable 给 DecryptSegment → 期望解压出原 plaintext
	decrypted, err := DecryptSegment(result.EncryptedData, result.Nonce, key, macKey, result.HMAC[:], CompressionModeZstd, result.SeekTable)
	if err != nil {
		t.Fatalf("DecryptSegment(zstd) failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("zstd round-trip mismatch:\n want_len=%d got_len=%d", len(plaintext), len(decrypted))
	}
}

// TestEncryptDecryptSegment_NoCompression_DefaultMode 验证 compressionMode="" 或 "none"
// 时不压缩、SeekTable 为空、ModeFlags 不含 CompressionZstd 位。
func TestEncryptDecryptSegment_NoCompression_DefaultMode(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	// 用 10KB 数据（即使很大也走 none 路径，不压缩）
	plaintext := bytes.Repeat([]byte("ABCDEFGH"), 1280) // 10240 bytes

	// 子场景 A：空字符串
	result, err := EncryptSegment(plaintext, key, macKey, 0, "")
	if err != nil {
		t.Fatalf("EncryptSegment(\"\") failed: %v", err)
	}
	if result.Compressed {
		t.Error("expected Compressed=false for empty compressionMode")
	}
	if len(result.SeekTable) != 0 {
		t.Error("expected empty SeekTable for empty compressionMode")
	}
	if result.ModeFlags&types.ModeFlagCompressionZstd != 0 {
		t.Errorf("expected ModeFlags & ModeFlagCompressionZstd == 0, got %d", result.ModeFlags)
	}

	// 子场景 B：显式 "none"
	result, err = EncryptSegment(plaintext, key, macKey, 1, CompressionModeNone)
	if err != nil {
		t.Fatalf("EncryptSegment(none) failed: %v", err)
	}
	if result.Compressed {
		t.Error("expected Compressed=false for CompressionModeNone")
	}
	if len(result.SeekTable) != 0 {
		t.Error("expected empty SeekTable for CompressionModeNone")
	}

	// round-trip：mode="none" 时不传 seekTable 也能解
	decrypted, err := DecryptSegment(result.EncryptedData, result.Nonce, key, macKey, result.HMAC[:], CompressionModeNone, nil)
	if err != nil {
		t.Fatalf("DecryptSegment(none) failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Error("none-mode round-trip mismatch")
	}

	// 子场景 C：未知 mode 字面量按 "none" 处理
	result, err = EncryptSegment(plaintext, key, macKey, 2, "brotli")
	if err != nil {
		t.Fatalf("EncryptSegment(unknown mode) failed: %v", err)
	}
	if result.Compressed {
		t.Error("expected Compressed=false for unknown compression mode (treated as none)")
	}
}

// TestEncryptDecryptSegment_SmallData_SkipsCompression 验证 < 1KB 阈值的数据
// 即使传 compressionMode="zstd" 也不会被压缩（自动跳过）。
func TestEncryptDecryptSegment_SmallData_SkipsCompression(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	// 100 字节 < MinimumCompressionSize(1024) → 不压缩
	plaintext := []byte("This is a small payload under 1KB. The quick brown fox jumps over the lazy dog. Hello, World!")

	if len(plaintext) >= MinimumCompressionSize {
		t.Fatalf("test setup: plaintext must be < %d bytes, got %d", MinimumCompressionSize, len(plaintext))
	}

	result, err := EncryptSegment(plaintext, key, macKey, 0, CompressionModeZstd)
	if err != nil {
		t.Fatalf("EncryptSegment(zstd on small data) failed: %v", err)
	}

	// 阈值不满足 → 不压缩
	if result.Compressed {
		t.Errorf("expected Compressed=false for data < %d bytes (skipped), got true", MinimumCompressionSize)
	}
	if len(result.SeekTable) != 0 {
		t.Error("expected empty SeekTable when compression is skipped due to size threshold")
	}
	if result.ModeFlags&types.ModeFlagCompressionZstd != 0 {
		t.Errorf("expected ModeFlags & ModeFlagCompressionZstd == 0 for skipped compression, got %d", result.ModeFlags)
	}

	// round-trip：解压缩路径不执行（即使传 "zstd" mode + nil seekTable 也应成功）
	decrypted, err := DecryptSegment(result.EncryptedData, result.Nonce, key, macKey, result.HMAC[:], CompressionModeZstd, nil)
	if err != nil {
		t.Fatalf("DecryptSegment(zstd, no seekTable) on small data should not error: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Error("small-data round-trip mismatch")
	}
}

// TestEncryptDecryptSegment_MixedModes 验证一个压缩一个不压缩的混合场景都能 round-trip。
//
// Segment A: 10KB + zstd 压缩
// Segment B: 500B + zstd 但 < 1KB 阈值 → 跳过压缩
// Segment C: 5KB + 显式 none
func TestEncryptDecryptSegment_MixedModes(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	segA := bytes.Repeat([]byte("A pattern that compresses well. "), 320)  // ~8.5KB
	segB := []byte("Short segment that should skip compression because it is under the 1KB threshold.")
	segC := bytes.Repeat([]byte("C pattern. "), 460)                       // ~5KB

	results := make([]*SegmentEncryptionResult, 3)
	modes := []string{CompressionModeZstd, CompressionModeZstd, CompressionModeNone}
	originals := [][]byte{segA, segB, segC}

	for i := range results {
		r, err := EncryptSegment(originals[i], key, macKey, uint32(i), modes[i])
		if err != nil {
			t.Fatalf("EncryptSegment %d failed: %v", i, err)
		}
		results[i] = r
	}

	// 验证压缩决策
	if !results[0].Compressed {
		t.Error("segment A (10KB + zstd) should be compressed")
	}
	if results[1].Compressed {
		t.Error("segment B (< 1KB + zstd) should NOT be compressed (skipped)")
	}
	if results[2].Compressed {
		t.Error("segment C (none mode) should NOT be compressed")
	}

	// round-trip 三个 segment
	for i, r := range results {
		// 用每段 EncryptSegment 时传的 mode 来解密
		decrypted, err := DecryptSegment(r.EncryptedData, r.Nonce, key, macKey, r.HMAC[:], modes[i], r.SeekTable)
		if err != nil {
			t.Fatalf("DecryptSegment %d failed: %v", i, err)
		}
		if !bytes.Equal(decrypted, originals[i]) {
			t.Errorf("segment %d round-trip mismatch: want_len=%d got_len=%d", i, len(originals[i]), len(decrypted))
		}
	}
}

// TestDecryptSegment_TamperedCiphertext_Compression 验证压缩 + 加密场景下，
// 翻转密文 1 bit 必须返回 ErrMACMismatch，**不能**走到解压路径（防 zstd 解压炸弹）。
func TestDecryptSegment_TamperedCiphertext_Compression(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	// 10KB 高度可压缩文本
	plaintext := bytes.Repeat([]byte("Decompression bomb defense test. "), 320)

	result, err := EncryptSegment(plaintext, key, macKey, 0, CompressionModeZstd)
	if err != nil {
		t.Fatalf("EncryptSegment(zstd) failed: %v", err)
	}
	if !result.Compressed {
		t.Fatal("setup: expected Compressed=true for 10KB + zstd")
	}

	// 翻转密文第 1 字节的第 0 位
	tampered := make([]byte, len(result.EncryptedData))
	copy(tampered, result.EncryptedData)
	tampered[0] ^= 0x01

	// 必须返回 ErrMACMismatch，**不能**走到 zstd 解压（即使传入合法 seekTable）
	decrypted, decErr := DecryptSegment(tampered, result.Nonce, key, macKey, result.HMAC[:], CompressionModeZstd, result.SeekTable)
	if decErr == nil {
		t.Fatal("DecryptSegment with tampered ciphertext should fail, got nil")
	}
	if !errors.Is(decErr, ErrMACMismatch) {
		t.Errorf("DecryptSegment must return ErrMACMismatch on tampered ciphertext, got: %v", decErr)
	}
	// 关键：不解压 → 不返回 ErrDecompressionFailed
	if errors.Is(decErr, ErrDecompressionFailed) {
		t.Error("DecryptSegment returned ErrDecompressionFailed - MAC check should fail first to prevent decompression-bomb DoS")
	}
	// 关键：不解密 → 不返回明文
	if len(decrypted) > 0 {
		t.Errorf("DecryptSegment with tampered ciphertext must return nil/empty plaintext, got %d bytes", len(decrypted))
	}
}

// TestEncryptSegment_CompressionFailure_GracefulDegrade 验证 zstd 压缩失败时
// EncryptSegment 必须**降级**为明文加密（不返回 error，不影响密文生成）。
//
// 实现：临时替换 compressZstdSeekableFunc 为总返回错误的桩函数，验证降级路径。
func TestEncryptSegment_CompressionFailure_GracefulDegrade(t *testing.T) {
	// 保存原始函数并在结束时恢复
	originalCompress := compressZstdSeekableFunc
	defer func() { compressZstdSeekableFunc = originalCompress }()

	// 替换为总返回错误的桩函数（模拟 zstd 库内部错误）
	compressZstdSeekableFunc = func(io.Reader) ([]byte, []byte, error) {
		return nil, nil, errors.New("simulated zstd encoder failure")
	}

	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	// 10KB 数据（>= 1KB 阈值才会触发压缩）
	plaintext := bytes.Repeat([]byte("Graceful degrade test payload. "), 340)
	if len(plaintext) < MinimumCompressionSize {
		t.Fatalf("test setup: plaintext must be >= %d bytes", MinimumCompressionSize)
	}

	// 压缩失败 → 降级为明文加密，不应返回 error
	result, err := EncryptSegment(plaintext, key, macKey, 0, CompressionModeZstd)
	if err != nil {
		t.Fatalf("EncryptSegment must degrade to plaintext encryption on compression failure, got error: %v", err)
	}

	// 降级特征：Compressed=false、SeekTable=nil、ModeFlags 不含 CompressionZstd
	if result.Compressed {
		t.Error("expected Compressed=false after graceful degradation")
	}
	if len(result.SeekTable) != 0 {
		t.Error("expected empty SeekTable after graceful degradation")
	}
	if result.ModeFlags&types.ModeFlagCompressionZstd != 0 {
		t.Errorf("expected ModeFlags & ModeFlagCompressionZstd == 0 after degradation, got %d", result.ModeFlags)
	}
	if result.ModeFlags&types.ModeFlagEncrypted == 0 {
		t.Error("expected ModeFlags still has ModeFlagEncrypted after degradation")
	}

	// 验证密文是 plaintext 的密文（降级路径）
	// 通过无 seekTable 的 round-trip 验证
	decrypted, err := DecryptSegment(result.EncryptedData, result.Nonce, key, macKey, result.HMAC[:], CompressionModeNone, nil)
	if err != nil {
		t.Fatalf("DecryptSegment after degradation failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Error("post-degradation round-trip mismatch — encryption did not fall back to plaintext path")
	}
}

// TestEncryptStreamToSegments_RespectsCompressionMode 验证 EncryptStreamToSegments
// 把 compressionMode 透传给每个 EncryptSegment 调用（< 1KB 段被跳过压缩）。
func TestEncryptStreamToSegments_RespectsCompressionMode(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	// 5KB 数据，segmentSize=1KB → 5 段全部 >= 1KB 全部会压缩
	data := bytes.Repeat([]byte("X"), 5*1024)

	results, err := EncryptStreamToSegments(bytes.NewReader(data), key, macKey, 1024, CompressionModeZstd)
	if err != nil {
		t.Fatalf("EncryptStreamToSegments failed: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 segments, got %d", len(results))
	}
	for i, r := range results {
		if !r.Compressed {
			t.Errorf("segment %d (1KB) should be compressed", i)
		}
	}
}

// TestEncryptStreamToSegments_SkipsCompressionOnSmallTail 验证流末尾的小段
//（< 1KB）即使 mode=zstd 也不会被压缩（与单次 EncryptSegment 行为一致）。
func TestEncryptStreamToSegments_SkipsCompressionOnSmallTail(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	// 2.5KB 数据，segmentSize=2KB → 2 段：[2KB][0.5KB]
	// 末段 0.5KB < 1KB 阈值 → 不压缩
	data := bytes.Repeat([]byte("Y"), 2*1024+512)

	results, err := EncryptStreamToSegments(bytes.NewReader(data), key, macKey, 2*1024, CompressionModeZstd)
	if err != nil {
		t.Fatalf("EncryptStreamToSegments failed: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(results))
	}
	if !results[0].Compressed {
		t.Error("first segment (2KB) should be compressed")
	}
	if results[1].Compressed {
		t.Error("last segment (0.5KB) should NOT be compressed (skipped due to size threshold)")
	}
}

// TestEncryptDecryptSegment_ZstdCompressed_CompressionRatio 验证 zstd 压缩确实生效：
// 加密后的字节数应明显小于明文。
func TestEncryptDecryptSegment_ZstdCompressed_CompressionRatio(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	// 100KB 高度重复文本
	plaintext := bytes.Repeat([]byte("ENCV zstd compression ratio test payload. "), 2500)
	if len(plaintext) < 100*1024 {
		t.Fatalf("test setup: expected >= 100KB, got %d", len(plaintext))
	}

	result, err := EncryptSegment(plaintext, key, macKey, 0, CompressionModeZstd)
	if err != nil {
		t.Fatalf("EncryptSegment failed: %v", err)
	}

	// 100KB 重复文本压缩比应 < 5%
	ratio := float64(len(result.EncryptedData)) / float64(len(plaintext))
	if ratio >= 0.05 {
		t.Errorf("expected compression ratio < 0.05 for 100KB repetitive text, got %.4f (orig=%d comp=%d)",
			ratio, len(plaintext), len(result.EncryptedData))
	}
}

// TestDecryptSegment_CompressionModeAndSeekTableConsistency 验证 compressionMode 和
// seekTable 的一致性约束：mode=zstd 但 seekTable=nil → 不解压（不报错）。
// 这与"反向"组合：mode=none 但 seekTable 非空 → 也不解压（防误用）。
func TestDecryptSegment_CompressionModeAndSeekTableConsistency(t *testing.T) {
	key := make([]byte, KeySize_v2)
	macKey := make([]byte, MACKeyLength)
	rand.Read(key)
	rand.Read(macKey)

	// 准备一个被 zstd 压缩过 + 加密的 segment
	plaintext := bytes.Repeat([]byte("Consistency test. "), 200)
	encRes, err := EncryptSegment(plaintext, key, macKey, 0, CompressionModeZstd)
	if err != nil {
		t.Fatalf("EncryptSegment failed: %v", err)
	}

	// 场景 A：compressionMode=zstd + seekTable=nil → 应直接返回密文（不解压）
	//          不报错，但返回的数据是 zstd 字节而非明文
	decA, err := DecryptSegment(encRes.EncryptedData, encRes.Nonce, key, macKey, encRes.HMAC[:], CompressionModeZstd, nil)
	if err != nil {
		t.Fatalf("DecryptSegment(zstd, nil seekTable) should not error: %v", err)
	}
	if bytes.Equal(decA, plaintext) {
		t.Error("decA should NOT be plaintext (seekTable missing → no decompression)")
	}

	// 场景 B：compressionMode=none + seekTable 非空 → 应直接返回密文（不读 seekTable）
	//          这是一种"误用"防御：避免无谓的解压尝试
	decB, err := DecryptSegment(encRes.EncryptedData, encRes.Nonce, key, macKey, encRes.HMAC[:], CompressionModeNone, encRes.SeekTable)
	if err != nil {
		t.Fatalf("DecryptSegment(none, valid seekTable) should not error: %v", err)
	}
	if bytes.Equal(decB, plaintext) {
		t.Error("decB should NOT be plaintext (mode=none → no decompression)")
	}
}
