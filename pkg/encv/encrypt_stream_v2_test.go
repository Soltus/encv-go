package encv_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	"github.com/Soltus/encv-go/internal/v2/plugins/text"
	"github.com/Soltus/encv-go/pkg/encv"
)

const streamPassword = "stream-password"

// streamCtx 复刻主应用启动时的配置上下文（口令 + 插件设置）。
func streamCtx() context.Context {
	return config.NewContext(context.Background(), &config.Config{
		Password:       streamPassword,
		PluginSettings: map[string]json.RawMessage{"text": json.RawMessage(`{"ext":".sccgt"}`)},
	})
}

// TestEncryptFileStreamV2_RoundTripByPlugin 流式加密的产物必须能被**插件路径**解开。
//
// 这是"反哺"的验收标准：wasm 与 CLI/Capacitor 后端共用 compose 编排，
// 产物不仅要"能解"，还要能被**对应插件**认（否则解密时报 index missing）。
func TestEncryptFileStreamV2_RoundTripByPlugin(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int
	}{
		{"小文件单段", 1234},
		{"多段（跨默认 4MB 段）", 5 << 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			plain := bytes.Repeat([]byte("反哺主应用 "), tc.size/16+1)
			src := filepath.Join(dir, "note.txt")
			if err := os.WriteFile(src, plain, 0o600); err != nil {
				t.Fatalf("写明文：%v", err)
			}
			container := filepath.Join(dir, "note.txt.sccgt")

			if err := encv.EncryptFileStreamV2(context.Background(), src, container, streamPassword); err != nil {
				t.Fatalf("流式加密失败：%v", err)
			}

			// 容器头必须就位（否则 open 阶段就废了）
			raw, err := os.ReadFile(container)
			if err != nil {
				t.Fatalf("读容器：%v", err)
			}
			if len(raw) <= len(plain) {
				t.Fatalf("容器异常小：%d（明文 %d）", len(raw), len(plain))
			}

			ctx := streamCtx()
			p := new(text.TextPlugin)
			if err := p.Initialize(ctx); err != nil {
				t.Fatalf("初始化 text 插件：%v", err)
			}
			outDir := filepath.Join(dir, "out")
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				t.Fatalf("MkdirAll：%v", err)
			}
			gotPath, err := plugins.DecryptContainerWithPlugin(ctx, p, container, outDir, nil)
			if err != nil {
				t.Fatalf("插件解不开流式产物：%v", err)
			}
			got, err := os.ReadFile(gotPath)
			if err != nil {
				t.Fatalf("读解密结果：%v", err)
			}
			if !bytes.Equal(got, plain) {
				t.Errorf("往返不一致：解出 %d 字节，原文 %d 字节", len(got), len(plain))
			}
		})
	}
}

// TestEncryptFileStreamV2_RejectsEmptyPassword 空口令要早失败，而不是写出一个废容器。
func TestEncryptFileStreamV2_RejectsEmptyPassword(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(src, []byte("x"), 0o600); err != nil {
		t.Fatalf("写明文：%v", err)
	}
	if err := encv.EncryptFileStreamV2(context.Background(), src, filepath.Join(dir, "a.sccgt"), ""); err == nil {
		t.Errorf("空口令应当报错")
	}
}
