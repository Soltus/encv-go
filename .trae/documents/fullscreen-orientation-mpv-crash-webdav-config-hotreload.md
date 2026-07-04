# 计划：全屏退出方向恢复 + MPV 异常防护 + 日志推送 + WebDAV 适配 + 配置热重载

## 问题 1：全屏时直接退出（系统返回键）没有恢复屏幕方向

### 根因分析
- `MpvPlayerModule.setFullscreen(false)` 会恢复方向为 `UNSPECIFIED`，但只在 JS 主动调用时触发
- 全屏时按系统返回键，`PlayerOverlayManager.hideOverlay()` → `cleanupOverlay()` 执行清理，但**没有恢复 `requestedOrientation`**
- `cleanupOverlay()` 释放了 MPV、销毁了 LynxView、移除了 overlay 布局，但 Activity 的 `requestedOrientation` 仍然是 `SENSOR_LANDSCAPE`
- 同样，全屏相关的 Window flags（`FLAG_FULLSCREEN`、`SYSTEM_UI_FLAG_*`）也没有清除

### 修复方案
在 `cleanupOverlay()` 中添加屏幕方向和全屏 flags 恢复：

```kotlin
// PlayerOverlayManager.kt - cleanupOverlay()
private fun cleanupOverlay() {
    try {
        // 恢复屏幕方向和全屏状态（无论是否全屏退出都需要恢复）
        activity?.let { act ->
            act.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
            act.window.clearFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN)
            @Suppress("DEPRECATION")
            act.window.decorView.systemUiVisibility = View.SYSTEM_UI_FLAG_VISIBLE
        }

        lynxView?.removeCallbacks(positionUpdateRunnable)
        MpvPlayerModule.getInstance()?.let { mpvModule ->
            mpvModule.detachFromLayout(overlayLayout ?: FrameLayout(activity ?: return@let))
            mpvModule.release()
        }
        // ... 其余清理逻辑不变
    }
}
```

需要添加 import：`android.content.pm.ActivityInfo`、`android.view.WindowManager`

---

## 问题 2：MPV 播放器防止异常闪退和播放异常可视化

### 根因分析
- 当前 MPV 事件处理只关注 `FILE_LOADED`、`END_FILE`、`SHUTDOWN`，没有处理错误详情
- `MPV_EVENT_END_FILE` 事件携带 reason 字段，可以区分正常结束和错误结束
- 损坏的视频/音频可能导致 MPV 内部崩溃，但前端无法感知
- 没有 MPV 进程存活检测机制

### 修复方案

**2a. MpvPlayerModule.kt - 增强 END_FILE 事件处理**

MPV 的 `MPV_EVENT_END_FILE` 通过 `eventProperty(property: String, value: Long)` 传递 `end-file-reason`：
- 0 = MPV_END_FILE_REASON_EOF（正常结束）
- 1 = MPV_END_FILE_REASON_STOP（用户停止）
- 2 = MPV_END_FILE_REASON_QUIT（退出）
- 3 = MPV_END_FILE_REASON_ERROR（错误）
- 4 = MPV_END_FILE_REASON_REDIRECT

需要：
1. 观察 `end-file-reason` 属性
2. 当 reason=3 时，获取 `error-text` 属性，dispatch error 状态
3. 在 `event(eventId: Int)` 中处理 `MPV_EVENT_END_FILE` 时检查错误原因

```kotlin
// 在 eventObserver 中添加：
override fun eventProperty(property: String, value: Long) {
    when (property) {
        "pause" -> dispatchStateChange(if (value) "paused" else "playing")
        "idle" -> { if (value) dispatchStateChange("ended") }
        "end-file-reason" -> {
            if (value == 3L) { // MPV_END_FILE_REASON_ERROR
                mainHandler.post {
                    try {
                        val errorText = MPVLib.getPropertyString("error-text") ?: "未知播放错误"
                        dispatchStateChange("error", "播放失败: $errorText")
                    } catch (e: Exception) {
                        dispatchStateChange("error", "播放失败")
                    }
                }
            }
        }
    }
}
```

同时在 `ensureMpvInitialized()` 中添加 `observeProperty("end-file-reason", MpvFormat.MPV_FORMAT_INT64)`

