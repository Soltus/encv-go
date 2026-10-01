// internal/v2/handler/content_range_bounds_test.go
//
// 回归锁：HTTP 协议边界两件事（2026-10-01 模拟器内真实后端实测抓到）
//
//   1) 416（Range Not Satisfiable）**必须**是空实体体 + `Content-Range: bytes */size`。
//      修前：parseRangeHeader 返回 416 时把 start/end 重置成"全文件"，
//      ServeFile 照常 io.Copy ⇒ 客户端收到 **416 + 整个文件的实体体**（白白解密 + 传全量），
//      且缺少 RFC 7233 要求的 `Content-Range: bytes */size`。
//
//   2) provider 交不出 reader（GetReader() 返回 nil，真实场景：解密/读文件失败后
//      LocalFileProvider 就是这么返回的）时，ServeFile **不能 panic**。
//      修前：io.Copy(w, io.LimitReader(nil, n)) ⇒ nil 指针解引用 ⇒
//      gin recovery 记 500，连接被掐。真机表现为"随机播放失败且后端刷 panic 栈"。
package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Soltus/encv-go/internal/v2/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/Soltus/encv-go/internal/testguard"
)

// TestServeFile_RangeBeyondEOF_Is416WithEmptyBody
func TestServeFile_RangeBeyondEOF_Is416WithEmptyBody(t *testing.T) {
	const size = 4096
	prov, _ := newRealProvider(t, size, true)

	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/stream", nil)
	r.Header.Set("Range", "bytes=999999-")

	h.ServeFile(w, r, prov)

	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, w.Code)
	assert.Equal(t, "bytes */4096", w.Header().Get("Content-Range"),
		"RFC 7233：416 必须带 Content-Range: bytes */<完整长度>")
	assert.Empty(t, w.Body.Bytes(),
		"416 不能带实体体（修前会把整个文件白送一遍）")
}

// TestServeFile_ReverseRange_Is416WithEmptyBody start > end
func TestServeFile_ReverseRange_Is416WithEmptyBody(t *testing.T) {
	const size = 4096
	prov, _ := newRealProvider(t, size, true)

	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/stream", nil)
	r.Header.Set("Range", "bytes=2000-1000")

	h.ServeFile(w, r, prov)

	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, w.Code)
	assert.Empty(t, w.Body.Bytes(), "416 不能带实体体")
}

// nilReaderProvider 模拟"解密/读取失败"的 provider：GetReader 交不出东西。
// 这是 LocalFileProvider 在 loadIntoMemory 失败后的真实行为，不是假想场景。
type nilReaderProvider struct{ size int64 }

func (p *nilReaderProvider) GetReader() io.ReadCloser            { return nil }
func (p *nilReaderProvider) GetSeeker() (io.Seeker, bool)        { return nil, false }
func (p *nilReaderProvider) GetSeekerTo() (provider.SeekerTo, bool) { return nil, false }
func (p *nilReaderProvider) GetSize() int64                      { return p.size }
func (p *nilReaderProvider) GetName() string                     { return "broken.mp4" }
func (p *nilReaderProvider) Close() error                        { return nil }

// TestServeFile_NilReader_Returns500WithoutPanic
func TestServeFile_NilReader_Returns500WithoutPanic(t *testing.T) {
	h := NewContentHandler()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/stream", nil)

	require.NotPanics(t, func() {
		h.ServeFile(w, r, &nilReaderProvider{size: 4096})
	}, "provider 交不出 reader 时 ServeFile 不能 panic（修前 io.Copy 对 nil reader 解引用）")

	assert.Equal(t, http.StatusInternalServerError, w.Code,
		"读不出来就该显式 500，而不是把责任甩给 gin recovery")
}
