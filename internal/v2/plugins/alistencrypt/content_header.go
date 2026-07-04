package alistencrypt

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const (
	AECTR2Magic       = "AECTR2"
	contentHeaderSize = 32
	magicLen          = 6
)

type ContentHeader struct {
	Magic      string
	Version    byte
	Reserved   byte
	NonceField []byte
	PlainSize  int64
}

func DetectContentHeader(data []byte) (*ContentHeader, error) {
	if len(data) < contentHeaderSize {
		return nil, fmt.Errorf("insufficient data for content header: got %d bytes, need %d", len(data), contentHeaderSize)
	}

	magic := string(data[:magicLen])
	if magic != AECTR2Magic {
		return nil, fmt.Errorf("%w: expected magic %q, got %q", ErrInvalidFormat, AECTR2Magic, magic)
	}

	version := data[6]
	if version != 0x02 {
		return nil, fmt.Errorf("unsupported content version: 0x%02x", version)
	}

	reserved := data[7]
	if reserved != 0x00 {
		return nil, fmt.Errorf("unexpected reserved byte: 0x%02x", reserved)
	}

	nonceField := make([]byte, 16)
	copy(nonceField, data[8:24])

	plainSize := int64(binary.BigEndian.Uint64(data[24:32]))
	if plainSize <= 0 {
		return nil, fmt.Errorf("invalid plaintext size in content header: %d", plainSize)
	}

	return &ContentHeader{
		Magic:      magic,
		Version:    version,
		Reserved:   reserved,
		NonceField: nonceField,
		PlainSize:  plainSize,
	}, nil
}

func IsV2Format(data []byte) bool {
	if len(data) < magicLen {
		return false
	}
	return bytes.Equal(data[:magicLen], []byte(AECTR2Magic))
}
