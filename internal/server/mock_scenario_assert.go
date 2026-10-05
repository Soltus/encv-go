package server

// mock_scenario_assert.go —— 剧本断言（vNext Round 3）
//
// 为什么要有这个文件：
//
// 旧剧本是"按延迟吐预录文案的播放器"：
//   - 真实工具执行了，但结果**不校验** ⇒ 跑完什么都证明不了
//   - realExecutor 缺失时回退**硬编码假数据** ⇒ 假数据在真实链路里复活
//   - 剧本只有"看起来在跑流程"，没有任何"跑对了"的证据
//
// 这就是用户评价的"自欺欺人的垃圾剧本"。禁用 mock 只是把垃圾藏起来，
// 本文件把剧本升级为「可执行验收剧本」：tool_call 可声明 expect 断言，
// 引擎拿到**真实**结果后逐条校验，失败立即记录并推 mock_assert_failed 事件。
// 断言结果才是剧本唯一的可信输出；静态文案只是演示皮。

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// 支持的断言算子。
const (
	OpExists    = "exists"     // 路径存在
	OpNotExists = "not_exists" // 路径不存在
	OpNonEmpty  = "non_empty"  // 非空（字符串/数组/对象长度 > 0，数字 != 0）
	OpEq        = "eq"         // 等于（数字按数值比较）
	OpNeq       = "neq"        // 不等于
	OpGt        = "gt"         // 大于
	OpGte       = "gte"        // 大于等于
	OpLt        = "lt"         // 小于
	OpLte       = "lte"        // 小于等于
	OpContains  = "contains"   // 字符串包含子串 / 数组含元素 / 对象含 key
)

// ValidOp 报告算子是否为已知算子。
func ValidOp(op string) bool {
	switch op {
	case OpExists, OpNotExists, OpNonEmpty, OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpContains:
		return true
	}
	return false
}

// Assertion 单条断言：对真实工具结果（JSON）的某个路径做检查。
//
// YAML 形态（挂在 tool_call 事件下）：
//
//   - type: tool_call
//     data: { id: call_files1, name: list_files, args: {...}, execute_real: true }
//     expect:
//   - path: "$.items"
//     op: non_empty
//     message: "真实挂载点下必须能列出条目"
//   - path: "$.count"
//     op: gt
//     value: 0
type Assertion struct {
	Path    string `yaml:"path" json:"path"`
	Op      string `yaml:"op" json:"op"`
	Value   any    `yaml:"value,omitempty" json:"value,omitempty"`
	Message string `yaml:"message,omitempty" json:"message,omitempty"`
}

// AssertionResult 单条断言的执行结果。
type AssertionResult struct {
	Assertion Assertion `json:"assertion"`
	Passed    bool      `json:"passed"`
	Actual    any       `json:"actual,omitempty"`
	Detail    string    `json:"detail,omitempty"`
}

// ScenarioAssertionFailure 一次剧本运行中的断言失败记录（供跑批与证据采集使用）。
type ScenarioAssertionFailure struct {
	ScenarioID string            `json:"scenarioId"`
	ToolCallID string            `json:"toolCallId"`
	ToolName   string            `json:"toolName"`
	Results    []AssertionResult `json:"results"`
}

