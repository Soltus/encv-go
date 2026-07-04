# 分层重构 Checklist

> 本 checklist 按 Phase 分组，**逐项**勾选。每个 Phase 完成后必须 100% 勾选才能进入下个 Phase。

---

## Phase 1: 后端 Plugin 接口与实现去重

### Task 1.1: Plugin 接口返回值评估
- [ ] `PreEncryptProcessor` / `PreDecryptProcessor` / `PostDecryptProcessor` 评估决策已记录
- [ ] 如改签：调用方已同步更新
- [ ] `go build ./...` 通过

### Task 1.2: V4 容器插件 PostEncryptProcessor 公共 helper
- [ ] `StandardPostEncryptForContainer` helper 已实现
- [ ] 6 个 V4 插件（video/audio/image/pdf/text/wps）已迁移
- [ ] 每个插件的 `PostEncryptProcessor` 函数体 < 20 行（只保留特化逻辑）
- [ ] `go build ./...` 通过
- [ ] `go test ./internal/v2/plugins/...` 通过

### Task 1.3: V4 容器插件 Decrypt 公共 helper
- [ ] `StandardDecryptForContainer` helper 已实现
- [ ] 6 个 V4 插件已迁移
- [ ] `go build ./...` 通过
- [ ] `go test ./internal/v2/plugins/...` 通过

### Task 1.4: TaskManager mutation API
- [ ] `completeTask` / `markCancelling` / `failTaskWithDetail` / `appendStep` 已实现
- [ ] `processEncrypt` / `processDecrypt` 改用新 API
- [ ] `tm.mu.Lock()` 散落位置 < 5 处（仅在 mutation API 内部）
- [ ] `go test ./internal/service/...` 通过

---

## Phase 2: 后端 HTTP Handler 与 Service 层模板化

### Task 2.1: Gin handler 通用模板
- [ ] `HandlerOpts` + `handleGinTemplate` 已实现
- [ ] `handleAlistEncryptStreamGin` 已迁移到模板
- [ ] 其他 `handleXxxGin` 评估迁移完成
- [ ] `go test ./internal/server/...` 通过

### Task 2.2: Service 层 `GetFileInfo` 简化
- [ ] `mobile_service.go` 中残留的 version==4 分支已清理
- [ ] 统一走 `ContainerHandle` 抽象
- [ ] `go test ./internal/service/...` 通过

### Task 2.3: Physical 打包层去重
- [ ] `file_single.go` 和 `file_multi.go` 的 `Pack()` 共同逻辑已提取
- [ ] `go build ./...` 通过
- [ ] `go test ./internal/v2/physical/...` 通过

---

## Phase 3: 前端 View/Component 拆分

### Task 3.1: Tasks.vue 拆分
- [ ] Pinia store 或 `useTasksStore` composable 已建立
- [ ] 列表/过滤/搜索/刷新/eventBus 订阅已迁移到 store
- [ ] `Tasks.vue` 行数 < 200
- [ ] eventBus 监听器已逐个审查，符合 §2.1 铁律
- [ ] `vue-tsc --noEmit` 零错误
- [ ] 端到端：tab 切换、任务创建、详情打开、产物跳转功能正常

### Task 3.2: TaskDetailModal 子组件化
- [ ] `<TaskBasicInfo>` 子组件已抽取
- [ ] `<TaskTimeline>` 子组件已抽取（含 steps 展开逻辑）
- [ ] `<TaskOutputInfo>` 子组件已抽取
- [ ] `<TaskErrorSection>` 子组件已抽取
- [ ] `<TaskWarningSection>` 子组件已抽取
- [ ] `<TaskActionButtons>` 子组件已抽取
- [ ] `TaskDetailModal.vue` 行数 < 100
- [ ] `vue-tsc --noEmit` 零错误
- [ ] 端到端：任务详情展示、产物跳转、错误展示正常

### Task 3.3: NewTaskModal 加密/解密模式拆分
- [ ] `<EncryptBody>` / `<DecryptBody>` 子组件已评估
- [ ] 如有拆分价值已完成
- [ ] `vue-tsc --noEmit` 零错误

---

## Phase 4: 横切关注点

### Task 4.1: Phase 名称类型化
- [ ] 后端 `internal/service/phase.go` Phase 常量/iota 已定义
- [ ] 前端 `src/types/phase.ts` 枚举已定义
- [ ] 后端散落的 phase 字符串（`"encrypting"` / `"completed"` 等）已替换
- [ ] 前端散落的 phase 字符串已替换
- [ ] `go build ./...` 通过
- [ ] `vue-tsc --noEmit` 零错误

### Task 4.2: i18n 拆分
- [ ] `useI18n.ts` 拆分评估完成
- [ ] 如有清晰模块边界，已拆分
- [ ] `vue-tsc --noEmit` 零错误

### Task 4.3: CI 去重
- [ ] `/.github/actions/setup-env` composite action 已建立
- [ ] `/.github/actions/frontend-check` composite action 已建立
- [ ] `test.yml` 和 `android.yml` 已使用 composite actions
- [ ] 至少一个 CI workflow 在 PR 验证通过

### Task 4.4: Makefile 入口统一
- [ ] `Makefile` / `hack/hack.mk` / `hack/hack-cli.mk` 重叠评估完成
- [ ] 合并/重构完成
- [ ] 本地 `make` 目标全可执行

---

## 跨 Phase 验证

- [ ] Phase 1 完成后: 端到端加密/解密流程正常（alist_encrypt + v4 容器）
- [ ] Phase 2 完成后: 所有 HTTP API 单元测试 + 集成测试通过
- [ ] Phase 3 完成后: 端到端 UI 流程正常（任务创建、详情、产物跳转）
- [ ] Phase 4 完成后: 编译 + 测试 + CI 全绿
