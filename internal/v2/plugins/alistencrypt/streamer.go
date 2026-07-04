package alistencrypt

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SeekableDecryptReader 包装 DecryptReader 并提供 io.ReadCloser 语义
// 同时复用 DecryptReader 自身的 io.Seeker（reader.go:80）
type SeekableDecryptReader struct {
	*DecryptReader
	closeFunc func() error
}

func (s *SeekableDecryptReader) Close() error {
	if s.closeFunc != nil {
		return s.closeFunc()
	}
	return nil
}

// resolveShowName decodes the encrypted filename back to its original name.
// Falls back to "orig_<basename>" if the decode fails, matching ConvertShowName's contract.
func (p *AlistEncryptPlugin) resolveShowName(path, password string) string {
	encType := p.settings.EncType
	if encType == "" {
		encType = "aesctr"
	}
	showName := ConvertShowName(filepath.Base(path), password, encType)
	return showName
}

func (p *AlistEncryptPlugin) Stream(path string, password string) (io.ReadCloser, int64, string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, "", "", fmt.Errorf("failed to open file: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, "", "", fmt.Errorf("failed to stat file: %w", err)
	}
	fileSize := info.Size()

	var plainSize int64
	if fileSize >= 32 {
		peekBuf := make([]byte, 32)
		n, _ := io.ReadFull(f, peekBuf)
		if n == 32 && bytes.Equal(peekBuf[:6], []byte(AECTR2Magic)) {
			header, headerErr := DetectContentHeader(peekBuf)
			if headerErr == nil && header != nil {
				plainSize = header.PlainSize
			} else {
				plainSize = fileSize
			}
		} else {
			plainSize = fileSize
		}
		if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
			f.Close()
			return nil, 0, "", "", fmt.Errorf("failed to seek back: %w", seekErr)
		}
	} else {
		plainSize = fileSize
	}

	dr, err := NewDecryptReader(f, password, fileSize)
	if err != nil {
		f.Close()
		return nil, 0, "", "", fmt.Errorf("failed to create decrypt reader: %w", err)
	}

	showName := p.resolveShowName(path, password)
	contentType := detectContentTypeByName(showName, path)

	sr := &SeekableDecryptReader{
		DecryptReader: dr,
		closeFunc:     f.Close,
	}

	return sr, plainSize, contentType, showName, nil
}

// contentTypeByExt returns the canonical MIME type for the given file extension.
// Falls back to application/octet-stream for unknown extensions.
func contentTypeByExt(ext string) string {
	mimeMap := map[string]string{
		".mp4":  "video/mp4",
		".mkv":  "video/x-matroska",
		".avi":  "video/x-msvideo",
		".mov":  "video/quicktime",
		".wmv":  "video/x-ms-wmv",
		".flv":  "video/x-flv",
		".webm": "video/webm",
		".mp3":  "audio/mpeg",
		".flac": "audio/flac",
		".wav":  "audio/wav",
		".ogg":  "audio/ogg",
		".m4a":  "audio/mp4",
		".aac":  "audio/aac",
		".pdf":  "application/pdf",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
		".bmp":  "image/bmp",
		".txt":  "text/plain",
		".html": "text/html",
		".xml":  "application/xml",
		".json": "application/json",
	}
	if ct, ok := mimeMap[strings.ToLower(ext)]; ok {
		return ct
	}
	return "application/octet-stream"
}

// detectContentTypeByName chooses the MIME type using the decoded filename when
// available (so encrypted `.bin` containers carrying, e.g., a real MP4 are
// served as video/mp4 instead of octet-stream). Falls back to the on-disk path's
// extension if decoding didn't yield a useful name.
func detectContentTypeByName(showName, onDiskPath string) string {
	if showName != "" && !strings.HasPrefix(showName, OrigPrefix) {
		if ct := contentTypeByExt(filepath.Ext(showName)); ct != "application/octet-stream" {
			return ct
		}
	}
	return contentTypeByExt(filepath.Ext(onDiskPath))
}

// detectContentType keeps the old behaviour for callers that only have the
// on-disk path (e.g., unit tests asserting the legacy code path).
func detectContentType(path string) string {
	return contentTypeByExt(filepath.Ext(path))
}
