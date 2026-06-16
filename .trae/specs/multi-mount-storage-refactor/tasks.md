# Tasks: multi-mount-storage-refactor

> 与 spec.md + checklist.md 配套的执行任务。每个任务有明确 owner（后端 / 前端 / 全栈）+ 验收标准。
>
> 命名规则：
> - 后端任务 = 改 `internal/`
> - 前端任务 = 改 `app/encv-mobile/src/`
> - 全栈任务 = 跨端，需要两边都改

---

## 阶段 A：骨架 + 向后兼容（不动现有功能）

> **Owner**: 后端
> **目标**: `internal/mount/` 完整可用 + `/api/mounts` CRUD + 单元测试 + 现有 0 改动
> **预计**: 5-7 天
> **前置**: 无

### A1. Mount 数据结构 + JSON 序列化
- **文件**: `internal/mount/mount.go` 🆕
- **内容**: `type Mount struct { ID, Name, MountPath, Driver, RootPath, Enabled, ReadOnly, DriverConfig, CreatedAt, UpdatedAt }`
- **验收**:
  - `go vet ./internal/mount/...` 通过
  - JSON tag 正确（snake_case）

### A2. MountRegistry + Trie 解析
- **文件**: `internal/mount/registry.go` 🆕
- **内容**: 
  - `type MountRegistry struct { mounts []*Mount; trie *pathTrie; byID map[string]*Mount; mu sync.RWMutex }`
  - `func (r *MountRegistry) Resolve(virtualPath string) (*Mount, string, error)`
  - `func (r *MountRegistry) List() []*Mount`
  - `func (r *MountRegistry) GetByID(id string) *Mount`
  - `func (r *MountRegistry) GetByName(name string) *Mount`
  - `func (r *MountRegistry) Create(m *Mount) error`
  - `func (r *MountRegistry) Update(m *Mount) error`
  - `func (r *MountRegistry) Delete(id string) error`
- **验收**:
  - `Resolve` 在 trie 上 O(log N)
  - 路径安全检查（escapes mount root 报错）
  - 单元测试 4 类路径

### A3. Driver 接口 + 3 个实现
- **文件**: `internal/mount/driver.go` 🆕 + `internal/mount/drivers/{local,appdata,sandbox}.go` 🆕
- **内容**:
  - `type Driver interface { Init/ResolveRoot/Stat/ReadDir/ReadFile/WriteFile/MkdirAll/Remove/CheckPermission }`
  - `LocalDriver{root string}` — 纯 FS
  - `AppDataDriver{root, subpath string}` — Android `/data/user/<uid>/<pkg>/files/<subpath>` + dev fallback
  - `SandboxDriver{root string}` — dev only（`!isDev` 报错）
- **验收**:
  - 3 driver 都实现接口
  - 单元测试覆盖 Stat/ReadDir/ReadFile/WriteFile
  - `appdata` 在 sandbox 下走 `$XDG_CACHE_HOME/encv-appdata/<subpath>`

### A4. Bootstrap + Migrate
- **文件**: `internal/mount/bootstrap.go` 🆕 + `internal/mount/migrate.go` 🆕
- **内容**:
  - `BootstrapFromConfig(cfg)` — 读 `cfg.ServingDir` 创建 primary；mobile/Android 创建 automation；dev 创建 sandbox
  - `MigrateFromServingDir(cfg)` — 检查 `mounts.json` 是否存在；不存在则调 Bootstrap
- **验收**:
  - 启动时如果 cfg.ServingDir != "" → 创建 primary
  - 启动时如果 isMobile → 创建 automation
  - 启动时如果 isDev → 创建 sandbox
  - 已存在的 mount 不覆盖

### A5. 持久化 Load/Save
- **文件**: `internal/mount/registry_persist.go` 🆕
- **内容**: 读 `$DATA_DIR/mounts.json`；写回；Load 失败回退到 Bootstrap
- **验收**:
  - 启动时 Load；mount 列表正确恢复
  - 修改 mount 后 Save；重启 mount 状态保留
  - mounts.json 损坏 → 回退到 Bootstrap（不 panic）

