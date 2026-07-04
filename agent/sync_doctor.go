package agent

import (
	"context"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// doctorVersion is the schema version of DoctorReport. Bumped
// only on breaking-shape changes; the front-end uses it to
// pick a parser. Overridable at build time via
//
//	-ldflags="-X 'agent.doctorVersion=vX.Y.Z'"
//
// so release builds can stamp a concrete version without
// requiring a code change.
var doctorVersion = "v0.1.0"

// DoctorReport is the JSON-serialised diagnostic payload
// returned by RunDoctor (and exposed via /api/sync/doctor).
//
// The shape is the wire contract between the agent and the
// encv-mobile front-end's "运行 sync 诊断" button. New
// fields may be added without bumping doctorVersion, but
// renames or removals require a major bump. The front-end
// renders this verbatim inside a <pre> block, so human
// readability is a design constraint — every nested struct
// has a json tag that doubles as a column label.
//
// SECURITY: this struct MUST NOT include plaintext
// secrets. API keys, tokens and passwords are either omitted
// (presence reported as a boolean) or, when surfaced as
// strings, are first passed through Redact. The `Issues`
// field is a list of short human-readable strings — never
// include raw error bodies that might contain a token.
type DoctorReport struct {
	GeneratedAtMs int64              `json:"generated_at_ms"`
	Version       string             `json:"version"`
	Agent         DoctorAgentInfo    `json:"agent"`
	Sessions      DoctorSessionInfo  `json:"sessions"`
	Tools         DoctorToolsInfo    `json:"tools"`
	OpenList      DoctorOpenListInfo `json:"openlist"`
	Skills        DoctorSkillsInfo   `json:"skills"`
	Issues        []string           `json:"issues"`
}

// DoctorAgentInfo captures runtime facts about the agent
// process. ServerInstanceID is the same value /api/health
// returns, so the front-end can correlate a doctor report
// with a specific server restart.
type DoctorAgentInfo struct {
	Version                string `json:"version"`
	ServerInstanceID       string `json:"server_instance_id"`
	GoVersion              string `json:"go_version"`
	GOMAXPROCS             int    `json:"gomaxprocs"`
	NumGoroutine           int    `json:"num_goroutine"`
	OpenAIAPIKeyConfigured bool   `json:"openai_api_key_configured"`
}

// DoctorSessionInfo summarises the in-memory SessionCache
// map and the durable SessionStore (when configured).
// LargestSessionSizeBytes is approximated as the sum of the
// Data string lengths across all events in the largest
// session — a cheap proxy for memory footprint without
// having to walk the event payloads.
type DoctorSessionInfo struct {
	TotalCached             int   `json:"total_cached"`
	TotalPersisted          int   `json:"total_persisted"`
	LargestSessionSizeBytes int64 `json:"largest_session_size_bytes"`
}

// DoctorToolsInfo lists the registered tools and the count.
// Names is sorted alphabetically so the report diff is
// stable across runs.
type DoctorToolsInfo struct {
	RegisteredCount int      `json:"registered_count"`
	Names           []string `json:"names"`
}

// DoctorOpenListInfo reports the OpenList backend state.
// BaseURLConfigured and TokenConfigured are the two boolean
// flags the front-end can show as "configured / missing";
// LastPingMs + LastError give a one-shot connectivity
// measurement taken at RunDoctor time. The ping is
// best-effort and never blocks longer than 2 seconds.
type DoctorOpenListInfo struct {
	BaseURLConfigured bool   `json:"base_url_configured"`
	TokenConfigured   bool   `json:"token_configured"`
	LastPingMs        int64  `json:"last_ping_ms"`
	LastError         string `json:"last_error,omitempty"`
}

// DoctorSkillsInfo summarises the loaded skill manifests.
// Skills are loaded once at NewAgent time from
// cfg.SkillsDir; the slice is read-only at runtime.
type DoctorSkillsInfo struct {
	LoadedCount int      `json:"loaded_count"`
	Names       []string `json:"names"`
}

// redactKeywordRe matches any of the recognised secret
// keywords in a case-insensitive way, followed by an `=` or
// `:` separator. The trailing value character class is
// `[^"'\s,;}]+` (unquoted) OR a quoted string, so we cover
// both `api_key=xxx` and `password="hunter2"` shapes. The
// keyword list is intentionally narrow to keep false
// positives low: api_key, password, secret, token, and the
// OAuth-specific access_token / refresh_token /
// client_secret variants.
var redactKeywordRe = regexp.MustCompile(
	`(?i)\b(` +
		`api[_-]?key|` +
		`password|` +
		`secret|` +
		`token|` +
		`access[_-]?token|` +
		`refresh[_-]?token|` +
		`client[_-]?secret` +
		`)\b(\s*[=:]\s*)(?:"[^"]*"|'[^']*'|[^\s,;}]+)`,
)

// Redact returns a copy of s with the value following any
// recognised secret keyword replaced by "***". The keyword
// match is case-insensitive and accepts "key=value",
// "key:value", or "key = value" as separators. Both quoted
// and unquoted values are handled:
//
//	"api_key=xxx"             -> "api_key=***"
//	`password="hunter2"`      -> `password=***`
//	`token: abc.def-ghi`      -> `token: ***`
//	"the weather is nice"     -> "the weather is nice" (unchanged)
//
// Redact is intentionally conservative: any substring that
// looks like KEY<sep>VALUE will be masked. The point is to
// guarantee no plaintext secret survives the diagnostic
// pass, even at the cost of an occasional false positive on
// a benign phrase (e.g. "key: insight" gets masked).
//
// An empty input is returned unchanged.
func Redact(s string) string {
	if s == "" {
		return s
	}
	// Group 1 = keyword, group 2 = separator (including
	// surrounding whitespace). We keep both in the
	// replacement so the output reads as "keyword=***"
	// with the original spacing preserved.
	return redactKeywordRe.ReplaceAllString(s, "$1$2***")
}

// RunDoctor collects a snapshot of agent state for
// diagnostic reporting. The method is read-only — it does
// NOT modify any persistent state. The OpenList ping is
// best-effort and never blocks longer than 2 seconds.
//
// The returned DoctorReport is safe to JSON-encode and
// ship to the front-end. See DoctorReport for the security
// contract.
func (a *Agent) RunDoctor(ctx context.Context) DoctorReport {
	report := DoctorReport{
		GeneratedAtMs: time.Now().UnixMilli(),
		Version:       doctorVersion,
		Agent: DoctorAgentInfo{
			Version:                doctorVersion,
			ServerInstanceID:       a.serverInstanceId,
			GoVersion:              runtime.Version(),
			GOMAXPROCS:             runtime.GOMAXPROCS(0),
			NumGoroutine:           runtime.NumGoroutine(),
			OpenAIAPIKeyConfigured: a.cfg.OpenAIAPIKey != "",
		},
	}

	// ── Sessions ─────────────────────────────────────────────
	// TotalCached walks the sync.Map; LargestSessionSizeBytes
	// is approximated as the sum of Data string lengths in
	// the largest session. We take the cache mutex per
	// session — the walk is O(n) but n is small (one
	// SessionCache per active chat session).
	var largest int64
	a.Sessions.Range(func(_, value any) bool {
		cache, ok := value.(*SessionCache)
		if !ok {
			return true
		}
		report.Sessions.TotalCached++
		cache.mu.Lock()
		var size int64
		for _, e := range cache.Events {
			if e == nil {
				continue
			}
			size += int64(len(e.Data))
		}
		cache.mu.Unlock()
		if size > largest {
			largest = size
		}
		return true
	})
	report.Sessions.LargestSessionSizeBytes = largest

	// TotalPersisted counts *.jsonl files in the store root.
	// A missing root (no session has been appended yet) is
	// reported as 0, not as an error — the user may simply
	// have never used the agent.
	if a.store != nil {
		if entries, err := os.ReadDir(a.store.Root()); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				if strings.HasSuffix(e.Name(), ".jsonl") {
					report.Sessions.TotalPersisted++
				}
			}
		}
	}

	// ── Tools ────────────────────────────────────────────────
	toolNames := a.Registry.Names()
	sort.Strings(toolNames)
	report.Tools = DoctorToolsInfo{
		RegisteredCount: len(toolNames),
		Names:           toolNames,
	}

	// ── OpenList ─────────────────────────────────────────────
	openList := DoctorOpenListInfo{
		BaseURLConfigured: a.cfg.OpenListBaseURL != "",
		TokenConfigured:   a.cfg.OpenListToken != "",
	}
	if a.cfg.OpenListBaseURL != "" && a.cfg.OpenListToken != "" {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		t0 := time.Now()
		err := NewOpenListClient(a.cfg.OpenListBaseURL, a.cfg.OpenListToken, nil).Ping(pingCtx)
		// LastPingMs is measured even on error — the round-trip
		// is itself diagnostic information (a fast error vs a
		// slow timeout are very different symptoms).
		openList.LastPingMs = time.Since(t0).Milliseconds()
		cancel()
		if err != nil {
			// SECURITY: err.Error() may contain the token in
			// the request URL. The current OpenList client
			// only echoes the path, but to be safe we run it
			// through Redact anyway. Defence in depth.
			openList.LastError = Redact(err.Error())
		}
	}
	report.OpenList = openList

	// ── Skills ───────────────────────────────────────────────
	skillNames := make([]string, 0, len(a.Skills))
	for _, s := range a.Skills {
		skillNames = append(skillNames, s.Name)
	}
	sort.Strings(skillNames)
	report.Skills = DoctorSkillsInfo{
		LoadedCount: len(skillNames),
		Names:       skillNames,
	}

	// ── Issues ───────────────────────────────────────────────
	report.Issues = a.collectDoctorIssues()

	return report
}

// collectDoctorIssues runs a few static sanity checks and
// returns a slice of short human-readable issue strings.
// An empty slice means "no problem detected". The checks
// are intentionally cheap (no I/O) and side-effect free.
func (a *Agent) collectDoctorIssues() []string {
	var issues []string
	if a.cfg.OpenAIAPIKey == "" {
		issues = append(issues, "openai_api_key is not configured; chat completions will fail")
	}
	if a.cfg.OpenListBaseURL == "" {
		issues = append(issues, "openlist_base_url is not configured; OpenList tools will not work")
	}
	if a.cfg.OpenListBaseURL != "" && a.cfg.OpenListToken == "" {
		issues = append(issues, "openlist_token is empty; OpenList requests will be rejected (401)")
	}
	if a.cfg.MaxToolCallsPerTurn <= 0 {
		issues = append(issues, "max_tool_calls_per_turn is 0 (unlimited); runaway tool loops are possible")
	}
	if a.cfg.OpenAIModel == "" {
		issues = append(issues, "openai_model is empty; OpenAI requests will fail with 400")
	}
	return issues
}
