package plugins

import (
	"encoding/json"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/pluginsext"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

// TestPluginExts_SyncWithDefaults 强制约束 pluginsext 常量与 plugin 源码默认同步。
//
// 任何 plugin 源码默认 Ext 变更都会让本测试失败（CI 红灯），
// 强制要求同时更新 pluginsext 常量。
//
// 这是"权威来源是 plugin 源码"原则的强制保证：
// - plugin.go 的 GetDefaultSettings() 返回值 = 权威
// - pluginsext 常量 = 镜像（必须与权威一致）
// - 任何不一致 = 测试失败
//
// 工作原理：直接读取每个 plugin 的 GetDefaultSettings() JSON，提取 Ext 字段，
// 然后断言它 == pluginsext 中对应的常量。这绕过了 InitializePlugins 的
// config 依赖，直接从 plugin 源码（权威）读取默认值。
func TestPluginExts_SyncWithDefaults(t *testing.T) {
	cases := []struct {
		pluginName string
		constant   string
	}{
		{"video", pluginsext.VideoExt},
		{"audio", pluginsext.AudioExt},
		{"image", pluginsext.ImageExt},
		{"text", pluginsext.TextExt},
		{"pdf", pluginsext.PdfExt},
		{"wps", pluginsext.WpsExt},
		{"alist_encrypt", pluginsext.AlistExt},
	}

	for _, c := range cases {
		p, err := FindPluginByName(c.pluginName)
		if err != nil {
			t.Errorf("plugin %q not found: %v", c.pluginName, err)
			continue
		}
		// 直接从 plugin 源码默认值读取
		defaults := p.GetDefaultSettings()
		// 尝试以通用 map 提取 Ext 字段（部分 plugin 用 "ext"，部分用 "suffix"）
		var extMap map[string]any
		if err := json.Unmarshal(defaults, &extMap); err != nil {
			t.Errorf("plugin %q GetDefaultSettings() invalid JSON: %v", c.pluginName, err)
			continue
		}
		var ext string
		if v, ok := extMap["ext"].(string); ok {
			ext = v
		} else if v, ok := extMap["suffix"].(string); ok {
			ext = v
		} else {
			t.Errorf("plugin %q GetDefaultSettings() missing 'ext' or 'suffix' string field, got: %v", c.pluginName, extMap)
			continue
		}
		if ext != c.constant {
			t.Errorf("pluginsext constant for %q is %q, but plugin.GetDefaultSettings() returns ext=%q. "+
				"plugin source (plugin.go) is authoritative — update pluginsext/pluginsext.go to match.",
				c.pluginName, c.constant, ext)
		}
	}
}
