package server

// peerlink_agent_api.go —— P4：远程 Agent 调用与授权（HTTP 层）
//
// 两端跑同一个二进制，因此该文件同时服务两种角色：
//   - **发起端**（桌面）：`POST /api/peerlink/agent/invoke` → 经 Hub 中继到对端执行
//   - **执行端**（手机）：`/api/peerlink/agent/pending|approve|trust|audit` → 本端 UI 审批
//
// 安全红线：审批**只在执行端**发生；挂起超时 → **自动 decline**（绝不默认同意）；
// `trust_device` 只存进程内存（重启失效）；破坏性工具即使已信任也强制确认。

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// agentExecTimeout 执行端工具执行上限（与审批挂起时长分开，避免工具挂死）
const agentExecTimeout = 60 * time.Second

// agentIdemGet 惰性创建幂等表（进程内存，重启即失效）。
func (s *Server) agentIdemGet() *agentIdemTable {
	s.agentIdemMu.Lock()
	defer s.agentIdemMu.Unlock()
	if s.agentIdem == nil {
		s.agentIdem = newAgentIdemTable()
	}
	return s.agentIdem
}

// agentApproverGet 惰性创建授权器（进程内存，重启即失效）。
func (s *Server) agentApproverGet() *peerlink.Approver {
	s.agentApproverMu.Lock()
	defer s.agentApproverMu.Unlock()
	if s.agentApprover == nil {
		s.agentApprover = peerlink.NewApprover(peerlink.ApproverOptions{
			Timeout: peerlink.DefaultApprovalTimeout,
			// 破坏性判定：工具注册表里 needConfirm=true 的（插件写入类工具）
			IsDestructive: s.isDestructiveAgentTool,
		})
	}
	return s.agentApprover
}

// isDestructiveAgentTool 判定工具是否破坏性（写文件/加密/删除等）。
// 依据：ListAgentTools 里该工具的 needConfirm=true。
func (s *Server) isDestructiveAgentTool(tool string) bool {
	for _, t := range s.ListAgentTools() {
		name, _ := t["name"].(string)
		if name != tool {
			continue
		}
		nc, ok := t["needConfirm"].(bool)
		return ok && nc
	}
	return false
}

// ── 执行端：被对端调用时的处理（注入给 Edge 的 OnAgentInvoke）──────────

// PeerAgentInvokeHandler 执行端入口：先过本端授权，再执行工具。
//
// ⚠️ 这是**唯一的执行端路径**：Hub 只搬运，不得绕过授权。
// ⚠️ Decision 必须透传授权器的真实决策（auto / accept / trust_device / decline / cancel），
//
//	调用端据此区分"逐次同意"与"已信任设备自动放行"（见 peerlink.AgentInvokeOutcome）。
func (s *Server) PeerAgentInvokeHandler(req peerlink.AgentInvokeRequest) (out peerlink.AgentInvokeOutcome) {
	if strings.TrimSpace(req.Tool) == "" {
		out.Err = errors.New("missing tool")
		return out
	}

	// ── 🆕 2026-10-04 幂等：同一 callId 不得执行两次 ──
	//
	//	重复提交的来源很常见：网络重试、用户误双击、前端重复渲染。
	//	只读工具看不出差别，写类工具（加密/删除）重跑第二次就是副作用事故。
	//	这里让重复请求**等在首次执行的 done 上**而不是另起一次 ——
	//	否则审批还在挂起时再提交一次，手机上会同时弹出两个审批框。
	var idemKey string
	var idemEntry *agentIdemEntry
	if req.CallId != "" {
		idemKey = req.FromId + "|" + req.Tool + "|" + req.CallId
		var isNew bool
		idemEntry, isNew = s.agentIdemGet().begin(idemKey, time.Now())
		if !isNew {
			waitFor := peerlink.DefaultApprovalTimeout + agentExecTimeout
			select {
			case <-idemEntry.done:
				return idemEntry.out
			case <-time.After(waitFor):
				out.Err = errors.New("duplicate call still running")
				return out
			}
		}
		defer func() {
			// 无论成败都要放行等待者，否则并发请求会挂死
			idemEntry.out = out
			close(idemEntry.done)
			// ⚠️ 只有失败才抹掉记录：成功结果要留给后续重复调用复用，
			//    失败（decline / 执行报错）则应允许重试。
			if out.Err != nil {
				s.agentIdemGet().drop(idemKey)
			}
		}()
	}

	ap := s.agentApproverGet()

	peerId := req.FromId
	if peerId == "" {
		peerId = "unknown"
	}
	// ① 挂起等待**本端 UI** 决策（超时自动 decline，绝不默认同意）
	//    注意：这里不能用带短超时的 ctx，否则会提前取消人工等待。
	decision, err := ap.Require(context.Background(), req, peerId, req.FromName)
	out.Decision = decision
	if err != nil {
		out.Err = err // ErrDeclined / ErrCancelled
		return out
	}

	// ② 真正执行（有上限，避免工具挂死）
	ctx, cancel := context.WithTimeout(context.Background(), agentExecTimeout)
	defer cancel()

	args := ""
	if len(req.Args) > 0 {
		args = string(req.Args)
	}
	resStr, execErr := s.executeAgentTool(ctx, req.Tool, args)
	if execErr != nil {
		out.Err = execErr
		return out
	}
	if strings.TrimSpace(resStr) == "" {
		resStr = "{}"
	}
	out.Result = json.RawMessage(resStr)
	return out
}

