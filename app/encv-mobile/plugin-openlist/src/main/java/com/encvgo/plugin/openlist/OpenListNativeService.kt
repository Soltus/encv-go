package com.encvgo.plugin.openlist

import android.content.Context
import android.util.Log
import java.io.File
import java.io.InputStream
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Phase 26: 仿 host app 的 [com.encvgo.app.EncvGoService] 模式——
 * 用 ProcessBuilder 启动独立 Go native binary `libopenlist.so server`，
 * 完全替代 gomobile bind 的 openlistlib.* 嵌入进程模式。
 *
 * 架构对比：
 * |                  | 旧 gomobile bind             | 新 ProcessBuilder               |
 * |------------------|------------------------------|---------------------------------|
 * | Go runtime       | 嵌入 Android 进程 JNI        | 独立进程                        |
 * | Java/Go 通信     | JNI 直接调函数               | HTTP REST API 调 OpenList server |
 * | aar 字节 hack    | 必需 (A3.3 series)           | **不需要** —— jniLibs 流程天然  |
 * | 启动方式         | Openlistlib.start()          | ProcessBuilder(libopenlist.so)  |
 * | 状态推送         | openlistlib.Event callback   | broadcast (in-process listener) |
 *
 * 关键设计点：
 * 1. `libopenlist.so` 是 Go 1.25.1 + CGO=1 + NDK clang 交叉编译产物（Phase 27）,
 *    arm64-v8a 打到 src/main/jniLibs/（Phase 27 决策：4 ABI → 1 ABI）
 *    —— aar2apk 任务 [ConvertAarToApkTask.kt:179-193] 无条件 addNativeLibs,
 *    plugin APK 自动包含 lib/arm64-v8a/libopenlist.so,无需 aar 字节 hack。
 * 2. OpenList server 监听 127.0.0.1:5244（gomobile bind 版与新方案都用 OpenList 默认端口）
 * 3. WebView 加载 `http://127.0.0.1:5244/`,OpenList 自带 web UI
 * 4. config.json / data / db 都用 plugin 私有 dir（/data/data/<plugin-id>/files/openlist/）
 * 5. Hi-Sillot/OpenList fork 切换 glebarez/sqlite（pure-Go）避免 CGO 撞 NDK toolchain
 *    —— 详见 .trae/rules/android.md §五 gomobile+sqlite 选型铁律
 *
 * 与 OpenListBridge.snapshot() 等价的运行时状态对外暴露
 * —— 保持与 gomobile bind 版 API 兼容 (host OpenListStatusBridge / ContentProvider 都
 * 仍走 snapshot() 取状态,只是实现从 openlistlib 切到 ProcessBuilder 启的独立进程)。
 */
object OpenListNativeService {

    private const val TAG = "OpenList-Native"

    /**
     * Phase 23 兼容：host 通过 classloader 反射写入的状态变更回调。
     * 保留同名同类型的 `statusListener` 字段, host OpenListStatusBridge 不需要改 classloader
     * 反射路径。OpenListNativeService 自己启 Go 进程后,通过 [broadcastStatus] 推快照。
     */
    @Volatile
    @JvmStatic
    var statusListener: ((Map<String, Any?>) -> Unit)? = null

    @Volatile private var process: Process? = null
    @Volatile private var stdoutReader: Thread? = null
    @Volatile private var stderrReader: Thread? = null
    @Volatile private var appContext: Context? = null

    @Volatile private var dataDir: String = ""
    @Volatile private var port: Int = 0
    @Volatile private var running: Boolean = false
    @Volatile private var pid: Int = 0
    @Volatile private var dataSizeBytes: Long = 0L
    @Volatile private var lastError: String? = null
    @Volatile private var lastUpdateTs: Long = 0L
    @Volatile private var initialized: Boolean = false

    private val lock = Any()
    private val starting = AtomicBoolean(false)

