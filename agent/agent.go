package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// SessionCache holds the in-memory event stream for one chat
// session. Events are appended as the agent core pushes them, so
// Resume can replay from any offset.
//
// The mutex is taken on every event append and every Resume
// read, so it is intentionally lightweight (no RWMutex — writes
// and reads are bursty in equal measure and the critical section
// is a slice append / length check).
type SessionCache struct {
	mu         sync.Mutex
	Events     []*Event
	IsFinished bool
	// Todos is the latest snapshot from the plan tool
	// (KindPlan / write_todos). The agent overwrites this
	// every time the LLM emits a new plan tool call so the
	// front-end can render a stable PlanBlock that reflects
	// the assistant's current plan.
	Todos []Todo
}

// appendEvent is the only legitimate way to grow the cache; it
// also broadcasts to any in-flight Resume polls.
func (c *SessionCache) appendEvent(e *Event) {
	c.mu.Lock()
	c.Events = append(c.Events, e)
	c.mu.Unlock()
}

// snapshot returns a defensive copy of the events slice from
// `offset` onward. If the cache has fewer than `offset` events,
// the returned slice has length zero — the caller is expected to
// poll.
func (c *SessionCache) snapshot(offset int) []*Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	if offset >= len(c.Events) {
		return nil
	}
	out := make([]*Event, len(c.Events)-offset)
	copy(out, c.Events[offset:])
	return out
}

// pendingCall is the state of a suspended session. We persist
// the in-flight messages so ConfirmTool can resume from where
// Chat() paused.
type pendingCall struct {
	ToolCallID string
	ToolName   string
	Args       string
	Messages   []openai.ChatCompletionMessage
}

// pendingMessageQueue is a per-session FIFO of message sets
// waiting to be processed by a future Chat call. It is the
// backing store for the queue send mode: ChatMode(mode="queue")
// appends, and the always-on pendingQueueDrainHook consumes the
// front element on HookTurnEnd and spawns a new Chat in a
// fresh goroutine.
type pendingMessageQueue struct {
	mu       sync.Mutex
	messages [][]openai.ChatCompletionMessage
}

func (q *pendingMessageQueue) enqueue(msgs []openai.ChatCompletionMessage) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.messages = append(q.messages, msgs)
}

// dequeue returns the front message set and true, or nil and
// false when the queue is empty. Callers must consume the
// returned slice by value — the queue keeps its own reference
// to the original storage until dequeue is called.
func (q *pendingMessageQueue) dequeue() ([]openai.ChatCompletionMessage, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.messages) == 0 {
		return nil, false
	}
	msgs := q.messages[0]
	q.messages = q.messages[1:]
	return msgs, true
}

// Agent is the long-lived orchestrator. It owns the registry, the
// OpenAI client, the per-session caches, the session-level
// approval grants, and the pending-confirmation state.
//
// store is optional (nil = in-memory only, the legacy mode). When
// non-nil, every event pushed to a session cache is also appended
// to the store's JSONL file so a process restart can replay the
// event stream.
type Agent struct {
	cfg      AgentConfig
	Registry *ToolRegistry
	llm      llmStream

	// serverInstanceId uniquely identifies this in-process agent
	// across its lifetime. It is generated once in NewAgent /
	// NewAgentWithLLM and stays constant for the process. The
	// value is exposed via /api/health so the front-end can
	// detect server restarts and clear stale SSE sequence
	// tracking state.
	serverInstanceId string

	// Sessions maps sessionID → *SessionCache.
	Sessions sync.Map

	// SessionGrants maps "<sessionID>|<toolName>" → struct{} so
	// subsequent calls of the same tool in the same session
	// auto-run.
	SessionGrants sync.Map

	// sessionsParent maps sessionID → parentSessionID. Task 23
	// (Side Conversation / Fork) populates it: when the user
	// clicks the "分叉" button in the front-end, the server
	// calls NewSession(parentID) which (a) generates a fresh
	// UUID for the new session, (b) records the parent link
	// here, and (c) returns the new id. The map is keyed by
	// child id; parent id is the value (empty string means
	// "no parent / root session"). Use Agent.ParentOf to read
	// it. sync.Map keeps the HTTP goroutine and the
	// runResume goroutine race-free without an extra mutex.
	sessionsParent sync.Map

	// PendingCalls maps sessionID → *pendingCall. Only one
	// pending call per session is allowed at a time; the
	// guarantee is enforced by Agent.mu.
	mu           sync.Mutex
	PendingCalls map[string]*pendingCall

	// pendingMessages maps sessionID → FIFO queue of
	// message sets waiting to be processed after the
	// current turn fully ends. The queue is populated by
	// ChatMode with mode="queue" and drained by the always-on
	// pendingQueueDrainHook on HookTurnEnd.
	pendingMessages map[string]*pendingMessageQueue
	pendingMu       sync.Mutex

	// store is the durable JSONL backend. nil means the agent
	// runs purely in memory; the existing 2-arg NewAgent
	// callers therefore keep working without any change.
	store *SessionStore

	// hooks is the user-registered callback list, invoked
	// synchronously on the chat goroutine at each of the 6
	// documented hook points (see HookEvent). Access is
	// serialised by hooksMu: RegisterHook takes the write
	// lock, runHooks snapshots under the read lock so a
	// long hook chain does not block new registrations.
	hooks   []HookFunc
	hooksMu sync.RWMutex

	// Skills is the set of skills loaded from
	// cfg.SkillsDir at NewAgent time. The slice is read-only
	// after construction; concurrent reads from hooks are
	// safe because the slice is never mutated.
	Skills []Skill

	// selectedSkillsBySession maps sessionID → []string
	// (the names the front-end asked to activate for that
	// session). The HTTP layer populates it before Chat;
	// the runLoop reads it when constructing the
	// HookSessionStart context. sync.Map provides the
	// happens-before guarantee between the HTTP goroutine
	// and the chat goroutine.
	selectedSkillsBySession sync.Map

	// systemPromptBySession maps sessionID → string. The
	// default skill-injection hook writes the joined skill
	// prompt here; chatRequest reads it and prepends a
	// system message when the value is non-empty. The map
	// is intentionally separate from the durable store: a
	// per-session override is a runtime concern, not a
	// history concern.
	systemPromptBySession sync.Map

	// planModeBySession maps sessionID → bool. Task 19
	// (Plan Mode toggle) populates it from ChatRequest.PlanMode:
	// a true value causes injectSystemPrompt to append a
	// "list steps first, wait for user confirmation" directive
	// to the per-session system prompt. A false value (or
	// never-set) removes the entry so planModeFor returns
	// false. sync.Map is the same shape as the other
	// per-session maps (selectedSkillsBySession,
	// systemPromptBySession, sessionOverrides) so HTTP
	// goroutines and runLoop goroutines stay race-free
	// without an extra mutex.
	planModeBySession sync.Map

	// permissionModeBySession maps sessionID → string. Task 20
	// (Permission Mode Switcher) populates it from
	// ChatRequest.PermissionMode; the agent core consults it
	// at tool-execution time to decide whether a tool that
	// registered NeedConfirm=true should still auto-run. The
	// default (no entry) means PermissionDefault, the legacy
	// "respect NeedConfirm" behaviour. Entries are removed
	// when the user toggles the switch back to default.
	permissionModeBySession sync.Map

	// compactor, when non-nil, is consulted at the top of
	// every streamOneTurn. If the running messages slice
	// exceeds compactor.threshold * window, the agent asks
	// the LLM for a one-shot summary and replaces the older
	// messages in place. NewAgent / NewAgentWithLLM build
	// a sensible default (DefaultModelContextWindow tokens,
	// 0.8 threshold) so existing callers automatically gain
	// compaction; tests that need a deterministic behaviour
	// can override via SetCompactor.
	compactor *Compactor

	// summaryFnForTest is a test-only hook that overrides
	// buildSummaryFn's return value. nil = use the default
	// (openai-backed) summary fn. Production code MUST NOT
	// touch this field; the SetSummaryFnForTest method is
	// the only legitimate setter.
	summaryFnForTest SummaryFunc
}

