// internal/server/agent_mock.go
//
// Mock LLM 引擎 — 不计费的剧本驱动 SSE 输出。
//
// 核心目标：
//  1. **零成本**：本地生成 SSE 事件流，不调真实 OpenAI，节省 API 费用
//  2. **确定性**：相同输入 → 相同输出，可做 e2e 断言
//  3. **场景覆盖**：内置 12 个剧本覆盖所有代码分支（工具/错误/截断/reasoning/并行/中文/无匹配）
//  4. **UI 0 改动**：mock 输出与真实输出走同一 s.sendAndCache 通道，事件结构字节级一致
//
// 触发匹配优先级：精确匹配 > 关键词 > 正则 > fallback
//   - builtin 模式无匹配 → 返回 default_friendly
//   - custom 模式无匹配 → 返回 nil（让调用方 fallback 到真实 API）
//
// 参考：
//   - Spec: /workspace/.trae/specs/agent-mock-mode/spec.md
//   - 接口契约：agentSession / AgentEvent / sendAndCache（见 agent_api.go）
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// ════════════════════════════════════════════════════════════════
// 类型定义
// ════════════════════════════════════════════════════════════════

// MockScenario 定义一个剧本化模拟对话。
// 触发匹配规则按以下优先级（第一个命中即返回）：
//  1. ExactMatch（精确匹配）
//  2. Keywords（任一关键词命中，不区分大小写）
//  3. Regex（正则编译后匹配）
//  4. fallback：ID == "default_friendly"
//
// v2 字段（agent-tools-scenarios-v2 spec）：
//   - Branches       分支选择列表（v2 mock_branch_choice 事件数据源）
//   - Rounds         总轮数（v2 多轮状态机，0 = 走 v1 线性路径）
//   - RoundContext   跨轮共享变量（round K+1 可读 K 写入的 key）
//   - TotalRounds    别名：与 Rounds 等价（兼容 spec 不同写法）
type MockScenario struct {
	ID          string
	Description string
	ExactMatch  string   // 精确匹配字符串
	Keywords    []string // 关键词匹配（任一命中，不区分大小写）
	Regex       string   // 正则匹配
	Steps       []MockStep
	// Presets 是「剧本激活时」向 UI 推送的快捷输入按钮。
	// 每个预设会被前端渲染为 chip；点击后自动填充或直接发送 userText。
	// 高级剧本可同时利用 mid-scenario 的 mock_presets 事件覆盖/扩展
	// 此初始列表（实现「随剧本进度更新」的多轮会话交互）。
	Presets []MockPreset
	// ── v2 多轮 / 分支字段 ──
	Branches     []Branch       // 可选的分支列表（mock_branch_choice 推送）
	Rounds       int            // 剧本总轮数（0 = v1 线性行为）
	RoundContext map[string]any // 跨轮共享变量（user 文本写入 → 后续 step 读取）
	TotalRounds  int            // 同 Rounds（spec 兼容字段）
}

// Branch 表示剧本内的一个分支选项。
//
// 触发匹配（PickBranch 时按此优先级）：
//  1. 精确匹配：branch.ID == userText
//  2. 关键词匹配：任一 TriggerKeyword 出现在 userText
//  3. 正则匹配：TriggerRegex 编译后 MatchString
//  4. 都不匹配 → 引擎重新推送 mock_branch_choice 提示
//
// 匹配后跳到 OnMatch 子剧本（独立 stream + EventCache）。
// InitialStepID 可选：在新 stream 中从哪个 step 开始。
type Branch struct {
	ID              string
	Label           string
	Description     string
	Icon            string
	TriggerKeywords []string
	TriggerRegex    string
	OnMatch         *MockScenario
	InitialStepID   string
}

// MockPreset 是单个预设输入按钮。
//
// ID 在剧本内唯一，用于去重 / 追踪点击。
// Label 显示在 chip 上，UserText 是点击后实际发送给 agent 的文本。
// Icon / Tooltip 可选（前端会安全 fallback）。
type MockPreset struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	UserText string `json:"userText"`
	Icon     string `json:"icon,omitempty"`
	Tooltip  string `json:"tooltip,omitempty"`
}

// MockStep 是一组事件 + 触发前延迟。
//
// 推送语义：
//   - 先按 step 顺序遍历 Steps
//   - 每个 step 内：先 sleep(DelayMs / speed) → 再依次推 Events
//   - 推事件过程中检测 ctx.Done() 立即退出
//
// v2 字段（agent-tools-scenarios-v2 spec）：
//   - BranchID       此 step 关联的分支 ID（v2 推 mock_branch_choice 用）
//   - RoundIdx       此 step 属于第几轮（0-based，v2 状态机）
//   - PauseForUser   推完后暂停剧本等待 user_text（推 mock_round_state）
//   - SetContext     推完后写入 RoundContext 的 key/value
//   - UseContext     推完前从 RoundContext 读这些 key 做模板插值
//   - BranchChoice   标记此 step 为分支选择（推 mock_branch_choice 事件）
type MockStep struct {
	DelayMs      int
	Events       []MockEvent
	BranchID     string         // v2：所属分支 ID
	RoundIdx     int            // v2：轮次索引
	PauseForUser bool           // v2：推完此 step 后暂停
	SetContext   map[string]any // v2：推完后写入 RoundContext
	UseContext   []string       // v2：使用 RoundContext 的 key 做模板插值
	BranchChoice bool           // v2：标记此 step 为分支选择点
}

