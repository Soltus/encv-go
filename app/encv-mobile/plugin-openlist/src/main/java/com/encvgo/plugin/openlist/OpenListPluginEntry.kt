package com.encvgo.plugin.openlist

import android.content.Context
import android.util.Log
import androidx.compose.runtime.Composable
import com.combo.core.api.IPluginEntryClass
import com.combo.core.model.PluginContext
import org.koin.core.module.Module

/**
 * ComboLite 插件入口（合规修复版）
 *
 * 合规修复（参考 MpvPluginEntry 模式）：
 *   - pluginModule = emptyList()（不注册 Koin module）
 *   - onLoad() 初始化运行时（OpenListBridge + Service）
 *   - onUnload() 清理运行时
 *   - Content() 渲染嵌入式 WebView 承载 OpenList UI（详见 OpenListEmbedWebView）
 *
 * UI 架构变更（spec openlist-extension-rewrite-capacitor-ui）：
 *   - 删除原本的 Compose Material3 UI（StatusCard/ControlCard/ConfigCard/InfoGrid）
 *   - 所有用户交互通过嵌入式 WebView + JSInterface（@encvgo/components + plugin-openlist/web）
 *   - 与主 app 共享 Vue 组件（pnpm workspace）
 *
 * 状态共享（与 UI 无关）：
 *   - 主 app 通过 ContentProvider 读 OpenListStatusProvider
 *   - 插件 Content() 内 WebView 通过 OpenListPluginJSInterface 调 OpenListBridge
 */
class OpenListPluginEntry : IPluginEntryClass {

    private val tag = "OpenList-PluginEntry"

    /**
     * 仿 MpvPluginEntry: 不注册任何 Koin module
     * （Bridge 初始化在 onLoad 中完成，Service 由 OpenListPluginJSInterface 按需触发）
     */
    override val pluginModule: List<Module> = emptyList()

    /**
     * ComboLite 框架加载插件时调用。
     *
     * 修复：原来几乎为空，运行时启动委托给 OpenListPluginService（Phase 25 前是 OpenListService）。
     * 现在：初始化 OpenListBridge + OpenListConfig（与 MpvPluginEntry 的 Content() 内
     * 初始化模式一致，只是提到 onLoad 阶段）。
     */
    /**
     * Phase 26: 改为 OpenListNativeService（仿 host app 的 EncvGoService，ProcessBuilder
     * 启 libopenlist.so）。不再调 OpenListBridge。
     */
    override fun onLoad(context: PluginContext) {
        Log.e(tag, "[OpenList] onLoad() | thread=${Thread.currentThread().name}")
        try {
            val appCtx: Context = context.application
            val cfg = OpenListConfig.load(appCtx)
            // 注入 ctx + config 到 native service（不立即启进程——启进程在 service.onStartCommand）
            OpenListNativeService.init(appCtx)
            OpenListNativeService.setPort(cfg.port)
            OpenListNativeService.setDataDir(cfg.dataDir)
            Log.e(tag, "[OpenList] onLoad() OK | port=${cfg.port} dataDir=${cfg.dataDir}")
        } catch (t: Throwable) {
            Log.e(tag, "[OpenList] onLoad() FAILED", t)
        }
    }

    /**
     * ComboLite 框架卸载插件时调用。
     *
     * 修复：原来只 log，运行时继续在后台。
     * 现在：shutdown NativeService + PluginService（彻底清理）。
     *
     * Phase 26: 改为 shutdown OpenListNativeService（ProcessBuilder 启的 Go 进程）。
     * 不再调 OpenListBridge.shutdown。
     */
    override fun onUnload() {
        Log.e(tag, "[OpenList] onUnload()")
        try {
            // 停止 plugin service（如有运行）
            OpenListPluginService.stopIfRunning()
            // shutdown native Go server process
            OpenListNativeService.shutdown(5_000L)
            Log.e(tag, "[OpenList] onUnload() OK")
        } catch (t: Throwable) {
            Log.e(tag, "[OpenList] onUnload() FAILED", t)
        }
    }

    /**
     * 渲染插件主页面。
     *
     * 修复：原来是 400 行 Compose Material3 UI（StatusCard/ControlCard/ConfigCard/InfoGrid）。
     * 现在：嵌入式 Android WebView + JSInterface（详见 OpenListEmbedWebView），
     *      加载 plugin-openlist/web 产出的 Vite bundle。
     *
     * 占位 Box：作为 fallback，当宿主不支持 AndroidView 时显示最小提示。
     * 实际渲染：宿主调用 OpenListEmbedWebView 作为 Content() 的实现。
     */
    @Composable
    override fun Content() {
        // Phase 26: WebView 加载 OpenList Go server 提供的 Web UI (http://127.0.0.1:5244/)
        // —— OpenListNativeService.start() 启 Go 进程后,OpenList server 监听 127.0.0.1:5244
        OpenListEmbedWebView(
            containerId = "openlist-plugin-embed",
            initialUrl = "http://127.0.0.1:5244/"
        )
    }
}
