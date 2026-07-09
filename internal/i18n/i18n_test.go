package i18n

import (
	"testing"
)

func TestT_Basic(t *testing.T) {
	val := T("common.cancel")
	if val == "" || val == "common.cancel" {
		t.Fatalf("expected translation for common.cancel, got: %q", val)
	}
	t.Logf("common.cancel = %q", val)
}

func TestT_MissingKey(t *testing.T) {
	val := T("nonexistent.key.12345")
	if val != "nonexistent.key.12345" {
		t.Fatalf("expected key itself for missing key, got: %q", val)
	}
}

func TestSetLocale(t *testing.T) {
	original := GetLocale()
	defer SetLocale(original)

	if err := SetLocale("en"); err != nil {
		t.Fatalf("SetLocale(en) failed: %v", err)
	}

	if GetLocale() != "en" {
		t.Fatalf("expected locale 'en', got: %s", GetLocale())
	}

	enVal := T("common.cancel")
	if enVal == "" || enVal == "common.cancel" {
		t.Fatalf("expected English translation, got: %q", enVal)
	}
	t.Logf("en common.cancel = %q", enVal)

	SetLocale("zh-CN")
	zhVal := T("common.cancel")
	if zhVal == enVal {
		t.Fatalf("zh-CN and en should differ, both: %q", zhVal)
	}
	t.Logf("zh-CN common.cancel = %q", zhVal)
}

func TestTWith(t *testing.T) {
	val := TWith("common.cancel", nil)
	if val == "" || val == "common.cancel" {
		t.Fatalf("expected translation, got: %q", val)
	}
}

func TestAvailableLocales(t *testing.T) {
	locales := AvailableLocales()
	if len(locales) < 2 {
		t.Fatalf("expected at least 2 locales, got: %v", locales)
	}
	t.Logf("available locales: %v", locales)
}

func TestNormalizeLocale(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"zh-CN", "zh-CN"},
		{"zh_cn", "zh-CN"},
		{"zh", "zh-CN"},
		{"zh-Hans", "zh-CN"},
		{"en-US", "en"},
		{"en_us", "en"},
		{"en", "en"},
		{"zh-CN.UTF-8", "zh-CN"},
		{"en_US.UTF-8", "en"},
	}

	for _, tt := range tests {
		got := normalizeLocale(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeLocale(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
