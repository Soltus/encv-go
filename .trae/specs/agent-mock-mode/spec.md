# Agent Mock 模式 Spec — 不计费的模拟 LLM 输出

## Why

当前 agent 服务每次用户提问都会调用真实 OpenAI API（gptgod 代理或 OpenAI 官方），导致：

1. **开发成本不可控** — 调试时反复触发真实 LLM，消耗付费额度；gptgod 代理对 QPS/日额度有限制
2. **CI 阻塞** — 没有 API key 的环境（CI runner、新人 onboarding）无法跑端到端测试
3. **演示 / 截图不稳定** — 真实 LLM 响应非确定性，无法对截图/e2e 做断言
4. **离线开发不可行** — 飞机模式 / 断网时无法继续开发 UI 逻辑
5. **回归保护缺失** — 前端流式渲染、工具调用 SSE 协议、ContextPopover 等组件改动后无自动化验证

**新方案核心价值**：

1. **零成本** — 切换到 mock 模式后所有"对话"由本地剧本驱动，0 token 消耗
2. **确定性** — 相同输入永远得到相同输出，可做 e2e 断言
3. **场景覆盖** — 内置 6 大类情景剧本（纯文本 / 工具调用 / 多轮 / 错误 / 截断 / reasoning），覆盖代码所有分支
4. **可扩展** — 用户可在 `agent_settings.mock_scenarios` 自定义剧本
5. **UI 完全不变** — 前端代码 0 改动，因为 mock 输出的 SSE 事件流与真实 agent 完全一致
6. **可观测** — 顶层添加 `X-Mock-Scenario` 响应头 / `mock: true` 字段，前端可显式标记

---

## What Changes

### 新增

- `internal/server/agent_mock.go` — Mock 引擎核心
  - `MockEngine` 结构体（剧本库 + 当前激活剧本 + step cursor）
  - `MockScenario` 类型（id / description / trigger 匹配规则 / steps[]）
  - `MockStep` 类型（事件列表：text_delta / reasoning_delta / tool_call / tool_status / tool_result / stream_end）
  - 内置 12+ 剧本（覆盖所有代码分支）
  - `Match(userText string) *MockScenario` — 关键词/正则匹配触发哪个剧本
  - `Run(ctx, sess, stepCh chan<- agentLoopEvent) error` — 推 SSE 事件到 stepCh
  - 时间模拟（可调速：1x / 0.5x / 实时）
  - 错误模拟（中途断流 / SSE chunk 损坏 / tool_call 缺少必填字段）
- `internal/config/config.go` — 新增 `Agent.MockMode` 字段（`"off"` | `"builtin"` | `"custom"`）
- `internal/config/config.go` — 新增 `Agent.MockSpeed` 字段（默认 `1.0`，范围 0.1-10.0）
- `internal/config/config.go` — 新增 `Agent.MockScenarios []MockScenario` 字段（custom 模式时读取）

### 修改

- `internal/server/agent_api.go` — `handleAgentChat` / `handleAgentConfirm` 在入口处增加分支：
  - 若 `cfg.Agent.MockMode != "off"` → 不调 `callOpenAIStream`，改调 `mockEngine.Run()`
  - 把 mock 产生的事件 `agentLoopEvent` 复用现有 `streamChat` 的事件循环（0 改动）
- `internal/config/config.go` — schema-driven 字段支持（`agent_settings.mock_mode` / `mock_speed` / `mock_scenarios`）
- `app/encv-mobile/src/views/AgentChat.vue` — 当 `useAgent` 拿到 `mock: true` 字段时显示「模拟模式」徽章（不阻断 UI）
- `app/encv-mobile/src/i18n/agent.ts` — 新增 `agent.mockBadge` / `agent.mockBadgeTooltip`

### 不影响

- 前端 `useAgent.ts` SSE 解析器（mock 输出与真实输出走同一通道）
- `renderTurnItems.ts` 渲染管线（mock 工具调用渲染为正常 GroupedOperationMessage）
- 真实 OpenAI 调用路径（mock 模式关闭时完全不变）

---

## ADDED Requirements

### Requirement: Mock 模式开关

`config.Agent.MockMode` SHALL 控制 mock 行为，3 个枚举值：

- `"off"`（默认）— 真实 OpenAI 调用
- `"builtin"` — 启用内置 12+ 剧本（无需配置）
- `"custom"` — 仅使用 `cfg.Agent.MockScenarios` 自定义剧本（忽略内置）

#### Scenario: Mock 模式持久化

