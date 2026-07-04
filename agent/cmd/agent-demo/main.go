// Command agent-demo runs the Go in-process agent as a
// stand-alone HTTP service on :5245. It is the executable form
// of the in-process agent described in
// `.trae/specs/go-in-process-agent/spec.md`.
//
// Endpoints:
//
//	POST /api/chat                   → SSE stream of agent events
//	POST /api/resume                 → resume a session from offset N
//	POST /api/confirm                → apply a confirmation decision
//	POST /api/agent/test             → ping OpenAI + OpenList
//	GET  /api/health                 → liveness probe
//	GET  /api/agent/tools            → list registered tools (debug)
//	GET  /api/network/lan-access     → enumerate LAN-routable IPv4 URLs
//
// Configuration is loaded from `agent_settings` inside the
// user config (see config_loader.go for the lookup order).
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	agent "github.com/encv/agent"
)

// version is the agent-demo build version. Override at build
// time with `-ldflags="-X main.version=vX.Y.Z"`.
var version = "dev"

func main() {
	addr := os.Getenv("AGENT_ADDR")
	if addr == "" {
		addr = ":5245"
	}

	// 1. Load config.
	settings, err := agent.LoadAgentSettings()
	if err != nil {
		log.Fatalf("load agent_settings: %v", err)
	}
	if err := settings.Validate(); err != nil {
		// Non-fatal: log and continue with zero values.
		log.Printf("agent_settings validation warning: %v", err)
	}
	cfg := settings.ToAgentConfig()
	// Resolve tokens / URLs through the AgentSettings helper
	// so the env-var fallback (OPENAI_API_KEY, OPENAI_BASE_URL,
	// OPENLIST_TOKEN, etc.) takes effect.
	cfg.OpenAIAPIKey = settings.ResolveOpenAIToken()
	cfg.OpenAIBaseURL = settings.ResolveOpenAIBaseURL()
	cfg.OpenAIModel = settings.ResolveOpenAIModel()
	cfg.OpenListBaseURL = settings.ResolveOpenListBaseURL()
	cfg.OpenListToken = settings.ResolveOpenListToken()

	// 2. Build the agent.
	registry := agent.NewRegistry()
	registerOpenListTools(registry, cfg)
	registerPluginTools(registry, cfg)
	applyEnabledTools(registry, cfg.EnabledTools)

	a := agent.NewAgent(cfg, registry)
	log.Printf("agent-demo v%s listening on %s", version, addr)
	log.Printf("  openai_base_url=%s model=%s key=%s",
		cfg.OpenAIBaseURL, cfg.OpenAIModel, redact(cfg.OpenAIAPIKey))
	log.Printf("  openlist_base_url=%s token=%s",
		cfg.OpenListBaseURL, redact(cfg.OpenListToken))
	log.Printf("  registered %d tools", countRegistered(registry))

	// 3. Routes.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", a.HandleChat)
	mux.HandleFunc("/api/resume", a.HandleResume)
	mux.HandleFunc("/api/confirm", a.HandleConfirm)
	mux.HandleFunc("/api/agent/test", a.HandleTest)
	mux.HandleFunc("/api/health", a.HandleHealth)
	mux.HandleFunc("/api/agent/tools", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"count": countRegistered(registry),
		})
	})
	// Task 26 (LAN Access): enumerate IPv4 interfaces that are
	// reachable from a peer on the local network. The port
	// query param defaults to 5245 (this binary's listen
	// port) so the URLs printed in the Settings panel match
	// the URL the user typed in the browser.
	mux.HandleFunc("/api/network/lan-access", a.HandleLanAccess)

	// 4. Listen.
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("listen: %v", err)
	}
}

