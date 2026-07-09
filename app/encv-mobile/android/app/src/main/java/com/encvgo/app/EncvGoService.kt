package com.encvgo.app

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import android.util.Log
import androidx.core.app.NotificationCompat
import java.io.BufferedReader
import java.io.File
import java.io.FileOutputStream
import java.io.InputStreamReader
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean
import org.json.JSONObject

class EncvGoService : Service() {
    companion object {
        private const val TAG = "ENCV-Service"
        private const val CHANNEL_ID = "encv_go_service"
        private const val NOTIFICATION_ID = 1001
        private const val BINARY_NAME = "encv-go"
        private const val DEFAULT_PORT = 2025
        private const val MAX_PORT_SCAN = 10
        private const val START_TIMEOUT_MS = 10_000L
        private const val POLL_INTERVAL_MS = 200L

        const val ACTION_START = "com.encvgo.action.START"
        const val ACTION_STOP = "com.encvgo.action.STOP"
        const val ACTION_RESTART = "com.encvgo.action.RESTART"
        const val ACTION_STATUS = "com.encvgo.action.STATUS"
        const val ACTION_EXTERNAL_START = "com.encvgo.action.EXTERNAL_START"
        const val ACTION_EXTERNAL_RESTART = "com.encvgo.action.EXTERNAL_RESTART"

        const val BROADCAST_BACKEND_READY = "com.encvgo.broadcast.BACKEND_READY"
        const val BROADCAST_BACKEND_STATUS = "com.encvgo.broadcast.BACKEND_STATUS"
        const val BROADCAST_EXTERNAL_RESULT = "com.encvgo.broadcast.EXTERNAL_RESULT"

        const val EXTRA_PORT = "port"
        const val EXTRA_ERROR = "error"
        const val EXTRA_RUNNING = "running"
        const val EXTRA_SOURCE = "source"
        const val EXTRA_COMMAND = "command"

        @Volatile
        var lastKnownPort: Int = 0

        @Volatile
        var isRunning: Boolean = false

        @Volatile
        var lastError: String? = null

        fun createIntent(context: Context, action: String, source: String = "manual"): Intent {
            return Intent(context, EncvGoService::class.java).apply {
                this.action = action
                putExtra(EXTRA_SOURCE, source)
            }
        }

        @Volatile
        private var instance: EncvGoService? = null

        fun getOutputSnapshot(): String {
            val svc = instance ?: return ""
            return synchronized(svc.outputBuffer) {
                svc.outputBuffer.toString()
            }
        }

        fun clearOutputSnapshot() {
            val svc = instance ?: return
            synchronized(svc.outputBuffer) {
                svc.outputBuffer.clear()
            }
        }
    }

    private val worker = Executors.newSingleThreadExecutor()
    private val startupGeneration = java.util.concurrent.atomic.AtomicInteger(0)
    private val readyKeywords = listOf("listening on", "server ready", "ready", "started")

    private var goProcess: Process? = null
    private var currentPort = DEFAULT_PORT
    private var configPort = DEFAULT_PORT
    private var currentSource = "manual"
    private var outputBuffer = StringBuilder()
    private var lastExitCode: Int? = null
    private val processReady = AtomicBoolean(false)
    private var wakeLock: PowerManager.WakeLock? = null

    // 🆕 2026-06-14：删除 heartbeatFile 字段
    //
    // 历史（重构成因）：
    //   - 2026-06-12 Phase 4 引入文件版心跳：Go 写 .encv_heartbeat，Kotlin 1s poll mtime
    //   - 2026-06-14 路径 bug：Kotlin / Go 读不同文件 → 7s 必死
    //   - 2026-06-14 改 HTTP /health JSON 探活（见 checkHeartbeatOk()）
    //   - 2026-06-14 全部删除：不再需要文件、不再需要 ENCV_HEARTBEAT_PATH env
    //                     不再需要 resolvedHeartbeatFile() / resolveServingDir() / 探写降级
    //
    // 见 spec/cross-process-ipc-refactor/spec.md §3.3, §3.5

    // 🆕 2026-06-12 崩溃根因修复：goProcess 死时自动重启（带指数退避 + 最多 3 次）
    //   死因：真机 libffmpeg.so 没编 encoder → go_GenerateMP3 走 NativeRunner → cgo panic
    //         cgo panic 跨 cgo boundary 杀整个进程 → 之前前端只看到 "Failed to fetch"
    //   修复：后台线程 1s poll process.isAlive；死 → publishFailure(完整 stderr) → 重启
    private var restartAttempts = 0
    private val MAX_RESTART_ATTEMPTS = 3
    private val restartExecutor = java.util.concurrent.Executors.newSingleThreadScheduledExecutor { r ->
        Thread(r, "encv-go-restart").apply { isDaemon = true }
    }

