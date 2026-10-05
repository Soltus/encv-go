package server

// stub_provider.go —— 模拟服务商挂载（vNext Round 4）
//
// 目的：让演示不再是"假剧本"。
//
// mock 剧本连工具结果都是写死的；模拟服务商只替代"模型大脑"——
// 说什么、决定调哪个工具由脚本给，但工具调用是真的、结果来自真实执行、
// SSE / 投影 / 前端渲染全是真的。
//
// 用法（与"用户自填服务商"完全同一条路径）：
//   AI 设置里填 openai_base_url = http://127.0.0.1:<port>
//                 openai_model   = stub:fs_overview
//   然后照常聊天 ⇒ 走的是真实 OpenAI 兼容链路 + 真实工具执行。
//
// 启用必须显式 ENCV_STUB_PROVIDER=1，且 /api/runtime 会对外声明
// stub_provider_enabled=true —— 替身大脑不允许悄悄冒充真实模型。

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

// stubProviderEnabled 报告是否启用模拟服务商。
//
// 只认显式 "1"：拼错 / 未设置一律关闭（fail closed）。
func (s *Server) StubProviderEnabled() bool {
	return strings.TrimSpace(os.Getenv("ENCV_STUB_PROVIDER")) == "1"
}

// registerStubProviderRoutes 挂载模拟服务商路由。
//
// 同时挂在两处：
//   - /v1/chat/completions                 —— 让 agent 配置的 base_url 直接填
//     http://127.0.0.1:<port> 即可（它就是"用户配置的服务商"）
//   - /api/stub-provider/v1/chat/completions —— 显式前缀，便于排查与文档引用
func (s *Server) registerStubProviderRoutes(r *gin.Engine) {
	if !s.StubProviderEnabled() {
		return
	}
	h := gin.WrapH(s.stubProvider.Handler())
	r.POST("/v1/chat/completions", h)
	r.POST("/api/stub-provider/v1/chat/completions", h)
	r.GET("/api/stub-provider/scripts", s.handleStubProviderScripts)
	slog.Warn("stub provider ENABLED — 模型大脑是替身，工具调用与结果仍是真实执行",
		"scripts", s.stubProvider.Scripts())
}

// handleStubProviderScripts 列出可用替身脚本与已处理请求数（便于确认"真的在跑"）。
func (s *Server) handleStubProviderScripts(c *gin.Context) {
	if !s.StubProviderEnabled() {
		c.JSON(http.StatusNotFound, gin.H{"error": "stub_provider_disabled"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"scripts":  s.stubProvider.Scripts(),
		"requests": s.stubProvider.RequestCount(),
		"note":     "替身只提供'说什么/调哪个工具'；工具结果是真实执行产生的",
	})
}

var _ = http.StatusOK
