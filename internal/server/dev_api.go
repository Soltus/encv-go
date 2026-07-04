// internal/server/dev_api.go — dev-only HTTP 端点
//
// 2026-07-03 新增（spec android-workmanager-split-start-stop Phase 1.6.2）
//
// 端点：
//   - POST /api/dev/kill-backend : 触发进程自杀（pm2 / systemd / EncvGoService 会自动重启）
//
// 设计：
//   - 仅 dev 模式开放（生产模式返回 403）
//   - 用途：Cypress E2E kernel-restart-restore 测试中模拟进程崩溃
//     → pm2 检测到进程退出 → 自动重启 → NewServer 重新构造 → kernelLifecycle.Start + Restore
//     → Cypress 验证 Stop 委托的 job 在新进程被 Restore 续跑
//   - 自杀前先 flush 一条日志 + HTTP 响应（让 Cypress 收到 200 后再开始等重启）
//
// 安全：
//   - 非 dev 模式返回 403（避免生产环境被恶意触发 DoS）
//   - 响应 200 后异步自杀（给 gin 时间把响应发出去）
package server

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// handleKillBackendGin 触发进程自杀（dev only）。
//
// 响应：
//
//	{
//	  "ok": true,
//	  "message": "backend will exit in 500ms (pm2 should restart)"
//	}
//
// 行为：
//  1. 返回 200 + JSON 响应
//  2. 500ms 后调 os.Exit(1)（让 gin 把响应发出去）
//  3. pm2 检测到进程退出 → 自动重启 → NewServer 重新构造 → kernelLifecycle.Start + Restore
//
// Cypress E2E 用法：
//
//	cy.request('POST', '/api/dev/kill-backend').then(() => {
//	  // 等 pm2 重启 + /health 探活
//	  cy.waitForBackendHealth('/health', 10000)
// })
func (s *Server) handleKillBackendGin(c *gin.Context) {
	if !isDevMode() {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "kill-backend is only available in dev mode",
		})
		return
	}

	// 先把 kernel lifecycle 停掉（委托 in-flight job 给 Ledger）
	// 这样新进程 Restore 时能拿到完整的 pending job 列表
	if s.kernelLifecycle != nil {
		_ = s.kernelLifecycle.Stop(0)
		fmt.Fprintln(os.Stderr, "[dev] kernel lifecycle stopped before kill-backend")
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "backend will exit in 500ms (pm2 should restart)",
		"pid":     os.Getpid(),
	})

	// 异步自杀（给 gin 时间把响应发出去）
	go func() {
		time.Sleep(500 * time.Millisecond)
		fmt.Fprintln(os.Stderr, "[dev] kill-backend: exiting process now")
		os.Exit(1)
	}()
}
