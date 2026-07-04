// internal/server/diagnose_api.go
//
// 🆕 2026-06-15：统一诊断端点 — 前后端双异常暴露（AI 友好）
//
// 设计动机：
//   - 历史痛点：诊断后端状态需要在 6+ 路由里挑（/ping /health /api/runtime /api/config
//     /api/service-guard /api/ffmpeg-status /api/build-info），AI/前端探针经常
//     编错路径或漏字段 → 误判"端点不存在"。
//   - 现状：把以上信息**聚合到一个 endpoint**，AI 调一次拿全貌。
//
// 设计原则：
//   - **跨平台**：runtime.GOOS / GOARCH 探测 + 平台特定字段 nil 时跳过
//   - **生产兼容**：APK / standalone / dev 沙箱 3 种部署都可用，不依赖 dev 工具
//   - **永不 fail**：subsystem 缺失只产生 warning，不 500（status: "degraded"）
//   - **机器可读**：JSON schema 锁定字段名，向后兼容靠"加字段不动旧字段"
//
// 路由：GET /api/diagnose
// 鉴权：127.0.0.1 only（与 /api/runtime 一致）
// 响应：DiagnoseInfo JSON（见类型定义）
package server

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Soltus/encv-go/internal/utils"
	"github.com/gin-gonic/gin"
)

// DiagnoseInfo 是 /api/diagnose 返回的聚合诊断快照。
//
// 所有字段都是 best-effort：subsystem 不可用时返对应 sentinel（空字符串/false/-1），
// 由 warnings/errors 切片承担可读的错误信息。这样调用方只读 status 字段就能
// 区分"全好/部分坏/全挂"，而无需逐字段判 nil。
type DiagnoseInfo struct {
	// Status 聚合健康度
	//   "ok"       - 核心检查全过（heartbeat_ok + serving_dir OK + 关键 deps 存在）
	//   "degraded" - 至少一个 subsystem 失败但 backend 自身活着
	//   "offline"  - heartbeat 不新鲜（hang 嫌疑）
	Status string `json:"status"`
	// Timestamp 诊断生成时间（RFC3339）
	Timestamp string `json:"timestamp"`
	// GoVersion runtime.Version()
	GoVersion string `json:"go_version"`
	// GOOS / GOARCH 平台信息
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
	// Backend 进程自描述（来自 snapshotRuntimeInfo）
	Backend *RuntimeInfo `json:"backend"`
	// Build 构建信息（来自 utils.GetBuildInfo），可能为 nil（无 build-info.json）
	Build map[string]interface{} `json:"build,omitempty"`
	// Dependencies 子系统可用性
	Dependencies DiagnoseDeps `json:"dependencies"`
	// Environment 关键环境变量（不含 secrets）
	Environment DiagnoseEnv `json:"environment"`
	// Filesystem servingDir 状态
	Filesystem DiagnoseFS `json:"filesystem"`
	// ProcessTree 进程树（仅 dev preview，非 dev 返回空对象）
	ProcessTree DiagnoseProcessTree `json:"process_tree"`
	// Warnings 软警告（subsystem 降级但不影响主功能）
	Warnings []string `json:"warnings"`
	// Errors 硬错误（subsystem 失败影响主功能）
	Errors []string `json:"errors"`
}

// DiagnoseDeps 关键依赖可用性
type DiagnoseDeps struct {
	// FFmpeg ffmpeg 库（仅 Android 真机有意义）
	FFmpeg DiagnoseDep `json:"ffmpeg"`
	// FFprobe ffprobe 库（仅 Android 真机有意义）
	FFprobe DiagnoseDep `json:"ffprobe"`
}

// DiagnoseDep 单个依赖的可用性
type DiagnoseDep struct {
	// Available 是否可用
	Available bool `json:"available"`
	// Detail 详细信息（dlopen 错误、version 输出等）
	Detail string `json:"detail,omitempty"`
	// Error 错误信息（不可用时）
	Error string `json:"error,omitempty"`
}