    // 🆕 2026-06-14：HTTP /health 连续失败计数器
    //
    // 用法：checkHeartbeatOk() 返回 false 时 +1，返回 true 时清零。
    // 触发 hang 阈值：>= 2（连续 2 次失败 ≈ 2s 抖动容差）。
    //
    // 为什么用计数：HTTP 偶发超时（网络栈 / WebView 切换 / OS doze）很常见，
    // 单次失败就 kill 进程会太激进。
    private var httpFailCount = 0

    override fun onCreate() {
        super.onCreate()
        instance = this
        createNotificationChannel()
        startProcessAliveMonitor()  // 🆕 2026-06-12：进程死后自动重启
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        currentSource = intent?.getStringExtra(EXTRA_SOURCE) ?: "manual"
        intent?.let { it ->
            when (it.action) {
                ACTION_STOP -> worker.execute { stopGoProcess("stopped", stopService = true) }
                ACTION_RESTART, ACTION_EXTERNAL_RESTART -> worker.execute {
                    restartGoProcess(currentSource, it.getStringExtra(EXTRA_COMMAND))
                }
                ACTION_STATUS -> publishStatus(lastError)
                ACTION_START, ACTION_EXTERNAL_START -> worker.execute {
                    startGoProcess(currentSource, it.getStringExtra(EXTRA_COMMAND))
                }
            }
        } ?: run {
            // 无 intent 视为默认 start（向后兼容旧版无 action 调用）
            worker.execute { startGoProcess(currentSource, null) }
        }
        return START_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        instance = null
        stopGoProcess("service_destroyed", stopService = false)
        worker.shutdownNow()
        restartExecutor.shutdownNow()  // 🆕 2026-06-12
        super.onDestroy()
    }

    // 🆕 2026-06-14 简化：HTTP /health JSON 探活（单一策略）
    //
    // 旧实现有 if-else 灰度（HTTP / mtime），验证后切到 HTTP-only 并删 mtime 分支。
    //
    // 行为：
    //   1) Process.isAlive()=false → 进程真死（exit/crash）→ 老逻辑处理
    //   2) checkHeartbeatOk()=false 连续 2 次（约 2s）→ 进程 alive 但内部 hang
    //      （cgo ffmpeg_run 阻塞 OS thread 不响应 ctx cancel）→ kill + restart
    //   3) checkHeartbeatOk()=true → 健康，重置 httpFailCount 和 restartAttempts
    //
    // 触发后 publishFailure reason="http_health_failed_Nx_in_2s"（前端按 go_hang 分类）
    private fun startProcessAliveMonitor() {
        restartExecutor.scheduleWithFixedDelay({
            val proc = goProcess ?: return@scheduleWithFixedDelay
            val wasReady = processReady.get()
            if (!wasReady) return@scheduleWithFixedDelay  // 启动阶段失败由 startGoProcess 自己处理

            // ① 进程真死 → 老逻辑
            if (!proc.isAlive) {
                processReady.set(false)
                restartAttempts += 1
                val exitCode = try { proc.exitValue() } catch (e: Exception) { -1 }
                val tail = outputBuffer.toString().takeLast(2 * 1024)
                val reason = "go_exit:$exitCode|attempts=$restartAttempts|output:${if (tail.isEmpty()) "(empty)" else tail}"
                publishFailure(reason, "alive_monitor", null)
                lastExitCode = exitCode
                scheduleAutoRestart(reason)
                return@scheduleWithFixedDelay
            }

            // ② HTTP /health JSON 探活
            val httpOk = checkHeartbeatOk()
            if (httpOk) {
                httpFailCount = 0
            } else {
                httpFailCount += 1
                // 容忍 1 次抖动，连续 2 次失败（≈2s）才判 hang
                if (httpFailCount >= 2) {
                    handleHang("http_health_failed_${httpFailCount}x_in_2s")
                    return@scheduleWithFixedDelay
                }
            }

            // ③ 进程健康 — 重置重启计数
            if (restartAttempts > 0) {
                restartAttempts = 0
            }
        }, 1, 1, java.util.concurrent.TimeUnit.SECONDS)
    }

    // 🆕 2026-06-12 Phase 4：mtime 探活触发 hang 处理
    //   - 不依赖 isAlive()（cgo hang 时仍 true）
    //   - 强杀 + 1s/2s/4s 退避重启
    //   - 通过 publishFailure 推 CustomEvent('encv:backend-status') 给前端
    private fun handleHang(reason: String) {
        processReady.set(false)
        restartAttempts += 1
        val tail = outputBuffer.toString().takeLast(2 * 1024)
        val fullReason = "go_hang:$reason|attempts=$restartAttempts|output:${if (tail.isEmpty()) "(empty)" else tail}"
        Log.e(TAG, "⚠️  Backend hang detected: $reason — destroying and scheduling restart")
        publishFailure(fullReason, "alive_monitor", null)
        try {
            goProcess?.destroyForcibly()
        } catch (e: Throwable) {
            Log.w(TAG, "destroyForcibly failed", e)
        }
        scheduleAutoRestart(fullReason)
    }

