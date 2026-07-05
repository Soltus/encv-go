package com.encvgo.app

import android.os.Bundle
import android.view.View
import android.view.WindowManager
import androidx.appcompat.app.AppCompatActivity
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient

class WorldActivity : AppCompatActivity() {
    private lateinit var webView: WebView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        
        // 初始竖屏（显示 SimVerse 首页）
        requestedOrientation = android.content.pm.ActivityInfo.SCREEN_ORIENTATION_PORTRAIT
        
        // 设置状态栏透明
        window.statusBarColor = android.graphics.Color.TRANSPARENT
        val decorView = window.decorView
        decorView.systemUiVisibility = (
            View.SYSTEM_UI_FLAG_LAYOUT_STABLE
            or View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
        )
        window.addFlags(WindowManager.LayoutParams.FLAG_DRAWS_SYSTEM_BAR_BACKGROUNDS)
        
        // 创建 WebView
        webView = WebView(this)
        setContentView(webView)
        
        // WebView 设置
        val settings = webView.settings
        settings.javaScriptEnabled = true
        settings.domStorageEnabled = true
        settings.mediaPlaybackRequiresUserGesture = false
        settings.mixedContentMode = WebSettings.MIXED_CONTENT_ALWAYS_ALLOW
        
        // 加载 simverse-frontend（路由到 /simverse-home）
        webView.webViewClient = WebViewClient()
        webView.loadUrl("http://10.0.2.2:8200/#/simverse-home")
        
        // 桥接：通知 Go 后端世界页面已打开
        EncvGoService.sendCommand("world_activity_opened")
    }
    
    override fun onBackPressed() {
        if (webView.canGoBack()) {
            webView.goBack()
        } else {
            // 退出世界，通知 Go 后端 checkpoint
            EncvGoService.sendCommand("world_activity_closed")
            finish()
        }
    }
    
    override fun onDestroy() {
        webView.destroy()
        super.onDestroy()
    }
}