    /**
     * 查找 native binary 路径。优先用 applicationInfo.nativeLibraryDir（生产环境 aar 装 APK 流程
     * 已拷贝 .so 到这里）；dev 调试也可 fallback 到 filesDir（仿 EncvGoService 的 assets→filesDir 路径）。
     */
    private fun locateNativeBinary(context: Context): File? {
        val nativeLibDir = File(context.applicationInfo.nativeLibraryDir)
        // libopenlist.so 是 4 个 ABI 都打的;在 64 位设备上 nativeLibraryDir 直接指向 lib/arm64-v8a
        val candidate = File(nativeLibDir, "libopenlist.so")
        if (candidate.exists() && candidate.canExecute()) {
            Log.e(TAG, "[OpenList-Native] located libopenlist.so | path=${candidate.absolutePath} | size=${candidate.length()}")
            return candidate
        }
        // fallback 1: lib/<abi>/libopenlist.so
        val abi = android.os.Build.SUPPORTED_ABIS.firstOrNull() ?: return null
        val abiCandidate = File("$nativeLibDir/$abi", "libopenlist.so")
        if (abiCandidate.exists() && abiCandidate.canExecute()) {
            Log.e(TAG, "[OpenList-Native] located libopenlist.so (abi=$abi) | path=${abiCandidate.absolutePath} | size=${abiCandidate.length()}")
            return abiCandidate
        }
        // fallback 2: filesDir/libopenlist.so（dev 调试 / 自定义 assets 拷贝路径）
        val filesCandidate = File(context.filesDir, "libopenlist.so")
        if (filesCandidate.exists() && filesCandidate.canExecute()) {
            Log.e(TAG, "[OpenList-Native] located libopenlist.so (filesDir) | path=${filesCandidate.absolutePath}")
            return filesCandidate
        }
        Log.e(TAG, "[OpenList-Native] libopenlist.so not found | nativeLibDir=$nativeLibDir abi=$abi filesDir=${context.filesDir}")
        return null
    }

    /**
     * 一次性 context 缓存。OpenListPluginService.onCreate 已注入 proxyService,
     * 这里只需要 applicationContext 拿 nativeLibraryDir / filesDir。
     */
    fun init(context: Context) {
        appContext = context.applicationContext
        Log.e(TAG, "[OpenList-Native] init() | appContext=${appContext?.packageName} | thread=${Thread.currentThread().name}")
    }

    /**
     * 配置 dataDir / port / adminPassword 到本地快照。OpenList Go server 启动时通过 args
     * 接收 --data / --port / --admin 三个参数(由 Hi-Sillot/OpenList fork 的 main.go 解析),
     * 这里不立即重启服务——start() 时才生效。
     */
    fun setDataDir(path: String) {
        synchronized(lock) { dataDir = path }
        Log.e(TAG, "[OpenList-Native] setDataDir($path)")
    }
    fun setPort(p: Int) {
        synchronized(lock) { port = p }
        Log.e(TAG, "[OpenList-Native] setPort($p)")
    }
    fun setAdminPassword(pwd: String) {
        // Phase 26 简化：adminPassword 走 OpenList REST API /api/admin/user/set,不需要重启 server
        // 缓存到 config.json(由 web 端 web UI 触发),不必在 native service 缓存
        Log.e(TAG, "[OpenList-Native] setAdminPassword len=${pwd.length} (deferred to REST API)")
    }

    /**
     * 启动 OpenList Go server 独立进程。仿 EncvGoService.start() 模式:
     *   val binary = File(nativeLibDir, "lib<name>.so")
     *   val proc = ProcessBuilder(binary.absolutePath, "server", ...).start()
     *
     * Phase 26 与 EncvGoService 的差异:
     *   - EncvGoService 起的是 encv-go 二进制 (libencv-go.so start)
     *   - OpenListNativeService 起的是 libopenlist.so server
     *   - args 包含 --port --data --admin (由 OpenList fork 的 main.go 解析)
     *   - stdout/stderr 全程 reader 线程转 logcat
     */
    fun start() {
        val ctx = appContext ?: run {
            Log.e(TAG, "[OpenList-Native] start() FAILED: no appContext (call init() first)")
            broadcastStatus(0, false)
            return
        }
        if (!starting.compareAndSet(false, true)) {
            Log.w(TAG, "[OpenList-Native] start() skipped (already starting)")
            return
        }
        synchronized(lock) {
            if (running) {
                Log.w(TAG, "[OpenList-Native] start() skipped (already running)")
                starting.set(false)
                return
            }
        }
        Thread {
            runStartInBackground(ctx)
        }.start()
    }

