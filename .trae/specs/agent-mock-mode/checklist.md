# Checklist — Agent Mock 模式

> 验证标准：每项必须为 ✅ 才能视为完成。

---

## Mock 引擎核心

- [ ] MockEngine 核心类型定义完整（MockEngine / MockScenario / MockStep / Match / Run）
- [ ] 12 个内置剧本全部实现（default_friendly / list_files_query / encrypt_video / read_and_summarize / multi_step_search / streaming_error / truncation_long_text / reasoning_chain / tool_call_with_args / multi_tool_parallel / context_exhausted / chinese_greeting）
- [ ] Match 优先级正确：精确 > 关键词 > 正则 > fallback
- [ ] 关键词匹配不区分大小写
- [ ] 正则编译失败不 panic
- [ ] builtin 模式无匹配 → 返回 default_friendly
- [ ] custom 模式无匹配 → 返回 nil（走真实 API）

## 时间模拟

- [ ] MockSpeed=1.0 误差 ≤ ±50ms
- [ ] MockSpeed=0.1 实际 delay = DelayMs / 0.1
- [ ] MockSpeed=10 实际 delay = DelayMs / 10
- [ ] MockSpeed=0 零延迟同步推完

## 错误模拟

- [ ] mid_stream_disconnect 推 2 text_delta 后 SSE close
- [ ] sse_corrupt_chunk 推损坏 JSON 后前端不 panic
- [ ] tool_call_missing_field 后端 slog.Warn 跳过
- [ ] timeout 30s 无 stream_end 前端 abort
- [ ] upstream_5xx 立即推 stream_status(5xx)

## 关键剧本（[真机问题] 验证）

- [ ] list_files_query 触发 2 个 tool_call + 2 个 tool_result
- [ ] list_files_query 工具调用渲染为 GroupedOperationMessage（不是裸 JSON）
- [ ] list_files_query 回答完整不截断（text_delta 总和 = 剧本预期）
- [ ] multi_step_search 验证 3 轮 tool_call 递归逻辑

## 集成到 handleAgentChat

- [ ] cfg.Agent.MockMode != "off" 时走 mock 路径
- [ ] 响应头含 X-Mock-Scenario
- [ ] 响应头含 X-Mock-Mode
- [ ] 第一个 SSE 事件含 mock: true 字段
- [ ] 真实 OpenAI 路径代码完全不变
- [ ] builtin fallback 失败 → 走真实
- [ ] custom fallback 失败 → 走真实

## Config 字段

- [ ] agent_settings.mock_mode（select: off/builtin/custom）
- [ ] agent_settings.mock_speed（number: 0-10, step 0.1）
- [ ] agent_settings.mock_scenarios（array of object）
- [ ] Settings 二级页能渲染新字段（沿用 ConfigFieldItem）
- [ ] 保存走 useConfig.saveConfig()

## 前端

- [ ] useAgent 暴露 isMockMode / mockScenario
- [ ] processSSE 解析响应头 + 第一个事件 mock: true
- [ ] AgentChat.vue 顶部显示「🧪 模拟」徽章
- [ ] 徽章 tooltip 显示 scenario ID
- [ ] 徽章样式沿用 StatusBadge idle tone + flaskOutline

## i18n

- [ ] agent.mockBadge / mockBadgeTooltip zh + en
- [ ] agent.mockMode / mockModeOff / mockModeBuiltin / mockModeCustom zh + en
- [ ] agent.mockSpeed / mockSpeedHelp zh + en
- [ ] settings.mockBuiltinHint / mockCustomHint / mockScenarios zh + en

## 编译 / 测试

- [ ] go build ./cmd/encv 0 错误
- [ ] go test ./internal/server/... 全部通过
- [ ] vue-tsc --noEmit 0 错误
- [ ] vite build 0 错误
- [ ] start-preview.sh 启动后 /health 返回 200

## 端到端验证

- [ ] 设置 mock_mode=builtin → 提问"有哪些视频文件" → 工具调用 = 结构化卡片
- [ ] 提问"触发超时" → 错误 toast 显示
- [ ] 设置 mock_speed=0.1 → SSE 明显变慢
- [ ] 自定义剧本（custom 模式）→ 用户输入命中关键词 → 触发自定义剧本

