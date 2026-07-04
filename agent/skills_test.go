package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// ----------------------------------------------------------------------
// SKILL.md parsing tests.
//
// These cover the four documented behaviours of
// parseSkillFile / parseSkillBytes:
//   1. A well-formed SKILL.md with frontmatter is parsed end
//      to end (name, description, prompt).
//   2. A SKILL.md with NO frontmatter is still parsed: the
//      entire file becomes the prompt, and Name falls back
//      to the directory name.
//   3. A SKILL.md whose prompt body contains "---" lines is
//      not corrupted by the frontmatter scanner (we stop at
//      the first close).
//   4. An empty / missing root directory is not an error;
//      ScanSkills returns (nil, nil).
// ----------------------------------------------------------------------

// writeSkillFile writes a SKILL.md under <root>/<name>/ with
// the supplied body. The test framework's t.TempDir() provides
// the root, so each test is fully isolated.
func writeSkillFile(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s/SKILL.md: %v", dir, err)
	}
}

// TestParseSkillFile_NormalFrontmatter covers the happy path
// of parseSkillFile: a SKILL.md with name, description, and
// a multi-line prompt body is parsed into a Skill with all
// three fields populated correctly.
func TestParseSkillFile_NormalFrontmatter(t *testing.T) {
	root := t.TempDir()
	const body = `---
name: video-encrypt
description: Encrypt a video file using the encv-go plugin system.
---

# Video Encrypt

Use the video_encrypt tool with the supplied password.
Do not invent a different container version; default to
the value of agent_settings.default_container_version.
`
	writeSkillFile(t, root, "video-encrypt", body)

	got, err := parseSkillFile(filepath.Join(root, "video-encrypt", "SKILL.md"))
	if err != nil {
		t.Fatalf("parseSkillFile: %v", err)
	}
	if got.Name != "video-encrypt" {
		t.Errorf("Name: got %q want %q", got.Name, "video-encrypt")
	}
	if got.Description != "Encrypt a video file using the encv-go plugin system." {
		t.Errorf("Description: got %q", got.Description)
	}
	// Prompt must be trimmed of leading and trailing blank
	// lines but preserve the internal formatting.
	if !strings.HasPrefix(got.Prompt, "# Video Encrypt") {
		t.Errorf("Prompt should start with the heading, got %q", got.Prompt)
	}
	if !strings.Contains(got.Prompt, "default_container_version") {
		t.Errorf("Prompt should preserve the body, got %q", got.Prompt)
	}
}

// TestParseSkillFile_NoFrontmatterFallsBackToDirName covers
// the second documented case: a SKILL.md that omits the
// "---" frontmatter block entirely. The entire file becomes
// the prompt, and Name falls back to the directory name
// ("video-decrypt" in this test).
func TestParseSkillFile_NoFrontmatterFallsBackToDirName(t *testing.T) {
	root := t.TempDir()
	const body = `# Video Decrypt

Use the video_decrypt tool. The container version is
detected from the file extension.
`
	writeSkillFile(t, root, "video-decrypt", body)

	got, err := parseSkillFile(filepath.Join(root, "video-decrypt", "SKILL.md"))
	if err != nil {
		t.Fatalf("parseSkillFile: %v", err)
	}
	if got.Name != "video-decrypt" {
		t.Errorf("Name fallback: got %q want %q", got.Name, "video-decrypt")
	}
	if got.Description != "" {
		t.Errorf("Description should be empty when no frontmatter, got %q", got.Description)
	}
	if !strings.HasPrefix(got.Prompt, "# Video Decrypt") {
		t.Errorf("Prompt should be the whole file, got %q", got.Prompt)
	}
	if !strings.Contains(got.Prompt, "video_decrypt tool") {
		t.Errorf("Prompt should contain the body content, got %q", got.Prompt)
	}
}

