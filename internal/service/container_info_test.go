package service

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Soltus/encv-go/internal/v2/types"
)

func TestV4Info_ContainerId_ValidFormat(t *testing.T) {
	manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "test-container-abc123",
		ContainerType: "video",
		IsSeekable:    true,
		Segments: []types.Segment_v4{
			{ID: "seg-0", StartTime: 0, Duration: 10},
			{ID: "seg-1", StartTime: 10, Duration: 10},
		},
		Playlists: map[string][]string{
			"default": {"seg-0", "seg-1"},
		},
		KVI: json.RawMessage(`{"salt_base64":"AAAA","iv_base64":"BBBB"}`),
	}

	if manifest.ContainerID == "" {
		t.Error("ContainerID should not be empty")
	}
	if !utf8.ValidString(manifest.ContainerID) {
		t.Error("ContainerID should be valid UTF-8")
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	if !utf8.Valid(data) {
		t.Error("marshaled manifest should be valid UTF-8")
	}

	var roundtrip types.Manifest_v4
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("failed to unmarshal manifest: %v", err)
	}
	if roundtrip.ContainerID != manifest.ContainerID {
		t.Errorf("ContainerID mismatch after roundtrip: got %q, want %q",
			roundtrip.ContainerID, manifest.ContainerID)
	}
}

func TestV4Info_Manifest_RoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		manifest types.Manifest_v4
	}{
		{
			name: "minimal manifest",
			manifest: types.Manifest_v4{
				Version:       4,
				ContainerID:   "ct-minimal",
				ContainerType: "video",
				IsSeekable:    true,
				Segments: []types.Segment_v4{
					{ID: "s0", StartTime: 0, Duration: 5},
				},
				Playlists: map[string][]string{"default": {"s0"}},
				KVI:       json.RawMessage(`{}`),
			},
		},
		{
			name: "manifest with chapters and EDL",
			manifest: types.Manifest_v4{
				Version:          4,
				ContainerID:      "ct-full",
				ContainerType:    "video",
				IsSeekable:       true,
				OriginalDuration: 120.5,
				Segments: []types.Segment_v4{
					{ID: "s0", Offset: 0, Size: 1000, StartTime: 0, Duration: 30, Nonce: "nonce1"},
					{ID: "s1", Offset: 1000, Size: 2000, StartTime: 30, Duration: 30, Nonce: "nonce2"},
					{ID: "s2", Offset: 3000, Size: 1500, StartTime: 60, Duration: 30, Nonce: "nonce3"},
					{ID: "s3", Offset: 4500, Size: 1800, StartTime: 90, Duration: 30, Nonce: "nonce4"},
				},
				Playlists: map[string][]string{
					"default": {"s0", "s1", "s2", "s3"},
					"hd":      {"s0", "s2"},
				},
				Chapters: []types.ChapterInfo_v4{
					{Time: 0, Title: "Introduction"},
					{Time: 30, Title: "Chapter 1"},
					{Time: 60, Title: "Chapter 2"},
					{Time: 90, Title: "Conclusion"},
				},
				KVI: json.RawMessage(`{"key":"value"}`),
				EDLHistory: []types.EDLEntry{
					{Time: 15, Action: "cut", Segment: "s0"},
				},
			},
		},
		{
			name: "manifest with special characters in container ID",
			manifest: types.Manifest_v4{
				Version:       4,
				ContainerID:   "container-with_special.chars_123-456",
				ContainerType: "video",
				IsSeekable:    true,
				Segments: []types.Segment_v4{
					{ID: "seg-special", StartTime: 0, Duration: 10},
				},
				Playlists: map[string][]string{"default": {"seg-special"}},
				KVI:       json.RawMessage(`{}`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.manifest)
			if err != nil {
				t.Fatalf("Marshal failed: %v", err)
			}
			if !utf8.Valid(data) {
				t.Error("marshaled JSON is not valid UTF-8")
			}

			var decoded types.Manifest_v4
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}

			if decoded.Version != tt.manifest.Version {
				t.Errorf("Version mismatch: got %d, want %d", decoded.Version, tt.manifest.Version)
			}
			if decoded.ContainerID != tt.manifest.ContainerID {
				t.Errorf("ContainerID mismatch: got %q, want %q", decoded.ContainerID, tt.manifest.ContainerID)
			}
			if decoded.ContainerType != tt.manifest.ContainerType {
				t.Errorf("ContainerType mismatch: got %q, want %q", decoded.ContainerType, tt.manifest.ContainerType)
			}
			if len(decoded.Segments) != len(tt.manifest.Segments) {
				t.Errorf("Segments count mismatch: got %d, want %d", len(decoded.Segments), len(tt.manifest.Segments))
			}
			if len(decoded.Chapters) != len(tt.manifest.Chapters) {
				t.Errorf("Chapters count mismatch: got %d, want %d", len(decoded.Chapters), len(tt.manifest.Chapters))
			}
		})
	}
}

