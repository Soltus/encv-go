package server

// peerlink_api.go —— 双端互联 HTTP + WebSocket 端点（spec desktop-web-android-pairing P2a）
//
// 端点：
//
//	GET  /api/peerlink/hello        未鉴权，返回本端 peerId/name/version（探活用）
//	POST /api/peerlink/ticket       桌面端申请一次性票据（120s、一次性）→ 编码进二维码
//	POST /api/peerlink/pair         扫码方（安卓）提交 proof 完成配对 → 拿 token + SAS
//	GET  /api/peerlink/ws?token=    手机端**主动出网**建立的长连接（心跳/后续 RPC 通道）
//	POST /api/peerlink/ping         心跳（REST 兜底）
//	POST /api/peerlink/unpair       解配（token 立即作废）
//	GET  /api/peerlink/peers        已配对设备列表（脱敏，无密钥）
//
// 安全红线：
//  1. 除 hello 外，全部端点必须带有效 token（未配对一律 401），不因"在云上/有 TLS"放宽
//  2. psk / token / 密钥只存进程内存，绝不写盘
//  3. 二维码内容**不含任何内网地址**（R3：https 页面请求 http 内网会被浏览器按混合内容拦截）

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// registerPeerlinkRoutes —— 双端互联路由的**唯一注册点**。
//
// ⚠️ 必须被 `RegisterRoutes`（生产）与测试辅助（如 newPeerlinkRouter）**共用**：
//
//	曾经三次因为"只在 routes.go 加了路由、测试辅助没加"导致 e2e 假红（404）。
//	新增 peerlink 端点时只改这里，不要各处复制。
func registerPeerlinkRoutes(s *Server, r *gin.Engine) {
	r.GET("/api/peerlink/hello", s.handlePeerlinkHello)
	r.GET("/api/peerlink/peers", s.handlePeerlinkPeers)
	r.GET("/api/peerlink/pairing/status", s.handlePeerlinkPairingStatus)
	r.GET("/api/peerlink/search", s.handlePeerlinkSearch)
	r.GET("/api/peerlink/file", s.handlePeerlinkFile)
	// P4：远程 Agent（发起端调对端 / 执行端本地审批）
	r.POST("/api/peerlink/agent/invoke", s.handlePeerlinkAgentInvoke)
	r.GET("/api/peerlink/agent/pending", s.handlePeerlinkAgentPending)
	r.POST("/api/peerlink/agent/approve", s.handlePeerlinkAgentApprove)
	r.GET("/api/peerlink/agent/trust", s.handlePeerlinkAgentTrust)
	r.DELETE("/api/peerlink/agent/trust", s.handlePeerlinkAgentTrust)
	r.GET("/api/peerlink/agent/audit", s.handlePeerlinkAgentAudit)
	// P2a：本端作为 Edge 连远端 Hub（扫码配对后启动 / 状态 / 停止）
	r.POST("/api/peerlink/edge/pair", s.handlePeerlinkEdgePair)
	r.GET("/api/peerlink/edge/status", s.handlePeerlinkEdgeStatus)
	r.POST("/api/peerlink/edge/stop", s.handlePeerlinkEdgeStop)
	r.POST("/api/peerlink/ticket", s.handlePeerlinkTicket)
	r.POST("/api/peerlink/pair", s.handlePeerlinkPair)
	r.POST("/api/peerlink/ping", s.handlePeerlinkPing)
	r.POST("/api/peerlink/unpair", s.handlePeerlinkUnpair)
	r.GET("/api/peerlink/ws", s.handlePeerlinkWS)
}

// peerTokenFrom 从 Authorization: Peer <token> 或 ?token= 取令牌。
func peerTokenFrom(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Peer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Peer "))
	}
	return c.Query("token")
}

// requirePeer 鉴权：未配对一律 401（安全红线 ①）。
func (s *Server) requirePeer(c *gin.Context) (string, bool) {
	tok := peerTokenFrom(c)
	if tok == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "peer_unauthorized", "message": "缺少 peer token"})
		return "", false
	}
	if s.peerHub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "peerlink_disabled"})
		return "", false
	}
	peerID, ok := s.peerHub.PeerIDForToken(tok)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "peer_unauthorized", "message": "token 无效或已解配"})
		return "", false
	}
	return peerID, true
}