// ── 发起端：调对端工具 ─────────────────────────────────────────────

type agentInvokeBody struct {
	PeerId   string          `json:"peerId"`
	Tool     string          `json:"tool"`
	Args     json.RawMessage `json:"args,omitempty"`
	CallId   string          `json:"callId,omitempty"`
	FromName string          `json:"fromName,omitempty"`
}

// handlePeerlinkAgentInvoke —— POST /api/peerlink/agent/invoke
func (s *Server) handlePeerlinkAgentInvoke(c *gin.Context) {
	limitKey := "op:" + c.ClientIP()
	if !isOperator(c) {
		id, ok := s.requirePeer(c)
		if !ok {
			return
		}
		limitKey = "peer:" + id
	}
	var body agentInvokeBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json", "detail": err.Error()})
		return
	}
	body.PeerId = strings.TrimSpace(body.PeerId)
	body.Tool = strings.TrimSpace(body.Tool)
	if body.PeerId == "" || body.Tool == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "peerId 与 tool 必填"})
		return
	}
	if _, ok := s.peerHub.PeerByID(body.PeerId); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "peer_not_found"})
		return
	}
	// R12：限流 + 熔断（远程 Agent 是最贵的通道，限额最低）
	if ok, wait := s.rateAllow("agent", limitKey); !ok {
		writeRateLimited(c, wait)
		return
	}
	if !s.peerCircuitAllow(c, body.PeerId) {
		return
	}

	req := peerlink.AgentInvokeRequest{
		CallId:   body.CallId,
		Tool:     body.Tool,
		Args:     body.Args,
		FromId:   s.peerHub.Info()["peerId"],
		FromName: body.FromName,
	}
	if req.FromName == "" {
		req.FromName = s.peerHub.Info()["name"]
	}

	// 发起端台账：脱敏后记录"我调了谁、调了什么"（成功/失败都记，见 agentCallLog）
	argBytes, argKeys := argKeysOf(body.Args)
	logCall := func(decision string, ok bool, errType string) {
		s.agentCallLogGet().add(AgentCallEntry{
			At:        time.Now(),
			PeerId:    body.PeerId,
			PeerName:  req.FromName,
			Tool:      body.Tool,
			CallId:    body.CallId,
			Decision:  decision,
			Ok:        ok,
			ErrorType: errType,
			ArgBytes:  argBytes,
			ArgKeys:   argKeys,
		})
	}

	res, err := s.peerCalls.Call(c.Request.Context(), body.PeerId, "agent_invoke", req, peerlink.AgentCallTimeout)
	if err != nil {
		if peerBusyIfErr(c, body.PeerId, err) {
			return
		}
		s.peerCircuitRecord(body.PeerId, err.Error()) // R12
		// 失败也要留痕：否则"发起过但没成功"的调用在台账里凭空消失，审计就不完整
		errType := "peer_call_failed"
		switch {
		case errors.Is(err, peerlink.ErrPeerOffline):
			errType = "peer_offline"
		case errors.Is(err, peerlink.ErrCallTimeout):
			errType = "peer_timeout"
		case errors.Is(err, peerlink.ErrPeerRejected):
			errType = "peer_rejected"
		case errors.Is(err, peerlink.ErrPeerBusy):
			errType = "peer_busy"
		}
		logCall("", false, errType)
		switch errType {
		case "peer_offline":
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "peer_offline", "peerId": body.PeerId})
		case "peer_timeout":
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "peer_timeout", "peerId": body.PeerId})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"error": "peer_call_failed", "detail": err.Error(), "peerId": body.PeerId})
		}
		return
	}
	var out peerlink.AgentInvokeResult
	if err := json.Unmarshal(res, &out); err != nil {
		logCall("", false, "bad_peer_payload")
		c.JSON(http.StatusBadGateway, gin.H{"error": "bad_peer_payload", "detail": err.Error()})
		return
	}
	// 2026-10-04：工具**业务失败**不得伪装成成功（契约统一）。
	//
	//	真机实测（Hub → 安卓真机）：调 search_files 少传 mount_id ⇒
	//	对端回 `{"ok":true,"decision":"auto","result":{"error":"mount_id is required"}}`。
	//	Ok 只表达"RPC 送达 + 审批通过"（edge.go: Ok = out.Err == nil），
	//	而 fs/plugin 工具把业务错误包成 errJSON 塞进 result（见 agent_fs_bridge.go）。
	//	⇒ 发起端（含前端 UI）只看 ok 会把失败当成功，远程调试时表现为静默失败。
	//	修法：发起端识别 errJSON 形状载荷 ⇒ ok=false + 顶层 error/errorCode，
	//	result 原样保留（向后兼容，调用方仍可解析原始载荷）。
	//	HTTP 仍 200 —— RPC 确实送达并被对端执行了，用 4xx/5xx 会与"链路故障"混淆。
	errType := ""
	if out.Ok {
		if code, msg, isErr := agentToolErrorOf(out.Result); isErr {
			out.Ok = false
			out.ErrorCode = code
			out.Error = msg
			errType = "tool_error"
		}
	}
	logCall(out.Decision, out.Ok, errType)
	c.JSON(http.StatusOK, out)
}

