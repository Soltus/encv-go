package com.encvgo.app

import android.util.Log

/**
 * 统一的日志桥接器：同时写入 Android Logcat + AppLogger（内存 buffer）+ 
 * 通过 GoProcessPlugin.pushKotlinLog() 实时推送到前端 DevLogs。
 */
object LogBridge {
    fun d(tag: String, msg: String) {
        Log.d(tag, msg)
        AppLogger.log("D", tag, msg)
        GoProcessPlugin.pushKotlinLog("debug", tag, msg)
    }

    fun i(tag: String, msg: String) {
        Log.i(tag, msg)
        AppLogger.log("I", tag, msg)
        GoProcessPlugin.pushKotlinLog("info", tag, msg)
    }

    fun w(tag: String, msg: String, t: Throwable? = null) {
        if (t != null) Log.w(tag, msg, t) else Log.w(tag, msg)
        AppLogger.log("W", tag, if (t != null) "$msg: ${t.message}" else msg)
        GoProcessPlugin.pushKotlinLog("warn", tag, if (t != null) "$msg: ${t.message}" else msg)
    }

    fun e(tag: String, msg: String, t: Throwable? = null) {
        if (t != null) Log.e(tag, msg, t) else Log.e(tag, msg)
        AppLogger.log("E", tag, if (t != null) "$msg: ${t.message}" else msg)
        GoProcessPlugin.pushKotlinLog("error", tag, if (t != null) "$msg: ${t.message}" else msg, t?.let {
            val sw = java.io.StringWriter(); it.printStackTrace(java.io.PrintWriter(sw)); sw.toString()
        })
    }
}