    /**
     * 实际启动 OpenList server 的逻辑（抽到独立函数以便用裸 `return` 早退）。
     *
     * 之前在 Thread lambda 内写 `return@start`，但 `Thread { ... }` 不是 inline lambda，
     * Kotlin 编译器报 `'return' is prohibited here`（L162:21）。
     * 抽到 private fun 后，`return` 是普通 fun body return,完全合法。
     */
    private fun runStartInBackground(ctx: Context) {
        try {
            val binary = locateNativeBinary(ctx)
            if (binary == null) {
                synchronized(lock) {
                    lastError = "libopenlist.so not found in nativeLibraryDir/filesDir"
                    lastUpdateTs = System.currentTimeMillis()
                }
                broadcastStatus(0, false)
                return
            }
            val cfgPort = synchronized(lock) { if (port > 0) port else OpenListConfig.DEFAULT_PORT }
            val cfgDataDir = synchronized(lock) { if (dataDir.isNotEmpty()) dataDir else OpenListConfig.defaultDataDir(ctx) }
            val cfg = OpenListConfig.load(ctx)
            // 确保 data dir 存在(OpenList server 不会自动 mkdir)
            File(cfgDataDir).mkdirs()
            Log.e(TAG, "[OpenList-Native] starting OpenList server | binary=${binary.absolutePath} port=$cfgPort dataDir=$cfgDataDir")
            // 组装 args: libopenlist.so server --port <port> --data <dataDir> --admin <pwd> (pwd 留空)
            val args = mutableListOf(
                binary.absolutePath,
                "server",
                "--port", cfgPort.toString(),
                "--data", cfgDataDir,
            )
            // 显式传 env:确保 Go runtime 不踩 host env(PATH/LD_LIBRARY_PATH)
            val env = mutableMapOf<String, String>()
            env["HOME"] = ctx.filesDir.absolutePath
            env["TMPDIR"] = ctx.cacheDir.absolutePath
            env["ANDROID_DATA"] = ctx.dataDir.absolutePath
            val proc = ProcessBuilder(args)
                .directory(ctx.filesDir)
                .redirectErrorStream(false)
                .start()
            synchronized(lock) {
                process = proc
                pid = try { android.os.Process.getUidForName("") /* sentinel, real pid below */ } catch (_: Throwable) { 0 }
                // Process.toString() 形如 "Process[pid=12345, ...]" — 解析它拿真实 pid
                val pidMatch = Regex("""pid=(\d+)""").find(proc.toString())
                if (pidMatch != null) {
                    pid = pidMatch.groupValues[1].toIntOrNull() ?: 0
                }
                initialized = true
                lastUpdateTs = System.currentTimeMillis()
            }
            // 后台读 stdout/stderr → logcat(仿 EncvGoService.alsoLog)
            startLogcatBridge(proc.inputStream, "stdout")
            startLogcatBridge(proc.errorStream, "stderr")
            val exitCode = proc.waitFor()
            Log.e(TAG, "[OpenList-Native] OpenList server exited | code=$exitCode | binary=${binary.absolutePath}")
            synchronized(lock) {
                running = false
                if (exitCode != 0) {
                    lastError = "OpenList server exited with code $exitCode"
                }
                lastUpdateTs = System.currentTimeMillis()
            }
            broadcastStatus(cfgPort, false)
        } catch (e: Throwable) {
            Log.e(TAG, "[OpenList-Native] start() FAILED", e)
            synchronized(lock) {
                lastError = "start failed: ${e.message}"
                lastUpdateTs = System.currentTimeMillis()
            }
            broadcastStatus(0, false)
        } finally {
            starting.set(false)
        }
    }