// TestParseSkillFile_BodyContainsDelimiterIsNotConfused
// covers the third documented case: a SKILL.md whose prompt
// body contains a "---" line. The frontmatter scanner stops
// at the FIRST close delimiter, so the body delimiter is
// preserved verbatim as part of the prompt.
func TestParseSkillFile_BodyContainsDelimiterIsNotConfused(t *testing.T) {
	root := t.TempDir()
	const body = `---
name: advanced
description: A skill whose body has its own delimiter.
---

# Advanced

Use the following YAML snippet in your reply:

---
key: value
nested:
  - a
  - b
---

End of skill.
`
	writeSkillFile(t, root, "advanced", body)

	got, err := parseSkillFile(filepath.Join(root, "advanced", "SKILL.md"))
	if err != nil {
		t.Fatalf("parseSkillFile: %v", err)
	}
	if got.Name != "advanced" {
		t.Errorf("Name: got %q want %q", got.Name, "advanced")
	}
	// The body delimiter MUST be preserved.
	if !strings.Contains(got.Prompt, "key: value") {
		t.Errorf("Prompt should preserve the body delimiter, got %q", got.Prompt)
	}
	if !strings.Contains(got.Prompt, "End of skill.") {
		t.Errorf("Prompt should preserve the trailing text, got %q", got.Prompt)
	}
	// The prompt must NOT include the frontmatter block.
	if strings.Contains(got.Prompt, "A skill whose body") {
		t.Errorf("Prompt leaked frontmatter: %q", got.Prompt)
	}
}

// TestParseSkillFile_UnclosedFrontmatterIsTreatedAsBody is
// the defensive case: the file starts with "---" but never
// closes. The scanner does NOT error — it discards the
// half-written frontmatter and uses the whole file as the
// prompt body. The fallback name is still applied.
func TestParseSkillFile_UnclosedFrontmatterIsTreatedAsBody(t *testing.T) {
	root := t.TempDir()
	const body = `---
name: half-written
description: this frontmatter is never closed

# Body

Use the half_baked tool.
`
	writeSkillFile(t, root, "half-baked", body)

	got, err := parseSkillFile(filepath.Join(root, "half-baked", "SKILL.md"))
	if err != nil {
		t.Fatalf("parseSkillFile: %v", err)
	}
	if got.Name != "half-baked" {
		t.Errorf("Name fallback: got %q want %q", got.Name, "half-baked")
	}
	if !strings.Contains(got.Prompt, "half_baked tool") {
		t.Errorf("Prompt should fall back to the whole file, got %q", got.Prompt)
	}
}

// TestScanSkills_EmptyAndMissingRoot covers the fourth
// documented case: a missing or empty skills root is not an
// error. ScanSkills returns (nil, nil).
func TestScanSkills_EmptyAndMissingRoot(t *testing.T) {
	// Missing root: returns nil error, nil slice.
	t.Run("missing", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		got, err := ScanSkills(missing)
		if err != nil {
			t.Errorf("ScanSkills(missing): unexpected error %v", err)
		}
		if len(got) != 0 {
			t.Errorf("ScanSkills(missing): got %d skills, want 0", len(got))
		}
	})
	// Empty root: a directory exists but contains nothing.
	t.Run("empty", func(t *testing.T) {
		empty := t.TempDir()
		got, err := ScanSkills(empty)
		if err != nil {
			t.Errorf("ScanSkills(empty): unexpected error %v", err)
		}
		if len(got) != 0 {
			t.Errorf("ScanSkills(empty): got %d skills, want 0", len(got))
		}
	})
	// Empty string: a degenerate input that callers might
	// pass when SkillsDir is unset; must not panic.
	t.Run("empty-string", func(t *testing.T) {
		got, err := ScanSkills("")
		if err != nil {
			t.Errorf("ScanSkills(\"\"): unexpected error %v", err)
		}
		if len(got) != 0 {
			t.Errorf("ScanSkills(\"\"): got %d skills, want 0", len(got))
		}
	})
}

// TestScanSkills_MultipleSortedByName covers the canonical
// happy path: scan a directory with three SKILL.md files and
// confirm the result is sorted by name (not by directory
// read order, which is filesystem-dependent).
func TestScanSkills_MultipleSortedByName(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, root, "zeta", `---
name: zeta
description: z
---
z body
`)
	writeSkillFile(t, root, "alpha", `---
name: alpha
description: a
---
a body
`)
	writeSkillFile(t, root, "mu", `---
name: mu
description: m
---
m body
`)

	got, err := ScanSkills(root)
	if err != nil {
		t.Fatalf("ScanSkills: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d skills, want 3", len(got))
	}
	wantNames := []string{"alpha", "mu", "zeta"}
	for i, w := range wantNames {
		if got[i].Name != w {
			t.Errorf("skills[%d].Name: got %q want %q", i, got[i].Name, w)
		}
	}
}

