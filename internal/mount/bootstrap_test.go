package mount

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

// stubBootstrapCfg 是 BootstrapFromConfig 测试专用 ConfigProvider 桩。
// 2026-06-15 修复：所有 mode 下都应创建 primary + automation（无论 IsMobile/IsDev）
type stubBootstrapCfg struct {
	servingDir string
	dataDir    string
	isDev      bool
	isMobile   bool
}

func (s *stubBootstrapCfg) IsMobile() bool             { return s.isMobile }
func (s *stubBootstrapCfg) IsDev() bool                { return s.isDev }
func (s *stubBootstrapCfg) AndroidPackageName() string { return "com.test.encv" }
func (s *stubBootstrapCfg) DataDir() string            { return s.dataDir }
func (s *stubBootstrapCfg) AppDataFallbackDir() string { return s.servingDir }
func (s *stubBootstrapCfg) DevSandboxDir() string      { return s.servingDir }
func (s *stubBootstrapCfg) ServingDir() string         { return s.servingDir }
func (s *stubBootstrapCfg) AutomationDriver() string   { return "local" } // 测试用 local（无需 appdata driver 真实初始化）

// makeRegistryWithDrivers 构造一个带 3 个 driver factory 的 MountRegistry（Bootstrap 需要 instantiate driver）。
func makeRegistryWithDrivers(t *testing.T, cfg ConfigProvider) *MountRegistry {
	t.Helper()
	r := NewRegistry(cfg, "")
	r.RegisterDriverFactory(DriverLocal, func() Driver { return &fakeLocalDriver{} })
	r.RegisterDriverFactory(DriverAppData, func() Driver { return &fakeAppDataDriver{} })
	r.RegisterDriverFactory(DriverSandbox, func() Driver { return &fakeSandboxDriver{} })
	return r
}

// fakeLocalDriver / fakeAppDataDriver / fakeSandboxDriver 是测试用最小 driver。
// 不依赖真实 FS（Init 只设置 RootPath），不调 cfg 包内真实 Init。
type fakeLocalDriver struct{ root string }

