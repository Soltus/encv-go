package com.encvgo.app

import java.util.concurrent.ConcurrentLinkedQueue

object AppLogger {
    private val buffer = ConcurrentLinkedQueue<String>()
    private const val MAX = 3000

    fun log(level: String, tag: String, msg: String) {
        val entry = "${System.currentTimeMillis()} $level/$tag: $msg"
        buffer.add(entry)
        while (buffer.size > MAX) {
            buffer.poll()
        }
    }

    fun getLogs(): String = buffer.joinToString("\n")

    fun clear() = buffer.clear()
}
