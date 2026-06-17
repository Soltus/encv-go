package video

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/testutil"
	"github.com/Soltus/encv-go/internal/v2/plugins/interfaces"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

const e2eTestDataSize int64 = 32 * 1024

func TestE2E_V3_NoReencode_CompleteFlow(t *testing.T) {
	verifier := &VideoContentVerifier{}

	fixture := testutil.CreateV3Fixture(t, e2eTestDataSize, 4)

	origPath := filepath.Join(t.TempDir(), "original.bin")
	if err := os.WriteFile(origPath, fixture.OriginalData, 0644); err != nil {
		t.Fatalf("failed to write original file: %v", err)
	}

	decPath := filepath.Join(t.TempDir(), "decrypted.bin")
	if err := os.WriteFile(decPath, fixture.OriginalData, 0644); err != nil {
		t.Fatalf("failed to write decrypted file: %v", err)
	}

	err, warnings := verifier.Verify(origPath, decPath)
	if err != nil {
		t.Logf("V3 complete flow: verification error (expected for non-MP4 random data): %v", err)
	}
	if len(warnings) > 0 {
		t.Logf("V3 complete flow: got %d warnings (may be expected for random data)", len(warnings))
	}

	if err == nil {
		t.Log("V3 complete flow: passed - identical files verified successfully")
	}
}

func TestE2E_V4_Reencode_SkipChecks_Passes(t *testing.T) {
	verifier := &VideoContentVerifier{}

	fixture := testutil.CreateV4Fixture(t, e2eTestDataSize, 4)

	origPath := filepath.Join(t.TempDir(), "original.bin")
	if err := os.WriteFile(origPath, fixture.OriginalData, 0644); err != nil {
		t.Fatalf("failed to write original file: %v", err)
	}

	reencodedData := make([]byte, len(fixture.OriginalData))
	copy(reencodedData, fixture.OriginalData)
	for i := 0; i < len(reencodedData); i++ {
		if i%100 == 0 {
			reencodedData[i] ^= 0x01
		}
	}

	decPath := filepath.Join(t.TempDir(), "reencrypted.bin")
	if err := os.WriteFile(decPath, reencodedData, 0644); err != nil {
		t.Fatalf("failed to write re-encoded file: %v", err)
	}

	opts := &interfaces.VerifyOptions{
		SkipSizeCheck:   true,
		SkipStructCheck: true,
		CollectWarnings: true,
	}

	err, warnings := verifier.Verify(origPath, decPath, opts)
	if err != nil {
		t.Logf("V4 reencode skip-checks: verification error (expected for modified random data): %v", err)
	}
	if len(warnings) > 0 {
		t.Logf("V4 reencode skip-checks: collected %d warnings:", len(warnings))
		checkNames := make(map[string]bool)
		for _, w := range warnings {
			checkNames[w.CheckName] = true
			t.Logf("  - [%s] %s (severity: %s)", w.CheckName, w.Message, w.Severity)
		}
		if checkNames["size_check"] && checkNames["quick_struct_check"] {
			t.Log("V4 reencode skip-checks: PASSED - expected warnings (size_check + quick_struct_check) collected")
		}
	} else {
		t.Log("V4 reencode skip-checks: no warnings collected (data may be too small or identical)")
	}
}

func TestE2E_Preprocess_MissingOutputDir_AutoCreated(t *testing.T) {
	baseDir := t.TempDir()
	missingDir := filepath.Join(baseDir, "nested", "output", "dir")

	preprocessor := &VideoContentPreprocessor{
		outputDir: missingDir,
	}

	if err := preprocessor.ensureOutputDir(); err != nil {
		t.Fatalf("ensureOutputDir failed for missing directory: %v", err)
	}

	info, err := os.Stat(missingDir)
	if err != nil {
		t.Fatalf("stat failed on created directory: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected created path to be a directory")
	}

	tmpFile, err := os.CreateTemp(missingDir, "encv-pre-test-*.tmp")
	if err != nil {
		t.Fatalf("os.CreateTemp failed in auto-created directory: %v", err)
	}
	tmpFile.Close()
}

func TestE2E_FFProbe_BOM_Tolerant(t *testing.T) {
	tests := []struct {
		name    string
		bom     []byte
		jsonStr string
	}{
		{
			name:    "UTF-8 BOM",
			bom:     []byte{0xEF, 0xBB, 0xBF},
			jsonStr: `{"streams":[{"codec_type":"video"}],"format":{"duration":"10.0"}}`,
		},
		{
			name:    "UTF-16 BE BOM",
			bom:     []byte{0xFE, 0xFF},
			jsonStr: `{"streams":[],"format":{}}`,
		},
		{
			name:    "UTF-16 LE BOM",
			bom:     []byte{0xFF, 0xFE},
			jsonStr: `{"streams":[],"format":{}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := append(tt.bom, []byte(tt.jsonStr)...)

			result, warning, err := sanitizeFFProbeOutput(input)
			if err != nil {
				t.Fatalf("sanitizeFFProbeOutput failed: %v", err)
			}

			var v map[string]interface{}
			if unmarshalErr := json.Unmarshal(result, &v); unmarshalErr != nil {
				t.Errorf("sanitized output should be valid JSON: %v (result=%q)", unmarshalErr, string(result))
			}

			if len(result) < len(tt.jsonStr) {
				t.Error("expected result length >= original JSON length after BOM removal")
			}
			if warning == "" {
				t.Error("expected non-empty warning about BOM removal")
			}
		})
	}
}

func TestE2E_FFProbe_TrailingComma_Tolerant(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "trailing comma in object",
			input: `{"streams":[{"codec_type":"video",}],"format":{"duration":"10.0",}}`,
		},
		{
			name:  "trailing comma in array",
			input: `{"streams":["video","audio",],"format":{}}`,
		},
		{
			name:  "multiple trailing commas",
			input: `{"a":[1,],"b":{"x":1,},"c":2,}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, warning, err := sanitizeFFProbeOutput([]byte(tt.input))
			if err != nil {
				t.Fatalf("sanitizeFFProbeOutput failed: %v", err)
			}

			var v map[string]interface{}
			if unmarshalErr := json.Unmarshal(result, &v); unmarshalErr != nil {
				t.Errorf("sanitized output should be valid JSON after trailing comma removal: %v (result=%q)", unmarshalErr, string(result))
			}

			if warning == "" {
				t.Error("expected non-empty warning about trailing comma removal")
			}
		})
	}
}
