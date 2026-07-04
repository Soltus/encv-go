package com.encvgo.combolite.model

data class PluginState(
    val id: String,
    val name: String,
    val versionName: String,
    val versionCode: Long,
    val enabled: Boolean,
    val installed: Boolean,
    val entryClass: String?,
    val description: String? = null,
)