// NewAgent constructs an Agent. The registry must already contain
// all tools; NewAgent does not auto-register anything. The store
// is optional: when omitted (or passed as nil) the agent runs
// purely in memory; when provided, every event is also appended
// to the store so a process restart can replay sessions via
// Resume.
//
// NewAgent also walks cfg.SkillsDir (defaulting to
// "$HOME/.encv/skills" when empty) and registers a default
// session_start hook that injects the front-end's selected
// skills into the per-session system prompt. The hook is a
// no-op when no skills are loaded, so existing callers
// continue to behave exactly as before.
func NewAgent(cfg AgentConfig, registry *ToolRegistry, storeOpt ...*SessionStore) *Agent {
	if registry == nil {
		registry = NewRegistry()
	}
	var store *SessionStore
	if len(storeOpt) > 0 {
		store = storeOpt[0]
	}
	a := &Agent{
		cfg:               cfg,
		Registry:          registry,
		llm:               &openaiStream{client: newOpenAIClient(cfg)},
		PendingCalls:      make(map[string]*pendingCall),
		pendingMessages:   make(map[string]*pendingMessageQueue),
		store:             store,
		serverInstanceId:  generateServerInstanceId(),
	}
	a.loadSkillsAndRegisterHook(cfg.SkillsDir)
	return a
}

// NewAgentWithLLM lets tests inject a fake llmStream. Production
// code should use NewAgent. The store argument follows the same
// variadic-nil convention as NewAgent. The skills scan follows
// the same convention as NewAgent: configured via
// cfg.SkillsDir, with a default session_start hook registered
// only when at least one skill is found.
func NewAgentWithLLM(cfg AgentConfig, registry *ToolRegistry, llm llmStream, storeOpt ...*SessionStore) *Agent {
	if registry == nil {
		registry = NewRegistry()
	}
	var store *SessionStore
	if len(storeOpt) > 0 {
		store = storeOpt[0]
	}
	a := &Agent{
		cfg:              cfg,
		Registry:         registry,
		llm:              llm,
		PendingCalls:     make(map[string]*pendingCall),
		pendingMessages:  make(map[string]*pendingMessageQueue),
		store:            store,
		serverInstanceId: generateServerInstanceId(),
	}
	a.loadSkillsAndRegisterHook(cfg.SkillsDir)
	a.registerPendingQueueDrainHook()
	return a
}

// loadSkillsAndRegisterHook scans the skills directory and
// registers the default session_start hook. It is split out
// from NewAgent / NewAgentWithLLM so both constructors share
// the same one-line implementation.
//
// A missing or unreadable skills directory is not an error:
// the agent simply runs with no skills. The hook is registered
// only when at least one skill was parsed, so a skill-free
// agent does not pay the per-session hook dispatch cost.
func (a *Agent) loadSkillsAndRegisterHook(skillsDir string) {
	dir := skillsDir
	if dir == "" {
		dir = defaultSkillsDir()
	}
	skills, err := ScanSkills(dir)
	if err != nil {
		// We log to stderr rather than failing the agent
		// bootstrap: a broken skill manifest is a
		// user-correctable configuration error, not a
		// reason to refuse to start.
		fmt.Fprintf(os.Stderr, "agent: skills scan from %s failed: %v\n", dir, err)
		return
	}
	if len(skills) == 0 {
		return
	}
	a.Skills = skills
	a.RegisterHook(defaultSkillInjectionHook(a))
}

// generateServerInstanceId returns a string that uniquely
// identifies the current process. It combines the host name, the
// process ID, and the construction time. The value is stable for
// the lifetime of the process and is regenerated on every
// restart. Nanosecond precision is used so two NewAgent calls
// within the same second still receive distinct ids — this
// matters for test suites that build multiple agents in quick
// succession.
func generateServerInstanceId() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s-%d-%d", host, os.Getpid(), time.Now().UnixNano())
}

// ServerInstanceId returns the process-wide unique id for this
// Agent. The value is generated once in NewAgent and stays
// constant for the lifetime of the process. HTTP handlers expose
// it via /api/health.
func (a *Agent) ServerInstanceId() string {
	return a.serverInstanceId
}

// ensureSession returns the cache for sessionID, creating one if
// it does not exist yet.
func (a *Agent) ensureSession(sessionID string) *SessionCache {
	if v, ok := a.Sessions.Load(sessionID); ok {
		if cache, ok := v.(*SessionCache); ok {
			return cache
		}
	}
	c := &SessionCache{}
	actual, _ := a.Sessions.LoadOrStore(sessionID, c)
	if cache, ok := actual.(*SessionCache); ok {
		return cache
	}
	// The map somehow holds a non-SessionCache value; recover by
	// storing and returning a fresh one. Callers always end up
	// with a usable cache, just a "lost" one in the slot.
	c2 := &SessionCache{}
	a.Sessions.Store(sessionID, c2)
	return c2
}

// getSession fetches a cache without creating it.
func (a *Agent) getSession(sessionID string) (*SessionCache, bool) {
	v, ok := a.Sessions.Load(sessionID)
	if !ok {
		return nil, false
	}
	cache, ok := v.(*SessionCache)
	if !ok {
		return nil, false
	}
	return cache, true
}

// Chat kicks off a streaming turn. The returned channel emits
// Events in real time and is closed when the stream ends (either
// naturally or because a tool call awaits confirmation).
//
// If a previous turn on the same sessionID is still pending
// confirmation, the new call is rejected so we never end up with
// two concurrent goroutines mutating the same session cache.
//
// The first Chat call on a sessionID triggers
// [HookSessionStart]; subsequent calls on the same sessionID do
// not re-fire it. The detection is "did the session exist before
// this call?", not "was this Chat's caller an HTTP request?" —
// internal re-entry via ConfirmTool is treated as the same
// session and does not re-fire.
func (a *Agent) Chat(
	ctx context.Context,
	sessionID string,
	messages []openai.ChatCompletionMessage,
) (<-chan *Event, error) {
	return a.ChatMode(ctx, sessionID, messages, "")
}

// NewSession (Task 23 — Side Conversation / Fork) mints a
// fresh, never-used session id and records the parent → child
// link in a.sessionsParent. The returned id is distinct from
// the parent id (UUID v4) and the parent's SessionCache is
// not mutated by the call. An empty parentID is treated as
// "root": a brand-new id is still returned and ParentOf on
// the new id will return ("", false) — the explicit empty
// parent entry is reserved for a future iteration of the
// spec.
//
// This stub is intentionally minimal: it does NOT deep-copy
// the parent's messages into the child. The deep-copy
// semantics are a follow-up task; the test suite only locks
// id-uniqueness and parent-link invariants at this stage.
func (a *Agent) NewSession(parentID string) string {
	newID := generateNewSessionID()
	a.sessionsParent.Store(newID, parentID)
	// Pre-create the SessionCache so a subsequent Chat /
	// Resume call on the new id does not race the first
	// emit. This mirrors the "always pre-create" pattern
	// used by ensureSession for the Chat entry point.
	a.ensureSession(newID)
	return newID
}