- **WHEN** 用户在 Settings → Agent 二级页选择 mock 模式
- **THEN** 写入 `config.user.json` 的 `agent_settings.mock_mode` 字段
- **AND** 前端 `useAgent` 的 `configLoaded` 状态触发 refetch
- **AND** 下次 `POST /api/chat` 走 mock 路径

#### Scenario: Mock 模式运行时切换

- **WHEN** 服务运行中修改 `config.user.json` 的 `mock_mode`
- **THEN** **不**自动 reload（避免半路切换导致事件流混乱）
- **AND** 下次新会话生效
- **AND** 已在 streaming 的会话不受影响（继续走原路径）

---

### Requirement: 触发匹配（Trigger Matching）

`MockEngine.Match(userText)` SHALL 根据用户最后一条消息匹配激活剧本。

#### Scenario: 匹配规则优先级

- **WHEN** `MockEngine.Match` 被调用
- **THEN** 按以下优先级匹配：
  1. **精确匹配** — `userText == scenario.ExactMatch`
  2. **关键词匹配** — `scenario.Keywords` 数组中**任一**关键词出现在 userText（不区分大小写）
  3. **正则匹配** — `scenario.Regex` 编译后匹配 userText
  4. **默认回退** — 匹配 `scenario.ID == "default_friendly"` 的剧本
- **AND** 第一个匹配命中即返回，后续不再尝试
- **AND** 全部不匹配 → 返回 nil（走真实 OpenAI，**仅当** mock_mode == "builtin" 时强制返回 default）

#### Scenario: 内置剧本清单（12 个）

```
ID                        | 触发关键词            | 用途
--------------------------|----------------------|-------------------------------
default_friendly          | （fallback 默认）    | 纯文本闲聊，3 段流式
list_files_query          | "有哪些文件"         | 触发 list_mounts + list_files
encrypt_video             | "加密视频"           | 触发 plugin 工具 video_encrypt
read_and_summarize        | "总结" + 文件名      | 触发 read_file + 总结
multi_step_search         | "搜索" + "视频"      | 多轮：list_files → grep → read_file
streaming_error           | "触发错误"           | 流中途发送 stream_status=error
truncation_long_text      | "写一篇长文"         | 输出超 2000 字符，验证截断
reasoning_chain           | "推理"               | reasoning_delta + text_delta 交替
tool_call_with_args       | "调用工具"           | 单 tool_call 带完整 args JSON
multi_tool_parallel       | "批量"               | 同时 2-3 个 tool_call
context_exhausted         | "上下文爆了"         | 模拟 finish_reason=length 警告
chinese_greeting          | "你好"               | 验证中文分词 + 字符级流式
```

#### Scenario: 触发响应头

- **WHEN** MockEngine 激活某个剧本
- **THEN** HTTP 响应头加 `X-Mock-Scenario: <scenario_id>` + `X-Mock-Mode: builtin|custom`
- **AND** SSE 事件流中第一个 `stream_start` 事件 data 含 `{mock: true, scenario: "..."}`
- **AND** 前端 useAgent 检测到 `mock: true` 后，display 一个「🧪 模拟模式」灰色徽章在会话顶部

---

### Requirement: Mock 剧本结构

`MockScenario` SHALL 由 `steps` 数组驱动，每个 step 是一组事件 + 时间间隔。

```go
type MockScenario struct {
    ID          string
    Description string
    ExactMatch  string    // 精确匹配字符串
    Keywords    []string  // 关键词匹配
    Regex       string    // 正则匹配
    Steps       []MockStep
}

type MockStep struct {
    DelayMs    int                  // 推送前延迟（毫秒）
    Events     []agentLoopEvent     // 该 step 要推的所有事件
}
```

#### Scenario: 事件序列化

- **WHEN** `MockEngine.Run` 推 `agentLoopEvent`
- **THEN** 走**现有**的 `s.sendAndCache` 通道（与真实路径完全相同）
- **AND** 事件结构与 `callOpenAIStream` 输出**完全一致**：
  - `text_delta` → `Data: {seq, text}`  // 复用 {seq,text} 修复后的格式
  - `reasoning_delta` → `Data: {seq, text}`
  - `tool_call` → `Data: ToolCallData{ID, Name, Args, AutoRun, Kind}`
  - `tool_status` → `Data: {id, status}`  // running → success
  - `tool_result` → `Data: ToolResultData{ID, Name, Result, IsError, Status, DurationMs}`
  - `stream_end` → `Data: {usage, finishReason}`

#### Scenario: 时间模拟

- **WHEN** `cfg.Agent.MockSpeed != 1.0`
- **THEN** 所有 `DelayMs` 实际等待 = `DelayMs / MockSpeed`
- **AND** `MockSpeed = 0.1` → 10x 慢放（用于调试 SSE 事件流细节）
- **AND** `MockSpeed = 10.0` → 10x 快进（用于 e2e 测试）
- **AND** `MockSpeed = 0` → 零延迟（同步推完整个剧本）

