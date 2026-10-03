package server

// peerlink_edge_runtime.go —— P2a Task 2.2：**把 Edge 挂进 Go 进程**
//
// 角色：本端（手机/另一台设备）扫 Hub 的二维码后，作为 Edge **主动出网**连上去并常驻。
// 长连接放 Go 侧（E2 决策）：息屏 / 后台 / Activity 重建都不影响链路（R7/R9），
// Go 进程由 EncvGoService（已是前台服务）承载。
//
// ⚠️ 存储纪律：token / psk **只存进程内存**，不落盘（Task 5.2）。
//    ⇒ 进程重启后必须重新扫码配对（安全优先，与"信任态重启失效"同一套语义）。

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/fts"
	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// ── Edge 运行时状态 ─────────────────────────────────────────────────

type edgeRuntime struct {
	hubURL    string
	peerID    string
	deviceID  string
	startedAt time.Time
	cancel    context.CancelFunc
	edge      *peerlink.Edge
	lastErr   string
}

// peerEdgeHandlers 允许测试/宿主覆盖 Edge 的三个处理器（默认走本端真实实现）。
type peerEdgeHandlers struct {
	OnSearch      func(peerlink.SearchRequest) (json.RawMessage, error)
	OnRead        func(peerlink.ReadRequest) ([]byte, error)
	OnAgentInvoke func(peerlink.AgentInvokeRequest) peerlink.AgentInvokeOutcome
}

func (s *Server) edgeHandlers() peerEdgeHandlers {
	if s.peerEdgeHandlersOverride != nil {
		return *s.peerEdgeHandlersOverride
	}
	return peerEdgeHandlers{
		OnSearch:      s.peerLocalSearch,
		OnRead:        s.peerLocalRead,
		OnAgentInvoke: s.PeerAgentInvokeHandler,
	}
}

// ── 本端真实实现（被对端查询时执行）────────────────────────────────

// peerLocalSearch 对端查询**本端**索引（复用 FTS5 全文索引，与 /api/files/search-fulltext 同源）。
func (s *Server) peerLocalSearch(req peerlink.SearchRequest) (json.RawMessage, error) {
	idx := GetFullTextIndex()
	if idx == nil {
		return nil, errors.New("fulltext_unavailable")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200 // R13：控制面小报文
	}
	res, err := idx.Search(context.Background(), req.Q, fts.SearchOptions{Limit: limit, IncludeDirs: false})
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(res))
	for _, r := range res {
		items = append(items, map[string]any{
			"path":  r.Path,
			"name":  r.Name,
			"score": r.Score,
		})
	}
	return json.Marshal(items)
}

// peerLocalRead 对端读**本端**文件分片（在线打开 / 缩略图，R13 有上限）。
func (s *Server) peerLocalRead(req peerlink.ReadRequest) ([]byte, error) {
	abs, err := s.resolveUserPath(req.Path)
	if err != nil {
		return nil, errors.New("bad_path")
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, errors.New("open_failed")
	}
	defer f.Close()

	n := req.Length
	if n <= 0 {
		n = peerlink.DefaultReadChunk
	}
	if n > peerlink.MaxReadChunk {
		n = peerlink.MaxReadChunk
	}
	buf := make([]byte, n)
	if req.Offset > 0 {
		if _, err := f.Seek(req.Offset, io.SeekStart); err != nil {
			return nil, errors.New("seek_failed")
		}
	}
	read, err := io.ReadFull(f, buf)
	if err != nil && (err != io.ErrUnexpectedEOF && err != io.EOF) {
		return nil, errors.New("read_failed")
	}
	// ⚠️ 错误文本不含真实路径（R14 脱敏）
	return buf[:read], nil
}

// ── 远端配对（扫码后调用）──────────────────────────────────────────

type edgePairBody struct {
	Hub       string `json:"hub"`       // Hub 基址（二维码里带，必须是 https，R3）
	PairingID string `json:"pairingId"` // 二维码里的秘密
	PSK       string `json:"psk"`       // 二维码里的 psk（hex，与票据 PSKHex 同编码）
	DeviceID  string `json:"deviceId"`
	Name      string `json:"name"`
}

