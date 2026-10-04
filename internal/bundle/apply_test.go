package bundle

// apply_test.go —— 资源包安装器的回归锁（2026-10-04）
//
// 锁的是四条不变式（任何一条破了，热更新就会把设备搞成白屏或存储打满）：
//  1. 先 staging 后切换 ⇒ 目标目录在切换前始终可用
//  2. hash 不符 ⇒ **完全不碰**目标目录
//  3. 缺必含文件 ⇒ 不切换，旧版继续服务
//  4. 切换失败 ⇒ 能回滚到上一版

import (
	"archive/zip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildZip 造一个内存 zip 并返回其路径与 sha256。
func buildZip(t *testing.T, entries map[string]string) (path string, sha string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "bundle.zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatalf("创建 zip 失败: %v", err)
	}
	zw := zip.NewWriter(f)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip entry 失败: %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write 失败: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close 失败: %v", err)
	}
	_ = f.Close()
	sha, err = SHA256File(p)
	if err != nil {
		t.Fatalf("计算 sha256 失败: %v", err)
	}
	return p, sha
}

func TestApply_OK_WritesVersion(t *testing.T) {
	zipPath, sha := buildZip(t, map[string]string{
		"index.html": "<html>v1</html>",
		"main.js":    "console.log('v1')",
	})
	target := filepath.Join(t.TempDir(), "web")

	res, err := Apply(Spec{
		Name:      "web",
		TargetDir: target,
		Version:   "v1",
		SHA256:    sha,
		Required:  []string{"index.html", "main.js"},
	}, zipPath, Options{})
	if err != nil {
		t.Fatalf("Apply 失败: %v", err)
	}
	if res.Version != "v1" || res.AppliedAt == "" {
		t.Fatalf("Result 形态不对: %+v", res)
	}
	b, err := os.ReadFile(filepath.Join(target, "index.html"))
	if err != nil || string(b) != "<html>v1</html>" {
		t.Fatalf("目标目录内容不对: %q err=%v", string(b), err)
	}
	if got := CurrentVersion(target); got != "v1" {
		t.Fatalf("version.json = %q, want v1", got)
	}
	// staging 必须已清理（不能留垃圾）
	if _, err := os.Stat(filepath.Join(filepath.Dir(target), "web-staging")); !os.IsNotExist(err) {
		t.Fatal("staging 目录应已清理")
	}
}

// TestApply_Upgrade_Then_Rollback —— 升级后回滚，旧版必须完整回来
func TestApply_Upgrade_Then_Rollback(t *testing.T) {
	z1, s1 := buildZip(t, map[string]string{"index.html": "v1", "main.js": "1"})
	z2, s2 := buildZip(t, map[string]string{"index.html": "v2", "main.js": "2"})
	target := filepath.Join(t.TempDir(), "web")

	if _, err := Apply(Spec{Name: "web", TargetDir: target, Version: "v1", SHA256: s1, Required: []string{"index.html"}}, z1, Options{}); err != nil {
		t.Fatalf("装 v1 失败: %v", err)
	}
	res2, err := Apply(Spec{Name: "web", TargetDir: target, Version: "v2", SHA256: s2, Required: []string{"index.html"}}, z2, Options{})
	if err != nil {
		t.Fatalf("装 v2 失败: %v", err)
	}
	if res2.PreviousVersion != "v1" {
		t.Fatalf("应记录上一版 v1, got %q", res2.PreviousVersion)
	}
	b, _ := os.ReadFile(filepath.Join(target, "index.html"))
	if string(b) != "v2" {
		t.Fatalf("升级后应为 v2, got %q", string(b))
	}

	// 回滚（模拟"新包有问题，云控要求退回"）
	if err := Rollback("web", target); err != nil {
		t.Fatalf("Rollback 失败: %v", err)
	}
	b, _ = os.ReadFile(filepath.Join(target, "index.html"))
	if string(b) != "v1" {
		t.Fatalf("回滚后应回到 v1, got %q", string(b))
	}
	if got := CurrentVersion(target); got != "v1" {
		t.Fatalf("回滚后版本应为 v1, got %q", got)
	}
}