// TestScanSkills_SkipsDirectoriesWithoutSkillMD covers the
// "stray folder" case: a directory under the skills root
// that does not contain a SKILL.md file is silently skipped,
// not surfaced as an error.
func TestScanSkills_SkipsDirectoriesWithoutSkillMD(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, root, "real", `---
name: real
description: r
---
real body
`)
	// Add a directory with no SKILL.md inside.
	if err := os.MkdirAll(filepath.Join(root, "no-manifest"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Add a stray file at the root (not a directory); the
	// scanner must also ignore it.
	if err := os.WriteFile(filepath.Join(root, "stray.md"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ScanSkills(root)
	if err != nil {
		t.Fatalf("ScanSkills: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d skills, want 1 (the stray folders must be skipped)", len(got))
	}
	if got[0].Name != "real" {
		t.Errorf("Name: got %q want %q", got[0].Name, "real")
	}
}

// ----------------------------------------------------------------------
// Frontmatter edge cases.
//
// These are NOT in the spec-mandated list but are the
// obvious "what does the parser do with quirky input?"
// follow-ups. They live in the same file because they
// exercise parseSkillBytes directly.
// ----------------------------------------------------------------------

// TestExtractFrontmatterField_HandlesQuotingAndComments
// covers the supported value shapes: bare value, single-quoted,
// double-quoted, and trailing comment.
func TestExtractFrontmatterField_HandlesQuotingAndComments(t *testing.T) {
	cases := []struct {
		frontmatter string
		key         string
		want        string
	}{
		{"name: bare", "name", "bare"},
		{"name: 'with spaces'", "name", "with spaces"},
		{`name: "with: colon"`, "name", "with: colon"},
		{"name: bare # trailing comment", "name", "bare"},
		{"name: 'quoted' # comment", "name", "quoted"},
		{"description: missing", "name", ""},
		{"missing: name", "name", ""},
	}
	for i, c := range cases {
		got := extractFrontmatterField(c.frontmatter, c.key)
		if got != c.want {
			t.Errorf("case %d: extractFrontmatterField(%q, %q) = %q, want %q", i, c.frontmatter, c.key, got, c.want)
		}
	}
}

// TestComposeSkillPrompt_SkipsUnknownAndOrdersByLoadedList
// is the helper used by the default session_start hook.
// Unknown names are dropped, and the output is wrapped in
// a header so the LLM can see which skills are active.
func TestComposeSkillPrompt_SkipsUnknownAndOrdersByLoadedList(t *testing.T) {
	skills := []Skill{
		{Name: "alpha", Description: "a", Prompt: "alpha body"},
		{Name: "beta", Description: "b", Prompt: "beta body"},
	}
	got := composeSkillPrompt(skills, []string{"alpha", "ghost", "beta"})
	if !strings.Contains(got, "### Skill: alpha") {
		t.Errorf("missing alpha heading: %q", got)
	}
	if !strings.Contains(got, "### Skill: beta") {
		t.Errorf("missing beta heading: %q", got)
	}
	if strings.Contains(got, "ghost") {
		t.Errorf("unknown skill leaked into prompt: %q", got)
	}
	if !strings.Contains(got, "alpha body") || !strings.Contains(got, "beta body") {
		t.Errorf("bodies missing: %q", got)
	}
	// Empty input → empty output.
	if got := composeSkillPrompt(skills, nil); got != "" {
		t.Errorf("nil names should produce empty output, got %q", got)
	}
	if got := composeSkillPrompt(nil, []string{"alpha"}); got != "" {
		t.Errorf("no skills loaded should produce empty output, got %q", got)
	}
}

// ----------------------------------------------------------------------
// Agent integration: NewAgent scans the configured directory,
// the default session_start hook injects the selected skills
// into the per-session system prompt, and chatRequest prepends
// a system message to the LLM call.
// ----------------------------------------------------------------------

// TestNewAgent_LoadsSkillsFromConfiguredDir confirms that
// NewAgent populates Agent.Skills from cfg.SkillsDir.
func TestNewAgent_LoadsSkillsFromConfiguredDir(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, root, "video-encrypt", `---
name: video-encrypt
description: Encrypt a video.
---
video body
`)
	writeSkillFile(t, root, "pdf-decrypt", `---
name: pdf-decrypt
description: Decrypt a PDF.
---
pdf body
`)

	a := NewAgent(AgentConfig{SkillsDir: root}, NewRegistry())
	if len(a.Skills) != 2 {
		t.Fatalf("expected 2 skills loaded, got %d", len(a.Skills))
	}
	// Skills are sorted by name.
	if a.Skills[0].Name != "pdf-decrypt" || a.Skills[1].Name != "video-encrypt" {
		t.Errorf("unexpected order: %+v", a.Skills)
	}
}

// TestNewAgent_NoSkillsDirIsNotAnError locks the
// "missing skills directory is benign" contract: an empty
// SkillsDir pointing at a non-existent path must NOT fail
// the constructor; the agent simply runs with zero skills.
func TestNewAgent_NoSkillsDirIsNotAnError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-skills-here")
	a := NewAgent(AgentConfig{SkillsDir: missing}, NewRegistry())
	if len(a.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(a.Skills))
	}
}

