package server

// peerlink_file_test.go —— P3.4 远端读通道（在线打开 / 缩略图）端到端
//
// 契约（spec §3 非目标 + E4 + R13）：
//   - 远端命中**不产生本地路径语义**：每次读都是显式跨端请求，响应带 `X-Peer-*` 来源标注
//   - **不是挂载**：没有统一命名空间，客户端必须自己带 peerId + 远端 path
//   - R13：单次读取有硬上限 → 超限 **413**
//   - E4：整文件取回默认禁止穿透云端（本通道只服务小报文）

import (
	"fmt"
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

	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", nil, func(req peerlink.ReadRequest) ([]byte, int64, error) {
		if req.Path != "/sdcard/a.txt" {
			return nil, 0, peerlink.ErrBadProof // 借 sentinel 表达"路径不存在"
		}
		start := req.Offset
		if start < 0 || start > int64(len(fixture)) {
			return nil, 0, peerlink.ErrBadProof
		}
		end := start + int64(req.Length)
		if end > int64(len(fixture)) {
			end = int64(len(fixture))
		}
		return fixture[start:end], int64(len(fixture)), nil
	})
	defer stop()

	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/file?peerId="+peerID+"&path=/sdcard/a.txt&offset=10&length=10", nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("file 请求失败: %v", err)
	}
	defer resp.Body.Close()

	// ⚠️ 2026-10-04 契约修正：**非最后一片必须 206**，不能一律 200。
	//	旧实现无条件返回 200 ⇒ 调用方以为拿到了完整文件（实际被 DefaultReadChunk=256KB 截断）
	//	⇒ 静默的数据损坏。这里读的是 [10,20)，而文件总长 36 ⇒ 后面还有数据 ⇒ 必须 206。
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("期望 206 Partial Content（还有后续分片）, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 10-19/36" {
		t.Fatalf("Content-Range 应为 bytes 10-19/36, got %q", got)
	}
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("应声明 Accept-Ranges: bytes, got %q", resp.Header.Get("Accept-Ranges"))
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

	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", nil, func(peerlink.ReadRequest) ([]byte, int64, error) {
		return fixture, int64(len(fixture)), nil
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

// TestPeerlinkFile_LastChunk_200 锁住"读到最后一片必须是 **200**"（2026-10-04 补）。
//
// 只校验 206 是不够的：若连最后一片也返回 206，播放器/下载器会以为还有后续分片，
// 白白多一轮请求甚至报错 ⇒ 必须区分"还有数据(206)"与"已读完整(200)"两种信号。
func TestPeerlinkFile_LastChunk_200(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", nil, func(req peerlink.ReadRequest) ([]byte, int64, error) {
		if req.Path != "/sdcard/a.txt" {
			return nil, 0, peerlink.ErrBadProof
		}
		start := req.Offset
		if start < 0 || start > int64(len(fixture)) {
			return nil, 0, peerlink.ErrBadProof
		}
		end := start + int64(req.Length)
		if end > int64(len(fixture)) {
			end = int64(len(fixture))
		}
		return fixture[start:end], int64(len(fixture)), nil
	})
	defer stop()

	// length=100 > 文件长度 36 ⇒ 一次读到末尾 ⇒ 必须是 200，且不带 Content-Range
	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/file?peerId="+peerID+"&path=/sdcard/a.txt&length=100", nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("最后一片期望 200, got %d", resp.StatusCode)
	}
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		t.Fatalf("最后一片不应带 Content-Range, got %q", cr)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != string(fixture) {
		t.Fatalf("内容应是一整份 fixture: %q", string(body))
	}
}

// TestPeerlinkFile_UnknownTotal_NoRange 对端取不到文件大小（total=0）时，
// 发起端**不得**谎称自己是完整响应 —— 这是 2026-10-04 静默截断的根因容错：
// 宁可给 206，也不能给没有 Content-Range 的 200。
func TestPeerlinkFile_UnknownTotal_206(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	// total 返回 0（模拟 stat 失败）
	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", nil, func(peerlink.ReadRequest) ([]byte, int64, error) {
		return fixture, 0, nil
	})
	defer stop()

	req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/file?peerId="+peerID+"&path=/sdcard/a.txt", nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("总大小未知时应保守地给 206（不能谎称完整）, got %d", resp.StatusCode)
	}
	if th := resp.Header.Get("X-Peer-Total-Size"); th != "" {
		t.Fatalf("总大小未知时不应输出 X-Peer-Total-Size, got %q", th)
	}
}

// TestPeerlinkFile_RejectedNotCircuitBreaker 锁回归（2026-10-04 真机缺陷）：
//
//	**非法入参不得计入熔断**。此前路径非法（bad_path）被当成"对端故障"计入失败累计，
//	连续 2 次就把 peer 熔断开路 ⇒ 之后的**合法**请求一起 503（整条远端读链路瘫到冷却结束），
//	而用户看到的只是"突然全挂"，完全想不到是刚才输错了一次路径。
//
//	正确语义："你这个请求不对"是对端**健康地**回答了不行 ⇒ 应给 400，且要复位失败计数。
func TestPeerlinkFile_RejectedNotCircuitBreaker(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", nil, func(req peerlink.ReadRequest) ([]byte, int64, error) {
		if req.Path != "/sdcard/a.txt" {
			return nil, 0, fmt.Errorf("%w: bad_path", peerlink.ErrPeerRejected)
		}
		return fixture, int64(len(fixture)), nil
	})
	defer stop()

	get := func(path string) *http.Response {
		req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/file?peerId="+peerID+"&path="+url.QueryEscape(path), nil)
		req.Header.Set("X-Peerlink-Operator", "1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		return resp
	}

	// ① 连续 6 次非法路径（远超熔断阈值）：每次都必须是 **400**，绝不能变成 5xx
	for i := 0; i < 6; i++ {
		resp := get("../../../../etc/passwd")
		code := resp.StatusCode
		resp.Body.Close()
		if code != http.StatusBadRequest {
			t.Fatalf("第 %d 次非法路径期望 400 peer_rejected, got %d", i+1, code)
		}
	}

	// ② 紧接着的合法请求必须照常成功 —— 这里是防"连坐"的关键断言
	resp := get("/sdcard/a.txt")
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusServiceUnavailable {
		t.Fatalf("合法请求被熔断连坐（peer_circuit_open）—— 非法入参不应计入熔断, got 503")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("合法请求应正常返回 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != string(fixture) {
		t.Fatalf("内容错误: %q", string(body))
	}
}

func TestPeerlinkFile_ChunkTooLarge_413(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", nil, func(peerlink.ReadRequest) ([]byte, int64, error) {
		return fixture, int64(len(fixture)), nil
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

	peerID, stop := startPairedEdge(t, r, s, srv.URL+"/api/peerlink", nil, func(peerlink.ReadRequest) ([]byte, int64, error) {
		return fixture, int64(len(fixture)), nil
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
