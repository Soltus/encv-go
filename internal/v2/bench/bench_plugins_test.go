package bench_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
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
