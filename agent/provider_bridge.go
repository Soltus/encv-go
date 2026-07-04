package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	openai "github.com/sashabaranov/go-openai"

	"github.com/encv/agent/ai"
)

// legacyLLMProvider is the bridge between the new
// [ai.Provider] abstraction and the agent's legacy
// `llmStream` field. It lets the existing test suite
// (which injects fakes via [NewAgentWithLLM]) keep working
// unchanged while the agent's hot path migrates to consume
// [ai.Delta] values.
//
// The bridge resolves a.llm at *call* time, not at
// construction time, so tests that mutate a.llm after the
// agent is built (a common pattern in agent_test.go) still
// see their fake picked up by the next StreamChat call.
// This is a deliberate divergence from the convention
// elsewhere in the agent (other helpers snapshot
// dependencies in their struct value) because the existing
// test contract is "a.llm is the canonical test seam".
type legacyLLMProvider struct {
	agent *Agent
}

// Name returns "legacy" so log readers can pivot by
// backend. The value is intentionally distinct from
// "openai" so a future operator can tell, from logs alone,
// whether the stream went through the new Provider
// abstraction (e.g. a real Anthropic provider) or through
// the openai-shaped bridge.
func (p *legacyLLMProvider) Name() string { return "legacy" }

// StreamChat opens a streaming chat completion by
// delegating to the agent's legacy `llmStream` field and
// draining the openai stream into a channel of [ai.Delta]
// values. The agent's hot path is supposed to consume
// [ai.Delta] from this channel and emit the same
// assistant / tool-call / finish events it does today —
// the bridge is a translation layer, not a behaviour
// change.
//
// The bridge deliberately does not handle context
// cancellation: it forwards the ctx to the underlying
// CreateChatCompletionStream call, which is the layer that
// owns the network connection. The drain loop respects
// ctx via the select-on-send below.
func (p *legacyLLMProvider) StreamChat(ctx context.Context, req ai.StreamRequest) (<-chan ai.Delta, error) {
	if p == nil || p.agent == nil {
		return nil, errors.New("agent: legacyLLMProvider not initialised")
	}
	llm := p.agent.llm
	if llm == nil {
		return nil, errors.New("agent: no llm configured (legacy bridge has nothing to delegate to)")
	}
	if req.Model == "" {
		return nil, errors.New("agent: stream request missing model")
	}
	oaReq, err := legacyToOpenAIRequest(req, p.agent.chatTools())
	if err != nil {
		return nil, fmt.Errorf("agent: build openai request: %w", err)
	}
	stream, err := llm.CreateChatCompletionStream(ctx, oaReq)
	if err != nil {
		return nil, fmt.Errorf("create chat stream: %w", err)
	}
	out := make(chan ai.Delta, 32)
	go func() {
		defer close(out)
		defer stream.Close()
		for {
			chunk, recvErr := stream.Recv()
			if recvErr != nil {
				// EOF is the natural end of the
				// stream; anything else is a
				// mid-flight error. In both cases
				// we just close the channel — the
				// agent's "channel closed" branch
				// is the canonical "stream done"
				// path.
				return
			}
			delta, ok := openaiChunkToAIDelta(chunk)
			if !ok {
				// Usage-only / sentinel chunk
				// with no payload; skip.
				continue
			}
			select {
			case <-ctx.Done():
				return
			case out <- delta:
			}
		}
	}()
	return out, nil
}

