package i18n

import (
	"log"
	"testing"
)

func TestDemo_LogMessages(t *testing.T) {
	t.Log("=== i18n 日志输出演示 ===")
	t.Log()

	original := GetLocale()
	defer SetLocale(original)

	t.Logf("当前语言: %s", GetLocale())
	t.Logf("可用语言: %v", AvailableLocales())
	t.Log()

	t.Log("【中文】")
	SetLocale("zh-CN")
	t.Logf("  取消: %s", T("common.cancel"))
	t.Logf("  确认: %s", T("common.confirm"))
	t.Logf("  设置: %s", T("common.settings"))
	t.Log()

	t.Log("【英文】")
	SetLocale("en")
	t.Logf("  取消: %s", T("common.cancel"))
	t.Logf("  确认: %s", T("common.confirm"))
	t.Logf("  设置: %s", T("common.settings"))
	t.Log()

	t.Log("=== 演示结束 ===")
}

func BenchmarkT(b *testing.B) {
	SetLocale("zh-CN")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = T("common.cancel")
	}
}

func BenchmarkT_Missing(b *testing.B) {
	SetLocale("zh-CN")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = T("nonexistent.key.benchmark")
	}
}

func ExampleT() {
	SetLocale("zh-CN")
	log.Printf("操作: %s", T("common.cancel"))
}