// ParentOf returns the parent session id for childID. The
// boolean is true when an explicit entry exists; the value
// is the parent id (which may itself be empty for "root"
// forks). This is the read-side counterpart of NewSession.
//
// An empty childID or an unknown id returns ("", false) —
// sync.Map.Load is the only branch that ever returns
// ok=false, so unknown sessions are reported exactly like
// the legacy "no parent" sentinel without a separate code
// path.
func (a *Agent) ParentOf(childID string) (string, bool) {
	if childID == "" {
		return "", false
	}
	v, ok := a.sessionsParent.Load(childID)
	if !ok {
		return "", false
	}
	pid, _ := v.(string)
	return pid, true
}

// HandleFork is the HTTP entry point for POST /api/agent/fork.
// The front-end dispatches a fork request when the user
// clicks the "分叉" button in the chat header. The handler:
//
//   1. Rejects non-POST methods with 405 (requirePOST contract).
//   2. Rejects missing/empty session_id with 400.
//   3. On success, mints a new session id via
//      [Agent.NewSession] and returns a JSON body of shape
//      {"session_id": "<new>", "parent_session_id": "<parent>"}.
//
// The handler is a thin shim over NewSession + ParentOf —
// it exists so the front-end can stay decoupled from the
// in-memory map and so future iterations of the spec (deep
// copy of messages, parent breadcrumb in the SSE stream)
// have a single place to wire into.
func (a *Agent) HandleFork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SessionID == "" {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	newID := a.NewSession(req.SessionID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"session_id":        newID,
		"parent_session_id": req.SessionID,
	})
}

// generateNewSessionID returns a hex-encoded 16-byte random
// id suitable for use as a session id. It is split out from
// the server-instance-id generator because session ids must
// be (a) unique per call and (b) cheap to mint, whereas the
// server instance id is generated once at process start and
// is allowed to be deterministic (hostname + pid hash). A
// crypto/rand failure falls back to a time-based id so a
// faulty RNG never wedges the fork path.
func generateNewSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is essentially never; if it ever
		// happens the only correct behaviour is to crash the
		// goroutine rather than risk a collision. Use
		// log.Fatalf-style panic: returning a deterministic
		// fallback (time.Now().UnixNano) can collide under
		// high-concurrency SessionStart storms.
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b[:])
}

// ChatMode is the mode-aware entry point that powers the
// front-end's "steer" / "queue" send buttons. mode is one of:
//   - "" or "start": identical to Chat — a fresh turn is
//     started immediately. This is the legacy behaviour.
//   - "steer":       also a fresh turn, semantically marking
//     the message as a course-correction to an in-flight turn
//     the front-end knows about. The agent runs the LLM call
//     in the same way as start; the difference is purely a
//     client/server contract marker so the front-end can tell
//     turns apart. The confirm flow is untouched: if the
//     steered response triggers a tool call, the
//     [ApprovalCard] still surfaces.
//   - "queue":       the message is held in
//     agent.pendingMessages[sessionID] and a new Chat is
//     triggered after the current turn fully ends. The
//     returned channel is closed immediately so HTTP callers
//     can finish their response; the actual events surface
//     through the existing SSE / Resume stream once the
//     queued turn starts running.
//
// Unknown mode values return an error so typos surface
// immediately rather than silently reverting to start.
func (a *Agent) ChatMode(
	ctx context.Context,
	sessionID string,
	messages []openai.ChatCompletionMessage,
	mode string,
) (<-chan *Event, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("agent: sessionID must not be empty")
	}
	switch mode {
	case "", "start", "steer":
		// Fall through to the existing Chat path.
	case "queue":
		return a.enqueueAndReturnClosed(sessionID, messages), nil
	default:
		return nil, fmt.Errorf("agent: unknown chat mode %q (want \"\", \"start\", \"steer\" or \"queue\")", mode)
	}
	a.mu.Lock()
	if _, busy := a.PendingCalls[sessionID]; busy {
		a.mu.Unlock()
		return nil, fmt.Errorf("agent: session %q is awaiting confirmation", sessionID)
	}
	a.mu.Unlock()

	_, existed := a.getSession(sessionID)
	_ = a.ensureSession(sessionID)
	out := make(chan *Event, 32)
	go a.runLoop(ctx, sessionID, messages, out, !existed)
	return out, nil
}

// SetPlanMode (Task 19) records the front-end's plan-mode
// toggle state for sessionID. The HTTP layer calls this on
// every Chat so the flag is always up-to-date with what the
// user sees in the composer. Passing false (the zero value)
// deletes the entry so a session that toggles plan mode off
// reverts to the default behaviour without leaving a stale
// "true" behind.
//
// The flag is consulted by injectSystemPrompt, which appends
// a plan-aware instruction to the system message when set.
// It does NOT alter the tool registry — see package doc.
func (a *Agent) SetPlanMode(sessionID string, enabled bool) {
	if sessionID == "" {
		return
	}
	if !enabled {
		a.planModeBySession.Delete(sessionID)
		return
	}
	a.planModeBySession.Store(sessionID, true)
}

