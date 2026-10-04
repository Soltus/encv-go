package peerlink

// hub.go —— Hub 侧（cnb 上运行）：票据 / 配对 / 会话 / 心跳 / 解配
//
// 存储纪律（2026-10-05 修订）：
//   - **票据**与**会话**可落盘（应用私有目录 / 0600 / 原子写，`ENCV_PEERLINK_PERSIST=0` 可关）
//     ⇒ 后端重启后"正在展示的二维码"仍然有效、已配对设备带着旧 token 就能重连。
//     不这么做，重启 = 手机必然 401（真机"连不上"事故，见 docs/HANDOVER-cloud-hot-update.md §4）。
//   - **信任态（trust_device）** 仍只存内存：恢复的是"身份与通道"，不是"授权"。

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultTicketTTL 票据有效期（二维码 120s 过期，一次性）。
const DefaultTicketTTL = 120 * time.Second

// SessionIdleTTL 会话心跳超时（连续 3 次 15s 心跳未到即视为离线）。
const SessionIdleTTL = 45 * time.Second

// PairedResultTTL / MaxPairedResults —— 配对结果（桌面端轮询 `pairing/status` 用）的保留策略。
//
// ⚠️ 为什么必须有：票据是"取出即销毁"的，但**没人扫的票据**和**配完之后的配对结果**
// 都没有清理路径 ⇒ 长时间运行会一直堆在内存里（R11：Hub session 清理 / pairingId 逐出）。
// 结果与票据一样只存内存，逐出不影响安全（重新出码即可）。
const (
	PairedResultTTL  = 10 * time.Minute
	MaxPairedResults = 256
)

type Hub struct {
	mu sync.RWMutex

	name    string // 本端名称（展示用）
	peerID  string // 本端 peer id
	version string

	tickets  map[string]*Ticket
	peers    map[string]*Peer
	sessions map[string]*Session // token → session
	// used 已消费（或已过期）票据的痕迹：pairingID → 过期时间。
	//
	// ⚠️ 为什么要有它：票据是"取出即销毁"的，销毁后同一张码再来一次，
	//    与"压根没这张码"在内存里长得一模一样 ⇒ 日志只能写一句含糊的
	//    "not found (consumed or unknown)"，排障时根本分不清用户是扫了旧码、
	//    码过期了、还是后端重启过。留痕只存 ID + 过期时间（**不含 psk**）。
	used map[string]time.Time
	// pairedByTicket：以 pairingID 为键记录"哪张票据完成了配对"，供桌面端轮询配对状态。
	// 键是二维码里的 32 位 hex 秘密（只有扫码方与桌面端知道），因此**无需 token 即可按 ID 查询**。
	pairedByTicket map[string]*PairResult

	// store 票据的落盘通道（nil = 纯内存，与旧纪律一致）。
	store *Store
	// persistErr 最近一次落盘失败（不阻断流程，但要可观测 —— 静默失败比不持久化更难查）。
	persistErr error
}

// NewHub 创建一个 Hub（进程内单例）。
func NewHub(name, version string) *Hub {
	id, err := NewPairingID()
	if err != nil {
		// rand 失败在实践中不会发生；退化为空 id 也好于 panic
		id = "hub"
	}
	return &Hub{
		name:           name,
		peerID:         id,
		version:        version,
		tickets:        make(map[string]*Ticket),
		peers:          make(map[string]*Peer),
		sessions:       make(map[string]*Session),
		used:           make(map[string]time.Time),
		pairedByTicket: make(map[string]*PairResult),
	}
}

// SetStore 接上落盘通道（票据持久化；nil = 纯内存）。
//
// 为什么不在 NewHub 里传：Hub 本身是纯内存对象，持久化是"可选外挂"，
// 绝大多数单测不该被磁盘行为干扰（显式接上才启用）。
func (h *Hub) SetStore(st *Store) { h.store = st }

// SetDeviceID 指定本端设备指纹（跨重装稳定的 deviceId）。
//
// 默认 peerID 是**每次进程启动随机生成**的 ⇒ 同一台安卓机重装/清缓存后再配对，
// Hub 侧就多一条 peer 记录（真机实测同机两条）。设备指纹让"同一台设备"始终
// 是同一个 ID。见 internal/server/peerlink_device.go。
func (h *Hub) SetDeviceID(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	h.mu.Lock()
	h.peerID = id
	h.mu.Unlock()
}

