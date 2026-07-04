package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// DefaultModelContextWindow is the model context window (in
// tokens) used when the Agent does not configure one explicitly.
// 128 000 is a conservative default that fits most modern
// long-context models (GPT-4o, Claude 3.x, etc.) without
// accidentally triggering compaction on a model that only has
// 4 096 tokens of context.
const DefaultModelContextWindow = 128_000

// DefaultCompactionThreshold is the fraction of the model
// context window that triggers an automatic compaction. 0.8
// means "compact when the running history exceeds 80% of the
// model's window". The remaining 20% is left for the next
// assistant turn and any tool-call responses.
const DefaultCompactionThreshold = 0.8

// SummaryFunc is the shape of the LLM-backed callback the
// Compactor uses to ask the model for a one-shot summary of
// the older messages. It is passed in as a parameter (rather
// than hard-wired to the agent's llmStream) so tests can swap
// in a deterministic implementation and so the Compactor
// stays a pure helper that has no opinion on which LLM client
// to use.
//
// The function returns the summary text. Any non-nil error is
// propagated verbatim to the Compactor.Compact caller.
type SummaryFunc func(ctx context.Context, toSummarise []openai.ChatCompletionMessage) (string, error)

// TokenEstimator turns a message slice into a token count.
// Implementations are intentionally pluggable: production
// could use tiktoken (offline) or a real tokenizer; tests use
// SimpleTokenEstimator (a 4-chars-per-token rule of thumb).
type TokenEstimator interface {
	CountTokens(text string) int
}

// SimpleTokenEstimator implements the "approximately 4 English
// characters == 1 token" rule of thumb. It is intentionally
// crude: the goal is to detect "about to blow past the model
// window", not to compute exact token counts. Replace with a
// real tokenizer (e.g. tiktoken-go) for production precision.
type SimpleTokenEstimator struct{}

// CountTokens returns ceil(len(text) / 4). Empty input returns
// 0. The function is total — it never panics on nil/odd inputs.
func (SimpleTokenEstimator) CountTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	// Integer ceiling: (n + 3) / 4
	return (len(text) + 3) / 4
}

// Compactor is the unit that decides when to compact a
// conversation and produces the replacement summary message.
//
// It is constructed once at Agent creation (or per-session if
// the model window varies) and reused across turns.
//
// The default Threshold is 0.8; the default ModelWindowTokens
// is DefaultModelContextWindow. NewCompactor(window) sets
// ModelWindowTokens to the supplied value (or the default if
// window <= 0) and leaves Threshold at the default. Tests can
// adjust Threshold directly after construction.
type Compactor struct {
	estimator         TokenEstimator
	modelWindowTokens int
	threshold         float64
}

// NewCompactor builds a Compactor with the supplied model
// context window. A non-positive window falls back to
// DefaultModelContextWindow; the threshold is left at
// DefaultCompactionThreshold (override via direct field
// assignment in tests).
func NewCompactor(windowTokens int) *Compactor {
	if windowTokens <= 0 {
		windowTokens = DefaultModelContextWindow
	}
	return &Compactor{
		estimator:         SimpleTokenEstimator{},
		modelWindowTokens: windowTokens,
		threshold:         DefaultCompactionThreshold,
	}
}

// SetEstimator swaps the token estimator. Returns the
// Compactor so the call can be chained. nil is rejected to
// keep ShouldCompact / Compact panic-free in production.
func (c *Compactor) SetEstimator(est TokenEstimator) *Compactor {
	if est == nil {
		est = SimpleTokenEstimator{}
	}
	c.estimator = est
	return c
}

// ModelWindowTokens returns the configured model context
// window. Useful for tests and for diagnostics logged when a
// compaction fires.
func (c *Compactor) ModelWindowTokens() int {
	return c.modelWindowTokens
}

// Threshold returns the configured compaction threshold
// fraction (0..1).
func (c *Compactor) Threshold() float64 {
	return c.threshold
}

// EstimateTokens returns the total estimated token count of
// the supplied messages. It walks each message's text fields
// and sums the estimator's output. Tool calls and tool
// results contribute their JSON representation so the estimate
// reflects "real" memory pressure (long tool arguments count).
func (c *Compactor) EstimateTokens(messages []openai.ChatCompletionMessage) int {
	if c.estimator == nil {
		c.estimator = SimpleTokenEstimator{}
	}
	total := 0
	for i := range messages {
		m := messages[i]
		if m.Content != "" {
			total += c.estimator.CountTokens(m.Content)
		}
		if m.ReasoningContent != "" {
			// o1-style reasoning: not in the response payload
			// sent back to the model, but it does consume the
			// model's context window on the previous turn, so
			// we still count it for compaction estimation.
			total += c.estimator.CountTokens(m.ReasoningContent)
		}
		if m.Name != "" {
			total += c.estimator.CountTokens(m.Name)
		}
		for j := range m.ToolCalls {
			tc := m.ToolCalls[j]
			total += c.estimator.CountTokens(tc.Function.Name)
			total += c.estimator.CountTokens(tc.Function.Arguments)
		}
	}
	return total
}

// ShouldCompact returns true when the estimated total tokens
// for messages exceeds threshold * modelWindow. A
// non-positive window or non-finite threshold returns false
// (defensive: the compactor is a no-op in that case rather
// than a panic).
func (c *Compactor) ShouldCompact(messages []openai.ChatCompletionMessage) bool {
	if c.modelWindowTokens <= 0 {
		return false
	}
	if c.threshold <= 0 {
		return false
	}
	limit := int(float64(c.modelWindowTokens) * c.threshold)
	if limit <= 0 {
		return false
	}
	return c.EstimateTokens(messages) > limit
}

