// internal/server/server.go
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Soltus/encv-go/internal/auth"
	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/mount"
	"github.com/Soltus/encv-go/internal/mount/drivers"
	"github.com/Soltus/encv-go/internal/openlist"
	"github.com/Soltus/encv-go/internal/register"
	mobileservice "github.com/Soltus/encv-go/internal/service"
	"github.com/Soltus/encv-go/internal/search"
	"github.com/Soltus/encv-go/internal/tools"
	"github.com/Soltus/encv-go/internal/utils"
	"github.com/Soltus/encv-go/internal/v2/container/detector"
	"github.com/Soltus/encv-go/internal/v2/handler"
	"github.com/Soltus/encv-go/internal/v2/namer"
	"github.com/Soltus/encv-go/internal/v2/plugins"
	"github.com/Soltus/encv-go/internal/v2/service"
	"github.com/Soltus/encv-go/internal/webdav"
	"github.com/dustin/go-humanize"
	"github.com/gin-gonic/gin"
)

type Server struct {
	server         *http.Server
	cfg            *config.Config
	configPath     string
	configMu       sync.Mutex
	// 🆕 2026-06-11 修复：mock generate 并发 race
	// 多 goroutine 同时写同一文件（os.WriteFile 非原子）→ 部分覆盖 + count 不稳定
	// 加全局互斥串行化（dev tool，低频，代价可接受）
	mockGenMu      sync.Mutex
	servingDir     string
	version        string
	instanceID     string
	actualPort     int
	webdavDir      string
	webdavPath     string
	readerService  *service.ReaderService
	mobileSvc      *mobileservice.MobileService
	contentHandler *handler.ContentHandler
	chunkNamers    []namer.ChunkNamer
	jwtManager     *auth.JWTManager
	webdavFS       webdav.IndexProvider
	// 🆕 2026-06-17：多挂载点 webdav 实例表（multi-mount-storage-refactor spec 续）
	// 启动期按 mount registry 填充；key = mount.Name（primary / automation / sandbox 等）
	webdavFSByMount map[string]*webdavFSEntry
	mockEngine     *MockEngine
	// scenarioLoader 是剧本外置 spec 引入的加载器。
	// 若 agent_settings.mock_scenarios_dir 非空，
	// NewServer 会创建 loader + 加载 YAML 覆盖 builtin 剧本。
	// 热重载（-mock-scenarios-hot-reload）由 Start() 启动 goroutine。
	scenarioLoader *ScenarioLoader
	scenariosDir   string
	// toolDeps 是 tools 包的依赖注入（v2 工具注册表使用）。
	// Server 启动时构造一次，executeAgentTool 路径下注入到 tools.GlobalRegistry.Dispatch。
	toolDeps *tools.ToolDeps
	// 🆕 2026-06-15：多挂载点注册表（multi-mount-storage-refactor spec）
	// 启动时构造，与 cfg.servingDir / cfg.isMobile / cfg.isDev 一起决定默认 mount
	mountRegistry *mount.MountRegistry
	// 🆕 2026-06-16：mount 启动期错误（不再静默 — /api/mounts 响应里暴露给前端）
	// 典型来源：migrateLegacyDataPath / Load / BootstrapFromConfig / Save 失败
	// 历史：fmt.Fprintf(os.Stderr, ...) → 只进 stderr → DevLogs 看不到 → 用户不知道 mount 为何失败
	mountBootstrapErrors []string
	// 🆕 2026-06-14：跨进程 IPC 重构 — RuntimeInfo 内存字段
	//
	// 单一来源：Go 自己持有，HTTP /api/runtime 对外声明。
	// parent (Kotlin EncvGoService) 不再写文件、不再 set env、不再改 config。
	// 见 internal/server/runtime_api.go
	runtimeInfo   RuntimeInfo
	runtimeInfoMu sync.RWMutex
	// lastHeartbeatMs 最后心跳时间（Unix ms），独立 goroutine 每 2s 写一次。
	// atomic.LoadInt64 无锁读，atomic.StoreInt64 写。
	// HeartbeatStaleThreshold = 30s，见 runtime_api.go
	lastHeartbeatMs int64
	// 🆕 2026-07-03：向量搜索服务（Turso 原生向量检索 + 中文 bigram 分词）
	searchSvc *vectorsearch.SearchService
	// 🆕 2026-07-02：数据库引擎降级真实原因（修复 handleDatabaseInfo 硬编码"当前平台不支持"的问题）
	//
	// 历史：InitDatabase 通过 slog.Error 记录错误，但没保存到 Server，
	//       handleDatabaseInfo 只能硬编码"当前平台不支持 Turso/LibSQL 引擎"，
	//       误导用户和调试 — 真实原因可能是 C 库加载失败、PRAGMA 失败、schema 失败、panic 等。
	//
	// 修复：InitDatabase 在降级时把真实失败原因写入此字段，handleDatabaseInfo 优先使用它。
	// 参见 .trae/rules/graceful-degradation.md L2 降级规范："降级原因明确"。
	dbFallbackReason string
}

