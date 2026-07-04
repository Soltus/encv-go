// internal/server/mock_scenario_schema.go
//
// 剧本 YAML schema 定义。
//
// 核心铁律（用户原话反复强调）：
//   1. ❌ YAML 严禁出现 `tool_result` 事件 —— 由 MockEngine 在 tool_call 后自动生成
//   2. ❌ YAML 严禁出现模板语法 `{{ ... }}` 或 `{% ... %}` —— text_delta 必须是静态文案
//   3. ❌ YAML 严禁硬编码路径、文件名、文件大小、计数、错误文本 —— 这些必须从真实工具拿
//   4. ❌ YAML 严禁 user_text / free-form text 字段 —— 剧本 = 预设选项 chip
//   5. ✅ 工具参数（args）是"过滤条件"（如 ext: ".mp4"），不是"数据"
//
// 校验函数 Validate() 在 loader.LoadAll() 阶段被调用，违反任一铁律 → 拒绝该文件。
package server

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// unmarshalYAML 是 schema 测试用的便利包装。
// 任何 yaml.v3 反序列化错误（格式错、字段类型错）都会冒泡。
func unmarshalYAML(data []byte, v interface{}) error {
	return yaml.Unmarshal(data, v)
}

// ════════════════════════════════════════════════════════════════
// 硬编码数据红线正则（CI 必备 — 见 mock_scenario_schema_test.go）
// ════════════════════════════════════════════════════════════════

// 禁止的硬编码模式：
//   - 路径形如 `dir/dir/file.ext`（如 Movies/2024/big.mp4）
//   - 常见媒体/数据文件扩展名
//   - 文件大小数字（"524MB" / "11.4 KB" / "268000000 bytes"）
//   - 计数数字（"0 个匹配" / "3 个文件" / "5 个结果"）
//   - 错误信息文本（"ERROR: xxx" / "失败: xxx"）
//   - 模板占位符
var (
	// 路径模式：至少 2 层目录 + 文件名（如 Movies/2024/big.mp4 / 01-plain-media/video/sample.mp4）
	// 注意：故意不匹配单个文件名（如 notes.txt），因为 chip label 可能含单文件名词
	// 区分依据：是否含路径分隔符（`/` 或 `\`）+ 至少 2 个目录层级
	reHardcodedPath = regexp.MustCompile(`(?:[A-Za-z0-9_.\-]+/){1,}[A-Za-z0-9_.\-]+\.(?:mp4|mkv|avi|mov|flv|webm|ts|m4v|wmv|json|log|txt|bin|md|csv|xml|yaml|yml)`)

	// 文件大小（带单位）："524MB" / "11.4 KB" / "1.2 GB" / "268000000 bytes"
	reHardcodedSize = regexp.MustCompile(`\b\d+(?:\.\d+)?\s*(?:MB|KB|GB|TB|bytes?|B)\b`)

	// 字节数（裸数字 + 出现在 size 上下文）：超过 4 位纯数字 + 邻近 size/length
	// 例外：timestamp / version / ID 允许纯数字，故本规则保守一些
	// 不放：避免误伤

	// 计数模式（中文）："0 个匹配" / "3 个文件" / "5 个结果"
	reHardcodedCount = regexp.MustCompile(`\b\d+\s*个(?:文件|匹配|结果|条目|记录|项|子目录|目录)`)

	// 错误信息文本
	reHardcodedError = regexp.MustCompile(`(?:ERROR|失败|错误)[:：]\s*\S+`)

	// 模板占位符（Go template / Jinja / 旧版 {%id%}）
	reTemplateSyntax = regexp.MustCompile(`\{\{|\{\%`)

	// 工具结果事件关键字（出现在 events.type 字段）
	reToolResultEvent = regexp.MustCompile(`(?i)\btool_result\b`)

	// user_text / free-form input 字段（仅允许在 mock_branch_choice 等结构化场景出现）
	reFreeFormInput = regexp.MustCompile(`(?i)\b(?:user_text|userText|user_input|userInput|free_text|freeText|raw_text|rawText)\b`)

	// 真实存在的扩展名集合（用于 size 数字 + 扩展名组合判定）
	commonMediaExts = map[string]bool{
		"mp4": true, "mkv": true, "avi": true, "mov": true,
		"flv": true, "webm": true, "ts": true, "m4v": true,
		"wmv": true, "json": true, "log": true, "txt": true,
		"bin": true, "md": true, "csv": true, "xml": true,
	}
)

