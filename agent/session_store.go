package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// SessionStore persists agent session events to disk as JSONL
// (one JSON object per line) so they can be replayed after a
// process restart. The on-disk layout is:
//
//	<root>/{sessionId}.jsonl
//
// where <root> defaults to ~/.encv/agent/sessions/ when empty.
//
// Concurrency: a single mutex serialises both Append (write) and
// the directory bootstrap. Reads (Load, Exists) are not guarded;
// callers that need read-after-write consistency (Agent.Resume)
// achieve it indirectly because the JSONL file is only appended
// to, never truncated, and the in-memory cache is authoritative
// while the process is alive.
//
// Failure semantics: Append logs a warning AND returns the
// error. Agent integration in agent.go intentionally ignores
// the return value so a disk-full or permission-denied failure
// never blocks the live business path — the SessionCache is
// always the source of truth during a turn.
type SessionStore struct {
	root string
	mu   sync.Mutex
}

// NewSessionStore constructs a SessionStore rooted at the given
// directory. An empty root resolves to "<home>/.encv/agent/
// sessions" where <home> is os.UserHomeDir(); on the rare
// platforms where UserHomeDir errors, we fall back to a
// relative ".encv/agent/sessions" path so the program can still
// run (subsequent Appends will surface the real error).
//
// The directory is created lazily on the first successful
// Append so read-only processes do not pollute the user's
// home.
func NewSessionStore(root string) *SessionStore {
	if strings.TrimSpace(root) == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			root = filepath.Join(home, ".encv", "agent", "sessions")
		} else {
			root = filepath.Join(".encv", "agent", "sessions")
		}
	}
	return &SessionStore{root: root}
}

// Root returns the resolved on-disk directory. Useful for tests
// and for diagnostic endpoints that want to print the path.
func (s *SessionStore) Root() string {
	return s.root
}

// pathFor returns the JSONL file path for a given sessionID.
// Sanitises "../" segments defensively so a malicious session ID
// cannot escape the root directory.
func (s *SessionStore) pathFor(sessionID string) string {
	return filepath.Join(s.root, filepath.Base(sessionID)+".jsonl")
}

// Append appends a single event to the session's JSONL file. The
// file is created if it does not exist. Append is safe to call
// concurrently for distinct session IDs and for the same ID
// (the mutex is global to keep the implementation simple — the
// critical section is a single Write of a small JSON line).
//
// On a write failure (mkdir, marshal, open, write) Append logs a
// warning via log.Printf so the failure is visible in DevLogs /
// stderr, AND returns the error so callers that DO want to react
// can. The Agent integration in agent.go intentionally drops the
// return value to keep the business path non-blocking.
func (s *SessionStore) Append(sessionID string, evt Event) error {
	if sessionID == "" {
		return fmt.Errorf("session_store: sessionID must not be empty")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.root, 0o755); err != nil {
		log.Printf("[session_store] mkdir %s failed: %v", s.root, err)
		return fmt.Errorf("mkdir: %w", err)
	}

	line, err := json.Marshal(evt)
	if err != nil {
		log.Printf("[session_store] marshal event for %s failed: %v", sessionID, err)
		return fmt.Errorf("marshal: %w", err)
	}

	path := s.pathFor(sessionID)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("[session_store] open %s failed: %v", path, err)
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		log.Printf("[session_store] write %s failed: %v", path, err)
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// Load reads the entire JSONL file for a session and returns the
// decoded events. Corrupt lines (malformed JSON, partial writes
// from a crash) are logged at warning level and skipped; an
// empty result is returned if the file exists but contains
// nothing valid.
//
// Load returns an error only when the file cannot be opened or
// read at all (e.g. permission denied). A missing file yields an
// error from os.ReadFile — callers that want to distinguish
// "no such session" from "broken session" should use Exists
// first.
func (s *SessionStore) Load(sessionID string) ([]Event, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_store: sessionID must not be empty")
	}
	data, err := os.ReadFile(s.pathFor(sessionID))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}
	out := make([]Event, 0)
	for _, raw := range splitJSONL(data) {
		// Strip a trailing CR (Windows line endings) so the
		// JSON decoder does not reject otherwise-valid lines.
		raw = trimTrailingCR(raw)
		if isAllWhitespace(raw) {
			continue
		}
		var evt Event
		if err := json.Unmarshal(raw, &evt); err != nil {
			log.Printf("[session_store] corrupt line in %s.jsonl skipped: %v", sessionID, err)
			continue
		}
		out = append(out, evt)
	}
	return out, nil
}

// trimTrailingCR removes a single trailing '\r' from b (without
// allocating a new slice when the byte is absent). This keeps
// Windows-style CRLF JSONL files decodable.
func trimTrailingCR(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\r' {
		return b[:n-1]
	}
	return b
}

// isAllWhitespace reports whether b is empty or contains only
// ASCII whitespace. We avoid strings.TrimSpace so the bytes are
// not re-allocated on every call.
func isAllWhitespace(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			return false
		}
	}
	return true
}

// Exists returns true if a JSONL file exists for the given
// session ID, false otherwise. An empty sessionID returns false
// without touching the disk.
func (s *SessionStore) Exists(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	info, err := os.Stat(s.pathFor(sessionID))
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// splitJSONL splits a JSONL byte slice into lines. Empty trailing
// lines from a final '\n' are included as zero-length slices and
// filtered out by the caller; this keeps the implementation
// branch-free. The returned slices share storage with the input.
func splitJSONL(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	var out [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			out = append(out, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}
