package peerlink

// store.go —— 互联状态的**持久化**（2026-10-04，决策变更）
//
// ⚠️ 决策变更（用户 2026-10-04 拍板）：原纪律是"psk / token / 密钥只存进程内存，
//    绝不写盘"，代价是**服务端一重启，已配对设备全部失效、必须重新扫码**
//    ——— 真机调试时每次改后端都要重新扫码，不可接受。
//
// 新纪律（"把钥匙锁进抽屉"，而不是"把钥匙扔了"）：
//  1. 落盘位置 = **应用私有数据目录**（Android: app filesDir/.encv/peerlink，
//     桌面: XDG/LOCALAPPDATA 下），不是 servingDir、不是用户可见存储；
//  2. 文件权限 **0600**、目录 **0700**；写入走"临时文件 + rename"保证原子
//     （半截文件会让"恢复"读到一份坏状态，比没有更糟）；
//  3. **可撤销**：解配（unpair）立即删记录；`ENCV_PEERLINK_PERSIST=0` 可整体关闭
//     （关闭后行为退回旧纪律：重启即失效）；
//  4. psk 仍**不落盘**（它只在扫码那一次有用），落的是配对后派生的会话凭据与 AEAD 密钥
//     —— 丢了它就得重新扫码，这正是要避免的；泄漏面等价于"设备被信任"，与 trust_device 同级。

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// StoredPeer 一台已配对设备的可持久状态（Hub 侧）。
type StoredPeer struct {
	Peer      Peer  `json:"peer"`
	Token     string `json:"token"`
	RemoteKey []byte `json:"remoteKey"` // Hub→Edge AEAD 密钥
	LocalKey  []byte `json:"localKey"`  // Edge→Hub AEAD 密钥
}

// EdgeSession 本端作为 Edge 连远端 Hub 的会话（设备侧）。
type EdgeSession struct {
	Hub      string    `json:"hub"`
	Token    string    `json:"token"`
	PeerID   string    `json:"peerId,omitempty"`
	DeviceID string    `json:"deviceId,omitempty"`
	Name     string    `json:"name,omitempty"`
	Platform string    `json:"platform,omitempty"`
	SavedAt  time.Time `json:"savedAt"`
}

// Store 互联状态的落盘读写。
//
// dir 由调用方传入（server 侧传 config.AppDataDir("peerlink")），
// 本包**不依赖 config**，保持纯逻辑、易测。
type Store struct {
	mu  sync.Mutex
	dir string
}

// NewStore 创建存储（dir 为空 = 禁用持久化，所有写入是 no-op）。
func NewStore(dir string) *Store { return &Store{dir: dir} }

// Dir 返回落盘目录（空串表示禁用）。
func (st *Store) Dir() string { return st.dir }

// Enabled 是否启用持久化。
func (st *Store) Enabled() bool { return st != nil && st.dir != "" }

// ErrDisabled 持久化被关闭（调用方应静默忽略，不算错误）。
var ErrDisabled = errors.New("peerlink: persistence disabled")

