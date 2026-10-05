package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMode_Valid(t *testing.T) {
	valid := []Mode{ModeReal, ModeReplay, ModeSynthetic}
	for _, m := range valid {
		if !m.Valid() {
			t.Errorf("mode %q should be valid", m)
		}
	}
	for _, m := range []Mode{"", "REAL", "prod", "mock"} {
		if m.Valid() {
			t.Errorf("mode %q should be invalid", m)
		}
	}
}

func TestRecord_Validate(t *testing.T) {
	base := Record{RequirementID: "AGT-004", Mode: ModeReal, Result: "ok"}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	cases := []struct {
		name string
		mut  func(r *Record)
	}{
		{"missing requirementId", func(r *Record) { r.RequirementID = "" }},
		{"invalid mode", func(r *Record) { r.Mode = Mode("fake") }},
		{"empty result", func(r *Record) { r.Result = "" }},
	}
	for _, c := range cases {
		r := base
		c.mut(&r)
		if err := r.Validate(); err == nil {
			t.Errorf("%s: expected validation error", c.name)
		}
	}
}

func TestNewEvidenceID_Unique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 64; i++ {
		id, err := NewEvidenceID()
		if err != nil {
			t.Fatalf("NewEvidenceID: %v", err)
		}
		if id == "" {
			t.Fatal("empty id")
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestStore_AppendListFilter(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "evidence")
	st, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	realRec := NewRecord("SEC-001", ModeReal, "target-1", "denied", []string{"step1"}, nil, nil)
	synRec := NewRecord("AGT-004", ModeSynthetic, "target-1", "presets empty", nil, nil, nil)
	if err := st.Append(realRec); err != nil {
		t.Fatalf("Append real: %v", err)
	}
	if err := st.Append(synRec); err != nil {
		t.Fatalf("Append synthetic: %v", err)
	}
	all, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len = %d, want 2", len(all))
	}
	if all[0].EvidenceID == "" || all[0].RecordedAt == "" {
		t.Fatalf("auto fields not filled: %+v", all[0])
	}
	if all[0].ProducerInstanceID == "" {
		t.Fatal("producer instance id empty")
	}
	realOnly, err := st.Filter("SEC-001", ModeReal)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(realOnly) != 1 || realOnly[0].Mode != ModeReal {
		t.Fatalf("filter by mode failed: %+v", realOnly)
	}
	synOnly, err := st.Filter("", ModeSynthetic)
	if err != nil {
		t.Fatalf("Filter synthetic: %v", err)
	}
	if len(synOnly) != 1 || synOnly[0].RequirementID != "AGT-004" {
		t.Fatalf("filter synthetic failed: %+v", synOnly)
	}
}

func TestStore_AppendRejectsInvalid(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "evidence"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := st.Append(Record{RequirementID: "X", Mode: Mode("bogus"), Result: "ok"}); err == nil {
		t.Fatal("invalid mode should be rejected")
	}
	if err := st.Append(Record{Mode: ModeReal, Result: "ok"}); err == nil {
		t.Fatal("missing requirementId should be rejected")
	}
	all, err := st.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("rejected records must not be persisted, got %d", len(all))
	}
}

func TestStore_ListEmptyWhenMissing(t *testing.T) {
	st, err := NewStore(filepath.Join(t.TempDir(), "evidence"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	all, err := st.List()
	if err != nil {
		t.Fatalf("List on empty store: %v", err)
	}
	if len(all) != 0 {
		t.Fatalf("want 0, got %d", len(all))
	}
}

func TestRecord_RoundTripJSON(t *testing.T) {
	r := NewRecord("EVD-001", ModeReplay, "rt-9", "projection equal", []string{"a", "b"}, []string{"obs"}, []string{"log"})
	r.EvidenceID = "ev-1"
	r.RecordedAt = "2026-10-06T00:00:00Z"
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Record
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.EvidenceID != r.EvidenceID || got.Mode != r.Mode || got.RequirementID != r.RequirementID {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	if len(got.Steps) != 2 {
		t.Fatalf("steps lost: %+v", got.Steps)
	}
}

func TestCurrentArtifactDigest_NonEmpty(t *testing.T) {
	// 测试二进制本身就是可执行文件 ⇒ 摘要必须非空。
	if got := CurrentArtifactDigest(); got == "" {
		t.Fatal("artifact digest must not be empty for a running binary")
	}
}

func TestCurrentWorkspaceCommit_NoPanic(t *testing.T) {
	// 取不到（非 VCS 构建）时返回空串，但不得 panic。
	_ = CurrentWorkspaceCommit()
}

func TestNewStore_RejectsEmptyDir(t *testing.T) {
	if _, err := NewStore(""); err == nil {
		t.Fatal("empty dir must be rejected")
	}
}

func TestStore_FilePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "evidence")
	st, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := st.Append(NewRecord("SEC-002", ModeReal, "t", "ok", nil, nil, nil)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	info, err := os.Stat(st.path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("evidence file perm = %v, want 0600", info.Mode().Perm())
	}
}
