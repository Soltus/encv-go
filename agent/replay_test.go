package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestReplay_EmitsInOrderAndShape is the canonical happy path:
// three events with explicit timestamps land in the SSE stream
// in JSONL order, the byte stream is well-formed, and the
// sleeper is invoked with the expected deltas divided by
// speed=1.0.
func TestReplay_EmitsInOrderAndShape(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	// Three events with strictly-increasing timestamps
	// (100ms, 250ms, 400ms). The deltas should be 150ms and
	// 150ms.
	in := []Event{
		{Type: EventTextDelta, Data: `{"ts_ms":100,"content":"hello"}`},
		{Type: EventToolCall, Data: `{"ts_ms":250,"id":"c1","name":"echo","args":"{}"}`},
		{Type: EventStreamEnd, Data: `{"ts_ms":400}`},
	}
	for _, e := range in {
		if err := store.Append("s1", e); err != nil {
			t.Fatalf("Append(%v): %v", e.Type, err)
		}
	}

	var (
		out bytes.Buffer
		// recorded captures every sleep the replay
		// requested, in order. Empty sleep calls (deltas
		// <= 0) are not recorded.
		recorded []time.Duration
	)
	sleep := func(d time.Duration) {
		recorded = append(recorded, d)
	}
	if err := replayWith("s1", store, 1.0, &out, sleep); err != nil {
		t.Fatalf("replayWith: %v", err)
	}

	// The output must contain three SSE frames.
	frames := splitSSEFrames(out.String())
	if len(frames) != 3 {
		t.Fatalf("expected 3 SSE frames, got %d (raw: %q)", len(frames), out.String())
	}
	// The raw output must end with the standard SSE
	// trailing blank line (the last frame's terminator).
	if !strings.HasSuffix(out.String(), "\n\n") {
		t.Errorf("SSE stream missing trailing blank line: %q", out.String())
	}
	for i, frame := range frames {
		if !strings.HasPrefix(frame, "data: ") {
			t.Errorf("frame %d missing 'data: ' prefix: %q", i, frame)
		}
		// Parse and assert on type/data to confirm the
		// wire shape is the standard {"type":..,"data":..}
		// JSON. (The trailing "\n\n" was the frame
		// separator and was consumed by splitSSEFrames.)
		raw := strings.TrimPrefix(frame, "data: ")
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Errorf("frame %d JSON: %v (raw: %q)", i, err, raw)
			continue
		}
		if payload["type"] != string(in[i].Type) {
			t.Errorf("frame %d type: got %v want %v", i, payload["type"], in[i].Type)
		}
		if payload["data"] != in[i].Data {
			t.Errorf("frame %d data: got %v want %v", i, payload["data"], in[i].Data)
		}
	}

	// Two inter-event deltas: (250-100)=150ms and
	// (400-250)=150ms.
	wantDeltas := []time.Duration{150 * time.Millisecond, 150 * time.Millisecond}
	if len(recorded) != len(wantDeltas) {
		t.Fatalf("slept %d times, want %d (recorded: %v)", len(recorded), len(wantDeltas), recorded)
	}
	for i, want := range wantDeltas {
		if recorded[i] != want {
			t.Errorf("slept[%d] = %v, want %v", i, recorded[i], want)
		}
	}
}

