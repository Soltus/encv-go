package agent

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestSessionStore_AppendAndLoadRoundTrip covers the
// canonical happy path: write N events, read them back, verify
// content + order.
func TestSessionStore_AppendAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	in := []Event{
		{Type: EventTextDelta, Data: `{"content":"hello"}`},
		{Type: EventTextDelta, Data: `{"content":" "}`},
		{Type: EventToolCall, Data: `{"id":"c1","name":"echo","args":"{}"}`},
		{Type: EventToolResult, Data: `{"id":"c1","name":"echo","result":"ok"}`},
		{Type: EventStreamEnd, Data: ""},
	}
	for _, e := range in {
		if err := store.Append("s1", e); err != nil {
			t.Fatalf("Append(%v): %v", e.Type, err)
		}
	}

	got, err := store.Load("s1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != len(in) {
		t.Fatalf("event count: got %d want %d", len(got), len(in))
	}
	for i := range in {
		if got[i].Type != in[i].Type {
			t.Errorf("event %d type: got %q want %q", i, got[i].Type, in[i].Type)
		}
		if got[i].Data != in[i].Data {
			t.Errorf("event %d data: got %q want %q", i, got[i].Data, in[i].Data)
		}
	}
}

// TestSessionStore_PathGeneration locks the on-disk layout
// contract: <root>/<basename(id)>.jsonl. The test uses
// t.TempDir() so each run is isolated.
func TestSessionStore_PathGeneration(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	if store.Root() != dir {
		t.Errorf("Root(): got %q want %q", store.Root(), dir)
	}

	// Single ASCII id — typical case.
	if err := store.Append("abc", Event{Type: EventTextDelta, Data: "{}"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "abc.jsonl")); err != nil {
		t.Errorf("expected file at %s/abc.jsonl: %v", dir, err)
	}

	// Different id → different file.
	if err := store.Append("xyz", Event{Type: EventTextDelta, Data: "{}"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "xyz.jsonl")); err != nil {
		t.Errorf("expected file at %s/xyz.jsonl: %v", dir, err)
	}

	// Sanitised id — filepath.Base strips any "../" segments so
	// a malicious id cannot escape the root.
	if err := store.Append("../../etc/passwd", Event{Type: EventTextDelta, Data: "{}"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "passwd.jsonl")); err != nil {
		t.Errorf("expected sanitised file passwd.jsonl: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "..", "etc", "passwd.jsonl")); err == nil {
		t.Errorf("path traversal should NOT have escaped the root")
	}
}

// TestSessionStore_Exists is the boolean precheck used by
// Agent.Resume to decide between "session not found" and
// "broken session".
func TestSessionStore_Exists(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	if store.Exists("nope") {
		t.Errorf("Exists() should be false for a missing session")
	}
	if store.Exists("") {
		t.Errorf("Exists(\"\") should be false without touching the disk")
	}

	if err := store.Append("present", Event{Type: EventTextDelta, Data: "{}"}); err != nil {
		t.Fatal(err)
	}
	if !store.Exists("present") {
		t.Errorf("Exists() should be true after a successful Append")
	}
}

// TestSessionStore_LoadSkipsCorruptLines covers the "process
// crashed mid-write" / "external tool wrote garbage" scenarios.
// Load must NOT panic; it must log + skip the bad line and
// return the well-formed ones.
func TestSessionStore_LoadSkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	// Manually craft a file with a mix of good, corrupt, and
	// empty lines.
	path := filepath.Join(dir, "broken.jsonl")
	content := strings.Join([]string{
		`{"type":"text_delta","data":"line1"}`,     // good
		``,                                          // empty (skipped silently)
		`{"type":"text_delta","data":nope}`,         // corrupt (JSON parse fail)
		`not even close to json`,                    // corrupt
		`{"type":"stream_end","data":""}`,           // good
		`{"type":"text_delta","data":"line3"}`,      // good
		``,                                          // trailing empty
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load("broken")
	if err != nil {
		t.Fatalf("Load should not error on corrupt lines, got: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 valid events (corrupt + empty skipped), got %d: %+v", len(got), got)
	}
	if got[0].Type != EventTextDelta {
		t.Errorf("event 0 type: got %q want %q", got[0].Type, EventTextDelta)
	}
	if got[1].Type != EventStreamEnd {
		t.Errorf("event 1 type: got %q want %q", got[1].Type, EventStreamEnd)
	}
	if got[2].Type != EventTextDelta {
		t.Errorf("event 2 type: got %q want %q", got[2].Type, EventTextDelta)
	}
}

// TestSessionStore_AppendFailureDoesNotPanic verifies the
// "disk full / permission denied" path. Append must return an
// error AND not panic. The Agent integration in agent.go
// ignores the return value precisely so a storage hiccup does
// not block the live business path.
func TestSessionStore_AppendFailureDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	// Pre-create a FILE at the path we want to use as root.
	// The store will try to MkdirAll <root> and fail because
	// the path is not a directory.
	lockFile := filepath.Join(dir, "lock")
	if err := os.WriteFile(lockFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := NewSessionStore(lockFile)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Append must not panic, got: %v", r)
		}
	}()

	err := store.Append("sess", Event{Type: EventTextDelta, Data: "{}"})
	if err == nil {
		t.Errorf("expected Append to fail when root is a non-directory, got nil")
	}
}

// TestSessionStore_EmptySessionIDRejected is a defensive check:
// the public methods should never accept the empty string and
// should always return an error.
func TestSessionStore_EmptySessionIDRejected(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	if err := store.Append("", Event{Type: EventTextDelta, Data: "{}"}); err == nil {
		t.Errorf("Append(\"\") should error")
	}
	if _, err := store.Load(""); err == nil {
		t.Errorf("Load(\"\") should error")
	}
	if store.Exists("") {
		t.Errorf("Exists(\"\") should be false")
	}
}

// TestSessionStore_AppendConcurrent exercises the mutex by
// hammering Append from many goroutines on the same and
// different session IDs. The on-disk file must end up with
// exactly `goroutines * writesPerGoroutine` lines per session,
// all valid JSON.
func TestSessionStore_AppendConcurrent(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)

	const (
		goroutines       = 8
		writesPerRoutine = 25
	)
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < writesPerRoutine; i++ {
				if err := store.Append("shared", Event{
					Type: EventTextDelta,
					Data: `{"g":` + itoa(gid) + `,"i":` + itoa(i) + `}`,
				}); err != nil {
					t.Errorf("Append: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	got, err := store.Load("shared")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := goroutines * writesPerRoutine; len(got) != want {
		t.Errorf("event count: got %d want %d", len(got), want)
	}
}

// TestSessionStore_DefaultRootResolves verifies the empty-root
// fallback path used by production code. We can't reliably
// write into the user's real home dir during a test, so we
// just check that the root is non-empty and contains the
// expected suffix.
func TestSessionStore_DefaultRootResolves(t *testing.T) {
	store := NewSessionStore("")
	root := store.Root()
	if root == "" {
		t.Fatalf("default root should not be empty")
	}
	if !strings.HasSuffix(root, filepath.Join(".encv", "agent", "sessions")) {
		t.Errorf("default root suffix mismatch: got %q", root)
	}
}

// itoa is a tiny stdlib-free integer formatter for the
// concurrent test. Avoids pulling in strconv just for this
// helper.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
