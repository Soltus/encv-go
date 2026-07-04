package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestRedact_HidesApiKey locks the wire-level contract of
// Redact: a string of the form "api_key=<value>" must come
// back as "api_key=***", with the keyword preserved and the
// value replaced. The front-end relies on this guarantee
// when it pastes an error message into the doctor report —
// if Redact ever misses a key/value pair, the user could
// share their token with a screenshot.
//
// We also test a handful of related shapes so future
// refactors of the regex can't regress common cases without
// a test failure.
func TestRedact_HidesApiKey(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		// The spec-locked shape.
		{
			name:  "spec example",
			input: "api_key=xxx",
			want:  "api_key=***",
		},
		// Most OpenAI libraries use Bearer tokens, so we
		// cover the bare `sk-...` shape too. The value is
		// replaced even if it looks like a JWT.
		{
			name:  "openai key with sk- prefix",
			input: "api_key=sk-proj-AbCdEf1234567890",
			want:  "api_key=***",
		},
		// Quoted values.
		{
			name:  "double-quoted password",
			input: `password="hunter2"`,
			want:  `password=***`,
		},
		// Colon separator.
		{
			name:  "colon separator with token",
			input: "token: abc.def.ghi",
			want:  "token: ***",
		},
		// Mixed case keyword.
		{
			name:  "case-insensitive keyword",
			input: "API_KEY=super-secret",
			want:  "API_KEY=***",
		},
		// oauth refresh token. The bare "refresh_token=..."
		// form (no JSON quoting) is handled; the "key":"value"
		// JSON form is a corner case we explicitly skip so
		// the regex doesn't grow a JSON parser.
		{
			name:  "refresh_token bare form",
			input: "refresh_token=rt_xyz",
			want:  "refresh_token=***",
		},
		// client_secret.
		{
			name:  "client_secret with equals and padding",
			input: "client_secret=cs_abc==",
			want:  "client_secret=***",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Redact(tc.input)
			if got != tc.want {
				t.Errorf("Redact(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestRedact_LeavesBenignTextAlone locks the false-positive
// guard. Most of the agent's error messages and log lines
// must NOT be redacted — only substrings that look like a
// real KEY=VALUE pair should be masked. This test exercises
// the common benign inputs the doctor endpoint actually
// receives.
func TestRedact_LeavesBenignTextAlone(t *testing.T) {
	cases := []string{
		// Plain English.
		"the weather is nice today",
		// A normal log line.
		"2026-06-06T12:34:56Z INFO agent run started session_id=sess_abc123",
		// A JSON document with no secret keywords.
		`{"status":"ok","count":42,"items":["a","b","c"]}`,
		// A path or URL.
		"http://127.0.0.1:8080/api/health",
		// "key" as a substring of a non-keyword word.
		// "monkey" must NOT be redacted because the keyword
		// list is anchored on word boundaries.
		"the monkey ate a banana",
		// Empty input.
		"",
	}
	for _, input := range cases {
		t.Run(input, func(t *testing.T) {
			got := Redact(input)
			if got != input {
				t.Errorf("Redact(%q) = %q, want unchanged", input, got)
			}
		})
	}
}

// TestRedact_EmptyInput is split out so a future refactor
// that strips the early-return guard fails loudly. The
// behaviour is part of the public contract.
func TestRedact_EmptyInput(t *testing.T) {
	if got := Redact(""); got != "" {
		t.Errorf("Redact(\"\") = %q, want \"\"", got)
	}
}

// TestRunDoctor_BasicShape drives RunDoctor on a minimal
// agent (no LLM, no OpenList, no skills) and asserts the
// returned DoctorReport has the expected shape and that
// every section is populated with the right kind of
// values:
//
//   - GeneratedAtMs is recent (within 5s of now).
//   - Version matches the package-level doctorVersion.
//   - Agent block has Go runtime facts and the right
//     OpenAI key presence flag.
//   - Sessions, Tools, OpenList, Skills each carry the
//     expected type and a non-nil but possibly empty slice.
//   - Issues is non-nil (an empty slice, not nil) so the
//     front-end can render "no problems" without a null
//     check.
func TestRunDoctor_BasicShape(t *testing.T) {
	cfg := AgentConfig{
		OpenAIModel: "gpt-4o",
		// OpenAIAPIKey is intentionally left empty so we
		// can assert that the flag flips to false.
	}
	a := NewAgent(cfg, NewRegistry())
	// Pre-populate a couple of sessions and tools so the
	// counters are non-zero (and we can sanity-check
	// that the walker visits them).
	a.ensureSession("sess_a")
	a.ensureSession("sess_b")
	a.ensureSession("sess_c")
	a.Registry.Register("echo", nil, func(args string) (string, error) {
		return `{}`, nil
	}, false, KindCommand)
	a.Registry.Register("list_files", nil, func(args string) (string, error) {
		return `{}`, nil
	}, true, KindReadOnly)

	report := a.RunDoctor(context.Background())

	// GeneratedAtMs: within the last 5 seconds.
	nowMs := time.Now().UnixMilli()
	if report.GeneratedAtMs < nowMs-5000 || report.GeneratedAtMs > nowMs+5000 {
		t.Errorf("GeneratedAtMs out of range: got %d, now %d", report.GeneratedAtMs, nowMs)
	}
	if report.Version == "" {
		t.Errorf("Version should be non-empty (the package-level doctorVersion)")
	}
	if report.Version != doctorVersion {
		t.Errorf("Version: got %q, want %q", report.Version, doctorVersion)
	}

	// Agent block.
	if report.Agent.Version != doctorVersion {
		t.Errorf("Agent.Version: got %q, want %q", report.Agent.Version, doctorVersion)
	}
	if report.Agent.GoVersion == "" {
		t.Errorf("Agent.GoVersion should be non-empty")
	}
	if report.Agent.GOMAXPROCS <= 0 {
		t.Errorf("Agent.GOMAXPROCS should be > 0, got %d", report.Agent.GOMAXPROCS)
	}
	if report.Agent.NumGoroutine <= 0 {
		t.Errorf("Agent.NumGoroutine should be > 0, got %d", report.Agent.NumGoroutine)
	}
	if report.Agent.OpenAIAPIKeyConfigured {
		t.Errorf("Agent.OpenAIAPIKeyConfigured: got true, want false (no key set)")
	}

	// Sessions: 3 cached, 0 persisted (no store configured).
	if report.Sessions.TotalCached != 3 {
		t.Errorf("Sessions.TotalCached: got %d, want 3", report.Sessions.TotalCached)
	}
	if report.Sessions.TotalPersisted != 0 {
		t.Errorf("Sessions.TotalPersisted: got %d, want 0 (no store configured)", report.Sessions.TotalPersisted)
	}
	if report.Sessions.LargestSessionSizeBytes != 0 {
		t.Errorf("Sessions.LargestSessionSizeBytes: got %d, want 0 (no events emitted)", report.Sessions.LargestSessionSizeBytes)
	}

	// Tools: 2 registered, sorted alphabetically.
	if report.Tools.RegisteredCount != 2 {
		t.Errorf("Tools.RegisteredCount: got %d, want 2", report.Tools.RegisteredCount)
	}
	wantTools := []string{"echo", "list_files"}
	if !reflect.DeepEqual(report.Tools.Names, wantTools) {
		t.Errorf("Tools.Names: got %v, want %v", report.Tools.Names, wantTools)
	}

	// OpenList: nothing configured, no error to surface.
	if report.OpenList.BaseURLConfigured {
		t.Errorf("OpenList.BaseURLConfigured should be false (no URL set)")
	}
	if report.OpenList.TokenConfigured {
		t.Errorf("OpenList.TokenConfigured should be false (no token set)")
	}
	if report.OpenList.LastError != "" {
		t.Errorf("OpenList.LastError should be empty when not configured, got %q", report.OpenList.LastError)
	}

	// Skills: no skills loaded.
	if report.Skills.LoadedCount != 0 {
		t.Errorf("Skills.LoadedCount: got %d, want 0", report.Skills.LoadedCount)
	}
	if report.Skills.Names == nil {
		t.Errorf("Skills.Names should be a non-nil empty slice")
	}

	// Issues: non-nil, and at least one issue about the
	// missing OpenAI key.
	if report.Issues == nil {
		t.Errorf("Issues should be a non-nil slice")
	}
	var foundKeyIssue bool
	for _, s := range report.Issues {
		if strings.Contains(s, "openai_api_key") {
			foundKeyIssue = true
		}
	}
	if !foundKeyIssue {
		t.Errorf("expected an issue mentioning openai_api_key, got: %v", report.Issues)
	}
}

// TestRunDoctor_DetectsConfiguredOpenAIKey flips the boolean
// the other way: a configured key must be reported as
// configured (without leaking the key value into any field).
func TestRunDoctor_DetectsConfiguredOpenAIKey(t *testing.T) {
	a := NewAgent(AgentConfig{
		OpenAIAPIKey: "sk-proj-test-do-not-leak",
		OpenAIModel:  "gpt-4o",
	}, NewRegistry())
	report := a.RunDoctor(context.Background())
	if !report.Agent.OpenAIAPIKeyConfigured {
		t.Errorf("OpenAIAPIKeyConfigured should be true when a key is set")
	}
	// SECURITY: scan the entire report for the secret value.
	// RunDoctor must not echo the key in any field.
	if leaks(report, "sk-proj-test-do-not-leak") {
		t.Errorf("RunDoctor leaked the OpenAI key in some field")
	}
}

// TestRunDoctor_CountsPersistedSessions sets up a SessionStore
// in a temp dir, drops two *.jsonl files in it, and asserts
// the count survives a doctor run. This is the only test
// that touches the durable-session side of the report.
func TestRunDoctor_CountsPersistedSessions(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	// Two session files + one unrelated file to make sure
	// the filter is precise.
	if err := os.WriteFile(filepath.Join(dir, "sess_1.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sess_2.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := NewAgent(AgentConfig{OpenAIModel: "gpt-4o"}, NewRegistry(), store)
	report := a.RunDoctor(context.Background())
	if report.Sessions.TotalPersisted != 2 {
		t.Errorf("TotalPersisted: got %d, want 2", report.Sessions.TotalPersisted)
	}
}

// TestRunDoctor_OpenListPingOnUnreachableHost covers the
// "configured but unreachable" case: a doctor run on an
// OpenList server that does not exist must populate
// LastError and LastPingMs without panicking. We use a
// closed httptest server URL so the connection refused
// error happens fast and reliably.
func TestRunDoctor_OpenListPingOnUnreachableHost(t *testing.T) {
	// Spin up a server and immediately close it so the
	// URL is definitely dead.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := srv.URL
	srv.Close()

	a := NewAgent(AgentConfig{
		OpenAIModel:     "gpt-4o",
		OpenListBaseURL: deadURL,
		OpenListToken:   "tk_test",
	}, NewRegistry())
	report := a.RunDoctor(context.Background())

	if !report.OpenList.BaseURLConfigured {
		t.Errorf("BaseURLConfigured should be true")
	}
	if !report.OpenList.TokenConfigured {
		t.Errorf("TokenConfigured should be true")
	}
	if report.OpenList.LastError == "" {
		t.Errorf("LastError should be non-empty for a dead server")
	}
	// LastError must be redacted: the token must NOT appear
	// in the field. Defence in depth, even though the
	// current OpenList client only echoes the path.
	if strings.Contains(report.OpenList.LastError, "tk_test") {
		t.Errorf("LastError leaked the token: %q", report.OpenList.LastError)
	}
}

// TestRunDoctor_JSONShape asserts the JSON wire format is
// stable: every public field must be present (even if zero)
// so the front-end never has to defend against missing keys.
// This is the de-facto "schema" test for the API.
func TestRunDoctor_JSONShape(t *testing.T) {
	a := NewAgent(AgentConfig{OpenAIModel: "gpt-4o"}, NewRegistry())
	report := a.RunDoctor(context.Background())
	b, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Round-trip into a generic map so we can assert
	// top-level keys without depending on the Go struct.
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wantTop := []string{
		"generated_at_ms", "version",
		"agent", "sessions", "tools", "openlist", "skills", "issues",
	}
	for _, k := range wantTop {
		if _, ok := got[k]; !ok {
			t.Errorf("DoctorReport JSON missing top-level key %q (got %s)", k, string(b))
		}
	}
	wantAgent := []string{
		"version", "server_instance_id", "go_version",
		"gomaxprocs", "num_goroutine", "openai_api_key_configured",
	}
	agentMap, ok := got["agent"].(map[string]any)
	if !ok {
		t.Fatalf("agent block is not an object: %s", string(b))
	}
	for _, k := range wantAgent {
		if _, ok := agentMap[k]; !ok {
			t.Errorf("DoctorReport.Agent JSON missing key %q", k)
		}
	}
}

// TestHandleSyncDoctor_HTTPContract is the end-to-end HTTP
// test: it calls the handler through httptest, asserts the
// status code, content-type, and that the body unmarshals
// into a DoctorReport.
func TestHandleSyncDoctor_HTTPContract(t *testing.T) {
	a := NewAgent(AgentConfig{OpenAIModel: "gpt-4o"}, NewRegistry())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/doctor", a.HandleSyncDoctor)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/sync/doctor", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: got %q, want application/json", ct)
	}
	var report DoctorReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report.Version == "" {
		t.Errorf("decoded report missing version")
	}
}

// TestHandleSyncDoctor_RejectsBadMethod locks the negative
// path of the HTTP handler: PUT, DELETE etc. must return
// 405 with an Allow header. This is the contract the
// front-end depends on when it surfaces a method error.
func TestHandleSyncDoctor_RejectsBadMethod(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	mux := http.NewServeMux()
	mux.HandleFunc("/api/sync/doctor", a.HandleSyncDoctor)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/api/sync/doctor", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("PUT status: got %d, want 405", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); allow != "GET, POST" {
		t.Errorf("Allow header: got %q, want GET, POST", allow)
	}
}

// leaks reports whether s contains the secret needle. We
// stringify the report so a future struct field cannot
// accidentally bypass the test by, e.g., being a custom
// type with a String() that echoes the secret.
func leaks(r DoctorReport, needle string) bool {
	b, err := json.Marshal(r)
	if err != nil {
		return true
	}
	return strings.Contains(string(b), needle)
}
