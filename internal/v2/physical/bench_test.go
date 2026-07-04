package physical

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/namer"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/internal/v2/writer"
)

func TestMain(m *testing.M) {
	types.RegisterKVIProvider("video", func(rawKVI json.RawMessage) (types.KVIProvider, error) {
		var kvi types.KVI
		if err := json.Unmarshal(rawKVI, &kvi); err != nil {
			return nil, fmt.Errorf("failed to unmarshal KVI: %w", err)
		}
		return physBenchKVI{KVI: kvi}, nil
	})
	m.Run()
}

type physBenchKVI struct {
	types.KVI
}

func (k physBenchKVI) GetKind() types.IndexKind     { return "video" }
func (k physBenchKVI) GetEncryptionInfo() types.KVI { return k.KVI }
func (k physBenchKVI) GetIndex() types.Index {
	return &physBenchIndex{size: 0}
}

type physBenchIndex struct{ size int64 }

func (i *physBenchIndex) GetOriginalFilename() string { return "bench_video.mp4" }
func (i *physBenchIndex) GetOriginalFileSize() int64  { return i.size }
func (i *physBenchIndex) GetOriginalFileMD5() string  { return "" }
func (i *physBenchIndex) GetEncryptedFileMD5() string { return "" }
func (i *physBenchIndex) GetMimeType() string         { return "video/mp4" }

// benchMaxDataSize 根据环境变量决定最大测试数据尺寸
func benchMaxDataSize() int64 {
	if os.Getenv("ENCV_BENCH_LOW_MEM") != "" {
		return 1 * 1024 * 1024 // 低内存模式：最大 1MB
	}
	return 5 * 1024 * 1024 // 正常模式：最大 5MB
}

func createPhysBenchManifest(dataSize int64, fragCount int) (*types.Manifest, []byte, string) {
	password := "bench-phys-password"
	salt, _ := crypto.GenerateSalt_v2(types.SaltSize_v2)
	iv, _ := crypto.GenerateIV_v2(types.IVSize_v2)
	key := crypto.GenerateKey(password, salt, types.KeySize_v2)

	originalData := make([]byte, dataSize)
	rand.Read(originalData)

	encData, _ := crypto.EncryptBytes_v2(originalData, key, iv)
	payloadData := encData

	kvi := &physBenchKVI{
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
	return manifest, payloadData, password
}

func BenchmarkSinglePhysicalPacker_Pack(b *testing.B) {
	maxSize := benchMaxDataSize()
	sizes := []int64{1 * 1024 * 1024}
	if maxSize >= 5*1024*1024 {
		sizes = append(sizes, 5*1024*1024)
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%dMB", size/(1024*1024)), func(b *testing.B) {
			manifest, payloadData, _ := createPhysBenchManifest(size, 10)

			// 在循环外创建一个临时目录，循环内复用并手动清理
			workDir := b.TempDir()

			b.SetBytes(size)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				packer := NewSinglePhysicalPacker()

				req := &PackRequest{
					EncryptedDataReader: bytes.NewReader(payloadData),
					OutputDir:           workDir,
					FinalFileName:       "bench_pack.sccgv",
					HeaderVersion:       3,
					SpecialIDType:       types.IDType_Raw,
					SpecialID:           nil,
				}

				resultPath, err := packer.Pack(manifest, req)
				if err != nil {
					b.Fatal(err)
				}
				os.Remove(resultPath)
			}
		})
	}
}

func BenchmarkSinglePhysicalUnpacker_Unpack(b *testing.B) {
	maxSize := benchMaxDataSize()
	sizes := []int64{1 * 1024 * 1024}
	if maxSize >= 5*1024*1024 {
		sizes = append(sizes, 5*1024*1024)
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%dMB", size/(1024*1024)), func(b *testing.B) {
			manifest, payloadData, _ := createPhysBenchManifest(size, 10)
			containerPath := buildSingleContainer(b, manifest, payloadData)

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				unpacker := NewSinglePhysicalUnpacker()
				path, cleanup, err := unpacker.Unpack(b.Context(), containerPath)
				if err != nil {
					b.Fatal(err)
				}
				cleanup()
				_ = path
			}
		})
	}
}

