package peerlink

// hub_ticket_persist_test.go —— 配对票据的**持久化**与**错误三态**回归锁（2026-10-05）
//
// 起因（真机事故，见 docs/HANDOVER-cloud-hot-update.md §4）：
//   - 票据只存进程内存 ⇒ 后端一重启，桌面端正在展示的二维码立刻失效，
//     手机拿旧码配对必然 401（`ticket not found (consumed or unknown)`）⇒ "连不上"；
//   - redeem() 把「不存在 / 已使用」合成**同一个** sentinel ⇒ 日志里永远分不清
//     用户是扫了旧码、码过期了、还是后端重启过。
//
// 锁的四条语义：
//  1. 票据与会话一起落盘 ⇒ 重启后旧码仍然可用
//  2. **过期票据不得被"复活"**（读回时按 ExpiresAt 清一遍）
//  3. 不存在 / 已过期 / 已使用 必须是三个**不同**的错误
//  4. 落盘的"已消费"痕迹**不含 psk**（它只用来回答"这张码用过了吗"）

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHub_Ticket_PersistAndRestoreAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	h := NewHub("h", "v")
	h.SetStore(NewStore(dir))

	tk, err := h.CreateTicket("http://127.0.0.1:2025/api/peerlink", 0)
	if err != nil {
		t.Fatal(err)
	}
	// 待用票据必须真的落盘（否则重启即丢 —— 这正是事故根因）
	raw, err := os.ReadFile(filepath.Join(dir, "tickets.json"))
	if err != nil {
		t.Fatalf("出码后应落盘 tickets.json: %v", err)
	}
	if !strings.Contains(string(raw), tk.PairingID) {
		t.Fatalf("tickets.json 里没有这张票据: %s", raw)
	}

	// ── 重启：新 Hub，只靠磁盘恢复 ──
	h2 := NewHub("h", "v")
	h2.SetStore(NewStore(dir))
	if n := h2.RestoreTickets(); n != 1 {
		t.Fatalf("重启后应恢复 1 张待用票据, got %d", n)
	}
	if err := h2.LastPersistError(); err != nil {
		t.Fatalf("落盘/恢复不应出错: %v", err)
	}
	// 旧票据必须仍然可用（真机 "连不上" 的直接判据）
	psk, err := DecodePSK(tk.PSKHex)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h2.Pair(tk.PairingID, "dev-0001", "Pixel", "android", Proof(psk, tk.PairingID, "dev-0001")); err != nil {
		t.Fatalf("重启后旧票据应当仍然可用, got %v", err)
	}
}

// TestHub_Ticket_ErrorsDistinguishable —— 三种失败必须是三个不同错误（此前混成一个）
func TestHub_Ticket_ErrorsDistinguishable(t *testing.T) {
	h := NewHub("h", "v")

	// ① 已使用：真实票据先配一次，再配第二次
	tk, err := h.CreateTicket("http://127.0.0.1:2025/api/peerlink", 0)
	if err != nil {
		t.Fatal(err)
	}
	psk, _ := DecodePSK(tk.PSKHex)
	if _, err := h.Pair(tk.PairingID, "dev-1", "n", "android", Proof(psk, tk.PairingID, "dev-1")); err != nil {
		t.Fatalf("首次配对应成功: %v", err)
	}
	_, errUsed := h.Pair(tk.PairingID, "dev-1", "n", "android", Proof(psk, tk.PairingID, "dev-1"))
	if !errors.Is(errUsed, ErrTicketUsed) {
		t.Fatalf("二次配对应为 ErrTicketUsed, got %v", errUsed)
	}

	// ② 已过期：签发一张 20ms 就过期的码
	tkExp, err := h.CreateTicket("http://127.0.0.1:2025/api/peerlink", 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	pskExp, _ := DecodePSK(tkExp.PSKHex)
	_, errExpired := h.Pair(tkExp.PairingID, "dev-2", "n", "android", Proof(pskExp, tkExp.PairingID, "dev-2"))
	if !errors.Is(errExpired, ErrTicketExpired) {
		t.Fatalf("过期码应为 ErrTicketExpired, got %v", errExpired)
	}

	// ③ 不存在：一张压根没签发过的码
	_, errUnknown := h.Pair(hex.EncodeToString(make([]byte, 16)), "dev-3", "n", "android", "deadbeef")
	if !errors.Is(errUnknown, ErrTicketNotFound) {
		t.Fatalf("从未签发的码应为 ErrTicketNotFound, got %v", errUnknown)
	}

	// 反向锁：三者互不相同（合成一个 sentinel 就是这次要修的 bug 本身）
	if errors.Is(errUsed, ErrTicketNotFound) || errors.Is(errExpired, ErrTicketUsed) || errors.Is(errUnknown, ErrTicketUsed) {
		t.Fatalf("三种票据错误必须互不相等: used=%v expired=%v unknown=%v", errUsed, errExpired, errUnknown)
	}
}

// TestHub_Ticket_ExpiredNotRevivedOnRestore —— 过期票据绝不能被"读回来复活"
func TestHub_Ticket_ExpiredNotRevivedOnRestore(t *testing.T) {
	dir := t.TempDir()
	h := NewHub("h", "v")
	h.SetStore(NewStore(dir))
	tk, err := h.CreateTicket("http://127.0.0.1:2025/api/peerlink", 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)

	h2 := NewHub("h", "v")
	h2.SetStore(NewStore(dir))
	if n := h2.RestoreTickets(); n != 0 {
		t.Fatalf("过期票据不得被恢复（把失效的码复活比不持久化更危险）, got %d", n)
	}
	psk, _ := DecodePSK(tk.PSKHex)
	if _, err := h2.Pair(tk.PairingID, "dev-1", "n", "android", Proof(psk, tk.PairingID, "dev-1")); !errors.Is(err, ErrTicketNotFound) {
		t.Fatalf("恢复不了即视为不存在, got %v", err)
	}
}

// TestHub_Ticket_UsedRecordHasNoPSK —— 消费痕迹只存 ID + 过期时间，不得把 psk 留在盘上
func TestHub_Ticket_UsedRecordHasNoPSK(t *testing.T) {
	dir := t.TempDir()
	h := NewHub("h", "v")
	h.SetStore(NewStore(dir))
	tk, err := h.CreateTicket("http://127.0.0.1:2025/api/peerlink", 0)
	if err != nil {
		t.Fatal(err)
	}
	psk, _ := DecodePSK(tk.PSKHex)
	if _, err := h.Pair(tk.PairingID, "dev-1", "n", "android", Proof(psk, tk.PairingID, "dev-1")); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "tickets.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), tk.PSKHex) {
		t.Fatalf("票据消费后磁盘上不应再留 psk: %s", raw)
	}
	if !strings.Contains(string(raw), tk.PairingID) {
		t.Fatalf("消费痕迹应留下 pairingId（否则用没用过就问不出来了）: %s", raw)
	}
}
