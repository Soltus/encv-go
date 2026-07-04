package alistencrypt

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

const testPassword = "test-password-123"

func newPluginWithSettings(t *testing.T, suffix, password, encType string) (*AlistEncryptPlugin, context.Context) {
	t.Helper()
	cfg := &config.Config{
		Password: "global-password",
		PluginSettings: map[string]json.RawMessage{
			"alist_encrypt": mustMarshalJSON(t, AlistEncryptPluginConfig{
				Suffix:          suffix,
				DefaultPassword: password,
				EncType:         encType,
			}),
		},
	}
	ctx := config.NewContext(context.Background(), cfg)
	p := &AlistEncryptPlugin{}
	return p, ctx
}

func mustMarshalJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

func createTestEncryptedFile(t *testing.T, dir string, plaintext []byte, password string) string {
	t.Helper()
	reader := bytes.NewReader(plaintext)
	settings := &AlistEncryptPluginConfig{
		Suffix:  ".bin",
		EncType: "aesctr",
	}
	result, err := EncryptToFile(reader, password, dir, settings)
	require.NoError(t, err)
	require.NotNil(t, result)
	return result.TempPath
}

func createV2EncryptedFile(t *testing.T, dir string, plaintext []byte, password string) string {
	t.Helper()
	return createTestEncryptedFile(t, dir, plaintext, password)
}

func TestPluginInitialization(t *testing.T) {
	t.Run("Initialize_success", func(t *testing.T) {
		p, ctx := newPluginWithSettings(t, ".bin", testPassword, "aesctr")
		err := p.Initialize(ctx)
		require.NoError(t, err)
		assert.Equal(t, ".bin", p.settings.Suffix)
		assert.Equal(t, testPassword, p.settings.DefaultPassword)
		assert.Equal(t, "aesctr", p.settings.EncType)
	})

	t.Run("suffix_sccgv_reserved_allowed", func(t *testing.T) {
		// .sccgv is the V2 reserved extension; alist_encrypt IS the V2
		// implementation, so it must be allowed to use .sccgv (no fallback).
		// Cross-plugin conflicts with .sccgv are detected by
		// ValidateExtensionUniqueness(), not silently masked here.
		p, ctx := newPluginWithSettings(t, ".sccgv", testPassword, "aesctr")
		err := p.Initialize(ctx)
		require.NoError(t, err)
		assert.Equal(t, ".sccgv", p.settings.Suffix, "reserved V2 suffix should be preserved")
	})

	t.Run("suffix_no_dot_auto_fix", func(t *testing.T) {
		p, ctx := newPluginWithSettings(t, "bin", testPassword, "aesctr")
		err := p.Initialize(ctx)
		require.NoError(t, err)
		assert.Equal(t, ".bin", p.settings.Suffix, "suffix without dot should be corrected to .bin")
	})

	t.Run("suffix_too_long_fallback", func(t *testing.T) {
		longSuffix := ".abcdefghijklmnopqrstuvwxyz"
		p, ctx := newPluginWithSettings(t, longSuffix, testPassword, "aesctr")
		err := p.Initialize(ctx)
		require.NoError(t, err)
		assert.Equal(t, ".bin", p.settings.Suffix, "suffix >16 chars should fall back to .bin")
	})

	t.Run("enc_type_non_aesctr_warn", func(t *testing.T) {
		p, ctx := newPluginWithSettings(t, ".bin", testPassword, "rc4md5")
		err := p.Initialize(ctx)
		require.NoError(t, err, "unsupported enc_type should not cause Initialize to fail")
		assert.Equal(t, "rc4md5", p.settings.EncType, "enc_type value preserved as-is")
	})
}

