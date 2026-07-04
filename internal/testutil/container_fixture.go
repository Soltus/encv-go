package testutil

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/pluginsext"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/internal/v2/writer"
)

const fixturePassword = "fixture-password"

// ContainerFixture 封装一个完整测试容器的所有信息
type ContainerFixture struct {
	Path         string
	Password     string
	OriginalData []byte
	Manifest     *types.Manifest
	FragCount    int
}

// V4ContainerFixture 封装 V4 容器的额外信息
type V4ContainerFixture struct {
	ContainerFixture
	ManifestV4 *types.Manifest_v4
	HeaderV4   *types.EnvelopeHeaderV4
}

// fixtureKVI 测试专用 KVI 实现，满足 types.KVIProvider 接口
type fixtureKVI struct {
	types.KVI
}

func (k fixtureKVI) GetKind() types.IndexKind { return "video" }

func (k fixtureKVI) GetEncryptionInfo() types.KVI { return k.KVI }

func (k fixtureKVI) GetIndex() types.Index { return &fixtureIndex{} }

// fixtureIndex 测试专用的 Index 实现
type fixtureIndex struct{}

func (i *fixtureIndex) GetOriginalFilename() string { return "fixture_video.mp4" }
func (i *fixtureIndex) GetOriginalFileSize() int64  { return 0 }
func (i *fixtureIndex) GetOriginalFileMD5() string  { return "" }
func (i *fixtureIndex) GetEncryptedFileMD5() string { return "" }
func (i *fixtureIndex) GetMimeType() string         { return "video/mp4" }

// RandomBytes 使用 crypto/rand 生成指定长度的随机字节
func RandomBytes(size int64) []byte {
	data := make([]byte, size)
	rand.Read(data)
	return data
}

