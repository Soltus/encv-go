package server

// mock_scenario_assert_round3_test.go —— vNext Round 3 回归锁
//
// 锁定：剧本不再是"预录文案播放器"，而是可执行验收剧本：
//   1. 真实工具结果必须过 expect 断言，失败要记录 + 推 mock_assert_failed
//   2. realExecutor 缺失时拒绝回退硬编码假数据
//   3. LastFailures() 是跑批/证据判定"剧本到底跑没跑对"的唯一依据

import (
	"context"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestResolveJSONPath(t *testing.T) {
	root := map[string]any{
		// 真实路径下 root 来自 json.Unmarshal ⇒ 数字是 float64，这里保持一致
		"count": float64(2),
		"items": []any{
			map[string]any{"name": "a.mp4", "size": 100},
			map[string]any{"name": "b.txt"},
		},
		"nested": map[string]any{"deep": map[string]any{"k": "v"}},
		"empty":  "",
	}
	cases := []struct {
		path  string
		want  any
		found bool
	}{
		{"$", root, true},
		{"$.count", float64(2), true},
		{"$.items[0].name", "a.mp4", true},
		{"$.items[1].size", nil, false},
		{"$.nested.deep.k", "v", true},
		{"$.missing", nil, false},
		{"$.items[9]", nil, false},
		{"$.count.x", nil, false},
	}
	for _, c := range cases {
		got, found := resolveJSONPath(root, c.path)
		if found != c.found {
			t.Errorf("path %q: found=%v want %v", c.path, found, c.found)
			continue
		}
		// 用 DeepEqual：取到的值可能是 map/slice（不可直接用 != 比较，会 panic）
		if c.found && !reflect.DeepEqual(got, c.want) {
			t.Errorf("path %q: got %v want %v", c.path, got, c.want)
		}
	}
}

func TestAssertion_Evaluate(t *testing.T) {
	root := map[string]any{
		"count": float64(2),
		"items": []any{map[string]any{"name": "a.mp4"}},
		"title": "hello world",
		"err":   nil,
	}
	cases := []struct {
		name string
		a    Assertion
		want bool
	}{
		{"exists ok", Assertion{Path: "$.count", Op: OpExists}, true},
		{"exists missing", Assertion{Path: "$.nope", Op: OpExists}, false},
		{"not_exists ok", Assertion{Path: "$.nope", Op: OpNotExists}, true},
		{"not_exists fail", Assertion{Path: "$.count", Op: OpNotExists}, false},
		{"non_empty ok", Assertion{Path: "$.title", Op: OpNonEmpty}, true},
		{"non_empty fail", Assertion{Path: "$.err", Op: OpNonEmpty}, false},
		{"eq numeric", Assertion{Path: "$.count", Op: OpEq, Value: 2}, true},
		{"eq numeric fail", Assertion{Path: "$.count", Op: OpEq, Value: 3}, false},
		{"gt", Assertion{Path: "$.count", Op: OpGt, Value: 1}, true},
		{"gte equal", Assertion{Path: "$.count", Op: OpGte, Value: 2}, true},
		{"lt fail", Assertion{Path: "$.count", Op: OpLt, Value: 1}, false},
		{"contains substr", Assertion{Path: "$.title", Op: OpContains, Value: "world"}, true},
		{"contains fail", Assertion{Path: "$.title", Op: OpContains, Value: "zzz"}, false},
		{"items non_empty", Assertion{Path: "$.items", Op: OpNonEmpty}, true},
		{"unknown op fails closed", Assertion{Path: "$.count", Op: "whatever"}, false},
		{"non numeric gt fails", Assertion{Path: "$.title", Op: OpGt, Value: 1}, false},
	}
	for _, c := range cases {
		if got := c.a.Evaluate(root); got.Passed != c.want {
			t.Errorf("%s: passed=%v want %v (detail=%s)", c.name, got.Passed, c.want, got.Detail)
		}
	}
}

func TestAssertion_UnknownOpIsVisible(t *testing.T) {
	res := Assertion{Path: "$.a", Op: "bogus"}.Evaluate(map[string]any{"a": 1})
	if res.Passed {
		t.Fatal("unknown op must fail closed")
	}
	if !strings.Contains(res.Detail, "unknown op") {
		t.Fatalf("detail must name the problem, got %q", res.Detail)
	}
}

// runAssertionScenario 跑一个只含一个 tool_call + auto tool_result 的剧本，
// 返回 engine 的失败记录。fakeResult 是"真实执行器"返回的 JSON。
func runAssertionScenario(t *testing.T, fakeResult string, expect []Assertion) (*MockEngine, *agentSession) {
	t.Helper()
	eng := NewMockEngine()
	eng.SetRealExecutor(func(ctx context.Context, toolName, argsJSON string) (string, error) {
		return fakeResult, nil
	})
	sc := &MockScenario{
		ID: "round3_assert",
		Steps: []MockStep{{DelayMs: 0, Events: []MockEvent{
			{Type: "stream_start", Data: map[string]any{"scenario": "round3_assert"}},
			{Type: "tool_call", Data: map[string]any{
				"id": "c1", "name": "list_files", "args": `{}`,
				"auto_run": true, "execute_real": true,
			}, Expect: expect},
			{Type: "tool_result", Data: map[string]any{"id": "c1", "__yaml_auto_generated": true}},
			{Type: "stream_end", Data: map[string]any{"finishReason": "stop"}},
		}}},
	}
	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()
	if err := eng.Run(context.Background(), s, sess, rec, rec, sc, 10.0, true); err != nil {
		t.Fatalf("Run: %v", err)
	}
	return eng, sess
}

func TestScenarioAssertions_Pass(t *testing.T) {
	eng, sess := runAssertionScenario(t,
		`{"items":[{"name":"a.mp4"}],"count":1}`,
		[]Assertion{{Path: "$.items", Op: OpNonEmpty}, {Path: "$.count", Op: OpGt, Value: 0}},
	)
	if f := eng.LastFailures(); len(f) != 0 {
		t.Fatalf("expected no assertion failures, got %+v", f)
	}
	if ev := findEventOfType(sess, "mock_assert_failed"); ev != nil {
		t.Fatalf("unexpected mock_assert_failed event: %+v", ev)
	}
}

func TestScenarioAssertions_FailIsRecorded(t *testing.T) {
	// 真实工具"成功"返回了空列表 ⇒ 剧本声称的"列出文件"其实没跑对
	eng, sess := runAssertionScenario(t,
		`{"items":[],"count":0}`,
		[]Assertion{{Path: "$.items", Op: OpNonEmpty, Message: "必须有条目"}},
	)
	fails := eng.LastFailures()
	if len(fails) != 1 {
		t.Fatalf("expected 1 failure record, got %d (%+v)", len(fails), fails)
	}
	if fails[0].ScenarioID != "round3_assert" || fails[0].ToolCallID != "c1" {
		t.Fatalf("failure record missing identity: %+v", fails[0])
	}
	if ev := findEventOfType(sess, "mock_assert_failed"); ev == nil {
		t.Fatal("expected mock_assert_failed event to be emitted")
	}
}

func TestScenarioAssertions_NonJSONResultFails(t *testing.T) {
	eng, _ := runAssertionScenario(t, `not-json`, []Assertion{{Path: "$.items", Op: OpExists}})
	if f := eng.LastFailures(); len(f) != 1 {
		t.Fatalf("non-JSON result must be recorded as failure, got %+v", f)
	}
}

func TestScenarioAssertions_NoExpectNoFailure(t *testing.T) {
	// 没写断言的剧本：不产生失败记录（但也不证明任何事 —— 由跑批负责暴露"无断言剧本"）
	eng, _ := runAssertionScenario(t, `{"whatever":1}`, nil)
	if f := eng.LastFailures(); len(f) != 0 {
		t.Fatalf("no expect ⇒ no failures, got %+v", f)
	}
}
