# Tasks — 分层重构

> 本 tasks 列表是**分阶段、原子化**的。每个 Phase 完成后单独 review + merge。
> 进度更新在每个 Phase 完成后勾选 `[x]`。
> 子任务细节由各 Phase 独立 spec 详细化。

---

## Phase 1: 后端 Plugin 接口与实现去重（P1）

- [ ] **Task 1.1**: Plugin 接口返回值评估
  - [ ] SubTask 1.1.1: 审查 `PreEncryptProcessor`/`PreDecryptProcessor`/`PostDecryptProcessor` 是否有结构化数据需返回
  - [ ] SubTask 1.1.2: 决定是否改签名为 `(Status, error)` 或保持 `error`
  - [ ] SubTask 1.1.3: 输出决策记录（不改 / 改）

- [ ] **Task 1.2**: V4 容器插件 PostEncryptProcessor 提取公共 helper
  - [ ] SubTask 1.2.1: 在 `internal/v2/plugins/interfaces/packer/` 中新增 `StandardPostEncryptForContainer(plugin, inputPath, inputRootDir, outputDir) (string, error)`
  - [ ] SubTask 1.2.2: 迁移 6 个 V4 插件调用公共 helper
  - [ ] SubTask 1.2.3: 验证 `go build` + `go test ./internal/v2/plugins/...` 通过

- [ ] **Task 1.3**: V4 容器插件 Decrypt 提取公共 helper
  - [ ] SubTask 1.3.1: 在 `internal/v2/plugins/interfaces/` 中新增 `StandardDecryptForContainer(plugin, containerPath, outputDir) (string, error)`
  - [ ] SubTask 1.3.2: 迁移 6 个 V4 插件调用公共 helper
  - [ ] SubTask 1.3.3: 验证编译 + 测试

- [ ] **Task 1.4**: TaskManager mutation API 化
  - [ ] SubTask 1.4.1: 在 `internal/service/task_manager.go` 中新增 `completeTask(id, outputPath)` / `markCancelling(id)` / `failTaskWithDetail(id, errMsg, errDetail)` / `appendStep(id, phase, detail)` 方法
  - [ ] SubTask 1.4.2: 重构 `processEncrypt` / `processDecrypt` 调用新 API
  - [ ] SubTask 1.4.3: 验证 task_manager 测试通过

---

## Phase 2: 后端 HTTP Handler 与 Service 层模板化（P2）

- [ ] **Task 2.1**: 提取 Gin handler 通用模板
  - [ ] SubTask 2.1.1: 在 `internal/server/` 中新增 `handler_template.go`，定义 `HandlerOpts` + `handleGinTemplate`
  - [ ] SubTask 2.1.2: 重构 `handleAlistEncryptStreamGin` 使用模板
  - [ ] SubTask 2.1.3: 评估其他 `handleXxxGin` 迁移（如果适用）
  - [ ] SubTask 2.1.4: 验证 server 测试通过

- [ ] **Task 2.2**: Service 层 `GetFileInfo` 简化
  - [ ] SubTask 2.2.1: 检查 `mobile_service.go` 中残留的 version==4 分支
  - [ ] SubTask 2.2.2: 如有，统一走 `ContainerHandle` 抽象
  - [ ] SubTask 2.2.3: 验证 service 测试通过

- [ ] **Task 2.3**: Physical 打包层去重
  - [ ] SubTask 2.3.1: 对比 `file_single.go` 和 `file_multi.go` 的 `Pack()` 实现
  - [ ] SubTask 2.3.2: 提取共同 manifest 写盘逻辑
  - [ ] SubTask 2.3.3: 验证编译

---

## Phase 3: 前端 View/Component 拆分（P1-P2）

- [ ] **Task 3.1**: Tasks.vue 拆分（store + view）
  - [ ] SubTask 3.1.1: 引入 Pinia 或新建 `useTasksStore` composable（如项目无 Pinia）
  - [ ] SubTask 3.1.2: 迁移列表/过滤/搜索/刷新/eventBus 订阅到 store
  - [ ] SubTask 3.1.3: Tasks.vue < 200 行，只做 UI 渲染
  - [ ] SubTask 3.1.4: 重新审查 eventBus 监听是否违反 §2.1 铁律
  - [ ] SubTask 3.1.5: 验证 vue-tsc 通过 + 端到端测试

- [ ] **Task 3.2**: TaskDetailModal 子组件化
  - [ ] SubTask 3.2.1: 拆分 `<TaskBasicInfo>` 子组件
  - [ ] SubTask 3.2.2: 拆分 `<TaskTimeline>` 子组件（已含 steps 展开逻辑）
  - [ ] SubTask 3.2.3: 拆分 `<TaskOutputInfo>` 子组件
  - [ ] SubTask 3.2.4: 拆分 `<TaskErrorSection>` / `<TaskWarningSection>` 子组件
  - [ ] SubTask 3.2.5: 拆分 `<TaskActionButtons>` 子组件
  - [ ] SubTask 3.2.6: TaskDetailModal.vue < 100 行
  - [ ] SubTask 3.2.7: 验证 vue-tsc 通过

- [ ] **Task 3.3**: NewTaskModal 加密/解密模式拆分
  - [ ] SubTask 3.3.1: 评估 `<EncryptBody>` / `<DecryptBody>` 子组件拆分价值
  - [ ] SubTask 3.3.2: 如有价值则拆分
  - [ ] SubTask 3.3.3: 验证 vue-tsc 通过

---

## Phase 4: 横切关注点（P2）

- [ ] **Task 4.1**: Phase 名称类型化（后端 + 前端）
  - [ ] SubTask 4.1.1: 后端新增 `internal/service/phase.go` 定义 Phase 常量/iota
  - [ ] SubTask 4.1.2: 前端新增 `src/types/phase.ts` 枚举
  - [ ] SubTask 4.1.3: 替换后端/前端散落的 phase 字符串
  - [ ] SubTask 4.1.4: 验证编译 + vue-tsc 通过

- [ ] **Task 4.2**: i18n 拆分
  - [ ] SubTask 4.2.1: 评估按模块拆分 `useI18n.ts` 的可行性
  - [ ] SubTask 4.2.2: 实施拆分（如有清晰边界）
  - [ ] SubTask 4.2.3: 验证 vue-tsc 通过

- [ ] **Task 4.3**: CI 去重
  - [ ] SubTask 4.3.1: 提取 `/.github/actions/setup-env` composite action
  - [ ] SubTask 4.3.2: 提取 `/.github/actions/frontend-check` composite action
  - [ ] SubTask 4.3.3: 重构 `test.yml` 和 `android.yml` 使用 composite actions
  - [ ] SubTask 4.3.4: 验证 CI 通过

- [ ] **Task 4.4**: Makefile 入口统一
  - [ ] SubTask 4.4.1: 评估 `Makefile` + `hack/hack.mk` + `hack/hack-cli.mk` 重叠
  - [ ] SubTask 4.4.2: 合并/重构
  - [ ] SubTask 4.4.3: 验证本地构建通过

---

# Task Dependencies

- Phase 1 全部完成 → 才允许进入 Phase 2
- Phase 2 全部完成 → 才允许进入 Phase 3
- Phase 3 全部完成 → 才允许进入 Phase 4
- Phase 4 的 4 个 Task 之间无强依赖，可视评估结果选择性执行

# Parallelizable Work

- Phase 1 内：Task 1.2 / 1.3 / 1.4 可并行（不互相依赖）
- Phase 3 内：Task 3.1 / 3.2 / 3.3 可并行
- Phase 4 内：4 个 Task 互相独立
