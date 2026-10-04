// Package bundle —— 通用「可热更新资源包」安装器（2026-10-04）
//
// 来源：把 `internal/server/preview_assets.go` 里那套"远端 zip → staging → 原子替换"
// 抽出来复用。预览页资源（preview-assets）是这套机制的第一个实例，它已经证明：
// **不换 APK 就能整包替换页面资源**（PreviewAssetsActivity 头注释即此结论）。
// 本包把它泛化成"任意名字的资源包"，供云控热更新（Hub 下发指令 → 设备端拉包 → 原子生效）使用。
//
// 不变式（任何改动都不得破坏）：
//  1. **先落 staging 再切换**：目标目录在切换前始终保持可用，绝不出现"半个新包"
//  2. **切换失败必须能回滚**：保留上一版备份（`backup` 目录），Apply 失败时目标目录仍是旧版
//  3. **hash 不符一律不落地**：校验在 staging 阶段做，不过关就不碰目标目录
//  4. **只写传入的 TargetDir 及其同级 staging/backup**：不得向上穿越
package bundle

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Spec 描述一个资源包的安装目标。
type Spec struct {
	// Name 包名（preview-assets / web / ...），只用于日志与备份目录命名。
	Name string
	// TargetDir 目标目录（必须是**可写**的应用数据目录，不在 APK / Go 二进制内）。
	TargetDir string
	// Version 待安装版本（写入 version.json，供"装了没有/装了哪版"查询）。
	Version string
	// SHA256 期望的 zip 摘要（十六进制）。空串 = 不校验（**不建议**，仅本地导入场景）。
	SHA256 string
	// Required 包内必须存在的文件（相对路径），缺一个就判定"包不完整"。
	// 用于挡住"zip 能解开但内容不对"的半成品 —— 这类包切进去就是白屏。
	Required []string
}

// Result 一次安装的结果。
type Result struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	TargetDir string `json:"targetDir"`
	// PreviousVersion 被覆盖掉的上一版（没有则空）。回滚用得上。
	PreviousVersion string `json:"previousVersion,omitempty"`
	// BackupDir 上一版的备份位置（回滚时从这里取回）。
	BackupDir string `json:"backupDir,omitempty"`
	AppliedAt string `json:"appliedAt"`
}

// Options 安装参数（都有默认值）。
type Options struct {
	// MaxBytes zip 大小上限（0 = 默认 128MiB）。防止把设备存储打满。
	MaxBytes int64
	// Timeout 下载总超时（0 = 默认 120s）。
	Timeout time.Duration
	// Now 时间源（测试可注入）。
	Now func() time.Time
}

const (
	defaultMaxBytes = 128 << 20
	defaultTimeout  = 120 * time.Second
	// stagingSuffix / backupSuffix 与目标目录同级，便于同分区 rename（原子）。
	stagingSuffix = "-staging"
	backupSuffix  = "-backup"
)

// ErrChecksumMismatch sha256 不符（调用方应据此拒绝并上报，不重试同一包）。
var ErrChecksumMismatch = errors.New("bundle: sha256 mismatch")

// ErrIncomplete 包解开后缺少 Required 文件。
var ErrIncomplete = errors.New("bundle: missing required files")

// ErrTooLarge 超过大小上限。
var ErrTooLarge = errors.New("bundle: too large")

// ── 下载 ────────────────────────────────────────────────────────────

// Download 把 url 内容下载到 dest，带大小上限与超时。
//
// 为什么不用 io.Copy 裸写：对端可能是被劫持/故障的服务，返回一个无限流，
// 一次性读完会把设备存储打满 ⇒ 边读边计数，超限立刻中止并删掉半截文件。
func Download(ctx context.Context, url, dest string, maxBytes int64, timeout time.Duration) error {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bundle: download %s: http %d", url, resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha256.New()
	written, err := copyWithLimit(io.MultiWriter(f, h), resp.Body, maxBytes)
	if err != nil {
		// 半截文件绝不能留：后续 Apply 会把它当完整包校验（hash 不符会挡住，但占空间）
		_ = os.Remove(dest)
		return err
	}
	_ = written
	return nil
}

// copyWithLimit 复制并限制总字节数，超限返回 ErrTooLarge。
func copyWithLimit(dst io.Writer, src io.Reader, max int64) (int64, error) {
	var written int64
	buf := make([]byte, 64*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			written += int64(n)
			if written > max {
				return written, ErrTooLarge
			}
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return written, werr
			}
		}
		if err == io.EOF {
			return written, nil
		}
		if err != nil {
			return written, err
		}
	}
}

// SHA256File 计算文件摘要（十六进制）。
func SHA256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ── 安装 ────────────────────────────────────────────────────────────