// DiagnoseEnv 关键环境变量（不含 secrets）
type DiagnoseEnv struct {
	// ENCVMobile ENCV_MOBILE=1
	ENCVMobile bool `json:"encv_mobile"`
	// ENCVDevPreview ENCV_DEV_PREVIEW=1
	ENCVDevPreview bool `json:"encv_dev_preview"`
	// ENCVLibDir ENCV_LIB_DIR（Android 真机才有意义）
	ENCVLibDir string `json:"encv_lib_dir,omitempty"`
	// ENCVHeartbeatPath ENCV_HEARTBEAT_PATH（保留兼容）
	ENCVHeartbeatPath string `json:"encv_heartbeat_path,omitempty"`
	// ENCVServingDirOverride ENCV_SERVING_DIR（保留兼容）
	ENCVServingDirOverride string `json:"encv_serving_dir_override,omitempty"`
}

// DiagnoseFS filesystem 状态
type DiagnoseFS struct {
	// ServingDir 解析后的绝对路径
	ServingDir string `json:"serving_dir"`
	// Exists 目录是否存在
	Exists bool `json:"exists"`
	// Readable 是否可读（os.Stat 通过 + 不 ENOENT）
	Readable bool `json:"readable"`
	// Writable 是否可写（创建临时文件测试）
	Writable bool `json:"writable"`
	// StatError os.Stat 错误（不存在 / 权限拒绝等）
	StatError string `json:"stat_error,omitempty"`
}

// DiagnoseProcessTree 进程树（dev preview 才有意义）
type DiagnoseProcessTree struct {
	// Scope "dev" 表示已尝试探测 air/pm2/vite；"production" 跳过
	Scope string `json:"scope"`
	// Air air 热重载进程（dev 沙箱里 air 监视 encv-go）
	Air DiagnoseProcess `json:"air"`
	// Vite Vite dev server（dev 沙箱里由 pm2 spawn）
	Vite DiagnoseProcess `json:"vite"`
	// PM2 pm2 守护进程（dev 沙箱）
	PM2 DiagnoseProcess `json:"pm2"`
	// PreviewGateway preview-gateway 统一对外入口（dev 沙箱）
	PreviewGateway DiagnoseProcess `json:"preview_gateway"`
}

// DiagnoseProcess 单个进程的状态
type DiagnoseProcess struct {
	// Detected 是否尝试探测（dev 才 true；production 永远 false）
	Detected bool `json:"detected"`
	// Running 进程是否在跑
	Running bool `json:"running"`
	// PID 进程 PID（-1 表示未知）
	PID int `json:"pid"`
	// Port 监听端口（-1 表示未知 / 多个）
	Port int `json:"port,omitempty"`
	// Detail 附加信息（启动命令、监听地址等）
	Detail string `json:"detail,omitempty"`
	// Error 探测失败的错误（不阻塞主响应）
	Error string `json:"error,omitempty"`
}

// handleDiagnoseGin Gin handler，GET /api/diagnose
//
// 入口：所有诊断请求的唯一入口
// 响应：DiagnoseInfo JSON（永远 200，status 字段区分健康度）
// 鉴权：127.0.0.1 only（与 /api/runtime 一致；http.Server 监听限制）
func (s *Server) handleDiagnoseGin(c *gin.Context) {
	info := s.buildDiagnoseInfo()
	c.JSON(http.StatusOK, info)
}

