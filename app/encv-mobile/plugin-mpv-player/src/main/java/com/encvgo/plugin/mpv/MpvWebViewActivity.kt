package com.encvgo.plugin.mpv

import android.os.Bundle
import androidx.activity.compose.setContent
import com.combo.core.component.activity.BasePluginActivity
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.ui.Modifier
import com.encvgo.plugin.mpv.theme.EncvMpVPlayerTheme

class MpvWebViewActivity : BasePluginActivity() {

    companion object {
        private const val TAG = "MpvWebViewActivity"
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val host = proxyActivity ?: return

        android.util.Log.i(TAG, "onCreate")

        host.setContent {
            EncvMpVPlayerTheme {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = MaterialTheme.colorScheme.background
                ) {
                    MpvEmbedWebView()
                }
            }
        }
    }
}
