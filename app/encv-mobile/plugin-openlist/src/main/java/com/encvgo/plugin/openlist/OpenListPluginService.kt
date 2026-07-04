package com.encvgo.plugin.openlist

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.PowerManager
import android.util.Log
import androidx.core.app.NotificationCompat
import com.combo.core.component.service.BasePluginService
import java.net.InetSocketAddress
import java.net.Socket
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Phase 26: 仿 host app 的 [com.encvgo.app.EncvGoService] + 上一版 OpenListPluginService
 * 合并——用 ProcessBuilder 启 libopenlist.so（仿 EncvGoService）替代 gomobile bind 嵌入
 * 进程模式（Openlistlib.start）。
 *
 * 启动链路：
 *   host OpenListPluginService::class.java, "main"）
 *     → ProxyManager.acquireServiceProxy
 *     → BaseHostService.onStartCommand
 *     → initPluginService → newInstance() + onAttach + onCreate
 *     → onAttach(proxyService) 注入 host service 引用（plugin 拿 Context）
 *     → onCreate() (本类) → 走 startupSequence
 *     → startupSequence → OpenListNativeService.start() (ProcessBuilder 启 libopenlist.so)
 *     → 等待 OpenList Go server "listening on 5244" 日志 → broadcastStatus(port, true)
 */
class OpenListPluginService : BasePluginService() {

    companion object {
        private const val TAG = "OpenList-PluginService"
        private const val CHANNEL_ID = "openlist_server"
        private const val FOREGROUND_ID = 5224
        private const val PORT_CONFLICT_TIMEOUT_MS = 2_000

        const val ACTION_SHUTDOWN = "com.encvgo.plugin.openlist.ACTION_SHUTDOWN"
        const val BROADCAST_PORT_CONFLICT = "com.encvgo.plugin.openlist.BROADCAST_PORT_CONFLICT"
        const val BROADCAST_LOG = "com.encvgo.plugin.openlist.BROADCAST_LOG"
        const val EXTRA_CONFLICT_PORT = "conflict_port"

        @Volatile
        var isRunning: Boolean = false
            private set

        @Volatile
        var currentPort: Int = 0
            private set

        @Volatile
        private var instance: OpenListPluginService? = null

        fun getInstance(): OpenListPluginService? = instance

        fun stopIfRunning() {
            val svc = instance
            if (svc == null) {
                Log.w(TAG, "stopIfRunning: no active instance (service not started via startPluginService)")
                return
            }
            svc.shutdownFromExternal()
        }
    }

    private val handler = Handler(Looper.getMainLooper())
    private val worker = Executors.newSingleThreadExecutor()
    private val started = AtomicBoolean(false)
    private var wakeLock: PowerManager.WakeLock? = null

    // === IPluginService lifecycle ===

    override fun onCreate() {
        Log.e(TAG, "[SAT-DBG][OpenList] OpenListPluginService.onCreate() | thread=${Thread.currentThread().name} | ts=${System.currentTimeMillis()}")
        super.onCreate()
        val ctx = proxyService
        if (ctx == null) {
            Log.e(TAG, "[SAT-DBG][OpenList] onCreate() FAILED: proxyService is null (host didn't call onAttach)")
            return
        }
        instance = this
        try {
            createNotificationChannel(ctx)
        } catch (e: Throwable) {
            Log.e(TAG, "[SAT-DBG][OpenList] createNotificationChannel FAILED", e)
        }
        // 把 ctx 注入 OpenListNativeService（让 native service 能 locate libopenlist.so）
        OpenListNativeService.init(ctx)
        Log.e(TAG, "[SAT-DBG][OpenList] onCreate() done | notification channel created")
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val portOverride = intent?.getIntExtra("port", -1) ?: -1
        Log.e(TAG, "[SAT-DBG][OpenList] onStartCommand() | action=${intent?.action} portOverride=$portOverride flags=$flags startId=$startId | thread=${Thread.currentThread().name} | ts=${System.currentTimeMillis()}")
        val ctx = proxyService
        if (ctx == null) {
            Log.e(TAG, "[SAT-DBG][OpenList] onStartCommand() FAILED: proxyService is null")
            return android.app.Service.START_NOT_STICKY
        }
        try {
            ctx.startForeground(FOREGROUND_ID, buildNotification(ctx, "OpenList 启动中"))
        } catch (e: Throwable) {
            Log.e(TAG, "[SAT-DBG][OpenList] startForeground FAILED", e)
        }
        acquireWakeLock()
        if (started.compareAndSet(false, true)) {
            worker.execute { startupSequence(portOverride) }
        }
        if (intent?.action == ACTION_SHUTDOWN) {
            worker.execute { shutdownSequence() }
        }
        return android.app.Service.START_STICKY
    }

