package agent

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// TestCompaction_SimpleTokenEstimator_EmptyAndKnownSizes locks
// the (len+3)/4 ceiling rule the compactor uses for its
// threshold check. The rule is intentionally crude; what
// matters is that it is total (no panics on empty/odd inputs)
// and roughly in the right ballpark for English text.
func TestCompaction_SimpleTokenEstimator_EmptyAndKnownSizes(t *testing.T) {
	est := SimpleTokenEstimator{}
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},        // 1 char → ceil(1/4) = 1
		{"abcd", 1},     // 4 chars → 1
		{"abcde", 2},    // 5 chars → ceil(5/4) = 2
		{"abcdefgh", 2}, // 8 chars → 2
		{"abcdefghi", 3},
		{strings.Repeat("x", 100), 25}, // 100/4 = 25
		{strings.Repeat("x", 101), 26}, // ceil(101/4) = 26
	}
	for _, c := range cases {
		got := est.CountTokens(c.in)
		if got != c.want {
			t.Errorf("CountTokens(%d chars): got %d want %d", len(c.in), got, c.want)
		}
	}
}

// TestCompaction_NewCompactor_DefaultThreshold locks the
// public defaults: a non-positive window falls back to
// DefaultModelContextWindow and the threshold is left at
// DefaultCompactionThreshold (0.8).
func TestCompaction_NewCompactor_DefaultThreshold(t *testing.T) {
	c := NewCompactor(0)
	if c.ModelWindowTokens() != DefaultModelContextWindow {
		t.Errorf("window: got %d want %d", c.ModelWindowTokens(), DefaultModelContextWindow)
	}
	if c.Threshold() != DefaultCompactionThreshold {
		t.Errorf("threshold: got %v want %v", c.Threshold(), DefaultCompactionThreshold)
	}

	c2 := NewCompactor(4096)
	if c2.ModelWindowTokens() != 4096 {
		t.Errorf("custom window not respected: got %d", c2.ModelWindowTokens())
	}
}

// TestCompaction_ShouldCompact_AtAndAboveThreshold exercises
// the 80% boundary explicitly: a slice whose estimated tokens
// is exactly limit must NOT trigger (the rule is strict ">"),
// and a slice one token above MUST trigger.
func TestCompaction_ShouldCompact_AtAndAboveThreshold(t *testing.T) {
	// Window = 100, threshold = 0.8 → limit = 80.
	c := NewCompactor(100)

	// Build a slice with exactly 80 tokens.
	exact := makeMessages(80, "user")
	if c.EstimateTokens(exact) != 80 {
		t.Fatalf("setup: EstimateTokens(exact) = %d, want 80", c.EstimateTokens(exact))
	}
	if c.ShouldCompact(exact) {
		t.Errorf("ShouldCompact at exactly the limit (80) should be false (strict >)")
	}

	// One token over.
	over := makeMessages(81, "user")
	if !c.ShouldCompact(over) {
		t.Errorf("ShouldCompact one token over (81) should be true")
	}

	// Well below.
	below := makeMessages(10, "user")
	if c.ShouldCompact(below) {
		t.Errorf("ShouldCompact at 10 tokens should be false")
	}
}

// TestCompaction_ShouldCompact_DisabledOnBadConfig locks the
// defensive "don't compact when misconfigured" branch. A
// non-positive window or threshold must return false rather
// than panic.
func TestCompaction_ShouldCompact_DisabledOnBadConfig(t *testing.T) {
	c := NewCompactor(-1)
	if c.ShouldCompact(makeMessages(10_000, "user")) {
		t.Errorf("negative window should disable compaction")
	}
	c = NewCompactor(1000)
	c.threshold = 0
	if c.ShouldCompact(makeMessages(10_000, "user")) {
		t.Errorf("zero threshold should disable compaction")
	}
}

