package peerlink

// edge.go —— Edge 客户端（**长连接放 Go 侧**，E2 决策）
//
// 为什么放 Go 侧而不是 WebView：安卓端 Activity/息屏/切后台都会影响 WebView 生命周期，
// 而 Go 进程由 EncvGoService（已是前台服务）承载，链路不受 UI 影响（R7/R9）。
//
// 拓扑：Edge（手机，NAT 后）**主动出网**连 Hub（cnb 公网）。
//   - 不需要 Hub 能连回手机（手机没有公网地址，R1）
//   - 断线指数退避 + 抖动重连（换 Wi-Fi / 息屏唤醒 / 服务重启场景）
//   - 心跳间隔自适应：前台 15s / 后台 60s（省电省流量，R8）
//
// ⚠️ token 由调用方从内存传入；本包**不写盘**（存储纪律）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// edgeLog —— 2026-10-04：整个 peerlink 子系统此前**零条日志**（全包 grep 'slog.' 0 命中），
//   用户在 DevLogs 里排查"配对成功但连不上"时一条相关信息都没有（真机反馈：DevLogs
//   没有可用日志信息）。连接/断线/token 失效必须写后端日志，才会被 WS 广播到 DevLogs。
//   ⚠️ 只记网络层原因与主机，**绝不记 token / psk**。
func edgeLog(level slog.Level, msg string, args ...any) {
	slog.Log(context.Background(), level, msg, args...)
}

// hubHost 只取会合点主机（wsURL 里带 token，绝不进日志）。
func (e *Edge) hubHost() string {
	u, err := url.Parse(e.wsURL())
	if err != nil {
		return "(unparsable)"
	}
	return u.Host
}

// 默认心跳间隔（前台 / 后台）与退避参数。
const (
	DefaultHeartbeatForeground = 15 * time.Second
	DefaultHeartbeatBackground = 60 * time.Second
	DefaultMinBackoff          = 1 * time.Second
	DefaultMaxBackoff          = 60 * time.Second
)

// EdgeOptions 创建 Edge 的参数（时长字段可注入，便于单测）。
type EdgeOptions struct {
	HubURL   string // 例如 https://host/api/peerlink
	Token    string // 配对得到的 token（仅内存）
	DeviceID string
	Name     string
	Platform string

	HeartbeatForeground time.Duration
	HeartbeatBackground time.Duration
	MinBackoff          time.Duration
	MaxBackoff          time.Duration

	// Dialer 可选注入（测试可替换/加超时）
	Dialer *websocket.Dialer
	// OnFrame 收到服务端帧时的回调（type + 原始负载）
	OnFrame func(typ string, payload []byte)
	// OnAgentInvoke 处理来自对端的**远程 Agent 调用**（P4）。
	// ⚠️ 审批发生在**执行端**（宿主注入的实现负责挂起 + 等本端 UI 决策）；
	//    返回 ErrDeclined / ErrCancelled 会被翻译成 decline / cancel 结果。
	//    Outcome.Decision 必须透传授权器的真实决策（auto / accept / trust_device）。
	OnAgentInvoke func(req AgentInvokeRequest) AgentInvokeOutcome
	// OnRead 处理来自对端的**远端读**请求（P3.4：在线打开 / 缩略图）。
	// 由宿主注入真实实现；未注入返回 not_supported。
	// ⚠️ 只允许在**本端**读，不得把远端路径解析成本端可写路径。
	//
	// 返回 (数据, 源文件总字节数, error)：
	//   2026-10-04 起要求一并返回文件总长度，让发起端能表达 HTTP Range 语义
	//   （206 + Content-Range），而不是把分片读的结果伪装成完整响应（200）。
	OnRead func(req ReadRequest) (data []byte, total int64, err error)
	// OnSearch 处理来自对端的**联邦搜索**请求（P3）。
	// 由宿主（手机上的 encv-go）注入真实搜索实现；未注入则返回 not_supported。
	// ⚠️ 搜索只在本地索引上跑，**不把远端路径伪装成本地路径**。
	OnSearch func(req SearchRequest) (json.RawMessage, error)
	// OnBundleUpdate 处理来自对端的**云控热更新**指令（2026-10-04）。
	// Hub 只给"包名字/版本/摘要"，zip 由本端主动出网 HTTP 拉取（数据面不走 WS）。
	// ⚠️ 执行端必须校验 sha256；失败要能回滚到上一版，绝不留下半新半旧的资源目录。
	OnBundleUpdate func(req BundleUpdateRequest) BundleUpdateResult
	// OnBundleRollback 处理来自对端的**云控回滚**指令（2026-10-05）。
	// ⚠️ 没接线时**必须**回 not_supported（而不是"成功"）—— 否则云控侧会以为回滚成功。
	OnBundleRollback func(req BundleRollbackRequest) BundleRollbackResult
	// OnBundleReload 处理来自对端的**云控重载/重启**指令（2026-10-06）。
	// 三级：web（WebView 重载）/ activity（recreate）/ app（整个进程重启）。
	// ⚠️ 没接线时**必须**回 not_supported，不能假装成功。
	OnBundleReload func(req BundleReloadRequest) BundleReloadResult
	// OnPeerCapabilities 回答"你是谁、你能做啥"（vNext Round 12）。
	// 云控据此决策，而不是靠硬编码常量猜。
	OnPeerCapabilities func(req PeerCapabilitiesRequest) PeerCapabilitiesResult
}

