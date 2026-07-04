# 计划：播放器日志桥接 + Native Module 错误定位 + 收尾清理

## 问题分析

### 问题 1：前端日志无法在主应用 DevLogs 中显示
- **根因**：DevLogs 的 backend 标签通过 WebSocket 接收 Go 后端的 `{type:'log'}` 消息，不是读取 Android logcat
- **现状**：LogBridgeModule 已创建并注册到 LynxView，JS 端 lynxLog 已实现，输出到 logcat tag=`LynxPlayer`
- **缺口**：logcat 输出 ≠ DevLogs 显示，两者无桥梁

### 问题 2：后端（native）日志不显示
- **根因**：同上。所有 Native Module 的 `Log.d/e/i/w()` 都写入 Android logcat，但 DevLogs 不读 logcat
- **用户预期**：native 模块的日志应该自动出现在 DevLogs 中

### 问题 3：Native Module 调用报错（点击播放后）
- **现象**：`Player error: NativeModule: In module 'MpvPlaverModule' method '...` （Toast 截断）
- **原因推测**：Lynx JS 运行时调用 NativeModules 方法时失败
- **当前障碍**：typing.d.ts 缺 LogBridgeModule 声明可能影响编译/运行

### 收尾项
- typing.d.ts 缺 LogBridgeModule 类型声明
- 品红色调试背景色 `#CC0010` 未移除

## 现有架构（关键发现）

Go 后端已有完整的 **slog → WebSocket → DevLogs** 日志管道：

```
slog.Info/Error/Warn("msg")
  → WSLogHandler.Handle()          [internal/server/ws_log_handler.go]
  → WSHub.BroadcastRaw({"type":"log", "data":{level, message, timestamp}})
  → 所有 WS 客户端接收
  → DevLogs.vue onWsMessage()      [type==='log' 分支]
  → 渲染到 backend 标签 ✅
```

**只需一个 HTTP 入口让 Android 端注入日志到这个管道！**

## 解决方案

### 架构流程

```
Lynx Player Activity (独立原生 Activity)
│
├── JS 层 AppComponent.tsx
│   └── lynxLog.info/error()
│       └── NativeModules.LogBridge.log(level, msg)
│           └── LogBridgeModule.@LynxMethod log()
│               └── LogRelay.relay("LynxPlayer", level, msg)
│                   ├── Log.i/d/e/w(tag, msg)          → logcat（保留原有通道）
│                   └── HTTP POST /api/logs {level,msg} → Go 后端 slog → WS → DevLogs ✅
│
├── MpvPlayerModule / GoBackendModule (Kotlin Native Modules)
│   └── 关键节点调用 LogRelay.relay(TAG, level, msg)
│       └── 同上双通道输出
│
Go 后端 (HTTP Server, 同进程)
└── POST /api/logs  (新增端点)
    └── slog[level]("[" + tag + "] " + message)
        └── WSLogHandler 自动桥接到 WebSocket
            └── DevLogs.vue backend 标签渲染 ✅
```

## 实施步骤

### Step 1：更新 typing.d.ts — 添加 LogBridgeModule 类型声明
**文件**：`lynx-player/src/typing.d.ts`
- 在 NativeModules 接口中添加：
  ```typescript
  LogBridgeModule: {
    log(level: string, msg: string, callback: (result: any) => void): void;
  };
  ```

### Step 2：移除调试背景色
**文件**：`android-overlay/.../PlayerActivityLynx.kt` 第 309 行
- 删除 `lynxView?.setBackgroundColor(android.graphics.Color.parseColor("#CC0010"))`

### Step 3：增强错误回调 — 完整显示不被截断
**文件**：`PlayerActivityLynx.kt` 的 `onReceivedError` 和 `onReceivedJSError`
- Toast 改为拼接 summaryMessage + rootCause + getMsg()
- 同时将完整错误通过 LogRelay 发送到 Go 后端 DevLogs

### Step 4：Go 后端 — 添加 /api/logs 端点
**文件**：`internal/server/server.go`
- 在路由注册区（第 131 行附近）添加：
  ```go
  mux.HandleFunc("/api/logs", s.handleAPILogs)
  ```
- 新增 handler 方法：
  ```go
  func (s *Server) handleAPILogs(w http.ResponseWriter, r *http.Request) {
      if r.Method != http.MethodPost {
          http.Error(w, "method not allowed", 405)
          return
      }
      var req struct {
          Level     string `json:"level"`
          Message   string `json:"message"`
          Tag       string `json:"tag,omitempty"`
          Timestamp int64  `json:"timestamp,omitempty"`
      }
      json.NewDecoder(r.Body).Decode(&req)
      msg := req.Message
      if req.Tag != "" { msg = "[" + req.Tag + "] " + msg }
      switch req.Level {
      case "error": slog.Error(msg)
      case "warn":  slog.Warn(msg)
      default:      slog.Info(msg)
      }
      w.WriteHeader(204)
  }
  ```
- **原理**：调用 `slog.Error/Info/Warn()` 后，已注册的 **WSLogHandler** 会自动将日志通过 WebSocket 广播为 `{type:'log', ...}` 格式，DevLogs 直接接收显示

