package peerlink

// hub_sweep_r11_test.go —— P2c Task 2.15 剩余（**R11**：Hub session 清理 + pairingId 逐出）
//
// 背景（为什么要这把锁）：票据"取出即销毁"只覆盖了**被消费**的情况 ——
//   - 没人扫的票据：**无人清理**，只在使用时才判过期 ⇒ 长跑进程内存里一直堆；
//   - 配完之后的 `pairedByTicket` 回执：**没有任何清理路径** ⇒ 只增不减。
//
// 契约：
//   1. Sweep 清掉过期未消费票据；
//   2. Sweep 清掉超过 PairedResultTTL 的配对结果（PairingStatus 随之返回 nil）；
//   3. 结果数量超过 MaxPairedResults 时逐出**最旧**的一批；
//   4. **绝不**碰 sessions / peers —— token 必须能在断线重连后继续用，
//      已配对设备要显示为"离线"而不是凭空消失（唯一清理入口是 Unpair）。

import (
	"testing"
	"time"
)

func pairOnce(t *testing.T, h *Hub, deviceID string) string {
	t.Helper()
	tk, err := h.CreateTicket("https://hub.test/api/peerlink", DefaultTicketTTL)
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	psk, _ := DecodePSK(tk.PSKHex)
	if _, err := h.Pair(tk.PairingID, deviceID, "Pixel", "android", Proof(psk, tk.PairingID, deviceID)); err != nil {
		t.Fatalf("Pair: %v", err)
	}
	return tk.PairingID
}

func TestHub_Sweep_RemovesExpiredTickets(t *testing.T) {
	h := NewHub("test-hub", "test")

	// 一枚马上过期的票据（没人扫）+ 一枚正常票据
	if _, err := h.CreateTicket("https://hub.test/api/peerlink", time.Millisecond); err != nil {
		t.Fatalf("CreateTicket(短): %v", err)
	}
	if _, err := h.CreateTicket("https://hub.test/api/peerlink", DefaultTicketTTL); err != nil {
		t.Fatalf("CreateTicket(长): %v", err)
	}
	if n := h.PendingTickets(); n != 2 {
		t.Fatalf("建票后应有 2 枚待消费票据, got %d", n)
	}

	time.Sleep(20 * time.Millisecond)
	st := h.Sweep(time.Now())
	if st.Tickets != 1 {
		t.Fatalf("应只清掉过期的那 1 枚, got %+v", st)
	}
	if n := h.PendingTickets(); n != 1 {
		t.Fatalf("未过期的票据必须保留, got %d", n)
	}
}

func TestHub_Sweep_EvictsStaleAndOverflowPairingResults(t *testing.T) {
	h := NewHub("test-hub", "test")

	// ① 过期逐出：配对结果超过 TTL 后应消失（PairingStatus 随之 nil）
	id := pairOnce(t, h, "dev-sweep-1")
	if h.PairingStatus(id) == nil {
		t.Fatalf("刚配对应能查到配对状态")
	}
	st := h.Sweep(time.Now().Add(PairedResultTTL + time.Minute))
	if st.PairResults != 1 {
		t.Fatalf("过期配对结果应被清理, got %+v", st)
	}
	if h.PairingStatus(id) != nil {
		t.Fatalf("清理后 PairingStatus 应返回 nil")
	}

	// ② 上限逐出：结果数不得超过 MaxPairedResults（多出来的按最旧先走）
	for i := 0; i < MaxPairedResults+8; i++ {
		pairOnce(t, h, "dev-sweep-bulk")
		h.Sweep(time.Now()) // 不推进时间 ⇒ 只触发上限逐出，不触发 TTL 逐出
	}
	if n := h.PairedResults(); n > MaxPairedResults {
		t.Fatalf("配对结果数应被压在上限内, got %d > %d", n, MaxPairedResults)
	}
}

func TestHub_Sweep_NeverTouchesSessionsOrPeers(t *testing.T) {
	h := NewHub("test-hub", "test")

	tk, _ := h.CreateTicket("https://hub.test/api/peerlink", DefaultTicketTTL)
	psk, _ := DecodePSK(tk.PSKHex)
	res, err := h.Pair(tk.PairingID, "dev-sweep-2", "Pixel", "android", Proof(psk, tk.PairingID, "dev-sweep-2"))
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}

	// 清到"天荒地老"：会话与已配对设备必须还在（离线要能显示、重连要能复用 token）
	h.Sweep(time.Now().Add(24 * 365 * time.Hour))

	if pid, ok := h.PeerIDForToken(res.Token); !ok || pid != res.PeerID {
		t.Fatalf("Sweep 不得清掉会话（token 仍应可反查 peer）, got %q ok=%v", pid, ok)
	}
	if !h.Heartbeat(res.Token) {
		t.Fatalf("Sweep 后心跳仍应有效")
	}
	found := false
	for _, p := range h.ListPeers() {
		if p["id"] == res.PeerID {
			found = true
		}
	}
	if !found {
		t.Fatalf("Sweep 不得清掉已配对设备（应在列表里显示为离线）")
	}
}
