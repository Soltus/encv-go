package service

import (
	"encoding/json"
	"testing"
)

// TestPhaseValues 断言每个 Phase 常量的字符串值与前端
// app/encv-mobile/src/lib/workflow/types.ts 的 Phase 枚举一致。
func TestPhaseValues(t *testing.T) {
	expected := map[Phase]string{
		PhaseCreated:       "created",
		PhaseAnalyzing:     "analyzing",
		PhaseInitializing:  "initializing",
		PhasePreprocessing: "preprocessing",
		PhaseEncrypting:    "encrypting",
		PhaseDecrypting:    "decrypting",
		PhasePacking:       "packing",
		PhaseVerifying:     "verifying",
		PhaseCompleted:     "completed",
	}
	for p, s := range expected {
		if string(p) != s {
			t.Errorf("Phase %s expected %q, got %q", p, s, string(p))
		}
	}
}

// TestPhaseJSONSerialization 验证 Phase（string 别名）序列化为裸字符串，
// 与前端枚举的 JSON 契约一致。
func TestPhaseJSONSerialization(t *testing.T) {
	cases := []struct {
		phase Phase
		want  string
	}{
		{PhaseCreated, `"created"`},
		{PhaseAnalyzing, `"analyzing"`},
		{PhaseInitializing, `"initializing"`},
		{PhasePreprocessing, `"preprocessing"`},
		{PhaseEncrypting, `"encrypting"`},
		{PhaseDecrypting, `"decrypting"`},
		{PhasePacking, `"packing"`},
		{PhaseVerifying, `"verifying"`},
		{PhaseCompleted, `"completed"`},
	}
	for _, c := range cases {
		got, err := json.Marshal(c.phase)
		if err != nil {
			t.Fatalf("json.Marshal(%s) error: %v", c.phase, err)
		}
		if string(got) != c.want {
			t.Errorf("json.Marshal(%s) expected %s, got %s", c.phase, c.want, got)
		}
	}
}
