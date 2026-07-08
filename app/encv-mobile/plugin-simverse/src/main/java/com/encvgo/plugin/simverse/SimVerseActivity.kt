package com.encvgo.plugin.simverse

import android.app.AlertDialog
import android.os.Bundle
import android.view.Gravity
import android.view.ViewGroup
import android.widget.Button
import android.widget.FrameLayout
import android.widget.ScrollView
import android.widget.TextView
import androidx.activity.compose.setContent
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.viewinterop.AndroidView
import com.combo.core.component.activity.BasePluginActivity

class SimVerseActivity : BasePluginActivity() {

    companion object {
        private const val TAG = "SimVerseActivity"
        private const val SAT_TAG = "SimVerse-SAT"
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val host = proxyActivity ?: return
        val hostIntent = host.intent ?: return

        val worldId = hostIntent.getStringExtra("world_id") ?: "default"
        val worldName = hostIntent.getStringExtra("world_name") ?: "Default"
        val apiBaseUrl = hostIntent.getStringExtra("api_base_url") ?: ""

        android.util.Log.e(TAG, "onCreate: worldId=$worldId worldName=$worldName apiBaseUrl=$apiBaseUrl")

        host.setContent {
            androidx.compose.foundation.layout.Box(
                modifier = Modifier
                    .fillMaxSize()
                    .background(Color.Black)
            ) {
                SimVerseEmbedWebView(
                    containerId = "simverse-activity-embed",
                )

                AndroidView(
                    modifier = Modifier.fillMaxSize(),
                    factory = { ctx ->
                        FrameLayout(ctx).apply {
                            val diagBtn = Button(ctx).apply {
                                text = "🔍"
                                textSize = 18f
                                setBackgroundColor(0x66000000.toInt())
                                setTextColor(0xFFFFFFFF.toInt())
                                val size = dpToPx(ctx, 48)
                                val params = FrameLayout.LayoutParams(size, size).apply {
                                    gravity = Gravity.TOP or Gravity.END
                                    setMargins(0, dpToPx(ctx, 8), dpToPx(ctx, 8), 0)
                                }
                                layoutParams = params
                                alpha = 0.3f
                                setOnClickListener {
                                    showDiagnosticDialog()
                                }
                                setOnLongClickListener {
                                    showDiagnosticDialog()
                                    true
                                }
                            }
                            addView(diagBtn)
                        }
                    }
                )
            }
        }
    }

    private fun dpToPx(ctx: android.content.Context, dp: Int): Int {
        val density = ctx.resources.displayMetrics.density
        return (dp * density).toInt()
    }

    private fun showDiagnosticDialog() {
        val ctx = proxyActivity ?: return
        val report = SimVerseWebViewDiagnostic.getReport()

        val scrollView = ScrollView(ctx).apply {
            setPadding(dpToPx(ctx, 16), dpToPx(ctx, 16), dpToPx(ctx, 16), dpToPx(ctx, 16))
            val textView = TextView(ctx).apply {
                text = report
                textSize = 11f
                typeface = android.graphics.Typeface.MONOSPACE
                setTextColor(0xFFE0E0E0.toInt())
            }
            addView(textView)
        }

        AlertDialog.Builder(ctx)
            .setTitle("🌍 SimVerse WebView 诊断")
            .setView(scrollView)
            .setPositiveButton("复制") { _, _ ->
                val clipboard = ctx.getSystemService(android.content.Context.CLIPBOARD_SERVICE) as android.content.ClipboardManager
                clipboard.setPrimaryClip(android.content.ClipData.newPlainText("SimVerse WebView Diagnostic", report))
            }
            .setNegativeButton("关闭", null)
            .show()
    }

    override fun onDestroy() {
        super.onDestroy()
        android.util.Log.e(TAG, "onDestroy")
    }
}