// MockEvent 是单个 SSE 事件。
//
// Type 取值：stream_start / text_delta / reasoning_delta / tool_call /
// tool_status / tool_result / stream_status / stream_end /
// mid_stream_disconnect / sse_corrupt_chunk
type MockEvent struct {
	Type string                 // 事件类型
	Data map[string]interface{} // 事件 data 载荷
}

// MockEngine 持有激活的剧本集合（builtin + custom）。
//
// 线程安全说明：本类型主要在 handleAgentChat 单请求上下文中使用，
// LoadCustom 在配置变更时调用，调用方需自行加锁（典型用法是 Server 启动时一次）。
//
// 关键变更（剧本外置 spec）：scenarios 改为 map[id]*MockScenario，O(1) 查询。
// 保留 builtinScenarios / customScenarios 两个 slice 用于 AllScenarios() 兼容性输出。
type MockEngine struct {
	builtinScenarios []*MockScenario
	customScenarios  []*MockScenario

	// scenariosByID 是 builtin + custom 合并的 O(1) 查询 map。
	// 构造时（NewMockEngine）一次性建好，运行时只读。
	scenariosByID map[string]*MockScenario

	// realExecutor 是 tool_call 事件 data 中 execute_real=true 时
	// 实际调用以拿真实结果的回调。典型为 (*Server).executeAgentTool。
	// nil 时 execute_real=true 的 tool_call 仍按剧本硬编码 result 推送
	// （保持单测可用 + 容灾）。
	realExecutor func(ctx context.Context, toolName, argsJSON string) (string, error)
}

// SetRealExecutor 注入真实工具执行器。生产环境（*Server）启动时调用一次，
// 把 s.executeAgentTool 绑进来；单测环境不调用，全部走硬编码剧本。
func (e *MockEngine) SetRealExecutor(fn func(ctx context.Context, toolName, argsJSON string) (string, error)) {
	e.realExecutor = fn
}

// ════════════════════════════════════════════════════════════════
// 构造函数 + 辅助方法
// ════════════════════════════════════════════════════════════════

// NewMockEngine 返回预加载所有内置剧本的引擎（v1 + v2 共 20 个，编译期从 YAML 嵌入）。
//
// 单一数据源：剧本只存在于 internal/server/mock_scenarios/builtin/*.yaml，
// 通过 //go:embed 在编译期嵌入二进制。Go 字面量剧本已废弃（硬编码路径/大小/计数是反模式）。
//
// v2 场景（mockScenariosV2）由 MockEngineV2 路径消费，**不**追加到 builtinScenarios
// （保持 builtin 集合 v1+v2 各自有清晰边界；测试可独立断言）。
func NewMockEngine() *MockEngine {
	e := &MockEngine{}
	e.builtinScenarios = append(e.builtinScenarios, mockScenariosBuiltin...)
	e.rebuildIndex()
	return e
}

// NewMockEngineWithScenarios 用外部剧本集合构造引擎。
//
// 用于剧本外置 spec：Server 启动时从 YAML 加载，传入此构造函数。
// 外部剧本会覆盖 builtin 集合（若 id 冲突）。
func NewMockEngineWithScenarios(scenarios []*MockScenario) *MockEngine {
	e := &MockEngine{}
	for _, sc := range scenarios {
		if sc == nil {
			continue
		}
		e.builtinScenarios = append(e.builtinScenarios, sc)
	}
	e.rebuildIndex()
	return e
}

// rebuildIndex 重建 scenariosByID 查询表。
func (e *MockEngine) rebuildIndex() {
	e.scenariosByID = make(map[string]*MockScenario, len(e.builtinScenarios)+len(e.customScenarios))
	for _, sc := range e.builtinScenarios {
		if sc != nil && sc.ID != "" {
			e.scenariosByID[sc.ID] = sc
		}
	}
	for _, sc := range e.customScenarios {
		if sc != nil && sc.ID != "" {
			e.scenariosByID[sc.ID] = sc
		}
	}
}

// LoadCustom 替换 custom 剧本集合，并对每个剧本做轻量验证。
// 验证失败的剧本 slog.Warn 后跳过，不影响其他剧本加载。
func (e *MockEngine) LoadCustom(custom []MockScenario) {
	validated := make([]*MockScenario, 0, len(custom))
	seenIDs := make(map[string]bool, len(custom))
	for i := range custom {
		sc := custom[i]
		if sc.ID == "" {
			slog.Warn("mock: skip custom scenario (empty id)", "index", i)
			continue
		}
		if seenIDs[sc.ID] {
			slog.Warn("mock: skip custom scenario (duplicate id)", "id", sc.ID)
			continue
		}
		if sc.ExactMatch == "" && len(sc.Keywords) == 0 && sc.Regex == "" {
			slog.Warn("mock: skip custom scenario (no trigger rule)", "id", sc.ID)
			continue
		}
		// 验证 Regex 可编译（失败不 panic，仅 warn 跳过）
		if sc.Regex != "" {
			if _, err := regexp.Compile(sc.Regex); err != nil {
				slog.Warn("mock: skip custom scenario (regex compile failed)",
					"id", sc.ID, "regex", sc.Regex, "error", err)
				continue
			}
		}
		seenIDs[sc.ID] = true
		validated = append(validated, &sc)
	}
	e.customScenarios = validated
	e.rebuildIndex()
	slog.Info("mock: custom scenarios loaded", "count", len(validated))
}