// Token 返回本 Edge 持有的配对 token（云控下载时要用它做鉴权）。
//
// ⚠️ 调用方不得打印/落盘/回传给对端：token 等价身份凭证。
func (e *Edge) Token() string { return e.opts.Token }

// HubURL 返回本 Edge 连的会合点基址（数据面下载要基于它拼 URL）。
func (e *Edge) HubURL() string { return e.opts.HubURL }

// Supports 报告本 Edge 是否**接线**了某个 RPC 方法的处理器。
//
// 为什么要有它：对端回 "not_supported" 时，无法区分到底是
//  ① 对端版本旧（没这个方法）还是 ② 本端没把 handler 传进 EdgeOptions。
//  2026-10-05 真机首测就是 ②（bundle_update 只在 handlers 映射里加了，忘了传给 Edge），
//  本方法让它变成可断言的事实，而不是靠猜。
func (e *Edge) Supports(method string) bool {
	switch method {
	case "read":
		return e.opts.OnRead != nil
	case "search":
		return e.opts.OnSearch != nil
	case "agent_invoke":
		return e.opts.OnAgentInvoke != nil
	case MethodBundleUpdate:
		return e.opts.OnBundleUpdate != nil
	case MethodBundleRollback:
		return e.opts.OnBundleRollback != nil
	case MethodBundleReload:
		return e.opts.OnBundleReload != nil
	case MethodPeerCapabilities:
		return e.opts.OnPeerCapabilities != nil
	default:
		return false
	}
}

// Edge 是手机侧的长连接客户端。
type Edge struct {
	opts EdgeOptions

	mu       sync.Mutex
	conn     *websocket.Conn
	closed   bool
	bg       bool
	attempts int
	// lastErr：最近一次连接失败的原因（2026-10-04）。
	//   原先 Start() 把 runSession 的错误**吞掉**（只用来算退避），于是手机端
	//   /edge/status 只能报 running=true/connected=false ⇒ UI 永远显示"连接中…"，
	//   用户（和我们自己）都看不出到底是地址不通、还是代理不支持 WebSocket 升级。
	//   ⚠️ 只存**网络层**错误文本，绝不含 token / psk 等密钥。
	lastErr string
	// writeMu 串行化**所有**写帧（心跳 ping / SendJSON / RPC 响应）。
	// websocket 不支持并发写：并发回写会 panic "concurrent write to websocket connection"。
	writeMu sync.Mutex
	// hbSwitch 用于心跳间隔切换（前台↔后台）时唤醒心跳协程
	hbSwitch chan struct{}
}