### A6. HTTP API CRUD
- **文件**: `internal/server/mount_api.go` 🆕
- **端点**:
  - `GET /api/mounts`
  - `GET /api/mounts/:id`
  - `POST /api/mounts` （admin 校验）
  - `PUT /api/mounts/:id` （admin 校验）
  - `DELETE /api/mounts/:id` （admin 校验；primary 不可删）
  - `POST /api/mounts/:id/resolve` （debug：virtual → abs）
  - `GET /api/mounts/:id/usage` （du 占用）
- **验收**:
  - 7 个端点全部 HTTP 测试通过
  - 权限校验：admin token 才可增删改
  - primary 删除 → 409 Conflict

### A7. 注入到 server 上下文
- **文件**: `internal/server/server.go` （改）
- **内容**:
  - `Server` 加 `mountRegistry *mount.MountRegistry` 字段
  - `NewServer(cfg)` 初始化 registry + Bootstrap
  - 通过 `WithMountRegistry(r)` 注入到 service
- **验收**:
  - 启动后 `s.MountRegistry().List()` 至少 1 个 primary mount
  - **不改** 任何 service 内部对 `s.servingDir` 的引用

### A8. 单元测试 + 集成测试
- **文件**: `internal/mount/mount_test.go` 🆕 + `internal/mount/drivers/*_test.go` 🆕 + `internal/server/mount_api_test.go` 🆕
- **覆盖**:
  - MountRegistry.Resolve 4 类路径
  - 持久化 Load/Save 往返
  - Bootstrap 从 cfg 创建 3 个 mount
  - LocalDriver FS 操作
  - AppDataDriver sandbox fallback
  - HTTP CRUD 端点
- **验收**:
  - `go test ./internal/mount/... -v` 全过
  - `go test ./internal/server/... -run TestMountAPI` 全过
  - 覆盖率 > 80%

### A9. 0 回归
- **内容**: 不动任何 mobile_service / task_manager / mock_generator 代码
- **验收**:
  - `go test ./...` 全过（**所有**现有测试）
  - `pnpm exec vitest run` 全过
  - `pnpm exec vue-tsc --noEmit` 0 error
  - 启动 backend：旧 `/api/files?path=/foo` 仍 200
  - `git diff --stat` 范围内只有新文件 + 必要的 server.go 注入

**M1 完成标准**（Phase A 全过）：新 mount 系统骨架可用，**旧代码 0 改动 0 回归**。

---

## 阶段 B：mock 切到 automation mount（验证模式）

> **Owner**: 全栈
> **目标**: 真机 release 跑通自动化测试，0 EACCES
> **预计**: 2-3 天
> **前置**: M1

### B1. mock_generator 接受 mount path
- **文件**: `internal/server/mock_generator.go` （改）
- **内容**:
  - `mockRootAllowList` 改为接受 `/d/automation/...` 形式
  - 旧形式 `/storage/emulated/0/encv-automation` **保留**（双轨期；记录 deprecation warning）
  - `handleMockGenerate` 解析 root → 拿 mount.RootPath → 写文件
- **验收**:
  - 旧的 `/storage/emulated/0/encv-automation` 调用仍工作
  - 新的 `/d/automation` 调用工作
  - 单元测试覆盖两种 root

### B2. mobile_api service-guard 改用 primary mount
- **文件**: `internal/server/mobile_api.go:210` （改）
- **内容**: `expectedDir := "/storage/emulated/0"` → `expectedDir := mountRegistry.GetByName("primary").RootPath`
- **验收**: 真机上 service-guard 仍 200（cfg.ServingDir 被 primary mount 继承）

### B3. mobile_service 同样替换
- **文件**: `internal/service/mobile_service.go:186` （改）
- **内容**: 注释里的 `/storage/emulated/0` 改用 mount.RootPath
- **验收**: 编译通过；运行时行为不变