// ════════════════════════════════════════════════════════════════
// 顶层 Schema
// ════════════════════════════════════════════════════════════════

// LoadedScenario 是单个 YAML 剧本的反序列化目标。
//
// 与 MockScenario 的区别：
//   - 字段名 snake_case（YAML 习惯）
//   - 事件类型是 string 而非枚举（YAML 灵活）
//   - 不含 tool_result 事件（运行时由 MockEngine 注入）
//   - 不含 legacy {%id%} 模板（用 collectedResults 已被新规则替代）
type LoadedScenario struct {
	// ID 全局唯一，必填
	ID string `yaml:"id"`
	// Description 可选，给运维/开发看的注释
	Description string `yaml:"description,omitempty"`
	// ExactMatch 精确匹配字符串（与原 MockScenario.ExactMatch 一致）
	ExactMatch string `yaml:"exact_match,omitempty"`
	// Keywords 关键词列表，任一命中即触发
	Keywords []string `yaml:"keywords,omitempty"`
	// Regex 正则匹配模式
	Regex string `yaml:"regex,omitempty"`
	// Steps 步骤序列，必填，至少 1 个
	Steps []YAMLStep `yaml:"steps"`
	// Presets 初始预设输入按钮（chip），可选
	Presets []YAMLPreset `yaml:"presets,omitempty"`
	// Branches 分支选项（用于 mock_branch_choice 事件），可选
	Branches []YAMLBranch `yaml:"branches,omitempty"`
	// Rounds 多轮总轮数（v2 兼容字段），可选
	Rounds int `yaml:"rounds,omitempty"`
}

// YAMLPreset 是 chip 按钮定义。
type YAMLPreset struct {
	ID       string `yaml:"id"`
	Label    string `yaml:"label"`
	UserText string `yaml:"user_text,omitempty"` // ⚠️ 严禁自由文本 → 校验会拒绝
	Icon     string `yaml:"icon,omitempty"`
	Tooltip  string `yaml:"tooltip,omitempty"`
}

// YAMLBranch 是分支选项（v2 兼容字段）。
type YAMLBranch struct {
	ID              string           `yaml:"id"`
	Label           string           `yaml:"label"`
	Description     string           `yaml:"description,omitempty"`
	Icon            string           `yaml:"icon,omitempty"`
	TriggerKeywords []string         `yaml:"trigger_keywords,omitempty"`
	TriggerRegex    string           `yaml:"trigger_regex,omitempty"`
	InitialStepID   string           `yaml:"initial_step_id,omitempty"`
	OnMatch         *LoadedScenario  `yaml:"on_match,omitempty"` // 子剧本（YAML 内联）
}

// YAMLStep 是单个步骤。
//
// 设计要点：
//   - 每个 step 有 id 便于在 branch_pick 时定位
//   - events 是该步骤推流的事件序列
//   - when_tool_error 可选，指定当某 tool_call 失败时的备选文案
//   - branch_choice 标记此 step 是分支选择点
type YAMLStep struct {
	ID             string                 `yaml:"id"`
	Events         []YAMLEvent            `yaml:"events"`
	DelayMs        int                    `yaml:"delay_ms,omitempty"`
	BranchID       string                 `yaml:"branch_id,omitempty"`
	RoundIdx       int                    `yaml:"round_idx,omitempty"`
	PauseForUser   bool                   `yaml:"pause_for_user,omitempty"`
	BranchChoice   bool                   `yaml:"branch_choice,omitempty"`
	WhenToolError  map[string]YAMLStepErr `yaml:"when_tool_error,omitempty"`
	SetContext     map[string]any         `yaml:"set_context,omitempty"`
	UseContext     []string               `yaml:"use_context,omitempty"`
}

