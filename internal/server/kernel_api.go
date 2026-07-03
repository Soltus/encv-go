// internal/server/kernel_api.go — kernel 内核 HTTP API
//
// 2026-07-03 新增：特色微服务内核接入主代码库
//
// 端点：
//   - GET /api/kernel/services  : 列出所有已注册的 kernel.Service（含 name + call stats）
//   - GET /api/kernel/health    : 聚合所有 kernel.Service 的 Health（并行调用）
//   - POST /api/kernel/call     : 通过 kernel.Call 调用某 service 的 method（debug 用）
//
// 设计说明：
//   - GET 端点对前端只读暴露（不影响安全，无敏感数据）
//   - POST /api/kernel/call 仅在 dev 模式开放（生产模式返回 403）
//     避免恶意调用任意 service.method（如 fts.rebuilder.rebuild 可耗资源）
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Soltus/encv-go/internal/kernel"
	"github.com/gin-gonic/gin"
)

// isDevMode 与 configMountProvider.IsDev() 保持一致：
// ENCV_DEV=1 或 ENCV_DEV_PREVIEW=1 任一为真即为 dev 模式。
func isDevMode() bool {
	return os.Getenv("ENCV_DEV") == "1" || os.Getenv("ENCV_DEV_PREVIEW") == "1"
}

// kernelListNames 返回已注册的 kernel.Service 名列表（启动日志用）。
func kernelListNames() []string {
	return kernel.List()
}

// handleKernelServicesGin 列出所有已注册的 kernel.Service。
//
// 响应：
//
//	{
//	  "services": ["search.vector", "ws.hub", "fts.rebuilder"],
//	  "count": 3
//	}
func (s *Server) handleKernelServicesGin(c *gin.Context) {
	names := kernel.List()
	c.JSON(http.StatusOK, gin.H{
		"services": names,
		"count":    len(names),
	})
}

// handleKernelHealthGin 聚合所有 kernel.Service 的 Health 状态。
//
// 响应：
//
//	{
//	  "ok": true,
//	  "services": [
//	    {"name": "search.vector", "ok": true, "latency": 1200000},
//	    {"name": "ws.hub",        "ok": true, "latency":  500000},
//	    {"name": "fts.rebuilder", "ok": false, "error": "not initialized", "latency": 200000}
//	  ]
//	}
//
// ok 字段：所有 service.Health 都返回 nil 时为 true，任一错误为 false。
// HTTP 状态码：ok=true 时 200，ok=false 时 503（让前端 / 监控能区分）。
func (s *Server) handleKernelHealthGin(c *gin.Context) {
	ctx := kernel.NewContext(c.Request.Context())
	statuses := kernel.HealthAll(ctx)

	allOK := true
	for _, st := range statuses {
		if !st.OK {
			allOK = false
			break
		}
	}

	code := http.StatusOK
	if !allOK {
		code = http.StatusServiceUnavailable
	}

	c.JSON(code, gin.H{
		"ok":       allOK,
		"services": statuses,
	})
}