func TestCanDecrypt(t *testing.T) {
	t.Run("bin_file_returns_true", func(t *testing.T) {
		p, ctx := newPluginWithSettings(t, ".bin", testPassword, "aesctr")
		require.NoError(t, p.Initialize(ctx))

		tmpDir := t.TempDir()
		plaintext := []byte("test data for can decrypt")
		encPath := createV2EncryptedFile(t, tmpDir, plaintext, testPassword)

		finalPath := filepath.Join(tmpDir, "test.bin")
		require.NoError(t, os.Rename(encPath, finalPath))

		assert.True(t, p.CanDecrypt(finalPath), ".bin file with AECTR2 header should return true")
	})

	t.Run("mp4_file_returns_false", func(t *testing.T) {
		p, ctx := newPluginWithSettings(t, ".bin", testPassword, "aesctr")
		require.NoError(t, p.Initialize(ctx))

		tmpDir := t.TempDir()
		mp4Path := filepath.Join(tmpDir, "video.mp4")
		require.NoError(t, os.WriteFile(mp4Path, []byte("fake mp4"), 0644))

		assert.False(t, p.CanDecrypt(mp4Path), ".mp4 file should return false")
	})

	t.Run("BIN_uppercase_returns_true", func(t *testing.T) {
		p, ctx := newPluginWithSettings(t, ".bin", testPassword, "aesctr")
		require.NoError(t, p.Initialize(ctx))

		tmpDir := t.TempDir()
		plaintext := []byte("uppercase test")
		encPath := createV2EncryptedFile(t, tmpDir, plaintext, testPassword)

		finalPath := filepath.Join(tmpDir, "test.BIN")
		require.NoError(t, os.Rename(encPath, finalPath))

		assert.True(t, p.CanDecrypt(finalPath), ".BIN uppercase should return true (case insensitive)")
	})

	t.Run("sccgv_returns_false", func(t *testing.T) {
		p, ctx := newPluginWithSettings(t, ".bin", testPassword, "aesctr")
		require.NoError(t, p.Initialize(ctx))

		tmpDir := t.TempDir()
		sccgvPath := filepath.Join(tmpDir, "file.sccgv")
		require.NoError(t, os.WriteFile(sccgvPath, []byte("v4 container"), 0644))

		assert.False(t, p.CanDecrypt(sccgvPath), ".sccgv should return false (V4 container has its own plugin)")
	})

	t.Run("V1_legacy_no_magic_returns_true", func(t *testing.T) {
		// Legacy / v1 alist-encrypt ciphertexts have no AECTR2 magic header;
		// the plugin should still claim ownership so that predict-plugin
		// returns the alist_encrypt candidate and the modal can render.
		p, ctx := newPluginWithSettings(t, ".bin", testPassword, "aesctr")
		require.NoError(t, p.Initialize(ctx))

		plaintext := make([]byte, 256)
		for i := range plaintext {
			plaintext[i] = byte((i * 7) ^ 0xA5)
		}

		cipher, err := Create(testPassword, "aesctr", int64(len(plaintext)))
		require.NoError(t, err)
		cipher.Encrypt(plaintext)

		tmpDir := t.TempDir()
		v1Path := filepath.Join(tmpDir, "legacy_no_magic.bin")
		require.NoError(t, os.WriteFile(v1Path, plaintext, 0644))

		assert.True(t, p.CanDecrypt(v1Path),
			"V1 (no AECTR2 magic) .bin should still be claimed by alist_encrypt")
	})

	t.Run("renamed_mp4_with_bin_ext_returns_false", func(t *testing.T) {
		// A user-renamed MP4 (e.g. uploaded as .bin) must not be claimed
		// by the alist_encrypt plugin so the regular file flow handles it.
		p, ctx := newPluginWithSettings(t, ".bin", testPassword, "aesctr")
		require.NoError(t, p.Initialize(ctx))

		tmpDir := t.TempDir()
		mp4Bytes := append([]byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'},
			make([]byte, 100)...)
		renamed := filepath.Join(tmpDir, "fake.bin")
		require.NoError(t, os.WriteFile(renamed, mp4Bytes, 0644))

		assert.False(t, p.CanDecrypt(renamed),
			"plain MP4 magic should be detected and excluded from alist_encrypt")
	})
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	t.Run("V1_raw_stream_roundtrip", func(t *testing.T) {
		plaintext := make([]byte, 1024)
		for i := range plaintext {
			plaintext[i] = byte(i % 256)
		}

		cipher, err := Create(testPassword, "aesctr", int64(len(plaintext)))
		require.NoError(t, err)

		encrypted := make([]byte, len(plaintext))
		copy(encrypted, plaintext)
		cipher.Encrypt(encrypted)

		reader := bytes.NewReader(encrypted)
		dr, err := NewDecryptReader(reader, testPassword, int64(len(plaintext)))
		require.NoError(t, err)

		decrypted, err := io.ReadAll(dr)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted, "V1 roundtrip data should match")
	})

	t.Run("V2_with_header_roundtrip", func(t *testing.T) {
		plaintext := make([]byte, 1024)
		for i := range plaintext {
			plaintext[i] = byte(i % 256)
		}

		tmpDir := t.TempDir()
		encPath := createV2EncryptedFile(t, tmpDir, plaintext, testPassword)

		f, err := os.Open(encPath)
		require.NoError(t, err)
		defer f.Close()

		info, err := f.Stat()
		require.NoError(t, err)

		dr, err := NewDecryptReader(f, testPassword, info.Size())
		require.NoError(t, err)

		decrypted, err := io.ReadAll(dr)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted, "V2 roundtrip data should match")
	})

	t.Run("empty_data_roundtrip", func(t *testing.T) {
		plaintext := []byte{}

		tmpDir := t.TempDir()
		encPath := createV2EncryptedFile(t, tmpDir, plaintext, testPassword)

		f, err := os.Open(encPath)
		require.NoError(t, err)
		defer f.Close()

		info, err := f.Stat()
		require.NoError(t, err)

		_, err = NewDecryptReader(f, testPassword, info.Size())
		require.Error(t, err, "V2 header with PlainSize=0 must be rejected by DetectContentHeader")
		assert.Contains(t, err.Error(), "invalid plaintext size")
	})
}