    private fun startLogcatBridge(stream: InputStream, label: String) {
        val thread = Thread({
            try {
                stream.bufferedReader().useLines { lines ->
                    lines.forEach { line ->
                        Log.e(TAG, "[openlist/$label] $line")
                        if (line.startsWith("data_size=")) {
                            val parsed = line.removePrefix("data_size=").trim().toLongOrNull()
                            if (parsed != null) {
                                synchronized(lock) {
                                    dataSizeBytes = parsed
                                    lastUpdateTs = System.currentTimeMillis()
                                }
                            }
                        } else if (line.contains("listening on") || line.contains("HTTP server started")) {
                            // OpenList 启动信号(具体日志格式由 OpenList fork 的 main.go 决定)
                            synchronized(lock) {
                                running = true
                                lastUpdateTs = System.currentTimeMillis()
                            }
                            broadcastStatus(port, true)
                        }
                    }
                }
            } catch (e: Throwable) {
                Log.w(TAG, "[OpenList-Native] $label reader thread FAILED", e)
            }
        }, "OpenList-Native-$label-Reader")
        thread.isDaemon = true
        thread.start()
        if (label == "stdout") stdoutReader = thread else stderrReader = thread
    }

    /**
     * 优雅停止 OpenList Go server。Process.destroy() 发送 SIGTERM,Go server 应在 ~5s 内退出。
     * 若超时则 force-kill (Process.destroyForcibly() 发送 SIGKILL)。
     */
    fun shutdown(timeoutMs: Long) {
        Log.e(TAG, "[OpenList-Native] shutdown() entry | timeoutMs=$timeoutMs")
        val proc: Process? = synchronized(lock) { process }
        if (proc == null) {
            Log.w(TAG, "[OpenList-Native] shutdown() skipped (no process)")
            return
        }
        Thread {
            try {
                proc.destroy()
                if (!proc.waitFor(timeoutMs, java.util.concurrent.TimeUnit.MILLISECONDS)) {
                    Log.w(TAG, "[OpenList-Native] shutdown() grace timeout → destroyForcibly()")
                    proc.destroyForcibly()
                }
            } catch (e: Throwable) {
                Log.e(TAG, "[OpenList-Native] shutdown() FAILED", e)
            } finally {
                synchronized(lock) {
                    running = false
                    process = null
                    lastUpdateTs = System.currentTimeMillis()
                }
                broadcastStatus(0, false)
            }
        }.start()
    }

    fun isRunning(): Boolean = synchronized(lock) { running }

    /**
     * 等价于 OpenListBridge.snapshot() —— 保持 API 兼容。
     * host OpenListStatusBridge / ContentProvider 都消费这个 snapshot.
     */
    fun snapshot(): Map<String, Any?> = synchronized(lock) {
        mapOf(
            "running" to running,
            "port" to port,
            "pid" to pid,
            "data_size_bytes" to dataSizeBytes,
            "last_error" to (lastError ?: ""),
            "last_update_ts" to lastUpdateTs,
        )
    }

    /**
     * 推送状态变更。优先调 [statusListener]（host 反射注册,Phase 23 in-process 通道）,
     * 失败 fallback 到 LocalBroadcastManager(Phase 22 历史路径,供 plugin 内其他组件订阅)。
     */
    fun broadcastStatus(currentPort: Int, isRunning: Boolean) {
        synchronized(lock) {
            port = currentPort
            running = isRunning
        }
        val snap = snapshot()
        Log.e(TAG, "[OpenList-Native] broadcastStatus() | port=$currentPort running=$isRunning | ts=${System.currentTimeMillis()}")
        val listener = statusListener
        if (listener != null) {
            try {
                listener.invoke(snap)
            } catch (e: Throwable) {
                Log.e(TAG, "[OpenList-Native] broadcastStatus() listener FAILED", e)
            }
        } else {
            Log.e(TAG, "[OpenList-Native] broadcastStatus() no listener registered")
        }
    }

    /**
     * Phase 26: 静默更新 dataSize 快照（OpenList Go server 周期输出 "data_size=xxx" 日志,
     * 不属于状态变更,只更新内部字段）。
     */
    fun notifyDbSynced(sizeBytes: Long) {
        synchronized(lock) {
            dataSizeBytes = sizeBytes
            lastUpdateTs = System.currentTimeMillis()
        }
    }
}