func BenchmarkFileChunkerPhysicalPacker_Pack(b *testing.B) {
	maxSize := benchMaxDataSize()
	dataSize := int64(1 * 1024 * 1024)
	if maxSize >= 5*1024*1024 {
		dataSize = 5 * 1024 * 1024
	}

	chunkSizes := []int64{512 * 1024}
	if maxSize >= 5*1024*1024 {
		chunkSizes = append(chunkSizes, 2*1024*1024)
	}

	for _, chunkSize := range chunkSizes {
		b.Run(fmt.Sprintf("size=%dMB/chunk=%dMB", dataSize/(1024*1024), chunkSize/(1024*1024)), func(b *testing.B) {
			manifest, payloadData, _ := createPhysBenchManifest(dataSize, 10)
			bn := namer.NewDefaultBaseNamer()
			chunkNamer := namer.NewSequentialNamer(".sccgv", bn, ".part")

			// 循环外创建临时目录，循环内复用并手动清理
			workDir := b.TempDir()

			b.SetBytes(dataSize)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				packer := NewFileChunkerPhysicalPacker(chunkSize, chunkNamer)

				req := &PackRequest{
					EncryptedDataReader: bytes.NewReader(payloadData),
					BaseName:            "bench_chunk",
					OutputDir:           workDir,
					Namer:               chunkNamer,
					HeaderVersion:       3,
					SpecialIDType:       types.IDType_Raw,
					SpecialID:           nil,
				}

				resultPath, err := packer.Pack(manifest, req)
				if err != nil {
					b.Fatal(err)
				}

				// 手动清理分片文件，避免磁盘堆积
				dir := filepath.Dir(resultPath)
				entries, _ := os.ReadDir(dir)
				for _, e := range entries {
					os.Remove(filepath.Join(dir, e.Name()))
				}
			}
		})
	}
}

func BenchmarkFileChunkerPhysicalUnpacker_Unpack(b *testing.B) {
	maxSize := benchMaxDataSize()
	dataSize := int64(5 * 1024 * 1024)
	if maxSize < 5*1024*1024 {
		dataSize = 1 * 1024 * 1024
	}
	chunkSize := int64(2 * 1024 * 1024)

	b.Run(fmt.Sprintf("size=%dMB", dataSize/(1024*1024)), func(b *testing.B) {
		manifest, payloadData, _ := createPhysBenchManifest(dataSize, 10)
		bn := namer.NewDefaultBaseNamer()
		chunkNamer := namer.NewSequentialNamer(".sccgv", bn, ".part")

		packDir := b.TempDir()
		packer := NewFileChunkerPhysicalPacker(chunkSize, chunkNamer)
		req := &PackRequest{
			EncryptedDataReader: bytes.NewReader(payloadData),
			BaseName:            "bench_chunk",
			OutputDir:           packDir,
			Namer:               chunkNamer,
			HeaderVersion:       3,
			SpecialIDType:       types.IDType_Raw,
			SpecialID:           nil,
		}
		mainPath, err := packer.Pack(manifest, req)
		if err != nil {
			b.Fatal(err)
		}

		unpacker := NewFileChunkerPhysicalUnpacker(chunkNamer)

		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			path, cleanup, err := unpacker.Unpack(mainPath)
			if err != nil {
				b.Fatal(err)
			}
			cleanup()
			_ = path
		}
	})
}

func buildSingleContainer(b *testing.B, manifest *types.Manifest, payloadData []byte) string {
	b.Helper()

	tempDir := b.TempDir()
	containerPath := filepath.Join(tempDir, "bench_unpack.sccgv")

	header, _ := types.CreateHeaderV3(true, types.IDType_Raw, nil)
	w, err := writer.NewSingleFileContainerWriter(containerPath, header)
	if err != nil {
		b.Fatalf("failed to create writer: %v", err)
	}

	kviBytes, _ := json.Marshal(manifest.KVI)
	if err := w.WriteKVI(kviBytes); err != nil {
		b.Fatalf("failed to write KVI: %v", err)
	}

	written := uint64(0)
	for _, frag := range manifest.Fragments {
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

var _ io.Reader = bytes.NewReader(nil)
