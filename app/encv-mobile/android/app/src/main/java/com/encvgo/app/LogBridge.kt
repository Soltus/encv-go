package com.encvgo.app

import android.util.Log

object LogBridge {
    fun d(tag: String, msg: String) {
        Log.d(tag, msg)
        AppLogger.log("D", tag, msg)
    }

    fun i(tag: String, msg: String) {
        Log.i(tag, msg)
        AppLogger.log("I", tag, msg)
    }

    fun w(tag: String, msg: String, t: Throwable? = null) {
        if (t != null) Log.w(tag, msg, t) else Log.w(tag, msg)
        AppLogger.log("W", tag, if (t != null) "$msg: ${t.message}" else msg)
    }

    fun e(tag: String, msg: String, t: Throwable? = null) {
        if (t != null) Log.e(tag, msg, t) else Log.e(tag, msg)
        AppLogger.log("E", tag, if (t != null) "$msg: ${t.message}" else msg)
    }
}