// mountRegistryDataPath 返回 mounts.json 的持久化路径。
//
// 平台 + 环境矩阵（2026-06-15 重设计）：
//
//	平台    环境              路径
//	─────  ───────────       ──────────────────────────────────────────
//	Android 任意              <ENCV_APP_FILES_DIR>/.encv/mounts.json
//	                          （默认 fallback: /data/user/0/com.encvgo.app/files/.encv/mounts.json）
//	Linux   dev (ENCV_DEV=1)  $XDG_DATA_HOME/encv-dev/mounts.json
//	                          （默认: $HOME/.local/share/encv-dev/mounts.json）
//	Linux   production        $XDG_DATA_HOME/encv/mounts.json
//	macOS   dev               $HOME/Library/Application Support/encv-dev/mounts.json
//	macOS   production        $HOME/Library/Application Support/encv/mounts.json
//	Windows dev (ENCV_DEV=1)  %LOCALAPPDATA%\encv-dev\mounts.json
//	Windows production        %LOCALAPPDATA%\encv\mounts.json
//
// 设计原则（2026-06-15 用户反馈"放应用 data 路径，不要 /storage/emulated/0"）：
//  1. Android 必须放 app 私有目录（其他 app 看不到，符合 Android scoped storage）
//     — /storage/emulated/0 是公共存储根，会污染用户媒体视图
//  2. 桌面端遵循 XDG / Windows 标准，避免污染用户工作目录
//  3. dev / production 用不同 sub 目录，避免 dev 配置覆盖 production 数据
//  4. 全局 .encv 隐藏子目录（多一层），dotfile filter 进一步保护
//  5. 优先级：ENCV_MOUNTS_FILE（明确指定） > 默认值
//
// 兼容性：
//   - 老路径 `<servingDir>/.encv/mounts.json`（dev 沙箱老用法）由 migrateLegacyDataPath
//     自动迁移到新位置
//   - 老路径 `<servingDir>/mounts.json`（v1 之前散落根目录）由 migrateLegacyDataPath
//     归档到 `<servingDir>/.encv/mounts.json.migrated-<unix>`（不再跟用户媒体混）
func mountRegistryDataPath(cfg *config.Config) string {
	if v := os.Getenv("ENCV_MOUNTS_FILE"); v != "" {
		return v
	}
	return defaultMountRegistryDataPath()
}

// defaultMountRegistryDataPath 平台/env 默认路径实现。
//
// 测试覆盖：
//   - TestMountRegistryDataPath_Android
//   - TestMountRegistryDataPath_LinuxDev
//   - TestMountRegistryDataPath_LinuxProd
//   - TestMountRegistryDataPath_WindowsDev
//   - TestMountRegistryDataPath_WindowsProd
//   - TestMountRegistryDataPath_MacOS
func defaultMountRegistryDataPath() string {
	isAndroid := os.Getenv("ENCV_MOBILE") == "1"
	if isAndroid {
		return androidMountRegistryPath()
	}
	isDev := os.Getenv("ENCV_DEV") == "1" || os.Getenv("ENCV_DEV_PREVIEW") == "1"
	return desktopMountRegistryPath(isDev)
}

// androidMountRegistryPath Android 端 app 私有目录。
//
// 优先 ENCV_APP_FILES_DIR（Kotlin 端 EncvGoService 通过 `context.filesDir.absolutePath` 注入）。
// Fallback：硬编码 `/data/user/0/com.encvgo.app/files`（与 android/app/build.gradle.kts
// applicationId 一致，可被 ENCV_PACKAGE_NAME 覆盖）。
func androidMountRegistryPath() string {
	filesDir := os.Getenv("ENCV_APP_FILES_DIR")
	if filesDir == "" {
		pkg := os.Getenv("ENCV_PACKAGE_NAME")
		if pkg == "" {
			pkg = "com.encvgo.app"
		}
		filesDir = filepath.Join("/data/user/0", pkg, "files")
	}
	return filepath.Join(filesDir, ".encv", "mounts.json")
}