func TestEncryptWithV2Header(t *testing.T) {
	t.Run("header_32_bytes_structure_correct", func(t *testing.T) {
		plaintext := []byte("Hello World AES-CTR test data!!!")
		tmpDir := t.TempDir()
		encPath := createV2EncryptedFile(t, tmpDir, plaintext, testPassword)

		data, err := os.ReadFile(encPath)
		require.NoError(t, err)
		require.GreaterOrEqual(t, len(data), 32, "encrypted file must be at least 32 bytes (header)")

		header := data[:32]
		magic := string(header[:6])
		assert.Equal(t, AECTR2Magic, magic, "header magic should be AECTR2")

		version := header[6]
		assert.Equal(t, byte(0x02), version, "header version should be 0x02")

		reserved := header[7]
		assert.Equal(t, byte(0x00), reserved, "header reserved byte should be 0x00")

		nonceField := header[8:24]
		assert.Len(t, nonceField, 16, "NonceField should be 16 bytes")

		plainSize := binary.BigEndian.Uint64(header[24:32])
		assert.Equal(t, uint64(len(plaintext)), plainSize, "PlainSize should match original data length")
	})

	t.Run("decrypt_auto_skips_32_byte_header", func(t *testing.T) {
		plaintext := make([]byte, 512)
		for i := range plaintext {
			plaintext[i] = byte(i % 256)
		}

		tmpDir := t.TempDir()
		encPath := createV2EncryptedFile(t, tmpDir, plaintext, testPassword)

		f, err := os.Open(encPath)
		require.NoError(t, err)
		defer f.Close()

		info, err := f.Stat()
		require.NoError(t, err)

		dr, err := NewDecryptReader(f, testPassword, info.Size())
		require.NoError(t, err)

		decrypted, err := io.ReadAll(dr)
		require.NoError(t, err)
		assert.Equal(t, plaintext, decrypted, "after auto-skipping 32-byte header, data should match plaintext")
	})
}