**2b. PlayerApp.tsx - 增强错误可视化**

- 当收到 error 状态时，显示友好的错误信息（已部分实现）
- 添加"重试"按钮，允许用户重新尝试播放
- 对于损坏文件，显示明确的"文件可能已损坏"提示

**2c. MpvPlayerModule.kt - 添加 MPV 崩溃防护**

在关键方法（play、seekTo 等）中添加 try-catch，防止 MPV 崩溃导致应用闪退：

```kotlin
@LynxMethod
fun play(url: String, callback: Callback) {
    try {
        ensureMpvInitialized()
        // ... 现有逻辑
    } catch (e: Exception) {
        LogRelay.get().relay(TAG, "error", "play failed: ${e.message}")
        dispatchStateChange("error", "播放失败: ${e.message}")
        callback.invoke(e.message)
    }
}
```

---

## 问题 3：WebDAV 等模块缺少推送日志到 devlogs 显示

### 根因分析
经过调查，日志推送管道已经存在：

1. **Go 后端 → 前端 DevLogs**：`WSLogHandler` 桥接 `slog` → WebSocket → 前端 DevLogs（已实现）
2. **Android 原生 → Go 后端**：`LogRelay` 通过 `POST /api/logs` → `handleAPILogsGin` → `slog` → `WSLogHandler`（已实现）
3. **Lynx 前端 → Go 后端**：`LogBridgeModule` → `LogRelay` → `POST /api/logs`（已实现）

**问题所在**：
- `WSLogHandler` 只转发 `Level >= Info`（过滤了 debug 级别），但 `config.mobile.json` 设置了 `"log": {"level": "debug"}`，用户期望看到 debug 日志
- `handleAPILogsGin` 中 `default` 分支使用 `slog.Info`，但前端可能发送了 debug 级别的日志
- 需要让 WSLogHandler 的过滤级别与配置的 log.level 保持一致

### 修复方案

**3a. WSLogHandler 支持动态日志级别**

```go
// ws_log_handler.go
type WSLogHandler struct {
    inner      slog.Handler
    hub        *service.WSHub
    minLevel   slog.Level // 新增：可配置的最低转发级别
}

func (h *WSLogHandler) Handle(ctx context.Context, r slog.Record) error {
    err := h.inner.Handle(ctx, r)
    if err != nil {
        return err
    }

    if r.Level < h.minLevel {  // 改为使用可配置级别
        return nil
    }
    // ... 其余不变
}
```

在 `server.go` 的 `Start()` 中根据 `cfg.Log.Level` 设置 `minLevel`。

**3b. handleAPILogsGin 支持 debug 级别**

```go
case "debug":
    slog.Debug(msg)
```

---

## 问题 4：encv WebDAV 服务适配安卓路径

### 根因分析
- `config.mobile.json` 中 `"webdav": {"dir": ""}`，WebDAV 在移动端是禁用的
- 当用户在移动端启用 WebDAV 时，`dir` 需要指向 Android 路径如 `/storage/emulated/0/`
- `NewENCVFS` 使用 `filepath.Abs(dir)` 解析路径，在 Android 上 `/storage/emulated/0/` 已经是绝对路径，`filepath.Abs` 会直接返回
- 但 `filepath.Abs` 在相对路径时会使用 `os.Getwd()`，在 Android 上可能不是用户期望的目录
- 需要确保 WebDAV Dir 在 Android 上正确解析

### 修复方案

**4a. 移动端 WebDAV Dir 默认值**

当 WebDAV Dir 为空但 Root 不为空时，默认使用 Server.Dir 作为 WebDAV 的 Dir：

```go
// server.go Start() 中
if s.cfg.Webdav.Dir == "" && s.cfg.Webdav.Root != "" {
    s.webdavDir = s.servingDir // 复用主服务目录
    slog.Info("WebDAV dir not specified, using server dir", "dir", s.webdavDir)
}
```

**4b. fs_v2.go 路径解析兼容**

`NewENCVFS` 中的 `filepath.Abs(dir)` 已经能正确处理 Android 绝对路径。但需要处理 `dir == "/"` 的情况（当前已有处理）。

---

## 问题 5：移动端 WebDAV 复用索引

### 根因分析
当前存在两套独立的索引系统：

