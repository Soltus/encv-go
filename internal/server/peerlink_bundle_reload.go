package server

// peerlink_bundle_reload.go —— 云控三级重载（2026-10-06，Round 10 简化）
//
// 起因：热更包应用后**只能靠用户手动重启 App** 才生效
// （Kotlin 的 applyHotWebBundleIfPresent 只在启动时判定）。
//
// 三级（由轻到重）：
//
//	web      —— 只让 WebView 重新加载：Go 广播 WS，前端 reload，**无感**
//	activity —— recreate 当前 Activity（重建页面与插件桥）
//	app      —— 整个 App 进程重启（换执行体 / 换 Go 二进制时用）
//
// ⚠️ 架构边界（Round 10 纠正）：
//
//	Go **只广播指令**，不做任何"把指令塞给 Kotlin"的旁门左道
//	（不再有 pending 轮询 / 不再有 HTTP 交接接口 —— 那会让后端逻辑
//	 泄漏到 Activity，是架构错误）。
//
//	真正执行的一定是**原生层**，且链路是事件驱动：
//	  Go --WS--> 前端(WsBackend) --插件桥--> GoProcessPlugin.reloadApp()
//	      --> 原生弹二次确认 --> recreate / 重启进程
//
//	activity / app 级**必须二次确认**（重建页面丢状态、重启中断操作），
//	远端不能替用户做这个决定 ⇒ 由原生弹窗，用户取消就不执行。

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// peerLocalBundleReload 执行端（手机）收到云控重载指令。
//
// 只做一件事：把指令广播出去。执行与确认都在原生层。
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

	// 广播给已连接的前端：web 级前端就地 reload；activity/app 级由前端
	// 经插件桥转交原生（原生弹确认后才执行）。
	s.BroadcastMessage("bundle_reload", map[string]interface{}{
		"level":  level,
		"reason": req.Reason,
	})

	return peerlink.BundleReloadResult{
		Ok: true,
		// web 级当场生效；activity/app 级要等原生确认，这里不能宣称已生效
		Applied: level == peerlink.ReloadLevelWeb,
		Level:   level,
	}
}

// handlePeerlinkBundleReload —— POST /api/peerlink/bundle/reload（云控，运维）
//
// 入参：{ peerId, level: "web"|"activity"|"app", reason? }
//
// ⚠️ 返回的 `applied=false` **不是失败**：表示指令已送达设备，
// 但 activity/app 级要等用户在原生确认框点"确定"才会真正执行。
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
		"note":    "applied=false 表示已送达设备，activity/app 级需用户在原生确认框确认后才执行",
	})
}
