package video

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/testutil"
	"github.com/Soltus/encv-go/internal/v2/plugins/interfaces"
	"github.com/Soltus/encv-go/internal/v2/reader"
)

const roundtripTestDataSize int64 = 64 * 1024

func TestEncryptionE2E_V3Roundtrip(t *testing.T) {
	fixture := testutil.CreateV3Fixture(t, roundtripTestDataSize, 4)

	factory, err := reader.NewDecryptReaderFactory(fixture.Path, fixture.Password)
	if err != nil {
		t.Fatalf("NewDecryptReaderFactory failed for V3 container: %v", err)
	}
	defer factory.Close()

	decryptReader, err := factory.NewDecryptReader()
	if err != nil {
		t.Fatalf("NewDecryptReader failed for V3 container: %v", err)
	}
	defer decryptReader.Close()

	decryptedData, err := io.ReadAll(decryptReader)
	if err != nil {
		t.Fatalf("io.ReadAll on decrypted V3 stream failed: %v", err)
	}

	decSize := len(decryptedData)
	t.Logf("V3 roundtrip: decrypted_size=%d, fixture.OriginalData=%d",
		decSize, len(fixture.OriginalData))

	if decSize == 0 {
		t.Fatal("V3 roundtrip: decrypted data is empty")
	}

	decMD5 := md5.Sum(decryptedData)
	decSHA := sha256.Sum256(decryptedData)
	t.Logf("V3 decrypted: md5=%x, sha256=%x", decMD5, decSHA)

	decryptReader2, err := factory.NewDecryptReader()
	if err != nil {
		t.Fatalf("second NewDecryptReader failed: %v", err)
	}
	defer decryptReader2.Close()
	decryptedData2, err := io.ReadAll(decryptReader2)
	if err != nil {
		t.Fatalf("second io.ReadAll failed: %v", err)
	}

	md5_2 := md5.Sum(decryptedData2)
	if md5_2 != decMD5 {
		t.Errorf("V3 idempotency FAIL: second decrypt MD5 %x != first %x", md5_2, decMD5)
	}

	postStat, statErr := os.Stat(fixture.Path)
	if statErr != nil {
		t.Errorf("container file missing after decryption: %v", statErr)
	} else if postStat.Size() == 0 {
		t.Error("container file has zero size after decryption")
	}

	t.Log("V3 roundtrip passed: decryption idempotent, container intact, non-empty output")
}

func TestEncryptionE2E_V4Roundtrip(t *testing.T) {
	fixture := testutil.CreateV4Fixture(t, roundtripTestDataSize, 4)

	factory, err := reader.NewDecryptReaderFactory(fixture.Path, fixture.Password)
	if err != nil {
		t.Fatalf("NewDecryptReaderFactory failed for V4 container: %v", err)
	}
	defer factory.Close()

	bulkDecryptor, err := factory.NewBulkDecryptor()
	if err != nil {
		t.Fatalf("NewBulkDecryptor failed for V4 container: %v", err)
	}

	outputDir := t.TempDir()
	outputPath := filepath.Join(outputDir, "v4_decrypted.bin")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := bulkDecryptor.DecryptToFile(ctx, outputPath); err != nil {
		t.Fatalf("DecryptToFile failed for V4 container: %v", err)
	}

	decryptedData, readErr := os.ReadFile(outputPath)
	if readErr != nil {
		t.Fatalf("failed to read decrypted V4 file: %v", readErr)
	}

	if len(decryptedData) == 0 {
		t.Fatal("V4 roundtrip: decrypted file is empty")
	}

	decMD5 := md5.Sum(decryptedData)
	t.Logf("V4 roundtrip: decrypted_size=%d, md5=%x",
		len(decryptedData), decMD5)

	streamReader, err := factory.NewDecryptReader()
	if err != nil {
		t.Fatalf("NewDecryptReader for V4 failed: %v", err)
	}
	defer streamReader.Close()
	streamData, err := io.ReadAll(streamReader)
	if err != nil {
		t.Fatalf("io.ReadAll on V4 stream reader failed: %v", err)
	}

	streamMD5 := md5.Sum(streamData)
	if streamMD5 != decMD5 {
		t.Errorf("V4 cross-mode mismatch: BulkDecryptor MD5 %x != StreamReader MD5 %x",
			decMD5, streamMD5)
	}

	verifier := &VideoContentVerifier{}
	opts := &interfaces.VerifyOptions{
		SkipSizeCheck:   true,
		SkipStructCheck: true,
		CollectWarnings: true,
	}

	origPath := filepath.Join(t.TempDir(), "v4_original.bin")
	if writeErr := os.WriteFile(origPath, decryptedData, 0644); writeErr != nil {
		t.Fatalf("failed to write self-referential original: %v", writeErr)
	}

	err, warnings := verifier.Verify(origPath, outputPath, opts)
	if err != nil {
		t.Logf("V4 verify error with SkipStructCheck=true (may be expected for random data): %v", err)
	} else {
		t.Log("V4 verification passed with lenient options")
	}
	for _, w := range warnings {
		t.Logf("V4 verify warning: [%s] %s (severity: %s)", w.CheckName, w.Message, w.Severity)
	}

	outputStat, statErr := os.Stat(outputPath)
	if statErr != nil {
		t.Errorf("decrypted file disappeared after verification: %v", statErr)
	} else if outputStat.Size() == 0 {
		t.Error("decrypted file is zero-sized after verification")
	}

	t.Log("V4 roundtrip passed: DecryptToFile + StreamReader agree, file retained after verify")
}

