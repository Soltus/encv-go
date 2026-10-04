package server

// peerlink_agent_calllog_test.go —— **发起端**调用台账的契约锁
//
// 为什么需要（2026-10-04 真机压测发现的缺口）：
//
//	`/api/peerlink/agent/audit` 原本只返回 Approver 的审计，而 Approver 只在**执行端**工作
//	⇒ 桌面端（发起端）永远查不到"我远程调用过什么"。对"远程读别人手机上的文件"来说，
//	这是合规与排障的硬缺口：出事只能到手机上翻日志，而手机日志没人看。

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/peerlink"
)

// auditResponse /agent/audit 的完整响应体（含新增的 outbound）
type auditResponse struct {
	Items         []peerlink.AuditEntry `json:"items"`
	Count         int                   `json:"count"`
	Outbound      []AgentCallEntry      `json:"outbound"`
	OutboundCount int                   `json:"outboundCount"`
}

func getAudit(t *testing.T, srvURL string) auditResponse {
	t.Helper()
	req, _ := http.NewRequest("GET", srvURL+"/api/peerlink/agent/audit", nil)
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("audit 请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit 应 200, got %d", resp.StatusCode)
	}
	var out auditResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("audit 响应解析失败: %v", err)
	}
	return out
}

func invokeTool(t *testing.T, srvURL, peerID, tool string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", srvURL+"/api/peerlink/agent/invoke",
		bytes.NewReader([]byte(`{"peerId":"`+peerID+`","tool":"`+tool+`","callId":"AUDIT-`+tool+`","args":{"secret":"SWORDFISH","path":"/sdcard/a.mp4"}}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("invoke 请求失败: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// TestPeerlinkAgentAudit_OutboundRecords 发起端必须能查到自己发起了什么。
func TestPeerlinkAgentAudit_OutboundRecords(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdgeFull(t, r, s, srv.URL+"/api/peerlink", nil, nil,
		func(req peerlink.AgentInvokeRequest) peerlink.AgentInvokeOutcome {
			return peerlink.AgentInvokeOutcome{
				Decision: peerlink.DecisionAccept,
				Result:   json.RawMessage(`{"ok":true}`),
			}
		})
	defer stop()

	if code := invokeTool(t, srv.URL, peerID, "list_mounts"); code != http.StatusOK {
		t.Fatalf("调用应成功, got %d", code)
	}

	got := getAudit(t, srv.URL)
	if got.OutboundCount == 0 || len(got.Outbound) == 0 {
		t.Fatal("发起端台账为空 —— 桌面端查不到自己发起过什么（本用例针对的就是这个缺口）")
	}
	last := got.Outbound[len(got.Outbound)-1]
	if last.Tool != "list_mounts" {
		t.Fatalf("台账 tool 错误: %q", last.Tool)
	}
	if !last.Ok {
		t.Fatal("成功的调用应记为 Ok=true")
	}
	if last.Decision != peerlink.DecisionAccept {
		t.Fatalf("应透传执行端的真实决策, got %q", last.Decision)
	}
	if last.PeerId != peerID {
		t.Fatalf("台账 peerId 错误: %q", last.PeerId)
	}
}

// TestPeerlinkAgentAudit_OutboundIsRedacted 脱敏：台账**不得**记录参数的值（R14）。
// 只保留字节数与顶层键名，避免"审计表自己变成一份敏感数据副本"。
func TestPeerlinkAgentAudit_OutboundIsRedacted(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdgeFull(t, r, s, srv.URL+"/api/peerlink", nil, nil,
		func(peerlink.AgentInvokeRequest) peerlink.AgentInvokeOutcome {
			return peerlink.AgentInvokeOutcome{Decision: peerlink.DecisionAccept, Result: json.RawMessage(`{}`)}
		})
	defer stop()

	// 参数里放一个明显的值，台账任何字段都不应出现它
	invokeTool(t, srv.URL, peerID, "read_file")

	raw, _ := json.Marshal(getAudit(t, srv.URL).Outbound)
	if strings.Contains(string(raw), "SWORDFISH") {
		t.Fatalf("台账泄露了参数值（R14）: %s", raw)
	}
	// 顶层键名可以留（便于排障），但值不能留
	if !strings.Contains(string(raw), "secret") {
		t.Fatalf("台账应保留顶层键名以便排障: %s", raw)
	}
}

// TestPeerlinkAgentAudit_FailedCallStillRecorded 失败的调用也要留痕：
// 否则"发起过但没成功"的调用会从台账里凭空消失，审计就不完整。
func TestPeerlinkAgentAudit_FailedCallStillRecorded(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdgeFull(t, r, s, srv.URL+"/api/peerlink", nil, nil,
		func(peerlink.AgentInvokeRequest) peerlink.AgentInvokeOutcome {
			return peerlink.AgentInvokeOutcome{Decision: peerlink.DecisionAccept, Result: json.RawMessage(`{}`)}
		})
	stop()
	waitPeerGone(t, s, peerID)

	code := invokeTool(t, srv.URL, peerID, "list_mounts")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("对端离线应 503, got %d", code)
	}

	got := getAudit(t, srv.URL)
	found := false
	for _, e := range got.Outbound {
		if e.ErrorType == "peer_offline" {
			found = true
			if e.Ok {
				t.Fatal("失败的调用不应记为 Ok=true")
			}
		}
	}
	if !found {
		t.Fatalf("离线调用必须留在台账里（ErrorType=peer_offline）: %+v", got.Outbound)
	}
}
