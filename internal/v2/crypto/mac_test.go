// internal/v2/crypto/mac_test.go
package crypto

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"testing"
)

// TestHMACSHA1_80_KnownVector 使用 RFC 2202 已知向量验证 HMAC-SHA1 的计算正确性。
//
// RFC 2202 test case 1:
//   key  = 0x0b repeated 20 times
//   data = "Hi There"
//   full SHA1 = b617318655057264e28bc0b6fb378c8ef146be00
//   truncated to 10 bytes = b617318655057264e28b
//
// 这是 HMAC-SHA1 实现的标准 "Hello World" 向量。
// 验证步骤：
//   1. 全 20 字节 SHA-1 输出与 RFC 2202 完全一致（用标准库作为 ground truth）
//   2. 前 10 字节截断与预期完全一致
//   3. 返回类型是 [10]byte 而非 []byte
func TestHMACSHA1_80_KnownVector(t *testing.T) {
	key := bytes.Repeat([]byte{0x0b}, 20)
	data := []byte("Hi There")

	// 完整 20 字节 HMAC-SHA1（参考值，来自 RFC 2202）
	expectedFullHex := "b617318655057264e28bc0b6fb378c8ef146be00"
	expectedFull, err := hex.DecodeString(expectedFullHex)
	if err != nil {
		t.Fatalf("hex.DecodeString failed: %v", err)
	}

	// 截断到 10 字节（v4 规范要求）
	expectedTruncatedHex := "b617318655057264e28b"
	expectedTruncated, err := hex.DecodeString(expectedTruncatedHex)
	if err != nil {
		t.Fatalf("hex.DecodeString failed: %v", err)
	}

	got := HMACSHA1_80(key, data)

	// 校验长度严格为 10 字节
	if len(got) != HMACSize_v4 {
		t.Fatalf("HMACSHA1_80 returned %d bytes, want %d", len(got), HMACSize_v4)
	}
	if HMACSize_v4 != 10 {
		t.Fatalf("HMACSize_v4 = %d, must be 10 (WinZip AE-2 spec)", HMACSize_v4)
	}

	// 校验截断后的 10 字节
	if !bytes.Equal(got[:], expectedTruncated) {
		t.Errorf("HMACSHA1_80 truncated = %s, want %s",
			hex.EncodeToString(got[:]), expectedTruncatedHex)
	}

	// 进一步验证：截断的前 10 字节必须等于完整 SHA-1 的前 10 字节
	if !bytes.Equal(got[:], expectedFull[:HMACSize_v4]) {
		t.Errorf("HMACSHA1_80 truncated != SHA-1[:10]\n"+
			"  HMAC-80:  %s\n"+
			"  SHA-1[:10]: %s",
			hex.EncodeToString(got[:]),
			hex.EncodeToString(expectedFull[:HMACSize_v4]))
	}

	// 验证完整 SHA-1 计算正确（用 crypto/hmac 标准库作为 ground truth）
	mac := hmac.New(sha1.New, key)
	mac.Write(data)
	full := mac.Sum(nil)
	if !bytes.Equal(full, expectedFull) {
		t.Errorf("Full HMAC-SHA1 = %s, want %s", hex.EncodeToString(full), expectedFullHex)
	}
}

// TestVerifyHMACSHA1_80_ConstantTime 验证 Verify 函数的三大基本场景：
//   1. 正确 MAC + 正确 key → true
//   2. 错误 MAC → false
//   3. expected 长度 != 10 → false（不进入比较）
func TestVerifyHMACSHA1_80_ConstantTime(t *testing.T) {
	key := []byte("test-key-12345678")
	message := []byte("Hello, HMAC-SHA1-80 world!")

	// 1. 正确 MAC
	mac := HMACSHA1_80(key, message)
	macBytes := mac[:]
	if !VerifyHMACSHA1_80(macBytes, message, key) {
		t.Error("VerifyHMACSHA1_80 returned false for correct MAC, want true")
	}

	// 2. 错误 MAC：翻转最后一个 bit
	corrupted := make([]byte, len(macBytes))
	copy(corrupted, macBytes)
	corrupted[len(corrupted)-1] ^= 0x01 // 翻转最后 1 bit
	if VerifyHMACSHA1_80(corrupted, message, key) {
		t.Error("VerifyHMACSHA1_80 returned true for 1-bit-flipped MAC, want false")
	}

	// 3. 错误 MAC：完全不同的字节
	allZero := make([]byte, HMACSize_v4)
	if VerifyHMACSHA1_80(allZero, message, key) {
		t.Error("VerifyHMACSHA1_80 returned true for all-zero MAC, want false")
	}

	// 4. expected 长度 != 10 → false（不进入比较）
	shortExpected := macBytes[:9]
	if VerifyHMACSHA1_80(shortExpected, message, key) {
		t.Error("VerifyHMACSHA1_80 returned true for 9-byte expected, want false (length check)")
	}
	longExpected := append(macBytes, 0x00)
	if VerifyHMACSHA1_80(longExpected, message, key) {
		t.Error("VerifyHMACSHA1_80 returned true for 11-byte expected, want false (length check)")
	}
	emptyExpected := []byte{}
	if VerifyHMACSHA1_80(emptyExpected, message, key) {
		t.Error("VerifyHMACSHA1_80 returned true for empty expected, want false (length check)")
	}
}

