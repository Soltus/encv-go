package server

// bundle_filename_test.go —— 仓库文件名 → (包名, 版本) 的切分契约
//
// 两类真实文件名必须同时对（任何一边错了都会变成"云端找不到包 / 设备拉包 404"）：
//   ① 版本带连字符：web-v0.0.1-test      ⇒ web / v0.0.1-test
//   ② 包名带连字符：go-binary-v0.0.5-logs ⇒ go-binary / v0.0.5-logs   ← 2026-10-05 抓到
//
// ② 的失败方式很隐蔽：按第一个 '-' 切会变成 name="go"，清单里只有 "go"，
// 于是 `push{name:"go-binary"}` 一律 bundle_not_found —— 云控"看起来坏了"，
// 实际是清单把包名切错了。

import "testing"

func TestSplitBundleFileName(t *testing.T) {
	cases := []struct {
		base    string
		name    string
		version string
		ok      bool
	}{
		// 包名不含 '-'（最常见的资源包）
		{"web-v0.0.7-devshell", "web", "v0.0.7-devshell", true},
		{"web-v0.0.1-test", "web", "v0.0.1-test", true},
		{"preview-assets-v2", "preview-assets", "v2", true},
		// ⚠️ 包名本身带 '-'（Go 二进制热更就是这个形态）
		{"go-binary-v0.0.5-logs", "go-binary", "v0.0.5-logs", true},
		{"go-binary-v1", "go-binary", "v1", true},
		// 版本不以 v 开头、以数字开头
		{"preview-assets-1.2.3", "preview-assets", "1.2.3", true},
		{"web-2024.10.05", "web", "2024.10.05", true},
		// 兜底与非法
		{"weird-v1", "weird", "v1", true},
		{"noversion", "", "", false},
		{"", "", "", false},
	}
	for _, c := range cases {
		name, version, ok := splitBundleFileName(c.base)
		if ok != c.ok || name != c.name || version != c.version {
			t.Errorf("splitBundleFileName(%q) = (%q,%q,%v), want (%q,%q,%v)",
				c.base, name, version, ok, c.name, c.version, c.ok)
		}
	}
}

// TestBundleManifest_ListsGoBinary —— 端到端：仓库里有 go-binary-v0.0.5-logs.zip
// 时，清单必须给出 name=go-binary（否则云控 push 会找不到它）
func TestBundleManifest_ListsGoBinary(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENCV_BUNDLES_DIR", dir)
	writeTestBundle(t, dir, "go-binary", "v0.0.5-logs", map[string]string{"encv-go": "binary"})

	var found bool
	for _, it := range loadBundleManifest() {
		if it.Name == "go-binary" && it.Version == "v0.0.5-logs" {
			found = true
		}
		if it.Name == "go" {
			t.Fatalf("包名不得被切成 %q（应为 go-binary）: %+v", it.Name, it)
		}
	}
	if !found {
		t.Fatalf("清单未列出 go-binary@v0.0.5-logs: %+v", loadBundleManifest())
	}
}