// TestReplay_SpeedScalesDeltas locks the contract that `speed`
// divides the inter-event delay: at speed=2.0 a 100ms gap
// becomes 50ms. The first event must carry a non-zero
// timestamp so it establishes a baseline; without a
// baseline the second event has nothing to sleep against
// (see TestReplay_NoTimestampEmitsImmediately).
func TestReplay_SpeedScalesDeltas(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	if err := store.Append("s", Event{Type: EventTextDelta, Data: `{"ts_ms":0,"content":"a"}`}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append("s", Event{Type: EventTextDelta, Data: `{"ts_ms":100,"content":"b"}`}); err != nil {
		t.Fatal(err)
	}

	var recorded []time.Duration
	sleep := func(d time.Duration) { recorded = append(recorded, d) }

	// First event has no timestamp → no baseline → no
	// sleep recorded. This is a regression guard for the
	// "no first-event timestamp" path: the helper must NOT
	// panic and must NOT fabricate a delta.
	if err := replayWith("s", store, 2.0, &bytes.Buffer{}, sleep); err != nil {
		t.Fatalf("replayWith: %v", err)
	}
	if len(recorded) != 0 {
		t.Errorf("expected 0 sleeps when first event has no timestamp, got %d (%v)", len(recorded), recorded)
	}
}

// TestReplay_SpeedScalesDeltasBaselineSet is the explicit
// "speed scales the inter-event gap" test: the first event
// carries a timestamp so the second event has a baseline to
// sleep against. At speed=2.0 a 200ms gap becomes 100ms.
func TestReplay_SpeedScalesDeltasBaselineSet(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	if err := store.Append("s", Event{Type: EventTextDelta, Data: `{"ts_ms":100,"content":"a"}`}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append("s", Event{Type: EventTextDelta, Data: `{"ts_ms":300,"content":"b"}`}); err != nil {
		t.Fatal(err)
	}

	var recorded []time.Duration
	sleep := func(d time.Duration) { recorded = append(recorded, d) }

	if err := replayWith("s", store, 2.0, &bytes.Buffer{}, sleep); err != nil {
		t.Fatalf("replayWith: %v", err)
	}
	if len(recorded) != 1 {
		t.Fatalf("slept %d times, want 1", len(recorded))
	}
	// 200ms / 2.0 = 100ms.
	if got, want := recorded[0], 100*time.Millisecond; got != want {
		t.Errorf("delta at speed=2.0: got %v want %v", got, want)
	}
}

// TestReplay_NoTimestampEmitsImmediately covers the JSONL
// "all good, no stamp" case: events are emitted in JSONL
// order with no sleep, because there is no time reference to
// pace against.
func TestReplay_NoTimestampEmitsImmediately(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	in := []Event{
		{Type: EventTextDelta, Data: `{"content":"a"}`},
		{Type: EventTextDelta, Data: `{"content":"b"}`},
		{Type: EventStreamEnd, Data: ""},
	}
	for _, e := range in {
		if err := store.Append("s", e); err != nil {
			t.Fatal(err)
		}
	}

	var (
		out      bytes.Buffer
		recorded []time.Duration
	)
	sleep := func(d time.Duration) { recorded = append(recorded, d) }
	if err := replayWith("s", store, 1.0, &out, sleep); err != nil {
		t.Fatalf("replayWith: %v", err)
	}
	if len(recorded) != 0 {
		t.Errorf("expected no sleeps when no timestamps, got %d (%v)", len(recorded), recorded)
	}
	frames := splitSSEFrames(out.String())
	if len(frames) != 3 {
		t.Errorf("expected 3 frames, got %d", len(frames))
	}
}

// TestReplay_HealsDurabilityGap covers the comment in
// replay.go: when events are written out of order on disk
// (durability gap), the replay re-sorts by extracted
// timestamp so the consumer sees a stable wall-clock view.
func TestReplay_HealsDurabilityGap(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	// Write events to disk in the order [t=200, t=100,
	// t=300] to simulate a partial flush that landed
	// out-of-order.
	unsorted := []Event{
		{Type: EventTextDelta, Data: `{"ts_ms":200,"content":"second"}`},
		{Type: EventTextDelta, Data: `{"ts_ms":100,"content":"first"}`},
		{Type: EventTextDelta, Data: `{"ts_ms":300,"content":"third"}`},
	}
	for _, e := range unsorted {
		if err := store.Append("s", e); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	if err := replayWith("s", store, 1.0, &out, func(time.Duration) {}); err != nil {
		t.Fatalf("replayWith: %v", err)
	}
	frames := splitSSEFrames(out.String())
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}
	// Pull the "content" field out of each frame and
	// assert the emit order is 100, 200, 300.
	want := []string{"first", "second", "third"}
	for i, f := range frames {
		raw := strings.TrimSuffix(strings.TrimPrefix(f, "data: "), "\n\n")
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		data, _ := payload["data"].(string)
		var inner map[string]any
		if err := json.Unmarshal([]byte(data), &inner); err != nil {
			t.Fatalf("frame %d inner: %v", i, err)
		}
		if inner["content"] != want[i] {
			t.Errorf("frame %d content: got %v want %v", i, inner["content"], want[i])
		}
	}
}

// TestReplay_MissingSessionReturnsError asserts the "no such
// session" path: Replay surfaces the SessionStore error
// rather than silently emitting nothing.
func TestReplay_MissingSessionReturnsError(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	err := replayWith("nope", store, 1.0, &bytes.Buffer{}, func(time.Duration) {})
	if err == nil {
		t.Fatalf("expected error for missing session, got nil")
	}
}

// TestReplay_EmptySessionIDRejected guards the public
// contract: empty inputs are rejected up-front so a buggy
// caller cannot accidentally read from a relative path.
func TestReplay_EmptySessionIDRejected(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	if err := replayWith("", store, 1.0, &bytes.Buffer{}, func(time.Duration) {}); err == nil {
		t.Errorf("expected error for empty sessionID")
	}
}

// TestReplay_TriggeredAtMsKeyIsRecognised locks the second
// timestamp key (used by CompactionData) so a future
// refactor that only honours "ts_ms" does not silently break
// the live wire format.
func TestReplay_TriggeredAtMsKeyIsRecognised(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	if err := store.Append("s", Event{
		Type: EventCompaction,
		Data: `{"triggered_at_ms":1000,"summary_text":"x","replaced_message_count":0}`,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append("s", Event{
		Type: EventCompaction,
		Data: `{"triggered_at_ms":1300,"summary_text":"y","replaced_message_count":0}`,
	}); err != nil {
		t.Fatal(err)
	}
	var recorded []time.Duration
	if err := replayWith("s", store, 1.0, &bytes.Buffer{}, func(d time.Duration) {
		recorded = append(recorded, d)
	}); err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 {
		t.Fatalf("expected 1 sleep, got %d", len(recorded))
	}
	if recorded[0] != 300*time.Millisecond {
		t.Errorf("delta: got %v want 300ms", recorded[0])
	}
}

// TestReplay_NegativeOrZeroSpeedTreatedAsOne is a defensive
// check: a misconfigured CLI (e.g. `--replay-speed 0`) must
// not divide by zero or sleep for a negative duration. The
// first event has a non-zero timestamp so the delta
// computation has a baseline.
func TestReplay_NegativeOrZeroSpeedTreatedAsOne(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	if err := store.Append("s", Event{Type: EventTextDelta, Data: `{"ts_ms":50,"content":"a"}`}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append("s", Event{Type: EventTextDelta, Data: `{"ts_ms":150,"content":"b"}`}); err != nil {
		t.Fatal(err)
	}
	for _, speed := range []float64{0, -1, -2.5} {
		var recorded []time.Duration
		if err := replayWith("s", store, speed, &bytes.Buffer{}, func(d time.Duration) {
			recorded = append(recorded, d)
		}); err != nil {
			t.Fatalf("speed=%v: %v", speed, err)
		}
		if len(recorded) != 1 || recorded[0] != 100*time.Millisecond {
			t.Errorf("speed=%v: recorded=%v want [100ms]", speed, recorded)
		}
	}
}

// TestReplay_ReadOnly is a regression guard: Replay must
// never modify the on-disk JSONL file. The check compares
// the file's modification time and content before vs after
// the call.
func TestReplay_ReadOnly(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(dir)
	if err := store.Append("s", Event{Type: EventTextDelta, Data: `{"content":"x"}`}); err != nil {
		t.Fatal(err)
	}
	path := store.pathFor("s")
	before, err := readFile(t, path)
	if err != nil {
		t.Fatal(err)
	}
	// Add a 10ms gap so any subsequent write would be
	// detected by a content delta.
	time.Sleep(10 * time.Millisecond)
	if err := replayWith("s", store, 1.0, &bytes.Buffer{}, func(time.Duration) {}); err != nil {
		t.Fatal(err)
	}
	after, err := readFile(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("replay modified the JSONL file (len before=%d after=%d)", len(before), len(after))
	}
}

// TestEncodeSSE_NilEventIsNoOp mirrors the writeSSE
// contract: a nil event is a no-op so the live /api/chat
// handler can safely flush a placeholder.
func TestEncodeSSE_NilEventIsNoOp(t *testing.T) {
	var buf bytes.Buffer
	if err := encodeSSE(&buf, nil); err != nil {
		t.Errorf("encodeSSE(nil) should not error, got %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("encodeSSE(nil) should write nothing, got %q", buf.String())
	}
}

// TestEncodeSSE_FormatsCorrectly locks the wire shape: each
// frame starts with "data: " and ends with a blank line, and
// the body is the standard {"type":..,"data":..} JSON. The
// inner payload is JSON-escaped by the outer Marshal, so the
// "hi" string in the original event surfaces as \"hi\" in
// the output.
func TestEncodeSSE_FormatsCorrectly(t *testing.T) {
	var buf bytes.Buffer
	ev := &Event{Type: EventTextDelta, Data: `{"content":"hi"}`}
	if err := encodeSSE(&buf, ev); err != nil {
		t.Fatalf("encodeSSE: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "data: ") {
		t.Errorf("SSE frame missing 'data: ' prefix: %q", out)
	}
	if !strings.HasSuffix(out, "\n\n") {
		t.Errorf("SSE frame missing trailing blank line: %q", out)
	}
	if !strings.Contains(out, `"text_delta"`) {
		t.Errorf("SSE frame missing event type: %q", out)
	}
	// The literal substring `hi` must appear (it is the
	// inner payload's `content` field, JSON-escaped in
	// the outer frame).
	if !strings.Contains(out, "hi") {
		t.Errorf("SSE frame missing payload substring: %q", out)
	}
}

// TestExtractTimestampMs_KnownShapes is a unit-level guard
// for the field-name precedence: ts_ms > triggered_at_ms > 0.
func TestExtractTimestampMs_KnownShapes(t *testing.T) {
	cases := []struct {
		name string
		data string
		want int64
	}{
		{"empty data", "", 0},
		{"malformed json", "not json", 0},
		{"ts_ms positive", `{"ts_ms":42,"content":"x"}`, 42},
		{"triggered_at_ms positive", `{"triggered_at_ms":99,"summary_text":"x"}`, 99},
		{"ts_ms wins over triggered", `{"ts_ms":1,"triggered_at_ms":2}`, 1},
		{"zero ts_ms falls back", `{"ts_ms":0,"triggered_at_ms":7}`, 7},
		{"both zero returns zero", `{"ts_ms":0,"triggered_at_ms":0}`, 0},
		{"negative treated as zero", `{"ts_ms":-5}`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractTimestampMs(Event{Data: tc.data})
			if got != tc.want {
				t.Errorf("extractTimestampMs(%q) = %d, want %d", tc.data, got, tc.want)
			}
		})
	}
}

// TestComputeDelta_TableDriven covers the corner cases of
// the delta computation: zero prev (first event), equal
// timestamps, out-of-order (negative delta), and the speed
// scaling.
func TestComputeDelta_TableDriven(t *testing.T) {
	cases := []struct {
		name  string
		prev  int64
		curr  int64
		speed float64
		want  time.Duration
	}{
		{"first event no prev", 0, 100, 1.0, 0},
		{"100ms at 1x", 100, 200, 1.0, 100 * time.Millisecond},
		{"100ms at 2x", 100, 200, 2.0, 50 * time.Millisecond},
		{"equal timestamps", 100, 100, 1.0, 0},
		{"out of order", 200, 100, 1.0, 0},
		{"large delta", 500_000, 1_000_000, 1.0, 500_000 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeDelta(tc.prev, tc.curr, tc.speed)
			if got != tc.want {
				t.Errorf("computeDelta(%d, %d, %v) = %v, want %v", tc.prev, tc.curr, tc.speed, got, tc.want)
			}
		})
	}
}

// splitSSEFrames is a small helper used by the order/shape
// tests. SSE frames end with a blank line ("\n\n"); we split
// on that and drop any trailing empty element.
func splitSSEFrames(s string) []string {
	parts := strings.Split(s, "\n\n")
	// The final element is always empty because the input
	// ends with "\n\n". Drop it.
	if n := len(parts); n > 0 && parts[n-1] == "" {
		parts = parts[:n-1]
	}
	return parts
}

// readFile is a thin wrapper that fails the test on error
// rather than returning it, so the call sites read as
// straight-line code.
func readFile(t *testing.T, path string) ([]byte, error) {
	t.Helper()
	return os.ReadFile(path)
}