// desktopMountRegistryPath 桌面端（Linux / macOS / Windows）mounts.json 路径。
//
// dev / production 用不同 sub 目录防覆盖。
func desktopMountRegistryPath(isDev bool) string {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = os.Getenv("APPDATA")
		}
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
		return filepath.Join(base, desktopAppDirName(isDev), "mounts.json")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Application Support", desktopAppDirName(isDev), "mounts.json")
	default: // linux / 其他 unix
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, ".local", "share")
		}
		return filepath.Join(base, desktopAppDirName(isDev), "mounts.json")
	}
}

// desktopAppDirName 桌面端 app 目录名（dev 加 -dev 后缀）。
func desktopAppDirName(isDev bool) string {
	if isDev {
		return "encv-dev"
	}
	return "encv"
}

// resolveUserPath 解析用户路径为绝对路径（multi-mount-aware）。
//
// 优先级：
//  1. mountRegistry 已注入 + 路径以 /d/ 开头 → 走 mount.Resolve（最长前缀匹配）
//  2. 否则 → 走旧 SafeResolveToAbsPath(servingDir, ...) 兜底（向后兼容）
//
// 返回 raw err；调用方负责 wrap 成 HTTP 状态码 / JSON error。
//
// 2026-06-15 Phase D：替换 server 包 12 处 SafeResolveToAbsPath 调用
// （mobile_api.go 5 / admin_handlers.go 6 / server_handle.go 1）。
func (s *Server) resolveUserPath(userPath string) (string, error) {
	if s.mountRegistry != nil && strings.HasPrefix(userPath, "/d/") {
		res, err := s.mountRegistry.Resolve(userPath)
		if err != nil {
			return "", fmt.Errorf("mount resolve %q: %w", userPath, err)
		}
		return res.AbsPath, nil
	}
	return utils.SafeResolveToAbsPath(s.servingDir, userPath)
}

