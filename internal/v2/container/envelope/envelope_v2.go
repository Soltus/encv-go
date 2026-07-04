// internal/v2/container/envelope_v2.go 信封层
package envelope

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/Soltus/encv-go/internal/v2/types"
)

// ReadEnvelopeFooter_v2 从文件末尾读取 Footer
func ReadEnvelopeFooter_v2(r io.ReadSeeker) (*types.EnvelopeFooter_v2, error) {
	var footer types.EnvelopeFooter_v2

	// Seek to the beginning of the footer
	_, err := r.Seek(-int64(types.EnvelopeFooterSize_v2), io.SeekEnd)
	if err != nil {
		return nil, fmt.Errorf("failed to seek to footer: %w", err)
	}

	err = binary.Read(r, types.ByteOrder_v2, &footer)
	if err != nil {
		return nil, fmt.Errorf("failed to read footer: %w", err)
	}

	if !bytes.Equal(footer.Magic[:], types.MagicFooter_v2[:]) {
		return nil, types.ErrInvalidMagic_v2
	}

	return &footer, nil
}

// ParseEnvelopeFooterFromBytes 从字节切片（通常是从文件结尾）中解析并返回 Footer
// 权威来源：处理 []byte，用于远程流或内存分析
func ParseEnvelopeFooterFromBytes(data []byte) (*types.EnvelopeFooter_v2, error) {
	footerSize := int64(binary.Size(types.EnvelopeFooter_v2{}))
	if int64(len(data)) < footerSize {
		return nil, fmt.Errorf("data is too small to contain a footer")
	}

	// 从末尾切片
	footerData := data[len(data)-int(footerSize):]

	reader := bytes.NewReader(footerData)
	var footer types.EnvelopeFooter_v2
	if err := binary.Read(reader, types.ByteOrder_v2, &footer); err != nil {
		return nil, fmt.Errorf("failed to read footer from bytes: %w", err)
	}

	if footer.Magic != types.MagicFooter_v2 {
		return nil, types.ErrInvalidMagic_v2
	}

	return &footer, nil
}