// LastPersistError 最近一次票据落盘的失败原因（nil = 一切正常）。
func (h *Hub) LastPersistError() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.persistErr
}

// saveTickets 把票据快照落盘（无 store / 未启用 = no-op）。
func (h *Hub) saveTickets() {
	h.mu.Lock()
	snap := TicketSnapshot{
		Pending: make([]Ticket, 0, len(h.tickets)),
		Used:    make([]UsedTicket, 0, len(h.used)),
	}
	for _, t := range h.tickets {
		snap.Pending = append(snap.Pending, *t)
	}
	for id, exp := range h.used {
		snap.Used = append(snap.Used, UsedTicket{PairingID: id, ExpiresAt: exp})
	}
	st := h.store
	h.mu.Unlock()

	if st == nil || !st.Enabled() {
		return
	}
	err := st.SaveTickets(snap)
	h.mu.Lock()
	h.persistErr = err
	h.mu.Unlock()
}

// RestoreTickets 启动时把落盘票据读回内存，返回恢复的**待用**票据数。
//
// ⚠️ 过期的一律不恢复（读回就立刻按 ExpiresAt 过一遍）—— 把过期码"复活"
//    比没有持久化更危险：它会让一个本该失效的二维码重新可用。
func (h *Hub) RestoreTickets() int {
	if h.store == nil || !h.store.Enabled() {
		return 0
	}
	snap, err := h.store.LoadTickets()
	if err != nil {
		h.mu.Lock()
		h.persistErr = err
		h.mu.Unlock()
		return 0
	}
	now := time.Now()
	n := 0
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, t := range snap.Pending {
		if t.PairingID == "" || t.PSKHex == "" || now.After(t.ExpiresAt) {
			continue
		}
		if _, exists := h.tickets[t.PairingID]; exists {
			continue
		}
		cp := t
		h.tickets[cp.PairingID] = &cp
		n++
	}
	for _, u := range snap.Used {
		if u.PairingID == "" || now.After(u.ExpiresAt) {
			continue
		}
		h.used[u.PairingID] = u.ExpiresAt
	}
	return n
}

// Info 返回 hello 响应（未鉴权，只暴露非敏信息）。
func (h *Hub) Info() map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return map[string]string{
		"peerId":  h.peerID,
		"name":    h.name,
		"version": h.version,
	}
}

// SweepStats 一次清理的产出（观测 / 测试用）。
type SweepStats struct {
	Tickets     int // 清掉的过期票据
	PairResults int // 清掉的过期/超额配对结果
}

// Sweep 惰性清理（R11）：清掉**过期未消费**的票据与**过期**配对结果，并对配对结果做上限逐出。
//
// ⚠️ 刻意**不**清 sessions / peers：token 必须在断线重连后继续可用（Edge 心跳会重建连接），
//
//	已配对设备在列表里要显示为"离线"而不是凭空消失；它们的唯一清理入口是 `Unpair`。
func (h *Hub) Sweep(now time.Time) SweepStats {
	h.mu.Lock()
	var st SweepStats

	for id, t := range h.tickets {
		if now.After(t.ExpiresAt) {
			delete(h.tickets, id)
			st.Tickets++
		}
	}
	// 消费痕迹同样要清（它是"某张码用过"的证据，票据有效期一过就没有意义了）
	for id, exp := range h.used {
		if now.After(exp) {
			delete(h.used, id)
			st.Tickets++
		}
	}
	changed := st.Tickets > 0
	h.mu.Unlock()
	if changed {
		h.saveTickets()
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for id, res := range h.pairedByTicket {
		ts := time.Time{}
		if res != nil && res.Peer != nil {
			ts = res.Peer.PairedAt
		}
		if now.Sub(ts) > PairedResultTTL {
			delete(h.pairedByTicket, id)
			st.PairResults++
		}
	}

	// 上限逐出：超额时删最旧的一批（配对结果只是"配对完成"的回执，可安全丢弃）
	if over := len(h.pairedByTicket) - MaxPairedResults; over > 0 {
		type entry struct {
			id string
			ts time.Time
		}
		all := make([]entry, 0, len(h.pairedByTicket))
		for id, res := range h.pairedByTicket {
			ts := time.Time{}
			if res != nil && res.Peer != nil {
				ts = res.Peer.PairedAt
			}
			all = append(all, entry{id: id, ts: ts})
		}
		sort.Slice(all, func(i, j int) bool { return all[i].ts.Before(all[j].ts) })
		for i := 0; i < over && i < len(all); i++ {
			delete(h.pairedByTicket, all[i].id)
			st.PairResults++
		}
	}
	return st
}

// PendingTickets / PairedResults —— 观测与测试用计数（不泄漏任何密钥材料）。
func (h *Hub) PendingTickets() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.tickets)
}

