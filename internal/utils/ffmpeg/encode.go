// Package ffmpeg 是 encv-go 调 ffmpeg/ffprobe 的统一入口。
//
// 🆕 2026-06-15 重构（拆 Runner 抽象层）：
//   - 旧设计：Runner interface + global SetRunner + 各平台 Runner 实现（Worker / Native / Exec）
//   - 新设计：包级公开函数 Encode / Probe / IsAvailable
//     - 平台差异用 build tag 隔离（internal: encode_android.go / probe_android.go / available_android.go）
//     - 不再有全局可变状态，编译时决定路径选择
//   - Encode：跑 ffmpeg 转码
//     - 沙箱（!android）：os.Exec("/usr/bin/ffmpeg", args...) 直调
//     - 真机（android）：spawn worker (cmd/ffmpeg-worker/ffmpeg_worker.c) 子进程 + JSON-RPC
//   - Probe：跑 ffprobe 提取 metadata
//     - 沙箱：os.Exec("/usr/bin/ffprobe", args...) 直调
//     - 真机：utils.CallFFprobeNative in-process cgo（不走 worker）
//   - IsAvailable：探活
//     - 沙箱：检查 /usr/bin/ffmpeg + /usr/bin/ffprobe
//     - 真机：utils.CheckFFmpegAvailable in-process cgo dlopen 探活
//
// 历史：
//   - 2026-06-11 Phase 1: WorkerRunner 解决真机 cgo hang
//   - 2026-06-11 Phase 2: WorkerRunner 选为首选
//   - 2026-06-15: 重构为 Encode/Probe/IsAvailable 三函数 + 删 Runner 抽象 + 删旧 Go worker
//   - 原 runner.go / worker_runner.go / worker_client.go / exec_runner.go / native_runner.go 全部删除
//   - 原 cmd/ffmpeg-worker/main_android.go / main_exec.go（旧 Go worker）替换为 ffmpeg_worker.c
package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/utils"
)
// EncodeResult 是 ffmpeg/ffprobe 调用的标准结果。
type EncodeResult struct {
	Stdout     []byte
	Stderr     string
	ExitCode   int
	DurationMs int64
	// Error 是 worker 响应 JSON 里的 "error" 字段（仅失败时填）。
	// 调用方（如 mock_generator.go）拿不到 worker 进程内部错误时，可从这里取。
	// 之前漏掉这个字段 → mock_generator 拼装 stderr 时 res.Error 编译不过。
	Error string
}

// workerRequest/Response 与 cmd/ffmpeg-worker/ffmpeg_worker.c 的 JSON 协议对齐。
// 协议细节：worker 读 stdin JSON，argv[0] 由 worker 内部 prepend "ffmpeg"，
// 写 stdout JSON {"exit_code":N,"stdout":"...","stderr":"...","duration_ms":M,"error":"..."}。
type workerRequest struct {
	Args      []string `json:"args"`
	FFmpegBin string   `json:"ffmpeg_bin,omitempty"`
	LibDir    string   `json:"lib_dir,omitempty"`
	// 🆕 2026-06-16：tmp_dir 告诉 worker 写 stdout/stderr 重定向文件的位置
	//   旧实现 worker 硬编码 /tmp/ffmpeg_stdout_XXXXXX → Android 上 /tmp/ 不可写 → EACCES
	//   现在 Go 父进程传 os.TempDir()（gomobile 注入为 context.cacheDir）→ 真机可写
	//   worker fallback 链：JSON tmp_dir > $TMPDIR env > /data/local/tmp/ > 已知 cacheDir
	TmpDir    string   `json:"tmp_dir,omitempty"`
	TimeoutMs int      `json:"timeout_ms,omitempty"`
	Mode      string   `json:"mode,omitempty"` // "ffmpeg"（默认） | "ffprobe"
}

