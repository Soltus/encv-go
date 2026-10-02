package peerlink

// peerlink_test.go —— P2a 契约测试（spec desktop-web-android-pairing）
//
// 覆盖：票据一次性 / 过期、proof 校验、配对成功、心跳、解配即失效、
// AEAD 双向密钥、SAS 双端一致（防 Hub 侧 MITM 的人工核对依据）。

import (
	"bytes"
	"testing"
	"time"
)

func TestCreateTicket_Pair_HappyPath(t *testing.T) {
	h := NewHub("cnb-hub", "test")
	tk, err := h.CreateTicket("https://hub.example/api/peerlink", DefaultTicketTTL)
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	if tk.PairingID == "" || tk.PSKHex == "" || tk.Hub == "" {
		t.Fatalf("ticket 字段不完整: %+v", tk)
	}

	psk, err := DecodePSK(tk.PSKHex)
	if err != nil {
		t.Fatalf("DecodePSK: %v", err)
	}
	proof := Proof(psk, tk.PairingID, "device-abc-123456")

	res, err := h.Pair(tk.PairingID, "device-abc-123456", "Pixel", "android", proof)
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if res.Token == "" || res.PeerID == "" || res.SAS == "" {
		t.Fatalf("PairResult 不完整: %+v", res)
	}
	if len(res.SAS) != 6 {
		t.Fatalf("SAS 应为 6 位, got %q", res.SAS)
	}
	if _, ok := h.SessionByToken(res.Token); !ok {
		t.Fatal("配对后应能用 token 取到会话")
	}
	if !h.Heartbeat(res.Token) {
		t.Fatal("心跳应成功")
	}
}

func TestTicket_OneTime(t *testing.T) {
	h := NewHub("hub", "test")
	tk, _ := h.CreateTicket("https://hub/api/peerlink", time.Minute)
	psk, _ := DecodePSK(tk.PSKHex)
	proof := Proof(psk, tk.PairingID, "dev-1")

	if _, err := h.Pair(tk.PairingID, "dev-1", "A", "android", proof); err != nil {
		t.Fatalf("首次配对应成功: %v", err)
	}
	if _, err := h.Pair(tk.PairingID, "dev-1", "A", "android", proof); err != ErrTicketUsed {
		t.Fatalf("第二次配对应报 ErrTicketUsed（一次性）, got %v", err)
	}
}

func TestTicket_Expired(t *testing.T) {
	h := NewHub("hub", "test")
	tk, _ := h.CreateTicket("https://hub/api/peerlink", 5*time.Millisecond)
	time.Sleep(20 * time.Millisecond)

	psk, _ := DecodePSK(tk.PSKHex)
	proof := Proof(psk, tk.PairingID, "dev-1")
	if _, err := h.Pair(tk.PairingID, "dev-1", "A", "android", proof); err != ErrTicketExpired {
		t.Fatalf("过期票据应报 ErrTicketExpired, got %v", err)
	}
}

func TestPair_BadProof(t *testing.T) {
	h := NewHub("hub", "test")
	tk, _ := h.CreateTicket("https://hub/api/peerlink", time.Minute)

	// 用错的 psk 生成 proof（模拟没扫到二维码的伪造方）
	wrongPSK := make([]byte, 32)
	proof := Proof(wrongPSK, tk.PairingID, "dev-1")
	if _, err := h.Pair(tk.PairingID, "dev-1", "A", "android", proof); err != ErrBadProof {
		t.Fatalf("错误 proof 应报 ErrBadProof, got %v", err)
	}
}

func TestUnpair_InvalidatesToken(t *testing.T) {
	h := NewHub("hub", "test")
	tk, _ := h.CreateTicket("https://hub/api/peerlink", time.Minute)
	psk, _ := DecodePSK(tk.PSKHex)
	res, err := h.Pair(tk.PairingID, "dev-1", "A", "android", Proof(psk, tk.PairingID, "dev-1"))
	if err != nil {
		t.Fatalf("Pair: %v", err)
	}
	if ok := h.Unpair(res.PeerID); !ok {
		t.Fatal("Unpair 应返回 true")
	}
	if _, ok := h.SessionByToken(res.Token); ok {
		t.Fatal("解配后 token 必须立即失效")
	}
	if h.Heartbeat(res.Token) {
		t.Fatal("解配后心跳必须失败")
	}
}

func TestHeartbeat_UnknownToken(t *testing.T) {
	h := NewHub("hub", "test")
	if h.Heartbeat("nope") {
		t.Fatal("未知 token 心跳应失败")
	}
}

func TestDeriveKeys_SealOpen_BothDirections(t *testing.T) {
	psk, _ := NewPSK()
	a2b, b2a, err := DeriveKeys(psk)
	if err != nil {
		t.Fatalf("DeriveKeys: %v", err)
	}
	if bytes.Equal(a2b, b2a) {
		t.Fatal("两个方向的密钥必须不同")
	}

	msg := []byte(`{"q":"报告.pdf"}`)
	ct, err := Seal(a2b, msg)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(ct, msg) {
		t.Fatal("密文中不得出现明文（Hub 不可信）")
	}
	pt, err := Open(a2b, ct)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(pt, msg) {
		t.Fatalf("往返不一致: %q", pt)
	}
	// 用另一方向的密钥解不开（Hub 即便拿到密文也没有对应密钥）
	if _, err := Open(b2a, ct); err == nil {
		t.Fatal("用错误方向密钥不应解开")
	}
}

func TestSAS_DeterministicAcrossSides(t *testing.T) {
	psk, _ := NewPSK()
	// 双端各自从同一个 psk 派生，必须一致（人工核对的前提）
	if SAS(psk) != SAS(psk) {
		t.Fatal("SAS 必须确定性")
	}
	other, _ := NewPSK()
	if SAS(psk) == SAS(other) {
		t.Fatal("不同 psk 的 SAS 不应相同（除极小概率碰撞）")
	}
}

func TestListPeers_NoKeysLeaked(t *testing.T) {
	h := NewHub("hub", "test")
	tk, _ := h.CreateTicket("https://hub/api/peerlink", time.Minute)
	psk, _ := DecodePSK(tk.PSKHex)
	if _, err := h.Pair(tk.PairingID, "dev-1", "Pixel", "android", Proof(psk, tk.PairingID, "dev-1")); err != nil {
		t.Fatalf("Pair: %v", err)
	}
	list := h.ListPeers()
	if len(list) != 1 {
		t.Fatalf("应有 1 个 peer, got %d", len(list))
	}
	for _, k := range []string{"RemoteKey", "LocalKey"} {
		if _, ok := list[0][k]; ok {
			t.Fatalf("ListPeers 不得包含密钥字段 %s", k)
		}
	}
}