func NewServer(ctx context.Context, configPath string) *Server {
	cfg := config.FromContext(ctx)
	containerManager := service.NewContainerManager()
	readerService := service.NewReaderService(containerManager)
	contentHandler := handler.NewContentHandler()
	mobileSvc := mobileservice.NewMobileService("", cfg)
	s := &Server{
		cfg:            cfg,
		configPath:     configPath,
		readerService:  readerService,
		mobileSvc:      mobileSvc,
		contentHandler: contentHandler,
		instanceID:     fmt.Sprintf("%x", time.Now().UnixNano()),
		mockEngine:     NewMockEngine(),
	}
	// 把 mock 引擎的 tool_call.execute_real 真实执行器绑到 s.executeAgentTool
	// ——剧本里声明 execute_real=true 的工具调用会被实际执行（覆盖硬编码 result）。
	// 见 internal/server/agent_mock.go §executeRealAndEmit
	s.mockEngine.SetRealExecutor(s.executeAgentTool)

	// 注册 v2 工具（search_files / get_metadata / command_run / edit_metadata / batch_rename / read_file_v2）
	// 见 internal/tools/register.go §RegisterAll
	tools.RegisterAll()

	// 构造工具依赖注入：把所有需要 Server 状态的回调（mount 解析 / 沙箱检查）绑进来
	s.toolDeps = &tools.ToolDeps{
		ResolveMount: func(mountID string) (string, bool) {
			return s.resolveMount(mountID)
		},
		SandboxCheck: func(absPath string) bool {
			// 沙箱校验：检查 absPath 是否在任意一个已配置的 mount 根目录内
			return s.isInAnyMount(absPath)
		},
		Config: cfg,
	}

	// 🆕 2026-06-15：多挂载点注册表（multi-mount-storage-refactor spec）
	// 启动顺序：
	//   1. NewRegistry
	//   2. RegisterDriverFactory x 3
	//   3. MigrateFromServingDir（Load → 必要时 Bootstrap → Save）
	// 完成后 s.mountRegistry 可用于 HTTP API + 未来 task_manager 路径解析
	s.mountRegistry = mount.NewRegistry(&configMountProvider{cfg: cfg}, mountRegistryDataPath(cfg))
	s.mountRegistry.RegisterDriverFactory(mount.DriverLocal, func() mount.Driver { return drivers.NewLocalDriver() })
	s.mountRegistry.RegisterDriverFactory(mount.DriverAppData, func() mount.Driver { return drivers.NewAppDataDriver() })
	s.mountRegistry.RegisterDriverFactory(mount.DriverSandbox, func() mount.Driver { return drivers.NewSandboxDriver() })
	if err := s.mountRegistry.MigrateFromServingDir(context.Background()); err != nil {
		// 🆕 2026-06-16：不再静默
		//   历史：fmt.Fprintf(os.Stderr, ...) → 只进 stderr，DevLogs 看不到 → 用户不知道 mount 为何失败
		//   现在：slog.Error 让 WSLogHandler 推到 DevLogs + 存到 s.mountBootstrapErrors 让 /api/mounts 暴露
		msg := fmt.Sprintf("mount MigrateFromServingDir failed: %v", err)
		slog.Error("mount bootstrap failed", "err", err)
		s.mountBootstrapErrors = append(s.mountBootstrapErrors, msg)
	}
	// 挂载点创建确认日志（真机调试：logcat 可搜 [mount] 关键字确认 automation mount 是否就绪）
	mounts := s.mountRegistry.List()
	for _, m := range mounts {
		fmt.Fprintf(os.Stderr, "[mount] ready: name=%s driver=%s path=%s root=%s\n",
			m.Name, m.Driver, m.MountPath, m.RootPath)
	}
	// 🆕 2026-06-16: 启动时**无条件** slog.Info 当前 mount list（推到 DevLogs）
	//   真机用户能立即在 DevLogs 看到「mount registry ready: count=3 names=[primary automation sandbox]」
	//   启动期错误也通过 slog.Error 推到 DevLogs（不再静默 — 旧实现 fmt.Fprintf(stderr)）
	mountNames := make([]string, 0, len(mounts))
	for _, m := range mounts {
		mountNames = append(mountNames, m.Name)
	}
	slog.Info("mount registry ready (startup)", "count", len(mounts), "names", mountNames)

	// 🆕 2026-06-15 multi-mount: 把 mount registry 注入 MobileService
	//   - primaryRootProvider 适配器桥接 mount.MountRegistry → service.MountRootProvider
	//   - taskMountResolver 适配器桥接 mount.MountRegistry → service.MountResolver
	//   - MobileService.SetMountRegistry 一次性注入两个用途（task 路径解析 + DeleteFile 守卫）
	mobileSvc.SetMountRegistry(
		&primaryRootProvider{reg: s.mountRegistry},
		&taskMountResolver{reg: s.mountRegistry},
	)

	// 🆕 2026-07-01：数据库存储引擎初始化
	actualEngine, dbPath := s.InitDatabase(s.servingDir)

	// 🆕 2026-07-03：向量搜索服务初始化
	s.InitVectorSearch(dbPath, actualEngine)

	// 剧本外置 spec：若 agent_settings.mock_scenarios_dir 非空，
	// 用 ScenarioLoader 加载 YAML/JSON 剧本，注入到 MockEngine。
	// 详见 internal/server/mock_scenarios/SCHEMA.md。
	s.loadScenariosFromAgentConfig()
	return s
}

// isInAnyMount 检查 absPath 是否落在任一挂载点根目录下。
// 用于 tools.ToolDeps.SandboxCheck（command_run 等需白名单的工具）。
func (s *Server) isInAnyMount(absPath string) bool {
	if absPath == "" {
		return false
	}
	mounts := s.ListFSMounts()
	for _, m := range mounts {
		if !m.Available {
			continue
		}
		var root string
		if m.Type == "serving" {
			root = s.servingDir
		} else if m.Type == "webdav" {
			root = s.webdavDir
		}
		if root == "" {
			continue
		}
		rel, err := filepath.Rel(root, absPath)
		if err != nil {
			continue
		}
		// rel 必须以 "." 或 "/" 开头（或等于 absPath），且不含 ".."
		if rel == "." || (len(rel) > 0 && rel[0] != '.' && rel[0] != '/') {
			// 简化：直接比对前缀
			_ = rel
			if strings.HasPrefix(absPath, root) {
				return true
			}
		}
	}
	return false
}

func (s *Server) GetInstanceID() string {
	return s.instanceID
}

func (s *Server) GetCredentials() (string, string) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	return s.cfg.Webdav.Username, s.cfg.Webdav.Password
}