// TestVerifyHMACSHA1_80_ConstantTime_AntiTiming 极端边界场景：
//   - 空 message（HMAC 仍然可计算）
//   - 空 key（HMAC 标准允许，规范要求补 0）
//   - 极长 message（验证大块数据不破坏）
//   - 多次校验稳定性（无状态污染）
//
// 由于 crypto/subtle.ConstantTimeCompare 已保证常量时间，
// 本测试不直接测量时间（CI 环境不稳定），而是通过"多次调用结果一致"
// 间接证明无状态污染，并验证边界场景返回值符合预期。
func TestVerifyHMACSHA1_80_ConstantTime_AntiTiming(t *testing.T) {
	// 场景 1：空 message
	t.Run("empty_message", func(t *testing.T) {
		key := []byte("some-key")
		mac := HMACSHA1_80(key, []byte{})
		if !VerifyHMACSHA1_80(mac[:], []byte{}, key) {
			t.Error("Verify failed for empty message with correct MAC")
		}
		// 修改 message 任意 bit 应导致失败
		msgWithBit := []byte{0x00}
		if VerifyHMACSHA1_80(mac[:], msgWithBit, key) {
			t.Error("Verify returned true for empty message MAC verified against non-empty message")
		}
	})

	// 场景 2：空 key（HMAC 标准允许，且规范要求补 0 到块大小）
	// 因此空 key 与 "\x00\x00...\x00"（20 字节全零）产生的 MAC 应当一致。
	// 这是 RFC 2104 的明确规定，不是 bug。
	t.Run("empty_key", func(t *testing.T) {
		message := []byte("data")
		macEmpty := HMACSHA1_80([]byte{}, message)
		macZero := HMACSHA1_80(make([]byte, sha1.BlockSize), message) // 20 字节全零 = HMAC 内部补 0 结果
		if !bytes.Equal(macEmpty[:], macZero[:]) {
			t.Error("HMAC-SHA1 with empty key should equal HMAC-SHA1 with zero-padded key (RFC 2104)")
		}

		// 正确 MAC 必须校验通过
		if !VerifyHMACSHA1_80(macEmpty[:], message, []byte{}) {
			t.Error("Verify failed for empty key with correct MAC")
		}
		// 用 zero-padded key 校验空 key 产生的 MAC 也应通过
		if !VerifyHMACSHA1_80(macEmpty[:], message, make([]byte, sha1.BlockSize)) {
			t.Error("Verify should accept zero-padded key for empty key MAC")
		}
		// 用真正不同的 key（长度 >= 块大小）应失败
		diffKey := bytes.Repeat([]byte{0xAA}, sha1.BlockSize)
		if VerifyHMACSHA1_80(macEmpty[:], message, diffKey) {
			t.Error("Verify returned true for empty key MAC verified against different (length=block) key")
		}
	})

	// 场景 3：极长 message（4 MB）
	t.Run("large_message", func(t *testing.T) {
		key := []byte("large-msg-key")
		largeMsg := bytes.Repeat([]byte("X"), 4*1024*1024)
		mac := HMACSHA1_80(key, largeMsg)
		if !VerifyHMACSHA1_80(mac[:], largeMsg, key) {
			t.Error("Verify failed for 4MB message with correct MAC")
		}
		// 修改 message 最后一字节应失败
		largeMsgMod := make([]byte, len(largeMsg))
		copy(largeMsgMod, largeMsg)
		largeMsgMod[len(largeMsgMod)-1] ^= 0x80
		if VerifyHMACSHA1_80(mac[:], largeMsgMod, key) {
			t.Error("Verify returned true for modified 4MB message")
		}
	})

	// 场景 4：多次校验稳定性（无状态污染）
	t.Run("no_state_pollution", func(t *testing.T) {
		key := []byte("state-test-key")
		msg := []byte("state-test-message")
		mac := HMACSHA1_80(key, msg)

		// 校验 1000 次，每次都应该返回 true
		for i := 0; i < 1000; i++ {
			if !VerifyHMACSHA1_80(mac[:], msg, key) {
				t.Fatalf("Verify returned false on iteration %d (state pollution?)", i)
			}
		}
	})
}

// TestHMACSHA1_80_DifferentKeys 验证不同 key 产生不同 MAC。
func TestHMACSHA1_80_DifferentKeys(t *testing.T) {
	msg := []byte("test message")
	key1 := []byte("key-1")
	key2 := []byte("key-2")

	mac1 := HMACSHA1_80(key1, msg)
	mac2 := HMACSHA1_80(key2, msg)

	if bytes.Equal(mac1[:], mac2[:]) {
		t.Error("Different keys produced identical MACs")
	}
}

// TestHMACSHA1_80_DifferentMessages 验证不同 message 产生不同 MAC。
func TestHMACSHA1_80_DifferentMessages(t *testing.T) {
	key := []byte("same-key")
	msg1 := []byte("message-1")
	msg2 := []byte("message-2")

	mac1 := HMACSHA1_80(key, msg1)
	mac2 := HMACSHA1_80(key, msg2)

	if bytes.Equal(mac1[:], mac2[:]) {
		t.Error("Different messages produced identical MACs")
	}
}

// TestHMACSHA1_80_ReturnTypeIsArray 验证返回类型严格为 [10]byte（栈分配）。
// 编译期已保证，但运行期再 sanity check 一次：转换 []byte 后长度必须为 10。
func TestHMACSHA1_80_ReturnTypeIsArray(t *testing.T) {
	got := HMACSHA1_80([]byte("k"), []byte("m"))
	if len(got) != 10 {
		t.Errorf("len(got) = %d, want 10", len(got))
	}
}