// validateHubURL R3：跨端地址**禁止明文 http**（混合内容 + 明文泄露）。
// 仅允许 https，或本机回环（开发/自测）。
func validateHubURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", errors.New("invalid_hub")
	}
	if u.Scheme == "https" {
		return strings.TrimRight(u.String(), "/"), nil
	}
	host := u.Hostname()
	if u.Scheme == "http" && (host == "127.0.0.1" || host == "localhost" || host == "::1") {
		return strings.TrimRight(u.String(), "/"), nil
	}
	return "", errors.New("hub_must_be_https")
}

// hubBaseURL 归一出 Hub 的**基址**（scheme://host[:port]）。
//
// ⚠️ 真 bug（2026-10-03 抓出）：票据里的 `hub` 是**带路径**的
// （`https://host/api/peerlink`，见 handlePeerlinkTicket），而 pairToRemoteHub 又会
// 拼上 `/api/peerlink/pair` ⇒ 双前缀 `.../api/peerlink/api/peerlink/pair` ⇒ **404**
// （扫码配对在真机上直接失败：`pair_rejected:404`）。
// 这里统一把已带的 peerlink 前缀剥掉，两种写法都能用。
func hubBaseURL(raw string) string {
	h := strings.TrimRight(strings.TrimSpace(raw), "/")
	h = strings.TrimSuffix(h, "/api/peerlink")
	return strings.TrimRight(h, "/")
}

// hubHostOf 只取会合点主机用于日志（票据/地址里不落 token 等密钥到日志）。
func hubHostOf(hub string) string {
	u, err := url.Parse(hub)
	if err != nil {
		return "(unparsable)"
	}
	return u.Host
}

