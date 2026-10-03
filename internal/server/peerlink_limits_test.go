package server

// peerlink_limits_test.go —— P5 / R12：速率限制 + 失败熔断
//
// 防护对象：Hub 跑在**公网**（cnb）⇒ 端点可被扫描/误撞；
// 对端不可达时若无熔断，前端轮询会持续打 RPC 并每次等满超时 ⇒ 雪崩。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// offlinePeer 在给定 Hub 上登记一个"已配对但离线"的对端，返回其 peerID。
func offlinePeer(t *testing.T, r *gin.Engine, s *Server, hubURL string) string {
	t.Helper()
	id, stop := startPairedEdge(t, r, s, hubURL, nil, nil)
	stop()
	// 等 Hub 把它标记为离线（连接关闭 → 立即生效，这里给一点余量）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, open, _, _ := s.peerLimits().call.state(id); !open {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return id
}

func TestPeerlinkLimits_RateLimit_429(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	// 用"已配对但离线"的对端：调用会快速失败（503），不会拖慢测试
	peerID := offlinePeer(t, r, s, srv.URL+"/api/peerlink")

	// search 限额 30 次/分钟（见 peerlink_limits.go）
	var lastStatus int
	var lastErr string
	for i := 0; i < 31; i++ {
		req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/search?peerId="+peerID+"&q=x", nil)
		req.Header.Set("X-Peerlink-Operator", "1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		lastStatus = resp.StatusCode
		var body struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		lastErr = body.Error
	}
	if lastStatus != http.StatusTooManyRequests || lastErr != "rate_limited" {
		t.Fatalf("第 31 次应 429 rate_limited, got %d %q", lastStatus, lastErr)
	}
}

// TestPeerlinkLimits_AgentRateAllowsInteractiveUse 锁住远程 Agent 的限流阈值。
//
// 2026-10-04：agent 原为 **10 次/分钟**，而远程调试是**交互式**的（列目录 → 展开几层 →
// 逐个读文件），随手点几下就撞线并被冷却 ~60s，表现得像"功能卡死"。
// 这条锁的意义是防止有人把它调回个位数 —— 阈值必须明显高于人类操作频率。
func TestPeerlinkLimits_AgentRateAllowsInteractiveUse(t *testing.T) {
	w := newRateWindow(rateLimitAgent, time.Minute)
	now := time.Now()

	for i := 0; i < rateLimitAgent; i++ {
		if ok, _ := w.allow("k", now.Add(time.Duration(i)*time.Millisecond)); !ok {
			t.Fatalf("第 %d 次调用就被限流 —— 交互式远程调试不能这么低（阈值 %d/分钟）", i+1, rateLimitAgent)
		}
	}
	// 超过阈值仍要拦住：放宽是为了体验，不是放弃防扫描
	if ok, _ := w.allow("k", now.Add(time.Duration(rateLimitAgent)*time.Millisecond)); ok {
		t.Fatalf("超过 %d 次/分钟仍放行 ⇒ 防扫描失效", rateLimitAgent)
	}
}

func TestPeerlinkLimits_CircuitBreaker_Open503(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID := offlinePeer(t, r, s, srv.URL+"/api/peerlink")
	// 测试钩子：把阈值/冷却调小，避免测试跑几十秒
	s.setPeerLimitsForTests(2, 250*time.Millisecond)

	do := func() (int, string) {
		req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/search?peerId="+peerID+"&q=x", nil)
		req.Header.Set("X-Peerlink-Operator", "1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		defer resp.Body.Close()
		var body struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&body)
		return resp.StatusCode, body.Error
	}

	// 连续失败到阈值 → 开路
	if code, e := do(); code != http.StatusServiceUnavailable || e != "peer_offline" {
		t.Fatalf("首次应 503 peer_offline, got %d %q", code, e)
	}
	if code, e := do(); code != http.StatusServiceUnavailable || e != "peer_offline" {
		t.Fatalf("第二次应 503 peer_offline, got %d %q", code, e)
	}
	// 第三次：已开路 → 直接 503 peer_circuit_open（**不再消耗调用超时预算**）
	code, e := do()
	if code != http.StatusServiceUnavailable || e != "peer_circuit_open" {
		t.Fatalf("第三次应 503 peer_circuit_open, got %d %q", code, e)
	}

	// 冷却结束 → 半开探测（放行一次，表现为正常的 peer_offline 而非 circuit_open）
	time.Sleep(320 * time.Millisecond)
	code, e = do()
	if e == "peer_circuit_open" {
		t.Fatalf("冷却后应允许半开探测, got %d %q", code, e)
	}
}

// TestPeerlinkLimits_RateWindow_Unit —— 滑动窗口本身的行为（不依赖网络）
func TestPeerlinkLimits_RateWindow_Unit(t *testing.T) {
	w := newRateWindow(3, time.Minute)
	now := time.Now()
	for i := 0; i < 3; i++ {
		if ok, _ := w.allow("k", now.Add(time.Duration(i)*time.Second)); !ok {
			t.Fatalf("第 %d 次应放行", i+1)
		}
	}
	if ok, _ := w.allow("k", now.Add(4*time.Second)); ok {
		t.Fatalf("超出限额应拒绝")
	}
	// 窗口滑过 → 重新放行
	if ok, _ := w.allow("k", now.Add(61*time.Second)); !ok {
		t.Fatalf("窗口滑过后应放行")
	}
	// 不同 key 互不影响
	if ok, _ := w.allow("other", now.Add(5*time.Second)); !ok {
		t.Fatalf("不同 key 应互不影响")
	}
}

// 保证断言里用到的 peerlink 包引用不被误删（编译期锚点）
var _ = peerlink.DefaultCallTimeout
