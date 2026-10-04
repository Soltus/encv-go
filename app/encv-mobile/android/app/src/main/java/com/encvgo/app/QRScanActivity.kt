package com.encvgo.app

import android.content.Intent
import com.google.zxing.Result
import com.king.camera.scan.AnalyzeResult
import com.king.camera.scan.CameraScan
import com.king.camera.scan.analyze.Analyzer
import com.king.zxing.BarcodeCameraScanActivity
import com.king.zxing.DecodeConfig
import com.king.zxing.DecodeFormatManager
import com.king.zxing.analyze.QRCodeAnalyzer

/**
 * QRScanActivity —— 扫码界面（ZXingLite，2026-10-04 替换 ML Kit）
 *
 * 为什么换：原实现走 `@capacitor-mlkit/barcode-scanning`，它依赖 **Google Play 服务**
 * （ML Kit / Google Code Scanner）⇒ 没有 GMS 的设备（国内多数 ROM、类原生系统）用不了。
 * ZXingLite 是纯本地解码（ZXing 精简版），零 Google 依赖。
 *
 * 用法与签名均取自 ZXingLite 3.4.1 实际字节码（javap 核对，不靠记忆）：
 *   BarcodeCameraScanActivity : BaseCameraScanActivity<com.google.zxing.Result>
 *   override fun createAnalyzer(): Analyzer<Result>
 *   AnalyzeResult<Result>.getResult()   ← 注意是 getResult()，不是 .result 字段
 *
 * 只识别二维码（QRCodeAnalyzer + QR_CODE_HINTS）：本项目扫码只用于读配对码，
 * 收窄格式更快也避免误扫其它条码。布局用库自带的 zxl_camera_scan.xml（不覆写 getLayoutId）。
 */
class QRScanActivity : BarcodeCameraScanActivity() {

    override fun createAnalyzer(): Analyzer<Result> {
        val decodeConfig = DecodeConfig()
            .setHints(DecodeFormatManager.QR_CODE_HINTS) // 只解二维码，效率更高
            .setFullAreaScan(false) // 按取景框裁剪识别，更快更准
            .setAreaRectRatio(0.8f) // 识别区域比例（默认 0.8）
        return QRCodeAnalyzer(decodeConfig)
    }

    // ⚠️ 参数是**非空**的 AnalyzeResult<Result>（javap：onScanResultCallback(AnalyzeResult<T>)），
    //    写成可空类型会导致 "overrides nothing"。
    override fun onScanResultCallback(result: AnalyzeResult<Result>) {
        // 命中即停止分析，避免同一张码被反复回调
        cameraScan?.setAnalyzeImage(false)
        val text = result.getResult()?.text
        val intent = Intent().apply {
            putExtra(CameraScan.SCAN_RESULT, text)
        }
        setResult(RESULT_OK, intent)
        finish()
    }
}
