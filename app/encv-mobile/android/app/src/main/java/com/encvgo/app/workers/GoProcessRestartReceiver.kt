package com.encvgo.app.workers

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log
import com.encvgo.app.EncvGoService

/**
 * GoProcessRestartReceiver — 监听 Go 后端就绪广播，触发 pending Worker 立即重试。
 *
 * 2026-07-03 spec android-workmanager-split-start-stop Phase 3.3
 *
 * 场景：
 *   用户取消任务时 Go 进程已死 → EncvTaskCancelWorker 标记 retry →
 *   WorkManager 排队等退避时间 → Go 进程重启 → 广播 BACKEND_READY →
 *   本 Receiver 收到 → 触发所有 pending cancel Worker 立即重试（不等退避）
 *
 * 注册方式：在 AndroidManifest.xml 中静态注册（exported=false，只接收应用内广播）。
 * 也可以在 EncvApplication 中动态注册。
 *
 * ⚠️ 注意：
 *   Android 8.0+ 静态注册的隐式广播大部分被限制，
 *   但 BROADCAST_BACKEND_READY 是我们自己发的显式 Intent（指定 package），
 *   静态注册也能收到。不过更稳妥的方式是在 EncvApplication 里动态注册。
 */
class GoProcessRestartReceiver : BroadcastReceiver() {

    companion object {
        private const val TAG = "ENCV-RestartReceiver"
    }

    override fun onReceive(context: Context, intent: Intent) {
        val action = intent.action
        Log.d(TAG, "onReceive: action=$action")

        when (action) {
            EncvGoService.BROADCAST_BACKEND_READY -> {
                // Go 后端就绪了，触发所有 pending 的 EncvTaskCancelWorker 立即重试
                Log.i(TAG, "Go backend ready, triggering pending cancel workers")
                EncvTaskCancelWorker.enqueueAllPending(context)
            }
        }
    }
}
