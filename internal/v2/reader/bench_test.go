package reader

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/crypto"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/Soltus/encv-go/internal/v2/writer"
)

func TestMain(m *testing.M) {
	for _, kind := range []types.IndexKind{"text", "image", "video"} {
		kind := kind
		types.RegisterKVIProvider(kind, func(rawKVI json.RawMessage) (types.KVIProvider, error) {
			var kvi types.KVI
			if err := json.Unmarshal(rawKVI, &kvi); err != nil {
				return nil, fmt.Errorf("failed to unmarshal KVI: %w", err)
			}
			return &benchKVI{KVI: kvi, testKind: kind}, nil
		})
	}
	m.Run()
}

// benchReaderMaxSize 根据环境变量决定最大测试数据尺寸
func benchReaderMaxSize() int64 {
	if os.Getenv("ENCV_BENCH_LOW_MEM") != "" {
		return 1 * 1024 * 1024 // 低内存模式：最大 1MB
	}
	return 10 * 1024 * 1024 // 正常模式：最大 10MB
}

// containerFixture 封装一个完整的测试容器及其元数据
type containerFixture struct {
	ContainerPath string
	Password      string
	DataSize      int64
	OriginalData  []byte
	Manifest      *types.Manifest
	FragCount     int
	cleanup       func()
}

// createContainerFixture 生成一个完整的加密容器文件
// 整个过程使用项目自身的代码，零外部依赖
func createContainerFixture(tb testing.TB, dataSize int64, fragCount int) *containerFixture {
	tb.Helper()

	password := "bench-test-password"
	tempDir := tb.TempDir()

	originalData := make([]byte, dataSize)
	rand.Read(originalData)

	salt, _ := crypto.GenerateSalt_v2(types.SaltSize_v2)
	iv, _ := crypto.GenerateIV_v2(types.IVSize_v2)
	key := crypto.GenerateKey(password, salt, types.KeySize_v2)

	var encryptedBuf bytes.Buffer
	crypto.EncryptStream_v2(bytes.NewReader(originalData), &encryptedBuf, key, iv)

	kvi := &benchKVI{
		KVI: types.KVI{
			SaltBase64: crypto.Base64Encode_v2(salt),
			IVBase64:   crypto.Base64Encode_v2(iv),
		},
		testKind: "video",
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
	containerPath := filepath.Join(tempDir, "bench_container.sccgv")

	w, err := writer.NewSingleFileContainerWriter(containerPath, header)
	if err != nil {
		tb.Fatalf("failed to create writer: %v", err)
	}

	kviBytes, _ := json.Marshal(kvi)
	if err := w.WriteKVI(kviBytes); err != nil {
		tb.Fatalf("failed to write KVI: %v", err)
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
			tb.Fatalf("failed to write fragment %s: %v", frag.ID, err)
		}
		written = end
	}

	if err := w.WriteManifest(manifest); err != nil {
		tb.Fatalf("failed to write manifest: %v", err)
	}
	if err := w.Close(); err != nil {
		tb.Fatalf("failed to close writer: %v", err)
	}

	return &containerFixture{
		ContainerPath: containerPath,
		Password:      password,
		DataSize:      dataSize,
		OriginalData:  originalData,
		Manifest:      manifest,
		FragCount:     fragCount,
	}
}

// benchKVI 是测试专用的 KVI 实现
type benchKVI struct {
	types.KVI
	testKind types.IndexKind
}

func (k benchKVI) GetKind() types.IndexKind { return k.testKind }

func (k benchKVI) GetEncryptionInfo() types.KVI {
	return k.KVI
}

func (k benchKVI) GetIndex() types.Index {
	return &benchIndex{size: 0}
}

type benchIndex struct {
	size int64
}

func (i *benchIndex) GetOriginalFilename() string { return "bench_video.mp4" }
func (i *benchIndex) GetOriginalFileSize() int64  { return i.size }
func (i *benchIndex) GetOriginalFileMD5() string  { return "" }
func (i *benchIndex) GetEncryptedFileMD5() string { return "" }
func (i *benchIndex) GetMimeType() string         { return "video/mp4" }

// --- L3 基准测试 ---

func BenchmarkDecryptReaderFactory_ParseAndCache(b *testing.B) {
	fixture := createContainerFixture(b, 10*1024*1024, 10)
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		factory, err := NewDecryptReaderFactory(fixture.ContainerPath, fixture.Password)
		if err != nil {
			b.Fatal(err)
		}
		factory.Close()
	}
}

func BenchmarkDecryptReaderFactory_NewDecryptReader(b *testing.B) {
	fixture := createContainerFixture(b, 10*1024*1024, 10)
	factory, err := NewDecryptReaderFactory(fixture.ContainerPath, fixture.Password)
	if err != nil {
		b.Fatal(err)
	}
	defer factory.Close()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		r, err := factory.NewDecryptReader()
		if err != nil {
			b.Fatal(err)
		}
		r.Close()
	}
}