func TestStreamRange(t *testing.T) {
	plaintext := make([]byte, 1024)
	for i := range plaintext {
		plaintext[i] = byte(i % 256)
	}

	tmpDir := t.TempDir()
	encPath := createV2EncryptedFile(t, tmpDir, plaintext, testPassword)

	p, ctx := newPluginWithSettings(t, ".bin", testPassword, "aesctr")
	require.NoError(t, p.Initialize(ctx))

	// 走统一范式：Stream() 拿 reader + size + name，构造 FileContentProvider，
	// 委托给 ContentHandler.ServeFile 处理 HTTP 协议。
	// 这与 v4 容器预览走同一套逻辑，避免在插件内重复实现 Range/206。
	helper := func(t *testing.T, req *http.Request) *httptest.ResponseRecorder {
		t.Helper()
		rc, size, _, showName, err := p.Stream(encPath, testPassword)
		require.NoError(t, err)
		sr, ok := rc.(*SeekableDecryptReader)
		require.True(t, ok, "Stream() must return *SeekableDecryptReader")
		prov := NewAlistEncryptFileProvider(sr, size, showName)
		defer prov.Close()
		rec := httptest.NewRecorder()
		handler.NewContentHandler().ServeFile(rec, req, prov)
		return rec
	}

	t.Run("range_full_200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test.bin", nil)
		rec := helper(t, req)
		assert.Equal(t, http.StatusOK, rec.Result().StatusCode)

		body := rec.Body.Bytes()
		assert.Equal(t, plaintext, body, "full range should return all data")
	})

	t.Run("range_partial_206", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test.bin", nil)
		req.Header.Set("Range", "bytes=100-200")
		rec := helper(t, req)
		assert.Equal(t, http.StatusPartialContent, rec.Result().StatusCode)
		assert.Equal(t, int64(101), rec.Result().ContentLength,
			"Content-Length must be 101 (the partial length)")

		body := rec.Body.Bytes()
		assert.Len(t, body, 101, "range bytes=100-200 should return 101 bytes (100-200 inclusive)")
	})

	t.Run("range_end_exceeds_truncated_206", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test.bin", nil)
		req.Header.Set("Range", "bytes=900-9999")
		rec := helper(t, req)
		assert.Equal(t, http.StatusPartialContent, rec.Result().StatusCode,
			"end > fileSize should return 206 with truncation")

		body := rec.Body.Bytes()
		assert.Len(t, body, 124, "truncated range 900-1023 should return 124 bytes")
	})

	t.Run("range_start_exceeds_returns_416", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test.bin", nil)
		req.Header.Set("Range", "bytes=9999-10000")
		rec := helper(t, req)
		assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, rec.Result().StatusCode,
			"start > fileSize must return 416 Range Not Satisfiable")
	})

	t.Run("range_suffix_returns_206", func(t *testing.T) {
		// RFC 7233 §2.1: "bytes=-50" 是合法的 suffix-range，
		// 表示"最后 50 个字节"，应返回 206 + Content-Range + 最后 50 字节。
		// 这与 v4 容器预览走同一套 ContentHandler 行为，验证统一范式下的协议合规。
		req := httptest.NewRequest(http.MethodGet, "/test.bin", nil)
		req.Header.Set("Range", "bytes=-50")
		rec := helper(t, req)
		assert.Equal(t, http.StatusPartialContent, rec.Result().StatusCode,
			"suffix-range must return 206 Partial Content with last N bytes")
		assert.Equal(t, int64(50), rec.Result().ContentLength,
			"Content-Length must be 50 (the suffix length)")

		body := rec.Body.Bytes()
		expected := plaintext[len(plaintext)-50:]
		assert.Equal(t, expected, body, "body must be the last 50 bytes of plaintext")
	})
}

func TestBoundaryEmptyFile(t *testing.T) {
	p, ctx := newPluginWithSettings(t, ".bin", testPassword, "aesctr")
	require.NoError(t, p.Initialize(ctx))

	tmpDir := t.TempDir()
	emptyPath := filepath.Join(tmpDir, "empty.bin")
	require.NoError(t, os.WriteFile(emptyPath, []byte{}, 0644))

	canDecrypt := p.CanDecrypt(emptyPath)
	assert.False(t, canDecrypt, "empty file cannot be a valid encrypted file (no AECTR2 magic)")
}

func TestBoundaryTooSmallFile(t *testing.T) {
	t.Run("V2_magic_but_falls_back_to_V1", func(t *testing.T) {
		shortData := make([]byte, 16)
		copy(shortData[:], []byte(AECTR2Magic))
		tmpDir := t.TempDir()
		shortPath := filepath.Join(tmpDir, "short.bin")
		require.NoError(t, os.WriteFile(shortPath, shortData, 0644))

		f, err := os.Open(shortPath)
		require.NoError(t, err)
		defer f.Close()

		info, err := f.Stat()
		require.NoError(t, err)

		dr, err := NewDecryptReader(f, testPassword, info.Size())
		require.NoError(t, err,
			"actual behavior: n=16 < contentHeaderSize(32), skips V2 detection, falls through to V1 path")

		buf := make([]byte, 100)
		n, readErr := dr.Read(buf)
		assert.LessOrEqual(t, n, 16, "V1 mode: should read at most the 16 available bytes")
		if readErr != nil {
			assert.Contains(t, readErr.Error(), "EOF")
		}
	})

	t.Run("V1_tiny_data_read_eof", func(t *testing.T) {
		tinyData := []byte{0x01, 0x02, 0x03}
		reader := bytes.NewReader(tinyData)

		dr, err := NewDecryptReader(reader, testPassword, int64(len(tinyData)))
		if err != nil {
			return
		}

		buf := make([]byte, 100)
		n, readErr := dr.Read(buf)
		assert.LessOrEqual(t, n, len(tinyData), "should read at most the available bytes")
		if readErr != nil {
			assert.Contains(t, readErr.Error(), "EOF")
		}
	})
}

