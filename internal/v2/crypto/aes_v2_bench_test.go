package crypto

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"testing"
)

// --- 密钥派生 ---

func BenchmarkGenerateKey_v2(b *testing.B) {
	salt := make([]byte, SaltSize_v2)
	rand.Read(salt)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		GenerateKey("benchmark-password", salt, KeySize_v2)
	}
}

func BenchmarkGenerateSalt_v2(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = GenerateSalt_v2(SaltSize_v2)
	}
}

func BenchmarkGenerateIV_v2(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = GenerateIV_v2(IVSize_v2)
	}
}

// --- 流式加解密吞吐量 ---

func BenchmarkEncryptStream_v2(b *testing.B) {
	maxSize := benchCryptoMaxSize()
	sizes := []int64{
		1 * 1024,
		64 * 1024,
		256 * 1024,
		1 * 1024 * 1024,
	}
	if maxSize >= 4*1024*1024 {
		sizes = append(sizes, 4*1024*1024)
	}
	if maxSize >= 16*1024*1024 {
		sizes = append(sizes, 16*1024*1024)
	}

	salt, _ := GenerateSalt_v2(SaltSize_v2)
	key := GenerateKey("benchmark-password", salt, KeySize_v2)
	iv, _ := GenerateIV_v2(IVSize_v2)

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%s", humanBytes(size)), func(b *testing.B) {
			data := make([]byte, size)
			rand.Read(data)

			b.SetBytes(size)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				src := bytes.NewReader(data)
				_ = EncryptStream_v2(src, io.Discard, key, iv)
			}
		})
	}
}

func BenchmarkDecryptStream_v2(b *testing.B) {
	maxSize := benchCryptoMaxSize()
	sizes := []int64{
		1 * 1024,
		64 * 1024,
		256 * 1024,
		1 * 1024 * 1024,
	}
	if maxSize >= 4*1024*1024 {
		sizes = append(sizes, 4*1024*1024)
	}
	if maxSize >= 16*1024*1024 {
		sizes = append(sizes, 16*1024*1024)
	}

	salt, _ := GenerateSalt_v2(SaltSize_v2)
	key := GenerateKey("benchmark-password", salt, KeySize_v2)
	iv, _ := GenerateIV_v2(IVSize_v2)

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%s", humanBytes(size)), func(b *testing.B) {
			plaintext := make([]byte, size)
			rand.Read(plaintext)

			var ciphertext bytes.Buffer
			EncryptStream_v2(bytes.NewReader(plaintext), &ciphertext, key, iv)
			encData := ciphertext.Bytes()

			b.SetBytes(size)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				_ = DecryptStream_v2(bytes.NewReader(encData), io.Discard, key, iv)
			}
		})
	}
}

// --- 内存中加解密 ---

func BenchmarkEncryptBytes_v2(b *testing.B) {
	sizes := []int64{
		1 * 1024,
		64 * 1024,
		1 * 1024 * 1024,
	}

	salt, _ := GenerateSalt_v2(SaltSize_v2)
	key := GenerateKey("benchmark-password", salt, KeySize_v2)
	iv, _ := GenerateIV_v2(IVSize_v2)

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%s", humanBytes(size)), func(b *testing.B) {
			data := make([]byte, size)
			rand.Read(data)

			b.SetBytes(size)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				_, _ = EncryptBytes_v2(data, key, iv)
			}
		})
	}
}

func BenchmarkDecryptBytes_v2(b *testing.B) {
	sizes := []int64{
		1 * 1024,
		64 * 1024,
		1 * 1024 * 1024,
	}

	salt, _ := GenerateSalt_v2(SaltSize_v2)
	key := GenerateKey("benchmark-password", salt, KeySize_v2)
	iv, _ := GenerateIV_v2(IVSize_v2)

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%s", humanBytes(size)), func(b *testing.B) {
			plaintext := make([]byte, size)
			rand.Read(plaintext)
			ciphertext, _ := EncryptBytes_v2(plaintext, key, iv)

			b.SetBytes(size)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				_, _ = DecryptBytes_v2(ciphertext, key, iv)
			}
		})
	}
}

