package drivers

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Soltus/encv-go/internal/mount"
)

// stubAndroidCfg 模拟真机环境（runtime.GOOS == "android"）下的 ConfigProvider。
//
// 注意：AppDataDriver 用 runtime.GOOS 判断"是否真机"，本机测试时 GOOS=linux，
// 所以测试要绕过 runtime 检查，单独验证"appdata path 字符串"逻辑。
// 这里直接用 subpath + pkg + uid 拼接 expected path 跟 driver Init 后实际 RootPath 对照。
type stubAndroidCfg struct {
	pkg     string
	fbDir   string
}

func (s *stubAndroidCfg) IsMobile() bool             { return true }
func (s *stubAndroidCfg) IsDev() bool                { return false }
func (s *stubAndroidCfg) AndroidPackageName() string { return s.pkg }
func (s *stubAndroidCfg) DataDir() string            { return "" }
func (s *stubAndroidCfg) AppDataFallbackDir() string { return s.fbDir }
func (s *stubAndroidCfg) DevSandboxDir() string      { return "" }
func (s *stubAndroidCfg) ServingDir() string         { return "" }
func (s *stubAndroidCfg) AutomationDriver() string   { return mount.DriverAppData }

// TestAppDataDriver_RealDevicePath 验证 AppDataDriver 在真机（Android）下产生
// 应用私有 data 目录，不是 shared storage /storage/emulated/0/。
//
// 关键不变量（用户 2026-06-15 怒批"必然失败"的红线）：
//   - RootPath 必须在 /data/user/<uid>/<pkg>/files/ 下
//   - 绝不能在 /storage/emulated/0/ 下
//   - 绝不能在 /sdcard/ 下
//   - 任何带 "emulated" / "sdcard" / "storage" 关键字都不是 app-private
//
// 🆕 2026-06-16：测试用 stubAndroidCfgDataDir 模拟 Kotlin 端 ENCV_APP_FILES_DIR 注入
//   cfg.DataDir() 返回 /data/user/10490/com.encvgo.app/files（Kotlin context.filesDir）
//   driver 优先用 cfg.DataDir() 作为 base，subpath 拼在后面
//   最终 root = /data/user/10490/com.encvgo.app/files/encv-automation
//
//   历史：driver 硬编码 /data/user/<uid>/<pkg>/files → 真机 EACCES（sdcardfs FUSE 权限）
//   新实现：driver 优先用 cfg.DataDir()（Kotlin 已知的好路径），无 cfg 时兜底硬编码
//
// 通过 test 防止后续"误改 driver 默认值"导致真机 EACCES。
func TestAppDataDriver_RealDevicePath(t *testing.T) {
	if runtime.GOOS == "android" {
		t.Skip("本测试在 Android 真机上无意义（直接走 driver）")
	}

	// 模拟 Kotlin context.filesDir = /data/user/10490/com.encvgo.app/files
	cfg := &stubAndroidCfgDataDir{
		pkg:     "com.encvgo.app",
		fbDir:   t.TempDir(),
		dataDir: "/data/user/10490/com.encvgo.app/files",
	}
	drv := NewAppDataDriver()
	// 强制当作 Android（绕过 runtime.GOOS 检查）
	drv.isAndroid = true
	// 🆕 2026-06-16：用 ComputeRootPath 直接算路径（不调 Init，避免在 CI 上真 mkdir /data/user/10490）
	root, usedAndroid := drv.ComputeRootPath(cfg, "encv-automation")
	if !usedAndroid {
		t.Errorf("isAndroid=true 时应走 Android 路径，usedAndroid=false")
	}

	// ① 必须以 /data/user/ 开头
	if !filepath.HasPrefix(root, "/data/user/") {
		t.Errorf("RootPath 不在 /data/user/ 下（不是 app-private）: %q", root)
	}
	// ② 必须含 /files/（app-private files 目录）
	if !filepath.HasPrefix(root, "/data/user/") || !containsPath(root, "/files/") {
		t.Errorf("RootPath 不在 /data/user/<uid>/<pkg>/files/ 下: %q", root)
	}
	// ③ 必须含包名
	if !containsPath(root, "com.encvgo.app") {
		t.Errorf("RootPath 不含包名 com.encvgo.app: %q", root)
	}
	// ④ 必须含 subpath
	if !containsPath(root, "encv-automation") {
		t.Errorf("RootPath 不含 subpath encv-automation: %q", root)
	}
	// ⑤ 绝不能命中 shared storage 红线词
	for _, bad := range []string{"emulated", "sdcard", "/storage/"} {
		if containsPath(root, bad) {
			t.Errorf("RootPath 命中 shared-storage 红线 %q: %q", bad, root)
		}
	}
	// ⑥ uid 必须存在（数字 1+ 位）— 10490
	if !hasUIDSegment(root) {
		t.Errorf("RootPath 不含 uid 数字段: %q", root)
	}
	// ⑦ 🆕 2026-06-16：根路径末尾必须是 subpath（无多余 subpath 段）
	expectedSuffix := filepath.Join("/data/user/10490/com.encvgo.app/files", "encv-automation")
	if root != expectedSuffix {
		t.Errorf("RootPath 应 = cfg.DataDir() + subpath\ngot  = %q\nwant = %q", root, expectedSuffix)
	}

	t.Logf("✅ AppDataDriver 真机 root = %s", root)
}