// NewEdge 创建 Edge（未连接）。
func NewEdge(opts EdgeOptions) *Edge {
	if opts.HeartbeatForeground <= 0 {
		opts.HeartbeatForeground = DefaultHeartbeatForeground
	}
	if opts.HeartbeatBackground <= 0 {
		opts.HeartbeatBackground = DefaultHeartbeatBackground
	}
	if opts.MinBackoff <= 0 {
		opts.MinBackoff = DefaultMinBackoff
	}
	if opts.MaxBackoff <= 0 {
		opts.MaxBackoff = DefaultMaxBackoff
	}
	if opts.Dialer == nil {
		opts.Dialer = websocket.DefaultDialer
	}
	return &Edge{opts: opts, hbSwitch: make(chan struct{}, 1)}
}

// wsURL 把 http(s)://host/api/peerlink 转成 ws(s)://host/api/peerlink/ws?token=xxx
func (e *Edge) wsURL() string {
	u := strings.TrimSuffix(e.opts.HubURL, "/")
	switch {
	case strings.HasPrefix(u, "https://"):
		u = "wss://" + strings.TrimPrefix(u, "https://")
	case strings.HasPrefix(u, "http://"):
		u = "ws://" + strings.TrimPrefix(u, "http://")
	}
	return u + "/ws?token=" + e.opts.Token
}

// SetBackground 切换前台/后台心跳间隔（省电，R8）。
func (e *Edge) SetBackground(bg bool) {
	e.mu.Lock()
	changed := e.bg != bg
	e.bg = bg
	e.mu.Unlock()
	if changed {
		select {
		case e.hbSwitch <- struct{}{}:
		default:
		}
	}
}

// LastError 最近一次连接失败的原因（无失败则空串）。
// 供 /edge/status 暴露给 UI —— 失败必须可见，禁止静默"连接中…"。
func (e *Edge) LastError() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastErr
}

// Attempts 已重试次数（用于 UI 提示"已重试 N 次"）。
func (e *Edge) Attempts() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.attempts
}

func (e *Edge) setLastErr(err error) {
	e.mu.Lock()
	e.lastErr = err.Error()
	e.mu.Unlock()
}

// Online 是否已连上。
func (e *Edge) Online() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.conn != nil
}

// Close 关闭连接并停止重连。
func (e *Edge) Close() {
	e.mu.Lock()
	e.closed = true
	conn := e.conn
	e.conn = nil
	e.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// Start 阻塞式运行：连接 → 读帧 + 心跳 → 断线退避重连，直到 ctx 结束或 Close()。
func (e *Edge) Start(ctx context.Context) {
	for {
		e.mu.Lock()
		if e.closed {
			e.mu.Unlock()
			return
		}
		e.mu.Unlock()

		if err := e.runSession(ctx); err != nil {
			// ⚠️ 记录失败原因（2026-10-04）：不记录的话 UI 只能显示"连接中…"，
			//    用户无法区分"正在连"和"根本连不上（如代理不支持 WS 升级）"。
			e.setLastErr(err)

			// 写后端日志（会被 WS 广播到 DevLogs）。
			// 节流：退避重连会反复失败，只在**首次、每 5 次、以及 token 被拒**时记，
			// 否则一条日志/秒地把 DevLogs 冲垮。
			e.mu.Lock()
			attempts := e.attempts
			e.mu.Unlock()
			unauthorized := strings.Contains(err.Error(), "unauthorized")
			if unauthorized {
				edgeLog(slog.LevelWarn, "peerlink edge rejected by hub: token invalid, re-pair required",
					"hub", e.hubHost(), "attempts", attempts, "err", err.Error())
			} else if attempts <= 1 || attempts%5 == 0 {
				edgeLog(slog.LevelWarn, "peerlink edge connect failed",
					"hub", e.hubHost(), "attempts", attempts, "err", err.Error())
			}
			// 真断线（非 ctx/Close 导致的退出）才计数退避重连
			if err == errDisconnected {
				e.mu.Lock()
				e.attempts++
				e.mu.Unlock()
			}
			// 退避重连（指数 + 抖动），上限 MaxBackoff
			backoff := e.nextBackoffLocked()
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			continue
		}
		// runSession 正常返回 = 上层 ctx 结束或被 Close
		return
	}
}

// runSession 建立一次连接并阻塞到连接断开；ctx 结束/Close 返回 nil。
func (e *Edge) runSession(ctx context.Context) error {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	conn, resp, err := e.opts.Dialer.DialContext(dialCtx, e.wsURL(), http.Header{})
	cancel()
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			// token 失效（已解配）→ 不重试语义交给调用方：这里仍退避但记录状态
			e.mu.Lock()
			e.attempts++
			e.mu.Unlock()
			return fmt.Errorf("peerlink: dial unauthorized: %w", err)
		}
		e.mu.Lock()
		e.attempts++
		e.mu.Unlock()
		return err
	}

	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		_ = conn.Close()
		return nil
	}
	e.conn = conn
	e.lastErr = ""
	e.attempts = 0
	e.mu.Unlock()

	edgeLog(slog.LevelInfo, "peerlink edge connected", "hub", e.hubHost())

	done := make(chan struct{})
	go e.readLoop(ctx, conn, done)
	go e.heartbeatLoop(ctx, conn, done)

	<-done

	// 区分「优雅停止」与「真断线」：
	//   优雅（ctx 取消 / 已 Close）→ nil，Start 直接返回
	//   真断线（服务端断开 / 网络中断）→ errDisconnected，Start 走退避重连
	// ⚠️ 这里曾返回 nil，导致断线后 Start 直接退出、永不重连（单测 TestEdge_Reconnect_AfterDrop 抓出）。
	e.mu.Lock()
	closed := e.closed
	e.mu.Unlock()
	if closed || ctx.Err() != nil {
		return nil
	}
	edgeLog(slog.LevelWarn, "peerlink edge disconnected, will backoff-retry", "hub", e.hubHost())
	return errDisconnected
}

