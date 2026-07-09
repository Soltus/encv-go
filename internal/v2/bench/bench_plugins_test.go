package bench_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	v2crypto "github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/plugins"
)

const (
	benchPluginPassword = "bench-plugin-password-123"
)

func generateRandomData(size int) []byte {
	data := make([]byte, size)
	rand.Read(data)
	return data
}

func makePluginBenchContext() context.Context {
	userSettings := map[string]json.RawMessage{
		"video": json.RawMessage(`{
			"ext": ".sccgv",
			"chunk_size_mb": 0,
			"light_main_chunk_enabled": true,
			"verify_after_pack": false,
			"track_extensions": ".ass,.srt,.dm.ass,.vtt",
			"plugin_cache_dir": ""
		}`),
		"audio": json.RawMessage(`{"ext": ".sccga"}`),
		"image": json.RawMessage(`{"ext": ".sccgi"}`),
		"pdf":   json.RawMessage(`{"ext": ".sccgp"}`),
		"text":  json.RawMessage(`{"ext": ".sccgt"}`),
		"wps":   json.RawMessage(`{"ext": ".sccgw"}`),
	}

	fullSettings, err := plugins.BuildFullPluginSettings(userSettings)
	if err != nil {
		panic(fmt.Sprintf("构建插件配置失败: %v", err))
	}

	cfg := &config.Config{
		Password:       benchPluginPassword,
		PluginSettings: fullSettings,
	}
	return config.NewContext(context.Background(), cfg)
}

func findEncryptedFile(dir string) string {
	var found string
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := filepath.Ext(path)
		if len(ext) >= 5 && ext[:5] == ".sccg" {
			found = path
		}
		return nil
	})
	return found
}

func benchmarkPluginEncrypt(b *testing.B, p plugins.Plugin, ext string, data []byte, ctx context.Context) {
	b.Helper()

	srcDir := b.TempDir()
	srcPath := filepath.Join(srcDir, "testfile"+ext)
	if err := os.WriteFile(srcPath, data, 0644); err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		outputDir := b.TempDir()
		if _, err := plugins.EncryptFileWithPlugin(ctx, p, srcPath, srcDir, outputDir, nil); err != nil {
			b.Fatalf("加密失败 [%s]: %v", p.Name(), err)
		}
	}
}

func benchmarkPluginDecrypt(b *testing.B, p plugins.Plugin, ext string, data []byte, ctx context.Context) {
	b.Helper()

	srcDir := b.TempDir()
	srcPath := filepath.Join(srcDir, "testfile"+ext)
	if err := os.WriteFile(srcPath, data, 0644); err != nil {
		b.Fatal(err)
	}

	encryptDir := b.TempDir()
	if _, err := plugins.EncryptFileWithPlugin(ctx, p, srcPath, srcDir, encryptDir, nil); err != nil {
		b.Fatalf("预加密失败 [%s]: %v", p.Name(), err)
	}

	containerPath := findEncryptedFile(encryptDir)
	if containerPath == "" {
		b.Fatal("未找到加密后的容器文件")
	}

	dp, err := plugins.FindDecryptingPlugin(containerPath)
	if err != nil {
		b.Fatalf("查找解密插件失败: %v", err)
	}

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		decryptDir := b.TempDir()
		if _, err := plugins.DecryptContainerWithPlugin(ctx, dp, containerPath, decryptDir, nil); err != nil {
			b.Fatalf("解密失败 [%s]: %v", p.Name(), err)
		}
	}
}

