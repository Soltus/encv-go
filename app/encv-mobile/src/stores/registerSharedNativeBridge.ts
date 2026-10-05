// registerSharedNativeBridge.ts - 在应用启动时把 shared 通用模块所需的原生桥接能力
// 注入共享抽象层（@encv/shared-components/runtime/nativeBridge）。
// 必须早于任何使用这些能力的运行时调用（任务取消双写 / server 状态探测 /
// OpenList 桥接 / 依赖清单 / 云控重载）。
import {
  addOpenListStatusListener,
  enqueueCancelWorker,
  getAndroidDeps,
  getBackendStatus,
  getOpenListRuntime,
  isNative,
  reloadApp,
  restartBackend,
  stopBackend,
} from "@/plugins/GoProcess";
import { eventBus } from "@encv/shared-components/composables/useEventBus";
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

  // ── vNext Round 10：云控重载的**事件驱动**接线 ────────────────────────────
  //
  // 链路：Go 广播 WS(bundle_reload) → WsBackend emit ws:message → 这里 → 原生。
  //
  // 为什么走事件而不是轮询：轮询耗电、有延迟、还要求 App 在前台 —— 是垃圾方案。
  //
  // 分工：
  //   web 级       —— 前端自己 location.reload()（无感，**不经过原生**）
  //   activity/app —— 必须经原生：**原生弹二次确认**后才执行
  //                   （重建页面会丢状态、重启会中断进行中的操作，
  //                     远端不能替用户做这个决定）
  eventBus.on("ws:message", (payload: unknown) => {
    const msg = payload as { type?: string; data?: { level?: string; reason?: string } } | null;
    if (!msg || msg.type !== "bundle_reload") return;
    const level = String(msg.data?.level ?? "web");
    // web 级由 WsBackend 就地 reload，无需原生介入
    if (level === "web") return;
    void reloadApp(level as "activity" | "app", msg.data?.reason).then(res => {
      console.info(`[reload] level=${level} confirmed=${res.confirmed ?? false} success=${res.success}`);
    });
  });
}