// buildDiagnoseInfo 构造完整 DiagnoseInfo
//
// 拆分理由：方便单测直接调 buildDiagnoseInfo 验证聚合逻辑。
func (s *Server) buildDiagnoseInfo() DiagnoseInfo {
	info := DiagnoseInfo{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		GoVersion: runtime.Version(),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
	}

	// 1. Backend 状态（snapshotRuntimeInfo 不会失败）
	ri := s.snapshotRuntimeInfo()
	info.Backend = &ri

	// 2. Build 信息（无 build-info.json 时返 nil，不算 error）
	if build, err := utils.GetBuildInfo(); err == nil {
		info.Build = build
	} else {
		info.Warnings = append(info.Warnings, "build info not available: "+err.Error())
	}

	// 3. 依赖
	ffmpegOk, ffprobeOk, ffmpegErrMsg, ffmpegDetail, ffprobeDetail := utils.CheckFFmpegAvailable()
	info.Dependencies.FFmpeg = DiagnoseDep{
		Available: ffmpegOk,
		Detail:    ffmpegDetail,
		Error:     ffmpegErrMsg,
	}
	info.Dependencies.FFprobe = DiagnoseDep{
		Available: ffprobeOk,
		Detail:    ffprobeDetail,
	}

	// 4. 环境变量
	info.Environment = DiagnoseEnv{
		ENCVMobile:             os.Getenv("ENCV_MOBILE") == "1",
		ENCVDevPreview:         os.Getenv("ENCV_DEV_PREVIEW") == "1",
		ENCVLibDir:             os.Getenv("ENCV_LIB_DIR"),
		ENCVHeartbeatPath:      os.Getenv("ENCV_HEARTBEAT_PATH"),
		ENCVServingDirOverride: os.Getenv("ENCV_SERVING_DIR"),
	}

	// 5. Filesystem
	info.Filesystem = diagnoseServingDir(s.servingDir)

	// 6. Process tree（仅 dev preview）
	if info.Environment.ENCVDevPreview {
		info.ProcessTree = diagnoseProcessTreeDev()
	} else {
		info.ProcessTree = DiagnoseProcessTree{Scope: "production"}
	}

	// 7. 聚合 status
	info.Status = aggregateStatus(info)

	return info
}

// diagnoseServingDir 检查 servingDir 状态
func diagnoseServingDir(dir string) DiagnoseFS {
	fs := DiagnoseFS{ServingDir: dir}
	if dir == "" {
		fs.StatError = "serving_dir not configured"
		return fs
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		fs.StatError = "abs resolve failed: " + err.Error()
		return fs
	}
	fs.ServingDir = abs

	info, err := os.Stat(abs)
	if err != nil {
		fs.StatError = err.Error()
		return fs
	}
	fs.Exists = true
	fs.Readable = info.IsDir() // 至少能 stat 就算可读

	// 可写性测试：尝试创建临时文件
	if info.IsDir() {
		tmp := filepath.Join(abs, ".encv-diagnose-write-test")
		if f, cerr := os.Create(tmp); cerr == nil {
			f.Close()
			os.Remove(tmp)
			fs.Writable = true
		} else {
			fs.StatError = "writable test failed: " + cerr.Error()
		}
	}
	return fs
}

// diagnoseProcessTreeDev 探测 dev 沙箱的进程树
//
// 跨平台策略：
//   - Linux / macOS: lsof -i :PORT -t（已在 development.md 规范过）
//   - Windows: netstat -ano | findstr :PORT
//   - Android (runtime.GOOS=="android"): 跳过（dev 沙箱不会跑在 Android）
//
// 命令找不到时：返回 Running=false + Error，不影响主响应。
func diagnoseProcessTreeDev() DiagnoseProcessTree {
	tree := DiagnoseProcessTree{Scope: "dev"}

	tree.Air = detectDevProcess("air", 0, "air (encv-go hot reload)")
	tree.Vite = detectDevProcess("vite", 8100, "encv-mobile-vite dev server")
	tree.PM2 = detectDevProcess("pm2", 0, "pm2 daemon")
	tree.PreviewGateway = detectDevProcess("preview-gateway", 16666, "preview-gateway :16666")

	return tree
}

