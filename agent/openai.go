package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	openai "github.com/sashabaranov/go-openai"
)

// llmStream abstracts the OpenAI streaming client so the agent core
// can be unit-tested without making real HTTP calls. Production
// wires this to *openai.Client; tests can plug in a fake.
type llmStream interface {
	CreateChatCompletionStream(ctx context.Context, req openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error)
}

// openaiStream wraps a real *openai.Client so the agent can swap it
// for a fake in tests.
type openaiStream struct {
	client *openai.Client
}

func newOpenAIClient(cfg AgentConfig) *openai.Client {
	oc := openai.DefaultConfig(cfg.OpenAIAPIKey)
	if cfg.OpenAIBaseURL != "" {
		oc.BaseURL = cfg.OpenAIBaseURL
	}
	return openai.NewClientWithConfig(oc)
}

func (s *openaiStream) CreateChatCompletionStream(ctx context.Context, req openai.ChatCompletionRequest) (*openai.ChatCompletionStream, error) {
	return s.client.CreateChatCompletionStream(ctx, req)
}

// chatTools converts the registry's schemas to the OpenAI
// "tools" field. Each schema is expected to be either an
// openai.Tool or a JSON-Schema fragment that can be unmarshalled
// to one.
func (a *Agent) chatTools() []openai.Tool {
	schemas := a.Registry.GetAllSchemas()
	out := make([]openai.Tool, 0, len(schemas))
	for _, s := range schemas {
		switch v := s.(type) {
		case openai.Tool:
			out = append(out, v)
		case *openai.Tool:
			if v != nil {
				out = append(out, *v)
			}
		case map[string]any:
			if t, ok := convertMapToTool(v); ok {
				out = append(out, t)
			}
		case *openai.FunctionDefinition:
			if v != nil {
				out = append(out, openai.Tool{Type: openai.ToolTypeFunction, Function: v})
			}
		case openai.FunctionDefinition:
			fn := v
			out = append(out, openai.Tool{Type: openai.ToolTypeFunction, Function: &fn})
		}
	}
	return out
}

func convertMapToTool(m map[string]any) (openai.Tool, bool) {
	raw, err := json.Marshal(m)
	if err != nil {
		return openai.Tool{}, false
	}
	var t openai.Tool
	if err := json.Unmarshal(raw, &t); err != nil || t.Type == "" {
		// The schema may be a bare FunctionDefinition; wrap it.
		var fn openai.FunctionDefinition
		if err := json.Unmarshal(raw, &fn); err == nil && fn.Name != "" {
			return openai.Tool{Type: openai.ToolTypeFunction, Function: &fn}, true
		}
		return openai.Tool{}, false
	}
	return t, true
}

// chatRequest builds the OpenAI request for one turn.
func (a *Agent) chatRequest(messages []openai.ChatCompletionMessage, model string) openai.ChatCompletionRequest {
	return openai.ChatCompletionRequest{
		Model:    model,
		Messages: messages,
		Tools:    a.chatTools(),
		Stream:   true,
	}
}

// parsedDelta is the projection of an
// openai.ChatCompletionStreamChoice. We collapse the entire
// struct into primitives that the agent loop can act on without
// depending on the OpenAI types directly (which makes it easy to
// swap in a fake stream).
type parsedDelta struct {
	Text      string
	Reasoning string
	ToolCalls []parsedToolCall
	Finished  bool
}

// parsedToolCall is one tool invocation requested by the LLM. ID
// and Name come from the delta directly; Arguments is the running
// concatenation of all delta.Arguments fragments (OpenAI sends them
// piecewise).
type parsedToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// parseDelta converts a single
// openai.ChatCompletionStreamResponse into a parsedDelta. The
// response may be empty (e.g. when the stream emits a usage
// chunk); in that case we return Finished=true so the caller can
// break out of the loop.
func parseDelta(resp openai.ChatCompletionStreamResponse) parsedDelta {
	out := parsedDelta{}
	if len(resp.Choices) == 0 {
		// No choices means usage-only or sentinel chunk; the
		// stream is done from the LLM's perspective.
		out.Finished = true
		return out
	}
	choice := resp.Choices[0]
	d := choice.Delta
	if d.Content != "" {
		out.Text = d.Content
	}
	if d.ReasoningContent != "" {
		// Only the o1 family populates this field; older
		// models leave it empty.
		out.Reasoning = d.ReasoningContent
	}
	for _, tc := range d.ToolCalls {
		ptc := parsedToolCall{}
		if tc.ID != "" {
			ptc.ID = tc.ID
		}
		if tc.Function.Name != "" {
			ptc.Name = tc.Function.Name
		}
		if tc.Function.Arguments != "" {
			ptc.Arguments += tc.Function.Arguments
		}
		out.ToolCalls = append(out.ToolCalls, ptc)
	}
	if choice.FinishReason != "" {
		out.Finished = true
	}
	return out
}

// readStream drains the stream into a slice of parsedDeltas and
// returns them. The stream is closed by the caller via
// defer stream.Close().
func readStream(stream *openai.ChatCompletionStream) ([]parsedDelta, error) {
	var out []parsedDelta
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, fmt.Errorf("openai stream recv: %w", err)
		}
		out = append(out, parseDelta(chunk))
	}
}