### Step 5：创建 LogRelay 工具类 — 双通道日志中继
**文件**：`android-overlay/.../LogRelay.kt`（新建）
- 单例，提供 `relay(tag: String, level: String, message: String)`
- 通道 1：`Log.d/e/i/w(tag, message)` → logcat（保留原通道不变）
- 通道 2：异步 HTTP POST `http://127.0.0.1:{port}/api/logs` body:`{level, message, tag}` → Go 后端 → slog → WS → DevLogs
- 使用 Handler(Looper.getMainLooper()).post + Thread 异步发送，不阻塞调用方
- port 从 `EncvGoService.lastKnownPort` 获取，<=0 时跳过通道 2
- POST 超时 1000ms，失败静默降级（只丢 WS 通道，logcat 不受影响）

```kotlin
package com.encvgo.app

import android.os.Handler
import android.os.Looper
import android.util.Log
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

class LogRelay private constructor() {
    companion object {
        @Volatile private var instance: LogRelay? = null
        fun get(): LogRelay = instance ?: synchronized(this) { instance ?: LogRelay().also { instance = it } }
    }

    private val handler = Handler(Looper.getMainLooper())

    fun relay(tag: String, level: String, message: String) {
        when (level) {
            "info" -> Log.i(tag, message)
            "error" -> Log.e(tag, message)
            "warn" -> Log.w(tag, message)
            else -> Log.d(tag, message)
        }
        handler.post { sendToBackend(tag, level, message) }
    }

    private fun sendToBackend(tag: String, level: String, message: String) {
        val port = EncvGoService.lastKnownPort
        if (port <= 0) return
        Thread {
            try {
                val url = URL("http://127.0.0.1:$port/api/logs")
                val conn = url.openConnection() as HttpURLConnection
                conn.requestMethod = "POST"
                conn.setRequestProperty("Content-Type", "application/json")
                conn.doOutput = true
                conn.connectTimeout = 1000
                conn.readTimeout = 1000
                val body = JSONObject().apply {
                    put("level", level)
                    put("message", message)
                    put("tag", tag)
                    put("timestamp", System.currentTimeMillis())
                }.toString()
                conn.outputStream.write(body.toByteArray())
                conn.outputStream.flush()
                conn.inputStream.close()
                conn.disconnect()
            } catch (_e: Exception) { }
        }.start()
    }
}
```

### Step 6：改造 LogBridgeModule — 使用 LogRelay
**文件**：`LogBridgeModule.kt`
- `log()` 方法体改为：`LogRelay.get().relay("LynxPlayer", level, msg); callback.onSuccess(null)`

### Step 7：MpvPlayerModule 关键节点接入 LogRelay
**文件**：`MpvPlayerModule.kt`
- 将以下关键位置的 `Log.d/e/i/w(TAG, msg)` 替换为 `LogRelay.get().relay(TAG, level, msg)`：
  - `init` 块：模块创建
  - `ensureMpvInitialized()`：初始化成功/失败
  - `play()`：入口、command 调用前后、异常捕获
  - `attachToLayout()`：成功/失败
  - Surface 回调：surfaceCreated/surfaceDestroyed 成功/失败
  - `dispatchStateChange()`：每次状态变化
  - 所有 catch 块
- **保持 TAG 不变**（="MpvPlayerModule"），方便在 DevLogs 中过滤

### Step 8：GoBackendModule 关键节点接入 LogRelay
**文件**：`GoBackendModule.kt`
- 同步替换关键日志点：init、getBackendStatus、startBackend、getStreamUrl、broadcast receiver
- TAG 保持 ="GoBackendModule"

### Step 9：PlayerActivityLynx 关键节点接入 LogRelay
**文件**：`PlayerActivityLynx.kt`
- onCreate 各步骤、createLynxView 各步骤、LynxViewClient 所有回调
- TAG 保持 ="PlayerActivityLynx"

## 预期效果

| 场景 | logcat (adb logcat) | DevLogs backend 标签 |
|------|---------------------|---------------------|
| JS `lynxLog.info("step1")` | ✅ `I/LynxPlayer: step1` | ✅ `[LynxPlayer] step1` |
| MPV play() 被调用 | ✅ `D/MpvPlayerModule: play url=...` | ✅ `[MpvPlayerModule] play url=...` |
| MPV init 失败 | ✅ `E/MpvPlayerModule: ...` | ✅ `[MpvPlayerModule] ...` (红色) |
| Backend getStreamUrl | ✅ `D/GoBackendModule: ...` | ✅ `[GoBackendModule] ...` |
| LynxView 加载错误 | ✅ `E/LynxPlayerClient: ...` | ✅ `[PlayerActivityLynx] ...` |

## 依赖关系图

```
Step 1 (typing.d.ts) ──┐
Step 2 (去背景色)    ─┤
Step 3 (增强Toast)  ─┤──→ 全部独立，可并行
Step 4 (/api/logs)  ─┤
Step 5 (LogRelay)   ─┘
                         ↓
Step 6 (LogBridgeModule) ──→ 依赖 Step 5
Step 7 (MpvPlayerModule) ──→ 依赖 Step 5
Step 8 (GoBackendModule)  ──→ 依赖 Step 5
Step 9 (PlayerActivity)   ──→ 依赖 Step 5
```

## 风险与降级

| 风险 | 影响 | 降级策略 |
|------|------|----------|
| Go 后端未启动 | port=0，跳过 POST | logcat 通道正常工作 |
| POST 超时/拒绝连接 | WS 通道丢失该条日志 | 异步 Thread + catch 静默，不影响主流程 |
| /api/logs 端点不存在（旧版本） | 同上 | 静默失败，logcat 不受影响 |
| 大量日志导致 POST 频繁 | 极小网络开销 | 仅关键节点接入，非每行日志都 POST |
