package video

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeFFProbeOutput_RemovesUTF8BOM(t *testing.T) {
	cleanJSON := `{"streams":[],"format":{}}`
	bomJSON := "\xEF\xBB\xBF" + cleanJSON

	result, warning, err := sanitizeFFProbeOutput([]byte(bomJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(result) != cleanJSON {
		t.Errorf("expected %q, got %q", cleanJSON, string(result))
	}
	if !strings.Contains(warning, "UTF-8 BOM") {
		t.Errorf("expected UTF-8 BOM warning, got %q", warning)
	}

	var v interface{}
	if err := json.Unmarshal(result, &v); err != nil {
		t.Errorf("sanitized JSON should be valid: %v", err)
	}
}

func TestSanitizeFFProbeOutput_RemovesTrailingCommas(t *testing.T) {
	input := `{"streams":[{"codec_type":"video",}],"format":{"duration":"10.0",},}`
	result, warning, err := sanitizeFFProbeOutput([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(warning, "trailing commas") {
		t.Errorf("expected trailing comma warning, got %q", warning)
	}

	var v map[string]interface{}
	if err := json.Unmarshal(result, &v); err != nil {
		t.Errorf("sanitized JSON should be valid after removing trailing commas: %v (result=%q)", err, string(result))
	}
}

func TestSanitizeFFProbeOutput_DetectsTruncatedJSON(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "unclosed object",
			input:   `{"streams":[]`,
			wantErr: true,
		},
		{
			name:    "unclosed array",
			input:   `{"streams":[`,
			wantErr: true,
		},
		{
			name:    "unclosed nested",
			input:   `{"streams":[{"codec_type":"video"}`,
			wantErr: true,
		},
		{
			name:    "extra closing brace",
			input:   `{"streams":[]}}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := sanitizeFFProbeOutput([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("sanitizeFFProbeOutput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSanitizeFFProbeOutput_PassesCleanJSON(t *testing.T) {
	input := `{
		"streams": [
			{
				"index": 0,
				"codec_name": "h264",
				"codec_type": "video",
				"width": 1920,
				"height": 1080
			},
			{
				"index": 1,
				"codec_name": "aac",
				"codec_type": "audio"
			}
		],
		"format": {
			"format_name": "mov,mp4,m4a",
			"duration": "120.5",
			"size": "10485760"
		}
	}`

	result, warning, err := sanitizeFFProbeOutput([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error for clean JSON: %v", err)
	}
	if warning != "" {
		t.Errorf("expected no warnings for clean JSON, got %q", warning)
	}
	if string(result) != input {
		t.Errorf("clean JSON should pass through unchanged")
	}

	var raw struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(result, &raw); err != nil {
		t.Errorf("failed to unmarshal sanitized clean JSON: %v", err)
	}
	if len(raw.Streams) != 2 {
		t.Errorf("expected 2 streams, got %d", len(raw.Streams))
	}
	if raw.Format.Duration != "120.5" {
		t.Errorf("expected duration 120.5, got %s", raw.Format.Duration)
	}
}
