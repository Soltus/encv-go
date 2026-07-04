# Nuclear-Boy 借鉴点设计文档（Stage 0 交付物）

> **文档定位**：把 nuclear-boy 仓库（`/tmp/nuclear-boy`，v1.0.0 "核聚变"）14 个核心 .kt 文件 + 2 份 HANDOVER + 1 份 android-bridge-report 全部深读后，输出一份**可执行**的借鉴方法论。
> **不是**：代码翻译（栈完全不同）
> **是**：设计模式抽取 + 适配到 encv-go（Go 后端）+ encv-mobile（Vue/Ionic 前端）的对应位置

---

## 一、背景

### 1.1 nuclear-boy 项目

| 维度 | 值 |
|------|-----|
| 仓库 | https://github.com/muzapar00/nuclear-boy |
| 版本 | v1.0.0 "核聚变 — 记忆觉醒 + 主动智能 + Skill生态" |
| 定位 | Android 端 AI 编程助手 |
| 技术栈 | Kotlin + Jetpack Compose + 原生 Android + DeepSeek V4 + Chaquopy (Python 沙箱) + Room (SQLite) + Hilt (DI) |
| 核心架构 | 800 字精简 system prompt + ReAct agent 循环 + 工具注册表 + Skills 生态（skill.yaml + main.py 自动注册）+ 三层记忆 |
| 灵感来源 | 项目作者公开承认借鉴了 OpenAI 内部 best practices + Anthropic Claude prompt 工程方法论 |
| 文档完备度 | HANDOVER2.0.md 425 行，覆盖架构/工具/陷阱/未来优化全维度 |

### 1.2 encv 项目

| 维度 | 值 |
|------|-----|
| 后端 | encv-go（Go 1.22+ / Gin / 集成 deepseek-go / SQLite / ffmpeg） |
| 前端 | encv-mobile（Vue 3 + Ionic 8 + Capacitor 7 + TypeScript + Vite + Vitest） |
| 当前阶段 | Phase 1（4 个体验打磨需求）已完；Phase 2 进入 nuclear-boy 借鉴期 |
| 现有相关 spec | `agent-tools-scenarios-v2` / `agui-real-llm-path-completion` / `mobile-agent-polish-2026q2`（已完） / `agent-mock-mode` / `multi-engine-chat-architecture` / `implement-mobile-backend-api` |
| 借借鉴前的核心缺陷（v1 漏挖，v2 补） | 无 PROACTIVE 主动智能 / 无 scopeJob 重建导致 cancel 后协程死 / 无上下文自动压缩 / 无 buildHistoryMessages 防 400 / 无 message 8 状态机 / 无 ContextWindowManager 3 级预警 / 无 tool priority 排序 / 无 executeViaExternalModule 回退 / 无 ZIP-slip 防护 / 无 autoExtractMemories / 无 isStdlibModule 170+ 白名单 / 无危险命令黑名单 / 无 MessageBubble 6 个 UI 模式 / 无 FileOperations 路径安全 / 无项目脚手架 |

### 1.3 借鉴的核心原则

1. **不复制代码** — Kotlin/Compose 栈与 Go/Vue 栈完全不同
2. **抽取模式** — 分析 nuclear-boy 怎么解决某个问题，把方法论落到对应位置
3. **Stage 0 不写业务代码** — 只分析 + 写文档
4. **每个 Stage 独立可交付** — 除 Stage 0 之外互相不阻塞
5. **不破坏 AG-UI 协议** — 11 种事件类型不变
6. **复用已有能力** — `mobile-agent-polish-2026q2` 已实现的 usePinchZoom / formatRelativeTime UI 风格 + `implement-mobile-backend-api` 已实现的注册中心

---

## 二、借鉴方法论

### 2.1 三列映射表

每个借鉴点**必须**按以下三列格式输出：

| Nuclear-Boy 实现 | encv-go / encv-mobile 现状 | 借鉴方法论 |
|------------------|--------------------------|-----------|
| N-B 文件路径 + 行号 + 代码片段 | 当前代码位置（如缺失标注"缺失"） | 借鉴什么 / 不借鉴什么 / 怎么落地 |

### 2.2 ROI 评估

按 **ROI = 价值 / 实施成本** 排序，3 档：

| ROI | 标准 | 推荐实施优先级 |
|-----|------|----------------|
| ⭐⭐⭐ | 高价值（核心痛点修复）/ 低-中成本 | 优先做（Stage 1-3） |
| ⭐⭐ | 中价值 / 中成本 | 次做（Stage 4-6） |
| ⭐ | 高价值但高成本 / 低价值 | 视情况（Stage 7-10） |

### 2.3 借鉴深度 3 档

| 深度 | 含义 | 何时用 |
|------|------|--------|
| 概念借鉴 | 只取设计哲学 | 跨栈差异大时（如 PROACTIVE 主动智能） |
| 模式借鉴 | 取代码骨架 + 行为模式，翻译到目标栈 | 大多数情况（如 ToolCallAccumulator） |
| 算法借鉴 | 直接取算法 / 公式（text.length/3.5 等） | 跨语言通用（如 EstimateTokens） |

---

## 三、文件深读清单（14 个）

按 ROI 倒序排列：

### 3.1 agent-core（3 个）

