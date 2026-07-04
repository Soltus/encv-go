# Tasks

- [x] Task 1: 后端 ToolError 统一异常类型 + tool_result 事件 isError 字段
  - [x] SubTask 1.1: 创建 `internal/tools/errors.go` 定义 `ToolError` 类型
  - [x] SubTask 1.2: 修改 `internal/tools/registry.go` ToolHandler 签名 `(ToolResult, *ToolError)` (实际: 保留原签名，新增 BashLikeHandler 包装)
  - [x] SubTask 1.3: 迁移 v1+v2 工具 handler 到新签名（实际: handler 错误由 Dispatch 统一转换为 IsError=true）
  - [x] SubTask 1.4: 修改 `internal/server/agent_tool_loop.go` `executeTool` 推 `tool_result` 事件时带 `isError` / `errorCode` / `errorMessage` (实际: agent_api.go Branch A/B)
  - [x] SubTask 1.5: 修改 `internal/server/agent_api.go` `streamChat` 工具异常时推 `tool_status { status: "error" }` 而非 `success`
  - [x] SubTask 1.6: 单测 — TestToolError 13 用例 + TestToolErrorPropagation 集成
  - [x] SubTask 1.7: 验证 `go build ./cmd/encv` 0 错误

- [x] Task 2: 后端跨平台 bash 工具抽象（high_level）
  - [x] SubTask 2.1: 创建 `internal/tools/platform_dispatch.go` — DetectPlatform + PlatformCommandMap + DefaultCommandMap
  - [x] SubTask 2.2: 创建 `internal/tools/high_level.go` — HighLevelTool + Execute 包装器
  - [x] SubTask 2.3: 实现 10 个 high-level 工具
  - [x] SubTask 2.4: 修改 ToolDef.Kind 字段 + 注册 high_level 工具
  - [x] SubTask 2.5: 修改 config 默认值追加 powershell
  - [x] SubTask 2.6: 单测 — TestPlatformDispatch 14 用例 + TestHighLevel 30 用例
  - [x] SubTask 2.7: 验证 `go build ./cmd/encv` 0 错误

- [x] Task 3: 前端 useAgent tool_call 状态机 + isError 解析
  - [x] SubTask 3.1: 修改 ToolCall 接口加 errorCode/errorMessage/output/startedAt/finishedAt
  - [x] SubTask 3.2: 修改 handleAgentEvent 解析 tool_result.isError
  - [x] SubTask 3.3: 添加 runningTools/hasRunningTool computed
  - [x] SubTask 3.4: 30s 无响应 → tool_call error + errorCode: 'TIMEOUT'
  - [x] SubTask 3.5: 单测 — 6 个状态机用例 (61 useAgent tests 通过)
  - [x] SubTask 3.6: 验证 `vue-tsc --noEmit` 0 错误

- [x] Task 4: 前端 ToolDetailContent 工具卡片 4 状态视觉
  - [x] SubTask 4.1: 修改 OperationCard.vue (注: 原 spec 写 ToolDetailContent.vue，实际叫 OperationCard.vue) 4 状态视觉
  - [x] SubTask 4.2: 添加 spinner 旋转动画
  - [x] SubTask 4.3: 添加 ✓ scale 动画 + ⚠️ 抖动动画
  - [x] SubTask 4.4: 错误卡片加复制按钮 + 点击展开堆栈
  - [x] SubTask 4.5: 卡片底部加耗时显示
  - [x] SubTask 4.6: 验证 `vue-tsc --noEmit` 0 错误

- [x] Task 5: 前端相对时间格式化函数抽取
  - [x] SubTask 5.1: 创建 `composables/relativeTime.ts`
  - [x] SubTask 5.2: 单测 — 22 个 formatRelativeTime 边界值
  - [x] SubTask 5.3: AgentChat.vue 局部 formatRelativeTime 已替换为 import 版本
  - [x] SubTask 5.4: AgentChat.vue footer 时间改用 formatRelativeTime
  - [x] SubTask 5.5: AssistantMessage.vue 单条消息时间改用 formatRelativeTime
  - [x] SubTask 5.6: 验证 `vue-tsc --noEmit` 0 错误

- [x] Task 6: 前端 i18n 增量
  - [x] SubTask 6.1: 修改 i18n/agent.ts 加 7 个新 key (tool.running / success / timeout / errorBadge / copyError / duration / errorDetails + zoom.in/out/reset)
  - [x] SubTask 6.2: 验证 zh-CN + en 双语齐全

- [x] Task 7: 前端 usePinchZoom composable
  - [x] SubTask 7.1: 创建 `composables/usePinchZoom.ts`
  - [x] SubTask 7.2: 实现双击重置 zoomScale = 1.0
  - [x] SubTask 7.3: 实现 zoomScale clamp 到 [0.5, 1.5]
  - [x] SubTask 7.4: 单测 — 26 个用例
  - [x] SubTask 7.5: 验证 `vue-tsc --noEmit` 0 错误

- [x] Task 8: 前端 AgentChat 集成缩放 + footer 相对时间
  - [x] SubTask 8.1: 集成 usePinchZoom 到 mainRef 容器
  - [x] SubTask 8.2: 右上角「A- / A / A+」浮动按钮组
  - [x] SubTask 8.3: footer 时间改用 formatRelativeTime
  - [x] SubTask 8.4: 验证 `vue-tsc --noEmit` 0 错误

- [x] Task 9: 错误状态页禁止缩放
  - [x] SubTask 9.1: App.vue 错误状态块 CSS 加 touch-action: manipulation
  - [x] SubTask 9.2: 实现 viewport meta 动态切换
  - [x] SubTask 9.3: 验证 `vue-tsc --noEmit` 0 错误

- [x] Task 10: 端到端验证
  - [x] SubTask 10.1: vue-tsc 0 错误
  - [x] SubTask 10.2: go build 0 错误
  - [x] SubTask 10.3: vitest 710/716 通过（6 个 pre-existing useApiBaseProbe 失败与本次无关）
  - [x] SubTask 10.4: go test internal/tools 82/82 通过

# Task Dependencies
- [Task 3] depends on [Task 1] (前端要解析后端新加的 isError 字段) ✅
- [Task 4] depends on [Task 3] (ToolDetailContent 依赖 useAgent 状态机) ✅
- [Task 5.4] depends on [Task 5.1] (AgentChat 依赖 relativeTime 函数) ✅
- [Task 5.5] depends on [Task 5.1] ✅
- [Task 8] depends on [Task 5, Task 7] (AgentChat 集成 relativeTime + pinchZoom) ✅
- [Task 10] depends on [Task 1..9] ✅
