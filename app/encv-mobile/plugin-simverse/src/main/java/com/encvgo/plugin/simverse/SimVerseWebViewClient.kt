package com.encvgo.plugin.simverse

import android.util.Log
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient

class SimVerseWebViewClient : WebViewClient() {

    private val tag = "SimVerse-WebViewClient"

    override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean {
        val url = request?.url
        Log.e(tag, "shouldOverrideUrlLoading: $url")
        if (url == null) return false
        val scheme = url.scheme
        if (scheme == "http" || scheme == "https" || scheme == "file") {
            view?.loadUrl(url.toString())
            return true
        }
        return false
    }

    override fun onPageFinished(view: WebView?, url: String?) {
        super.onPageFinished(view, url)
        Log.e(tag, "onPageFinished: $url")
    }
}