// AllScenarios 返回 builtin + custom 全部剧本（供 inspection / debug）。
func (e *MockEngine) AllScenarios() []*MockScenario {
	out := make([]*MockScenario, 0, len(e.builtinScenarios)+len(e.customScenarios))
	out = append(out, e.builtinScenarios...)
	out = append(out, e.customScenarios...)
	return out
}

// GetScenarioByID 按 ID O(1) 查询剧本。找不到返回 nil。
//
// 用于 branch-pick 端点 / v2 resume 路径。
func (e *MockEngine) GetScenarioByID(id string) *MockScenario {
	if e.scenariosByID == nil {
		return nil
	}
	return e.scenariosByID[id]
}

// ════════════════════════════════════════════════════════════════
// 匹配逻辑
// ════════════════════════════════════════════════════════════════

// Match 返回最高优先级的匹配剧本。
//
// mode 取值：
//   - "builtin"：先搜 custom → 失败再搜 builtin → 失败返回 default_friendly
//   - "custom"：仅搜 custom → 失败返回 nil（让调用方 fallback 到真实 API）
//   - 其他值：等同于 "builtin"（保守 fallback）
func (e *MockEngine) Match(userText string, mode string) *MockScenario {
	if userText == "" {
		// 空文本：builtin 模式返回 default，custom 模式返回 nil
		return e.fallbackForMode(mode)
	}

	// custom 模式优先（spec：custom 模式仅匹配 custom 剧本）
	if mode == "custom" {
		if sc := matchInList(userText, e.customScenarios); sc != nil {
			return sc
		}
		return nil
	}

	// builtin 模式：custom 也可命中（允许用户覆盖默认剧本）
	if sc := matchInList(userText, e.customScenarios); sc != nil {
		return sc
	}
	if sc := matchInList(userText, e.builtinScenarios); sc != nil {
		return sc
	}
	return e.defaultFriendly()
}

// fallbackForMode 根据 mode 决定空匹配时的 fallback。
func (e *MockEngine) fallbackForMode(mode string) *MockScenario {
	if mode == "custom" {
		return nil
	}
	return e.defaultFriendly()
}

// defaultFriendly 返回内置 default_friendly 剧本（不存在则 panic — 配置错误）。
//
// 依赖：NewMockEngine 从 mockScenariosBuiltin 填充 builtinScenarios，
// 而 mockScenariosBuiltin 在 init() 期从 mock_scenarios/builtin/*.yaml 加载，
// 其中 01_default_friendly.yaml 必含 id="default_friendly"。
// 如果 YAML 集合未含此剧本 → init() 已 panic（embed 文件系统错误或 YAML 解析失败）。
func (e *MockEngine) defaultFriendly() *MockScenario {
	for _, sc := range e.builtinScenarios {
		if sc.ID == "default_friendly" {
			return sc
		}
	}
	// 不会发生：mockScenariosBuiltin 必含 default_friendly
	panic("mock engine: builtin scenarios missing 'default_friendly' (YAML 01_default_friendly.yaml 缺失或解析失败)")
}

// matchInList 在剧本列表中按优先级匹配（精确 > 关键词 > 正则）。
// 找到第一个命中即返回。Regex 编译失败 slog.Warn 后跳过该剧本。
func matchInList(userText string, scenarios []*MockScenario) *MockScenario {
	for _, sc := range scenarios {
		// 1. 精确匹配
		if sc.ExactMatch != "" && userText == sc.ExactMatch {
			return sc
		}
		// 2. 关键词匹配（不区分大小写）
		if len(sc.Keywords) > 0 {
			lower := strings.ToLower(userText)
			for _, kw := range sc.Keywords {
				if kw == "" {
					continue
				}
				if strings.Contains(lower, strings.ToLower(kw)) {
					return sc
				}
			}
		}
		// 3. 正则匹配
		if sc.Regex != "" {
			re, err := regexp.Compile(sc.Regex)
			if err != nil {
				slog.Warn("mock: scenario regex compile failed, skipping",
					"id", sc.ID, "regex", sc.Regex, "error", err)
				continue
			}
			if re.MatchString(userText) {
				return sc
			}
		}
	}
	return nil
}

// ════════════════════════════════════════════════════════════════
// 剧本执行
// ════════════════════════════════════════════════════════════════

