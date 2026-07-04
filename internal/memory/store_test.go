package memory

import (
	"strings"
	"testing"
)

// ─── Store CRUD ─────────────────────────────────────────────

func TestStore_InitialEmpty(t *testing.T) {
	s := NewStore()
	stats := s.Stats()
	if stats["projects"] != 0 || stats["profiles"] != 0 || stats["semantics"] != 0 {
		t.Errorf("new store should be empty, got %+v", stats)
	}
}

func TestStore_ProjectCRUD(t *testing.T) {
	s := NewStore()
	s.SaveProject(ProjectMemory{
		ProjectPath: "/tmp/proj",
		Summary:     "React + TypeScript",
		Files:       []string{"package.json", "tsconfig.json"},
	})

	got := s.GetProject("/tmp/proj")
	if got == nil {
		t.Fatal("expected project, got nil")
	}
	if got.Summary != "React + TypeScript" {
		t.Errorf("Summary = %q", got.Summary)
	}

	if !s.DeleteProject("/tmp/proj") {
		t.Error("DeleteProject should return true")
	}
	if s.GetProject("/tmp/proj") != nil {
		t.Error("should be nil after delete")
	}
}

func TestStore_ProfileCRUD(t *testing.T) {
	s := NewStore()
	s.SaveProfile(UserProfile{Key: "preferred_language", Value: "TypeScript", Confidence: 0.7})
	s.SaveProfile(UserProfile{Key: "build_command", Value: "npm", Confidence: 0.7})

	profiles := s.ListProfiles()
	if len(profiles) != 2 {
		t.Fatalf("ListProfiles = %d, want 2", len(profiles))
	}
	// 排序：confidence 高 → 低（都是 0.7，顺序由 map 决定，不严格断言）
}

func TestStore_HighConfidenceProfiles_FilterAndLimit(t *testing.T) {
	s := NewStore()
	s.SaveProfile(UserProfile{Key: "k1", Value: "v1", Confidence: 0.3})
	s.SaveProfile(UserProfile{Key: "k2", Value: "v2", Confidence: 0.6})
	s.SaveProfile(UserProfile{Key: "k3", Value: "v3", Confidence: 0.8})
	s.SaveProfile(UserProfile{Key: "k4", Value: "v4", Confidence: 0.9})

	high := s.HighConfidenceProfiles(0.5, 10)
	if len(high) != 3 {
		t.Errorf("expected 3 profiles > 0.5, got %d", len(high))
	}
	// 顺序：k4 (0.9) > k3 (0.8) > k2 (0.6)
	if high[0].Key != "k4" || high[1].Key != "k3" || high[2].Key != "k2" {
		t.Errorf("ordering wrong: %+v", high)
	}
}

func TestStore_HighConfidenceProfiles_MaxCount(t *testing.T) {
	s := NewStore()
	for i := 0; i < 5; i++ {
		key := "k" + string(rune('a'+i))
		s.SaveProfile(UserProfile{Key: key, Value: "v", Confidence: 0.9})
	}
	high := s.HighConfidenceProfiles(0.5, 2)
	if len(high) != 2 {
		t.Errorf("MaxCount=2 should cap to 2, got %d", len(high))
	}
}

func TestStore_ProfileSavePreservesHigherConfidence(t *testing.T) {
	s := NewStore()
	s.SaveProfile(UserProfile{Key: "k", Value: "old", Confidence: 0.9})
	s.SaveProfile(UserProfile{Key: "k", Value: "new", Confidence: 0.5})

	got := s.GetProfile("k")
	if got.Confidence != 0.9 {
		t.Errorf("should preserve 0.9, got %f", got.Confidence)
	}
}

func TestStore_SemanticCRUD(t *testing.T) {
	s := NewStore()
	s.SaveSemantic(SemanticMemory{ID: "sm1", Content: "User likes tabs"})

	got := s.GetSemantic("sm1")
	if got == nil {
		t.Fatal("expected semantic, got nil")
	}
	if got.RecallCount != 1 {
		t.Errorf("first recall should give RecallCount=1, got %d", got.RecallCount)
	}

	// 第二次 recall
	got = s.GetSemantic("sm1")
	if got.RecallCount != 2 {
		t.Errorf("second recall should give RecallCount=2, got %d", got.RecallCount)
	}
}