// handlePeerlinkHello —— GET /api/peerlink/hello（未鉴权）
func (s *Server) handlePeerlinkHello(c *gin.Context) {
	if s.peerHub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "peerlink_disabled"})
		return
	}
	c.JSON(http.StatusOK, s.peerHub.Info())
}

// handlePeerlinkTicket —— POST /api/peerlink/ticket
//
// 入参：{ hub?: string }（不传则用当前请求的 scheme+host 拼，天然同源）
// 出参：{ pairingId, psk, hub, expiresIn } —— 桌面端据此渲染二维码
func (s *Server) handlePeerlinkTicket(c *gin.Context) {
	if s.peerHub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "peerlink_disabled"})
		return
	}
	var body struct {
		Hub string `json:"hub"`
	}
	_ = c.ShouldBindJSON(&body)

	hubURL := strings.TrimSpace(body.Hub)
	if hubURL == "" {
		scheme := "http"
		if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		hubURL = scheme + "://" + c.Request.Host + "/api/peerlink"
	}
	tk, err := s.peerHub.CreateTicket(hubURL, peerlink.DefaultTicketTTL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ticket_failed", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"pairingId": tk.PairingID,
		"psk":       tk.PSKHex,
		"hub":       tk.Hub,
		"expiresIn": int(time.Until(tk.ExpiresAt).Seconds()),
	})
}

