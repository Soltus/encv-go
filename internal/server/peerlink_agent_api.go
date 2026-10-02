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
	"net/http"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// agentExecTimeout 执行端工具执行上限（与审批挂起时长分开，避免工具挂死）
const agentExecTimeout = 60 * time.Second

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

	res, err := s.peerCalls.Call(c.Request.Context(), body.PeerId, "agent_invoke", req, peerlink.AgentCallTimeout)
	if err != nil {
		s.peerCircuitRecord(body.PeerId, err.Error()) // R12
		switch {
		case errors.Is(err, peerlink.ErrPeerOffline):
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "peer_offline", "peerId": body.PeerId})
		case errors.Is(err, peerlink.ErrCallTimeout):
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "peer_timeout", "peerId": body.PeerId})
		default:
			c.JSON(http.StatusBadGateway, gin.H{"error": "peer_call_failed", "detail": err.Error(), "peerId": body.PeerId})
		}
		return
	}
	var out peerlink.AgentInvokeResult
	if err := json.Unmarshal(res, &out); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "bad_peer_payload", "detail": err.Error()})
		return
	}
	c.JSON(http.StatusOK, out)
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
	c.JSON(http.StatusOK, gin.H{"items": items, "count": len(items)})
}
