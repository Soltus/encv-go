package bundle

// apply_file_test.go —— 单文件（执行体）原子替换的回归锁（I3，2026-10-05）
//
// 换的是 **Go 二进制本体**：换坏了设备就起不来后端，而后端挂了连远程调试都救不回来。
// 因此这里锁的四条比目录型更硬：
//  1. 摘要不符 ⇒ 一个字节都不动目标文件
//  2. 缺必含文件 ⇒ 不切换
//  3. 切换后必须保留上一版备份，且 RollbackFile 能把它换回来
//  4. sidecar（version/abi）必须写：Kotlin 侧靠它识别"这是热更放的"并校验架构

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApplyFile_AtomicReplace_And_Sidecar(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "encv-go")
	// 旧版（模拟 APK 内正在跑的二进制）
	writeFile(t, target, "OLD-BINARY")

	src := t.TempDir()
	writeFile(t, filepath.Join(src, "encv-go"), "NEW-BINARY")
	zipPath, sha := buildZip(t, map[string]string{"encv-go": "NEW-BINARY"})
	_ = src

	res, err := ApplyFile(FileSpec{
		Name:     "go-binary",
		Target:   target,
		Version:  "v-hot-1",
		ABI:      "arm64-v8a",
		SHA256:   sha,
		Required: []string{"encv-go"},
	}, zipPath, Options{})
	if err != nil {
		t.Fatalf("ApplyFile 失败: %v", err)
	}
	if res.BackupDir == "" {
		t.Fatal("换执行体必须留备份（否则无法回滚）")
	}

	got, err := os.ReadFile(target)
	if err != nil || string(got) != "NEW-BINARY" {
		t.Fatalf("目标文件未替换: %q err=%v", string(got), err)
	}
	// 权限必须是可执行的
	st, _ := os.Stat(target)
	if st.Mode().Perm()&0o111 == 0 {
		t.Fatalf("热更二进制必须可执行, got %v", st.Mode().Perm())
	}
	// sidecar：Kotlin 侧靠它判定来源与架构
	if v := readFileOrEmpty(target + ".version"); v != "v-hot-1" {
		t.Fatalf("version sidecar = %q", v)
	}
	if a := readFileOrEmpty(target + ".abi"); a != "arm64-v8a" {
		t.Fatalf("abi sidecar = %q", a)
	}

	// 回滚：旧二进制必须完整回来
	if err := RollbackFile("go-binary", target); err != nil {
		t.Fatalf("RollbackFile 失败: %v", err)
	}
	got, _ = os.ReadFile(target)
	if string(got) != "OLD-BINARY" {
		t.Fatalf("回滚后应恢复旧二进制, got %q", string(got))
	}
}

func TestApplyFile_ChecksumMismatch_KeepsRunningBinary(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "encv-go")
	writeFile(t, target, "OLD-BINARY")

	zipPath, _ := buildZip(t, map[string]string{"encv-go": "TAMPERED"})
	_, err := ApplyFile(FileSpec{
		Name:     "go-binary",
		Target:   target,
		Version:  "v-hot-2",
		ABI:      "arm64-v8a",
		SHA256:   strings.Repeat("0", 64),
		Required: []string{"encv-go"},
	}, zipPath, Options{})
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("应报 ErrChecksumMismatch, got %v", err)
	}
	// ⚠️ 关键：正在运行的二进制必须一字不动（动了就起不来后端）
	got, _ := os.ReadFile(target)
	if string(got) != "OLD-BINARY" {
		t.Fatalf("摘要失败后目标二进制必须完好, got %q", string(got))
	}
}

func TestApplyFile_MissingBinary_KeepsOld(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "encv-go")
	writeFile(t, target, "OLD-BINARY")

	// zip 里没有可执行文件
	zipPath, sha := buildZip(t, map[string]string{"README": "not a binary"})
	if _, err := ApplyFile(FileSpec{
		Name:     "go-binary",
		Target:   target,
		Version:  "v-hot-3",
		ABI:      "arm64-v8a",
		SHA256:   sha,
		Required: []string{"encv-go"},
	}, zipPath, Options{}); err == nil {
		t.Fatal("缺二进制文件应报错")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "OLD-BINARY" {
		t.Fatalf("缺文件后旧二进制必须完好, got %q", string(got))
	}
}

// TestApplyFile_NoBackup_NoRollback —— 首次安装（没有旧文件）时回滚应明确报错而不是造空文件
func TestApplyFile_NoBackup_NoRollback(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "encv-go")
	if err := RollbackFile("go-binary", target); err == nil {
		t.Fatal("没有备份时回滚应报错")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("回滚失败不得顺手造出一个空的目标文件")
	}
}