// YAMLStepErr 定义当工具失败时的回退配置。
type YAMLStepErr struct {
	TextDelta string `yaml:"text_delta,omitempty"` // 失败时显示的 UI 文案
	SkipRest  bool   `yaml:"skip_rest,omitempty"`  // 失败时是否跳过后续事件
}

// YAMLEvent 是单个事件。
//
// 严禁 type == "tool_result"（loader 校验拒绝）。
type YAMLEvent struct {
	Type string                 `yaml:"type"`
	Data map[string]interface{} `yaml:"data,omitempty"`
}

// YAMLBranchOption 是 mock_branch_choice 事件的 options 列表中的单个选项。
//
// 字段：
//   - ID    全局唯一（与 step 内其他 option 不冲突）
//   - Label 用户可见文字
//   - Icon  可选 emoji（如 "🎚️"）
//   - Keywords  可选（v2 兼容字段，本 spec 不依赖）
//   - NextStepID  选此项后跳转的 step.id（v2 兼容；新剧本可在 step 内用 BranchID 路由）
type YAMLBranchOption struct {
	ID         string   `yaml:"id"`
	Label      string   `yaml:"label"`
	Icon       string   `yaml:"icon,omitempty"`
	Keywords   []string `yaml:"keywords,omitempty"`
	NextStepID string   `yaml:"next_step_id,omitempty"`
}

// ════════════════════════════════════════════════════════════════
// 校验
// ════════════════════════════════════════════════════════════════

// Validate 校验剧本 schema 与铁律。返回第一个错误即终止。
//
// 校验规则（按 T1.2 spec）：
//   1. id 必填且非空
//   2. steps 必填且至少 1 个
//   3. 每个 step.events 必填且至少 1 个
//   4. mock_branch_choice 事件的 options 至少 2 个（由具体 step 校验，下面有独立方法）
//   5. text_delta.text 不含 `{{` 或 `{%`
//   6. tool_result 事件 → 拒绝（铁律 1）
//   7. 硬编码路径/大小/计数/错误 → 拒绝（铁律 3）
//   8. user_text / free-form input 字段 → 拒绝（铁律 4）
func (s *LoadedScenario) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("scenario: missing id")
	}
	if len(s.Steps) == 0 {
		return fmt.Errorf("scenario %q: steps is empty (must have at least 1 step)", s.ID)
	}

	stepIDs := make(map[string]bool, len(s.Steps))
	for stepIdx, step := range s.Steps {
		// step id 唯一性
		if step.ID != "" {
			if stepIDs[step.ID] {
				return fmt.Errorf("scenario %q: duplicate step id %q (at step #%d)", s.ID, step.ID, stepIdx)
			}
			stepIDs[step.ID] = true
		}
		if err := step.Validate(s.ID, stepIdx); err != nil {
			return err
		}
	}

	// 校验 Presets 不含自由文本
	for presetIdx, p := range s.Presets {
		if err := p.Validate(s.ID, presetIdx); err != nil {
			return err
		}
	}

	// 校验 Branches 内联子剧本
	for branchIdx, b := range s.Branches {
		if err := b.Validate(s.ID, branchIdx); err != nil {
			return err
		}
	}

	return nil
}

// Validate 校验单 step 的事件序列。
func (step *YAMLStep) Validate(scenarioID string, stepIdx int) error {
	if len(step.Events) == 0 {
		return fmt.Errorf("scenario %q step #%d: events is empty (must have at least 1 event)", scenarioID, stepIdx)
	}

	for evIdx, ev := range step.Events {
		if err := ev.Validate(scenarioID, step.ID, stepIdx, evIdx); err != nil {
			return err
		}
	}
	return nil
}

