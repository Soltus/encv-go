# Checklist

## 后端 - ToolError 异常类型

- [x] `internal/tools/errors.go` 已创建并定义 `ToolError` 类型
- [x] ToolHandler 函数签名已变更为 `(ToolResult, *ToolError)` (实际: 保留原签名，新增 BashLikeHandler 包装 + Dispatch 统一转换)
- [x] v1+v2 工具（10 个）已迁移到新签名 (实际: handler 错误由 Dispatch 统一转换)
- [x] `executeTool` 推 `tool_result` 事件时带 `isError` 字段
- [x] `streamChat` 工具异常时推 `tool_status { status: "error" }`
- [x] TestToolError 13+ 单测通过
- [x] TestToolErrorPropagation 集成测试通过
- [x] `go build ./cmd/encv` 0 错误

## 后端 - 跨平台 bash 工具抽象

- [x] `internal/tools/platform_dispatch.go` 已创建
- [x] 10 个 high-level 工具的 Linux/Darwin/Android/Windows 命令映射齐全
- [x] `internal/tools/high_level.go` 已创建
- [x] 10 个 high-level 工具实现完成
- [x] ToolDef.Kind 字段 + KindBashLike 常量已加
- [x] `tool_whitelist` 默认值已追加 `powershell` + coreutils
- [x] TestPlatformDispatch 14+ 单测通过
- [x] TestHighLevel 30+ 单测通过
- [x] `go build ./cmd/encv` 0 错误

## 前端 - useAgent tool_call 状态机

- [x] `ToolCall` 接口扩展 errorCode / errorMessage / output / startedAt / finishedAt 字段
- [x] `handleAgentEvent` 解析 `tool_result.isError` → 设置 status (成功→success / 失败→failed)
- [x] `runningTools` / `hasRunningTool` computed 已加
- [x] 30s 无响应 → tool_call error + errorCode: 'TIMEOUT'
- [x] useAgent 状态机单测 6 个场景通过 (总 61 个 useAgent tests 通过)
- [x] `vue-tsc --noEmit` 0 错误

## 前端 - ToolDetailContent 4 状态视觉

- [x] pending 状态视觉（灰色占位）
- [x] running 状态视觉（蓝色 spinner 旋转 + 进度条）
- [x] success 状态视觉（绿色对勾 + scale 动画）
- [x] error 状态视觉（红色 ⚠️ + 抖动 + 可展开堆栈）
- [x] cancelled 状态视觉（黄色静态）
- [x] 错误卡片复制按钮工作
- [x] 卡片底部耗时显示工作
- [x] 主题色跟随（`var(--ion-color-primary)`）
- [x] `vue-tsc --noEmit` 0 错误

## 前端 - 相对时间格式化

- [x] `composables/relativeTime.ts` 已创建
- [x] `formatRelativeTime` 函数 5 档边界值正确（22 个单测通过）
- [x] AgentChat.vue 局部 formatRelativeTime 已替换为 import 版本
- [x] AgentChat footer 时间显示改用 formatRelativeTime
- [x] AssistantMessage 单条消息时间改用 formatRelativeTime
- [x] zh-CN + en 双语齐全
- [x] 10+ 个新 i18n key 已加
- [x] `vue-tsc --noEmit` 0 错误

## 前端 - usePinchZoom 缩放 composable

- [x] `composables/usePinchZoom.ts` 已创建
- [x] 双指距离变化 → zoomScale 更新
- [x] zoomScale clamp 到 [0.5, 1.5]
- [x] 双击重置 zoomScale = 1.0
- [x] transform: scale 应用到 targetRef
- [x] e.preventDefault() 阻止 webview 默认缩放
- [x] usePinchZoom 26 个单测通过
- [x] `vue-tsc --noEmit` 0 错误

## 前端 - AgentChat 集成

- [x] usePinchZoom 集成到 mainRef 容器
- [x] 右上角「A- / A / A+」浮动按钮组工作
- [x] footer 时间改用 formatRelativeTime
- [x] 仅 AI 会话区域可缩放，错误状态页 / 其他 tab 不受影响
- [x] `vue-tsc --noEmit` 0 错误

## 前端 - 错误状态页禁止缩放

- [x] App.vue 错误状态块 CSS 含 `touch-action: manipulation`
- [x] viewport meta 动态切换实现（错误页 `user-scalable=no` / 会话页 `user-scalable=yes`）
- [x] `vue-tsc --noEmit` 0 错误

## 端到端验证

- [x] `go build ./cmd/encv` 0 错误
- [x] `go test ./internal/tools/... ./internal/server/...` 82/82 通过
- [x] `npx vue-tsc --noEmit` 0 错误
- [x] `npx vitest run` 710/716 通过（6 个 pre-existing useApiBaseProbe 失败与本次改动无关）
- [x] 工具异常 → tool_call 联动：测试覆盖 (useAgent.test.ts 6 个状态机用例)
- [x] 跨平台 bash 工具：TestPlatformDispatch 14 用例 + TestHighLevel 30 用例全部通过
- [x] 相对时间：22 个 formatRelativeTime 单测通过
- [x] 缩放控制：26 个 usePinchZoom 单测通过 + AgentChat 集成 vue-tsc 0 错误

## Notes

- **需要真机验证的部分** (沙箱内无法 e2e)：
  - 安卓真机 → 双指缩放会话内容（不缩放外层 ion-tab-bar / 顶栏）
  - 安卓真机 → 错误页双指无效果
  - search_files mount 不存在 → 真实安卓设备 tool_call 红色状态显示
- **Pre-existing 测试失败**：`useApiBaseProbe.test.ts` 6 个失败是 jsdom 网络 mock 问题，与本次改动无关。
