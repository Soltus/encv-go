# 工具调用流程诊断报告

> 审查范围：`D:\dev\encv-go\agent`
> 审查日期：2026-06-07
> 审查目标：追踪 LLM 工具调用从注册 → 发送 → 执行 → 结果返回的完整生命周期，找出所有 bug、缺陷和不一致

---

## 一、工具调用架构概览

```
┌────────────────────────────────────────────────────┐
│                    Agent                            │
│  ┌──────────────┐  ┌──────────────────────────┐    │
│  │  ToolRegistry │  │      runLoop (goroutine)  │   │
│  │              │  │                          │    │
│  │  Register()  │  │  for turn := 0;; turn++  │    │
│  │  Get()       │  │                          │    │
│  │  GetAll()    │  │  messages → streamOneTurn│    │
│  └──────────────┘  │       ↓                  │    │
│         ↑          │  return (shouldContinue) │    │
│  ┌──────┴───────┐  │       ↓                  │    │
│  │  Tool  定义   │  │  next iteration         │    │
│  │  Schema      │  └──────────────────────────┘    │
│  │  Handler     │                                  │
│  └──────────────┘                                   │
└────────────────────────────────────────────────────┘
```

---

## 二、Critical（严重问题）— 必须立即修复

### C1. `streamOneTurn` 的 messages 追加不回传 `runLoop` — auto-run 循环完全失效

**严重性**: 🔴 Critical | **文件**: `agent.go:776` + `agent.go:797-1088`

#### 问题描述

`runLoop` 将 `messages` 按值传给 `streamOneTurn`。Go 中 slice 传参会复制 header（ptr + len + cap），因此 `streamOneTurn` 内部所有的 `messages = append(messages, ...)` 只更新了**局部副本的 slice header**，对 `runLoop` 的 `messages` 变量**完全不可见**。

```go
// agent.go:751-784  runLoop
func (a *Agent) runLoop(..., messages []openai.ChatCompletionMessage, ...) {
    for turn := 0; ; turn++ {
        shouldContinue, err := a.streamOneTurn(ctx, sessionID, messages, out)
        //    ↑ runLoop 的 messages 从未被更新!
        //      下一次循环传入的还是旧的 messages
        if !shouldContinue { return }
    }
}
```

#### 影响路径

`streamOneTurn` 在第 914、1017-1021、1068-1079 行追加了 assistant 消息和 tool result：

```go
// agent.go:914
messages = append(messages, assistant)  // ← 仅局部可见

// agent.go:1017-1021
messages = append(messages, openai.ChatCompletionMessage{
    Role: openai.ChatMessageRoleTool, ...  // ← 仅局部可见
})
```

当 `streamOneTurn` 返回 `shouldContinue=true`（表示有 auto-run 工具需要继续），`runLoop` 进入下一轮时，**传入的仍然是 append 之前的旧 `messages`**。这意味着：

1. **第 1 轮**：LLM 返回 tool call → 执行 → 结果追加到局部 `messages`
2. **第 2 轮**：`runLoop` 传入旧 `messages`（没有 tool result）→ LLM 看不到结果
3. **后续轮**：同样丢失

→ **所有 auto-run multi-turn 场景完全失效**

#### 验证场景

```go
// 用户请求："先搜索文件，再读取内容"
// 期望流程：
//   turn 1: search_files → result → messages 追加 result
//   turn 2: LLM 看到 result → 决定 read_file → ...
// 
// 实际流程：
//   turn 1: search_files → result → 仅局部可见
//   turn 2: LLM 看到旧 messages → 不知道结果 → 乱回答
```

---

### C2. `emitData` 注释声称非阻塞但实际阻塞

**严重性**: 🔴 Critical | **文件**: `agent.go:1475-1490`

#### 问题描述

```go
func (a *Agent) emitData(sessionID string, out chan<- *Event, t EventType, data string) {
    ev := &Event{Type: t, Data: data}
    a.cacheAndPersist(sessionID, ev)
    // 注释: "We use a non-blocking send" ← 谎话
    select {
    case out <- ev:
    default:
        // 注释: "Drop on the floor with a buffered channel"
        //       ← 根本没有 drop！
        out <- ev  // ← 这是阻塞发送！
    }
}
```

#### 分析

- `default` 分支的 `out <- ev` 是**阻塞的**。如果 channel 缓冲区满且消费者（SSE writer）慢，goroutine 会卡住
- 注释写的是 "non-blocking send"，但代码写的是阻塞的
- 注释写 "Drop on the floor"，但代码没有 drop
- 效果等同于直接的 `out <- ev`，`select` 徒增复杂度

#### 影响

整个 `runLoop` goroutine 被阻塞 → 所有工具调用卡住 → 前端 SSE 连接可能超时断开