    // 提取自 startProcessAliveMonitor，复用于 go_exit / go_hang 两条路径
    private fun scheduleAutoRestart(reason: String) {
        if (restartAttempts > MAX_RESTART_ATTEMPTS) {
            Log.e(TAG, "❌ Go process died/hung $restartAttempts times; giving up auto-restart")
            return
        }
        val delayMs = (1L shl (restartAttempts - 1)) * 1000L  // 1s / 2s / 4s
        Log.w(TAG, "Auto-restart in ${delayMs}ms (attempt ${restartAttempts}/${MAX_RESTART_ATTEMPTS}): $reason")
        restartExecutor.schedule({
            try {
                startGoProcess("auto_restart:$reason", null)
            } catch (e: Throwable) {
                Log.e(TAG, "auto-restart failed", e)
            }
        }, delayMs, java.util.concurrent.TimeUnit.MILLISECONDS)
    }

    // 🆕 2026-06-14：删除 touchHeartbeat() 方法（之前在 startGoProcess 末尾调用）。
    // 心跳改 HTTP /health 探活，Kotlin 不再需要 touch 心跳文件。
    // 见 spec/cross-process-ipc-refactor/spec.md §3.3

    /**
     * 🆕 2026-06-14：HTTP /health JSON 心跳探活
     *
     * 替代 mtime 文件探活的设计原则：
     *   - 单一来源：Go 端 `startHeartbeatLoopInMemory` 每 2s 写 atomic.Int64
     *   - HTTP /health JSON 包含 heartbeat_ok / heartbeat_age_ms 字段
     *   - parent (Kotlin) 通过 GET /health 读，无需文件、无需 env 协商
     *
     * 调用方：startProcessAliveMonitor
     * 行为：
     *   - 1s 超时
     *   - HTTP 200 + heartbeat_ok=true → 进程健康
     *   - HTTP 200 + heartbeat_ok=false → 进程 alive 但内部 hang（cgo 阻塞等）
     *   - 非 200 / 连接失败 / 超时 / 解析失败 → 视为失败
     *
     * 端口：传 0 表示用 lastKnownPort；传非 0 表示用指定端口（启动期）
     *
     * 行业参考：Android Studio、VS Code、Firebase CLI 全部用 HTTP /health
     * 见 spec/cross-process-ipc-refactor/spec.md §3
     */
    private fun checkHeartbeatOk(port: Int = 0): Boolean {
        val p = if (port > 0) port else lastKnownPort
        if (p <= 0) {
            // 还没确定端口（启动早期）→ 不算 hang
            return true
        }
        return try {
            val conn = URL("http://127.0.0.1:$p/health").openConnection() as HttpURLConnection
            conn.connectTimeout = 1_000
            conn.readTimeout = 1_000
            conn.requestMethod = "GET"
            conn.setRequestProperty("Connection", "close")  // 避免 keep-alive 占用连接
            val code = conn.responseCode
            if (code != 200) {
                Log.d(TAG, "checkHeartbeatOk: HTTP $code from /health")
                conn.disconnect()
                return false
            }
            val body = conn.inputStream.bufferedReader().use { it.readText() }
            conn.disconnect()
            val json = JSONObject(body)
            val ok = json.optBoolean("heartbeat_ok", false)
            if (!ok) {
                val age = json.optLong("heartbeat_age_ms", -1L)
                Log.w(TAG, "checkHeartbeatOk: heartbeat_stale (age=${age}ms)")
            }
            ok
        } catch (e: Throwable) {
            Log.d(TAG, "checkHeartbeatOk: connection failed (port=$p) — ${e.javaClass.simpleName}: ${e.message}")
            false
        }
    }