---

# 增量 Checklist：Mock 剧本预设输入控件

## 数据结构 & SSE 协议

- [x] `MockPreset` 类型定义完整（ID / Label / UserText / Icon / Tooltip）
- [x] `MockScenario.Presets []MockPreset` 字段
- [x] `mock_presets` SSE 事件类型（data: `{ scenario, phase, presets: MockPreset[] }`）
- [x] `mock_presets_clear` SSE 事件类型
- [x] stream_start 之后**立即**推初始 mock_presets
- [x] stream_end 之后**立即**推 mock_presets_clear
- [x] mid-scenario step 可推 mock_presets 透传更新（连续会话）
- [x] 12 个剧本**全部**包含 Presets 字段（每个 ≥3 个）
- [x] `multi_step_search` 剧本在 mid-scenario 推第二组 mock_presets

## execute_real 真实工具执行

- [x] `MockStep.ExecuteReal bool` 字段
- [x] `MockEngine.realExecutor` 字段 + `SetRealExecutor` 方法
- [x] `pendingRealCalls` map 追踪 execute_real=true 的 tool_call
- [x] tool_result 事件处理时调 realExecutor 拿真结果覆盖硬编码
- [x] nil realExecutor 时 fallback 到硬编码（向后兼容）
- [x] `server.NewServer` 在 return 前调 `SetRealExecutor(s.executeAgentTool)`
- [x] 12 个剧本**所有** tool_call 标 `execute_real: true`

## 前端 useAgent

- [x] `AgentEventType` 联合类型含 `mock_presets` / `mock_presets_clear`
- [x] `MockPreset` interface 导出
- [x] `mockPresets` / `mockPresetsPhase` / `mockPresetsScenario` ref 声明
- [x] `case 'mock_presets':` 解析 JSON 覆盖 mockPresets.value
- [x] `case 'mock_presets_clear':` 清空三个 ref
- [x] `pickMockPreset(preset)` 函数 → `send(preset.userText, { mode: 'start' })`
- [x] 状态检查（busy 时静默忽略）
- [x] export 4 个新成员（mockPresets / mockPresetsPhase / mockPresetsScenario / pickMockPreset）

## 前端 MockPresetBar 组件

- [x] props：`presets` / `scenario` / `phase` / `disabled`
- [x] emits：`pick [preset: MockPreset]`
- [x] header（🧪 + scenario 名 + phase + 「点击直接发送」hint）
- [x] chip 列表（水平滚动 + icon + label）
- [x] 暗黑模式适配（半透明 primary tint）
- [x] 流式进行中 disabled 状态（防止重复触发）
- [x] v-if = `isMockMode && mockPresets.length > 0`（AgentChat 集成）
- [x] 位于 AttachmentTray 之后、footerInputRow 之前（输入框上方覆盖）

## i18n 增量

- [x] `agent.mockPresetBarAria` zh + en
- [x] `agent.mockPresetBarDefaultScenario` zh + en
- [x] `agent.mockPresetBarHint` zh + en

## 单元测试

- [x] `TestMockEngine_ExecuteReal_OverridesHardcoded`
- [x] `TestMockEngine_ExecuteReal_FallbackWhenNoExecutor`
- [x] `TestMockEngine_ExecuteReal_ErrorPropagatesAsIsError`
- [x] `TestMockEngine_EmitsInitialPresetsOnStreamStart`
- [x] `TestMockEngine_NoPresetsWhenScenarioEmpty`
- [x] `TestMockEngine_MidScenarioPresetUpdate`
- [x] `TestMockEngine_EmitsClearOnStreamEnd`
- [x] `TestMockEngine_AllBuiltinScenariosHavePresets`

## 编译 / 构建

- [x] go build ./cmd/encv 0 错误
- [x] pnpm vue-tsc --noEmit 0 错误
- [x] pnpm vite build 0 错误
- [x] go test ./internal/server/... 全部通过
