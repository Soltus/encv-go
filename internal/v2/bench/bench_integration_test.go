//go:build integration

package bench_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	"github.com/Soltus/encv-go/internal/v2/reader"
)

func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func findTestVideos(b *testing.B) []string {
	b.Helper()

	root := findProjectRoot()
	if root == "" {
		b.Skip("跳过：无法找到项目根目录（缺少 go.mod）")
	}

	absDir := filepath.Join(root, "_videos")
	entries, err := os.ReadDir(absDir)
	if err != nil {
		b.Skipf("跳过：无法读取测试视频目录 %s: %v", absDir, err)
	}

	var videos []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		for _, p := range plugins.Plugins {
			for _, supported := range p.SupportedExtensions() {
				if ext == "."+supported {
					videos = append(videos, filepath.Join(absDir, e.Name()))
					break
				}
			}
		}
	}

	if len(videos) == 0 {
		b.Skipf("跳过：在 %s 中未找到支持的视频文件", absDir)
	}

	return videos
}

func makeBenchContext(b *testing.B) context.Context {
	b.Helper()

	userSettings := map[string]json.RawMessage{
		"video": json.RawMessage(`{
			"ext": ".sccgv",
			"chunk_size_mb": 0,
			"light_main_chunk_enabled": true,
			"verify_after_pack": false,
			"track_extensions": ".ass,.srt,.dm.ass,.vtt",
			"plugin_cache_dir": ""
		}`),
	}

	fullSettings, err := plugins.BuildFullPluginSettings(userSettings)
	if err != nil {
		b.Fatalf("构建插件配置失败: %v", err)
	}

	cfg := &config.Config{
		Password:       "bench-integration-password",
		PluginSettings: fullSettings,
	}
	return config.NewContext(context.Background(), cfg)
}

func findContainerFile(dir string) string {
	var containerPath string
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if filepath.Ext(path) == ".sccgv" {
			containerPath = path
		}
		return nil
	})
	return containerPath
}

func BenchmarkVideoPlugin_Encrypt(b *testing.B) {
	videos := findTestVideos(b)

	for _, videoPath := range videos {
		info, _ := os.Stat(videoPath)
		b.Run(filepath.Base(videoPath), func(b *testing.B) {
			outputDir := b.TempDir()
			ctx := makeBenchContext(b)

			if err := plugins.InitializePlugins(ctx); err != nil {
				b.Fatal(err)
			}

			p, err := plugins.FindEncryptingPlugin(videoPath)
			if err != nil {
				b.Fatal(err)
			}

			b.SetBytes(info.Size())
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				if _, err := plugins.EncryptFileWithPlugin(ctx, p, videoPath, filepath.Dir(videoPath), outputDir, nil); err != nil {
					b.Fatalf("加密失败: %v", err)
				}
				os.RemoveAll(outputDir)
				os.MkdirAll(outputDir, 0755)
			}
		})
	}
}

func BenchmarkVideoPlugin_Decrypt(b *testing.B) {
	videos := findTestVideos(b)

	for _, videoPath := range videos {
		b.Run(filepath.Base(videoPath), func(b *testing.B) {
			encryptDir := b.TempDir()
			ctx := makeBenchContext(b)

			if err := plugins.InitializePlugins(ctx); err != nil {
				b.Fatal(err)
			}

			p, err := plugins.FindEncryptingPlugin(videoPath)
			if err != nil {
				b.Fatal(err)
			}

			if _, err := plugins.EncryptFileWithPlugin(ctx, p, videoPath, filepath.Dir(videoPath), encryptDir, nil); err != nil {
				b.Fatalf("加密失败: %v", err)
			}

			containerPath := findContainerFile(encryptDir)
			if containerPath == "" {
				b.Fatal("未找到加密后的容器文件")
			}

			info, _ := os.Stat(videoPath)
			b.SetBytes(info.Size())
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				decryptDir := b.TempDir()
				dp, _ := plugins.FindDecryptingPlugin(containerPath)
				if _, err := plugins.DecryptContainerWithPlugin(ctx, dp, containerPath, decryptDir, nil); err != nil {
					b.Fatal(err)
				}
				os.RemoveAll(decryptDir)
			}
		})
	}
}

func BenchmarkVideoPlugin_FullRoundTrip(b *testing.B) {
	videos := findTestVideos(b)

	for _, videoPath := range videos {
		info, _ := os.Stat(videoPath)
		b.Run(filepath.Base(videoPath), func(b *testing.B) {
			ctx := makeBenchContext(b)

			if err := plugins.InitializePlugins(ctx); err != nil {
				b.Fatal(err)
			}

			p, err := plugins.FindEncryptingPlugin(videoPath)
			if err != nil {
				b.Fatal(err)
			}

			b.SetBytes(info.Size())
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				encryptDir := b.TempDir()
				decryptDir := b.TempDir()

				if _, err := plugins.EncryptFileWithPlugin(ctx, p, videoPath, filepath.Dir(videoPath), encryptDir, nil); err != nil {
					b.Fatal(err)
				}

				containerPath := findContainerFile(encryptDir)
				dp, _ := plugins.FindDecryptingPlugin(containerPath)
				if _, err := plugins.DecryptContainerWithPlugin(ctx, dp, containerPath, decryptDir, nil); err != nil {
					b.Fatal(err)
				}

				os.RemoveAll(encryptDir)
				os.RemoveAll(decryptDir)
			}
		})
	}
}

func BenchmarkVideoPlugin_SeekableStream(b *testing.B) {
	videos := findTestVideos(b)

	for _, videoPath := range videos {
		b.Run(filepath.Base(videoPath), func(b *testing.B) {
			encryptDir := b.TempDir()
			ctx := makeBenchContext(b)

			if err := plugins.InitializePlugins(ctx); err != nil {
				b.Fatal(err)
			}

			p, err := plugins.FindEncryptingPlugin(videoPath)
			if err != nil {
				b.Fatal(err)
			}

			if _, err := plugins.EncryptFileWithPlugin(ctx, p, videoPath, filepath.Dir(videoPath), encryptDir, nil); err != nil {
				b.Fatalf("加密失败: %v", err)
			}

			containerPath := findContainerFile(encryptDir)
			if containerPath == "" {
				b.Fatal("未找到加密后的容器文件")
			}

			info, _ := os.Stat(videoPath)
			b.SetBytes(info.Size())
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				factory, err := reader.NewDecryptReaderFactory(containerPath, "bench-integration-password")
				if err != nil {
					b.Fatal(err)
				}
				r, err := factory.NewDecryptReader()
				if err != nil {
					b.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, r)
				r.Close()
				factory.Close()
			}
		})
	}
}