1. **MobileService.fileIndex**（`mobile_service.go`）
   - 简单的扁平索引，只用于搜索
   - 不包含加密容器解密信息
   - 通过 `RebuildIndex()` 手动触发构建

2. **encvWebDAVFS.indexes**（`fs_v2.go`）
   - 复杂的索引，包含 pathMap、dirMap、fileInfoMap、reversePathMap
   - 包含加密容器的虚拟文件映射
   - 通过 fsnotify 自动增量更新

**核心问题**：WebDAV.Dir 和 Server.Dir 可能不一致：
- 场景 A：`Server.Dir = /storage/emulated/0/`，`WebDAV.Dir = /storage/emulated/0/`（相同）
- 场景 B：`Server.Dir = /storage/emulated/0/`，`WebDAV.Dir = /storage/emulated/0/Music/`（WebDAV 是子目录）
- 场景 C：`Server.Dir = /storage/emulated/0/Docs/`，`WebDAV.Dir = /storage/emulated/0/Music/`（完全不同）

不能简单共享同一个索引，因为：
1. 两套索引的数据结构完全不同（扁平 vs 嵌套映射）
2. WebDAV 索引包含容器解密虚拟文件映射，MobileService 不需要
3. 路径范围可能不同

### 修复方案：共享文件发现层，独立索引处理

核心思路：**避免重复扫描文件系统，但保持各自的索引结构**。

**5a. 引入 FileDiscoveryService 共享文件发现**

```go
// internal/service/file_discovery.go
type FileDiscoveryService struct {
    mu          sync.RWMutex
    rootDir     string
    fileEntries map[string]os.FileInfo  // 相对路径 -> FileInfo
    dirEntries  map[string][]string     // 目录 -> 子项列表
    watchers    []FileDiscoveryListener
    watcher     *fsnotify.Watcher
}

type FileDiscoveryListener interface {
    OnFileAdded(relPath string, info os.FileInfo)
    OnFileRemoved(relPath string)
    OnFileModified(relPath string, info os.FileInfo)
}
```

- FileDiscoveryService 负责扫描和监控文件系统
- MobileService 和 WebDAV FS 都注册为 Listener
- 当文件变化时，FileDiscoveryService 通知所有 Listener
- 每个 Listener 维护自己的索引结构

**5b. 处理路径不一致的情况**

- 当 WebDAV.Dir == Server.Dir 时：共享同一个 FileDiscoveryService
- 当 WebDAV.Dir 是 Server.Dir 的子目录时：共享同一个 FileDiscoveryService，WebDAV FS 只处理自己路径范围内的文件
- 当两者完全不同时：各自有独立的 FileDiscoveryService（但这种情况在移动端很少见）

**5c. 具体实现步骤**

1. 创建 `FileDiscoveryService`，封装文件遍历和 fsnotify 监控
2. 修改 `MobileService`：使用 FileDiscoveryService 替代自己的 `RebuildIndex`
3. 修改 `encvWebDAVFS`：使用 FileDiscoveryService 替代自己的 `runIndexer`
4. 在 `Server.Start()` 中创建 FileDiscoveryService 实例，根据路径关系决定共享策略

**5d. 简化方案（推荐先实施）**

考虑到重构复杂度，先实施简化版：

1. 在 `Server` 中持有 `encvWebDAVFS` 实例引用
2. 给 `encvWebDAVFS` 添加公开方法 `GetIndexStats()` 和 `SearchInIndex(keyword, queryPath string, maxResults int) []FileInfo`
3. 当 WebDAV 启用且 WebDAV.Dir == Server.Dir 时，`MobileService.SearchFiles` 和 `handleListFilesGin` 优先使用 WebDAV FS 的索引
4. 当 WebDAV 未启用或路径不同时，使用 MobileService 自己的索引
5. MobileService 不再需要 `RebuildIndex`（当 WebDAV FS 已有索引时）

这样在移动端最常见的场景（WebDAV.Dir == Server.Dir）下，只有一套索引在工作。

**5e. 移动端设置界面索引管理适配**

当前 `CacheDetail.vue` 和 `Settings.vue` 中的索引管理直接调用 `MobileService` 的索引 API（`/api/index/stats`、`/api/index/rebuild`、`/api/index/clear`），需要适配复用索引后的变化：