// Run 执行剧本：按 step 顺序遍历，每个 step 先 sleep(DelayMs/speed) 再依次推事件。
//
// 参数：
//   - ctx：取消信号（客户端断开 / 请求超时）
//   - s：Server 实例（用于调 sendAndCache）
//   - sess：当前 session（用于 EventCache 断点续传）
//   - w / flusher：SSE writer
//   - scenario：要执行的剧本
//   - speed：速率倍率（1.0=正常，0.1=10x慢，10=10x快，0=零延迟）
//   - mockFlag：是否在第一个 stream_start 事件注入 mock: true 字段
//   - aguiMode：是否使用 AG-UI 协议格式输出事件（Phase 4 新增）
//
// 返回：
//   - 客户端断开 / ctx 取消：返回 ctx.Err()
//   - mid_stream_disconnect 错误类型：推 2 个 text_delta 后返回 nil（关闭 SSE）
//   - 其他错误：返回对应错误
func (e *MockEngine) Run(ctx context.Context, s *Server, sess *agentSession, w http.ResponseWriter, flusher http.Flusher, scenario *MockScenario, speed float64, mockFlag bool, aguiMode ...bool) error {
	if scenario == nil {
		return nil
	}

	// 速率归一化：避免 speed 为 0 或负数时除零
	effectiveSpeed := speed
	if effectiveSpeed <= 0 {
		effectiveSpeed = 0 // 表示零延迟（见 sleepDelay 逻辑）
	}

	textSeq := 0
	reasoningSeq := 0
	streamStartEmitted := false

	// ════════════════════════════════════════════════════════════
	// AG-UI 协议模式（Phase 4）
	// ════════════════════════════════════════════════════════════
	// 当 aguiMode=true 时，使用 AGUIEventMapper 将内部事件映射为标准 AG-UI 格式。
	// aguiMode 是 variadic bool 参数（向后兼容：旧调用方不传时默认 false）。
	useAGUI := len(aguiMode) > 0 && aguiMode[0]
	var aguiMapper *AGUIEventMapper
	if useAGUI {
		sessID := ""
		if sess != nil {
			sessID = sess.SessionID
		}
		aguiMapper = NewAGUIMapper(w, flusher, sessID)
		slog.Info("mock: AG-UI protocol mode enabled", "scenario", scenario.ID)
	}

	// emitEvent 统一事件发送入口：根据 useAGUI 选择输出通道。
	//   - useAGUI=false → 走原有 sendAndCache（自定义 SSE 格式 + EventCache 缓存）
	//   - useAGUI=true  → 走 AGUIEventMapper.MapEvent（AG-UI 标准格式，不缓存）
	emitEvent := func(ev MockEvent, stepIdx, evIdx int) {
		if useAGUI && aguiMapper != nil {
			aguiMapper.MapEvent(ev, stepIdx, evIdx)
		} else {
			s.sendAndCache(sess, w, flusher, ev.Type, ev.Data)
		}
	}

	// pendingRealCalls 记录剧本中声明 execute_real=true 的 tool_call。
	// 键为 call ID，值为 (toolName, argsJSON)。
	// 在后续 step 遇到匹配的 tool_result 时，realExecutor 实际执行，
	// 真实返回值覆盖剧本的硬编码 result。
	pendingRealCalls := make(map[string]pendingRealCall)

	// collectedResults 收集所有已完成的 tool_result（id → 结果信息）。
	// 用于 text_delta_templated 事件类型从真实结果动态生成文本，
	// 避免剧本 Step 7 硬编码文件名/大小等数值。
	collectedResults := make(map[string]toolResultInfo)

	// ─── Task 27 调试：写独立日志文件（stdout 被 air-run.sh exec 吞掉后看不到） ──
	// 每次 Run 写入 /tmp/mock-debug-{scenarioID}.log，追加模式。
	// 格式：每行一个 event（ts | step | evIdx | type | data摘要）
	// 用完后 `cat /tmp/mock-debug-list_files_query.log` 直接看。
	debugLog, debugLogErr := os.OpenFile(
		"/tmp/mock-debug-"+scenario.ID+".log",
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644,
	)
	if debugLogErr != nil {
		slog.Warn("mock: cannot open debug log file", "error", debugLogErr)
	}
	defer func() { //nolint:errcheck
		if debugLog != nil {
			debugLog.Close()
		}
	}()

	writeDebug := func(step, ev int, evType string, dataSummary string) {
		if debugLog == nil {
			return
		}
		line := fmt.Sprintf("%s | step=%d ev=%d type=%s data=%s\n",
			time.Now().Format("15:04:05.000"), step, ev, evType, dataSummary)
		debugLog.WriteString(line) //nolint:errcheck
	}

	for stepIdx, step := range scenario.Steps {
		// 1. 推 step 前的延迟
		if err := sleepDelay(ctx, step.DelayMs, effectiveSpeed); err != nil {
			slog.Info("mock: ctx cancelled during step delay",
				"scenario", scenario.ID, "step", stepIdx, "error", err)
			return err
		}

		// 2. 推 step 内每个事件
		for evIdx, ev := range step.Events {
			// 中途 ctx 取消检查
			select {
			case <-ctx.Done():
				slog.Info("mock: ctx cancelled mid-step",
					"scenario", scenario.ID, "step", stepIdx, "ev", evIdx)
				return ctx.Err()
			default:
			}

			switch ev.Type {
			case "mid_stream_disconnect":
				// 推 2 个 text_delta 后关闭 SSE（模拟客户端断连）
				textSeq++
				emitEvent(MockEvent{Type: "text_delta", Data: map[string]interface{}{"seq": textSeq, "text": "我先说一半..."}}, stepIdx, evIdx)
				textSeq++
				emitEvent(MockEvent{Type: "text_delta", Data: map[string]interface{}{"seq": textSeq, "text": "嗯"}}, stepIdx, evIdx)
				slog.Info("mock: mid_stream_disconnect triggered",
					"scenario", scenario.ID)
				return nil // SSE 由调用方关闭

			case "sse_corrupt_chunk":
				// 推一段非 JSON 的 SSE 数据（验证前端 JSON 解析容错）
				sendCorruptChunk(w)
				slog.Info("mock: sse_corrupt_chunk injected", "scenario", scenario.ID)
				continue

			case "tool_call":
				// 验证必填字段
				name, _ := ev.Data["name"].(string)
				id, _ := ev.Data["id"].(string)
				if name == "" || id == "" {
					slog.Warn("mock: skip tool_call event (missing name or id)",
						"scenario", scenario.ID, "step", stepIdx, "ev", evIdx)
					continue
				}
				// execute_real 字段：true 时把 (id → name+args) 登记到 pendingRealCalls，
				// 后续匹配 tool_result 时实际调 realExecutor 拿真实结果。
				// 缺省 false（保持向后兼容：单测/无 executor 时按硬编码剧本走）。
				if execReal, _ := ev.Data["execute_real"].(bool); execReal {
					args, _ := ev.Data["args"].(string)
					pendingRealCalls[id] = pendingRealCall{name: name, args: args}
					slog.Debug("mock: tool_call marked execute_real=true, will call real handler at result",
						"scenario", scenario.ID, "id", id, "name", name)
				}
				emitEvent(ev, stepIdx, evIdx)
				writeDebug(stepIdx, evIdx, "tool_call", fmt.Sprintf("id=%s name=%s", id, name))

			case "stream_start":
				// 首个 stream_start 注入 mock: true 字段（前端用来显示「模拟模式」徽章）
				if mockFlag && !streamStartEmitted {
					merged := make(map[string]interface{}, len(ev.Data)+1)
					for k, v := range ev.Data {
						merged[k] = v
					}
					merged["mock"] = true
					merged["scenario"] = scenario.ID
					emitEvent(MockEvent{Type: "stream_start", Data: merged}, stepIdx, evIdx)
					streamStartEmitted = true
					writeDebug(stepIdx, evIdx, "stream_start", fmt.Sprintf("mock=%v scenario=%s", mockFlag, scenario.ID))

					// 紧随 stream_start 推送初始预设按钮（如果有）。
					// 前端 MockPresetBar 监听到 mock_presets 事件即渲染 chip。
					// mid-scenario 期间的预设更新通过单独的 mock_presets 事件
					// 在 steps 内推（见下方 switch case）。
					if len(scenario.Presets) > 0 {
						emitEvent(MockEvent{Type: "mock_presets", Data: map[string]interface{}{
							"scenario": scenario.ID,
							"phase":    "initial",
							"presets":  scenario.Presets,
						}}, stepIdx, evIdx)
					}
				} else {
					emitEvent(ev, stepIdx, evIdx)
				}

			case "mock_presets":
				// mid-scenario 预设更新（高级剧本多轮会话）：
				// 剧本可在任意 step 推一个 mock_presets 事件，data.presets 覆盖
				// 当前激活的预设列表。前端基于此实时刷新 chip 按钮。
				// data.phase 字段由剧本作者标记当前阶段（initial/middle/followup），
				// 仅做调试 / 埋点用，前端不强依赖。
				emitEvent(ev, stepIdx, evIdx)

			case "text_delta":
				textSeq++
				text, _ := ev.Data["text"].(string)
				emitEvent(MockEvent{Type: "text_delta", Data: map[string]interface{}{"seq": textSeq, "text": text}}, stepIdx, evIdx)
				writeDebug(stepIdx, evIdx, "text_delta", text[:min(80, len(text))])

			case "text_delta_templated":
				// 动态文本模板：data.text 中可包含 {%id%} 占位符，
				// Run 方法从 collectedResults 中取对应 tool_result 的 JSON 替换。
				// 用途：Step 7 总结文本从真实工具返回值动态生成文件名/大小等，
				// 避免硬编码数值与实际数据不一致。
				textSeq++
				template, _ := ev.Data["text"].(string)
				rendered := renderTextTemplate(template, collectedResults)
				emitEvent(MockEvent{Type: "text_delta_templated", Data: map[string]interface{}{"seq": textSeq, "text": rendered}}, stepIdx, evIdx)
				writeDebug(stepIdx, evIdx, "text_delta_templated", rendered[:min(100, len(rendered))])

			case "reasoning_delta":
				reasoningSeq++
				text, _ := ev.Data["text"].(string)
				emitEvent(MockEvent{Type: "reasoning_delta", Data: map[string]interface{}{"seq": reasoningSeq, "text": text}}, stepIdx, evIdx)

			case "stream_status":
				emitEvent(ev, stepIdx, evIdx)
				// 错误状态：自动追加 stream_end(finishReason=error) 终止流
				if isErrorStatus(ev.Data) {
					emitEvent(MockEvent{Type: "stream_end", Data: map[string]interface{}{"finishReason": "error"}}, stepIdx, evIdx)
					s.endMockPresets(sess, w, flusher, scenario.ID, "stream_status_error")
					slog.Info("mock: stream_status=error injected auto stream_end",
						"scenario", scenario.ID)
					return nil
				}

			case "tool_result":
				// 铁律检查：YAML auto-injected tool_result（__yaml_auto_generated=true）
				// 必须由真实工具执行覆盖。Run 阶段直接调 executor 调真实工具。
				if autoGen, _ := ev.Data["__yaml_auto_generated"].(bool); autoGen {
					id, _ := ev.Data["id"].(string)
					if err := e.executeAutoToolResult(ctx, s, sess, w, flusher, ev, pendingRealCalls, id, writeDebug, emitEvent, stepIdx, evIdx); err != nil {
						return err
					}
					continue
				}
				// 若对应的 tool_call 标记了 execute_real=true 且 realExecutor 已注入，
				// 实际调用以拿真实结果，覆盖剧本的硬编码 result。
				// 否则按剧本硬编码 data 原样推送。
				id, _ := ev.Data["id"].(string)
				name, _ := ev.Data["name"].(string)
				if pending, ok := pendingRealCalls[id]; ok {
					delete(pendingRealCalls, id)
					if e.realExecutor != nil {
						// 先检查 ctx 取消（避免 realExecutor 长耗时后做无用功）
						select {
						case <-ctx.Done():
							slog.Info("mock: ctx cancelled before real tool exec",
								"scenario", scenario.ID, "id", id, "name", pending.name)
							return ctx.Err()
						default:
						}
						// 关键：把 emitEvent 闭包传进去，让真实工具结果也走 AG-UI mapper
						// 否则 useAGUI=true 时 executeRealAndEmit 走 sendAndCache
						// 推 legacy tool_result 格式 → 前端 AG-UI parser 无法解析
						realResult := e.executeRealAndEmit(ctx, s, sess, w, flusher, pending, id, name, writeDebug, emitEvent, stepIdx, evIdx)
						collectedResults[id] = toolResultInfo{id: id, name: name, result: realResult}
						continue
					}
					// realExecutor == nil：单测/容灾路径，硬编码剧本 result
					slog.Debug("mock: execute_real=true but realExecutor nil, falling back to hardcoded result",
						"scenario", scenario.ID, "id", id)
				}
				// 收集结果（硬编码路径 + realExecutor=nil fallback）
				resultStr, _ := ev.Data["result"].(string)
				if resultStr != "" && id != "" {
					collectedResults[id] = toolResultInfo{id: id, name: name, result: resultStr}
				}
				emitEvent(ev, stepIdx, evIdx)
				writeDebug(stepIdx, evIdx, "tool_result", fmt.Sprintf("id=%s name=%s", id, name))

			case "stream_end":
				// 推送 stream_end 后**不再**清空 chip —— 用户视角的"覆盖显示"语义：
				// chip 在 mock 模式开启期间永远覆盖在输入框上方，剧本结束后保留
				// 当前阶段 chip（mid-scenario 推过的话保留 mid；没推过保留 initial），
				// 下次 stream_start 后推的 mock_presets 会**覆盖**当前 chip。
				// 仅当用户**主动**退出 mock 模式（前端点 "🧪 模拟" 切换）才发
				// mock_presets_clear。
				emitEvent(ev, stepIdx, evIdx)
				writeDebug(stepIdx, evIdx, "stream_end", "")

			default:
				// 其他类型：原样推送
				emitEvent(ev, stepIdx, evIdx)
				writeDebug(stepIdx, evIdx, ev.Type, "")
			}
		}
	}

	// 剧本结束时尚未消费的 pending real calls（剧本只发了 tool_call
	// 没发对应 tool_result）→ 静默丢弃。Run 局部 map 随函数返回 GC。
	if len(pendingRealCalls) > 0 {
		slog.Debug("mock: scenario ended with unmatched real-exec tool_calls (will be GC'd)",
			"scenario", scenario.ID, "count", len(pendingRealCalls))
	}

	// 防御性兜底已移除：剧本结束不再发 clear —— chip 在 mock 模式开启期间
	// 永远覆盖显示。endMockPresets 函数仍保留，供"用户主动退出 mock 模式"
	// 路径调用（由前端 setMockMode("off") 触发，会清空 chip）。

	return nil
}

