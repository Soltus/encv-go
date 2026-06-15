# Checklist: multi-mount-storage-refactor

> 与 spec.md 配套的执行清单。完成一项勾选一项，不要批量勾。

## Phase A — 骨架 + 向后兼容（不影响功能）

- [ ] **A1** 新建 `internal/mount/mount.go` — `Mount` 数据结构 + JSON tag
- [ ] **A2** 新建 `internal/mount/registry.go` — `MountRegistry` + `Resolve(virtualPath)` + `trie` 解析
- [ ] **A3** 新建 `internal/mount/driver.go` — `Driver` 接口
- [ ] **A4** 新建 `internal/mount/drivers/local.go` — `LocalDriver`（封装 `cfg.ServingDir`）
- [ ] **A5** 新建 `internal/mount/drivers/appdata.go` — `AppDataDriver`（真机 `/data/user/<uid>/<pkg>/files/<subpath>`）
- [ ] **A6** 新建 `internal/mount/drivers/sandbox.go` — `SandboxDriver`（dev only）
- [ ] **A7** 新建 `internal/mount/bootstrap.go` — `BootstrapFromConfig(cfg)`
- [ ] **A8** 新建 `internal/mount/migrate.go` — `MigrateFromServingDir(cfg)`（读 cfg.ServingDir → 创建 primary mount）
- [ ] **A9** 新建 `internal/mount/registry_persist.go` — `Load()` / `Save()` mounts.json
- [ ] **A10** 加 HTTP handler `internal/server/mount_api.go` — CRUD 端点
- [ ] **A11** `internal/server/server.go` 把 `MountRegistry` 注入到 server 上下文
- [ ] **A12** `internal/mount/mount_test.go` — 单元测试（Resolve / Persistence / Bootstrap）
- [ ] **A13** `internal/mount/drivers/local_test.go` — LocalDriver 真实 FS 操作
- [ ] **A14** `internal/mount/drivers/appdata_test.go` — sandbox 下 fallback 行为
- [ ] **A15** `internal/server/mount_api_test.go` — `/api/mounts` CRUD HTTP 测试
- [ ] **A16** **不改** mobile_service / task_manager / mock_generator（**关键**，保留兼容）
- [ ] **A17** **不改** `cfg.ServingDir` 字段（仅读取作 bootstrap 入口）
- [ ] **A18** vue-tsc / vitest / go test 现有用例**全过**（0 回归）

**Validation Phase A**:
- `go test ./internal/mount/...` 全过
- `pnpm exec vitest run` 全过
- 启动 backend 一次，`/api/mounts` 返回至少 1 个 `primary` mount
- 旧 `/api/files?path=/foo` 仍 200（不依赖 mount 解析）

---

## Phase B — mock 切到 automation mount（验证模式）

- [ ] **B1** `mock_generator.go` `mockRootAllowList` 替换：白名单改为"必须以 `/d/automation/` 开头" + 兼容旧的 `/storage/emulated/0/encv-automation`（双轨期）
- [ ] **B2** `mobile_api.go:210` `expectedDir := "/storage/emulated/0"` 替换为 `mountRegistry.GetByName("primary").RootPath`
- [ ] **B3** `mobile_service.go:186` 同样替换
- [ ] **B4** 前端 `useAutomationTests.ts:92` `DEFAULT_AUTOMATION_SOURCE` 改为 `/d/automation/01-plain-media/video/sample.mp4`
- [ ] **B5** 前端 `WorkflowDashboard.vue:201, 426, 449` 派生 mount path
- [ ] **B6** 前端 `AutomationTestsDetail.vue:373` 派生 mount path
- [ ] **B7** `usePathResolver.withSafetyBoundary` **保留**但降级为 no-op（migration 期兼容）
- [ ] **B8** 端到端测试：dev 沙箱 + 真机 release 跑 mock 生成 → 不 EACCES

**Validation Phase B**:
- 真机 release 跑自动化测试，文件落到 `automation` mount 的 RootPath（dev 沙箱 = `/data/.../cache/...`；真机 = `/data/user/<uid>/<pkg>/files/encv-automation/`）
- 不出现 "source file not found"
- 不出现 EACCES
- `pnpm exec vitest run __tests__/useAutomationTests.test.ts` 全过（更新 mock `/api/mounts`）

---

## Phase C — task_manager 切到 mount