// Evaluate 对已解码的 JSON 根对象执行本断言。
//
// 未知算子 / 路径取不到 / 类型不匹配 一律判失败（fail closed）：
// 断言写错必须暴露，不能因为"算不出来"就当成通过。
func (a Assertion) Evaluate(root any) AssertionResult {
	res := AssertionResult{Assertion: a}

	if !ValidOp(a.Op) {
		res.Passed = false
		res.Detail = fmt.Sprintf("unknown op %q (want one of %s)", a.Op,
			strings.Join([]string{OpExists, OpNotExists, OpNonEmpty, OpEq, OpNeq, OpGt, OpGte, OpLt, OpLte, OpContains}, ","))
		return res
	}

	val, found := resolveJSONPath(root, a.Path)
	res.Actual = val

	switch a.Op {
	case OpExists:
		res.Passed = found
		if !found {
			res.Detail = "path not found"
		}

	case OpNotExists:
		res.Passed = !found
		if found {
			res.Detail = "path exists but must not"
		}

	case OpNonEmpty:
		res.Passed = found && nonEmpty(val)
		if !res.Passed {
			res.Detail = "value is empty or missing"
		}

	case OpEq:
		res.Passed = found && looseEqual(val, a.Value)
		if !res.Passed {
			res.Detail = fmt.Sprintf("want == %v", a.Value)
		}

	case OpNeq:
		res.Passed = !found || !looseEqual(val, a.Value)
		if !res.Passed {
			res.Detail = fmt.Sprintf("want != %v", a.Value)
		}

	case OpGt, OpGte, OpLt, OpLte:
		actual, okA := toFloat(val)
		want, okB := toFloat(a.Value)
		if !okA || !okB {
			res.Passed = false
			res.Detail = fmt.Sprintf("non-numeric operand (actual=%v want=%v)", val, a.Value)
			return res
		}
		switch a.Op {
		case OpGt:
			res.Passed = actual > want
		case OpGte:
			res.Passed = actual >= want
		case OpLt:
			res.Passed = actual < want
		case OpLte:
			res.Passed = actual <= want
		}
		if !res.Passed {
			res.Detail = fmt.Sprintf("want %s %v", a.Op, want)
		}

	case OpContains:
		res.Passed = found && containsVal(val, a.Value)
		if !res.Passed {
			res.Detail = fmt.Sprintf("want contains %v", a.Value)
		}
	}

	if res.Passed && res.Detail == "" {
		res.Detail = "ok"
	}
	return res
}

// FailedResults 返回一批结果中失败的那些。
func FailedResults(rs []AssertionResult) []AssertionResult {
	out := make([]AssertionResult, 0, len(rs))
	for _, r := range rs {
		if !r.Passed {
			out = append(out, r)
		}
	}
	return out
}