// agentToolErrorOf 识别工具返回的**错误载荷**（errJSON 形状）。
//
// 判定必须保守：只认 errJSON 的两种形状 {"error":code,"message":msg} /
// {"error":code,"detail":msg}（键集合 ⊆ {error,message,detail}，且 error 是非空字符串）。
// 任何带其它键的载荷（如 {"count":N,"items":[...]}）都视为成功 —— 否则会把
// 内含 error 字段的正常业务结果误判成失败（宁可漏判，不可误判成功为失败）。
//
// 返回 (code, msg, isErr)；msg 缺失时回退为 code（真机上确有只带 error 的实现）。
func agentToolErrorOf(res json.RawMessage) (code string, msg string, isErr bool) {
	if len(res) == 0 {
		return "", "", false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(res, &m); err != nil || len(m) == 0 {
		return "", "", false
	}
	for k := range m {
		if k != "error" && k != "message" && k != "detail" {
			return "", "", false
		}
	}
	rawCode, ok := m["error"]
	if !ok {
		return "", "", false
	}
	if err := json.Unmarshal(rawCode, &code); err != nil {
		return "", "", false // error 不是字符串 ⇒ 不是 errJSON 形状
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return "", "", false
	}
	if rawMsg, ok := m["message"]; ok {
		_ = json.Unmarshal(rawMsg, &msg)
	}
	if msg == "" {
		if rawDetail, ok := m["detail"]; ok {
			_ = json.Unmarshal(rawDetail, &msg)
		}
	}
	if msg == "" {
		msg = code
	}
	return code, msg, true
}

// ── 执行端 UI 接口 ─────────────────────────────────────────────────

// handlePeerlinkAgentPending —— GET /api/peerlink/agent/pending
func (s *Server) handlePeerlinkAgentPending(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	ap := s.agentApproverGet()
	c.JSON(http.StatusOK, gin.H{"items": ap.Pending()})
}

type agentApproveBody struct {
	CallId   string `json:"callId"`
	Decision string `json:"decision"` // accept | decline | cancel | trust_device
}

// handlePeerlinkAgentApprove —— POST /api/peerlink/agent/approve
func (s *Server) handlePeerlinkAgentApprove(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var body agentApproveBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json", "detail": err.Error()})
		return
	}
	ap := s.agentApproverGet()
	if err := ap.Decide(strings.TrimSpace(body.CallId), body.Decision); err != nil {
		switch {
		case errors.Is(err, peerlink.ErrBadDecision):
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_decision",
				"message": "decision 必须是 accept / decline / cancel / trust_device",
			})
		case errors.Is(err, peerlink.ErrNoPending):
			c.JSON(http.StatusNotFound, gin.H{"error": "no_pending", "message": "该请求已被处理或已超时"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "decide_failed", "detail": err.Error()})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "callId": body.CallId, "decision": body.Decision})
}

