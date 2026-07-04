# Checklist (Borrow Nuclear-Boy 2026Q2 — 多阶段借鉴 V2)

> **总览**：10 个 Stage 的实施 checklist。V2 整合 9 个 v1 漏掉的高价值借鉴点。

---

## Stage 0 — 仓库深读

- [ ] 仓库 `/tmp/nuclear-boy` 已 clone
- [ ] 读完 HANDOVER.md + HANDOVER2.0.md
- [ ] 读完 CLAUDE.md + INFO.md
- [ ] 深读 agent-core 3 个 .kt（AgentEngine / SystemPromptBuilder / ToolRegistry）
- [ ] 深读 api-deepseek 4 个 .kt（DeepSeekApiClient / ContextWindowManager / TokenTracker / ModelRouter）
- [ ] 深读 common 4 个 .kt（AppConstants / AppError / Models / Extensions）
- [ ] 深读 memory 3 个 .kt（MemoryDao / MemoryDatabase / MemoryStore）
- [ ] 深读 skills 3 个 .kt（SkillManager / SkillManifest / SkillMarketPlace）
- [ ] 深读 python-bridge 2 个 .kt（PythonSandbox / SandboxPolicy）
- [ ] 深读 tools-docgen 2 个 .kt（FileOperations / DocumentGenerator）
- [ ] 深读 ui-chat 4 个 .kt（ChatScreen / ChatViewModel / TokenHudBar / MessageBubble）
- [ ] 读 android-bridge-v1.0 报告
- [ ] 输出 `/workspace/.trae/documents/nuclear-boy-borrowing-design.md` ≥400 行
- [ ] 输出 `/workspace/.trae/specs/borrow-nuclear-boy-2026q2/borrowing-points.md` ≥20 项

---

## Stage 1 — System Prompt 工程化 + PROACTIVE

- [ ] `/workspace/internal/agent/prompt.go` 创建
- [ ] 5 大原则（工具描述 > prompt / 避免否定 / 正面示例 / 精简 / thinking disabled）实现
- [ ] **PROACTIVE 段**注入（v2 新增）
- [ ] 动态内容（用户偏好 / 项目 / Skills / PROACTIVE）放在末尾
- [ ] 显式传 `{"thinking": {"type": "disabled"}}`
- [ ] prompt_test.go 单测：否定词报错 / 工具缺失报错 / 长度警告
- [ ] `go build ./cmd/encv` 0 错误
- [ ] `go test ./internal/agent` 通过
- [ ] 实际生成 prompt 长度 < 1500 字

---

## Stage 2 — ToolCallAccumulator + scopeJob + maxToolIterations

- [ ] `/workspace/app/encv-mobile/src/composables/useToolCallAccumulator.ts` 创建
- [ ] 4 状态机（pending / accumulating / complete / executed）
- [ ] `clear()` 在 ReAct 循环开始时调用（**不**在 tool_call_start 时清）
- [ ] **scopeJob 重建**（v2 新增，cancel() 创建新 ctx）
- [ ] **maxToolIterations = 20**（v2 新增，循环里 break + warnUser）
- [ ] 集成到 useAgent.ts send() / confirmTool()
- [ ] 6 个单测场景通过
  - 单一 tool call 完整累积
  - 同一轮 2-3 tool call 不互相覆盖
  - 中断累积不破坏下一个
  - args JSON 解析容错
  - maxToolIterations 到 20 触发 break
  - scopeJob 取消后 100ms 内新请求能起协程
- [ ] `vue-tsc --noEmit` 0 错误
- [ ] `vitest` 通过
- [ ] `go build` 0 错误

---

## Stage 3 — buildHistoryMessages + reasoningContent + 8 状态机

- [ ] `agent_api.go` buildHistoryMessages 实现
  - 按 toolCallId 去重
  - completedCalls 过滤
  - completedCalls 为空 → toolCalls=null（防 400）
- [ ] **reasoningContent 处理**（v2 新增）
  - 旧消息的 reasoning_content 不入 history
  - 最新一条可保留（前端折叠）
- [ ] useAgent.ts 处理 tool_result 按 toolCallId 去重
- [ ] **MessageStatus 8 状态机**（v2 新增）
  - SENDING / SENT / THINKING / STREAMING / EXECUTING / COMPLETE / ERROR / CANCELLED
- [ ] 5 个单测场景通过
  - 中断对话残留未完成 tool_call → 过滤
  - 同 toolCallId 推 2 次 → 只 1 条
  - 全部完成 → 完整保留
  - 旧消息 reasoning_content 剥离
  - 8 状态机正确流转
- [ ] 集成测试：中断对话后 LLM 不再 400
- [ ] `go build` + `vue-tsc` + `vitest` 全 0 错误

---

## Stage 4 — 参数别名 + Tool priority + paramHint