// resolveJSONPath 极简 JSONPath：`$.a.b`、`$.items[0].name`、`$.count`。
//
// 不引入第三方依赖：剧本断言只需要这几类取值。取不到返回 found=false。
func resolveJSONPath(root any, path string) (any, bool) {
	if root == nil {
		return nil, false
	}
	p := strings.TrimSpace(path)
	if p == "" || p == "$" {
		return root, true
	}
	p = strings.TrimPrefix(p, "$")
	p = strings.TrimPrefix(p, ".")

	cur := root
	for p != "" {
		// 跳过前导/连续的点（"a..b" 或 "$.count.x" 走到 ".x" 时）
		// —— 否则 seg 取到空串而 p 不前进 ⇒ 死循环（Round 3 首次运行就撞到）。
		if strings.HasPrefix(p, ".") {
			p = p[1:]
			continue
		}
		if strings.HasPrefix(p, "[") {
			end := strings.Index(p, "]")
			if end < 0 {
				return nil, false
			}
			idx, err := strconv.Atoi(strings.TrimSpace(p[1:end]))
			if err != nil {
				return nil, false
			}
			list, ok := cur.([]any)
			if !ok {
				return nil, false
			}
			if idx < 0 || idx >= len(list) {
				return nil, false
			}
			cur = list[idx]
			p = p[end+1:]
			p = strings.TrimPrefix(p, ".")
			continue
		}

		seg := p
		if i := strings.IndexAny(p, ".["); i >= 0 {
			seg = p[:i]
			p = p[i:]
		} else {
			p = ""
		}
		if seg == "" {
			continue
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[seg]
		if !ok {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

func nonEmpty(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case string:
		return len(strings.TrimSpace(t)) > 0
	case []any:
		return len(t) > 0
	case map[string]any:
		return len(t) > 0
	case bool:
		return t
	case float64:
		return t != 0
	case int:
		return t != 0
	}
	return true
}

// toFloat 把断言里的数字（JSON float64 / YAML int / 可解析字符串）归一成 float64。
func toFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	}
	return 0, false
}

// looseEqual 宽松相等：两边都能转数字时按数值比，否则字符串化后比。
func looseEqual(a, b any) bool {
	if fa, okA := toFloat(a); okA {
		if fb, okB := toFloat(b); okB {
			return fa == fb
		}
	}
	if reflect.DeepEqual(a, b) {
		return true
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func containsVal(v any, want any) bool {
	switch t := v.(type) {
	case string:
		return strings.Contains(t, fmt.Sprint(want))
	case []any:
		for _, e := range t {
			if looseEqual(e, want) {
				return true
			}
		}
		return false
	case map[string]any:
		_, ok := t[fmt.Sprint(want)]
		return ok
	}
	return false
}

// LastFailures 返回最近一次 Run 累积的断言失败记录。
//
// 跑批 / 证据采集读这里：为空 = 真实结果全部符合预期；非空 = 剧本实际没跑对。
func (e *MockEngine) LastFailures() []ScenarioAssertionFailure {
	e.assertMu.Lock()
	defer e.assertMu.Unlock()
	out := make([]ScenarioAssertionFailure, len(e.lastFailures))
	copy(out, e.lastFailures)
	return out
}

// ResetFailures 清空断言失败记录（Run 开头调用）。
func (e *MockEngine) ResetFailures() {
	e.assertMu.Lock()
	e.lastFailures = nil
	e.assertMu.Unlock()
}

func (e *MockEngine) recordFailure(f ScenarioAssertionFailure) {
	e.assertMu.Lock()
	e.lastFailures = append(e.lastFailures, f)
	e.assertMu.Unlock()
}

// applyAssertions 在拿到真实工具结果后逐条执行断言。
//
// 只在**成功**拿到真实结果时调用：工具本身报错由 tool_result 的 isError 表达，
// 不需要再用断言粉饰。断言失败 → 记录 + 推 mock_assert_failed 事件（DevLogs 可见）。
func (e *MockEngine) applyAssertions(
	scenarioID, toolCallID, toolName, resultJSON string,
	expect []Assertion,
	emitEvent func(ev MockEvent, stepIdx, evIdx int),
	stepIdx, evIdx int,
) {
	if len(expect) == 0 {
		return
	}

	var root any
	if err := json.Unmarshal([]byte(resultJSON), &root); err != nil {
		f := ScenarioAssertionFailure{
			ScenarioID: scenarioID,
			ToolCallID: toolCallID,
			ToolName:   toolName,
			Results: []AssertionResult{{
				Assertion: Assertion{Path: "$", Op: OpExists},
				Passed:    false,
				Detail:    "result is not valid JSON: " + err.Error(),
			}},
		}
		e.recordFailure(f)
		emitEvent(MockEvent{Type: "mock_assert_failed", Data: map[string]interface{}{
			"scenario": scenarioID, "toolCallId": toolCallID, "toolName": toolName,
			"failed": 1, "results": f.Results,
		}}, stepIdx, evIdx)
		return
	}

	results := make([]AssertionResult, 0, len(expect))
	for _, a := range expect {
		results = append(results, a.Evaluate(root))
	}
	fails := FailedResults(results)
	if len(fails) == 0 {
		return
	}

	e.recordFailure(ScenarioAssertionFailure{
		ScenarioID: scenarioID,
		ToolCallID: toolCallID,
		ToolName:   toolName,
		Results:    fails,
	})
	emitEvent(MockEvent{Type: "mock_assert_failed", Data: map[string]interface{}{
		"scenario": scenarioID, "toolCallId": toolCallID, "toolName": toolName,
		"failed": len(fails), "results": fails,
	}}, stepIdx, evIdx)
}