### B4. 前端 DEFAULT_AUTOMATION_SOURCE 切到 mount path
- **文件**: `app/encv-mobile/src/composables/useAutomationTests.ts:92` （改）
- **内容**:
  - 旧: `/storage/emulated/0/encv-automation/01-plain-media/video/sample.mp4`
  - 新: `/d/automation/01-plain-media/video/sample.mp4`
- **验收**:
  - 自动化测试 workflow 默认 sourcePath 用 mount path
  - 旧 fixture 测试更新（如果有）

### B5. 前端 WorkflowDashboard 派生 mount path
- **文件**: `app/encv-mobile/src/views/WorkflowDashboard.vue:201, 426, 449` （改）
- **内容**: 派生逻辑改为读 `/api/mounts` 拿 `automation` mount 的 `mount_path`
- **验收**:
  - 页面 mount 时 fetch `/api/mounts`
  - workflow 创建用 `/d/automation/...`
  - 加载时显示 loading state

### B6. 前端 AutomationTestsDetail 同样替换
- **文件**: `app/encv-mobile/src/views/AutomationTestsDetail.vue:373` （改）
- **验收**: 与 B5 同步

### B7. withSafetyBoundary 降级 no-op
- **文件**: `app/encv-mobile/src/composables/usePathResolver.ts` （改）
- **内容**:
  - 函数签名保留
  - 函数体改为 `return normalize(rawPath)`（不做改写）
  - 加 `@deprecated` 注释（Phase F 删）
- **验收**:
  - 旧调用方无感
  - 测试用例更新（断言 no-op 行为）

### B8. 真机 release 端到端验证
- **内容**:
  - 跑自动化测试 workflow
  - 观察 mock 文件落点（在 Android 上是 `/data/user/<uid>/com.encvgo.app/files/encv-automation/...`）
  - 观察任务能否读到
  - **不出现** EACCES
  - **不出现** "source file not found"
- **验收**:
  - 真机 logcat 0 EACCES
  - 任务 completed
  - `git diff --stat` 限定在 B1-B7 文件

**M2 完成标准**（Phase A + B 全过）：真机 release 跑通自动化测试，0 EACCES，0 path mismatch。

---

## 阶段 C：task_manager 切到 mount

> **Owner**: 全栈
> **目标**: 任务用 mount 路径创建 + 解析，**不再依赖 servingDir**
> **预计**: 3-4 天
> **前置**: M2

### C1. Task 数据结构扩展
- **文件**: `internal/service/task_manager.go` （改）
- **内容**:
  - 任务结构加 `MountID string` + `MountSubPath string` 双字段
  - 旧 `SourcePath string` 字段保留（兼容旧任务）
  - 任务创建时优先用 mount path；旧形式回退到 `servingDir` 解析
- **验收**:
  - 旧 fixture（`task_manager_state_test.go`）仍能加载
  - 新任务带 mount_id

### C2. 任务提交 API 接受 mount path
- **文件**: `internal/server/task_api.go` （改）
- **内容**:
  - 任务提交参数 `sourcePath` 接受 `/d/<mount>/...`
  - 解析 → 存 `MountID + SubPath`
  - 旧绝对路径 → 尝试匹配现有 mount（最长前缀）
- **验收**:
  - HTTP 测试覆盖 3 种 sourcePath 形式（mount path / 旧 absolute / 找不到）

### C3. 任务运行时 mount 解析
- **文件**: `internal/service/task_manager.go` （改）
- **内容**:
  - 任务执行时通过 `mountRegistry.Resolve(...)` 拿 abs path
  - 旧任务（只有 SourcePath）走 fallback 解析
- **验收**:
  - 新任务读 mount 解析后的绝对路径
  - 旧任务兼容

### C4. 任务 fixture 更新
- **文件**: `internal/service/task_manager_state_test.go` （改）
- **内容**:
  - 测试 fixture 改用 `MountID + SubPath` 形式
  - 旧形式测试保留（验证 fallback）
