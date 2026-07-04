// internal/server/agent_agui_adapter_test.go
//
// AG-UI 适配器单元测试。
//
// 覆盖范围（来自 spec tasks.md §1.3）：
//   - TestAGUIEventMapper_EmitsTextMessageStartBeforeContent
//   - TestAGUIEventMapper_StableMessageId
//   - TestAGUIEventMapper_EmptyArgs_SkipsTOOL_CALL_ARGS
//   - TestAGUIEventMapper_AllEventsIncludeThreadIdRunIdTimestamp
//   - TestAGUIThreadState_NextMessageID_IncrementsSeq
//   - TestAGUIEventMapper_EachEventHasTypeField
//
// 策略：通过 httptest.NewRecorder() 抓取 SSE 输出，
// 用正则解析 `event: <TYPE>\ndata: <JSON>\n\n` 单元，逐项断言。
package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// ─── 测试辅助函数 ───────────────────────────────────────────────

// sseFrame 解析单个 SSE 事件单元（event 行 + data 行 + 空行）。
type sseFrame struct {
	EventType string
	DataJSON  map[string]interface{}
}

// parseSSEFrames 把 SSE 文本分割为 frames slice。
// 格式: `event: <TYPE>\ndata: <JSON>\n\n`
func parseSSEFrames(t *testing.T, body string) []sseFrame {
	t.Helper()
	var frames []sseFrame

	// 按双换行分割 frame
	raw := strings.Split(body, "\n\n")
	for _, chunk := range raw {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" {
			continue
		}
		scanner := bufio.NewScanner(strings.NewReader(chunk))
		var eventType, dataLine string
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "event: ") {
				eventType = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				dataLine = strings.TrimPrefix(line, "data: ")
			}
		}
		if eventType == "" || dataLine == "" {
			continue
		}
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(dataLine), &data); err != nil {
			t.Fatalf("data JSON 解析失败: %v\nline=%q", err, dataLine)
		}
		frames = append(frames, sseFrame{EventType: eventType, DataJSON: data})
	}
	return frames
}

// newTestAGUIMapper 创建一个测试用 AGUIEventMapper + AGUIThreadState。
// 自动调用 state.NewRun()（保证 runId 非空）。
func newTestAGUIMapper(sessID string) (*AGUIEventMapper, *AGUIThreadState, *httptest.ResponseRecorder) {
	rec := httptest.NewRecorder()
	state := NewAGUIThreadState(sessID)
	state.NewRun()
	mapper := NewAGUIMapperWithState(rec, rec, state)
	return mapper, state, rec
}

// ─── 1.1: TestAGUIThreadState_NextMessageID_IncrementsSeq ─────

// 验证 NextMessageID 每次调用 seq 递增；多次 NewRun 后 seq 不重置。
func TestAGUIThreadState_NextMessageID_IncrementsSeq(t *testing.T) {
	state := NewAGUIThreadState("sess-A")
	state.NewRun()

	// 第一次
	id1 := state.NextMessageID()
	if !strings.HasPrefix(id1, "msg_") {
		t.Fatalf("id1 = %q, want prefix msg_", id1)
	}
	// 第二次
	id2 := state.NextMessageID()
	if id2 == id1 {
		t.Fatalf("id2 == id1, want unique: id1=%q id2=%q", id1, id2)
	}
	// 解析尾部的 seq 数字
	seq1 := mustExtractSeq(t, id1)
	seq2 := mustExtractSeq(t, id2)
	if seq2 != seq1+1 {
		t.Errorf("seq 增量 = %d, want %d (id1=%s id2=%s)", seq2, seq1+1, id1, id2)
	}

	// NewRun 后 seq 跨 run 全局递增（不重置）
	state.NewRun()
	id3 := state.NextMessageID()
	seq3 := mustExtractSeq(t, id3)
	if seq3 <= seq2 {
		t.Errorf("NewRun 后 seq 应继续递增, seq3=%d, seq2=%d", seq3, seq2)
	}

	// 并发安全：100 个 goroutine 同时调用，seq 应当全部唯一
	state2 := NewAGUIThreadState("sess-B")
	state2.NewRun()
	const N = 100
	ids := make([]string, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ids[i] = state2.NextMessageID()
		}(i)
	}
	wg.Wait()
	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			t.Errorf("重复 messageId = %q", id)
		}
		seen[id] = true
	}
	if len(seen) != N {
		t.Errorf("去重后 unique ids = %d, want %d", len(seen), N)
	}
}

