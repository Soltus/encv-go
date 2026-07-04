# 安卓 APP 实测"后端未连接" — 根因排查与修复方案

## 一、完整调用链路追踪

### Android 端启动流程（APK 运行时）

```
MainActivity.onCreate()
  ├─ copyBinaryFromAssets()  → 从 APK assets 提取 encv-go 二进制到 /data/data/com.encvgo.app/files/
  ├─ 设置 ENVC_CONFIG=/data/data/com.encvgo.app/files/config.user.json  ← ⚠️ Bug #1
  └─ ProcessBuilder("encv-go", "start").start()   ← 启动 Go 进程
        │
        ▼ Go 进程 (cmd/encv start)
  PersistentPreRun():
  ├─ FindConfigPath("")  ← 查找配置文件
  │   ├─ 检查 os.Getenv("ENVC_CONFIG_PATH")  ← ❌ 读的是这个，不是 ENVC_CONFIG！
  │   ├─ 检查 cwd/config.user.json           ← ❌ filesDir 不是 cwd
  │   ├─ 检查 exeDir/config.user.json        ← ❌ 二进制目录无此文件
  │   └─ 返回错误 → 使用 DefaultConfig()
  │       └─ Server.Port = 1999  ← ⚠️ Bug #2: 前端连接的是 2025！
  │
  ├─ encv.Init(rootCtx)      ← 初始化插件系统
  ├─ s.Start(Version)         ← 启动 Backend Server 于端口 1999 ✅ 能启动
  ├─ SetupAdminServer(...)    ← 启动 GoFrame Admin Server ← ⚠️ Bug #3: 可能崩溃
  └─ select {}                ← 阻塞主线程


前端 (WebView):
  App.vue.onMounted()
  ├─ initTheme()              ✅ 纯客户端
  └─ connect()  → WebSocket ws://127.0.0.1:2025/ws  ← ❌ 后端在 1999！

Files 页面:
  useServerStatus()
  └─ checkServerStatus() → fetch('http://127.0.0.1:2025/health')  ← ❌ Connection Refused
      └─ isOnline = false  → 显示"后端未连接"
```

---

## 二、已确认的根因（按严重程度排序）

### 🔴 Bug #1 [关键]: 环境变量名不匹配

