/**
 * Realtime Transport Backend 统一接口（2026-06-10 重构）
 *
 * 历史：
 *   - 之前 WS / polling / SSE 各自实现，散落在 useWebSocket / useServerStatus / useFrontendLogs
 *   - 沙箱 OpenPreview 浏览器 trae 反代 :16000 不支持 WebSocket upgrade
 *   - 每个用到 WS 的地方都自己写 isOpenPreviewBrowser 判断 → 维护地狱
 *
 * 方案：
 *   - useRealtimeTransport 单例负责选举 transport 模式（ws / http-poll / native-bridge）
 *   - 每个 backend 实现统一 Backend 接口
 *   - backend 通过 EventEmitter 把事件 emit 到 eventBus
 *   - 消费方继续 eventBus.on，不感知 transport 变化
 *
 * 设计原则：
 *   - 极简接口：start / stop / (optional) reset
 *   - EventEmitter 是纯函数 callback，不依赖 eventBus 单例（方便测试）
 *   - 状态管理交给 useRealtimeTransport（连接状态、模式选择）
 */

/** emit(type, data) — backend 把解析后的事件通过此 callback 通知 transport */
export type EventEmitter = (type: string, data: any) => void;

/**
 * 所有 transport backend 必须实现 start / stop
 * - start: 启动传输（WS 连接 / HTTP 轮询 / native bridge 注册）
 * - stop: 停止传输（保留 backend 实例，可被 start 复用）
 * - reset（可选）: 完全清空 backend 内部状态
 */
export interface Backend {
  start(): void;
  stop(): void;
  reset?(): void;
}

/** 状态机（被 useRealtimeTransport 持有） */
export type ConnectionState = "connecting" | "connected" | "disconnected";