// planModeFor returns true when SetPlanMode(sessionID, true)
// was last called for the given session. Returns false (the
// default) for sessions that have never sent planMode, never
// enabled it, or explicitly cleared it. Used by
// injectSystemPrompt to decide whether to append the
// plan-aware instruction.
func (a *Agent) planModeFor(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	v, ok := a.planModeBySession.Load(sessionID)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// SetSessionPermissionMode stores the per-session
// permission tier for sessionID. The reader
// ([Agent.permissionModeFor]) treats unknown / empty
// values as PermissionDefault at lookup time, so a
// caller that forgets to validate the string first
// still produces safe runtime behaviour. An empty
// sessionID is a no-op (defensive). A PermissionDefault
// (or empty) value deletes the per-session entry so a
// session that toggles back to "default" reverts on the
// very next turn with no stale "auto-review" left behind
// — the legacy behaviour is exactly "no entry" for
// default mode.
//
// Task 20 (Permission Mode Switcher). See
// ChatRequest.PermissionMode for the wire contract.
func (a *Agent) SetSessionPermissionMode(sessionID string, mode PermissionMode) {
	if sessionID == "" {
		return
	}
	if mode == "" || mode == PermissionDefault {
		a.permissionModeBySession.Delete(sessionID)
		return
	}
	a.permissionModeBySession.Store(sessionID, mode)
}

// ClearSessionPermissionMode removes the per-session
// permission tier for sessionID, reverting subsequent
// emitToolCall decisions to the tool's registered
// NeedConfirm value. The method exists for parity with the
// per-session plan-mode toggle and is used by tests that
// share a single agent across multiple scenarios.
// An empty sessionID is a no-op; an unknown sessionID
// is also a no-op because sync.Map.Delete is idempotent.
func (a *Agent) ClearSessionPermissionMode(sessionID string) {
	if sessionID == "" {
		return
	}
	a.permissionModeBySession.Delete(sessionID)
}

// permissionModeFor returns the per-session permission
// tier for sessionID, or PermissionDefault when none is
// set or the stored value is not one of the three
// documented constants. The method is the read-side
// counterpart of SetSessionPermissionMode and is invoked
// from streamOneTurn at every emitToolCall call.
// An empty return value MUST be treated by the caller as
// "no override" — the agent falls back to the tool's
// registered NeedConfirm value in that case, which is the
// legacy behaviour.
func (a *Agent) permissionModeFor(sessionID string) PermissionMode {
	if sessionID == "" {
		return PermissionDefault
	}
	v, ok := a.permissionModeBySession.Load(sessionID)
	if !ok {
		return PermissionDefault
	}
	mode, _ := v.(PermissionMode)
	if !IsValidPermissionMode(mode) {
		return PermissionDefault
	}
	return mode
}

// enqueueAndReturnClosed stores the messages on the
// per-session pending queue and returns a channel that is
// already closed. The drain hook will pick the messages up
// on the next HookTurnEnd and start a fresh Chat.
//
// The whole critical section (map lookup + queue enqueue) is
// held under a.pendingMu to close the M1 race window where a
// drain hook could observe the queue as empty between the map
// insert and the enqueue call.
func (a *Agent) enqueueAndReturnClosed(
	sessionID string,
	messages []openai.ChatCompletionMessage,
) <-chan *Event {
	a.pendingMu.Lock()
	q, ok := a.pendingMessages[sessionID]
	if !ok {
		q = &pendingMessageQueue{}
		a.pendingMessages[sessionID] = q
	}
	q.messages = append(q.messages, messages)
	a.pendingMu.Unlock()
	out := make(chan *Event, 1)
	close(out)
	return out
}

// registerPendingQueueDrainHook wires the always-on HookTurnEnd
// listener that drains the per-session queue after every turn
// ends. Multiple queued messages are processed in FIFO order —
// each drain spawns a new runLoop, whose own HookTurnEnd
// re-fires the hook to pick the next message.
func (a *Agent) registerPendingQueueDrainHook() {
	a.RegisterHook(a.pendingQueueDrainHook)
}

// pendingQueueDrainHook is the body of the drain hook. It is
// exported indirectly via RegisterHook so the registration
// stays in one place; the implementation is small enough that
// inlining it in NewAgentWithLLM would clutter that
// constructor.
func (a *Agent) pendingQueueDrainHook(ctx context.Context, hc *HookContext) error {
	if hc == nil || hc.Event != HookTurnEnd {
		return nil
	}
	a.pendingMu.Lock()
	q, ok := a.pendingMessages[hc.SessionID]
	if !ok {
		a.pendingMu.Unlock()
		return nil
	}
	if len(q.messages) == 0 {
		a.pendingMu.Unlock()
		return nil
	}
	msgs := q.messages[0]
	q.messages = q.messages[1:]
	a.pendingMu.Unlock()

	// Spawn a new Chat in its own goroutine — calling Chat
	// directly from the hook would deadlock because the
	// current runLoop is still on the call stack (the hook
	// fires synchronously from streamOneTurn's defer).
	//
	// Context: use the hook's context (M3) so the queued
	// Chat inherits the original request lifetime. The
	// previous context.Background() could leak goroutines
	// if the user disconnected mid-flight.
	//
	// M1 retry: if the goroutine observes the queue as
	// empty (e.g. an enqueue lost the race against our
	// drain), retry a few times before giving up so the
	// user does not have to re-send. Bounded retries keep
	// the worst case bounded.
	_ = ctx // silence linter when not directly used in this branch
	drainCtx := ctx
	if drainCtx == nil {
		drainCtx = context.Background()
	}
	go func() {
		a.spawnQueuedChat(drainCtx, hc.SessionID, msgs)
	}()
	return nil
}

// spawnQueuedChat wraps the fire-and-forget Chat call with a
// short retry loop in case the dequeue lost a race against
// enqueueAndReturnClosed.
func (a *Agent) spawnQueuedChat(ctx context.Context, sessionID string, msgs []openai.ChatCompletionMessage) {
	for attempt := 0; attempt < 5; attempt++ {
		// Re-check: maybe an enqueue raced ahead of us.
		a.pendingMu.Lock()
		if len(msgs) == 0 {
			q, ok := a.pendingMessages[sessionID]
			if !ok || len(q.messages) == 0 {
				a.pendingMu.Unlock()
				return
			}
			msgs = q.messages[0]
			q.messages = q.messages[1:]
		}
		a.pendingMu.Unlock()

		ch, err := a.Chat(ctx, sessionID, msgs)
		if err == nil {
			// Drain the channel so the runLoop keeps producing
			// (it would deadlock waiting for a consumer on
			// the unbuffered path).
			for range ch {
			}
			return
		}
		// The most common cause is a pending tool
		// confirmation; in that case the user can
		// simply re-queue. We do not retry the Chat
		// itself, but we DO yield so the producer can
		// finish enqueuing, then re-check the queue.
		msgs = nil
		time.Sleep(5 * time.Millisecond)
	}
}

// runLoop is the worker goroutine behind Chat/ConfirmTool. It is
// the single owner of the session cache and the OpenAI stream
// for one turn; the loop is re-entered after auto-run tools to
// fetch the next assistant message.
//
// Channel lifecycle: the channel is closed by `finishAndClose`
// which guarantees the final stream_end event lands before the
// channel is closed. The single-owner rule (only runLoop or
// resumeAfterDecision may call finishAndClose) avoids the
// double-close pitfall.
//
// isNewSession distinguishes the very first runLoop entry for a
// session (a brand-new Chat) from a re-entry (ConfirmTool
// handing control back to the LLM after a tool decision). Only
// the former fires [HookSessionStart]; the latter is a turn
// continuation within an already-open session.
func (a *Agent) runLoop(
	ctx context.Context,
	sessionID string,
	messages []openai.ChatCompletionMessage,
	out chan<- *Event,
	isNewSession bool,
) {
	// Use a pointer to the local messages slice so that any
	// append inside streamOneTurn propagates back to the next
	// loop iteration. The previous code passed the slice by
	// value, which silently dropped tool result messages
	// (see tool_call_diagnosis.md C1 / C6 — auto-run multi-turn
	// tool calling loop was completely broken).
	//
	// We retain the named local `messages` so the rest of
	// runLoop reads naturally; only the call to streamOneTurn
	// takes the address.
	defer a.finishAndClose(sessionID, out)

	if isNewSession {
		a.runHooks(ctx, &HookContext{
			Event:          HookSessionStart,
			SessionID:      sessionID,
			Messages:       messages,
			SelectedSkills: a.selectedSkillsFor(sessionID),
		})
	}

	for turn := 0; ; turn++ {
		if a.cfg.MaxToolCallsPerTurn > 0 && turn >= a.cfg.MaxToolCallsPerTurn {
			a.emitError(sessionID, out, "max_tool_calls_exceeded",
				fmt.Sprintf("reached MaxToolCallsPerTurn=%d", a.cfg.MaxToolCallsPerTurn))
			return
		}

		shouldContinue, err := a.streamOneTurn(ctx, sessionID, &messages, out)
		if err != nil {
			a.emitError(sessionID, out, "openai_error", err.Error())
			return
		}
		if !shouldContinue {
			return
		}
	}
}

// streamOneTurn runs ONE round of "OpenAI stream → tool calls →
// either auto-execute or suspend". It returns shouldContinue=true
// when the loop should fetch the next assistant message
// (i.e. all tool calls were auto-executed).
//
// [HookTurnStart] fires synchronously at the top, before the
// LLM stream is opened. [HookTurnEnd] fires via defer on every
// return path (success, error, suspended) so the hook always
// sees the final messages slice regardless of how the turn
// terminated.
//
// messages is a POINTER to the caller's slice: every append /
// truncate in this function writes through *messages so the
// caller (runLoop) observes the post-turn state on its next
// iteration. Passing a plain slice would break auto-run
// multi-turn tool chains (see tool_call_diagnosis.md C1, C6).
func (a *Agent) streamOneTurn(
	ctx context.Context,
	sessionID string,
	messages *[]openai.ChatCompletionMessage,
	out chan<- *Event,
) (bool, error) {
	a.runHooks(ctx, &HookContext{
		Event:     HookTurnStart,
		SessionID: sessionID,
		Messages:  *messages,
	})
	// Deferred so we observe the post-tool-call messages
	// snapshot rather than the pre-LLM one. The closure
	// captures the messages parameter by reference (a
	// function parameter is still a local variable), so any
	// append inside this body is visible to the hook.
	defer func() {
		a.runHooks(ctx, &HookContext{
			Event:     HookTurnEnd,
			SessionID: sessionID,
			Messages:  *messages,
		})
	}()

	// Compaction: when the running history exceeds the
	// configured threshold, ask the LLM (via the agent's
	// own openai client) for a one-shot summary and
	// replace the older messages in place. The event is
	// pushed BEFORE the LLM stream starts so the front-end
	// sees the divider at the position the compacted
	// messages used to occupy. If compaction fails (e.g. the
	// LLM call errors out), we surface the error and abort
	// the turn — half-applied state would be worse than no
	// compaction at all.
	if a.compactor != nil {
		_, replaced, compactErr := a.maybeCompact(ctx, sessionID, *messages, out)
		if compactErr != nil {
			return false, fmt.Errorf("compaction failed: %w", compactErr)
		}
		if replaced > 0 {
			// Truncate the slice header so subsequent
			// append(...) calls in this turn see the
			// compacted length, not the original.
			// Write through the pointer so runLoop's
			// next iteration also sees the compacted
			// length (C6 fix).
			keep := len(*messages) - replaced
			if keep < 0 {
				keep = 0
			}
			*messages = (*messages)[:1+keep]
		}
	}

	req := a.chatRequest(a.injectSystemPrompt(sessionID, *messages), a.cfg.OpenAIModel)
	stream, err := a.llm.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return false, fmt.Errorf("create chat stream: %w", err)
	}
	defer stream.Close()

	// 1. Drain the stream, accumulate deltas, push events.
	assistant := openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant}
	var toolCallsByIndex = make(map[int]*parsedToolCall)

	for {
		chunk, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return false, fmt.Errorf("openai stream recv: %w", err)
		}
		delta := parseDelta(chunk)
		if delta.Text != "" {
			assistant.Content += delta.Text
			a.emitData(sessionID, out, EventTextDelta,
				mustJSON(map[string]string{"content": delta.Text}))
		}
		if delta.Reasoning != "" {
			a.emitData(sessionID, out, EventReasoningDelta,
				mustJSON(map[string]string{"content": delta.Reasoning}))
		}
		for _, ptc := range delta.ToolCalls {
			idx := len(toolCallsByIndex)
			existing, ok := toolCallsByIndex[idx]
			if !ok {
				toolCallsByIndex[idx] = &parsedToolCall{ID: ptc.ID, Name: ptc.Name, Arguments: ptc.Arguments}
			} else {
				if ptc.ID != "" {
					existing.ID = ptc.ID
				}
				if ptc.Name != "" {
					existing.Name = ptc.Name
				}
				existing.Arguments += ptc.Arguments
			}
		}
		if delta.Finished {
			break
		}
	}

	// 2. Persist the assistant message into the rolling history.
	if len(toolCallsByIndex) > 0 {
		openAITCs := make([]openai.ToolCall, 0, len(toolCallsByIndex))
		for i := 0; i < len(toolCallsByIndex); i++ {
			ptc := toolCallsByIndex[i]
			openAITCs = append(openAITCs, openai.ToolCall{
				ID:   ptc.ID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      ptc.Name,
					Arguments: ptc.Arguments,
				},
			})
		}
		assistant.ToolCalls = openAITCs
	}
	if assistant.Content != "" || len(assistant.ToolCalls) > 0 {
		*messages = append(*messages, assistant)
	}

	// 3. No tool calls → turn is over.
	if len(toolCallsByIndex) == 0 {
		return false, nil
	}

	// 4. Process each tool call. The order is the order in which
	//    the LLM emitted them.
	anySuspended := false
	for i := 0; i < len(toolCallsByIndex); i++ {
		ptc := toolCallsByIndex[i]
		if ptc.ID == "" {
			// The LLM sometimes omits the ID on later deltas of
			// the same tool call. Skip the empty filler chunks;
			// the real one will arrive with the same index.
			continue
		}

		def, ok := a.Registry.Get(ptc.Name)
		if !ok {
			// Unknown tool → push a synthetic error result so
			// the LLM can recover and try a different tool.
			a.emitToolCall(sessionID, out, ptc, def, false)
			a.emitData(sessionID, out, EventToolResult, mustJSON(ToolResultData{
				ID:      ptc.ID,
				Name:    ptc.Name,
				Result:  fmt.Sprintf(`{"error":"unknown_tool","name":%q}`, ptc.Name),
				IsError: true,
				Status:  "failed",
			}))
			*messages = append(*messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: ptc.ID,
				Content:    fmt.Sprintf(`{"error":"unknown_tool","name":%q}`, ptc.Name),
			})
			continue
		}

		// Task 20 — Permission Mode Switcher. The legacy
		// autoRun computation is preserved exactly; the new
		// modes (auto-review / full-access) simply force
		// autoRun=true so the ApprovalCard is skipped. The
		// tool_call + tool_status("running") + tool_result
		// event sequence is unchanged, which means the
		// audit trail (HookPreToolCall / HookPostToolCall,
		// the recorder store, etc.) is unaffected — only
		// the user-facing confirmation flow changes.
		autoRun := !def.NeedConfirm || a.isGranted(sessionID, ptc.Name)
		if mode := a.permissionModeFor(sessionID); mode == PermissionAutoReview || mode == PermissionFullAccess {
			autoRun = true
		}
		a.emitToolCall(sessionID, out, ptc, def, autoRun)

		if !autoRun {
			// Suspend: store the messages history and break
			// out of the loop. ConfirmTool will resume.
			a.mu.Lock()
			a.PendingCalls[sessionID] = &pendingCall{
				ToolCallID: ptc.ID,
				ToolName:   ptc.Name,
				Args:       ptc.Arguments,
				Messages:   cloneMessages(*messages),
			}
			a.mu.Unlock()
			anySuspended = true
			continue
		}

		// Auto-run path. PreToolCall / PostToolCall hooks
		// fire around the handler invocation; a hook that
		// sets hc.Cancel=true short-circuits the execution
		// and synthesises a "cancelled" tool result so the
		// LLM can continue the turn.
		kind := def.Kind
		if kind == "" {
			kind = KindUnknown
		}
		hookToolCall := &ToolCallData{
			ID:      ptc.ID,
			Name:    ptc.Name,
			Args:    ptc.Arguments,
			AutoRun: true,
			Kind:    kind,
		}
		cancel := false
		a.runHooks(ctx, &HookContext{
			Event:     HookPreToolCall,
			SessionID: sessionID,
			Messages:  *messages,
			ToolCall:  hookToolCall,
			Cancel:    &cancel,
		})
		if cancel {
			cancelled := `{"error":"cancelled_by_hook"}`
			a.emitData(sessionID, out, EventToolResult, mustJSON(ToolResultData{
				ID:      ptc.ID,
				Name:    ptc.Name,
				Result:  cancelled,
				IsError: true,
				Status:  "cancelled",
			}))
			*messages = append(*messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: ptc.ID,
				Content:    cancelled,
			})
			continue
		}

		// Plan tool (KindPlan / write_todos) has its own
		// bespoke execution path: the handler signature does
		// not include sessionID, so the agent core bridges
		// the call to runPlanTool which parses the todos and
		// stores them on SessionCache.Todos. EventToolStatus
		// and EventToolResult are still pushed on the wire
		// (the rest of the loop below handles that) so the
		// front-end sees a uniform tool-call lifecycle.
		var resultStr string
		var status string
		var durMs int64
		var runErr error
		if def.Kind == KindPlan {
			resultStr, status, durMs, runErr = a.runPlanTool(sessionID, ptc.Arguments)
		} else {
			resultStr, status, durMs, runErr = a.runTool(def, ptc.Arguments)
		}
		hookToolResult := &ToolResultData{
			ID:         ptc.ID,
			Name:       ptc.Name,
			Result:     resultStr,
			IsError:    runErr != nil,
			Status:     status,
			DurationMs: durMs,
		}
		a.runHooks(ctx, &HookContext{
			Event:      HookPostToolCall,
			SessionID:  sessionID,
			Messages:   *messages,
			ToolCall:   hookToolCall,
			ToolResult: hookToolResult,
		})
		a.emitData(sessionID, out, EventToolResult, mustJSON(ToolResultData{
			ID:         ptc.ID,
			Name:       ptc.Name,
			Result:     resultStr,
			IsError:    runErr != nil,
			Status:     status,
			DurationMs: durMs,
		}))
		if runErr != nil {
			// Failure result is still appended to the message
			// history so the LLM can see what went wrong and
			// try a different approach.
			*messages = append(*messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				ToolCallID: ptc.ID,
				Content:    resultStr,
			})
			continue
		}
		*messages = append(*messages, openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			ToolCallID: ptc.ID,
			Content:    resultStr,
		})
	}

	if anySuspended {
		// Do not loop back to the LLM; the user (or the
		// frontend) must ConfirmTool.
		return false, nil
	}
	return true, nil
}