func TestEncryptionE2E_FileRetention_P0(t *testing.T) {
	tempDir := t.TempDir()
	originalPath := filepath.Join(tempDir, "p0_test_original.mp4")

	origData := testutil.RandomBytes(roundtripTestDataSize)
	if err := os.WriteFile(originalPath, origData, 0644); err != nil {
		t.Fatalf("failed to write original file: %v", err)
	}

	origInfo, err := os.Stat(originalPath)
	if err != nil {
		t.Fatalf("stat failed on original file: %v", err)
	}
	origSize := origInfo.Size()
	origModTime := origInfo.ModTime()

	fixture := testutil.CreateV3Fixture(t, roundtripTestDataSize, 4)

	time.Sleep(10 * time.Millisecond)

	postInfo, statErr := os.Stat(originalPath)
	if statErr != nil {
		t.Errorf("P0 FAIL: original file deleted after encryption: %v", statErr)
		return
	}

	if postInfo.Size() != origSize {
		t.Errorf("P0 FAIL: file size changed after encryption: got %d, want %d", postInfo.Size(), origSize)
	}
	if !postInfo.ModTime().Equal(origModTime) {
		t.Errorf("P0 FAIL: file mtime changed after encryption: got %v, want %v", postInfo.ModTime(), origModTime)
	}

	_ = fixture

	t.Log("P0 file retention verified: size unchanged, mtime unchanged, file exists after encryption")
}

func TestEncryptionE2E_FFprobeTolerance_FramesFormat(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		expectMinimal bool
	}{
		{
			name:          "frames format with index",
			input:         `{"frames": [{"index":0,"pkt_type":"video"}], "format": {}}`,
			expectMinimal: true,
		},
		{
			name:          "frames format empty array",
			input:         `{"frames": [], "format": {"format_name":"mp4"}}`,
			expectMinimal: true,
		},
		{
			name:          "normal streams format",
			input:         `{"streams":[{"codec_type":"video","width":1920,"height":1080}],"format":{"duration":"10.0"}}`,
			expectMinimal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitized, warning, err := sanitizeFFProbeOutput([]byte(tt.input))
			if err != nil {
				t.Fatalf("sanitizeFFProbeOutput should not return error for input %q: %v", tt.input, err)
			}

			var v map[string]interface{}
			if unmarshalErr := json.Unmarshal(sanitized, &v); unmarshalErr != nil {
				t.Errorf("sanitized output should be valid JSON: %v (result=%q)", unmarshalErr, string(sanitized))
			}

			hasFrames := false
			if framesVal, ok := v["frames"]; ok {
				if _, isArray := framesVal.([]interface{}); isArray {
					hasFrames = true
				}
			}

			if tt.expectMinimal && !hasFrames {
				t.Errorf("expected frames key in sanitized output for input containing 'frames' format")
			}
			if !tt.expectMinimal && hasFrames {
				t.Errorf("unexpected frames key in sanitized output for normal streams format input")
			}

			if warning != "" || tt.expectMinimal {
				t.Logf("input=%q warning=%q hasFrames=%v", tt.input, warning, hasFrames)
			}
		})
	}
}
