package agent

import (
	"context"
	"fmt"
	"os"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// defaultSkillInjectionHook returns a [HookFunc] that, on
// [HookSessionStart], looks up the front-end's selected skill
// names for the session and stores the joined skill prompt
// under the session's per-session system prompt override.
//
// The hook is a no-op when no skills are selected. The skill
// prompts are appended to the (possibly empty) global
// SystemPrompt from AgentConfig, so the LLM always sees the
// base policy AND any skill activations for the session.
//
// Errors are intentionally swallowed: a malformed skill name
// in the request must not abort the chat goroutine. A
// log line is emitted to stderr so an operator can see the
// mismatch in DevLogs.
func defaultSkillInjectionHook(a *Agent) HookFunc {
	return func(ctx context.Context, hc *HookContext) error {
		if hc.Event != HookSessionStart {
			return nil
		}
		if len(hc.SelectedSkills) == 0 {
			return nil
		}
		prompt := composeSkillPrompt(a.Skills, hc.SelectedSkills)
		if prompt == "" {
			fmt.Fprintf(os.Stderr,
				"agent: session %q selected unknown skills %v (loaded: %v)\n",
				hc.SessionID, hc.SelectedSkills, skillNames(a.Skills))
			return nil
		}
		base := a.cfg.SystemPrompt
		var merged string
		if base == "" {
			merged = prompt
		} else {
			merged = base + "\n\n" + prompt
		}
		a.systemPromptBySession.Store(hc.SessionID, merged)
		return nil
	}
}

// skillNames returns a defensive copy of the names of s in
// stable (insertion) order. Used for diagnostic logging.
func skillNames(s []Skill) []string {
	out := make([]string, len(s))
	for i, sk := range s {
		out[i] = sk.Name
	}
	return out
}

// SetSelectedSkills stores the skill names the front-end wants
// to activate for the given session. The HTTP layer calls this
// before Chat so the session_start hook (which runs inside
// runLoop) can read the list from the per-session map.
//
// Passing a nil or empty slice clears the selection. The
// per-session system prompt override written by the default
// hook is NOT cleared; that override is the user's "session
// was opened with these skills" record and persists for the
// session's lifetime.
func (a *Agent) SetSelectedSkills(sessionID string, names []string) {
	if sessionID == "" {
		return
	}
	if len(names) == 0 {
		a.selectedSkillsBySession.Delete(sessionID)
		return
	}
	// Defensive copy: the caller's slice may be reused.
	out := make([]string, len(names))
	copy(out, names)
	a.selectedSkillsBySession.Store(sessionID, out)
}

// selectedSkillsFor returns the skill names registered for
// sessionID, or nil if none. The returned slice is a copy
// held under the agent's mutex-equivalent (sync.Map's
// Load-or-store semantics), so callers can mutate it
// without affecting subsequent calls.
func (a *Agent) selectedSkillsFor(sessionID string) []string {
	v, ok := a.selectedSkillsBySession.Load(sessionID)
	if !ok {
		return nil
	}
	names, ok := v.([]string)
	if !ok {
		return nil
	}
	out := make([]string, len(names))
	copy(out, names)
	return out
}

// SessionSystemPrompt returns the per-session system prompt
// override (the global SystemPrompt from AgentConfig plus the
// activated skills for the session). Returns an empty string
// when no override is set. The chat loop uses this value to
// prepend a system message before each LLM call.
func (a *Agent) SessionSystemPrompt(sessionID string) string {
	v, ok := a.systemPromptBySession.Load(sessionID)
	if !ok {
		return a.cfg.SystemPrompt
	}
	s, _ := v.(string)
	return s
}

// ClearSessionSystemPrompt removes the per-session override,
// reverting subsequent LLM calls to the global SystemPrompt.
// Useful for tests and for an explicit "reset" admin path.
func (a *Agent) ClearSessionSystemPrompt(sessionID string) {
	if sessionID == "" {
		return
	}
	a.systemPromptBySession.Delete(sessionID)
	a.selectedSkillsBySession.Delete(sessionID)
}

// injectSystemPrompt prepends a system message with the
// session's resolved system prompt when one is set. The
// returned slice is freshly allocated so the caller's
// messages slice is never mutated; the original slice is
// reused as the tail to avoid an unnecessary copy of the
// chat history.
//
// The injected message is a single "system" role entry
// with the merged prompt. We do NOT add a per-turn "you
// have these skills available" header — the LLM only needs
// to see the static prompt once per turn, and the chat
// history already carries the conversation.
//
// Task 19 (Plan Mode): when the front-end's toggle is on
// for this session, the plan-mode instruction is appended
// to the system prompt body. The flag does NOT alter the
// tool registry — only the system text — so the agent
// still sees the same tool set, just nudged into
// "list steps first, wait for user confirmation before
// executing" mode.
func (a *Agent) injectSystemPrompt(sessionID string, messages []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	sp := a.SessionSystemPrompt(sessionID)
	if a.planModeFor(sessionID) {
		sp = appendPlanModeInstruction(sp)
	}
	if strings.TrimSpace(sp) == "" {
		return messages
	}
	out := make([]openai.ChatCompletionMessage, 0, len(messages)+1)
	out = append(out, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: sp,
	})
	out = append(out, messages...)
	return out
}

// planModeInstruction is the appended directive text used
// by injectSystemPrompt when planModeFor(sessionID) is true.
// It is package-private so tests can assert the exact
// string the agent sends to the LLM.
const planModeInstruction = "你处于「目标 / Plan 模式」。在执行任何工具或命令之前，先用文字列出完成该任务的完整步骤（每条 1 句、3-7 条），并明确「请用户确认是否执行」。只有在用户回复「确认 / 继续 / go」等明确指令后，才可以开始实际执行。"

// appendPlanModeInstruction returns base with the plan-mode
// directive appended. A leading separator is added when base
// is non-empty so the two halves read as distinct paragraphs
// to the LLM. An all-whitespace base is treated as empty and
// the directive is returned on its own.
func appendPlanModeInstruction(base string) string {
	if strings.TrimSpace(base) == "" {
		return planModeInstruction
	}
	return base + "\n\n" + planModeInstruction
}