    // 🆕 2026-06-14：startGoProcess 启动 Go 子进程
    //
    // 重构后只剩 5 个 env var（删了 ENCV_SERVING_DIR、ENCV_HEARTBEAT_PATH）：
    //   - ENCV_CONFIG_PATH   — Kotlin 拥有，告诉 Go 读哪个 config
    //   - ENCV_MOBILE=1      — Go 行为开关（启 mobile overlay）
    //   - HOME               — Go 找 user home
    //   - ENCV_LIB_DIR       — Kotlin 显式知道 native lib 位置
    //   - ENCV_FFMPEG_WORKER — 同上（ffmpeg worker 路径）
    //
    // 不再设：
    //   - ENCV_SERVING_DIR    — Go 自己决定（mobile overlay + os.Getwd() fallback）
    //   - ENCV_HEARTBEAT_PATH — 心跳全在内存（Server.startHeartbeatLoopInMemory + HTTP /health）
    //
    // 见 spec/cross-process-ipc-refactor/spec.md §3.5
    private fun startGoProcess(source: String, command: String?) {
        if (goProcess?.isAlive == true && processReady.get()) {
            publishStatus(null, source, command)
            return
        }

        startForeground(NOTIFICATION_ID, buildNotification("后端启动中"))
        acquireWakeLock()
        resetStateForStart(source)
        GoProcessPlugin.pushKotlinLog("warn", TAG, "Go backend startup initiated by: $source")

        try {
            ensureConfigExists()
            ensureBuildInfoExists()
            configPort = readConfigPort()
            val binary = findExecutableBinary() ?: run {
                GoProcessPlugin.pushKotlinLog("error", TAG, "Go backend start failed: no binary found")
                publishFailure("no_binary", source, command)
                return
            }

            val configPath = File(filesDir, "config.user.json").absolutePath
            Log.i(TAG, "Starting backend: ${binary.absolutePath} start")
            GoProcessPlugin.pushKotlinLog("info", TAG, "Starting Go binary: ${binary.absolutePath}")

            // 🆕 2026-07-04：预检 ObjectBox 依赖。
            // Go 二进制如果以 -tags objectbox 编译，运行时需要 libobjectbox-jni.so。
            // 若 APK 内未打包该 .so，Android linker 会直接拒绝执行（CANNOT LINK EXECUTABLE）。
            // 此处提前检测并给出清晰错误，同时尝试改 config 为 sqlite。
            checkObjectBoxNativeLib()

            // 🆕 2026-06-14：先探测 servingDir 是否可写，不可写则降级到 filesDir。
            //
            // 背景：Android 11+ scoped storage 限制 + 真机用户拒绝存储权限 →
            //   /storage/emulated/0 写不了（即使是 app 自己的目录）→ Go 端
            //   servingDir/.encv_heartbeat 写失败 → 即使 mtime 路径一致 Go 也写不了。
            //
            // 防御：每次启动 Go 前探写一次，失败就改写 config.user.json 的
            //   mobile.server.dir 到 filesDir（永远可写、无需权限），并把同
            //   路径也写进 ENCV_HEARTBEAT_PATH。
            // 🆕 2026-06-14：Kotlin 不再设 ENCV_SERVING_DIR / ENCV_HEARTBEAT_PATH 给 Go。
            //
            // 历史：Kotlin 用 resolveServingDir() 探测 /storage/emulated/0 可写性，
            //       失败时降级到 filesDir 并写 config.user.json。
            //       Go 端有 mobile overlay 自己会决定 servingDir。
            //
            // 新：Kotlin 不管 servingDir（Go 自己处理），零文件依赖，零 env 协商。
            //     如果需要知道 Go 在用哪个目录，调 GET /api/runtime 读 serving_dir 字段。
            //     见 spec/cross-process-ipc-refactor/spec.md §3.5
            //
            // 【防回归】不要重新引入：
            //   - resolveServingDir() — 探测并改写 config
            //   - heartbeatFile — 文件版心跳
            //   - touchHeartbeat() — 启动时 touch 文件
            //   - ENCV_HEARTBEAT_PATH env — Go 端不再读
            //   - ENCV_SERVING_DIR env — Go 端不再读

            goProcess = ProcessBuilder(binary.absolutePath, "start").apply {
                environment()["ENCV_CONFIG_PATH"] = configPath
                environment()["ENCV_MOBILE"] = "1"
                environment()["HOME"] = filesDir.absolutePath
                // 显式告诉 Go 端 app 私有文件目录（mount 系统 + 日志持久化需要）
                // 不依赖 appdata.go 的硬编码 fallback，确保路径 100% 正确
                environment()["ENCV_APP_FILES_DIR"] = filesDir.absolutePath
                // - ENCV_LIB_DIR 给 cgo CallFFmpegNative 用（dlopen libffmpeg.so）
                // - ENCV_FFMPEG_WORKER 给 ffmpeg worker 路径用（workerClient.locateWorker 优先选这个）
                //   改用 subprocess worker 调 ffmpeg 后，父进程 ctx cancel 时可以 SIGKILL worker
                //   解锁（之前 in-process cgo 阻塞 OS thread 没法 cancel，hang spinner forever）
                environment()["ENCV_LIB_DIR"] = applicationInfo.nativeLibraryDir
                // 🆕 2026-07-04：LD_LIBRARY_PATH 让 Android linker 能找到子进程的 .so。
                // Go 二进制 (libencv-go.so) 通过 ProcessBuilder 直接 exec 执行，
                // 不是用 System.loadLibrary 加载。linker64 解析 DT_NEEDED 时默认
                // 不搜应用私有 native lib 目录，必须显式设置 LD_LIBRARY_PATH。
                // 否则 libobjectbox-jni.so、libsql_experimental.so 等会“not found”。
                val oldLd = environment()["LD_LIBRARY_PATH"]?.takeIf { it.isNotBlank() }
                environment()["LD_LIBRARY_PATH"] = listOfNotNull(oldLd, applicationInfo.nativeLibraryDir).joinToString(":")
                environment()["ENCV_FFMPEG_WORKER"] =
                    File(applicationInfo.nativeLibraryDir, "libffmpeg-worker.so").absolutePath
                // 🆕 2026-06-14：删除 ENCV_SERVING_DIR / ENCV_HEARTBEAT_PATH
                // Go 自己决定 servingDir（mobile overlay + os.Getwd() fallback）
                // 心跳全在内存（Server.startHeartbeatLoopInMemory + HTTP /health）
                // 见 spec/cross-process-ipc-refactor/spec.md §3
                redirectErrorStream(true)
                directory(filesDir)
            }.start()

            monitorProcessOutput(startupGeneration.incrementAndGet(), source, command)
            waitForBackendReady(startupGeneration.get(), source, command)
        } catch (e: Exception) {
            Log.e(TAG, "Failed to start backend", e)
            GoProcessPlugin.pushKotlinLog("error", TAG, "Start exception: ${e.message}")
            publishFailure("start_failed:${e.message ?: "unknown"}", source, command)
        }
    }

