package server

// peerlink_agent_decision_test.go —— P4 回归锁：**调用决策必须**原样**回传给调用端**
//
// 背景（2026-10-02 模拟器端到端 `scripts/emu-peerlink-e2e.sh` T8 抓出）：
//   执行端点了「信任此设备」（trust_device）后，同一工具再被调用时
//   执行端确实走了免确认路径（`pending` 为空、秒回），但回传给调用端的 decision
//   却是 **accept** ⇒ 调用端永远分不清「用户逐次同意」与「因信任自动放行」，
//   也永远看不到用户曾授权 trust_device（审计/UI 语义失真）。
//
// 根因：`internal/peerlink/edge.go` 的 `agent_invoke` 分支成功时**硬编码**
//   `out.Decision = DecisionAccept`，而 `Approver.Require` 返回的真实决策
//   （auto / trust_device / accept）在执行端就被丢掉了。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
)

// TestPeerlinkAgentInvoke_DecisionPropagatedToCaller 断言：
//
//	第 1 次调用（用户点 trust_device）→ 回传 decision=trust_device
//	第 2 次调用（已信任，免确认）     → 回传 decision=auto
func TestPeerlinkAgentInvoke_DecisionPropagatedToCaller(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	// 注入短超时授权器（生产默认 90s，测试不要等）；本用例不关心破坏性判定
	ap := peerlink.NewApprover(peerlink.ApproverOptions{
		Timeout:       10 * time.Second,
		IsDestructive: func(tool string) bool { return false },
	})
	s.agentApprover = ap

	// Edge 侧执行器 = **真实全链路**（执行端授权 → 执行工具）
	peerID, stop := startPairedEdgeFull(t, r, s, srv.URL+"/api/peerlink", nil, nil,
		func(req peerlink.AgentInvokeRequest) peerlink.AgentInvokeOutcome {
			return s.PeerAgentInvokeHandler(req)
		})
	defer stop()

	// 自动审批协程：执行端 UI 轮询 pending → 决策 trust_device（模拟"信任此设备"按钮）
	go func() {
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			req, _ := http.NewRequest("GET", srv.URL+"/api/peerlink/agent/pending", nil)
			req.Header.Set("X-Peerlink-Operator", "1")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			var body struct {
				Items []struct {
					CallId string `json:"callId"`
					Tool   string `json:"tool"`
				} `json:"items"`
			}
			json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			if len(body.Items) > 0 {
				req2, _ := http.NewRequest("POST", srv.URL+"/api/peerlink/agent/approve",
					bytes.NewReader([]byte(`{"callId":"`+body.Items[0].CallId+`","decision":"trust_device"}`)))
				req2.Header.Set("Content-Type", "application/json")
				req2.Header.Set("X-Peerlink-Operator", "1")
				resp2, err := http.DefaultClient.Do(req2)
				if err == nil {
					resp2.Body.Close()
				}
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	invoke := func() peerlink.AgentInvokeResult {
		req, _ := http.NewRequest("POST", srv.URL+"/api/peerlink/agent/invoke",
			bytes.NewReader([]byte(`{"peerId":"`+peerID+`","tool":"list_mounts"}`)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Peerlink-Operator", "1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("invoke 失败: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("invoke 期望 200, got %d", resp.StatusCode)
		}
		var out peerlink.AgentInvokeResult
		json.NewDecoder(resp.Body).Decode(&out)
		return out
	}

	first := invoke()
	if !first.Ok {
		t.Fatalf("第 1 次调用应成功: %+v", first)
	}
	if first.Decision != peerlink.DecisionTrust {
		t.Fatalf("第 1 次调用（用户点 trust_device）应回传 decision=trust_device, got %q", first.Decision)
	}

	// 第 2 次：已信任 ⇒ 不再挂起（秒回），且必须回传 auto
	second := invoke()
	if !second.Ok {
		t.Fatalf("第 2 次调用应成功: %+v", second)
	}
	if second.Decision != peerlink.DecisionAuto {
		t.Fatalf("已信任设备的调用应回传 decision=auto（而不是 accept）, got %q", second.Decision)
	}
}
