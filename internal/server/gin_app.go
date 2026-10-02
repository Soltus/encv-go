package server

import (
	"fmt"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// =============================================================================
// CORS 策略
// =============================================================================
//
// 关键修复（ai-routing-cors-preflight-fix）：
//   1. AllowHeaders 改为通配 "*"
//      原先只列了 "Origin, Content-Type, Accept, Authorization, X-Forwarded-*"
//      但前端 useAgent.send() / sendConfirm / sendResume 都会带
//      "X-Agent-Protocol: agui" 自定义 header（AG-UI 协议协商）
//      → 浏览器 OPTIONS 预检失败 → POST 被拦截 → "Failed to fetch"
//   2. AllowOrigins 改为显式 allowlist（替换 AllowAllOrigins: true）
//      避免 AllowAllOrigins + AllowCredentials 组合的浏览器安全警告
//      覆盖：Capacitor WebView origin (https://localhost)
//           本地开发 origin (http://localhost, http://127.0.0.1:2025 等)
//   3. AllowCredentials 改为 false
//      本 app ↔ 本地 server 不需要 cookie / 认证头；去掉以彻底消除
//      "Access-Control-Allow-Origin: * + Allow-Credentials: true" 非法组合
//
// 安全模型：encv-go 跑在用户本机 / LAN，仅本机 / 内网访问，通配 header
//          + 固定 origin allowlist 不会引入实质风险。

// sanitizedLogFormatter R14：访问日志**脱敏** —— 命中敏感路径/敏感参数时
// 丢弃查询串（只留路径），其余走 gin 默认格式。
//
// 被丢弃的内容包括：`/api/peerlink/ws?token=…`（**令牌**）、
// `search?q=…`（查询词）、`file?path=…`（远端路径全文）。
func sanitizedLogFormatter(param gin.LogFormatterParams) string {
	// ⚠️ 巨坑（实测）：gin 在 `SkipQueryString=false`（默认）时已把查询串
	//    拼进 `param.Path` 了 ⇒ 必须用 `param.Request.URL.Path` 重新取纯路径，
	//    否则"丢弃查询串"根本不生效（2026-10-02 排查）。
	if param.Request == nil {
		return fmt.Sprintf("[GIN] %v | %3d | %s\n", param.TimeStamp, param.StatusCode, param.Path)
	}
	path := param.Request.URL.Path
	if param.Request.URL.RawQuery != "" && !shouldRedactQuery(path, param.Request.URL.RawQuery) {
		path = path + "?" + param.Request.URL.RawQuery
	}
	var statusColor, methodColor, resetColor string
	if param.IsOutputColor() {
		statusColor = param.StatusCodeColor()
		methodColor = param.MethodColor()
		resetColor = param.ResetColor()
	}
	if param.Latency > time.Minute {
		param.Latency = param.Latency.Truncate(time.Second)
	}
	return fmt.Sprintf("[GIN] %v |%s %3d %s| %13v | %15s |%s %-7s %s %s\n%s",
		param.TimeStamp.Format("2006/01/02 - 15:04:05"),
		statusColor, param.StatusCode, resetColor,
		param.Latency,
		param.ClientIP,
		methodColor, param.Method, resetColor,
		path,
		param.ErrorMessage,
	)
}

// shouldRedactQuery 判定是否需要在日志里丢弃查询串。
func shouldRedactQuery(path, query string) bool {
	if strings.HasPrefix(path, "/api/peerlink/") {
		return true
	}
	// 其它路径：只要出现凭证类参数也丢弃
	for _, k := range []string{"token=", "psk=", "pairingId=", "secret="} {
		if strings.Contains(query, k) {
			return true
		}
	}
	return false
}

func NewGinApp(cfg *config.Config) *gin.Engine {
	if cfg.Log.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// ⚠️ 不用裸 gin.Logger()：它把**查询串**一起写进访问日志。
	//    peerlink 的查询串里承载 token（/ws?token=）、查询词（search?q=）、
	//    远端路径（file?path=）⇒ 裸日志 = 密钥与隐私外泄（R14/R5）。
	r.Use(gin.LoggerWithConfig(gin.LoggerConfig{Formatter: sanitizedLogFormatter}))
	r.Use(gin.Recovery())

	r.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool {
			// 允许所有 localhost / 127.0.0.1 来源（本地开发/测试）
			if strings.HasPrefix(origin, "http://localhost") ||
				strings.HasPrefix(origin, "http://127.0.0.1") ||
				strings.HasPrefix(origin, "https://localhost") {
				return true
			}
			// 允许 ComboLite 插件的虚拟 https 域名（plugin-simverse, plugin-mpv-player 等）
			// 格式：https://{name}-plugin.local
			if strings.HasSuffix(origin, "-plugin.local") &&
				strings.HasPrefix(origin, "https://") {
				return true
			}
			return false
		},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS", "HEAD", "PATCH"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"Content-Length", "X-Mock-Mode", "X-Mock-Scenario", "X-Agent-Protocol"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	r.Use(ConfigMiddleware(cfg))

	return r
}

func ConfigMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := config.NewContext(c.Request.Context(), cfg)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
