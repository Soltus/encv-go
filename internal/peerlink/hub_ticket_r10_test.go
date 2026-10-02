package peerlink

// hub_ticket_r10_test.go —— P2c Task 2.14（**R10**）回归锁
//
// 契约：配对的安全判定**只认一次性票据（nonce 语义）**，不靠"时间新鲜度"做放行：
//   - 票据**取出即销毁**（一次性），无论成功还是失败都不复活 ⇒ 无重放、无重试 oracle；
//   - 过期只用于**拒绝**（且在取出之后判定），绝不用于**放行**；
//   - 因此"把时钟拨一拨"或"稍后再试"都不能让一枚票据多配一次。

import (
	"errors"
	"testing"
	"time"
)

func TestHub_Ticket_SecurityUsesOneShotNotFreshness(t *testing.T) {
	h := NewHub("test-hub", "test")

	// ① 正常路径：配对成功后票据即销毁（一次性）
	tk, err := h.CreateTicket("https://hub.test/api/peerlink", DefaultTicketTTL)
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	psk, _ := DecodePSK(tk.PSKHex)
	if _, err := h.Pair(tk.PairingID, "dev-r10-1", "Pixel", "android", Proof(psk, tk.PairingID, "dev-r10-1")); err != nil {
		t.Fatalf("首次配对应成功: %v", err)
	}
	if _, err := h.Pair(tk.PairingID, "dev-r10-1", "Pixel", "android", Proof(psk, tk.PairingID, "dev-r10-1")); !errors.Is(err, ErrTicketUsed) {
		t.Fatalf("票据必须一次性（第二次应 ErrTicketUsed）, got %v", err)
	}

	// ② 过期票据：判为过期，且**已销毁**（不是"等一会儿还能用"）
	exp, err := h.CreateTicket("https://hub.test/api/peerlink", time.Millisecond)
	if err != nil {
		t.Fatalf("CreateTicket(exp): %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	eps, _ := DecodePSK(exp.PSKHex)
	if _, err := h.Pair(exp.PairingID, "dev-r10-2", "Pixel", "android", Proof(eps, exp.PairingID, "dev-r10-2")); !errors.Is(err, ErrTicketExpired) {
		t.Fatalf("过期票据应 ErrTicketExpired, got %v", err)
	}
	if _, err := h.Pair(exp.PairingID, "dev-r10-2", "Pixel", "android", Proof(eps, exp.PairingID, "dev-r10-2")); !errors.Is(err, ErrTicketUsed) {
		t.Fatalf("过期票据也必须被销毁（不得复活）, got %v", err)
	}

	// ③ 错 proof：报 ErrBadProof，且票据**同样被销毁**（不给重试 oracle）
	tk3, _ := h.CreateTicket("https://hub.test/api/peerlink", DefaultTicketTTL)
	psk3, _ := DecodePSK(tk3.PSKHex)
	if _, err := h.Pair(tk3.PairingID, "dev-r10-3", "Pixel", "android", "deadbeef"); !errors.Is(err, ErrBadProof) {
		t.Fatalf("错 proof 应 ErrBadProof, got %v", err)
	}
	if _, err := h.Pair(tk3.PairingID, "dev-r10-3", "Pixel", "android", Proof(psk3, tk3.PairingID, "dev-r10-3")); !errors.Is(err, ErrTicketUsed) {
		t.Fatalf("错 proof 后票据应已销毁（无重试 oracle）, got %v", err)
	}

	// ④ pairingId 未知：与"已用过"同错（不区分时序信息，避免探测）
	if _, err := h.Pair("00000000000000000000000000000000", "dev-x", "X", "android", "x"); !errors.Is(err, ErrTicketUsed) {
		t.Fatalf("未知 pairingId 应 ErrTicketUsed, got %v", err)
	}
}