func benchmarkPluginRoundTrip(b *testing.B, p plugins.Plugin, ext string, data []byte, ctx context.Context) {
	b.Helper()

	srcDir := b.TempDir()
	srcPath := filepath.Join(srcDir, "testfile"+ext)
	if err := os.WriteFile(srcPath, data, 0644); err != nil {
		b.Fatal(err)
	}

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		encryptDir := b.TempDir()
		decryptDir := b.TempDir()

		if _, err := plugins.EncryptFileWithPlugin(ctx, p, srcPath, srcDir, encryptDir, nil); err != nil {
			b.Fatalf("加密失败 [%s]: %v", p.Name(), err)
		}

		containerPath := findEncryptedFile(encryptDir)
		if containerPath == "" {
			b.Fatal("未找到加密后的容器文件")
		}

		dp, _ := plugins.FindDecryptingPlugin(containerPath)
		if _, err := plugins.DecryptContainerWithPlugin(ctx, dp, containerPath, decryptDir, nil); err != nil {
			b.Fatalf("解密失败 [%s]: %v", p.Name(), err)
		}
	}
}

var pluginBenchSizes = []struct {
	name string
	size int
}{
	{"1MB", 1 * 1024 * 1024},
}

func BenchmarkAllPlugins_Encrypt(b *testing.B) {
	pluginNames := []string{"video", "audio", "image", "pdf", "text", "wps"}

	ctx := makePluginBenchContext()
	if err := plugins.InitializePlugins(ctx); err != nil {
		b.Fatal(err)
	}

	for _, pname := range pluginNames {
		p, err := plugins.FindPluginByName(pname)
		if err != nil {
			b.Logf("跳过插件 %s: %v", pname, err)
			continue
		}

		exts := p.SupportedExtensions()
		if len(exts) == 0 {
			b.Logf("跳过插件 %s: 无支持的扩展名", pname)
			continue
		}
		ext := "." + exts[0]

		for _, sz := range pluginBenchSizes {
			data := generateRandomData(sz.size)
			b.Run(fmt.Sprintf("%s/%s/Encrypt", pname, sz.name), func(b *testing.B) {
				benchmarkPluginEncrypt(b, p, ext, data, ctx)
			})
		}
	}
}

func BenchmarkAllPlugins_Decrypt(b *testing.B) {
	pluginNames := []string{"video", "audio", "image", "pdf", "text", "wps"}

	ctx := makePluginBenchContext()
	if err := plugins.InitializePlugins(ctx); err != nil {
		b.Fatal(err)
	}

	for _, pname := range pluginNames {
		p, err := plugins.FindPluginByName(pname)
		if err != nil {
			b.Logf("跳过插件 %s: %v", pname, err)
			continue
		}

		exts := p.SupportedExtensions()
		if len(exts) == 0 {
			b.Logf("跳过插件 %s: 无支持的扩展名", pname)
			continue
		}
		ext := "." + exts[0]

		for _, sz := range pluginBenchSizes {
			data := generateRandomData(sz.size)
			b.Run(fmt.Sprintf("%s/%s/Decrypt", pname, sz.name), func(b *testing.B) {
				benchmarkPluginDecrypt(b, p, ext, data, ctx)
			})
		}
	}
}

func BenchmarkAllPlugins_RoundTrip(b *testing.B) {
	pluginNames := []string{"video", "audio", "image", "pdf", "text", "wps"}

	ctx := makePluginBenchContext()
	if err := plugins.InitializePlugins(ctx); err != nil {
		b.Fatal(err)
	}

	for _, pname := range pluginNames {
		p, err := plugins.FindPluginByName(pname)
		if err != nil {
			b.Logf("跳过插件 %s: %v", pname, err)
			continue
		}

		exts := p.SupportedExtensions()
		if len(exts) == 0 {
			b.Logf("跳过插件 %s: 无支持的扩展名", pname)
			continue
		}
		ext := "." + exts[0]

		for _, sz := range pluginBenchSizes {
			data := generateRandomData(sz.size)
			b.Run(fmt.Sprintf("%s/%s/RoundTrip", pname, sz.name), func(b *testing.B) {
				benchmarkPluginRoundTrip(b, p, ext, data, ctx)
			})
		}
	}
}

