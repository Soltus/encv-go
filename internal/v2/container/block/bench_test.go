package block

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/types"
)

func BenchmarkWriteBlock_v2(b *testing.B) {
	sizes := []int{
		1 * 1024,
		64 * 1024,
		256 * 1024,
		1 * 1024 * 1024,
		4 * 1024 * 1024,
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%s", humanBytes(int64(size))), func(b *testing.B) {
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i % 256)
			}

			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				var buf bytes.Buffer
				_, _ = WriteBlock(&buf, types.BlockTypeData_v2, data)
			}
		})
	}
}

func BenchmarkReadBlockHeader_v2(b *testing.B) {
	data := make([]byte, 1024)
	var buf bytes.Buffer
	WriteBlock(&buf, types.BlockTypeData_v2, data)
	blockBytes := buf.Bytes()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		r := bytes.NewReader(blockBytes)
		_, _ = ReadBlockHeader(r)
	}
}

func BenchmarkReadBlockData_v2(b *testing.B) {
	sizes := []int{
		1 * 1024,
		64 * 1024,
		1 * 1024 * 1024,
	}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size=%s", humanBytes(int64(size))), func(b *testing.B) {
			data := make([]byte, size)
			for i := range data {
				data[i] = byte(i % 256)
			}

			var buf bytes.Buffer
			WriteBlock(&buf, types.BlockTypeData_v2, data)
			blockBytes := buf.Bytes()

			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()

			for b.Loop() {
				r := bytes.NewReader(blockBytes)
				header, _ := ReadBlockHeader(r)
				_, _ = ReadBlockData(r, header)
			}
		})
	}
}

func humanBytes(n int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
	)
	switch {
	case n >= MB:
		return fmt.Sprintf("%dMB", n/MB)
	case n >= KB:
		return fmt.Sprintf("%dKB", n/KB)
	default:
		return fmt.Sprintf("%dB", n)
	}
}