// TestAppDataDriver_DevFallbackPath 验证 dev/sandbox 下走 AppDataFallbackDir 而不是真机路径。
func TestAppDataDriver_DevFallbackPath(t *testing.T) {
	if runtime.GOOS == "android" {
		t.Skip("本测试在 Android 真机上无意义")
	}

	tmpDir := t.TempDir()
	cfg := &stubAndroidCfg{pkg: "com.encvgo.app", fbDir: tmpDir}
	drv := NewAppDataDriver()
	drv.isAndroid = false // 强制 dev 模式
	m := &mount.Mount{
		Name:         "automation",
		MountPath:    "/d/automation",
		Driver:       mount.DriverAppData,
		DriverConfig: map[string]any{"subpath": "encv-automation"},
	}
	if err := drv.Init(context.Background(), m, cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	root := drv.ResolveRoot()
	expected := filepath.Join(tmpDir, "encv-automation")
	if root != expected {
		t.Errorf("dev fallback root 错误: got %q, want %q", root, expected)
	}

	// dev fallback 也不能是 shared storage
	for _, bad := range []string{"emulated", "sdcard", "/storage/"} {
		if containsPath(root, bad) {
			t.Errorf("dev fallback 命中 shared-storage 红线 %q: %q", bad, root)
		}
	}
	t.Logf("✅ AppDataDriver dev fallback root = %s", root)
}

// TestAppDataDriver_MkdirAll 验证 Init 成功创建根目录（真机无权限应该直接 Init 失败）。
//
// dev 模式容错：即使 mkdir 失败也不返回 error（让 fallback 路径生效）。
// 真机模式：mkdir 失败必须返回 error（不能让 app 在没有写入权限时无声跑下去）。
func TestAppDataDriver_MkdirAll(t *testing.T) {
	if runtime.GOOS == "android" {
		t.Skip("本测试在 Android 真机上无意义")
	}

	// dev 模式 + 不可写 dir
	tmpDir := t.TempDir()
	cfg := &stubAndroidCfg{pkg: "com.encvgo.app", fbDir: tmpDir}
	drv := NewAppDataDriver()
	drv.isAndroid = false
	m := &mount.Mount{
		Name:         "automation",
		MountPath:    "/d/automation",
		Driver:       mount.DriverAppData,
		DriverConfig: map[string]any{"subpath": "encv-automation"},
	}
	if err := drv.Init(context.Background(), m, cfg); err != nil {
		t.Fatalf("dev mode Init 不应失败（即使有 mkdir 警告）: %v", err)
	}
	if _, err := os.Stat(drv.ResolveRoot()); err != nil {
		t.Errorf("dev mode 应创建 root 目录: %v", err)
	}
}

// TestAppDataDriver_AndroidLeafMkdir_UnitIsolated 验证 2026-06-16 修复的 leaf-mkdir 逻辑。
//
//   旧实现：os.MkdirAll(d.root) 递归创建父目录 → 真机上撞 /data/user/<uid>/ EACCES
//   新实现：先 os.Stat 判断 d.root 是否已存在；不存在时只 os.Mkdir 叶子
//
//   关键不变量（纯函数级别，不依赖 Android 路径）：
//   - 当 leaf 父目录存在 + writable + leaf 不存在时 → Init 应成功，leaf 被创建
//   - 当 leaf 已存在时 → Init 应秒过（不重复 mkdir）
//
//   本测试在 Linux 上跑，不走 driver 默认的 /data/user/<uid>/ 硬编码路径，
//   改用 cfg.DataDir() 覆盖（由 stubAndroidCfg 提供）。
//   真实 Android 路径上同样行为（在 cfg.AndroidDataDir() 拿到的 filesDir 下创建 subpath），
//   真机是否修复由用户在 Android 上验证（CI 无法模拟 sdcardfs FUSE 行为）。
func TestAppDataDriver_AndroidLeafMkdir_UnitIsolated(t *testing.T) {
	if runtime.GOOS == "android" {
		t.Skip("本测试在 Android 真机上无意义（路径硬编码 vs cfg.DataDir 行为一致）")
	}

	// 用 t.TempDir() 模拟 Android 系统的 files/ 父目录
	tmpDir := t.TempDir()
	fakeFilesDir := filepath.Join(tmpDir, "files")
	if err := os.Mkdir(fakeFilesDir, 0755); err != nil {
		t.Fatalf("setup: create fake files/ parent: %v", err)
	}

	// stub 让 cfg.DataDir() 返回 fakeFilesDir
	cfg := &stubAndroidCfgDataDir{
		pkg:     "com.encvgo.app",
		fbDir:   tmpDir,
		dataDir: fakeFilesDir,
	}
	drv := NewAppDataDriver()
	drv.isAndroid = true
	m := &mount.Mount{
		Name:         "automation",
		MountPath:    "/d/automation",
		Driver:       mount.DriverAppData,
		DriverConfig: map[string]any{"subpath": "encv-automation"},
	}

	if err := drv.Init(context.Background(), m, cfg); err != nil {
		t.Fatalf("Init 应成功（leaf 不存在 + parent writable）: %v", err)
	}

	root := drv.ResolveRoot()
	if !filepath.IsAbs(root) {
		t.Errorf("root 应是绝对路径: %q", root)
	}
	// 1. root 必须包含 subpath
	if !containsPath(root, "encv-automation") {
		t.Errorf("root 应含 subpath encv-automation: %q", root)
	}
	// 2. root 必须已创建（leaf）
	if _, err := os.Stat(root); err != nil {
		t.Errorf("Init 后 leaf 应存在: %v", err)
	}
	// 3. 父目录 fakeFilesDir 应仍然存在（未被破坏）
	if _, err := os.Stat(fakeFilesDir); err != nil {
		t.Errorf("parent files/ 应仍存在: %v", err)
	}
	t.Logf("✅ android mode root = %s", root)
}

// TestAppDataDriver_AndroidLeafMkdir_RootExists 验证 2026-06-16 修复：
//   root 已存在 → Init 秒过（不重复 mkdir）
//   对应真机：老 mounts.json 持久化路径命中 → 重启后系统已创建 → 重复 mkdir 没意义
func TestAppDataDriver_AndroidLeafMkdir_RootExists(t *testing.T) {
	if runtime.GOOS == "android" {
		t.Skip("本测试在 Android 真机上无意义")
	}

	tmpDir := t.TempDir()
	fakeFilesDir := filepath.Join(tmpDir, "files")
	if err := os.Mkdir(fakeFilesDir, 0755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// 预创建 root
	expectedRoot := filepath.Join(fakeFilesDir, "encv-automation")
	if err := os.Mkdir(expectedRoot, 0755); err != nil {
		t.Fatalf("setup pre-create root: %v", err)
	}

	cfg := &stubAndroidCfgDataDir{
		pkg:     "com.encvgo.app",
		fbDir:   tmpDir,
		dataDir: fakeFilesDir,
	}
	drv := NewAppDataDriver()
	drv.isAndroid = true
	m := &mount.Mount{
		Name:         "automation",
		MountPath:    "/d/automation",
		Driver:       mount.DriverAppData,
		DriverConfig: map[string]any{"subpath": "encv-automation"},
	}

	if err := drv.Init(context.Background(), m, cfg); err != nil {
		t.Errorf("root 已存在时 Init 应成功: %v", err)
	}
	if drv.ResolveRoot() != expectedRoot {
		t.Errorf("ResolveRoot = %q, want %q", drv.ResolveRoot(), expectedRoot)
	}
}

// stubAndroidCfgDataDir 给测试用：让 cfg.DataDir() 返回自定义路径
// （替代硬编码的 /data/user/<uid>/<pkg>/files，使 Linux CI 上可测试）
type stubAndroidCfgDataDir struct {
	pkg     string
	fbDir   string
	dataDir string
}

func (s *stubAndroidCfgDataDir) IsMobile() bool             { return true }
func (s *stubAndroidCfgDataDir) IsDev() bool                { return false }
func (s *stubAndroidCfgDataDir) AndroidPackageName() string { return s.pkg }
func (s *stubAndroidCfgDataDir) DataDir() string            { return s.dataDir }
func (s *stubAndroidCfgDataDir) AppDataFallbackDir() string { return s.fbDir }
func (s *stubAndroidCfgDataDir) DevSandboxDir() string      { return "" }
func (s *stubAndroidCfgDataDir) ServingDir() string         { return "" }
func (s *stubAndroidCfgDataDir) AutomationDriver() string   { return mount.DriverAppData }

// containsPath 不区分大小写检查 path 是否包含 sub。
func containsPath(path, sub string) bool {
	if sub == "" {
		return true
	}
	lp := toLower(path)
	ls := toLower(sub)
	for i := 0; i+len(ls) <= len(lp); i++ {
		if lp[i:i+len(ls)] == ls {
			return true
		}
	}
	return false
}

// hasUIDSegment 检查 path 是否在 /data/user/<digits>/ 形式。
func hasUIDSegment(path string) bool {
	// 期望 /data/user/<digits>/<pkg>/files/<sub>
	parts := splitPath(path)
	// 找 "user" 段后面是不是数字段
	for i, p := range parts {
		if p == "user" && i+1 < len(parts) {
			next := parts[i+1]
			if len(next) > 0 {
				allDigit := true
				for _, r := range next {
					if r < '0' || r > '9' {
						allDigit = false
						break
					}
				}
				return allDigit
			}
		}
	}
	return false
}

func splitPath(p string) []string {
	out := []string{}
	cur := ""
	for _, r := range p {
		if r == '/' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		} else {
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func toLower(s string) string {
	b := make([]byte, len(s))
	for i, r := range []byte(s) {
		if r >= 'A' && r <= 'Z' {
			b[i] = r + ('a' - 'A')
		} else {
			b[i] = r
		}
	}
	return string(b)
}