func BenchmarkCryptoCore_WrapUnwrapDEK(b *testing.B) {
	salt := make([]byte, 16)
	rand.Read(salt)
	password := "bench-test-password"
	dek := generateRandomData(16)

	kek := v2crypto.DeriveKEK(password, salt)
	aad := salt

	b.Run("WrapDEK", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_, err := v2crypto.WrapDEK(dek, kek, aad)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("UnwrapDEK", func(b *testing.B) {
		wd, err := v2crypto.WrapDEK(dek, kek, aad)
		if err != nil {
			b.Fatal(err)
		}

		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_, err := v2crypto.UnwrapDEK(wd, kek)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("DeriveKEK", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_ = v2crypto.DeriveKEK(password, salt)
		}
	})

	b.Run("GenerateDEK", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			_, err := v2crypto.GenerateDEK(v2crypto.KeySize_v4_128)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// --- alistencrypt 对比基准 ---

func BenchmarkAlistEncrypt_Compare(b *testing.B) {
	ctx := makePluginBenchContext()
	if err := plugins.InitializePlugins(ctx); err != nil {
		b.Fatal(err)
	}

	sizes := []struct {
		name string
		size int
	}{
		{"1MB", 1 * 1024 * 1024},
		{"10MB", 10 * 1024 * 1024},
	}

	alistPlugin, err := plugins.FindPluginByName("alist_encrypt")
	if err != nil {
		b.Fatalf("alist_encrypt 插件未找到: %v", err)
	}
	textPlugin, err := plugins.FindPluginByName("text")
	if err != nil {
		b.Fatalf("text 插件未找到: %v", err)
	}

	for _, sz := range sizes {
		data := generateRandomData(sz.size)

		b.Run(fmt.Sprintf("alistencrypt/%s/Encrypt", sz.name), func(b *testing.B) {
			srcDir := b.TempDir()
			srcPath := filepath.Join(srcDir, "testfile.txt")
			os.WriteFile(srcPath, data, 0644)

			b.SetBytes(int64(sz.size))
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				outputDir := b.TempDir()
				if _, err := plugins.EncryptFileWithPlugin(ctx, alistPlugin, srcPath, srcDir, outputDir, nil); err != nil {
					b.Fatalf("alist_encrypt 加密失败: %v", err)
				}
			}
		})

		b.Run(fmt.Sprintf("text_v4/%s/Encrypt", sz.name), func(b *testing.B) {
			srcDir := b.TempDir()
			srcPath := filepath.Join(srcDir, "testfile.txt")
			os.WriteFile(srcPath, data, 0644)

			b.SetBytes(int64(sz.size))
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				outputDir := b.TempDir()
				if _, err := plugins.EncryptFileWithPlugin(ctx, textPlugin, srcPath, srcDir, outputDir, nil); err != nil {
					b.Fatalf("text 加密失败: %v", err)
				}
			}
		})
	}
}

// --- 大文件等效模拟（零内存占用，使用零 Reader） ---
// 模拟 100G 文件的加密吞吐量，但不实际分配内存
// 用 io.LimitReader + 复用零 buffer 模拟大文件数据流

var zeroBufPool = make([]byte, 1024*1024) // 1MB 预清零 buffer 复用

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	n := len(p)
	if n > len(zeroBufPool) {
		n = len(zeroBufPool)
	}
	copy(p[:n], zeroBufPool[:n])
	return n, nil
}

func BenchmarkLargeFileSimulate_Encrypt(b *testing.B) {
	sizes := []struct {
		name string
		size int64
	}{
		{"1GB", 1 * 1024 * 1024 * 1024},
		{"10GB", 10 * 1024 * 1024 * 1024},
		{"100GB", 100 * 1024 * 1024 * 1024},
	}

	password := "bench-large-file-password"
	salt, _ := v2crypto.GenerateSalt_v2(16)
	iv, _ := v2crypto.GenerateIV_v2(16)
	kek := v2crypto.DeriveKEK(password, salt)
	dek, _ := v2crypto.GenerateDEK(v2crypto.KeySize_v4_128)

	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			b.SetBytes(sz.size)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				src := io.LimitReader(zeroReader{}, sz.size)
				err := v2crypto.EncryptStream_v2(src, io.Discard, dek, iv)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	_ = kek
}

