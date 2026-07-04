package detector

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/internal/v2/writer"
)

func TestMain(m *testing.M) {
	types.RegisterKVIProvider("video", func(rawKVI json.RawMessage) (types.KVIProvider, error) {
		var kvi types.KVI
		if err := json.Unmarshal(rawKVI, &kvi); err != nil {
			return nil, fmt.Errorf("failed to unmarshal KVI: %w", err)
		}
		return benchKVI{KVI: kvi}, nil
	})
	m.Run()
}

// suppressOutput 抑制 fmt.Printf 等输出，返回恢复函数
func suppressOutput() func() {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	go io.Copy(io.Discard, r)
	return func() {
		w.Close()
		os.Stdout = oldStdout
	}
}

type benchKVI struct {
	types.KVI
}

func (k benchKVI) GetKind() types.IndexKind     { return "video" }
func (k benchKVI) GetEncryptionInfo() types.KVI { return k.KVI }
func (k benchKVI) GetIndex() types.Index {
	return &benchIndex{size: 0}
}

type benchIndex struct{ size int64 }

func (i *benchIndex) GetOriginalFilename() string { return "bench_video.mp4" }
func (i *benchIndex) GetOriginalFileSize() int64  { return i.size }
func (i *benchIndex) GetOriginalFileMD5() string  { return "" }
func (i *benchIndex) GetEncryptedFileMD5() string { return "" }
func (i *benchIndex) GetMimeType() string         { return "video/mp4" }

func createDetectorFixture(b *testing.B, dataSize int64, fragCount int) string {
	b.Helper()

	password := "bench-detector-password"
	tempDir := b.TempDir()

	originalData := make([]byte, dataSize)
	rand.Read(originalData)

	salt, _ := crypto.GenerateSalt_v2(types.SaltSize_v2)
	iv, _ := crypto.GenerateIV_v2(types.IVSize_v2)
	key := crypto.GenerateKey(password, salt, types.KeySize_v2)

	var encryptedBuf struct{ WriteTo func() error }
	_ = encryptedBuf

	encData, _ := crypto.EncryptBytes_v2(originalData, key, iv)
	saltIVSize := len(salt) + len(iv)
	payloadData := encData[saltIVSize:]

	kvi := &benchKVI{
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
	containerPath := filepath.Join(tempDir, "bench_detect.sccgv")

	w, err := writer.NewSingleFileContainerWriter(containerPath, header)
	if err != nil {
		b.Fatalf("failed to create writer: %v", err)
	}

	kviBytes, _ := json.Marshal(kvi)
	if err := w.WriteKVI(kviBytes); err != nil {
		b.Fatalf("failed to write KVI: %v", err)
	}

	written := uint64(0)
	for _, frag := range fragments {
		end := written + frag.Length
		if end > uint64(len(payloadData)) {
			end = uint64(len(payloadData))
		}
		chunk := payloadData[written:end]
		if err := w.WriteFragment(&frag, chunk); err != nil {
			b.Fatalf("failed to write fragment %s: %v", frag.ID, err)
		}
		written = end
	}

	if err := w.WriteManifest(manifest); err != nil {
		b.Fatalf("failed to write manifest: %v", err)
	}
	if err := w.Close(); err != nil {
		b.Fatalf("failed to close writer: %v", err)
	}

	return containerPath
}

func BenchmarkDetectContainer(b *testing.B) {
	sizes := []int64{
		1 * 1024 * 1024,
		10 * 1024 * 1024,
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%dMB", size/(1024*1024)), func(b *testing.B) {
			path := createDetectorFixture(b, size, 10)

			restore := suppressOutput()
			defer restore()

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				_, _ = DetectContainer(path)
			}
		})
	}
}

func BenchmarkDetectIndexKind(b *testing.B) {
	path := createDetectorFixture(b, 10*1024*1024, 10)

	restore := suppressOutput()
	defer restore()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = DetectIndexKind(path)
	}
}

func BenchmarkIsEncvContainerFromBytes(b *testing.B) {
	path := createDetectorFixture(b, 10*1024*1024, 10)
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatal(err)
	}

	invalidData := make([]byte, 4096)

	b.Run("valid", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = IsEncvContainerFromBytes(data)
		}
	})

	b.Run("invalid", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = IsEncvContainerFromBytes(invalidData)
		}
	})
}
