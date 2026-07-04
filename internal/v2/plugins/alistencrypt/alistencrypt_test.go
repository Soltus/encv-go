package alistencrypt

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	"golang.org/x/crypto/pbkdf2"
)

func TestRegistryIsolation(t *testing.T) {
	t.Run("unregistered_rc4md5_returns_ErrExtensionRequired", func(t *testing.T) {
		_, err := Create("test", "rc4md5", 1024)
		if err == nil {
			t.Fatal("expected error for unregistered rc4md5")
		}
		if !errors.Is(err, ErrExtensionRequired) {
			t.Fatalf("expected ErrExtensionRequired, got: %v", err)
		}
	})

	t.Run("unregistered_chacha20_returns_ErrExtensionRequired", func(t *testing.T) {
		_, err := Create("test", "chacha20", 1024)
		if err == nil {
			t.Fatal("expected error for unregistered chacha20")
		}
		if !errors.Is(err, ErrExtensionRequired) {
			t.Fatalf("expected ErrExtensionRequired, got: %v", err)
		}
	})

	t.Run("aesctr_creates_successfully", func(t *testing.T) {
		c, err := Create("test", "aesctr", 1024)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c == nil {
			t.Fatal("expected non-nil cipher")
		}
		if c.Algorithm() != "AES-128-CTR" {
			t.Errorf("expected algorithm AES-128-CTR, got %q", c.Algorithm())
		}
		if c.BlockSize() != 16 {
			t.Errorf("expected block size 16, got %d", c.BlockSize())
		}
	})

	t.Run("same_params_produce_independent_instances", func(t *testing.T) {
		c1, _ := Create("password", "aesctr", 2048)
		c2, _ := Create("password", "aesctr", 2048)

		data := []byte("hello world")
		d1 := make([]byte, len(data))
		d2 := make([]byte, len(data))
		copy(d1, data)
		copy(d2, data)

		c1.Encrypt(d1)
		c2.Encrypt(d2)

		if !bytes.Equal(d1, d2) {
			t.Error("same params should produce identical encryption output")
		}
	})
}

