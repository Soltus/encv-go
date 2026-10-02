package peerlink

// rpc.go —— Hub → Edge 的请求/响应关联（联邦搜索与远程 Agent 的通道基础）
//
// 帧格式（JSON 文本帧）：
//
//	请求：{"type":"req","id":"<uuid>","method":"search","payload":{...}}
//	响应：{"type":"res","id":"<uuid>","result":{...}}  或 {"type":"res","id":"...","error":"..."}
//
// 关键约束：
//   - 单一连接上并发调用由 call id 关联（不能靠"一问一答"顺序假设）
//   - 必须有超时（对端离线/息屏时不能无限挂住 HTTP 请求）
//   - payload 是**密文**时由调用方先 Seal：本层只管关联与超时，不解释业务

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// DefaultCallTimeout 默认 RPC 超时（联邦搜索对端无响应就降级，不阻塞主搜索）。
const DefaultCallTimeout = 2 * time.Second

type Caller struct {
	conns *ConnRegistry

	mu      sync.Mutex
	seq     uint64
	pending map[string]chan rawResult
}

type rawResult struct {
	payload json.RawMessage
	err     string
}

func NewCaller(conns *ConnRegistry) *Caller {
	return &Caller{conns: conns, pending: make(map[string]chan rawResult)}
}

// Call 向对端发起一次 RPC，等待响应或超时。
func (c *Caller) Call(ctx context.Context, peerID, method string, payload any, timeout time.Duration) (json.RawMessage, error) {
	if timeout <= 0 {
		timeout = DefaultCallTimeout
	}
	if !c.conns.Has(peerID) {
		return nil, fmt.Errorf("%w: %s", ErrPeerOffline, peerID)
	}

	c.mu.Lock()
	c.seq++
	id := fmt.Sprintf("%d-%d", time.Now().UnixNano(), c.seq)
	ch := make(chan rawResult, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.conns.WriteJSON(peerID, map[string]any{
		"type":    "req",
		"id":      id,
		"method":  method,
		"payload": payload,
	}); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPeerOffline, err)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.err != "" {
			return nil, fmt.Errorf("peerlink: remote error: %s", res.err)
		}
		return res.payload, nil
	case <-timer.C:
		return nil, fmt.Errorf("%w: %s timeout after %s", ErrCallTimeout, method, timeout)
	}
}

// Deliver 由 WS 读循环调用：把响应交给等待中的调用方。
func (c *Caller) Deliver(id string, payload json.RawMessage, errMsg string) bool {
	c.mu.Lock()
	ch, ok := c.pending[id]
	c.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- rawResult{payload: payload, err: errMsg}:
		return true
	default:
		return false
	}
}

// SearchRequest 联邦搜索请求负载（method = "search"）。
type SearchRequest struct {
	Q     string `json:"q"`
	Kind  string `json:"kind"`  // file | fulltext | vector
	Limit int    `json:"limit"` // <=0 时由对端自行取默认
}

// ── P3.4：远端文件「在线打开 / 缩略图」读取（**不是挂载**）─────────────
//
// ⚠️ E4 决策：大流量（整文件取回）**默认禁止穿透云端**；
//    本通道只服务「在线打开（分片/Range）」与「缩略图」这类小报文。
//
// R13：单次读取有硬上限，防大报文打爆 Hub 内存；前端要整文件必须自己分片多次调用，
//      且应优先走"在线播放/预览"而不是"下载到本地"。

const (
	// DefaultReadChunk 未指定 length 时的单片大小
	DefaultReadChunk = 256 * 1024
	// MaxReadChunk 单次读取上限（R13）
	MaxReadChunk = 4 * 1024 * 1024
	// ReadCallTimeout 读超时（比搜索长：对端要真的读盘）
	ReadCallTimeout = 10 * time.Second
)

// ReadRequest 远端读请求（offset/length 语义 = HTTP Range 的子集）。
type ReadRequest struct {
	Path   string `json:"path"`
	Offset int64  `json:"offset"`
	Length int    `json:"length"`
}

// ReadResult 远端读响应（Data 经 JSON 编码为 base64）。
type ReadResult struct {
	Data   []byte `json:"data"`
	Size   int    `json:"size"`
	Offset int64  `json:"offset"`
}

// ErrPeerOffline / ErrCallTimeout —— 调用方据以降级（前端显示"该端离线/超时"，不得阻塞主结果）。
var (
	ErrPeerOffline = fmt.Errorf("peerlink: peer offline")
	ErrCallTimeout = fmt.Errorf("peerlink: call timeout")
)
