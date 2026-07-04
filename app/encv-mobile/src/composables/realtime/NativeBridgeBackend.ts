/**
 * NativeBridgeBackend — Capacitor APK native bridge transport（占位）
 *
 * 适用场景：
 *   - Capacitor 打包成 APK（设备本地 backend，native plugin 桥）
 *   - 实际实现依赖 @capacitor/core 的桥接 + 后端 native module
 *
 * 现状（2026-06-10）：
 *   - 暂未实现，先 throw + TODO
 *   - APK 模式下 WS 通常也能工作（直连 127.0.0.1:2025），所以暂不需要 native bridge
 *   - 留位置给未来 SSE / 设备本地 socket 实现
 */

import type { Backend, EventEmitter } from "./Backend";

export function createNativeBridgeBackend(_emit: EventEmitter): Backend {
  // TODO: 实现 APK native bridge transport
  // - 注册 @capacitor/core bridge listener
  // - 解析 native module 推送的消息 → emit(event, data)
  // - 必要时回写到 native side（player control 等）
  console.warn("[NativeBridgeBackend] not yet implemented; falling back to noop");

  let running = false;

  return {
    start() {
      if (running) return;
      running = true;
      // 占位：先 emit online:true 让 UI 不卡
      _emit("server:status", { online: true });
      console.warn("[NativeBridgeBackend] start() called but backend not implemented");
    },
    stop() {
      running = false;
    },
  };
}