func TestAesCtrKeyDerivation(t *testing.T) {
	t.Run("normal_password_key_and_iv_length", func(t *testing.T) {
		c, err := NewAesCtr("test123", 1024)
		if err != nil {
			t.Fatalf("NewAesCtr failed: %v", err)
		}
		a := c.(*aesCtr)

		if len(a.key) != 16 {
			t.Errorf("key length expected 16, got %d", len(a.key))
		}
		if len(a.iv) != 16 {
			t.Errorf("iv length expected 16, got %d", len(a.iv))
		}
		if len(a.sourceIv) != 16 {
			t.Errorf("sourceIv length expected 16, got %d", len(a.sourceIv))
		}
	})

	t.Run("normal_password_derivation_chain_deterministic", func(t *testing.T) {
		password := "test123"
		fileSize := int64(1024)

		pbkdfKey := pbkdf2.Key([]byte(password), []byte("AES-CTR"), 1000, 16, sha256.New)
		passwdOutward := hex.EncodeToString(pbkdfKey)

		passwdSalt := passwdOutward + "1024"
		expectedKey := md5.Sum([]byte(passwdSalt))

		expectedIV := md5.Sum([]byte("1024"))

		c, _ := NewAesCtr(password, fileSize)
		a := c.(*aesCtr)

		if !bytes.Equal(a.key, expectedKey[:]) {
			t.Errorf("key mismatch\n  got:  %x\n  want: %x", a.key, expectedKey[:])
		}
		if !bytes.Equal(a.iv, expectedIV[:]) {
			t.Errorf("iv mismatch\n  got:  %x\n  want: %x", a.iv, expectedIV[:])
		}
	})

	t.Run("32char_hex_password_uses_passwdOutward_directly", func(t *testing.T) {
		password := "0123456789abcdef0123456789abcdef"
		fileSize := int64(100)

		c, err := NewAesCtr(password, fileSize)
		if err != nil {
			t.Fatalf("NewAesCtr failed: %v", err)
		}
		a := c.(*aesCtr)

		passwdSalt := password + "100"
		expectedKey := md5.Sum([]byte(passwdSalt))

		expectedIV := md5.Sum([]byte("100"))

		if !bytes.Equal(a.key, expectedKey[:]) {
			t.Errorf("key mismatch\n  got:  %x\n  want: %x", a.key, expectedKey[:])
		}
		if !bytes.Equal(a.iv, expectedIV[:]) {
			t.Errorf("iv mismatch\n  got:  %x\n  want: %x", a.iv, expectedIV[:])
		}
	})

	t.Run("empty_password_derives_normally", func(t *testing.T) {
		c, err := NewAesCtr("", 512)
		if err != nil {
			t.Fatalf("NewAesCtr with empty password failed: %v", err)
		}
		a := c.(*aesCtr)

		if len(a.key) != 16 {
			t.Errorf("empty password: key length expected 16, got %d", len(a.key))
		}
		if len(a.iv) != 16 {
			t.Errorf("empty password: iv length expected 16, got %d", len(a.iv))
		}

		data := []byte("test data for empty password")
		enc := make([]byte, len(data))
		copy(enc, data)
		c.Encrypt(enc)

		c2, _ := NewAesCtr("", 512)
		dec := make([]byte, len(enc))
		copy(dec, enc)
		c2.Decrypt(dec)

		if !bytes.Equal(dec, data) {
			t.Error("empty password encrypt/decrypt roundtrip failed")
		}
	})

	t.Run("different_file_sizes_produce_different_keys", func(t *testing.T) {
		c1, _ := NewAesCtr("test123", 100)
		c2, _ := NewAesCtr("test123", 200)

		a1 := c1.(*aesCtr)
		a2 := c2.(*aesCtr)

		if bytes.Equal(a1.key, a2.key) {
			t.Error("different file sizes should produce different keys")
		}
		if bytes.Equal(a1.iv, a2.iv) {
			t.Error("different file sizes should produce different IVs")
		}
	})
}

func TestAesCtrEncryptDecryptRoundtrip(t *testing.T) {
	testCases := []struct {
		name string
		data string
	}{
		{"empty", ""},
		{"1_byte", "\x00"},
		{"15_bytes_less_than_block", strings.Repeat("a", 15)},
		{"16_bytes_exact_block", strings.Repeat("b", 16)},
		{"17_bytes_one_over_block", strings.Repeat("c", 17)},
		{"1KB", strings.Repeat("x", 1024)},
		{"1MB", strings.Repeat("y", 1024*1024)},
		{"all_zero_bytes", string(make([]byte, 100))},
		{"all_ff_bytes", string(make([]byte, 50))},
		{"mixed_binary", "\x00\x01\x02\xff\xfe\xfd hello world \x80\x7f"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			original := []byte(tc.data)
			cipher, err := NewAesCtr("testPassword", int64(len(original)))
			if err != nil {
				t.Fatalf("NewAesCtr failed: %v", err)
			}

			encrypted := make([]byte, len(original))
			copy(encrypted, original)
			cipher.Encrypt(encrypted)

			if len(original) > 0 && bytes.Equal(encrypted, original) {
				t.Error("encrypted data should differ from plaintext")
			}

			cipher2, err := NewAesCtr("testPassword", int64(len(original)))
			if err != nil {
				t.Fatalf("NewAesCtr #2 failed: %v", err)
			}
			decrypted := make([]byte, len(encrypted))
			copy(decrypted, encrypted)
			cipher2.Decrypt(decrypted)

			if !bytes.Equal(decrypted, original) {
				t.Errorf("roundtrip mismatch\n  original : %x\n  decrypted: %x", original, decrypted)
			}
		})
	}

	t.Run("wrong_password_produces_garbage", func(t *testing.T) {
		original := []byte("sensitive data")
		encCipher, _ := NewAesCtr("correct_password", int64(len(original)))

		encrypted := make([]byte, len(original))
		copy(encrypted, original)
		encCipher.Encrypt(encrypted)

		wrongDecCipher, _ := NewAesCtr("wrong_password", int64(len(original)))
		decrypted := make([]byte, len(encrypted))
		copy(decrypted, encrypted)
		wrongDecCipher.Decrypt(decrypted)

		if bytes.Equal(decrypted, original) {
			t.Error("wrong password should not decrypt to original data")
		}
	})

	t.Run("wrong_fileSize_produces_garbage", func(t *testing.T) {
		original := []byte("sensitive data")
		encCipher, _ := NewAesCtr("password", 999)

		encrypted := make([]byte, len(original))
		copy(encrypted, original)
		encCipher.Encrypt(encrypted)

		wrongDecCipher, _ := NewAesCtr("password", 888)
		decrypted := make([]byte, len(encrypted))
		copy(decrypted, encrypted)
		wrongDecCipher.Decrypt(decrypted)

		if bytes.Equal(decrypted, original) {
			t.Error("wrong fileSize should not decrypt to original data")
		}
	})
}