- [ ] **C1** `task_manager.go` 任务结构里 `SourcePath` 字段从 absolute 改为 `MountID + SubPath` 双字段
- [ ] **C2** 任务提交 API 接受 `sourcePath = "/d/<mount>/..."` 形式（解析 → 存双字段）
- [ ] **C3** 任务运行时通过 `mountRegistry.Resolve(...)` 拿绝对路径（不再用 `servingDir`）
- [ ] **C4** 旧任务（只有 absolute path 的）保留兼容：尝试从现有 mount 找最长前缀匹配
- [ ] **C5** `task_manager_state_test.go` 更新 fixture（用 `mount_id + sub_path`）

**Validation Phase C**:
- 端到端：mock 生成 → 提交加密任务 → 任务读 mount 解析后的绝对路径 → 加密 → 输出到 `primary/encrypted/...`
- 旧 fixture 仍能加载（fallback 路径）

---

## Phase D — mobile_service 切到 mount（最大面积）

- [ ] **D1** `/api/files` 接受 `/d/<mount>/...` 形式
- [ ] **D2** 旧形式 `/foo` 自动 rewrite 到 `primary/foo`
- [ ] **D3** `mobile_service.go` 30+ 处 `SafeResolveToAbsPath(servingDir, path)` 替换为 `mountRegistry.Resolve(path)`
- [ ] **D4** `/api/files` 列表响应里加 `mount_id` 字段
- [ ] **D5** `/api/files` 上传/下载接受 `/d/...` 形式
- [ ] **D6** 集成测试覆盖：多 mount 下的 list/upload/download

**Validation Phase D**:
- 文件浏览/上传/下载/重命名/删除全过
- 多 mount 列表正确显示 mount 归属

---

## Phase E — 前端 UI（Settings/Mounts.vue）

- [ ] **E1** 新建 `src/views/Settings/Mounts.vue`
- [ ] **E2** 显示挂载点列表（name / mount_path / driver / root_path / usage / read_only）
- [ ] **E3** "刷新挂载点"按钮（POST `/api/mounts/refresh` 重建 trie）
- [ ] **E4** "添加挂载点"按钮（仅 dev mode 显示；release 隐藏）
- [ ] **E5** "删除"按钮（primary 不可删，其他二次确认）
- [ ] **E6** 路由配置（`/tabs/settings/mounts`）
- [ ] **E7** `useMountResolver.ts` 🆕 composable（前端 mount path 解析）

**Validation Phase E**:
- Settings 页能列出挂载点
- 刷新按钮可点
- dev mode 能添加/删除；release 锁死

---

## Phase F — 清理 + 删除旧代码

- [ ] **F1** `cfg.ServingDir` 移除（或改为 read-only migration 入口）
- [ ] **F2** `usePathResolver.withSafetyBoundary` 移除（所有调用方已切到 mount 路径）
- [ ] **F3** `mockRootAllowList` 硬编码白名单移除
- [ ] **F4** `mobile_api.go:210` 硬编码 `expectedDir` 移除
- [ ] **F5** `grep -r "storage/emulated" internal/` 仅在 mount bootstrap / driver 实现里出现
- [ ] **F6** 文档更新：development.md §六 WAF 双重编码章节 + capacitor.md 路径解析章节
- [ ] **F7** 旧 `unify-path-resolver` spec 标 done，引用本 spec

**Validation Phase F**:
- 0 旧代码引用
- 0 硬编码路径（除 mount driver 内部）
- 完整 E2E 跑通（dev + 真机）

---

## 跨阶段检查项

- [ ] **X1** 每次 commit 前跑 `pnpm exec vue-tsc --noEmit`（**0 type error**）
- [ ] **X2** 每次 commit 前跑 `pnpm exec vitest run`（**全过**）
- [ ] **X3** 每次 commit 前跑 `go test ./...`（**全过**）
- [ ] **X4** 每次 commit 前跑 `golangci-lint run`（如已配置）
- [ ] **X5** PR 描述里说明改动的 Phase 阶段（A/B/C/D/E/F）
- [ ] **X6** 涉及 `internal/mount/` 的改动必须有单元测试
- [ ] **X7** 涉及前端路径解析的改动必须有 vitest 用例
- [ ] **X8** 涉及 config schema 改动的必须更新 `unify-path-resolver` / `mock-router-refactor` 等联动 spec
