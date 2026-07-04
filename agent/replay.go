package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

// Replay reads the JSONL event log for sessionID and streams
// the events back to stdout in the SSE wire format, with the
// inter-event delay scaled by `speed` (1.0 = real time, 2.0 =
// twice as fast, 0.5 = half as fast, <=0 = as fast as
// possible).
//
// Replay is a read-only operation: it never modifies the
// session file, the SessionStore, or any external state. The
// purpose is to let a developer inspect a historical session
// from the CLI (e.g. `agent-demo --replay <id>`) or to feed
// the events into an external log analyser.
//
// The on-disk events are already in emission order; Replay
// only re-sorts by an extracted timestamp when at least one
// event carries a parseable one, so a partial flush that
// landed out-of-order ("durability gap") is healed silently.
// When no event has a timestamp, the JSONL order is kept
// verbatim and the loop emits back-to-back with no sleep.
func Replay(sessionID, root string, speed float64) error {
	return ReplayTo(sessionID, root, speed, os.Stdout)
}

// ReplayTo is the testable form of Replay. It exposes the
// writer so unit tests can capture the byte stream and assert
// on the SSE format. The sleeper is the standard time.Sleep
// so production callers go through this entry point.
func ReplayTo(sessionID, root string, speed float64, w io.Writer) error {
	return replayWith(sessionID, NewSessionStore(root), speed, w, time.Sleep)
}

// replayWith is the lowest-level entry point: it accepts an
// already-built SessionStore and a sleeper function so tests
// can inject a deterministic clock and assert on the
// resulting cadence without depending on real wall-clock
// time. The store and the sleeper are not retained beyond the
// call, so Replay remains a pure read operation.
func replayWith(sessionID string, store *SessionStore, speed float64, w io.Writer, sleep func(time.Duration)) error {
	if sessionID == "" {
		return errors.New("replay: sessionID must not be empty")
	}
	if store == nil {
		return errors.New("replay: store must not be nil")
	}
	if w == nil {
		return errors.New("replay: writer must not be nil")
	}
	if speed <= 0 {
		speed = 1.0
	}
	events, err := store.Load(sessionID)
	if err != nil {
		return fmt.Errorf("replay: load %q: %w", sessionID, err)
	}
	if len(events) == 0 {
		return nil
	}

	// Decorate each event with an extracted timestamp
	// (0 = no timestamp available). When the file is in
	// JSONL order and no event has a timestamp, the slice
	// stays as-is and the loop below emits events
	// back-to-back.
	decorated := make([]decoratedEvent, len(events))
	for i, ev := range events {
		decorated[i] = decoratedEvent{Event: ev, tsMs: extractTimestampMs(ev)}
	}

	// Sort by extracted timestamp; events without a
	// timestamp keep their relative JSONL order (stable
	// sort). We only sort when at least one timestamp is
	// present so a fully-stamp-less log does not pay the
	// sort cost or shuffle a known-good order.
	if hasAnyTimestamp(decorated) {
		sort.SliceStable(decorated, func(i, j int) bool {
			return decorated[i].tsMs < decorated[j].tsMs
		})
	}

	// Emit each event as an SSE frame, sleeping the
	// inter-event delta divided by `speed`. The first event
	// is emitted immediately (no leading sleep).
	var prevTs int64
	for i, d := range decorated {
		if i > 0 {
			if delta := computeDelta(prevTs, d.tsMs, speed); delta > 0 {
				sleep(delta)
			}
		}
		if err := encodeSSE(w, &d.Event); err != nil {
			return fmt.Errorf("replay: encode event %d: %w", i, err)
		}
		if d.tsMs > 0 {
			prevTs = d.tsMs
		}
	}
	return nil
}

// decoratedEvent pairs an Event with the millisecond
// timestamp extracted from its data payload (0 = none).
type decoratedEvent struct {
	Event
	tsMs int64
}

// hasAnyTimestamp reports whether at least one event in the
// slice carries a parseable timestamp.
func hasAnyTimestamp(events []decoratedEvent) bool {
	for _, e := range events {
		if e.tsMs > 0 {
			return true
		}
	}
	return false
}

// computeDelta returns the elapsed time between prev and
// curr, scaled by 1/speed. When curr <= prev (out of order,
// or no timestamps) the delta is 0 — we never sleep for a
// non-positive duration because time.Sleep would panic on
// the most negative values and would otherwise produce
// confusing "play in reverse" behaviour.
//
// A non-positive speed is replaced with 1.0 upstream, so we
// do not re-validate here.
func computeDelta(prev, curr int64, speed float64) time.Duration {
	if curr <= prev || prev == 0 {
		return 0
	}
	delta := time.Duration(curr-prev) * time.Millisecond
	return time.Duration(float64(delta) / speed)
}

// extractTimestampMs pulls a millisecond timestamp out of an
// event's data payload. It looks for (in priority order):
//
//   - top-level "ts_ms"
//   - top-level "triggered_at_ms"
//
// Both keys are accepted because the existing event payloads
// are inconsistent: TextDelta / ToolCall / ToolResult do not
// carry a timestamp at all, while CompactionData uses
// "triggered_at_ms" (see types.go). A handful of
// forward-compatible tests prefix their payloads with
// "ts_ms" so the helper can stay general.
//
// Returns 0 when the field is absent, malformed, or
// non-positive. A zero return is the safe default that
// collapses to "emit as fast as possible" downstream.
func extractTimestampMs(ev Event) int64 {
	if ev.Data == "" {
		return 0
	}
	var probe struct {
		TsMs          int64 `json:"ts_ms"`
		TriggeredAtMs int64 `json:"triggered_at_ms"`
	}
	if err := json.Unmarshal([]byte(ev.Data), &probe); err != nil {
		return 0
	}
	if probe.TsMs > 0 {
		return probe.TsMs
	}
	if probe.TriggeredAtMs > 0 {
		return probe.TriggeredAtMs
	}
	return 0
}

// encodeSSE serialises one Event into the SSE wire format and
// writes it to w. It mirrors the shape used by writeSSE in
// http.go so the on-the-wire format stays identical between
// the live /api/chat stream and a CLI replay:
//
//	data: {"type":"...","data":"..."}\n\n
//
// A nil ev is a no-op (the live stream has the same contract
// because flusher.Write of a nil SSE frame would corrupt the
// protocol).
func encodeSSE(w io.Writer, ev *Event) error {
	if ev == nil {
		return nil
	}
	payload := map[string]any{
		"type": ev.Type,
		"data": ev.Data,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", raw)
	return err
}