- [ ] tool registry 加 `ParamAliases map[string][]string`
- [ ] 5 工具参数别名互通（path/filePath/filename/url/link/query/output_path/projectName）
- [ ] **Tool priority 排序**（v2 新增）
  - priorityTools 集合置顶 0
  - requiresConfirmation 工具放最后 2
- [ ] **executeSafe paramHint**（v2 新增）
  - 工具失败时错误含：工具名 + 缺失参数名 + 类型 + 完整示例
- [ ] parseToolParams 容错（JSON 解析失败 fallback emptyMap）
- [ ] 单测通过
  - 5 工具参数别名互通
  - priority 工具在 token 预算紧张时仍在前 N
  - LLM 漏传参数时错误信息含示例
- [ ] `go test` + `vue-tsc` 通过

---

## Stage 5 — AppResult + AppError + classifyException + fromHttpCode

- [ ] `errors.go` ToolError 加 `HumanMessage string`
- [ ] AppErrorType 8 个枚举
- [ ] **classifyException** 函数（v2 新增，classify.go）
  - errors.As 区分 url.Error / net.OpError / context.DeadlineExceeded / context.Canceled
- [ ] **fromHttpCode 静态方法**（v2 新增）
  - 401 → ApiKeyInvalid
  - 402 → InsufficientBalance
  - 429 → RateLimited
  - 5xx → ServerError
- [ ] humanMessage 文案表（8 个 AppErrorType × 中英文 + emoji）
- [ ] AppResult helper（Go Result[T] + TypeScript 联合类型 + RunCatching）
- [ ] 注入到 ReAct 循环 catch 块
- [ ] 7 个单测通过
  - 401 → ApiKeyInvalid + humanMessage
  - 429 → RateLimited + isRetryable=true
  - 500 → ServerError + isRetryable=true
  - context.Canceled → UserCancelled
  - url.Error{Timeout: true} → NetworkTimeout
  - AppResult.runCatching 自动捕获 panic
  - map 链式调用正确传递 Failure
- [ ] i18n 切换正常

---

## Stage 6 — ContextWindowManager + TokenHudBar UI

- [ ] `context_window.go` ContextWindowManager 实现
  - EstimateTokens = len(text) / 3.5
  - updateAllocation(parts) → AllocationResult
  - emergencyCompress (RED) / compressConversation (YELLOW) / 截断 (FORCE)
- [ ] agent_api.go ReAct 循环每轮调用 updateAllocation
- [ ] 阈值常量（WARNING_YELLOW/RED/FORCE）
- [ ] **token_tracker.go** TokenTracker 实现
  - per-request cache hit rate
  - 平均延迟公式
- [ ] SSE 协议加 `token_stats` 事件（每 5 秒）
- [ ] 前端 `useChatStats.ts` composable
- [ ] `<TokenHudBar />` 7 行指标组件
  - 输入 / 输出 / 缓存 / 思考 / 上下文 / 速度 / 延迟
  - 颜色：GREEN < 80% / YELLOW 80-95% / RED > 95%
  - 复用 mobile-agent-polish-2026q2 UI 风格
- [ ] 6 个单测通过
  - EstimateTokens 精度 ±5%
  - 6 段总和 < 80% → GREEN
  - 6 段总和 85% → YELLOW → compressConversation
  - 6 段总和 96% → RED → emergencyCompress
  - emergencyCompress 后总和 < 80%
  - per-request cache hit rate 正确
- [ ] 5 秒聚合一次（性能 OK）

---

## Stage 7 — Skills + executeViaExternalModule + ZIP-slip

- [ ] 启动扫描 `~/.config/encv-go/skills/`，解析 skill.yaml
- [ ] SkillManifest YAML 解析（4 维权限）
  - filesystem (glob) / network (allowed_hosts) / packages / shell
  - isSandboxed 计算属性
  - 参数验证（int/float/bool/choice/string）
- [ ] **executeViaExternalModule 回退**（v2 新增）
  - 本地 → Skills 插件 → 错误
- [ ] **ZIP-slip 防护**（v2 新增，safeUnzip）
  - filepath.Clean + 前缀检查
  - 任何 os.Create 前必须做
- [ ] MarketPlace API（HTTP GET）
- [ ] 工具名 = `skill_<name>` 避免冲突
- [ ] 单测通过
  - Skill 加载流程 e2e
  - 本地 + Skills 工具都能用
  - ZIP-slip 攻击被拒
- [ ] `go test` 通过

---

## Stage 8 — 三层记忆 + autoExtract

- [ ] `memory/store.go` 三层记忆（SQLite + WAL）
  - ProjectMemoryEntity / UserProfileEntity / SemanticMemoryEntity
  - `PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;`
- [ ] **autoExtract** 函数（v2 新增）
  - 3 pattern regex
  - confidence: 显式 1.0 / 推断 0.7
