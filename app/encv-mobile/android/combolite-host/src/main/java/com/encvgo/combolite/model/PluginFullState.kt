package com.encvgo.combolite.model

data class PluginFullState(
    val id: String,
    val status: String,
    val name: String? = null,
    val version: String? = null,
)