var knownRoutePrefixes = []string{
	"/api/", "/admin", "/login", "/logout",
	"/p", "/p-api", "/openlist",
	"/preview/", "/stream", "/decrypt",
	"/ws", "/ping", "/health",
}

func checkWebdavRouteConflict(webdavRoot string) string {
	cleanRoot := strings.TrimSuffix(strings.TrimSpace(webdavRoot), "/")

	if cleanRoot == "" {
		return "<root>"
	}

	for _, prefix := range knownRoutePrefixes {
		cleanPrefix := strings.TrimSuffix(prefix, "/")
		if strings.HasPrefix(cleanPrefix, cleanRoot) || strings.HasPrefix(cleanRoot, cleanPrefix) {
			return prefix
		}
	}
	return ""
}

func sanitizeWebdavRootInConfig(configPath string) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return
	}
	wd, ok := cfg["webdav"].(map[string]interface{})
	if !ok {
		return
	}
	wd["root"] = ""
	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(configPath, append(updated, '\n'), 0644); err != nil {
		slog.Warn("Failed to sanitize webdav root in config", "error", err)
		return
	}
	slog.Info("Sanitized webdav root in config file (set to empty to disable WebDAV)", "path", configPath)
}

func (s *Server) Start(version string) (string, error) {
	// 【关键修改】在启动时初始化版本和实例ID
	s.version = version // 从 main 包获取编译时注入的版本

	// 1. 解析并存储主服务目录
	dir := s.cfg.Server.Dir
	var err error

	s.servingDir, err = filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path for directory '%s': %w", dir, err)
	}
	s.mobileSvc.SetServingDir(s.servingDir)
	// 🆕 2026-06-14：跨进程 IPC 重构 — 填充 RuntimeInfo 单一来源
	//
	// 历史：原方案让 Kotlin (parent) 写 config.user.json.mobile.server.dir
	// → Go 读 → 双源。Android scoped storage 拒权限时挂掉。
	//
	// 新方案：Go 自己解析路径（s.servingDir 已 Abs），声明在 runtimeInfo。
	// Kotlin 通过 GET /api/runtime 读取，零文件依赖、零 env 协商。
	s.runtimeInfoMu.Lock()
	s.runtimeInfo = RuntimeInfo{
		PID:         resolvePID(),
		Version:     s.version,
		InstanceID:  s.instanceID,
		ServingDir:  s.servingDir,
		Port:        0, // 下面 register.StartGinWithRetry 成功后回填
		StartedAt:   time.Now().UnixMilli(),
		Mobile:      os.Getenv("ENCV_MOBILE") == "1",
		ConfigPath:  s.configPath,
		UptimeMs:    0,
		HeartbeatOK: false, // 启动时还没开始 tick
	}
	s.runtimeInfoMu.Unlock()

	// 🆕 2026-06-14：in-memory 心跳 loop（替代 ffmpeg.StartHeartbeatLoop 的文件版本）
	//
	// 历史：ffmpeg.StartHeartbeatLoop 写 .encv_heartbeat 文件，
	//       Kotlin 1s poll mtime。路径协商 + FAT32 精度 2s 引发 7s 必死 bug。
	//
	// 新：atomic.Int64 内存字段，独立 goroutine 每 2s 写。
	//     HTTP /health JSON 读，HTTP /api/runtime 也读。
	//     父进程用 HTTP 探活，不需文件。
	s.startHeartbeatLoopInMemory(context.Background())

	// 🆕 2026-06-14：删除 ENCV_HEARTBEAT_PATH 文件版心跳（ffmpeg.StartHeartbeatLoop）
	//
	// 心跳现在完全在内存：startHeartbeatLoopInMemory() 写 atomic.Int64，
	// HTTP /health JSON 读，HTTP /api/runtime 也读。
	// 不再需要 ENCV_HEARTBEAT_PATH env var、.encv_heartbeat 文件、ffmpeg writeHeartbeat()。
	//
	// 见 spec/cross-process-ipc-refactor/spec.md §3.3, §3.5
	//
	// 【防回归 - 旧 bug】
	// 文件版心跳 + FAT32 精度 2s + Kotlin / Go 路径不同步 → 7s 必死。
	// 不要重新引入文件版心跳。如果需要跨进程状态，用 HTTP /api/runtime。
	//
	// 删：ffmpeg.StartHeartbeatLoop(context.Background())
	// 删：os.Setenv("ENCV_HEARTBEAT_PATH", ...)
	// 删：filepath.Join(s.servingDir, ".encv_heartbeat")
	chunkNamers := plugins.GetAllRegisteredChunkNamers()
	s.chunkNamers = chunkNamers
	s.mobileSvc.SetEncryptedFileDeps(s.readerService, s.contentHandler, chunkNamers)

	// 2. 解析并存储 WebDAV 目录和路径
	if s.cfg.Webdav.Root != "" {
		webdavDir := s.cfg.Webdav.Dir
		if webdavDir == "" {
			webdavDir = s.servingDir
			s.cfg.Webdav.Dir = s.servingDir
			slog.Info("WebDAV dir not specified, using server dir", "dir", webdavDir)
		}
		s.webdavDir, err = filepath.Abs(webdavDir)
		if err != nil {
			return "", fmt.Errorf("failed to resolve absolute path for webdav dir '%s': %w", webdavDir, err)
		}
		s.webdavPath = s.cfg.Webdav.Root
		if s.webdavPath == "" {
			s.webdavPath = "/webdav/"
		}
		if !strings.HasPrefix(s.webdavPath, "/") {
			s.webdavPath = "/" + s.webdavPath
		}
		if !strings.HasSuffix(s.webdavPath, "/") {
			s.webdavPath += "/"
		}
		if conflict := checkWebdavRouteConflict(s.webdavPath); conflict != "" {
			slog.Error("WebDAV route conflicts with existing route, DISABLING WebDAV to avoid crash",
				"webdav_path", s.webdavPath,
				"conflict_with", conflict,
			)
			s.webdavDir = ""
			s.webdavPath = ""
			sanitizeWebdavRootInConfig(s.configPath)
		} else {
			slog.Info("WebDAV enabled", "dir", s.webdavDir, "path", s.webdavPath)
		}
	}

	slog.Info("Server starting", "instance", s.instanceID, "version", s.version)
	slog.Info("Main service serving from", "dir", s.servingDir)

	openlist.TryRegisterLocalLoopback(s.cfg.Proxy.Sites)

	wsMinLevel := slog.LevelInfo
	switch s.cfg.Log.Level {
	case "debug":
		wsMinLevel = slog.LevelDebug
	case "warn":
		wsMinLevel = slog.LevelWarn
	case "error":
		wsMinLevel = slog.LevelError
	}
	wsLogHandler := NewWSLogHandler(slog.Default().Handler(), s.mobileSvc.GetWSHub(), wsMinLevel)
	slog.SetDefault(slog.New(wsLogHandler))
	slog.Info("WSLogHandler initialized, logs will be bridged to WebSocket")

	// 3. 创建 Gin 引擎并注册路由
	r := NewGinApp(s.cfg)

	RegisterRoutes(s, r)

	s.InitWebDAV(r)

	r.NoRoute(gin.WrapF(s.handleRequest))

	srv, addr, err := register.StartGinWithRetry(r, s.cfg.Server.Port, s.instanceID, s.version)
	if err != nil {
		return "", err
	}
	s.server = srv
	_, portStr, splitErr := net.SplitHostPort(addr)
	if splitErr == nil {
		if p, parseErr := strconv.Atoi(portStr); parseErr == nil {
			s.actualPort = p
			// 🆕 2026-06-14：回填 runtimeInfo.Port，/api/runtime 立即可读
			s.runtimeInfoMu.Lock()
			s.runtimeInfo.Port = p
			s.runtimeInfoMu.Unlock()
		}
	}
	return addr, nil
}

