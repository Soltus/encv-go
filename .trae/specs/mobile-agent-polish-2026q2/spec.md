# Mobile Agent 体验打磨 2026Q2 Spec

## Why

当前 mobile agent（[useAgent.ts](file:///workspace/app/encv-mobile/src/composables/useAgent.ts) 驱动）有四个独立但都属于「用户可感知的体验缺陷」的方向：

1. **工具调用异常没有传导到外层** —— 后端 `tool_result { isError: true }` 或工具抛错时，外层 `tool_call.status` 仍显示 `'success'`，**外层 success 误判**导致用户/调用链误以为工具成功执行
2. **缺少常用 bash 工具封装** —— 现有 `command_run`（[agent-tools-scenarios-v2](file:///workspace/.trae/specs/agent-tools-scenarios-v2) 设计）是底层 `os/exec`，**没有跨平台命令映射层**。`ls` 在安卓 WebView 后端是玩具 mock、`pwd` 在 windows 失效、`cat` 在 windows 不存在。需要 high-level 工具（`list_dir` / `show_file` / `tail_lines` / `find_by_name` 等）跨平台统一行为
3. **footer 时间显示是绝对时间** —— 任务列表 / 会话历史列表的 footer 用了相对时间（「刚刚 / 5 分钟前 / 1 天前」），但 AI 会话聊天 footer 还在显示 `2026-06-08 22:15:30`，**不一致**
4. **真机错误信息可缩放** —— 错误消息区域在安卓 webview 里**可以被双指缩放**，但安卓 webview 的标准交互是「双指缩放应该被业务接管、否则禁用」。当前错误块没有声明 `user-scalable=no` 也不接缩放事件 → 不符合 Material Design 规范。在 AI 会话里，缩放应该能**调整会话渲染比例**（50%-150%），让长文 / 代码更易读

新方案核心价值：
- **错误可见性** — 工具异常必须如实反映到外层 + UI 动态效果配合
- **跨平台工具抽象** — 用户无需关心后端是 bash / cmd / sh，每个工具语义稳定
- **时间一致性** — 所有时间显示统一为相对时间（基于 [sessionList 实现](file:///workspace/app/encv-mobile/src/composables/useChatEngine.ts)）
- **手势业务化** — 缩放不再失控，业务能稳定利用

---

## What Changes

### 新增

**后端 - 工具层（`internal/tools/`）**
- `internal/tools/errors.go`（新增）— 统一 `ToolError` 类型（`Code string` / `Message string` / `UnderlyingErr error`），所有工具 handler 返回 `(*ToolResult, *ToolError)`
- `internal/tools/high_level.go`（新增）— 高级跨平台 bash 封装工具集：
  - `list_dir` —— 包装 `ls`/`Get-ChildItem`/`ls -la`
  - `show_file` —— 包装 `cat`/`Get-Content`，支持 start_line / end_line / max_bytes
  - `tail_lines` —— 包装 `tail -n` / `Get-Content -Tail n`
  - `head_lines` —— 包装 `head -n` / `Get-Content -Head n`
  - `find_by_name` —— 包装 `find -name` / `Get-ChildItem -Recurse -Filter`
  - `find_by_content` —— 包装 `grep -r` / `Select-String`
  - `word_count` —— 包装 `wc` / `Measure-Object`
  - `disk_usage` —— 包装 `du -sh` / `Get-ChildItem | Measure Length`
  - `get_env` —— 包装 `env` / `Get-ChildItem Env:`
  - `which_cmd` —— 包装 `which` / `Get-Command`
- `internal/tools/platform_dispatch.go`（新增）— 平台检测 + 命令名映射表
  - `getPlatform() -> "linux" | "darwin" | "windows" | "android" | "ios"`
  - `commandMap map[string]map[string]string`（command_id → platform → real cmd name）
- `internal/tools/registry.go` 修改 — 工具定义加 `Kind string`（`"bash_like"` / `"native"`），bash_like 自动用 platform_dispatch 转换

**后端 - 工具异常传导**
- `internal/server/agent_tool_loop.go` 修改 — `executeTool` 返回值结构化：`{output: any, isError: bool, errorCode?: string, errorMessage?: string}` 推 `tool_result` 事件时带 `isError: true`
- `internal/server/agent_api.go` 修改 — `streamChat` 在工具抛错时也推 `tool_status { status: "error" }` 而非 `success`

**前端 - 工具异常 UI（`useAgent.ts` + 组件）**
- [useAgent.ts](file:///workspace/app/encv-mobile/src/composables/useAgent.ts) 修改 — `handleAgentEvent` 解析 `tool_result.isError = true` → 设置 `tool_call.status = 'error'`，设置 `tool_call.errorMessage`
- [useAgent.ts](file:///workspace/app/encv-mobile/src/composables/useAgent.ts) 新增 — 工具动态效果 ref `runningTools: Set<string>`（包含 `pending` / `running` / `success` / `error` 4 状态实时计数 + 转动 spinner）
- [components/agent/ToolDetailContent.vue](file:///workspace/app/encv-mobile/src/components/agent/ToolDetailContent.vue) 修改 — 工具卡片根据 status 显示不同视觉：
  - `pending` → 灰色占位 spinner
  - `running` → 蓝色 spinner + 进度条
  - `success` → 绿色对勾
  - `error` → 红色 ⚠️ + 错误详情可展开

**前端 - footer 相对时间**
- [useAgent.ts](file:///workspace/app/encv-mobile/src/composables/useAgent.ts) 修改 — 暴露 `formatRelativeTime(ts: number): string` 函数（与 sessionList 列表完全一致的逻辑：`< 60s` → "刚刚"，`< 1h` → "X 分钟前"，`< 24h` → "X 小时前"，`>= 24h` → "X 天前"）
- [composables/useChatEngine.ts](file:///workspace/app/encv-mobile/src/composables/useChatEngine.ts) 抽离 — 把相对时间格式化函数单独抽到 `composables/relativeTime.ts`，sessionList 和 footer 共享同一份实现
- [views/AgentChat.vue](file:///workspace/app/encv-mobile/src/views/AgentChat.vue) 修改 — footer 时间戳改用 `formatRelativeTime(lastMessageAt)` + 每 30s 自动重算
- [components/agent/AssistantMessage.vue](file:///workspace/app/encv-mobile/src/components/agent/AssistantMessage.vue) 修改 — 单条消息的 timestamp 也用 `formatRelativeTime`

**前端 - 安卓 webview 缩放控制**
- [views/AgentChat.vue](file:///workspace/app/encv-mobile/src/views/AgentChat.vue) 新增 — `usePinchZoom.ts` composable：
  - 监听 `.chat-message-content` 容器（**仅在 AI 会话区域**）的 `touchstart` / `touchmove` / `touchend`
  - 维护 `zoomScale: ref(1.0)` 范围 `[0.5, 1.5]`
  - 双指距离变化 → 计算 delta → 更新 `transform: scale(zoomScale)` 应用到该消息
  - 双击重置回 1.0
- [views/AgentChat.vue](file:///workspace/app/encv-mobile/src/views/AgentChat.vue) 修改 — 错误块加 `user-scalable=no` / `touch-action: pan-x pan-y` 阻止 webview 默认缩放
- 全局 `viewport` meta 修复 — `<meta name="viewport" content="user-scalable=no">` 仅在错误页生效，会话页不限制
- [views/AgentChat.vue](file:///workspace/app/encv-mobile/src/views/AgentChat.vue) 新增 — UI 控件：右上角「A- / A / A+」按钮（点击调 zoomScale 0.85 / 1.0 / 1.15）

### 修改

- [useAgent.ts](file:///workspace/app/encv-mobile/src/composables/useAgent.ts) — `handleAgentEvent` 解析 `tool_result` 事件时识别 `isError` 字段
- [App.vue](file:///workspace/app/encv-mobile/src/App.vue) — 错误状态 UI 加 `touch-action: manipulation` 阻止 webview 双击缩放
- [composables/useChatEngine.ts](file:///workspace/app/encv-mobile/src/composables/useChatEngine.ts) — 抽出 `formatRelativeTime` 到独立文件
- [i18n/agent.ts](file:///workspace/app/encv-mobile/src/i18n/agent.ts) — 新增 6+ 个 key

### 不影响

- 现有 tool registry（v1+v2 工具继续工作，bash_like 是新 kind）
- mock 剧本（继续发送 tool_call 事件，异常由 mock 剧本自身控制）
- AG-UI 协议层（异常字段通过现有 `tool_result.data` JSON 字段透传）
- 其他 4 个 tab（缩放 + footer 改动只在 AgentChat 范围）

---

## ADDED Requirements

### Requirement: ToolError 统一异常类型

`internal/tools/errors.go` SHALL 定义 `ToolError` 类型，所有工具 handler 在异常时返回此类型而不是裸 `error`。

#### Scenario: ToolError 数据结构

```go
type ToolError struct {
    Code        string // 错误代码（如 "ENOENT" / "PERMISSION_DENIED" / "TIMEOUT" / "INVALID_ARGS"）
    Message     string // 给用户看的本地化消息
    Underlying  error  // 原始错误（用于日志）
    Recoverable bool   // 是否可恢复（如重试）
}

func (e *ToolError) Error() string { return e.Message }
func (e *ToolError) IsError() bool { return e != nil }
```

#### Scenario: handler 返回值签名变更

**BEFORE**:
```go
type ToolHandler func(ctx context.Context, args json.RawMessage, deps *ToolDeps) (ToolResult, error)
```

**AFTER**:
```go
type ToolHandler func(ctx context.Context, args json.RawMessage, deps *ToolDeps) (ToolResult, *ToolError)
```

- handler 成功 → 返回 `(result, nil)`
- handler 失败 → 返回 `(zero, &ToolError{Code: "ENOENT", Message: "file not found: ..."})`
- 后端 `executeTool` 把 `*ToolError` 转换为 `tool_result` 事件的 `data.isError = true` + `data.errorCode` + `data.errorMessage`

---

### Requirement: tool_result 事件带 isError 字段

后端推 `tool_result` 事件时 SHALL 包含 `isError` 字段标识是否成功。

#### Scenario: 成功执行

```json
{
  "type": "tool_result",
  "data": {
    "toolCallId": "call_xxx",
    "isError": false,
    "output": { "files": [...] }
  }
}
```

#### Scenario: 工具失败

```json
{
  "type": "tool_result",
  "data": {
    "toolCallId": "call_xxx",
    "isError": true,
    "errorCode": "ENOENT",
    "errorMessage": "file not found: /mnt/abc.mp4",
    "output": null
  }
}
```

#### Scenario: tool_status 同步

- 工具开始 → `tool_status { status: "running" }`
- 工具成功 → `tool_status { status: "success" }` + `tool_result { isError: false, ... }`
- 工具失败 → `tool_status { status: "error" }` + `tool_result { isError: true, ... }`

外层 `tool_call.status` 永远反映真实状态，**不允许在错误时回退到 `success`**。

---

### Requirement: 前端 tool_call 状态机

[useAgent.ts](file:///workspace/app/encv-mobile/src/composables/useAgent.ts) SHALL 在 `handleAgentEvent` 中实现完整 tool_call 状态机。

#### Scenario: ToolCall 类型扩展

```typescript
type ToolStatus = 'pending' | 'running' | 'success' | 'error' | 'cancelled'

interface ToolCall {
  id: string
  name: string
  args: Record<string, any>
  status: ToolStatus           // ← 严格状态机
  errorCode?: string          // ← 新增
  errorMessage?: string       // ← 新增
  output?: any                // ← 新增（成功时的输出）
  startedAt?: number          // ← 新增（运行开始时间）
  finishedAt?: number         // ← 新增（运行结束时间）
}
```

#### Scenario: 状态转换规则

| 当前状态 | 收到事件 | 新状态 | 备注 |
|---------|---------|--------|------|
| `pending` | `tool_status {running}` | `running` | 设置 `startedAt` |
| `running` | `tool_result {isError: false}` | `success` | 设置 `output` + `finishedAt` |
| `running` | `tool_result {isError: true}` | `error` | 设置 `errorCode` + `errorMessage` + `finishedAt` |
| `running` | 用户取消 | `cancelled` | 设置 `finishedAt` |
| `pending` | 30s 无响应 | `error` | `errorCode: "TIMEOUT"` |
| **任何** | 错误时**不可**回到 `success` | - | 状态机严格单向 |

#### Scenario: 动态效果 ref

```typescript
const runningTools = computed(() => 
  messages.value.flatMap(m => m.tool_calls.filter(tc => tc.status === 'running'))
)

const hasRunningTool = computed(() => runningTools.value.length > 0)
```

- `runningTools.length > 0` → footer 显示「🔄 工具执行中…」
- 全部 success → footer 显示「✅ 完成」
- 有 error → footer 显示「❌ 工具执行失败」+ 红色边框

---

### Requirement: ToolDetailContent 工具卡片视觉

[ToolDetailContent.vue](file:///workspace/app/encv-mobile/src/components/agent/ToolDetailContent.vue) SHALL 根据 tool_call.status 渲染不同视觉。

#### Scenario: 四种状态视觉

| status | 颜色 | 动画 | 内容 |
|--------|------|------|------|
| `pending` | 灰色 (#888) | 静态占位 | "等待执行..." |
| `running` | 蓝色 (primary) | spinner 旋转 360° / 1s | "执行中..." + 可选进度条 |
| `success` | 绿色 (#22c55e) | ✓ 出现 200ms scale 动画 | 输出 JSON / 文本 |
| `error` | 红色 (#ef4444) | ⚠️ 抖动 200ms | 错误码 + 错误消息 + 折叠堆栈 |
| `cancelled` | 黄色 (#f59e0b) | 静态 | "已取消" |

#### Scenario: 错误状态交互

- 错误卡片右上角：复制错误按钮
- 点击错误卡片 → 展开完整堆栈（如果有）
- 长按错误卡片 → 显示建议操作（重试 / 跳过 / 报告）

---

### Requirement: 跨平台 bash 工具抽象

后端 SHALL 提供 high-level 跨平台 bash 工具集，自动按运行平台分派命令名。

#### Scenario: PlatformCommandMap 数据结构

```go
// internal/tools/platform_dispatch.go
type PlatformCommandMap map[string]map[string]string

// key1: command_id; key2: platform; value: real command name
var DefaultCommandMap = PlatformCommandMap{
    "list_dir": {
        "linux":   "ls",
        "darwin":  "ls",
        "android": "ls",
        "windows": "cmd",
    },
    "show_file": {
        "linux":   "cat",
        "darwin":  "cat",
        "android": "cat",
        "windows": "powershell",
    },
    "tail_lines": {
        "linux":   "tail",
        "darwin":  "tail",
        "android": "tail",
        "windows": "powershell",
    },
    // ...
}
```

#### Scenario: 高层工具 → 真实命令转换

| 工具名 (LLM-facing) | Linux/Darwin | Windows | 安卓 (后端) |
|---------------------|--------------|---------|------------|
| `list_dir` | `ls -la {path}` | `powershell Get-ChildItem {path} \| Format-List` | `ls -la {path}` |
| `show_file` | `cat {path}` | `powershell Get-Content {path} -TotalCount {max_lines}` | `cat {path}` |
| `tail_lines` | `tail -n {n} {path}` | `powershell Get-Content {path} -Tail {n}` | `tail -n {n} {path}` |
| `head_lines` | `head -n {n} {path}` | `powershell Get-Content {path} -Head {n}` | `head -n {n} {path}` |
| `find_by_name` | `find {root} -name "{pattern}"` | `powershell Get-ChildItem {root} -Recurse -Filter {pattern}` | `find {root} -name "{pattern}"` |
| `find_by_content` | `grep -rn "{regex}" {root}` | `powershell Select-String -Path {root} -Pattern "{regex}"` | `grep -rn "{regex}" {root}` |
| `word_count` | `wc -l {path}` | `powershell Get-Content {path} \| Measure-Object -Line` | `wc -l {path}` |
| `disk_usage` | `du -sh {path}` | `powershell Get-ChildItem {path} -Recurse \| Measure-Object -Property Length -Sum` | `du -sh {path}` |
| `get_env` | `env` | `powershell Get-ChildItem Env:` | `env` |
| `which_cmd` | `which {cmd}` | `powershell Get-Command {cmd}` | `which {cmd}` |

#### Scenario: 工具 Handler 通用包装器

```go
// internal/tools/high_level.go
type HighLevelTool struct {
    ToolID       string  // LLM-facing name
    ArgsParser   func(json.RawMessage) (BashArgs, error)
    RealCmdMap   PlatformCommandMap
    BuildShellCmd func(platform string, args BashArgs) (cmd string, args []string)
    ParseOutput  func(platform string, stdout string) (parsed any, err *ToolError)
}

func (h *HighLevelTool) Execute(ctx context.Context, args json.RawMessage, deps *ToolDeps) (ToolResult, *ToolError) {
    platform := deps.Platform
    parsed, err := h.ArgsParser(args)
    if err != nil { return zero, &ToolError{Code: "INVALID_ARGS", ...} }
    
    cmdName, cmdArgs := h.BuildShellCmd(platform, parsed)
    if !isWhitelisted(cmdName) { return zero, &ToolError{Code: "DENIED", ...} }
    
    stdout, stderr, err := runCommand(ctx, cmdName, cmdArgs, 5*time.Second)
    if err != nil { return zero, &ToolError{Code: "EXEC_FAILED", ...} }
    
    parsed, perr := h.ParseOutput(platform, stdout)
    if perr != nil { return zero, perr }
    return ToolResult{Output: parsed}, nil
}
```

#### Scenario: 沙箱 + 白名单

- 所有 high_level 工具都经过 `tool_whitelist` 检查
- 真实命令名（如 `ls`、`powershell`）必须在白名单中
- 默认白名单追加：`powershell`（Windows 后端使用）

---

### Requirement: Platform detection 平台检测

后端 SHALL 检测运行平台（用于命令分派）。

#### Scenario: 平台检测函数

```go
// internal/tools/platform_dispatch.go
func DetectPlatform() string {
    switch runtime.GOOS {
    case "linux":
        // 进一步检测：是否有 /system/bin/sh → Android
        if _, err := os.Stat("/system/bin/sh"); err == nil {
            // 检查 /system/build.prop 是否存在
            if _, err := os.Stat("/system/build.prop"); err == nil {
                return "android"
            }
        }
        return "linux"
    case "darwin":
        return "darwin"
    case "windows":
        return "windows"
    default:
        return runtime.GOOS
    }
}
```

- 平台信息注入到 `ToolDeps.Platform`
- handler 通过 `deps.Platform` 决定走哪条命令

---

### Requirement: 相对时间格式化函数

`composables/relativeTime.ts` SHALL 提供统一的相对时间格式化函数。

#### Scenario: 格式规则

| 时间差 | 输出 |
|--------|------|
| `< 60s` | "刚刚" / "just now" |
| `< 60min` | "X 分钟前" / "X minutes ago" |
| `< 24h` | "X 小时前" / "X hours ago" |
| `< 7d` | "X 天前" / "X days ago" |
| `>= 7d` | "YYYY-MM-DD"（绝对日期 fallback）|

#### Scenario: 函数签名

```typescript
// composables/relativeTime.ts
export function formatRelativeTime(ts: number, now: number = Date.now()): string
export function useRelativeTime(): {
  format: (ts: number) => string
  /** 强制刷新所有调用了 format 的位置 */
  refreshTick: Ref<number>
}
```

#### Scenario: 自动刷新

`useRelativeTime` 内部 setInterval(30s) 增加 `refreshTick.value`，所有 computed 依赖它的位置自动重算（无需手动调用）。

- AgentChat footer 时间每 30s 自动更新
- sessionList 列表项时间每 30s 自动更新
- 单条消息 timestamp hover 仍显示绝对时间（tooltip）

#### Scenario: 现有代码迁移

- `useChatEngine.ts` 中的相对时间逻辑 → 替换为 `useRelativeTime().format`
- `AgentChat.vue` footer 时间显示 → 替换为 `useRelativeTime().format(lastMessageAt)`
- `AssistantMessage.vue` 单条消息时间 → 替换为 `useRelativeTime().format(msg.timestamp)`

---

### Requirement: 安卓 webview 缩放控制

[AgentChat.vue](file:///workspace/app/encv-mobile/src/views/AgentChat.vue) SHALL 在 AI 会话区域实现双指缩放（50%-150%），其他视图/外层元素不受影响。

#### Scenario: 缩放范围

- 最小：`0.5`（50%）
- 默认：`1.0`（100%）
- 最大：`1.5`（150%）
- 步进：`0.05`（5%）
- 双击重置：1.0

#### Scenario: usePinchZoom composable

```typescript
// composables/usePinchZoom.ts
export function usePinchZoom(targetRef: Ref<HTMLElement | null>) {
  const zoomScale = ref(1.0)
  const minScale = 0.5
  const maxScale = 1.5

  let initialDistance = 0
  let initialScale = 1.0

  function onTouchStart(e: TouchEvent) {
    if (e.touches.length !== 2) return
    initialDistance = touchDistance(e.touches)
    initialScale = zoomScale.value
  }

  function onTouchMove(e: TouchEvent) {
    if (e.touches.length !== 2) return
    e.preventDefault()  // ← 阻止 webview 默认缩放
    const currentDistance = touchDistance(e.touches)
    const ratio = currentDistance / initialDistance
    const newScale = clamp(initialScale * ratio, minScale, maxScale)
    zoomScale.value = newScale
    applyZoom()
  }

  function applyZoom() {
    if (!targetRef.value) return
    targetRef.value.style.transform = `scale(${zoomScale.value})`
    targetRef.value.style.transformOrigin = 'top left'
  }

  function resetZoom() { zoomScale.value = 1.0; applyZoom() }
  
  return { zoomScale, resetZoom, applyZoom }
}
```

#### Scenario: 仅作用于 AI 会话区域

- `targetRef` 绑定 `.chat-message-content` 容器（不是整个 ion-content）
- 其他 tab（Files / Tasks / Settings）不受影响
- 错误状态页（App.vue 错误块）单独处理：**禁止缩放**（见下条）

#### Scenario: UI 控件

右上角浮动按钮组：
- 「A-」按钮 → `zoomScale = max(zoomScale - 0.1, 0.5)`
- 「A」按钮 → `zoomScale = 1.0`（重置）
- 「A+」按钮 → `zoomScale = min(zoomScale + 0.1, 1.5)`

按钮固定在 ion-content 右上角，浮于内容之上。

---

### Requirement: 错误状态页禁止缩放

[App.vue](file:///workspace/app/encv-mobile/src/App.vue) 错误状态块 SHALL 显式禁止 webview 默认缩放。

#### Scenario: 错误块 touch-action

```css
.error-state {
  touch-action: manipulation;  /* 禁止双击缩放 */
  user-select: text;           /* 允许选择错误信息文本 */
  -webkit-user-select: text;
}
```

#### Scenario: viewport meta 按场景切换

- 错误状态页：`<meta name="viewport" content="width=device-width, initial-scale=1.0, maximum-scale=1.0, user-scalable=no">`
- 正常会话页：`<meta name="viewport" content="width=device-width, initial-scale=1.0, user-scalable=yes">`（让 usePinchZoom 接管）

切换通过运行时改 `<meta>` 标签的 content 属性实现（无需刷页）。

---

### Requirement: 工具动态效果

工具调用期间 SHALL 提供清晰的视觉反馈。

#### Scenario: 工具卡片执行中动画

- `running` 状态：左侧蓝色 spinner 旋转（`@keyframes spin { to { transform: rotate(360deg) } }` 1s 无限循环）
- spinner 颜色：跟随 `var(--ion-color-primary)` 主题色
- 卡片背景：`rgba(var(--ion-color-primary-rgb), 0.05)` 浅色高亮
- 卡片右上角显示「执行中...」文字 + 动态省略号（`...` → `..` → `.` 循环）

#### Scenario: 工具执行时长显示

- 卡片底部显示「耗时 1.2s」/「耗时 3.5s」（基于 `startedAt` / `finishedAt`）
- 超过 5s 自动显示「耗时较长」红色提示

---

## MODIFIED Requirements

### Requirement: ToolHandler 返回值签名

**BEFORE**: `func(ctx, args, deps) (ToolResult, error)`
**AFTER**: `func(ctx, args, deps) (ToolResult, *ToolError)`

- 现有 v1+v2 工具 handler 迁移到新签名
- handler 内部错误用 `&ToolError{Code: "...", Message: "..."}` 包装
- 真实 LLM 路径在工具异常时**不再**继续推后续 step，直接 stream_end 报失败

### Requirement: useAgent.handleAgentEvent tool_result 处理

**BEFORE**: 收到 `tool_result` 事件只更新 `tool_call.output`，不更新 status
**AFTER**: 收到 `tool_result` 事件同时检查 `isError` 字段：
- `isError = false` → `tool_call.status = 'success'`
- `isError = true` → `tool_call.status = 'error'`，并填充 `errorCode` / `errorMessage`

### Requirement: sessionList 时间显示

**BEFORE**: `useChatEngine` 内嵌相对时间逻辑（重复实现）
**AFTER**: 抽出到 `composables/relativeTime.ts`，sessionList 与 footer 共享

### Requirement: AgentChat footer 时间显示

**BEFORE**: 显示绝对时间戳 `2026-06-08 22:15:30`
**AFTER**: 显示相对时间 `5 分钟前`，每 30s 自动重算

---

## REMOVED Requirements

无（仅修改 + 新增，不删除任何现有能力）

---

## 约束与限制

1. **tool_call 状态机严格单向** — `error` 状态永远不能转为 `success`（一旦失败即失败）
2. **跨平台命令必须有 fallback** — 如果目标平台没有对应命令，工具返回 `ToolError{Code: "UNSUPPORTED_PLATFORM"}`
3. **缩放范围严格 50%-150%** — 超出范围 clamp 住，不抛错
4. **相对时间每 30s 刷新** — 用户停留页面期间时间显示自动更新
5. **错误状态永远禁止缩放** — 这是安卓 webview 规范要求
6. **AG-UI 协议透传异常** — `isError` / `errorCode` / `errorMessage` 通过现有 `tool_result.data` JSON 透传，不引入新的 AG-UI 事件类型
7. **i18n 同步** — 新增 6+ 个 key 必须 zh-CN + en 双语
8. **不动 mock 剧本** — 现有 12 + 8 个 mock 剧本继续工作，新功能不破坏向后兼容

---

## 与现有 spec 的关系

| 现有 spec | 关系 |
|----------|------|
| `agent-tools-scenarios-v2` | **基础** — 本 spec 在其之上叠加跨平台 bash 抽象 + ToolError 异常类型 |
| `agui-real-llm-path-completion` | **受益** — 异常通过现有 `tool_result.data` 透传 |
| `agent-mock-mode` | 无关（mock 剧本自己控制异常） |
| `multi-engine-chat-architecture` | 无关（引擎层不受影响） |
| `go-in-process-agent` | **修改点** — `executeAgentTool` 改用新 ToolError 返回签名 |

---

## 验证步骤

### 后端验证

1. **类型检查** — `go build ./cmd/encv` 0 错误
2. **ToolError 单测** — `go test ./internal/tools/... -run TestToolError -v` 5+ 用例通过
3. **跨平台命令单测** — `go test ./internal/tools/... -run TestPlatformDispatch -v` 10+ 用例（每平台 2-3 个）
4. **high_level 工具单测** — `go test ./internal/tools/... -run TestHighLevel -v` 20+ 用例（每工具 2+）
5. **异常传导集成测试** — `go test ./internal/server/... -run TestToolErrorPropagation -v` 4+ 用例

### 前端验证

6. **类型检查** — `npx vue-tsc --noEmit` 0 错误
7. **构建** — `npx vite build` 0 错误
8. **useAgent 状态机单测** — `npm test -- useAgent` 全部通过
9. **relativeTime 单测** — `npm test -- relativeTime` 全部通过
10. **usePinchZoom 单测** — `npm test -- usePinchZoom` 全部通过
11. **ToolDetailContent 视觉** — 截屏验证 4 种状态视觉

### 端到端验证

12. **异常传导** — 触发 `search_files` mount 不存在 → 验证 tool_call 变 `error` 状态 + UI 显示红色 ⚠️
13. **bash 跨平台** — 后端起在 macOS / Windows 容器（Docker）→ 调 `list_dir` / `tail_lines` 验证输出格式正确
14. **相对时间** — session 停留 5 分钟 → footer 时间从「刚刚」→「1 分钟前」→「5 分钟前」自动更新
15. **缩放控制** — 安卓真机打开会话 → 双指缩放 → 验证会话内容缩放（不缩放外层）→ 错误页打开 → 双指缩放无效果
16. **AG-UI 透传** — mock 工具返回 `isError: true` → 前端 tool_call 状态 `error` + UI 红色

---

## 关键文件 / 函数

| 文件 | 关键类型/函数 |
|------|--------------|
| `internal/tools/errors.go` | `ToolError` / `IsError` |
| `internal/tools/platform_dispatch.go` | `DetectPlatform` / `PlatformCommandMap` / `DefaultCommandMap` |
| `internal/tools/high_level.go` | `HighLevelTool` / `Execute` / 10 个高阶工具实现 |
| `internal/server/agent_tool_loop.go` | `executeTool` 改用新 ToolError 返回 |
| `internal/server/agent_api.go` | `streamChat` 工具异常分支处理 |
| `app/encv-mobile/src/composables/useAgent.ts` | `handleAgentEvent` tool_result isError 处理 / `runningTools` computed |
| `app/encv-mobile/src/composables/relativeTime.ts`（新）| `formatRelativeTime` / `useRelativeTime` |
| `app/encv-mobile/src/composables/usePinchZoom.ts`（新）| `zoomScale` / `onTouchStart` / `onTouchMove` / `resetZoom` |
| `app/encv-mobile/src/components/agent/ToolDetailContent.vue` | 4 种状态视觉 / 错误展开 |
| `app/encv-mobile/src/views/AgentChat.vue` | 集成 usePinchZoom / useRelativeTime / 缩放 UI 控件 |
| `app/encv-mobile/src/App.vue` | 错误状态块 `touch-action: manipulation` |
| `app/encv-mobile/src/composables/useChatEngine.ts` | 抽出 relativeTime 逻辑 |
| `app/encv-mobile/src/i18n/agent.ts` | 6+ 个新 i18n key |