// --- CTR IV 推导（Seek 性能关键） ---

func BenchmarkDeriveCTRIVForOffset_v2(b *testing.B) {
	baseIV, _ := GenerateIV_v2(IVSize_v2)

	offsets := []uint64{
		0,
		1 * 1024 * 1024,         // 1MB
		100 * 1024 * 1024,       // 100MB
		1 * 1024 * 1024 * 1024,  // 1GB
		10 * 1024 * 1024 * 1024, // 10GB
	}

	for _, offset := range offsets {
		b.Run(fmt.Sprintf("offset=%s", humanBytes(int64(offset))), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = DeriveCTRIVForOffset_v2(baseIV, offset)
			}
		})
	}
}

// --- 系统密钥加解密（Manifest 专用） ---

func BenchmarkEncryptSystemPayload(b *testing.B) {
	sizes := []int64{
		1 * 1024,
		10 * 1024,
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%s", humanBytes(size)), func(b *testing.B) {
			data := make([]byte, size)
			rand.Read(data)

			b.SetBytes(size)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				_, _ = EncryptSystemPayload(data)
			}
		})
	}
}

func BenchmarkDecryptSystemPayload(b *testing.B) {
	sizes := []int64{
		1 * 1024,
		10 * 1024,
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%s", humanBytes(size)), func(b *testing.B) {
			data := make([]byte, size)
			rand.Read(data)
			encData, _ := EncryptSystemPayload(data)

			b.SetBytes(size)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				_, _ = DecryptSystemPayload(encData)
			}
		})
	}
}

// benchCryptoMaxSize 根据环境变量决定最大测试数据尺寸
func benchCryptoMaxSize() int64 {
	if os.Getenv("ENCV_BENCH_LOW_MEM") != "" {
		return 4 * 1024 * 1024 // 低内存模式：最大 4MB
	}
	return 16 * 1024 * 1024 // 正常模式：最大 16MB
}

// --- 完整加密到临时文件流程（含磁盘I/O） ---

func BenchmarkEncryptToTempFile_v2(b *testing.B) {
	maxSize := benchCryptoMaxSize()
	sizes := []int64{1 * 1024 * 1024} // 1MB 始终测试
	if maxSize >= 10*1024*1024 {
		sizes = append(sizes, 10*1024*1024) // 10MB
	}

	password := "benchmark-password"

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%s", humanBytes(size)), func(b *testing.B) {
			data := make([]byte, size)
			rand.Read(data)

			// 循环外创建临时目录，避免 b.TempDir() 在循环内堆积
			workDir := b.TempDir()

			b.SetBytes(size)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				src := bytes.NewReader(data)
				result, err := EncryptToTempFile_v2(src, password, workDir)
				if err != nil {
					b.Fatal(err)
				}
				os.Remove(result.TempPath)
			}
		})
	}
}

// --- 分层密钥（信封加密）基准 ---

func BenchmarkDeriveKEK(b *testing.B) {
	salt := make([]byte, SaltSize_v2)
	rand.Read(salt)
	password := "benchmark-password"

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = DeriveKEK(password, salt)
	}
}

func BenchmarkGenerateDEK(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = GenerateDEK(KeySize_v4_128)
	}
}

func BenchmarkWrapUnwrapDEK(b *testing.B) {
	salt := make([]byte, SaltSize_v2)
	rand.Read(salt)
	password := "benchmark-password"
	kek := DeriveKEK(password, salt)
	dek, _ := GenerateDEK(KeySize_v4_128)
	aad := []byte("benchmark-aad")

	b.Run("WrapDEK", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_, _ = WrapDEK(dek, kek, aad)
		}
	})

	wd, _ := WrapDEK(dek, kek, aad)
	b.Run("UnwrapDEK", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_, _ = UnwrapDEK(wd, kek)
		}
	})
}

// --- 辅助函数 ---

func humanBytes(n int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%dGB", n/GB)
	case n >= MB:
		return fmt.Sprintf("%dMB", n/MB)
	case n >= KB:
		return fmt.Sprintf("%dKB", n/KB)
	default:
		return fmt.Sprintf("%dB", n)
	}
}