- **验收**: 单元测试全过

**M3 完成标准**（Phase A + B + C 全过）：任务从 mount 创建到运行全链路走 mount 解析。

---

## 阶段 D：mobile_service 切到 mount（最大面积）

> **Owner**: 后端
> **目标**: 文件浏览 / 上传 / 下载全走 mount 解析
> **预计**: 5-7 天
> **前置**: M3

### D1. /api/files 接受 mount path
- **文件**: `internal/server/mobile_api.go` （改）
- **内容**:
  - 解析 path：
    - `/d/<mount>/...` → mount 解析
    - `/foo` → 旧形式，rewrite 到 `/d/primary/foo`
  - 列表响应加 `mount_id` 字段
- **验收**:
  - 旧 `/api/files?path=/foo` 自动走 primary
  - 新 `/api/files?path=/d/sandbox/foo` 走 sandbox

### D2. mobile_service 30+ 处替换
- **文件**: `internal/service/mobile_service.go` （改）
- **内容**: 30+ 处 `SafeResolveToAbsPath(s.servingDir, path)` → `s.mountRegistry.Resolve(path)`
- **验收**:
  - `go build ./...` 通过
  - `go test ./internal/service/...` 全过
  - 旧 fixture（用 `servingDir`）更新为 `mount_id`

### D3. 上传/下载走 mount
- **文件**: `internal/server/mobile_api.go` （改）
- **内容**:
  - 上传 endpoint 接受 `/d/<mount>/...`
  - 下载 endpoint 接受 `/d/<mount>/...`
  - 写入用对应 mount 的 driver
- **验收**:
  - 上传到 `/d/sandbox/foo` 写到 sandbox 的 RootPath
  - 下载从 `/d/automation/bar` 读 automation 的 RootPath
  - 集成测试覆盖多 mount 上下文件操作

### D4. 列表 mount 信息
- **文件**: `internal/server/mobile_api.go` （改）
- **内容**:
  - `/api/files` 列表响应加 `mount_id`, `mount_name`, `mount_path`
- **验收**: 列表返回每个 file 带 mount 信息

**M4 完成标准**（Phase A-D 全过）：文件浏览/上传/下载/重命名/删除全过，多 mount 列表显示归属。

---

## 阶段 E：前端 UI（Settings/Mounts.vue）

> **Owner**: 前端
> **目标**: Settings 页能看挂载点 + 增删（dev mode）
> **预计**: 2-3 天
> **前置**: M4

### E1. useMountResolver composable
- **文件**: `app/encv-mobile/src/composables/useMountResolver.ts` 🆕
- **内容**:
  - `resolve({mount, subPath})` → `/d/<mount>/<subPath>`
  - `resolveByPath('/d/<mount>/<subPath>')` → `{mount, subPath}`
  - `useMounts()` → reactive 挂载点列表（fetch `/api/mounts`）
- **验收**:
  - 单元测试覆盖 resolve / resolveByPath / 404 场景

### E2. Settings/Mounts.vue 页面
- **文件**: `app/encv-mobile/src/views/Settings/Mounts.vue` 🆕
- **内容**:
  - 挂载点列表（ion-card 渲染每个 mount）
  - 显示：name / mount_path / driver / root_path / usage / read_only
  - 按钮：刷新 / 添加（dev mode）/ 删除（带二次确认）
- **验收**:
  - 渲染正确
  - 刷新按钮工作
  - dev mode 添加/删除可用；release 锁死

### E3. 路由配置
- **文件**: `app/encv-mobile/src/router/index.ts` （改）
- **内容**: 加 `/tabs/settings/mounts` 路由
- **验收**: 能跳转

### E4. 替换 usePathResolver 调用方
- **文件**: 多处（按需 grep）
- **内容**: 把 `withSafetyBoundary` 调用替换为 `useMountResolver().resolve(...)`
- **验收**: grep `withSafetyBoundary` 调用次数 → 0（除 usePathResolver.ts 自身）

