package mount

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubConfigProviderLegacy 是 ConfigProvider 的最小测试桩。
// 2026-06-15：legacyDataPathCandidates 依赖 cfg.ServingDir()，必须传非空。
type stubConfigProviderLegacy struct {
	servingDir string
}

func (s *stubConfigProviderLegacy) IsMobile() bool                  { return false }
func (s *stubConfigProviderLegacy) IsDev() bool                     { return false }
func (s *stubConfigProviderLegacy) AndroidPackageName() string      { return "com.test.encv" }
func (s *stubConfigProviderLegacy) DataDir() string                 { return s.servingDir }
func (s *stubConfigProviderLegacy) AppDataFallbackDir() string      { return s.servingDir }
func (s *stubConfigProviderLegacy) DevSandboxDir() string           { return "" }
func (s *stubConfigProviderLegacy) ServingDir() string              { return s.servingDir }
func (s *stubConfigProviderLegacy) AutomationDriver() string        { return "local" }

// TestMigrateLegacyDataPath_RenamesOldToNew 验证从老路径（serving_dir/mounts.json）
// 迁移到新路径（serving_dir/.encv/mounts.json）的核心逻辑。
//
// 场景：
//   - 老路径存在有效 JSON
//   - 新路径不存在
//   - 期望：原子 rename 后新路径存在、老路径消失、内容一致
func TestMigrateLegacyDataPath_RenamesOldToNew(t *testing.T) {
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "mounts.json")
	newPath := filepath.Join(tmpDir, ".encv", "mounts.json")

	// 老路径写一份有效 JSON（即使内容是假 JSON 也不会被验证，只看是否搬过去）
	original := `{"version":1,"mounts":[],"saved_at":"2026-06-15T00:00:00Z"}`
	if err := os.WriteFile(oldPath, []byte(original), 0644); err != nil {
		t.Fatalf("seed old path: %v", err)
	}

	// cfg 传 tmpDir → legacyDataPathCandidates() 会列出 <tmpDir>/mounts.json 和 <tmpDir>/.encv/mounts.json
	r := &MountRegistry{
		dataPath: newPath,
		cfg:      &stubConfigProviderLegacy{servingDir: tmpDir},
	}
	if err := r.migrateLegacyDataPath(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 新路径必须存在 + 内容一致
	got, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read new path: %v", err)
	}
	if string(got) != original {
		t.Errorf("migrated content mismatch: got %q want %q", got, original)
	}
	// 老路径必须消失
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("legacy file should be gone, stat err = %v", err)
	}
}

// TestMigrateLegacyDataPath_SwapsWhenBothExist 验证新 + 老都存在时的 swap 行为。
//
// 2026-06-15 修复根因：原版"新文件存在就跳过"导致老文件永远残留在根目录，被 FileList 列出。
// 新语义：两个文件都存在 → atomic swap —— 老文件的内容（用户真实数据）变成新文件，
// 之前的新文件 rename 到 .migrated-<unix> forensic 备份。
//
// 这里验证 forensic 备份存在 + 备份内容是之前的"新文件"内容。
func TestMigrateLegacyDataPath_SwapsWhenBothExist(t *testing.T) {
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "mounts.json")
	newPath := filepath.Join(tmpDir, ".encv", "mounts.json")

	// 老文件 = 用户真实数据
	if err := os.WriteFile(oldPath, []byte("USER_DATA"), 0644); err != nil {
		t.Fatalf("seed old: %v", err)
	}
	// 新文件 = Bootstrap 默认配置
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("BOOTSTRAP_DATA"), 0644); err != nil {
		t.Fatalf("seed new: %v", err)
	}

	r := &MountRegistry{
		dataPath: newPath,
		cfg:      &stubConfigProviderLegacy{servingDir: tmpDir},
	}
	if err := r.migrateLegacyDataPath(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// 新位置必须是用户数据（不是 Bootstrap）
	gotNew, _ := os.ReadFile(newPath)
	if string(gotNew) != "USER_DATA" {
		t.Errorf("new path should be user data, got %q", gotNew)
	}
	// 老文件必须消失
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("legacy file should be moved, stat err = %v", err)
	}
	// forensic 备份必须存在
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(newPath), "mounts.json.migrated-*"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 migrated backup, got %d: %v", len(matches), matches)
	}
	gotBak, _ := os.ReadFile(matches[0])
	if string(gotBak) != "BOOTSTRAP_DATA" {
		t.Errorf("backup should be old new file content, got %q", gotBak)
	}
}

