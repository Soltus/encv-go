/**
 * 注册共享层「ApiProxy 原生插件 + 平台检测」：
 * 注入 @capacitor/core 平台检测与 @/plugins/ApiProxy 插件方法。
 */
import { Capacitor } from "@capacitor/core";
import { ApiProxy } from "@/plugins/ApiProxy";
import { setApiProxy } from "@encv/shared-components/runtime/apiProxy";

export function registerSharedApiProxy(): void {
  setApiProxy({
    isAndroid: () => Capacitor.isNativePlatform() && Capacitor.getPlatform() === "android",
    fetchOnce: options => ApiProxy.fetchOnce(options),
    streamStart: options => ApiProxy.streamStart(options),
    streamCancel: options => ApiProxy.streamCancel(options),
    addListener: (eventName, listener) => ApiProxy.addListener(eventName, listener as never),
    removeAllListeners: eventName => ApiProxy.removeAllListeners(eventName),
  });
}
