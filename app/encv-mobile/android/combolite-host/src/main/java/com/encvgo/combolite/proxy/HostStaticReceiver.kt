package com.encvgo.combolite.proxy

import android.util.Log
import com.combo.core.component.receiver.BaseHostReceiver

/**
 * Phase 25 A2：Host 端 BroadcastReceiver 统一代理。
 *
 * Plugin 端的静态广播接收器通过插件 manifest 的 <receiver> 声明被
 * [com.combo.core.runtime.installer.InstallerManager.parseStaticReceivers] 解析，
 * 存到 [com.combo.core.proxy.ProxyManager.staticReceiverRegistry]。
 *
 * Plugin 端 plugin manifest 示例：
 *   <receiver android:name=".MyReceiver" android:exported="false">
 *     <intent-filter>
 *       <action android:name="com.encvgo.plugin.openlist.MY_ACTION" />
 *     </intent-filter>
 *   </receiver>
 *
 * Host 端触发（plugin 端不能用裸 sendBroadcast，必须用 sendInternalBroadcast）：
 *   context.sendInternalBroadcast(Intent("com.encvgo.plugin.openlist.MY_ACTION").apply {
 *       putExtra("key", "value")
 *   })
 *
 * BaseHostReceiver.onReceive 自动：
 *   ① goAsync() + 协程
 *   ② PluginManager.proxyManager.findReceiversForIntent(intent) 匹配 plugin receivers
 *   ③ 对每个匹配：newInstance + pluginReceiver.onReceive(context, intent)
 *
 * 单一实例即可，框架按 intent-filter action 自动分发。
 * Manifest 必须 exported=true，且建议加足够 intent-filter 覆盖 plugin 端的 action。
 */
class HostStaticReceiver : BaseHostReceiver() {
    companion object {
        private const val TAG = "Encv-HostStaticReceiver"
    }

    override fun onReceive(context: android.content.Context, intent: android.content.Intent) {
        Log.i(TAG, "onReceive: action=${intent.action} extras=${intent.extras?.keySet()}")
        super.onReceive(context, intent)
    }
}
