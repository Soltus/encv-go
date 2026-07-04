# 真机测试修复计划：回答不完整 + 工具调用未渲染为结构化组件

> **基于 LobeChat (35K★) / LangChain agent-chat-ui / Vercel AI SDK 三大开源项目源码对比分析**

---

## 一、问题现象

真机提问"有哪些视频文件"，AI 回复：
```
回答"有哪些视频文件"，必须先调用 list_mounts 查看挂载点，然后再查看目录内容。

[{"name":"list_mounts","arguments":{}}] studio_video_1762059800961.mp4 (约 554MB)

其他条目都是子目录...在 /Movies 目录下的可用视频文件有：
```

| # | 症状 | 严重度 |
|---|------|--------|
| 1 | 工具调用以裸 `[{"name":"list_mounts",...}]` JSON 文本显示，而非 GroupedOperationMessage 结构化卡片 | P0 |
| 2 | 回答在句子中间截断（"在 /Movies 目录下的可用视频文件有：" 之后无内容） | P0 |

---

## 二、三大开源项目源码分析

### 2.1 LobeChat — [gatewayEventHandler.ts](tmp/lobechat-analysis/src/store/chat/slices/aiChat/actions/gatewayEventHandler.ts)

**核心发现：协议级内容分离**

LobeChat 的 SSE 事件协议将 text 和 tool_calls 作为 **完全独立的事件类型**：

```
stream_start → 重置 accumulatedContent = ''
stream_chunk:
  ├─ chunkType='text'      → accumulatedContent += content → dispatchMessage({content})
  ├─ chunkType='reasoning' → accumulatedReasoning += reasoning → dispatchMessage({reasoning})
  └─ chunkType='tools_calling' → dispatchMessage({tools: data.toolsCalling}) ← 独立通道！
tool_start / tool_execute / tool_end → 工具生命周期
stream_end → 清除 tool calling 动画
step_complete / agent_runtime_end → 最终状态
```