// runTool invokes a handler and times the call. The duration is
// pushed as a ToolStatus update so the UI can render a "running"
// badge that becomes "success" / "failed".
func (a *Agent) runTool(def ToolDefinition, args string) (string, string, int64, error) {
	t0 := time.Now()
	result, err := def.Handler(args)
	dur := time.Since(t0).Milliseconds()
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), "failed", dur, err
	}
	return result, "success", dur, nil
}

// runPlanTool is the bespoke executor for the KindPlan
// (write_todos) tool. It parses the args into a []Todo and
// overwrites the session cache's Todos slice, so the front-end
// can read the latest plan snapshot from a single field
// (SessionCache.Todos) without having to walk the event log.
//
// The handler is registered as a no-op closure in
// NewPlanToolHandler — the agent core bridges sessionID into
// the call here because the standard handler signature
// `func(args string) (string, error)` does not include it.
//
// The tool result is always success; malformed input is
// surfaced as a tool result with status="failed" so the LLM
// can see the error in the next turn.
func (a *Agent) runPlanTool(sessionID, args string) (string, string, int64, error) {
	t0 := time.Now()
	var payload struct {
		Todos []Todo `json:"todos"`
	}
	if err := json.Unmarshal([]byte(args), &payload); err != nil {
		return fmt.Sprintf(`{"error":"invalid_args","message":%q}`, err.Error()), "failed", time.Since(t0).Milliseconds(), nil
	}
	cache := a.ensureSession(sessionID)
	cache.mu.Lock()
	cache.Todos = payload.Todos
	cache.mu.Unlock()
	return `{"ok":true,"count":` + fmt.Sprintf("%d", len(payload.Todos)) + `}`, "success", time.Since(t0).Milliseconds(), nil
}

