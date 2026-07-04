package video

import (
	"crypto/rand"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/testutil"
	"github.com/Soltus/encv-go/internal/v2/types"
)

const securityTestDataSize int64 = 64 * 1024

type verifyTestPair struct {
	originalPath  string
	decryptedPath string
}

func setupVerifyPair(t *testing.T, origData, decData []byte) verifyTestPair {
	t.Helper()
	dir := t.TempDir()

	origPath := filepath.Join(dir, "original.bin")
	if err := os.WriteFile(origPath, origData, 0644); err != nil {
		t.Fatalf("failed to write original file: %v", err)
	}

	decPath := filepath.Join(dir, "decrypted.bin")
	if err := os.WriteFile(decPath, decData, 0644); err != nil {
		t.Fatalf("failed to write decrypted file: %v", err)
	}

	return verifyTestPair{originalPath: origPath, decryptedPath: decPath}
}

func newVerifier() *VideoContentVerifier {
	return &VideoContentVerifier{}
}

func TestVerify_TamperedHeaderMagic(t *testing.T) {
	verifier := newVerifier()

	t.Run("V3_Container_HeaderMagic_Corrupted", func(t *testing.T) {
		fixture := testutil.CreateV3Fixture(t, securityTestDataSize, 4)
		containerData, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read v3 container: %v", err)
		}

		tampered := make([]byte, len(containerData))
		copy(tampered, containerData)
		tampered[0] = 'F'
		tampered[1] = 'A'
		tampered[2] = 'K'
		tampered[3] = 'E'

		pair := setupVerifyPair(t, fixture.OriginalData, tampered)
		err, _ = verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Error("expected Verify to detect tampered header magic (ENVC->FAKE), got nil")
		}
	})

	t.Run("V4_Container_HeaderMagic_Corrupted", func(t *testing.T) {
		fixture := testutil.CreateV4Fixture(t, securityTestDataSize, 4)
		containerData, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read v4 container: %v", err)
		}

		tampered := make([]byte, len(containerData))
		copy(tampered, containerData)
		tampered[0] = 'F'
		tampered[1] = 'A'
		tampered[2] = 'K'
		tampered[3] = 'E'

		pair := setupVerifyPair(t, fixture.OriginalData, tampered)
		err, _ = verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Error("expected Verify to detect tampered V4 header magic, got nil")
		}
	})

	t.Run("DecryptedOutput_FirstBytes_Corrupted", func(t *testing.T) {
		origData := testutil.RandomBytes(securityTestDataSize)
		decData := make([]byte, len(origData))
		copy(decData, origData)
		decData[0] ^= 0xFF
		decData[1] ^= 0xFF
		decData[2] ^= 0xFF
		decData[3] ^= 0xFF

		pair := setupVerifyPair(t, origData, decData)
		err, _ := verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Error("expected Verify to detect corrupted first 4 bytes of decrypted output, got nil")
		}
	})
}

