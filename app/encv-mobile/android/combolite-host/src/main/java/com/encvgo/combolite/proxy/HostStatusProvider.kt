package com.encvgo.combolite.proxy

import android.util.Log
import com.combo.core.component.provider.BaseHostProvider

/**
 * Phase 25 A2：Host 端 ContentProvider 统一代理。
 *
 * Plugin 端的 ContentProvider（如 OpenListStatusProvider）通过插件 manifest
 * 的 <provider> 声明被 [com.combo.core.runtime.installer.InstallerManager] 解析，
 * 存到 [com.combo.core.proxy.ProxyManager.providerRegistry]。
 *
 * Plugin 端调用 `contentResolver.queryPlugin(uri)` 时：
 *   ① build proxy URI: content://<host_authority>/<encoded_plugin_authority>/path
 *   ② Android 系统路由到本 HostStatusProvider（authority = host_authority）
 *   ③ BaseHostProvider.query 解析 proxy URI 还原 plugin authority + class
 *   ④ PluginManager.getInterface(ContentProvider::class.java, className).newInstance()
 *   ⑤ instance.attachInfo(context, null)  ← 关键：必须 attach 才能 query
 *   ⑥ pluginProvider.query(rewrittenUri, ...)
 *
 * authority 必须在 [com.encvgo.combolite.EncvComboLiteHost.setupFramework] 中
 * 通过 setHostProviderAuthority("...") 注入。Manifest 也要声明同 authority。
 *
 * 单一实例即可（plugin provider 在框架侧用 ProviderInfo 区分）。
 */
class HostStatusProvider : BaseHostProvider() {
    companion object {
        private const val TAG = "Encv-HostStatusProvider"
    }

    override fun onCreate(): Boolean {
        Log.i(TAG, "onCreate: host ContentProvider proxy ready")
        return super.onCreate()
    }
}