// mustExtractSeq 提取 messageId `msg_<uuid>_<seq>` 末尾的 seq 数字。
func mustExtractSeq(t *testing.T, msgID string) int {
	t.Helper()
	// 格式: msg_<36-char-uuid>_<seq>
	idx := strings.LastIndex(msgID, "_")
	if idx < 0 || idx == len(msgID)-1 {
		t.Fatalf("messageId 格式错误: %q", msgID)
	}
	seqStr := msgID[idx+1:]
	seq, err := strconv.Atoi(seqStr)
	if err != nil {
		t.Fatalf("messageId 末尾 seq 解析失败: %q (err=%v)", seqStr, err)
	}
	return seq
}

// ─── 1.2: TestAGUIEventMapper_EmitsTextMessageStartBeforeContent ─

// 验证 EmitTextMessageStart 在 EmitTextMessageContent 之前。
func TestAGUIEventMapper_EmitsTextMessageStartBeforeContent(t *testing.T) {
	mapper, state, rec := newTestAGUIMapper("sess-start")

	msgID := state.NextMessageID()
	mapper.EmitTextMessageStart(msgID)
	mapper.EmitTextMessageContent(msgID, "hello")
	mapper.EmitTextMessageContent(msgID, " world")
	mapper.EmitTextMessageEnd(msgID)

	frames := parseSSEFrames(t, rec.Body.String())
	if len(frames) < 2 {
		t.Fatalf("期望 >= 2 frames (START + CONTENT), got %d", len(frames))
	}

	if frames[0].EventType != "TEXT_MESSAGE_START" {
		t.Errorf("frame[0].EventType = %q, want TEXT_MESSAGE_START", frames[0].EventType)
	}
	if frames[1].EventType != "TEXT_MESSAGE_CONTENT" {
		t.Errorf("frame[1].EventType = %q, want TEXT_MESSAGE_CONTENT", frames[1].EventType)
	}
	if frames[len(frames)-1].EventType != "TEXT_MESSAGE_END" {
		t.Errorf("最后一帧 = %q, want TEXT_MESSAGE_END", frames[len(frames)-1].EventType)
	}

	// 验证 START 的 role = "assistant"
	if role, _ := frames[0].DataJSON["role"].(string); role != "assistant" {
		t.Errorf("TEXT_MESSAGE_START.role = %q, want assistant", role)
	}
	// 验证 START 的 messageId 与 CONTENT 一致
	if startMsg, _ := frames[0].DataJSON["messageId"].(string); startMsg != msgID {
		t.Errorf("START.messageId = %q, want %q", startMsg, msgID)
	}
	if contentMsg, _ := frames[1].DataJSON["messageId"].(string); contentMsg != msgID {
		t.Errorf("CONTENT.messageId = %q, want %q", contentMsg, msgID)
	}
}

// ─── 1.3: TestAGUIEventMapper_StableMessageId ─────────────────

// 验证同一 message 多次 EmitTextMessageContent 共用同一 messageId。
func TestAGUIEventMapper_StableMessageId(t *testing.T) {
	mapper, state, rec := newTestAGUIMapper("sess-stable")

	msgID := state.NextMessageID()
	mapper.EmitTextMessageStart(msgID)
	mapper.EmitTextMessageContent(msgID, "foo")
	mapper.EmitTextMessageContent(msgID, "bar")
	mapper.EmitTextMessageContent(msgID, "baz")
	mapper.EmitTextMessageEnd(msgID)

	frames := parseSSEFrames(t, rec.Body.String())
	contentFrames := 0
	for _, f := range frames {
		if f.EventType == "TEXT_MESSAGE_CONTENT" {
			contentFrames++
			if got, _ := f.DataJSON["messageId"].(string); got != msgID {
				t.Errorf("CONTENT[%d].messageId = %q, want stable %q", contentFrames, got, msgID)
			}
		}
	}
	if contentFrames != 3 {
		t.Errorf("CONTENT 事件数 = %d, want 3", contentFrames)
	}
}

// ─── 1.4: TestAGUIEventMapper_EmptyArgs_SkipsTOOL_CALL_ARGS ───