// TestSessionStartHook_StoresMergedSystemPrompt is the
// integration test for the default session_start hook.
// The flow: HTTP layer calls SetSelectedSkills, the chat
// goroutine fires HookSessionStart with SelectedSkills
// populated, the default hook stores the joined prompt
// under the session id, and SessionSystemPrompt returns
// the merged value.
func TestSessionStartHook_StoresMergedSystemPrompt(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, root, "video-encrypt", `---
name: video-encrypt
description: Encrypt a video.
---
use video_encrypt
`)
	writeSkillFile(t, root, "pdf-decrypt", `---
name: pdf-decrypt
description: Decrypt a PDF.
---
use pdf_decrypt
`)

	a := NewAgent(AgentConfig{
		SkillsDir:   root,
		SystemPrompt: "base policy",
	}, NewRegistry())
	if len(a.Skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(a.Skills))
	}

	// 1. Simulate the HTTP layer: write the selection.
	a.SetSelectedSkills("sess-1", []string{"video-encrypt"})

	// 2. Fire the session_start hook directly (we are
	//    skipping the LLM path because the rest of the
	//    flow is already covered by the hooks tests).
	hc := &HookContext{
		Event:          HookSessionStart,
		SessionID:      "sess-1",
		SelectedSkills: a.selectedSkillsFor("sess-1"),
	}
	if err := defaultSkillInjectionHook(a)(context.Background(), hc); err != nil {
		t.Fatalf("defaultSkillInjectionHook: %v", err)
	}

	// 3. Verify the merged prompt is stored and that
	//    SessionSystemPrompt returns it.
	got := a.SessionSystemPrompt("sess-1")
	if !strings.Contains(got, "base policy") {
		t.Errorf("merged prompt should include base, got %q", got)
	}
	if !strings.Contains(got, "use video_encrypt") {
		t.Errorf("merged prompt should include the skill body, got %q", got)
	}
	// A different session sees only the base policy.
	if sp := a.SessionSystemPrompt("sess-2"); sp != "base policy" {
		t.Errorf("unrelated session: got %q want %q", sp, "base policy")
	}

	// 4. ClearSessionSystemPrompt reverts to the base.
	a.ClearSessionSystemPrompt("sess-1")
	if sp := a.SessionSystemPrompt("sess-1"); sp != "base policy" {
		t.Errorf("after clear: got %q want %q", sp, "base policy")
	}
}

// TestInjectSystemPrompt_PrependsWhenSet locks the wire-side
// behaviour: when a session has a per-session override, the
// chat loop prepends a system message to the messages
// slice. When no override is set, the slice is returned
// unchanged (no extra allocation, no extra message).
func TestInjectSystemPrompt_PrependsWhenSet(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	msgs := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: "hi"},
	}

	// No override → unchanged (and same slice).
	out := a.injectSystemPrompt("sess-nil", msgs)
	if len(out) != 1 || out[0].Role != openai.ChatMessageRoleUser {
		t.Errorf("no override should leave the slice unchanged, got %+v", out)
	}

	// Override set → a system message is prepended.
	a.systemPromptBySession.Store("sess-1", "you are a video encryption expert")
	out = a.injectSystemPrompt("sess-1", msgs)
	if len(out) != 2 {
		t.Fatalf("override should prepend one message, got %d messages", len(out))
	}
	if out[0].Role != openai.ChatMessageRoleSystem {
		t.Errorf("prepended message role: got %q want system", out[0].Role)
	}
	if out[0].Content != "you are a video encryption expert" {
		t.Errorf("prepended message content: got %q", out[0].Content)
	}
	// Original messages must be preserved in order.
	if out[1].Role != openai.ChatMessageRoleUser || out[1].Content != "hi" {
		t.Errorf("original messages mutated: %+v", out[1])
	}
}