func TestSetPositionSeek(t *testing.T) {
	original := []byte("this is a test payload longer than 16 bytes for seeking!!")

	encCipher, _ := NewAesCtr("seek_test", int64(len(original)))
	encrypted := make([]byte, len(original))
	copy(encrypted, original)
	encCipher.Encrypt(encrypted)

	tests := []struct {
		name        string
		position    int64
		expectStart int
	}{
		{"position_0_from_start", 0, 0},
		{"position_16_skip_one_block", 16, 16},
		{"position_32_skip_two_blocks", 32, 32},
		{"position_mid", int64(len(original) / 2), len(original) / 2},
		{"position_10_non_aligned", 10, 10},
		{"position_1_non_aligned", 1, 1},
		{"position_15_non_aligned", 15, 15},
		{"position_17_non_aligned", 17, 17},
		{"position_last_byte", int64(len(original) - 1), len(original) - 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decCipher, _ := NewAesCtr("seek_test", int64(len(original)))
			err := decCipher.SetPosition(tc.position)
			if err != nil {
				t.Fatalf("SetPosition(%d) failed: %v", tc.position, err)
			}

			remaining := original[tc.expectStart:]
			decBuf := make([]byte, len(remaining))
			copy(decBuf, encrypted[tc.expectStart:])
			decCipher.Decrypt(decBuf)

			if !bytes.Equal(decBuf, remaining) {
				t.Errorf("decrypt at position %d mismatch\n  expect: %q (%x)\n  got:    %q (%x)",
					tc.position, remaining, remaining, decBuf, decBuf)
			}
		})
	}

	t.Run("negative_position_returns_error", func(t *testing.T) {
		c, _ := NewAesCtr("test", 100)
		err := c.SetPosition(-1)
		if err == nil {
			t.Error("expected error for negative position")
		}
	})

	t.Run("sequential_seeks_work_correctly", func(t *testing.T) {
		c, _ := NewAesCtr("seek_test", int64(len(original)))

		buf := make([]byte, 5)
		copy(buf, encrypted[0:5])
		c.Decrypt(buf)
		if !bytes.Equal(buf, original[0:5]) {
			t.Errorf("first read mismatch: got %q, want %q", buf, original[0:5])
		}

		err := c.SetPosition(20)
		if err != nil {
			t.Fatalf("SetPosition(20) failed: %v", err)
		}

		buf2 := make([]byte, 5)
		copy(buf2, encrypted[20:25])
		c.Decrypt(buf2)
		if !bytes.Equal(buf2, original[20:25]) {
			t.Errorf("seek to 20 mismatch: got %q, want %q", buf2, original[20:25])
		}

		err = c.SetPosition(0)
		if err != nil {
			t.Fatalf("SetPosition(0) failed: %v", err)
		}

		buf3 := make([]byte, 5)
		copy(buf3, encrypted[0:5])
		c.Decrypt(buf3)
		if !bytes.Equal(buf3, original[0:5]) {
			t.Errorf("seek back to 0 mismatch: got %q, want %q", buf3, original[0:5])
		}
	})
}

