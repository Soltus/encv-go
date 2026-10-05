package server

// peerlink_bundle_reload.go —— 云控三级重载（2026-10-06）
//
// 起因（真机反馈）：热更包应用后**只能靠用户手动重启 App** 才生效
// （Kotlin 的 applyHotWebBundleIfPresent 只在启动时判定）⇒ 推完还要人手点一次，
// 既慢又容易让人误以为"没生效"。
//
// 三级（由轻到重）：
//
//	web      —— 只让 WebView 重新加载：Go 广播 WS 事件，前端 reload，**当场生效**
//	activity —— recreate 当前 Activity（重建页面与插件桥）
//	app      —— 整个 App 进程重启（换执行体 / 换 Go 二进制时用）
//
// 分工（Go 能做什么、不能做什么，必须写清楚，别让调用方以为"返回 ok 就等于重启了"）：
//
//	Go 能直接做：web 级（广播 WS 让前端 reload）
//	Go 做不到：activity / app（Android 组件生命周期只能由 Kotlin 操作）
//	            ⇒ Go 只**落指令**（pending），由 Kotlin 侧取走执行并 ack
//
// 因此 `applied` 字段就是用来区分的：
//
//	applied=true  ⇒ 当场生效
//	applied=false ⇒ 指令已落，等 Kotlin 接管（**不是失败**）

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

var (
	reloadMu      sync.Mutex
	reloadPending string // 待 Kotlin 执行的级别（"" / "activity" / "app"）
	reloadReason  string
)

func setPendingReload(level, reason string) {
	reloadMu.Lock()
	defer reloadMu.Unlock()
	// 更重的一级优先：app > activity
	if reloadPending == peerlink.ReloadLevelApp && level != peerlink.ReloadLevelApp {
		return
	}
	reloadPending = level
	reloadReason = reason
}

func peekPendingReload() (level, reason string) {
	reloadMu.Lock()
	defer reloadMu.Unlock()
	return reloadPending, reloadReason
}

func takePendingReload() (level, reason string) {
	reloadMu.Lock()
	defer reloadMu.Unlock()
	level, reason = reloadPending, reloadReason
	reloadPending, reloadReason = "", ""
	return
}

// peerLocalBundleReload 执行端（手机）收到云控重载指令。
func (s *Server) peerLocalBundleReload(req peerlink.BundleReloadRequest) peerlink.BundleReloadResult {
	level := strings.ToLower(strings.TrimSpace(req.Level))
	if level == "" {
		level = peerlink.ReloadLevelWeb
	}
	switch level {
	case peerlink.ReloadLevelWeb, peerlink.ReloadLevelActivity, peerlink.ReloadLevelApp:
	default:
		return peerlink.BundleReloadResult{
			Rejected: true,
			Error:    "unknown reload level: " + req.Level,
		}
	}

	if level == peerlink.ReloadLevelWeb {
		// web 级：广播给已连接前端，前端监听后自行 reload ⇒ **用户无感**
		s.BroadcastMessage("bundle_reload", map[string]interface{}{
			"level":  level,
			"reason": req.Reason,
		})
		// ⚠️ 同时落一份 pending：前端 WS 监听**尚未接入**之前，
		// 由 Kotlin 在下次 onCreate 取走并执行 WebView.reload() 兜底。
		// 这不是"无感"（要重开 App），只是先保证不用人手去点。
		// 前端监听接上后，可移除这一行（Kotlin 侧 web 分支保留即可）。
		setPendingReload(level, req.Reason)
		return peerlink.BundleReloadResult{Ok: true, Level: level, Applied: true}
	}

	// activity / app：Go 落指令，等 Kotlin 取走
	setPendingReload(level, req.Reason)
	s.BroadcastMessage("bundle_reload", map[string]interface{}{
		"level":   level,
		"reason":  req.Reason,
		"applied": false,
	})
	return peerlink.BundleReloadResult{Ok: true, Level: level, Applied: false}
}

// handleReloadPending —— GET /api/reload/pending
//
// Kotlin 侧（MainActivity）查询"有没有待执行的重载"。这是 Go 与 Kotlin 之间
// **不依赖文件路径约定**的交接方式（Go 无法直接操作 Activity，只能等原生来取）。
func (s *Server) handleReloadPending(c *gin.Context) {
	level, reason := peekPendingReload()
	c.JSON(http.StatusOK, gin.H{"level": level, "reason": reason})
}

// handleReloadAck —— POST /api/reload/ack
// Kotlin 执行完毕后确认，清掉 pending（避免重复重启）。
func (s *Server) handleReloadAck(c *gin.Context) {
	level, _ := takePendingReload()
	c.JSON(http.StatusOK, gin.H{"ok": true, "cleared": level})
}

// handlePeerlinkBundleReload —— POST /api/peerlink/bundle/reload（云控，运维）
//
// 入参：{ peerId, level: "web"|"activity"|"app", reason? }
func (s *Server) handlePeerlinkBundleReload(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var body struct {
		PeerId string `json:"peerId"`
		Level  string `json:"level"`
		Reason string `json:"reason,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json", "detail": err.Error()})
		return
	}
	body.PeerId = strings.TrimSpace(body.PeerId)
	if body.PeerId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "peerId 必填"})
		return
	}
	if _, ok := s.peerHub.PeerByID(body.PeerId); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "peer_not_found"})
		return
	}

	req := peerlink.BundleReloadRequest{Level: body.Level, Reason: body.Reason}
	res, err := s.peerCalls.Call(c.Request.Context(), body.PeerId, peerlink.MethodBundleReload, req, peerlink.BundleCallTimeout)
	if err != nil {
		if peerBusyIfErr(c, body.PeerId, err) {
			return
		}
		if s.peerRejectedIfErr(c, body.PeerId, err) {
			return
		}
		s.peerCircuitRecord(body.PeerId, err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": "reload_failed", "detail": err.Error(), "peerId": body.PeerId})
		return
	}
	s.peerCircuitRecordSuccess(body.PeerId)

	var out peerlink.BundleReloadResult
	if err := json.Unmarshal(res, &out); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "bad_peer_payload", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":      out.Ok,
		"peerId":  body.PeerId,
		"level":   out.Level,
		"applied": out.Applied,
		"note":    "applied=false 表示指令已落、等 Kotlin 接管（activity/app 级），不是失败",
	})
}

// registerReloadRoutes 挂载本端重载交接路由（供 Kotlin 取指令 / 回执）。
func (s *Server) registerReloadRoutes(r *gin.Engine) {
	r.GET("/api/reload/pending", s.handleReloadPending)
	r.POST("/api/reload/ack", s.handleReloadAck)
}
