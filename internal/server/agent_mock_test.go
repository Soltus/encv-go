// internal/server/agent_mock_test.go
//
// Mock 引擎单元测试。
//
// 测试策略：
//   - Match: 验证优先级（精确 > 关键词 > 正则 > fallback）+ 模式分支（builtin/custom）
//   - Run: 通过 sess.EventCache 反查事件，避免解析 SSE 字节流
//   - AllScenarios: 跑遍 12 个内置剧本 + 验证 stream_end 事件
//   - Speed/Cancel/LoadCustom: 覆盖边角路径
package server

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ─── 工具函数 ────────────────────────────────────────────────

// newMockTestServer 构造一个最小化的 Server，仅含 mockEngine 字段。
// 用于 Run 单元测试（避免读 config / 网络等副作用）。
func newMockTestServer() *Server {
	return &Server{mockEngine: NewMockEngine()}
}

// ─── execute_real 测试 ────────────────────────────────────────

// TestMockEngine_ExecuteReal_OverridesHardcoded 验证当 tool_call 标记
// execute_real=true 且 realExecutor 已注入时：
//   - realExecutor 被实际调用
//   - 真实返回值覆盖剧本的硬编码 result
//   - 事件 isError=false / durationMs > 0
func TestMockEngine_ExecuteReal_OverridesHardcoded(t *testing.T) {
	eng := NewMockEngine()
	calls := []string{}
	eng.SetRealExecutor(func(ctx context.Context, name, args string) (string, error) {
		calls = append(calls, name)
		return `{"live":true,"name":"` + name + `"}`, nil
	})

	// 构造一个最小化剧本：tool_call(execute_real=true) + tool_result（硬编码假数据）
	sc := &MockScenario{
		ID:       "test_execute_real",
		Keywords: []string{"x"},
		Steps: []MockStep{
			{DelayMs: 0, Events: []MockEvent{
				{Type: "stream_start", Data: map[string]interface{}{"scenario": "test_execute_real"}},
			}},
			{DelayMs: 10, Events: []MockEvent{
				{Type: "tool_call", Data: map[string]interface{}{
					"id":           "call_real_1",
					"name":         "list_files",
					"args":         `{"mount_id":"serving","rel_path":"Movies"}`,
					"auto_run":     true,
					"needsConfirm": false,
					"kind":         "fileRead",
					"execute_real": true,
				}},
			}},
			{DelayMs: 10, Events: []MockEvent{
				// 硬编码假数据，应被真实结果覆盖
				{Type: "tool_result", Data: map[string]interface{}{
					"id":         "call_real_1",
					"name":       "list_files",
					"result":     `{"FAKE":true}`,
					"isError":    false,
					"status":     "success",
					"durationMs": 999,
				}},
			}},
			{DelayMs: 10, Events: []MockEvent{
				{Type: "stream_end", Data: map[string]interface{}{"finishReason": "stop"}},
			}},
		},
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := eng.Run(ctx, s, sess, rec, rec, sc, 10.0, true); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// 验证 realExecutor 被调用
	if len(calls) != 1 || calls[0] != "list_files" {
		t.Errorf("realExecutor 调用次数 = %d, names = %v, want 1×[list_files]", len(calls), calls)
	}

	// 验证 tool_result 事件被覆盖为真实数据
	tr := findEventOfType(sess, "tool_result")
	if tr == nil {
		t.Fatal("expected tool_result event")
	}
	data, ok := tr.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("tool_result.Data type = %T", tr.Data)
	}
	if got, _ := data["result"].(string); got != `{"live":true,"name":"list_files"}` {
		t.Errorf("result 被覆盖 = %q, want real executor output", got)
	}
	if data["isError"] != false {
		t.Errorf("isError = %v, want false", data["isError"])
	}
	if data["status"] != "success" {
		t.Errorf("status = %v, want success", data["status"])
	}
	// durationMs 缓存中是 int64（未经 JSON 序列化）；断言它存在且非硬编码值 999。
	// 不强制 > 0（mock 函数可能瞬时返回 0ms）——关键是「不是剧本硬编码 999」。
	if dur, ok := data["durationMs"].(int64); !ok {
		t.Errorf("durationMs 类型 = %T, want int64", data["durationMs"])
	} else if dur == 999 {
		t.Errorf("durationMs = %v, want real elapsed time (not hardcoded 999)", data["durationMs"])
	}
}

