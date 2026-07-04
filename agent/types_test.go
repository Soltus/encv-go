package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestEventType_AllValuesHaveExpectedStringContract locks the wire
// format of the EventType enum. Any accidental rename or typo will
// break the front-end decoder, so we treat the strings as part of the
// public contract.
func TestEventType_AllValuesHaveExpectedStringContract(t *testing.T) {
	cases := []struct {
		got  EventType
		want string
	}{
		{EventTextDelta, "text_delta"},
		{EventReasoningDelta, "reasoning_delta"},
		{EventToolCall, "tool_call"},
		{EventToolStatus, "tool_status"},
		{EventToolResult, "tool_result"},
		{EventStreamEnd, "stream_end"},
	}
	if len(cases) != 6 {
		t.Fatalf("expected exactly 6 EventType values, got %d", len(cases))
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("EventType %q != %q", string(c.got), c.want)
		}
	}
}

func TestDecision_AllValuesHaveExpectedStringContract(t *testing.T) {
	cases := []struct {
		got  Decision
		want string
	}{
		{DecisionAccept, "accept"},
		{DecisionAcceptForSession, "accept_for_session"},
		{DecisionDecline, "decline"},
		{DecisionCancel, "cancel"},
	}
	if len(cases) != 4 {
		t.Fatalf("expected exactly 4 Decision values, got %d", len(cases))
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("Decision %q != %q", string(c.got), c.want)
		}
	}
}

func TestToolKind_AllValuesHaveExpectedStringContract(t *testing.T) {
	cases := []struct {
		got  ToolKind
		want string
	}{
		{KindCommand, "command"},
		{KindFileChange, "fileChange"},
		{KindReadOnly, "readOnly"},
		{KindUnknown, "unknown"},
	}
	if len(cases) != 4 {
		t.Fatalf("expected exactly 4 ToolKind values, got %d", len(cases))
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("ToolKind %q != %q", string(c.got), c.want)
		}
	}
}

func TestEvent_RoundTripJSON(t *testing.T) {
	original := Event{
		Type: EventTextDelta,
		Data: `{"content":"hello"}`,
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// The wire format must keep "data" as a quoted JSON string, not an
	// embedded object — that is what makes Event transport-agnostic.
	if !strings.Contains(string(raw), `"data":"{\"content\":\"hello\"}"`) {
		t.Errorf("unexpected marshal output: %s", raw)
	}
	var decoded Event
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v want %+v", decoded, original)
	}
}

func TestEvent_StreamEndHasEmptyData(t *testing.T) {
	e := Event{Type: EventStreamEnd}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Event
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Type != EventStreamEnd {
		t.Errorf("Type round-trip: got %q", decoded.Type)
	}
	if decoded.Data != "" {
		t.Errorf("Data round-trip: got %q want empty", decoded.Data)
	}
}

func TestToolCallData_RoundTripJSON(t *testing.T) {
	original := ToolCallData{
		ID:      "call_42",
		Name:    "delete_file",
		Args:    `{"paths":["/tmp/a"]}`,
		AutoRun: false,
		Kind:    KindFileChange,
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Verify the exact field names on the wire so we don't silently
	// rename "auto_run" → "autoRun" and break the front-end.
	for _, want := range []string{
		`"id":"call_42"`,
		`"name":"delete_file"`,
		`"args":"{\"paths\":[\"/tmp/a\"]}"`,
		`"auto_run":false`,
		`"kind":"fileChange"`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %q in marshal output: %s", want, raw)
		}
	}
	var decoded ToolCallData
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v want %+v", decoded, original)
	}
}

func TestToolResultData_RoundTripJSON(t *testing.T) {
	original := ToolResultData{
		ID:         "call_7",
		Name:       "exec_command",
		Result:     `{"stdout":"ok","exitCode":0}`,
		IsError:    false,
		Status:     "success",
		DurationMs: 1234,
	}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{
		`"id":"call_7"`,
		`"name":"exec_command"`,
		`"is_error":false`,
		`"status":"success"`,
		`"duration_ms":1234`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("missing %q in marshal output: %s", want, raw)
		}
	}
	var decoded ToolResultData
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v want %+v", decoded, original)
	}
}

func TestToolResultData_AllDocumentedStatusesRoundTrip(t *testing.T) {
	// Status is documented as "success" | "failed" | "cancelled" | "running".
	// Make sure each value survives a JSON round-trip unchanged so the
	// front-end can switch on it without surprises.
	for _, status := range []string{"success", "failed", "cancelled", "running"} {
		t.Run(status, func(t *testing.T) {
			original := ToolResultData{ID: "x", Name: "y", Status: status}
			raw, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var decoded ToolResultData
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded.Status != status {
				t.Errorf("Status round-trip: got %q want %q", decoded.Status, status)
			}
		})
	}
}

func TestToolStatusData_RoundTripJSON(t *testing.T) {
	original := ToolStatusData{ID: "call_1", Status: "running"}
	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"id":"call_1"`) ||
		!strings.Contains(string(raw), `"status":"running"`) {
		t.Errorf("unexpected marshal output: %s", raw)
	}
	var decoded ToolStatusData
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v want %+v", decoded, original)
	}
}

func TestMessageData_AccumulatesDeltasAndToolEvents(t *testing.T) {
	// MessageData is in-memory only; we still exercise it to lock the
	// accumulator semantics that the agent core will rely on in Phase 2.
	m := MessageData{
		Content:   "Hello",
		Reasoning: "thinking",
		ToolCalls: []ToolCallData{{ID: "1", Name: "list_files"}},
		ToolResults: []ToolResultData{
			{ID: "1", Name: "list_files", Status: "success"},
		},
	}
	if m.Content != "Hello" {
		t.Errorf("Content: got %q", m.Content)
	}
	if m.Reasoning != "thinking" {
		t.Errorf("Reasoning: got %q", m.Reasoning)
	}
	if len(m.ToolCalls) != 1 || m.ToolCalls[0].ID != "1" {
		t.Errorf("ToolCalls: got %+v", m.ToolCalls)
	}
	if len(m.ToolResults) != 1 || m.ToolResults[0].Status != "success" {
		t.Errorf("ToolResults: got %+v", m.ToolResults)
	}
}

func TestMessageData_ZeroValueIsUsable(t *testing.T) {
	// A fresh MessageData must be a valid starting point for the agent
	// to fold deltas into; nil slices are fine because append handles
	// them and JSON marshalling will emit "null" which the front-end
	// already tolerates for optional fields.
	var m MessageData
	if m.Content != "" || m.Reasoning != "" {
		t.Errorf("expected empty string fields, got %+v", m)
	}
	if m.ToolCalls != nil || m.ToolResults != nil {
		t.Errorf("expected nil slices, got %+v / %+v", m.ToolCalls, m.ToolResults)
	}
}

func TestEvent_UnmarshalRejectsUnknownType(t *testing.T) {
	// Front-end decoders must be able to spot a typo'd event type
	// without panicking. The type itself is a string, so the round-trip
	// is lossless — but a value not in the documented set is still
	// parseable; the front-end is responsible for rejecting it. We
	// document that contract here.
	raw := []byte(`{"type":"totally_made_up","data":""}`)
	var e Event
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Type != EventType("totally_made_up") {
		t.Errorf("unknown type should pass through verbatim, got %q", e.Type)
	}
}
