package com.encvgo.app

import android.app.Application
import com.tencent.bugly.crashreport.CrashReport
import com.combo.core.runtime.app.BaseHostApplication
import com.encvgo.combolite.EncvComboLiteHost
import android.util.Log

class EncvApplication : BaseHostApplication() {
    companion object {
        private const val TAG = "ENCV-App"
    }

    override fun onCreate() {
        super.onCreate()
        initBugly()
    }

    override fun onFrameworkSetup(): suspend () -> Unit = {
        // Phase 25 A2：注册 Service 池（16 个）+ Provider 代理 authority
        val servicePool = listOf(
            com.encvgo.combolite.proxy.HostService1::class.java,
            com.encvgo.combolite.proxy.HostService2::class.java,
            com.encvgo.combolite.proxy.HostService3::class.java,
            com.encvgo.combolite.proxy.HostService4::class.java,
            com.encvgo.combolite.proxy.HostService5::class.java,
            com.encvgo.combolite.proxy.HostService6::class.java,
            com.encvgo.combolite.proxy.HostService7::class.java,
            com.encvgo.combolite.proxy.HostService8::class.java,
            com.encvgo.combolite.proxy.HostService9::class.java,
            com.encvgo.combolite.proxy.HostService10::class.java,
            com.encvgo.combolite.proxy.HostService11::class.java,
            com.encvgo.combolite.proxy.HostService12::class.java,
            com.encvgo.combolite.proxy.HostService13::class.java,
            com.encvgo.combolite.proxy.HostService14::class.java,
            com.encvgo.combolite.proxy.HostService15::class.java,
            com.encvgo.combolite.proxy.HostService16::class.java,
        )
        EncvComboLiteHost.setupFramework(
            hostActivityClass = EncvHostActivity::class.java,
            hostServicePool = servicePool,
            hostProviderAuthority = "com.encvgo.app.provider",
        )
        Log.i(TAG, "onFrameworkSetup: complete via EncvComboLiteHost, PluginManager.isInitialized=${com.combo.core.runtime.PluginManager.isInitialized}")
    }

    private fun initBugly() {
        try {
            val appId = BuildConfig.BUGLY_APP_ID
            if (appId.isEmpty()) {
                Log.w("ENCV-Bugly", "BUGLY_APP_ID not configured, skipping")
                return
            }
            CrashReport.initCrashReport(applicationContext, appId, false)
            Log.i("ENCV-Bugly", "Bugly initialized: appId=$appId")
        } catch (e: Exception) {
            Log.e("ENCV-Bugly", "Failed to initialize Bugly", e)
        }
    }
}
