package envelope

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/types"
)

func BenchmarkParseEnvelopeFooterFromBytes(b *testing.B) {
	footer := types.EnvelopeFooter_v2{
		Magic:          types.MagicFooter_v2,
		ManifestOffset: 1024,
		ManifestLength: 512,
		ManifestCRC32:  0xABCDEF01,
		GlobalCRC32:    0x12345678,
	}

	footerBytes := make([]byte, types.EnvelopeFooterSize_v2)
	buf := bytes.NewBuffer(footerBytes[:0])
	binary.Write(buf, types.ByteOrder_v2, &footer)
	validData := make([]byte, 1024+types.EnvelopeFooterSize_v2)
	copy(validData[len(validData)-types.EnvelopeFooterSize_v2:], buf.Bytes())

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = ParseEnvelopeFooterFromBytes(validData)
	}
}

func BenchmarkReadEnvelopeFooter_v2(b *testing.B) {
	footer := types.EnvelopeFooter_v2{
		Magic:          types.MagicFooter_v2,
		ManifestOffset: 1024,
		ManifestLength: 512,
		ManifestCRC32:  0xABCDEF01,
		GlobalCRC32:    0x12345678,
	}

	footerBytes := make([]byte, types.EnvelopeFooterSize_v2)
	buf := bytes.NewBuffer(footerBytes[:0])
	binary.Write(buf, types.ByteOrder_v2, &footer)

	fullData := make([]byte, 4096+types.EnvelopeFooterSize_v2)
	copy(fullData[len(fullData)-types.EnvelopeFooterSize_v2:], buf.Bytes())

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		r := bytes.NewReader(fullData)
		_, _ = ReadEnvelopeFooter_v2(r)
	}
}
