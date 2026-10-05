package stubprovider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func post(t *testing.T, p *Provider, model string, stream bool, roles ...string) *httptest.ResponseRecorder {
	t.Helper()
	reqs := make([]map[string]any, 0, len(roles))
	for _, r := range roles {
		reqs = append(reqs, map[string]any{"role": r, "content": "x"})
	}
	body, _ := json.Marshal(map[string]any{
		"model":    model,
		"stream":   stream,
		"messages": reqs,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	p.Handler().ServeHTTP(rec, req)
	return rec
}

func TestProvider_Stream_EmitsToolCall(t *testing.T) {
	p := New()
	rec := post(t, p, "stub:fs_overview", true, "user")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "list_mounts") {
		t.Fatalf("turn0 must emit list_mounts tool_call, body=%s", body)
	}
	if !strings.Contains(body, `"finish_reason":"tool_calls"`) {
		t.Fatalf("finish_reason must be tool_calls, body=%s", body)
	}
	if !strings.HasSuffix(strings.TrimSpace(body), "data: [DONE]") {
		t.Fatalf("stream must end with [DONE], body=%s", body)
	}
	if p.RequestCount() != 1 {
		t.Fatalf("RequestCount=%d want 1", p.RequestCount())
	}
}

func TestProvider_AdvancesTurnAfterToolResult(t *testing.T) {
	p := New()
	// user → assistant(tool_calls) → tool(结果) ⇒ 第二个 assistant 回合 = turn1
	rec := post(t, p, "stub:fs_overview", true, "user", "assistant", "tool")
	body := rec.Body.String()
	if !strings.Contains(body, "list_files") {
		t.Fatalf("turn1 must emit list_files, body=%s", body)
	}
	if strings.Contains(body, "list_mounts") {
		t.Fatalf("turn1 must not repeat turn0 tool_call, body=%s", body)
	}
}

func TestProvider_FinalTurnIsPlainText(t *testing.T) {
	p := New()
	// 两轮工具都已完成 ⇒ turn2 = 纯文本总结
	rec := post(t, p, "stub:fs_overview", true, "user", "assistant", "tool", "assistant", "tool")
	body := rec.Body.String()
	if strings.Contains(body, "tool_calls") {
		t.Fatalf("final turn must not emit tool_calls, body=%s", body)
	}
	if !strings.Contains(body, `"finish_reason":"stop"`) {
		t.Fatalf("final turn finish_reason must be stop, body=%s", body)
	}
	if !strings.Contains(body, "真实工具") {
		t.Fatalf("final turn must carry the closing text, body=%s", body)
	}
}

func TestProvider_UnknownScriptFailsClosed(t *testing.T) {
	p := New()
	rec := post(t, p, "stub:does_not_exist", true, "user")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown script must be 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unknown script") {
		t.Fatalf("error must explain, body=%s", rec.Body.String())
	}
}

func TestProvider_NonStream(t *testing.T) {
	p := New()
	rec := post(t, p, "stub:storage", false, "user")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	var got struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, rec.Body.String())
	}
	if len(got.Choices) != 1 {
		t.Fatalf("want 1 choice, got %d", len(got.Choices))
	}
	if len(got.Choices[0].Message.ToolCalls) != 1 ||
		got.Choices[0].Message.ToolCalls[0].Function.Name != "get_storage_info" {
		t.Fatalf("tool_calls mismatch: %+v", got.Choices[0].Message.ToolCalls)
	}
	if got.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("finish_reason=%q", got.Choices[0].FinishReason)
	}
}

func TestProvider_DefaultScriptWhenModelNotPrefixed(t *testing.T) {
	p := New()
	rec := post(t, p, "gpt-4o", true, "user")
	if !strings.Contains(rec.Body.String(), "list_mounts") {
		t.Fatalf("default script should be used, body=%s", rec.Body.String())
	}
}

func TestProvider_RegisterScriptRejectsEmpty(t *testing.T) {
	p := New()
	if err := p.RegisterScript(&Script{ID: "x"}); err == nil {
		t.Fatal("script with no turns must be rejected")
	}
	if err := p.RegisterScript(nil); err == nil {
		t.Fatal("nil script must be rejected")
	}
}

func TestProvider_CustomScript(t *testing.T) {
	p := New()
	if err := p.RegisterScript(&Script{ID: "custom", Turns: []Turn{
		{Text: "只看挂载点", ToolCalls: []ToolCall{{Name: "list_mounts"}}},
		{Text: "结束"},
	}}); err != nil {
		t.Fatal(err)
	}
	rec := post(t, p, "stub:custom", true, "user")
	if !strings.Contains(rec.Body.String(), "list_mounts") {
		t.Fatalf("custom script not used: %s", rec.Body.String())
	}
}
