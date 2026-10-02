package server

// peerlink_cors_r4_test.go —— P2c Task 2.11（**R4**）回归锁
//
// 契约：CORS allowlist **不得**为 peer 放开 —— 桌面端（浏览器）只允许**同源**访问自己的后端，
// 跨端一律走 Hub（WSS）中继。**任何"给对端地址开 CORS"的改动都会重新引入
// "浏览器直连安卓端"的越权面**，这里把它钉死。
//
// 放行范围（与 gin_app.go 保持一致）：
//   http(s)://localhost / http://127.0.0.1 / https://{name}-plugin.local
// 其余（LAN 地址、公网 peer 域名、capacitor:// 等非 http 来源）一律不放行。

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Soltus/encv-go/internal/peerlink"
	"github.com/gin-gonic/gin"
)

// newPeerlinkRouterWithCORS 构造**带真实 CORS 策略**的路由。
//
// ⚠️ 两个坑（都是假绿来源）：
//  1. 测试辅助 `newPeerlinkRouter()` 是裸 gin（没有 CORS 中间件）⇒ 直接测它等于测"没有 CORS"；
//  2. gin 在**注册路由时**就把中间件链固化了，所以 `r.Use()` 必须在 `registerPeerlinkRoutes()` **之前**，
//     注册之后再 Use 对已有路由无效。
func newPeerlinkRouterWithCORS() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(CorsAllowlistMiddleware())
	s := &Server{peerHub: peerlink.NewHub("test-hub", "test")}
	s.peerConns = peerlink.NewConnRegistry()
	s.peerCalls = peerlink.NewCaller(s.peerConns)
	registerPeerlinkRoutes(s, r)
	return r
}

func TestPeerlink_CORS_AllowlistDoesNotOpenToPeers(t *testing.T) {
	r := newPeerlinkRouterWithCORS()

	cases := []struct {
		name   string
		origin string
		allow  bool
	}{
		{"本地 dev（vite）", "http://localhost:8100", true},
		{"本地回环", "http://127.0.0.1:2025", true},
		{"https localhost", "https://localhost", true},
		{"插件虚拟域名", "https://simverse-plugin.local", true},
		// ↓↓↓ R4 红线：这些都不许放行
		{"LAN 对端（直连安卓的诱惑）", "http://192.168.1.50:2025", false},
		{"公网 peer 域名", "https://peer-abc.cnb.run", false},
		{"任意第三方站点", "https://evil.example", false},
		{"非 http 来源（capacitor）", "capacitor://localhost", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/peerlink/hello", nil)
			req.Header.Set("Origin", c.origin)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			acao := w.Header().Get("Access-Control-Allow-Origin")
			if c.allow {
				if acao != c.origin {
					t.Fatalf("应放行 %q，实际 ACAO=%q", c.origin, acao)
				}
				return
			}
			if acao != "" {
				t.Fatalf("R4 红线：不得为对端/第三方来源放行 CORS（origin=%q 却返回 ACAO=%q）", c.origin, acao)
			}
		})
	}
}

// 预检（OPTIONS）同样不得给对端开绿灯：浏览器只有预检过了才会真正发跨源请求。
func TestPeerlink_CORS_PreflightRejectedForPeerOrigin(t *testing.T) {
	r := newPeerlinkRouterWithCORS()

	req := httptest.NewRequest("OPTIONS", "/api/peerlink/search", nil)
	req.Header.Set("Origin", "http://192.168.1.50:2025")
	req.Header.Set("Access-Control-Request-Method", "GET")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if acao := w.Header().Get("Access-Control-Allow-Origin"); acao != "" {
		t.Fatalf("对端来源的预检不得放行（ACAO=%q）", acao)
	}
	if w.Code != http.StatusNoContent && w.Code != http.StatusOK {
		t.Logf("预检状态码=%d（非放行即视为拒绝）", w.Code)
	}
}
