package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TestRequest is the JSON payload accepted by /api/agent/test.
// The fields are the two endpoints to probe; either or both may
// be omitted (e.g. an OpenList-only deployment does not need an
// OpenAI key on the box running the test).
type TestRequest struct {
	OpenAIKey     string `json:"openai_key"`
	OpenAIBaseURL string `json:"openai_base_url"`
	OpenAIModel   string `json:"openai_model"`
	OpenListBaseURL string `json:"openlist_base_url"`
	OpenListToken string `json:"openlist_token"`
}

// TestResponse is the JSON shape returned by /api/agent/test.
//
// Both fields are independent: an OpenAI-only test returns
// openai_ok and an empty openlist_ok / openlist_error.
//
// Errors are surfaced per-endpoint so the Settings UI can
// highlight exactly which input is misconfigured.
type TestResponse struct {
	OpenAIOk       bool            `json:"openai_ok"`
	OpenAIError    string          `json:"openai_error,omitempty"`
	OpenAIStatus   int             `json:"openai_status,omitempty"`
	OpenListOk     bool            `json:"openlist_ok"`
	OpenListError  string          `json:"openlist_error,omitempty"`
	OpenListStatus int             `json:"openlist_status,omitempty"`
	Errors         map[string]string `json:"errors,omitempty"`
}

// HandleTest is the HTTP entry point for /api/agent/test. It
// pings OpenAI and OpenList concurrently with a 5-second
// timeout each, returning a TestResponse that the Settings UI
// uses to highlight which configuration is broken.
//
// The probe endpoints are chosen to be side-effect free:
//   - OpenAI: GET /v1/models  (lists available models)
//   - OpenList: GET /api/me   (returns the current user)
//
// We do NOT exercise the agent's full Chat flow here — that
// would consume tokens. A successful probe means the network
// path and credentials are sound.
func (a *Agent) HandleTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TestRequest
	if r.Method == http.MethodPost {
		// Body is optional: a GET-style call with no body is
		// also supported for debugging.
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	// Resolve credentials. The request body takes precedence
	// over the agent's configured values; this lets the
	// Settings UI probe a freshly-typed key without saving it
	// first.
	openaiKey := pickFirst(req.OpenAIKey, a.cfg.OpenAIAPIKey)
	openaiBase := pickFirst(req.OpenAIBaseURL, a.cfg.OpenAIBaseURL)
	if openaiBase == "" {
		openaiBase = "https://api.openai.com/v1"
	}
	openaiModel := req.OpenAIModel
	if openaiModel == "" {
		openaiModel = a.cfg.OpenAIModel
	}

	openlistBase := pickFirst(req.OpenListBaseURL, a.cfg.OpenListBaseURL)
	if openlistBase == "" {
		openlistBase = "http://127.0.0.1:5244"
	}
	openlistToken := pickFirst(req.OpenListToken, a.cfg.OpenListToken)

	// Run the two probes in parallel so the worst-case
	// response time is bounded by the slower probe, not the
	// sum of both.
	var wg sync.WaitGroup
	var openaiResult TestResponse
	var openlistResult TestResponse
	wg.Add(2)

	go func() {
		defer wg.Done()
		openaiResult = probeOpenAI(r.Context(), openaiKey, openaiBase)
	}()
	go func() {
		defer wg.Done()
		openlistResult = probeOpenList(r.Context(), openlistBase, openlistToken)
	}()
	wg.Wait()

	// Merge into a single response. Errors are only included
	// for the probes that actually ran; if a key was empty
	// we still run the probe and report the upstream's
	// verdict (the upstream is responsible for saying "401").
	resp := TestResponse{
		OpenAIOk:       openaiResult.OpenAIOk,
		OpenAIError:    openaiResult.OpenAIError,
		OpenAIStatus:   openaiResult.OpenAIStatus,
		OpenListOk:     openlistResult.OpenListOk,
		OpenListError:  openlistResult.OpenListError,
		OpenListStatus: openlistResult.OpenListStatus,
	}
	resp.Errors = map[string]string{}
	if !resp.OpenAIOk {
		if openaiKey == "" {
			resp.Errors["openai"] = "missing openai_api_key"
		} else {
			resp.Errors["openai"] = resp.OpenAIError
		}
	}
	if !resp.OpenListOk {
		if openlistToken == "" {
			resp.Errors["openlist"] = "missing openlist_token"
		} else {
			resp.Errors["openlist"] = resp.OpenListError
		}
	}
	if len(resp.Errors) == 0 {
		resp.Errors = nil
	}
	_ = openaiModel // reserved for future model-aware probes

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func pickFirst(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// probeOpenAI issues a GET {base}/v1/models request and reports
// success/failure. It is independent of the agent's own
// openai client so the test can run even when the rest of the
// agent is broken.
func probeOpenAI(parent context.Context, key, base string) TestResponse {
	out := TestResponse{}
	if key == "" {
		out.OpenAIError = "missing openai_api_key"
		return out
	}
	// Strip any trailing /v1 because we append /models below.
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		base = strings.TrimSuffix(base, "/v1")
	}
	url := base + "/v1/models"

	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		out.OpenAIError = err.Error()
		return out
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		out.OpenAIError = err.Error()
		return out
	}
	defer resp.Body.Close()
	out.OpenAIStatus = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.OpenAIError = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return out
	}
	out.OpenAIOk = true
	return out
}

// probeOpenList issues a GET {base}/api/me request and reports
// success/failure. It is the OpenList equivalent of probeOpenAI.
func probeOpenList(parent context.Context, base, token string) TestResponse {
	out := TestResponse{}
	if token == "" {
		out.OpenListError = "missing openlist_token"
		return out
	}
	base = strings.TrimRight(base, "/")
	url := base + "/api/me"

	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		out.OpenListError = err.Error()
		return out
	}
	req.Header.Set("Authorization", token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		out.OpenListError = err.Error()
		return out
	}
	defer resp.Body.Close()
	out.OpenListStatus = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.OpenListError = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return out
	}
	out.OpenListOk = true
	return out
}
