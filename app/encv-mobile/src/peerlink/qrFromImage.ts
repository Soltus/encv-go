// qrFromImage.ts —— 「从相册选择图片」解码二维码（spec P2b Task 2.6 的降级/补充路径）
//
// 为什么不用原生扫码插件去解码图片（原 MLKit 的 readBarcodesFromImage）：
//   · 它要求**原生文件路径**，而相册选择拿到的是 File/Blob；还要再绕 Filesystem 落盘，
//     链路长且**只能原生跑，沙箱/桌面浏览器一条路径都验不了**（曾在这类原生路径上栽过）。
//   · `jsqr` 是**纯 JS 解码器**（无原生依赖），在 WebView 与桌面浏览器走同一份代码
//     ⇒ 这条路径可以在真实浏览器里端到端验证。
//   · 相机实时扫描走原生 `BarcodeScanner` 插件（2026-10-04 起是 ZXingLite 实现，
//     不再依赖 Google Play 服务）。
//
// ⚠️ 失败必须可见：解码不出来返回明确的错误，由 UI 展示，绝不静默。

import jsQR from "jsqr";

export class QRDecodeError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "QRDecodeError";
  }
}

/** 把图片文件解码成 RGBA 像素（优先 createImageBitmap，退化到 <img>）。 */
async function decodeImageData(file: File): Promise<ImageData> {
  const bitmap = typeof createImageBitmap === "function" ? await createImageBitmap(file) : await loadViaImgElement(file);
  try {
    const w = bitmap.width;
    const h = bitmap.height;
    const canvas = document.createElement("canvas");
    canvas.width = w;
    canvas.height = h;
    const ctx = canvas.getContext("2d");
    if (!ctx) throw new QRDecodeError("无法创建画布上下文（浏览器不支持 canvas 2d）");
    ctx.drawImage(bitmap as CanvasImageSource, 0, 0);
    return ctx.getImageData(0, 0, w, h);
  } finally {
    if (typeof ImageBitmap !== "undefined" && bitmap instanceof ImageBitmap) bitmap.close();
  }
}

function loadViaImgElement(file: File): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const url = URL.createObjectURL(file);
    const img = new Image();
    img.onload = () => {
      URL.revokeObjectURL(url);
      resolve(img);
    };
    img.onerror = () => {
      URL.revokeObjectURL(url);
      reject(new QRDecodeError("图片读取失败（可能不是图片文件）"));
    };
    img.src = url;
  });
}

/**
 * 从图片文件里解出二维码文本。
 * @throws QRDecodeError（未识别到二维码 / 图片读取失败 / 环境不支持）
 */
export async function decodeQRFromImageFile(file: File): Promise<string> {
  if (!file) throw new QRDecodeError("没有选择图片");
  if (typeof document === "undefined") throw new QRDecodeError("当前环境不支持图片解码");
  const data = await decodeImageData(file);
  const res = jsQR(data.data, data.width, data.height, { inversionAttempts: "dontInvert" });
  const value = res?.data?.trim();
  if (!value) throw new QRDecodeError("这张图片里没有识别到二维码");
  return value;
}