// endMockPresets 推一个 mock_presets_clear 事件，前端 MockPresetBar 收到后清空 chip。
// reason 仅做调试 / 埋点用，前端不强依赖。
func (s *Server) endMockPresets(sess *agentSession, w http.ResponseWriter, flusher http.Flusher, scenarioID, reason string) {
	s.sendAndCache(sess, w, flusher, "mock_presets_clear", map[string]interface{}{
		"scenario": scenarioID,
		"reason":   reason,
	})
}

// pendingRealCall 是剧本中标记 execute_real=true 的 tool_call 的最小信息。
// 后续匹配的 tool_result 事件触发实际 realExecutor 调用时使用。
type pendingRealCall struct {
	name string
	args string
}

// toolResultInfo 记录已完成的 tool_result（用于动态文本模板）。
type toolResultInfo struct {
	id     string
	name   string
	result string // JSON 字符串
}

// executeRealAndEmit 实际调用 realExecutor 拿真实结果，推送 tool_result 事件。
//
// 返回真实结果字符串（供 Run 方法写入 collectedResults 用于动态模板）。
//
// 事件 data 字段：
//   - id, name：取自剧本 tool_call（保证 ID 一致性，前端可关联 tool_call ↔ tool_result）
//   - result：realExecutor 返回的 JSON 字符串
//   - isError / status：基于 realExecutor 的 error 返回值
//   - durationMs：真实执行耗时
//
// realExecutor 内部会自行处理 ctx 取消（executeFSTool/executePluginTool 都用 ctx 传下去）。
// 本函数不返回错误，slog 记录所有失败路径。
//
// emitEvent 闭包：传入 Run 上下文的统一事件出口，
//   - useAGUI=false → sendAndCache（legacy SSE 格式 + EventCache 缓存）
//   - useAGUI=true  → aguiMapper.MapEvent（AG-UI 标准格式）
//
// 这样真实工具路径与剧本路径走完全相同的 emit 通道，AG-UI mapper 不会被旁路。
func (e *MockEngine) executeRealAndEmit(
	ctx context.Context,
	s *Server,
	sess *agentSession,
	w http.ResponseWriter,
	flusher http.Flusher,
	pending pendingRealCall,
	id string,
	name string,
	writeDebug func(step, ev int, evType, dataSummary string),
	emitEvent func(ev MockEvent, stepIdx, evIdx int),
	stepIdx int,
	evIdx int,
) string {
	t0 := time.Now()
	out, err := e.realExecutor(ctx, pending.name, pending.args)
	dur := time.Since(t0).Milliseconds()

	if err != nil {
		// 真实工具执行报错 → 推送带 isError=true 的 tool_result
		resultStr := out
		if resultStr == "" {
			resultStr = fmt.Sprintf(`{"error":%q}`, err.Error())
		}
		slog.Warn("mock: real tool exec failed, emit tool_result with isError=true",
			"id", id, "name", pending.name, "args", pending.args, "dur_ms", dur, "error", err)
		// 走 emitEvent 闭包（统一通道），而不是直接 s.sendAndCache
		// 这是 AG-UI 模式下 tool_result 仍能正确推送的修复点
		emitEvent(MockEvent{Type: "tool_result", Data: map[string]interface{}{
			"id":         id,
			"name":       name,
			"result":     resultStr,
			"isError":    true,
			"status":     "failed",
			"durationMs": dur,
		}}, stepIdx, evIdx)
		writeDebug(-1, -1, "tool_result(err)", fmt.Sprintf("id=%s name=%s err=%s", id, name, err.Error()))
		return resultStr // 返回结果供 collectedResults 收集
	}

	slog.Info("mock: real tool exec succeeded",
		"id", id, "name", pending.name, "args", pending.args, "dur_ms", dur)
	// 走 emitEvent 闭包（统一通道）
	emitEvent(MockEvent{Type: "tool_result", Data: map[string]interface{}{
		"id":         id,
		"name":       name,
		"result":     out,
		"isError":    false,
		"status":     "success",
		"durationMs": dur,
	}}, stepIdx, evIdx)
	writeDebug(-1, -1, "tool_result(ok)", fmt.Sprintf("id=%s name=%s dur=%dms", id, name, dur))
	return out // 返回结果供 collectedResults 收集
}