// pairToRemoteHub 拿票据去远端 Hub 完成配对（proof = HMAC(psk, pairingId, deviceId)）。
//
// ⚠️ 第三个返回值 sas 不能丢（2026-10-04）：远端 /api/peerlink/pair 会回 SAS 6 位安全码，
//    扫码端（手机）要**显示**它给人工核对 —— 桌面端在等"一致，信任该设备"，
//    手机端却什么都不显示 ⇒ 用户根本不知道该拿什么去比对（真机反馈：困惑）。
func pairToRemoteHub(hub, pairingID, pskB64, deviceID, name string) (peerID string, token string, sas string, err error) {
	psk, err := peerlink.DecodePSK(pskB64)
	if err != nil {
		return "", "", "", fmt.Errorf("bad_psk: %w", err)
	}
	body, _ := json.Marshal(map[string]string{
		"pairingId": pairingID,
		"deviceId":  deviceID,
		"name":      name,
		"platform":  runtime.GOOS,
		"proof":     peerlink.Proof(psk, pairingID, deviceID),
	})
	resp, err := http.Post(hubBaseURL(hub)+"/api/peerlink/pair", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", "", "", fmt.Errorf("pair_request_failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("pair_rejected:%d", resp.StatusCode)
	}
	var out struct {
		PeerID string `json:"peerId"`
		Token  string `json:"token"`
		SAS    string `json:"sas"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Token == "" {
		return "", "", "", errors.New("bad_pair_response")
	}
	return out.PeerID, out.Token, out.SAS, nil
}

// ── HTTP API（本端 UI / 扫码后用）──────────────────────────────────

// handlePeerlinkEdgePair —— POST /api/peerlink/edge/pair
func (s *Server) handlePeerlinkEdgePair(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var body edgePairBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json", "detail": err.Error()})
		return
	}
	hub, err := validateHubURL(body.Hub)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_hub", "message": "Hub 地址必须是 https（本机回环除外）"})
		return
	}
	if strings.TrimSpace(body.PairingID) == "" || strings.TrimSpace(body.PSK) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "pairingId 与 psk 必填"})
		return
	}
	deviceID := strings.TrimSpace(body.DeviceID)
	if deviceID == "" {
		deviceID = s.peerHub.Info()["peerId"]
	}
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = s.peerHub.Info()["name"]
	}

	peerID, token, sas, err := pairToRemoteHub(hub, body.PairingID, body.PSK, deviceID, name)
	if err != nil {
		// 2026-10-04：扫码端配对失败必须进后端日志（此前整个 peerlink 零日志，
		//   用户只能看到"连接中…"，DevLogs 里一条相关信息都没有）
		slog.Warn("peerlink edge pair failed", "hub", hubHostOf(hub), "detail", err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": "pair_failed", "detail": err.Error()})
		return
	}

	slog.Info("peerlink edge paired, connecting", "hub", hubHostOf(hub), "peerId", peerID)
	s.startEdgeLocked(hub, peerID, deviceID, token)

	// sas 回给扫码端 UI（PeerScanPanel）显示，供双端人工核对；
	// 不落盘、不进日志（安全码只在配对会话内有效）。
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"hub":     hub,
		"peerId":  peerID,
		"sas":     sas,
		"running": true,
	})
}

// startEdgeLocked 启动（或重启）Edge 长连接。token 只存内存。
func (s *Server) startEdgeLocked(hub, peerID, deviceID, token string) {
	s.peerEdgeMu.Lock()
	defer s.peerEdgeMu.Unlock()

	// 重复配对：先停掉旧的，避免双连接
	if s.peerEdge != nil && s.peerEdge.cancel != nil {
		s.peerEdge.cancel()
	}

	h := s.edgeHandlers()
	ctx, cancel := context.WithCancel(context.Background())
	// ⚠️ 必须先用 hubBaseURL 归一化（2026-10-04 真因修复）：
	//   票据里的 hub **已经自带** `/api/peerlink` 后缀（如 http://host/api/peerlink），
	//   这里再拼一次 ⇒ Edge 去连 `/api/peerlink/api/peerlink/ws`（双前缀）
	//   ⇒ 404 / bad handshake ⇒ 手机端永远"连接中…"、桌面端显示离线
	//   ⇒ 表现就是"信任设备后还是连接失败"。
	hubNormalized := hubBaseURL(hub) + "/api/peerlink"
	edge := peerlink.NewEdge(peerlink.EdgeOptions{
		HubURL:        hubNormalized,
		Token:         token,
		DeviceID:      deviceID,
		OnSearch:      h.OnSearch,
		OnRead:        h.OnRead,
		OnAgentInvoke: h.OnAgentInvoke,
	})
	rt := &edgeRuntime{
		hubURL:    hubNormalized,
		peerID:    peerID,
		deviceID:  deviceID,
		startedAt: time.Now(),
		cancel:    cancel,
		edge:      edge,
	}
	s.peerEdge = rt
	go edge.Start(ctx)
}

// handlePeerlinkEdgeStatus —— GET /api/peerlink/edge/status
func (s *Server) handlePeerlinkEdgeStatus(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	s.peerEdgeMu.Lock()
	rt := s.peerEdge
	s.peerEdgeMu.Unlock()
	if rt == nil {
		c.JSON(http.StatusOK, gin.H{"running": false})
		return
	}
	connected := rt.edge != nil && rt.edge.Online()
	// ⚠️ 失败原因必须回给 UI（2026-10-04）：running=true 但 connected=false 时，
	//    没有 lastErr 前端只能显示"连接中…"，永远看不出是地址不通还是代理不支持 WS 升级。
	lastErr := ""
	attempts := 0
	if rt.edge != nil {
		lastErr = rt.edge.LastError()
		attempts = rt.edge.Attempts()
	}
	c.JSON(http.StatusOK, gin.H{
		"running":   true,
		"hub":       rt.hubURL,
		"peerId":    rt.peerID,
		"connected": connected,
		"lastErr":   lastErr,
		"attempts":  attempts,
		"startedAt": rt.startedAt,
	})
}

// handlePeerlinkEdgeStop —— POST /api/peerlink/edge/stop
func (s *Server) handlePeerlinkEdgeStop(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	s.peerEdgeMu.Lock()
	rt := s.peerEdge
	s.peerEdge = nil
	s.peerEdgeMu.Unlock()
	if rt != nil && rt.cancel != nil {
		rt.cancel()
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "running": false})
}