---

### C3. `resumeAfterDecision` 双重 close channel

**严重性**: 🔴 Critical | **文件**: `agent.go:1263` + `agent.go:1341`

#### 问题描述

```go
func (a *Agent) resumeAfterDecision(...) {
    defer a.finishAndClose(sessionID, out)       // 第 1263 行: 第一个 defer

    switch decision {
    case DecisionAccept:
        // ... 执行工具、发事件
    case DecisionAcceptForSession:
        // ... 同上
    case DecisionDecline:
        // ... 发拒绝事件
    case DecisionCancel:
        return  // ← 正确: 只走 defer，不调 runLoop
    }

    a.runLoop(ctx, sessionID, messages, out, false)  // 第 1341 行
    //             ↑ runLoop 内部另有 defer a.finishAndClose  ← 双重!
}
```

对于 `Accept` / `AcceptForSession` / `Decline`：
1. `defer a.finishAndClose()` → 执行 `finishSession` + `close(out)`
2. `a.runLoop(...)` → `defer a.finishAndClose()` → 再次执行 `finishSession` + `close(out)`

`close(out)` 两次 → panic → 被 `recover()` 吞掉 → 不崩但不该如此

#### 注释自己也意识到了

第 1251-1255 行：
> "the path that delegates MUST return without re-closing"

但代码里第 1263 行的 `defer` 无论如何都会执行，所以"must return without re-closing"的条件根本无法满足。

---

### C4. `runTool` 计算了 `DurationMs` 但丢弃，前端永远看到 0

**严重性**: 🔴 Critical | **文件**: `agent.go:1093-1102` + 所有 emit 处

#### 问题描述

```go
func (a *Agent) runTool(def ToolDefinition, args string) (string, string, error) {
    t0 := time.Now()
    result, err := def.Handler(args)
    dur := time.Since(t0).Milliseconds()
    _ = dur              // ← 算好了，扔掉了！
    ...
}
```

所有 emit `ToolResultData` 的位置都写死 `DurationMs: 0`：

| 位置 | 行号 |
|------|------|
| auto-run 路径（`streamOneTurn`） | 1047, 1062 |
| `resumeAfterDecision` Accept | 1276-1281 |
| `resumeAfterDecision` AcceptForSession | 1291-1296 |

#### 影响

前端 ApprovalCard / DevLogs 中工具执行耗时永远显示 0ms，用户体验受损。

---

### C5. `resumeAfterDecision` 中 `IsError` 字段缺失

**严重性**: 🔴 Critical | **文件**: `agent.go:1276-1281` + `agent.go:1291-1296`

#### 问题描述

对比两段代码：

**auto-run 路径（正确）**：
```go
a.emitData(sessionID, out, EventToolResult, mustJSON(ToolResultData{
    ID:         ptc.ID,
    Name:       ptc.Name,
    Result:     resultStr,
    IsError:    runErr != nil,   // ← 正确设置了
    Status:     status,
    DurationMs: 0,
}))
```

**`resumeAfterDecision` 的 Accept / AcceptForSession（错误）**：
```go
a.emitData(sessionID, out, EventToolResult, mustJSON(ToolResultData{
    ID:     toolCallID,
    Name:   toolName,
    Result: resultStr,
    Status: status,
    // IsError: 未设置 → 默认 false   ← 即使工具执行失败!
}))
```

此外，第 1275 行 `resultStr, status, _ := a.runTool(...)` **丢弃了 error**，所以即使工具执行失败，也无法区分 `IsError`。

#### 影响

前端判断工具执行是否成功时，`IsError=false` 但 `Status="failed"`，两者矛盾。

---

### C6. Compaction 后 slice 截断局部生效 — 叠加 C1 恶果

**严重性**: 🔴 Critical | **文件**: `agent.go:832-844`

#### 问题描述

```go
// agent.go:844 在 streamOneTurn 内部
messages = messages[:1+keep]  // ← 仅局部有效!
```

`streamOneTurn` 是值接收 slice 的。截断操作只影响局部 header，和 C1 是同一个根因。

更微妙的是，compaction 后 `toolCallsByIndex` 仍然指向被 compact 的 `messages` 索引位置。如果 compaction 发生在第一轮，第二轮 `runLoop` 传入旧 `messages`（compact 前的），tool call 的索引和内容完全错位。

---

## 三、Major（重要问题）— 应尽快修复

### M1. `enqueueAndReturnClosed` 的竞态条件

**严重性**: 🟠 Major | **文件**: `agent.go:660-675` + `agent.go:695-704`

#### 问题描述