    private fun restartGoProcess(source: String, command: String?) {
        stopGoProcess("restarting", stopService = false)
        startGoProcess(source, command)
    }

    private fun stopGoProcess(errorMessage: String?, stopService: Boolean) {
        processReady.set(false)
        isRunning = false
        lastKnownPort = 0
        currentPort = DEFAULT_PORT
        lastError = errorMessage
        releaseWakeLock()

        goProcess?.let {
            try {
                if (it.isAlive) {
                    it.destroyForcibly()
                    Log.i(TAG, "Backend process stopped")
                }
            } catch (e: Exception) {
                Log.w(TAG, "Failed to stop backend process", e)
            }
        }
        goProcess = null
        outputBuffer = StringBuilder()
        updateNotification(if (stopService) "后端已停止" else "后端重启中")
        publishStatus(errorMessage, currentSource, if (stopService) ACTION_STOP else null)

        if (stopService) {
            sendExternalResult(true, null, currentSource, "stop")
            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
        }
    }

    private fun monitorProcessOutput(generation: Int, source: String, command: String?) {
        Thread {
            try {
                val reader = BufferedReader(InputStreamReader(goProcess?.inputStream))
                var line: String?
                while (reader.readLine().also { line = it } != null) {
                    val content = line ?: continue
                    Log.i(TAG, "[go] $content")
                    synchronized(outputBuffer) {
                        outputBuffer.append(content).append('\n')
                    }
                    // 🆕 2026-07-04：将 Go 进程 stderr 通过 notifyListeners 实时推送到前端 DevLogs
                    if (content.contains("error", ignoreCase = true) ||
                        content.contains("fatal", ignoreCase = true) ||
                        content.contains("panic", ignoreCase = true) ||
                        content.contains("permission denied", ignoreCase = true)) {
                        lastError = "go_error:$content"
                        publishStatus(lastError, source, command)
                        GoProcessPlugin.pushKotlinLog("error", TAG, "[go] $content")
                    } else if (content.contains("warn", ignoreCase = true) ||
                               content.contains("failed", ignoreCase = true)) {
                        GoProcessPlugin.pushKotlinLog("warn", TAG, "[go] $content")
                    } else {
                        GoProcessPlugin.pushKotlinLog("info", TAG, "[go] $content")
                    }
                    if (!processReady.get() && readyKeywords.any { content.contains(it, ignoreCase = true) }) {
                        maybeMarkReady(generation, source, command)
                    }
                }

                lastExitCode = goProcess?.waitFor() ?: -1
                Log.w(TAG, "Backend exited with code: $lastExitCode")
                if (!processReady.get()) {
                    publishFailure("go_exit:${lastExitCode ?: -1}", source, command)
                } else {
                    isRunning = false
                    lastKnownPort = 0
                    publishStatus("go_exit:${lastExitCode ?: -1}", source, command)
                    updateNotification("后端已退出")
                }
            } catch (e: Exception) {
                Log.w(TAG, "Error monitoring backend output", e)
                if (!processReady.get()) {
                    publishFailure("monitor_error:${e.message ?: "unknown"}", source, command)
                }
            }
        }.start()
    }

    private fun waitForBackendReady(generation: Int, source: String, command: String?) {
        Thread {
            val startAt = System.currentTimeMillis()
            while (System.currentTimeMillis() - startAt < START_TIMEOUT_MS) {
                if (generation != startupGeneration.get()) return@Thread
                if (processReady.get()) return@Thread
                for (offset in 0..MAX_PORT_SCAN) {
                    val port = configPort + offset
                    if (checkHealth(port)) {
                        currentPort = port
                        maybeMarkReady(generation, source, command)
                        return@Thread
                    }
                }
                Thread.sleep(POLL_INTERVAL_MS)
            }

            if (!processReady.get()) {
                val tail = synchronized(outputBuffer) {
                    outputBuffer.takeLast(1000).toString().trim()
                }
                val exitInfo = lastExitCode?.let { "exit=$it" } ?: if (goProcess?.isAlive == true) "alive=true" else "alive=false"
                publishFailure("timeout:$exitInfo|output:${if (tail.isEmpty()) "(empty)" else tail}", source, command)
            }
        }.start()
    }

    @Synchronized
    private fun maybeMarkReady(generation: Int, source: String, command: String?) {
        if (generation != startupGeneration.get() || processReady.get()) return

        if (currentPort == DEFAULT_PORT) {
            for (offset in 0..MAX_PORT_SCAN) {
                val port = configPort + offset
                if (checkHealth(port)) {
                    currentPort = port
                    break
                }
            }
        }

        processReady.set(true)
        isRunning = true
        lastKnownPort = currentPort
        lastError = null
        updateNotification("后端已就绪 :$currentPort")
        publishReady(source, command)
    }