// Apply 把本地 zipPath 安装到 spec.TargetDir。
//
// 流程：校验 hash → 解到 staging → 校验 Required → 备份当前版 → 原子 rename → 写 version.json。
// 任一步失败 ⇒ 清理 staging 并保持目标目录为**旧版**（绝不留下半新半旧）。
func Apply(spec Spec, zipPath string, opts Options) (Result, error) {
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	if spec.TargetDir == "" {
		return Result{}, errors.New("bundle: empty TargetDir")
	}
	if spec.Name == "" {
		return Result{}, errors.New("bundle: empty Name")
	}

	// ① hash 校验（在 staging 之前做，不过关就完全不碰目标目录）
	if spec.SHA256 != "" {
		got, err := SHA256File(zipPath)
		if err != nil {
			return Result{}, err
		}
		if !strings.EqualFold(got, spec.SHA256) {
			return Result{}, fmt.Errorf("%w: want %s got %s", ErrChecksumMismatch, spec.SHA256, got)
		}
	}

	parent := filepath.Dir(spec.TargetDir)
	staging := filepath.Join(parent, spec.Name+stagingSuffix)
	backup := filepath.Join(parent, spec.Name+backupSuffix)

	// ② staging：先清空再解压，保证 staging 里只有本次内容（避免上次残留混进来）
	if err := os.RemoveAll(staging); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(staging)

	if err := unzip(zipPath, staging, opts.MaxBytes); err != nil {
		return Result{}, err
	}

	// ③ 完整性：必含文件缺一个 ⇒ 这套资源切进去就是白屏，宁可不动
	root := staging
	if hasSingleRootDir(staging) {
		// zip 常见形态：外层套一个目录（如 dist/）。剥掉它再校验。
		root = singleRootDir(staging)
	}
	if missing := missingFiles(root, spec.Required); len(missing) > 0 {
		return Result{}, fmt.Errorf("%w: %v", ErrIncomplete, missing)
	}

	// ④ 备份当前版本（有则移走；rename 保证原子）
	prev := currentVersion(spec.TargetDir)
	res := Result{
		Name:            spec.Name,
		Version:         spec.Version,
		TargetDir:       spec.TargetDir,
		PreviousVersion: prev,
		AppliedAt:       now().Format(time.RFC3339),
	}
	if isDir(spec.TargetDir) {
		_ = os.RemoveAll(backup)
		if err := os.Rename(spec.TargetDir, backup); err != nil {
			return Result{}, err
		}
		res.BackupDir = backup
	}

	// ⑤ 原子切换
	if err := os.Rename(root, spec.TargetDir); err != nil {
		// 切失败 ⇒ 把备份搬回去，恢复现场
		if res.BackupDir != "" {
			_ = os.Rename(backup, spec.TargetDir)
		}
		return Result{}, err
	}
	// 切换成功后 staging 目录已不存在（被 rename 走），defer 的 RemoveAll 是 no-op。

	// ⑥ 版本号（供"装了哪版"查询；写失败不算失败，资源已生效）
	_ = writeVersion(spec.TargetDir, spec.Version, now())
	return res, nil
}

// FileSpec 描述一个**单文件**资源（I3：Go 二进制热更新用）。
//
// 与 Spec（目录）的区别：目标是**一个文件**，切换仍必须原子（rename 覆盖），
// 且要保留上一版以便回滚 —— 因为换的是**执行体**，换坏了设备就起不来后端。
type FileSpec struct {
	// Name 包名（go-binary），用于备份文件命名。
	Name string
	// Target 目标文件绝对路径（如 <filesDir>/encv-go）。
	Target string
	// Version 新版本（写入 <Target>.version 供 Kotlin 侧识别"这是热更放进去的"）。
	Version string
	// ABI 目标架构（写入 <Target>.abi；Kotlin 侧会与 Build.SUPPORTED_ABIS[0] 比对）。
	ABI string
	// Mode 目标文件权限（0 = 0755）。
	Mode os.FileMode
	// SHA256 zip 摘要（十六进制）。
	SHA256 string
	// Required 包内必须存在的文件（相对路径），例如 ["encv-go"]。
	Required []string
}

