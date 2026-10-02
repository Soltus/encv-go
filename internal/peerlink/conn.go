package peerlink

// conn.go —— Hub 侧活跃连接表（**仅内存**）
//
// 为什么需要：Hub 要"反过来"向已连上的手机（Edge）发查询（联邦搜索 / 远程 Agent），
// 而 Edge 是**主动出网**连进来的 ⇒ Hub 必须保存这条连接才能发请求。
// 断线即删除，进程重启即消失（与 token/密钥同一套存储纪律）。

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

type connEntry struct {
	conn    *websocket.Conn
	writeMu sync.Mutex // websocket 不支持并发写，必须串行化
}

// ConnRegistry 保存 peerID → 活跃 WS 连接。
type ConnRegistry struct {
	mu    sync.RWMutex
	conns map[string]*connEntry
}

func NewConnRegistry() *ConnRegistry {
	return &ConnRegistry{conns: make(map[string]*connEntry)}
}

func (r *ConnRegistry) Set(peerID string, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns[peerID] = &connEntry{conn: conn}
}

func (r *ConnRegistry) Get(peerID string) (*websocket.Conn, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.conns[peerID]
	if !ok {
		return nil, false
	}
	return e.conn, true
}

func (r *ConnRegistry) Delete(peerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.conns, peerID)
}

// Has 判断对端是否在线（有活跃连接）。
func (r *ConnRegistry) Has(peerID string) bool {
	_, ok := r.Get(peerID)
	return ok
}

// WriteJSON 向对端写一帧（串行化，线程安全）。
func (r *ConnRegistry) WriteJSON(peerID string, v any) error {
	r.mu.RLock()
	e, ok := r.conns[peerID]
	r.mu.RUnlock()
	if !ok {
		return ErrNoSession
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	return e.conn.WriteJSON(v)
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