// errDisconnected 表示连接被对端/网络中断（需要重连），不是调用方主动结束。
var errDisconnected = fmt.Errorf("peerlink: disconnected")

func (e *Edge) readLoop(ctx context.Context, conn *websocket.Conn, done chan struct{}) {
	defer func() {
		e.mu.Lock()
		if e.conn == conn {
			e.conn = nil
		}
		e.mu.Unlock()
		_ = conn.Close()
		close(done)
	}()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var f struct {
			Type    string          `json:"type"`
			ID      string          `json:"id"`
			Method  string          `json:"method"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(msg, &f); err != nil {
			continue
		}
		if e.opts.OnFrame != nil {
			e.opts.OnFrame(f.Type, msg)
		}
		// P3：处理来自对端的 RPC 请求（目前只有 search）
		if f.Type == "req" {
			e.handleRequest(conn, f.ID, f.Method, f.Payload)
		}
	}
}

// handleRequest 执行对端请求并回写响应（res 帧）。
func (e *Edge) handleRequest(conn *websocket.Conn, id, method string, payload json.RawMessage) {
	// ⚠️ websocket **不支持并发写**：Hub 侧并发调用（R11 上限内仍可 4 路并发）
	// 会让多个请求处理同时回写 res 帧 ⇒ gorilla 直接 panic
	// ("concurrent write to websocket connection")。所有写帧必须串行化。
	write := func(result any, errMsg string) {
		e.writeMu.Lock()
		defer e.writeMu.Unlock()
		_ = conn.WriteJSON(map[string]any{"type": "res", "id": id, "result": result, "error": errMsg})
	}
	// writeRejected 标记"对端健康、但这个请求本身不被接受"（路径非法 / 文件不存在等）。
	//
	// 2026-10-04：此前这类错误和普通失败一样裸着过 wire ⇒ Hub 侧一律计入熔断
	// ⇒ 几次误传参数就把 peer 熔断掉，连坐所有合法调用。带上 errKind 让发起端能区分。
	// ⚠️ errMsg 必须是**脱敏**后的原因（不得含真实路径 / 参数全文）。
	writeRejected := func(result any, errMsg string) {
		e.writeMu.Lock()
		defer e.writeMu.Unlock()
		_ = conn.WriteJSON(map[string]any{
			"type": "res", "id": id, "result": result, "error": errMsg, "errKind": ErrKindRejected,
		})
	}

	switch method {
	case "read":
		if e.opts.OnRead == nil {
			write(nil, "not_supported")
			return
		}
		var req ReadRequest
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &req); err != nil {
				write(nil, "bad_payload")
				return
			}
		}
		// R13：单片上限在这里再兜一次底（对端不可信，Hub 侧也会校验）
		if req.Length <= 0 {
			req.Length = DefaultReadChunk
		}
		if req.Length > MaxReadChunk {
			req.Length = MaxReadChunk
		}
		data, total, err := e.opts.OnRead(req)
		if err != nil {
			// 业务性拒绝（路径非法 / 打不开 / 越权）⇒ 标 rejected，发起端不当成故障，
			// 也不计入熔断。其余错误（IO 真挂了等）仍按普通失败处理。
			if errors.Is(err, ErrPeerRejected) {
				writeRejected(nil, unwrapRejectReason(err))
				return
			}
			write(nil, err.Error())
			return
		}
		write(ReadResult{Data: data, Size: len(data), Offset: req.Offset, Total: total}, "")
	case "agent_invoke":
		if e.opts.OnAgentInvoke == nil {
			write(nil, "not_supported")
			return
		}
		var req AgentInvokeRequest
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &req); err != nil {
				write(nil, "bad_payload")
				return
			}
		}
		out := e.opts.OnAgentInvoke(req)
		res := AgentInvokeResult{Ok: out.Err == nil}
		switch {
		case out.Err == nil:
			// ⚠️ 不能硬编码 accept：要看**执行端授权器**的真实决策
			//    （auto=已信任免确认 / accept=逐次同意 / trust_device=本次同意并记住）
			res.Decision = out.Decision
			if res.Decision == "" {
				res.Decision = DecisionAccept
			}
			res.Result = out.Result
		case errors.Is(out.Err, ErrDeclined):
			res.Decision = DecisionDecline
			res.Error = "user_declined"
		case errors.Is(out.Err, ErrCancelled):
			res.Decision = DecisionCancel
			res.Error = "cancelled"
		default:
			res.Decision = DecisionDecline
			// ⚠️ 脱敏：出错原因可能含路径/参数，只回传错误类型文本的前 200 字符
			res.Error = truncateErr(out.Err.Error(), 200)
		}
		write(res, "")
	case "search":
		if e.opts.OnSearch == nil {
			write(nil, "not_supported")
			return
		}
		var req SearchRequest
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &req); err != nil {
				write(nil, "bad_payload")
				return
			}
		}
		out, err := e.opts.OnSearch(req)
		if err != nil {
			// 与 read 同理：查询写得不对是对端"健康地拒绝"，不是故障（不计入熔断）。
			if errors.Is(err, ErrPeerRejected) {
				writeRejected(nil, unwrapRejectReason(err))
				return
			}
			write(nil, err.Error())
			return
		}
		write(out, "")
	// 云控热更新（2026-10-04）：Hub 只下发指令，zip 由本端 HTTP 拉取。
	//
	//	失败语义分两类，与 read/search 同一套纪律：
	//	  ① 包名字不认识 / 摘要缺失 ⇒ **对端请求本身不对** ⇒ rejected（发起端 400，不计熔断）
	//	  ② 下载失败 / 解压失败 / 切换失败 ⇒ 本端故障 ⇒ 普通错误（但已回滚，旧版仍在服务）
	case "bundle_update":
		if e.opts.OnBundleUpdate == nil {
			write(nil, "not_supported")
			return
		}
		var req BundleUpdateRequest
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &req); err != nil {
				write(nil, "bad_payload")
				return
			}
		}
		out := e.opts.OnBundleUpdate(req)
		if out.Rejected {
			writeRejected(out, out.Error)
			return
		}
		write(out, "")
	// 云控回滚（2026-10-05）：把"上一版备份"搬回来。
	//
	//	失败语义与 bundle_update 完全一致：
	//	  ① 包名不认识 / **没有备份可退** ⇒ rejected（发起端 400，不计熔断）
	//	  ② 搬移失败 ⇒ 普通错误（本端故障）
	case MethodBundleRollback:
		if e.opts.OnBundleRollback == nil {
			write(nil, "not_supported")
			return
		}
		var req BundleRollbackRequest
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &req); err != nil {
				write(nil, "bad_payload")
				return
			}
		}
		out := e.opts.OnBundleRollback(req)
		if out.Rejected {
			writeRejected(out, out.Error)
			return
		}
		write(out, "")
	// 云控重载/重启（2026-10-06）：不再要求用户手动重启 App。
	//
	//	web      —— 本端 Go 直接广播 WS 事件 ⇒ 前端 reload，**当场生效**
	//	activity / app —— Go 只能落指令（写重载请求），由 Kotlin 侧接管执行
	case MethodBundleReload:
		if e.opts.OnBundleReload == nil {
			write(nil, "not_supported")
			return
		}
		var req BundleReloadRequest
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &req); err != nil {
				write(nil, "bad_payload")
				return
			}
		}
		out := e.opts.OnBundleReload(req)
		if out.Rejected {
			writeRejected(out, out.Error)
			return
		}
		write(out, "")
	// 能力自省（vNext Round 12）：让 Hub 问"你能做啥"，而不是靠约定猜
	case MethodPeerCapabilities:
		if e.opts.OnPeerCapabilities == nil {
			write(nil, "not_supported")
			return
		}
		var req PeerCapabilitiesRequest
		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &req); err != nil {
				write(nil, "bad_payload")
				return
			}
		}
		out := e.opts.OnPeerCapabilities(req)
		if !out.Ok {
			write(nil, out.Error)
			return
		}
		write(out, "")
	default:
		write(nil, "unknown_method:"+method)
	}
}

func (e *Edge) heartbeatLoop(ctx context.Context, conn *websocket.Conn, done chan struct{}) {
	for {
		interval := e.currentInterval()

		// 断线即退出（done 关闭）
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		default:
		}

		if err := e.writePing(conn); err != nil {
			return
		}

		ticker := time.NewTicker(interval)
		select {
		case <-done:
			ticker.Stop()
			return
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-e.hbSwitch:
			ticker.Stop()
			continue // 用新间隔重新开始
		case <-ticker.C:
			ticker.Stop()
		}
	}
}

func (e *Edge) currentInterval() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.bg {
		return e.opts.HeartbeatBackground
	}
	return e.opts.HeartbeatForeground
}

func (e *Edge) writePing(conn *websocket.Conn) error {
	e.mu.Lock()
	c := e.conn
	e.mu.Unlock()
	if c != conn {
		return fmt.Errorf("peerlink: stale connection")
	}
	payload, _ := json.Marshal(map[string]any{"type": "ping", "at": time.Now().UnixMilli()})
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	return conn.WriteMessage(websocket.TextMessage, payload)
}

// SendJSON 向 Hub 发送一帧（线程安全；未连接返回错误）。
func (e *Edge) SendJSON(v any) error {
	e.mu.Lock()
	conn := e.conn
	e.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("peerlink: not connected")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	return conn.WriteMessage(websocket.TextMessage, b)
}

// nextBackoffLocked 指数退避 + 抖动（attempts 已在失败处递增）。
func (e *Edge) nextBackoffLocked() time.Duration {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := e.attempts
	if n < 0 {
		n = 0
	}
	if n > 10 {
		n = 10
	}
	d := e.opts.MinBackoff * time.Duration(1<<uint(n))
	if d > e.opts.MaxBackoff || d <= 0 {
		d = e.opts.MaxBackoff
	}
	// ±20% 抖动，避免多端同时重连打爆 Hub（R12 相关）
	jitter := time.Duration(rand.Int63n(int64(d/5)+1)) - d/10
	if jitter < 0 {
		jitter = -jitter
	}
	return d/2 + jitter
}
