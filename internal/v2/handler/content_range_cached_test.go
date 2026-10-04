package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/provider"
	"github.com/Soltus/encv-go/internal/v2/reader"
	"github.com/Soltus/encv-go/internal/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// 强制激活 test-guard：拦截裸 go test 调用
	_ "github.com/Soltus/encv-go/internal/testguard"
)

// ============================================================================
// 回归锁：HTTP Range 必须真的从 start 开始返回（真机拖动 / 边下边播的生命线）
// ----------------------------------------------------------------------------
// 2026-10-01 在「模拟器内真实后端 + 宿主机浏览器」通路上实测到的真 bug：
//   小文件（<= 3MB，走 LocalFileProvider 的内存缓存分支）带 Range 请求时，
//   Content-Range 头写的是 bytes N-M/size，但**实体体从头开始返回**。
//   播放器据此拖动/续传会拿到错位字节 → 解码失败 → 页面"播放失败"。
// 根因：GetReader() 与 GetSeeker() 各 new 了一个独立的 bytes.Reader，
//   ContentHandler 只 Seek 了 seeker 那个，真正被 io.Copy 的 reader 仍在 0。
// ⚠️ 旧用例（TestServeFile_SeekableProvider_SeeksCorrectly）用 mock 时把同一个
//   bytes.Reader 同时塞给 ReaderVal/SeekerVal，恰好绕开了这个坑 —— 所以本文件
//   刻意用**真实的 LocalFileProvider**，不再用 mock 掩盖。
// ============================================================================

// fakeIndex —— types.Index 最小实现
type fakeIndex struct {
	name string
	size int64
}

func (f *fakeIndex) GetOriginalFilename() string { return f.name }
func (f *fakeIndex) GetOriginalFileSize() int64  { return f.size }
func (f *fakeIndex) GetOriginalFileMD5() string  { return "" }
func (f *fakeIndex) GetEncryptedFileMD5() string { return "" }
func (f *fakeIndex) GetMimeType() string         { return "video/mp4" }

// fakeDecryptReader —— 模拟"解密后的字节流"（可寻址，带 Seek）
type fakeDecryptReader struct {
	r *bytes.Reader
}

func (f *fakeDecryptReader) Read(p []byte) (int, error) { return f.r.Read(p) }
func (f *fakeDecryptReader) Seek(offset int64, whence int) (int64, error) {
	return f.r.Seek(offset, whence)
}
func (f *fakeDecryptReader) Close() error { return nil }

// fakeFactory —— reader.DecryptReaderFactory 最小实现
type fakeFactory struct {
	data     []byte
	seekable bool
}

func (f *fakeFactory) NewDecryptReader() (reader.DecryptReader, error) {
	return &fakeDecryptReader{r: bytes.NewReader(f.data)}, nil
}
func (f *fakeFactory) NewBulkDecryptor() (*reader.BulkDecryptor, error) { return nil, nil }
func (f *fakeFactory) GetIndex() types.Index {
	return &fakeIndex{name: "sample.mp4", size: int64(len(f.data))}
}
func (f *fakeFactory) GetOriginalSize() int64  { return int64(len(f.data)) }
func (f *fakeFactory) GetContainerPath() string { return "/data/local/tmp/out/sample.4pm.sccgv" }
func (f *fakeFactory) IsSeekable() bool         { return f.seekable }
func (f *fakeFactory) Close() error             { return nil }

// newRealProvider 构造**真实的** LocalFileProvider（不用 mock），返回明文数据以便比对
func newRealProvider(t *testing.T, size int, seekable bool) (provider.FileContentProvider, []byte) {
	t.Helper()
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 251) // 决定性内容：错位时一眼能看出来
	}
	f := &fakeFactory{data: data, seekable: seekable}
	dr, err := f.NewDecryptReader()
	require.NoError(t, err)
	prov, err := provider.NewLocalFileProvider(context.Background(), f, dr)
	require.NoError(t, err)
	return prov, data
}

// 小文件：走内存缓存分支（shouldCacheInMemory → true），Range 必须真的偏移
func TestServeFile_Range_SmallCachedContainer(t *testing.T) {
	const size = 4096
	prov, data := newRealProvider(t, size, true)

	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/stream", nil)
	r.Header.Set("Range", "bytes=1000-1099")

	h.ServeFile(w, r, prov)

	assert.Equal(t, http.StatusPartialContent, w.Code)
	assert.Equal(t, "bytes 1000-1099/4096", w.Header().Get("Content-Range"))
	assert.Equal(t, data[1000:1100], w.Body.Bytes(), "Range 必须从 start 开始返回，不能从头开始")
	assert.NotEqual(t, data[:100], w.Body.Bytes(), "返回文件头 = Range 被忽略（本 bug 的特征）")
}

// 小文件、多段连续 Range（模拟播放器分段拉取）
func TestServeFile_Range_SmallCachedContainer_MultipleOffsets(t *testing.T) {
	const size = 8192
	for _, off := range []int{0, 1, 1024, 4095, 4096, 8000} {
		prov, data := newRealProvider(t, size, true)
		h := NewContentHandler()
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/stream", nil)
		r.Header.Set("Range", "bytes="+itoa(off)+"-"+itoa(off+99))

		h.ServeFile(w, r, prov)

		assert.Equal(t, http.StatusPartialContent, w.Code, "offset=%d", off)
		assert.Equal(t, data[off:off+100], w.Body.Bytes(), "offset=%d 处字节错位", off)
	}
}

// 大文件：走流式分支（不进缓存），Seek 必须落在同一个 reader 上
func TestServeFile_Range_LargeStreamedContainer(t *testing.T) {
	const size = 4 << 20 // 4MB > 3MB 阈值 → 不缓存
	prov, data := newRealProvider(t, size, true)

	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/stream", nil)
	r.Header.Set("Range", "bytes=3000000-3000099")

	h.ServeFile(w, r, prov)

	assert.Equal(t, http.StatusPartialContent, w.Code)
	assert.Equal(t, "bytes 3000000-3000099/4194304", w.Header().Get("Content-Range"))
	assert.Equal(t, data[3000000:3000100], w.Body.Bytes())
}

// Provider 层直锁：GetReader 与 GetSeeker 必须共享同一个流位置
func TestLocalFileProvider_ReaderAndSeekerSharePosition(t *testing.T) {
	prov, data := newRealProvider(t, 4096, true)

	rd := prov.GetReader()
	require.NotNil(t, rd)
	sk, ok := prov.GetSeeker()
	require.True(t, ok, "小文件缓存后必须可 Seek")
	_, err := sk.Seek(512, io.SeekStart)
	require.NoError(t, err)

	got, err := io.ReadAll(io.LimitReader(rd, 16))
	require.NoError(t, err)
	assert.Equal(t, data[512:528], got, "Seek 必须作用于真正被读取的那个 reader")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [24]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