- [ ] prompt.go 注入用户偏好（LoadHighConfidence(0.5)，最多 20 条）
- [ ] 前端 `MemoryManager.vue`
  - 查看 / 编辑 / 清除
- [ ] 5 个单测通过
  - ProjectMemory CRUD
  - UserProfile confidence 过滤
  - autoExtract 三种 pattern 匹配
  - WAL 模式生效
  - "我习惯用 TypeScript" → UserProfile(preferred_language=TypeScript, 0.7)

---

## Stage 9 — Python 沙箱 + isStdlibModule + 黑名单 + 文档生成

- [ ] `python_sandbox.go` 4 沙箱模式
  - STRICT / STANDARD / RELAXED / DOCUMENT_GENERATION
  - buildPolicyPreamble 注入 Python 前置代码
- [ ] **isStdlibModule 170+ 白名单**（v2 新增）
  - STDLIB_MODULES map
  - STRICT 模式非 stdlib 一律 reject
- [ ] **DANGEROUS_COMMANDS 黑名单**（v2 新增）
  - rm -rf /, mkfs., dd if=, > /dev/sda, fork bomb, chmod 777 /, curl|sh
- [ ] `run_python` 接受 `sandbox` 参数
- [ ] generate_docx / generate_xlsx / generate_pptx 工具
  - 预装 python-docx / openpyxl / python-pptx
  - 预热 Python 解释器（cold start 500ms → warm 50ms）
- [ ] 8 个单测通过
  - STRICT 拒绝 import requests
  - STANDARD 允许 import requests
  - "rm -rf /" → reject
  - fork bomb → reject
  - DOCUMENT_GENERATION 允许 python-docx
  - sandbox 路径外写文件 → PermissionError
  - IsStdlibModule("os") == true
  - IsStdlibModule("requests") == false

---

## Stage 10 — MessageBubble 增强 + FileOperations 安全 + 项目脚手架

### 后端
- [ ] `file_ops.go` 加 ResolvePath（filepath.Clean + canonical + 前缀检查）
- [ ] SkipDirs 常量（.git / .agent / node_modules / __pycache__ / build / .gradle）
- [ ] `high_level.go` create_project 支持 techStack
- [ ] `file_scaffold.go` buildProjectDirectories / buildReadme / buildGitignore
  - Python / Kotlin / JS / Go 4 模板

### 前端
- [ ] useAgent.ts 接收 tool_result 时记录 ToolCallRecord
  - 字段：toolName / input / output / status / startedAt / completedAt
- [ ] MessageBubble.vue 加 6 个 UI 模式
  - [ ] ToolExecutionCard（5 状态颜色 + 展开/折叠 + 黑底输出）
  - [ ] ReasoningSection（折叠 + expandVertically 动画）
  - [ ] FileChangeCard（绿/蓝/红颜色编码）
  - [ ] CodeBlock（shiki/prismjs + 复制按钮 + toast）
  - [ ] ThinkingIndicator（3 点错位 200ms 动画）
  - [ ] CombinedClickable onClick + onLongClick
- [ ] MessageStatus 升级到 8 状态（与 Stage 3 联动）

### 测试
- [ ] 10 个单测通过
  - ResolvePath("../../etc/passwd") → SecurityException
  - ResolvePath("docs/readme.md") → 正确路径
  - searchFiles 跳过 .git / node_modules
  - create_project("myapp", "Python") → 创建 src/ + tests/
  - buildReadme 包含项目名 + techStack
  - buildGitignore(techStack=Python) 包含 __pycache__/
  - ToolExecutionCard 5 状态颜色正确
  - ReasoningSection 默认折叠
  - CodeBlock 复制按钮触发 toast
  - ThinkingIndicator 三点错位动画

---

## 整体验收

- [ ] **后端**：`go build ./...` 0 错误
- [ ] **后端**：`go test ./...` 100% 通过（≥100 测试）
- [ ] **后端**：`go vet ./...` 0 警告
- [ ] **前端**：`pnpm run type-check` 0 错误
- [ ] **前端**：`pnpm run test:unit` 全部通过
- [ ] **前端**：`pnpm run build` 成功
- [ ] **集成**：`./scripts/test-e2e.sh` 全绿
- [ ] **手动**（安卓真机）：见每个 Stage 的"手动验证"

---

## 提交规范

每个 Stage 完成后：

```bash
# 1. 跑完整验证
cd /workspace && go build ./... && go test ./... && go vet ./...
cd /workspace/app/encv-mobile && pnpm run type-check && pnpm run test:unit && pnpm run build

# 2. 勾选对应 Stage 的 checklist + 更新 tasks.md 状态

# 3. 报告（不提交代码，等 user 明确批准后再 commit）
```

**绝对不要**：未经 user 明确批准就 `git add` / `git commit` / `git push`。
