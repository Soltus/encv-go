package server

// peerlink_agent_contract_test.go —— 远程 Agent 调用的**结果契约**锁（2026-10-04）
//
// 背景（真机实测复现，Hub → 安卓真机配对设备）：
//
//	POST /api/peerlink/agent/invoke {tool:"search_files", args:{q:"a"}}
//	  → HTTP 200 {"ok":true,"decision":"auto","result":{"error":"mount_id is required"}}
//
// 工具业务失败被包成 errJSON 塞进 result，而 Ok 只表达"RPC 送达 + 审批通过"
// （edge.go: Ok = out.Err == nil）⇒ 发起端只看 ok 会把失败当成功。
// 这是记忆里"HTTP 契约不一致"遗留项的执行端真身：远程调试时表现为静默失败。
//
// 契约（本文件锁死）：
//  1. 工具返回 errJSON 形状载荷 ⇒ ok=false，错误码/信息提到顶层（errorCode / error）
//  2. result 原样保留（向后兼容）
//  3. HTTP 仍 200（RPC 确实送达并被对端执行；4xx/5xx 留给链路故障）
//  4. 正常载荷**即便内含 error 字段**也不得被误判为失败（宁漏判，不误判）

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Soltus/encv-go/internal/peerlink"
)

// invokeAgentOnce 发一次远程调用，返回 HTTP 码与解析后的结果。
func invokeAgentOnce(t *testing.T, srvURL, peerID, tool string) (int, peerlink.AgentInvokeResult) {
	t.Helper()
	req, _ := http.NewRequest("POST", srvURL+"/api/peerlink/agent/invoke",
		bytes.NewReader([]byte(`{"peerId":"`+peerID+`","tool":"`+tool+`","args":{}}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Peerlink-Operator", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	var out peerlink.AgentInvokeResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("响应不是 AgentInvokeResult: %v", err)
	}
	return resp.StatusCode, out
}

// TestPeerlinkAgentInvoke_ToolErrorPayload_OkFalse —— 工具业务失败 ⇒ ok=false（真机形状）
func TestPeerlinkAgentInvoke_ToolErrorPayload_OkFalse(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	// 真机上抓到的原始形状：只有 error，没有 message/detail
	peerID, stop := startPairedEdgeFull(t, r, s, srv.URL+"/api/peerlink", nil, nil,
		func(req peerlink.AgentInvokeRequest) peerlink.AgentInvokeOutcome {
			return peerlink.AgentInvokeOutcome{
				Decision: peerlink.DecisionAuto,
				Result:   json.RawMessage(`{"error":"mount_id is required"}`),
			}
		})
	defer stop()

	code, out := invokeAgentOnce(t, srv.URL, peerID, "search_files")
	if code != http.StatusOK {
		t.Fatalf("RPC 送达应仍 200, got %d", code)
	}
	if out.Ok {
		t.Fatalf("工具业务失败不得 ok=true: %+v", out)
	}
	if out.ErrorCode != "mount_id is required" {
		t.Fatalf("errorCode 应为工具返回的 code, got %q", out.ErrorCode)
	}
	if out.Error == "" {
		t.Fatal("error 不得为空（msg 缺失时应回退为 code）")
	}
	// result 必须原样保留 —— 既有调用方仍按 result 解析，不能断
	if !strings.Contains(string(out.Result), "mount_id is required") {
		t.Fatalf("result 应原样保留, got %s", string(out.Result))
	}
}

// TestPeerlinkAgentInvoke_ToolErrorPayload_WithMessage —— errJSON 完整形状同样成立
func TestPeerlinkAgentInvoke_ToolErrorPayload_WithMessage(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdgeFull(t, r, s, srv.URL+"/api/peerlink", nil, nil,
		func(req peerlink.AgentInvokeRequest) peerlink.AgentInvokeOutcome {
			return peerlink.AgentInvokeOutcome{
				Decision: peerlink.DecisionAuto,
				Result:   json.RawMessage(`{"error":"missing_args","message":"mount_id 和 rel_path 必填"}`),
			}
		})
	defer stop()

	_, out := invokeAgentOnce(t, srv.URL, peerID, "read_file")
	if out.Ok {
		t.Fatalf("errJSON 形状应判为失败: %+v", out)
	}
	if out.ErrorCode != "missing_args" || out.Error != "mount_id 和 rel_path 必填" {
		t.Fatalf("errorCode/error 提取错误: code=%q err=%q", out.ErrorCode, out.Error)
	}
}

// TestPeerlinkAgentInvoke_NormalPayload_StaysOk —— 反向锁：正常载荷不得被误判
func TestPeerlinkAgentInvoke_NormalPayload_StaysOk(t *testing.T) {
	r, s := newPeerlinkRouter()
	srv := httptest.NewServer(r)
	defer srv.Close()

	peerID, stop := startPairedEdgeFull(t, r, s, srv.URL+"/api/peerlink", nil, nil,
		func(req peerlink.AgentInvokeRequest) peerlink.AgentInvokeOutcome {
			// 含 error 字段但**不是** errJSON 形状（有其它键）⇒ 必须仍算成功
			return peerlink.AgentInvokeOutcome{
				Decision: peerlink.DecisionAuto,
				Result:   json.RawMessage(`{"count":1,"items":[{"error":"sub-item failed"}]}`),
			}
		})
	defer stop()

	_, out := invokeAgentOnce(t, srv.URL, peerID, "list_files")
	if !out.Ok {
		t.Fatalf("含其它键的载荷不是 errJSON，不得判为失败: %+v", out)
	}
	if out.ErrorCode != "" {
		t.Fatalf("成功响应不应带 errorCode: %q", out.ErrorCode)
	}
}

// TestAgentToolErrorOf —— 判定函数的边界（纯函数，快速跑）
func TestAgentToolErrorOf(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		isErr   bool
		code    string
		msg     string
	}{
		{"空载荷", ``, false, "", ""},
		{"数组载荷", `["error"]`, false, "", ""},
		{"纯成功载荷", `{"count":2,"items":[]}`, false, "", ""},
		{"只有 error（真机形状）", `{"error":"mount_id is required"}`, true, "mount_id is required", "mount_id is required"},
		{"error+message", `{"error":"missing_args","message":"x 必填"}`, true, "missing_args", "x 必填"},
		{"error+detail", `{"error":"read_failed","detail":"permission denied"}`, true, "read_failed", "permission denied"},
		{"error 是空串", `{"error":"","message":"x"}`, false, "", ""},
		{"error 非字符串", `{"error":42}`, false, "", ""},
		{"带额外键不误判", `{"error":"x","count":1}`, false, "", ""},
	}
	for _, tc := range cases {
		code, msg, isErr := agentToolErrorOf(json.RawMessage(tc.payload))
		if isErr != tc.isErr {
			t.Errorf("%s: isErr=%v want %v", tc.name, isErr, tc.isErr)
			continue
		}
		if !isErr {
			continue
		}
		if code != tc.code || msg != tc.msg {
			t.Errorf("%s: got (%q,%q) want (%q,%q)", tc.name, code, msg, tc.code, tc.msg)
		}
	}
}