// CreateV3Fixture 在 t.TempDir() 中生成完整的 V3 容器文件
//
// 流程：生成随机数据 → 加密 → 创建 KVI → 创建 Fragments → 创建 Manifest → 写入容器
func CreateV3Fixture(t testing.TB, dataSize int64, fragCount int) *ContainerFixture {
	t.Helper()

	tempDir := t.TempDir()
	originalData := RandomBytes(dataSize)

	salt, _ := crypto.GenerateSalt_v2(types.SaltSize_v2)
	iv, _ := crypto.GenerateIV_v2(types.IVSize_v2)
	key := crypto.GenerateKey(fixturePassword, salt, types.KeySize_v2)

	var encryptedBuf bytes.Buffer
	crypto.EncryptStream_v2(bytes.NewReader(originalData), &encryptedBuf, key, iv)

	kvi := &fixtureKVI{
		KVI: types.KVI{
			SaltBase64: crypto.Base64Encode_v2(salt),
			IVBase64:   crypto.Base64Encode_v2(iv),
		},
	}

	fragmentSize := dataSize / int64(fragCount)
	if fragmentSize == 0 {
		fragmentSize = dataSize
		fragCount = 1
	}
	fragments := make([]types.Fragment, 0, fragCount)
	var offset uint64
	for i := 0; i < fragCount; i++ {
		size := fragmentSize
		if i == fragCount-1 {
			size = dataSize - int64(offset)
		}
		fragments = append(fragments, types.Fragment{
			ID:                fmt.Sprintf("logical_fragment_%d", i),
			Type:              types.FragmentType_SeekableStream,
			Length:            uint64(size),
			GlobalStartOffset: offset,
		})
		offset += uint64(size)
	}

	manifest, _ := types.NewManifest(kvi, fragments)

	header, _ := types.CreateHeaderV3(true, types.IDType_Raw, nil)
	containerPath := filepath.Join(tempDir, "fixture_v3.sccgv")

	w, err := writer.NewSingleFileContainerWriter(containerPath, header)
	if err != nil {
		t.Fatalf("CreateV3Fixture: failed to create writer: %v", err)
	}

	kviBytes, _ := json.Marshal(kvi)
	if err := w.WriteKVI(kviBytes); err != nil {
		t.Fatalf("CreateV3Fixture: failed to write KVI: %v", err)
	}

	encPayload := encryptedBuf.Bytes()
	saltIVSize := len(salt) + len(iv)
	payloadData := encPayload[saltIVSize:]

	written := uint64(0)
	for _, frag := range fragments {
		end := written + frag.Length
		if end > uint64(len(payloadData)) {
			end = uint64(len(payloadData))
		}
		chunk := payloadData[written:end]
		if err := w.WriteFragment(&frag, chunk); err != nil {
			t.Fatalf("CreateV3Fixture: failed to write fragment %s: %v", frag.ID, err)
		}
		written = end
	}

	if err := w.WriteManifest(manifest); err != nil {
		t.Fatalf("CreateV3Fixture: failed to write manifest: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("CreateV3Fixture: failed to close writer: %v", err)
	}

	return &ContainerFixture{
		Path:         containerPath,
		Password:     fixturePassword,
		OriginalData: originalData,
		Manifest:     manifest,
		FragCount:    fragCount,
	}
}

// CreateV4Fixture 在 t.TempDir() 中生成完整的 V4 容器文件
//
// 使用 writer.WriteV4Container 写入，每个 segment 独立加密
func CreateV4Fixture(t testing.TB, dataSize int64, segCount int) *V4ContainerFixture {
	t.Helper()

	tempDir := t.TempDir()

	salt, _ := crypto.GenerateSalt_v2(types.SaltSize_v2)
	key := crypto.GenerateKey(fixturePassword, salt, types.KeySize_v2)
	macKey := crypto.DeriveMACKey(fixturePassword, bytes.Repeat([]byte{0xAB}, crypto.MACSaltLength))

	segSize := dataSize / int64(segCount)
	if segSize == 0 {
		segSize = dataSize
		segCount = 1
	}

	allOriginalData := make([]byte, 0, dataSize)
	segResults := make([]*crypto.SegmentEncryptionResult, 0, segCount)
	segments := make([]types.Segment_v4, 0, segCount)
	playlistIDs := make([]string, 0, segCount)

	for i := 0; i < segCount; i++ {
		size := segSize
		if i == segCount-1 {
			size = dataSize - int64(len(allOriginalData))
		}
		segData := RandomBytes(size)
		allOriginalData = append(allOriginalData, segData...)

		encResult, err := crypto.EncryptSegment(segData, key, macKey, uint32(i), crypto.CompressionModeNone)
		if err != nil {
			t.Fatalf("CreateV4Fixture: EncryptSegment %d failed: %v", i, err)
		}
		encResult.SegmentID = uint32(i)
		segResults = append(segResults, encResult)

		segID := fmt.Sprintf("seg-%d", i)
		segments = append(segments, types.Segment_v4{
			ID:        segID,
			StartTime: float64(i * 10),
			Duration:  10,
		})
		playlistIDs = append(playlistIDs, segID)
	}

	kviRaw := makeTestKVI_v4(salt)
	manifest := &types.Manifest_v4{
		Version:       4,
		ContainerID:   "fixture-container",
		ContainerType: "video",
		IsSeekable:    true,
		Segments:      segments,
		Playlists:     map[string][]string{"default": playlistIDs},
		KVI:           kviRaw,
	}

	containerPath := filepath.Join(tempDir, "fixture_v4"+pluginsext.VideoExt)

	err := writer.WriteV4Container(&writer.V4WriteParams{
		OutputPath:     containerPath,
		IsMain:         true,
		ContainerType:  types.ContainerTypeVideo,
		IsSeekable:     true,
		IDType:         types.IDType_Raw,
		Manifest:       manifest,
		SegmentResults: segResults,
	})
	if err != nil {
		t.Fatalf("CreateV4Fixture: WriteV4Container failed: %v", err)
	}

	f, err := os.Open(containerPath)
	if err != nil {
		t.Fatalf("CreateV4Fixture: failed to open container for header read: %v", err)
	}
	header, err := types.ReadHeaderV4(f)
	f.Close()
	if err != nil {
		t.Fatalf("CreateV4Fixture: failed to read back v4 header: %v", err)
	}

	return &V4ContainerFixture{
		ContainerFixture: ContainerFixture{
			Path:         containerPath,
			Password:     fixturePassword,
			OriginalData: allOriginalData,
			FragCount:    segCount,
		},
		ManifestV4: manifest,
		HeaderV4:   header,
	}
}

// CreateCorruptedFixture 接收一个已有 fixture，截断其 footer 使其损坏
// 用于测试 ContainerManager 的重建路径
func CreateCorruptedFixture(t testing.TB, original *ContainerFixture) *ContainerFixture {
	t.Helper()

	data, err := os.ReadFile(original.Path)
	if err != nil {
		t.Fatalf("CreateCorruptedFixture: failed to read original: %v", err)
	}

	cutoff := len(data) - int(types.EnvelopeFooterSize_v2) - 16
	if cutoff <= 0 {
		cutoff = len(data) / 2
	}

	corruptedPath := filepath.Join(filepath.Dir(original.Path), "corrupted_fixture.sccgv")
	if err := os.WriteFile(corruptedPath, data[:cutoff], 0644); err != nil {
		t.Fatalf("CreateCorruptedFixture: failed to write corrupted file: %v", err)
	}

	return &ContainerFixture{
		Path:         corruptedPath,
		Password:     original.Password,
		OriginalData: original.OriginalData,
		Manifest:     original.Manifest,
		FragCount:    original.FragCount,
	}
}

func makeTestKVI_v4(salt []byte) json.RawMessage {
	kvi := map[string]string{
		"salt_base64": base64.StdEncoding.EncodeToString(salt),
		"iv_base64":   base64.StdEncoding.EncodeToString(make([]byte, crypto.IVSize_v2)),
	}
	data, _ := json.Marshal(kvi)
	return data
}