// Validate 校验单事件。
//
// 核心检查：
//   - 禁止 type == "tool_result"
//   - text_delta.text 不能含模板语法
//   - mock_branch_choice 必须有 options（>=2）
//   - 任何字符串字段不能含硬编码路径/大小/计数/错误
func (ev *YAMLEvent) Validate(scenarioID, stepID string, stepIdx, evIdx int) error {
	ctx := fmt.Sprintf("scenario %q step #%d ev #%d", scenarioID, stepIdx, evIdx)
	if stepID != "" {
		ctx = fmt.Sprintf("scenario %q step %q ev #%d", scenarioID, stepID, evIdx)
	}

	// 铁律 1：禁止 tool_result
	if ev.Type == "tool_result" {
		return fmt.Errorf("%s: type=%q is FORBIDDEN (tool_result is auto-generated by MockEngine from real tool execution; do NOT declare in YAML)", ctx, ev.Type)
	}

	// 遍历 data map 检查每个字符串值
	for k, v := range ev.Data {
		switch val := v.(type) {
		case string:
			if err := validateStringField(ctx, ev.Type, k, val); err != nil {
				return err
			}
		case map[string]interface{}:
			// 嵌套对象 → 递归
			for nk, nv := range val {
				if ns, ok := nv.(string); ok {
					if err := validateStringField(ctx, ev.Type, k+"."+nk, ns); err != nil {
						return err
					}
				}
			}
		case []interface{}:
			// 数组 → 检查每个元素
			for arrIdx, item := range val {
				if s, ok := item.(string); ok {
					if err := validateStringField(ctx, ev.Type, fmt.Sprintf("%s[%d]", k, arrIdx), s); err != nil {
						return err
					}
				} else if m, ok := item.(map[string]interface{}); ok {
					// 数组里的对象 → 递归字符串字段
					for nk, nv := range m {
						if ns, ok := nv.(string); ok {
							if err := validateStringField(ctx, ev.Type, fmt.Sprintf("%s[%d].%s", k, arrIdx, nk), ns); err != nil {
								return err
							}
						}
					}
				}
			}
		}
	}

	// mock_branch_choice 必带 options 数组（>=2）
	if ev.Type == "mock_branch_choice" {
		opts, ok := ev.Data["options"].([]interface{})
		if !ok || len(opts) < 2 {
			return fmt.Errorf("%s: mock_branch_choice event must have data.options with at least 2 entries (got %d)", ctx, len(opts))
		}
		for optIdx, raw := range opts {
			optMap, ok := raw.(map[string]interface{})
			if !ok {
				return fmt.Errorf("%s: mock_branch_choice options[%d] must be an object", ctx, optIdx)
			}
			// 校验每个 option 字段
			for k, v := range optMap {
				if s, ok := v.(string); ok {
					if err := validateStringField(ctx, ev.Type, fmt.Sprintf("options[%d].%s", optIdx, k), s); err != nil {
						return err
					}
				}
			}
			// 必填 id 和 label
			if _, ok := optMap["id"].(string); !ok {
				return fmt.Errorf("%s: mock_branch_choice options[%d].id is required", ctx, optIdx)
			}
			if _, ok := optMap["label"].(string); !ok {
				return fmt.Errorf("%s: mock_branch_choice options[%d].label is required", ctx, optIdx)
			}
		}
	}

	// tool_call 校验：name / id 必填，args 必填（即使是 {}）
	if ev.Type == "tool_call" {
		if _, ok := ev.Data["name"].(string); !ok {
			return fmt.Errorf("%s: tool_call event must have data.name (string)", ctx)
		}
		if _, ok := ev.Data["id"].(string); !ok {
			return fmt.Errorf("%s: tool_call event must have data.id (string)", ctx)
		}
		if _, hasArgs := ev.Data["args"]; !hasArgs {
			return fmt.Errorf("%s: tool_call event must have data.args (object, even if empty {})", ctx)
		}
	}

	return nil
}

