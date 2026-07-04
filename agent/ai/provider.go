// Package ai defines the LLM Provider abstraction used by the
// agent. The agent core depends only on the [Provider] interface
// so individual model backends (OpenAI, Anthropic, Gemini, ...)
// can be swapped without touching the agent loop.
//
// The transport itself is intentionally tiny: a [Provider]
// receives a normalised [StreamRequest] and returns a channel of
// [Delta] values. The agent loop does the rest — text-delta
// accumulation, tool-call accumulation, finish detection, and
// error propagation. This keeps the per-backend code small (it
// just translates between the wire format and our internal
// types) and makes the per-backend code easy to fake in tests.
//
// The package is the recipient of Task 18 in the
// "mobile-agent-2026-gap-analysis" spec — the new home for the
// streaming logic that used to live entirely in
// agent/openai.go. The agent package keeps the existing
// openai-backed stream as a deprecated fallback (so the
// existing test suite keeps working) but new code should reach
// for [Provider] / [StreamRequest] / [Delta] instead.
package ai

import (
	"context"
	"encoding/json"
)

// Delta is one streaming chunk emitted by a Provider. The
// fields are mutually compatible across providers — content
// and reasoning are independent streams of unicode fragments,
// the tool call (if any) is the head of a tool invocation,
// and FinishReason is the LLM's signal that no more deltas
// will arrive on this stream.
//
// The agent loop folds successive deltas into a single
// assistant message in the running conversation: text/reasoning
// concatenate, tool calls are matched by ToolCall.Index, and
// any FinishReason terminates the stream.
type Delta struct {
	// Content is an incremental text fragment. Empty
	// when this delta carries reasoning or a tool call
	// instead. Concatenating all deltas' Content fields
	// in stream order reconstructs the full assistant
	// message.
	Content string

	// Reasoning is an incremental reasoning fragment
	// (e.g. OpenAI o1's reasoning_content or Claude's
	// thinking blocks). Empty on providers / models that
	// do not expose a separate reasoning stream.
	Reasoning string

	// ToolCall is one tool invocation requested by the
	// LLM. nil when this delta carries only text. The
	// agent loop accumulates tool calls by Index, with
	// the ID / Name / Arguments fields being
	// incrementally populated across successive deltas
	// (providers send them piecewise).
	ToolCall *ToolCallData

	// FinishReason is the LLM's "I'm done" signal
	// (e.g. "stop", "tool_calls", "end_turn", "stop_sequence").
	// Empty when the stream continues. When non-empty,
	// the agent loop must stop reading from the channel
	// and fold whatever has been collected so far.
	FinishReason string
}

// ToolCallData is one tool invocation requested by the LLM.
// ID and Name come from the delta directly; Arguments is the
// running concatenation of all delta.Arguments fragments —
// providers must emit the same Index for the same tool call
// across multiple deltas (mirroring OpenAI's streaming
// behaviour). The agent loop reassembles the final Arguments
// string when the stream terminates.
//
// This type lives in the ai package so individual providers
// (OpenAI, Anthropic, Gemini) can populate it without
// importing the agent package. The agent core converts each
// ai.ToolCallData into its internal agent.ToolCallData shape
// at the boundary.
type ToolCallData struct {
	ID        string
	Name      string
	Arguments string
	// Index disambiguates concurrent tool-call fragments
	// in the same stream. Providers that send only one
	// tool call at a time may set it to 0. Providers that
	// send multiple MUST assign distinct indices to
	// distinct tool calls.
	Index int
}

// Provider is the agent's LLM backend. Implementations must
// translate a normalised [StreamRequest] into a channel of
// [Delta] values, one per chunk, and close the channel when
// the stream ends. Errors are reported by returning a
// non-nil error from StreamChat (the stream cannot be
// opened) and by closing the channel without any further
// sends (the stream failed mid-flight). The agent loop
// treats both signals as "stream failed".
//
// Implementations are expected to be safe for concurrent use
// from a single goroutine (the chat loop). Concurrent use
// from multiple goroutines is not part of the contract —
// each chat goroutine should construct (or receive) its own
// Provider instance.
type Provider interface {
	// Name returns a stable identifier for the backend
	// ("openai", "anthropic", "gemini", ...). Used in
	// logs and metrics; the value MUST be lowercase and
	// contain no whitespace.
	Name() string

	// StreamChat opens a streaming chat completion. The
	// returned channel emits one or more [Delta] values
	// and is closed when the stream ends (naturally or
	// because the provider encountered an error). A
	// non-nil error return means the stream could not be
	// opened; in that case the channel is not returned
	// and the agent loop surfaces the error verbatim.
	StreamChat(ctx context.Context, req StreamRequest) (<-chan Delta, error)
}

// StreamRequest is the provider-agnostic shape of a chat
// completion request. Each field is a normalised projection
// of the corresponding provider-specific field; providers
// translate into the backend's native format internally.
//
// Tools is the list of tools the model may call. The agent
// passes the union of all registered tools; providers that
// need a specific encoding (e.g. Gemini's
// function_declarations vs Anthropic's input_schema) convert
// the slice internally. The Name / Description / Parameters
// contract is intentionally the same as OpenAI's
// function-calling schema so the registry can produce
// ToolSpec values without knowing the target backend.
type StreamRequest struct {
	Model       string
	Messages    []Message
	Tools       []ToolSpec
	Temperature float64
	MaxTokens   int
}

// Message is one turn in the conversation. Role is one of
// "system", "user", "assistant", "tool" (matching OpenAI's
// vocabulary so the agent's existing role constants can be
// reused). Content is the raw string the LLM should see;
// structured content (vision, multimodal) is flattened by
// the agent before the call so the provider does not have
// to know about it.
type Message struct {
	Role    string
	Content string
}

// ToolSpec is one function-calling tool the model may
// invoke. Parameters is a JSON-Schema fragment serialised
// as raw JSON so providers can pass it through verbatim
// without re-parsing.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}
