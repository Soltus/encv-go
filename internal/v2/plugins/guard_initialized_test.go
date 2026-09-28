package plugins

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/v2/plugins/text"
)

// TestGuardInitialized_NoPanic 锁住"没 Initialize 就加解密"的表现。
//
// 背景：插件的口令来自 p.cfg，而 cfg 只在 Initialize(ctx) 里赋值。没初始化就调
// Encrypt/Decrypt，取 p.cfg.Password 是一次**空指针 panic**。EncryptFileWithPlugin
// 里那层 recover 会把 panic 吞成空结果（静默失败），DecryptContainerWithPlugin 则
// 直接把进程带崩 —— 两种情况都比一条明确错误难排查得多。
//
// 所以这里断言的是"**不 panic 且有明确错误**"，不是具体的错误文案。
func TestGuardInitialized_NoPanic(t *testing.T) {
	// ctx 带配置，但插件**没有** Initialize
	ctx := config.NewContext(context.Background(), &config.Config{
		Password:       "x",
		PluginSettings: map[string]json.RawMessage{},
	})
	p := new(text.TextPlugin)

	if p.Initialized() {
		t.Fatalf("刚 new 出来的插件不应是已初始化状态")
	}

	// 解密侧：不得 panic
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("未初始化的插件调用解密发生了 panic：%v", r)
			}
		}()
		out, err := DecryptContainerWithPlugin(ctx, p, "/nonexistent.sccgt", t.TempDir(), nil)
		if err == nil {
			t.Errorf("未初始化的插件解密应返回错误，实际返回 out=%q", out)
			return
		}
		if !strings.Contains(err.Error(), "尚未初始化") {
			t.Errorf("错误信息应点明\"尚未初始化\"，实际：%v", err)
		}
	}()

	// 加密侧同样要守（EncryptFileWithPlugin 内部有 recover，panic 会被吞成空结果）
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("未初始化的插件调用加密发生了 panic：%v", r)
			}
		}()
		out, err := EncryptFileWithPlugin(ctx, p, "/nonexistent.txt", t.TempDir(), t.TempDir(), nil)
		if err == nil {
			t.Errorf("未初始化的插件加密应返回错误，实际返回 out=%q", out)
			return
		}
		if !strings.Contains(err.Error(), "尚未初始化") {
			t.Errorf("错误信息应点明\"尚未初始化\"，实际：%v", err)
		}
	}()
}

// TestInitializeRequiresConfigInContext Initialize 必须拒绝"没带配置的 ctx"：
// config.FromContext 在缺失时返回 nil，照原样赋给 p.cfg 就等于埋一颗后面才炸的雷。
func TestInitializeRequiresConfigInContext(t *testing.T) {
	p := new(text.TextPlugin)
	err := p.Initialize(context.Background()) // 故意不带 config
	if err == nil {
		t.Fatalf("Initialize(context.Background()) 应报错：ctx 里没有 config")
	}
	if p.Initialized() {
		t.Errorf("Initialize 失败后不应被标记为已初始化")
	}
}