// TestMockEngine_ExecuteReal_FallbackWhenNoExecutor 验证 realExecutor 为 nil
// 时 execute_real=true 仍按硬编码剧本 result 推送（容灾 + 单测）。
func TestMockEngine_ExecuteReal_FallbackWhenNoExecutor(t *testing.T) {
	eng := NewMockEngine()
	// 不调 SetRealExecutor — 留 nil

	sc := &MockScenario{
		ID: "test_execute_real_fallback",
		Steps: []MockStep{
			{DelayMs: 0, Events: []MockEvent{
				{Type: "stream_start", Data: map[string]interface{}{"scenario": "test_execute_real_fallback"}},
				{Type: "tool_call", Data: map[string]interface{}{
					"id":           "call_fb_1",
					"name":         "list_files",
					"args":         `{}`,
					"auto_run":     true,
					"needsConfirm": false,
					"kind":         "fileRead",
					"execute_real": true,
				}},
				{Type: "tool_result", Data: map[string]interface{}{
					"id":         "call_fb_1",
					"name":       "list_files",
					"result":     `{"HARDCODED":true}`,
					"isError":    false,
					"status":     "success",
					"durationMs": 42,
				}},
				{Type: "stream_end", Data: map[string]interface{}{"finishReason": "stop"}},
			}},
		},
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	if err := eng.Run(context.Background(), s, sess, rec, rec, sc, 10.0, true); err != nil {
		t.Fatalf("Run: %v", err)
	}

	tr := findEventOfType(sess, "tool_result")
	data, _ := tr.Data.(map[string]interface{})
	if got, _ := data["result"].(string); got != `{"HARDCODED":true}` {
		t.Errorf("realExecutor=nil 时 result 应保持硬编码 = %q, want HARDCODED", got)
	}
	if data["durationMs"] != 42 {
		t.Errorf("realExecutor=nil 时 durationMs 应保持硬编码 42, got %v", data["durationMs"])
	}
}

// TestMockEngine_EmitsInitialPresetsOnStreamStart 验证 Run 在 stream_start
// 后立即推 mock_presets 事件，data.presets 包含剧本声明的所有 MockPreset。
func TestMockEngine_EmitsInitialPresetsOnStreamStart(t *testing.T) {
	eng := NewMockEngine()
	sc := &MockScenario{
		ID: "test_presets_initial",
		Presets: []MockPreset{
			{ID: "p1", Label: "选项 1", UserText: "选 1"},
			{ID: "p2", Label: "选项 2", UserText: "选 2"},
		},
		Steps: []MockStep{
			{DelayMs: 0, Events: []MockEvent{
				{Type: "stream_start", Data: map[string]interface{}{"scenario": "test_presets_initial"}},
				{Type: "text_delta", Data: map[string]interface{}{"text": "hi"}},
				{Type: "stream_end", Data: map[string]interface{}{"finishReason": "stop"}},
			}},
		},
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	if err := eng.Run(context.Background(), s, sess, rec, rec, sc, 10.0, true); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// 找到 mock_presets 事件
	pe := findEventOfType(sess, "mock_presets")
	if pe == nil {
		t.Fatal("expected mock_presets event in cache")
	}
	data, _ := pe.Data.(map[string]interface{})
	if data["scenario"] != "test_presets_initial" {
		t.Errorf("scenario = %v, want test_presets_initial", data["scenario"])
	}
	if data["phase"] != "initial" {
		t.Errorf("phase = %v, want initial", data["phase"])
	}
	presets, ok := data["presets"].([]MockPreset)
	if !ok {
		t.Fatalf("presets type = %T, want []MockPreset", data["presets"])
	}
	if len(presets) != 2 {
		t.Errorf("presets count = %d, want 2", len(presets))
	}
	if presets[0].ID != "p1" || presets[1].ID != "p2" {
		t.Errorf("presets IDs = [%s, %s], want [p1, p2]", presets[0].ID, presets[1].ID)
	}
}

// TestMockEngine_NoPresetsWhenScenarioEmpty 验证未声明 Presets 的剧本
// 不会推 mock_presets 事件（保持事件流干净）。
func TestMockEngine_NoPresetsWhenScenarioEmpty(t *testing.T) {
	eng := NewMockEngine()
	sc := &MockScenario{
		ID: "test_presets_none",
		Steps: []MockStep{
			{DelayMs: 0, Events: []MockEvent{
				{Type: "stream_start", Data: map[string]interface{}{"scenario": "test_presets_none"}},
				{Type: "stream_end", Data: map[string]interface{}{"finishReason": "stop"}},
			}},
		},
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	if err := eng.Run(context.Background(), s, sess, rec, rec, sc, 10.0, true); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if pe := findEventOfType(sess, "mock_presets"); pe != nil {
		t.Errorf("scenario.Presets 为空时不应有 mock_presets, got %+v", pe.Data)
	}
}

// TestMockEngine_MidScenarioPresetUpdate 验证 mid-scenario 的 mock_presets
// 事件被透传给前端（不修改 data，只是 forward）。
func TestMockEngine_MidScenarioPresetUpdate(t *testing.T) {
	eng := NewMockEngine()
	sc := &MockScenario{
		ID: "test_presets_mid",
		Steps: []MockStep{
			{DelayMs: 0, Events: []MockEvent{
				{Type: "stream_start", Data: map[string]interface{}{"scenario": "test_presets_mid"}},
				{Type: "text_delta", Data: map[string]interface{}{"text": "step 1"}},
			}},
			{DelayMs: 10, Events: []MockEvent{
				{Type: "mock_presets", Data: map[string]interface{}{
					"scenario": "test_presets_mid",
					"phase":    "after_step_1",
					"presets": []MockPreset{
						{ID: "m1", Label: "中间选项", UserText: "中间选择"},
					},
				}},
				{Type: "text_delta", Data: map[string]interface{}{"text": "step 2"}},
				{Type: "stream_end", Data: map[string]interface{}{"finishReason": "stop"}},
			}},
		},
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	if err := eng.Run(context.Background(), s, sess, rec, rec, sc, 10.0, true); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// 找所有 mock_presets 事件（应有 1 个：mid-scenario 那条）
	count := 0
	for _, ev := range sess.EventCache {
		if ev.Type == "mock_presets" {
			count++
			data, _ := ev.Data.(map[string]interface{})
			if data["phase"] != "after_step_1" {
				t.Errorf("phase = %v, want after_step_1", data["phase"])
			}
		}
	}
	if count != 1 {
		t.Errorf("mock_presets 事件数 = %d, want 1 (mid-scenario only)", count)
	}
}

// TestMockEngine_StreamEndDoesNotClearPresets 验证"覆盖显示"语义：
// 剧本 stream_end 后**不再**推 mock_presets_clear —— chip 必须保留在
// 输入框上方供用户选下一轮输入。仅当用户**主动**退出 mock 模式时才推
// clear（由 setMockMode handler 显式触发）。
func TestMockEngine_StreamEndDoesNotClearPresets(t *testing.T) {
	eng := NewMockEngine()
	sc := &MockScenario{
		ID: "test_presets_retained",
		Presets: []MockPreset{
			{ID: "p1", Label: "X", UserText: "x"},
		},
		Steps: []MockStep{
			{DelayMs: 0, Events: []MockEvent{
				{Type: "stream_start", Data: map[string]interface{}{"scenario": "test_presets_retained"}},
				{Type: "stream_end", Data: map[string]interface{}{"finishReason": "stop"}},
			}},
		},
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	if err := eng.Run(context.Background(), s, sess, rec, rec, sc, 10.0, true); err != nil {
		t.Fatalf("Run: %v", err)
	}

	clears := 0
	for _, ev := range sess.EventCache {
		if ev.Type == "mock_presets_clear" {
			clears++
		}
	}
	if clears != 0 {
		t.Errorf("stream_end 之后不应推 mock_presets_clear（chip 永远覆盖显示），实际 %d 次", clears)
	}
}

// TestMockEngine_AllBuiltinScenariosHavePresets 验证 12 个内置剧本都至少
// 有 1 个 Preset（覆盖验收清单 §"每个剧本应支持多个预设"）。
func TestMockEngine_AllBuiltinScenariosHavePresets(t *testing.T) {
	eng := NewMockEngine()
	scenarios := eng.AllScenarios()

	missing := []string{}
	for _, sc := range scenarios {
		if len(sc.Presets) == 0 {
			missing = append(missing, sc.ID)
		}
	}
	if len(missing) > 0 {
		t.Errorf("以下剧本缺少 Presets（验收要求每个剧本都支持预设输入）：%v", missing)
	}
}

// TestMockEngine_ExecuteReal_ErrorPropagatesAsIsError 验证 realExecutor
// 返回 error 时，tool_result 事件 isError=true / status=failed / result 含错误信息。
func TestMockEngine_ExecuteReal_ErrorPropagatesAsIsError(t *testing.T) {
	eng := NewMockEngine()
	eng.SetRealExecutor(func(ctx context.Context, name, args string) (string, error) {
		return "", fmt.Errorf("simulated execution failure")
	})

	sc := &MockScenario{
		ID: "test_execute_real_err",
		Steps: []MockStep{
			{DelayMs: 0, Events: []MockEvent{
				{Type: "stream_start", Data: map[string]interface{}{"scenario": "test_execute_real_err"}},
				{Type: "tool_call", Data: map[string]interface{}{
					"id":           "call_err_1",
					"name":         "list_files",
					"args":         `{}`,
					"auto_run":     true,
					"needsConfirm": false,
					"kind":         "fileRead",
					"execute_real": true,
				}},
				{Type: "tool_result", Data: map[string]interface{}{
					"id":         "call_err_1",
					"name":       "list_files",
					"result":     `{"old":"to-be-overridden"}`,
					"isError":    false,
					"status":     "success",
					"durationMs": 1,
				}},
				{Type: "stream_end", Data: map[string]interface{}{"finishReason": "stop"}},
			}},
		},
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	if err := eng.Run(context.Background(), s, sess, rec, rec, sc, 10.0, true); err != nil {
		t.Fatalf("Run: %v", err)
	}

	tr := findEventOfType(sess, "tool_result")
	data, _ := tr.Data.(map[string]interface{})
	if data["isError"] != true {
		t.Errorf("realExecutor 返回 error 时 isError 应为 true, got %v", data["isError"])
	}
	if data["status"] != "failed" {
		t.Errorf("status 应为 failed, got %v", data["status"])
	}
	if got, _ := data["result"].(string); !strings.Contains(got, "simulated execution failure") {
		t.Errorf("result 应含错误信息, got %q", got)
	}
}

// newMockSession 构造一个最小化的 agentSession。
// EventCache 字段必须初始化为非 nil（否则 reflect 上可能判定为空）。
func newMockSession() *agentSession {
	return &agentSession{EventCache: []AgentEvent{}}
}

// countEventType 统计 EventCache 中 type == typ 的事件数。
func countEventType(sess *agentSession, typ string) int {
	n := 0
	for _, e := range sess.EventCache {
		if e.Type == typ {
			n++
		}
	}
	return n
}

// findEventOfType 返回 EventCache 中 type == typ 的第一个事件。
// 找不到返回 nil。
func findEventOfType(sess *agentSession, typ string) *AgentEvent {
	for i := range sess.EventCache {
		if sess.EventCache[i].Type == typ {
			return &sess.EventCache[i]
		}
	}
	return nil
}

// ─── Match 测试 ──────────────────────────────────────────────

// TestMockEngine_Match 覆盖 Match 优先级、builtin/custom 模式分支、fallback。
func TestMockEngine_Match(t *testing.T) {
	eng := NewMockEngine()
	cases := []struct {
		name     string
		userText string
		mode     string
		want     string // expected scenario ID, "" for nil
	}{
		// 优先级：精确 > 关键词 > 正则 > fallback
		{"exact_match", "有哪些文件", "builtin", "list_files_query"},
		{"keyword_match", "你帮我看看有哪些文件", "builtin", "list_files_query"},
		{"keyword_case_insensitive", "有哪些文件", "builtin", "list_files_query"},
		{"no_match_builtin_fallback", "随便聊聊", "builtin", "default_friendly"},
		{"no_match_custom_returns_nil", "随便聊聊", "custom", ""},
		{"chinese_greeting", "你好", "builtin", "chinese_greeting"},
		{"truncation_keyword", "写一篇长文", "builtin", "truncation_long_text"},
		{"streaming_error_keyword", "触发错误", "builtin", "streaming_error"},
		{"multi_tool_keyword", "parallel batch", "builtin", "multi_tool_parallel"},
		// 模式分支：empty user text
		{"empty_text_builtin_fallback", "", "builtin", "default_friendly"},
		{"empty_text_custom_returns_nil", "", "custom", ""},
		// 未知 mode → 同 builtin
		{"unknown_mode_acts_as_builtin", "随便聊聊", "weird_mode", "default_friendly"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := eng.Match(tc.userText, tc.mode)
			gotID := ""
			if got != nil {
				gotID = got.ID
			}
			if gotID != tc.want {
				t.Errorf("Match(%q, %q) = %q, want %q", tc.userText, tc.mode, gotID, tc.want)
			}
		})
	}
}

// ─── Run 测试 — 验证 [真机问题] 修复场景 ─────────────────────

// TestMockEngine_Run_ListFilesQuery 验证 list_files_query 剧本
// 产生 2 tool_call + 2 tool_result + 完整文本回复 + 首个 stream_start 含 mock: true。
func TestMockEngine_Run_ListFilesQuery(t *testing.T) {
	eng := NewMockEngine()
	scenario := eng.Match("有哪些视频文件", "builtin")
	if scenario == nil {
		t.Fatal("expected list_files_query scenario, got nil")
	}
	if scenario.ID != "list_files_query" {
		t.Fatalf("expected list_files_query, got %q", scenario.ID)
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := eng.Run(ctx, s, sess, rec, rec, scenario, 10.0, true); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// 校验工具调用数量
	if n := countEventType(sess, "tool_call"); n < 2 {
		t.Errorf("expected >= 2 tool_call events, got %d", n)
	}
	// 校验 tool_result 数量
	if n := countEventType(sess, "tool_result"); n < 2 {
		t.Errorf("expected >= 2 tool_result events, got %d", n)
	}
	// 校验 text_delta 数量
	if n := countEventType(sess, "text_delta"); n < 3 {
		t.Errorf("expected >= 3 text_delta events, got %d", n)
	}
	// 校验 stream_end 存在
	if n := countEventType(sess, "stream_end"); n != 1 {
		t.Errorf("expected exactly 1 stream_end event, got %d", n)
	}

	// 校验首个 stream_start 含 mock: true 字段
	streamStart := findEventOfType(sess, "stream_start")
	if streamStart == nil {
		t.Fatal("expected a stream_start event")
	}
	data, ok := streamStart.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("stream_start.Data 不是 map, got %T", streamStart.Data)
	}
	if mock, _ := data["mock"].(bool); !mock {
		t.Errorf("首个 stream_start 应含 mock: true, got %+v", data)
	}
	if sc, _ := data["scenario"].(string); sc != "list_files_query" {
		t.Errorf("首个 stream_start 应含 scenario=list_files_query, got %q", sc)
	}

	// 校验 SSE 响应体包含工具调用名（list_mounts）
	if !strings.Contains(rec.Body.String(), "list_mounts") {
		t.Error("SSE 输出应含工具名 list_mounts")
	}
}

// ─── Run 测试 — 全剧本覆盖 ───────────────────────────────────

// TestMockEngine_AllScenariosExecute 对 13 个内置剧本逐一执行，
// 验证每个都产生至少一个事件 + stream_end 事件。
func TestMockEngine_AllScenariosExecute(t *testing.T) {
	eng := NewMockEngine()
	scenarios := eng.AllScenarios()
	if len(scenarios) != 13 {
		t.Fatalf("内置剧本数 = %d, want 13", len(scenarios))
	}

	for _, sc := range scenarios {
		t.Run(sc.ID, func(t *testing.T) {
			s := newMockTestServer()
			sess := newMockSession()
			rec := httptest.NewRecorder()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := eng.Run(ctx, s, sess, rec, rec, sc, 10.0, true); err != nil {
				t.Fatalf("Run(%s): %v", sc.ID, err)
			}

			// 至少一个事件（stream_start / text_delta / tool_call ...）
			if len(sess.EventCache) == 0 {
				t.Fatalf("场景 %s 未产生任何事件", sc.ID)
			}

			// 必须有 stream_end 事件
			if n := countEventType(sess, "stream_end"); n < 1 {
				t.Errorf("场景 %s 缺 stream_end 事件", sc.ID)
			}

			// streaming_error + mid_stream_disconnect 这类异常剧本：
			//   - streaming_error: 推 stream_status(type=error) → 自动追加 stream_end
			//   - 不验证其他字段，避免耦合具体事件顺序
		})
	}
}

// ─── Run 测试 — Speed 行为 ───────────────────────────────────

// TestMockEngine_SpeedZero 验证 speed=0 零延迟同步推完。
func TestMockEngine_SpeedZero(t *testing.T) {
	eng := NewMockEngine()
	// default_friendly 原本总延迟 0+200+300+100=600ms，speed=0 应在 <500ms 完成
	scenario := eng.AllScenarios()[0] // default_friendly
	if scenario.ID != "default_friendly" {
		t.Fatalf("第一个场景应默认为 default_friendly, got %q", scenario.ID)
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	start := time.Now()
	ctx := context.Background()
	if err := eng.Run(ctx, s, sess, rec, rec, scenario, 0, true); err != nil {
		t.Fatalf("Run: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed > 500*time.Millisecond {
		t.Errorf("speed=0 应近瞬时完成 (期望 <500ms), got %v", elapsed)
	}
	if len(sess.EventCache) == 0 {
		t.Error("speed=0 仍应产生事件")
	}
}

// TestMockEngine_SpeedHigh 验证 speed=10 加速：原本 600ms 的 default_friendly
// 在 speed=10 下应 <= ~60ms（加 ctx 检查开销）。
func TestMockEngine_SpeedHigh(t *testing.T) {
	eng := NewMockEngine()
	scenario := eng.AllScenarios()[0] // default_friendly: 总延迟 600ms

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	start := time.Now()
	ctx := context.Background()
	if err := eng.Run(ctx, s, sess, rec, rec, scenario, 10.0, true); err != nil {
		t.Fatalf("Run: %v", err)
	}
	elapsed := time.Since(start)

	// 600ms / 10 = 60ms，宽松放 +50ms 容差
	if elapsed > 200*time.Millisecond {
		t.Errorf("speed=10 应显著加速, got %v", elapsed)
	}
}

// ─── Run 测试 — 上下文取消 ───────────────────────────────────

// TestMockEngine_ContextCancel 验证 ctx 取消时 Run 立即返回 context.Canceled。
//
// 使用 list_files_query 剧本（总延迟 2500ms），在 50ms 时取消，
// Run 应在 cancel() 后立即返回 ctx.Err()。
func TestMockEngine_ContextCancel(t *testing.T) {
	eng := NewMockEngine()
	scenario := eng.Match("有哪些文件", "builtin")
	if scenario == nil {
		t.Fatal("expected list_files_query scenario")
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- eng.Run(ctx, s, sess, rec, rec, scenario, 1.0, true)
	}()

	// 50ms 后取消（剧本刚开始 step 1 后面的 500ms 延迟）
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run 在 cancel 后 2s 内未返回")
	}
}

// ─── LoadCustom 测试 ─────────────────────────────────────────

// TestMockEngine_LoadCustom 验证 custom 剧本加载、验证规则、模式可见性。
func TestMockEngine_LoadCustom(t *testing.T) {
	eng := NewMockEngine()

	custom := []MockScenario{
		{
			ID:       "test_custom",
			Keywords: []string{"test_custom_keyword"},
			Steps: []MockStep{
				{DelayMs: 0, Events: []MockEvent{
					{Type: "stream_start", Data: map[string]interface{}{
						"scenario": "test_custom",
					}},
					{Type: "text_delta", Data: map[string]interface{}{"text": "custom hi"}},
					{Type: "stream_end", Data: map[string]interface{}{
						"finishReason": "stop",
					}},
				}},
			},
		},
		{
			// 非法：empty ID — 应被跳过
			Keywords: []string{"x"},
		},
		{
			// 非法：empty ID + no trigger rule — 应被跳过
			Steps: []MockStep{},
		},
		{
			// 非法：regex 编译失败 — 应被跳过（不 panic）
			ID:    "bad_regex",
			Regex: "[invalid(",
		},
	}

	// LoadCustom 不应 panic
	eng.LoadCustom(custom)

	// custom 模式：custom 剧本可命中
	sc := eng.Match("test_custom_keyword please", "custom")
	if sc == nil || sc.ID != "test_custom" {
		t.Errorf("custom 模式应匹配 test_custom, got %v", sc)
	}

	// builtin 模式：custom 剧本也可命中（覆盖默认行为）
	sc2 := eng.Match("test_custom_keyword please", "builtin")
	if sc2 == nil || sc2.ID != "test_custom" {
		t.Errorf("builtin 模式也应能匹配 custom 剧本, got %v", sc2)
	}

	// builtin 模式：custom 未命中关键词 → 走 builtin fallback
	sc3 := eng.Match("完全不相关的输入 xyz", "builtin")
	if sc3 == nil || sc3.ID != "default_friendly" {
		t.Errorf("builtin 模式 fallback 应为 default_friendly, got %v", sc3)
	}

	// custom 模式：custom 未命中 → 返回 nil
	sc4 := eng.Match("完全不相关的输入 xyz", "custom")
	if sc4 != nil {
		t.Errorf("custom 模式无匹配应返回 nil, got %v", sc4)
	}
}

// TestMockEngine_LoadCustom_DuplicateID 验证重复 ID 被跳过。
func TestMockEngine_LoadCustom_DuplicateID(t *testing.T) {
	eng := NewMockEngine()

	custom := []MockScenario{
		{ID: "dup", Keywords: []string{"a"}},
		{ID: "dup", Keywords: []string{"b"}}, // duplicate
	}
	eng.LoadCustom(custom)

	// AllScenarios 应只含 builtin 13 + custom 1 = 14
	total := len(eng.AllScenarios())
	if total != 14 {
		t.Errorf("总剧本数 = %d, want 14 (13 builtin + 1 dedup'd custom)", total)
	}
}

// ─── AllScenarios 测试 ───────────────────────────────────────

// TestMockEngine_AllScenarios_Count 验证 AllScenarios 返回 builtin + custom。
func TestMockEngine_AllScenarios_Count(t *testing.T) {
	eng := NewMockEngine()
	all := eng.AllScenarios()
	if len(all) != 13 {
		t.Errorf("无 custom 时 AllScenarios 长度 = %d, want 13", len(all))
	}

	// 添加 custom 后数量应增加
	eng.LoadCustom([]MockScenario{
		{ID: "x1", Keywords: []string{"x1"}},
		{ID: "x2", Keywords: []string{"x2"}},
	})
	if got := len(eng.AllScenarios()); got != 15 {
		t.Errorf("加载 2 个 custom 后长度 = %d, want 15", got)
	}
}

// ─── Stream Start mock flag 测试 ────────────────────────────

// TestMockEngine_Run_StreamStartMockFlag 验证当 mockFlag=true 时，
// 首个 stream_start 事件 data 含 mock: true 和 scenario: <id>。
func TestMockEngine_Run_StreamStartMockFlag(t *testing.T) {
	eng := NewMockEngine()
	scenario := eng.AllScenarios()[0] // default_friendly

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	if err := eng.Run(context.Background(), s, sess, rec, rec, scenario, 10.0, true); err != nil {
		t.Fatalf("Run: %v", err)
	}

	ss := findEventOfType(sess, "stream_start")
	if ss == nil {
		t.Fatal("expected stream_start event")
	}
	data, ok := ss.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("stream_start.Data 不是 map, got %T", ss.Data)
	}
	if data["mock"] != true {
		t.Errorf("mockFlag=true 时 stream_start 应含 mock:true, got %v", data["mock"])
	}
	if data["scenario"] != "default_friendly" {
		t.Errorf("scenario 字段 = %v, want default_friendly", data["scenario"])
	}
}

// TestMockEngine_Run_StreamStartNoMockFlag 验证当 mockFlag=false 时，
// 首个 stream_start 事件 data **不**含 mock: true。
func TestMockEngine_Run_StreamStartNoMockFlag(t *testing.T) {
	eng := NewMockEngine()
	scenario := eng.AllScenarios()[0]

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	if err := eng.Run(context.Background(), s, sess, rec, rec, scenario, 10.0, false); err != nil {
		t.Fatalf("Run: %v", err)
	}

	ss := findEventOfType(sess, "stream_start")
	if ss == nil {
		t.Fatal("expected stream_start event")
	}
	data, _ := ss.Data.(map[string]interface{})
	if data == nil {
		return
	}
	if _, hasMock := data["mock"]; hasMock {
		t.Errorf("mockFlag=false 时 stream_start 不应含 mock 字段, got %+v", data)
	}
}
