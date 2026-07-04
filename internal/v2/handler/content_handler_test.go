package handler

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Soltus/encv-go/internal/testutil"
	"github.com/stretchr/testify/assert"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

func TestParseRangeHeader_Empty(t *testing.T) {
	start, end, statusCode := parseRangeHeader("", 1000)
	assert.Equal(t, int64(0), start)
	assert.Equal(t, int64(999), end)
	assert.Equal(t, http.StatusOK, statusCode)
}

func TestParseRangeHeader_ValidRange(t *testing.T) {
	start, end, statusCode := parseRangeHeader("bytes=0-499", 1000)
	assert.Equal(t, int64(0), start)
	assert.Equal(t, int64(499), end)
	assert.Equal(t, http.StatusPartialContent, statusCode)
}

func TestParseRangeHeader_OpenEnd(t *testing.T) {
	start, end, statusCode := parseRangeHeader("bytes=500-", 1000)
	assert.Equal(t, int64(500), start)
	assert.Equal(t, int64(999), end)
	assert.Equal(t, http.StatusPartialContent, statusCode)
}

func TestParseRangeHeader_InvalidStart(t *testing.T) {
	start, end, statusCode := parseRangeHeader("bytes=9999-", 1000)
	assert.Equal(t, int64(0), start)
	assert.Equal(t, int64(999), end)
	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, statusCode)
}

func TestParseRangeHeader_StartGtEnd(t *testing.T) {
	start, end, statusCode := parseRangeHeader("bytes=800-700", 1000)
	assert.Equal(t, int64(0), start)
	assert.Equal(t, int64(999), end)
	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, statusCode)
}

func TestParseRangeHeader_Malformed(t *testing.T) {
	start, end, statusCode := parseRangeHeader("bytes=abc", 1000)
	assert.Equal(t, int64(0), start)
	assert.Equal(t, int64(999), end)
	assert.Equal(t, http.StatusOK, statusCode)
}

func TestParseRangeHeader_BadRegex(t *testing.T) {
	start, end, statusCode := parseRangeHeader("garbage", 1000)
	assert.Equal(t, int64(0), start)
	assert.Equal(t, int64(999), end)
	assert.Equal(t, http.StatusOK, statusCode)
}

func TestParseRangeHeader_SingleByte(t *testing.T) {
	start, end, statusCode := parseRangeHeader("bytes=0-0", 100)
	assert.Equal(t, int64(0), start)
	assert.Equal(t, int64(0), end)
	assert.Equal(t, http.StatusPartialContent, statusCode)
}

func TestParseRangeHeader_LastByte(t *testing.T) {
	start, end, statusCode := parseRangeHeader("bytes=99-", 100)
	assert.Equal(t, int64(99), start)
	assert.Equal(t, int64(99), end)
	assert.Equal(t, http.StatusPartialContent, statusCode)
}

// ==================== ServeFile 集成测试 ====================

// MockSeekerTo 实现 provider.SeekerTo 接口，用于测试降级路径
type MockSeekerTo struct {
	Offset int64
	Err    error
}

func (m *MockSeekerTo) SeekTo(offset int64) error {
	m.Offset = offset
	return m.Err
}

func TestServeFile_FullRequest_NoRange(t *testing.T) {
	data := bytes.Repeat([]byte("Hello World"), 100)[:1000]
	prov := testutil.NewMockProvider(data, "test.txt")

	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/file.txt", nil)

	h.ServeFile(w, r, prov)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "1000", w.Header().Get("Content-Length"))
	assert.Equal(t, data, w.Body.Bytes())
}

func TestServeFile_RangeRequest_Valid(t *testing.T) {
	data := bytes.Repeat([]byte("A"), 1000)
	prov := testutil.NewMockProvider(data, "test.bin")

	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/file.bin", nil)
	r.Header.Set("Range", "bytes=0-499")

	h.ServeFile(w, r, prov)

	assert.Equal(t, http.StatusPartialContent, w.Code)
	assert.Equal(t, "bytes 0-499/1000", w.Header().Get("Content-Range"))
	assert.Equal(t, data[:500], w.Body.Bytes())
}