// TestMigrateLegacyDataPath_NoOpWhenOldAbsent 验证老路径不存在时无副作用。
func TestMigrateLegacyDataPath_NoOpWhenOldAbsent(t *testing.T) {
	tmpDir := t.TempDir()
	newPath := filepath.Join(tmpDir, ".encv", "mounts.json")
	// 老路径不存在
	r := &MountRegistry{
		dataPath: newPath,
		cfg:      &stubConfigProviderLegacy{servingDir: tmpDir},
	}
	if err := r.migrateLegacyDataPath(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// 新路径不应被创建（无数据可搬）
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Errorf("new path should not be created, stat err = %v", err)
	}
	// 也不应创建 .encv 目录
	if _, err := os.Stat(filepath.Dir(newPath)); !os.IsNotExist(err) {
		t.Errorf(".encv dir should not be created, stat err = %v", err)
	}
}

// TestMigrateLegacyDataPath_EmptyDataPath 验证 dataPath 为空时直接返回。
func TestMigrateLegacyDataPath_EmptyDataPath(t *testing.T) {
	r := &MountRegistry{dataPath: ""}
	if err := r.migrateLegacyDataPath(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

// TestMigrateLegacyDataPath_HandlesTildeLikePaths 验证 filepath.Join 跨 OS 一致。
// （dev 在 Linux / Android 在 Linux / Windows dev 可能在 windows 上跑测试）
func TestMigrateLegacyDataPath_HandlesNestedDir(t *testing.T) {
	tmpDir := t.TempDir()
	// 模拟 serving dir = /tmp/xxx/，新路径 = /tmp/xxx/.encv/mounts.json
	servingDir := filepath.Join(tmpDir, "subdir")
	if err := os.MkdirAll(servingDir, 0755); err != nil {
		t.Fatalf("mkdir serving: %v", err)
	}
	oldPath := filepath.Join(servingDir, "mounts.json")
	newPath := filepath.Join(servingDir, ".encv", "mounts.json")
	if err := os.WriteFile(oldPath, []byte("CONTENT"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := &MountRegistry{
		dataPath: newPath,
		cfg:      &stubConfigProviderLegacy{servingDir: servingDir},
	}
	if err := r.migrateLegacyDataPath(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("new path missing: %v", err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old path should be gone, err = %v", err)
	}
}

// TestLoad_TriggersMigration 验证 Load() 在新路径不存在 + 老路径存在时自动迁移。
//
// 这是 2026-06-15 用户反馈"mounts.json 出现在文件列表"的端到端验证：
// Load() 应在解析前把老路径数据搬到隐藏子目录，后续的 Save() 写到新路径。
func TestLoad_TriggersMigration(t *testing.T) {
	tmpDir := t.TempDir()
	oldPath := filepath.Join(tmpDir, "mounts.json")
	newPath := filepath.Join(tmpDir, ".encv", "mounts.json")
	// 老路径写一份空 mounts（不是新文件格式 ok，Load 解析失败会返回 error，
	// 但迁移本身是独立于解析的，所以先验迁移成功，再看 Load 错误）
	original := `{"version":1,"mounts":[]}`
	if err := os.WriteFile(oldPath, []byte(original), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := &MountRegistry{
		dataPath: newPath,
		mounts:   nil,
		byID:     make(map[string]*Mount),
		byName:   make(map[string]*Mount),
		drivers:  make(map[string]DriverFactory),
		cfg:      &stubConfigProviderLegacy{servingDir: tmpDir},
	}
	// Load 内的解析会因为没有 driver 注册而失败，但这不是我们要测的；
	// 我们只关心迁移已经发生（老路径消失 + 新路径有数据）
	_ = r.Load()
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("legacy should be gone after Load, stat err = %v", err)
	}
	got, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("new path missing after Load: %v", err)
	}
	if !strings.Contains(string(got), "version") {
		t.Errorf("migrated content unexpected: %q", got)
	}
}
