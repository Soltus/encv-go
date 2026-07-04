package testutil

import (
	"bytes"
	"io"

	"github.com/Soltus/encv-go/internal/v2/provider"
)

// MockFileContentProvider 实现 provider.FileContentProvider 接口
// 用于测试 ContentHandler 等 HTTP 层代码
type MockFileContentProvider struct {
	ReaderVal   io.ReadCloser
	SeekerVal   io.Seeker
	SeekerToVal provider.SeekerTo
	HasSeeker   bool
	HasSeekerTo bool
	SizeVal     int64
	NameVal     string
	CloseErr    error
}

func (m *MockFileContentProvider) GetReader() io.ReadCloser     { return m.ReaderVal }
func (m *MockFileContentProvider) GetSeeker() (io.Seeker, bool) { return m.SeekerVal, m.HasSeeker }
func (m *MockFileContentProvider) GetSeekerTo() (provider.SeekerTo, bool) {
	return m.SeekerToVal, m.HasSeekerTo
}
func (m *MockFileContentProvider) GetSize() int64  { return m.SizeVal }
func (m *MockFileContentProvider) GetName() string { return m.NameVal }
func (m *MockFileContentProvider) Close() error    { return m.CloseErr }

// NewMockProvider 从 []byte 数据创建一个 FileContentProvider
// 自动包装为 io.ReadCloser + bytes.Reader(实现 io.Seeker)
func NewMockProvider(data []byte, filename string) *MockFileContentProvider {
	r := io.NopCloser(bytes.NewReader(data))
	seeker := bytes.NewReader(data)
	return &MockFileContentProvider{
		ReaderVal:   r,
		SeekerVal:   seeker,
		HasSeeker:   true,
		HasSeekerTo: false,
		SizeVal:     int64(len(data)),
		NameVal:     filename,
	}
}
