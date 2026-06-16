// internal/server/runtime_api.go
//
// 🆕 2026-06-14：跨进程 IPC 重构 — Go 主动声明状态
//
// 背景：原方案让 Kotlin (parent) 与 Go (child) 通过
//   - .encv_heartbeat 文件 mtime（共享文件系统依赖）
//   - ENCV_HEARTBEAT_PATH / ENCV_SERVING_DIR env var（双向协商）
//   - config.user.json.mobile.server.dir（双向改写）
// 协调。这违反了 "single source of truth" 原则，是 7s 必死 bug 的根因。
//
// 重构后：Go 自己持有运行时状态，HTTP 端点声明，parent 只读。
//   - GET /api/runtime → JSON 完整状态（pid、port、servingDir、heartbeat）
//   - GET /health      → JSON {status, heartbeat_age_ms, heartbeat_ok}
//
// 参考：spec/cross-process-ipc-refactor/spec.md §3, §4

package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// RuntimeInfo 是 encv-go 进程对外声明的运行时状态。
//
// 设计原则：
//   - 单一来源：本结构体的所有字段由 Go 自己维护，外部只读
//   - 强类型契约：JSON tag 锁字段名，向后兼容靠"加字段不动旧字段"
//   - 进程级自描述：pid / startedAt / version 用于调试；servingDir 用于跨进程对齐
//   - 心跳健康度：heartbeat_ok 由 /health 计算后注入，runtime 端点不重复计算
type RuntimeInfo struct {
	// PID 进程 pid，用于调试
	PID int `json:"pid"`
	// Version 编译时注入的版本字符串（-ldflags "-X main.Version=..."）
	Version string `json:"version"`
	// InstanceID 启动时生成的唯一实例 ID（用于日志关联）
	InstanceID string `json:"instance_id"`
	// ServingDir Go 实际在用的主服务目录（已 Abs 解析）
	ServingDir string `json:"serving_dir"`
	// Port 监听端口（动态分配后回填）
	Port int `json:"port"`
	// StartedAt 启动时间（Unix ms）
	StartedAt int64 `json:"started_at"`
	// Mobile 是否 mobile overlay 模式（ENCV_MOBILE=1）
	Mobile bool `json:"mobile"`
	// ConfigPath 加载的配置路径
	ConfigPath string `json:"config_path"`
	// UptimeMs 进程运行时长（毫秒），调用时计算
	UptimeMs int64 `json:"uptime_ms"`
	// HeartbeatOK 心跳是否新鲜（≤30s），调用时根据 lastHeartbeatMs 计算
	HeartbeatOK bool `json:"heartbeat_ok"`
	// HeartbeatAgeMs 心跳距离现在多久（毫秒）
	HeartbeatAgeMs int64 `json:"heartbeat_age_ms"`
}

// HeartbeatStaleThreshold 心跳陈旧阈值。超过这个时间没更新认为进程 hang。
//
// 历史：旧实现用文件 mtime，Android 共享存储 FAT32/exFAT 精度 2s → 8s 误判。
// 新实现用内存 atomic.Int64，精度纳秒级，30s 阈值与 ws_hub.go 的 pongWait=60s 错开。
const HeartbeatStaleThreshold = 30 * time.Second

// handleRuntimeAPI Gin handler，GET /api/runtime
//
// 调用方：Kotlin EncvGoService（不再用，未来会切）、前端 debug、preview-gateway。
// 响应：RuntimeInfo JSON。
// 安全：127.0.0.1 only 即可（http.Server 监听 127.0.0.1，不暴露公网）。
func (s *Server) handleRuntimeAPI(c *gin.Context) {
	info := s.snapshotRuntimeInfo()
	c.JSON(http.StatusOK, info)
}

// snapshotRuntimeInfo 读取 Server 的运行时状态生成 RuntimeInfo。
//
// 读取 lastHeartbeatMs 用 atomic.LoadInt64（无锁），读取其他字段用
// runtimeInfoMu.RLock（写者极少：Start() 一次）。
func (s *Server) snapshotRuntimeInfo() RuntimeInfo {
	s.runtimeInfoMu.RLock()
	snapshot := s.runtimeInfo
	s.runtimeInfoMu.RUnlock()

	// 实时字段：uptime + heartbeat（snapshot 是 Start() 时的值，这里补充）
	nowMs := time.Now().UnixMilli()
	hbMs := atomic.LoadInt64(&s.lastHeartbeatMs)
	hbAge := nowMs - hbMs
	if hbMs == 0 {
		// 还没启动过 heartbeat loop
		snapshot.HeartbeatAgeMs = -1
		snapshot.HeartbeatOK = false
	} else {
		snapshot.HeartbeatAgeMs = hbAge
		snapshot.HeartbeatOK = hbAge < HeartbeatStaleThreshold.Milliseconds()
	}
	snapshot.UptimeMs = nowMs - snapshot.StartedAt
	return snapshot
}

// startHeartbeatLoopInMemory 启动独立 goroutine，每 2s 写内存心跳。
//
// 🆕 2026-06-14：替代 ffmpeg.StartHeartbeatLoop 的文件版本。
//
// 为什么内存中：
//   - 父进程（Kotlin EncvGoService）通过 HTTP /health JSON 读，无需文件
//   - 避免 mtime 精度（FAT32/exFAT 2s）、路径协商、scoped storage 权限问题
//   - 进程退出 goroutine 自动跟随退出
//
// 为什么 2s：
//   - HeartbeatStaleThreshold = 30s
//   - 2s 更新一次，丢失 14 次才判 hang（裕量足够）
//   - 比 1s 省一半 atomic.Store，但响应足够快
func (s *Server) startHeartbeatLoopInMemory(ctx context.Context) {
	// 首次立即写一次（避免 /api/runtime 首次返回 HeartbeatOK=false）
	atomic.StoreInt64(&s.lastHeartbeatMs, time.Now().UnixMilli())

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Info("Heartbeat loop stopping (ctx done)")
				return
			case <-ticker.C:
				atomic.StoreInt64(&s.lastHeartbeatMs, time.Now().UnixMilli())
			}
		}
	}()
	slog.Info("Heartbeat loop started (in-memory, 2s interval)")
}

// TouchHeartbeat 提供给 ffmpeg worker 等"做完一次工作"的场景主动 touch 心跳。
// 可选调用 — 即使不调用，独立 goroutine 也会每 2s 更新一次。
// 这里保留 hook 便于未来按需扩展（例如 ffmpeg 调用后立即刷新）。
func (s *Server) TouchHeartbeat() {
	atomic.StoreInt64(&s.lastHeartbeatMs, time.Now().UnixMilli())
}

// resolvePID 跨平台取当前进程 pid。
// Unix: os.Getpid() 直接返回。
// Windows: 同上。
func resolvePID() int {
	return os.Getpid()
}