type workerResponse struct {
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// locateWorker 找 ffmpeg-worker binary 路径。
//
// 优先级：
//  1. $ENCV_FFMPEG_WORKER（绝对路径或 PATH basename）
//  2. $ENCV_LIB_DIR/libffmpeg-worker.so 或 ffmpeg-worker（真机 nativeLibraryDir）
//  3. $ENCV_BIN_DIR/ffmpeg-worker
//  4. <exe-dir>/ffmpeg-worker
//  5. <exe-dir>/../../cmd/ffmpeg-worker/ffmpeg-worker（dev mode）
//  6. exec.LookPath("ffmpeg-worker")
//
// 真机：Kotlin EncvGoService.kt 设 ENCV_FFMPEG_WORKER = nativeLibraryDir + "/libffmpeg-worker.so"，
// 所以 (2) 命中。
func locateWorker() (string, error) {
	candidates := []string{}
	if v := os.Getenv("ENCV_FFMPEG_WORKER"); v != "" {
		candidates = append(candidates, v)
	}
	if dir := os.Getenv("ENCV_LIB_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "libffmpeg-worker.so"))
		candidates = append(candidates, filepath.Join(dir, "ffmpeg-worker"))
	}
	if dir := os.Getenv("ENCV_BIN_DIR"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "ffmpeg-worker"))
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, "ffmpeg-worker"))
		candidates = append(candidates, filepath.Join(exeDir, "..", "..", "cmd", "ffmpeg-worker", "ffmpeg-worker"))
	}
	if path, err := exec.LookPath("ffmpeg-worker"); err == nil {
		candidates = append(candidates, path)
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if !strings.Contains(c, string(os.PathSeparator)) {
			if full, err := exec.LookPath(c); err == nil {
				return full, nil
			}
			continue
		}
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs, nil
		}
	}
	return "", errors.New("ffmpeg-worker binary not found (set ENCV_FFMPEG_WORKER or build it: `go build -o /usr/local/bin/ffmpeg-worker ./cmd/ffmpeg-worker`)")
}

