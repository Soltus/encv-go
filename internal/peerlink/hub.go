package peerlink

// hub.go —— Hub 侧（cnb 上运行）：票据 / 配对 / 会话 / 心跳 / 解配
//
// 全部状态只存内存：进程重启 → 票据、会话、密钥、信任态全部消失（符合 spec 存储纪律）。

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// DefaultTicketTTL 票据有效期（二维码 120s 过期，一次性）。
const DefaultTicketTTL = 120 * time.Second

// SessionIdleTTL 会话心跳超时（连续 3 次 15s 心跳未到即视为离线）。
const SessionIdleTTL = 45 * time.Second

type Hub struct {
	mu sync.RWMutex

	name    string // 本端名称（展示用）
	peerID  string // 本端 peer id
	version string

	tickets  map[string]*Ticket
	peers    map[string]*Peer
	sessions map[string]*Session // token → session
	// pairedByTicket：以 pairingID 为键记录"哪张票据完成了配对"，供桌面端轮询配对状态。
	// 键是二维码里的 32 位 hex 秘密（只有扫码方与桌面端知道），因此**无需 token 即可按 ID 查询**。
	pairedByTicket map[string]*PairResult
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
		pairedByTicket: make(map[string]*PairResult),
	}
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

// CreateTicket 桌面端申请一次性配对票据（二维码内容来源）。
func (h *Hub) CreateTicket(hubURL string, ttl time.Duration) (*Ticket, error) {
	if ttl <= 0 {
		ttl = DefaultTicketTTL
	}
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
	return t, nil
}

// ErrTicketExpired / ErrTicketUsed —— 票据错误（用于映射 HTTP 401/400）。
var (
	ErrTicketExpired = fmt.Errorf("peerlink: ticket expired")
	ErrTicketUsed    = fmt.Errorf("peerlink: ticket not found (consumed or unknown)")
	ErrBadProof      = fmt.Errorf("peerlink: bad proof")
	ErrNoSession     = fmt.Errorf("peerlink: no session")
)

// redeem 取出并**立即销毁**票据（一次性）。过期返回 ErrTicketExpired。
func (h *Hub) redeem(pairingID string) (*Ticket, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.tickets[pairingID]
	if !ok {
		return nil, ErrTicketUsed
	}
	delete(h.tickets, pairingID) // 一次性：取出即销毁
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
	return true
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
func (h *Hub) ListPeers() []map[string]any {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]map[string]any, 0, len(h.peers))
	for _, p := range h.peers {
		out = append(out, map[string]any{
			"id":       p.ID,
			"deviceId": p.DeviceID,
			"name":     p.Name,
			"platform": p.Platform,
			"pairedAt": p.PairedAt,
			"lastSeen": p.LastSeen,
			"online":   p.Online && time.Since(p.LastSeen) < SessionIdleTTL,
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