// TestSetSelectedSkills_StoresAndClears locks the lifecycle
// of the per-session selection map.
func TestSetSelectedSkills_StoresAndClears(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())

	// Empty sessionID is a no-op (defensive).
	a.SetSelectedSkills("", []string{"x"})
	if got := a.selectedSkillsFor(""); got != nil {
		t.Errorf("empty sessionID should not store, got %+v", got)
	}

	a.SetSelectedSkills("s1", []string{"a", "b"})
	if got := a.selectedSkillsFor("s1"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("Store: got %v want [a b]", got)
	}

	// Empty slice clears the entry.
	a.SetSelectedSkills("s1", nil)
	if got := a.selectedSkillsFor("s1"); got != nil {
		t.Errorf("nil names should clear, got %v", got)
	}

	// Defensive copy: mutating the caller's slice must NOT
	// affect subsequent reads.
	original := []string{"a", "b"}
	a.SetSelectedSkills("s2", original)
	original[0] = "MUTATED"
	if got := a.selectedSkillsFor("s2"); got[0] != "a" {
		t.Errorf("SetSelectedSkills must copy; got %v", got)
	}
}

// TestSkillByName_DefensiveAndOrdered ensures the helper
// used by the HTTP layer to render the slash menu keeps a
// stable (alphabetical) order regardless of the order of
// the input name list.
func TestSkillByName_DefensiveAndOrdered(t *testing.T) {
	a := &Agent{
		Skills: []Skill{
			{Name: "alpha", Description: "a", Prompt: "x"},
			{Name: "mu", Description: "m", Prompt: "x"},
			{Name: "zeta", Description: "z", Prompt: "x"},
		},
	}
	got := a.SkillByName([]string{"zeta", "alpha", "ghost"})
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Errorf("SkillByName: got %v want [alpha zeta]", got)
	}
}

// TestDefaultSkillInjectionHook_NoSkillsNoOp locks the
// "front-end sent no skills" path: the hook fires but
// stores nothing.
func TestDefaultSkillInjectionHook_NoSkillsNoOp(t *testing.T) {
	a := NewAgent(AgentConfig{}, NewRegistry())
	hook := defaultSkillInjectionHook(a)
	if err := hook(context.Background(), &HookContext{
		Event:     HookSessionStart,
		SessionID: "sess-x",
	}); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if sp := a.SessionSystemPrompt("sess-x"); sp != "" {
		t.Errorf("no skills should leave the session empty, got %q", sp)
	}
}

// TestDefaultSkillInjectionHook_UnknownSkillsLogsAndNoOps
// is the diagnostic path: the front-end asks for skills
// that do not exist. The hook must NOT crash; it logs to
// stderr and leaves the session unchanged.
func TestDefaultSkillInjectionHook_UnknownSkillsLogsAndNoOps(t *testing.T) {
	a := NewAgent(AgentConfig{SkillsDir: t.TempDir()}, NewRegistry())
	hook := defaultSkillInjectionHook(a)
	if err := hook(context.Background(), &HookContext{
		Event:          HookSessionStart,
		SessionID:      "sess-x",
		SelectedSkills: []string{"nope"},
	}); err != nil {
		t.Fatalf("hook: %v", err)
	}
	if sp := a.SessionSystemPrompt("sess-x"); sp != "" {
		t.Errorf("unknown skills should not produce a prompt, got %q", sp)
	}
}

// TestAgent_SkillScanIsConcurrentSafe runs a small
// race-detector check: many goroutines call ScanSkills
// and SetSelectedSkills against the same agent. The
// race detector (when run with -race) will catch any
// unprotected access.
func TestAgent_SkillScanIsConcurrentSafe(t *testing.T) {
	root := t.TempDir()
	writeSkillFile(t, root, "s1", "---\nname: s1\ndescription: d\n---\nbody\n")
	a := NewAgent(AgentConfig{SkillsDir: root}, NewRegistry())

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sid := "sess-" + string(rune('a'+i%26))
			a.SetSelectedSkills(sid, []string{"s1"})
			_ = a.selectedSkillsFor(sid)
			_ = a.SkillByName([]string{"s1"})
		}(i)
	}
	wg.Wait()
}
