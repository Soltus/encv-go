package com.encvgo.app

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.ConcurrentLinkedQueue

/**
 * Kotlin 层 DevLog 桥接器。
 * 
 * 设计目标：在 Go 后端启动失败（或根本没启动）的情况下，
 * Kotlin 层依然能收集并持久化日志，供前端 DevLogs 页面展示。
 * 
 * 写入格式：JSONL（每行一个 JSON 对象），与 Go 后端 /api/logs/recent
 * 返回的 JSON 结构兼容，前端无需额外解析逻辑。
 * 
 * 文件位置：context.filesDir / devlogs_kotlin.jsonl
 */
object KotlinDevLogBridge {

    data class Entry(
        val timestamp: String,
        val level: String,
        val message: String,
        val source: String = "kotlin",
        val tags: List<String> = emptyList(),
        val stack: String? = null
    )

    private const val MAX_BUFFER = 500
    private const val MAX_FILE_LINES = 2000
    private val buffer = ConcurrentLinkedQueue<Entry>()
    private val dateFormat = SimpleDateFormat("yyyy-MM-dd HH:mm:ss.SSS", Locale.US)
    private val writeFormat = SimpleDateFormat("HH:mm:ss", Locale.US)

    @Volatile
    private var initialized = false

    fun init(context: Context) {
        if (initialized) return
        initialized = true
        // 启动时把 buffer 中已有的条目刷盘（如果有的话）
        flushToDisk(context)
    }

    fun log(level: String, tag: String, message: String, stack: String? = null) {
        val entry = Entry(
            timestamp = dateFormat.format(Date()),
            level = level.lowercase(Locale.ROOT),
            message = message,
            source = tag,
            tags = listOf("kotlin", "android"),
            stack = stack
        )
        buffer.add(entry)
        while (buffer.size > MAX_BUFFER) {
            buffer.poll()
        }
    }

    fun logError(tag: String, message: String, throwable: Throwable? = null) {
        val stack = throwable?.let {
            val sw = java.io.StringWriter()
            it.printStackTrace(java.io.PrintWriter(sw))
            sw.toString()
        }
        log("error", tag, message, stack)
    }

    fun logWarn(tag: String, message: String) {
        log("warn", tag, message)
    }

    fun logInfo(tag: String, message: String) {
        log("info", tag, message)
    }

    fun logDebug(tag: String, message: String) {
        log("debug", tag, message)
    }

    /**
     * 将 buffer 中的日志写入 JSONL 文件。
     * 文件格式兼容 Go 后端日志格式，方便前端统一消费。
     */
    fun flushToDisk(context: Context): Boolean {
        if (buffer.isEmpty()) return true
        return try {
            val file = File(context.filesDir, "devlogs_kotlin.jsonl")
            file.parentFile?.mkdirs()
            file.outputStream().bufferedWriter().use { writer ->
                // 先写新日志
                for (entry in buffer) {
                    val json = JSONObject().apply {
                        put("timestamp", entry.timestamp)
                        put("level", entry.level)
                        put("message", entry.message)
                        put("source", entry.source)
                        put("tags", JSONArray(entry.tags))
                        entry.stack?.let { put("stack", it) }
                    }
                    writer.write(json.toString())
                    writer.newLine()
                }
            }
            // 清空已刷盘的 buffer
            buffer.clear()
            // 截断文件避免无限增长
            trimFileIfNeeded(file)
            true
        } catch (e: Exception) {
            android.util.Log.w("KotlinDevLog", "Failed to flush logs", e)
            false
        }
    }

    /**
     * 读取所有已持久化的 Kotlin 日志（JSON 数组格式）。
     * 供 GoProcessPlugin 暴露给前端。
     */
    fun readAll(context: Context): List<Map<String, Any?>> {
        val file = File(context.filesDir, "devlogs_kotlin.jsonl")
        if (!file.exists()) return emptyList()
        return try {
            val result = mutableListOf<Map<String, Any?>>()
            file.bufferedReader().useLines { lines ->
                lines.forEach { line ->
                    try {
                        val json = org.json.JSONObject(line)
                        result.add(mapOf(
                            "timestamp" to json.optString("timestamp"),
                            "level" to json.optString("level"),
                            "message" to json.optString("message"),
                            "source" to json.optString("source"),
                            "tags" to json.optJSONArray("tags")?.let { arr ->
                                (0 until arr.length()).map { arr.getString(it) }
                            } ?: emptyList<String>(),
                            "stack" to json.optString("stack", null)
                        ))
                    } catch (_: Exception) { }
                }
            }
            result
        } catch (e: Exception) {
            android.util.Log.w("KotlinDevLog", "Failed to read logs", e)
            emptyList()
        }
    }

    fun clear(context: Context) {
        buffer.clear()
        try {
            File(context.filesDir, "devlogs_kotlin.jsonl").delete()
        } catch (_: Exception) { }
    }

    private fun trimFileIfNeeded(file: File) {
        try {
            if (!file.exists()) return
            val lines = file.readLines()
            if (lines.size > MAX_FILE_LINES) {
                val keep = lines.takeLast(MAX_FILE_LINES)
                file.writeText(keep.joinToString("\n") + "\n")
            }
        } catch (_: Exception) { }
    }
}