func TestMixBase64FilenameRoundtrip(t *testing.T) {
	testCases := []struct {
		name     string
		filename string
		password string
		encType  string
	}{
		{"simple_ascii", "test.txt", "password", "aesctr"},
		{"chinese_filename", "测试视频.mp4", "密码123", "aesctr"},
		{"long_filename", strings.Repeat("a", 200) + ".dat", "longpass", "aesctr"},
		{"unicode_mixed", "ファイル名.dat", "パスワード", "aesctr"},
		{"numeric_name", "12345.txt", "12345", "aesctr"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := EncodeName(tc.filename, tc.password, tc.encType)
			if encoded == "" {
				t.Fatalf("EncodeName returned empty for %q", tc.filename)
			}
			if encoded == tc.filename {
				t.Error("encoded name should differ from plaintext")
			}

			decoded := DecodeName(encoded, tc.password, tc.encType)
			if decoded != tc.filename {
				t.Errorf("roundtrip mismatch\n  original: %q\n  encoded: %q\n  decoded: %q",
					tc.filename, encoded, decoded)
			}
		})
	}

	t.Run("empty_string_roundtrip", func(t *testing.T) {
		encoded := EncodeName("", "password", "aesctr")
		decoded := DecodeName(encoded, "password", "aesctr")
		if decoded != "" {
			t.Errorf("empty string roundtrip: expected empty, got %q", decoded)
		}
	})

	t.Run("wrong_password_returns_empty", func(t *testing.T) {
		encoded := EncodeName("secret.txt", "correct_pass", "aesctr")
		decoded := DecodeName(encoded, "wrong_pass", "aesctr")
		if decoded != "" {
			t.Errorf("wrong password should return empty, got %q", decoded)
		}
	})

	t.Run("short_input_returns_empty", func(t *testing.T) {
		result := DecodeName("a", "password", "aesctr")
		if result != "" {
			t.Errorf("input shorter than 2 chars should return empty, got %q", result)
		}
	})

	t.Run("corrupted_crc6_returns_empty", func(t *testing.T) {
		encoded := EncodeName("test.txt", "password", "aesctr")
		if len(encoded) < 2 {
			t.Fatal("encoded too short")
		}
		corrupted := encoded[:len(encoded)-1] + "X"
		decoded := DecodeName(corrupted, "password", "aesctr")
		if decoded != "" {
			t.Errorf("corrupted CRC6 should return empty, got %q", decoded)
		}
	})

	t.Run("same_name_different_passwords_different_output", func(t *testing.T) {
		enc1 := EncodeName("file.txt", "pass_a", "aesctr")
		enc2 := EncodeName("file.txt", "pass_b", "aesctr")
		if enc1 == enc2 {
			t.Error("different passwords should produce different encoded names")
		}
	})
}

// TestConvertRealNameParity 验证 ConvertRealName 不再产生 "ext 重复" 的容器文件名
// 范式：EncodeName(baseName) + ext（ext 是用户原 ext，不是 .bin）
func TestConvertRealNameParity(t *testing.T) {
	cases := []struct {
		showName string
		password string
		wantExt  string
	}{
		{"CAD放样.mp4", "8682268", ".mp4"},
		{"test.txt", "pw", ".txt"},
		{"noext", "pw", ""}, // 无 ext
		{"a.b.c.d", "pw", ".d"},
	}
	for _, c := range cases {
		t.Run(c.showName, func(t *testing.T) {
			got := ConvertRealName(c.showName, c.password, "aesctr")
			if got == "" {
				t.Fatalf("ConvertRealName returned empty for %q", c.showName)
			}
			if !strings.HasSuffix(got, c.wantExt) {
				t.Errorf("ConvertRealName(%q) = %q, want suffix %q", c.showName, got, c.wantExt)
			}
			// 不应出现 ".ext.ext" 的双重 ext
			// （仅当 ext 非空时检查）
			if c.wantExt != "" && strings.HasSuffix(got, c.wantExt+c.wantExt) {
				t.Errorf("ConvertRealName(%q) = %q has doubled ext %q", c.showName, got, c.wantExt)
			}
		})
	}

	t.Run("orig_prefix_short_circuits", func(t *testing.T) {
		origName := OrigPrefix + "user_file.txt"
		got := ConvertRealName(origName, "pw", "aesctr")
		if got != "user_file.txt" {
			t.Errorf("orig_ prefix should be stripped, got %q", got)
		}
	})
}