func (d *fakeLocalDriver) Name() string { return DriverLocal }
func (d *fakeLocalDriver) Init(_ context.Context, m *Mount, _ ConfigProvider) error {
	d.root = m.RootPath
	m.RootPath = m.RootPath
	return nil
}
func (d *fakeLocalDriver) ResolveRoot() string { return d.root }
func (d *fakeLocalDriver) CheckPermission() error {
	if d.root == "" {
		return nil
	}
	_, err := os.Stat(d.root)
	return err
}
func (d *fakeLocalDriver) Stat(relPath string) (os.FileInfo, error) {
	return os.Stat(filepath.Join(d.root, relPath))
}
func (d *fakeLocalDriver) ReadDir(relPath string) ([]os.DirEntry, error) {
	return os.ReadDir(filepath.Join(d.root, relPath))
}
func (d *fakeLocalDriver) ReadFile(relPath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(d.root, relPath))
}
func (d *fakeLocalDriver) WriteFile(relPath string, data []byte, perm os.FileMode) error {
	abs := filepath.Join(d.root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	return os.WriteFile(abs, data, perm)
}
func (d *fakeLocalDriver) MkdirAll(relPath string, perm os.FileMode) error {
	return os.MkdirAll(filepath.Join(d.root, relPath), perm)
}
func (d *fakeLocalDriver) Remove(relPath string) error        { return os.Remove(filepath.Join(d.root, relPath)) }
func (d *fakeLocalDriver) Rename(oldR, newR string) error     { return os.Rename(filepath.Join(d.root, oldR), filepath.Join(d.root, newR)) }
func (d *fakeLocalDriver) Reload(m *Mount) error              { d.root = m.RootPath; return nil }

type fakeAppDataDriver struct{ root string }

func (d *fakeAppDataDriver) Name() string { return DriverAppData }
func (d *fakeAppDataDriver) Init(_ context.Context, m *Mount, _ ConfigProvider) error {
	sub, _ := m.DriverConfig["subpath"].(string)
	if sub == "" {
		sub = "encv-appdata"
	}
	d.root = filepath.Join("/tmp/encv-fake-appdata", sub)
	m.RootPath = d.root
	return nil
}
func (d *fakeAppDataDriver) ResolveRoot() string { return d.root }
func (d *fakeAppDataDriver) CheckPermission() error { return nil }
func (d *fakeAppDataDriver) Stat(relPath string) (os.FileInfo, error) {
	return os.Stat(filepath.Join(d.root, relPath))
}
func (d *fakeAppDataDriver) ReadDir(relPath string) ([]os.DirEntry, error) {
	return os.ReadDir(filepath.Join(d.root, relPath))
}
func (d *fakeAppDataDriver) ReadFile(relPath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(d.root, relPath))
}
func (d *fakeAppDataDriver) WriteFile(relPath string, data []byte, perm os.FileMode) error {
	abs := filepath.Join(d.root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	return os.WriteFile(abs, data, perm)
}
func (d *fakeAppDataDriver) MkdirAll(relPath string, perm os.FileMode) error {
	return os.MkdirAll(filepath.Join(d.root, relPath), perm)
}
func (d *fakeAppDataDriver) Remove(relPath string) error     { return os.Remove(filepath.Join(d.root, relPath)) }
func (d *fakeAppDataDriver) Rename(oldR, newR string) error { return os.Rename(filepath.Join(d.root, oldR), filepath.Join(d.root, newR)) }
func (d *fakeAppDataDriver) Reload(m *Mount) error          { d.root = m.RootPath; return nil }

type fakeSandboxDriver struct{ root string }

func (d *fakeSandboxDriver) Name() string { return DriverSandbox }
func (d *fakeSandboxDriver) Init(_ context.Context, m *Mount, _ ConfigProvider) error {
	d.root = "/tmp/encv-fake-sandbox"
	m.RootPath = d.root
	return nil
}
func (d *fakeSandboxDriver) ResolveRoot() string  { return d.root }
func (d *fakeSandboxDriver) CheckPermission() error { return nil }
func (d *fakeSandboxDriver) Stat(relPath string) (os.FileInfo, error) {
	return os.Stat(filepath.Join(d.root, relPath))
}
func (d *fakeSandboxDriver) ReadDir(relPath string) ([]os.DirEntry, error) {
	return os.ReadDir(filepath.Join(d.root, relPath))
}
func (d *fakeSandboxDriver) ReadFile(relPath string) ([]byte, error) {
	return os.ReadFile(filepath.Join(d.root, relPath))
}
func (d *fakeSandboxDriver) WriteFile(relPath string, data []byte, perm os.FileMode) error {
	abs := filepath.Join(d.root, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return err
	}
	return os.WriteFile(abs, data, perm)
}
func (d *fakeSandboxDriver) MkdirAll(relPath string, perm os.FileMode) error {
	return os.MkdirAll(filepath.Join(d.root, relPath), perm)
}
func (d *fakeSandboxDriver) Remove(relPath string) error     { return os.Remove(filepath.Join(d.root, relPath)) }
func (d *fakeSandboxDriver) Rename(oldR, newR string) error { return os.Rename(filepath.Join(d.root, oldR), filepath.Join(d.root, newR)) }
func (d *fakeSandboxDriver) Reload(m *Mount) error          { d.root = m.RootPath; return nil }

// TestBootstrap_AlwaysCreatesPrimaryAndAutomation 验证 2026-06-15 修复：
// 即使 IsMobile()=false，automation mount 也要创建。
//
// 历史 bug：旧版 Bootstrap 用 cfg.IsMobile() 门 → 真机启动时 mobile overlay 未生效
// （cfg.Mobile=nil）或 dev 模式（ENCV_DEV=1，IsMobile=false）→ automation mount 不创建
// → 用户跑自动化测试 POST /api/mock/generate {root:"/d/automation"} → 403 invalid mount path
func TestBootstrap_AlwaysCreatesPrimaryAndAutomation(t *testing.T) {
	tmp := t.TempDir()
	cfg := &stubBootstrapCfg{
		servingDir: tmp,
		dataDir:    tmp,
		isDev:      false,
		isMobile:   false,
	}
	r := makeRegistryWithDrivers(t, cfg)
	if err := r.BootstrapFromConfig(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	mounts := r.List()
	if len(mounts) != 2 {
		t.Fatalf("expected 2 mounts (primary+automation), got %d: %+v", len(mounts), mounts)
	}
	names := map[string]*Mount{}
	for _, m := range mounts {
		names[m.Name] = m
	}
	if names[NamePrimary] == nil {
		t.Errorf("primary mount not created")
	}
	if names[NameAutomation] == nil {
		t.Errorf("automation mount not created (regression: 2026-06-15 user 22:29 feedback)")
	}
	// sandbox 不应创建（非 dev 模式）
	if names[NameSandbox] != nil {
		t.Errorf("sandbox mount should not be created in non-dev mode")
	}
}

// TestBootstrap_DevModeAddsSandbox 验证 dev 模式多创建 sandbox mount。
func TestBootstrap_DevModeAddsSandbox(t *testing.T) {
	tmp := t.TempDir()
	cfg := &stubBootstrapCfg{servingDir: tmp, dataDir: tmp, isDev: true, isMobile: false}
	r := makeRegistryWithDrivers(t, cfg)
	if err := r.BootstrapFromConfig(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	names := map[string]*Mount{}
	for _, m := range r.List() {
		names[m.Name] = m
	}
	if names[NamePrimary] == nil {
		t.Errorf("primary missing")
	}
	if names[NameAutomation] == nil {
		t.Errorf("automation missing (regression)")
	}
	if names[NameSandbox] == nil {
		t.Errorf("sandbox missing in dev mode")
	}
}

// TestBootstrap_PrimaryFallsBackToDataDir 验证 ServingDir 为空时用 DataDir 兜底。
func TestBootstrap_PrimaryFallsBackToDataDir(t *testing.T) {
	cfg := &stubBootstrapCfg{
		servingDir: "",     // 空
		dataDir:    "/tmp/encv-fallback-data",
		isDev:      false,
		isMobile:   false,
	}
	r := makeRegistryWithDrivers(t, cfg)
	if err := r.BootstrapFromConfig(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	primary := r.GetByName(NamePrimary)
	if primary == nil {
		t.Fatal("primary not created")
	}
	if primary.RootPath != "/tmp/encv-fallback-data" {
		t.Errorf("primary root should fall back to dataDir, got %q", primary.RootPath)
	}
}

// TestBootstrap_NilCfgRejected 验证 cfg=nil 返回 error。
func TestBootstrap_NilCfgRejected(t *testing.T) {
	r := NewRegistry(nil, "")
	if err := r.BootstrapFromConfig(context.Background()); err == nil {
		t.Fatal("expected error when cfg is nil")
	}
}

// TestBootstrap_DoesNotOverwriteExisting 验证同名 mount 已存在时不覆盖。
func TestBootstrap_DoesNotOverwriteExisting(t *testing.T) {
	tmp := t.TempDir()
	cfg := &stubBootstrapCfg{servingDir: tmp, dataDir: tmp, isDev: true}
	r := makeRegistryWithDrivers(t, cfg)
	// 预创建 primary
	preset := &Mount{
		Name:      NamePrimary,
		MountPath: "/d/" + NamePrimary,
		Driver:    DriverLocal,
		RootPath:  "/preset/path",
		Enabled:   true,
	}
	if err := r.Create(preset); err != nil {
		t.Fatalf("preset: %v", err)
	}
	if err := r.BootstrapFromConfig(context.Background()); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	got := r.GetByName(NamePrimary)
	if got.RootPath != "/preset/path" {
		t.Errorf("primary should be untouched, got %q", got.RootPath)
	}
}