func TestVerify_TamperedDataSegment(t *testing.T) {
	verifier := newVerifier()

	t.Run("V3_DataSegment_ByteFlip", func(t *testing.T) {
		fixture := testutil.CreateV3Fixture(t, securityTestDataSize, 4)
		containerData, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read v3 container: %v", err)
		}

		headerSize := int(types.EnvelopeHeaderSize_v3)
		blockHeaderSize := int(14)
		dataStart := headerSize + blockHeaderSize

		if dataStart >= len(containerData) {
			t.Fatalf("container too small to locate data segment (size=%d)", len(containerData))
		}

		flipOffset := dataStart + len(containerData)/4
		if flipOffset >= len(containerData) {
			flipOffset = dataStart + 10
		}

		tampered := make([]byte, len(containerData))
		copy(tampered, containerData)
		tampered[flipOffset] ^= 0xFF

		pair := setupVerifyPair(t, fixture.OriginalData, tampered)
		err, _ = verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Errorf("expected Verify to detect tampered data segment at offset %d, got nil", flipOffset)
		}
	})

	t.Run("V4_SegmentData_ByteFlip", func(t *testing.T) {
		fixture := testutil.CreateV4Fixture(t, securityTestDataSize, 4)
		containerData, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read v4 container: %v", err)
		}

		headerSize := int(types.EnvelopeHeaderSize_v4)
		segHeaderSize := binary.Size(types.SegmentHeader{})
		dataRegionStart := headerSize + segHeaderSize

		if dataRegionStart >= len(containerData) {
			t.Fatalf("container too small (size=%d)", len(containerData))
		}

		flipOffset := dataRegionStart + len(containerData)/4
		if flipOffset >= len(containerData) {
			flipOffset = dataRegionStart + 10
		}

		tampered := make([]byte, len(containerData))
		copy(tampered, containerData)
		tampered[flipOffset] ^= 0xFF

		pair := setupVerifyPair(t, fixture.OriginalData, tampered)
		err, _ = verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Errorf("expected Verify to detect tampered V4 segment data at offset %d, got nil", flipOffset)
		}
	})

	t.Run("MultipleRandomFlips_In_DecryptedOutput", func(t *testing.T) {
		origData := testutil.RandomBytes(securityTestDataSize)
		decData := make([]byte, len(origData))
		copy(decData, origData)

		flipPositions := []int{100, 5000, 20000, 45000}
		for _, pos := range flipPositions {
			if pos < len(decData) {
				decData[pos] ^= 0xAB
			}
		}

		pair := setupVerifyPair(t, origData, decData)
		err, _ := verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Error("expected Verify to detect multiple byte flips in decrypted output, got nil")
		}
	})
}

func TestVerify_TamperedManifest(t *testing.T) {
	verifier := newVerifier()

	t.Run("V3_ManifestRegion_Modified", func(t *testing.T) {
		fixture := testutil.CreateV3Fixture(t, securityTestDataSize, 4)
		containerData, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read v3 container: %v", err)
		}

		searchStart := len(containerData) - int(types.EnvelopeFooterSize_v2) - 4096
		if searchStart < 0 {
			searchStart = 0
		}

		manifestOffset := -1
		for i := searchStart; i < len(containerData)-4; i++ {
			if containerData[i] == '{' && containerData[i+1] == '"' {
				candidate := string(containerData[i:min(i+16, len(containerData))])
				if containsManifestMarker(candidate) {
					manifestOffset = i
					break
				}
			}
		}

		if manifestOffset == -1 {
			footerStart := len(containerData) - int(types.EnvelopeFooterSize_v2)
			manifestOffset = footerStart / 2
		}

		tampered := make([]byte, len(containerData))
		copy(tampered, containerData)
		tampered[manifestOffset] ^= 0x01

		pair := setupVerifyPair(t, fixture.OriginalData, tampered)
		err, _ = verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Errorf("expected Verify to detect tampered manifest region near offset %d, got nil", manifestOffset)
		}
	})

	t.Run("DecryptedOutput_JSONManifest_Corrupted", func(t *testing.T) {
		origData := []byte(`{"version":3,"fragments":[{"id":"f0","length":1024},{"id":"f1","length":1024}]}`)
		decData := make([]byte, len(origData))
		copy(decData, origData)
		jsonStart := indexOf(decData, []byte(`"`))
		if jsonStart >= 0 && jsonStart+1 < len(decData) {
			decData[jsonStart+1] = 'X'
		}

		pair := setupVerifyPair(t, origData, decData)
		err, _ := verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Error("expected Verify to detect tampered JSON manifest content, got nil")
		}
	})
}