// legacyToOpenAIRequest translates an [ai.StreamRequest]
// into an [openai.ChatCompletionRequest], reusing the
// existing chatTools() output for the tool list. The
// conversion is a pure projection: each ai.Message becomes
// a [openai.ChatCompletionMessage]; each ai.ToolSpec
// becomes a [openai.Tool]. Temperature / MaxTokens are
// forwarded when non-zero so the upstream's own default
// kicks in otherwise.
func legacyToOpenAIRequest(req ai.StreamRequest, oaTools []openai.Tool) (openai.ChatCompletionRequest, error) {
	out := openai.ChatCompletionRequest{
		Model:    req.Model,
		Messages: make([]openai.ChatCompletionMessage, 0, len(req.Messages)),
		Tools:    make([]openai.Tool, 0, len(oaTools)),
		Stream:   true,
	}
	if req.Temperature != 0 {
		out.Temperature = float32(req.Temperature)
	}
	if req.MaxTokens > 0 {
		out.MaxTokens = req.MaxTokens
	}
	for _, m := range req.Messages {
		out.Messages = append(out.Messages, openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	for _, t := range oaTools {
		out.Tools = append(out.Tools, t)
	}
	return out, nil
}

// openaiChunkToAIDelta projects a single openai chunk into
// one (or zero) [ai.Delta]. The second return is false
// when the chunk has no payload (e.g. usage-only sentinel
// or a chunk with no Content / Reasoning / ToolCalls /
// FinishReason), in which case the caller skips the send.
//
// The function is the in-package mirror of
// [aiprovopenai.fromOpenAIResponse]: they implement the
// same projection. Keeping the copy in the agent package
// avoids a circular import (aiprovopenai imports the
// openai SDK directly, and the agent package already
// re-exports several openai types) and lets the test suite
// use a different projection in the future without
// touching aiprovopenai.
//
// The openai library types Index as *int and FinishReason
// as the named string type [openai.FinishReason]; both
// need explicit dereferencing / conversion to land in the
// agent's [ai.Delta] / [ai.ToolCallData] shape.
func openaiChunkToAIDelta(r openai.ChatCompletionStreamResponse) (ai.Delta, bool) {
	if len(r.Choices) == 0 {
		return ai.Delta{}, false
	}
	c := r.Choices[0]
	d := c.Delta
	out := ai.Delta{
		Content:   d.Content,
		Reasoning: d.ReasoningContent,
	}
	hasPayload := d.Content != "" || d.ReasoningContent != "" || c.FinishReason != ""
	if len(d.ToolCalls) > 0 {
		tc := d.ToolCalls[0]
		tcd := &ai.ToolCallData{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		}
		if tc.Index != nil {
			tcd.Index = *tc.Index
		}
		out.ToolCall = tcd
		hasPayload = true
	}
	if c.FinishReason != "" {
		out.FinishReason = string(c.FinishReason)
	}
	if !hasPayload {
		return ai.Delta{}, false
	}
	return out, true
}

// aiDeltaToParsed projects an [ai.Delta] into the agent's
// internal [parsedDelta] shape. The agent's hot path
// (streamOneTurn) was written against parsedDelta before
// the Provider abstraction existed; this projection keeps
// the hot path's event-accumulation logic unchanged so we
// only have to swap the source of deltas, not the
// downstream processing.
//
// The projection is the inverse of [openaiChunkToAIDelta]:
// what one direction encodes, the other decodes.
func aiDeltaToParsed(d ai.Delta) parsedDelta {
	out := parsedDelta{
		Text:      d.Content,
		Reasoning: d.Reasoning,
	}
	if d.ToolCall != nil {
		out.ToolCalls = []parsedToolCall{{
			ID:        d.ToolCall.ID,
			Name:      d.ToolCall.Name,
			Arguments: d.ToolCall.Arguments,
		}}
	}
	if d.FinishReason != "" {
		out.Finished = true
	}
	return out
}

// toStreamRequest builds the [ai.StreamRequest] shape from
// the agent's internal state (rolling history +
// AgentConfig). The conversion is intentionally local —
// callers (the agent's hot path) pass the messages slice
// in the openai-native shape and we project to
// [ai.Message] inline. The tool list comes from chatTools
// (the existing openai-flavoured tool encoder) and is
// re-encoded into [ai.ToolSpec] format via JSON round-trip:
// chatTools' Parameters field is a map[string]any, and
// the provider wants raw JSON.
func (a *Agent) toStreamRequest(messages []openai.ChatCompletionMessage, model string) ai.StreamRequest {
	if model == "" {
		model = a.cfg.OpenAIModel
	}
	aiMsgs := make([]ai.Message, 0, len(messages))
	for _, m := range messages {
		aiMsgs = append(aiMsgs, ai.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	oaTools := a.chatTools()
	aiTools := make([]ai.ToolSpec, 0, len(oaTools))
	for _, t := range oaTools {
		if t.Function == nil {
			continue
		}
		var params json.RawMessage
		if t.Function.Parameters != nil {
			if b, err := json.Marshal(t.Function.Parameters); err == nil {
				params = b
			}
		}
		aiTools = append(aiTools, ai.ToolSpec{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  params,
		})
	}
	return ai.StreamRequest{
		Model:    model,
		Messages: aiMsgs,
		Tools:    aiTools,
	}
}
