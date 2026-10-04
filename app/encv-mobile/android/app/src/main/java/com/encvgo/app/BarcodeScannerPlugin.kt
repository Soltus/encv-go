package com.encvgo.app

import android.Manifest
import android.app.Activity
import android.content.Intent
import android.util.Log
import com.getcapacitor.JSArray
import com.getcapacitor.JSObject
import com.getcapacitor.PermissionState
import com.getcapacitor.Plugin
import com.getcapacitor.PluginCall
import com.getcapacitor.PluginMethod
import com.getcapacitor.annotation.CapacitorPlugin
import com.getcapacitor.annotation.Permission
import com.getcapacitor.annotation.PermissionCallback
import com.king.camera.scan.CameraScan

/**
 * BarcodeScannerPlugin —— Capacitor 扫码插件（ZXingLite 实现，2026-10-04）
 *
 * 背景：此前用 `@capacitor-mlkit/barcode-scanning`，它依赖 **Google Play 服务**，
 *   没装 GMS 的设备上扫码不可用。改为自建插件 + ZXingLite（纯本地解码，零 Google 依赖）。
 *
 * 对外 API **刻意与 ML Kit 插件保持一致**（scan → { barcodes: [{ rawValue, displayValue, format }] }），
 *   这样前端 src/peerlink/barcodeScanner.ts 不需要改调用方式。
 *
 * ⚠️ requestPermissions / checkPermissions 在 Capacitor 的 Plugin 基类里**已存在**，
 *    必须用 override（否则 Kotlin 报 "hides member of supertype"）。
 */
@CapacitorPlugin(
    name = "BarcodeScanner",
    permissions = [
        Permission(strings = [Manifest.permission.CAMERA], alias = BarcodeScannerPlugin.CAMERA_ALIAS),
    ],
)
class BarcodeScannerPlugin : Plugin() {

    companion object {
        private const val TAG = "BarcodeScanner"
        private const val SCAN_REQUEST_CODE = 9231
        internal const val CAMERA_ALIAS = "camera"
    }

    private var pendingCall: PluginCall? = null

    private fun cameraGranted(): Boolean = getPermissionState(CAMERA_ALIAS) == PermissionState.GRANTED

    @PluginMethod
    override fun requestPermissions(call: PluginCall) {
        if (cameraGranted()) {
            call.resolve(JSObject().apply { put(CAMERA_ALIAS, "granted") })
            return
        }
        requestPermissionForAlias(CAMERA_ALIAS, call, "cameraPermissionCallback")
    }

    @PluginMethod
    override fun checkPermissions(call: PluginCall) {
        // 与 Capacitor 惯例一致：granted / denied / prompt
        val value = when (getPermissionState(CAMERA_ALIAS)) {
            PermissionState.GRANTED -> "granted"
            PermissionState.DENIED -> "denied"
            else -> "prompt"
        }
        call.resolve(JSObject().apply { put(CAMERA_ALIAS, value) })
    }

    @PluginMethod
    fun scan(call: PluginCall) {
        if (pendingCall != null) {
            call.reject("scan already in progress")
            return
        }
        if (!cameraGranted()) {
            call.reject("camera permission not granted")
            return
        }
        pendingCall = call
        startActivityForResult(call, Intent(context, QRScanActivity::class.java), SCAN_REQUEST_CODE)
    }

    @PluginMethod
    fun stopScan(call: PluginCall) {
        // 扫码界面命中即 finish；这里只清理挂起的调用
        pendingCall?.reject("scan cancelled")
        pendingCall = null
        call.resolve()
    }

    override fun handleOnActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.handleOnActivityResult(requestCode, resultCode, data)
        if (requestCode != SCAN_REQUEST_CODE) return

        val call = pendingCall ?: return
        pendingCall = null

        if (resultCode != Activity.RESULT_OK) {
            call.reject("scan cancelled")
            return
        }
        val text = data?.getStringExtra(CameraScan.SCAN_RESULT)
        if (text.isNullOrBlank()) {
            call.reject("no barcode detected")
            return
        }
        Log.i(TAG, "scan result captured (len=${text.length})")

        val barcode = JSObject().apply {
            put("rawValue", text)
            put("displayValue", text)
            put("format", "QR_CODE")
        }
        call.resolve(JSObject().apply { put("barcodes", JSArray().apply { put(barcode) }) })
    }

    /**
     * 相机权限申请的**回调**（由 [requestPermissionForAlias] 按名字回调到这里）。
     *
     * ⚠️ 必须是 `@PermissionCallback`，**不是** `@PluginMethod`（2026-10-04 真机 bug）：
     *   写成 @PluginMethod 时 Capacitor 在权限申请结束后找不到回调，直接抛
     *   `There is no PermissionCallback method registered for the name: cameraPermissionCallback`
     *   ⇒ 扫码在真机上一步都走不出去。方法名必须与 requestPermissionForAlias 的第三个参数一致。
     */
    @PermissionCallback
    fun cameraPermissionCallback(call: PluginCall) {
        call.resolve(JSObject().apply {
            put(CAMERA_ALIAS, if (cameraGranted()) "granted" else "denied")
        })
    }
}