func TestVerify_AppendedGarbageData(t *testing.T) {
	verifier := newVerifier()

	t.Run("V3_Container_Appended_1024Bytes_Garbage", func(t *testing.T) {
		fixture := testutil.CreateV3Fixture(t, securityTestDataSize, 4)
		containerData, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read v3 container: %v", err)
		}

		garbage := make([]byte, 1024)
		rand.Read(garbage)
		appended := append(containerData, garbage...)

		pair := setupVerifyPair(t, fixture.OriginalData, appended)
		err, _ = verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Error("expected Verify to detect 1024 bytes of appended garbage data, got nil")
		}
	})

	t.Run("V4_Container_Appended_1024Bytes_Garbage", func(t *testing.T) {
		fixture := testutil.CreateV4Fixture(t, securityTestDataSize, 4)
		containerData, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read v4 container: %v", err)
		}

		garbage := make([]byte, 1024)
		rand.Read(garbage)
		appended := append(containerData, garbage...)

		pair := setupVerifyPair(t, fixture.OriginalData, appended)
		err, _ = verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Error("expected Verify to detect appended garbage on V4 container, got nil")
		}
	})

	t.Run("IdenticalFiles_Appended_Garbage_SizeMismatch", func(t *testing.T) {
		origData := testutil.RandomBytes(securityTestDataSize)
		garbage := make([]byte, 1024)
		rand.Read(garbage)
		decData := append(origData, garbage...)

		pair := setupVerifyPair(t, origData, decData)
		err, _ := verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Error("expected Verify to fail with size mismatch due to appended garbage, got nil")
		}
	})
}

func TestVerify_TamperedCRC32(t *testing.T) {
	verifier := newVerifier()

	t.Run("V3_Block_CRC32_Mismatch", func(t *testing.T) {
		fixture := testutil.CreateV3Fixture(t, securityTestDataSize, 4)
		containerData, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read v3 container: %v", err)
		}

		headerSize := int(types.EnvelopeHeaderSize_v3)
		blockHeaderSize := 14

		var offset int64 = int64(headerSize)
		found := false
		for offset < int64(len(containerData))-int64(blockHeaderSize) {
			blockType := types.ByteOrder_v2.Uint16(containerData[offset : offset+2])
			dataLen := types.ByteOrder_v2.Uint64(containerData[offset+2 : offset+10])

			if blockType == uint16(types.BlockTypeData_v2) && dataLen > 0 && int(offset)+blockHeaderSize+int(dataLen) <= len(containerData) {
				dataStart := int(offset) + blockHeaderSize
				tamperPos := dataStart + int(dataLen)/2
				if tamperPos >= len(containerData) {
					tamperPos = dataStart
				}

				tampered := make([]byte, len(containerData))
				copy(tampered, containerData)
				tampered[tamperPos] ^= 0xFF

				pair := setupVerifyPair(t, fixture.OriginalData, tampered)
				err, _ = verifier.Verify(pair.originalPath, pair.decryptedPath)
				if err == nil {
					t.Errorf("expected Verify to detect CRC32 mismatch after data tampering at offset %d, got nil", tamperPos)
				}
				found = true
				break
			}

			offset += int64(blockHeaderSize) + int64(dataLen)
			if dataLen == 0 {
				offset += 1
			}
		}

		if !found {
			t.Skip("could not locate a data block in V3 container for CRC32 test")
		}
	})

	t.Run("V4_SegmentHeader_CRC32_Tampered", func(t *testing.T) {
		fixture := testutil.CreateV4Fixture(t, securityTestDataSize, 4)
		containerData, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read v4 container: %v", err)
		}

		headerSize := int(types.EnvelopeHeaderSize_v4)

		// DataCRC32 字段在 SegmentHeader 末尾 - 4 字节处（v4 升级布局 34 字节中 offset 18）
		segHeaderSize := int(types.SegmentHeaderSize)
		if headerSize+segHeaderSize > len(containerData) {
			t.Fatalf("V4 container too small for segment header (size=%d)", len(containerData))
		}

		crc32FieldOffset := headerSize + segHeaderSize - 4

		tampered := make([]byte, len(containerData))
		copy(tampered, containerData)
		tampered[crc32FieldOffset] ^= 0xFF
		tampered[crc32FieldOffset+1] ^= 0xFF
		tampered[crc32FieldOffset+2] ^= 0xFF
		tampered[crc32FieldOffset+3] ^= 0xFF

		pair := setupVerifyPair(t, fixture.OriginalData, tampered)
		err, _ = verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Error("expected Verify to detect tampered V4 segment header CRC32 field, got nil")
		}
	})

	t.Run("SingleByteChange_Detected_ByHashCheck", func(t *testing.T) {
		origData := testutil.RandomBytes(securityTestDataSize)
		decData := make([]byte, len(origData))
		copy(decData, origData)
		decData[len(decData)-1] ^= 0x01

		pair := setupVerifyPair(t, origData, decData)
		err, _ := verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Error("expected Verify to detect single-byte change via hash mismatch, got nil")
		}
	})
}

