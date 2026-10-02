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

// SetExclusive 登记连接并**保证一个 peer 只有一条活跃会话**（R11）。
//
// 同一 token 二次建连（网络切换后的重连、或客户端 bug / 滥用）时，先关闭旧连接再替换，
// 避免同一 peer 在 Hub 上堆出 N 条会话（连接、goroutine、读缓冲都是资源）。
// 返回是否顶掉过旧连接。
func (r *ConnRegistry) SetExclusive(peerID string, conn *websocket.Conn) (replaced bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.conns[peerID]; ok && old.conn != nil && old.conn != conn {
		_ = old.conn.Close()
		replaced = true
	}
	r.conns[peerID] = &connEntry{conn: conn}
	return replaced
}

// DeleteConn 仅当表里登记的就是这条连接时才移除。
//
// ⚠️ 为什么不能直接用 Delete：被顶掉的旧连接退出时（defer）若按 peerID 直接删除，
// 会把**顶替它的新连接**一起删掉（R11 的会话替换才引入了这个竞态）。
func (r *ConnRegistry) DeleteConn(peerID string, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.conns[peerID]; ok && e.conn == conn {
		delete(r.conns, peerID)
	}
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