func BenchmarkSequentialDecryptReader_Read(b *testing.B) {
	maxSize := benchReaderMaxSize()
	sizes := []int64{1 * 1024 * 1024}
	if maxSize >= 10*1024*1024 {
		sizes = append(sizes, 10*1024*1024)
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%s", humanBytes(size)), func(b *testing.B) {
			fixture := createContainerFixture(b, size, 1)

			factory, err := NewDecryptReaderFactory(fixture.ContainerPath, fixture.Password)
			if err != nil {
				b.Fatal(err)
			}
			defer factory.Close()

			b.SetBytes(size)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				r, err := factory.NewDecryptReader()
				if err != nil {
					b.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, r)
				r.Close()
			}
		})
	}
}

func BenchmarkSequentialSeekableDecryptReader_Read(b *testing.B) {
	maxSize := benchReaderMaxSize()
	sizes := []int64{1 * 1024 * 1024}
	if maxSize >= 10*1024*1024 {
		sizes = append(sizes, 10*1024*1024)
	}
	fragCounts := []int{1, 10}
	if maxSize >= 10*1024*1024 {
		fragCounts = append(fragCounts, 100)
	}

	for _, size := range sizes {
		for _, fragCount := range fragCounts {
			b.Run(fmt.Sprintf("size=%s/frags=%d", humanBytes(size), fragCount), func(b *testing.B) {
				fixture := createContainerFixture(b, size, fragCount)

				factory, err := NewDecryptReaderFactory(fixture.ContainerPath, fixture.Password)
				if err != nil {
					b.Fatal(err)
				}
				defer factory.Close()

				b.SetBytes(size)
				b.ReportAllocs()
				b.ResetTimer()

				for b.Loop() {
					r, err := factory.NewDecryptReader()
					if err != nil {
						b.Fatal(err)
					}
					_, _ = io.Copy(io.Discard, r)
					r.Close()
				}
			})
		}
	}
}

func BenchmarkVirtualSeekableDecryptReader_Seek(b *testing.B) {
	fixture := createContainerFixture(b, 10*1024*1024, 10)

	cr, err := NewEncryptedContainerReaderFromFile(fixture.ContainerPath)
	if err != nil {
		b.Fatal(err)
	}
	defer cr.Close()

	dr, err := NewVirtualSeekableDecryptReader(cr, fixture.Password)
	if err != nil {
		b.Fatal(err)
	}
	defer dr.Close()

	seeker := dr.(io.Seeker)

	positions := []struct {
		name   string
		offset int64
	}{
		{"head", 0},
		{"25%", fixture.DataSize / 4},
		{"50%", fixture.DataSize / 2},
		{"75%", fixture.DataSize * 3 / 4},
		{"tail", fixture.DataSize - 1024},
	}

	for _, pos := range positions {
		b.Run(fmt.Sprintf("pos=%s", pos.name), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, err := seeker.Seek(pos.offset, io.SeekStart)
				if err != nil {
					b.Fatal(err)
				}
				buf := make([]byte, 1024)
				_, _ = dr.Read(buf)
			}
		})
	}
}

func BenchmarkSequentialSeekableDecryptReader_Seek(b *testing.B) {
	fixture := createContainerFixture(b, 10*1024*1024, 10)

	cr, err := NewEncryptedContainerReaderFromFile(fixture.ContainerPath)
	if err != nil {
		b.Fatal(err)
	}
	defer cr.Close()

	dr, err := NewSequentialSeekableDecryptReader(cr, fixture.Password)
	if err != nil {
		b.Fatal(err)
	}
	defer dr.Close()

	seeker := dr.(io.Seeker)

	positions := []struct {
		name   string
		offset int64
	}{
		{"head", 0},
		{"25%", fixture.DataSize / 4},
		{"50%", fixture.DataSize / 2},
		{"75%", fixture.DataSize * 3 / 4},
		{"tail", fixture.DataSize - 1024},
	}

	for _, pos := range positions {
		b.Run(fmt.Sprintf("pos=%s", pos.name), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, err := seeker.Seek(pos.offset, io.SeekStart)
				if err != nil {
					b.Fatal(err)
				}
				buf := make([]byte, 1024)
				_, _ = dr.Read(buf)
			}
		})
	}
}

func BenchmarkBulkDecryptor_DecryptToFile(b *testing.B) {
	maxSize := benchReaderMaxSize()
	sizes := []int64{1 * 1024 * 1024}
	if maxSize >= 10*1024*1024 {
		sizes = append(sizes, 10*1024*1024)
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%s", humanBytes(size)), func(b *testing.B) {
			fixture := createContainerFixture(b, size, 10)

			// 循环外创建临时目录，避免 b.TempDir() 在循环内堆积
			workDir := b.TempDir()

			b.SetBytes(size)
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				bd := NewBulkDecryptor(fixture.ContainerPath, fixture.Password)
				outPath := filepath.Join(workDir, "decrypted.mp4")
				if err := bd.DecryptToFile(b.Context(), outPath); err != nil {
					b.Fatal(err)
				}
				os.Remove(outPath)
			}
		})
	}
}