// runWorkerJSON 通过 worker binary 跑 args（mode="ffmpeg" 走 ffmpeg_run，mode="ffprobe" 走 ffprobe_run）。
// ctx cancel + 硬 timer SIGKILL 兜底（防 cgo 阻塞父进程）。
//
// 🆕 2026-06-15：替代旧 workerClient.RunWithOutput（go 文件已删除）；协议与 ffmpeg_worker.c 同步。
func runWorkerJSON(ctx context.Context, workerBin string, mode string, args []string) (*EncodeResult, error) {
	timeoutMs := 0
	if deadline, ok := ctx.Deadline(); ok {
		timeoutMs = int(time.Until(deadline).Milliseconds())
		if timeoutMs < 0 {
			timeoutMs = 0
		}
	}

	// 🆕 2026-06-15 修 #3：libDir 在 JSON 请求和 cmd.Env 注入都要用——
	// C worker 从 JSON 读 lib_dir（不是从 env），所以这里必须给到非空值，
	// 不能直接用 os.Getenv（真机 Android Java 端不保证注入 env 到 Go 进程）。
	// 修法：libDir 优先 os.Getenv("ENCV_LIB_DIR")，空时兜底 utils.GetLibDir()（包级缓存）。
	libDir := os.Getenv("ENCV_LIB_DIR")
	if libDir == "" {
		libDir = utils.GetLibDir() // utils 包公开 getLibDir 兜底
	}

	req := workerRequest{
		Args:      args,
		FFmpegBin: locateFFmpegSystem(),
		// 🆕 2026-06-15 修 #3：JSON lib_dir 用上面算好的 libDir（兜底后非空）
		LibDir:    libDir,
		// 🆕 2026-06-16：传 os.TempDir() 给 worker（gomobile 注入为 context.cacheDir）
		//   Worker 优先用 JSON tmp_dir 写 stdout/stderr 重定向文件（不再硬编码 /tmp/）
		TmpDir:    os.TempDir(),
		TimeoutMs: timeoutMs,
		Mode:      mode,
	}
	reqBytes, _ := json.Marshal(req)

	cmd := exec.CommandContext(ctx, workerBin)
	cmd.Stdin = bytes.NewReader(reqBytes)
	var stdoutBuf, stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// 🆕 2026-06-15 修 #3：显式注入 ENCV_LIB_DIR 给 worker subprocess（双保险）
	// 理由：之前旧 worker_client.go 是显式 cmd.Env = append(os.Environ(), "ENCV_LIB_DIR="+getLibDir()) 注入。
	// 重构成 Encode(ctx, args) 后丢了这一行 → 父进程 env 空时 worker dlopen 系统路径 /libffmpeg.so 失败 → exit_code -1。
	// 修法：同上，libDir 用上面兜底后的非空值，强制注入到 cmd.Env。
	cmd.Env = append(os.Environ(),
		"ENCV_LIB_DIR="+libDir,
		// 🆕 2026-06-16：显式注入 TMPDIR=os.TempDir()，让 worker 即便没读到 JSON tmp_dir
		//   也能用 env 兜底（gomobile 启动 Go 进程时通常已设 TMPDIR，这里再双保险）
		"TMPDIR="+os.TempDir(),
	)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start worker: %w", err)
	}

	// 硬 timer SIGKILL 兜底（防 ctx cancel 失败时 cgo 阻塞父进程）
	var hardTimer *time.Timer
	if timeoutMs > 0 {
		hardTimer = time.AfterFunc(time.Duration(timeoutMs+500)*time.Millisecond, func() {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
		})
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var runErr error
	select {
	case runErr = <-done:
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		runErr = ctx.Err()
	}
	if hardTimer != nil {
		hardTimer.Stop()
	}

	// 解析 worker stdout（最后一行 JSON）
	stdout := stdoutBuf.String()
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
		if stdout == "" {
			return &EncodeResult{Stderr: stderrBuf.String(), ExitCode: exitCode}, runErr
		}
	}

	var resp workerResponse
	for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if err := json.Unmarshal([]byte(line), &resp); err == nil {
			break
		}
	}

	finalErr := runErr
	if finalErr == nil && resp.Error != "" {
		finalErr = fmt.Errorf("ffmpeg worker reported: %s", resp.Error)
	}

	return &EncodeResult{
		Stdout:     []byte(resp.Stdout),
		Stderr:     resp.Stderr + stderrBuf.String(),
		ExitCode:   resp.ExitCode,
		DurationMs: resp.DurationMs,
		Error:      resp.Error,
	}, finalErr
}

// locateFFmpegSystem 找系统 ffmpeg binary（沙箱走）。
// 真机返回 ""（不参与决策，worker 自己会用 lib_dir）。
func locateFFmpegSystem() string {
	if v := os.Getenv("ENCV_FFMPEG_BIN"); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v
		}
	}
	if dir := os.Getenv("ENCV_BIN_DIR"); dir != "" {
		p := filepath.Join(dir, "ffmpeg")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if path, err := exec.LookPath("ffmpeg"); err == nil {
		return path
	}
	return ""
}

// DetectVideoFormat 检测视频文件的容器格式（mp4 / mkv / 其他）。
//
// 内部用 ffmpeg.Probe（沙箱调系统 ffprobe binary，真机 in-process cgo 调 libffprobe.so）。
// 短时调用（< 100ms），用 background ctx 即可。
func DetectVideoFormat(filePath string) (string, error) {
	output, err := Probe(context.Background(),
		"-v", "error",
		"-show_entries", "format=format_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)
	if err != nil {
		return "", fmt.Errorf("ffprobe failed: %w", err)
	}

	formatName := strings.TrimSpace(string(output))
	if formatName == "" {
		return "", fmt.Errorf("could not determine container format")
	}

	switch {
	case strings.Contains(formatName, "matroska"):
		return "mkv", nil
	case strings.Contains(formatName, "mp4"):
		return "mp4", nil
	default:
		parts := strings.Split(formatName, ",")
		return strings.ToLower(parts[0]), nil
	}
}