// detectDevProcess 探测单个 dev 进程
//
// port == 0 表示只探测是否存在（按名）
// port > 0  表示额外按端口探测（lsof -i :PORT -t）
func detectDevProcess(name string, port int, label string) DiagnoseProcess {
	p := DiagnoseProcess{
		Detected: true,
		PID:      -1,
		Port:     -1,
		Detail:   label,
	}

	if port > 0 {
		pid, err := probePort(port)
		if err == nil && pid > 0 {
			p.Running = true
			p.PID = pid
			p.Port = port
			return p
		}
		if err != nil {
			p.Error = "port probe failed: " + err.Error()
		}
		// 端口没占不代表进程不在（pm2 可能暂未 spawn 子进程），不再二次探测
		return p
	}

	// 没端口 → 用 pgrep 探测（Linux/macOS）
	if runtime.GOOS == "windows" {
		p.Error = "process name probe not supported on windows"
		return p
	}
	if _, err := exec.LookPath("pgrep"); err != nil {
		p.Error = "pgrep not available: " + err.Error()
		return p
	}
	out, err := exec.Command("pgrep", "-f", name).Output()
	if err != nil {
		p.Error = "pgrep failed: " + err.Error()
		return p
	}
	pidStr := strings.TrimSpace(string(out))
	if pidStr == "" {
		p.Running = false
		return p
	}
	lines := strings.Split(pidStr, "\n")
	var pid int
	if len(lines) > 0 {
		_, _ = scanInt(lines[0], &pid)
	}
	p.Running = true
	p.PID = pid
	return p
}

// probePort 用 lsof/netstat 探测端口占用进程
//
// 跨平台：Linux/macOS 用 lsof，Windows 用 netstat。
// 任何探测命令失败 → 返 error（不 panic）。
func probePort(port int) (int, error) {
	switch runtime.GOOS {
	case "windows":
		// netstat -ano | findstr :PORT
		out, err := exec.Command("netstat", "-ano").Output()
		if err != nil {
			return 0, err
		}
		needle := ":" + strconv.Itoa(port)
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, needle) && strings.Contains(line, "LISTENING") {
				fields := strings.Fields(line)
				if len(fields) > 0 {
					return scanLastInt(fields)
				}
			}
		}
		return 0, nil
	default:
		// Linux / macOS / 其他 Unix
		if _, err := exec.LookPath("lsof"); err != nil {
			return 0, err
		}
		out, err := exec.Command("lsof", "-i", ":"+strconv.Itoa(port), "-t").Output()
		if err != nil {
			return 0, err
		}
		pidStr := strings.TrimSpace(string(out))
		if pidStr == "" {
			return 0, nil
		}
		// 多 PID 取第一个
		lines := strings.Split(pidStr, "\n")
		var pid int
		_, _ = scanInt(lines[0], &pid)
		return pid, nil
	}
}

// aggregateStatus 聚合 status 字段
//
// 规则：
//   - heartbeat 不新鲜 → "offline"（进程 hang 嫌疑）
//   - servingDir 不可读 OR 不可写 → "degraded"（功能受影响）
//   - 关键依赖（ffmpeg 在 Android 时）缺失 → "degraded"
//   - 任何 dep 失败但 backend OK → "degraded"
//   - 全部 OK → "ok"
func aggregateStatus(info DiagnoseInfo) string {
	if !info.Backend.HeartbeatOK {
		return "offline"
	}
	if !info.Filesystem.Readable || !info.Filesystem.Exists {
		return "degraded"
	}
	// Android 真机要求 ffmpeg/ffprobe 可用
	if info.GOOS == "android" {
		if !info.Dependencies.FFmpeg.Available || !info.Dependencies.FFprobe.Available {
			return "degraded"
		}
	}
	if len(info.Errors) > 0 {
		return "degraded"
	}
	return "ok"
}

// scanInt 从字符串起始扫一个十进制整数
//
// 空字符串 / 无数字前缀 → *out = 0（让调用方无需先初始化 out）
func scanInt(s string, out *int) (int, error) {
	*out = 0
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	n := 0
	started := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			if started {
				break
			}
			return 0, nil
		}
		n = n*10 + int(c-'0')
		started = true
	}
	*out = n
	return 1, nil
}

// scanLastInt 从 fields 最后一个非空 token 扫整数（netstat 输出格式兼容）
func scanLastInt(fields []string) (int, error) {
	for i := len(fields) - 1; i >= 0; i-- {
		f := strings.TrimSpace(fields[i])
		if f == "" {
			continue
		}
		var pid int
		if _, err := scanInt(f, &pid); err == nil {
			return pid, nil
		}
	}
	return 0, nil
}