// executeAutoToolResult 处理 YAML auto-injected tool_result（剧本外置 spec）。
//
// 在 Run() 阶段遇到 __yaml_auto_generated=true 的 tool_result 时调用：
//  1. 从 tool_call event (在 pendingRealCalls 中) 取 name + args
//  2. 调 realExecutor（或 s.executeAgentToolAsExecutor）拿真实结果
//  3. 推送真实 tool_status(success/failed) + tool_result
//  4. 写入 collectedResults
//
// 与原 executeRealAndEmit 的区别：
//   - executeRealAndEmit 要求 tool_call event 显式标 execute_real=true
//   - executeAutoToolResult 是 YAML 模式的"无条件"路径（schema 保证 tool_call 必配 auto tool_result）
func (e *MockEngine) executeAutoToolResult(
	ctx context.Context,
	s *Server,
	sess *agentSession,
	w http.ResponseWriter,
	flusher http.Flusher,
	ev MockEvent,
	pendingRealCalls map[string]pendingRealCall,
	callID string,
	writeDebug func(step, ev int, evType, dataSummary string),
	emitEvent func(ev MockEvent, stepIdx, evIdx int),
	stepIdx int,
	evIdx int,
) error {
	id, _ := ev.Data["id"].(string)
	if id == "" {
		slog.Warn("mock: auto tool_result has empty id, skipping",
			"scenario", "?", "step", stepIdx, "ev", evIdx)
		return nil
	}

	pending, ok := pendingRealCalls[id]
	if !ok {
		// 找不到对应的 tool_call：auto tool_result 是个孤儿（剧本写错了）
		// 推一个失败 tool_result 让前端能看到错误
		slog.Error("mock: auto tool_result without matching tool_call",
			"id", id, "step", stepIdx, "ev", evIdx)
		emitEvent(MockEvent{Type: "tool_result", Data: map[string]interface{}{
			"id":         id,
			"isError":    true,
			"status":     "failed",
			"durationMs": 0,
			"result":     `{"error":"orphan tool_result: no matching tool_call"}`,
		}}, stepIdx, evIdx)
		return nil
	}
	delete(pendingRealCalls, id)

	// 调真实执行器（与 executeRealAndEmit 共享逻辑）
	if e.realExecutor != nil {
		e.executeRealAndEmit(ctx, s, sess, w, flusher, pending, id, pending.name, writeDebug, emitEvent, stepIdx, evIdx)
	} else {
		// 真实执行器未注入（单测 / 容灾）：推失败 tool_result
		slog.Warn("mock: realExecutor not injected, auto tool_result fails",
			"id", id, "name", pending.name)
		emitEvent(MockEvent{Type: "tool_result", Data: map[string]interface{}{
			"id":         id,
			"name":       pending.name,
			"isError":    true,
			"status":     "failed",
			"durationMs": 0,
			"result":     `{"error":"realExecutor not configured"}`,
		}}, stepIdx, evIdx)
	}
	return nil
}