func TestV2ContentHeaderDetection(t *testing.T) {
	t.Run("valid_AECTR2_header_detected", func(t *testing.T) {
		header := make([]byte, contentHeaderSize)
		copy(header[:magicLen], AECTR2Magic)
		header[6] = 0x02
		header[7] = 0x00
		nonceField := make([]byte, 16)
		for i := range nonceField {
			nonceField[i] = byte(i + 1)
		}
		copy(header[8:24], nonceField)
		binary.BigEndian.PutUint64(header[24:32], uint64(12345))

		if !IsV2Format(header) {
			t.Error("IsV2Format should return true for valid AECTR2 header")
		}

		parsed, err := DetectContentHeader(header)
		if err != nil {
			t.Fatalf("DetectContentHeader failed: %v", err)
		}
		if parsed.Magic != AECTR2Magic {
			t.Errorf("magic mismatch: got %q, want %q", parsed.Magic, AECTR2Magic)
		}
		if parsed.Version != 0x02 {
			t.Errorf("version mismatch: got 0x%02x, want 0x02", parsed.Version)
		}
		if parsed.Reserved != 0x00 {
			t.Errorf("reserved mismatch: got 0x%02x, want 0x00", parsed.Reserved)
		}
		if !bytes_equal(parsed.NonceField, nonceField) {
			t.Errorf("NonceField mismatch")
		}
		if parsed.PlainSize != 12345 {
			t.Errorf("PlainSize mismatch: got %d, want 12345", parsed.PlainSize)
		}
	})

	t.Run("non_AECTR2_data_not_detected", func(t *testing.T) {
		data := []byte("this is not an AECTR2 encrypted file content here")
		if IsV2Format(data) {
			t.Error("IsV2Format should return false for non-AECTR2 data")
		}
	})

	t.Run("shorter_than_magic_not_V2", func(t *testing.T) {
		shortData := []byte("AEC")
		if IsV2Format(shortData) {
			t.Error("IsV2Format should return false for data shorter than magic length")
		}
	})

	t.Run("exactly_magic_len_but_wrong_content", func(t *testing.T) {
		wrongMagic := []byte("AECTR9")
		if IsV2Format(wrongMagic) {
			t.Error("IsV2Format should return false for wrong magic")
		}
	})

	t.Run("31_bytes_shorter_than_header", func(t *testing.T) {
		header := make([]byte, contentHeaderSize-1)
		copy(header[:magicLen], AECTR2Magic)
		if !IsV2Format(header) {
			t.Error("31-byte header should still match IsV2Format by magic check")
		}
		_, err := DetectContentHeader(header)
		if err == nil {
			t.Error("DetectContentHeader should fail for short data")
		}
	})

	t.Run("unsupported_version_rejected", func(t *testing.T) {
		header := make([]byte, contentHeaderSize)
		copy(header[:magicLen], AECTR2Magic)
		header[6] = 0x99
		header[7] = 0x00
		binary.BigEndian.PutUint64(header[24:32], uint64(100))

		_, err := DetectContentHeader(header)
		if err == nil {
			t.Error("unsupported version should be rejected")
		}
	})

	t.Run("nonzero_reserved_rejected", func(t *testing.T) {
		header := make([]byte, contentHeaderSize)
		copy(header[:magicLen], AECTR2Magic)
		header[6] = 0x02
		header[7] = 0xFF
		binary.BigEndian.PutUint64(header[24:32], uint64(100))

		_, err := DetectContentHeader(header)
		if err == nil {
			t.Error("nonzero reserved byte should be rejected")
		}
	})

	t.Run("zero_plain_size_rejected", func(t *testing.T) {
		header := make([]byte, contentHeaderSize)
		copy(header[:magicLen], AECTR2Magic)
		header[6] = 0x02
		header[7] = 0x00
		binary.BigEndian.PutUint64(header[24:32], 0)

		_, err := DetectContentHeader(header)
		if err == nil {
			t.Error("zero PlainSize should be rejected")
		}
	})
}