// validateStringField 校验单个字符串字段。
//
// 检查项（按顺序）：
//   1. 模板语法 `{{` / `{%` → 拒绝
//   2. 硬编码路径（dir/dir/file.ext）→ **仅在非 args 字段拒绝**（args 是用户输入）
//   3. 硬编码文件大小（数字 + 单位）→ 拒绝
//   4. 硬编码计数（"N 个 X"）→ 拒绝
//   5. 硬编码错误信息（"ERROR: ..."）→ 拒绝
//   6. 自由文本字段（user_text 等）→ 拒绝
//
// 豁免：
//   - key 以 "args." 开头（tool_call args 是用户输入的过滤条件，不是剧本硬编码）
//   - key == "scenario" / "branch_id"（元数据）
//   - key == "id"（call ID 或 event ID）
//   - key == "name"（工具名 / 字段名）
//   - key == "type"（事件类型）
//   - key 以 ".icon" 结尾（emoji 通常无危险）
//   - key 以 ".label" 结尾（chip 显示文本）
//   - key 以 ".user_text" 结尾（chip 预设消息，由 preset.Validate 单独校验）
//   - key == "user_text"（在 YAMLPreset 顶层，不在 event data 内）
func validateStringField(ctx, evType, key, value string) error {
	if value == "" {
		return nil // 空字符串无意义跳过
	}

	// 字段名本身若是 "user_text" / "userText" 等自由文本字段 → 拒绝
	// 注意：preset.user_text 走独立路径（YAMLPreset.Validate），不在此处处理
	//
	// 例外：mock_presets 事件 data.presets[].user_text 是 chip 点击后发送的预定义消息，
	// 是剧本预设的一部分（不是用户自由输入），允许出现。
	// 区分依据：evType == "mock_presets" 且 key 含 ".user_text" → 豁免
	isMockPresetsUserText := evType == "mock_presets" && (key == "user_text" || strings.HasSuffix(key, ".user_text"))
	if reFreeFormInput.MatchString(key) && !isMockPresetsUserText {
		return fmt.Errorf("%s: field %q uses forbidden free-form input key %q (scenarios MUST use preset chips, not user text input)",
			ctx, key, key)
	}

	// 模板语法
	if reTemplateSyntax.MatchString(value) {
		return fmt.Errorf("%s: field %q contains template syntax %q (FORBIDDEN — scenarios must use static text; tool_result is auto-generated, no templates needed)",
			ctx, key, firstMatch(value, reTemplateSyntax))
	}

	// 硬编码路径 — 豁免 args.* 字段（用户输入）
	if !isExemptFromPathCheck(key) {
		if m := reHardcodedPath.FindString(value); m != "" {
			return fmt.Errorf("%s: field %q contains hardcoded path %q (FORBIDDEN — paths must come from real tool results, NOT hardcoded in YAML)",
				ctx, key, m)
		}
	}

	// 硬编码文件大小
	if m := reHardcodedSize.FindString(value); m != "" {
		return fmt.Errorf("%s: field %q contains hardcoded size %q (FORBIDDEN — sizes must come from real tool results, NOT hardcoded in YAML)",
			ctx, key, m)
	}

	// 硬编码计数
	if m := reHardcodedCount.FindString(value); m != "" {
		return fmt.Errorf("%s: field %q contains hardcoded count %q (FORBIDDEN — counts must come from real tool results, NOT hardcoded in YAML)",
			ctx, key, m)
	}

	// 硬编码错误信息
	if m := reHardcodedError.FindString(value); m != "" {
		return fmt.Errorf("%s: field %q contains hardcoded error text %q (FORBIDDEN — error messages must come from real tool failure, NOT hardcoded in YAML)",
			ctx, key, m)
	}

	return nil
}