// renderTextTemplate 将模板中的 {%id%} 和 {%id:field%} 占位符替换为 tool_result 真实数据。
//
// 语法：
//   - {%call_id%}           → 完整 JSON 结果
//   - {%call_id:field%}     → JSON 中指定字段的值（自动格式化）
//   - {%call_id:files%}     → list_files 结果的文件列表摘要（名称+大小）
//   - {%call_id:mounts%}    → list_mounts 结果的挂载点列表摘要
//
// 示例模板：
//
//	"发现文件：{%call_files2:files%}"
//	→ "发现文件：comedy.mkv (11.4 KB) / sample.mp4 (21.5 KB)"
func renderTextTemplate(template string, results map[string]toolResultInfo) string {
	// 快速路径：无占位符直接返回
	if !strings.Contains(template, "{%") {
		return template
	}

	// 正则匹配 {%id%} 或 {%id:field%}
	re := regexp.MustCompile(`\{%(\w+)(?::(\w+))?\%}`)
	result := re.ReplaceAllStringFunc(template, func(match string) string {
		// 提取 id 和可选 field
		submatches := re.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match // 格式不匹配，原样保留
		}
		callID := submatches[1]
		field := submatches[2]

		info, ok := results[callID]
		if !ok {
			return fmt.Sprintf("(未知工具:%s)", callID)
		}

		// 无字段指定 → 返回完整 JSON（截断防过长）
		if field == "" {
			trimmed := info.result
			if len(trimmed) > 200 {
				trimmed = trimmed[:200] + "..."
			}
			return trimmed
		}

		// 有字段指定 → 从 JSON 中提取并格式化
		return extractField(info.result, callID, field)
	})

	return result
}