1. **后端 API 适配**：
   - `handleIndexStatsGin`：当 WebDAV FS 启用且路径一致时，返回 WebDAV FS 的索引统计（更准确，包含容器虚拟文件信息）
   - `handleIndexRebuildGin`：当 WebDAV FS 启用时，触发 WebDAV FS 的索引重建（而非 MobileService 的 `RebuildIndex`）
   - `handleIndexClearGin`：当 WebDAV FS 启用时，同时清除 WebDAV FS 的索引缓存
   - `IndexStats` 结构添加 `source` 字段（`"mobile"` 或 `"webdav"`），前端可据此显示索引来源

2. **前端 CacheDetail.vue 适配**：
   - 显示索引来源（WebDAV 索引 / 移动端索引）
   - WebDAV 索引模式下，显示额外的统计信息（如加密容器数量）
   - 重建索引按钮在 WebDAV 模式下触发 WebDAV FS 的索引重建

3. **前端 Settings.vue 适配**：
   - 索引状态行显示来源标识

---

## 问题 6：配置热重载评估

### 当前状态
- `handlePutConfigGin` 只写入文件，**不更新内存中的 `s.cfg`**
- 配置在 `Start()` 时一次性加载到 `s.cfg`
- 没有 `ReloadConfig` 机制

### 各配置项热重载可行性分析

| 配置项 | 热重载？ | 原因 | 实现方式 |
|--------|---------|------|---------|
| `password` | ✅ 可以 | 运行时用于解密，修改 `s.cfg.Password` 即可 | 直接修改 cfg 字段 |
| `recover` | ✅ 可以 | 运行时标志 | 直接修改 cfg 字段 |
| `output_path` | ✅ 可以 | 运行时用于加密输出目录 | 直接修改 cfg 字段 |
| `plugin_settings` | ✅ 可以 | 运行时通过 ConfigProvider 获取 | 直接修改 cfg 字段 |
| `server.port` | ❌ 必须重启 | HTTP 服务器绑定端口，无法运行时更改 | 需要重启 |
| `server.dir` | ⚠️ 理论可以但不推荐 | servingDir 被多处引用，修改需要级联更新 MobileService、索引等 | 需要级联更新 |
| `admin.password` | ✅ 可以 | JWT 认证密码，更新 JWTManager 即可 | 更新 jwtManager.secret |
| `webdav.root` | ❌ 必须重启 | 路由在启动时注册到 Gin | 需要重启 |
| `webdav.dir` | ❌ 必须重启 | WebDAV FS 在启动时初始化，索引需要重建 | 需要重启 |
| `webdav.username/password` | ✅ 可以 | BasicAuth 中间件每次请求读取 | 修改 cfg 字段 + BasicAuth 中间件改为动态读取 |
| `proxy.sites` | ✅ 已支持 | 已有 add/update/delete API | 已实现 |
| `log.level` | ✅ 可以 | 重新初始化 slog handler | 更新 handler 级别 |
| `log.file` | ⚠️ 理论可以 | 需要重新打开日志文件 | 重新初始化 logger |
| `log.console` | ✅ 可以 | 控制台输出开关 | 重新初始化 logger |

### 修复方案

**6a. handlePutConfigGin 增加热重载逻辑**

