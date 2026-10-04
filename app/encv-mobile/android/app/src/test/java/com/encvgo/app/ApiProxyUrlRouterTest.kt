package com.encvgo.app

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * ApiProxyUrlRouterTest — ApiProxy 的 URL 路由契约锁（**纯 JVM**，不碰任何 Android 类）
 *
 * 【真机事故 2026-10-03】
 *   现象：安卓真机连不上后端，日志
 *     `[ENCV] Failed to load config: Error: proxy fetch failed:
 *      Failed to connect to localhost/127.0.0.1:443`
 *   而通知栏显示后端端口是 2025。
 *
 *   根因：capacitor.config.ts 配 `server.androidScheme: 'https'` ⇒ WebView 页面的
 *   origin 是 `https://localhost`。前端把它当后端 base 传给本插件后，旧实现对绝对 URL
 *   **原样透传** ⇒ HttpURLConnection 打 localhost + 默认端口 **443** ⇒ connect refused。
 *
 * 本文件钉死两条契约：
 *   1. WebView 自身 origin（localhost 且无显式端口 / 80 / 443）必须重写到真后端端口；
 *   2. 其它本机服务（如 :5244 openlist）与局域网地址必须原样透传，不许误伤。
 */
class ApiProxyUrlRouterTest {

    @Test
    fun rewriteWebViewOriginToRealBackendPort() {
        // 这就是事故里那条会打到 443 的 URL
        assertEquals("http://127.0.0.1:2025/api/config", ApiProxyUrlRouter.resolve("https://localhost/api/config", 2025))
    }

    @Test
    fun rewriteWebViewOriginKeepsQueryString() {
        assertEquals("http://127.0.0.1:2025/api/config?a=1&b=2", ApiProxyUrlRouter.resolve("https://localhost/api/config?a=1&b=2", 2025))
    }

    @Test
    fun rewriteWebViewOriginHonorsPortDrift() {
        // 后端端口 2025 被占时会漂到 2026..2034 —— 重写必须跟到实际端口，不是写死 2025
        assertEquals("http://127.0.0.1:2031/api/config", ApiProxyUrlRouter.resolve("https://localhost/api/config", 2031))
    }

    @Test
    fun rewriteWebViewOriginHandlesImplicitRootPath() {
        assertEquals("http://127.0.0.1:2025/", ApiProxyUrlRouter.resolve("https://localhost", 2025))
    }

    @Test
    fun keepRelativePathRouting() {
        assertEquals("http://127.0.0.1:2025/api/config", ApiProxyUrlRouter.resolve("/api/config", 2025))
        assertEquals("http://127.0.0.1:2025/api/config", ApiProxyUrlRouter.resolve("api/config", 2025))
    }

    @Test
    fun passThroughOtherLoopbackServices() {
        // openlist 跑在 :5244，是 JS 主动要连的本机服务 —— 绝不能被重写到后端端口
        assertEquals("http://127.0.0.1:5244/api/list", ApiProxyUrlRouter.resolve("http://127.0.0.1:5244/api/list", 2025))
        assertEquals("http://localhost:5244/api/list", ApiProxyUrlRouter.resolve("http://localhost:5244/api/list", 2025))
    }

    @Test
    fun passThroughLanAndRemoteHosts() {
        assertEquals("http://192.168.1.99:2025/api/config", ApiProxyUrlRouter.resolve("http://192.168.1.99:2025/api/config", 2025))
        assertEquals("https://encv.example.com/api/config", ApiProxyUrlRouter.resolve("https://encv.example.com/api/config", 2025))
    }

    @Test
    fun keepExplicitlyConfiguredLoopbackPortUntouched() {
        // 前端手配了别的 loopback 端口：按用户意图走，不重写
        assertEquals("http://127.0.0.1:2044/api/config", ApiProxyUrlRouter.resolve("http://127.0.0.1:2044/api/config", 2031))
    }

    @Test
    fun tolerateMalformedUrl() {
        // URI 解析失败（未编码字符等）时保持原值，交给下游 HttpURLConnection 报错，不在这里吞掉
        assertEquals("https://localhost/a b?x=|", ApiProxyUrlRouter.resolve("https://localhost/a b?x=|", 2025))
    }
}