    override fun onDestroy() {
        Log.e(TAG, "[SAT-DBG][OpenList] onDestroy() | thread=${Thread.currentThread().name} | ts=${System.currentTimeMillis()}")
        instance = null
        releaseWakeLock()
        worker.shutdownNow()
        super.onDestroy()
        Log.e(TAG, "[SAT-DBG][OpenList] onDestroy() done")
    }

    private fun shutdownFromExternal() {
        try {
            worker.execute { shutdownSequence() }
        } catch (e: Throwable) {
            Log.w(TAG, "shutdownFromExternal FAILED", e)
        }
    }

    // === 业务逻辑（仿 EncvGoService + 旧 OpenListService.startupSequence） ===

    private fun startupSequence(portOverride: Int = -1) {
        Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() begin | portOverride=$portOverride | thread=${Thread.currentThread().name}")
        val ctx = proxyService ?: run {
            Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() FAILED: proxyService null")
            return
        }

        // Step 1: load config
        val t0 = System.currentTimeMillis()
        Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() step1: loading config...")
        val cfg = OpenListConfig.load(ctx)
        val port = if (portOverride > 0) {
            Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() step1: portOverride=$portOverride → override config port ${cfg.port}")
            portOverride
        } else {
            cfg.port
        }
        currentPort = port
        Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() step1 done: config loaded | port=$port dataDir=${cfg.dataDir} | elapsed=${System.currentTimeMillis() - t0}ms")

        // Step 2: port check
        val t1 = System.currentTimeMillis()
        Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() step2: checking port $port...")
        if (isPortOccupied(port)) {
            Log.w(TAG, "Port $port already in use, broadcasting PORT_CONFLICT")
            androidx.localbroadcastmanager.content.LocalBroadcastManager.getInstance(ctx).sendBroadcast(
                Intent(BROADCAST_PORT_CONFLICT).putExtra(EXTRA_CONFLICT_PORT, port)
            )
            Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() step2 failed: PORT_CONFLICT at $port | elapsed=${System.currentTimeMillis() - t1}ms")
            try { ctx.stopForeground(android.app.Service.STOP_FOREGROUND_REMOVE) } catch (_: Throwable) {}
            try { ctx.stopSelf() } catch (_: Throwable) {}
            return
        }
        Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() step2 done: port $port is free | elapsed=${System.currentTimeMillis() - t1}ms")

        // Step 3: apply config to native service
        val t2 = System.currentTimeMillis()
        Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() step3: applying config to native service...")
        try {
            OpenListNativeService.setPort(port)
            OpenListNativeService.setDataDir(cfg.dataDir)
            // adminPassword 不传 native (走 OpenList REST API 在 web 端触发)
            Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() step3 done: config applied | elapsed=${System.currentTimeMillis() - t2}ms")

            // Step 4: native service init + start
            val t3 = System.currentTimeMillis()
            Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() step4: starting native service...")
            OpenListNativeService.init(ctx)
            OpenListNativeService.start()
            // OpenListNativeService.start() 是非阻塞的(内部 Thread{} 包 ProcessBuilder),
            // isRunning 会在 stdout reader 线程捕获 "listening on" 日志后被设 true
            Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() step4 done: native service started (non-blocking) | elapsed=${System.currentTimeMillis() - t3}ms")

            // 注意:Phase 26 简化 — startupSequence 不再 poll 等待 running=true,
            // 由 OpenListNativeService 自己的 stdout reader 捕获 "listening on" 后
            // broadcastStatus(port, true) 推 host.
            // 这里只设本地 isRunning 标记(假设 server 启动成功——失败由 onLog/onProcessExit 路径处理)
            isRunning = true
            OpenListNativeService.broadcastStatus(port, true)
            try { updateNotification(ctx, "OpenList 运行中 :$port") } catch (_: Throwable) {}
            Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() complete | total=${System.currentTimeMillis() - t0}ms")
        } catch (e: Exception) {
            Log.e(TAG, "[SAT-DBG][OpenList] startupSequence() FAILED | elapsed=${System.currentTimeMillis() - t0}ms", e)
            OpenListNativeService.broadcastStatus(0, false)
            try { ctx.stopForeground(android.app.Service.STOP_FOREGROUND_REMOVE) } catch (_: Throwable) {}
            try { ctx.stopSelf() } catch (_: Throwable) {}
        }
    }