// extractField 从 tool_result JSON 中提取指定字段并返回人类可读的文本。
func extractField(jsonStr, callID, field string) string {
	var raw interface{}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return fmt.Sprintf("(解析失败:%s)", field)
	}

	obj, ok := raw.(map[string]interface{})
	if !ok {
		return "(非对象结果)"
	}

	switch field {
	case "files", "items":
		// 文件/目录列表 → "name (size)" 格式
		arr, _ := obj[field].([]interface{})
		var parts []string
		for _, item := range arr {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := itemMap["name"].(string)
			size, _ := itemMap["size"].(float64)
			isDir, _ := itemMap["is_dir"].(bool)

			if isDir {
				parts = append(parts, name+"/")
			} else if size > 0 {
				parts = append(parts, fmt.Sprintf("%s (%s)", name, formatBytes(int64(size))))
			} else if name != "" {
				parts = append(parts, name)
			}
		}
		if len(parts) == 0 {
			return "(空列表)"
		}
		return strings.Join(parts, " / ")

	case "count":
		if count, ok := obj["count"].(float64); ok {
			return fmt.Sprintf("%d", int(count))
		}
		return "?"

	case "error":
		if err, ok := obj["error"].(string); ok {
			return err
		}
		return ""

	default:
		// 通用字段提取
		val, exists := obj[field]
		if !exists {
			return fmt.Sprintf("(无字段:%s)", field)
		}
		return fmt.Sprintf("%v", val)
	}
}

// formatBytes 将字节数转为人类可读的大小。
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// sleepDelay 按 (DelayMs / speed) 等待，遇 ctx 取消立即返回。
// speed == 0 时不等待（零延迟）。
func sleepDelay(ctx context.Context, delayMs int, speed float64) error {
	if delayMs <= 0 || speed == 0 {
		// 检查 ctx 即使在零延迟场景（避免被 hang 住）
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	d := time.Duration(float64(delayMs)/speed) * time.Millisecond
	if d <= 0 {
		d = time.Millisecond // 最小 1ms
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// isErrorStatus 判断 stream_status 事件 data 是否表示错误。
func isErrorStatus(data map[string]interface{}) bool {
	if data == nil {
		return false
	}
	if t, ok := data["type"].(string); ok && t == "error" {
		return true
	}
	if s, ok := data["status"].(string); ok && s == "error" {
		return true
	}
	return false
}

// sendCorruptChunk 直接向 writer 写入一段非 JSON 的 SSE 数据。
// 不走 sendAndCache 是因为它会触发 JSON 编码。
func sendCorruptChunk(w http.ResponseWriter) {
	// 写入 "data: NOT-JSON\n\n" —— 前端 useAgent 解析器会 catch 错误并丢弃
	_, _ = w.Write([]byte("data: NOT-JSON\n\n"))
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