// emitToolCall pushes an EventToolCall (with AutoRun and Kind) and
// an EventToolStatus("running") so the UI can render the badge.
func (a *Agent) emitToolCall(sessionID string, out chan<- *Event, ptc *parsedToolCall, def ToolDefinition, autoRun bool) {
	kind := def.Kind
	if kind == "" {
		kind = KindUnknown
	}
	name := ptc.Name
	if name == "" && def.Kind != "" {
		// Some defs have a fixed name registered; the LLM will
		// never send us an empty name, so this branch is for
		// defensive completeness only.
		name = "<unknown>"
	}
	a.emitData(sessionID, out, EventToolCall, mustJSON(ToolCallData{
		ID:      ptc.ID,
		Name:    name,
		Args:    ptc.Arguments,
		AutoRun: autoRun,
		Kind:    kind,
	}))
	if autoRun {
		a.emitData(sessionID, out, EventToolStatus, mustJSON(ToolStatusData{
			ID:     ptc.ID,
			Status: "running",
		}))
	}
}

// isGranted checks the session-level grant map.
func (a *Agent) isGranted(sessionID, toolName string) bool {
	_, ok := a.SessionGrants.Load(grantKey(sessionID, toolName))
	return ok
}

func grantKey(sessionID, toolName string) string {
	return sessionID + "|" + toolName
}

// RegisterHook adds a callback to the agent's hook list. Hooks
// are invoked synchronously on the chat goroutine, in
// registration order, at each of the 6 documented
// [HookEvent] points. Safe to call from any goroutine,
// concurrently with hook dispatch (the dispatch path
// snapshots the slice under a read lock).
//
// The callback MUST NOT block for long. Long-running side
// work (network calls, disk I/O) should be dispatched onto a
// background goroutine inside the hook body, otherwise the
// chat goroutine stalls.
//
// The callback MUST NOT call back into the agent's public
// API (Chat / ConfirmTool / ShutdownSession) for the same
// session, or it will deadlock the worker goroutine that
// is currently invoking it.
func (a *Agent) RegisterHook(h HookFunc) {
	a.hooksMu.Lock()
	a.hooks = append(a.hooks, h)
	a.hooksMu.Unlock()
}

// ShutdownSession fires the [HookSessionShutdown] event for
// the given sessionID. It is the explicit "session ended"
// trigger called by the front-end (or a maintenance job)
// when the user is done with a session.
//
// The agent does NOT destroy any state: the SessionCache
// stays in memory so a subsequent Resume call can still
// replay cached events. The hook exists so audit loggers /
// durable state flushers can react to the user's intent.
func (a *Agent) ShutdownSession(ctx context.Context, sessionID string) {
	a.runHooks(ctx, &HookContext{
		Event:     HookSessionShutdown,
		SessionID: sessionID,
	})
}

// ConfirmTool applies a user decision to a pending tool call and
// resumes the loop. The returned channel is identical in shape to
// Chat's.
func (a *Agent) ConfirmTool(
	ctx context.Context,
	sessionID, toolCallID string,
	decision Decision,
) (<-chan *Event, error) {
	if !isValidDecision(decision) {
		return nil, fmt.Errorf("agent: invalid decision %q", decision)
	}

	a.mu.Lock()
	pc, ok := a.PendingCalls[sessionID]
	if !ok {
		a.mu.Unlock()
		return nil, fmt.Errorf("agent: no pending call for session %q", sessionID)
	}
	if pc.ToolCallID != toolCallID {
		a.mu.Unlock()
		return nil, fmt.Errorf("agent: pending toolCallID %q does not match %q", pc.ToolCallID, toolCallID)
	}
	delete(a.PendingCalls, sessionID)
	a.mu.Unlock()

	messages := pc.Messages
	out := make(chan *Event, 32)

	// We push the chosen decision's effect as a tool result
	// synchronously into a wrapper channel, then start the
	// resumption loop. Because the wrapper approach is hard to
	// reason about, we instead hand the loop a pre-populated
	// "first action" function. The cleanest way to express that
	// is to inline the decision handling here.
	go a.resumeAfterDecision(ctx, sessionID, pc.ToolName, toolCallID, decision, messages, out)
	return out, nil
}

