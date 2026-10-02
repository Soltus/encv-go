// barcodeScanner.ts —— 扫码端（安卓）相机扫描抽象（spec P2b Task 2.6）
//
// 为什么用 `registerPlugin('BarcodeScanner')` 而不是直接 import 插件包：
//   1. 插件（`@capacitor-mlkit/barcode-scanning`）**只在原生工程里存在**，web 构建里没有它；
//      直接静态 import 会让 web 产物构建失败（web 端根本不需要相机）。
//   2. Capacitor 的 `registerPlugin` 按**插件名**拿到运行时代理 —— 原生侧装好插件后
//      （`pnpm add @capacitor-mlkit/barcode-scanning` + `npx cap sync android`），
//      同一个名字即可解析到真实实现；没装时调用会 reject，而不是编译失败。
//
// ⚠️ 失败必须可见（今天多次踩到"静默失败"的坑）：本模块**不吞错误**，
//    相机不可用 / 权限被拒 / 用户取消 都以明确的错误码抛给 UI 展示。
//
// ⚠️ 真机前置（沙箱无法验证，已列入 checklist）：
//   · 依赖：`@capacitor-mlkit/barcode-scanning`（peer: @capacitor/core >= 8，本仓库 8.3.4 满足）
//   · `npx cap sync android` 且设备有 Google Play services（MLKit 依赖）
//   · AndroidManifest 相机权限由插件自带；首次调用走 requestPermissions()

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

/** 我们只用到插件的这两个能力（与 @capacitor-mlkit/barcode-scanning 的 API 对齐）。 */
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
    throw new ScanError("unavailable", "扫码插件不可用：需安装 @capacitor-mlkit/barcode-scanning 并 cap sync android");
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