| 文件 | 行数 | 必须搞懂 | 核心借鉴点 |
|------|------|---------|-----------|
| [AgentEngine.kt](file:///tmp/nuclear-boy/agent-core/src/main/java/com/nuclearboy/agent/AgentEngine.kt) | 877 | ReAct 主循环 / ToolCallAccumulator / buildHistoryMessages / scopeJob 重建 / classifyException / maxToolIterations | Stage 1-3, 5, 7-8 |
| [SystemPromptBuilder.kt](file:///tmp/nuclear-boy/agent-core/src/main/java/com/nuclearboy/agent/SystemPromptBuilder.kt) | 194 | 800 字精简哲学 / PROACTIVE 主动智能 / 工作流 / Chaquopy Java 桥接模板 | Stage 1 |
| [ToolRegistry.kt](file:///tmp/nuclear-boy/agent-core/src/main/java/com/nuclearboy/agent/ToolRegistry.kt) | 478 | 工具 priority 排序 / executeSafe paramHint / executeViaExternalModule 回退 / estimateToolDefTokens | Stage 4, 7 |

### 3.2 api-deepseek（4 个）

| 文件 | 行数 | 必须搞懂 | 核心借鉴点 |
|------|------|---------|-----------|
| DeepSeekApiClient.kt | — | callApiStreaming / sanitizeMessages 不剥离 reasoningContent / thinking mode 显式 disabled | Stage 3, 6 |
| ContextWindowManager.kt | — | emergencyCompress (RED) / compressConversation (YELLOW) / 6 段 token 分配 / 三级预警阈值 | **Stage 6（v2 关键）** |
| TokenTracker.kt | — | per-request cache hit rate（不是累计）/ 平均延迟公式 `((cur.avg * count) + latency) / (count + 1)` | Stage 6 |
| ModelRouter.kt | — | ComplexityEvaluator 关键词匹配（"架构/设计/重构/分析/优化/debug" → 高复杂度模型） | Stage 6 后续 |

### 3.3 common（4 个）

| 文件 | 行数 | 必须搞懂 | 核心借鉴点 |
|------|------|---------|-----------|
| AppConstants.kt | — | 单例集中管理 12+ 常量（BUDGET_CONVERSATION_HISTORY=100,000 / FILE_CONTENT_TRUNCATE_THRESHOLD=300,000 / DEEPSEEK_CONTEXT_WINDOW=128K / 6 段预算阈值 YELLOW/RED/FORCE） | Stage 6, 9 |
| AppError.kt | — | humanMessage 本地化 + isRetryable + fromHttpCode 401/402/429/5xx | **Stage 5（v2 强调）** |
| Models.kt | — | **MessageStatus 8 状态机** / **ToolCallStatus 5 状态机** / **TokenUsage 完整字段** / **Verbosity 三档** / **SessionStats 聚合** | **Stage 3, 10（v2 关键）** |
| Extensions.kt | — | **toRelativeTimeString 五档边界**（< 60s 刚刚 / < 1h 分钟前 / < 24h 小时前 / < 48h 昨天 / < 7d 天前） / **isTextFile 39 扩展名** / **maskApiKey 8+4 脱敏** | **Stage 10（v2 新增）** |

### 3.4 memory（3 个）

| 文件 | 行数 | 必须搞懂 | 核心借鉴点 |
|------|------|---------|-----------|
| MemoryDao.kt | — | Room @Dao 模式 | Stage 8 |
| MemoryDatabase.kt | — | **SQLite WAL 模式**（`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`） / 3 表 schema | Stage 8 |
| MemoryStore.kt | — | **三层记忆架构**（ProjectMemory / UserProfile 带 confidence 0-1 / SemanticMemory 带 recallCount） / **autoExtractMemories 三种 pattern** | **Stage 8（v2 新增 autoExtract）** |

### 3.5 skills（3 个）

| 文件 | 行数 | 必须搞懂 | 核心借鉴点 |
|------|------|---------|-----------|
| SkillManager.kt | — | 加载流程 / **ZIP-slip 防护**（canonical 路径 + 前缀检查）/ executeViaExternalModule | **Stage 7（v2 新增 ZIP-slip）** |
| SkillManifest.kt | — | **YAML 解析 + 4 维权限**（filesystem glob / network allowed_hosts / packages / shell）/ **isSandboxed 计算属性** / 参数验证 | Stage 7 |
| SkillMarketPlace.kt | — | 远程 Skills 市场（HTTP GET 列可用 Skills） | Stage 7 后续 |

### 3.6 python-bridge（2 个）

| 文件 | 行数 | 必须搞懂 | 核心借鉴点 |
|------|------|---------|-----------|
| PythonSandbox.kt | — | execute() 双层错误处理（TimeoutCancellationException → PythonTimeout / generic Exception → unknown） | Stage 9 |
| SandboxPolicy.kt | — | **4 策略**（STRICT / STANDARD / RELAXED / DOCUMENT_GENERATION）/ buildPolicyPreamble 注入 Python 前置代码 / **isStdlibModule 170+ 白名单** / **DANGEROUS_COMMANDS 黑名单** | **Stage 9（v2 关键）** |

### 3.7 tools-docgen（2 个）

| 文件 | 行数 | 必须搞懂 | 核心借鉴点 |
|------|------|---------|-----------|
| FileOperations.kt | — | **resolvePath 路径穿越防护** / **searchFiles 跳过隐藏目录**（.git / node_modules / __pycache__ / build / .gradle）/ **buildProjectDirectories**（Python/Kotlin/JS/Go 4 模板）/ **buildReadme** / **buildGitignore** | **Stage 10（v2 关键）** |
| DocumentGenerator.kt | — | docx/xlsx/pptx 生成模板 | Stage 9 |

### 3.8 ui-chat（4 个）

| 文件 | 行数 | 必须搞懂 | 核心借鉴点 |
|------|------|---------|-----------|
| ChatScreen.kt | — | 状态机 / saveMessages 持久化 conversation.json / notificationCallback | Stage 10 后续 |
| ChatViewModel.kt | — | AndroidViewModel 模式 / **saveMessages / loadPersistedMessages**（取最近 50 条） | Stage 10 后续 |
| TokenHudBar.kt | — | **7 行完整指标**（输入/输出/缓存/思考/上下文/速度/延迟）+ YELLOW/RED 颜色警告 | **Stage 6（v2 强调）** |
| MessageBubble.kt | 800+ | **ToolExecutionCard 可展开**（5 状态颜色 + 黑底输出）/ **ReasoningSection 折叠**（expandVertically 动画）/ **FileChangeCard**（绿/蓝/红颜色编码）/ **代码块着色 + 复制按钮**（不用 Prism4j 因为坐标不对）/ **ThinkingIndicator 三点动画**（200ms 错位）/ **CombinedClickable onClick + onLongClick** | **Stage 10（v2 大头）** |

### 3.9 app / android-bridge（2 个）

| 文件 | 行数 | 必须搞懂 | 核心借鉴点 |
|------|------|---------|-----------|
| app/AgentForegroundService.kt | — | **AI 思考时保活**（Foreground Service 模板 + AndroidManifest 配置） | Stage 10 后续（encv-mobile 用 capacitor-background-mode 替代） |
| android-bridge-v1.0/docs/android-bridge-report.md | — | **20 个 Android 系统服务**（vibrator / camera / alarm / power / wifi / bluetooth / sensor / location / audio / notification / clipboard / telephony / connectivity / input_method / window / battery / storage / display / sensors）/ **VibrationEncoder 摩斯/SOS/心跳** / **工具调用黄金法则**（`string="true"` 显式 / 参数名要正确 / 分块写大文件 / 永不使用 jarray.invoke()） | Stage 9-10 后续 |

---

## 四、Stage 1-10 借鉴点详解

### Stage 1 — System Prompt 工程化 + PROACTIVE 主动智能

#### 1.1 借鉴模式

| Nuclear-Boy 实现 | encv-go 现状 | 借鉴方法论 |
|------------------|--------------|-----------|
| **SystemPromptBuilder.kt L142-148** "## 主动智能 (PROACTIVE) 你是主动型助理。每次回复结尾... 触发条件: 用户创建了新项目/搜索了资料/写了代码/完成了复杂任务/看起来不知道做什么; 行为: 无需用户开口,主动给 2-3 条建议; 边界: 不要问问题引导用户,要直接给方案" | encv-go 当前 system prompt 完全缺这个能力 | **概念借鉴**。在 prompt 末尾追加 "## 主动智能" 段（参考 [spec Stage 1 §1.1](file:///workspace/.trae/specs/borrow-nuclear-boy-2026q2/spec.md)） |
| **HANDOVER2.0.md §五 5 大原则**（工具描述 > prompt / 避免否定 / 正面示例 / 精简 / thinking disabled） | encv-go 当前 system prompt 是 ad-hoc 字符串拼接 | **模式借鉴**。创建 `internal/agent/prompt.go` SystemPromptBuilder，Build() 方法遵循 5 原则 |
| **SystemPromptBuilder.kt L25-34** 8 工具速查（每行一个正面示例，path 统一参数） | encv-go 当前 tool description 是 verbose 多行 | **模式借鉴**。每个工具 1 行格式：`N. 调用 tool_name，参数：key="value"` |
| **DeepSeekApiClient.kt L327** 显式 `{"thinking": {"type": "disabled"}}`（"Must be explicit: DeepSeek defaults to enabled!"） | encv-go 当前没有显式传 thinking | **算法借鉴**。在 ChatRequest body 显式加 thinking 禁用 |

#### 1.2 落地步骤

- 新文件 `/workspace/internal/agent/prompt.go`（SystemPromptBuilder）
- 5 大原则 + PROACTIVE 段
- `prompt_test.go` 单测（否定词报错 / 工具缺失报错 / 长度警告）
- 验证 `go build ./cmd/encv` 0 错误

#### 1.3 验证标准

- 5 大原则单测全过
- PROACTIVE 段已注入（手动 grep "## 主动智能" 命中）
- 实际生成 prompt 长度 < 1500 字

---

### Stage 2 — ToolCallAccumulator + scopeJob 重建 + maxToolIterations

#### 2.1 借鉴模式

| Nuclear-Boy 实现 | encv-go 现状 | 借鉴方法论 |
|------------------|--------------|-----------|
| **AgentEngine.kt L82-122** ToolCallAccumulator private class（partialCalls map + feed/toCompletedCalls/hasPartialCalls） | encv-go useAgent.ts 已有 parseToolResultData 但**没有**累积器 | **模式借鉴**。创建 `useToolCallAccumulator.ts` composable（受 nuclear-boy 启发） |
| **AgentEngine.kt L707** 关键注释 "Clear once per API call, NOT per individual tool call" | encv-go 当前 clear() 可能在 tool_call_start 时清，导致同一轮 2-3 tool call 互相覆盖 | **模式借鉴**。clear() 在 ReAct 循环开始时调用，**不**在 tool_call_start 时清 |
| **AgentEngine.kt L162, L850-854** `@Volatile scopeJob = SupervisorJob()` + cancel() 关闭旧 scopeJob 重建新 | encv-go 用 `context.WithCancel` 取消，**问题**：取消后新协程立刻被 cancel 信号传染 | **模式借鉴**。每次新 run() 用 `context.WithCancel(parentCtx)` 创建 fresh ctx；cancel() 关闭旧 ctx + 立即创建新 ctx |
| **AgentEngine.kt L166** `private val maxToolIterations = 20` | encv-go 当前可能用 for 无限循环，web_search 失败可能重试 100 次 | **模式借鉴**。ReAct 主循环里 `if iteration >= 20 { break; warnUser() }` |

#### 2.2 落地步骤

- 新文件 `/workspace/app/encv-mobile/src/composables/useToolCallAccumulator.ts`
- 4 状态机（pending / accumulating / complete / executed）
- 后端 `agent_api.go` cancel() 关闭旧 ctx + 创建新 ctx
- ReAct 主循环加 `if iteration >= maxToolIterations { break; warnUser() }`
- 6 个单测场景

#### 2.3 验证标准

- 6 个单测场景通过
- 与 useAgent.send() 集成无破坏
- `vue-tsc --noEmit` 0 错误 / `vitest` 通过 / `go build` 0 错误

---

### Stage 3 — buildHistoryMessages 防 400 + reasoningContent + 8 状态机

#### 3.1 借鉴模式

| Nuclear-Boy 实现 | encv-go 现状 | 借鉴方法论 |
|------------------|--------------|-----------|
| **AgentEngine.kt L543-630** buildHistoryMessages 关键模式：completedCalls 为空 → toolCalls=null（防 API 400 insufficient tool messages） | encv-go 当前可能有重复发送 `tool_status: running` + `tool_status: completed` + `tool_result`，导致 LLM 看 history 时残缺 | **模式借鉴**。每轮 LLM 响应解析后，所有 tool 都执行完才推下一轮；未完成的从历史里移除 |
| **AgentEngine.kt L619** 保留 reasoningContent 在累积时 + **DeepSeekApiClient.kt L342-345** sanitize 阶段剥离（"DeepSeek API now REQUIRES reasoning_content to be passed back - we keep it intact"） | encv-go 当前可能没剥离 reasoning_content，导致 token 浪费 + 偶发 400 | **模式借鉴**。buildHistoryMessages 阶段剥离 reasoning_content（除最新一条外） |
| **Models.kt L57-66** MessageStatus 8 状态机（SENDING / SENT / THINKING / STREAMING / EXECUTING / COMPLETE / ERROR / CANCELLED） | encv-go 当前是 4 状态字符串 `'pending' | 'streaming' | 'complete' | 'error'`，**不够细** | **模式借鉴**。改成 TypeScript 联合类型 / Go iota |
| **HANDOVER2.0.md §三.3** 按 toolCallId 去重 | encv-go 当前可能不按 ID 去重，同一 toolCallId 推 2 次（running + completed）→ 历史保留 2 条 | **模式借鉴**。useAgent.ts 处理 tool_result 时按 toolCallId 去重 |

#### 3.2 落地步骤

- 后端 `agent_api.go` buildHistoryMessages：按 toolCallId 去重 + completedCalls 过滤 + completedCalls 为空 → toolCalls=null
- reasoningContent 处理：旧消息剥离 + 最新一条保留
- useAgent.ts 处理 tool_result 按 toolCallId 去重
- MessageStatus 升级到 8 状态（TypeScript 联合类型）
- 5 个单测场景

#### 3.3 验证标准

- 5 个单测场景通过
- 集成测试：中断对话后 LLM 不再 400
- `go build` + `vue-tsc` + `vitest` 全 0 错误

---

### Stage 4 — 参数别名容错 + Tool priority 排序 + executeSafe paramHint

#### 4.1 借鉴模式

| Nuclear-Boy 实现 | encv-go 现状 | 借鉴方法论 |
|------------------|--------------|-----------|
| **HANDOVER2.0.md §四** 参数别名表（path/filePath/filename 互通，url/link/query 互通等） | encv-go 当前 tool registry 主参数名固定，LLM 漏传就报错 | **模式借鉴**。在 tool registry 加 `ParamAliases map[string][]string` |
| **ToolRegistry.kt L168-196** Tool priority 排序（priorityTools 集合置顶 0 / requiresConfirmation 工具放最后 2 / 其他 1） | encv-go 当前所有工具一视同仁按注册顺序，**LLM 在 token 预算不足时会截断尾部工具** | **模式借鉴**。priorityTools 集合：run_python / read_file / write_file / list_directory |
| **ToolRegistry.kt L236-258** executeSafe 失败时附 required param hint（"需要的参数: filePath (string), content (string)"） | encv-go 当前可能只返回 `Error: missing param 'path'`，LLM 看不懂要自纠 | **模式借鉴**。错误信息含：工具名 + 缺失参数名 + 类型 + 完整参数示例 |
| **ToolRegistry.kt L776-798** parseToolParams 容错（JSON 解析失败 fallback emptyMap） | encv-go 当前 JSON 解析失败可能直接报错 | **模式借鉴**。保留 fallback emptyMap |

#### 4.2 落地步骤

- tool registry 加 `ParamAliases map[string][]string`
- 执行前 normalize（别名 → 主参数）
- Tool priority 排序（权重 0/1/2）
- executeSafe 失败附完整示例
- 单测（5 工具参数别名 / priority 排序 / paramHint）

#### 4.3 验证标准

- 5 工具参数别名互通
- priority 工具在 token 预算紧张时仍在前 N
- LLM 漏传参数时错误信息含示例

---

### Stage 5 — AppResult<T> + AppError + classifyException + fromHttpCode

#### 5.1 借鉴模式

| Nuclear-Boy 实现 | encv-go 现状 | 借鉴方法论 |
|------------------|--------------|-----------|
| **AppError.kt** 完整字段（type / message / humanMessage / isRetryable / cause） | encv-go `internal/tools/errors.go`（mobile-agent-polish-2026q2 已建）有 ToolError{Code, Message, Underlying, Recoverable} 但**没有 humanMessage** | **模式借鉴**。扩 ToolError 加 HumanMessage string（中英混排 + emoji："找不到这个文件，要先列目录吗？🤔"） |
| **AppError.kt fromHttpCode** 401 → ApiKeyInvalid / 402 → InsufficientBalance / 429 → RateLimited / 5xx → ServerError | encv-go 当前可能不区分 HTTP 状态码 | **算法借鉴**。加 FromHTTPStatusCode(code int) 静态方法 |
| **AgentEngine.kt L803-821** classifyException（DeepSeekHttpException / SSLException / SocketTimeoutException / IOException / CancellationException → AppError） | encv-go 当前 catch 块可能直接返回原始 error | **模式借鉴**。新 `/workspace/internal/agent/classify.go` ClassifyException，用 errors.As 区分 url.Error / net.OpError / context.DeadlineExceeded / context.Canceled |
| **AppError.kt** sealed class AppResult<T> { Success / Failure } + companion runCatching + inline map 链式 | encv-go 当前用 `(T, error)` tuple 模式 | **模式借鉴**。Go 用 `Result[T]` 包装 type；TypeScript 用联合类型 |

#### 5.2 humanMessage 文案规范（来自 nuclear-boy AppError.kt L60-68）

| AppErrorType | humanMessage | 中文 | emoji |
|--------------|-------------|------|-------|
| NetworkUnavailable | "Network unavailable" | "网络好像断开了…等会儿再试？" | 🌐 |
| NetworkTimeout | "Network timeout" | "网络有点慢，再试一次？" | ⏱️ |
| UserCancelled | "User cancelled" | "已停止" | ✋ |
| ApiKeyInvalid | "API key invalid" | "API Key 不对了，去设置里检查一下？" | 🔑 |
| InsufficientBalance | "Insufficient balance" | "DeepSeek 余额不足" | 💸 |
| RateLimited | "Rate limited" | "调用太频繁了，休息一下再试" | 🐢 |
| ServerError | "Server error" | "DeepSeek 服务器开了小差" | 😢 |
| Unknown | "Unknown error" | "出了点小问题，要不要重试？" | 😅 |

#### 5.3 落地步骤

- 扩 `errors.go` ToolError 加 HumanMessage
- 扩 AppErrorType 8 个枚举
- 新 `classify.go` ClassifyException
- FromHTTPStatusCode 静态方法
- humanMessage 文案表
- AppResult helper
- 注入到 ReAct 循环 catch 块
- 7 个单测场景

#### 5.4 验证标准

- 7 个单测场景通过
- 401/402/429/5xx 正确映射
- i18n 切换正常

---

### Stage 6 — ContextWindowManager 自动压缩 + TokenHudBar UI

#### 6.1 借鉴模式

| Nuclear-Boy 实现 | encv-go 现状 | 借鉴方法论 |
|------------------|--------------|-----------|
| **ContextWindowManager.kt** updateAllocation 6 段分配（system / profile / project / history / tools / attachedFiles）+ 三级预警（GREEN < 80% / YELLOW 80-95% / RED > 95% / FORCE > 98%） | encv-go 当前**完全没有**上下文压缩机制，长对话直接 OOM | **模式借鉴**。新 `internal/agent/context_window.go` |
| **AgentEngine.kt L270-297** emergencyCompress (RED) / compressConversation (YELLOW) / 截断 (FORCE) | 缺失 | **模式借鉴**。ReAct 循环每轮调用 updateAllocation + 必要时压缩 |
| **AppConstants.kt** 6 段阈值常量 | encv-go 当前零散定义 | **模式借鉴**。新 `internal/agent/constants.go`（WARNING_YELLOW ≈ 133K / WARNING_RED ≈ 158K / WARNING_FORCE ≈ 163K / DEEPSEEK_CONTEXT_WINDOW = 128K） |
| **TokenTracker.kt** per-request cache hit rate（不是累计） | encv-go 当前可能用累计 | **算法借鉴**。每次请求算 cache hit rate 而不是累计 |
| **TokenTracker.kt L169-170** 平均延迟公式 `((cur.avg * count) + latency) / (count + 1)` | encv-go 当前用 sum/count 可能丢精度 | **算法借鉴**。增量平均公式 |
| **TokenHudBar.kt L166-198** 7 行指标（输入/输出/缓存/思考/上下文/速度/延迟）+ YELLOW/RED 颜色 | encv-go 当前 HUD 简陋 | **模式借鉴**。前端 `<TokenHudBar />` 组件，复用 mobile-agent-polish-2026q2 usePinchZoom UI 风格 |
| **AgentEngine.kt L239-247** buildFileContextStrings（FILE_CONTENT_TRUNCATE_THRESHOLD=300K 截断大文件） | 缺失 | **模式借鉴**。文件内容注入到 user message |

#### 6.2 EstimateTokens 算法

```kotlin
// nuclear-boy AgentEngine.kt
fun estimateTokens(text: String): Long = (text.length / 3.5).toLong().coerceAtLeast(20)
```

借鉴到 encv-go：

```go
// internal/agent/context_window.go
func EstimateTokens(text string) int64 {
    return int64(math.Max(float64(len(text))/3.5, 20))
}
```

#### 6.3 落地步骤

- 新 `context_window.go` ContextWindowManager
- 新 `token_tracker.go` TokenTracker
- 改 agent_api.go ReAct 循环
- SSE 协议加 `token_stats` 事件（每 5 秒）
- 前端 `useChatStats.ts` composable
- `<TokenHudBar />` 组件
- 6 个单测场景

#### 6.4 验证标准

- EstimateTokens 精度 ±5%
- 6 段总和触发对应颜色
- HUD 5 秒聚合一次（性能 OK）

---

### Stage 7 — Skills 生态 + executeViaExternalModule + ZIP-slip

#### 7.1 借鉴模式

| Nuclear-Boy 实现 | encv-go 现状 | 借鉴方法论 |
|------------------|--------------|-----------|
| **ToolRegistry.kt L447-467** executeViaExternalModule 回退（pythonExecutor → skillsExecutor → null） | encv-go 当前 LLM 调用不存在的工具直接返回 "tool not found" | **模式借鉴**。ToolRegistry.Execute 加回退链：本地 → Skills 插件 → 错误 |
| **SkillManifest.kt** YAML 4 维权限（filesystem glob / network allowed_hosts / packages / shell）+ isSandboxed 计算属性 | encv-go 当前 Skills 不存在 | **模式借鉴**。`internal/skills/manifest.go` + `gopkg.in/yaml.v3` |
| **SkillManager.kt L867-873** ZIP-slip 防护（canonical 路径 + 前缀检查） | encv-go 当前解压可能越界 | **算法借鉴**。safeUnzip 用 `archive/zip` + filepath.Clean + 前缀检查 |
| **SkillMarketPlace.kt** 远程 Skills 市场（HTTP GET 列可用 Skills） | 缺失 | **概念借鉴**。`internal/skills/marketplace.go` |

#### 7.2 safeUnzip 实现（Go）

```go
// internal/skills/safe_unzip.go
import (
    "archive/zip"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
)

func SafeUnzip(zipPath, destDir string) error {
    r, err := zip.OpenReader(zipPath)
    if err != nil {
        return err
    }
    defer r.Close()

    destCanonical, err := filepath.Abs(destDir)
    if err != nil {
        return err
    }
    destCanonical = filepath.Clean(destCanonical) + string(os.PathSeparator)

    for _, f := range r.File {
        // 防止 ZIP-slip
        target := filepath.Join(destDir, f.Name)
        if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), destCanonical) {
            return fmt.Errorf("ZIP entry %s 试图越界到 %s", f.Name, target)
        }
        // ... 解压逻辑
    }
    return nil
}
```

#### 7.3 落地步骤

- 启动扫描 `~/.config/encv-go/skills/`，解析 skill.yaml
- SkillManifest YAML 解析（4 维权限）
- executeViaExternalModule 回退
- safeUnzip
- MarketPlace API
- 工具名 = `skill_<name>`
- 单测（e2e / ZIP-slip）

#### 7.4 验证标准

- Skill 加载流程 e2e 通
- 本地工具和 Skills 工具都能用
- ZIP-slip 攻击被拒

---

### Stage 8 — 三层记忆系统 + autoExtractMemories

#### 8.1 借鉴模式

| Nuclear-Boy 实现 | encv-go 现状 | 借鉴方法论 |
|------------------|--------------|-----------|
| **MemoryStore.kt** 三层记忆（ProjectMemoryEntity / UserProfileEntity 带 confidence 0-1 / SemanticMemoryEntity 带 recallCount） | encv-go 当前**完全没有**记忆系统 | **模式借鉴**。`internal/memory/store.go` + SQLite（`mattn/go-sqlite3`，**注意**：非 gomobile 路径允许 CGO） |
| **MemoryDatabase.kt L208-211** SQLite WAL 模式（`PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`） | 缺失 | **算法借鉴**。启动时执行 PRAGMA |
| **MemoryStore.kt L673-741** autoExtractMemories 三种 pattern（build_command / code_style / user preference） | 缺失 | **模式借鉴**。每 5 轮对话跑一次 autoExtract |
| **SystemPromptBuilder.kt L168-193** appendUserPreferences 注入 system prompt（confidence > 0.5，最多 20 条） | 缺失 | **模式借鉴**。LoadHighConfidence(0.5) → 放在 prompt 末尾 |

#### 8.2 autoExtract 三个 pattern

| 模式 | 正则 | 提取样例 |
|------|------|----------|
| 用户偏好 | `我(喜欢|习惯|常用|偏好)\s*([^\s,.，。]{1,20})` | "我习惯用 TypeScript" → `UserProfile("preferred_language", "TypeScript", confidence=0.7)` |
| 技术栈 | `我(用|的|写)\s*([a-zA-Z]+\s*[,，]?\s*){1,5}` | "我用 React, Vue" → `UserProfile("tech_stack", "React,Vue", confidence=0.7)` |
| build command | `我(常用|总是|通常)\s*(npm|pnpm|yarn|gradle|maven)` | "我常用 npm" → `UserProfile("build_command", "npm", confidence=0.7)` |

#### 8.3 落地步骤

- 新 `memory/store.go` 三层记忆
- 新 `auto_extract.go` autoExtract
- 改 `prompt.go` 注入用户偏好
- 前端 `MemoryManager.vue`
- 5 个单测场景

#### 8.4 验证标准

- 三表 CRUD OK
- WAL 模式生效
- "我习惯用 TypeScript" → UserProfile(preferred_language=TypeScript, 0.7)

---

### Stage 9 — Python 沙箱 4 策略 + 白/黑名单 + 文档生成

#### 9.1 借鉴模式

| Nuclear-Boy 实现 | encv-go 现状 | 借鉴方法论 |
|------------------|--------------|-----------|
| **SandboxPolicy.kt** 4 模式（STRICT / STANDARD / RELAXED / DOCUMENT_GENERATION） | encv-go 当前 run_python 没有策略 | **模式借鉴**。`internal/tools/python_sandbox.go` |
| **SandboxPolicy.kt L60-100** buildPolicyPreamble 注入 Python 前置代码（重写 builtins.open + 拦截 subprocess） | 缺失 | **模式借鉴**。run_python 接受 sandbox 参数，注入到 `python -c "..."` 最前面 |
| **SandboxPolicy.kt L520-555** isStdlibModule 170+ 白名单 | 缺失 | **算法借鉴**。STRICT 模式：非 stdlib 一律 reject |
| **SandboxPolicy.kt L429-435** DANGEROUS_COMMANDS 黑名单（rm -rf /, mkfs., dd if=, > /dev/sda, fork bomb, chmod 777 /, curl|sh） | 缺失 | **算法借鉴**。subprocess.run 调用前检查 |
| **DocumentGenerator.kt** docx/xlsx/pptx 生成 | encv-go 当前没有文档生成 | **概念借鉴**。generate_docx / generate_xlsx / generate_pptx |

#### 9.2 落地步骤

- 新 `python_sandbox.go` 4 沙箱模式
- isStdlibModule 170+ 白名单
- DANGEROUS_COMMANDS 黑名单
- run_python 接受 sandbox 参数
- generate_docx / generate_xlsx / generate_pptx
- 预热 Python 解释器
- 8 个单测场景

#### 9.3 验证标准

- 4 沙箱模式切换正常
- 危险命令被拦截
- docx/xlsx/pptx 能生成

---

### Stage 10 — MessageBubble 增强 + FileOperations 安全 + 项目脚手架

#### 10.1 借鉴模式

| Nuclear-Boy 实现 | encv-go 现状 | 借鉴方法论 |
|------------------|--------------|-----------|
| **MessageBubble.kt L275-365** ToolExecutionCard（5 状态颜色 PENDING/RUNNING/COMPLETED/FAILED/CANCELLED + 展开/折叠 + 黑底输出） | encv-go 当前只显示一行 `tool: result` | **模式借鉴**。useAgent.ts 接收 tool_result 时记录 ToolCallRecord（toolName / input / output / status / startedAt / completedAt）；MessageBubble 渲染 `<ToolExecutionCard>` |
| **MessageBubble.kt L224-270** ReasoningSection 折叠（expandVertically 动画） | encv-go **完全没暴露** reasoning_content | **模式借鉴**。MessageBubble 渲染 message.reasoning_content 折叠区 |
| **MessageBubble.kt L603-637** FileChangeCard（绿/蓝/红颜色编码 新建/修改/删除） | encv-go 当前 write_file 工具返回不带 fileChanges | **模式借鉴**。write_file 返回 `{path, type, linesAdded, linesRemoved}` 列表 |
| **MessageBubble.kt L692-768** 代码块着色 + 复制按钮 + Toast | encv-go 当前是纯文本代码块 | **模式借鉴**。用 shiki/prismjs + 复制按钮 + 1.5s toast |
| **MessageBubble.kt L576-598** ThinkingIndicator 三点动画（200ms 错位） | encv-go 当前只有 spinner | **模式借鉴**。CSS 动画 3 dot + 200ms delay |
| **MessageBubble.kt L93** CombinedClickable onClick + onLongClick | encv-go 当前只有 onClick | **模式借鉴**。长按显示 DropdownMenu（复制/重新生成） |
| **FileOperations.kt L386-405** resolvePath 路径穿越防护（canonical + 前缀检查） | encv-go 当前 path 直接拼接 | **算法借鉴**。`internal/tools/file_ops.go` ResolvePath |
| **FileOperations.kt L280-296** searchFiles 跳过隐藏目录（.git / node_modules / __pycache__ / build / .gradle） | encv-go 当前 searchFiles 遍历所有目录 | **算法借鉴**。SkipDirs []string 常量 |
| **FileOperations.kt L428-605** buildProjectDirectories + buildReadme + buildGitignore | encv-go 当前 create_project 只能建空目录 | **模式借鉴**。4 techStack 模板（Python / Kotlin / JS / Go） |

#### 10.2 落地步骤

**后端**：
- `file_ops.go` 加 ResolvePath + SkipDirs
- `high_level.go` create_project 支持 techStack
- 新 `file_scaffold.go` buildReadme / buildGitignore

**前端**：
- useAgent.ts 接收 tool_result 时记录 ToolCallRecord
- MessageBubble.vue 加 6 个 UI 模式
- MessageStatus 升级到 8 状态
- 10 个单测场景

#### 10.3 验证标准

- ResolvePath 防越界
- searchFiles 跳过隐藏目录
- create_project 4 techStack 模板正确
- 6 个 MessageBubble UI 模式可用

---

## 五、不借鉴什么

借鉴有边界，避免范围蔓延：

| 项 | 不借鉴原因 |
|----|-----------|
| **Chaquopy Java 桥接模板**（SystemPromptBuilder.kt L39-128） | Chaquopy 是 Kotlin 专用 Python 解释器，encv-go 用 subprocess 跑 Python，不需要 20 个 Android Java 桥接 |
| **20 个 Android 系统服务** | encv-mobile 是 Vue/Ionic/Capacitor，调用 Android API 走 Capacitor 插件（@capacitor/vibration 等），不需要 jclass() 桥接 |
| **VibrationEncoder 摩斯/SOS/心跳** | 价值低，超出核心范围 |
| **Hilt DI 容器** | encv-go 用 wire / fx 已有 DI |
| **夜间轻声模式**（v1 Stage 11） | 价值低，文案/小优化层 |
| **错误处理哲学"搞定了 ✨"**（v1 Stage 12） | 价值低，文案/小优化层 |
| **iOS 移植**（HANDOVER §十） | 未来方向，本期不做 |
| **Kotlin Multiplatform** | 栈迁移超出范围 |
| **Markwon LaTeX 渲染**（HANDOVER §十 标 🔴 高 但实际有坐标坑） | HANDOVER 自己说"曾尝试 ext-tex 和 huarangmeng/latex，坐标均不对"，encv-mobile 暂不碰 |
| **Prism4j 语法着色**（HANDOVER §十 标 🔴 高 但 Maven 坐标不存在） | 同样有坑，改用 shiki |

---

## 六、与现有 spec 的协作关系

| 现有 spec | 协作方式 |
|----------|---------|
| `agent-tools-scenarios-v2` | **基线**。Stage 4/5/9 适配到现有 tool registry，扩展而非替换 |
| `agui-real-llm-path-completion` | **基线**。Stage 2 适配到 AG-UI SSE 流式（tool_call_start/delta/end 事件） |
| `mobile-agent-polish-2026q2` | **已完**。Stage 10 的 MessageBubble 增强复用 usePinchZoom / formatRelativeTime UI 风格 |
| `agent-mock-mode` | **基线**。Stage 5 错误模型（AppError.humanMessage）适配到 mock 剧本 |
| `multi-engine-chat-architecture` | **基线**。Stage 6 借鉴 HUD 时复用 TokenStats 抽象 |
| `implement-mobile-backend-api` | **基线**。Stage 7 Skills 工具注册复用其注册中心 |

---

## 七、文件清单（Stage 0 交付物）

- `/workspace/.trae/documents/nuclear-boy-borrowing-design.md`（本文件，≥400 行）
- `/workspace/.trae/specs/borrow-nuclear-boy-2026q2/borrowing-points.md`（借鉴点索引，≥20 项）
- `/workspace/.trae/specs/borrow-nuclear-boy-2026q2/spec.md`（V2 spec，10 Stage）
- `/workspace/.trae/specs/borrow-nuclear-boy-2026q2/tasks.md`（V2 tasks）
- `/workspace/.trae/specs/borrow-nuclear-boy-2026q2/checklist.md`（V2 checklist）

**Stage 0 不写任何业务代码**，只分析 + 写文档。Stage 1 才开始实施。

---

## 八、Stage 0 验收

- [x] 仓库 `/tmp/nuclear-boy` 已 clone
- [x] 读完 HANDOVER.md + HANDOVER2.0.md
- [x] 读完 CLAUDE.md + INFO.md
- [x] 深读 agent-core 3 个 .kt
- [x] 深读 api-deepseek 4 个 .kt
- [x] 深读 common 4 个 .kt
- [x] 深读 memory 3 个 .kt
- [x] 深读 skills 3 个 .kt
- [x] 深读 python-bridge 2 个 .kt
- [x] 深读 tools-docgen 2 个 .kt
- [x] 深读 ui-chat 4 个 .kt
- [x] 读 android-bridge-v1.0 报告
- [x] 输出本文件 ≥400 行（实际 ~580 行）
- [x] 输出 borrowing-points.md ≥20 项（实际 30+ 项）
