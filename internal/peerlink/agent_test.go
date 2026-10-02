package peerlink

// agent_test.go —— P4 远程 Agent 授权模型（**执行端**）
//
// 锁定红线：
//  1. 挂起超时 → **自动 decline**（绝不默认同意）
//  2. `trust_device` 只影响**非破坏性**工具；破坏性工具即使已信任也强制确认（Task 4.6）
//  3. 信任态**进程级**：新 Approver（等价进程重启）信任表必然为空（Task 4.4 回归锁）
//  4. 审计**脱敏**：不记参数全文/路径全文（R14）

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func destructiveSet(names ...string) func(string) bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return func(tool string) bool { return m[tool] }
}

func TestApprover_RequireApproval_ThenAccept(t *testing.T) {
	var notified []ApprovalRequest
	ap := NewApprover(ApproverOptions{
		Timeout:       2 * time.Second,
		IsDestructive: destructiveSet("encrypt_video"),
		Notify:        func(r ApprovalRequest) { notified = append(notified, r) },
	})

	req := AgentInvokeRequest{CallId: "c1", Tool: "read_file", Args: json.RawMessage(`{"path":"/sdcard/a.txt"}`)}
	done := make(chan struct {
		d string
		e error
	}, 1)
	go func() {
		d, err := ap.Require(context.Background(), req, "peer-a", "Desktop")
		done <- struct {
			d string
			e error
		}{d, err}
	}()

	// 等挂起登记
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(ap.Pending()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(ap.Pending()) != 1 {
		t.Fatalf("期望 1 条挂起, got %d", len(ap.Pending()))
	}
	if len(notified) != 1 || notified[0].CallId != "c1" {
		t.Fatalf("未推给执行端 UI: %+v", notified)
	}

	if err := ap.Decide("c1", DecisionAccept); err != nil {
		t.Fatalf("Decide 失败: %v", err)
	}
	res := <-done
	if res.e != nil || res.d != DecisionAccept {
		t.Fatalf("期望 accept, got %q err=%v", res.d, res.e)
	}
}

func TestApprover_Timeout_AutoDecline(t *testing.T) {
	ap := NewApprover(ApproverOptions{Timeout: 80 * time.Millisecond})
	req := AgentInvokeRequest{CallId: "c2", Tool: "read_file"}

	d, err := ap.Require(context.Background(), req, "peer-a", "Desktop")
	if !errors.Is(err, ErrDeclined) {
		t.Fatalf("超时应自动 decline（不是 accept）, got decision=%q err=%v", d, err)
	}
	if d != DecisionTimeout {
		t.Fatalf("决策应为 timeout, got %q", d)
	}
	if len(ap.Pending()) != 0 {
		t.Fatalf("超时后不应还有挂起")
	}
	// 审计应记录 timeout 决策
	entries := ap.Audit()
	if len(entries) != 1 || entries[0].Decision != DecisionTimeout {
		t.Fatalf("审计应记 timeout: %+v", entries)
	}
}

func TestApprover_TrustDevice_ProcessLevel_AndDestructiveForced(t *testing.T) {
	ap := NewApprover(ApproverOptions{
		Timeout:       60 * time.Second,
		IsDestructive: destructiveSet("encrypt_video"),
	})

	if !ap.RequiresApproval("peer-a", "read_file") {
		t.Fatalf("未信任时应需确认")
	}
	ap.Trust("peer-a")
	if ap.RequiresApproval("peer-a", "read_file") {
		t.Fatalf("已信任的非破坏性工具不应再询问")
	}
	// Task 4.6：破坏性工具即使已信任也强制确认
	if !ap.RequiresApproval("peer-a", "encrypt_video") {
		t.Fatalf("破坏性工具即使已信任也必须确认")
	}

	// 免确认路径：Require 立即返回 auto
	d, err := ap.Require(context.Background(), AgentInvokeRequest{Tool: "read_file"}, "peer-a", "Desktop")
	if err != nil || d != DecisionAuto {
		t.Fatalf("已信任应立即 auto, got %q err=%v", d, err)
	}
	// 破坏性仍挂起 → 超时 decline（短超时验证）
	ap2 := NewApprover(ApproverOptions{Timeout: 60 * time.Millisecond, IsDestructive: destructiveSet("encrypt_video")})
	ap2.Trust("peer-a")
	d2, err2 := ap2.Require(context.Background(), AgentInvokeRequest{Tool: "encrypt_video"}, "peer-a", "Desktop")
	if !errors.Is(err2, ErrDeclined) {
		t.Fatalf("破坏性工具不得因信任自动放行, got %q err=%v", d2, err2)
	}
}

// TestApprover_TrustIsProcessScoped —— 重启失效回归锁（Task 4.4）
func TestApprover_TrustIsProcessScoped(t *testing.T) {
	ap := NewApprover(ApproverOptions{Timeout: time.Second})
	ap.Trust("peer-a")
	if !ap.Trusted("peer-a") || len(ap.TrustedPeers()) != 1 {
		t.Fatalf("信任未生效")
	}
	ap.Untrust("peer-a")
	if ap.Trusted("peer-a") {
		t.Fatalf("撤销失败")
	}
	// 模拟"进程重启"：新建实例 = 全新内存
	fresh := NewApprover(ApproverOptions{Timeout: time.Second})
	if len(fresh.TrustedPeers()) != 0 || fresh.Trusted("peer-a") {
		t.Fatalf("重启后信任态必须为空（trust_device 只存进程内存）")
	}
}

func TestApprover_DecisionViaTrustDevice_ThenAutoNext(t *testing.T) {
	ap := NewApprover(ApproverOptions{Timeout: 2 * time.Second})
	done := make(chan string, 1)
	go func() {
		d, _ := ap.Require(context.Background(), AgentInvokeRequest{CallId: "c3", Tool: "read_file"}, "peer-b", "Desktop")
		done <- d
	}()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && len(ap.Pending()) == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if err := ap.Decide("c3", DecisionTrust); err != nil {
		t.Fatalf("Decide(trust_device) 失败: %v", err)
	}
	if got := <-done; got != DecisionTrust {
		t.Fatalf("期望 trust_device, got %q", got)
	}
	if !ap.Trusted("peer-b") {
		t.Fatalf("trust_device 应记住该设备")
	}
}

func TestApprover_Decide_Errors(t *testing.T) {
	ap := NewApprover(ApproverOptions{Timeout: time.Second})
	if err := ap.Decide("nope", DecisionAccept); !errors.Is(err, ErrNoPending) {
		t.Fatalf("未知 callId 应 ErrNoPending, got %v", err)
	}
	if err := ap.Decide("nope", "maybe"); !errors.Is(err, ErrBadDecision) {
		t.Fatalf("非法 decision 应 ErrBadDecision, got %v", err)
	}
}

// TestApprover_AuditSanitized —— R14：审计不得出现参数/路径全文
func TestApprover_AuditSanitized(t *testing.T) {
	// 短超时：本例只关心"审计里写了什么"，不关心走完人工决策
	ap := NewApprover(ApproverOptions{Timeout: 50 * time.Millisecond})
	secretPath := "/sdcard/私密/工资单.pdf"
	req := AgentInvokeRequest{
		CallId: "c4",
		Tool:   "read_file",
		Args:   json.RawMessage(`{"path":"` + secretPath + `","limit":10}`),
	}
	// 决策结果不重要（短超时会 decline），本例只校验审计内容
	_, _ = ap.Require(context.Background(), req, "peer-a", "Desktop")
	entries := ap.Audit()
	if len(entries) != 1 {
		t.Fatalf("应有 1 条审计, got %d", len(entries))
	}
	e := entries[0]
	raw, _ := json.Marshal(e)
	if strings.Contains(string(raw), secretPath) || strings.Contains(string(raw), "工资单") {
		t.Fatalf("审计泄露了路径全文: %s", string(raw))
	}
	if e.ArgBytes == 0 {
		t.Fatalf("应记录参数字节数")
	}
	if len(e.ArgKeys) == 0 {
		t.Fatalf("应记录参数顶层键（脱敏后）")
	}
}