func TestServeFile_RangeRequest_OpenEnd(t *testing.T) {
	data := bytes.Repeat([]byte("B"), 1000)
	prov := testutil.NewMockProvider(data, "test.bin")

	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/file.bin", nil)
	r.Header.Set("Range", "bytes=500-")

	h.ServeFile(w, r, prov)

	assert.Equal(t, http.StatusPartialContent, w.Code)
	assert.Equal(t, data[500:], w.Body.Bytes())
}

func TestServeFile_RangeRequest_SingleByte(t *testing.T) {
	data := bytes.Repeat([]byte("X"), 100)
	prov := testutil.NewMockProvider(data, "test.bin")

	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/file.bin", nil)
	r.Header.Set("Range", "bytes=50-50")

	h.ServeFile(w, r, prov)

	assert.Equal(t, http.StatusPartialContent, w.Code)
	assert.Equal(t, 1, w.Body.Len())
	assert.Equal(t, []byte("X"), w.Body.Bytes())
}

func TestServeFile_InvalidRange_Overflow(t *testing.T) {
	data := bytes.Repeat([]byte("Y"), 100)
	prov := testutil.NewMockProvider(data, "test.bin")

	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/file.bin", nil)
	r.Header.Set("Range", "bytes=9999-")

	h.ServeFile(w, r, prov)

	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, w.Code)
}

func TestServeFile_NonSeekableWithNonZeroRange(t *testing.T) {
	data := bytes.Repeat([]byte("Z"), 100)
	reader := io.NopCloser(bytes.NewReader(data))
	prov := &testutil.MockFileContentProvider{
		ReaderVal:   reader,
		SeekerVal:   nil,
		HasSeeker:   false,
		HasSeekerTo: false,
		SizeVal:     100,
		NameVal:     "test.bin",
	}

	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/file.bin", nil)
	r.Header.Set("Range", "bytes=10-20")

	h.ServeFile(w, r, prov)

	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, w.Code)
}

func TestServeFile_SeekableProvider_SeeksCorrectly(t *testing.T) {
	data := bytes.Repeat([]byte("S"), 1000)
	br := bytes.NewReader(data)
	prov := &testutil.MockFileContentProvider{
		ReaderVal:   io.NopCloser(br),
		SeekerVal:   br,
		HasSeeker:   true,
		HasSeekerTo: false,
		SizeVal:     1000,
		NameVal:     "seekable.bin",
	}

	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/seekable.bin", nil)
	r.Header.Set("Range", "bytes=100-199")

	h.ServeFile(w, r, prov)

	assert.Equal(t, http.StatusPartialContent, w.Code)
	body := w.Body.Bytes()
	assert.Equal(t, 100, len(body))
	assert.Equal(t, data[100:200], body)
}

func TestServeFile_ContentHeadersSet(t *testing.T) {
	data := []byte("header-check-content")
	prov := testutil.NewMockProvider(data, "video.mp4")

	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/video.mp4", nil)

	h.ServeFile(w, r, prov)

	ct := w.Header().Get("Content-Type")
	cd := w.Header().Get("Content-Disposition")
	ar := w.Header().Get("Accept-Ranges")

	assert.NotEmpty(t, ct, "应设置 Content-Type")
	assert.Contains(t, cd, "video.mp4", "Content-Disposition 应包含文件名")
	assert.Equal(t, "bytes", ar, "Accept-Ranges 应为 bytes")
}

func TestServeFile_SeekerTo_Fallback(t *testing.T) {
	data := bytes.Repeat([]byte("D"), 1000)
	mockSeekerTo := &MockSeekerTo{}

	prov := &testutil.MockFileContentProvider{
		ReaderVal:   io.NopCloser(bytes.NewReader(data)),
		SeekerVal:   nil,
		SeekerToVal: mockSeekerTo,
		HasSeeker:   false,
		HasSeekerTo: true,
		SizeVal:     1000,
		NameVal:     "stream.dat",
	}

	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/stream.dat", nil)
	r.Header.Set("Range", "bytes=0-499")

	h.ServeFile(w, r, prov)

	assert.Equal(t, int64(0), mockSeekerTo.Offset, "SeekerTo.SeekTo 应被调用且 offset=0")
	assert.Equal(t, http.StatusPartialContent, w.Code)
	assert.Equal(t, 500, w.Body.Len())
}
