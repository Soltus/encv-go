package com.encvgo.app.workers

import android.content.Context
import android.util.Log
import androidx.work.CoroutineWorker
import androidx.work.ExistingWorkPolicy
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import androidx.work.workDataOf
import com.encvgo.app.EncvGoService
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.Executor
import java.util.concurrent.TimeUnit

/**
 * EncvTaskCancelWorker — 持久化 cancel 意图的 CoroutineWorker。
 *
 * 2026-07-03 spec android-workmanager-split-start-stop Phase 3.2
 *
 * 背景：
 *   Go 后端进程（EncvGoService）可能被杀（系统回收 / 用户手动停止 / OOM），
 *   用户点"取消任务"时如果 Go 进程已死，cancel 请求会丢失。
 *
 * 设计：
 *   - 前端取消任务时**双写**：直接 HTTP cancel + WorkManager enqueue
 *   - Worker 负责持久化 cancel 意图（WorkManager 内部 SQLite）
 *   - Go 进程死亡时 Worker 标记 retry，退避重试
 *   - Go 进程重启后（BROADCAST_BACKEND_READY），GoProcessRestartReceiver
 *     触发所有 pending cancel Worker 立即重试
 *
 * 去重：
 *   每个 taskId 用 unique work name（"cancel:$taskId"），ExistingWorkPolicy.REPLACE
 *   避免同一任务重复入队。
 *
 * 约束（用户原话）：
 *   - 不 mock backend：Worker 打真实的 127.0.0.1:PORT HTTP 端点
 *   - 业务逻辑在 Go：Worker 只是薄包装，不做加密/解密
 */