func bytes_equal(a, b []byte) bool {
	return bytes.Equal(a, b)
}

func buildV2Header(plainSize int64, nonceField []byte) []byte {
	header := make([]byte, contentHeaderSize)
	copy(header[:magicLen], AECTR2Magic)
	header[6] = 0x02
	header[7] = 0x00
	if nonceField == nil {
		nonceField = make([]byte, 16)
	}
	copy(header[8:24], nonceField)
	binary.BigEndian.PutUint64(header[24:32], uint64(plainSize))
	return header
}

func TestDecryptReader(t *testing.T) {
	plaintext := []byte("Hello, DecryptReader! This is a test of the V1 and V2 streaming decryption.")
	password := "reader_test"

	t.Run("V1_raw_stream_decrypt", func(t *testing.T) {
		fileSize := int64(len(plaintext))

		encCipher, _ := NewAesCtr(password, fileSize)
		encrypted := make([]byte, len(plaintext))
		copy(encrypted, plaintext)
		encCipher.Encrypt(encrypted)

		reader := bytes.NewReader(encrypted)
		dr, err := NewDecryptReader(reader, password, fileSize)
		if err != nil {
			t.Fatalf("NewDecryptReader V1 failed: %v", err)
		}
		if dr.v2Header != nil {
			t.Error("V1 format should have nil v2Header")
		}

		decrypted := make([]byte, len(plaintext))
		n, err := readAll(dr, decrypted)
		if err != nil && err != io.EOF {
			t.Fatalf("Read failed: %v", err)
		}
		if n != len(plaintext) {
			t.Errorf("read %d bytes, expected %d", n, len(plaintext))
		}
		if !bytes.Equal(decrypted, plaintext) {
			t.Errorf("V1 decrypt mismatch\n  plaintext:  %q\n  decrypted: %q", plaintext, decrypted)
		}
	})

	t.Run("V2_with_header_auto_skip", func(t *testing.T) {
		plainSize := int64(len(plaintext))

		v2Header := buildV2Header(plainSize, nil)
		encCipher, _ := NewAesCtr(password, plainSize)
		encrypted := make([]byte, len(plaintext))
		copy(encrypted, plaintext)
		encCipher.Encrypt(encrypted)

		fullData := append(v2Header, encrypted...)

		reader := bytes.NewReader(fullData)
		dr, err := NewDecryptReader(reader, password, 0)
		if err != nil {
			t.Fatalf("NewDecryptReader V2 failed: %v", err)
		}
		if dr.v2Header == nil {
			t.Fatal("V2 format should have non-nil v2Header")
		}
		if dr.v2Header.PlainSize != plainSize {
			t.Errorf("v2Header.PlainSize = %d, want %d", dr.v2Header.PlainSize, plainSize)
		}

		decrypted := make([]byte, len(plaintext))
		n, err := readAll(dr, decrypted)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		if n != len(plaintext) {
			t.Errorf("read %d bytes, expected %d", n, len(plaintext))
		}
		if !bytes.Equal(decrypted, plaintext) {
			t.Errorf("V2 decrypt mismatch\n  plaintext:  %q\n  decrypted: %q", plaintext, decrypted)
		}
	})

	t.Run("V1_multiple_small_reads", func(t *testing.T) {
		fileSize := int64(len(plaintext))

		encCipher, _ := NewAesCtr(password, fileSize)
		encrypted := make([]byte, len(plaintext))
		copy(encrypted, plaintext)
		encCipher.Encrypt(encrypted)

		reader := bytes.NewReader(encrypted)
		dr, _ := NewDecryptReader(reader, password, fileSize)

		var result []byte
		buf := make([]byte, 10)
		for {
			n, err := dr.Read(buf)
			if n > 0 {
				result = append(result, buf[:n]...)
			}
			if err != nil {
				break
			}
		}

		if !bytes.Equal(result, plaintext) {
			t.Errorf("multi-read V1 decrypt mismatch\n  expected: %d bytes\n  got:      %d bytes",
				len(plaintext), len(result))
		}
	})

	t.Run("V2_large_data_1MB", func(t *testing.T) {
		largePlaintext := make([]byte, 1024*1024)
		for i := range largePlaintext {
			largePlaintext[i] = byte(i % 256)
		}
		plainSize := int64(len(largePlaintext))

		v2Header := buildV2Header(plainSize, nil)
		encCipher, _ := NewAesCtr(password, plainSize)
		encrypted := make([]byte, len(largePlaintext))
		copy(encrypted, largePlaintext)
		encCipher.Encrypt(encrypted)

		fullData := append(v2Header, encrypted...)

		reader := bytes.NewReader(fullData)
		dr, _ := NewDecryptReader(reader, password, 0)

		decrypted := make([]byte, len(largePlaintext))
		_, err := readAll(dr, decrypted)
		if err != nil {
			t.Fatalf("ReadAll 1MB failed: %v", err)
		}
		if !bytes.Equal(decrypted, largePlaintext) {
			t.Errorf("1MB V2 decrypt mismatch at first diff position")
			for i := range decrypted {
				if decrypted[i] != largePlaintext[i] {
					t.Errorf("first difference at byte %d: got 0x%02x, want 0x%02x",
						i, decrypted[i], largePlaintext[i])
					break
				}
			}
		}
	})

	t.Run("V1_empty_data", func(t *testing.T) {
		emptyEncrypted := []byte{}
		reader := bytes.NewReader(emptyEncrypted)
		dr, err := NewDecryptReader(reader, "pwd", 0)
		if err != nil {
			t.Fatalf("NewDecryptReader with empty data failed: %v", err)
		}

		buf := make([]byte, 100)
		n, err := dr.Read(buf)
		if err == nil {
			t.Error("expected EOF or error reading empty data")
		}
		if n != 0 {
			t.Errorf("expected 0 bytes from empty read, got %d", n)
		}
	})

	t.Run("V2_seek_updates_reader_position", func(t *testing.T) {
		plaintext := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
		plainSize := int64(len(plaintext))

		v2Header := buildV2Header(plainSize, nil)
		encCipher, _ := NewAesCtr(password, plainSize)
		encrypted := make([]byte, len(plaintext))
		copy(encrypted, plaintext)
		encCipher.Encrypt(encrypted)

		fullData := append(v2Header, encrypted...)

		reader := bytes.NewReader(fullData)
		dr, err := NewDecryptReader(reader, password, 0)
		if err != nil {
			t.Fatalf("NewDecryptReader V2 failed: %v", err)
		}

		pos, err := dr.Seek(20, io.SeekStart)
		if err != nil {
			t.Fatalf("Seek to 20 failed: %v", err)
		}
		if pos != 20 {
			t.Errorf("Seek returned %d, want 20", pos)
		}
		if dr.pos != 20 {
			t.Errorf("dr.pos = %d, want 20", dr.pos)
		}
		if dr.readerPos != 20+contentHeaderSize {
			t.Errorf("dr.readerPos = %d, want %d (pos + headerSize)", dr.readerPos, 20+contentHeaderSize)
		}

		buf := make([]byte, 10)
		n, _ := dr.Read(buf)
		expected := plaintext[20 : 20+10]
		if !bytes.Equal(buf[:n], expected) {
			t.Errorf("after seek+read: got %q, want %q", buf[:n], expected)
		}
		if dr.pos != 30 {
			t.Errorf("dr.pos after read = %d, want 30", dr.pos)
		}
		if dr.readerPos != 30+contentHeaderSize {
			t.Errorf("dr.readerPos after read = %d, want %d", dr.readerPos, 30+contentHeaderSize)
		}
	})

	t.Run("V2_seek_mid_stream_then_read_remaining", func(t *testing.T) {
		plaintext := make([]byte, 256)
		for i := range plaintext {
			plaintext[i] = byte(i)
		}
		plainSize := int64(len(plaintext))

		v2Header := buildV2Header(plainSize, nil)
		encCipher, _ := NewAesCtr(password, plainSize)
		encrypted := make([]byte, len(plaintext))
		copy(encrypted, plaintext)
		encCipher.Encrypt(encrypted)

		fullData := append(v2Header, encrypted...)
		reader := bytes.NewReader(fullData)
		dr, _ := NewDecryptReader(reader, password, 0)

		dr.Seek(100, io.SeekStart)
		buf := make([]byte, 156)
		n, _ := readAll(dr, buf)

		expected := plaintext[100:]
		if !bytes.Equal(buf[:n], expected) {
			t.Errorf("mid-stream seek+read remaining mismatch at first diff")
		}
	})

	t.Run("V2_seek_from_end", func(t *testing.T) {
		plaintext := []byte("end-seek-test-data!!")
		plainSize := int64(len(plaintext))

		v2Header := buildV2Header(plainSize, nil)
		encCipher, _ := NewAesCtr(password, plainSize)
		encrypted := make([]byte, len(plaintext))
		copy(encrypted, plaintext)
		encCipher.Encrypt(encrypted)

		fullData := append(v2Header, encrypted...)
		reader := bytes.NewReader(fullData)
		dr, _ := NewDecryptReader(reader, password, 0)

		pos, err := dr.Seek(-5, io.SeekEnd)
		if err != nil {
			t.Fatalf("Seek from end failed: %v", err)
		}
		wantPos := plainSize - 5
		if pos != wantPos {
			t.Errorf("seek from end -5: got %d, want %d", pos, wantPos)
		}

		buf := make([]byte, 10)
		n, _ := dr.Read(buf)
		expected := plaintext[plainSize-5:]
		if !bytes.Equal(buf[:n], expected) {
			t.Errorf("seek from end read: got %q, want %q", buf[:n], expected)
		}
	})

	t.Run("V1_non_seekable_source_seek_only_cipher", func(t *testing.T) {
		plaintext := []byte("non-seekable-source-test")
		fileSize := int64(len(plaintext))

		encCipher, _ := NewAesCtr(password, fileSize)
		encrypted := make([]byte, len(plaintext))
		copy(encrypted, plaintext)
		encCipher.Encrypt(encrypted)

		pipeReader, pipeWriter := io.Pipe()
		go func() {
			pipeWriter.Write(encrypted)
			pipeWriter.Close()
		}()

		dr, err := NewDecryptReader(pipeReader, password, fileSize)
		if err != nil {
			t.Fatalf("NewDecryptReader V1 pipe failed: %v", err)
		}
		if dr.seeker != nil {
			t.Error("Pipe reader should not be seekable, seeker should be nil")
		}

		pos, err := dr.Seek(5, io.SeekStart)
		if err != nil {
			t.Fatalf("Seek on non-seekable source should still update cipher: %v", err)
		}
		if pos != 5 {
			t.Errorf("Seek returned %d, want 5", pos)
		}
		if dr.pos != 5 {
			t.Errorf("dr.pos = %d after seek, want 5", dr.pos)
		}
	})

	t.Run("seek_negative_rejected", func(t *testing.T) {
		plaintext := []byte("test")
		reader := bytes.NewReader([]byte{})
		dr, _ := NewDecryptReader(reader, "pwd", int64(len(plaintext)))

		_, err := dr.Seek(-1, io.SeekStart)
		if err == nil {
			t.Error("negative Seek should return error")
		}
	})
}

type readCloser struct {
	io.Reader
}

func readAll(r io.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