func TestBoundaryV2ZeroPlainSize(t *testing.T) {
	header := make([]byte, 32)
	copy(header[:6], []byte(AECTR2Magic))
	header[6] = 0x02
	header[7] = 0x00
	binary.BigEndian.PutUint64(header[24:32], 0)

	_, err := DetectContentHeader(header)
	require.Error(t, err, "V2 header with PlainSize=0 should be rejected")
	assert.Contains(t, err.Error(), "invalid plaintext size")
}

func TestBoundarySizeMismatch(t *testing.T) {
	declaredPlainSize := int64(100)
	actualCiphertext := make([]byte, 50)

	header := make([]byte, 32)
	copy(header[:6], []byte(AECTR2Magic))
	header[6] = 0x02
	header[7] = 0x00
	binary.BigEndian.PutUint64(header[24:32], uint64(declaredPlainSize))

	fullData := append(header, actualCiphertext...)
	reader := bytes.NewReader(fullData)

	dr, err := NewDecryptReader(reader, testPassword, 0)
	require.NoError(t, err, "NewDecryptReader should succeed (size mismatch detected at Read time)")

	buf := make([]byte, 1024)
	n, _ := io.ReadFull(dr, buf)
	assert.LessOrEqual(t, n, int(declaredPlainSize),
		"read bytes (%d) should not exceed declared PlainSize (%d)", n, declaredPlainSize)
	if n < int(declaredPlainSize) {
		t.Logf("size mismatch confirmed: read %d bytes but V2 header declares %d plain bytes", n, declaredPlainSize)
	}
}

func TestBoundaryPasswordHeuristic(t *testing.T) {
	plaintext := []byte("Hello World AES-CTR test data!!! This is known plaintext for heuristic checking!!!")

	cipher, err := Create(testPassword, "aesctr", int64(len(plaintext)))
	require.NoError(t, err)

	encrypted := make([]byte, len(plaintext))
	copy(encrypted, plaintext)
	cipher.Encrypt(encrypted)

	header := make([]byte, 32)
	copy(header[:6], []byte(AECTR2Magic))
	header[6] = 0x02
	header[7] = 0x00
	binary.BigEndian.PutUint64(header[24:32], uint64(len(plaintext)))

	fullData := append(header, encrypted...)
	reader := bytes.NewReader(fullData)

	wrongPassword := "completely-wrong-password-99999"
	dr, err := NewDecryptReader(reader, wrongPassword, 0)
	if err != nil {
		var decErr *DecryptionError
		if errors.As(err, &decErr) {
			assert.True(t, errors.Is(decErr.Err, ErrInvalidPassword),
				"wrong password should produce ErrInvalidPassword")
		}
		return
	}

	decrypted, err := io.ReadAll(dr)
	require.NoError(t, err)

	isGarbage := !bytes.Equal(decrypted, plaintext)
	assert.True(t, isGarbage, "wrong password should NOT decrypt to original plaintext")

	if len(decrypted) > 0 {
		printableCount := 0
		checkLen := len(plaintext)
		if checkLen > len(decrypted) {
			checkLen = len(decrypted)
		}
		for _, b := range decrypted[:checkLen] {
			if (b >= 0x20 && b <= 0x7e) || b == '\n' || b == '\r' || b == '\t' {
				printableCount++
			}
		}
		ratio := float64(printableCount) / float64(checkLen)
		assert.Less(t, ratio, 0.9,
			"wrong password decryption should have low printable ratio, got %.2f", ratio)
	}
}