func TestStore_GetSemantic_Missing(t *testing.T) {
	s := NewStore()
	if got := s.GetSemantic("nonexistent"); got != nil {
		t.Error("missing should return nil")
	}
}

// ─── AutoExtract ─────────────────────────────────────────────

func TestAutoExtract_UserPreference_Language(t *testing.T) {
	profiles := AutoExtract("我习惯用 TypeScript")
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d: %+v", len(profiles), profiles)
	}
	if profiles[0].Key != "preferred_language" {
		t.Errorf("Key = %q, want preferred_language", profiles[0].Key)
	}
	if profiles[0].Value != "TypeScript" {
		t.Errorf("Value = %q, want TypeScript", profiles[0].Value)
	}
	if profiles[0].Confidence != AutoExtractConfidence {
		t.Errorf("Confidence = %f, want %f", profiles[0].Confidence, AutoExtractConfidence)
	}
}

func TestAutoExtract_UserPreference_Editor(t *testing.T) {
	profiles := AutoExtract("我常用 vscode")
	if len(profiles) != 1 {
		t.Fatalf("expected 1, got %d", len(profiles))
	}
	if profiles[0].Key != "preferred_editor" {
		t.Errorf("Key = %q, want preferred_editor", profiles[0].Key)
	}
}

func TestAutoExtract_TechStack_Multiple(t *testing.T) {
	profiles := AutoExtract("我用 React, Vue, TypeScript")
	if len(profiles) != 1 {
		t.Fatalf("expected 1 tech_stack profile, got %d: %+v", len(profiles), profiles)
	}
	if profiles[0].Key != "tech_stack" {
		t.Errorf("Key = %q, want tech_stack", profiles[0].Key)
	}
	parts := strings.Split(profiles[0].Value, ",")
	if len(parts) != 3 {
		t.Errorf("expected 3 stack items, got %d: %q", len(parts), profiles[0].Value)
	}
}

func TestAutoExtract_BuildCommand(t *testing.T) {
	cases := []struct {
		msg  string
		want string
	}{
		{"我常用 npm", "npm"},
		{"我总是 pnpm", "pnpm"},
		{"我通常用 yarn", "yarn"},
		{"我常用 gradle", "gradle"},
	}
	for _, c := range cases {
		profiles := AutoExtract(c.msg)
		if len(profiles) != 1 {
			t.Errorf("%q: expected 1, got %d", c.msg, len(profiles))
			continue
		}
		if profiles[0].Value != c.want {
			t.Errorf("%q: Value = %q, want %q", c.msg, profiles[0].Value, c.want)
		}
		if profiles[0].Key != "build_command" {
			t.Errorf("%q: Key = %q, want build_command", c.msg, profiles[0].Key)
		}
	}
}

func TestAutoExtract_NoMatch(t *testing.T) {
	profiles := AutoExtract("今天天气真好")
	if len(profiles) != 0 {
		t.Errorf("expected 0, got %d: %+v", len(profiles), profiles)
	}
}

func TestAutoExtractAndStore_Persists(t *testing.T) {
	s := NewStore()
	n := AutoExtractAndStore(s, "我习惯用 TypeScript")
	if n != 1 {
		t.Errorf("saved 1 profile, got %d", n)
	}
	if s.GetProfile("preferred_language") == nil {
		t.Error("profile should be persisted")
	}
}

func TestAutoExtract_MultiplePatternsInOne(t *testing.T) {
	profiles := AutoExtract("我习惯用 TypeScript，我常用 pnpm")
	// 至少 2 个 profile（1 language + 1 build_command）
	if len(profiles) < 2 {
		t.Errorf("expected >= 2 profiles, got %d", len(profiles))
	}
}