func (s *Server) Stop() error {
	s.readerService.Cleanup()
	if s.server != nil {
		slog.Info("Shutting down server")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

// handleRequest 是主路由 / 处理器
func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	slog.Debug("handleRequest", "path", r.URL.Path)

	// 2. 传递给servePath处理
	s.servePath(w, r, r.URL.Path)
}

// 能处理文件和目录
func (s *Server) servePath(w http.ResponseWriter, r *http.Request, relativePath string) {
	// 使用通用工具函数进行安全解析
	fullPath, err := utils.SafeURLToAbsPath(s.servingDir, relativePath)
	if err != nil {
		if strings.Contains(err.Error(), "forbidden") {
			http.Error(w, "Forbidden", http.StatusForbidden)
		} else {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		return
	}

	// 检查路径信息
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.NotFound(w, r)
		} else {
			http.Error(w, "Could not access path", http.StatusInternalServerError)
		}
		return
	}

	// 如果是目录，则列出该目录的内容
	if info.IsDir() {
		// 为目录 URL 添加末尾斜杠，以便正确生成相对链接
		urlPath := "/" + strings.TrimSuffix(relativePath, "/") + "/"
		s.listFilesInDir(w, r, fullPath, urlPath)
		return
	}

	// 如果是文件，则继续处理
	s.serveFile(w, r, fullPath)
}