// handleKernelCallGin 通过 kernel.Call 调用某 service 的 method（debug 用）。
//
// 请求 body：
//
//	{
//	  "service": "search.vector",
//	  "method":  "search_files",
//	  "payload": {"query": "video", "limit": 10}
//	}
//
// 响应：
//
//	{
//	  "ok": true,
//	  "response": {...}
//	}
//
// 安全：仅在 dev 模式（cfg.isDev=true）开放，生产模式返回 403。
func (s *Server) handleKernelCallGin(c *gin.Context) {
	if !isDevMode() {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "kernel.call is only available in dev mode",
		})
		return
	}

	var req struct {
		Service string          `json:"service"`
		Method  string          `json:"method"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request: " + err.Error()})
		return
	}
	if req.Service == "" || req.Method == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "service and method are required"})
		return
	}

	ctx := kernel.NewContext(c.Request.Context())
	resp, err := kernel.Call(ctx, req.Service, req.Method, req.Payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	// resp 是 json.RawMessage，直接作为 JSON 返回（避免二次序列化）
	c.Data(http.StatusOK, "application/json", []byte(`{"ok":true,"response":`+string(resp)+`}`))
}

// ─── Lifecycle 端点（2026-07-03 新增，spec android-workmanager-split-start-stop Phase 1.3） ──────
//
// 这组端点支撑 Cypress E2E 频繁启停测试 + WorkManager 风格断点续跑验证：
//
//	GET  /api/kernel/pools             — 列出受管 Pool 状态（queue / submitted / lastRestore）
//	POST /api/kernel/restore           — 手动触发 Restore（dev only，Cypress 用）
//	GET  /api/kernel/lifecycle/stats   — Lifecycle 启停耗时 + 内存 + MemGuard 状态
//	POST /api/kernel/lifecycle/start   — 启动 Lifecycle（dev only，Cypress 频繁启停用）
//	POST /api/kernel/lifecycle/stop    — 停止 Lifecycle（dev only，委托 in-flight 给 Ledger）
//
// 设计：
//   - lifecycle/{start,stop,restore} 仅 dev 开放（生产模式返回 403）
//     避免恶意触发 Stop 把 in-flight job 全部委托导致服务不可用
//   - pools / lifecycle/stats 只读，对所有模式开放（监控用）
//   - Lifecycle 未启用（ENCV_USE_KERNEL_POOL != 1）时返回 503 Service Unavailable
//
// 用户硬约束：
//   - 启动 ≤ 500ms / 停止 ≤ 200ms（lifecycle/stats 暴露 lastStartDurationMs / lastStopDurationMs）
//   - 内存守卫（lifecycle/stats 暴露 memGuardEnabled / memGuardTriggered / mem.heapAllocMB）
//   - 不消耗 TCP 端口（Lifecycle 是进程内对象，无 listen）

// kernelLifecycleOr503 返回 Lifecycle；未启用时写 503 响应并返回 nil。
func (s *Server) kernelLifecycleOr503(c *gin.Context) *kernel.Lifecycle {
	if s.kernelLifecycle == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "kernel lifecycle not enabled (set ENCV_USE_KERNEL_POOL=1)",
		})
		return nil
	}
	return s.kernelLifecycle
}

// handleKernelPoolsGin 列出所有受管 Pool 的状态。
//
// 响应：
//
//	{
//	  "pools": [
//	    {
//	      "name": "task-manager",
//	      "size": 4,
//	      "queueLen": 2,
//	      "queueSize": 64,
//	      "submitted": 12,
//	      "finished": 10,
//	      "failed": 0,
//	      "retried": 1,
//	      "ledgerEnabled": true,
//	      "lastRestoreCount": 3,
//	      "lastRestoreAt": "2026-07-03T10:00:00Z"
//	    }
//	  ],
//	  "count": 1
//	}
func (s *Server) handleKernelPoolsGin(c *gin.Context) {
	lc := s.kernelLifecycleOr503(c)
	if lc == nil {
		return
	}
	stats := lc.Stats()
	c.JSON(http.StatusOK, gin.H{
		"pools": stats.Pools,
		"count": len(stats.Pools),
	})
}

// handleKernelRestoreGin 手动触发 Restore（dev only）。
//
// 用途：Cypress E2E kernel-restart-restore 测试中，验证 Stop 委托的 job 在下次 Start 时被 Restore。
// 也可用于生产环境运维（如 ledger 文件手动恢复后触发）。
//
// 响应：
//
//	{
//	  "ok": true,
//	  "restored": 3,
//	  "perPool": [{"name":"task-manager","restored":3}]
//	}
func (s *Server) handleKernelRestoreGin(c *gin.Context) {
	if !isDevMode() {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "kernel.restore is only available in dev mode",
		})
		return
	}
	lc := s.kernelLifecycleOr503(c)
	if lc == nil {
		return
	}
	// 手动触发 Restore = Stop + Start 循环（WorkManager 风格的"重启续跑"）
	// Stop 会把 in-flight job 委托给 Ledger，Start 内部会调 Restore 把它们重投。
	// 这样 Cypress E2E 可以验证 Stop→Start 循环后 job 续跑正确。
	if err := lc.Stop(0); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "stop: " + err.Error()})
		return
	}
	if err := lc.Start(context.Background()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": "start: " + err.Error()})
		return
	}
	// 重新拿 stats 看 lastRestoreCount（Start 内部 Restore 后会更新 LastRestoreInfo）
	type perPool struct {
		Name     string `json:"name"`
		Restored int    `json:"restored"`
	}
	newStats := lc.Stats()
	per := make([]perPool, 0, len(newStats.Pools))
	total := 0
	for _, p := range newStats.Pools {
		per = append(per, perPool{Name: p.Name, Restored: p.LastRestoreCount})
		total += p.LastRestoreCount
	}
	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"restored": total,
		"perPool":  per,
	})
}

// handleKernelLifecycleStatsGin 返回 Lifecycle 运行时统计。
//
// 响应（关键字段）：
//
//	{
//	  "name": "main",
//	  "ready": true,
//	  "startedAt": "2026-07-03T10:00:00Z",
//	  "stoppedAt": "0001-01-01T00:00:00Z",
//	  "lastStartDurationMs": 0.45,   ← 启动耗时（用户硬约束 ≤ 500ms）
//	  "lastStopDurationMs": 50.2,    ← 停止耗时（用户硬约束 ≤ 200ms）
//	  "pools": [...],
//	  "mem": {
//	    "heapAllocMB": 12.3,
//	    "heapInuseMB": 15.0,
//	    "sysMB": 50.0,
//	    "numGC": 42,
//	    "sampleAt": "2026-07-03T10:01:00Z"
//	  },
//	  "memGuardEnabled": true,
//	  "memGuardTriggered": false
//	}
func (s *Server) handleKernelLifecycleStatsGin(c *gin.Context) {
	lc := s.kernelLifecycleOr503(c)
	if lc == nil {
		return
	}
	c.JSON(http.StatusOK, lc.Stats())
}

// handleKernelLifecycleStartGin 启动 Lifecycle（dev only）。
//
// 用于 Cypress E2E 频繁启停测试：测 Stop 委托 + Start Restore 的循环。
// 已启动时返回 409 Conflict。
//
// ⚠️ 关键：必须用 context.Background()，不能用 c.Request.Context()。
// Lifecycle 生命周期跨越多个 HTTP 请求（Start 后要持续服务业务请求），
// 如果用 request ctx，响应发送后 ctx 被 cancel → pool.ctx 被 cancel →
// Submit 立即返回 "pool closed"（即使 lifecycle.ready=true）。
// 这与 server.go 初始 Start 用 context.Background() 保持一致。
func (s *Server) handleKernelLifecycleStartGin(c *gin.Context) {
	if !isDevMode() {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "kernel.lifecycle.start is only available in dev mode",
		})
		return
	}
	lc := s.kernelLifecycleOr503(c)
	if lc == nil {
		return
	}
	if err := lc.Start(context.Background()); err != nil {
		c.JSON(http.StatusConflict, gin.H{"ok": false, "error": err.Error()})
		return
	}
	stats := lc.Stats()
	c.JSON(http.StatusOK, gin.H{
		"ok":                 true,
		"lastStartDurationMs": stats.LastStartDurationMs,
		"ready":              stats.Ready,
	})
}

// handleKernelLifecycleStopGin 停止 Lifecycle（dev only）。
//
// 停止时 in-flight job 委托给 Ledger（drainPendingToLedger），
// 下次 Start 时 Restore 会把它们续跑。
//
// graceful 参数（query string，毫秒）控制 grace 窗口，默认用 Lifecycle 配置的 100ms。
// 已停止时返回 200（幂等）。
func (s *Server) handleKernelLifecycleStopGin(c *gin.Context) {
	if !isDevMode() {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "kernel.lifecycle.stop is only available in dev mode",
		})
		return
	}
	lc := s.kernelLifecycleOr503(c)
	if lc == nil {
		return
	}
	grace := 0 // 0 = 用 Lifecycle 默认（100ms）
	if v := c.Query("graceMs"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil && ms >= 0 {
			grace = ms
		}
	}
	if err := lc.Stop(time.Duration(grace) * time.Millisecond); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"ok": false, "error": err.Error()})
		return
	}
	stats := lc.Stats()
	c.JSON(http.StatusOK, gin.H{
		"ok":                true,
		"lastStopDurationMs": stats.LastStopDurationMs,
		"ready":             stats.Ready,
	})
}

// handleKernelSubmitGin 异步提交一个 job 到 kernel Pool（dev only）。
//
// 请求 body：
//
//	{
//	  "service": "fts.rebuilder",
//	  "method":  "rebuild",
//	  "payload": {"any": "json"},
//	  "jobId":   "optional-custom-id"
//	}
//
// 响应（202 Accepted）：
//
//	{
//	  "ok":    true,
//	  "jobId": "fts.rebuilder-<timestamp>",
//	  "traceID": "abc-123"
//	}
//
// 用途：Cypress E2E kernel-restart-restore 测试提交长任务到 kernel Pool，
// 然后调 /api/dev/kill-backend 模拟进程崩溃，验证 Restore 续跑。
//
// 与 /api/kernel/call 的区别：
//   - /api/kernel/call 是同步的（等结果返回），适合短任务调试
//   - /api/kernel/submit 是异步的（立即返回 jobId），适合长任务 + Restore 测试
//
// job 状态查询：GET /api/kernel/pools 看 submitted/finished/failed 计数
// job 取消：POST /api/kernel/lifecycle/stop 会委托 in-flight job 给 Ledger
func (s *Server) handleKernelSubmitGin(c *gin.Context) {
	if !isDevMode() {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "kernel.submit is only available in dev mode",
		})
		return
	}
	if s.kernelPool == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "kernel pool not enabled (set ENCV_USE_KERNEL_POOL=1)",
		})
		return
	}

	var req struct {
		Service string          `json:"service"`
		Method  string          `json:"method"`
		Payload json.RawMessage `json:"payload"`
		JobID   string          `json:"jobId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad request: " + err.Error()})
		return
	}
	if req.Service == "" || req.Method == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "service and method are required"})
		return
	}
	if req.JobID == "" {
		req.JobID = fmt.Sprintf("%s-%d", req.Service, time.Now().UnixNano())
	}

	ctx := kernel.NewContext(c.Request.Context(),
		kernel.WithServiceName("kernel.submit"),
	)
	job := kernel.Job{
		ID:      req.JobID,
		Service: req.Service,
		Method:  req.Method,
		Payload: req.Payload,
	}
	if err := s.kernelPool.Submit(ctx, job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{
		"ok":       true,
		"jobId":    req.JobID,
		"traceID":  ctx.TraceID(),
		"service":  req.Service,
		"method":   req.Method,
	})
}