// 验证 EmitToolCallArgs 收到空 delta 时不发任何 SSE 事件。
func TestAGUIEventMapper_EmptyArgs_SkipsTOOL_CALL_ARGS(t *testing.T) {
	mapper, _, rec := newTestAGUIMapper("sess-empty-args")

	// 先发一个 START 事件（用作 baseline）
	mapper.EmitToolCallStart("tc_1", "list_files")

	// 推空 args（应该被跳过）
	mapper.EmitToolCallArgs("tc_1", "")

	// 推非空 args（应该发送）
	mapper.EmitToolCallArgs("tc_1", `{"path":"/"}`)

	mapper.EmitToolCallEnd("tc_1")

	frames := parseSSEFrames(t, rec.Body.String())
	argsCount := 0
	for _, f := range frames {
		if f.EventType == "TOOL_CALL_ARGS" {
			argsCount++
		}
	}
	if argsCount != 1 {
		t.Errorf("TOOL_CALL_ARGS 事件数 = %d, want 1 (空 args 应被跳过)", argsCount)
	}

	// 同时验证 TOOL_CALL_END 仍然发送（spec §"字段缺失 graceful 降级"）
	endCount := 0
	for _, f := range frames {
		if f.EventType == "TOOL_CALL_END" {
			endCount++
		}
	}
	if endCount != 1 {
		t.Errorf("TOOL_CALL_END 事件数 = %d, want 1", endCount)
	}

	// 验证 START 仍然发送
	startCount := 0
	for _, f := range frames {
		if f.EventType == "TOOL_CALL_START" {
			startCount++
		}
	}
	if startCount != 1 {
		t.Errorf("TOOL_CALL_START 事件数 = %d, want 1", startCount)
	}
}

// ─── 1.5: TestAGUIEventMapper_AllEventsIncludeThreadIdRunIdTimestamp ─

// 验证所有 Emit* 事件都含 threadId / runId / timestamp 字段。
func TestAGUIEventMapper_AllEventsIncludeThreadIdRunIdTimestamp(t *testing.T) {
	mapper, state, rec := newTestAGUIMapper("sess-fields")
	expectedThread := state.ThreadId()
	expectedRun := state.RunId()

	// 推所有 11 种事件
	mapper.EmitRunStarted()
	msgID := state.NextMessageID()
	mapper.EmitTextMessageStart(msgID)
	mapper.EmitTextMessageContent(msgID, "hi")
	mapper.EmitTextMessageEnd(msgID)
	mapper.EmitToolCallStart("tc_x", "read_file")
	mapper.EmitToolCallArgs("tc_x", `{"path":"/a"}`)
	mapper.EmitToolCallEnd("tc_x")
	mapper.EmitToolCallResult("tc_x", `{"ok":true}`)
	mapper.EmitStateSnapshot(map[string]interface{}{"step": 1})
	mapper.EmitMessagesSnapshot([]string{"hello"})
	mapper.EmitRunFinished()

	frames := parseSSEFrames(t, rec.Body.String())
	if len(frames) < 11 {
		t.Errorf("frames 数量 = %d, want >= 11", len(frames))
	}

	timestampRe := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

	for i, f := range frames {
		// threadId 校验
		if got, _ := f.DataJSON["threadId"].(string); got != expectedThread {
			t.Errorf("frame[%d] (%s) threadId = %q, want %q", i, f.EventType, got, expectedThread)
		}
		// runId 校验
		if got, _ := f.DataJSON["runId"].(string); got != expectedRun {
			t.Errorf("frame[%d] (%s) runId = %q, want %q", i, f.EventType, got, expectedRun)
		}
		// timestamp 校验
		ts, _ := f.DataJSON["timestamp"].(string)
		if ts == "" {
			t.Errorf("frame[%d] (%s) timestamp 缺失", i, f.EventType)
		} else if !timestampRe.MatchString(ts) {
			t.Errorf("frame[%d] (%s) timestamp = %q 不符合 ISO 8601 毫秒格式", i, f.EventType, ts)
		}
	}
}

// ─── 1.6: TestAGUIEventMapper_EachEventHasTypeField ───────────