**M5 完成标准**（Phase A-E 全过）：Settings/Mounts.vue 完整可用，前端 0 `withSafetyBoundary` 引用。

---

## 阶段 F：清理 + 删除旧代码

> **Owner**: 全栈
> **目标**: 删 cfg.ServingDir / usePathResolver.withSafetyBoundary / mockRootAllowList / 硬编码 expectedDir
> **预计**: 1-2 天
> **前置**: M5

### F1. cfg.ServingDir 移除
- **文件**: `internal/config/config.go` （改）
- **内容**:
  - 字段标记为 deprecated
  - 读取时仅用作 migration 入口
  - 加 log 提示用户迁到 mounts.json
- **验收**: 编译通过；启动 log 有 deprecation 警告

### F2. usePathResolver.withSafetyBoundary 移除
- **文件**: `app/encv-mobile/src/composables/usePathResolver.ts` （改）
- **内容**:
  - 函数体删除
  - 文件保留作为 utility（normalize / resolveFileItem 等）
- **验收**: vue-tsc 0 error；grep 0 引用

### F3. mockRootAllowList 移除
- **文件**: `internal/server/mock_generator.go` （改）
- **内容**:
  - 移除白名单硬编码
  - 替换为 mount path 校验（必须以 `/d/automation/` 开头）
- **验收**: 单元测试覆盖；老 absolute path 报错 400

### F4. mobile_api expectedDir 移除
- **文件**: `internal/server/mobile_api.go:210` （改）
- **内容**:
  - 硬编码 `/storage/emulated/0` 改为 mount registry 查询
- **验收**: 代码 grep 0 `storage/emulated` 引用（除 mount bootstrap / driver）

### F5. 文档更新
- **文件**: `.trae/rules/development.md` §六 + `.trae/rules/capacitor.md`
- **内容**:
  - 双重编码章节适配 mount path
  - 路径解析章节引用新 mount registry
- **验收**: 文档与代码一致

### F6. 旧 spec 联动更新
- **文件**: `.trae/specs/unify-path-resolver/`, `.trae/specs/mock-router-refactor/`
- **内容**:
  - `unify-path-resolver` 标 done，引用新 spec
  - `mock-router-refactor` 标"已合并到 multi-mount-storage-refactor"
- **验收**: 旧 spec 有最终状态

### F7. 最终回归
- **内容**: 完整跑所有测试
- **验收**:
  - `go test ./...` 全过
  - `pnpm exec vitest run` 全过
  - `pnpm exec vue-tsc --noEmit` 0 error
  - `grep -r "storage/emulated" internal/` 仅在 mount bootstrap / driver
  - `grep -r "withSafetyBoundary" app/encv-mobile/src/` 0 引用

**M6 完成标准**（Phase A-F 全过）：0 旧代码引用，0 硬编码路径（除 mount driver 内部），dev + 真机 E2E 全过。

---

## 风险登记表

| 风险 | 概率 | 影响 | 缓解 | 任务 |
|------|------|------|------|------|
| Phase D 改 mobile_service 30+ 处漏改 | 中 | 高 | 旧 API 双轨期；Phase B 跑通后再动 D | D2 |
| AppDataDriver 真机 vs dev 行为差异 | 中 | 高 | 沙箱 fallback 路径；真机 build tag 集成测试 | A3 |
| trie 性能不如前缀扫描 | 低 | 低 | benchmark；用 radix tree | A2 |
| 旧 fixture 漏更新 | 中 | 中 | 保留旧形式作为 fallback 测试 | C1, C4 |
| 前端 useMountResolver 与后端 mount path 格式不一致 | 低 | 高 | 后端权威；前端从 `/api/mounts` 拉 mount_path 渲染 | E1 |
| cfg.ServingDir 删除后真机配置回不来 | 低 | 中 | migration-backup.json；mounts.json 缺失时 Bootstrap | A4, A5, F1 |
