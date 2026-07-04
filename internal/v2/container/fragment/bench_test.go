package fragment

import (
	"fmt"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/types"
)

func BenchmarkCalculateFragmentSize(b *testing.B) {
	sizes := []int64{
		100 * 1024 * 1024,       // 100MB
		1 * 1024 * 1024 * 1024,  // 1GB
		10 * 1024 * 1024 * 1024, // 10GB
		50 * 1024 * 1024 * 1024, // 50GB
	}

	physicalSizes := []int64{
		0,                 // 不分片
		30 * 1024 * 1024,  // 30MB
		100 * 1024 * 1024, // 100MB
	}

	for _, totalSize := range sizes {
		for _, physSize := range physicalSizes {
			b.Run(fmt.Sprintf("total=%s/phys=%s", humanBytes(totalSize), humanBytes(physSize)), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					CalculateFragmentSize(totalSize, physSize)
				}
			})
		}
	}
}

func BenchmarkCreateLogicalFragmentsFromSize(b *testing.B) {
	sizes := []int64{
		100 * 1024 * 1024,
		1 * 1024 * 1024 * 1024,
		10 * 1024 * 1024 * 1024,
	}

	for _, totalSize := range sizes {
		fragSize := CalculateFragmentSize(totalSize, 0)
		b.Run(fmt.Sprintf("total=%s", humanBytes(totalSize)), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = CreateLogicalFragmentsFromSize(totalSize, fragSize, types.FragmentType_SeekableStream)
			}
		})
	}
}

func BenchmarkCreateLogicalFragmentsFromSizeAligned(b *testing.B) {
	totalSize := int64(1 * 1024 * 1024 * 1024) // 1GB
	physicalSizes := []int64{
		0,
		30 * 1024 * 1024,
		100 * 1024 * 1024,
	}

	for _, physSize := range physicalSizes {
		effectivePhysSize := physSize
		if effectivePhysSize == 0 {
			effectivePhysSize = totalSize
		}
		fragSize := CalculateFragmentSize(totalSize, physSize)

		b.Run(fmt.Sprintf("phys=%s/fragSize=%s", humanBytes(physSize), humanBytes(fragSize)), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, _ = CreateLogicalFragmentsFromSizeAligned(totalSize, fragSize, effectivePhysSize, types.FragmentType_SeekableStream)
			}
		})
	}
}

func BenchmarkValidateGlobalStartOffsets(b *testing.B) {
	fragCounts := []int{10, 100, 1000}

	for _, count := range fragCounts {
		b.Run(fmt.Sprintf("frags=%d", count), func(b *testing.B) {
			totalSize := int64(1 * 1024 * 1024 * 1024)
			fragSize := CalculateFragmentSize(totalSize, 0)
			frags, _ := CreateLogicalFragmentsFromSize(totalSize, fragSize, types.FragmentType_SeekableStream)
			if len(frags) > count {
				frags = frags[:count]
			}
			manifest := &types.Manifest_v2{
				Version:   types.ContainerVersion,
				Kind:      "video",
				Fragments: frags,
			}

			b.ReportAllocs()
			for b.Loop() {
				_ = ValidateGlobalStartOffsets(manifest)
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
