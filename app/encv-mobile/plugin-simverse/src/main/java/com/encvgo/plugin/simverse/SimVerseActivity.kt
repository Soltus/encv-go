package com.encvgo.plugin.simverse

import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import com.combo.core.component.activity.BasePluginActivity

class SimVerseActivity : BasePluginActivity() {

    companion object {
        private const val TAG = "SimVerseActivity"
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val host = proxyActivity ?: return
        val hostIntent = host.intent ?: return

        val worldId = hostIntent.getStringExtra("world_id") ?: "default"
        val worldName = hostIntent.getStringExtra("world_name") ?: "Default"
        val apiBaseUrl = hostIntent.getStringExtra("api_base_url") ?: ""

        android.util.Log.e(TAG, "onCreate: worldId=$worldId worldName=$worldName apiBaseUrl=$apiBaseUrl")

        host.setContent {
            androidx.compose.foundation.layout.Box(
                modifier = Modifier
                    .fillMaxSize()
                    .background(Color.Black)
            ) {
                SimVerseEmbedWebView(
                    containerId = "simverse-activity-embed",
                    initialUrl = "file:///android_asset/simverse/index.html"
                )
            }
        }
    }

    override fun onDestroy() {
        super.onDestroy()
        android.util.Log.e(TAG, "onDestroy")
    }
}