---

### Requirement: 关键剧本设计

#### Scenario: `list_files_query` 剧本（修复 [真机测试] 问题的核心场景）

- **WHEN** 用户输入 "有哪些视频文件"
- **THEN** 剧本推送序列：
  ```
  Step 1 (delay 0):
    - text_delta: "好的，我先查看挂载点。\n\n"
  
  Step 2 (delay 500ms):
    - tool_call: {id: "call_1", name: "list_mounts", args: {}, kind: "fileRead"}
    - tool_status: {id: "call_1", status: "running"}
  
  Step 3 (delay 300ms):
    - tool_status: {id: "call_1", status: "success"}
    - tool_result: {id: "call_1", name: "list_mounts", 
                    result: '{"count":1,"items":[{"id":"serving",...}]}',
                    status: "success", durationMs: 12}
  
  Step 4 (delay 400ms):
    - text_delta: "已找到挂载点 serving。继续查看 Movies 目录..."
  
  Step 5 (delay 500ms):
    - tool_call: {id: "call_2", name: "list_files", 
                   args: '{"mount_id":"serving","rel_path":"Movies"}', 
                   kind: "fileRead"}
    - tool_status: {id: "call_2", status: "running"}
  
  Step 6 (delay 400ms):
    - tool_status: {id: "call_2", status: "success"}
    - tool_result: {id: "call_2", name: "list_files",
                    result: '{"files":[{"name":"studio_video.mp4","size":554000000},...]}',
                    status: "success", durationMs: 18}
  
  Step 7 (delay 300ms):
    - text_delta: "在 /Movies 目录下发现 1 个视频文件：\n\n"
    - text_delta: "- studio_video_1762059800961.mp4 (约 554MB)\n\n"
    - text_delta: "其他条目都是子目录，如 QQ、Subtitles、qqmusic..."
    - text_delta: "如需进一步查看子目录内容，请告诉我。"
  
  Step 8 (delay 100ms):
    - stream_end: {finishReason: "stop"}
  ```
- **AND** 这个剧本**完整覆盖真机问题**：(a) 工具调用以结构化组件渲染（不是裸 JSON），(b) 回答完整不截断

#### Scenario: `streaming_error` 剧本（错误分支覆盖）

- **WHEN** 用户输入 "触发错误"
- **THEN** 剧本推送：
  ```
  Step 1: text_delta: "正在处理..."
  Step 2 (delay 800ms): stream_status: {type: "error", code: "upstream_timeout", 
                                       message: "上游 LLM 服务超时（模拟）"}
  Step 3: stream_end: {finishReason: "error"}
  ```
- **AND** 前端 `useAgent` 收到 `stream_status` 事件触发 error toast

#### Scenario: `truncation_long_text` 剧本（截断覆盖）

- **WHEN** 用户输入 "写一篇长文"
- **THEN** 剧本推送 1 个 text_delta 含 3000 字符的 lorem ipsum
- **AND** 验证 SSE {seq,text} 排序重建能正确处理超长单 chunk

#### Scenario: `reasoning_chain` 剧本（reasoning 覆盖）

- **WHEN** 用户输入 "推理一下"
- **THEN** 剧本交替推送 reasoning_delta + text_delta：
  ```
  Step 1: reasoning_delta: "让我先分析问题..."
  Step 2: reasoning_delta: "需要考虑 X、Y、Z 三个因素。"
  Step 3: text_delta: "根据以上分析，"
  Step 4: text_delta: "我的答案是..."
  ```
- **AND** 验证 `useAgent` 的 `appendSequencedChunk` 对 reasoning 字段的 Map 排序逻辑

#### Scenario: `multi_tool_parallel` 剧本（并行工具调用覆盖）

- **WHEN** 用户输入 "批量加密"
- **THEN** 剧本在同一 step 中推送 3 个 tool_call（不同 ID）
- **AND** 验证 `streamChat` 的工具累积逻辑能正确处理并发工具调用

#### Scenario: `context_exhausted` 剧本（finish_reason=length 覆盖）

- **WHEN** 用户输入 "上下文爆了"
- **THEN** 剧本推送：
  ```
  Step 1: text_delta: "（长文本达到模型上限）"
  Step 2: stream_end: {finishReason: "length", 
                       usage: {totalTokens: 131072, maxTokens: 128000}}
  ```
- **AND** 验证 `useAgent` 检测到 `finish_reason === "length"` 显示黄色警告

