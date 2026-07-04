// Package openai implements [ai.Provider] backed by
// github.com/sashabaranov/go-openai. It is the default
// provider for the agent — the [New] constructor takes a
// pre-configured *openai.Client and produces a provider that
// streams chat completions as [ai.Delta] values.
//
// The package is the receiver of the migration described in
// Task 18 of the "mobile-agent-2026-gap-analysis" spec. The
// legacy code in agent/openai.go (which builds a
// *openai.Client from an AgentConfig and calls
// CreateChatCompletionStream directly) is kept as a
// deprecated shim; new code paths should reach for this
// Provider instead.
package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	oa "github.com/sashabaranov/go-openai"

	"github.com/encv/agent/ai"
)

// Provider is the OpenAI-flavoured [ai.Provider]. It is safe
// for concurrent use from one goroutine (the chat loop) but
// not for concurrent StreamChat calls — each chat goroutine
// should construct (or receive) its own Provider instance.
type Provider struct {
	client *oa.Client
}

// New wraps a pre-configured *oa.Client into an [ai.Provider].
// The function does not ping the upstream API; the first
// StreamChat call is when the network is touched. A nil
// client is accepted (the failure is deferred to StreamChat
// time) so tests can construct a Provider before wiring a
// real client.
func New(client *oa.Client) *Provider {
	return &Provider{client: client}
}

// Name returns "openai" — the lowercased backend identifier.
// The value is stable across versions so log readers can
// pivot by backend without worrying about drift.
func (p *Provider) Name() string { return "openai" }

// StreamChat opens a streaming chat completion and returns a
// channel of [ai.Delta] values. The channel is closed when
// the stream ends (either naturally or because of an error).
// A non-nil error means the stream could not be opened at
// all; the channel is not returned in that case.
//
// The implementation bridges between the openai library's
// callback-shaped [oa.ChatCompletionStream] and the agent's
// channel-shaped [ai.Delta] stream. Each chunk from the
// openai library is translated into an [ai.Delta] in a
// single goroutine; the agent loop reads from the returned
// channel.
//
// The provider deliberately does NOT surface mid-stream
// errors as an extra [Delta] value: the openai library
// surfaces them via stream.Recv() returning a non-EOF
// error, which we treat as "the stream is done, close the
// channel". The agent loop's own retry / error policy is
// the right place for the more granular error handling.
func (p *Provider) StreamChat(ctx context.Context, req ai.StreamRequest) (<-chan ai.Delta, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("ai/openai: provider is not initialised (nil client)")
	}
	if req.Model == "" {
		return nil, errors.New("ai/openai: model is required")
	}
	oaReq, err := toOpenAIRequest(req)
	if err != nil {
		return nil, fmt.Errorf("ai/openai: build request: %w", err)
	}
	stream, err := p.client.CreateChatCompletionStream(ctx, oaReq)
	if err != nil {
		return nil, fmt.Errorf("ai/openai: create stream: %w", err)
	}
	out := make(chan ai.Delta, 32)
	go func() {
		defer close(out)
		defer stream.Close()
		for {
			chunk, err := stream.Recv()
			if err != nil {
				// EOF is the natural end of the
				// stream; anything else is a
				// mid-flight error. In both
				// cases we just close the
				// channel — the agent loop's
				// "channel closed without a
				// FinishReason" branch is the
				// canonical "stream failed"
				// path.
				return
			}
			delta, ok := fromOpenAIResponse(chunk)
			if !ok {
				// Usage-only / sentinel
				// chunk: nothing to emit.
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

// toOpenAIRequest translates an [ai.StreamRequest] into an
// [oa.ChatCompletionRequest]. The conversion is a pure
// projection: each field maps 1:1 except for tools (which
// the openai library wants as []oa.Tool with an embedded
// *FunctionDefinition). Temperature / MaxTokens are
// forwarded when non-zero; an empty MaxTokens is left
// unset so the upstream's own default kicks in.
func toOpenAIRequest(req ai.StreamRequest) (oa.ChatCompletionRequest, error) {
	out := oa.ChatCompletionRequest{
		Model:    req.Model,
		Messages: make([]oa.ChatCompletionMessage, 0, len(req.Messages)),
		Tools:    make([]oa.Tool, 0, len(req.Tools)),
		Stream:   true,
	}
	if req.Temperature != 0 {
		out.Temperature = float32(req.Temperature)
	}
	if req.MaxTokens > 0 {
		out.MaxTokens = req.MaxTokens
	}
	for _, m := range req.Messages {
		out.Messages = append(out.Messages, oa.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	for _, t := range req.Tools {
		var params map[string]any
		if len(t.Parameters) > 0 {
			if err := json.Unmarshal(t.Parameters, &params); err != nil {
				return out, fmt.Errorf("tool %q parameters: %w", t.Name, err)
			}
		}
		out.Tools = append(out.Tools, oa.Tool{
			Type: oa.ToolTypeFunction,
			Function: &oa.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}
	return out, nil
}

// fromOpenAIResponse projects a single openai chunk into one
// (or zero) [ai.Delta]. The second return is false when the
// chunk has no payload (e.g. usage-only sentinel, or a chunk
// with no Content / Reasoning / ToolCalls / FinishReason),
// in which case the caller skips the send.
//
// Tool-call fragments are accumulated by Index; the openai
// library assigns the same Index to all deltas belonging to
// the same tool call, mirroring the spec the agent loop
// already understands. The openai library types Index as
// *int and FinishReason as the named string type
// [openai.FinishReason]; both need explicit dereferencing
// / conversion to land in [ai.Delta] / [ai.ToolCallData].
func fromOpenAIResponse(r oa.ChatCompletionStreamResponse) (ai.Delta, bool) {
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