**关键代码**（[gatewayEventHandler.ts:348-409](tmp/lobechat-analysis/src/store/chat/slices/aiChat/actions/gatewayEventHandler.ts#L348-L409)）：
```typescript
case 'stream_chunk':
  if (data.chunkType === 'text') {
    accumulatedContent += data.content;     // 文本累积到 content 字段
    dispatchMessage({ content: accumulatedContent });
  }
  if (data.chunkType === 'tools_calling') {
    dispatchMessage({ tools: data.toolsCalling }); // 工具调用走独立字段！
    toggleToolCallingStreaming(currentAssistantMessageId, ...);
  }
```

**渲染层**（[ai.tsx:161-181](tmp/lcui-analysis/src/components/thread/messages/ai.tsx#L161-L181)）：
```tsx
{/* 文本和工具调用完全分开渲染 */}
{contentString.length > 0 && <MarkdownText>{contentString}</MarkdownText>}
{hasToolCalls && <ToolCalls toolCalls={message.tool_calls} />}
```

**结论：LobeChat 的 content 字段永远只包含自然语言文本，tool_calls 永远是独立的数组字段。不存在 JSON 泄漏问题，因为后端协议层面就分离了。**

### 2.2 LangChain agent-chat-ui — [ai.tsx](tmp/lcui-analysis/src/components/thread/messages/ai.tsx) + [utils.ts](tmp/lcui-analysis/src/components/thread/utils.ts)

**核心发现：消息结构级分离 + getContentString 过滤**

LangChain 的 AIMessage 原生支持 `content` 和 `tool_calls` 分离：

```typescript
interface AIMessage {
  content: string | MessageContentComplex[];  // 只有文本
  tool_calls: ToolCall[];                    // 工具调用独立字段
}
```

**getContentString()**（[utils.ts:9-15](tmp/lcui-analysis/src/components/thread/utils.ts#L9-L15)）— 只提取文本部分：
```typescript
export function getContentString(content): string {
  if (typeof content === "string") return content;
  const texts = content
    .filter(c => c.type === "text")   // ← 只取 text 类型，忽略 tool_use
    .map(c => c.text);
  return texts.join(" ");
}
```

即使 Anthropic 流式响应将工具调用嵌入 content（作为 `tool_use` type block），前端也会通过 `parseAnthropicStreamedToolCalls()` 解析到 `tool_calls` 数组中，渲染时 `getContentString()` 自动跳过非 text 块。

### 2.3 Vercel AI SDK — 结构化 SSE 协议

Python streaming 参考实现中的事件类型：

```python
# 文本事件（独立通道）
yield {"type": "text-start", "id": "text-1"}
yield {"type": "text-delta", "id": "text-1", "delta": "Hello"}

# 工具调用事件（独立通道，与文本完全隔离）
yield {"type": "tool-input-start", "toolCallId": "call_123", "toolName": "list_mounts"}
yield {"type": "tool-input-delta", "toolCallId": "call_123", "delta": '{"path":"/"}'}
yield {"type": "tool-input-end", "toolCallId": "call_123"}
```

### 2.4 三项目对比总结

| 维度 | LobeChat | LangChain agent-chat-ui | Vercel AI SDK | **我们的项目** |
|------|----------|------------------------|---------------|---------------|
| **SSE 协议** | `chunkType` 区分 text/tools_calling | SDK 内置分离 | `type` 区分 text/tool-input | ❌ 全部混在 text_delta |
| **消息结构** | `{content, tools, reasoning}` 分离 | `{content, tool_calls}` 分离 | 分离事件流 | ⚠️ content 可能含裸 JSON |
| **后端检测** | AgentRuntime.step() 解析后分发 | OpenAI/Anthropic SDK 自动解析 | SDK 自动解析 | ⚠️ 文本启发式检测 extractToolCallsFromText |
| **前端渲染** | MarkdownText + ToolCalls 组件分开 | 同左 | 同左 | ⚠️ renderTurnItems 支持但 content 含脏数据 |
| **remainingText 处理** | 不需要（协议已分离） | 不需要 | 不需要 | ❌ **缺失！Branch B 未补发** |

**根本差距**：三个主流项目都在 **协议/SDK 层面** 实现了 content 与 tool_calls 的分离。我们的项目因为 gptgod 代理不发送标准 `tool_call_chunk` 事件，被迫在 **文本层面做启发式检测**，且检测后的处理链路不完整。

---

## 三、根因分析（基于开源对照）

### 3.1 问题 1 根因：嵌入式工具调用 JSON 绕过缓冲 → 泄漏到 content

**时序还原**：

```
t=0ms   text_delta: "回答\"有哪些视频文件\"，必须先调用..."
        → bufMode=true, 缓冲中（<60字符）

t=50ms  text_delta: "，然后再查看目录内容。\n\n"
        → roundTextContent 长度 > 60, 触发 looksLikeToolCheck()
        → 检测：文本以中文开头 → 返回 false
        → flushBuffer()！bufMode=false ★ 缓冲释放

t=100ms text_delta: "[{\"name\":\"list_mounts\",\"arguments\":{}}]"
        → bufMode=false → 直接作为 text_delta 转发给前端 ★★ JSON 泄漏！

t=150ms text_delta: "\n\nstudio_video...在 /Movies 目录下的可用视频文件有："
        → bufMode=false → 直接转发

t=200ms stream_end
        → gotToolCalls=false（gptgod 未发 tool_call_chunk）
        → Branch B: extractToolCallsFromText(roundTextContent) 成功 ✅
        → emitToolCallEvent() 发送 tool_call 事件 ✅
        → 但 remainingText 未发送 ❌
        → 已泄漏的 JSON 也无法撤回 ❌
```

**对照 LobeChat**：LobeChat 不会遇到此问题，因为它的后端（AgentRuntime）在调用 LLM API 时使用官方 SDK，`tool_calls` delta 通过独立的事件通道（`chunkType='tools_calling'`）传输，永远不会混入 text 通道。

### 3.2 问题 2 根因：Branch B remainingText 未补发

`extractToolCallsFromText()` 正确返回了 `remainingText`（JSON 之后的自然语言部分），但 Branch B 成功路径只做了 emitToolCallEvent + 执行工具 + continue，**从未将 remainingText 作为 text_delta 补发给前端**。

**对照三个开源项目**：它们都不需要补发 remainingText，因为在协议层面文本和工具调用就是分开的。我们的架构决定了必须手动补发。

---

## 四、修复方案（参考 LobeChat/LangChain 模式）

### 设计原则（从三项目提炼）

> **P0: content 字段只包含自然语言文本，tool_calls 通过独立事件/字段传递**
>
> **P1: 后端检测到工具调用后，必须从文本流中剥离 JSON 并补发剩余文本**
>
> **P2: 前端渲染前做安全网过滤（防御性编程）**

### 4.1 后端修复 A：增强缓冲检测 —— 支持嵌入式 + 二次捕获

**文件**: `/workspace/internal/server/agent_api.go`
**参考**: LobeChat 的 `chunkType` 分发逻辑——我们在无法修改上游协议的情况下，用更强的启发式检测模拟同样的效果

#### 4.1.1 新增 `containsEmbeddedToolCallPattern()` 辅助函数

在流处理循环之前（约 L805 附近）新增：

```go
// containsEmbeddedToolCallPattern 在任意位置扫描工具调用 JSON 特征。
// 参考 LobeChat 的协议级分离思路：由于 gptgod 代理不发送标准 tool_call_chunk，
// 我们需要在文本层做更精确的检测来模拟同样的效果。
func containsEmbeddedToolCallPattern(s string) bool {
	if len(s) < 20 { return false }
	return strings.Contains(s, `[{"name"`) ||
		strings.Contains(s, `{"name":"`) ||
		strings.Contains(s, `"function":`) ||
		strings.Contains(s, `"arguments":`)
}
```

#### 4.1.2 改造 `looksLikeToolCheck` 闭包

增加策略 2（嵌入式检测）：

```go
looksLikeToolCall := func(s string) bool {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) < 3 { return false }
	// 策略 1：文本以 [ 或 { 开头（原有逻辑）
	if (trimmed[0] == '[' || trimmed[0] == '{') &&
		strings.Contains(trimmed, `"name"`) {
		return true
	}
	// ★ 策略 2：文本任意位置嵌入工具调用 JSON 特征（新增）
	return containsEmbeddedToolCallPattern(trimmed)
}
```

#### 4.1.3 实时模式增加二次检测

修改 else 分支（正常实时模式，当前 L862-L867）：

```go
} else {
	// 正常实时模式或缓冲已释放
	// ★ 二次检测：如果累积文本中出现嵌入式工具调用特征，
	//   立即重新进入缓冲模式（参考 LobeChat 的 chunkType 分发思路）
	if !suspectedToolCall && containsEmbeddedToolCallPattern(roundTextContent) {
		slog.Info("agent: mid-stream embedded tool call detected, re-buffering",
			"text_len", len(roundTextContent),
			"chunk_preview", truncateStr(textChunk, 40))
		suspectedToolCall = true
		bufMode = true
		textBuf = append(textBuf, textChunk)
	} else {
		textSeq++
		s.sendAndCache(sess, c.Writer, flusher, "text_delta",
			map[string]interface{}{"seq": textSeq, "text": textChunk})
	}
}
```

#### 4.1.4 新增 `truncateStr` 辅助函数

```go
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen { return s }
	return s[:maxLen] + "..."
}
```

### 4.2 后端修复 B：Branch B 成功路径补发 remainingText

**文件**: `/workspace/internal/server/agent_api.go`
**位置**: Branch B 成功路径末尾、continue 之前（约 L1082 之后）
**参考**: LobeChat 的 `accumulatedContent` 模式——我们用 splitTextIntoChunks 模拟同样的增量推送效果

```go
// ★ 补发剩余文本（参考 LobeChat stream_chunk chunkType='text' 的增量模式）
// extractToolCallsFromText 返回的 remainingText 是剥离工具调用 JSON 后的
// 自然语言部分。LobeChat 不需要这个步骤因为它的协议天然分离。
if remainingText != "" {
	chunks := splitTextIntoChunks(remainingText, 100)
	for _, ch := range chunks {
		textSeq++
		s.sendAndCache(sess, c.Writer, flusher, "text_delta",
			map[string]interface{}{"seq": textSeq, "text": ch})
	}
	slog.Info("agent: Branch B remaining text sent to client",
		"remaining_len", len(remainingText), "chunks", len(chunks))
}
```

#### 4.2.1 新增 `splitTextIntoChunks` 辅助函数

```go
func splitTextIntoChunks(text string, chunkSize int) []string {
	runes := []rune(text)
	if len(runes) <= chunkSize {
		return []string{text}
	}
	var chunks []string
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) { end = len(runes) }
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}
```

### 4.3 后端修复 C：Branch A 入口处 flush 前置文本残留

**文件**: `/workspace/internal/server/agent_api.go`
**位置**: Branch A 入口（L912 之后）

```go
if gotToolCalls && len(roundToolCalls) > 0 {
	// ★ 如果有缓冲区残留的前置文本（工具调用之前的正常正文），立即发送
	if len(textBuf) > 0 {
		flushBuffer()
		slog.Info("agent: Branch A flushed pre-tool-call buffered text",
			"buf_chunks", len(textBuf))
		textBuf = nil
	}
	// ... 后续代码不变
```

### 4.4 前端修复 D：渲染层安全网（参考 LangChain getContentString 模式）

**文件**: `/workspace/app/encv-mobile/src/composables/renderTurnItems.ts`
**参考**: LangChain 的 `getContentString()` — 只提取文本，忽略工具调用块

#### 4.4.1 新增 `stripToolCallJSON()` 函数

```typescript
/**
 * 从消息文本中剥离工具调用 JSON 片段。
 *
 * 参考 LangChain agent-chat-ui 的 getContentString() 设计：
 * - LobeChat: 协议级分离，content 永远不含 tool JSON
 * - LangChain: content 可能含 tool_use block，渲染时 getContentString() 过滤
 * - 我们: 后端可能泄漏 tool JSON 到 content，渲染前需清理
 *
 * 匹配 OpenAI function calling 格式:
 *   [{"name":"xxx","arguments":{...}}]  — 数组形式
 *   {"name":"xxx","arguments":{...}}      — 单对象形式
 */
export function stripToolCallJSON(text: string): string {
  if (!text) return text
  let cleaned = text
  // 模式 1: [...{"name":"...",...}] 数组形式
  cleaned = cleaned.replace(
    /\[\s*\{\s*"name"\s*:\s*"[^"]*"(?:\s*,\s*"[^"]*"\s*:\s*(?:\{[^}]*\}|"[^"]*"))*\s*\}\s*\]/g,
    '',
  )
  // 模式 2: {"name":"...",...} 单对象形式（独立行）
  cleaned = cleaned.replace(
    /^\s*\{\s*"name"\s*:\s*"[^"]*"(?:\s*,\s*"[^"]*"\s*:\s*(?:\{[^}]*\}|"[^"]*"))*\s*\}\s*/gm,
    '',
  )
  // 清理多余空行
  cleaned = cleaned.replace(/\n{3,}/g, '\n\n')
  return cleaned.trim()
}
```

#### 4.4.2 修改 assistant content 渲染逻辑

**位置**: [renderTurnItems.ts:355-363](app/encv-mobile/src/composables/renderTurnItems.ts#L355-L363)

```typescript
// ── 原代码 ──
const contentText = contentToText(msg.content, 'assistant')
if (contentText && contentText.trim().length > 0) {
  out.push({ type: 'assistantText', messageId: `a-${idx}`, text: contentText, streaming })
}

// ── 新代码（参考 LangChain ai.tsx:163-179 的分离渲染模式）───
const rawContentText = contentToText(msg.content, 'assistant')
// 安全网：若本消息已被解析出 tool_calls，从显示文本中剥离可能的 JSON 残留
const displayText =
  (msg.tool_calls?.length ?? 0) > 0
    ? stripToolCallJSON(rawContentText)
    : rawContentText
if (displayText && displayText.trim().length > 0) {
  out.push({ type: 'assistantText', messageId: `a-${idx}`, text: displayText, streaming })
}
```

---

## 五、改动清单

| # | 文件 | 改动类型 | 改动摘要 | 参考来源 |
|---|------|---------|---------|---------|
| 1 | `internal/server/agent_api.go` | 修改 | 新增 `containsEmbeddedToolCallPattern()` + `splitTextIntoChunks()` + `truncateStr()` | LobeChat chunkType 分发 |
| 2 | `internal/server/agent_api.go` | 修改 | 改造 `looksLikeToolCheck`: 增加策略 2（嵌入式检测） | LobeChat 协议分离思路 |
| 3 | `internal/server/agent_api.go` | 修改 | 实时模式 else 分支增加二次检测 + 重入缓冲 | LobeChat stream_chunk 分发 |
| 4 | `internal/server/agent_api.go` | 修改 | Branch B 成功路径补发 remainingText（分块 text_delta） | LobeChat accumulatedContent |
| 5 | `internal/server/agent_api.go` | 修改 | Branch A 入口 flush 前置文本残留 | LobeChat stream_start 重置 |
| 6 | `app/encv-mobile/src/composables/renderTurnItems.ts` | 修改 | 新增 `stripToolCallJSON()` 导出函数 | LangChain getContentString |
| 7 | `app/encv-mobile/src/composables/renderTurnItems.ts` | 修改 | assistant content 渲染前调用 stripToolCallJSON | LangChain ai.tsx 分离渲染 |

---

## 六、边界情况与防护

| 场景 | 预期行为 | 保证机制 |
|------|---------|---------|
| 纯文本回复（无工具调用） | 正常实时流式输出 | `containsEmbeddedToolCallPattern` 对普通文本返回 false |
| 回复以工具调用 JSON 开头 | 被 60 字符窗口内的原有检测拦截 | 策略 1（开头检测）保持不变 |
| 正文 + 多个工具调用 JSON | 全部被拦截，仅 remainingText 显示 | `extractToolCallsFromText` Strategy 6 提取所有匹配数组 |
| gptgod 正确发送 tool_call_chunk | 走 Branch A，不受缓冲逻辑影响 | Branch A 有独立的 flush 残留处理 |
| remainingText 本身也包含工具调用 | 仅发送一次 remainingText，不会无限循环 | Branch B continue 进入新 round，新 round 有独立检测 |
| 前端 content 为空但 tool_calls 存在 | `stripToolCallJSON("")` 返回空字符串 | 函数入口有空值守卫 |
| JSON 残留在 ```json 代码块中 | 正则覆盖代码块内外两种格式 | stripToolCallJSON 同时匹配两种格式 |

---

## 七、验证步骤

1. **编译验证**
   ```bash
   cd /workspace && go build ./cmd/encv       # Go 0 错误
   cd /workspace/app/encv-mobile && npx vue-tsc --noEmit  # TS 0 错误
   cd /workspace/app/encv-mobile && npx vite build        # Vite 0 错误
   ```

2. **按规范重启**（禁止手动 go build / nohup）
   ```bash
   bash app/encv-mobile/scripts/start-preview.sh
   ```

3. **功能验证（真机）**

   | 测试用例 | 预期结果 | 对照标准 |
   |----------|---------|---------|
   | 提问"有哪些视频文件" | 工具调用 = GroupedOperationMessage 卡片；无裸 JSON；回答完整 | LobeChat 同等体验 |
   | 提问普通问题（不触发工具） | 正常流式输出，无延迟感 | LobeChat stream_chunk text |
   | 提问触发多轮工具调用的问题 | 每轮工具调用都显示为结构化组件；最终回答完整 | LobeChat multi-step |
   | 切换不同模型（4o / 4o-mini） | 模型切换生效；各模型下的工具调用均正确渲染 | — |