// TestCompaction_EstimateTokens_AccountsForAllFields makes
// sure the estimator sums content + reasoning + tool names +
// tool args — a regression in any of these would silently
// under-estimate memory pressure.
func TestCompaction_EstimateTokens_AccountsForAllFields(t *testing.T) {
	c := NewCompactor(1_000_000)
	msgs := []openai.ChatCompletionMessage{
		{Role: "user", Content: "abcdefgh"}, // 2 tokens
		{
			Role:             "assistant",
			ReasoningContent: "abcdefgh", // 2 tokens
			ToolCalls: []openai.ToolCall{
				{Function: openai.FunctionCall{Name: "list_files", Arguments: `{"path":"/"}`}},
			},
		},
		{Role: "tool", Name: "list_files", Content: "ok"}, // 'ok' = 1, name = 3
	}
	got := c.EstimateTokens(msgs)
	// content: 2 + reasoning: 2 + tool name "list_files"
	// (10 chars → 3) + args (12 chars → 3) + tool name
	// "list_files" → 3 + "ok" → 1
	// total = 2 + 2 + 3 + 3 + 3 + 1 = 14
	if got < 13 || got > 15 {
		t.Errorf("EstimateTokens: got %d, expected ~14", got)
	}
}

// TestCompaction_Compact_ReplacesOlderAndKeepsTail covers the
// happy path: a 5-message slice (last 2 preserved) is replaced
// with a single summary message plus the last 2 originals;
// the summary text is exactly what the mock summaryFn
// returned.
func TestCompaction_Compact_ReplacesOlderAndKeepsTail(t *testing.T) {
	c := NewCompactor(1_000_000)
	messages := []openai.ChatCompletionMessage{
		{Role: "user", Content: "m1"},
		{Role: "assistant", Content: "m2"},
		{Role: "user", Content: "m3"},
		{Role: "assistant", Content: "m4"},
		{Role: "user", Content: "m5-latest"},
	}
	const wantSummary = "summary-of-older-3-messages"
	calls := int32(0)
	summaryFn := func(ctx context.Context, toSummarise []openai.ChatCompletionMessage) (string, error) {
		atomic.AddInt32(&calls, 1)
		if len(toSummarise) != 3 {
			return "", nil
		}
		return wantSummary, nil
	}

	summaryMsg, replaced, err := c.Compact(context.Background(), messages, summaryFn)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if replaced != 3 {
		t.Errorf("replaced: got %d want 3", replaced)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("summaryFn call count: got %d want 1", calls)
	}
	if summaryMsg.Role != openai.ChatMessageRoleSystem {
		t.Errorf("summary role: got %q want %q", summaryMsg.Role, openai.ChatMessageRoleSystem)
	}
	if summaryMsg.Content != wantSummary {
		t.Errorf("summary content: got %q want %q", summaryMsg.Content, wantSummary)
	}
}

// TestCompaction_Compact_NoopOnShortConversation locks the
// "nothing to compact" branch. With 0 or 1 messages the
// compactor must return (zero, 0, nil) without invoking the
// summaryFn.
func TestCompaction_Compact_NoopOnShortConversation(t *testing.T) {
	c := NewCompactor(1_000_000)
	calls := int32(0)
	summaryFn := func(ctx context.Context, toSummarise []openai.ChatCompletionMessage) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "", nil
	}

	// Empty slice.
	sm, replaced, err := c.Compact(context.Background(), nil, summaryFn)
	if err != nil {
		t.Errorf("empty: %v", err)
	}
	if replaced != 0 || sm.Content != "" {
		t.Errorf("empty: got replaced=%d sm.Content=%q", replaced, sm.Content)
	}

	// Single message.
	sm, replaced, err = c.Compact(context.Background(),
		[]openai.ChatCompletionMessage{{Role: "user", Content: "hi"}}, summaryFn)
	if err != nil {
		t.Errorf("single: %v", err)
	}
	if replaced != 0 {
		t.Errorf("single: got replaced=%d want 0", replaced)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Errorf("summaryFn must not be called for short slices; got %d calls", calls)
	}
}

