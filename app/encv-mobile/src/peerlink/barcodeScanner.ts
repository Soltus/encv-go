// barcodeScanner.ts —— 扫码端（安卓）相机扫描抽象（spec P2b Task 2.6）
//
// 为什么用 `registerPlugin('BarcodeScanner')` 而不是直接 import 插件包：
//   1. 插件**只在原生工程里存在**，web 构建里没有它；
//      直接静态 import 会让 web 产物构建失败（web 端根本不需要相机）。
//   2. Capacitor 的 `registerPlugin` 按**插件名**拿到运行时代理 —— 原生侧插件就绪后，
//      同一个名字即可解析到真实实现；没装时调用会 reject，而不是编译失败。
//
// 🆕 2026-10-04：原生实现从 `@capacitor-mlkit/barcode-scanning` 换成**自建 ZXingLite 插件**。
//   原因：ML Kit 依赖 **Google Play 服务**，没有 GMS 的设备（国内多数 ROM、类原生系统）
//   扫码不可用；ZXingLite 是纯本地解码（ZXing 精简版），零 Google 依赖。
//   ⇒ **本文件的 API 形状刻意保持不变**（scan → { barcodes: [{ rawValue }] }），
//     所以前端不需要改调用方式，只换了底层实现。
//
// ⚠️ 失败必须可见（今天多次踩到"静默失败"的坑）：本模块**不吞错误**，
//    相机不可用 / 权限被拒 / 用户取消 都以明确的错误码抛给 UI 展示。
//
// ⚠️ 原生侧前置（不再需要 npm 插件与 cap sync，改为随 APK 一起编译）：
//   · 插件实现：`android/app/src/main/java/com/encvgo/app/BarcodeScannerPlugin.kt`
//      + 扫码界面 `QRScanActivity.kt`（继承 ZXingLite 的 BarcodeCameraScanActivity）
//   · **必须在 MainActivity 注册**：`registerPlugin(BarcodeScannerPlugin::class.java)`
//     （自建插件不会像 npm 插件那样自动进 capacitor.plugins.json）
//   · 依赖：`com.github.jenly1314:zxing-lite:3.4.1` + `camera-scan:1.5.0`
//     （settings.gradle.kts 里 com.github.* 只把 `com.github.getActivity` 路由到 JitPack，
//      jenly1314 走 Maven Central 镜像 —— 它不在 JitPack 上）
//   · **必须声明 `android.permission.CAMERA`**：插件不自带权限 ⇒ 不声明则系统设置里
//     没有相机开关，用户连手动授权都做不到（曾踩过）。
//   · 无相机设备（TV/盒子）走 `canScan() === false` → UI 降级为「粘贴配对码」。

import { Capacitor, registerPlugin } from "@capacitor/core";

export type ScanFailureReason = "unsupported" | "permission_denied" | "cancelled" | "unavailable" | "unknown";

export class ScanError extends Error {
  readonly reason: ScanFailureReason;
  constructor(reason: ScanFailureReason, message: string) {
    super(message);
    this.name = "ScanError";
    this.reason = reason;
  }
}

/** 我们只用到插件的这两个能力（自建插件与 ML Kit 插件的 API 形状保持一致）。 */
interface BarcodeScannerLike {
  requestPermissions?: () => Promise<{ camera?: string }>;
  scan?: (opts?: { formats?: string[] }) => Promise<{ barcodes: Array<{ rawValue?: string; displayValue?: string }> }>;
}

// 未装插件时这是个"空代理"：调用会 reject（下面用 isAvailable 提前判定，给出可读错误）
const BarcodeScanner = registerPlugin<BarcodeScannerLike>("BarcodeScanner");

/** 当前环境是否**真的**能扫码（web 上 Capacitor.isPluginAvailable 只看 web stub，不可信）。 */
export function canScan(): boolean {
  return Capacitor.isNativePlatform() && Capacitor.isPluginAvailable("BarcodeScanner") && typeof BarcodeScanner?.scan === "function";
}

/**
 * 扫一次二维码，返回原始文本。
 * @throws ScanError（reason 供 UI 给出对应文案）
 */
export async function scanOnce(): Promise<string> {
  if (!Capacitor.isNativePlatform()) {
    throw new ScanError("unsupported", "当前不是原生环境（桌面浏览器无相机），请改用「粘贴配对码」");
  }
  if (!canScan()) {
    throw new ScanError("unavailable", "扫码插件不可用：需确认 APK 内置了 BarcodeScanner 插件（ZXingLite）并已授予相机权限");
  }
  const perms = await BarcodeScanner.requestPermissions?.();
  if (perms && perms.camera && perms.camera !== "granted") {
    throw new ScanError("permission_denied", "没有相机权限，请在系统设置里授权后重试");
  }
  try {
    const res = await BarcodeScanner.scan?.({ formats: ["QR_CODE"] });
    const value = res?.barcodes?.[0]?.rawValue || res?.barcodes?.[0]?.displayValue || "";
    if (!value) {
      throw new ScanError("cancelled", "没有识别到二维码（或已取消）");
    }
    return value;
  } catch (e) {
    if (e instanceof ScanError) throw e;
    // 用户主动取消：插件以字符串 "User cancelled the scan" 之类抛出
    const msg = e instanceof Error ? e.message : String(e);
    if (/cancel/i.test(msg)) throw new ScanError("cancelled", msg);
    throw new ScanError("unknown", msg);
  }
}
