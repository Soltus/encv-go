package manifest

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/types"
)

func makeBenchManifest(fragCount int) *types.Manifest {
	fragments := make([]types.Fragment, fragCount)
	for i := 0; i < fragCount; i++ {
		fragments[i] = types.Fragment{
			ID:                fmt.Sprintf("logical_fragment_%d", i),
			Type:              types.FragmentType_SeekableStream,
			Length:            uint64(1024 * 1024),
			GlobalStartOffset: uint64(i) * 1024 * 1024,
			PhysicalOffset:    uint64(i) * (1024*1024 + 32),
			DataCRC32:         uint32(i),
		}
	}

	kviData, _ := json.Marshal(types.KVI{
		SaltBase64: "dGVzdHNhbHQ=",
		IVBase64:   "dGVzdGl2",
	})

	return &types.Manifest{
		Version:   types.ManifestSchemaVersion,
		Kind:      "video",
		KVI:       kviData,
		Fragments: fragments,
	}
}

func BenchmarkSerializeManifest(b *testing.B) {
	fragCounts := []int{1, 10, 100, 500}

	for _, count := range fragCounts {
		b.Run(fmt.Sprintf("frags=%d", count), func(b *testing.B) {
			m := makeBenchManifest(count)

			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				_, _ = m.SerializeToJSON()
			}
		})
	}
}

func BenchmarkDeserializeManifest(b *testing.B) {
	fragCounts := []int{1, 10, 100, 500}

	for _, count := range fragCounts {
		b.Run(fmt.Sprintf("frags=%d", count), func(b *testing.B) {
			m := makeBenchManifest(count)
			data, _ := m.SerializeToJSON()

			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()

			for b.Loop() {
				_, _ = DeserializeFromJSON(data)
			}
		})
	}
}

func BenchmarkEncryptManifest(b *testing.B) {
	fragCounts := []int{1, 10, 100}

	for _, count := range fragCounts {
		b.Run(fmt.Sprintf("frags=%d", count), func(b *testing.B) {
			m := makeBenchManifest(count)
			data, _ := m.SerializeToJSON()

			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()

			for b.Loop() {
				_, _ = EncryptManifest(data)
			}
		})
	}
}

func BenchmarkDecryptManifest(b *testing.B) {
	fragCounts := []int{1, 10, 100}

	for _, count := range fragCounts {
		b.Run(fmt.Sprintf("frags=%d", count), func(b *testing.B) {
			m := makeBenchManifest(count)
			data, _ := m.SerializeToJSON()
			encData, _ := EncryptManifest(data)

			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			b.ResetTimer()

			for b.Loop() {
				_, _ = DecryptManifest(encData)
			}
		})
	}
}