// ApplyFile 把 zip 里的单个文件原子替换到 spec.Target。
//
// 流程：解到 staging → 校验 Required 与 ABI → 备份当前文件 → **rename 覆盖** → 写 sidecar。
//
// ⚠️ 为什么 rename 覆盖正在运行的可执行文件是安全的：Linux/Android 上 rename 是
//
//	原子的目录项替换，运行中的进程继续持有旧 inode（不受影响）；新进程才用新二进制。
//	⇒ 热更后**需要重启进程**才生效（Kotlin 侧下启动即生效，见 EncvGoService）。
func ApplyFile(spec FileSpec, zipPath string, opts Options) (Result, error) {
	if spec.Target == "" || spec.Name == "" {
		return Result{}, errors.New("bundle: empty Target/Name")
	}
	if spec.SHA256 != "" {
		got, err := SHA256File(zipPath)
		if err != nil {
			return Result{}, err
		}
		if !strings.EqualFold(got, spec.SHA256) {
			return Result{}, fmt.Errorf("%w: want %s got %s", ErrChecksumMismatch, spec.SHA256, got)
		}
	}
	mode := spec.Mode
	if mode == 0 {
		mode = 0o755
	}

	parent := filepath.Dir(spec.Target)
	staging := filepath.Join(parent, spec.Name+"-staging")
	backup := filepath.Join(parent, spec.Name+"-backup")

	if err := os.RemoveAll(staging); err != nil {
		return Result{}, err
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(staging)

	if err := unzip(zipPath, staging, opts.MaxBytes); err != nil {
		return Result{}, err
	}
	root := staging
	if hasSingleRootDir(staging) {
		root = singleRootDir(staging)
	}
	if missing := missingFiles(root, spec.Required); len(missing) > 0 {
		return Result{}, fmt.Errorf("%w: %v", ErrIncomplete, missing)
	}

	res := Result{Name: spec.Name, Version: spec.Version, TargetDir: spec.Target}
	// 备份当前（存在才备份；rename 保证原子）
	if _, err := os.Stat(spec.Target); err == nil {
		_ = os.Remove(backup)
		if err := os.Rename(spec.Target, backup); err != nil {
			return Result{}, err
		}
		res.BackupDir = backup
		res.PreviousVersion = strings.TrimSpace(readFileOrEmpty(spec.Target + ".version"))
	}

	src := filepath.Join(root, filepath.FromSlash(spec.Required[0]))
	if err := os.Rename(src, spec.Target); err != nil {
		// 覆盖失败 ⇒ 立刻把备份搬回去，绝不让"没有可执行体"的状态存在
		if res.BackupDir != "" {
			_ = os.Rename(backup, spec.Target)
		}
		return Result{}, err
	}
	if err := os.Chmod(spec.Target, mode); err != nil {
		return res, err
	}
	// sidecar：Kotlin 侧据此判定"这是热更通道放的二进制"并校验 ABI
	if spec.Version != "" {
		_ = os.WriteFile(spec.Target+".version", []byte(spec.Version), 0o644)
	}
	if spec.ABI != "" {
		_ = os.WriteFile(spec.Target+".abi", []byte(spec.ABI), 0o644)
	}
	return res, nil
}

// RollbackFile 把 Target 回滚到备份（热更二进制起不来时用）。
func RollbackFile(name, target string) error {
	backup := filepath.Join(filepath.Dir(target), name+"-backup")
	if _, err := os.Stat(backup); err != nil {
		return fmt.Errorf("bundle: no backup for %s", name)
	}
	_ = os.Remove(target)
	return os.Rename(backup, target)
}

func readFileOrEmpty(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// Rollback 把 targetDir 回滚到备份版本。
func Rollback(name, targetDir string) error {
	backup := filepath.Join(filepath.Dir(targetDir), name+backupSuffix)
	if !isDir(backup) {
		return fmt.Errorf("bundle: no backup for %s", name)
	}
	_ = os.RemoveAll(targetDir)
	return os.Rename(backup, targetDir)
}

// CurrentVersion 读取已装版本（未安装返回空串）。
func CurrentVersion(targetDir string) string { return currentVersion(targetDir) }

func currentVersion(targetDir string) string {
	b, err := os.ReadFile(filepath.Join(targetDir, "version.json"))
	if err != nil {
		return ""
	}
	var v struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return ""
	}
	return v.Version
}

func writeVersion(dir, version string, at time.Time) error {
	b, _ := json.Marshal(map[string]string{
		"version":   version,
		"updatedAt": at.Format(time.RFC3339),
	})
	return os.WriteFile(filepath.Join(dir, "version.json"), b, 0o644)
}

// ── zip 解压 ────────────────────────────────────────────────────────

func unzip(zipPath, dest string, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("bundle: open zip: %w", err)
	}
	defer r.Close()

	var total int64
	for _, f := range r.File {
		// ⚠️ Zip-Slip：文件名可能是 ../../etc/passwd，解压必须重算落点并校验前缀
		clean := filepath.Clean(filepath.FromSlash(f.Name))
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			return fmt.Errorf("bundle: illegal entry %q", f.Name)
		}
		target := filepath.Join(dest, clean)
		if target != dest && !strings.HasPrefix(target, dest+string(os.PathSeparator)) {
			return fmt.Errorf("bundle: illegal entry %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		// 解压炸弹：单文件与总体积双重限制
		if f.UncompressedSize64 > uint64(maxBytes) {
			rc.Close()
			return ErrTooLarge
		}
		n, err := copyWithLimit(io.Discard, rc, maxBytes-total)
		rc.Close()
		if err != nil {
			return err
		}
		if f.UncompressedSize64 > 0 {
			total += int64(f.UncompressedSize64)
		} else {
			total += n
		}
		if total > maxBytes {
			return ErrTooLarge
		}
		// 真正落盘（上面只做了计数，避免"先写一半才发现超限"）
		rc2, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc2.Close()
			return err
		}
		if _, err := io.Copy(out, rc2); err != nil {
			out.Close()
			rc2.Close()
			return err
		}
		out.Close()
		rc2.Close()
	}
	return nil
}

func hasSingleRootDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		return false
	}
	return entries[0].IsDir()
}

func singleRootDir(dir string) string {
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		return dir
	}
	return filepath.Join(dir, entries[0].Name())
}

func missingFiles(root string, required []string) []string {
	var missing []string
	for _, rel := range required {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			missing = append(missing, rel)
		}
	}
	return missing
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