// 验证所有事件的 JSON 顶层都有 type 字段（与 event: 行重复保险）。
func TestAGUIEventMapper_EachEventHasTypeField(t *testing.T) {
	mapper, state, rec := newTestAGUIMapper("sess-typefield")

	// 推各种类型的事件
	mapper.EmitRunStarted()
	msgID := state.NextMessageID()
	mapper.EmitTextMessageStart(msgID)
	mapper.EmitTextMessageContent(msgID, "x")
	mapper.EmitTextMessageEnd(msgID)
	mapper.EmitToolCallStart("tc_1", "fn1")
	mapper.EmitToolCallArgs("tc_1", "a")
	mapper.EmitToolCallEnd("tc_1")
	mapper.EmitToolCallResult("tc_1", "r")
	mapper.EmitStateSnapshot(map[string]interface{}{})
	mapper.EmitMessagesSnapshot([]interface{}{})
	mapper.EmitRunFinished()

	frames := parseSSEFrames(t, rec.Body.String())
	for i, f := range frames {
		typ, ok := f.DataJSON["type"].(string)
		if !ok {
			t.Errorf("frame[%d] (%s) type 字段缺失", i, f.EventType)
			continue
		}
		if typ != f.EventType {
			t.Errorf("frame[%d] type=%q, event:行=%q 不一致", i, typ, f.EventType)
		}
	}
}

// ─── 1.7: TestAGUIEventMapper_MapEvent_CompatibilityLegacy ────

// 验证 MapEvent（旧 API）依然工作 — RUN_STARTED 输出含 threadId/runId/timestamp。
// 防止重构破坏 MockEngine.Run 路径。
func TestAGUIEventMapper_MapEvent_CompatibilityLegacy(t *testing.T) {
	mapper, state, rec := newTestAGUIMapper("sess-legacy")

	mapper.MapEvent(MockEvent{Type: "stream_start", Data: map[string]interface{}{}}, 0, 0)
	mapper.MapEvent(MockEvent{Type: "text_delta", Data: map[string]interface{}{"text": "hello"}}, 0, 1)
	mapper.MapEvent(MockEvent{Type: "stream_end", Data: map[string]interface{}{}}, 0, 2)

	frames := parseSSEFrames(t, rec.Body.String())
	if len(frames) != 3 {
		t.Fatalf("期望 3 frames, got %d", len(frames))
	}

	// RUN_STARTED: 应含 threadId/runId/timestamp
	if !hasAllFields(t, frames[0].DataJSON, "threadId", "runId", "timestamp", "type") {
		t.Errorf("RUN_STARTED 缺字段: %+v", frames[0].DataJSON)
	}
	if frames[0].EventType != "RUN_STARTED" {
		t.Errorf("frame[0].EventType = %q, want RUN_STARTED", frames[0].EventType)
	}
	// TEXT_MESSAGE_CONTENT: 应含 messageId/delta
	if frames[1].EventType != "TEXT_MESSAGE_CONTENT" {
		t.Errorf("frame[1].EventType = %q, want TEXT_MESSAGE_CONTENT", frames[1].EventType)
	}
	if got, _ := frames[1].DataJSON["delta"].(string); got != "hello" {
		t.Errorf("frame[1].delta = %q, want hello", got)
	}
	// RUN_FINISHED
	if frames[2].EventType != "RUN_FINISHED" {
		t.Errorf("frame[2].EventType = %q, want RUN_FINISHED", frames[2].EventType)
	}

	// threadId 应等于 state.ThreadId()
	if got, _ := frames[0].DataJSON["threadId"].(string); got != state.ThreadId() {
		t.Errorf("threadId = %q, want %q", got, state.ThreadId())
	}
}

// ─── 1.8: TestAGUIEventMapper_MapEvent_EmptyArgs_SkipsArgs ────

// 验证 MapEvent 路径（MockEngine 兼容）下，tool_call 空 args 跳过 TOOL_CALL_ARGS。
func TestAGUIEventMapper_MapEvent_EmptyArgs_SkipsArgs(t *testing.T) {
	mapper, _, rec := newTestAGUIMapper("sess-legacy-empty")

	mapper.MapEvent(MockEvent{
		Type: "tool_call",
		Data: map[string]interface{}{
			"id":   "tc_legacy",
			"name": "list_mounts",
			"args": "", // 空 args
		},
	}, 0, 0)

	frames := parseSSEFrames(t, rec.Body.String())
	argsCount := 0
	startCount := 0
	for _, f := range frames {
		switch f.EventType {
		case "TOOL_CALL_ARGS":
			argsCount++
		case "TOOL_CALL_START":
			startCount++
		}
	}
	if argsCount != 0 {
		t.Errorf("空 args 时 TOOL_CALL_ARGS 事件数 = %d, want 0", argsCount)
	}
	if startCount != 1 {
		t.Errorf("TOOL_CALL_START 事件数 = %d, want 1 (START 仍应发出)", startCount)
	}
}

// ─── 1.9: TestAGUIEventMapper_EmitStateSnapshot ───────────────

