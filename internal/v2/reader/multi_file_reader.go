// internal/v2/reader/multi_file_reader.go
package reader

import (
	"io"
	"os"
)

// MultiFileReader 按顺序读取多个文件的 io.Reader 实现
type MultiFileReader struct {
	paths   []string
	current int
	file    *os.File
}

// NewMultiFileReader 创建一个新的多文件读取器
func NewMultiFileReader(paths []string) *MultiFileReader {
	return &MultiFileReader{
		paths:   paths,
		current: -1,
	}
}

// Read 实现 io.Reader 接口，按顺序读取所有文件
func (m *MultiFileReader) Read(p []byte) (n int, err error) {
	// 如果当前没有打开的文件，打开下一个
	if m.file == nil {
		m.current++
		if m.current >= len(m.paths) {
			return 0, io.EOF
		}
		m.file, err = os.Open(m.paths[m.current])
		if err != nil {
			return 0, err
		}
	}

	// 从当前文件读取
	n, err = m.file.Read(p)
	if err == io.EOF {
		// 当前文件读完，关闭并准备读取下一个
		m.file.Close()
		m.file = nil
		// 如果还有更多文件，继续读取
		if m.current < len(m.paths)-1 {
			return n, nil
		}
		// 所有文件读完
		return n, io.EOF
	}
	return n, err
}

// Close 关闭当前打开的文件
func (m *MultiFileReader) Close() error {
	if m.file != nil {
		err := m.file.Close()
		m.file = nil
		return err
	}
	return nil
}
