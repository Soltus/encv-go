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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
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

	// ── 各包的期望：**从本端落地目标派生**，不是硬编码 ───────────────────
	//    · 文件型（go-binary）：必含文件 = 落地目标的**文件名**（本端自报）
	//    · 目录型（web / preview-assets）：必含入口 index.html（与本端切换判据一致）
	//    · native-lib：目录型，.so 文件名不固定 ⇒ Required 留空，但必须校验 ABI
	specs := make([]peerlink.BundleSpec, 0, 4)
	for _, name := range []string{"web", "preview-assets", "go-binary", "native-lib"} {
		sp := peerlink.BundleSpec{Name: name}
		switch {
		case name == "native-lib":
			if dir, ok := bundleTargetDirFor(name); ok {
				sp.Kind, sp.Available, sp.AbiRequired = "dir", true, true
				_ = dir
			}
		default:
			if dir, ok := bundleTargetDirFor(name); ok {
				sp.Kind, sp.Available = "dir", true
				sp.Required = []string{"index.html"}
				_ = dir
			} else if f, ok := bundleTargetFileFor(name); ok {
				sp.Kind, sp.Available = "file", true
				sp.Required = []string{filepath.Base(f)}
				sp.AbiRequired = name == "go-binary"
			}
		}
		specs = append(specs, sp)
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
		BundleSpecs:   specs,
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

// handlePeerlinkLocalCapabilities —— GET /api/peerlink/capabilities（**本端自省**）
//
// 与 /api/peerlink/peer/capabilities 的区别：
//   · peer/capabilities —— Hub 经 RPC 问**对端**（受控端）
//   · capabilities      —— 本端问**自己**（运行在手机上的 UI 用它判断"我这台设备能干啥"）
//
// UI 用途（vNext Round 13）：按能力禁用按钮 ——
//   不支持 bundle_reload ⇒ 重载/重启按钮置灰并说明原因，而不是点了才报错。
func (s *Server) handlePeerlinkLocalCapabilities(c *gin.Context) {
	out := s.peerLocalCapabilities(peerlink.PeerCapabilitiesRequest{})
	if !out.Ok || out.Caps == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "capabilities_unavailable", "detail": out.Error})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":   true,
		"caps": out.Caps,
		"hints": gin.H{
			"reloadSupported":  containsString(out.Caps.Methods, peerlink.MethodBundleReload),
			"updateSupported":  containsString(out.Caps.Methods, peerlink.MethodBundleUpdate),
			"rollbackSupported": containsString(out.Caps.Methods, peerlink.MethodBundleRollback),
		},
	})
}

// resolveRequiredFiles 决定下发时的"包内必须存在的文件"清单。
//
// 优先级（vNext Round 13）：
//   1. **受控端自报**（peer_capabilities 的 bundleSpecs）—— 权威来源。
//      它的 Required 是从自己的落地目标派生的，不是 Hub 写死的常量。
//   2. Hub 侧硬编码 —— 只为兼容**不支持能力自省**的旧设备，且会 warn。
//   3. 都没有 ⇒ 返回错误，**绝不猜**。
//
// 返回 (required, from, error)；from = "peer" | "hub-fallback"。
func (s *Server) resolveRequiredFiles(ctx context.Context, peerID, name string) ([]string, string, error) {
	if caps, err := s.queryPeerCapabilities(ctx, peerID); err == nil && caps != nil {
		for _, sp := range caps.BundleSpecs {
			if sp.Name != name {
				continue
			}
			if !sp.Available {
				return nil, "", fmt.Errorf("受控端声明包 %q 在本端没有落地目标（不可用）", name)
			}
			// 目录型（如 native-lib）没有固定必含文件 ⇒ 空清单是合法的
			return sp.Required, "peer", nil
		}
		return nil, "", fmt.Errorf("受控端未声明包 %q", name)
	}
	if local := bundleRequiredFiles(name); len(local) > 0 {
		slog.Warn("bundle: 回退 Hub 侧硬编码的 Required（受控端未自报能力，多半是旧版本二进制）",
			"name", name, "peerId", peerID)
		return local, "hub-fallback", nil
	}
	return nil, "", fmt.Errorf("无法确定包 %q 的必含文件：受控端未自报且 Hub 无硬编码", name)
}

// queryPeerCapabilities 向受控端问一次能力。失败不致命 —— 由调用方降级处理。
func (s *Server) queryPeerCapabilities(ctx context.Context, peerID string) (*peerlink.PeerCapabilities, error) {
	res, err := s.peerCalls.Call(ctx, peerID, peerlink.MethodPeerCapabilities,
		peerlink.PeerCapabilitiesRequest{}, peerlink.BundleCallTimeout)
	if err != nil {
		return nil, err
	}
	var out peerlink.PeerCapabilitiesResult
	if err := json.Unmarshal(res, &out); err != nil {
		return nil, err
	}
	if !out.Ok || out.Caps == nil {
		msg := out.Error
		if msg == "" {
			msg = "peer_capabilities unavailable"
		}
		return nil, errors.New(msg)
	}
	return out.Caps, nil
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
