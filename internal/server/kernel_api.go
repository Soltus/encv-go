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
	"encoding/json"
	"net/http"
	"os"

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
