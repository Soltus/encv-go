package mount

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// stubConfigProvider 是 ConfigProvider 的最小测试桩。
//
// 2026-06-15：MigrateFromServingDir 的核心修复 —— 启动期先 migrate + Load，再判断 Bootstrap。
// stub 必须提供 ServingDir 非空让 Bootstrap 至少能创建一个 primary，否则测试兜底场景无法验证。
type stubConfigProvider struct {
	servingDir string
	isMobile   bool
	isDev      bool
}

func (s *stubConfigProvider) IsMobile() bool                  { return s.isMobile }
func (s *stubConfigProvider) IsDev() bool                     { return s.isDev }
func (s *stubConfigProvider) AndroidPackageName() string      { return "com.test.encv" }
func (s *stubConfigProvider) DataDir() string                 { return s.servingDir }
func (s *stubConfigProvider) AppDataFallbackDir() string      { return s.servingDir }
func (s *stubConfigProvider) DevSandboxDir() string           { return s.servingDir }
func (s *stubConfigProvider) ServingDir() string              { return s.servingDir }
func (s *stubConfigProvider) AutomationDriver() string        { return DriverLocal }

// stubDriver 是 Driver 接口最小实现（不执行任何 FS 操作，全部 no-op + 返回 nil）。
//
// 2026-06-15：用户数据 mount 里的 Driver 字段是 "local"，Load 后 Registry 不再调它
// （Init 只在 Bootstrap 路径触发）；但 GetByName / List 仍会触碰 in-memory 状态，
// 我们只验证 in-memory 字段，不验证 driver 行为。
type stubDriver struct {
	root string
}

func (d *stubDriver) Name() string                                  { return DriverLocal }
func (d *stubDriver) Init(_ context.Context, m *Mount, _ ConfigProvider) error { d.root = m.RootPath; return nil }
func (d *stubDriver) ResolveRoot() string                           { return d.root }
func (d *stubDriver) CheckPermission() error                        { return nil }
func (d *stubDriver) Stat(_ string) (os.FileInfo, error)            { return nil, os.ErrNotExist }
func (d *stubDriver) ReadDir(_ string) ([]os.DirEntry, error)       { return nil, os.ErrNotExist }
func (d *stubDriver) ReadFile(_ string) ([]byte, error)              { return nil, os.ErrNotExist }
func (d *stubDriver) WriteFile(_ string, _ []byte, _ os.FileMode) error { return nil }
func (d *stubDriver) MkdirAll(_ string, _ os.FileMode) error        { return nil }
func (d *stubDriver) Remove(_ string) error                         { return nil }
func (d *stubDriver) Rename(_, _ string) error                      { return nil }
func (d *stubDriver) Reload(_ *Mount) error                         { return nil }

// TestMigrateFromServingDir_MigratesAndLoadsLegacyData 端到端验证核心修复：
// 1. 老路径 serving_dir/mounts.json 存在（用户历史数据）
// 2. 新路径 serving_dir/.encv/mounts.json 不存在
// 3. MigrateFromServingDir 启动流程
// 期望：
//   - 老文件被原子 rename 到新路径
//   - in-memory registry 通过 Load 恢复 mount 列表
//   - Bootstrap 不被触发（避免覆盖用户数据）
//
// 2026-06-15 用户反馈"过滤 mounts.json 是错误方案"的真正根因：
// 之前 MigrateFromServingDir 只检查 dataPath 是否存在 + GetByName，
// 从未调用 Load → 每次启动都走 Bootstrap → 覆盖用户数据 + 老文件成孤儿。
func TestMigrateFromServingDir_MigratesAndLoadsLegacyData(t *testing.T) {
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "mounts.json")
	newPath := filepath.Join(tmpDir, ".encv", "mounts.json")

	// 老路径写一份只含一个自定义 mount 的 JSON
	userJSON := `{
  "version": 1,
  "mounts": [
    {
      "id": "user-custom-mount-id",
      "name": "user-data",
      "mount_path": "/d/user-data",
      "driver": "local",
      "config": {"path": "/tmp/user-data-root"}
    }
  ],
  "saved_at": "2026-06-14T10:00:00Z"
}`
	if err := os.WriteFile(oldPath, []byte(userJSON), 0644); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	// 构造 registry + 注册 stub driver（Load 不需要 driver，但保险起见）
	r := NewRegistry(&stubConfigProvider{servingDir: tmpDir}, newPath)
	r.RegisterDriverFactory(DriverLocal, func() Driver { return &stubDriver{} })

	if err := r.MigrateFromServingDir(context.Background()); err != nil {
		t.Fatalf("MigrateFromServingDir: %v", err)
	}

	// 老文件必须被移走
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("legacy file should be renamed away, stat err = %v", err)
	}

	// 新文件必须存在
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("new path missing: %v", err)
	}

	// 内存里必须有用户那个 mount（Load 把它恢复了）
	got := r.GetByName("user-data")
	if got == nil {
		t.Fatalf("user-data mount not loaded into memory; mounts = %+v", r.List())
	}
	if got.ID != "user-custom-mount-id" {
		t.Errorf("loaded mount id mismatch: got %q want %q", got.ID, "user-custom-mount-id")
	}
}