func BenchmarkLargeFileSimulate_Decrypt(b *testing.B) {
	sizes := []struct {
		name string
		size int64
	}{
		{"1GB", 1 * 1024 * 1024 * 1024},
		{"10GB", 10 * 1024 * 1024 * 1024},
		{"100GB", 100 * 1024 * 1024 * 1024},
	}

	password := "bench-large-file-password"
	salt, _ := v2crypto.GenerateSalt_v2(16)
	iv, _ := v2crypto.GenerateIV_v2(16)
	kek := v2crypto.DeriveKEK(password, salt)
	dek, _ := v2crypto.GenerateDEK(v2crypto.KeySize_v4_128)

	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			b.SetBytes(sz.size)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				src := io.LimitReader(zeroReader{}, sz.size)
				err := v2crypto.DecryptStream_v2(src, io.Discard, dek, iv)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	_ = kek
}

// --- 物理分片大文件模拟 ---

func BenchmarkPhysicalChunking_LargeFile(b *testing.B) {
	chunkSizes := []struct {
		name string
		size int64
	}{
		{"512MB", 512 * 1024 * 1024},
		{"1GB", 1 * 1024 * 1024 * 1024},
		{"2GB", 2 * 1024 * 1024 * 1024},
	}

	fileSizes := []struct {
		name string
		size int64
	}{
		{"10GB", 10 * 1024 * 1024 * 1024},
		{"100GB", 100 * 1024 * 1024 * 1024},
	}

	for _, fsz := range fileSizes {
		for _, csz := range chunkSizes {
			chunkCount := fsz.size / csz.size
			if chunkCount == 0 {
				continue
			}

			b.Run(fmt.Sprintf("%s/chunk_%s/%d_chunks", fsz.name, csz.name, chunkCount), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()

				for b.Loop() {
					// 模拟分片索引构建开销（纯计算，无 I/O）
					_ = simulateChunkIndex(fsz.size, csz.size)
				}
			})
		}
	}
}

func simulateChunkIndex(fileSize, chunkSize int64) []uint64 {
	chunkCount := fileSize / chunkSize
	if fileSize%chunkSize != 0 {
		chunkCount++
	}
	offsets := make([]uint64, chunkCount)
	for i := range offsets {
		offsets[i] = uint64(i) * uint64(chunkSize)
	}
	return offsets
}

// --- 加密核心吞吐量对比（内存到内存，无 I/O 干扰） ---

func BenchmarkCryptoCore_Throughput(b *testing.B) {
	sizes := []struct {
		name string
		size int
	}{
		{"64KB", 64 * 1024},
		{"1MB", 1 * 1024 * 1024},
		{"16MB", 16 * 1024 * 1024},
	}

	password := "bench-password"
	salt, _ := v2crypto.GenerateSalt_v2(16)
	iv, _ := v2crypto.GenerateIV_v2(16)
	key128 := v2crypto.GenerateKey_v4(password, salt, 16)
	key256 := v2crypto.GenerateKey_v4(password, salt, 32)

	for _, sz := range sizes {
		plaintext := make([]byte, sz.size)
		rand.Read(plaintext)

		b.Run(fmt.Sprintf("AES128-CTR/Encrypt/%s", sz.name), func(b *testing.B) {
			b.SetBytes(int64(sz.size))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, _ = v2crypto.EncryptBytes_v2(plaintext, key128, iv)
			}
		})

		b.Run(fmt.Sprintf("AES256-CTR/Encrypt/%s", sz.name), func(b *testing.B) {
			b.SetBytes(int64(sz.size))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, _ = v2crypto.EncryptBytes_v2(plaintext, key256, iv)
			}
		})

		b.Run(fmt.Sprintf("AES128-CTR/StreamEncrypt/%s", sz.name), func(b *testing.B) {
			b.SetBytes(int64(sz.size))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				src := bytes.NewReader(plaintext)
				_ = v2crypto.EncryptStream_v2(src, io.Discard, key128, iv)
			}
		})
	}
}