#### Scenario: `chinese_greeting` 剧本（中文分词 + 字符级流式覆盖）

- **WHEN** 用户输入 "你好"
- **THEN** 剧本按字符级推送：`text_delta: "你"` → `"好"` → `"，" ` → `"我"` → `...`
- **AND** 验证单字符 chunk 累积不丢字

---

### Requirement: 自定义剧本（Custom Mode）

`cfg.Agent.MockScenarios` SHALL 允许用户通过 Settings 二级页添加自定义剧本。

#### Scenario: 自定义剧本格式

```json
{
  "id": "my_custom_scenario",
  "description": "演示如何添加自定义 mock 剧本",
  "keywords": ["演示", "demo"],
  "steps": [
    { "delayMs": 0, "events": [
        {"type": "text_delta", "data": {"seq": 1, "text": "这是自定义剧本。"}}
      ]
    }
  ]
}
```

#### Scenario: 自定义剧本加载

- **WHEN** `cfg.Agent.MockMode == "custom"` 且 `cfg.Agent.MockScenarios` 非空
- **THEN** 启动时验证每个 scenario：
  - ID 必须非空且全局唯一
  - keywords / regex / exactMatch 至少有一个非空
  - events 每个必须有合法 type
  - 验证失败 → slog.Warn 跳过该 scenario
- **AND** `MockEngine.Match` 只在自定义剧本中匹配
- **AND** 如果一个都没匹配到 → 走**真实 OpenAI**（custom 模式不强制 fallback）

#### Scenario: 热重载自定义剧本

- **WHEN** `config.user.json` 改动 `agent_settings.mock_scenarios`
- **THEN** **不**自动 reload（与 MockMode 一致）
- **AND** Server 在每次 `handleAgentChat` 入口重新读 `cfg`（已经是单例引用，热路径是直接读取）

---

### Requirement: Mock 引擎与真实路径的统一接口

`MockEngine.Run` SHALL 输出与 `callOpenAIStream` **字节级一致**的事件流。

#### Scenario: 事件结构契约

| 事件类型 | Mock 输出 | 真实 OpenAI 输出 | 必须一致 |
|---------|----------|-----------------|---------|
| `text_delta` | `{"seq":1,"text":"hi"}` | `{"seq":1,"text":"hi"}` | ✅ |
| `reasoning_delta` | `{"seq":1,"text":"..."}` | `{"seq":1,"text":"..."}` | ✅ |
| `tool_call` | `ToolCallData{ID,Name,Args,AutoRun,Kind}` | `ToolCallData{ID,Name,Args,AutoRun,Kind}` | ✅ |
| `tool_status` | `{"id":"call_1","status":"running"}` | `{"id":"call_1","status":"running"}` | ✅ |
| `tool_result` | `ToolResultData{...}` | `ToolResultData{...}` | ✅ |
| `stream_end` | `{"finishReason":"stop","usage":{...}}` | `{"finishReason":"stop","usage":{...}}` | ✅ |

- **AND** 所有 `seq` 计数器在 mock 路径下也全局递增（与真实路径行为一致）
- **AND** 所有事件走 `s.sendAndCache` 通道，自动写入 `sess.EventCache`（支持断点续传）
- **AND** 触发后，**前端代码 0 改动**即可消费

#### Scenario: 集成到 handleAgentChat

```go
func (s *Server) handleAgentChat(c *gin.Context) {
    // ... 现有解析 body / 鉴权逻辑 ...
    
    cfg := s.getAgentConfig()
    if cfg.MockMode != "off" {
        // Mock 路径
        scenario := s.mockEngine.Match(lastUserText)
        if scenario != nil {
            c.Header("X-Mock-Scenario", scenario.ID)
            c.Header("X-Mock-Mode", cfg.MockMode)
            s.streamMockScenario(c, sess, scenario, ...)  // 推 SSE
            return
        }
        // builtin 模式 fallback 失败 → 走真实
        if cfg.MockMode == "custom" {
            // 真实调用
        }
    }
    
    // 真实 OpenAI 路径（完全不变）
    streamCh, err := callOpenAIStream(...)
    // ...
}
```

---

### Requirement: 错误模拟（用于测试前端错误处理）

Mock 引擎 SHALL 支持在剧本中途注入错误，验证前端错误处理代码路径。

#### Scenario: 错误类型枚举