func (st *Store) path(name string) (string, error) {
	if !st.Enabled() {
		return "", ErrDisabled
	}
	if err := os.MkdirAll(st.dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(st.dir, name), nil
}

// writeJSONAtomic 原子写：临时文件 → fsync → rename。
func (st *Store) writeJSONAtomic(name string, v any) error {
	p, err := st.path(name)
	if err != nil {
		return err
	}
	st.mu.Lock()
	defer st.mu.Unlock()

	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (st *Store) readJSON(name string, out any) error {
	p, err := st.path(name)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// ── Hub 侧：已配对设备 ──────────────────────────────────────────

// SavePeers 覆盖写入已配对设备（Hub 侧，成对变化时调用）。
func (st *Store) SavePeers(items []StoredPeer) error {
	if !st.Enabled() {
		return ErrDisabled
	}
	if items == nil {
		items = []StoredPeer{}
	}
	return st.writeJSONAtomic("peers.json", items)
}

// LoadPeers 读回已配对设备。文件不存在返回空切片（不是错误）。
func (st *Store) LoadPeers() ([]StoredPeer, error) {
	if !st.Enabled() {
		return nil, ErrDisabled
	}
	var items []StoredPeer
	if err := st.readJSON("peers.json", &items); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []StoredPeer{}, nil
		}
		// 坏文件绝不静默吞：返回错误让调用方记日志并**不恢复**（保守）
		return nil, fmt.Errorf("peerlink: peers.json 损坏: %w", err)
	}
	return items, nil
}

// ── Hub 侧：配对票据（2026-10-05）──────────────────────────────
//
// 为什么票据也要落盘（与会话同一套纪律）：
//
//	I1 只把「会话」落盘了，票据仍在内存 ⇒ 后端一重启，桌面上**正在展示的二维码**
//	立刻失效，手机拿它来配对必然 401（`ticket not found (consumed or unknown)`）
//	⇒ 真机表现就是"热更/重启后连不上"（2026-10-05 事故，见 docs/HANDOVER）。
//	票据与会话是同一次配对的两半，只持久化一半 = 设计不一致。
//
// 边界（降低"psk 落盘"的代价）：
//  1. 只落**未消费**的票据（消费完立刻从 pending 移到 used，used 只留 pairingId +
//     过期时间，**不带 psk**）⇒ 磁盘上的 psk 最长只活到票据过期（默认 120s）；
//  2. 读回时先按 ExpiresAt 清一遍（过期的票据绝不能被"复活"）；
//  3. 与 peers.json 同一目录、同一套 0600 + 原子写；`ENCV_PEERLINK_PERSIST=0` 同样关闭。

// UsedTicket 一张**已消费**（或已过期）票据的痕迹。
//
// ⚠️ 刻意**不含 psk**：它唯一的用途是区分「这张码被用过了」与「根本没这张码」
// （否则排障时日志里只能看到一句含糊的 "consumed or unknown"）。
type UsedTicket struct {
	PairingID string    `json:"pairingId"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// TicketSnapshot 票据的落盘快照。
type TicketSnapshot struct {
	Pending []Ticket     `json:"pending"`
	Used    []UsedTicket `json:"used"`
}

// SaveTickets 覆盖写入票据快照。
func (st *Store) SaveTickets(snap TicketSnapshot) error {
	if !st.Enabled() {
		return ErrDisabled
	}
	if snap.Pending == nil {
		snap.Pending = []Ticket{}
	}
	if snap.Used == nil {
		snap.Used = []UsedTicket{}
	}
	return st.writeJSONAtomic("tickets.json", snap)
}

// LoadTickets 读回票据快照；文件不存在返回空快照（不是错误）。
func (st *Store) LoadTickets() (TicketSnapshot, error) {
	empty := TicketSnapshot{Pending: []Ticket{}, Used: []UsedTicket{}}
	if !st.Enabled() {
		return empty, ErrDisabled
	}
	var snap TicketSnapshot
	if err := st.readJSON("tickets.json", &snap); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return empty, nil
		}
		return empty, fmt.Errorf("peerlink: tickets.json 损坏: %w", err)
	}
	return snap, nil
}

// ── 通用状态（供 Hub 侧其它台账复用同一套落盘纪律）──────────────────
//
// name 只能是**文件名**（不得含路径分隔符 / 上级引用），否则一律拒绝 ——
// 调用方是内部代码，但"拼路径"这类口子留着迟早被外部输入碰到。

func safeStateName(name string) bool {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return false
	}
	return true
}

// SaveState 原子写入一份命名状态（0600）。
func (st *Store) SaveState(name string, v any) error {
	if !st.Enabled() {
		return ErrDisabled
	}
	if !safeStateName(name) {
		return fmt.Errorf("peerlink: 非法的状态文件名 %q", name)
	}
	return st.writeJSONAtomic(name, v)
}

// LoadState 读回命名状态；ok=false 表示文件不存在（不是错误）。
func (st *Store) LoadState(name string, out any) (bool, error) {
	if !st.Enabled() {
		return false, ErrDisabled
	}
	if !safeStateName(name) {
		return false, fmt.Errorf("peerlink: 非法的状态文件名 %q", name)
	}
	if err := st.readJSON(name, out); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("peerlink: %s 损坏: %w", name, err)
	}
	return true, nil
}

// ── Edge 侧：连 Hub 的会话 ──────────────────────────────────────

// SaveEdgeSession 保存本端作为 Edge 的会话（扫码配对成功后调用）。
func (st *Store) SaveEdgeSession(s EdgeSession) error {
	if !st.Enabled() {
		return ErrDisabled
	}
	if s.SavedAt.IsZero() {
		s.SavedAt = time.Now()
	}
	return st.writeJSONAtomic("edge-session.json", s)
}

// LoadEdgeSession 读回 Edge 会话；没有返回 ok=false。
func (st *Store) LoadEdgeSession() (EdgeSession, bool, error) {
	if !st.Enabled() {
		return EdgeSession{}, false, ErrDisabled
	}
	var s EdgeSession
	if err := st.readJSON("edge-session.json", &s); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return EdgeSession{}, false, nil
		}
		return EdgeSession{}, false, fmt.Errorf("peerlink: edge-session.json 损坏: %w", err)
	}
	if s.Hub == "" || s.Token == "" {
		return EdgeSession{}, false, nil
	}
	return s, true, nil
}

// ClearEdgeSession 删除 Edge 会话（解配 / 主动断开时调用）。
func (st *Store) ClearEdgeSession() error {
	if !st.Enabled() {
		return ErrDisabled
	}
	p, err := st.path("edge-session.json")
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Clear 清空全部持久化状态（"忘记所有设备"）。
func (st *Store) Clear() error {
	if !st.Enabled() {
		return ErrDisabled
	}
	_ = st.ClearEdgeSession()
	p, err := st.path("peers.json")
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// ── Hub 的导出 / 恢复 ──────────────────────────────────────────

// ExportState 导出 Hub 当前全部已配对设备（供落盘）。
func (h *Hub) ExportState() []StoredPeer {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]StoredPeer, 0, len(h.peers))
	for _, p := range h.peers {
		cp := *p
		out = append(out, StoredPeer{Peer: cp, Token: tokenOf(h, p.ID), RemoteKey: p.RemoteKey, LocalKey: p.LocalKey})
	}
	return out
}

// tokenOf 反查某 peer 的 token（Hub 内部：sessions 是 token→session）。
func tokenOf(h *Hub, peerID string) string {
	for tok, s := range h.sessions {
		if s.PeerID == peerID {
			return tok
		}
	}
	return ""
}

// RestoreState 把落盘的设备恢复到内存，返回恢复条数。
//
// ⚠️ 恢复的是"**已配对**"状态：设备仍需重新建立 WS 长连接才会 Online。
//    这正是"重启后自动恢复"的关键 —— Hub 记得它，它带着旧 token 连回来即可，
//    不需要重新扫码。
func (h *Hub) RestoreState(items []StoredPeer) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, it := range items {
		if it.Peer.ID == "" || it.Token == "" {
			continue
		}
		p := it.Peer
		p.Online = false // 在线状态由长连接重建，绝不从磁盘"恢复"成在线
		p.RemoteKey = it.RemoteKey
		p.LocalKey = it.LocalKey
		h.peers[p.ID] = &p
		h.sessions[it.Token] = &Session{PeerID: p.ID, Token: it.Token, Created: p.PairedAt}
		n++
	}
	return n
}