// resumeAfterDecision runs the post-decision logic: apply the
// decision, push the resulting tool result event, then continue
// the streaming loop with updated messages.
//
// Channel ownership: the channel is closed by finishAndClose
// on the way out, before the goroutine returns. The accept /
// accept_for_session / decline paths delegate to runLoop
// (which also calls finishAndClose), so the path that
// delegates MUST return without re-closing.
func (a *Agent) resumeAfterDecision(
	ctx context.Context,
	sessionID, toolName, toolCallID string,
	decision Decision,
	messages []openai.ChatCompletionMessage,
	out chan<- *Event,
) {
	// No `defer a.finishAndClose` here on purpose:
	//
	//   - Accept / AcceptForSession / Decline fall through to
	//     a.runLoop below; runLoop owns the close via its own
	//     defer. If we also deferred close here, we would
	//     double-close the channel and panic (recovered but
	//     emits stream_end twice).
	//   - Cancel / default return before runLoop, so they
	//     must call finishAndClose manually below.
	//
	// The previous "must return without re-closing" comment
	// in this function was therefore impossible to honour —
	// the defer ran unconditionally.

	def, _ := a.Registry.Get(toolName)
	if def.Handler == nil {
		// We did not find the tool. We still need to emit a
		// synthetic result so the LLM can recover, but without
		// running the handler.
		def = ToolDefinition{Kind: KindUnknown}
	}

	switch decision {
	case DecisionAccept:
		resultStr, status, durMs, runErr := a.runTool(def, pendingArgs(messages, toolCallID))
		a.emitData(sessionID, out, EventToolResult, mustJSON(ToolResultData{
			ID:         toolCallID,
			Name:       toolName,
			Result:     resultStr,
			IsError:    runErr != nil,
			Status:     status,
			DurationMs: durMs,
		}))
		messages = append(messages, openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			ToolCallID: toolCallID,
			Content:    resultStr,
		})

	case DecisionAcceptForSession:
		a.SessionGrants.Store(grantKey(sessionID, toolName), struct{}{})
		resultStr, status, durMs, runErr := a.runTool(def, pendingArgs(messages, toolCallID))
		a.emitData(sessionID, out, EventToolResult, mustJSON(ToolResultData{
			ID:         toolCallID,
			Name:       toolName,
			Result:     resultStr,
			IsError:    runErr != nil,
			Status:     status,
			DurationMs: durMs,
		}))
		messages = append(messages, openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			ToolCallID: toolCallID,
			Content:    resultStr,
		})

	case DecisionDecline:
		cancelled := `{"error":"user_rejected"}`
		a.emitData(sessionID, out, EventToolResult, mustJSON(ToolResultData{
			ID:      toolCallID,
			Name:    toolName,
			Result:  cancelled,
			IsError: true,
			Status:  "cancelled",
		}))
		messages = append(messages, openai.ChatCompletionMessage{
			Role:       openai.ChatMessageRoleTool,
			ToolCallID: toolCallID,
			Content:    cancelled,
		})

	case DecisionCancel:
		cancelled := `{"error":"user_cancelled"}`
		a.emitData(sessionID, out, EventToolResult, mustJSON(ToolResultData{
			ID:      toolCallID,
			Name:    toolName,
			Result:  cancelled,
			IsError: true,
			Status:  "cancelled",
		}))
		// No further LLM call; turn ends here. Close the
		// channel manually because runLoop (which would
		// have closed it via defer) is not entered.
		a.finishAndClose(sessionID, out)
		return

	default:
		a.emitError(sessionID, out, "invalid_decision", string(decision))
		a.finishAndClose(sessionID, out)
		return
	}

	// Continue the loop with the post-decision messages.
	// isNewSession=false: ConfirmTool is a continuation of
	// the same session, never a brand-new one. runLoop
	// owns the close via its own defer a.finishAndClose.
	a.runLoop(ctx, sessionID, messages, out, false)
}

func pendingArgs(messages []openai.ChatCompletionMessage, toolCallID string) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role != openai.ChatMessageRoleAssistant {
			continue
		}
		for _, tc := range m.ToolCalls {
			if tc.ID == toolCallID {
				return tc.Function.Arguments
			}
		}
	}
	return ""
}

// isValidDecision covers the four documented values plus the
// empty string (which the HTTP layer treats as a bad request).
func isValidDecision(d Decision) bool {
	switch d {
	case DecisionAccept, DecisionAcceptForSession, DecisionDecline, DecisionCancel:
		return true
	}
	return false
}

// Resume replays cached events from `offset`. If the session is
// still running, the loop polls every 50ms for new events. If
// the session is finished and we have caught up, it emits a
// stream_end and closes.
//
// When the in-memory SessionCache is missing (typically because
// the process restarted), Resume falls back to the durable
// SessionStore: if the JSONL file exists, the cache is rebuilt
// from it and marked finished (no new events will arrive from a
// previous run). A missing store OR a missing JSONL file yields
// the original "session not found" error so callers do not need
// to distinguish in-memory from durable lookups.
func (a *Agent) Resume(
	ctx context.Context,
	sessionID string,
	offset int,
) (<-chan *Event, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("agent: sessionID must not be empty")
	}
	cache, ok := a.getSession(sessionID)
	if !ok {
		// Cache miss — try to recover from durable storage.
		if a.store == nil || !a.store.Exists(sessionID) {
			return nil, fmt.Errorf("agent: session %q not found", sessionID)
		}
		events, err := a.store.Load(sessionID)
		if err != nil {
			return nil, fmt.Errorf("agent: failed to load session %q: %w", sessionID, err)
		}
		if len(events) == 0 {
			return nil, fmt.Errorf("agent: session %q has no recoverable events", sessionID)
		}
		// Reconstruct the in-memory cache from the JSONL file.
		// We mark the cache finished because the process that
		// produced these events is gone — no goroutine is
		// still writing to this session.
		cache = a.ensureSession(sessionID)
		cache.mu.Lock()
		for i := range events {
			ev := events[i] // local copy so &ev is unique per iteration
			cache.Events = append(cache.Events, &ev)
		}
		cache.IsFinished = true
		cache.mu.Unlock()
	}
	out := make(chan *Event, 32)
	go a.runResume(ctx, cache, offset, out)
	return out, nil
}

