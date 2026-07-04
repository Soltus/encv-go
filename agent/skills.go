package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Skill is a single skill loaded from <root>/<name>/SKILL.md.
// The expected on-disk shape is:
//
//	---
//	name: <name>
//	description: <description>
//	---
//	<prompt body>
//
// The Prompt field is the markdown body that follows the YAML
// frontmatter and is intended to be appended to the agent's
// system prompt at session_start time when the user has
// selected this skill via HookContext.SelectedSkills.
//
// Name falls back to the directory name when the frontmatter
// does not declare one, so a single-line SKILL.md that omits
// the frontmatter still parses into a usable Skill.
type Skill struct {
	Name        string
	Description string
	Prompt      string
}

// defaultSkillsDir returns the canonical on-disk location
// for skill manifests. The path is "$HOME/.encv/skills"
// when $HOME is set, else ".encv/skills" relative to the
// working directory. The directory does not need to exist;
// ScanSkills treats a missing root as "no skills loaded".
func defaultSkillsDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".encv", "skills")
	}
	return filepath.Join(".encv", "skills")
}

// ScanSkills walks <root>/<skill>/SKILL.md and returns the
// successfully-parsed skills, sorted by name. A missing root
// or a non-directory root returns (nil, nil); an empty root
// also returns an empty slice with a nil error.
//
// Parse errors on individual files are surfaced via the
// returned error: the first parse failure short-circuits and
// returns the partial result accumulated so far. Callers
// that prefer best-effort behaviour can inspect len(result)
// and decide whether to discard the error.
//
// The directory structure follows the Claude Code / pi-repo
// convention so the on-disk layout is portable to the wider
// agent ecosystem:
//
//	~/.encv/skills/
//	    video-encrypt/
//	        SKILL.md
//	    pdf-decrypt/
//	        SKILL.md
func ScanSkills(root string) ([]Skill, error) {
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("scan skills: stat %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("scan skills: %s is not a directory", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("scan skills: read %s: %w", root, err)
	}
	var out []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillMD := filepath.Join(root, entry.Name(), "SKILL.md")
		if _, statErr := os.Stat(skillMD); statErr != nil {
			// Skip directories that do not contain a SKILL.md
			// manifest — a stray folder is not an error.
			continue
		}
		s, err := parseSkillFile(skillMD)
		if err != nil {
			return out, fmt.Errorf("scan skills: %s: %w", skillMD, err)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// parseSkillFile reads a single SKILL.md from disk and
// returns the parsed Skill. The function derives the
// fallback name from the parent directory (e.g. for
// "<root>/video-encrypt/SKILL.md" the fallback is
// "video-encrypt") so callers do not have to thread the
// directory name separately.
//
// The expected file shape is:
//
//	---
//	name: <name>
//	description: <description>
//	---
//	<prompt body>
//
// Behaviour summary (each case is independently testable):
//
//   - Missing or unreadable file → error wrapping os.ReadFile.
//   - File that does NOT start with a "---" line → the entire
//     file becomes the Prompt; Name falls back to the
//     directory name; Description is empty.
//   - File that starts with "---" but has no closing "---"
//     → the entire file becomes the Prompt; frontmatter is
//     discarded (a half-written manifest is not an error).
//   - File with both delimiters → the frontmatter is parsed
//     for `name:` / `description:` (one-line each), and the
//     prompt is everything after the closing "---". A
//     `---` line embedded in the prompt body does NOT break
//     parsing because the scanner stops at the first close.
func parseSkillFile(path string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, fmt.Errorf("read %s: %w", path, err)
	}
	fallback := filepath.Base(filepath.Dir(path))
	return parseSkillBytes(data, fallback), nil
}

// parseSkillBytes is the testable form of parseSkillFile. It
// accepts the raw file contents and the directory-derived
// fallback name. The function is intentionally side-effect
// free: callers can fuzz it cheaply.
func parseSkillBytes(data []byte, fallbackName string) Skill {
	s := Skill{Name: fallbackName}
	body := string(data)
	// Normalise line endings once so the delimiter scan is
	// CRLF-safe. The original body is kept untouched; only
	// the frontmatter scan operates on the normalised copy.
	normalized := strings.ReplaceAll(body, "\r\n", "\n")

	if !startsWithFrontmatterDelimiter(normalized) {
		s.Prompt = strings.TrimSpace(body)
		return s
	}

	lines := strings.Split(normalized, "\n")
	closeIdx := -1
	// lines[0] is the opening "---"; scan from lines[1] for
	// the matching close. We pick the FIRST close so a body
	// that happens to contain "---" further down is preserved
	// verbatim as part of the prompt.
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx < 0 {
		// No closing delimiter — treat the entire file as
		// prompt body. Name falls back to the directory
		// name, which parseSkillFile already supplied.
		s.Prompt = strings.TrimSpace(body)
		return s
	}

	frontmatter := strings.Join(lines[1:closeIdx], "\n")
	if name := extractFrontmatterField(frontmatter, "name"); name != "" {
		s.Name = name
	}
	s.Description = extractFrontmatterField(frontmatter, "description")

	rest := strings.Join(lines[closeIdx+1:], "\n")
	s.Prompt = strings.TrimSpace(rest)
	return s
}

// startsWithFrontmatterDelimiter reports whether s begins
// with "---" on its own line. The two accepted shapes are
// "---" followed by "\n" and the empty-body case "---".
func startsWithFrontmatterDelimiter(s string) bool {
	if s == "---" {
		return true
	}
	return strings.HasPrefix(s, "---\n")
}

// frontmatterFieldRe matches a single "key: value" line in
// the frontmatter. The key must start with a letter or
// underscore and may contain letters, digits, underscores,
// and hyphens (the typical slug set). The value runs to the
// end of the line; trim, quote-stripping, and comment
// stripping happen in extractFrontmatterField.
var frontmatterFieldRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_-]*)\s*:\s*(.*)$`)

// extractFrontmatterField returns the value of the named
// key in a frontmatter block, or "" if the key is missing
// or empty. The parser is intentionally minimal: a single
// physical line per field, with optional surrounding quotes
// and a trailing "# comment" stripped. The skill registry
// only requires name and description, both of which are
// single-line ASCII strings in real SKILL.md files; we
// therefore avoid pulling in a YAML library.
func extractFrontmatterField(frontmatter, key string) string {
	for _, line := range strings.Split(frontmatter, "\n") {
		m := frontmatterFieldRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if m[1] != key {
			continue
		}
		v := strings.TrimSpace(m[2])
		// Strip a trailing " # comment" that is preceded by
		// whitespace and not inside a quoted string.
		if idx := indexUnquotedHash(v); idx >= 0 {
			v = strings.TrimSpace(v[:idx])
		}
		// Strip a single pair of matching surrounding quotes
		// (double or single). Embedded quotes inside the
		// value are left as-is.
		if len(v) >= 2 {
			first, last := v[0], v[len(v)-1]
			if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
				v = v[1 : len(v)-1]
			}
		}
		return v
	}
	return ""
}

// indexUnquotedHash returns the byte index of the first
// unquoted " # ..." comment marker in s, or -1 if none is
// present. Quote characters toggle a "we are inside a
// string" state so a hash inside a quoted value is not
// treated as a comment delimiter.
func indexUnquotedHash(s string) int {
	inDouble, inSingle := false, false
	for i, r := range s {
		switch r {
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '#':
			if !inDouble && !inSingle && i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
				return i
			}
		}
	}
	return -1
}

// composeSkillPrompt joins the prompt bodies of the named
// skills into a single string suitable for appending to the
// agent's system prompt. Names that do not match any loaded
// skill are silently skipped. An empty input or zero matches
// yields an empty string.
//
// The output is wrapped in a short header so the LLM can see
// which skills are active for the session. Each skill's
// prompt is fenced under "### Skill: <name>" and skills are
// separated by a horizontal rule.
func composeSkillPrompt(skills []Skill, names []string) string {
	if len(names) == 0 {
		return ""
	}
	byName := make(map[string]Skill, len(skills))
	for _, s := range skills {
		byName[s.Name] = s
	}
	var parts []string
	for _, n := range names {
		s, ok := byName[n]
		if !ok {
			continue
		}
		parts = append(parts, "### Skill: "+s.Name+"\n\n"+s.Prompt)
	}
	if len(parts) == 0 {
		return ""
	}
	return "## Active Skills\n\n" + strings.Join(parts, "\n\n---\n\n")
}

// SkillByName returns a defensive copy of the agent's skill
// list filtered to the supplied names. Names that do not
// match any loaded skill are silently skipped. The result
// preserves the agent's sort order (by name) regardless of
// the order of `names`. Returns an empty slice when no
// requested skill is loaded.
func (a *Agent) SkillByName(names []string) []Skill {
	if len(names) == 0 || len(a.Skills) == 0 {
		return nil
	}
	byName := make(map[string]Skill, len(a.Skills))
	for _, s := range a.Skills {
		byName[s.Name] = s
	}
	out := make([]Skill, 0, len(names))
	for _, s := range a.Skills {
		for _, n := range names {
			if s.Name == n {
				out = append(out, s)
				break
			}
		}
		_ = byName // keep byName referenced for future map lookups
	}
	return out
}