// isExemptFromPathCheck 判断 key 是否豁免硬编码路径检查。
//
// 豁免规则：
//   - "args.*"：tool_call args 是用户输入的过滤条件
//   - "scenario" / "branch_id"：元数据
//   - "id" / "name" / "type"：标识符 / 字段名
//   - "*.icon" / "*.label"：UI 元数据（emoji / chip 标签）
//   - "preset.user_text" 单独豁免（preset 走自己的 Validate）
func isExemptFromPathCheck(key string) bool {
	if key == "" {
		return false
	}
	// 字段名豁免
	exemptExact := map[string]bool{
		"args":             true,
		"scenario":         true,
		"branch_id":        true,
		"id":               true,
		"name":             true,
		"type":             true,
		"status":           true,
		"finishReason":     true,
		"kind":             true,
		"execute_real":     true,
		"auto_run":         true,
		"needsConfirm":     true,
		"phase":            true,
		"options":          true,
		"label":            true,
		"icon":             true,
		"tooltip":          true,
		"user_text":        true, // preset.user_text 单独处理
		"next_step_id":     true,
		"initial_step_id":  true,
		"trigger_regex":    true,
		"trigger_keywords": true,
	}
	if exemptExact[key] {
		return true
	}
	// 前缀豁免
	for _, prefix := range []string{"args.", "options[", ".icon", ".label", ".tooltip", ".user_text", ".keywords", ".next_step_id"} {
		if strings.HasPrefix(key, prefix) {
			return true
		}
		// 也匹配嵌套的 args.foo[0].bar
		if strings.Contains(key, prefix) {
			// 例: "options[0].label" 含 "label" → 豁免
			if prefix == ".label" || prefix == ".icon" {
				return true
			}
		}
	}
	return false
}

// isPresetUserTextKey 保留 — 当前未使用（user_text 在 event data 内必拒，
// preset.user_text 走 YAMLPreset.Validate 单独校验），保留供未来扩展。
//
// Deprecated: 不再调用此函数。
func isPresetUserTextKey(key string) bool {
	return key == "user_text"
}

// Validate 校验 preset。
//
// 注意：preset.UserText 是 chip 点击后发送的"预定义消息"，是**剧本预设**的一部分，
// 不是用户自由输入（用户已经通过点击 chip 做出了"预设选择"），允许出现。
// 唯一禁止：text_delta / mock_branch_choice 等事件 data 里塞 user_text。
func (p *YAMLPreset) Validate(scenarioID string, idx int) error {
	ctx := fmt.Sprintf("scenario %q preset #%d", scenarioID, idx)
	if p.ID == "" {
		return fmt.Errorf("%s: preset.id is required", ctx)
	}
	if p.Label == "" {
		return fmt.Errorf("%s: preset.label is required", ctx)
	}
	// user_text 是 chip 预设消息（不是硬编码数据）— 仅做基本非空校验
	// 不调 validateStringField（那里有 user_text 字段名黑名单会误伤）
	if p.UserText != "" {
		// 校验模板语法 / 硬编码路径 — 仍然禁（消息文本是用户看到的，预设消息也该是干净的）
		if reTemplateSyntax.MatchString(p.UserText) {
			return fmt.Errorf("%s: preset.user_text contains template syntax (FORBIDDEN)", ctx)
		}
	}
	return nil
}

// Validate 校验 branch。
func (b *YAMLBranch) Validate(scenarioID string, idx int) error {
	ctx := fmt.Sprintf("scenario %q branch #%d", scenarioID, idx)
	if b.ID == "" {
		return fmt.Errorf("%s: branch.id is required", ctx)
	}
	if b.Label == "" {
		return fmt.Errorf("%s: branch.label is required", ctx)
	}
	if b.TriggerRegex != "" {
		if _, err := regexp.Compile(b.TriggerRegex); err != nil {
			return fmt.Errorf("%s: branch.trigger_regex %q does not compile: %v", ctx, b.TriggerRegex, err)
		}
	}
	if b.OnMatch != nil {
		if err := b.OnMatch.Validate(); err != nil {
			return fmt.Errorf("%s: branch.on_match invalid: %w", ctx, err)
		}
	}
	return nil
}