func (a *Agent) runResume(
	ctx context.Context,
	cache *SessionCache,
	offset int,
	out chan<- *Event,
) {
	defer close(out)
	for {
		evs := cache.snapshot(offset)
		for _, e := range evs {
			select {
			case <-ctx.Done():
				return
			case out <- e:
			}
		}
		offset += len(evs)
		cache.mu.Lock()
		finished := cache.IsFinished
		cache.mu.Unlock()
		if finished {
			return
		}
		// No new events yet; sleep briefly and retry.
		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// cacheAndPersist is the single chokepoint through which every
// event lands in the in-memory SessionCache AND (when a store
// is configured) in the durable JSONL file. It is the only
// legitimate way to grow the cache; direct appendEvent calls
// are no longer used in the agent core.
//
// If the session has never been observed before, ensureSession
// creates the cache so the in-memory write is never silently
// dropped. Persistence failures (disk full, permission denied)
// are intentionally dropped on the floor via "_ =": the live
// turn MUST NOT be blocked by a storage hiccup. SessionStore.Append
// has already logged the warning, so DevLogs / stderr will
// surface the problem for operators.
func (a *Agent) cacheAndPersist(sessionID string, ev *Event) {
	cache := a.ensureSession(sessionID)
	cache.appendEvent(ev)
	if a.store != nil {
		_ = a.store.Append(sessionID, *ev)
	}
}

// emitData builds an Event and pushes it to BOTH the session
// cache and the consumer's channel.
func (a *Agent) emitData(sessionID string, out chan<- *Event, t EventType, data string) {
	ev := &Event{Type: t, Data: data}
	a.cacheAndPersist(sessionID, ev)
	// Blocking send. The channel is buffered (size 32) so
	// short bursts are absorbed; once the buffer fills, the
	// agent goroutine applies backpressure to the producer
	// path rather than silently dropping events. Silent
	// drops would corrupt the persisted event log and make
	// resume-after-restart impossible.
	out <- ev
}

// emitError pushes a stream_end with an error payload.
func (a *Agent) emitError(sessionID string, out chan<- *Event, code, msg string) {
	payload := map[string]string{"code": code, "message": msg}
	ev := &Event{Type: EventStreamEnd, Data: mustJSON(payload)}
	a.cacheAndPersist(sessionID, ev)
	out <- ev
}

// finishSession marks the cache as finished and emits a
// stream_end. Called by both Chat and ConfirmTool on exit.
func (a *Agent) finishSession(sessionID string, out chan<- *Event) {
	if v, ok := a.Sessions.Load(sessionID); ok {
		if cache, ok := v.(*SessionCache); ok {
			cache.mu.Lock()
			cache.IsFinished = true
			cache.mu.Unlock()
		}
	}
	ev := &Event{Type: EventStreamEnd, Data: ""}
	a.cacheAndPersist(sessionID, ev)
	// Best-effort send. The consumer might have disconnected.
	defer func() { _ = recover() }()
	out <- ev
}

// finishAndClose is the canonical exit sequence for a Chat /
// ConfirmTool goroutine: it marks the session finished, pushes
// the terminal stream_end event, and THEN closes the channel
// (so the consumer's streamSSE writer sees the event before
// the close).
//
// Order matters: the send must succeed before close, otherwise
// the stream_end event would be lost. The buffered channel
// (size 32) plus the recover() in finishSession cover the
// "consumer already gone" edge case gracefully.
func (a *Agent) finishAndClose(sessionID string, out chan<- *Event) {
	defer func() {
		// Close in a recover-protected block so a panic from
		// a double-close (caller bug) does not crash the
		// whole process.
		defer func() { _ = recover() }()
		close(out)
	}()
	a.finishSession(sessionID, out)
}

// mustJSON is the non-erroring sibling of json.Marshal; the
// payloads we send are always under our control and never fail
// to marshal.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(b)
}

func cloneMessages(in []openai.ChatCompletionMessage) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, len(in))
	copy(out, in)
	return out
}

// SetCompactor swaps the agent's auto-compaction unit. Passing
// nil disables compaction entirely (the agent will never call
// runCompaction on its own). Tests use this to install a
// compactor with a custom threshold / estimator.
func (a *Agent) SetCompactor(c *Compactor) {
	a.compactor = c
}

// SetSummaryFnForTest injects a deterministic SummaryFn for
// tests so the agent's compaction path can be exercised
// without standing up an httptest server. The function is
// only honoured when a.compactor is non-nil; passing nil
// restores the default (openai-backed) summary fn. The hook
// is stored in a test-only field; production code MUST NOT
// call this method.
func (a *Agent) SetSummaryFnForTest(fn SummaryFunc) {
	a.summaryFnForTest = fn
}

// maybeCompact inspects the running messages slice and, if
// the compactor decides the conversation has grown past the
// configured threshold, asks the LLM for a summary and
// rewrites the slice in place. The function returns the
// summary message (empty when no compaction happened) and
// the number of messages that were replaced; the caller
// truncates the slice to the new length (1 + the trailing
// `keep` messages).
//
// The summary message is inserted at index 0 — the agent's
// chat APIs all see "head of the conversation" as the
// canonical position for system-level context, and OpenAI's
// chat completion API treats the first system message as the
// global directive.
//
// The EventCompaction event is pushed to the consumer
// channel after a successful compaction so the front-end
// can render a divider at the right position.
func (a *Agent) maybeCompact(
	ctx context.Context,
	sessionID string,
	messages []openai.ChatCompletionMessage,
	out chan<- *Event,
) (summaryMsg openai.ChatCompletionMessage, replaced int, err error) {
	if a.compactor == nil {
		return openai.ChatCompletionMessage{}, 0, nil
	}
	if !a.compactor.ShouldCompact(messages) {
		return openai.ChatCompletionMessage{}, 0, nil
	}
	summaryFn := a.buildSummaryFn()
	if summaryFn == nil {
		// No usable LLM client (e.g. the test agent
		// injected a fake llmStream). We degrade to a
		// no-op rather than failing the turn — the user
		// can still get a reply, they just lose the
		// auto-compression benefit.
		return openai.ChatCompletionMessage{}, 0, nil
	}
	summaryMsg, replaced, err = a.compactor.Compact(ctx, messages, summaryFn)
	if err != nil {
		return openai.ChatCompletionMessage{}, 0, err
	}
	if replaced == 0 {
		return openai.ChatCompletionMessage{}, 0, nil
	}
	// Copy the summary into the backing array at index 0,
	// and shift the trailing `keep` messages down by
	// `replaced` positions. We cannot shrink the slice
	// header from inside this function (slices are passed
	// by value); the caller truncates via `messages =
	// messages[:1+keep]` after we return.
	keep := len(messages) - replaced
	if keep < 0 {
		keep = 0
	}
	// Shift the trailing `keep` messages down by `replaced`
	// positions so they start at index 1.
	for i := 0; i < keep; i++ {
		messages[1+i] = messages[replaced+i]
	}
	// Overwrite index 0 with the summary.
	messages[0] = summaryMsg

	// Push the compaction event for the front-end.
	a.emitData(sessionID, out, EventCompaction, mustJSON(CompactionData{
		SummaryText:          summaryMsg.Content,
		ReplacedMessageCount: replaced,
		TriggeredAtMs:        nowMs(),
	}))
	return summaryMsg, replaced, nil
}

// buildSummaryFn returns a SummaryFn backed by the agent's
// real OpenAI client (via openaiStream) when one is available.
// Tests that wire a fake llmStream get a non-streaming summary
// through the same openai.Client that the streaming path uses
// (realLLMFake in http_test.go wraps a real *openai.Client);
// for tests using fakeLLM (no client at all) the function
// returns nil and maybeCompact degrades to a no-op.
func (a *Agent) buildSummaryFn() SummaryFunc {
	// Test hook wins (lets tests inject a deterministic
	// summary fn without standing up an httptest server).
	if a.summaryFnForTest != nil {
		return a.summaryFnForTest
	}
	// Look at the agent's llmStream: the only real
	// production path is openaiStream, which carries an
	// *openai.Client. We accept the type via a type
	// assertion so tests can pass any llmStream and we
	// simply degrade to "no summary" if it's not the
	// real one.
	if os, ok := a.llm.(*openaiStream); ok && os != nil && os.client != nil {
		return NewOpenAISummaryFn(os.client, a.cfg.OpenAIModel)
	}
	return nil
}
