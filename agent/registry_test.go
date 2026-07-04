package agent

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// noopHandler is the minimal handler shape used across tests; it lets
// the test focus on registry behaviour without encoding any business
// logic.
func noopHandler(args string) (string, error) {
	return `{"echo":` + jsonQuote(args) + `}`, nil
}

// jsonQuote wraps a string in JSON quotes so we can embed it inside a
// hand-rolled JSON literal. Kept local to tests — production code uses
// encoding/json.
func jsonQuote(s string) string {
	return fmt.Sprintf("%q", s)
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
	}
	r.Register("list_files", schema, noopHandler, false, KindReadOnly)

	def, ok := r.Get("list_files")
	if !ok {
		t.Fatal("Get returned !ok for registered tool")
	}
	if def.Kind != KindReadOnly {
		t.Errorf("Kind: got %q want %q", def.Kind, KindReadOnly)
	}
	if def.NeedConfirm {
		t.Errorf("NeedConfirm: got true want false")
	}
	if def.Schema == nil {
		t.Errorf("Schema should not be nil")
	}
}

func TestRegistry_GetMissingReturnsZero(t *testing.T) {
	r := NewRegistry()
	def, ok := r.Get("does_not_exist")
	if ok {
		t.Errorf("Get should return false for missing tool, got %+v", def)
	}
	// The returned definition must be the zero value — callers may
	// rely on this to skip the struct entirely.
	if def.Handler != nil || def.Schema != nil || def.Kind != "" || def.NeedConfirm {
		t.Errorf("Get on missing tool should return zero value, got %+v", def)
	}
}

func TestRegistry_RegisterOverwrites(t *testing.T) {
	r := NewRegistry()
	r.Register("x", "first", noopHandler, false, KindReadOnly)
	r.Register("x", "second", noopHandler, true, KindCommand)

	def, ok := r.Get("x")
	if !ok {
		t.Fatal("Get returned !ok after re-register")
	}
	if def.Schema != "second" {
		t.Errorf("Schema not overwritten: got %v", def.Schema)
	}
	if !def.NeedConfirm {
		t.Errorf("NeedConfirm not overwritten: got false")
	}
	if def.Kind != KindCommand {
		t.Errorf("Kind not overwritten: got %q", def.Kind)
	}
}

func TestRegistry_HandlerIsInvokable(t *testing.T) {
	// Spec says Handler is the only callable entry point. Verify that
	// the registry stores the function verbatim — we don't wrap or
	// rewrite it.
	r := NewRegistry()
	called := false
	r.Register("echo", nil, func(args string) (string, error) {
		called = true
		return "ok:" + args, nil
	}, false, KindReadOnly)
	def, _ := r.Get("echo")
	out, err := def.Handler(`{"x":1}`)
	if err != nil {
		t.Fatalf("Handler error: %v", err)
	}
	if !called {
		t.Errorf("Handler was not invoked")
	}
	if out != `ok:{"x":1}` {
		t.Errorf("Handler output: got %q", out)
	}
}

func TestRegistry_KindFieldRoundTrips(t *testing.T) {
	// Lock the Kind contract for every documented value, since the
	// front-end picks icons based on this string.
	cases := []struct {
		name string
		kind ToolKind
	}{
		{"cmd", KindCommand},
		{"file", KindFileChange},
		{"read", KindReadOnly},
		{"unk", KindUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := NewRegistry()
			r.Register(c.name, nil, noopHandler, false, c.kind)
			def, ok := r.Get(c.name)
			if !ok {
				t.Fatal("missing tool")
			}
			if def.Kind != c.kind {
				t.Errorf("Kind: got %q want %q", def.Kind, c.kind)
			}
		})
	}
}

func TestRegistry_GetAllSchemas_ReturnsAllRegistered(t *testing.T) {
	r := NewRegistry()
	schemas := []any{"s1", "s2", "s3", map[string]any{"type": "object"}}
	for i, s := range schemas {
		name := fmt.Sprintf("tool_%d", i)
		r.Register(name, s, noopHandler, false, KindReadOnly)
	}

	got := r.GetAllSchemas()
	if len(got) != len(schemas) {
		t.Fatalf("GetAllSchemas length: got %d want %d", len(got), len(schemas))
	}

	// GetAllSchemas makes no ordering guarantee, so compare as a set.
	counts := map[string]int{}
	for _, s := range got {
		key := fmt.Sprintf("%T:%v", s, s)
		counts[key]++
	}
	for _, s := range schemas {
		key := fmt.Sprintf("%T:%v", s, s)
		if counts[key] == 0 {
			t.Errorf("schema %v missing from GetAllSchemas output", s)
		}
	}
}

