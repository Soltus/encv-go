// Package gemini implements a skeleton [ai.Provider] for
// the Google Gemini streamGenerateContent endpoint.
//
// The implementation is intentionally a skeleton: it
// validates the request, builds the wire-format payload
// (system instruction hoisted to a top-level
// systemInstruction, tools encoded as functionDeclarations),
// and returns a valid [ai.Delta] channel that closes
// cleanly. It does NOT yet open the streaming HTTP
// request, parse the Server-Sent-Event / JSON-array
// frames, or translate candidate deltas into tool-call /
// reasoning fragments — those are deliberately out of
// scope for the initial provider cut (see the "Provider
// abstraction layer" section of
// .trae/specs/mobile-agent-2026-gap-analysis/spec.md for
// the rollout plan).
//
// The skeleton does NOT pull in
// github.com/google/generative-ai-go — the wire format is
// built with stdlib [encoding/json] and the eventual HTTP
// path will use [net/http] directly. The goal is to prove
// the abstraction compiles end-to-end (see the
// corresponding ai_test.go for the compile-time + runtime
// contract) without locking the project into a vendor SDK
// whose API surface we do not control.
package gemini

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
	// DefaultBaseURL is the production Gemini API
	// endpoint. Tests can override it via the BaseURL
	// field.
	DefaultBaseURL = "https://generativelanguage.googleapis.com"

	// DefaultAPIVersion is the Gemini API path segment
	// ("v1beta" today; bump this if Google ships a v2).
	// Bumping is part of the provider's contract: a
	// versioned path is the only knob keeping our
	// callers insulated from upstream wire-format
	// changes.
	DefaultAPIVersion = "v1beta"
)

// Provider is the Gemini-flavoured [ai.Provider]. It streams
// chat completions from the streamGenerateContent endpoint
// into [ai.Delta] values. The zero value is unusable; use
// [New] to construct one.
type Provider struct {
	// APIKey is the Google AI Studio / Vertex API key.
	// Required for the eventual full implementation;
	// the skeleton tolerates an empty key and returns
	// a closed channel.
	APIKey string

	// BaseURL is the API root. Defaults to
	// [DefaultBaseURL]. Override for testing or
	// self-hosted deployments.
	BaseURL string

	// APIVersion is the URL path segment
	// (e.g. "v1beta"). Defaults to
	// [DefaultAPIVersion].
	APIVersion string

	// Model is the model name to send in the request
	// (e.g. "gemini-1.5-pro"). Required.
	Model string

	// MaxTokens is the maxOutputTokens request field.
	// Zero leaves the field unset so Gemini's own
	// default kicks in.
	MaxTokens int

	// HTTPClient overrides the default HTTP client.
	// nil means http.DefaultClient. Used by tests to
	// wire an httptest server.
	HTTPClient *http.Client
}

// New constructs a Provider with the supplied API key and
// model. BaseURL / APIVersion fall back to the Default*
// constants; callers can override them by setting the field
// directly after construction.
func New(apiKey, model string) *Provider {
	return &Provider{
		APIKey:     apiKey,
		Model:      model,
		BaseURL:    DefaultBaseURL,
		APIVersion: DefaultAPIVersion,
	}
}

// Name returns "gemini" — lowercase, no whitespace.
func (p *Provider) Name() string { return "gemini" }

// geminiRequest is the wire shape of POST
// /v1beta/models/{model}:streamGenerateContent. We define
// it here (rather than reusing [ai.StreamRequest]) because
// Gemini's API has a different field naming convention
// (systemInstruction as a top-level object, contents
// rather than messages, functionDeclarations rather than
// tools, etc.).
type geminiRequest struct {
	SystemInstruction *geminiContent  `json:"systemInstruction,omitempty"`
	Contents          []geminiContent `json:"contents"`
	Tools             []geminiTools   `json:"tools,omitempty"`
	GenerationConfig  *geminiGenCfg   `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text,omitempty"`
}

type geminiTools struct {
	FunctionDeclarations []geminiFunctionDecl `json:"functionDeclarations"`
}

type geminiFunctionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type geminiGenCfg struct {
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
	Temperature     float64 `json:"temperature,omitempty"`
}

// StreamChat opens a streaming Gemini request and returns
// a channel of [ai.Delta] values.
//
// Skeleton contract: the function must (1) compile, (2)
// accept a well-formed [ai.StreamRequest] without error,
// and (3) return a non-nil channel that closes
// deterministically. The eventual full implementation will
// (a) POST the body built by [BuildRequest] to
// BaseURL+"/"+APIVersion+"/models/"+Model+":streamGenerateContent",
// (b) parse the JSON-array streaming response, and (c)
// translate each candidate delta into an [ai.Delta] before
// sending on the channel.
//
// Tests that want to exercise the wire format can call
// [Provider.BuildRequest] directly; the production path
// remains a no-op until the HTTP plumbing lands.
func (p *Provider) StreamChat(ctx context.Context, req ai.StreamRequest) (<-chan ai.Delta, error) {
	if p == nil {
		return nil, fmt.Errorf("ai/gemini: nil provider")
	}
	if req.Model == "" {
		return nil, fmt.Errorf("ai/gemini: model is required")
	}
	if _, err := p.buildRequest(req); err != nil {
		return nil, err
	}
	out := make(chan ai.Delta, 4)
	go func() {
		defer close(out)
		// Skeleton: no HTTP call yet.
		_ = ctx
	}()
	return out, nil
}

// BuildRequest translates an [ai.StreamRequest] into the
// Gemini-specific wire shape. System messages are hoisted
// to the top-level systemInstruction field; user /
// assistant messages are mapped to the corresponding
// Gemini "contents" entries. Tools use
// functionDeclarations rather than parameters.
//
// The function is exported so tests can lock the wire
// format without standing up an httptest server.
func (p *Provider) BuildRequest(req ai.StreamRequest) (geminiRequest, error) {
	return p.buildRequest(req)
}

func (p *Provider) buildRequest(req ai.StreamRequest) (geminiRequest, error) {
	out := geminiRequest{
		Contents: make([]geminiContent, 0, len(req.Messages)),
	}
	for _, t := range req.Tools {
		params := t.Parameters
		if len(params) == 0 {
			params = json.RawMessage(`{}`)
		}
		out.Tools = append(out.Tools, geminiTools{
			FunctionDeclarations: []geminiFunctionDecl{{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			}},
		})
	}
	for _, m := range req.Messages {
		if m.Role == "system" {
			out.SystemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: m.Content}},
			}
			continue
		}
		role := m.Role
		if role == "" {
			role = "user"
		}
		// Gemini uses "model" for assistant turns.
		if role == "assistant" {
			role = "model"
		}
		out.Contents = append(out.Contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: m.Content}},
		})
	}
	if req.MaxTokens > 0 || req.Temperature != 0 {
		out.GenerationConfig = &geminiGenCfg{
			MaxOutputTokens: req.MaxTokens,
			Temperature:     req.Temperature,
		}
	}
	return out, nil
}

// readSSE is a tiny SSE / JSON-array reader used by the
// eventual full implementation. It is left in the package
// as a building block so the future full-implementation
// commit can be a one-line change. The function is
// unexported; the test in ai_test.go exercises the public
// Provider path.
func readSSE(r io.Reader, onLine func(line string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			line = strings.TrimPrefix(line, "data: ")
		}
		if err := onLine(line); err != nil {
			return err
		}
	}
	return scanner.Err()
}