// TestApply_ChecksumMismatch_KeepsOld —— hash 不符：目标目录必须一字不动
func TestApply_ChecksumMismatch_KeepsOld(t *testing.T) {
	z1, s1 := buildZip(t, map[string]string{"index.html": "v1"})
	target := filepath.Join(t.TempDir(), "web")
	if _, err := Apply(Spec{Name: "web", TargetDir: target, Version: "v1", SHA256: s1, Required: []string{"index.html"}}, z1, Options{}); err != nil {
		t.Fatalf("装 v1 失败: %v", err)
	}

	// 换一个包，但谎报 v1 的摘要
	z2, _ := buildZip(t, map[string]string{"index.html": "tampered"})
	if _, err := Apply(Spec{Name: "web", TargetDir: target, Version: "v2", SHA256: s1, Required: []string{"index.html"}}, z2, Options{}); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("应报 ErrChecksumMismatch, got %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(target, "index.html"))
	if string(b) != "v1" {
		t.Fatalf("摘要失败后旧版必须完好, got %q", string(b))
	}
	if got := CurrentVersion(target); got != "v1" {
		t.Fatalf("摘要失败后版本应仍是 v1, got %q", got)
	}
}

// TestApply_MissingRequired_KeepsOld —— 包能解开但内容不全 ⇒ 不切换（切进去就是白屏）
func TestApply_MissingRequired_KeepsOld(t *testing.T) {
	z1, s1 := buildZip(t, map[string]string{"index.html": "v1", "main.js": "1"})
	target := filepath.Join(t.TempDir(), "web")
	if _, err := Apply(Spec{Name: "web", TargetDir: target, Version: "v1", SHA256: s1, Required: []string{"index.html"}}, z1, Options{}); err != nil {
		t.Fatalf("装 v1 失败: %v", err)
	}

	// 新包缺 main.js（外层还套了一层目录，顺便验证"剥单层根目录"）
	z2, s2 := buildZip(t, map[string]string{"dist/index.html": "v2"})
	if _, err := Apply(Spec{Name: "web", TargetDir: target, Version: "v2", SHA256: s2,
		Required: []string{"index.html", "main.js"}}, z2, Options{}); err == nil {
		t.Fatal("缺必含文件应报错")
	}
	b, _ := os.ReadFile(filepath.Join(target, "index.html"))
	if string(b) != "v1" {
		t.Fatalf("缺文件后旧版必须完好, got %q", string(b))
	}
}

// TestApply_ZipSlip —— 恶意 zip 不得写穿目标目录
func TestApply_ZipSlip(t *testing.T) {
	z, s := buildZip(t, map[string]string{"../evil.txt": "pwned"})
	parent := t.TempDir()
	target := filepath.Join(parent, "web")

	if _, err := Apply(Spec{Name: "web", TargetDir: target, Version: "v1", SHA256: s}, z, Options{}); err == nil {
		t.Fatal("Zip-Slip 应被拒绝")
	}
	if _, err := os.Stat(filepath.Join(parent, "evil.txt")); !os.IsNotExist(err) {
		t.Fatal("evil.txt 被写到目标目录之外 —— 路径穿越防护失效")
	}
}

// TestDownload_TooLarge —— 超限必须中止并删掉半截文件（否则会占满设备存储）
func TestDownload_TooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat("a", 64*1024)))
		for i := 0; i < 100; i++ {
			if _, err := w.Write([]byte(strings.Repeat("b", 64*1024))); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "dl.zip")
	err := Download(context.Background(), srv.URL, dest, 64*1024, 2*time.Second)
	if err == nil {
		t.Fatal("超出上限应报错")
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatal("超限后必须删掉半截文件（否则残留会占空间、也可能被当完整包用）")
	}
}

// TestDownload_OK —— 正常下载落盘并可算摘要
func TestDownload_OK(t *testing.T) {
	body := strings.Repeat("x", 4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "dl.zip")
	if err := Download(context.Background(), srv.URL, dest, 1<<20, 2*time.Second); err != nil {
		t.Fatalf("Download 失败: %v", err)
	}
	st, err := os.Stat(dest)
	if err != nil || st.Size() != int64(len(body)) {
		t.Fatalf("下载大小不对: %v", err)
	}
}
