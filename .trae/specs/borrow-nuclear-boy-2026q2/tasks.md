# Tasks (Borrow Nuclear-Boy 2026Q2 — 多阶段借鉴 V2)

> **v2 变更**：删除 v1 的 Stage 11+12，新增 9 个高价值借鉴点到 Stage 1-10。
> **总览**：10 个独立可交付 Stage，每个 Stage 都有「借鉴模式 + 实施步骤 + 单测 + 验证」闭环。
> **Stage 0 必须最先做**（无它，后续 Stage 借鉴什么模糊）。

---

## Stage 0: 仓库深读 + 借鉴点设计文档

> **目标**：把 nuclear-boy 仓库 14 个核心 .kt + 2 份 HANDOVER + 1 份 android-bridge-report 吃透，输出 ≥400 行设计文档。

- [ ] Task 0.1: 仓库已克隆到 `/tmp/nuclear-boy`（已完成）
- [ ] Task 0.2: 读 HANDOVER.md + HANDOVER2.0.md 完整内容（含 §十未来优化）
- [ ] Task 0.3: 深读 agent-core 3 个文件（AgentEngine.kt 877 行 / SystemPromptBuilder.kt 194 行 / ToolRegistry.kt 478 行）
- [ ] Task 0.4: 深读 api-deepseek 4 个文件（DeepSeekApiClient / ContextWindowManager / TokenTracker / ModelRouter）
- [ ] Task 0.5: 深读 common 3 个文件（AppConstants / AppError / Models / Extensions）
- [ ] Task 0.6: 深读 memory 3 个文件（MemoryDao / MemoryDatabase / MemoryStore）
- [ ] Task 0.7: 深读 skills 3 个文件（SkillManager / SkillManifest / SkillMarketPlace）
- [ ] Task 0.8: 深读 python-bridge 2 个文件（PythonSandbox / SandboxPolicy）
- [ ] Task 0.9: 深读 tools-docgen 2 个文件（FileOperations / DocumentGenerator）
- [ ] Task 0.10: 深读 ui-chat 4 个文件（ChatScreen / ChatViewModel / TokenHudBar / MessageBubble 800+ 行）
- [ ] Task 0.11: 读 android-bridge-v1.0 报告（20 个系统服务 + 工具调用黄金法则）
- [ ] Task 0.12: 输出 `/workspace/.trae/documents/nuclear-boy-borrowing-design.md`（≥400 行）
  - 每个借鉴点列出 3 列映射表（N-B 实现 / encv 现状 / 借鉴方法论）
  - 14 个文件 + 9 个 v2 新增借鉴点（见 spec §v1→v2 变更清单）
- [ ] Task 0.13: 输出 `/workspace/.trae/specs/borrow-nuclear-boy-2026q2/borrowing-points.md`（借鉴点索引，≥20 项）

**Stage 0 验收**：
- [ ] 文档 ≥400 行
- [ ] 14 个 .kt 文件深读全部完成
- [ ] 借鉴点索引 ≥20 个

---

## Stage 1: System Prompt 工程化 + PROACTIVE 主动智能

> **目标**：800 字精简哲学（正面示例 > 规则 / 避免否定 / 工具描述即文档）+ PROACTIVE 主动智能哲学 落到 encv-go。

- [ ] Task 1.1: 创建 `/workspace/internal/agent/prompt.go`（SystemPromptBuilder）
- [ ] Task 1.2: 实现 Build() 方法遵循 5 大原则（来自 HANDOVER2.0.md §五）
  - 工具描述 > prompt
  - 避免否定表述
  - 正面示例 > 规则
  - 精简至上（≤1500 字）
  - DeepSeek thinking 显式 disabled
- [ ] Task 1.3: **PROACTIVE 主动智能段**（v2 新增，参考 SystemPromptBuilder.kt L142-148）
  - 触发条件：用户连续发 3+ 消息 / 完成 1 个工具 / 收到工具失败
  - 行为：自动追加 2-3 条 `建议:`
