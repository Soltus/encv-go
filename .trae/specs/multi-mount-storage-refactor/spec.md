# 多挂载点存储系统重构 (multi-mount-storage-refactor)

> **状态**: 📝 Draft（待用户审批）
> **创建**: 2026-06-15
> **触发**: 自动化测试 mockRoot 路径不可靠（Android scoped storage / 多用户 / dev 沙箱）+ 用户要求按 OpenList 风格重构
> **目标读者**: 后端 / 前端 / 测试

---

## 1. 背景与痛点（Context）

### 1.1 现状：单根 `servingDir` 架构

整个 encv-go 后端目前**只有一个存储根**：

```go
// internal/config/config.go:298-302
ServingDir string `json:"serving_dir"`   // ← 全局唯一
```

所有路径（文件浏览、加密源、输出、mock 数据、ffmpeg scratch）都从这个根派生：

- `internal/service/mobile_service.go` — 30+ 处使用 `s.servingDir` 做 `SafeResolveToAbsPath`
- `internal/service/task_manager*.go` — 加密/解密任务从 `servingDir` 派生源/输出
- `internal/server/mock_generator.go` — mock 数据写到 `mockRoot`（白名单 /storage/emulated/0/encv-automation 等）
- `internal/server/mobile_api.go:210` — service-guard 硬编码 `expectedDir := "/storage/emulated/0"`
- `app/encv-mobile/src/composables/usePathResolver.ts:17-18` — 前端 `withSafetyBoundary` 硬编码 `REAL_STORAGE_ROOT = '/storage/emulated/0'`、`SAFETY_NAMESPACE = 'encv-automation'`

### 1.2 4 个具体痛点

| # | 痛点 | 现象 | 触发 |
|---|------|------|------|
| **P1** | **Android 11+ scoped storage 写入失败** | Capacitor app 没有 `MANAGE_EXTERNAL_STORAGE`，写 `/storage/emulated/0/encv-automation/...` 触发 `EACCES` | 真机 release 构建 |
| **P2** | **多用户 remap 失败** | `/storage/emulated/0/` 是 uid=0 专用；uid=10 看到 `/storage/emulated/10/`（FUSE remap 易失败） | 工作配置 / 平板多用户模式 |
| **P3** | **dev 沙箱 ≠ 真机** | 沙箱里的 `/storage/emulated/0/` 是 dev 目录，task 找不到 mock 写的位置（"source file not found"） | 沙箱环境 |
| **P4** | **"安全边界"只在客户端** | `withSafetyBoundary` 把用户路径改写到 `encv-automation/` 命名空间，但后端**没有这个概念**——它只看到一个绝对路径，无法强制测试数据进独立命名空间 | 任何 release 自动化运行 |

### 1.3 OpenList 参考实现