```go
// enqueueAndReturnClosed（Goroutine A: ChatMode - queue）
a.pendingMu.Lock()
q, ok := a.pendingMessages[sessionID]
if !ok {
    q = &pendingMessageQueue{}    // (1) 创建新队列
    a.pendingMessages[sessionID] = q
}
a.pendingMu.Unlock()
q.enqueue(messages)               // (4) 入队

// pendingQueueDrainHook（Goroutine B: HookTurnEnd）
a.pendingMu.Lock()
q, ok := a.pendingMessages[hc.SessionID]  // (2) 发现队列存在
a.pendingMu.Unlock()
msgs, ok := q.dequeue()                    // (3) 无消息 → 退出
```

如果 Goroutine B 在 (3) 处 dequeue 为空返回 `nil, false` 并退出，而 Goroutine A 在 (4) 才入队，这条消息就**永远不会被处理**。

#### 根因

锁粒度过细：检查存在性和 dequeue 之间没有保护；创建队列和入队之间没有原子性保证。需要用条件变量或更粗的锁范围。

---

### M2. `getSession` / `ensureSession` 不安全类型断言

**严重性**: 🟠 Major | **文件**: `agent.go:356-361` + `agent.go:365-371` + `agent.go:586` + `agent.go:1503-1504`

#### 问题描述

多处使用无 `ok` 模式的类型断言，`sync.Map` 如果意外存了非预期类型会 panic：

```go
func (a *Agent) ensureSession(sessionID string) *SessionCache {
    v, ok := a.Sessions.Load(sessionID)
    if !ok {
        c := &SessionCache{}
        actual, _ := a.Sessions.LoadOrStore(sessionID, c)
        return actual.(*SessionCache)  // ← 如果 LoadOrStore 返回非 *SessionCache 类型 → panic
    }
    return v.(*SessionCache)           // ← 同理
}

func (a *Agent) getSession(sessionID string) (*SessionCache, bool) {
    v, ok := a.Sessions.Load(sessionID)
    if !ok { return nil, false }
    return v.(*SessionCache), true     // ← 同理
}
```

同类问题还出现在 `permissionModeFor`（第 586 行 `v.(string)`）和 `finishSession`（第 1504 行 `v.(*SessionCache)`）。

---

### M3. `pendingQueueDrainHook` 使用 `context.Background()` 无法取消

**严重性**: 🟠 Major | **文件**: `agent.go:715-716`

#### 代码

```go
go func() {
    ch, err := a.Chat(context.Background(), hc.SessionID, msgs)
```

即使原始 session 已被关闭或用户已断开连接，新的 goroutine 也无法被取消。每次 `HookTurnEnd` 都可能启动一个不可取消的 Chat goroutine。

---

### M4. `httpContext()` 丢弃 cancel 函数

**严重性**: 🟠 Major | **文件**: `cmd/agent-demo/schemas.go:14-17`

#### 代码

```go
func httpContext() context.Context {
    ctx, _ := context.WithTimeout(context.Background(), 30*time.Second)
    return ctx
}
```

- `go vet` 会报告 `cancel` 函数未调用
- 虽然不是内存泄漏（30s 后 GC），但最佳实践违规

---

### M5. `resumeAfterDecision` 中 Accept 路径缺少 PendingCall 删除验证

**严重性**: 🟠 Major | **文件**: `agent.go:1274-1301`

#### 问题

`ConfirmTool` 中已删除 `PendingCalls[sessionID]`（第 1231 行），但 `resumeAfterDecision` 内部没有再次检查 session 的有效性。如果在 accept 和 complete 之间另一个 goroutine 修改了 session 状态，会导致数据竞争。

---

## 四、Minor（次要问题）

### m1. SessionGrants 用 `|` 分隔符可能冲突

**文件**: `agent.go:1167-1169`

```go
func grantKey(sessionID, toolName string) string {
    return sessionID + "|" + toolName  // 如果 sessionID 或 toolName 含 "|"
}
```

建议：`fmt.Sprintf("%s\x00%s", sessionID, toolName)`

### m2. `emitToolCall` 中 name fallback 逻辑不合理

**文件**: `agent.go:1139-1145`

```go
if name == "" && def.Kind != "" {
    name = "<unknown>"
}
```

用 `def.Kind != ""` 判断工具是否存在是间接且不可靠的。应该在调用处用 `Registry.Get` 的 `ok` 值来判断。

### m3. `cloneMessages` 是浅拷贝

**文件**: `agent.go:1547-1551`

```go
func cloneMessages(in []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
    out := make([]openai.ChatCompletionMessage, len(in))
    copy(out, in)
    return out
}
```

`ChatCompletionMessage.ToolCalls` 是 `[]ToolCall`（slice），浅拷贝后共享底层数组。并发环境下不安全。

### m4. `pluginFieldType("object")` 返回无 properties 的 object