// handlePeerlinkPair —— POST /api/peerlink/pair
func (s *Server) handlePeerlinkPair(c *gin.Context) {
	if s.peerHub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "peerlink_disabled"})
		return
	}
	var body struct {
		PairingID string `json:"pairingId"`
		DeviceID  string `json:"deviceId"`
		Name      string `json:"name"`
		Platform  string `json:"platform"`
		Proof     string `json:"proof"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json", "detail": err.Error()})
		return
	}
	if body.PairingID == "" || body.DeviceID == "" || body.Proof == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "pairingId/deviceId/proof 必填"})
		return
	}

	res, err := s.peerHub.Pair(body.PairingID, body.DeviceID, body.Name, body.Platform, body.Proof)
	if err != nil {
		// 2026-10-04：配对失败原因必须进后端日志（DevLogs 此前一条互联日志都没有）
		slog.Warn("peerlink pair rejected", "pairingId", body.PairingID, "reason", err.Error())
		switch err {
		case peerlink.ErrTicketExpired:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ticket_expired", "message": "配对码已过期，请刷新二维码"})
		case peerlink.ErrTicketUsed:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "ticket_used", "message": "配对码已使用或不存在"})
		case peerlink.ErrBadProof:
			c.JSON(http.StatusUnauthorized, gin.H{"error": "bad_proof", "message": "配对校验失败"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "pair_failed", "detail": err.Error()})
		}
		return
	}
	slog.Info("peerlink paired", "peerId", res.PeerID, "name", res.Peer.Name, "platform", res.Peer.Platform)
	c.JSON(http.StatusOK, gin.H{
		"peerId": res.PeerID,
		"token":  res.Token,
		"sas":    res.SAS,
		"peer": gin.H{
			"id":       res.Peer.ID,
			"deviceId": res.Peer.DeviceID,
			"name":     res.Peer.Name,
			"platform": res.Peer.Platform,
		},
	})
}

// handlePeerlinkPing —— POST /api/peerlink/ping
func (s *Server) handlePeerlinkPing(c *gin.Context) {
	if _, ok := s.requirePeer(c); !ok {
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "at": time.Now().UnixMilli()})
}

// handlePeerlinkUnpair —— POST /api/peerlink/unpair
func (s *Server) handlePeerlinkUnpair(c *gin.Context) {
	var body struct {
		PeerID string `json:"peerId"`
	}
	_ = c.ShouldBindJSON(&body)
	target := strings.TrimSpace(body.PeerID)

	// ⚠️ 顺序铁律：先判运维身份，再走对端 token 鉴权。
	//    反过来写会先由 requirePeer 写出 401（gin 一旦写响应，后续身份判定都无效）
	//    ⇒ 运维解配永远 401（2026-10-02 e2e 抓出）。
	if !isOperator(c) {
		peerID, ok := s.requirePeer(c)
		if !ok {
			return
		}
		if target == "" {
			target = peerID // 对端不传则解自己
		}
	}
	if target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "peerId 必填"})
		return
	}
	if !s.peerHub.Unpair(target) {
		c.JSON(http.StatusNotFound, gin.H{"error": "peer_not_found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "peerId": target})
}

// peerBusyIfErr —— R11 背压：单 peer 在途调用已达上限。
//
// 返回 true 表示已写出 429（调用方直接 return）。
// ⚠️ 背压**不是对端故障**：必须**先于**熔断失败累计判定，且绝不计入失败次数 ——
//    否则一次限流就会把健康对端熔断掉（限流自己打自己）。
func peerBusyIfErr(c *gin.Context, peerID string, err error) bool {
	if err == nil || !errors.Is(err, peerlink.ErrPeerBusy) {
		return false
	}
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error":   "peer_busy",
		"peerId":  peerID,
		"limit":   peerlink.MaxConcurrentCallsPerPeer,
		"message": "该设备并发调用已达上限，请稍后重试",
	})
	return true
}

// isOperator 判断是否为"本机运维者"（桌面端 UI，与 Hub 同源）。
//
// ⚠️ 现状：本应用对 localhost/同源运维请求本就无鉴权（与既有 /api/* 一致），
//
//	这里仅用一个显式头区分"运维查看"与"对端调用"，避免误放开给未配对的对端。
//	**P5/R12 TODO**：运维侧应接入真正的鉴权（JWT/本地会话），不要长期依赖此头。
func isOperator(c *gin.Context) bool {
	return c.GetHeader("X-Peerlink-Operator") == "1"
}

// handlePeerlinkPairingStatus —— GET /api/peerlink/pairing/status?pairingId=<secret>
//
// 桌面端轮询"扫码方是否已完成配对"。按 pairingID（二维码里的 32 位 hex 秘密）查询，
// 不需要 token：唯二知情者是桌面端与扫码方。未配对返回 404（前端继续轮询）。
func (s *Server) handlePeerlinkPairingStatus(c *gin.Context) {
	if s.peerHub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "peerlink_disabled"})
		return
	}
	id := strings.TrimSpace(c.Query("pairingId"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "pairingId 必填"})
		return
	}
	st := s.peerHub.PairingStatus(id)
	if st == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_paired"})
		return
	}
	c.JSON(http.StatusOK, st)
}

// handlePeerlinkPeers —— GET /api/peerlink/peers（脱敏列表）
// 允许两种身份：已配对对端（token）或本机运维者（X-Peerlink-Operator）。
func (s *Server) handlePeerlinkPeers(c *gin.Context) {
	if !isOperator(c) {
		if _, ok := s.requirePeer(c); !ok {
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": s.peerHub.ListPeers()})
}

// handlePeerlinkWS —— GET /api/peerlink/ws?token=<token>
//
// 手机端**主动出网**建立长连接（Edge → Hub），是后续联邦搜索 / 远程 Agent 的通道。
// 帧协议（P2a 最小集，JSON 文本帧）：
//
//	→ {"type":"ping"}                  → {"type":"pong","at":<ms>}
//	→ {"type":"hello","platform":...}  → {"type":"hello_ok","peerId":...}
//
// 未带有效 token 一律拒绝升级（401）。
func (s *Server) handlePeerlinkWS(c *gin.Context) {
	if s.peerHub == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "peerlink_disabled"})
		return
	}
	token := peerTokenFrom(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "peer_unauthorized", "message": "缺少 peer token"})
		return
	}
	peerID, ok := s.peerHub.PeerIDForToken(token)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "peer_unauthorized", "message": "token 无效或已解配"})
		return
	}

	up := websocket.Upgrader{
		// ⚠️ 生产由 Hub 域名收敛；这里显式校验 origin，避免任意站点借用浏览器凭据建连
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // 非浏览器客户端（Go/原生）无 Origin
			}
			return true // P2a：信任已持有 token 的调用方；P5 再收紧为配对 origin 白名单
		},
	}
	conn, err := up.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// R11：一个 peer 只允许一条活跃会话（二次建连顶掉旧连接）
	s.peerConns.SetExclusive(peerID, conn)
	// ⚠️ 必须按「连接身份」删除：被顶掉的旧连接退出时若按 peerID 直接删，
	//    会把顶替它的新连接一起删掉。
	defer s.peerConns.DeleteConn(peerID, conn)

	s.peerHub.Heartbeat(token)
	defer s.peerHub.MarkOffline(token)

	// ⚠️ 写帧必须走连接表的串行化通道：Hub 侧反向发 req 帧也在写这条连接，
	//    直接 conn.WriteJSON 会与它并发（gorilla 会 panic）。
	_ = s.peerConns.WriteJSON(peerID, gin.H{"type": "hello_ok", "peerId": peerID})

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var frame struct {
			Type   string          `json:"type"`
			ID     string          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if err := json.Unmarshal(msg, &frame); err != nil {
			_ = s.peerConns.WriteJSON(peerID, gin.H{"type": "error", "error": "bad_frame"})
			continue
		}
		switch frame.Type {
		case "ping":
			s.peerHub.Heartbeat(token)
			_ = s.peerConns.WriteJSON(peerID, gin.H{"type": "pong", "at": time.Now().UnixMilli()})
		case "res":
			// RPC 响应 → 交给等待中的调用方（联邦搜索）
			s.peerCalls.Deliver(frame.ID, frame.Result, frame.Error)
		default:
			_ = s.peerConns.WriteJSON(peerID, gin.H{"type": "error", "error": "unknown_type", "type_": frame.Type})
		}
	}
}

// handlePeerlinkFile —— GET /api/peerlink/file?peerId=&path=&offset=&length=
//
// P3.4：远端命中的**在线打开 / 缩略图**通道（**不是挂载**：不产生本地路径语义，
// 每次都是显式的跨端读请求，结果以 `X-Peer-*` 标注来源）。
//
// R13：单次读取有硬上限 MaxReadChunk；超限返回 **413**（前端应分片或改用在线播放）。
// E4：整文件取回**默认禁止穿透云端**，本端点只服务小报文场景。
func (s *Server) handlePeerlinkFile(c *gin.Context) {
	limitKey := "op:" + c.ClientIP()
	if !isOperator(c) {
		id, ok := s.requirePeer(c)
		if !ok {
			return
		}
		limitKey = "peer:" + id
	}
	peerID := strings.TrimSpace(c.Query("peerId"))
	remotePath := c.Query("path")
	if peerID == "" || remotePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "peerId 与 path 必填"})
		return
	}
	p, ok := s.peerHub.PeerByID(peerID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "peer_not_found"})
		return
	}
	// R12：限流 + 熔断（对端不可达时不再每次都等满超时）
	if ok, wait := s.rateAllow("file", limitKey); !ok {
		writeRateLimited(c, wait)
		return
	}
	if !s.peerCircuitAllow(c, peerID) {
		return
	}

	var offset int64
	if v := strings.TrimSpace(c.Query("offset")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			offset = n
		}
	}
	length := peerlink.DefaultReadChunk
	if v := strings.TrimSpace(c.Query("length")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			length = n
		}
	}
	if length > peerlink.MaxReadChunk {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"error":    "chunk_too_large",
			"maxChunk": peerlink.MaxReadChunk,
			"message":  "单次读取超过上限，请分片（在线播放/缩略图场景）",
		})
		return
	}

	res, err := s.peerCalls.Call(c.Request.Context(), peerID, "read",
		peerlink.ReadRequest{Path: remotePath, Offset: offset, Length: length}, peerlink.ReadCallTimeout)
	if err != nil {
		if peerBusyIfErr(c, peerID, err) {
			return
		}
		s.peerCircuitRecord(peerID, err.Error()) // R12：失败累计 → 开路
		switch {
		case errors.Is(err, peerlink.ErrPeerOffline):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "peer_offline", "peerId": peerID})
		case errors.Is(err, peerlink.ErrCallTimeout):
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "peer_timeout", "peerId": peerID})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"error": "peer_call_failed", "detail": err.Error(), "peerId": peerID})
		}
		return
	}
	s.peerCircuitRecordSuccess(peerID)

	var rr peerlink.ReadResult
	if err := json.Unmarshal(res, &rr); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "bad_peer_payload", "detail": err.Error()})
		return
	}

	// 来源标识常驻响应头：客户端任何后续处理都能看到"这字节来自哪台设备"
	c.Header("X-Peer-Id", p.ID)
	c.Header("X-Peer-Name", p.Name)
	// ⚠️ HTTP 头按规范是 latin-1：中文路径直接塞进头会被客户端解成乱码
	//    （2026-10-02 真实浏览器验证抓出）⇒ 非 ASCII 走百分号编码。
	c.Header("X-Peer-Remote-Path", headerSafe(remotePath))
	// RFC 5987：ASCII 兜底名 + filename* 给真实名
	base := filepath.Base(remotePath)
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%q; filename*=UTF-8''%s", asciiFallback(base), url.PathEscape(base)))
	c.Data(http.StatusOK, "application/octet-stream", rr.Data)
}

// headerSafe 非 ASCII 路径改百分号编码，避免响应头乱码（latin-1 语义）。
func headerSafe(s string) string {
	if isASCII(s) {
		return s
	}
	return url.PathEscape(s)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// asciiFallback 把非 ASCII 字符替换成 '_'，作为 Content-Disposition 的兜底名。
func asciiFallback(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] < 0x80 {
			b.WriteByte(s[i])
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "remote.bin"
	}
	return b.String()
}

// handlePeerlinkSearch —— GET /api/peerlink/search?peerId=&q=&kind=fulltext&limit=50
//
// 联邦搜索：**只在对端本地索引上查询，结果带 peer 标注返回**，
// 不做挂载、不合并命名空间、不把远端路径伪装成本地路径（spec §3 非目标）。
//
// 降级语义（重要）：对端离线 → 503 peer_offline；超时 → 504 peer_timeout。
// 前端必须能把这两种状态降级为"该端无结果"，**不得阻塞本端主搜索**。
func (s *Server) handlePeerlinkSearch(c *gin.Context) {
	limitKey := "op:" + c.ClientIP()
	if !isOperator(c) {
		id, ok := s.requirePeer(c)
		if !ok {
			return
		}
		limitKey = "peer:" + id
	}
	peerID := strings.TrimSpace(c.Query("peerId"))
	q := c.Query("q")
	if peerID == "" || q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "peerId 与 q 必填"})
		return
	}
	kind := c.Query("kind")
	if kind == "" {
		kind = "fulltext"
	}
	limit := 50
	if v := strings.TrimSpace(c.Query("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	p, ok := s.peerHub.PeerByID(peerID)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "peer_not_found"})
		return
	}
	// R12：限流 + 熔断
	if ok, wait := s.rateAllow("search", limitKey); !ok {
		writeRateLimited(c, wait)
		return
	}
	if !s.peerCircuitAllow(c, peerID) {
		return
	}

	res, err := s.peerCalls.Call(c.Request.Context(), peerID, "search",
		peerlink.SearchRequest{Q: q, Kind: kind, Limit: limit}, peerlink.DefaultCallTimeout)
	if err != nil {
		if peerBusyIfErr(c, peerID, err) {
			return
		}
		s.peerCircuitRecord(peerID, err.Error()) // R12
		switch {
		case errors.Is(err, peerlink.ErrPeerOffline):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "peer_offline", "peerId": peerID})
		case errors.Is(err, peerlink.ErrCallTimeout):
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "peer_timeout", "peerId": peerID})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"error": "peer_call_failed", "detail": err.Error(), "peerId": peerID})
		}
		return
	}
	s.peerCircuitRecordSuccess(peerID)

	c.JSON(http.StatusOK, gin.H{
		"peer": gin.H{
			"id":       p.ID,
			"name":     p.Name,
			"platform": p.Platform,
		},
		"items": res,
	})
}

// ── R12 辅助：限流 / 熔断的 HTTP 表现 ──────────────────────────────

func writeRateLimited(c *gin.Context, wait time.Duration) {
	secs := int(wait / time.Second)
	if secs < 1 {
		secs = 1
	}
	c.Header("Retry-After", strconv.Itoa(secs))
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error":        "rate_limited",
		"retryAfterMs": int(wait / time.Millisecond),
		"message":      "调用过于频繁，请稍后再试",
	})
}

// peerCircuitAllow 熔断判定；开路时直接 503（不再消耗调用超时预算）。
func (s *Server) peerCircuitAllow(c *gin.Context, peerID string) bool {
	_, open, retryAfter, _ := s.peerLimits().call.state(peerID)
	if !open {
		return true
	}
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"error":        "peer_circuit_open",
		"peerId":       peerID,
		"retryAfterMs": int(retryAfter / time.Millisecond),
		"message":      "对端连续失败已熔断，冷却后自动重试",
	})
	return false
}

func (s *Server) peerCircuitRecord(peerID, errMsg string) {
	s.peerLimits().call.recordFailure(peerID, errMsg, time.Now())
}

func (s *Server) peerCircuitRecordSuccess(peerID string) {
	s.peerLimits().call.recordSuccess(peerID)
}