- [ ] Task 1.4: 动态内容后置（用户偏好 / 项目上下文 / Skills 列表 / PROACTIVE）
- [ ] Task 1.5: 显式传 `{"thinking": {"type": "disabled"}}`（DeepSeek 默认 enabled 是坑）
- [ ] Task 1.6: 创建 `/workspace/internal/agent/prompt_test.go` 单测
- [ ] Task 1.7: 验证 `go build ./cmd/encv` 0 错误

**Stage 1 验收**：
- [ ] 5 大原则单测全过
- [ ] PROACTIVE 段已注入（手动 grep "## 主动智能" 命中）
- [ ] 实际生成 prompt 长度 < 1500 字

---

## Stage 2: ToolCallAccumulator + scopeJob 重建 + maxToolIterations

> **目标**：前端流式累积 + 取消后协程不死亡 + 显式 ReAct 上限。

- [ ] Task 2.1: 创建 `/workspace/app/encv-mobile/src/composables/useToolCallAccumulator.ts`
  - 状态：`pending` / `accumulating` / `complete` / `executed`
  - `clear()` 在 ReAct 循环开始时调用（不在 tool_call_start 时清 — nuclear-boy 实战踩坑）
- [ ] Task 2.2: 处理 tool_call_start / delta / end 三种事件
- [ ] Task 2.3: 集成到 useAgent.ts send() / confirmTool() 流程
- [ ] Task 2.4: **scopeJob 重建模式**（v2 新增，参考 AgentEngine.kt L850-854）
  - 后端 `internal/agent/agent_api.go` cancel() 关闭旧 ctx + 创建新 ctx
  - 验证：cancel() 后 100ms 内发新请求能起协程
- [ ] Task 2.5: **maxToolIterations = 20**（v2 新增）
  - ReAct 主循环里 `if iteration >= 20 { break; warnUser() }`
- [ ] Task 2.6: 单测覆盖（6 场景）
- [ ] Task 2.7: 验证 `npx vue-tsc --noEmit` + `go build` 双 0 错误

**Stage 2 验收**：
- [ ] 6 个单测场景通过
- [ ] 与 useAgent.send() 集成无破坏

---

## Stage 3: buildHistoryMessages 防 400 + reasoningContent + 8 状态机

> **目标**：解决 400 insufficient tool messages + 剥离 reasoningContent + 升级 message status。

- [ ] Task 3.1: 后端 `/workspace/internal/server/agent_api.go` buildHistoryMessages
  - 按 toolCallId 去重
  - completedCalls 过滤（output != null && toolCallId != null）
  - completedCalls 为空 → toolCalls=null（防 400）
- [ ] Task 3.2: **reasoningContent 处理**（v2 新增）
  - 旧消息的 reasoning_content **不入 history**（防 token 浪费 + 400）
  - 最新一条 assistant 的 reasoning_content 可保留（让前端折叠展示）
- [ ] Task 3.3: 前端 useAgent.ts 处理 tool_result 按 toolCallId 去重
- [ ] Task 3.4: **MessageStatus 8 状态机**（v2 新增，参考 Models.kt L57-66）
  - SENDING / SENT / THINKING / STREAMING / EXECUTING / COMPLETE / ERROR / CANCELLED
  - 改 TypeScript 联合类型 / Go iota
- [ ] Task 3.5: 单测（5 场景）+ 验证 `go build` + `vue-tsc` 双 0 错误

**Stage 3 验收**：
- [ ] 5 个单测场景通过
- [ ] 集成测试：中断对话后 LLM 不再 400

---

## Stage 4: 参数别名 + Tool priority + executeSafe paramHint

> **目标**：path/filePath 互通 + 防 LLM 截断 + 错误时附示例帮 LLM 自纠。

- [ ] Task 4.1: 后端 tool registry 加 `ParamAliases map[string][]string` 字段
- [ ] Task 4.2: 执行前 normalize（别名 → 主参数）
- [ ] Task 4.3: 保留 nuclear-boy `parseToolParams` 容错（JSON 解析失败 fallback emptyMap）
- [ ] Task 4.4: **Tool priority 排序**（v2 新增，参考 ToolRegistry.kt L168-196）
  - priorityTools 集合（run_python / read_file / write_file / list_directory）置顶 0
  - requiresConfirmation 工具放最后 2
  - 其他 1