**文件**: `plugin_scanner.go:299`

JSON Schema 中 `{"type": "object"}` 不会 `properties`，LLM 无法知道 object 结构。

### m5. `convertMessages` 不验证 role 值

**文件**: `http.go:333`

role 只检查了非空，不验证是否为已知值（user/assistant/system/tool），无效 role 会透传给 OpenAI。

### m6. `generateNewSessionID` 的 fallback 使用 `time.Now().UnixNano()` 可能冲突

**文件**: `agent.go:489-495`

当 `crypto/rand` 失败时（熵池耗尽），fallback 使用纳秒时间，高并发下可能重复。

### m7. `NewAgent` 未初始化 `pendingMessages`

**文件**: `agent.go:259-268`

`NewAgent` 没有初始化 `pendingMessages`，虽然目前没有注册 hook 不会访问，但未来扩展会 panic（nil map write）。

---

## 五、问题根因分类

| 根因类别 | 问题编号 | 说明 |
|----------|---------|------|
| **Go slice 传值语义** | C1, C6 | 局部修改不回传，核心循环状态不同步 |
| **代码与注释不一致** | C2 | 注释写非阻塞，代码写阻塞 |
| **defer + 委托调用冲突** | C3 | defer 和嵌套函数都试图 close 同一个 channel |
| **返回值未传递** | C4, C5 | 计算了耗时但不返回；error 被 _ 丢弃 |
| **锁粒度过细** | M1 | 检查存在性和操作之间不是原子的 |
| **缺少防御性代码** | M2, M3 | 类型断言无 ok 模式；context 无取消 |

---

## 六、修复优先级建议

### P0（立刻修 — 功能无法正常工作）

1. **C1 + C6**: 改 `streamOneTurn` 签名，回传 `messages`
2. **C2**: `emitData` 改为真正非阻塞 or 明确阻塞
3. **C3**: 移除 `resumeAfterDecision` 的 `defer`，改为手动在 Cancel 分支 close

### P1（尽快修 — 功能缺陷明显）

4. **C4**: `runTool` 返回 `DurationMs`，所有 emit 处使用真实值
5. **C5**: `resumeAfterDecision` 的 Accept/AcceptForSession 补 `IsError`

### P2（应该修 — 稳定性和安全性）

6. **M1**: `enqueueAndReturnClosed` 使用条件变量或更粗锁范围
7. **M2**: 所有类型断言使用 `ok` 模式
8. **M3**: 传递可取消的 context 给后台 goroutine
9. **M4**: 调用 `cancel()`

---

## 七、工具调用链路各环节问题清单

```
用户请求
  │
  ▼
HandleChat (http.go)
  │
  ▼
ChatMode (agent.go)
  │
  ▼
runLoop ───────────────────────────────── C1: messages 不回传
  │                                              C6: compaction 截断不回传
  │
  ▼
streamOneTurn
  │
  ├─ maybeCompact ───────────────────── C6: slice 截断仅局部
  ├─ LLM stream → parseDelta
  ├─ messages = append(messages) ────── C1: 仅局部
  │
  ├─ Registry.Get(name) ─────────────── 正常
  ├─ 计算 autoRun
  ├─ emitToolCall ───────────────────── m2: name fallback 逻辑错误
  │
  ├─ [非 autoRun] PendingCalls ──────── M5: 缺少重复确认保护
  │                → ConfirmTool
  │                  → resumeAfterDecision
  │                    ├─ C3: 双重 close
  │                    ├─ C4: DurationMs=0
  │                    └─ C5: IsError 缺失
  │
  ├─ [autoRun] HookPreToolCall
  │            └─ runTool ────────────── C4: DurationMs 丢弃
  │
  ├─ HookPostToolCall
  ├─ emit EventToolResult ───────────── C4: DurationMs=0
  └─ messages = append(tool result) ─── C1: 不回传
       │
       ▼
  return shouldContinue=true
       │
       ▼
下一轮 runLoop ─── 传入旧 messages ──── C1: LLM 看不到结果!
```

---

## 八、总结

**最核心的 bug 是 C1**：`streamOneTurn` 的 `messages` 不回传导致 Auto-run 多轮工具调用循环完全失效。这个问题的本质是 Go slice 传值语义的组合陷阱，加上没有用指针或返回值来同步状态。

**第二严重的是 C2**：`emitData` 的注释和代码不一致，看似非阻塞的 select 实际是阻塞的，可能在生产中导致 `runLoop` 卡死。

**第三是 C3**：`resumeAfterDecision` 的 defer + 嵌套 runLoop 双重 close，虽然被 recover 保护不崩，但 `finishSession` 发了两次 `stream_end`。

建议优先修 P0 的三个问题，它们直接影响核心业务逻辑的正确性。
