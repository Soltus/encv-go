package bundle

// applyfile_required_test.go —— 换执行体时 Required 缺失**必须报错而不是 panic**
//
// 真机事故（2026-10-05）：云控下发 go-binary 一直 `504 peer_timeout`。
//   排查时最像"包太大/网络慢"，实际是 ApplyFile 直接取 `spec.Required[0]`，
//   而 Hub 侧下发**从不带** Required ⇒ 空数组越界 ⇒ 执行端 goroutine panic
//   ⇒ RPC 永远等不到回包 ⇒ 云端只看到超时，看不到 panic（设备上也不好查）。
//
// 契约：
//   1. Required 为空 ⇒ 返回 error，**绝不 panic**；
//   2. 给了 Required 时行为不变（既有用例覆盖）。

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyFile_EmptyRequiredReturnsErrorNotPanic(t *testing.T) {
	dir := t.TempDir()
	zipPath, _ := buildZip(t, map[string]string{"encv-go": "fake-binary"})
	target := filepath.Join(dir, "out", "encv-go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}

	// 旧实现在这里 panic（Required[0]），gomaxprocs 下整个进程可能挂掉
	_, err := ApplyFile(FileSpec{
		Name:    "go-binary",
		Target:  target,
		Version: "v1",
		SHA256:  "",
	}, zipPath, Options{})
	if err == nil {
		t.Fatal("Required 为空必须返回错误（否则执行端会 panic ⇒ RPC 永不回包）")
	}
}

func TestApplyFile_WithRequiredStillWorks(t *testing.T) {
	dir := t.TempDir()
	zipPath, _ := buildZip(t, map[string]string{"encv-go": "fake-binary"})
	target := filepath.Join(dir, "out", "encv-go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := ApplyFile(FileSpec{
		Name:     "go-binary",
		Target:   target,
		Version:  "v1",
		Required: []string{"encv-go"},
	}, zipPath, Options{})
	if err != nil {
		t.Fatalf("带 Required 时应正常安装: %v", err)
	}
	if res.Version != "v1" {
		t.Fatalf("version 不对: %+v", res)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("目标文件应存在: %v", err)
	}
}
