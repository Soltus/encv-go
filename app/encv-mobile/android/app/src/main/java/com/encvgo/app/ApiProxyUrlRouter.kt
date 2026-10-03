package com.encvgo.app

/**
 * ApiProxyUrlRouter — ApiProxy 的 URL 路由（**纯 Kotlin，零 Android / Capacitor 依赖**）
 *
 * 为什么抽成独立 object：
 *   - 这段逻辑是 2026-10-03 真机事故的 native 侧防线，必须有 JVM 单测锁住；
 *     但 `ApiProxyPlugin` 继承 `com.getcapacitor.Plugin`，实例化它会拉进 Android 依赖，
 *     让 unit test 变得脆弱（需 Robolectric / mocking）。
 *   - 抽成 object 后，`ApiProxyUrlRouterTest` 可以纯 JVM 直接测，不需要任何 Android 类。
 *
 * 【2026-10-03 真机事故】
 *   现象：`Failed to connect to localhost/127.0.0.1:443`（后端实际在 :2025）。
 *   链路：`capacitor.config.ts` 配 `server.androidScheme: 'https'`
 *        ⇒ WebView 页面 origin = `https://localhost`
 *        ⇒ 前端把它当后端 base ⇒ `https://localhost/api/config` 传给 ApiProxy
 *        ⇒ 旧实现对绝对 URL **原样透传** ⇒ HttpURLConnection 打 localhost 默认端口 443
 *   防线：见 [rewriteWebViewOrigin]。
 */
object ApiProxyUrlRouter {

    /**
     * 解析 JS 端发来的 url：
     *   - WebView 自身的 origin（`https://localhost`）→ 重写到真实后端 [backendPort]
     *   - 其它绝对 URL（`http://` / `https://`）原样用
     *   - 相对路径（`/api/...` / `api/...`）→ http://127.0.0.1:<backendPort>/api/...
     */
    fun resolve(url: String, backendPort: Int): String {
        if (url.startsWith("http://") || url.startsWith("https://")) return rewriteWebViewOrigin(url, backendPort)
        val path = if (url.startsWith("/")) url else "/$url"
        return "http://127.0.0.1:$backendPort$path"
    }

    /**
     * 把「WebView 自身的 origin」重写到真实后端地址。
     *
     * 判定范围刻意收得很窄（只认「没显式端口 / 80 / 443」的 localhost），因为：
     *   - 带明确端口的 loopback 服务（如 :5244 的 openlist）是 JS 主动要连的本机服务，
     *     重写到后端端口会把请求打到错误的上游；
     *   - 局域网地址（192.168.x.x）同理，必须透传。
     */
    internal fun rewriteWebViewOrigin(url: String, backendPort: Int): String {
        val uri = runCatching { java.net.URI(url) }.getOrNull() ?: return url
        val host = uri.host ?: return url
        if (!host.equals("localhost", ignoreCase = true)) return url
        val explicitPort = uri.port
        if (explicitPort > 0 && explicitPort != 80 && explicitPort != 443) return url

        val path = if (uri.rawPath.isNullOrBlank()) "/" else uri.rawPath
        val query = if (uri.rawQuery.isNullOrBlank()) "" else "?${uri.rawQuery}"
        return "http://127.0.0.1:$backendPort$path$query"
    }
}