// 验证 EmitStateSnapshot 序列化完整 state 对象。
func TestAGUIEventMapper_EmitStateSnapshot(t *testing.T) {
	mapper, _, rec := newTestAGUIMapper("sess-state")

	state := map[string]interface{}{
		"contextWindow": 128000,
		"model":         "gpt-4o",
		"step":          3,
	}
	mapper.EmitStateSnapshot(state)

	frames := parseSSEFrames(t, rec.Body.String())
	if len(frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames))
	}
	f := frames[0]
	if f.EventType != "STATE_SNAPSHOT" {
		t.Errorf("EventType = %q, want STATE_SNAPSHOT", f.EventType)
	}
	snapState, ok := f.DataJSON["state"].(map[string]interface{})
	if !ok {
		t.Fatalf("state 字段类型错误: %T", f.DataJSON["state"])
	}
	if got, _ := snapState["model"].(string); got != "gpt-4o" {
		t.Errorf("state.model = %q, want gpt-4o", got)
	}
}

// ─── 1.10: TestAGUIEventMapper_EmitMessagesSnapshot ───────────

// 验证 EmitMessagesSnapshot 序列化 messages 数组。
func TestAGUIEventMapper_EmitMessagesSnapshot(t *testing.T) {
	mapper, _, rec := newTestAGUIMapper("sess-msgs")

	msgs := []map[string]interface{}{
		{"role": "user", "content": "hello"},
		{"role": "assistant", "content": "hi there"},
	}
	mapper.EmitMessagesSnapshot(msgs)

	frames := parseSSEFrames(t, rec.Body.String())
	if len(frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(frames))
	}
	if frames[0].EventType != "MESSAGES_SNAPSHOT" {
		t.Errorf("EventType = %q, want MESSAGES_SNAPSHOT", frames[0].EventType)
	}
	msgsArr, ok := frames[0].DataJSON["messages"].([]interface{})
	if !ok {
		t.Fatalf("messages 字段类型错误: %T", frames[0].DataJSON["messages"])
	}
	if len(msgsArr) != 2 {
		t.Errorf("messages 长度 = %d, want 2", len(msgsArr))
	}
}

// ─── 1.11: TestAGUIThreadState_ThreadId_DerivedFromSessID ─────

// 验证 threadId 派生规则：`thread_<sessID>`。
func TestAGUIThreadState_ThreadId_DerivedFromSessID(t *testing.T) {
	tests := []struct {
		sessID string
		want   string
	}{
		{"abc123", "thread_abc123"},
		{"sess-xyz", "thread_sess-xyz"},
		{"", "thread_default"}, // 空 sessID → "default"
	}
	for _, tt := range tests {
		t.Run(tt.sessID, func(t *testing.T) {
			state := NewAGUIThreadState(tt.sessID)
			if got := state.ThreadId(); got != tt.want {
				t.Errorf("ThreadId() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ─── 1.12: TestAGUIThreadState_NewRun_GeneratesUniqueIDs ──────

// 验证连续 NewRun 生成不同的 runId。
func TestAGUIThreadState_NewRun_GeneratesUniqueIDs(t *testing.T) {
	state := NewAGUIThreadState("sess-runs")
	state.NewRun()
	first := state.RunId()
	if first == "" {
		t.Fatal("NewRun 后 runId 为空")
	}
	// UUID v4 格式: 36 字符
	if len(first) != 36 {
		t.Errorf("runId 长度 = %d, want 36 (UUID)", len(first))
	}
	state.NewRun()
	second := state.RunId()
	if second == first {
		t.Errorf("两次 NewRun runId 相同: %q", first)
	}
}

// ─── 1.13: TestAGUIThreadState_CurrentTimestamp_Format ────────

// 验证 CurrentTimestamp 输出 ISO 8601 毫秒格式。
func TestAGUIThreadState_CurrentTimestamp_Format(t *testing.T) {
	state := NewAGUIThreadState("sess-ts")
	ts := state.CurrentTimestamp()

	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)
	if !re.MatchString(ts) {
		t.Errorf("timestamp = %q, 不符合 ISO 8601 毫秒格式", ts)
	}
}

// ─── 辅助 ────────────────────────────────────────────────────

// hasAllFields 检查 map 是否包含所有指定 key。
func hasAllFields(t *testing.T, m map[string]interface{}, keys ...string) bool {
	t.Helper()
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return false
		}
	}
	return true
}

// 防止 go vet 警告 unused（strconv 在 mustExtractSeq 用过，但 import 检测可能误判）
var _ = fmt.Sprintf