- [ ] Task 4.5: **executeSafe paramHint**（v2 新增，参考 ToolRegistry.kt L236-258）
  - 工具执行失败时错误信息含：工具名 + 缺失参数名 + 类型 + 完整参数示例
- [ ] Task 4.6: 单测覆盖（参数别名 / priority 排序 / paramHint）
- [ ] Task 4.7: 验证 `go test` + `vue-tsc` 通过

**Stage 4 验收**：
- [ ] 5 工具参数别名互通
- [ ] priority 工具在 token 预算紧张时仍在前 N
- [ ] LLM 漏传参数时错误信息含示例

---

## Stage 5: AppResult + AppError + classifyException + fromHttpCode

> **目标**：错误四件套（Result 类型 + 本地化消息 + 异常分类 + HTTP 状态码映射）。

- [ ] Task 5.1: 扩 `/workspace/internal/tools/errors.go` `ToolError` 加 `HumanMessage string`
- [ ] Task 5.2: 扩 `AppErrorType` 枚举（NetworkUnavailable / NetworkTimeout / UserCancelled / ApiKeyInvalid / InsufficientBalance / RateLimited / ServerError / Unknown）
- [ ] Task 5.3: **classifyException** 函数（v2 新增，参考 AgentEngine.kt L803-821）
  - 新 `/workspace/internal/agent/classify.go`
  - 用 errors.As 区分 url.Error / net.OpError / context.DeadlineExceeded / context.Canceled
- [ ] Task 5.4: **fromHttpCode 静态方法**（v2 新增）
  - 401 → ApiKeyInvalid / 402 → InsufficientBalance / 429 → RateLimited / 5xx → ServerError
- [ ] Task 5.5: **humanMessage 文案表**（8 个 AppErrorType × 中英文）
- [ ] Task 5.6: AppResult helper（Go tuple 模式 `Result[T]` + TypeScript 联合类型 + RunCatching 自动捕获 panic）
- [ ] Task 5.7: 注入到 ReAct 循环的 catch 块
- [ ] Task 5.8: 单测覆盖（7 场景）+ 验证 `go test` 通过

**Stage 5 验收**：
- [ ] 7 个单测场景通过
- [ ] 401/402/429/5xx 正确映射
- [ ] i18n 切换正常

---

## Stage 6: ContextWindowManager 自动压缩 + TokenHudBar UI

> **目标**：3 级预警（YELLOW/RED/FORCE）+ 6 段 token 分配 + 7 行 HUD 指标。

- [ ] Task 6.1: 新 `/workspace/internal/agent/context_window.go` 实现 ContextWindowManager
  - EstimateTokens(text string) int64 = int64(len(text) / 3.5)
  - updateAllocation(parts) → AllocationResult
  - emergencyCompress / compressConversation / 截断
- [ ] Task 6.2: 改 agent_api.go ReAct 循环，每轮调用 updateAllocation + 必要时压缩
  - 阈值常量：WARNING_YELLOW ≈ 133K / WARNING_RED ≈ 158K / WARNING_FORCE ≈ 163K
- [ ] Task 6.3: 新 `/workspace/internal/agent/token_tracker.go` 实现 TokenTracker
  - per-request cache hit rate（不是累计）
  - 平均延迟 `((cur.avg * count) + latency) / (count + 1)`
- [ ] Task 6.4: SSE 协议加 `token_stats` 事件类型（每 5 秒一次）
- [ ] Task 6.5: 前端 `/workspace/app/encv-mobile/src/composables/useChatStats.ts`
- [ ] Task 6.6: 新组件 `<TokenHudBar />` 7 行指标（输入/输出/缓存/思考/上下文/速度/延迟）
  - 复用 mobile-agent-polish-2026q2 usePinchZoom UI 风格
  - 颜色：GREEN < 80% / YELLOW 80-95% / RED > 95%