| | 实际值 |
|---|---|
| **Android MainActivity.kt 设置** | `ENVC_CONFIG` ([MainActivity.kt:31](file:///workspace/app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/MainActivity.kt#L31)) |
| **Go config.go 读取** | `ENVC_CONFIG_PATH` ([config.go:178](file:///workspace/internal/config/config.go#L178)) |

**结果**: 配置路径完全无法传递给 Go 进程。

### 🔴 Bug #2 [关键]: APK 中没有打包 config.user.json

查看 [.github/workflows/android.yml](file:///workspace/.github/workflows/android.yml)：
- 第 58-59 行：编译 Go 二进制 ✅
- 第 124-127 行：将二进制复制到 assets ✅
- **没有任何步骤复制 config.user.json 到 APK 中**

即使修复了 Bug #1，Go 进程也找不到配置文件，只能用默认值。

### 🟡 Bug #3 [重要]: 默认端口不一致 + start 命令过重

| 组件 | 默认端口 |
|------|---------|
| Go `DefaultConfig()` | **1999** ([config.go:77](file:///workspace/internal/config/config.go#L77)) |
| 前端 `DEFAULT_API_BASE_URL` | **2025** ([encv.ts:2](file:///workspace/app/encv-mobile/src/api/encv.ts#L2)) |
| 用户 config.user.json | **2025** ✅ 但未打包进 APK |

同时 `encv start` 命令会拉起 **Admin Server (GoFrame)**：
- 引用了含 MySQL link 的 [config.yaml](file:///workspace/internal/admin/manifest/config/config.yaml)（`mysql:root:12345678@tcp(127.0.0.1:3306)/test`）
- 在 Android 上 MySQL 不存在 → **可能导致 panic/log.Fatal 导致整个进程退出**

### 🟢 Issue #4 [潜在]: Android 明文流量策略

[capacitor.config.ts](file:///workspace/app/encv-mobile/capacitor.config.ts#L8) 设置了 `androidScheme: 'https'`：
- Capacitor WebView 以 `https://localhost` 加载应用
- 前端 fetch 到 `http://127.0.0.1:2025` 属于 **Mixed Content**
- Android 9+ 默认阻止明文 HTTP 流量
- 当前 AndroidManifest.xml 中没有 `network_security_config.xml`

---

## 三、修复方案

### 修复原则

> **不创建轻量级 dev-server**。直接修复 Android 集成链路中的 bug，让正式的 `encv start`（或其移动适配版本）能在 Android 上正确运行。

### Step 1: 修复环境变量名（Bug #1）

**修改文件**: `app/encv-mobile/android-overlay/app/src/main/java/com/encvgo/app/MainActivity.kt`

```kotlin
// 第 31 行: ENVC_CONFIG → ENVC_CONFIG_PATH
pb.environment()["ENCV_CONFIG_PATH"] = configPath
```

### Step 2: 创建移动端专用配置并打包进 APK（Bug #2）

**新建文件**: `app/encv-mobile/assets/config.mobile.json`

```json
{
  "password": "",
  "recover": false,
  "output_path": "/storage/emulated/0/encv-output",
  "plugin_settings": {},
  "server": { "port": 2025, "dir": "/storage/emulated/0" },
  "admin": { "port": 18080, "password": "" },
  "webdav": { "port": 12340, "root": "/webdav/", "dir": "", "username": "", "password": "" },
  "proxy": { "sites": {}, "disable_signature_verification": true },
  "log": { "level": "debug", "file": "", "console": true }
}
```

**修改文件**: `MainActivity.kt` — 启动前将配置从 assets 复制到 filesDir

```kotlin
private fun ensureConfigExists() {
    val dest = File(filesDir, "config.user.json")
    if (!dest.exists()) {
        assets.open("config.mobile.json").use { input ->
            FileOutputStream(dest).use { output ->
                input.copyTo(output)
            }
        }
    }
}
```

在 `startGoDaemon()` 开头调用 `ensureConfigExists()`。

### Step 3: 让 start 命令在移动端跳过 Admin Server（Bug #3）

**方案 A（推荐）**: 在 `servers.go` 的 `startCmd.Run` 中检测移动端环境，跳过 Admin：

```go
// cmd/encv/servers.go startCmd.Run
// 在启动 admin 前检查:
if os.Getenv("ENCV_MOBILE") == "1" || cfg.Admin.Port == 0 {
    log.Println("Mobile mode: skipping admin server")
} else {
    // ... existing admin setup ...
}
```

**同步修改** `MainActivity.kt` 设置环境变量:

```kotlin
pb.environment()["ENCV_MOBILE"] = "1"
```

**方案 B（备选）**: 新增 `encv start-mobile` 子命令，只启动 Backend Server。但用户明确反对新增入口，故优先用方案 A。

### Step 4: 添加 Android 网络安全配置（Issue #4）

**新建文件**: `app/encv-mobile/android-overlay/app/src/main/res/xml/network_security_config.xml`

```xml
<?xml version="1.0" encoding="utf-8"?>
<network-security-config>
    <domain-config cleartextTrafficPermitted="true">
        <domain includeSubdomains="true">127.0.0.1</domain>
        <domain includeSubdomains="true">localhost</domain>
        <domain includeSubdomains="true">10.0.2.2</domain>
    </domain-config>
</network-security-config>
```

> 注: `10.0.2.2` 是 Android 模拟器访问主机 localhost 的特殊 IP。

**修改**: GitHub Actions workflow 中在 AndroidManifest.xml 添加引用:

```xml
<application android:networkSecurityConfig="@xml/network_security_config"
            ... >
```

### Step 5: 更新 CI 构建流程

**修改文件**: `.github/workflows/android.yml`

```yaml
# 在 "Copy Go binary to Android assets" 步骤之后添加:
- name: Copy mobile config to Android assets
  run: |
    mkdir -p app/encv-mobile/android/app/src/main/assets
    cp app/encv-mobile/assets/config.mobile.json app/encv-mobile/android/app/src/main/assets/config.mobile.json
```

---

## 四、验证方法

修复后重新构建 APK 并安装到真机：

1. **日志验证**: 通过 `logcat` 查看 `ENCV-go` tag 的日志
   - 应看到 `ENCV-go daemon started`（进程成功启动）
   - 应看到 `Backend server successfully started`（后端监听成功）
   - 应看到 `Mobile mode: skipping admin server`（如果采用方案 A）

2. **网络验证**: 在手机浏览器访问 `http://127.0.0.1:2025/health`
   - 应返回 `{"status":"ok"}`

3. **APP 验证**: 打开 ENCV-go APP
   - Settings 页面应显示"在线"状态（绿色）
   - Files 页面应能列出文件列表
   - WebSocket 连接应建立成功

---

## 五、关于 Dev 预览支持后端测试

Dev Preview 环境可以运行后台进程，可以直接使用正式的 `encv start` 命令（或编译后的二进制）配合已有的 `config.user.json` 启动后端：

```bash
# Terminal 1: 启动后端（需要 config.user.json）
go run ./cmd/encv/ start &

# Terminal 2: 启动 Vite 前端 + OpenPreview
cd app/encv-mobile && npm run dev
```

但需注意 `start` 会尝试启动 Admin Server（依赖 GoFrame），在沙箱环境中可能因为缺 SQLite/MySQL 而失败。如果遇到此问题，可在沙箱中设置 `ENVC_MOBILE=1` 环境变量来跳过 Admin。