// serveFile 处理对单个文件的请求
func (s *Server) serveFile(w http.ResponseWriter, r *http.Request, fullPath string) {
	fileName := filepath.Base(fullPath)

	// 判断是否是 ENCV 容器文件
	_, err := detector.DetectContainer(fullPath)
	if err == nil {
		// 如果 err 为 nil，说明文件是有效的 ENCV 容器
		slog.Debug("Serving ENCV container", "file", fileName)
		// 【关键修改】调用我们新的、统一的处理函数
		s.serveEncryptedFile(w, r, fullPath)
		return
	}

	// 如果不是容器文件，作为普通文件（如字幕）提供服务
	slog.Debug("Serving standard file", "file", fileName)
	http.ServeFile(w, r, fullPath)
}

// listFilesInDir 在指定目录生成一个文件列表页面
// urlPath 是当前目录对应的 URL 路径，用于生成正确的导航链接
func (s *Server) listFilesInDir(w http.ResponseWriter, r *http.Request, dirPath, urlPath string) {
	// 【核心】从 Header 中获取代理前缀
	forwardedPrefix := r.Header.Get("X-Forwarded-Prefix")
	if forwardedPrefix == "" {
		forwardedPrefix = "" // 如果没有代理，前缀为空
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, "Could not read directory", http.StatusInternalServerError)
		return
	}

	type FileInfo struct {
		Name        string
		Path        string
		IsDir       bool
		IsContainer bool
		HumanSize   string // 【修改】使用 humanize 格式化后的大小
		ModTime     time.Time
	}

	var files []FileInfo
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// 【关键修改】在生成文件路径时，加上代理前缀
		files = append(files, FileInfo{
			Name:        entry.Name(),
			Path:        utils.BuildURLPath(forwardedPrefix, urlPath, entry.Name(), entry.IsDir()),
			IsDir:       entry.IsDir(),
			IsContainer: !entry.IsDir() && plugins.IsContainer(entry.Name()),
			HumanSize:   humanize.Bytes(uint64(info.Size())),
			ModTime:     info.ModTime(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		// 目录排在文件前面
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	// 【关键修改】在计算父目录链接时，加上代理前缀
	// 【关键修改】使用 path.Dir 处理 URL 路径，确保使用 '/'
	cleanedUrlPath := path.Clean(urlPath)
	parentPath := forwardedPrefix + path.Dir(cleanedUrlPath)
	if parentPath == forwardedPrefix+"." {
		parentPath = forwardedPrefix + "/"
	}

	// 为模板准备数据
	data := struct {
		CurrentPath string
		ParentPath  string
		NotRoot     bool
		RootPath    string // 【新增】用于面包屑的根路径
		Ancestors   []struct{ Name, Path string }
		Files       []FileInfo
	}{
		CurrentPath: urlPath, // CurrentPath 用于显示，不需要前缀
		ParentPath:  parentPath,
		NotRoot:     urlPath != "/",
		RootPath:    forwardedPrefix + "/", // 【新增】设置根路径
		Files:       files,
	}

	// 【关键修改】在生成面包屑导航的路径时，加上代理前缀
	parts := strings.Split(strings.Trim(urlPath, "/"), "/")
	for i := 0; i < len(parts); i++ {
		// 跳过空的部分，比如当 urlPath 是 "/" 时
		if parts[i] == "" {
			continue
		}
		// 【正确】使用 strings.Join 来构建路径片段
		ancestorPath := "/" + strings.Join(parts[:i+1], "/") + "/"
		data.Ancestors = append(data.Ancestors, struct{ Name, Path string }{
			Name: parts[i],
			Path: forwardedPrefix + ancestorPath, // 加上前缀
		})
	}

	t, _ := template.New("list").Parse(tmpl_dynamic_files)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, data); err != nil {
		slog.Error("Error executing template", "error", err)
	}
}