| `errorType` 字段 | 触发行为 | 前端预期 |
|----------------|---------|---------|
| `"mid_stream_disconnect"` | 推 2 个 text_delta 后关闭 SSE 连接 | 显示「连接中断」 |
| `"sse_corrupt_chunk"` | 推送损坏的 SSE 数据（`data: NOT-JSON\n\n`） | useAgent catch JSON 错误 |
| `"tool_call_missing_field"` | tool_call 事件缺 Name 字段 | 后端 slog.Warn 跳过 |
| `"timeout"` | step 1 推完后 hang 30s 不推 stream_end | 前端 abort 超时 |
| `"upstream_5xx"` | 立即推 `stream_status{code:"upstream_5xx"}` | 显示「上游服务异常」 |

#### Scenario: 触发错误模拟

- **WHEN** 用户输入 "触发超时"（命中 `streaming_error` 剧本）
- **THEN** 该剧本在 step 2 推 `stream_status{type:"error",code:"upstream_timeout"}`
- **AND** 前端 useAgent 收到 stream_status 事件触发错误 toast + status='error'

---

### Requirement: 前端「模拟模式」徽章

`useAgent` SHALL 在检测到 mock 输出时显示常驻徽章。

#### Scenario: 徽章展示

- **WHEN** 后端响应头含 `X-Mock-Mode: builtin|custom`
- **THEN** useAgent 设置 `isMockMode = ref(true)`
- **AND** `AgentChat.vue` 顶部 header 显示「🧪 模拟模式」灰色徽章
- **AND** 徽章 tooltip 显示当前激活的 `scenario_id`（从 `X-Mock-Scenario` 头读取）
- **AND** 徽章点击不展开（仅 tooltip 提示），保持非侵入

#### Scenario: 徽章样式（沿用现有 StatusBadge 模式）

- 位置：会话 header 标题右侧、ContextIcon 左侧
- 颜色：`tone="idle"`（灰色）
- icon：ionicons `flaskOutline`（烧瓶，象征模拟）
- 文案：`{{ t('agent.mockBadge') }}` = "模拟"

---

### Requirement: i18n 文案

| key | zh-CN | en |
|-----|-------|---|
| `agent.mockBadge` | 模拟 | Mock |
| `agent.mockBadgeTooltip` | 当前为 mock 模式（不计费），场景：{scenario} | Mock mode active, scenario: {scenario} |
| `agent.mockMode` | 模拟模式 | Mock Mode |
| `agent.mockModeOff` | 关闭（真实 API） | Off (Real API) |
| `agent.mockModeBuiltin` | 内置剧本 | Built-in Scenarios |
| `agent.mockModeCustom` | 自定义剧本 | Custom Scenarios |
| `agent.mockSpeed` | 播放速度 | Playback Speed |
| `agent.mockSpeedHelp` | 1.0 = 正常速度，0.1 = 慢放，10 = 快进 | 1.0 normal, 0.1 slow, 10 fast |
| `settings.mockBuiltinHint` | 内置 12 个情景剧本，自动匹配用户输入 | 12 built-in scenarios auto-match user input |
| `settings.mockCustomHint` | 编辑 agent_settings.mock_scenarios 自定义剧本 | Edit agent_settings.mock_scenarios to customize |

---

### Requirement: 配置字段 schema

`config.schema.json` SHALL 新增以下字段（沿用 schema-driven ConfigFieldItem 渲染）：

```json
{
  "agent_settings": {
    "type": "object",
    "properties": {
      "mock_mode": {
        "type": "select",
        "options": ["off", "builtin", "custom"],
        "default": "off",
        "description": "agent.mockMode"
      },
      "mock_speed": {
        "type": "number",
        "min": 0,
        "max": 10,
        "step": 0.1,
        "default": 1.0,
        "description": "agent.mockSpeedHelp"
      },
      "mock_scenarios": {
        "type": "array",
        "items": { "type": "object" },
        "default": [],
        "description": "agent.mockScenarios"
      }
    }
  }
}
```

#### Scenario: ConfigFieldItem 渲染

- **WHEN** 用户进入 Settings → Agent 二级页
- **THEN** `mock_mode` 渲染为下拉选择
- **AND** `mock_speed` 渲染为 number input
- **AND** `mock_scenarios` 渲染为「行式编辑」数组（每行 JSON 文本框 + 校验）

---

### Requirement: 单元测试覆盖

`internal/server/agent_mock_test.go` SHALL 覆盖以下场景：

#### Scenario: MockEngine.Match 测试

- [x] 精确匹配优先级 > 关键词 > 正则 > fallback
- [x] 关键词不区分大小写
- [x] 正则编译失败不 panic
- [x] builtin 模式无匹配 → 返回 default_friendly
- [x] custom 模式无匹配 → 返回 nil

#### Scenario: 12 个内置剧本执行测试