- [ ] Task 6.7: 单测（6 场景）+ 验证 `go build` + `vue-tsc` + `vitest`

**Stage 6 验收**：
- [ ] EstimateTokens 精度 ±5%
- [ ] 6 段总和触发对应颜色
- [ ] HUD 5 秒聚合一次（性能 OK）

---

## Stage 7: Skills 生态 + executeViaExternalModule + ZIP-slip

> **目标**：skill.yaml 注册为工具 + 本地查不到回退到 Skills + 解压防越界。

- [ ] Task 7.1: 加载逻辑（启动扫描 `~/.config/encv-go/skills/`，解析 skill.yaml）
- [ ] Task 7.2: SkillManifest YAML 解析（4 维权限：filesystem/network/packages/shell）
  - 用 gopkg.in/yaml.v3
  - isSandboxed 计算属性
  - 参数验证（int/float/bool/choice/string）
- [ ] Task 7.3: **executeViaExternalModule 回退机制**（v2 新增，参考 ToolRegistry.kt L447-467）
  - ToolRegistry.Execute 加回退链：本地 → Skills 插件 → 错误
  - 错误信息告诉 LLM 用了哪个回退路径
- [ ] Task 7.4: **ZIP-slip 防护**（v2 新增，参考 SkillManager.kt L867-873）
  - safeUnzip：filepath.Clean + 前缀检查
  - 任何 os.Create 前必须做
- [ ] Task 7.5: MarketPlace API（HTTP GET 列可用 Skills）
- [ ] Task 7.6: 工具名 = `skill_<name>` 避免冲突
- [ ] Task 7.7: 单测 + 验证 `go test`

**Stage 7 验收**：
- [ ] Skill 加载流程 e2e 通
- [ ] 本地工具和 Skills 工具都能用
- [ ] ZIP-slip 攻击被拒

---

## Stage 8: 三层记忆 + autoExtract

> **目标**：项目级 / 用户 / 语义 三层记忆 + 自动从对话学习。

- [ ] Task 8.1: 新 `/workspace/internal/memory/store.go` 三层记忆（SQLite + WAL 模式）
  - ProjectMemoryEntity / UserProfileEntity / SemanticMemoryEntity
  - `PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`
- [ ] Task 8.2: 新 `/workspace/internal/memory/auto_extract.go` autoExtract
  - 三个 pattern regex：
    - `我(喜欢|习惯|常用|偏好)\s*([^\s,.，。]{1,20})`
    - `我(用|的|写)\s*([a-zA-Z]+\s*[,，]?\s*){1,5}`
    - `我(常用|总是|通常)\s*(npm|pnpm|yarn|gradle|maven)`
  - confidence: 显式 1.0 / 推断 0.7
- [ ] Task 8.3: 改 `/workspace/internal/agent/prompt.go` 注入用户偏好
  - LoadHighConfidence(0.5) → 最多 20 条 → 放在 prompt 末尾
- [ ] Task 8.4: 前端 `/workspace/app/encv-mobile/src/views/Settings/MemoryManager.vue`
  - 查看 / 编辑 / 清除 记忆
- [ ] Task 8.5: 单测（5 场景）+ 验证

**Stage 8 验收**：
- [ ] 三表 CRUD OK
- [ ] WAL 模式生效
- [ ] "我习惯用 TypeScript" → UserProfile(preferred_language=TypeScript, 0.7)

---

## Stage 9: Python 沙箱 4 策略 + isStdlibModule + 危险黑名单 + 文档生成

> **目标**：4 沙箱模式 + 170+ stdlib 白名单 + 危险命令黑名单 + docx/xlsx/pptx 生成。

- [ ] Task 9.1: 新 `/workspace/internal/tools/python_sandbox.go` 沙箱实现
  - 4 模式：STRICT / STANDARD / RELAXED / DOCUMENT_GENERATION
  - buildPolicyPreamble 注入 Python 前置代码（重写 builtins.open + 拦截 subprocess）