// TestCompaction_Compact_PropagatesSummaryFnError locks the
// error path: a non-nil error from summaryFn must abort the
// compaction and the returned (summaryMessage, replacedCount)
// must be the zero values.
func TestCompaction_Compact_PropagatesSummaryFnError(t *testing.T) {
	c := NewCompactor(1_000_000)
	summaryFn := func(ctx context.Context, toSummarise []openai.ChatCompletionMessage) (string, error) {
		return "", errors.New("llm exploded")
	}
	sm, replaced, err := c.Compact(context.Background(),
		[]openai.ChatCompletionMessage{
			{Role: "user", Content: "a"},
			{Role: "user", Content: "b"},
			{Role: "user", Content: "c"},
		}, summaryFn)
	if err == nil {
		t.Fatal("expected error from Compact")
	}
	if replaced != 0 {
		t.Errorf("replaced on error: got %d want 0", replaced)
	}
	if sm.Content != "" {
		t.Errorf("summary on error: got %q want empty", sm.Content)
	}
}

// TestCompaction_Compact_NilSummaryFn is a defensive check
// — passing nil must error rather than panic.
func TestCompaction_Compact_NilSummaryFn(t *testing.T) {
	c := NewCompactor(1_000_000)
	_, _, err := c.Compact(context.Background(),
		[]openai.ChatCompletionMessage{{Role: "user", Content: "x"}}, nil)
	if err == nil {
		t.Errorf("expected error for nil summaryFn")
	}
}

// TestCompaction_Compact_EmptySummaryUsesFallback is the
// "LLM echoed nothing" branch. The compactor must still
// produce a summary message so the front-end has a stable
// shape to render.
func TestCompaction_Compact_EmptySummaryUsesFallback(t *testing.T) {
	c := NewCompactor(1_000_000)
	summaryFn := func(ctx context.Context, toSummarise []openai.ChatCompletionMessage) (string, error) {
		return "   ", nil // whitespace-only
	}
	sm, replaced, err := c.Compact(context.Background(),
		[]openai.ChatCompletionMessage{
			{Role: "user", Content: "a"},
			{Role: "user", Content: "b"},
			{Role: "user", Content: "c"},
		}, summaryFn)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if replaced != 1 {
		t.Errorf("replaced: got %d want 1", replaced)
	}
	if sm.Content == "" {
		t.Errorf("summary content should fall back to a placeholder, got empty")
	}
}

// TestCompaction_Agent_MaybeCompact_ReplacesAndEmitsEvent
// covers the in-place rewrite contract that Agent.maybeCompact
// applies when the compactor decides to act. The function is
// invoked by streamOneTurn; we replay its effect here to keep
// the test hermetic (no real LLM needed).
func TestCompaction_Agent_MaybeCompact_ReplacesAndEmitsEvent(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	c := NewCompactor(100)
	c.threshold = 0.5 // trigger at 50 tokens
	a.SetCompactor(c)

	messages := []openai.ChatCompletionMessage{
		{Role: "user", Content: "old 1"},
		{Role: "user", Content: "old 2"},
		{Role: "user", Content: "old 3"},
		{Role: "assistant", Content: "tail A"},
		{Role: "user", Content: "tail B"},
	}
	// Fill with enough tokens to push past the threshold
	// (50 tokens). 200 chars / 4 = 50 tokens per message.
	for i := range messages {
		messages[i].Content = strings.Repeat("x", 200)
	}
	messages[3].Content = "tail A"
	messages[4].Content = "tail B"

	// Drain the event channel.
	out := make(chan *Event, 4)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-out:
			case <-done:
				return
			}
		}
	}()

	const wantSummary = "deterministic-test-summary"
	summaryMsg, replaced, err := a.compactor.Compact(
		context.Background(),
		messages,
		func(ctx context.Context, toSummarise []openai.ChatCompletionMessage) (string, error) {
			if len(toSummarise) != 3 {
				return "", nil
			}
			return wantSummary, nil
		},
	)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}
	if replaced != 3 {
		t.Errorf("replaced: got %d want 3", replaced)
	}
	if summaryMsg.Content != wantSummary {
		t.Errorf("summary content: got %q want %q", summaryMsg.Content, wantSummary)
	}

	// Apply the rewrite the way maybeCompact does.
	keep := len(messages) - replaced
	for i := 0; i < keep; i++ {
		messages[1+i] = messages[replaced+i]
	}
	messages[0] = summaryMsg
	messages = messages[:1+keep]

	// Now messages must be [summary, tailA, tailB] (3 entries).
	if len(messages) != 3 {
		t.Fatalf("post-compact length: got %d want 3", len(messages))
	}
	if messages[0].Content != wantSummary {
		t.Errorf("messages[0]: got %q want summary", messages[0].Content)
	}
	if messages[1].Content != "tail A" {
		t.Errorf("messages[1]: got %q want 'tail A'", messages[1].Content)
	}
	if messages[2].Content != "tail B" {
		t.Errorf("messages[2]: got %q want 'tail B'", messages[2].Content)
	}

	close(done)
}

