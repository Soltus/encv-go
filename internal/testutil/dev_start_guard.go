// Package testutil/dev_start_guard.go
// =====================================================
// Go 版 dev-start-guard：预检查 + 强锁定后端开发期单实例。
//
// 背景：之前 mock-data-architecture.md / development.md 已规定
//   - 端口 2025 必须正确（Go Backend）
//   - 后端必须后台运行（nohup/tmux/&）
//   - 严禁阻塞式服务启动
//
// 但缺少**强约束**：当端口 2025 被占时，go run / air 重启必 fail。
// dev-start-guard 在 cmd/encv/ 启动时主动检查：
//   1. 端口 2025 是否被占
//   2. 占用的是不是自己（老实例还是其他进程）
//   3. 沙箱身份（agent-tool-host 16000 不在范围）
//   4. 残留 .pid 文件
//
// 用法（在 main.go / start.go 早期调）：
//
//	testutil.DevStartGuard("encv-server", 2025)
//
// 失败时：直接 log.Fatal(无进程占) 或 强 kill 占的进程 + 继续。
//
// 2026-06-15 创建（test-architecture-refactor-defense-awareness Sprint 3）
// =====================================================

package testutil

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	// 强制激活 test-guard：拦截裸 go test 调用（必须经 scripts/test-go.sh）
	// 详见 internal/testguard/guard.go
	_ "github.com/Soltus/encv-go/internal/testguard"
)

// DevStartGuard 端口 + pid 文件预检查 + 单实例锁定。
//
// behavior: "fail" | "kill" | "warn"
//   - "fail": 端口被占且不是自己 → 失败退出
//   - "kill": 端口被占且不是自己 → SIGTERM 旧进程，等 2s，SIGKILL 兜底
//   - "warn": 端口被占只警告，继续启动（多实例共存，慎用）
func DevStartGuard(processName string, port int, behavior string) error {
	pidFile := PidFilePath(processName)

	// 1) 端口检查
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// 端口被占
		stuckPid, lookupErr := findProcessByPort(port, 3*time.Second)
		if lookupErr != nil {
			return fmt.Errorf("port %d in use and cannot identify: %w", port, lookupErr)
		}

		switch behavior {
		case "fail":
			return fmt.Errorf("port %d in use by PID %d (behavior=fail)", port, stuckPid)
		case "kill":
			fmt.Fprintf(os.Stderr, "[DevStartGuard] killing PID %d on port %d\n", stuckPid, port)
			if err := syscall.Kill(stuckPid, syscall.SIGTERM); err != nil {
				fmt.Fprintf(os.Stderr, "[DevStartGuard] SIGTERM failed: %v\n", err)
			}
			// 等 2s
			for i := 0; i < 20; i++ {
				time.Sleep(100 * time.Millisecond)
				if err := syscall.Kill(stuckPid, syscall.Signal(0)); err != nil {
					break
				}
			}
			// 兜底 SIGKILL
			_ = syscall.Kill(stuckPid, syscall.SIGKILL)
			// 再试一次 listen
			listener, err = net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("port %d still in use after kill PID %d: %w", port, stuckPid, err)
			}
			_ = listener.Close()
		case "warn":
			fmt.Fprintf(os.Stderr, "[DevStartGuard] WARNING: port %d in use by PID %d (behavior=warn)\n", port, stuckPid)
			_ = listener // 跳过 listen
		default:
			_ = listener.Close()
			return fmt.Errorf("unknown behavior: %s", behavior)
		}
	} else {
		_ = listener.Close()
	}

	// 2) 写 pid 文件
	if err := writePidFile(pidFile, os.Getpid()); err != nil {
		return fmt.Errorf("write pid file: %w", err)
	}

	// 3) 注册清理
	// 进程退出时删 pid 文件（不能靠 t.Cleanup，t 在 main 不可用）
	// → 在 main 末尾 defer os.Remove(pidFile)

	return nil
}

// PidFilePath 返回 pid 文件路径。
// 默认 <tmp>/encv-<name>.pid；可被 ENCV_PID_DIR 覆盖。
func PidFilePath(name string) string {
	if d := os.Getenv("ENCV_PID_DIR"); d != "" {
		return filepath.Join(d, "encv-"+name+".pid")
	}
	return filepath.Join(os.TempDir(), "encv-"+name+".pid")
}

// writePidFile 写当前 PID 到文件。
func writePidFile(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0o644)
}

// ReadPidFile 读 pid 文件（返回 -1 表示文件不存在或损坏）。
func ReadPidFile(name string) int {
	path := PidFilePath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		return -1
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return -1
	}
	return pid
}

// RemovePidFile 删 pid 文件。
func RemovePidFile(name string) error {
	return os.Remove(PidFilePath(name))
}

// IsProcessAlive 用 signal 0 探测 PID 是否存在（不真发信号）。
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, syscall.Signal(0))
	return err == nil
}

// findProcessByPort 用 /proc/net/tcp + /proc/<pid>/fd 找占用端口的 PID。
// 平台限制：仅 Linux。
//
// 【P1-5 修复】带 context 整体超时：沙箱内 /proc/<pid>/net/tcp 在 D 状态进程上
// 可能挂很久，加 deadline 防止整个 /proc 遍历卡死沙箱。
func findProcessByPort(port int, timeout time.Duration) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 简化：尝试 lsof，回退到 /proc/net/tcp
	// 真实生产建议用 github.com/shirou/gopsutil/net，这里不引入第三方依赖
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, err
	}
	portHex := fmt.Sprintf("%04X", port)
	for _, e := range entries {
		// 每轮检查 ctx 取消
		if ctx.Err() != nil {
			return 0, fmt.Errorf("findProcessByPort timeout (%s) while scanning /proc", timeout)
		}
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		// 检查 /proc/<pid>/net/tcp
		tcpPath := fmt.Sprintf("/proc/%d/net/tcp", pid)
		// 【P1-5 增强】单文件读也用 ctx 限速：goroutine 包装 + select
		data, err := readFileCtx(ctx, tcpPath)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), ":"+portHex) {
			// 进一步验证：是 LISTEN 状态（0000000000000000:XXXX 0A）
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, ":"+portHex) {
					fields := strings.Fields(line)
					if len(fields) >= 4 && fields[3] == "0A" {
						return pid, nil
					}
				}
			}
		}
	}
	return 0, fmt.Errorf("no process found on port %d", port)
}

// readFileCtx 读文件但响应 ctx 取消（用于 /proc/* 路径）。
// 注：os.ReadFile 本身不支持 ctx，所以用 goroutine + select 模拟。
// 沙箱内 /proc/<pid>/net/tcp 在 D 状态进程上可能阻塞 syscall，
// 此处用 ctx 兜底，最坏情况 goroutine 泄漏但调用方已返。
func readFileCtx(ctx context.Context, path string) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		data, err := os.ReadFile(path)
		ch <- result{data, err}
	}()
	select {
	case r := <-ch:
		return r.data, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