func TestRegistry_GetAllSchemas_ReturnsFreshSlice(t *testing.T) {
	// Mutating the returned slice must not affect the registry, so the
	// caller can filter / sort / re-slice freely.
	r := NewRegistry()
	r.Register("a", "schema-a", noopHandler, false, KindReadOnly)
	r.Register("b", "schema-b", noopHandler, false, KindReadOnly)

	got := r.GetAllSchemas()
	if len(got) != 2 {
		t.Fatalf("expected 2 schemas, got %d", len(got))
	}
	// Try to clobber the slice — if GetAllSchemas returns the
	// internal slice by reference, the next Get would see the change.
	got[0] = "tampered"
	if got[1] == "tampered" {
		t.Fatalf("GetAllSchemas seems to share its backing array; the registry is not isolated")
	}

	// And a second call returns the same schemas, proving the
	// tampering didn't leak.
	got2 := r.GetAllSchemas()
	if len(got2) != 2 {
		t.Fatalf("expected 2 schemas on second call, got %d", len(got2))
	}
	if got2[0] == "tampered" {
		t.Errorf("registry was mutated via GetAllSchemas slice")
	}
}

func TestRegistry_EmptyGetAllSchemas(t *testing.T) {
	r := NewRegistry()
	got := r.GetAllSchemas()
	// Documented expectation: a non-nil empty slice. Callers can
	// safely call `len()` and range over it without a nil check.
	if got == nil {
		t.Errorf("GetAllSchemas on empty registry should return non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("expected length 0, got %d", len(got))
	}
}

func TestRegistry_ConcurrentGet(t *testing.T) {
	// Race detector should be silent: many readers, one writer at
	// construction time.
	r := NewRegistry()
	const n = 100
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("tool_%d", i)
		r.Register(name, name, noopHandler, false, KindReadOnly)
	}

	const goroutines = 64
	var wg sync.WaitGroup
	var misses int64
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n*10; i++ {
				name := fmt.Sprintf("tool_%d", i%n)
				if _, ok := r.Get(name); !ok {
					atomic.AddInt64(&misses, 1)
				}
			}
		}()
	}
	wg.Wait()
	if misses != 0 {
		t.Errorf("expected 0 misses on concurrent Get, got %d", misses)
	}
}

func TestRegistry_ConcurrentReadAndWrite(t *testing.T) {
	// The interesting case: writers and readers in flight at the same
	// time. We don't make any specific ordering guarantees; we just
	// require that the registry never panics and reads never return a
	// half-written ToolDefinition.
	r := NewRegistry()
	const goroutines = 32
	const iterations = 200
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				name := fmt.Sprintf("g%d_i%d", id, i)
				if i%2 == 0 {
					r.Register(name, "schema", noopHandler, i%4 == 0, KindReadOnly)
				} else {
					_, _ = r.Get(name)
				}
				// Touch the read-mostly path as well.
				_ = r.GetAllSchemas()
			}
		}(g)
	}

	// Wait for the bounded write/read wave to drain BEFORE starting
	// the unbounded reader wave; otherwise the unbounded goroutines
	// would never observe the stop signal (close(stop) is gated by
	// wg.Wait, which would itself wait on the unbounded goroutines —
	// classic deadlock).
	wg.Wait()

	// A second wave keeps doing reads to make sure the lock
	// interleaving is non-trivial. We bound this wave by iteration
	// count as well so the test always terminates.
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations*4; i++ {
				select {
				case <-stop:
					return
				default:
					_ = r.GetAllSchemas()
				}
			}
		}()
	}

	wg.Wait()
	close(stop)
}

func TestRegistry_OverwriteUnderConcurrentRead(t *testing.T) {
	// Re-registering a key while readers are reading it. The contract
	// is that reads are always well-defined (either the old or the
	// new value, never a torn struct).
	r := NewRegistry()
	r.Register("hot", "v1", noopHandler, false, KindReadOnly)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					def, ok := r.Get("hot")
					if !ok {
						return
					}
					// Schema must be one of the two we wrote, never nil.
					if def.Schema == nil {
						t.Errorf("got nil schema under concurrent overwrite")
						return
					}
				}
			}
		}()
	}

	for i := 0; i < 1000; i++ {
		if i%2 == 0 {
			r.Register("hot", "v1", noopHandler, false, KindReadOnly)
		} else {
			r.Register("hot", "v2", noopHandler, true, KindCommand)
		}
	}
	close(stop)
	wg.Wait()
}

func TestRegistry_NeedConfirmFalseyAndTruthy(t *testing.T) {
	// NeedConfirm is the boolean that drives the AutoRun flag on
	// ToolCallData. Make sure both true and false survive a Register
	// + Get round-trip.
	r := NewRegistry()
	r.Register("auto", nil, noopHandler, false, KindReadOnly)
	r.Register("manual", nil, noopHandler, true, KindCommand)

	auto, _ := r.Get("auto")
	if auto.NeedConfirm {
		t.Errorf("auto.NeedConfirm: got true want false")
	}
	manual, _ := r.Get("manual")
	if !manual.NeedConfirm {
		t.Errorf("manual.NeedConfirm: got false want true")
	}
}