// registerOpenListTools is a stub: the real encv-go bridge
// lives in plugin_adapter.go and is only built inside
// cmd/agent-demo. The stub registers a small set of mock
// tools that exercise the OpenList client.
func registerOpenListTools(reg *agent.ToolRegistry, cfg agent.AgentConfig) {
	client := agent.NewOpenListClient(cfg.OpenListBaseURL, cfg.OpenListToken, nil)

	// Single shared 30-second context for all OpenList tool
	// calls. M4 fix: previously every handler called
	// httpContext() on each invocation and discarded the
	// cancel function, leaking the timer for 30s.
	slctx, slcancel := httpContext()
	defer slcancel()

	reg.Register("list_files",
		&openListListFilesSchema,
		func(args string) (string, error) {
			var in struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return agent.PluginErrorJSON("invalid_args", err.Error()), nil
			}
			res, err := client.ListFiles(slctx, in.Path)
			if err != nil {
				return agent.PluginErrorJSON("openlist_error", err.Error()), nil
			}
			return toJSON(res), nil
		},
		true, agent.KindReadOnly)

	reg.Register("read_file",
		&openListReadFileSchema,
		func(args string) (string, error) {
			var in struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return agent.PluginErrorJSON("invalid_args", err.Error()), nil
			}
			data, err := client.ReadFile(slctx, in.Path)
			if err != nil {
				return agent.PluginErrorJSON("openlist_error", err.Error()), nil
			}
			return string(data), nil
		},
		true, agent.KindReadOnly)

	reg.Register("write_file",
		&openListWriteFileSchema,
		func(args string) (string, error) {
			var in struct {
				Path    string `json:"path"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return agent.PluginErrorJSON("invalid_args", err.Error()), nil
			}
			ok, err := client.WriteFile(slctx, in.Path, []byte(in.Content))
			if err != nil {
				return agent.PluginErrorJSON("openlist_error", err.Error()), nil
			}
			return strconv.FormatBool(ok), nil
		},
		true, agent.KindFileChange)

	reg.Register("delete_file",
		&openListDeleteFileSchema,
		func(args string) (string, error) {
			var in struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return agent.PluginErrorJSON("invalid_args", err.Error()), nil
			}
			ok, err := client.DeleteFile(slctx, in.Path)
			if err != nil {
				return agent.PluginErrorJSON("openlist_error", err.Error()), nil
			}
			return strconv.FormatBool(ok), nil
		},
		true, agent.KindFileChange)

	reg.Register("rename",
		&openListRenameSchema,
		func(args string) (string, error) {
			var in struct {
				Src string `json:"src"`
				Dst string `json:"dst"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return agent.PluginErrorJSON("invalid_args", err.Error()), nil
			}
			ok, err := client.Rename(slctx, in.Src, in.Dst)
			if err != nil {
				return agent.PluginErrorJSON("openlist_error", err.Error()), nil
			}
			return strconv.FormatBool(ok), nil
		},
		true, agent.KindFileChange)

	reg.Register("exec_command",
		&openListExecCommandSchema,
		func(args string) (string, error) {
			var in struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return agent.PluginErrorJSON("invalid_args", err.Error()), nil
			}
			out, err := client.ExecCommand(slctx, in.Command)
			if err != nil {
				return agent.PluginErrorJSON("openlist_error", err.Error()), nil
			}
			return out, nil
		},
		true, agent.KindCommand)

	reg.Register("get_storage_info",
		&openListGetStorageInfoSchema,
		func(args string) (string, error) {
			info, err := client.GetStorageInfo(slctx)
			if err != nil {
				return agent.PluginErrorJSON("openlist_error", err.Error()), nil
			}
			return toJSON(info), nil
		},
		false, agent.KindReadOnly)

	// list_storages lets the LLM perceive OpenList's mounted
	// file systems before issuing list_files against an
	// unknown path. Without this tool the LLM has to guess
	// mount paths, which silently returns an empty list for
	// nonexistent paths (very confusing for the user).
	reg.Register("list_storages",
		&openListListStoragesSchema,
		func(args string) (string, error) {
			items, err := client.ListStorages(slctx)
			if err != nil {
				return agent.PluginErrorJSON("openlist_error", err.Error()), nil
			}
			return toJSON(items), nil
		},
		false, agent.KindReadOnly)

	reg.Register("search_files",
		&openListSearchFilesSchema,
		func(args string) (string, error) {
			var in struct {
				Parent  string `json:"parent"`
				Keyword string `json:"keyword"`
			}
			if err := json.Unmarshal([]byte(args), &in); err != nil {
				return agent.PluginErrorJSON("invalid_args", err.Error()), nil
			}
			hits, err := client.SearchFiles(slctx, in.Parent, in.Keyword)
			if err != nil {
				return agent.PluginErrorJSON("openlist_error", err.Error()), nil
			}
			return toJSON(hits), nil
		},
		true, agent.KindReadOnly)
}

// registerPluginTools wires the encv-go plugin system. In
// production this would call scanPluginTools against
// `interfaces.Plugins`; the demo registers a no-op plugin set
// so the wiring is exercised end-to-end.
func registerPluginTools(reg *agent.ToolRegistry, cfg agent.AgentConfig) {
	// The real implementation imports
	// `internal/v2/plugins/interfaces` and feeds it into
	// scanPluginTools. For the demo we register a small set
	// of placeholder plugin tools so the registry has
	// entries to enumerate.
	for _, name := range []string{"video", "audio", "image", "wps", "pdf", "text"} {
		op := "encrypt"
		schema := &pluginSchemaPlaceholder
		_ = op
		_ = schema
		reg.Register(name+"_encrypt", &pluginSchemaPlaceholder,
			func(args string) (string, error) {
				return agent.PluginErrorJSON("plugin_bridge_not_wired",
					"This demo build does not wire the encv-go plugin system. "+
						"Use the production binary."), nil
			},
			true, agent.KindFileChange)
	}
}

// applyEnabledTools drops any registered tool whose name is not
// in the enabledTools whitelist. An empty whitelist means
// "register everything" (no filter applied).
func applyEnabledTools(reg *agent.ToolRegistry, enabledTools []string) {
	if len(enabledTools) == 0 {
		return
	}
	keep := make(map[string]bool, len(enabledTools))
	for _, n := range enabledTools {
		keep[n] = true
	}
	for _, name := range reg.Names() {
		if !keep[name] {
			reg.Unregister(name)
		}
	}
}

// countRegistered returns the number of tools in the registry.
func countRegistered(reg *agent.ToolRegistry) int {
	return len(reg.Names())
}

// redact returns a redacted version of a secret suitable for
// log output. The original secret is replaced by a fixed-length
// string of asterisks; the leading and trailing two characters
// of the prefix are kept for debugging.
func redact(s string) string {
	if s == "" {
		return "<empty>"
	}
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return s[:2] + strings.Repeat("*", len(s)-4) + s[len(s)-2:]
}

// toJSON is a thin wrapper that always succeeds.
func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":"marshal_failed","message":%q}`, err.Error())
	}
	return string(b)
}