- [x] `default_friendly` → 收到 3 个 text_delta + stream_end
- [x] `list_files_query` → 收到 2 个 tool_call + 2 个 tool_result + 文本
- [x] `streaming_error` → 收到 stream_status(error) + stream_end
- [x] `truncation_long_text` → 单个 text_delta > 2000 字符
- [x] `reasoning_chain` → reasoning_delta + text_delta 交替
- [x] `multi_tool_parallel` → 同 step 内 3 个 tool_call
- [x] `context_exhausted` → finishReason=length
- [x] `chinese_greeting` → 单字符 chunk 不丢字
- [x] `encrypt_video` → 1 个 tool_call (kind=fileChange)
- [x] `read_and_summarize` → 1 个 read_file tool_call
- [x] `multi_step_search` → 3 轮 tool_call（验证递归）
- [x] `tool_call_with_args` → 完整 args JSON 不截断

#### Scenario: 错误模拟测试

- [x] `mid_stream_disconnect` → 推 2 个 text_delta 后 SSE close
- [x] `sse_corrupt_chunk` → 推损坏数据后 useAgent 不 panic
- [x] `tool_call_missing_field` → 后端跳过
- [x] `timeout` → 30s 后无 stream_end，前端 abort
- [x] `upstream_5xx` → 立即推 stream_status(5xx)

#### Scenario: 时间模拟测试

- [x] MockSpeed=1.0 → 实际 delay 与剧本 DelayMs 一致（±50ms 误差）
- [x] MockSpeed=0.1 → 实际 delay ≈ DelayMs / 0.1 = 10x DelayMs
- [x] MockSpeed=10 → 实际 delay ≈ DelayMs / 10
- [x] MockSpeed=0 → 零延迟，同步推完

#### Scenario: 端到端集成测试

- [x] `POST /api/chat` + MockMode=builtin + "有哪些视频文件" → 触发 list_files_query 剧本
- [x] 响应头含 `X-Mock-Scenario: list_files_query`
- [x] 响应头含 `X-Mock-Mode: builtin`
- [x] SSE 事件流第一个 `stream_start` 含 `mock: true`
- [x] 事件总数 = 剧本 steps 总事件数
- [x] 完成后 status='idle'

#### Scenario: 关键文件 / 函数

- `internal/server/agent_mock.go` — MockEngine + 12 个内置剧本 + Run/Match
- `internal/server/agent_mock_test.go` — 30+ 单元测试
- `internal/server/agent_api.go` — `handleAgentChat` / `handleAgentConfirm` 增加 mock 分支
- `internal/config/config.go` — `Agent.MockMode/MockSpeed/MockScenarios` 字段
- `app/encv-mobile/src/composables/useAgent.ts` — 检测 `X-Mock-Mode` 头 + `mock: true` 字段
- `app/encv-mobile/src/views/AgentChat.vue` — 顶部「模拟模式」徽章
- `app/encv-mobile/src/i18n/agent.ts` — 8 个新 i18n key
- `app/encv-mobile/src/i18n/settings.ts` — 3 个新 i18n key

---

## MODIFIED Requirements

### Requirement: handleAgentChat 增加 mock 分支

**BEFORE**: `handleAgentChat` 总是调 `callOpenAIStream` → 真实 OpenAI
**AFTER**: `handleAgentChat` 入口处先检查 `cfg.Agent.MockMode`，非 off 走 mock 路径

#### Scenario: Mock 路径不修改现有结构

- **THEN** 真实路径代码**完全不变**
- **AND** mock 路径复用 `s.sendAndCache` + `sess.EventCache` + 事件循环
- **AND** 仅在 `streamChat` 函数**外部**做分支（不侵入函数内部）

### Requirement: useAgent 增加 isMockMode ref

**BEFORE**: useAgent 不感知 mock 模式
**AFTER**: useAgent 检测响应头 + 第一个 SSE 事件 `mock: true` 字段

#### Scenario: 徽章状态机

- 初始：`isMockMode = ref(false)`
- 收到第一个事件 `mock: true` → `isMockMode = ref(true)`
- 用户切回真实 API 重新发送 → 重置为 `false`
- 不持久化（每次会话独立判断）

---

## REMOVED Requirements

无（仅新增，不删除任何现有能力）

---

## 约束与限制