[Hi-Sillot-OpenList-Frontend](https://github.com/Hi-Sillot/Hi-Sillot-OpenList-Frontend) 抽象了：

```ts
interface Storage {
  id: string             // 唯一 ID
  mountPath: string      // 虚拟挂载路径（URL 内可见）
  driver: string         // 'local' / 's3' / 'google_drive' / ...
  config: object         // driver-specific 配置
  enabled: boolean
  readOnly: boolean
  // ...
}
```

URL 形式 `/d/<mountPath>/<subPath>` 路由到对应 driver 的文件 API。增加/删除/编辑挂载点无需重启服务。

### 1.4 为什么"简单改 mockRoot"治标不治本

只改 mockRoot 到 `os.TempDir()` 派生：
- ✅ 短期能跑
- ❌ 用户文件（"/我的视频"）依然走 `/storage/emulated/0/`，依然 P1/P2
- ❌ 自动化测试数据**和**用户数据**依然共用一个根**，需要客户端 `withSafetyBoundary` 拼命运作
- ❌ 未来要加 S3 / WebDAV / SMB / OpenList bridge 都没地方挂

正确做法是**把"挂载点"做成第一类概念**。

---

## 2. 目标（Goals）

### G1. 多挂载点
后端维护一个**有序挂载点列表**，每个挂载点有独立根路径、driver、可读写、启用状态。前端可见、可（部分）增删。

### G2. 内置 driver 至少覆盖现有场景
- `local` — 本地文件系统（替代现有 `servingDir`）
- `appdata` — Android app-private dir（`/data/user/<uid>/<pkg>/files/`），多用户安全
- `sandbox` — dev/sandbox 默认（指向 dev 工作区），仅 dev 模式启用

### G3. 向后兼容（migration 友好）
- 单 `servingDir` 配置文件**不破**：自动迁移为 1 个名为 `primary` 的 `local` driver 挂载点
- 旧 API（`/api/files?path=/foo`）继续工作，内部走 `primary` 挂载点

### G4. 自动化测试走独立挂载点
- 内置一个 `automation` 挂载点（driver=`appdata`），永不与用户数据混合
- 关闭 `withSafetyBoundary` 客户端改写——后端自己知道哪些是测试路径
- 真机 release：默认 `automation` 写到 `/data/user/<uid>/com.encvgo.app/files/encv-automation/`

### G5. 前端可见、可读、不可乱删
- `/api/mounts` 列出所有挂载点
- 前端"设置"页可读挂载点信息（路径、driver、剩余空间）
- 增删需要明确确认（防误删 primary）

### G6. 性能不退化
- 路径解析从 O(N)（遍历所有挂载点）降为 O(1)（基于 mountPath 前缀的 trie/前缀树）
- 1M+ 任务路径仍然 < 1ms 解析

---

## 3. 非目标（Non-Goals）

| # | 不做 | 原因 |
|---|------|------|
| **N1** | 不实现 S3 / WebDAV / SMB 等远程 driver | 范围爆炸，本期只换骨架 + 3 个本地 driver |
| **N2** | 不做挂载点的热迁移（mount 在线迁移数据） | 需要后台 worker；本期只换"路径层" |
| **N3** | 不做用户配额（quota） | driver 抽象里留 hook，quota 留 v2 |
| **N4** | 不改 OpenList AAR 本身 | 我们复用 encv-go 自己的后端，OpenList 仍走 plugin |
| **N5** | 不动 `plugin-openlist` 集成 | 那是另一条线（`/d/<openlist-path>/...` 走 OpenList AAR），跟本期平行的"外部挂载 driver"——留到 v2 |

---

## 4. 设计（Design）

### 4.1 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│ Frontend (Ionic Vue)                                             │
│ ┌──────────────┐  ┌──────────────┐  ┌────────────────────────┐  │
│ │ Files.vue    │  │ NewTaskModal │  │ Settings/Mounts.vue 🆕 │  │
│ └──────┬───────┘  └──────┬───────┘  └──────────┬─────────────┘  │
│        └─────────────────┼─────────────────────┘                │
│                          ▼                                      │
│                  useMountResolver 🆕                            │
│                  (instead of usePathResolver.withSafetyBoundary)│
└──────────────────────────┬──────────────────────────────────────┘
                           │ HTTP /api/mounts/*
                           ▼
┌─────────────────────────────────────────────────────────────────┐
│ Backend (Go)                                                     │
│ ┌──────────────────────────────────────────────────────────────┐ │
│ │ MountRegistry 🆕 (internal/mount/)                            │ │
│ │  - mounts: []*Mount (ordered, sorted by mountPath length)    │ │
│ │  - Resolve(virtualPath string) → (mount, absPath)            │ │
│ │  - List / Get / Create / Update / Delete                      │ │
│ │  - Trie-based O(1) prefix lookup                              │ │
│ └──────────────────────────────────────────────────────────────┘ │
│              ▲                ▲                ▲               │
│              │                │                │               │
│   ┌──────────┴──┐   ┌─────────┴───┐   ┌────────┴────────┐     │
│   │ Driver:local │   │ Driver:appdata│ │ Driver:sandbox │     │
│   │ (FS ops)     │   │ (FS + uid)   │   │ (FS + dev only)│     │
│   └──────────────┘   └───────────────┘   └─────────────────┘     │
│              ▲                ▲                ▲               │
│              └────────────────┼────────────────┘               │
│                               │                                 │
│   ┌───────────────────────────┴──────────────────────────────┐ │
│   │ Existing services (mobile_service / task_manager /        │ │
│   │ mock_generator / mobile_api)                              │ │
│   │  - 旧代码: SafeResolveToAbsPath(servingDir, path)        │ │
│   │  - 新代码: mountRegistry.Resolve(virtualPath)             │ │
│   └──────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### 4.2 挂载点配置

```go
// internal/mount/mount.go
type Mount struct {
    ID          string            `json:"id"`           // uuid
    Name        string            `json:"name"`         // "primary" / "automation" / "sandbox"
    MountPath   string            `json:"mount_path"`   // virtual URL prefix, e.g. "/primary", "/automation"
    Driver      string            `json:"driver"`       // "local" | "appdata" | "sandbox"
    RootPath    string            `json:"root_path"`    // absolute FS path
    Enabled     bool              `json:"enabled"`
    ReadOnly    bool              `json:"read_only"`
    DriverConfig map[string]any   `json:"driver_config,omitempty"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}

type MountRegistry struct {
    mounts    []*Mount           // sorted by len(MountPath) DESC for prefix matching
    trie      *pathTrie          // O(1) prefix lookup
    byID      map[string]*Mount
    cfg       *config.Config     // to read android uid / sandbox root / etc.
}
```

### 4.3 Driver 接口

```go
// internal/mount/driver.go
type Driver interface {
    // Init once at startup; ResolveRoot() returns the absolute path for this mount's root.
    Init(ctx context.Context, cfg *config.Config) error
    ResolveRoot() string

    // FS operations (stat, read, write, list, mkdir, delete, rename) — same as existing utils
    // But the path passed in is RELATIVE to mount root, not absolute.
    Stat(relPath string) (os.FileInfo, error)
    ReadDir(relPath string) ([]os.DirEntry, error)
    ReadFile(relPath string) ([]byte, error)
    WriteFile(relPath string, data []byte, perm os.FileMode) error
    MkdirAll(relPath string, perm os.FileMode) error
    Remove(relPath string) error

    // Permission check (e.g., appdata driver checks the app has access)
    CheckPermission() error
}
```

### 4.4 3 个内置 driver

#### 4.4.1 `local` (replaces servingDir)
```go
type LocalDriver struct{ root string }

func (d *LocalDriver) Init(ctx, cfg) error {
    d.root = cfg.ServingDir   // ← 唯一引用旧 servingDir 的地方
    return nil
}
```

#### 4.4.2 `appdata` (new)
```go
type AppDataDriver struct{ root, subpath string }

func (d *AppDataDriver) Init(ctx, cfg) error {
    // Android: /data/user/<uid>/<package>/files/<subpath>
    //   uid = os.Getuid() (Linux); 10000 = first app's uid
    //   package = "com.encvgo.app" (从 build config 注入)
    // Dev/sandbox: ~/Library/Caches/encv-appdata/<subpath>  (or $XDG_CACHE_HOME/encv-appdata)
    uid := os.Getuid()
    pkg := cfg.AndroidPackageName  // 仅 Android 真机有效
    if runtime.GOOS == "android" {
        d.root = fmt.Sprintf("/data/user/%d/%s/files/%s", uid, pkg, d.subpath)
    } else {
        d.root = filepath.Join(cfg.AppDataFallbackDir, d.subpath)
    }
    return os.MkdirAll(d.root, 0755)
}
```

#### 4.4.3 `sandbox` (new, dev only)
```go
type SandboxDriver struct{ root string }

func (d *SandboxDriver) Init(ctx, cfg) error {
    if !import_meta_env_DEV() && runtime.GOOS != "linux" {
        return errors.New("sandbox driver only enabled in dev mode")
    }
    d.root = cfg.DevSandboxDir  // e.g., $WORKSPACE/.sandbox
    return nil
}
```

### 4.5 路径解析

```go
// Resolve takes a virtual path like "/primary/foo/bar.mp4" and returns
// the (mount, absPath) pair. O(log N) via trie.
func (r *MountRegistry) Resolve(virtualPath string) (*Mount, string, error) {
    if !strings.HasPrefix(virtualPath, "/") {
        return nil, "", errors.New("virtual path must be absolute")
    }
    mount, rel := r.trie.LongestPrefix(virtualPath)
    if mount == nil {
        return nil, "", fmt.Errorf("no mount matches %q", virtualPath)
    }
    // Safety check: resolved path stays inside mount root
    abs := filepath.Join(mount.RootPath, rel)
    if !strings.HasPrefix(abs, mount.RootPath+string(filepath.Separator)) && abs != mount.RootPath {
        return nil, "", errors.New("path escapes mount root")
    }
    return mount, abs, nil
}
```

### 4.6 启动时自动配置（migration）

```go
// internal/mount/bootstrap.go
func (r *MountRegistry) BootstrapFromConfig(cfg *config.Config) error {
    // 1. 如果 cfg.ServingDir != "" 且没有名为 "primary" 的 mount → 创建
    if cfg.ServingDir != "" {
        existing := r.GetByName("primary")
        if existing == nil {
            r.Create(&Mount{
                ID: uuid.New(), Name: "primary", MountPath: "/primary",
                Driver: "local", RootPath: cfg.ServingDir,
                Enabled: true, ReadOnly: false,
            })
        }
    }

    // 2. 如果在 Android (GOOS=android 或 cfg.IsMobile) 且没有 "automation" mount → 创建
    if cfg.IsMobile || runtime.GOOS == "android" {
        existing := r.GetByName("automation")
        if existing == nil {
            r.Create(&Mount{
                ID: uuid.New(), Name: "automation", MountPath: "/automation",
                Driver: "appdata", DriverConfig: map[string]any{"subpath": "encv-automation"},
                Enabled: true, ReadOnly: false,
            })
        }
    }

    // 3. dev 模式加 sandbox mount
    if import_meta_env_DEV() {
        r.Create(&Mount{
            ID: uuid.New(), Name: "sandbox", MountPath: "/sandbox",
            Driver: "sandbox", Enabled: true, ReadOnly: false,
        })
    }

    return nil
}
```

### 4.7 持久化

挂载点配置持久化到 `$DATA_DIR/mounts.json`：

```go
func (r *MountRegistry) Save() error {
    data, _ := json.MarshalIndent(r.mounts, "", "  ")
    return os.WriteFile(filepath.Join(r.cfg.DataDir, "mounts.json"), data, 0644)
}

func (r *MountRegistry) Load() error {
    data, err := os.ReadFile(filepath.Join(r.cfg.DataDir, "mounts.json"))
    if os.IsNotExist(err) { return nil }  // 首次启动，无持久化配置
    if err != nil { return err }
    var mounts []*Mount
    json.Unmarshal(data, &mounts)
    for _, m := range mounts { r.mounts = append(r.mounts, m) }
    r.rebuildTrie()
    return nil
}
```

### 4.8 虚拟路径格式

URL 形式从 `/<absolute-path>` 改为 `/d/<mount-path>/<sub-path>`：

```
旧:  /storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4
新:  /d/automation/01-plain-media/video/sample.mp4
```

但**保留** `/api/files?path=/foo` 兼容旧格式：
- 如果 path 以 `/d/...` 开头 → 走新 mount 解析
- 否则 → 默认挂载在 `primary` mount 下

---

## 5. 数据模型（Data Model）

### 5.1 `mounts.json` 示例

```json
{
  "version": 1,
  "mounts": [
    {
      "id": "uuid-primary-xxxx",
      "name": "primary",
      "mount_path": "/primary",
      "driver": "local",
      "root_path": "/storage/emulated/0",
      "enabled": true,
      "read_only": false,
      "created_at": "2026-06-15T10:00:00Z",
      "updated_at": "2026-06-15T10:00:00Z"
    },
    {
      "id": "uuid-automation-xxxx",
      "name": "automation",
      "mount_path": "/automation",
      "driver": "appdata",
      "root_path": "/data/user/0/com.encvgo.app/files/encv-automation",
      "enabled": true,
      "read_only": false,
      "driver_config": { "subpath": "encv-automation" },
      "created_at": "2026-06-15T10:00:00Z",
      "updated_at": "2026-06-15T10:00:00Z"
    }
  ]
}
```

### 5.2 与旧 `config.json` 的关系

| 旧字段 | 新映射 |
|--------|--------|
| `config.serving_dir` | 自动迁移为 `primary` mount；保留作为 `LocalDriver.root` 来源 |
| `config.is_mobile` (mobile overlay) | 影响 `automation` mount 是否自动创建 |
| `config.data_dir` | `mounts.json` 存放位置 |

---

## 6. API（API Surface）

### 6.1 新增端点

```
GET    /api/mounts                      # 列出所有挂载点
GET    /api/mounts/:id                  # 单个挂载点详情
POST   /api/mounts                      # 新增（需 admin）
PUT    /api/mounts/:id                  # 更新（需 admin）
DELETE /api/mounts/:id                  # 删除（需 admin，且非 primary）
POST   /api/mounts/:id/resolve          # 把 virtual path → abs path（debug）
GET    /api/mounts/:id/usage            # 占用空间（du）
```

### 6.2 修改端点（向后兼容）

```
GET  /api/files?path=/d/automation/01-plain-media/video/sample.mp4
                                       # 新格式
GET  /api/files?path=/foo/bar           # 旧格式 → 走 primary mount
POST /api/mock/generate   { root: "/d/automation" }
                                       # 旧 root="/storage/emulated/0/encv-automation"
                                       # 自动改写为 mount_path
```

### 6.3 WS 事件

```
mount:created    { mount }
mount:updated    { mount }
mount:deleted    { id }
```

---

## 7. 迁移（Migration）

### 7.1 数据迁移

```go
// internal/mount/migrate.go
func MigrateFromServingDir(cfg *config.Config) error {
    // Step 1: 读 cfg.ServingDir
    // Step 2: 如果 mounts.json 不存在 → 调 BootstrapFromConfig 自动创建 primary
    // Step 3: 写一份 migration-backup.json 到 cfg.DataDir（防回滚）
    return nil
}
```

### 7.2 代码迁移（增量，不要一次全改）

**Phase A** — 骨架 + 向后兼容（不影响功能）：
- A1. 新建 `internal/mount/` 包
- A2. 加 `MountRegistry` + 3 个 driver 实现
- A3. 加 `/api/mounts` CRUD
- A4. **不改**现有 service 代码（mobile_service / task_manager / mock_generator）
- A5. **不改**现有 config / servingDir
- Validation: 现有 Go 测试全过；新增 `mount_registry_test.go` 测 mount 解析 + 持久化

**Phase B** — mock 切到 automation mount（验证模式）：
- B1. `mock_generator.go` 的 `mockRootAllowList` 替换为"必须以 `/d/automation/` 开头"
- B2. `mobile_api.go:210` 替换 `expectedDir` 为 `mountRegistry.GetByName("primary").RootPath`
- B3. `mobile_service.go:186` 同样替换
- B4. 前端 `useAutomationTests.ts:92` 的 `DEFAULT_AUTOMATION_SOURCE` 改为 `/d/automation/01-plain-media/video/sample.mp4`
- B5. `usePathResolver.withSafetyBoundary` **保留但降级为 no-op**（migration 期兼容）
- Validation: 真机 release 跑自动化测试，文件落到 appdata 不再 EACCES

**Phase C** — task_manager 切到 mount：
- C1. `task_manager.go` 的 `servingDir` 字段替换为"任务里直接存 mount_id + virtual_path"
- C2. 任务提交 API 接受 `sourcePath = "/d/automation/..."` 形式
- C3. `task_manager_state_test.go` 更新 fixture
- Validation: 跑通端到端：创建任务 → 读源文件 → 加密 → 写输出

**Phase D** — mobile_service 切到 mount（最大面积）：
- D1. `/api/files` 接受 `/d/<mount>/...` 形式，旧形式自动 rewrite 到 primary
- D2. 30+ 处 `SafeResolveToAbsPath(servingDir, path)` 替换为 `mountRegistry.Resolve(path)`
- D3. `/api/files` 列表响应里给每个文件加 `mount_id` 字段
- Validation: 文件浏览 + 上传 + 下载全过

**Phase E** — 前端 UI（Settings/Mounts.vue）：
- E1. 新建 `Settings/Mounts.vue`，显示挂载点列表（name / mount_path / driver / root_path / usage）
- E2. 提供"刷新挂载点"按钮（重建 trie）
- E3. "添加挂载点"按钮（仅在 dev mode 显示）
- Validation: Settings 页能列出挂载点

**Phase F** — 清理 + 删除旧代码：
- F1. 移除 `cfg.ServingDir`（仍读取作为迁移入口，但代码层不再引用）
- F2. 移除 `usePathResolver.withSafetyBoundary`（所有调用方已切到 mount 路径）
- F3. 移除 `mockRootAllowList` 硬编码白名单
- F4. 移除 `mobile_api.go:210` 硬编码 `expectedDir`
- Validation: `grep -r "storage/emulated" internal/` 仅在 mount bootstrap / driver 实现里出现

### 7.3 前端路径层迁移

```ts
// 旧 (usePathResolver.ts)
const safePath = withSafetyBoundary(rawPath, { forceAutomation: true })
// safePath = "/storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4"

// 新
import { useMountResolver } from '@/composables/useMountResolver'
const { resolve } = useMountResolver()
const taskPath = resolve({
  mount: 'automation',     // 业务意图
  subPath: '01-plain-media/video/sample.mp4',
})
// taskPath = "/d/automation/01-plain-media/video/sample.mp4"
```

前端**永远不**直接拼绝对路径，永远用 `mount + subPath` 形式拼 → mount 注册表返回的 root 决定真实路径。

---

## 8. 影响面（Impact）

### 8.1 后端

| 文件 | 改动 |
|------|------|
| `internal/mount/mount.go` 🆕 | Mount 数据结构 |
| `internal/mount/registry.go` 🆕 | MountRegistry + 解析 + 持久化 |
| `internal/mount/driver.go` 🆕 | Driver 接口 |
| `internal/mount/drivers/local.go` 🆕 | LocalDriver |
| `internal/mount/drivers/appdata.go` 🆕 | AppDataDriver |
| `internal/mount/drivers/sandbox.go` 🆕 | SandboxDriver |
| `internal/mount/bootstrap.go` 🆕 | 启动时从 cfg 创建默认 mount |
| `internal/mount/migrate.go` 🆕 | 从旧 config 迁移 |
| `internal/mount/mount_test.go` 🆕 | 单元测试 |
| `internal/server/mobile_api.go` | 改 Phase B2 |
| `internal/server/mock_generator.go` | 改 Phase B1 |
| `internal/service/mobile_service.go` | 改 Phase D2（30+ 处） |
| `internal/service/task_manager.go` | 改 Phase C1 |
| `internal/config/config.go` | 改 Phase F1（删 ServingDir 字段） |
| `internal/server/server.go` | 挂载 MountRegistry 到 server 上下文 |

### 8.2 前端

| 文件 | 改动 |
|------|------|
| `src/composables/useMountResolver.ts` 🆕 | Mount 解析（替代 usePathResolver 的相关部分） |
| `src/composables/useAutomationTests.ts` | 改 Phase B4 |
| `src/views/WorkflowDashboard.vue` | 改 Phase B4 |
| `src/views/AutomationTestsDetail.vue` | 改 Phase B4 |
| `src/views/Settings/Mounts.vue` 🆕 | Phase E1 |
| `src/composables/usePathResolver.ts` | 改 Phase B5 → Phase F2 |

### 8.3 已有 spec / 文档

- `unify-path-resolver` spec — **吸收**：本期替代之，标 done
- `mock-router-refactor` spec — **联动**：mock router 接受 `/d/automation/...` 形式
- `wire-openlist-runtime-and-ui-v2` spec — **不影响**：OpenList 走另一条线
- `.trae/rules/development.md` §六 WAF 双重编码 — **影响**：双重编码的 path 现在是 `/d/<mount>/<encoded-encoded-subpath>`

---

## 9. 测试策略（Test Plan）

### 9.1 单元测试

| 测试 | 覆盖 |
|------|------|
| `mount_registry_test.go::TestResolve` | 4 类路径：匹配/不匹配/逃逸/二级 mount |
| `mount_registry_test.go::TestPersistence` | mounts.json 读写 + 重建 trie |
| `mount_registry_test.go::TestBootstrap` | 从 cfg.ServingDir 创建 primary mount |
| `drivers/local_test.go` | LocalDriver 真实 FS 操作 |
| `drivers/appdata_test.go` | AppDataDriver 在 sandbox 下走 fallback 路径 |
| `drivers/appdata_android_test.go` 🆕 | 集成测试（build tag `//go:build android`） |

### 9.2 集成测试

- `internal/server/mount_api_test.go` — POST /api/mounts 流程
- `internal/server/mock_generator_test.go` — 用 `/d/automation/...` 走通 mock 全流程
- `internal/service/task_manager_test.go` — 任务用 mount 路径创建

### 9.3 E2E（vitest + 模拟设备）

- `app/encv-mobile/__tests__/useMountResolver.test.ts` 🆕
  - mock `/api/mounts` 返回 2 个 mount
  - 测 `resolve({mount: 'automation', subPath: 'foo.mp4'})` → `/d/automation/foo.mp4`
  - 测 `resolveByPath('/d/automation/foo.mp4')` → `{mount: 'automation', subPath: 'foo.mp4'}`
  - 测 404 场景（mount 不存在）

### 9.4 真机 release 验证

- 在 Capacitor 真机上：
  - 设置 page 列出 2 个 mount（primary + automation）
  - 跑一次自动化测试：mock 数据落 `/data/user/0/com.encvgo.app/files/encv-automation/01-plain-media/...`
  - **不出现 EACCES**
  - **不出现 "source file not found"**（task 读 mount 解析后的路径，与 mock 写路径一致）
- 多用户真机：uid=10 时 mount path 解析为 `/data/user/10/com.encvgo.app/files/encv-automation/...`

### 9.5 性能验证

- 1M+ 任务 sourcePath 批量解析 < 100ms（实测 trie lookup）
- `/api/mounts` 响应 < 10ms
- 持久化加载 < 50ms

### 9.6 回归验证

- 现有 `__tests__/DevLogs.autoScroll.test.ts` 14 个测试**全过**
- 现有 `__tests__/useAutomationTests.test.ts` 全过（可能需要更新 mock `/api/mounts`）
- 现有 Go test suite 全过

---

## 10. 风险与回滚（Risks & Rollback）

### 10.1 风险

| 风险 | 概率 | 缓解 |
|------|------|------|
| Phase D 改 mobile_service 30+ 处时漏改 | 中 | Phase A 保留旧 `servingDir` API；Phase B 跑通后再动 D；D 阶段 PR review 必查 |
| trie 性能不如前缀树 | 低 | 用 radix tree；Benchmark `Resolve` vs 现有 `SafeResolveToAbsPath` |
| AppDataDriver 在 dev 沙箱行为与真机不一致 | 中 | 沙箱 fallback 用 `$XDG_CACHE_HOME/encv-appdata` 模拟；真机集成测试 build tag 单独跑 |
| 客户端 `withSafetyBoundary` 漏切导致旧路径打到后端 | 低 | Phase B5 仍保留 `withSafetyBoundary` 但降级为 no-op；Phase F2 才删 |
| 持久化 `mounts.json` 损坏导致服务启动失败 | 低 | `Load()` 出错时回退到 `BootstrapFromConfig`；写 `migration-backup.json` |

### 10.2 回滚

- 数据：删 `mounts.json` → 重启 → 自动 bootstrap
- 代码：git revert Phase 内的 commit（按 phase 分 commit）
- 配置：临时改 `cfg.ServingDir` + 删 `mounts.json` → 旧逻辑生效

### 10.3 不做回滚的场景

- Phase A 之后：`servingDir` 仍可读，向后兼容完整
- Phase B 之后：`mock_generator` 行为变更，但前端默认 sourcePath 已切
- Phase C/D 之后：API 形式变更，旧 API 已 rewrite

---

## 11. 里程碑（Milestones）

| Milestone | Phase | 验收 |
|-----------|-------|------|
| **M1** (骨架可用) | A | 新增 `internal/mount/` + 3 driver + `/api/mounts` CRUD + 单元测试；旧代码 0 改动 |
| **M2** (mock 切到 mount) | A + B | 真机 release 跑通自动化测试，无 EACCES；旧 API 仍工作 |
| **M3** (task 切到 mount) | A + B + C | 任务创建/读/写全部走 mount；旧 `servingDir` 仅 bootstrap 读 |
| **M4** (file browser 切到 mount) | A + B + C + D | 文件浏览/上传/下载支持多 mount，列表显示 mount 信息 |
| **M5** (前端 UI) | A-F | Settings/Mounts.vue 显示挂载点列表 |
| **M6** (清理) | F | 删旧代码；`grep -r "storage/emulated" internal/` 仅在 mount bootstrap / driver |

---

## 12. 开放问题（Open Questions）

> **需用户在 spec 审批时一并答复**

### Q1. 是否保留 `cfg.ServingDir` 字段作为兼容？
- 选项 A：保留到 Phase F，期间用作 bootstrap 输入
- 选项 B：Phase A 就移除，强制用户迁到 `mounts.json`
- **推荐 A**：回滚友好，迁移平滑

### Q2. 自动化测试数据是否**完全**走 appdata（不可见）？
- 选项 A：是（完全 appdata，真机文件管理器看不到）— **推荐**，更安全
- 选项 B：给个"真机可见"开关（写到 `/storage/emulated/0/...`，但要求用户授权）

### Q3. 多挂载点的 driver 抽象深度？
- 选项 A：本期只做 3 个本地 driver（S3/WebDAV 等 v2） — **推荐**
- 选项 B：本期就做 5+ driver（含 S3 骨架）

### Q4. UI 增删挂载点的权限？
- 选项 A：所有登录用户都能增删 — **不推荐**
- 选项 B：仅 admin — **推荐**
- 选项 C：dev mode 全开放，release 锁死

### Q5. `/d/...` 路径格式是否对外暴露？
- 选项 A：暴露在 URL（HTTP API 用 `/d/<mount>/...`）— **推荐**（OpenList 风格）
- 选项 B：仅内部，URL 仍用 `/api/files?path=<absolute>`

### Q6. 任务存储的 `source_path` 字段是 mount path 还是 absolute？
- 选项 A：mount path（`/d/automation/foo`）— **推荐**，与 mount 配置解耦
- 选项 B：absolute（绑定到当前 RootPath）— 不推荐，迁移困难

---

## 13. 文档附录

### A. 参考
- [Hi-Sillot-OpenList-Frontend](https://github.com/Hi-Sillot/Hi-Sillot-OpenList-Frontend) — mount 抽象
- [.trae/specs/unify-path-resolver/](../unify-path-resolver/spec.md) — 已废弃，吸收进本期
- [.trae/specs/wire-openlist-runtime-and-ui-v2/](../wire-openlist-runtime-and-ui-v2/spec.md) — OpenList 集成（平行业务线）
- [OpenList 文档](https://openlist.team/) — mount API 风格

### B. 关键文件清单

- `internal/mount/*` 🆕
- `internal/server/mock_generator.go` — Phase B
- `internal/server/mobile_api.go` — Phase B
- `internal/service/mobile_service.go` — Phase D
- `internal/service/task_manager.go` — Phase C
- `app/encv-mobile/src/composables/useMountResolver.ts` 🆕
- `app/encv-mobile/src/composables/usePathResolver.ts` — Phase F2 删除
- `app/encv-mobile/src/views/Settings/Mounts.vue` 🆕

### C. 命名约定

- mount name: `primary`, `automation`, `sandbox` (本期内置)；用户自定义 name 用 slug
- mount path: 必须以 `/` 开头，不含 `..`，唯一
- driver: 小写连字符 (`local`, `appdata`, `sandbox`)
