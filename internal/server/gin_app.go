package server

import (
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

func NewGinApp(cfg *config.Config) *gin.Engine {
	if cfg.Log.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	r.Use(gin.Logger())
	r.Use(gin.Recovery())

	r.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool {
			// 允许所有 localhost / 127.0.0.1 来源（本地开发/测试）
			if strings.HasPrefix(origin, "http://localhost") ||
				strings.HasPrefix(origin, "http://127.0.0.1") ||
				strings.HasPrefix(origin, "https://localhost") {
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
