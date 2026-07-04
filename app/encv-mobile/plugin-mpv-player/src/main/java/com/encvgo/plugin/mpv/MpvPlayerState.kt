package com.encvgo.plugin.mpv

sealed interface PlayerState {
    data object Idle : PlayerState
    data object Loading : PlayerState
    data object Playing : PlayerState
    data object Paused : PlayerState
    data object Ended : PlayerState
    data class Error(val errorType: MpvError, val detail: String = "") : PlayerState
    data object AudioOnly : PlayerState
}

enum class MpvError(val messageResId: Int? = null) {
    FILE_NOT_FOUND(null),
    NETWORK_ERROR(null),
    DECODE_ERROR(null),
    PERMISSION_DENIED(null),
    ADDRESS_INVALID(null),
    UNKNOWN(null)
}

fun classifyError(err: String): MpvError {
    val lower = err.lowercase()
    return when {
        lower.contains("播放失败") || lower.contains("corrupt") || lower.contains("damaged") ||
        lower.contains("invalid data") || lower.contains("demux") -> MpvError.DECODE_ERROR
        lower.contains("network") || lower.contains("connection") || lower.contains("timeout") ||
        lower.contains("http 4") || lower.contains("http 5") -> MpvError.NETWORK_ERROR
        lower.contains("not found") || lower.contains("no such file") || lower.contains("enoent") ||
        lower.contains("does not exist") -> MpvError.FILE_NOT_FOUND
        lower.contains("decode") || lower.contains("codec") || lower.contains("format") ||
        lower.contains("unsupported") -> MpvError.DECODE_ERROR
        lower.contains("empty") || lower.contains("为空") || lower.contains("地址") -> MpvError.ADDRESS_INVALID
        lower.contains("permission") || lower.contains("forbidden") || lower.contains("denied") ||
        lower.contains("access") -> MpvError.PERMISSION_DENIED
        else -> MpvError.UNKNOWN
    }
}

fun MpvError.displayMessage(): String = when (this) {
    MpvError.FILE_NOT_FOUND -> "文件不存在 / File Not Found"
    MpvError.NETWORK_ERROR -> "网络错误 / Network Error"
    MpvError.DECODE_ERROR -> "解码失败 / Decode Error"
    MpvError.PERMISSION_DENIED -> "权限不足 / Permission Denied"
    MpvError.ADDRESS_INVALID -> "地址无效 / Invalid Address"
    MpvError.UNKNOWN -> "未知错误 / Unknown Error"
}
