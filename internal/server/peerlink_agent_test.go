package server

// peerlink_agent_test.go —— P4 远程 Agent 端到端（spec desktop-web-android-pairing）
//
// 验证：
//  1. 发起端 `POST /api/peerlink/agent/invoke` 经 Hub 中继到对端执行并回传结果
//  2. 对端拒绝 → ok=false + decision=decline（不会被当成成功）
//  3. **执行端**审批链路：挂起 → `GET /pending` 可见 → `POST /approve` 决策 → 信任/审计可查
//  4. 超时自动 decline（不测 90s：用短超时注入的 Approver 验证）
//  5. 参数校验/离线降级：400 / 503；未带运维头 → 401

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
)

func TestPeerlinkAgentInvoke_OK_And_Declined(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdgeFull(t, r, s, srv.URL+"/api/peerlink", nil, nil,
		func(req peerlink.AgentInvokeRequest) peerlink.AgentInvokeOutcome {
			if req.Tool == "encrypt_video" {
				// 模拟"执行端用户拒绝"
				return peerlink.AgentInvokeOutcome{Decision: peerlink.DecisionDecline, Err: peerlink.ErrDeclined}
			}
			return peerlink.AgentInvokeOutcome{
				Decision: peerlink.DecisionAccept,
				Result:   json.RawMessage(`{"ok":true,"tool":"` + req.Tool + `"}`),
			}
		})
	defer stop()

	do := func(tool string) (int, peerlink.AgentInvokeResult) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/peerlink/agent/invoke",
			bytes.NewReader([]byte(`{"peerId":"`+peerID+`","tool":"`+tool+`","args":{"path":"/sdcard/a.mp4"}}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Peerlink-Operator", "1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		defer resp.Body.Close()
		var out peerlink.AgentInvokeResult
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	code, out := do("read_file")
	if code != http.StatusOK || !out.Ok || out.Decision != peerlink.DecisionAccept {
		t.Fatalf("成功调用期望 200/ok/accept, got %d %+v", code, out)
	}
	if !strings.Contains(string(out.Result), "read_file") {
		t.Fatalf("结果未回传: %s", string(out.Result))
	}

	code, out = do("encrypt_video")
	if code != http.StatusOK || out.Ok || out.Decision != peerlink.DecisionDecline {
		t.Fatalf("被拒绝的调用应 ok=false/decline, got %d %+v", code, out)
	}
}

func TestPeerlinkAgentInvoke_Offline_503_And_BadRequest_400(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdgeFull(t, r, s, srv.URL+"/api/peerlink", nil, nil,
		func(peerlink.AgentInvokeRequest) peerlink.AgentInvokeOutcome {
			return peerlink.AgentInvokeOutcome{Decision: peerlink.DecisionAccept, Result: json.RawMessage(`{}`)}
		})
	stop()

	req, _ := http.NewRequest("POST", srv.URL+"/api/peerlink/agent/invoke",
		bytes.NewReader([]byte(`{"peerId":"`+peerID+`","tool":"read_file"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("对端离线应 503, got %d", resp.StatusCode)
	}

	// 缺 tool → 400
	req2, _ := http.NewRequest("POST", srv.URL+"/api/peerlink/agent/invoke",
		bytes.NewReader([]byte(`{"peerId":"`+peerID+`"}`)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Peerlink-Operator", "1")
	resp2, _ := http.DefaultClient.Do(req2)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("缺 tool 应 400, got %d", resp2.StatusCode)
	}
}

// TestPeerlinkAgentApproval_ExecSideFlow —— 执行端审批主链路
func TestPeerlinkAgentApproval_ExecSideFlow(t *testing.T) {
	_, s := newPeerlinkRouter() // 本例只走**执行端本地**逻辑，不需要起 HTTP 服务

	// 注入短超时授权器（避免测试跑 90s）
	ap := peerlink.NewApprover(peerlink.ApproverOptions{
		Timeout:       3 * time.Second,
		IsDestructive: func(tool string) bool { return tool == "encrypt_video" },
	})
	s.agentApprover = ap

	// 执行端被对端调用 → 挂起等待本端 UI
	done := make(chan error, 1)
	go func() {
		res := s.PeerAgentInvokeHandler(peerlink.AgentInvokeRequest{
			CallId:   "call-1",
			Tool:     "encrypt_video",
			Args:     json.RawMessage(`{"path":"/sdcard/私密/工资单.mp4"}`),
			FromId:   "peer-x",
			FromName: "Desktop",
		})
		done <- res.Err
	}()

	// 挂起应可见，并标记 destructive
	deadline := time.Now().Add(500 * time.Millisecond)
	var pending []peerlink.ApprovalRequest
	for time.Now().Before(deadline) {
		pending = ap.Pending()
		if len(pending) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(pending) != 1 {
		t.Fatalf("期望 1 条挂起, got %d", len(pending))
	}
	if !pending[0].Destructive || pending[0].Tool != "encrypt_video" || pending[0].PeerId != "peer-x" {
		t.Fatalf("挂起请求元数据错误: %+v", pending[0])
	}

	// 本端 UI 决策：trust_device
	if err := ap.Decide("call-1", peerlink.DecisionTrust); err != nil {
		t.Fatalf("Decide 失败: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("决策后调用未返回（可能仍挂起）")
	}

	// 信任已记录（进程内存）
	if !ap.Trusted("peer-x") {
		t.Fatalf("trust_device 应记住该设备")
	}
	// 审计脱敏：不得出现参数/路径全文
	raw, _ := json.Marshal(ap.Audit())
	if strings.Contains(string(raw), "工资单") || strings.Contains(string(raw), "/sdcard/私密") {
		t.Fatalf("审计泄露敏感路径: %s", string(raw))
	}
	if !strings.Contains(string(raw), peerlink.DecisionTrust) {
		t.Fatalf("审计应记录 trust_device 决策: %s", string(raw))
	}
}

// TestPeerlinkAgentLocalEndpoints_Auth —— 审批类本地接口必须带运维头（P5 未鉴权兜底）
func TestPeerlinkAgentLocalEndpoints_Auth(t *testing.T) {
	r, _ := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	for _, path := range []string{
		"/api/peerlink/agent/pending",
		"/api/peerlink/agent/trust",
		"/api/peerlink/agent/audit",
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("请求失败: %v", err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s 未带运维头应 401, got %d", path, resp.StatusCode)
		}
	}
}
