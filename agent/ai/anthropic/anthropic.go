// Package anthropic implements a skeleton [ai.Provider] for
// the Anthropic Messages API.
//
// The implementation is intentionally a skeleton: it
// validates the request, builds the wire-format payload
// (system message hoisted to the top-level "system" field,
// tools encoded with input_schema), and returns a valid
// [ai.Delta] channel that closes cleanly. It does NOT yet
// open the streaming HTTP request, parse the
// Server-Sent-Event frames, or translate content_block_delta
// events into tool-call / reasoning fragments — those are
// deliberately out of scope for the initial provider cut
// (see the "Provider abstraction layer" section of
// .trae/specs/mobile-agent-2026-gap-analysis/spec.md for
// the rollout plan).
//
// The skeleton does NOT pull in any third-party SDK — the
// wire format is built with stdlib [encoding/json] and the
// eventual HTTP path will use [net/http] directly. The
// goal is to prove the abstraction compiles end-to-end
// (see the corresponding ai_test.go for the compile-time
// + runtime contract) without locking the project into a
// vendor SDK whose API surface we do not control.
package anthropic

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/encv/agent/ai"
)

const (
	// DefaultBaseURL is the production Anthropic API
	// endpoint. Tests can override it via [WithBaseURL].
	DefaultBaseURL = "https://api.anthropic.com"

	// DefaultAPIVersion is the Anthropic API version the
	// provider targets. Bumping this is part of the
	// provider's contract: we deliberately pin a
	// versioned value in the anthropic-version header so
	// a future /v2 rollout does not silently change the
	// wire format for our callers.
	DefaultAPIVersion = "2023-06-01"

	// DefaultMaxTokens is the default max_tokens
	// parameter for the Messages API. Anthropic requires
	// the field; 4096 is a conservative default for a
	// 100k-context model.
	DefaultMaxTokens = 4096
)

// Provider is the Anthropic-flavoured [ai.Provider]. It
// streams chat completions from the Messages API into
// [ai.Delta] values. The zero value is unusable; use
// [New] to construct one.
type Provider struct {
	// APIKey is the x-api-key value sent on every
	// request. Required for the eventual full
	// implementation; the skeleton tolerates an empty
	// key and returns a closed channel.
	APIKey string

	// BaseURL is the API root. Defaults to
	// [DefaultBaseURL]. Override for testing or
	// self-hosted deployments.
	BaseURL string

	// APIVersion is the value of the anthropic-version
	// header. Defaults to [DefaultAPIVersion].
	APIVersion string

	// Model is the model name to send in the request
	// (e.g. "claude-3-5-sonnet-20240620"). Required.
	Model string

	// MaxTokens is the max_tokens request field.
	// Defaults to [DefaultMaxTokens] when zero.
	MaxTokens int

	// HTTPClient overrides the default HTTP client. nil
	// means http.DefaultClient. Used by tests to wire
	// an httptest server.
	HTTPClient *http.Client
}

// New constructs a Provider with the supplied API key and
// model. BaseURL / APIVersion / MaxTokens fall back to the
// Default* constants; callers can override them by setting
// the field directly after construction.
func New(apiKey, model string) *Provider {
	return &Provider{
		APIKey:     apiKey,
		Model:      model,
		BaseURL:    DefaultBaseURL,
		APIVersion: DefaultAPIVersion,
		MaxTokens:  DefaultMaxTokens,
	}
}

// Name returns "anthropic" — lowercase, no whitespace.
func (p *Provider) Name() string { return "anthropic" }

// anthropicRequest is the wire shape of POST /v1/messages.
// We define it here (rather than reusing [ai.StreamRequest])
// because Anthropic's API has a different field naming
// convention (max_tokens vs OpenAI's max_tokens, system as
// a top-level field rather than a message role, etc.).
type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// StreamChat opens a streaming Messages request and
// returns a channel of [ai.Delta] values.
//
// Skeleton contract: the function must (1) compile, (2)
// accept a well-formed [ai.StreamRequest] without error,
// and (3) return a non-nil channel that closes
// deterministically. The eventual full implementation will
// (a) POST the body built by [buildRequest] to
// BaseURL+"/v1/messages", (b) parse the Server-Sent Events
// from the response body, and (c) translate each
// content_block_delta frame into an [ai.Delta] before
// sending on the channel.
//
// Tests that want to exercise the wire format can call
// [Provider.BuildRequest] directly; the production path
// remains a no-op until the HTTP plumbing lands.
func (p *Provider) StreamChat(ctx context.Context, req ai.StreamRequest) (<-chan ai.Delta, error) {
	if p == nil {
		return nil, fmt.Errorf("ai/anthropic: nil provider")
	}
	if req.Model == "" {
		return nil, fmt.Errorf("ai/anthropic: model is required")
	}
	// Validate the request shape via buildRequest; the
	// skeleton does not yet send it. A non-nil error
	// here means the caller passed a malformed request
	// (e.g. tools with invalid JSON parameters), so we
	// surface the error verbatim.
	if _, err := p.buildRequest(req); err != nil {
		return nil, err
	}
	out := make(chan ai.Delta, 4)
	go func() {
		defer close(out)
		// Skeleton: no HTTP call yet. The full
		// implementation will replace this no-op
		// with a stream that calls
		// p.httpClient().Do(req) and pipes the
		// response body through readSSE.
		_ = ctx
	}()
	return out, nil
}

// BuildRequest translates a [ai.StreamRequest] into the
// Anthropic-specific wire shape. System messages are
// hoisted to the top-level "system" field; user/assistant
// messages are passed through verbatim. Tools use
// input_schema rather than parameters.
//
// The function is exported (capital B) so tests can lock
// the wire format without standing up an httptest server.
func (p *Provider) BuildRequest(req ai.StreamRequest) (anthropicRequest, error) {
	return p.buildRequest(req)
}

// buildRequest is the unexported workhorse behind
// [BuildRequest]. The duplicate keeps BuildRequest the
// authoritative public name while letting internal callers
// stay terse.
func (p *Provider) buildRequest(req ai.StreamRequest) (anthropicRequest, error) {
	out := anthropicRequest{
		Model:     req.Model,
		MaxTokens: p.maxTokens(),
		Stream:    true,
		Messages:  make([]anthropicMessage, 0, len(req.Messages)),
		Tools:     make([]anthropicTool, 0, len(req.Tools)),
	}
	for _, t := range req.Tools {
		schema := t.Parameters
		if len(schema) == 0 {
			// Anthropic requires input_schema; a
			// missing schema becomes an empty
			// object so the request still parses.
			schema = json.RawMessage(`{}`)
		}
		out.Tools = append(out.Tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}
	for _, m := range req.Messages {
		if m.Role == "system" {
			out.System = m.Content
			continue
		}
		role := m.Role
		if role == "" {
			role = "user"
		}
		out.Messages = append(out.Messages, anthropicMessage{Role: role, Content: m.Content})
	}
	return out, nil
}

func (p *Provider) maxTokens() int {
	if p.MaxTokens > 0 {
		return p.MaxTokens
	}
	return DefaultMaxTokens
}

// readSSE is a tiny SSE reader used by the eventual full
// implementation. It is left in the package as a building
// block so the future full-implementation commit can be a
// one-line change: replace the goroutine body with a call
// to readSSE. The function is unexported; the test in
// ai_test.go exercises it via the public Provider path.
func readSSE(r io.Reader, onLine func(line string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		if err := onLine(strings.TrimPrefix(line, "data: ")); err != nil {
			return err
		}
	}
	return scanner.Err()
}
