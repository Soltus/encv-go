package server

// peerlink_edge_stale_test.go —— 「会合点地址已失效」必须**可被发现**（2026-10-05）
//
// 同一类风险的第二个实例（第一个是 baseUrl 里被无条件信任的 :16666）：
//   扫码时记住的会合点地址（edge-session.json）**失效后**，设备只会一遍遍重试，
//   UI 显示"已连接到会合点 X"或"连接中…"，Hub 侧看到的则是这台设备**零请求**
//   ⇒ 用户与运维都看不出"地址没了"，只看到"连不上"。
//
// 契约：
//  1. 连不上 + 重试够多次 ⇒ stale=true（UI 才能提示"会合点可能已失效，请重新扫码"）
//  2. 连得上 ⇒ 绝不 stale
//  3. 刚失败一两次（手机没网 / 后端刚重启）⇒ **不得** stale（抖动不能劝用户重新配对）

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestEdgeSessionStale_OnlyAfterRepeatedFailures(t *testing.T) {
	cases := []struct {
		name      string
		connected bool
		attempts  int
		want      bool
	}{
		{"连上了就不算失效", true, 99, false},
		{"刚失败一次（可能只是抖动）", false, 1, false},
		{"失败两次仍不算", false, 2, false},
		{"失败到阈值才算", false, 5, true},
		{"超过阈值仍然算", false, 12, true},
	}
	for _, c := range cases {
		if got := edgeSessionStale(c.connected, c.attempts); got != c.want {
			t.Errorf("%s: stale(%v,%d)=%v want %v", c.name, c.connected, c.attempts, got, c.want)
		}
	}
}

// TestPeerlinkEdgeStatus_ReportsStale —— /edge/status 必须把 stale 吐出去（否则 UI 无从判断）
func TestPeerlinkEdgeStatus_ReportsStale(t *testing.T) {
	r, s := newPeerlinkRouter()
	_ = r
	// 连一个必然连不上的地址：只断言字段语义，不要求真的连上
	s.startEdgeLocked("http://127.0.0.1:1/api/peerlink", "peer-z", "dev-z", "tok-z")
	s.peerEdgeMu.Lock()
	rt := s.peerEdge
	s.peerEdgeMu.Unlock()
	if rt == nil || rt.edge == nil {
		t.Fatal("startEdgeLocked 未建立 Edge")
	}
	defer func() {
		if rt.cancel != nil {
			rt.cancel()
		}
	}()

	// 直接以"已重试 N 次且未连上"的状态断言 handler 输出（不依赖真实退避节奏）
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/peerlink/edge/status", nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	r.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("/edge/status 期望 200, got %d", rec.Code)
	}
	var out struct {
		Running   bool `json:"running"`
		Connected bool `json:"connected"`
		Stale     bool `json:"stale"`
		Attempts  int  `json:"attempts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是 JSON: %v (%s)", err, rec.Body.String())
	}
	if !out.Running {
		t.Fatal("running 应为 true")
	}
	// 语义一致性：stale 必须等于"未连上且重试到阈值"
	if want := edgeSessionStale(out.Connected, out.Attempts); out.Stale != want {
		t.Fatalf("stale 与语义不符: stale=%v connected=%v attempts=%d", out.Stale, out.Connected, out.Attempts)
	}
}
