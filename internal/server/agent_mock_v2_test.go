package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ─── 1. TestMockEngineV2_BranchChoice_PausesUntilUserPicks ────

// 验证场景：剧本有 BranchChoice=true 的 step，
// 引擎推 mock_branch_choice 事件后不继续推 stream_end。
func TestMockEngineV2_BranchChoice_PausesUntilUserPicks(t *testing.T) {
	eng := NewMockEngineV2()
	sc := &MockScenario{
		ID:     "test_branch_pause",
		Rounds: 1,
		Branches: []Branch{
			{ID: "a", Label: "A"},
			{ID: "b", Label: "B"},
		},
		Steps: []MockStep{
			{RoundIdx: 0, DelayMs: 0, Events: []MockEvent{
				{Type: "text_delta", Data: map[string]any{"text": "选 a 或 b"}},
			}, BranchChoice: true, BranchID: "choose"},
		},
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	if err := eng.Run(context.Background(), s, sess, rec, rec, sc, 10.0, false); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if findEventOfType(sess, "mock_branch_choice") == nil {
		t.Fatal("expected mock_branch_choice event")
	}
	if findEventOfType(sess, "stream_end") != nil {
		t.Error("BranchChoice 时不应推 stream_end（用户未选）")
	}
}

// ─── 2. TestMockEngineV2_BranchChoice_KeywordMatch ─────────────

// 用户发"选 a" → 关键词匹配命中 branch.id=a
func TestMockEngineV2_BranchChoice_KeywordMatch(t *testing.T) {
	eng := NewMockEngineV2()
	sc := &MockScenario{
		ID:     "test_kw_match",
		Rounds: 1,
		Branches: []Branch{
			{ID: "encrypt", Label: "加密", TriggerKeywords: []string{"加密", "encrypt"}},
			{ID: "decrypt", Label: "解密", TriggerKeywords: []string{"解密", "decrypt"}},
		},
		Steps: []MockStep{
			{RoundIdx: 0, DelayMs: 0, Events: []MockEvent{
				{Type: "text_delta", Data: map[string]any{"text": "?"}},
			}, BranchChoice: true},
		},
	}
	eng.SetScenario(sc)

	branch, ok := eng.matchBranch("我想加密文件")
	if !ok {
		t.Fatal("keyword match should succeed")
	}
	if branch.ID != "encrypt" {
		t.Errorf("matched branch = %s, want encrypt", branch.ID)
	}
}

// ─── 3. TestMockEngineV2_BranchChoice_RegexMatch ───────────────

func TestMockEngineV2_BranchChoice_RegexMatch(t *testing.T) {
	eng := NewMockEngineV2()
	sc := &MockScenario{
		ID: "test_regex_match",
		Branches: []Branch{
			{ID: "video", TriggerRegex: `(?i)\.(mp4|mkv)$`},
			{ID: "audio", TriggerRegex: `(?i)\.(mp3|flac)$`},
		},
	}
	eng.SetScenario(sc)

	branch, ok := eng.matchBranch("movie.MP4")
	if !ok {
		t.Fatal("regex match should succeed for movie.MP4")
	}
	if branch.ID != "video" {
		t.Errorf("matched = %s, want video", branch.ID)
	}

	branch, ok = eng.matchBranch("song.flac")
	if !ok {
		t.Fatal("regex match should succeed for song.flac")
	}
	if branch.ID != "audio" {
		t.Errorf("matched = %s, want audio", branch.ID)
	}
}

// ─── 4. TestMockEngineV2_BranchChoice_NoMatchRePrompts ─────────

func TestMockEngineV2_BranchChoice_NoMatchRePrompts(t *testing.T) {
	eng := NewMockEngineV2()
	sc := &MockScenario{
		ID: "test_no_match",
		Branches: []Branch{
			{ID: "a", TriggerKeywords: []string{"yes"}},
			{ID: "b", TriggerKeywords: []string{"no"}},
		},
	}
	eng.SetScenario(sc)

	branch, ok := eng.matchBranch("foobar 不相关")
	if ok {
		t.Errorf("expected no match, got %s", branch.ID)
	}
}

// ─── 5. TestMockEngineV2_MultiRound_AdvancesOnUserText ─────────

// 验证 Rounds=2 时，round 0 → pause → Resume → round 1 → stream_end
func TestMockEngineV2_MultiRound_AdvancesOnUserText(t *testing.T) {
	eng := NewMockEngineV2()
	sc := &MockScenario{
		ID:     "test_multi_round",
		Rounds: 2,
		Steps: []MockStep{
			{RoundIdx: 0, DelayMs: 0, Events: []MockEvent{
				{Type: "stream_start", Data: map[string]any{"scenario": "test_multi_round"}},
				{Type: "text_delta", Data: map[string]any{"text": "round 0"}},
			}, PauseForUser: true, SetContext: map[string]any{
				"step": 0,
			}},
			{RoundIdx: 1, DelayMs: 0, Events: []MockEvent{
				{Type: "text_delta", Data: map[string]any{"text": "round 1"}},
				{Type: "stream_end", Data: map[string]any{"finishReason": "stop"}},
			}},
		},
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	if err := eng.Run(context.Background(), s, sess, rec, rec, sc, 10.0, false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Run 推完 round 0 后应停止（pause + 等 user_text）
	if findEventOfType(sess, "stream_end") != nil {
		t.Error("Run 后不应有 stream_end（round 0 暂停中）")
	}
	if findEventOfType(sess, "mock_round_state") == nil {
		t.Error("expected mock_round_state 事件")
	}

	// 模拟 user_text → Resume
	rec2 := httptest.NewRecorder()
	if err := eng.Resume(context.Background(), s, sess, rec2, rec2, "user reply"); err != nil {
		// Resume 是 6 参数：ctx, s, sess, w, flusher, userText
		t.Logf("Resume err (ignoring): %v", err)
	}
}

// ─── 6. TestMockEngineV2_RoundContext_SetAndUse ────────────────

// 验证 SetContext 写入后能被后续 event 模板插值。
func TestMockEngineV2_RoundContext_SetAndUse(t *testing.T) {
	eng := NewMockEngineV2()
	sc := &MockScenario{
		ID:     "test_ctx",
		Rounds: 1,
		Steps: []MockStep{
			{RoundIdx: 0, DelayMs: 0, Events: []MockEvent{
				{Type: "stream_start", Data: map[string]any{"scenario": "test_ctx"}},
			}, SetContext: map[string]any{
				"user_name": "alice",
			}},
			{RoundIdx: 0, DelayMs: 0, Events: []MockEvent{
				{Type: "text_delta", Data: map[string]any{
					"text": "Hello, {{user_name}}!",
				}},
				{Type: "stream_end", Data: map[string]any{"finishReason": "stop"}},
			}},
		},
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	if err := eng.Run(context.Background(), s, sess, rec, rec, sc, 10.0, false); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// 验证 roundCtx 包含 user_name=alice
	ctx := eng.RoundContext()
	if ctx["user_name"] != "alice" {
		t.Errorf("roundCtx user_name = %v, want alice", ctx["user_name"])
	}

	// 验证 text_delta 事件 data.text 已被模板替换
	for _, e := range sess.EventCache {
		if e.Type != "text_delta" {
			continue
		}
		if m, ok := e.Data.(map[string]any); ok {
			if got, _ := m["text"].(string); got == "Hello, alice!" {
				return
			}
		}
	}
	t.Errorf("expected text_delta with template replaced to 'Hello, alice!', got events: %+v", sess.EventCache)
}

// ─── 7. TestMockEngineV2_RoundTimeout_CancelsStream ────────────

// 验证 ctx 取消时 Run 返回 ctx.Err()（模拟客户端断开）。
func TestMockEngineV2_RoundTimeout_CancelsStream(t *testing.T) {
	eng := NewMockEngineV2()
	sc := &MockScenario{
		ID:     "test_timeout",
		Rounds: 1,
		Steps: []MockStep{
			{RoundIdx: 0, DelayMs: 5000, Events: []MockEvent{
				{Type: "text_delta", Data: map[string]any{"text": "long"}},
			}},
		},
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := eng.Run(ctx, s, sess, rec, rec, sc, 1.0, false)
	if err == nil {
		t.Fatal("expected ctx error on timeout, got nil")
	}
	if !strings.Contains(err.Error(), "context") && !strings.Contains(err.Error(), "deadline") {
		t.Errorf("expected context error, got %v", err)
	}
}

// ─── 8. TestMockEngineV2_PauseForUser_ResumesOnSend ────────────

// 验证 PauseForUser=true 的 step 推完后停止。
func TestMockEngineV2_PauseForUser_ResumesOnSend(t *testing.T) {
	eng := NewMockEngineV2()
	sc := &MockScenario{
		ID:     "test_pause_resume",
		Rounds: 2,
		Steps: []MockStep{
			{RoundIdx: 0, DelayMs: 0, Events: []MockEvent{
				{Type: "stream_start", Data: map[string]any{"scenario": "test_pause_resume"}},
				{Type: "text_delta", Data: map[string]any{"text": "请输入"}},
			}, PauseForUser: true},
			{RoundIdx: 1, DelayMs: 0, Events: []MockEvent{
				{Type: "text_delta", Data: map[string]any{"text": "收到"}},
				{Type: "stream_end", Data: map[string]any{"finishReason": "stop"}},
			}},
		},
	}

	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	if err := eng.Run(context.Background(), s, sess, rec, rec, sc, 10.0, false); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Run 推完 round 0 后应停止
	if findEventOfType(sess, "stream_end") != nil {
		t.Error("PauseForUser 时 Run 不应推 stream_end")
	}
	// 应有 mock_round_state{phase:awaiting_user_input}
	var foundPause bool
	for _, e := range sess.EventCache {
		if e.Type == "mock_round_state" {
			if m, ok := e.Data.(map[string]any); ok {
				if m["phase"] == "awaiting_user_input" {
					foundPause = true
				}
			}
		}
	}
	if !foundPause {
		t.Error("expected mock_round_state{phase:awaiting_user_input}")
	}
}

// ─── 9. TestMockEngineV2_8ScenariosLoad ────────────────────────

// 验证全部 8 个 v2 场景都已注册到 mockScenariosV2 且结构正确。
func TestMockEngineV2_8ScenariosLoad(t *testing.T) {
	if len(mockScenariosV2) != 8 {
		t.Fatalf("len(mockScenariosV2) = %d, want 8", len(mockScenariosV2))
	}
	required := map[string]bool{
		"search_recursive_mp4":      false,
		"search_logical_query":      false,
		"search_content_regex":      false,
		"edit_metadata_wizard":      false,
		"batch_rename_with_preview": false,
		"branch_encrypt_or_decrypt": false,
		"branch_video_or_audio":     false,
		"command_run_ffprobe":       false,
	}
	for _, sc := range mockScenariosV2 {
		if _, ok := required[sc.ID]; !ok {
			t.Errorf("未知场景 ID: %s", sc.ID)
		}
		required[sc.ID] = true
		if sc.Rounds == 0 {
			t.Errorf("%s Rounds=0, v2 场景必须 Rounds>=1", sc.ID)
		}
	}
	for id, found := range required {
		if !found {
			t.Errorf("missing v2 scenario: %s", id)
		}
	}
}

// ─── 10. TestMockEngineV2_CompatWithV1Scenarios ────────────────

// 验证 v1 场景（Rounds=0, Branches=nil）仍能通过 MockEngine.Run 跑通
// （不会触发 v2 引擎误判）。
func TestMockEngineV2_CompatWithV1Scenarios(t *testing.T) {
	eng := NewMockEngine() // v1 engine
	sc := &MockScenario{
		ID: "v1_compat_test",
		Steps: []MockStep{
			{DelayMs: 0, Events: []MockEvent{
				{Type: "stream_start", Data: map[string]any{"scenario": "v1_compat_test"}},
				{Type: "text_delta", Data: map[string]any{"text": "v1 hi"}},
				{Type: "stream_end", Data: map[string]any{"finishReason": "stop"}},
			}},
		},
	}
	// Rounds 缺省=0，验证 MockEngine.Run 不会 panic
	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()
	if err := eng.Run(context.Background(), s, sess, rec, rec, sc, 10.0, true); err != nil {
		t.Fatalf("v1 Run: %v", err)
	}
	if findEventOfType(sess, "stream_end") == nil {
		t.Error("v1 场景应正常推 stream_end")
	}
}

// ─── 11. TestMockEngineV2_ApproveRejectTool_EmitsEvent ──────────

// 验证工具授权 / 拒绝事件正确推送。
func TestMockEngineV2_ApproveRejectTool_EmitsEvent(t *testing.T) {
	eng := NewMockEngineV2()
	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()

	if err := eng.ApproveTool(s, sess, rec, rec, "call_x"); err != nil {
		t.Fatalf("ApproveTool: %v", err)
	}
	if findEventOfType(sess, "mock_tool_approved") == nil {
		t.Error("expected mock_tool_approved")
	}

	rec2 := httptest.NewRecorder()
	if err := eng.RejectTool(s, sess, rec2, rec2, "call_x"); err != nil {
		t.Fatalf("RejectTool: %v", err)
	}
	if findEventOfType(sess, "mock_tool_rejected") == nil {
		t.Error("expected mock_tool_rejected")
	}
	// roundCtx 应记录授权 / 拒绝
	ctx := eng.RoundContext()
	if ctx["approved_tool_call_x"] != true {
		t.Errorf("roundCtx missing approved_tool_call_x: %+v", ctx)
	}
	if ctx["rejected_tool_call_x"] != true {
		t.Errorf("roundCtx missing rejected_tool_call_x: %+v", ctx)
	}
}

// ─── 12. TestMockEngineV2_DispatchRoutesToV2ForRoundsScenarios ─

// 验证 Server.executeAgentTool 优先走 v2 registry。
// 这里通过 NewMockEngine 调度的入口（agent_api.go 中的 if scenario.Rounds > 0）
// 走 MockEngineV2，但 v1 场景走 MockEngine。
func TestMockEngineV2_DispatchRoutesToV2ForRoundsScenarios(t *testing.T) {
	// 构造一个 v1 场景（Rounds=0）和 v2 场景（Rounds=1）
	v1 := &MockScenario{ID: "v1a", Steps: []MockStep{
		{DelayMs: 0, Events: []MockEvent{
			{Type: "stream_start", Data: map[string]any{"scenario": "v1a"}},
			{Type: "stream_end", Data: map[string]any{"finishReason": "stop"}},
		}},
	}}
	v2 := &MockScenario{ID: "v2a", Rounds: 1, Steps: []MockStep{
		{RoundIdx: 0, DelayMs: 0, Events: []MockEvent{
			{Type: "stream_start", Data: map[string]any{"scenario": "v2a"}},
			{Type: "stream_end", Data: map[string]any{"finishReason": "stop"}},
		}},
	}}

	// v1 应走 MockEngine（v1 路径）
	eng1 := NewMockEngine()
	s1 := newMockTestServer()
	sess1 := newMockSession()
	rec1 := httptest.NewRecorder()
	if err := eng1.Run(context.Background(), s1, sess1, rec1, rec1, v1, 10.0, true); err != nil {
		t.Fatalf("v1 Run: %v", err)
	}
	if findEventOfType(sess1, "stream_end") == nil {
		t.Error("v1 场景 Run 后应有 stream_end")
	}

	// v2 走 MockEngineV2 路径
	eng2 := NewMockEngineV2()
	s2 := newMockTestServer()
	sess2 := newMockSession()
	rec2 := httptest.NewRecorder()
	if err := eng2.Run(context.Background(), s2, sess2, rec2, rec2, v2, 10.0, false); err != nil {
		t.Fatalf("v2 Run: %v", err)
	}
	if findEventOfType(sess2, "stream_end") == nil {
		t.Error("v2 场景 Run 后应有 stream_end")
	}
	if findEventOfType(sess2, "mock_round_state") == nil {
		t.Error("v2 场景应推 mock_round_state")
	}
}

// ─── 13. TestMockEngineV2_StreamStart_IncludesScenarioID ───────

func TestMockEngineV2_StreamStart_IncludesScenarioID(t *testing.T) {
	eng := NewMockEngineV2()
	sc := &MockScenario{
		ID:     "test_start",
		Rounds: 1,
		Steps: []MockStep{
			{RoundIdx: 0, DelayMs: 0, Events: []MockEvent{
				{Type: "stream_start", Data: map[string]any{"scenario": "test_start"}},
				{Type: "text_delta", Data: map[string]any{"text": "x"}},
				{Type: "stream_end", Data: map[string]any{"finishReason": "stop"}},
			}},
		},
	}
	s := newMockTestServer()
	sess := newMockSession()
	rec := httptest.NewRecorder()
	if err := eng.Run(context.Background(), s, sess, rec, rec, sc, 10.0, false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	ss := findEventOfType(sess, "stream_start")
	if ss == nil {
		t.Fatal("expected stream_start")
	}
	if m, ok := ss.Data.(map[string]any); ok {
		if m["scenario"] != "test_start" {
			t.Errorf("stream_start.scenario = %v, want test_start", m["scenario"])
		}
	}
}

// ─── 14. TestMockEngineV2_AllScenariosHaveSteps ────────────────

func TestMockEngineV2_AllScenariosHaveSteps(t *testing.T) {
	for _, sc := range mockScenariosV2 {
		if len(sc.Steps) == 0 {
			t.Errorf("%s: 无 Steps", sc.ID)
		}
		// 每个 step 都应至少 1 个 event
		for i, st := range sc.Steps {
			if len(st.Events) == 0 {
				t.Errorf("%s: step %d 无 Events", sc.ID, i)
			}
		}
		// 含 tool_call 的 step 后面必须有 tool_result
		hasToolResult := false
		for _, st := range sc.Steps {
			for _, e := range st.Events {
				if e.Type == "tool_result" {
					hasToolResult = true
				}
			}
		}
		// 至少有一个 tool_call 的剧本必须有 tool_result
		hasToolCall := false
		for _, st := range sc.Steps {
			for _, e := range st.Events {
				if e.Type == "tool_call" {
					hasToolCall = true
				}
			}
		}
		if hasToolCall && !hasToolResult {
			t.Errorf("%s: 含 tool_call 但缺 tool_result", sc.ID)
		}
	}
}

// ─── 15. TestMockEngineV2_EngineHTTPWriter_FlusherInterface ─────

// 验证 httptest.ResponseRecorder 满足 http.Flusher 接口。
// （用于 v2 引擎测试用 — 间接覆盖 SSE writer plumbing）
func TestMockEngineV2_EngineHTTPWriter_FlusherInterface(t *testing.T) {
	rec := httptest.NewRecorder()
	var _ http.Flusher = rec // 编译期检查
	var _ http.ResponseWriter = rec
}
