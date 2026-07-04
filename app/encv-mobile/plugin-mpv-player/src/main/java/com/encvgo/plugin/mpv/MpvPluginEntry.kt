package com.encvgo.plugin.mpv

import android.content.Context
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.platform.LocalContext
import com.combo.core.api.IPluginEntryClass
import com.combo.core.model.PluginContext

class MpvPluginEntry : IPluginEntryClass {
    override val pluginModule = emptyList<org.koin.core.module.Module>()

    override fun onLoad(context: PluginContext) {
    }

    override fun onUnload() {
    }

    @Composable
    override fun Content() {
        val context = LocalContext.current
        val engine = remember { MpvEngine(context) }
        MpvPlayerScreen(
            filePath = "",
            fileName = "",
            mimeType = "",
            isExternal = false,
            backendUrl = "",
            engine = engine,
            onBack = {}
        )
    }
}
