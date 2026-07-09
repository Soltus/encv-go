package com.encvgo.plugin.simverse

import android.util.Log
import androidx.compose.runtime.Composable
import com.combo.core.api.IPluginEntryClass
import com.combo.core.model.PluginContext

class SimVersePluginEntry : IPluginEntryClass {

    private val tag = "SimVerse-PluginEntry"
    private val satTag = "SimVerse-SAT"

    override val pluginModule = emptyList<org.koin.core.module.Module>()

    override fun onLoad(context: PluginContext) {
        Log.e(tag, "[SimVerse] onLoad() | thread=${Thread.currentThread().name}")
    }

    override fun onUnload() {
        Log.e(tag, "[SimVerse] onUnload()")
    }

    fun debugWebView(): String {
        return try {
            val report = SimVerseWebViewDiagnostic.getReport()
            Log.e(satTag, "[S14-WV-DIAG] debugWebView called:\n$report")
            report
        } catch (e: Exception) {
            "❌ WebView 诊断失败: ${e.javaClass.simpleName}: ${e.message}"
        }
    }

    @Composable
    override fun Content() {
        SimVerseEmbedWebView(
            containerId = "simverse-plugin-embed",
        )
    }
}