// TestMigrateFromServingDir_BootstrapsWhenEmpty 验证全新环境：
// 老文件不存在 + 新文件不存在 → 触发 Bootstrap。
func TestMigrateFromServingDir_BootstrapsWhenEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	newPath := filepath.Join(tmpDir, ".encv", "mounts.json")

	r := NewRegistry(&stubConfigProvider{servingDir: tmpDir}, newPath)
	r.RegisterDriverFactory(DriverLocal, func() Driver { return &stubDriver{} })

	if err := r.MigrateFromServingDir(context.Background()); err != nil {
		t.Fatalf("MigrateFromServingDir: %v", err)
	}

	// 全新数据被 Bootstrap + Save
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("new path should be created by bootstrap, stat err = %v", err)
	}
	if r.GetByName(NamePrimary) == nil {
		t.Errorf("primary mount should be bootstrapped")
	}
}

// TestMigrateFromServingDir_LoadsExistingNewPath 验证标准场景：
// 新路径已有数据（之前正常启动过），启动时直接 Load，不动老路径（无）。
func TestMigrateFromServingDir_LoadsExistingNewPath(t *testing.T) {
	tmpDir := t.TempDir()
	newPath := filepath.Join(tmpDir, ".encv", "mounts.json")
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	userJSON := `{
  "version": 1,
  "mounts": [
    {
      "id": "stable-mount-id",
      "name": "stable-mount",
      "mount_path": "/d/stable",
      "driver": "local",
      "config": {"path": "/tmp/stable"}
    }
  ]
}`
	if err := os.WriteFile(newPath, []byte(userJSON), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := NewRegistry(&stubConfigProvider{servingDir: tmpDir}, newPath)
	r.RegisterDriverFactory(DriverLocal, func() Driver { return &stubDriver{} })

	if err := r.MigrateFromServingDir(context.Background()); err != nil {
		t.Fatalf("MigrateFromServingDir: %v", err)
	}

	got := r.GetByName("stable-mount")
	if got == nil {
		t.Fatalf("stable-mount not loaded")
	}
	if got.ID != "stable-mount-id" {
		t.Errorf("id mismatch: got %q", got.ID)
	}
}

// TestMigrateFromServingDir_IdempotentOnSecondRun 验证连跑两次不会出问题：
// 第一次：老 → 新迁移 + Load
// 第二次：新已存在 → 直接 Load，迁移 no-op
func TestMigrateFromServingDir_IdempotentOnSecondRun(t *testing.T) {
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "mounts.json")
	newPath := filepath.Join(tmpDir, ".encv", "mounts.json")

	userJSON := `{"version":1,"mounts":[{"id":"x","name":"x","mount_path":"/d/x","driver":"local","config":{}}]}`
	if err := os.WriteFile(oldPath, []byte(userJSON), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 第一次跑
	r1 := NewRegistry(&stubConfigProvider{servingDir: tmpDir}, newPath)
	r1.RegisterDriverFactory(DriverLocal, func() Driver { return &stubDriver{} })
	if err := r1.MigrateFromServingDir(context.Background()); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// 老文件应被移走
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("first run should rename legacy, stat err = %v", err)
	}

	// 第二次跑
	r2 := NewRegistry(&stubConfigProvider{servingDir: tmpDir}, newPath)
	r2.RegisterDriverFactory(DriverLocal, func() Driver { return &stubDriver{} })
	if err := r2.MigrateFromServingDir(context.Background()); err != nil {
		t.Fatalf("second run: %v", err)
	}
	// 内存里 mount 应还在（Load 从新路径读出来）
	if r2.GetByName("x") == nil {
		t.Errorf("second run should load from new path")
	}
	// 新路径文件应保持（没被 Bootstrap 覆盖）
	got, _ := os.ReadFile(newPath)
	if len(got) == 0 {
		t.Errorf("new path should not be empty after second run")
	}
}
