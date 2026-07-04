package com.encvgo.app.workers

import android.content.Context
import android.content.SharedPreferences
import android.util.Log
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import com.encvgo.app.EncvGoService
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.TimeUnit

class SimverseHeartbeatWorker(
    ctx: Context,
    params: WorkerParameters
) : CoroutineWorker(ctx, params) {

    companion object {
        private const val TAG = "SimverseHeartbeat"
        private const val WORK_NAME = "encv:simverse:heartbeat"
        private const val PREFS_NAME = "encv_simverse_prefs"
        private const val PREFS_WORLD_RUNNING = "world_running"
        private const val PREFS_LAST_TICK = "last_tick"
        private const val PREFS_LAST_HEARTBEAT = "last_heartbeat"

        fun schedule(context: Context) {
            val request = PeriodicWorkRequestBuilder<SimverseHeartbeatWorker>(
                15, TimeUnit.MINUTES,
                5, TimeUnit.MINUTES
            )
                .addTag("simverse")
                .build()

            WorkManager.getInstance(context).enqueueUniquePeriodicWork(
                WORK_NAME,
                ExistingPeriodicWorkPolicy.UPDATE,
                request
            )
            Log.i(TAG, "scheduled periodic heartbeat (15min)")
        }

        fun cancel(context: Context) {
            WorkManager.getInstance(context).cancelUniqueWork(WORK_NAME)
            Log.i(TAG, "cancelled heartbeat")
        }

        fun setWorldRunning(context: Context, running: Boolean) {
            val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            prefs.edit().putBoolean(PREFS_WORLD_RUNNING, running).apply()
        }

        fun isWorldExpectedRunning(context: Context): Boolean {
            val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            return prefs.getBoolean(PREFS_WORLD_RUNNING, false)
        }

        fun getLastTick(context: Context): Long {
            val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            return prefs.getLong(PREFS_LAST_TICK, 0)
        }
    }

    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        val context = applicationContext
        val expectedRunning = isWorldExpectedRunning(context)
        val now = System.currentTimeMillis()

        Log.i(TAG, "heartbeat tick: expectedRunning=$expectedRunning")

        val port = EncvGoService.lastKnownPort
        if (port <= 0) {
            Log.w(TAG, "heartbeat: backend not running (port=0)")
            recordHeartbeat(context, now, false, 0)
            return@withContext Result.retry()
        }

        try {
          val state = fetchWorldState(port)
          val running = state?.optBoolean("running") == true
          val tick = state?.optInt("tick") ?: 0

          recordHeartbeat(context, now, running, tick.toLong())

          if (expectedRunning && !running) {
            Log.w(TAG, "heartbeat: world paused but expected running, resuming...")
            resumeWorld(port)
          }

          if (tick > 0) {
            saveWorldCheckpoint(port)
          }

          // 这里直接返回 Result.success()，作为 try 块的最后一个表达式
          Result.success()
        } catch (e: Exception) {
          Log.e(TAG, "heartbeat failed", e)
          // 这里也是直接返回 Result.retry()
          Result.retry()
        }
    }

    private fun fetchWorldState(port: Int): JSONObject? {
        return try {
            val url = URL("http://127.0.0.1:$port/api/simverse/world/state")
            val conn = url.openConnection() as HttpURLConnection
            conn.requestMethod = "GET"
            conn.connectTimeout = 3000
            conn.readTimeout = 5000

            if (conn.responseCode == 200) {
                val body = conn.inputStream.bufferedReader().readText()
                JSONObject(body)
            } else {
                null
            }
        } catch (e: Exception) {
            Log.w(TAG, "fetchWorldState failed: ${e.message}")
            null
        }
    }

    private fun resumeWorld(port: Int): Boolean {
        return try {
            val url = URL("http://127.0.0.1:$port/api/simverse/world/resume")
            val conn = url.openConnection() as HttpURLConnection
            conn.requestMethod = "POST"
            conn.connectTimeout = 3000
            conn.readTimeout = 5000
            conn.doOutput = true
            conn.outputStream.write(ByteArray(0))
            conn.responseCode == 200
        } catch (e: Exception) {
            Log.w(TAG, "resumeWorld failed: ${e.message}")
            false
        }
    }

    private fun saveWorldCheckpoint(port: Int): Boolean {
        return try {
            val url = URL("http://127.0.0.1:$port/api/simverse/world/save")
            val conn = url.openConnection() as HttpURLConnection
            conn.requestMethod = "POST"
            conn.connectTimeout = 3000
            conn.readTimeout = 10000
            conn.doOutput = true
            conn.outputStream.write(ByteArray(0))
            val ok = conn.responseCode == 200
            if (ok) {
                Log.i(TAG, "checkpoint saved via heartbeat")
            }
            ok
        } catch (e: Exception) {
            Log.w(TAG, "saveCheckpoint failed: ${e.message}")
            false
        }
    }

    private fun recordHeartbeat(context: Context, timestamp: Long, running: Boolean, tick: Long) {
        val prefs: SharedPreferences = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
        prefs.edit()
            .putLong(PREFS_LAST_HEARTBEAT, timestamp)
            .putLong(PREFS_LAST_TICK, tick)
            .apply()
    }
}
