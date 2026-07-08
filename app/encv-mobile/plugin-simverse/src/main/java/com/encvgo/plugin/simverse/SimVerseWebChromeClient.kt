package com.encvgo.plugin.simverse

import android.util.Log
import android.webkit.ConsoleMessage
import android.webkit.WebChromeClient

class SimVerseWebChromeClient(
    private val diagState: SimVerseWebViewClient.WebViewDiagnosticState
) : WebChromeClient() {

    private val tag = "SimVerse-WebChrome"
    private val satTag = "SimVerse-SAT"

    private fun satError(step: String, msg: String) {
        Log.e(satTag, "[$step] $msg")
    }

    override fun onConsoleMessage(consoleMessage: ConsoleMessage?): Boolean {
        if (consoleMessage == null) return false
        val level = consoleMessage.messageLevel()
        val msg = consoleMessage.message()
        val sourceId = consoleMessage.sourceId()
        val lineNumber = consoleMessage.lineNumber()
        val fullMsg = "[$level] $msg ($sourceId:$lineNumber)"

        when (level) {
            ConsoleMessage.MessageLevel.ERROR -> {
                if (diagState.consoleErrors.size < diagState.maxConsoleRecords) {
                    diagState.consoleErrors.add(fullMsg)
                }
                satError("S16-WV-CONSOLE-ERR", fullMsg)
            }
            ConsoleMessage.MessageLevel.WARNING -> {
                if (diagState.consoleWarnings.size < diagState.maxConsoleRecords) {
                    diagState.consoleWarnings.add(fullMsg)
                }
            }
            else -> {}
        }

        Log.e(tag, "console [$level] $msg ($sourceId:$lineNumber)")
        return false
    }

    override fun onProgressChanged(view: WebView?, newProgress: Int) {
        super.onProgressChanged(view, newProgress)
        Log.d(tag, "progress: $newProgress%")
    }

    override fun onReceivedTitle(view: WebView?, title: String?) {
        super.onReceivedTitle(view, title)
        Log.e(tag, "onReceivedTitle: $title")
    }
}