class EncvTaskCancelWorker(
    ctx: Context,
    params: WorkerParameters
) : CoroutineWorker(ctx, params) {

    companion object {
        private const val TAG = "ENCV-CancelWorker"
        private const val KEY_TASK_ID = "task_id"
        private const val WORK_NAME_PREFIX = "encv:cancel:"

        /** 健康检查失败最大次数（连续失败才认为 Go 进程死了） */
        private const val MAX_HEALTH_FAILS = 3

        /** 单次 HTTP 超时（ms） */
        private const val HTTP_TIMEOUT_MS = 3_000

        /** 退避时间（LinearBackoff） */
        private const val BACKOFF_SECONDS = 10L

        /** 最大重试次数（WorkManager 默认 24h，这里主动标记失败避免无限重试） */
        private const val MAX_RETRY_COUNT = 10

        /**
         * 入队一个 cancel Worker。
         * @param taskId 要取消的任务 ID
         * @return unique work name
         */
        fun enqueue(ctx: Context, taskId: String): String {
            val workName = "$WORK_NAME_PREFIX$taskId"
            val request = OneTimeWorkRequestBuilder<EncvTaskCancelWorker>()
                .setInputData(workDataOf(KEY_TASK_ID to taskId))
                .setInitialDelay(0, TimeUnit.SECONDS)
                .build()
            WorkManager.getInstance(ctx)
                .enqueueUniqueWork(workName, ExistingWorkPolicy.REPLACE, request)
            Log.d(TAG, "enqueued cancel worker for task=$taskId (workName=$workName)")
            return workName
        }

        /**
         * Go 进程重启后，触发所有 pending 的 cancel Worker 立即重试。
         * 由 GoProcessRestartReceiver 调用。
         *
         * 实现说明：
         *   WorkManager.getWorkInfosByTag() 返回 ListenableFuture，
         *   为避免阻塞主线程（广播接收器 onReceive 只有 10s 超时），
         *   使用 addListener + 后台线程获取结果。
         *   即使查询失败也不影响功能 — WorkManager 的 LinearBackoff
         *   本身就是持久化的，退避时间到了会自动重试。
         */
        fun enqueueAllPending(ctx: Context) {
            val wm = WorkManager.getInstance(ctx)
            val future = wm.getWorkInfosByTag(EncvTaskCancelWorker::class.java.name)

            // 用后台线程执行，避免阻塞主线程
            val bgExecutor = Executor { runnable ->
                Thread(runnable, "encv-cancel-worker-retrigger").start()
            }

            future.addListener(
                {
                    try {
                        val infos = future.get()
                        var retriggered = 0
                        for (info in infos) {
                            if (info.state.isFinished) continue
                            val taskId = info.inputData.getString(KEY_TASK_ID) ?: continue
                            // REPLACE 旧的，新的立即执行
                            val workName = "$WORK_NAME_PREFIX$taskId"
                            val request = OneTimeWorkRequestBuilder<EncvTaskCancelWorker>()
                                .setInputData(workDataOf(KEY_TASK_ID to taskId))
                                .build()
                            wm.enqueueUniqueWork(workName, ExistingWorkPolicy.REPLACE, request)
                            retriggered++
                        }
                        Log.d(TAG, "re-triggered $retriggered cancel workers (go restart)")
                    } catch (e: Exception) {
                        Log.w(TAG, "enqueueAllPending: failed: ${e.javaClass.simpleName}")
                        // 失败不影响功能，WorkManager 会按退避策略自动重试
                    }
                },
                bgExecutor
            )
        }
    }

    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        val taskId = inputData.getString(KEY_TASK_ID)
        if (taskId.isNullOrEmpty()) {
            Log.e(TAG, "doWork: task_id is null, fail")
            return@withContext Result.failure()
        }

        val retryCount = runAttemptCount
        Log.d(TAG, "doWork: task=$taskId attempt=$retryCount")

        // 1. 检查 Go 后端是否存活
        val port = EncvGoService.lastKnownPort
        if (port <= 0) {
            Log.w(TAG, "doWork: lastKnownPort=0, Go not running, retry")
            return@withContext retryOrFail(taskId, retryCount, "Go process not running (port=0)")
        }

        if (!checkGoHealth(port)) {
            Log.w(TAG, "doWork: Go health check failed, retry")
            return@withContext retryOrFail(taskId, retryCount, "Go health check failed")
        }

        // 2. Go 存活，先查 task 状态（已取消则 noop，避免重复请求）
        val taskStatus = tryGetTaskStatus(port, taskId)
        if (taskStatus == "cancelled" || taskStatus == "canceled") {
            Log.i(TAG, "doWork: task=$taskId already cancelled, success (noop)")
            return@withContext Result.success()
        }

        // 3. 发送 cancel 请求
        return@withContext try {
            val success = postCancel(port, taskId)
            if (success) {
                Log.i(TAG, "doWork: task=$taskId cancelled successfully")
                Result.success()
            } else {
                Log.w(TAG, "doWork: cancel request failed (non-200), retry")
                retryOrFail(taskId, retryCount, "cancel HTTP non-200")
            }
        } catch (e: Exception) {
            Log.w(TAG, "doWork: cancel exception: ${e.javaClass.simpleName}: ${e.message}")
            retryOrFail(taskId, retryCount, "cancel exception: ${e.message}")
        }
    }

    /**
     * 连续 MAX_HEALTH_FAILS 次 /health 失败才认为 Go 进程死了。
     * 避免偶发网络抖动导致不必要的 retry。
     */
    private fun checkGoHealth(port: Int): Boolean {
        repeat(MAX_HEALTH_FAILS) { i ->
            try {
                val conn = URL("http://127.0.0.1:$port/health").openConnection() as HttpURLConnection
                conn.connectTimeout = HTTP_TIMEOUT_MS
                conn.readTimeout = HTTP_TIMEOUT_MS
                conn.requestMethod = "GET"
                conn.setRequestProperty("Connection", "close")
                val code = conn.responseCode
                conn.disconnect()
                if (code == 200) {
                    return true
                }
                Log.d(TAG, "checkGoHealth: attempt ${i + 1} HTTP $code")
            } catch (e: Exception) {
                Log.d(TAG, "checkGoHealth: attempt ${i + 1} failed: ${e.javaClass.simpleName}")
            }
            // 间隔 500ms 再试
            Thread.sleep(500)
        }
        return false
    }

    /** 获取 task 状态，返回 status 字符串或 null（失败） */
    private fun tryGetTaskStatus(port: Int, taskId: String): String? {
        return try {
            val conn = URL("http://127.0.0.1:$port/api/tasks/$taskId").openConnection() as HttpURLConnection
            conn.connectTimeout = HTTP_TIMEOUT_MS
            conn.readTimeout = HTTP_TIMEOUT_MS
            conn.requestMethod = "GET"
            conn.setRequestProperty("Connection", "close")
            val code = conn.responseCode
            if (code != 200) {
                conn.disconnect()
                return null
            }
            val body = conn.inputStream.bufferedReader().use { it.readText() }
            conn.disconnect()
            val json = JSONObject(body)
            json.optString("status", null)
        } catch (e: Exception) {
            Log.d(TAG, "tryGetTaskStatus: failed: ${e.javaClass.simpleName}")
            null
        }
    }

    /** 发送 POST /api/tasks/:id/cancel，返回是否成功（HTTP 2xx） */
    private fun postCancel(port: Int, taskId: String): Boolean {
        val conn = URL("http://127.0.0.1:$port/api/tasks/$taskId/cancel").openConnection() as HttpURLConnection
        conn.connectTimeout = HTTP_TIMEOUT_MS
        conn.readTimeout = HTTP_TIMEOUT_MS
        conn.requestMethod = "POST"
        conn.doOutput = false // POST 无 body
        conn.setRequestProperty("Connection", "close")
        val code = conn.responseCode
        conn.disconnect()
        Log.d(TAG, "postCancel: task=$taskId HTTP $code")
        return code in 200..299
    }

    /** 超过最大重试次数则失败，否则 retry */
    private fun retryOrFail(taskId: String, retryCount: Int, reason: String): Result {
        if (retryCount >= MAX_RETRY_COUNT) {
            Log.e(TAG, "retryOrFail: task=$taskId exceeded max retries ($MAX_RETRY_COUNT), fail. reason=$reason")
            return Result.failure(workDataOf("error" to reason))
        }
        Log.d(TAG, "retryOrFail: task=$taskId retry ($retryCount/$MAX_RETRY_COUNT), reason=$reason")
        return Result.retry()
    }
}