    private fun publishReady(source: String, command: String?) {
        val readyIntent = Intent(BROADCAST_BACKEND_READY).apply {
            putExtra(EXTRA_PORT, currentPort)
            putExtra(EXTRA_RUNNING, true)
            putExtra(EXTRA_SOURCE, source)
            putExtra(EXTRA_COMMAND, command)
        }
        sendBroadcast(readyIntent)

        publishStatus(null, source, command)
        sendExternalResult(true, null, source, command)
    }

    private fun publishFailure(error: String, source: String, command: String?) {
        lastError = error
        isRunning = false
        processReady.set(false)
        lastKnownPort = 0
        updateNotification("后端启动失败")
        publishStatus(error, source, command)
        sendExternalResult(false, error, source, command)
        GoProcessPlugin.pushKotlinLog("error", TAG, "Backend failure: $error (source=$source)")
    }

    private fun publishStatus(error: String?, source: String = currentSource, command: String? = null) {
        val statusIntent = Intent(BROADCAST_BACKEND_STATUS).apply {
            putExtra(EXTRA_PORT, if (isRunning) currentPort else 0)
            putExtra(EXTRA_RUNNING, isRunning)
            putExtra(EXTRA_SOURCE, source)
            putExtra(EXTRA_COMMAND, command)
            if (!error.isNullOrEmpty()) {
                putExtra(EXTRA_ERROR, error)
            }
        }
        sendBroadcast(statusIntent)
    }

    private fun sendExternalResult(success: Boolean, error: String?, source: String, command: String?) {
        val intent = Intent(BROADCAST_EXTERNAL_RESULT).apply {
            putExtra("success", success)
            putExtra(EXTRA_PORT, if (success) currentPort else 0)
            putExtra(EXTRA_RUNNING, success)
            putExtra(EXTRA_SOURCE, source)
            putExtra(EXTRA_COMMAND, command)
            if (!error.isNullOrEmpty()) {
                putExtra(EXTRA_ERROR, error)
            }
        }
        sendBroadcast(intent)
    }

    private fun resetStateForStart(source: String) {
        currentSource = source
        currentPort = DEFAULT_PORT
        lastKnownPort = 0
        isRunning = false
        lastError = null
        lastExitCode = null
        processReady.set(false)
        outputBuffer = StringBuilder()
    }

    private fun buildNotification(text: String): Notification {
        val openIntent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        val pendingIntent = PendingIntent.getActivity(
            this,
            0,
            openIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.ic_dialog_info)
            .setContentTitle("ENCV-go")
            .setContentText(text)
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .build()
    }

