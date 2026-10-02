package server

// peerlink_file_test.go —— P3.4 远端读通道（在线打开 / 缩略图）端到端
//
// 契约（spec §3 非目标 + E4 + R13）：
//   - 远端命中**不产生本地路径语义**：每次读都是显式跨端请求，响应带 `X-Peer-*` 来源标注
//   - **不是挂载**：没有统一命名空间，客户端必须自己带 peerId + 远端 path
//   - R13：单次读取有硬上限 → 超限 **413**
//   - E4：整文件取回默认禁止穿透云端（本通道只服务小报文）

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/peerlink"
)

// fixture 模拟对端本地文件的内容
var fixture = []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ")

func TestPeerlinkFile_Read_OK_And_Headers(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", nil, func(req peerlink.ReadRequest) ([]byte, error) {
		if req.Path != "/sdcard/a.txt" {
			return nil, peerlink.ErrBadProof // 借 sentinel 表达"路径不存在"
		}
		start := req.Offset
		if start < 0 || start > int64(len(fixture)) {
			return nil, peerlink.ErrBadProof
		}
		end := start + int64(req.Length)
		if end > int64(len(fixture)) {
			end = int64(len(fixture))
		}
		return fixture[start:end], nil
	})
	defer stop()

	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/file?peerId="+peerID+"&path=/sdcard/a.txt&offset=10&length=10", nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("file 请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("期望 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ABCDEFGHIJ" {
		t.Fatalf("分片内容错误: %q", string(body))
	}
	// 来源标注必须常驻（客户端任何后续处理都能看到"这字节来自哪台设备"）
	if resp.Header.Get("X-Peer-Id") != peerID {
		t.Fatalf("缺少 X-Peer-Id: %q", resp.Header.Get("X-Peer-Id"))
	}
	if resp.Header.Get("X-Peer-Remote-Path") != "/sdcard/a.txt" {
		t.Fatalf("缺少/错误的 X-Peer-Remote-Path: %q", resp.Header.Get("X-Peer-Remote-Path"))
	}
	if !strings.Contains(resp.Header.Get("Content-Disposition"), "a.txt") {
		t.Fatalf("Content-Disposition 应带远端文件名: %q", resp.Header.Get("Content-Disposition"))
	}
}

// TestPeerlinkFile_NonASCIIHeader 锁定回归：HTTP 头是 latin-1，
// 中文远端路径直接塞进头会被客户端解成乱码（真实浏览器验证抓出）⇒ 必须百分号编码。
func TestPeerlinkFile_NonASCIIHeader(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", nil, func(peerlink.ReadRequest) ([]byte, error) {
		return fixture, nil
	})
	defer stop()

	remotePath := "/sdcard/下载/报告 2026.pdf"
	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/file?peerId="+peerID+"&path="+url.QueryEscape(remotePath), nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("期望 200, got %d", resp.StatusCode)
	}

	got := resp.Header.Get("X-Peer-Remote-Path")
	for i := 0; i < len(got); i++ {
		if got[i] >= 0x80 {
			t.Fatalf("响应头仍含非 ASCII 字节（会乱码）: %q", got)
		}
	}
	decoded, err := url.PathUnescape(got)
	if err != nil || decoded != remotePath {
		t.Fatalf("响应头应可还原为原始远端路径: got %q decoded %q err %v", got, decoded, err)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.Contains(cd, "filename*=UTF-8''") {
		t.Fatalf("Content-Disposition 缺 RFC5987 filename*: %q", cd)
	}
}

func TestPeerlinkFile_ChunkTooLarge_413(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", nil, func(peerlink.ReadRequest) ([]byte, error) {
		return fixture, nil
	})
	defer stop()

	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/file?peerId="+peerID+"&path=/sdcard/a.txt&length=99999999", nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("超上限应 413, got %d", resp.StatusCode)
	}
}

func TestPeerlinkFile_Offline_503_And_MissingParams_400(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", nil, func(peerlink.ReadRequest) ([]byte, error) {
		return fixture, nil
	})
	stop()

	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/file?peerId="+peerID+"&path=/sdcard/a.txt", nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("对端离线应 503, got %d", resp.StatusCode)
	}

	req2, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/file?peerId="+peerID, nil)
	req2.Header.Set("X-Peerlink-Operator", "1")
	resp2, _ := http.DefaultClient.Do(req2)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺 path 应 400, got %d", resp2.StatusCode)
	}
}
