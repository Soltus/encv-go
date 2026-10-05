package server

// peerlink_capabilities.go —— 受控端能力自省（vNext Round 12）
//
// 为什么要有它（2026-10-06 用户批评："一堆脆弱的校验 / 自导自演"）：
//
//	此前云控侧靠**硬编码约定**猜设备能力：
//	  包名/文件名、ABI 字符串、版本注入的变量名、支持哪些方法……
//	这些只有掌握全部上下文的人才说得对，换个设备/构建/维护者就崩。
//
// 现在反过来：**Hub 问，设备答**。设备自己上报：
//   - 身份：跑的二进制版本、ABI、架构、peerId、运行时长
//   - 能力：实际接线的 RPC 方法、支持的重载级别、各包真实安装状态
//   - 诚实标注"我不知道"（Unknowns），Hub 必须显式提示而不是当已知
//
// ⇒ 云控据此决策：不支持就明确说不支持（UI 禁用按钮 + 给原因），
//    而不是推一个注定失败的包等真机报错。

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// peerLocalCapabilities 本端（作为被控端时）回答"我是谁、我能做啥"。
//
// 所有字段都来自**本端真实状态**，不做任何按名字的推断。
func (s *Server) peerLocalCapabilities(_ peerlink.PeerCapabilitiesRequest) peerlink.PeerCapabilitiesResult {
	h := s.edgeHandlers()

	// ── 能力：只报**实际接线**的方法（handler 非 nil 才算支持）────────────
	methods := make([]string, 0, 8)
	if h.OnRead != nil {
		methods = append(methods, "read")
	}
	if h.OnSearch != nil {
		methods = append(methods, "search")
	}
	if h.OnAgentInvoke != nil {
		methods = append(methods, "agent_invoke")
	}
	if h.OnBundleUpdate != nil {
		methods = append(methods, peerlink.MethodBundleUpdate)
	}
	if h.OnBundleRollback != nil {
		methods = append(methods, peerlink.MethodBundleRollback)
	}
	if h.OnBundleReload != nil {
		methods = append(methods, peerlink.MethodBundleReload)
	}
	if h.OnPeerCapabilities != nil {
		methods = append(methods, peerlink.MethodPeerCapabilities)
	}

	// ── 重载级别：只在本端真的接了 reload 时才声明 ────────────────────────
	levels := make([]string, 0, 3)
	if h.OnBundleReload != nil {
		levels = append(levels, peerlink.ReloadLevelWeb, peerlink.ReloadLevelActivity, peerlink.ReloadLevelApp)
	}

	// ── 各包在本端的真实状态（不是 Hub 侧台账）──────────────────────────
	bundles := make([]peerlink.BundleCap, 0, 4)
	for _, st := range bundleLocalStates() {
		bundles = append(bundles, peerlink.BundleCap{
			Name:      st.Name,
			Installed: strings.TrimSpace(st.Version) != "",
			Version:   st.Version,
			Rollable:  st.Rollable,
		})
	}

	// ── 身份：拿不到就留空并记进 Unknowns（绝不猜）──────────────────────
	unknowns := make([]string, 0, 2)
	abi := strings.TrimSpace(os.Getenv("ENCV_APP_ABI"))
	if abi == "" {
		unknowns = append(unknowns, "abi")
	}
	if strings.TrimSpace(s.version) == "" || s.version == "dev" {
		// "dev" = 构建时没注入版本 ⇒ Hub 不能据此判断跑的是哪个包
		unknowns = append(unknowns, "binaryVersion")
	}

	// Hub.Info() 返回 map[string]string，取值本身即 string（无需类型断言）
	peerID := s.peerHub.Info()["peerId"]

	caps := &peerlink.PeerCapabilities{
		BinaryVersion: s.version,
		Abi:           abi,
		Goos:          runtime.GOOS,
		Goarch:        runtime.GOARCH,
		PeerId:        peerID,
		UptimeMs:      s.snapshotRuntimeInfo().UptimeMs,
		Methods:       methods,
		ReloadLevels:  levels,
		Bundles:       bundles,
		Unknowns:      unknowns,
	}
	return peerlink.PeerCapabilitiesResult{Ok: true, Caps: caps}
}

// handlePeerlinkPeerCapabilities —— GET /api/peerlink/peer/capabilities?peerId=…
//
// 云控在**下发/重载之前**先问这一句：
//   - 对端不支持 bundle_reload ⇒ 别推，UI 直接禁用
//   - 对端 abi 未知 ⇒ 别推 go-binary（换了执行体起不来会被自动作废）
//   - 对端 binaryVersion 是 dev ⇒ 提醒"没法确证跑的是哪个包"
func (s *Server) handlePeerlinkPeerCapabilities(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	peerID := strings.TrimSpace(c.Query("peerId"))
	if peerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "peerId 必填"})
		return
	}
	if _, ok := s.peerHub.PeerByID(peerID); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "peer_not_found"})
		return
	}

	res, err := s.peerCalls.Call(c.Request.Context(), peerID, peerlink.MethodPeerCapabilities,
		peerlink.PeerCapabilitiesRequest{}, peerlink.BundleCallTimeout)
	if err != nil {
		if peerBusyIfErr(c, peerID, err) {
			return
		}
		if s.peerRejectedIfErr(c, peerID, err) {
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": "capabilities_failed", "detail": err.Error(), "peerId": peerID})
		return
	}

	var out peerlink.PeerCapabilitiesResult
	if err := json.Unmarshal(res, &out); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "bad_peer_payload", "detail": err.Error()})
		return
	}
	if !out.Ok || out.Caps == nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "capabilities_unavailable", "detail": out.Error})
		return
	}

	// 给出**可执行**的判断，而不是丢一堆字段让调用方自己想
	advice := make([]string, 0, 3)
	if !containsString(out.Caps.Methods, peerlink.MethodBundleReload) {
		advice = append(advice, "对端不支持 bundle_reload：不要下发重载指令（多为空/旧版本二进制）")
	}
	if !containsString(out.Caps.Methods, peerlink.MethodBundleUpdate) {
		advice = append(advice, "对端不支持 bundle_update：热更通道不可用")
	}
	if containsString(out.Caps.Unknowns, "abi") {
		advice = append(advice, "对端 ABI 未知：不要推 go-binary（架构不符会起不来后端，会被自动作废）")
	}
	if containsString(out.Caps.Unknowns, "binaryVersion") {
		advice = append(advice, "对端版本未注入（dev）：无法远程确证跑的是 APK 内置还是热更包")
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"peerId":  peerID,
		"caps":    out.Caps,
		"advice":  advice,
		"note":    "能力由受控端自报；advice 为空表示没有已知阻塞项",
	})
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