// firstMatch 返回 value 中第一个匹配 re 的子串。
func firstMatch(value string, re *regexp.Regexp) string {
	return re.FindString(value)
}

// ════════════════════════════════════════════════════════════════
// ConvertToMockScenario — 转换为运行时 MockScenario
// ════════════════════════════════════════════════════════════════

// ConvertToMockScenario 把 LoadedScenario 转为 MockEngine 用的 MockScenario。
//
// 转换规则：
//   - Steps 直接映射为 []MockStep
//   - 每个 YAMLEvent.Type 转为 MockEvent.Type
//   - 触发匹配字段（ExactMatch / Keywords / Regex）直接赋值
//   - **tool_result 事件在转换前已被 Validate 拒绝**，所以此处不会遇到
//   - **自动注入 tool_result 生成逻辑**：当一个 step 含 tool_call 时，
//     转换后给该 step 追加一个自动 tool_result 事件占位（实际值由 executor 填）
func (s *LoadedScenario) ConvertToMockScenario() *MockScenario {
	sc := &MockScenario{
		ID:          s.ID,
		Description: s.Description,
		ExactMatch:  s.ExactMatch,
		Keywords:    s.Keywords,
		Regex:       s.Regex,
		Steps:       make([]MockStep, 0, len(s.Steps)),
		Rounds:      s.Rounds,
		TotalRounds: s.Rounds, // alias：spec §三.5 兼容性
	}

	// RoundContext（如未在 YAML 显式声明则保持 nil，与原 Go 字面量零值一致）

	// Presets
	for _, p := range s.Presets {
		sc.Presets = append(sc.Presets, MockPreset{
			ID:       p.ID,
			Label:    p.Label,
			UserText: p.UserText,
			Icon:     p.Icon,
			Tooltip:  p.Tooltip,
		})
	}

	// Branches（v2 兼容）
	for _, b := range s.Branches {
		branch := Branch{
			ID:              b.ID,
			Label:           b.Label,
			Description:     b.Description,
			Icon:            b.Icon,
			TriggerKeywords: b.TriggerKeywords,
			TriggerRegex:    b.TriggerRegex,
			InitialStepID:   b.InitialStepID,
		}
		if b.OnMatch != nil {
			branch.OnMatch = b.OnMatch.ConvertToMockScenario()
		}
		sc.Branches = append(sc.Branches, branch)
	}

	// Steps（含自动 tool_result 注入）
	for _, ys := range s.Steps {
		step := MockStep{
			DelayMs:      ys.DelayMs,
			BranchID:     ys.BranchID,
			RoundIdx:     ys.RoundIdx,
			PauseForUser: ys.PauseForUser,
			BranchChoice: ys.BranchChoice,
		}
		// 转换每个 event
		for _, ye := range ys.Events {
			step.Events = append(step.Events, MockEvent{
				Type: ye.Type,
				Data: ye.Data,
			})
			// ⚠️ 关键：如果遇到 tool_call，立即追加一个空 tool_result 标记事件
			// 真正的 result 会在 executor 阶段填入。
			// 标记方式：Data 包含 __yaml_auto_tool_result = true
			if ye.Type == "tool_call" {
				callID, _ := ye.Data["id"].(string)
				step.Events = append(step.Events, MockEvent{
					Type: "tool_result",
					Data: map[string]interface{}{
						"id":                    callID,
						"__yaml_auto_generated": true, // 标记：executor 必须覆盖
					},
				})
			}
		}
		sc.Steps = append(sc.Steps, step)
	}

	return sc
}