1. **Mock 输出**与**真实输出**走**同一** SSE 通道，事件结构字节级一致 — 前端代码 0 改动
2. **Mock 模式**与**真实模式**通过 schema 字段切换，不支持运行时热切换（避免半路切换导致事件流混乱）
3. **内置剧本**至少 12 个，覆盖所有代码分支（纯文本/工具/多轮/错误/截断/reasoning/并行工具/finish_reason/中文/单字符/无匹配/含 args）
4. **自定义剧本**通过 `agent_settings.mock_scenarios` JSON 数组配置，custom 模式无匹配走真实 OpenAI
5. **响应头标识**：`X-Mock-Scenario` / `X-Mock-Mode` + 第一个事件 `mock: true` 字段，让前端可显式标记
6. **错误模拟**通过 `errorType` 字段注入，覆盖前端所有错误处理路径
7. **时间模拟**通过 `mock_speed` 字段控制（0.1x ~ 10x），用于 e2e 测试加速或调试慢放
8. **不走工具执行**：mock 模式下的 tool_call 走前端展示但**不**调真实 handler（避免文件系统被真实修改），但 tool_result 事件仍按剧本推送，让前端渲染完整
9. **schema 驱动**：`mock_mode` / `mock_speed` / `mock_scenarios` 走现有 `ConfigFieldItem` 渲染（无需新增 UI 组件）

---

## 与现有 spec 的关系

| 现有 spec | 影响 |
|----------|------|
| `go-in-process-agent` | 在其 `handleAgentChat` 增加 mock 分支；agentContextUsage / agentFsBridge 不变 |
| `mock-router-refactor` | 完全独立：mock-router 是前端 dev 中间件，agent-mock 是后端 mock LLM 引擎 |
| `fix-toolcall-rendering-and-truncation` | mock 剧本 `list_files_query` 直接验证该 spec 修复的 [真机问题] |

---

## 验证步骤

1. **单元测试** — `cd /workspace && go test ./internal/server/... -run TestMockEngine -v` 全部通过
2. **类型检查** — `go build ./cmd/encv` 0 错误
3. **前端类型** — `npx vue-tsc --noEmit` 0 错误
4. **前端构建** — `npx vite build` 0 错误
5. **集成验证** — 启动服务 → 设置 mock_mode=builtin → 提问"有哪些视频文件" → 验证：
   - 响应头 `X-Mock-Scenario: list_files_query`
   - 工具调用显示为 GroupedOperationMessage 结构化卡片
   - 回答完整不截断
   - 顶部「🧪 模拟模式」徽章显示
6. **错误模拟验证** — 提问"触发超时" → 验证错误 toast 显示
7. **时间模拟验证** — 设置 `mock_speed=0.1` → 验证 SSE 事件明显变慢

---

## ADDED Requirements (增量 #1：Mock 剧本预设输入控件)

> **触发背景**：用户反馈 mock 模式下要提供"分支选择"入口 — 同一剧本的不同用户输入（不同分支、不同结果）以及高级剧本的连续会话预设都应在剧本运行前/中以可点击 chip 形式呈现，而不是让用户手敲。

### Requirement: MockPreset 数据结构

`MockScenario` SHALL 支持可选的 `Presets []MockPreset` 字段。每个预设是一个可点击的快速输入按钮。

```go
type MockPreset struct {
    ID       string `json:"id"`       // 全局唯一，用于前端 data-testid
    Label    string `json:"label"`    // chip 上显示的短文本
    UserText string `json:"userText"` // 点击后注入到对话的 user 消息
    Icon     string `json:"icon,omitempty"`    // 可选 emoji
    Tooltip  string `json:"tooltip,omitempty"` // 可选长描述
}
```

#### Scenario: 12 个内置剧本**必须**包含 Presets

- **WHEN** `agent_mock_scenarios.go` 的 `builtinScenarios` 列表加载
- **THEN** 12 个剧本每一个都 SHALL 至少包含 3 个 Preset（保证用户进入剧本立刻有可选项）
- **AND** 每个 Preset 的 `userText` SHALL 是能匹配其他剧本触发词的真实 user 消息（演示跨剧本跳转）
- **AND** 验收由 `TestMockEngine_AllBuiltinScenariosHavePresets` 单元测试强制检查

#### Scenario: 高级剧本连续会话预设

- **WHEN** 高级剧本（≥3 轮 tool_call）运行
- **THEN** 该剧本 SHALL 在 mid-scenario step 推送**第二次** `mock_presets` 事件以更新预设列表
- **AND** 第二次推送的 presets 反映当前剧本进度（阶段名 `phase` 不同）
- **AND** 前端每次 `mock_presets` 事件**完整替换**当前 chip 列表（mid-scenario 更新天然覆盖）

### Requirement: mock_presets SSE 事件协议

后端 MockEngine SHALL 在以下时机推 `mock_presets` 事件：

1. `stream_start` 事件**之后**立刻推**初始** `mock_presets`（scenario 当前阶段的预设）
2. 任意 mid-scenario step 内可推**更新版** `mock_presets`（实现连续会话）
3. `stream_end` 事件**之后**立刻推 `mock_presets_clear`（清空 chip 列表）