// handlePeerlinkAgentTrust —— GET（列）/ DELETE（撤销）/ POST（授予，等价 trust_device 决策）
func (s *Server) handlePeerlinkAgentTrust(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	ap := s.agentApproverGet()
	switch c.Request.Method {
	case http.MethodGet:
		c.JSON(http.StatusOK, gin.H{"items": ap.TrustedPeers()})
	case http.MethodDelete:
		id := strings.TrimSpace(c.Query("peerId"))
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "peerId 必填"})
			return
		}
		ap.Untrust(id)
		c.JSON(http.StatusOK, gin.H{"ok": true, "untrusted": id})
	case http.MethodPost:
		// vNext Round 5：授予信任 = 预先做 trust_device 决策。
		//
		// 旧行为里"信任"只能在**远端第一次调用**弹审批时顺手点信任 ——
		// 用户在连上之后、还没被调用之前，根本没法表达"我信任这台设备"。
		// 现在受控端（手机）连上即可主动授信，调用时不再被打断。
		//
		// 语义不变：进程内存、重启失效；破坏性工具即使已信任仍强制确认。
		var body struct {
			PeerID string `json:"peerId"`
		}
		_ = c.ShouldBindJSON(&body)
		id := strings.TrimSpace(body.PeerID)
		if id == "" {
			id = strings.TrimSpace(c.Query("peerId"))
		}
		if id == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "peerId 必填"})
			return
		}
		ap.Trust(id)
		slog.Info("peerlink: trust granted (pre-authorized by local user)", "peerId", id)
		c.JSON(http.StatusOK, gin.H{"ok": true, "trusted": id, "note": "进程级信任，重启失效；破坏性工具仍需逐次确认"})
	default:
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method_not_allowed"})
	}
}

// handlePeerlinkAgentAudit —— GET /api/peerlink/agent/audit（**脱敏**）
func (s *Server) handlePeerlinkAgentAudit(c *gin.Context) {
	if !isOperator(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	ap := s.agentApproverGet()
	items := ap.Audit()
	if items == nil {
		items = []peerlink.AuditEntry{}
	}
	// 2026-10-04：补上**发起端**台账。
	//
	//	`items` 是**执行端**记录（本端被对端调用时产生的授权审计），保持不变 ⇒ 不破坏既有调用方。
	//	新增 `outbound` = 本端作为发起端调出去的记录 —— 之前桌面端永远查不到自己发起过什么。
	//	两者语义不同，不混在一个数组里（否则前端无法区分"别人调我"与"我调别人"）。
	outbound := s.agentCallLogGet().snapshot()
	if outbound == nil {
		outbound = []AgentCallEntry{}
	}
	c.JSON(http.StatusOK, gin.H{
		"items":         items,
		"count":         len(items),
		"outbound":      outbound,
		"outboundCount": len(outbound),
	})
}