    private fun shutdownSequence() {
        Log.e(TAG, "[SAT-DBG][OpenList] shutdownSequence() begin | thread=${Thread.currentThread().name} | ts=${System.currentTimeMillis()}")
        val ctx = proxyService
        try {
            Log.e(TAG, "[SAT-DBG][OpenList] shutdownSequence() calling native service shutdown...")
            OpenListNativeService.shutdown(5_000L)
            Log.e(TAG, "[SAT-DBG][OpenList] shutdownSequence() native service shutdown returned")
        } catch (e: Exception) {
            Log.w(TAG, "OpenList shutdown error", e)
            Log.e(TAG, "[SAT-DBG][OpenList] shutdownSequence() native service shutdown error", e)
        }
        isRunning = false
        currentPort = 0
        OpenListNativeService.broadcastStatus(0, false)
        if (ctx != null) {
            try { ctx.stopForeground(android.app.Service.STOP_FOREGROUND_REMOVE) } catch (_: Throwable) {}
            try { ctx.stopSelf() } catch (_: Throwable) {}
        }
        Log.e(TAG, "[SAT-DBG][OpenList] shutdownSequence() done")
    }

    private fun isPortOccupied(port: Int): Boolean {
        val t0 = System.currentTimeMillis()
        return try {
            Socket().use { socket ->
                socket.connect(InetSocketAddress("127.0.0.1", port), PORT_CONFLICT_TIMEOUT_MS)
                val elapsed = System.currentTimeMillis() - t0
                Log.e(TAG, "[SAT-DBG][OpenList] isPortOccupied() true | port=$port | connectElapsed=${elapsed}ms")
                true
            }
        } catch (_: Exception) {
            false
        }
    }

    // === Notification / WakeLock 工具方法（接受 Context 参数） ===

    private fun createNotificationChannel(ctx: Context) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val manager = ctx.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            val channel = NotificationChannel(
                CHANNEL_ID,
                "OpenList Server",
                NotificationManager.IMPORTANCE_LOW
            )
            channel.setShowBadge(false)
            channel.lockscreenVisibility = Notification.VISIBILITY_PRIVATE
            manager.createNotificationChannel(channel)
        }
    }

    private fun buildNotification(ctx: Context, text: String): Notification {
        val flags = PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        val openIntent = ctx.packageManager.getLaunchIntentForPackage(ctx.packageName)
        val pendingIntent = if (openIntent != null) {
            PendingIntent.getActivity(ctx, 0, openIntent, flags)
        } else {
            val proxy = proxyService
            val pi = if (proxy != null) {
                PendingIntent.getService(ctx, 0, Intent(ctx, proxy.javaClass), flags)
            } else {
                PendingIntent.getService(ctx, 0, Intent(), flags)
            }
            pi
        }
        return NotificationCompat.Builder(ctx, CHANNEL_ID)
            .setSmallIcon(android.R.drawable.ic_dialog_info)
            .setContentTitle("OpenList")
            .setContentText(text)
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .build()
    }

    private fun updateNotification(ctx: Context, text: String) {
        val manager = ctx.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        manager.notify(FOREGROUND_ID, buildNotification(ctx, text))
    }

    private fun acquireWakeLock() {
        if (wakeLock?.isHeld == true) return
        val ctx = proxyService ?: return
        try {
            val pm = ctx.getSystemService(Context.POWER_SERVICE) as PowerManager
            wakeLock = pm.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "openlist::PluginService")
            wakeLock?.acquire()
        } catch (e: Exception) {
            Log.w(TAG, "Failed to acquire WakeLock", e)
        }
    }

    private fun releaseWakeLock() {
        wakeLock?.let {
            if (it.isHeld) {
                try {
                    it.release()
                } catch (e: Exception) {
                    Log.w(TAG, "Failed to release WakeLock", e)
                }
            }
        }
        wakeLock = null
    }
}