func TestVerify_TruncatedFile(t *testing.T) {
	verifier := newVerifier()

	t.Run("V3_Container_Truncated_Half", func(t *testing.T) {
		fixture := testutil.CreateV3Fixture(t, securityTestDataSize, 4)
		containerData, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read v3 container: %v", err)
		}

		cutoff := len(containerData) / 2
		truncated := containerData[:cutoff]

		pair := setupVerifyPair(t, fixture.OriginalData, truncated)
		err, _ = verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Errorf("expected Verify to detect truncated V3 container (cutoff=%d), got nil", cutoff)
		}
	})

	t.Run("V4_Container_Truncated_Half", func(t *testing.T) {
		fixture := testutil.CreateV4Fixture(t, securityTestDataSize, 4)
		containerData, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read v4 container: %v", err)
		}

		cutoff := len(containerData) / 2
		truncated := containerData[:cutoff]

		pair := setupVerifyPair(t, fixture.OriginalData, truncated)
		err, _ = verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Errorf("expected Verify to detect truncated V4 container (cutoff=%d), got nil", cutoff)
		}
	})

	t.Run("Truncated_By_1Byte", func(t *testing.T) {
		origData := testutil.RandomBytes(securityTestDataSize)
		truncated := origData[:len(origData)-1]

		pair := setupVerifyPair(t, origData, truncated)
		err, _ := verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Error("expected Verify to detect single-byte truncation, got nil")
		}
	})

	t.Run("Truncated_To_Zero_Length", func(t *testing.T) {
		origData := testutil.RandomBytes(securityTestDataSize)
		truncated := []byte{}

		pair := setupVerifyPair(t, origData, truncated)
		err, _ := verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Error("expected Verify to detect zero-length truncated file, got nil")
		}
	})

	t.Run("Truncated_FooterRemoved_V3", func(t *testing.T) {
		fixture := testutil.CreateV3Fixture(t, securityTestDataSize, 4)
		containerData, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read v3 container: %v", err)
		}

		cutoff := len(containerData) - int(types.EnvelopeFooterSize_v2)
		if cutoff <= 0 {
			cutoff = len(containerData) / 2
		}
		truncated := containerData[:cutoff]

		pair := setupVerifyPair(t, fixture.OriginalData, truncated)
		err, _ = verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Errorf("expected Verify to detect missing V3 footer (cutoff=%d), got nil", cutoff)
		}
	})

	t.Run("Truncated_FooterRemoved_V4", func(t *testing.T) {
		fixture := testutil.CreateV4Fixture(t, securityTestDataSize, 4)
		containerData, err := os.ReadFile(fixture.Path)
		if err != nil {
			t.Fatalf("failed to read v4 container: %v", err)
		}

		cutoff := len(containerData) - int(types.EnvelopeFooterSize_v4)
		if cutoff <= 0 {
			cutoff = len(containerData) / 2
		}
		truncated := containerData[:cutoff]

		pair := setupVerifyPair(t, fixture.OriginalData, truncated)
		err, _ = verifier.Verify(pair.originalPath, pair.decryptedPath)
		if err == nil {
			t.Errorf("expected Verify to detect missing V4 footer (cutoff=%d), got nil", cutoff)
		}
	})
}

func containsMarker(s string, marker string) bool {
	for i := 0; i <= len(s)-len(marker); i++ {
		if s[i:i+len(marker)] == marker {
			return true
		}
	}
	return false
}

func containsManifestMarker(candidate string) bool {
	return containsMarker(candidate, `"version"`) ||
		containsMarker(candidate, `"fragments"`) ||
		containsMarker(candidate, `"segments"`)
}

func indexOf(data []byte, target []byte) int {
	for i := 0; i <= len(data)-len(target); i++ {
		match := true
		for j := 0; j < len(target); j++ {
			if data[i+j] != target[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