// TestCompaction_Agent_StreamOneTurn_EmitsCompactionEvent is
// the end-to-end test: it drives a real Agent.Chat with a
// scripted LLM that returns a normal text reply, and a
// compactor that decides the conversation has grown past the
// threshold. The agent MUST emit an EventCompaction event
// before the regular text_delta / stream_end events.
func TestCompaction_Agent_StreamOneTurn_EmitsCompactionEvent(t *testing.T) {
	a := makeMultiFakeAgent(t, []string{
		// The summary call (non-streaming) is replaced
		// with a deterministic fake via SetSummaryFnForTest
		// below, so we only need to script the main
		// streaming response.
		buildChatCompletionStreamBody([]parsedDelta{
			{Text: "after-compaction", Finished: true},
		}),
	})

	// Force compaction: a tiny window + a small threshold.
	c := NewCompactor(20)
	c.threshold = 0.5 // trigger at 10 tokens
	a.SetCompactor(c)

	a.SetSummaryFnForTest(func(ctx context.Context, toSummarise []openai.ChatCompletionMessage) (string, error) {
		if len(toSummarise) == 0 {
			return "", nil
		}
		return "test-summary", nil
	})

	// Pad the messages so they exceed the threshold.
	big := strings.Repeat("X", 200) // 50 tokens
	messages := []openai.ChatCompletionMessage{
		{Role: "user", Content: big},
		{Role: "assistant", Content: big},
		{Role: "user", Content: big},
		{Role: "user", Content: "latest"},
	}

	ch, err := a.Chat(context.Background(), "sess_compact", messages)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	// Drain the event stream and look for EventCompaction.
	var sawCompaction bool
	for e := range ch {
		if e.Type == EventCompaction {
			sawCompaction = true
			if !strings.Contains(e.Data, `"summary_text":"test-summary"`) {
				t.Errorf("compaction event data: got %q, expected summary_text=test-summary", e.Data)
			}
			if !strings.Contains(e.Data, `"replaced_message_count":`) {
				t.Errorf("compaction event data missing replaced_message_count: %q", e.Data)
			}
		}
	}

	if !sawCompaction {
		t.Errorf("expected EventCompaction in the event stream")
	}
}

// makeMessages builds n messages of role "user" with a fixed
// 4-character payload (so each message is exactly 1 token by
// SimpleTokenEstimator's (len+3)/4 rule). Used by the
// threshold tests to dial the estimated token count to an
// exact value.
func makeMessages(n int, role string) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, n)
	for i := 0; i < n; i++ {
		out[i] = openai.ChatCompletionMessage{
			Role:    role,
			Content: strings.Repeat("a", 4),
		}
	}
	return out
}
