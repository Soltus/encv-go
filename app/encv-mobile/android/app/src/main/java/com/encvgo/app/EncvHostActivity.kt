package com.encvgo.app

import android.content.Intent
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import com.combo.core.component.activity.BaseHostActivity

class EncvHostActivity : BaseHostActivity() {
    private var resultSet = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        if (pluginActivity == null) {
            LogBridge.e(TAG, "onCreate: pluginActivity is null — plugin load failed, finishing with error")
            finishWithResult(false, "播放器加载失败", "pluginActivity==null after initPluginActivity()")
            return
        }

        LogBridge.i(TAG, "onCreate: plugin loaded successfully: ${pluginActivity?.javaClass?.name}")

        Handler(Looper.getMainLooper()).postDelayed({
            if (!isFinishing && !resultSet) {
                LogBridge.w(TAG, "Timeout: activity still alive after ${PROXY_TIMEOUT_MS}ms, no result set")
            }
        }, PROXY_TIMEOUT_MS)
    }

    override fun onDestroy() {
        if (!resultSet) {
            LogBridge.i(TAG, "onDestroy: setting default success result")
            setResult(RESULT_OK, Intent().apply {
                putExtra("player_success", true)
                putExtra("player_error", "")
                putExtra("player_error_detail", "")
            })
            resultSet = true
        }
        super.onDestroy()
    }

    private fun finishWithResult(success: Boolean, error: String, detail: String) {
        setResult(RESULT_OK, Intent().apply {
            putExtra("player_success", success)
            putExtra("player_error", error)
            putExtra("player_error_detail", detail)
        })
        resultSet = true
        finish()
    }

    companion object {
        const val TAG = "EncvHostActivity"
        const val PROXY_TIMEOUT_MS = 5000L
    }
}
