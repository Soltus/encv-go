package com.encvgo.plugin.simverse

import android.util.Log
import androidx.compose.runtime.Composable
import com.combo.core.api.IPluginEntryClass
import com.combo.core.model.PluginContext

class SimVersePluginEntry : IPluginEntryClass {

    private val tag = "SimVerse-PluginEntry"

    override val pluginModule = emptyList<org.koin.core.module.Module>()

    override fun onLoad(context: PluginContext) {
        Log.e(tag, "[SimVerse] onLoad() | thread=${Thread.currentThread().name}")
    }

    override fun onUnload() {
        Log.e(tag, "[SimVerse] onUnload()")
    }

    @Composable
    override fun Content() {
        SimVerseEmbedWebView(
            containerId = "simverse-plugin-embed",
            initialUrl = "file:///android_asset/simverse/index.html"
        )
    }
}