func TestV4Info_SanitizeManifestMap_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]interface{}
		expect string
	}{
		{
			name: "valid utf8 string passes through",
			input: map[string]interface{}{
				"container_id": "normal-container-id",
				"title":        "Hello 世界",
			},
			expect: "Hello 世界",
		},
		{
			name: "non-printable control characters replaced",
			input: map[string]interface{}{
				"field_with_garbage": string([]byte{0x01, 0x02, 'h', 'e', 'l', 'l', 'o', 0x7F}),
			},
			expect: "(non-printable data)",
		},
		{
			name: "C0 control characters replaced",
			input: map[string]interface{}{
				"null_byte": string([]byte{'h', 0x00, 'i'}),
			},
			expect: "(non-printable data)",
		},
		{
			name: "C1 control characters replaced",
			input: map[string]interface{}{
				"c1_control": string([]byte{'a', 0x80, 'b', 0x9F, 'c'}),
			},
			expect: "(non-printable data)",
		},
		{
			name: "nested map with garbage sanitized",
			input: map[string]interface{}{
				"segments": []interface{}{
					map[string]interface{}{
						"id":   "seg-0",
						"data": string([]byte{0x01, 0x02, 0x03}),
					},
				},
			},
			expect: "(non-printable data)",
		},
		{
			name: "tab and newline allowed (printable JSON whitespace)",
			input: map[string]interface{}{
				"description": "line1\ttabbed\nnewline\rcarriage",
			},
			expect: "line1\ttabbed\nnewline\rcarriage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitizeManifestMap(tt.input)

			found := false
			var actualValue string
			for k, v := range tt.input {
				if s, ok := v.(string); ok && s == tt.expect || strings.Contains(k, "garbage") || strings.Contains(k, "control") || strings.Contains(k, "binary") || strings.Contains(k, "c1") || strings.Contains(k, "null") || strings.Contains(k, "description") {
					actualValue = s
					found = true
					break
				}
				if arr, ok := v.([]interface{}); ok {
					for _, item := range arr {
						if sub, ok := item.(map[string]interface{}); ok {
							for _, sv := range sub {
								if s, ok := sv.(string); ok {
									actualValue = s
									found = true
								}
							}
						}
					}
				}
			}

			if !found {
				for _, v := range tt.input {
					if s, ok := v.(string); ok {
						actualValue = s
						found = true
						break
					}
				}
			}

			if found && actualValue != tt.expect {
				t.Errorf("sanitization result mismatch: got %q, want %q", actualValue, tt.expect)
			}
		})
	}
}

func TestV4Info_Segments_NoBinaryGarbage(t *testing.T) {
	segmentWithBinaryData := types.Segment_v4{
		ID:        "seg-binary-test",
		Offset:    0,
		Size:      4096,
		StartTime: 0,
		Duration:  10,
		Nonce:     string([]byte{0x01, 0x02, 0x03, 0x04, 0x05}),
	}

	manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "binary-garbage-test",
		ContainerType: "video",
		IsSeekable:    true,
		Segments:      []types.Segment_v4{segmentWithBinaryData},
		Playlists:     map[string][]string{"default": {"seg-binary-test"}},
		KVI:           json.RawMessage(`{}`),
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("failed to marshal manifest with binary data: %v", err)
	}

	outputStr := string(data)
	for i, r := range outputStr {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			t.Errorf("found C0 control character U+%04X at position %d in serialized output", r, i)
		}
		if r >= 0x7F && r <= 0x9F {
			t.Errorf("found C1 control character U+%04X at position %d in serialized output", r, i)
		}
	}

	var mfMap map[string]interface{}
	if err := json.Unmarshal(data, &mfMap); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	sanitizeManifestMap(mfMap)

	reEncoded, err := json.Marshal(mfMap)
	if err != nil {
		t.Fatalf("failed to re-marshal sanitized map: %v", err)
	}

	reStr := string(reEncoded)
	if strings.Contains(reStr, "\x01") || strings.Contains(reStr, "\x02") ||
		strings.Contains(reStr, "\x03") || strings.Contains(reStr, "\x04") || strings.Contains(reStr, "\x05") {
		t.Error("sanitized output still contains raw binary bytes")
	}
	if !strings.Contains(reStr, "(non-printable data)") {
		t.Error("expected sanitized output to contain placeholder for non-printable data")
	}
}
