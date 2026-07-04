package handle

import (
	"bytes"
	"io"
	"os"
)

type ContainerSource interface {
	io.ReaderAt
	io.Seeker
	Size() int64
	Name() string
}

type FileSource struct {
	file *os.File
	size int64
}

func NewFileSource(filePath string) (*FileSource, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &FileSource{file: f, size: fi.Size()}, nil
}

func (s *FileSource) ReadAt(p []byte, off int64) (int, error) {
	return s.file.ReadAt(p, off)
}

func (s *FileSource) Seek(off int64, whence int) (int64, error) {
	return s.file.Seek(off, whence)
}

func (s *FileSource) Size() int64 {
	return s.size
}

func (s *FileSource) Name() string {
	return s.file.Name()
}

func (s *FileSource) Close() error {
	return s.file.Close()
}

type BytesSource struct {
	reader *bytes.Reader
	name   string
	size   int64
}

func NewBytesSource(data []byte, name string) *BytesSource {
	return &BytesSource{
		reader: bytes.NewReader(data),
		name:   name,
		size:   int64(len(data)),
	}
}

func (s *BytesSource) ReadAt(p []byte, off int64) (int, error) {
	return s.reader.ReadAt(p, off)
}

func (s *BytesSource) Seek(off int64, whence int) (int64, error) {
	return s.reader.Seek(off, whence)
}

func (s *BytesSource) Size() int64 {
	return s.size
}

func (s *BytesSource) Name() string {
	return s.name
}

type RemoteSource struct {
	url     string
	headers map[string]string
}