- [ ] Task 9.2: **isStdlibModule 170+ 白名单**（v2 新增，参考 SandboxPolicy.kt L520-555）
  - 维护 STDLIB_MODULES map
  - STRICT 模式：非 stdlib 一律 reject
- [ ] Task 9.3: **DANGEROUS_COMMANDS 黑名单**（v2 新增，参考 SandboxPolicy.kt L429-435）
  - rm -rf /, mkfs., dd if=, > /dev/sda, fork bomb, chmod 777 /, curl | sh
  - subprocess.run 调用前检查
- [ ] Task 9.4: 工具 `run_python` 接受 `sandbox` 参数（默认 STANDARD）
- [ ] Task 9.5: 文档生成工具（generate_docx / generate_xlsx / generate_pptx）
  - subprocess 跑 Python
  - 预装 python-docx / openpyxl / python-pptx
- [ ] Task 9.6: 预热 Python 解释器（cold start 500ms → warm 50ms）
- [ ] Task 9.7: 单测（8 场景）+ 验证

**Stage 9 验收**：
- [ ] 4 沙箱模式切换正常
- [ ] 危险命令被拦截
- [ ] docx/xlsx/pptx 能生成

---

## Stage 10: MessageBubble 增强 + FileOperations 安全 + 项目脚手架

> **目标**：6 个 UI 模式 + 路径安全 + 隐藏目录跳过 + 项目模板生成。

### 后端
- [ ] Task 10.1: 改 `/workspace/internal/tools/file_ops.go` 加 ResolvePath
  - filepath.Clean + canonical + 前缀检查
  - 任何 read/write/list 前必须 ResolvePath
- [ ] Task 10.2: 加 SkipDirs 常量（.git / .agent / node_modules / __pycache__ / build / .gradle）
- [ ] Task 10.3: 改 `/workspace/internal/tools/high_level.go` create_project 支持 techStack
- [ ] Task 10.4: 新 `/workspace/internal/tools/file_scaffold.go`
  - buildProjectDirectories（Python/Kotlin/JS/Go 4 模板）
  - buildReadme / buildGitignore

### 前端
- [ ] Task 10.5: 改 useAgent.ts 接收 tool_result 时记录 ToolCallRecord
  - 字段：toolName / input / output / status / startedAt / completedAt
- [ ] Task 10.6: 改 MessageBubble.vue 加 6 个 UI 模式
  - ToolExecutionCard（5 状态颜色 + 展开/折叠 + 黑底输出）
  - ReasoningSection（折叠 + expandVertically 动画）
  - FileChangeCard（绿/蓝/红颜色编码）
  - CodeBlock（shiki/prismjs + 复制按钮 + toast）
  - ThinkingIndicator（3 点错位 200ms 动画）
  - CombinedClickable onClick + onLongClick
- [ ] Task 10.7: 升级 MessageStatus 到 8 状态（与 Stage 3 联动）
- [ ] Task 10.8: 单测（10 场景）+ 验证 `go build` + `vue-tsc` + `vitest`

**Stage 10 验收**：
- [ ] ResolvePath 防越界
- [ ] searchFiles 跳过隐藏目录
- [ ] create_project 4 techStack 模板正确
- [ ] 6 个 MessageBubble UI 模式可用

---

## 整体验收

所有 Stage 完成后：

- [ ] **后端**：`cd /workspace && go build ./...` 0 错误
- [ ] **后端**：`cd /workspace && go test ./...` 100% 通过（v2 期望 100+ 测试）
- [ ] **后端**：`cd /workspace && go vet ./...` 0 警告
- [ ] **前端**：`cd /workspace/app/encv-mobile && pnpm run type-check` 0 错误
- [ ] **前端**：`cd /workspace/app/encv-mobile && pnpm run test:unit` 全部通过
- [ ] **前端**：`cd /workspace/app/encv-mobile && pnpm run build` 成功
- [ ] **集成**：`cd /workspace && ./scripts/test-e2e.sh` 全绿
- [ ] **手动**（安卓真机）：见每个 Stage 的 "Scenario: 手动验证"
