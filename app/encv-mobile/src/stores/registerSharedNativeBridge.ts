// registerSharedNativeBridge.ts - 在应用启动时把 shared 通用模块所需的原生桥接能力
// 注入共享抽象层（@encv/shared-components/runtime/nativeBridge）。
// 必须早于任何使用这些能力的运行时调用（任务取消双写 / server 状态探测 /
// OpenList 桥接 / 依赖清单）。
import {
  addOpenListStatusListener,
  enqueueCancelWorker,
  getAndroidDeps,
  getBackendStatus,
  getOpenListRuntime,
  isNative,
  restartBackend,
  stopBackend,
} from "@/plugins/GoProcess";
import { setNativeBridge } from "@encv/shared-components/runtime/nativeBridge";

export function registerSharedNativeBridge(): void {
  setNativeBridge({
    isNative,
    enqueueCancelWorker,
    restartBackend,
    stopBackend,
    getBackendStatus,
    getAndroidDeps,
    addOpenListStatusListener,
    getOpenListRuntime,
  });
}