    private fun updateNotification(text: String) {
        val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        manager.notify(NOTIFICATION_ID, buildNotification(text))
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val manager = getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            val channel = NotificationChannel(
                CHANNEL_ID,
                "ENCV 后端服务",
                NotificationManager.IMPORTANCE_LOW
            )
            manager.createNotificationChannel(channel)
        }
    }

    private fun readConfigPort(): Int {
        return try {
            val configFile = File(filesDir, "config.user.json")
            if (!configFile.exists()) return DEFAULT_PORT
            val jsonObj = JSONObject(configFile.readText())
            jsonObj.optJSONObject("server")?.optInt("port", DEFAULT_PORT) ?: DEFAULT_PORT
        } catch (e: Exception) {
            Log.w(TAG, "Failed to read config port", e)
            DEFAULT_PORT
        }
    }

    private fun ensureConfigExists() {
        val dest = File(filesDir, "config.user.json")
        if (dest.exists()) {
            mergeConfigDefaults(dest)
            return
        }
        copyDefaultConfig(dest)
    }

    private fun ensureBuildInfoExists() {
        val dest = File(filesDir, "build-info.json")
        if (dest.exists()) return
        try {
            assets.open("build-info.json").use { input ->
                FileOutputStream(dest).use { output ->
                    input.copyTo(output)
                }
            }
            Log.i(TAG, "Copied build-info.json to filesDir")
        } catch (e: Exception) {
            Log.w(TAG, "Failed to copy build-info.json", e)
        }
    }

    private fun copyDefaultConfig(dest: File) {
        try {
            assets.open("config.user.json").use { input ->
                FileOutputStream(dest).use { output ->
                    input.copyTo(output)
                }
            }
            Log.i(TAG, "Default config copied to ${dest.absolutePath}")
        } catch (e: Exception) {
            Log.e(TAG, "Failed to copy default config", e)
            writeFallbackConfig(dest)
        }
    }

    private fun mergeConfigDefaults(dest: File) {
        try {
            val existing = JSONObject(dest.readText())
            var changed = false

            val defaults = try {
                JSONObject(assets.open("config.user.json").bufferedReader().use { it.readText() })
            } catch (e: Exception) {
                Log.w(TAG, "Cannot read default config for merge", e)
                return
            }

            val serverObj = existing.optJSONObject("server")
            val defaultServer = defaults.optJSONObject("server")
            if (serverObj != null && defaultServer != null) {
                if (!serverObj.has("port")) {
                    serverObj.put("port", defaultServer.optInt("port", DEFAULT_PORT))
                    changed = true
                }
                if (!serverObj.has("dir")) {
                    serverObj.put("dir", defaultServer.optString("dir", ""))
                    changed = true
                }
            }

            if (!existing.has("password")) {
                existing.put("password", defaults.optString("password", ""))
                changed = true
            }
            if (!existing.has("output_path")) {
                existing.put("output_path", defaults.optString("output_path", "/storage/emulated/0/encv-output"))
                changed = true
            }
            if (!existing.has("plugin_settings")) {
                existing.put("plugin_settings", defaults.optJSONObject("plugin_settings") ?: JSONObject())
                changed = true
            }
            if (!existing.has("log")) {
                existing.put("log", defaults.optJSONObject("log") ?: JSONObject().put("level", "info").put("console", true))
                changed = true
            }

            val existingMobile = existing.optJSONObject("mobile")
            val defaultMobile = defaults.optJSONObject("mobile")
            if (defaultMobile != null) {
                val targetMobile = existingMobile ?: JSONObject().also {
                    existing.put("mobile", it)
                    changed = true
                }
                if (!targetMobile.has("server")) {
                    targetMobile.put("server", defaultMobile.optJSONObject("server") ?: JSONObject())
                    changed = true
                }
                if (!targetMobile.has("output")) {
                    targetMobile.put("output", defaultMobile.optJSONObject("output") ?: JSONObject())
                    changed = true
                }
                if (!targetMobile.has("webdav")) {
                    targetMobile.put("webdav", defaultMobile.optJSONObject("webdav") ?: JSONObject())
                    changed = true
                }
            }

            if (!existing.has("recover")) {
                existing.put("recover", defaults.optBoolean("recover", false))
                changed = true
            }
            if (!existing.has("default_container_version")) {
                existing.put("default_container_version", defaults.optInt("default_container_version", 4))
                changed = true
            }
            if (!existing.has("admin")) {
                existing.put("admin", defaults.optJSONObject("admin") ?: JSONObject().put("password", ""))
                changed = true
            }
            if (!existing.has("webdav")) {
                val defaultWebdav = defaults.optJSONObject("webdav")
                existing.put("webdav", defaultWebdav ?: JSONObject().put("root", "").put("dir", "").put("username", "").put("password", ""))
                changed = true
            }
            if (!existing.has("proxy")) {
                val defaultProxy = defaults.optJSONObject("proxy")
                existing.put("proxy", defaultProxy ?: JSONObject().put("sites", JSONObject()).put("disable_signature_verification", true))
                changed = true
            }

            if (changed) {
                dest.writeText(existing.toString(2))
                Log.i(TAG, "Config merged with new defaults")
            }
        } catch (e: Exception) {
            Log.w(TAG, "Failed to merge config defaults", e)
        }
    }

    private fun writeFallbackConfig(dest: File) {
        val fallback = JSONObject().apply {
            put("password", "")
            put("recover", false)
            put("default_container_version", 4)
            put("output_path", "/storage/emulated/0/encv-output")
            put("server", JSONObject().put("port", DEFAULT_PORT).put("dir", "/storage/emulated/0"))
            put("admin", JSONObject().put("password", ""))
            put("webdav", JSONObject().put("root", "").put("dir", "").put("username", "").put("password", ""))
            put("proxy", JSONObject().put("sites", JSONObject()).put("disable_signature_verification", true))
            put("plugin_settings", JSONObject())
            put("log", JSONObject().put("level", "info").put("file", "").put("console", true))
            put("mobile", JSONObject().apply {
                put("server", JSONObject().put("dir", "/storage/emulated/0"))
                put("output", JSONObject().put("path", "/storage/emulated/0/encv-output"))
                put("webdav", JSONObject().put("dir", ""))
            })
        }
        dest.writeText(fallback.toString(2))
        Log.i(TAG, "Fallback config written to ${dest.absolutePath}")
    }

    private fun findExecutableBinary(): File? {
        val nativeLibDir = applicationInfo.nativeLibraryDir
        Log.i(TAG, "nativeLibraryDir: $nativeLibDir")

        val nativeBinary = File(nativeLibDir, "libencv-go.so")
        Log.i(TAG, "Checking native binary: exists=${nativeBinary.exists()}, canExecute=${nativeBinary.canExecute()}, path=${nativeBinary.absolutePath}")

        if (nativeBinary.exists() && nativeBinary.canExecute()) {
            Log.i(TAG, "Using binary from nativeLibraryDir: ${nativeBinary.absolutePath}")
            return nativeBinary
        }

        val libDir = File(nativeLibDir)
        if (libDir.exists()) {
            libDir.listFiles()?.forEach { f ->
                Log.i(TAG, "  lib dir entry: ${f.name} exe=${f.canExecute()}")
            }
        } else {
            Log.w(TAG, "nativeLibraryDir does not exist: $nativeLibDir")
        }

        Log.w(TAG, "nativeLibraryDir lookup failed, falling back to filesDir (may fail on Android 10+)")

        val candidateDirs = listOf(
            filesDir to "filesDir",
            cacheDir to "cacheDir",
            getExternalFilesDir(null) to "externalFilesDir",
        )

        for ((dir, name) in candidateDirs) {
            if (dir == null) continue
            val binary = File(dir, BINARY_NAME)
            if (!binary.exists()) {
                copyBinaryFromAssets(binary)
            }
            binary.setReadable(true)
            binary.setExecutable(true)
            binary.setWritable(true)
            if (binary.canExecute()) {
                Log.i(TAG, "Using binary from $name: ${binary.absolutePath}")
                return binary
            }
        }
        return null
    }

    private fun copyBinaryFromAssets(dest: File) {
        dest.parentFile?.mkdirs()
        try {
            assets.open(BINARY_NAME).use { input ->
                FileOutputStream(dest).use { output ->
                    val buffer = ByteArray(8192)
                    var len: Int
                    while (input.read(buffer).also { len = it } != -1) {
                        output.write(buffer, 0, len)
                    }
                }
            }
        } catch (e: Exception) {
            Log.w(TAG, "Binary not found in assets (expected on Android 10+ with jniLibs packaging)", e)
        }
    }

    private fun checkHealth(port: Int): Boolean {
        return try {
            val conn = URL("http://127.0.0.1:$port/health").openConnection() as HttpURLConnection
            conn.connectTimeout = 300
            conn.readTimeout = 300
            val code = conn.responseCode
            conn.disconnect()
            code == 200
        } catch (_: Exception) {
            false
        }
    }

    private fun acquireWakeLock() {
        if (wakeLock?.isHeld == true) return
        try {
            val pm = getSystemService(Context.POWER_SERVICE) as PowerManager
            wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "encvgo::GoService")
            wakeLock?.acquire()
            Log.i(TAG, "WakeLock acquired")
        } catch (e: Exception) {
            Log.w(TAG, "Failed to acquire WakeLock", e)
        }
    }

    private fun releaseWakeLock() {
        wakeLock?.let {
            if (it.isHeld) {
                it.release()
                Log.i(TAG, "WakeLock released")
            }
        }
        wakeLock = null
    }

    /**
     * 🆕 2026-07-04：预检 Go 二进制需要的 native 依赖。
     *
     * Go 二进制如果以 `-tags objectbox` 编译，Android linker 会在 exec 时
     * 立即解析 libobjectbox-jni.so，缺失则直接拒绝执行（CANNOT LINK EXECUTABLE）。
     *
     * 该检查无法从代码层面修复构建问题，但能做到：
     * 1. 提前发现并推送清晰错误到 DevLogs
     * 2. 尝试改写 config.user.json 为 sqlite（虽然二进制加载时已失败，但为后续修复后的启动做准备）
     * 3. 防止用户反复重试时每次都是同一个 cryptic linker 错误
     */
    private fun checkObjectBoxNativeLib() {
        val nativeLibDir = applicationInfo.nativeLibraryDir
        val objectBoxJni = File(nativeLibDir, "libobjectbox-jni.so")
        if (objectBoxJni.exists()) return  // 正常，lib 已打包

        // libobjectbox-jni.so 不存在 → Go 二进制无法 exec
        val msg = "预检失败：APK 缺少 libobjectbox-jni.so！" +
            " Go 二进制以 -tags objectbox 编译，但 APK 未打包该 native lib。" +
            " 修复：./scripts/build-objectbox-android.sh 下载后重新编译 Go，" +
            " 或将 libobjectbox-jni.so 放入 android/app/src/main/jniLibs/<arch>/"
        Log.e(TAG, msg)
        GoProcessPlugin.pushKotlinLog("error", TAG, msg)

        // 尝试改写 config 为 sqlite，为旧版二进制或不带 objectbox 的二进制做准备
        try {
            val configFile = File(filesDir, "config.user.json")
            if (configFile.exists()) {
                val json = org.json.JSONObject(configFile.readText())
                val db = json.optJSONObject("database") ?: org.json.JSONObject()
                val engine = db.optString("engine", "")
                if (engine == "objectbox") {
                    db.put("engine", "sqlite")
                    db.put("engineFallbackReason", "libobjectbox-jni.so not packaged in APK")
                    json.put("database", db)
                    configFile.writeText(json.toString(2))
                    Log.i(TAG, "Config rewritten: database.engine → sqlite (was objectbox)")
                    GoProcessPlugin.pushKotlinLog("warn", TAG, "config.user.json: database.engine 已从 objectbox 改为 sqlite（降级）")
                }
            }
        } catch (e: Exception) {
            Log.w(TAG, "Failed to update config for objectbox fallback", e)
        }
    }
}
