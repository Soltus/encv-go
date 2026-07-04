package alistencrypt

import (
	"bytes"
	"fmt"
	"io"
)

type DecryptReader struct {
	reader        io.Reader
	seeker        io.Seeker
	cipher        Cipher
	pos           int64
	readerPos     int64
	v2Header      *ContentHeader
	headerSkipped bool
}

func NewDecryptReader(r io.Reader, password string, fileSize int64) (*DecryptReader, error) {
	peekBuf := make([]byte, contentHeaderSize)
	n, err := io.ReadFull(r, peekBuf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	var header *ContentHeader
	var cipher Cipher
	var source io.Reader

	if n >= contentHeaderSize && IsV2Format(peekBuf) {
		header, err = DetectContentHeader(peekBuf)
		if err != nil {
			return nil, fmt.Errorf("failed to parse V2 header: %w", err)
		}
		cipher, err = Create(password, "aesctr", header.PlainSize)
		if err != nil {
			return nil, fmt.Errorf("failed to create cipher: %w", err)
		}
		source = r
	} else {
		cipher, err = Create(password, "aesctr", fileSize)
		if err != nil {
			return nil, fmt.Errorf("failed to create cipher: %w", err)
		}
		if seeker, ok := r.(io.Seeker); ok {
			if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr != nil {
				return nil, fmt.Errorf("failed to seek back to start: %w", seekErr)
			}
			source = r
		} else {
			source = io.MultiReader(bytes.NewReader(peekBuf[:n]), r)
		}
	}

	return &DecryptReader{
		reader:        source,
		seeker:        extractSeeker(r),
		cipher:        cipher,
		pos:           0,
		readerPos:     0,
		v2Header:      header,
		headerSkipped: header == nil,
	}, nil
}

func extractSeeker(r io.Reader) io.Seeker {
	if s, ok := r.(io.Seeker); ok {
		return s
	}
	return nil
}

func (dr *DecryptReader) Read(p []byte) (int, error) {
	if dr.v2Header != nil && !dr.headerSkipped {
		dr.headerSkipped = true
	}

	n, err := dr.reader.Read(p)
	if n > 0 {
		dr.cipher.Decrypt(p[:n])
		dr.pos += int64(n)
		dr.readerPos += int64(n)
	}
	return n, err
}

func (dr *DecryptReader) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = dr.pos + offset
	case io.SeekEnd:
		if dr.v2Header != nil {
			newPos = dr.v2Header.PlainSize + offset
		} else {
			return 0, fmt.Errorf("seek from end not supported for non-V2 format")
		}
	default:
		return 0, fmt.Errorf("invalid whence value")
	}

	if newPos < 0 {
		return 0, fmt.Errorf("negative position")
	}

	if err := dr.cipher.SetPosition(newPos); err != nil {
		return 0, err
	}
	dr.pos = newPos

	if dr.seeker != nil {
		var seekTarget int64
		if dr.v2Header != nil {
			seekTarget = newPos + contentHeaderSize
		} else {
			seekTarget = newPos
		}
		if _, err := dr.seeker.Seek(seekTarget, io.SeekStart); err != nil {
			return 0, fmt.Errorf("failed to seek underlying reader: %w", err)
		}
		dr.readerPos = seekTarget
		if reader, ok := dr.seeker.(io.Reader); ok {
			dr.reader = reader
		}
	}

	return newPos, nil
}

func (dr *DecryptReader) Position() int64 {
	return dr.pos
}
