import { toastController } from "@ionic/vue";
import { closeOutline } from "ionicons/icons";

interface ToastOptions {
  message: string;
  duration?: number;
  color?: string;
}

export async function showToast(options: ToastOptions) {
  const { message, duration = 2400, color = "primary" } = options;

  // 关键修复：toast button 的 icon 属性期望 SVG 数据字符串，不是 icon name。
  // 之前传 'close-outline' 字符串导致 ion-icon 内部 loadIcon('close-outline')
  // → new URL('close-outline') → TypeError: Failed to construct 'URL' 崩溃
  // 修复：导入 ionicons 的 closeOutline 常量（实际是 SVG 字符串）
  const toast = await toastController.create({
    message,
    duration,
    position: "top",
    cssClass: `encv-toast encv-toast--${color}`,
    buttons: [
      {
        icon: closeOutline,
        side: "end",
        role: "cancel",
      },
    ],
  });

  await toast.present();
}