```go
func (s *Server) handlePutConfigGin(c *gin.Context) {
    s.configMu.Lock()
    defer s.configMu.Unlock()

    // ... 现有的文件写入逻辑 ...

    // 热重载：解析新配置并更新内存
    var newCfg config.Config
    if err := json.Unmarshal(body, &newCfg); err != nil {
        // 已经写入了文件，但解析失败，记录警告
        slog.Warn("Config written to file but failed to parse for hot reload", "error", err)
        c.JSON(http.StatusOK, gin.H{"message": "config saved (hot reload skipped)"})
        return
    }

    // 更新可热重载的字段
    s.cfg.Password = newCfg.Password
    s.cfg.Recover = newCfg.Recover
    s.cfg.OutputPath = newCfg.OutputPath
    s.cfg.PluginSettings = newCfg.PluginSettings

    // Admin password - 更新 JWTManager
    if newCfg.Admin.Password != s.cfg.Admin.Password {
        s.cfg.Admin.Password = newCfg.Admin.Password
        if newCfg.Admin.Password != "" {
            s.jwtManager = auth.NewJWTManager(newCfg.Admin.Password, 7*24*time.Hour)
        } else {
            s.jwtManager = nil
        }
    }

    // WebDAV credentials - 直接修改 cfg（BasicAuth 中间件需要改为动态读取）
    s.cfg.Webdav.Username = newCfg.Webdav.Username
    s.cfg.Webdav.Password = newCfg.Webdav.Password

    // Log level - 重新初始化
    if newCfg.Log.Level != s.cfg.Log.Level {
        s.cfg.Log.Level = newCfg.Log.Level
        // 重新设置 slog 级别
        // ... 更新 WSLogHandler 的 minLevel
    }

    // 标记需要重启的配置变更
    needsRestart := false
    if newCfg.Server.Port != s.cfg.Server.Port {
        needsRestart = true
    }
    if newCfg.Webdav.Root != s.cfg.Webdav.Root || newCfg.Webdav.Dir != s.cfg.Webdav.Dir {
        needsRestart = true
    }
    if newCfg.Server.Dir != s.cfg.Server.Dir {
        needsRestart = true
    }

    // 更新不可热重载的字段（下次重启生效）
    s.cfg.Server = newCfg.Server
    s.cfg.Webdav.Root = newCfg.Webdav.Root
    s.cfg.Webdav.Dir = newCfg.Webdav.Dir
    s.cfg.Log = newCfg.Log

    msg := "config updated"
    if needsRestart {
        msg = "config saved, some changes require restart to take effect"
    }
    c.JSON(http.StatusOK, gin.H{"message": msg, "needsRestart": needsRestart})
}
```

**6b. WebDAV BasicAuth 中间件改为动态读取**

当前 `middleware.BasicAuth` 在启动时固定了用户名密码，需要改为每次请求从 cfg 动态读取。

---

## 实施步骤

### 步骤 1：全屏退出方向恢复
1. 修改 `PlayerOverlayManager.kt` 的 `cleanupOverlay()` 方法，添加方向和全屏 flags 恢复
2. 添加必要的 import

### 步骤 2：MPV 异常防护
1. 修改 `MpvPlayerModule.kt`：
   - 添加 `end-file-reason` 属性观察
   - 在 eventObserver 中处理错误结束原因
   - 获取 `error-text` 属性传递给前端
2. 修改 `PlayerApp.tsx`：
   - 添加重试按钮
   - 优化错误信息展示

### 步骤 3：日志推送优化
1. 修改 `ws_log_handler.go`：添加可配置的最低转发级别
2. 修改 `server.go`：根据 cfg.Log.Level 设置 WSLogHandler 的 minLevel
3. 修改 `handleAPILogsGin`：支持 debug 级别

### 步骤 4：WebDAV 安卓路径适配
1. 修改 `server.go`：WebDAV Dir 为空时默认使用 Server.Dir
2. 确认 `fs_v2.go` 的路径解析兼容 Android

### 步骤 5：移动端 WebDAV 复用索引（简化方案）
1. 在 `Server` 中持有 `encvWebDAVFS` 实例引用
2. 给 `encvWebDAVFS` 添加公开方法 `GetIndexStats()` 和 `SearchInIndex()`
3. `MobileService` 添加 `SetWebDAVFS()` 方法
4. `handleSearchFilesGin` 和 `handleListFilesGin` 优先使用 WebDAV FS 索引（当路径一致时）
5. 当 WebDAV 未启用或路径不同时，回退到 MobileService 自己的索引
6. 后端 API 适配：`handleIndexStatsGin`/`handleIndexRebuildGin`/`handleIndexClearGin` 支持 WebDAV FS 索引
7. `IndexStats` 结构添加 `source` 字段
8. 前端 `CacheDetail.vue` 适配：显示索引来源、WebDAV 模式额外统计
9. 前端 `Settings.vue` 适配：索引状态行显示来源标识

### 步骤 6：配置热重载
1. 修改 `handlePutConfigGin`：添加热重载逻辑
2. 修改 WebDAV BasicAuth 中间件：动态读取凭据
3. 修改 WSLogHandler：支持动态级别
4. 前端管理后台：显示"需要重启"提示