func (h *Hub) PairedResults() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.pairedByTicket)
}

// CreateTicket 桌面端申请一次性配对票据（二维码内容来源）。
func (h *Hub) CreateTicket(hubURL string, ttl time.Duration) (*Ticket, error) {
	if ttl <= 0 {
		ttl = DefaultTicketTTL
	}
	// 惰性清理：桌面端每次出码都顺带扫一遍（不新增长驻 goroutine）
	h.Sweep(time.Now())
	id, err := NewPairingID()
	if err != nil {
		return nil, err
	}
	psk, err := NewPSK()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	t := &Ticket{
		PairingID: id,
		PSKHex:    PSKHex(psk),
		Hub:       hubURL,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	h.mu.Lock()
	h.tickets[id] = t
	h.mu.Unlock()
	// 落盘（2026-10-05）：重启后这张码必须还在 —— 否则"正在展示的二维码"会因
	// 后端重启而失效，手机扫它必然 401（真机"连不上"事故的根因）。
	h.saveTickets()
	return t, nil
}

// 票据错误三态（2026-10-05 拆分，用于映射 HTTP 401 的不同提示）。
//
// ⚠️ 为什么必须拆开：此前「不存在」与「已使用」共用 ErrTicketUsed 一个 sentinel，
//    日志里只能看到一句 `ticket not found (consumed or unknown)` ⇒ 真机"连不上"时
//    根本分不清用户是扫了旧码、码过期了、还是后端重启丢了票据（排查成本极高）。
//    拆开后前端也能给出**不同**的引导：过期/已用/不存在都指向"刷新二维码"。
var (
	// ErrTicketNotFound 从来没签发过这张码（或已过期被清 / 后端重启且未持久化）。
	ErrTicketNotFound = fmt.Errorf("peerlink: ticket not found")
	// ErrTicketExpired 码签过，但已经超过有效期。
	ErrTicketExpired = fmt.Errorf("peerlink: ticket expired")
	// ErrTicketUsed 码已被消费过（一次性：一张码只能配一次）。
	ErrTicketUsed = fmt.Errorf("peerlink: ticket already used")
	ErrBadProof   = fmt.Errorf("peerlink: bad proof")
	ErrNoSession  = fmt.Errorf("peerlink: no session")
)

// redeem 取出并**立即销毁**票据（一次性）。
//
// 三态判定顺序：过期 > 已用 > 不存在。
// ⚠️ 销毁 = 从 pending 移到 used（留痕，不含 psk），不是直接 delete ——
//    否则"用过没有"就再也问不出来了。
func (h *Hub) redeem(pairingID string) (*Ticket, error) {
	h.mu.Lock()
	t, ok := h.tickets[pairingID]
	if !ok {
		exp, used := h.used[pairingID]
		h.mu.Unlock()
		if !used {
			return nil, ErrTicketNotFound
		}
		if time.Now().After(exp) {
			return nil, ErrTicketExpired
		}
		return nil, ErrTicketUsed
	}
	delete(h.tickets, pairingID) // 一次性：取出即销毁
	h.used[pairingID] = t.ExpiresAt
	h.mu.Unlock()

	h.saveTickets()
	if time.Now().After(t.ExpiresAt) {
		return nil, ErrTicketExpired
	}
	return t, nil
}

// PairResult 配对成功的结果（回给扫码方）。
type PairResult struct {
	PeerID string `json:"peerId"`
	Token  string `json:"token"`
	SAS    string `json:"sas"`
	Peer   *Peer  `json:"peer"`
}

// Pair 校验 proof 并建立会话（Hub 侧）。
//
// 流程：① 票据存在且未过期（取出即销毁）② proof = HMAC(psk, pairingID|deviceID)
// ③ 派生双向 AEAD 密钥与 SAS ④ 生成 token 建立会话
func (h *Hub) Pair(pairingID, deviceID, name, platform, proof string) (*PairResult, error) {
	t, err := h.redeem(pairingID)
	if err != nil {
		return nil, err
	}
	psk, err := DecodePSK(t.PSKHex)
	if err != nil {
		return nil, err
	}
	if !VerifyProof(psk, pairingID, deviceID, proof) {
		return nil, ErrBadProof
	}

	token, err := newToken()
	if err != nil {
		return nil, err
	}
	a2b, b2a, err := DeriveKeys(psk)
	if err != nil {
		return nil, err
	}

	peerID := shortID(deviceID)
	now := time.Now()
	p := &Peer{
		ID:        peerID,
		DeviceID:  deviceID,
		Name:      name,
		Platform:  platform,
		PairedAt:  now,
		LastSeen:  now,
		Online:    false,
		RemoteKey: a2b, // Hub→Edge
		LocalKey:  b2a, // Edge→Hub
	}

	res := &PairResult{PeerID: peerID, Token: token, SAS: SAS(psk), Peer: p}

	h.mu.Lock()
	h.peers[peerID] = p
	h.sessions[token] = &Session{PeerID: peerID, Token: token, Created: now}
	h.pairedByTicket[pairingID] = res
	h.mu.Unlock()

	return res, nil
}

// PairingStatus 按 pairingID（二维码里的秘密）查询配对是否完成。
//
// 桌面端拿不到 peer token（token 只给扫码方），所以配对状态查询**按 pairingID 走**：
// pairingID 是 32 位 hex 秘密，唯二知情者是桌面端与扫码方 ⇒ 不需要 token 也不泄密。
// 返回 nil 表示尚未配对（或 pairingID 不存在/已过期清理）。
func (h *Hub) PairingStatus(pairingID string) map[string]any {
	h.mu.RLock()
	defer h.mu.RUnlock()
	res, ok := h.pairedByTicket[pairingID]
	if !ok {
		return nil
	}
	p := res.Peer
	return map[string]any{
		"peerId":   res.PeerID,
		"sas":      res.SAS,
		"name":     p.Name,
		"platform": p.Platform,
		"pairedAt": p.PairedAt,
	}
}

// SessionByToken 取会话（未找到返回 false）。
func (h *Hub) SessionByToken(token string) (*Session, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.sessions[token]
	return s, ok
}

// Heartbeat 更新会话与 peer 的最后在线时间；token 无效返回 false。
func (h *Hub) Heartbeat(token string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[token]
	if !ok {
		return false
	}
	now := time.Now()
	if p, ok := h.peers[s.PeerID]; ok {
		p.LastSeen = now
		p.Online = true
	}
	_ = s
	// R11：长跑进程可能长时间不出新码，借心跳这条既有通道做惰性清理
	// （只在真有东西可清时才扫，避免每次心跳都遍历）
	if len(h.tickets) > 0 || len(h.pairedByTicket) > 0 {
		h.sweepLocked(now)
	}
	return true
}

// sweepLocked 在**已持有写锁**的前提下执行清理（Heartbeat 内部用）。
func (h *Hub) sweepLocked(now time.Time) {
	for id, t := range h.tickets {
		if now.After(t.ExpiresAt) {
			delete(h.tickets, id)
		}
	}
	for id, exp := range h.used {
		if now.After(exp) {
			delete(h.used, id)
		}
	}
	for id, res := range h.pairedByTicket {
		ts := time.Time{}
		if res != nil && res.Peer != nil {
			ts = res.Peer.PairedAt
		}
		if now.Sub(ts) > PairedResultTTL {
			delete(h.pairedByTicket, id)
		}
	}
}

// MarkOffline 断开时标记离线（不删会话，允许重连）。
func (h *Hub) MarkOffline(token string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[token]
	if !ok {
		return
	}
	if p, ok := h.peers[s.PeerID]; ok {
		p.Online = false
	}
}

// PeerByID 取 peer（含密钥；仅内部使用，绝不序列化密钥字段）。
func (h *Hub) PeerByID(id string) (*Peer, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	p, ok := h.peers[id]
	return p, ok
}

// ListPeers 列出已配对设备（脱敏：不含任何密钥）。
// “这台设备已经不在了”的两个判据（2026-10-05，见 docs/persisted-state-selfhealing.md P0-3）。
//
// 由来：Hub 侧把配对过的设备**永久**列在列表里（这是对的，解配才是删除入口），
//   但"离线"只有一枚徽章，没有任何引导。于是两种真实状态无人告知：
//   ① 配对**从未真正连上过**（会合点地址不对 / token 失效）⇒ 用户以为配好了；
//   ② 设备**早已不在**（重装、换机、长期关机）⇒ 列表里躺着永远离线的"旧自己"。
//   二者都会让云控下发推错对象，属于"持久化的坏值没被识别"的同一类问题。
//
// ⚠️ 判据必须保守：手机息屏/没网几小时是**正常**的，不能误判成"设备没了"
//   ⇒ 用"从未连上 + 已过 10 分钟"与"连续离线 > 24 小时"两个阈值区分抖动与真失效。
const (
	// PeerStaleNeverLinkedAfter 配对后多久仍未连上 ⇒ 这次配对没真正成功。
	PeerStaleNeverLinkedAfter = 10 * time.Minute
	// PeerStaleOfflineAfter 连续离线多久 ⇒ 这台设备大概已经不在了。
	PeerStaleOfflineAfter = 24 * time.Hour
)

// peerStale 判定一台已配对设备是否"僵死"，并给出原因码（空串 = 正常）。
func peerStale(p *Peer, online bool, now time.Time) (bool, string) {
	if p == nil || online {
		return false, ""
	}
	// ① 配对后从未有过一次心跳（LastSeen 还停在配对那一刻）
	if p.LastSeen.Sub(p.PairedAt) < time.Second && now.Sub(p.PairedAt) > PeerStaleNeverLinkedAfter {
		return true, "never_linked"
	}
	// ② 连过，但已经很久没出现了
	if now.Sub(p.LastSeen) > PeerStaleOfflineAfter {
		return true, "offline_too_long"
	}
	return false, ""
}

// ListPeers 列出已配对设备（脱敏，不含任何密钥）。
func (h *Hub) ListPeers() []map[string]any {
	h.mu.RLock()
	defer h.mu.RUnlock()
	now := time.Now()
	out := make([]map[string]any, 0, len(h.peers))
	for _, p := range h.peers {
		online := p.Online && now.Sub(p.LastSeen) < SessionIdleTTL
		stale, reason := peerStale(p, online, now)
		out = append(out, map[string]any{
			"id":          p.ID,
			"deviceId":    p.DeviceID,
			"name":        p.Name,
			"platform":    p.Platform,
			"pairedAt":    p.PairedAt,
			"lastSeen":    p.LastSeen,
			"online":      online,
			"offlineSec":  int(now.Sub(p.LastSeen).Seconds()),
			"stale":       stale,
			"staleReason": reason,
		})
	}
	return out
}

// Unpair 解配：删除会话 + peer（token 立即作废）。
func (h *Hub) Unpair(peerID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.peers[peerID]; !ok {
		return false
	}
	delete(h.peers, peerID)
	for tok, s := range h.sessions {
		if s.PeerID == peerID {
			delete(h.sessions, tok)
		}
	}
	return true
}

// PeerIDForToken 由 token 反查 peerID（供鉴权后的处理器使用）。
func (h *Hub) PeerIDForToken(token string) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	s, ok := h.sessions[token]
	if !ok {
		return "", false
	}
	return s.PeerID, true
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("peerlink: token: %w", err)
	}
	return hex.EncodeToString(b), nil
}