// Compact is the synchronous compaction entry point. It
// returns:
//
//   - summaryMessage: a synthetic "system" message whose
//     Content is the LLM-generated summary. The agent inserts
//     this at the head of the messages slice so subsequent
//     turns see the compressed context instead of the dropped
//     history.
//   - replacedCount: the number of older messages that were
//     folded into the summary (excludes the system / summary
//     message itself).
//   - err: any error from summaryFn.
//
// Edge cases:
//   - messages has 0 or 1 entries → no-op, returns err=nil
//     and replacedCount=0.
//   - summaryFn returns an error → the function aborts and
//     returns the error verbatim. The caller MUST treat a
//     non-nil err as "do not modify messages" (no half-applied
//     state).
//   - The two most recent messages are always preserved
//     verbatim so the very last user turn and any
//     in-flight assistant tool_call stay intact for the LLM
//     to continue from.
func (c *Compactor) Compact(
	ctx context.Context,
	messages []openai.ChatCompletionMessage,
	summaryFn SummaryFunc,
) (summaryMessage openai.ChatCompletionMessage, replacedCount int, err error) {
	if summaryFn == nil {
		return openai.ChatCompletionMessage{}, 0, errors.New("agent: compactor: summaryFn must not be nil")
	}
	if len(messages) <= 1 {
		return openai.ChatCompletionMessage{}, 0, nil
	}

	// Always keep the last message verbatim. The
	// second-to-last is also preserved so the most recent
	// user/assistant pair (and any in-flight tool_call)
	// survives the round-trip into the next LLM call.
	keep := 1
	if len(messages) >= 2 {
		keep = 2
	}
	if keep >= len(messages) {
		// 0-1 message: nothing to compact.
		return openai.ChatCompletionMessage{}, 0, nil
	}
	older := messages[:len(messages)-keep]

	// Hand the older slice to the LLM (or the fake in tests)
	// and ask for a one-shot summary.
	summaryText, err := summaryFn(ctx, older)
	if err != nil {
		return openai.ChatCompletionMessage{}, 0, fmt.Errorf("agent: compactor: summaryFn: %w", err)
	}
	summaryText = strings.TrimSpace(summaryText)
	if summaryText == "" {
		// Defensive: the LLM sometimes echoes an empty
		// string for trivially short contexts. We still
		// proceed (the summary message carries a fallback
		// text) but flag it via replacedCount so the caller
		// can choose to skip the compaction if it prefers.
		summaryText = "(context compacted with no summary text)"
	}

	summaryMessage = openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Name:    "compaction",
		Content: summaryText,
	}
	return summaryMessage, len(older), nil
}

// NewOpenAISummaryFn returns a SummaryFunc that calls the
// supplied openai.Client with a non-streaming chat completion
// request. The prompt asks the model to summarise the older
// messages into 5-7 bullet points; the returned text is the
// assistant message content.
//
// The model is taken from agent.cfg.OpenAIModel — callers
// that need a different model can build their own SummaryFunc
// from the same building blocks (see Compactor tests for
// the mock pattern).
func NewOpenAISummaryFn(client *openai.Client, model string) SummaryFunc {
	if client == nil || model == "" {
		return nil
	}
	return func(ctx context.Context, toSummarise []openai.ChatCompletionMessage) (string, error) {
		if len(toSummarise) == 0 {
			return "", nil
		}
		// Build the prompt. The user-side system message
		// sets the role; the user payload is the older
		// conversation flattened to text.
		var b strings.Builder
		b.WriteString("You are a conversation compactor. The following is the older portion of a chat. ")
		b.WriteString("Produce a concise summary in 5-7 bullet points that preserves any facts, file paths, ")
		b.WriteString("tool results, decisions, and pending tasks the next assistant turn will need. ")
		b.WriteString("Do not invent content; if a fact is not present, omit it.\n\n")
		for i := range toSummarise {
			m := toSummarise[i]
			if m.Role == "" {
				continue
			}
			b.WriteString("[")
			b.WriteString(m.Role)
			b.WriteString("] ")
			if m.Content != "" {
				b.WriteString(m.Content)
			}
			if m.Name != "" {
				b.WriteString(" (name=")
				b.WriteString(m.Name)
				b.WriteString(")")
			}
			b.WriteString("\n")
		}
		req := openai.ChatCompletionRequest{
			Model: model,
			Messages: []openai.ChatCompletionMessage{
				{Role: openai.ChatMessageRoleSystem, Content: "You are a precise, faithful conversation summariser."},
				{Role: openai.ChatMessageRoleUser, Content: b.String()},
			},
		}
		resp, err := client.CreateChatCompletion(ctx, req)
		if err != nil {
			return "", err
		}
		if len(resp.Choices) == 0 {
			return "", errors.New("agent: compactor: openai returned no choices")
		}
		return resp.Choices[0].Message.Content, nil
	}
}

// nowMs returns the current time in unix milliseconds. It is
// a thin wrapper kept here (rather than used inline) so tests
// can substitute a fake clock by overriding this single
// function via build tags / test helpers.
func nowMs() int64 {
	return time.Now().UnixMilli()
}