func humanBytes(n int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%dGB", n/GB)
	case n >= MB:
		return fmt.Sprintf("%dMB", n/MB)
	case n >= KB:
		return fmt.Sprintf("%dKB", n/KB)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// --- Seek 性能矩阵：不同分片数 ---

func BenchmarkVirtualSeekableDecryptReader_SeekMatrix(b *testing.B) {
	fragCounts := []int{1, 10, 50}
	maxSize := benchReaderMaxSize()
	if maxSize >= 10*1024*1024 {
		fragCounts = append(fragCounts, 200, 500)
	}
	dataSize := maxSize

	for _, fragCount := range fragCounts {
		b.Run(fmt.Sprintf("frags=%d", fragCount), func(b *testing.B) {
			fixture := createContainerFixture(b, dataSize, fragCount)

			cr, err := NewEncryptedContainerReaderFromFile(fixture.ContainerPath)
			if err != nil {
				b.Fatal(err)
			}
			defer cr.Close()

			dr, err := NewVirtualSeekableDecryptReader(cr, fixture.Password)
			if err != nil {
				b.Fatal(err)
			}
			defer dr.Close()

			seeker := dr.(io.Seeker)

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				_, _ = seeker.Seek(dataSize/2, io.SeekStart)
				buf := make([]byte, 1024)
				_, _ = dr.Read(buf)
			}
		})
	}
}

func BenchmarkSequentialSeekableDecryptReader_SeekMatrix(b *testing.B) {
	fragCounts := []int{1, 10, 50}
	maxSize := benchReaderMaxSize()
	if maxSize >= 10*1024*1024 {
		fragCounts = append(fragCounts, 200, 500)
	}
	dataSize := maxSize

	for _, fragCount := range fragCounts {
		b.Run(fmt.Sprintf("frags=%d", fragCount), func(b *testing.B) {
			fixture := createContainerFixture(b, dataSize, fragCount)

			cr, err := NewEncryptedContainerReaderFromFile(fixture.ContainerPath)
			if err != nil {
				b.Fatal(err)
			}
			defer cr.Close()

			dr, err := NewSequentialSeekableDecryptReader(cr, fixture.Password)
			if err != nil {
				b.Fatal(err)
			}
			defer dr.Close()

			seeker := dr.(io.Seeker)

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				_, _ = seeker.Seek(dataSize/2, io.SeekStart)
				buf := make([]byte, 1024)
				_, _ = dr.Read(buf)
			}
		})
	}
}

// --- 并发解密读取基准 ---

func BenchmarkConcurrentDecryptReader(b *testing.B) {
	concurrencyLevels := []int{1, 2, 4}
	maxSize := benchReaderMaxSize()
	if maxSize >= 10*1024*1024 {
		concurrencyLevels = append(concurrencyLevels, 8)
	}
	dataSize := maxSize

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("workers=%d", concurrency), func(b *testing.B) {
			fixture := createContainerFixture(b, dataSize, 10)

			b.SetBytes(dataSize * int64(concurrency))
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				var wg sync.WaitGroup
				errCh := make(chan error, concurrency)

				for i := 0; i < concurrency; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						factory, err := NewDecryptReaderFactory(fixture.ContainerPath, fixture.Password)
						if err != nil {
							errCh <- err
							return
						}
						defer factory.Close()

						r, err := factory.NewDecryptReader()
						if err != nil {
							errCh <- err
							return
						}
						defer r.Close()

						_, _ = io.Copy(io.Discard, r)
					}()
				}

				wg.Wait()
				close(errCh)
				for err := range errCh {
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

func BenchmarkConcurrentSeekRead(b *testing.B) {
	concurrencyLevels := []int{1, 2, 4}
	maxSize := benchReaderMaxSize()
	if maxSize >= 10*1024*1024 {
		concurrencyLevels = append(concurrencyLevels, 8)
	}
	dataSize := maxSize

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("workers=%d", concurrency), func(b *testing.B) {
			fixture := createContainerFixture(b, dataSize, 10)

			cr, err := NewEncryptedContainerReaderFromFile(fixture.ContainerPath)
			if err != nil {
				b.Fatal(err)
			}
			defer cr.Close()

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				var wg sync.WaitGroup
				errCh := make(chan error, concurrency)

				for i := 0; i < concurrency; i++ {
					wg.Add(1)
					go func(idx int) {
						defer wg.Done()
						dr, err := NewVirtualSeekableDecryptReader(cr, fixture.Password)
						if err != nil {
							errCh <- err
							return
						}
						defer dr.Close()

						seeker := dr.(io.Seeker)
						offset := int64(idx) * dataSize / int64(concurrency)
						_, _ = seeker.Seek(offset, io.SeekStart)
						buf := make([]byte, 64*1024)
						_, _ = dr.Read(buf)
					}(i)
				}

				wg.Wait()
				close(errCh)
				for err := range errCh {
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
