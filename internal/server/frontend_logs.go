package server

// frontend_logs.go —— 前端（WebView）日志回传**本机**后端（2026-10-05）
//
// 起因：远程调试的 `read_logs` 只能读到 Go 侧日志缓冲，而 DevLogs 的「前端日志」
//   那一栏活在 WebView 的模块内存里 ⇒ 桌面端**完全看不到**安卓端的 JS 日志
//   （扫码失败、渲染异常、console.error 这些恰恰是最需要远程看的）。
//
// 做法：前端把日志**批量**回传到本机后端（127.0.0.1:<后端端口>，回环不占跨端链路），
//   后端存进独立环形缓冲（300 条），字段与 Go 日志对齐并标 source=frontend
//   ⇒ `read_logs` 一次调用就能给出"DevLogs 同款"（后端 + 前端）。
//
// ⚠️ 纪律：
//   1. 只收**批量**（单批上限 100 条 / 单条 1000 字符），不接受无上限的单条刷；
//   2. 不做任何鉴权之外的副作用：写不进去就返回条数，**绝不**因此产生新的业务日志
//      （否则"上报失败 → 记日志 → 再上报"会形成递归）；
//   3. 本机端点与既有 /api/* 同纪律（同源运维请求无鉴权），因为它只在设备回环上可达。

import (
	"net/http"
	"strings"

	"github.com/Soltus/encv-go/internal/logger"
	"github.com/gin-gonic/gin"
)

// frontendLogBuffer 前端日志的环形缓冲（与 Go 日志分开存，避免互相挤掉）。
var frontendLogBuffer = logger.NewRingBuffer(300)

const (
	// frontendLogBatchMax 单批最多接收条数。
	frontendLogBatchMax = 100
	// frontendLogMsgMaxChars 单条 message 的截断长度（回环也别把缓冲撑爆）。
	frontendLogMsgMaxChars = 1000
)

type frontendLogItem struct {
	Level     string `json:"level"`
	Message   string `json:"message"`
	Source    string `json:"source,omitempty"`
	Tags      string `json:"tags,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

// handleAPILogsFrontendGin —— POST /api/logs/frontend（批量上报）
func (s *Server) handleAPILogsFrontendGin(c *gin.Context) {
	var body struct {
		Items []frontendLogItem `json:"items"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json", "detail": err.Error()})
		return
	}
	if len(body.Items) > frontendLogBatchMax {
		body.Items = body.Items[:frontendLogBatchMax]
	}

	n := 0
	for _, it := range body.Items {
		msg := strings.TrimSpace(it.Message)
		if msg == "" {
			continue
		}
		if len([]rune(msg)) > frontendLogMsgMaxChars {
			msg = string([]rune(msg)[:frontendLogMsgMaxChars]) + "…(truncated)"
		}
		level := strings.ToLower(strings.TrimSpace(it.Level))
		if level == "" {
			level = "info"
		}
		// ⚠️ timestamp 用前端给的 HH:MM:SS（与 Go 日志同格式 ⇒ 合并后可按字符串排序）
		entry := map[string]string{
			"timestamp": strings.TrimSpace(it.Timestamp),
			"level":     level,
			"message":   msg,
			"source":    "frontend",
		}
		if v := strings.TrimSpace(it.Source); v != "" {
			entry["origin"] = v // console.info / error-capture / vue-error ...
		}
		if v := strings.TrimSpace(it.Tags); v != "" {
			entry["tags"] = v
		}
		frontendLogBuffer.Push(entry)
		n++
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "received": n})
}