#### Scenario: 事件 data 形状

```json
// mock_presets
{
    "scenario": "list_files_query",
    "phase": "initial",
    "presets": [
        {"id": "p_qq", "label": "QQ 目录", "userText": "查看 QQ 目录", "icon": "📁", "tooltip": "..."},
        ...
    ]
}

// mock_presets_clear
{}
```

#### Scenario: stream_end 后立刻清空

- **WHEN** MockEngine 推 `stream_end` 事件
- **THEN** 紧跟着推一个 `mock_presets_clear` 事件
- **AND** 前端 `useAgent` 收到 `mock_presets_clear` 后清空 `mockPresets` ref
- **AND** 验收由 `TestMockEngine_EmitsClearOnStreamEnd` 单元测试强制检查

### Requirement: 前端 MockPresetBar 组件

`MockPresetBar.vue` SHALL 是 AgentChat 输入框上方的 chip 列表组件，仅在 mock 模式 + 有预设时显示。

#### Scenario: 组件契约

```vue
<MockPresetBar
    :presets="mockPresets"
    :scenario="mockScenario"
    :phase="mockPresetsPhase"
    :disabled="status === 'streaming'"
    @pick="(preset) => pickMockPreset(preset)"
/>
```

- 仅在 `isMockMode && mockPresets.length > 0` 时渲染
- header 显示 🧪 + scenario 名 + phase 阶段（调试可见）+ 「点击直接发送」hint
- chip 列表水平滚动（`overflow-x: auto`），每个 chip 是 `<button>` 含 icon + label
- 流式进行中时 chip 禁用（`pointer-events: none`），防止重复触发
- 暗黑模式：半透明 primary tint 背景，与 ion-content 背景融合

#### Scenario: 点击 chip → send

- **WHEN** 用户点击 chip
- **THEN** emit `pick` 事件，父组件 `AgentChat` 调 `useAgent().pickMockPreset(preset)`
- **AND** `pickMockPreset` 内部调 `send(preset.userText, { mode: 'start' })`
- **AND** 状态检查：`status === 'idle'` 时才发送（busy 时静默忽略）
- **AND** 调试日志：`[useAgent] pickMockPreset → <id> | userText = <text>`

### Requirement: i18n 文案（增量）

| key | zh-CN | en |
|-----|-------|---|
| `agent.mockPresetBarAria` | Mock 模式预设输入 | Mock mode preset inputs |
| `agent.mockPresetBarDefaultScenario` | 剧本 | Scenario |
| `agent.mockPresetBarHint` | 点击直接发送 | Click to send |

### Requirement: 关键文件 / 函数（增量）

- `internal/server/agent_mock.go` — `MockPreset` 类型 + `MockEngine.emitInitialPresets/endMockPresets` + `mock_presets` event case
- `internal/server/agent_mock_scenarios.go` — 12 个剧本全部加 `Presets` 字段
- `app/encv-mobile/src/composables/useAgent.ts` — `mock_presets` / `mock_presets_clear` 事件 + `mockPresets` ref + `pickMockPreset`
- `app/encv-mobile/src/components/agent/MockPresetBar.vue` — 新增 chip 列表组件
- `app/encv-mobile/src/views/AgentChat.vue` — 集成 MockPresetBar 覆盖在输入框上方
- `app/encv-mobile/src/i18n/agent.ts` — 3 个新 i18n key
- `internal/server/agent_mock_test.go` — 8 个新单元测试覆盖

### Requirement: 单元测试覆盖（增量）

- [x] `TestMockEngine_EmitsInitialPresetsOnStreamStart` — 验证初始 mock_presets 在 stream_start 之后推送
- [x] `TestMockEngine_NoPresetsWhenScenarioEmpty` — 验证无 Presets 字段时不推 mock_presets
- [x] `TestMockEngine_MidScenarioPresetUpdate` — 验证 mid-scenario 推 mock_presets 实现覆盖更新
- [x] `TestMockEngine_EmitsClearOnStreamEnd` — 验证 stream_end 触发 mock_presets_clear
- [x] `TestMockEngine_AllBuiltinScenariosHavePresets` — 验收：12 个剧本都至少有 1 个 Preset
- [x] `TestMockEngine_ExecuteReal_OverridesHardcoded` — 验证 execute_real=true 时调真实 handler
- [x] `TestMockEngine_ExecuteReal_FallbackWhenNoExecutor` — 验证 nil realExecutor 时 fallback 到硬编码
- [x] `TestMockEngine_ExecuteReal_ErrorPropagatesAsIsError` — 验证 error → isError=true
