/**
 * appServerRealtimeReducer
 *
 * 统一处理 SSE 实时事件的 reducer（参考 codex-web 仓库的
 * `apps/web/src/app/appServerRealtimeReducer.ts` 与 `realtimeState.ts` 设计模式）。
 *
 * **注意**：本模块不直接 import codex-web 仓库的代码（TypeScript 类型不通用），
 * 仅参考其设计模式：
 *   - 统一入口 `processRealtimeEvent` 决定事件是否应被消费
 *   - sequence 去重避免重连 / 重复 push
 *   - server instance 切换时清空去重缓存（视为新 server）
 *   - threadId / cacheVersion / serverInstanceId 从多种字段位置读取
 *
 * 本模块是「纯函数 + 状态对象」的纯 JS 模块，没有任何 Vue 依赖，便于单元测试。
 * 集成在后续 task 引入 useAgent 时再绑定到 reactive ref。
 *
 * SPEC: /workspace/.trae/specs/mobile-agent-2026-gap-analysis/spec.md
 *   - Capability 11: Server Instance + Sequence 去重
 * TASKS: /workspace/.trae/specs/mobile-agent-2026-gap-analysis/tasks.md
 *   - Task 3: appServerRealtimeReducer
 */

// =============================================================================
// 公共类型
// =============================================================================

/**
 * 最小化的实时事件结构。
 *
 * 不与后端实际推送格式强绑定——只声明 reducer 真正关心的字段。
 * 集成到 useAgent 时，后端推送的 AgentEvent 会被映射为 MinimalRealtimeEvent。
 */
export interface MinimalRealtimeEvent {
  type?: string;
  sequence?: number;
  payload?: unknown;
  params?: unknown;
  approval?: unknown;
  threadId?: string;
  conversationId?: string;
  cacheVersion?: number;
  atIso?: string;
  serverInstanceId?: string;
}

// =============================================================================
// 内部 helper
// =============================================================================

/**
 * 把任意 unknown 收敛为「普通对象」或 null。
 * - 普通对象（非数组、非 null） → 返回原对象
 * - 其他（数组、原始类型、null、undefined） → 返回 null
 */
export function asRecord(value: unknown): Record<string, unknown> | null {
  if (value == null) return null;
  if (typeof value !== "object") return null;
  if (Array.isArray(value)) return null;
  return value as Record<string, unknown>;
}

/**
 * 防御式读取字符串字段。
 * - 输入非 string → 返回空串
 * - 输入是 string → 原样返回
 */
export function readString(value: unknown): string {
  if (typeof value === "string") return value;
  return "";
}

/**
 * 从事件多种可能位置提取 thread id。
 * 检查顺序：
 *   1) event.threadId
 *   2) event.conversationId（视为兼容别名）
 *   3) event.payload.threadId
 *   4) event.params.threadId
 *   5) event.approval.threadId
 */
export function readRealtimeThreadId(event: MinimalRealtimeEvent): string {
  if (typeof event.threadId === "string" && event.threadId.length > 0) {
    return event.threadId;
  }
  if (typeof event.conversationId === "string" && event.conversationId.length > 0) {
    return event.conversationId;
  }
  for (const container of [event.payload, event.params, event.approval]) {
    const rec = asRecord(container);
    if (rec) {
      const id = rec.threadId;
      if (typeof id === "string" && id.length > 0) return id;
    }
  }
  return "";
}

/**
 * 读取 cache version。
 * 优先 payload.cacheVersion（reasoning/message 事件的主要载体），
 * 然后 params / approval / 顶层。
 * 任何非有限数（NaN、Infinity、string-number）都被规范化为 null。
 */
export function readRealtimeCacheVersion(event: MinimalRealtimeEvent): number | null {
  const candidates: unknown[] = [event.payload, event.params, event.approval];
  // 先扫描 payload-like 容器
  for (const container of candidates) {
    const rec = asRecord(container);
    if (rec) {
      const v = rec.cacheVersion;
      if (typeof v === "number" && Number.isFinite(v)) return v;
      if (typeof v === "string") {
        const n = Number(v);
        if (Number.isFinite(n)) return n;
      }
    }
  }
  // 最后看顶层
  if (typeof event.cacheVersion === "number" && Number.isFinite(event.cacheVersion)) {
    return event.cacheVersion;
  }
  if (typeof event.cacheVersion === "string") {
    const n = Number(event.cacheVersion);
    if (Number.isFinite(n)) return n;
  }
  return null;
}

/**
 * 仅在 type==='connected' 事件时返回 serverInstanceId；
 * 其它类型一律返回空串（避免误把常规事件里的字段当成 instance）。
 */
export function readRealtimeServerInstance(event: MinimalRealtimeEvent): string {
  if (event.type !== "connected") return "";
  if (typeof event.serverInstanceId === "string") return event.serverInstanceId;
  return "";
}

// =============================================================================
// Server instance tracker
// =============================================================================

/**
 * 根据单个事件决定 server instance 状态 + 是否应清空缓存。
 *
 * 算法：
 *   ① 读 readRealtimeServerInstance(event) → candidateInstance
 *   ② 若 candidateInstance 为空 → 维持 currentServerInstance 不变
 *   ③ 若 candidateInstance !== currentServerInstance → 视为新 server 接入：
 *        a) currentServerInstance 切到 candidateInstance
 *        b) versionsByThreadId 整个清空（避免误把旧 server 的 cacheVersion 复用）
 *   ④ 返回新的 currentServerInstance
 */
export function updateRealtimeServerInstance(
  versionsByThreadId: Map<string, number>,
  currentServerInstance: string,
  event: MinimalRealtimeEvent
): string {
  const candidate = readRealtimeServerInstance(event);
  if (candidate === "") {
    return currentServerInstance;
  }
  if (candidate === currentServerInstance) {
    return currentServerInstance;
  }
  // instance 切换：清空去重状态
  versionsByThreadId.clear();
  return candidate;
}

// =============================================================================
// Sequence tracker
// =============================================================================

/**
 * 单个 event 决策的输出：上游应据此决定是否 apply 到 reactive state。
 */
export interface RealtimeThreadEventDecision {
  /** 是否接受并继续处理 */
  accepted: boolean;
  /** 事件所属 thread id（可能为空串） */
  threadId: string;
  /** 事件携带的 cache version（可能为 null） */
  cacheVersion: number | null;
}

/**
 * 维护 sequence 去重的状态。
 *
 * - serverInstance: 当前活跃的 server instance id（用于跨 server 防漂移）
 * - seenSequences:   已见过的 event sequence 集合（容量上限 MAX_TRACKED_REALTIME_SEQUENCES）
 */
export interface RealtimeSequenceTrackerState {
  serverInstance: string;
  seenSequences: Set<number>;
}

/**
 * 超过此容量时，按 sequence 自然顺序（升序）淘汰最早加入的 1/4。
 * 2_000 来自 codex-web `realtimeState.ts:27` 的同名字常量。
 */
export const MAX_TRACKED_REALTIME_SEQUENCES = 2_000;

/**
 * 创建一个新的 sequence tracker state（默认无 server instance、零去重）。
 */
export function createRealtimeSequenceTrackerState(serverInstance: string = ""): RealtimeSequenceTrackerState {
  return {
    serverInstance,
    seenSequences: new Set<number>(),
  };
}

/**
 * 把 sequence 写入 seenSequences，必要时按容量上限淘汰最早的条目。
 *
 * 淘汰策略：因 Set 保持插入顺序，将 entries 转为数组后排序，
 * 删掉比当前 sequence 小的前 N/4 个，再插入当前 sequence。
 * 这样保证：
 *   - 高 sequence 永不被淘汰（避免正在活跃的事件被错杀）
 *   - 老 sequence 自然下沉并被回收
 */
function recordSequence(seen: Set<number>, sequence: number): void {
  if (seen.size < MAX_TRACKED_REALTIME_SEQUENCES) {
    seen.add(sequence);
    return;
  }
  // 已满：取出所有小于当前 sequence 的元素，按升序删除前 1/4
  const overflowCount = Math.floor(MAX_TRACKED_REALTIME_SEQUENCES / 4) || 1;
  const older = Array.from(seen)
    .filter(n => n < sequence)
    .sort((a, b) => a - b);
  const toRemove = older.slice(0, overflowCount);
  for (const n of toRemove) seen.delete(n);
  seen.add(sequence);
}

// =============================================================================
// 主 reducer
// =============================================================================

/**
 * processRealtimeEvent —— 统一 reducer 入口。
 *
 * 决策流程：
 *   1) 若事件携带新的 serverInstanceId 且与当前 state 不同：
 *        - 更新 state.serverInstance
 *        - 清空 state.seenSequences（视为新 server，去重状态不可继承）
 *      注：versionsByThreadId（Map<string, number>）是独立的状态对象，
 *         由调用方通过 `updateRealtimeServerInstance` 单独管理。
 *   2) 读 threadId / cacheVersion 作为 decision 输出
 *   3) 若 event.sequence 不是有限数字 → 视为非去重事件，直接 accepted=true
 *   4) 若 event.sequence 已在 seenSequences → 重复事件，accepted=false
 *   5) 否则 → 记录 sequence + accepted=true
 *
 * 返回 `{ accept, decision }`：
 *   - accept: 布尔值，UI 层可直接用作 `if (!result.accept) return;`
 *   - decision: 包含 threadId + cacheVersion 的详细决策
 */
export function processRealtimeEvent(
  state: RealtimeSequenceTrackerState,
  event: MinimalRealtimeEvent
): { accept: boolean; decision: RealtimeThreadEventDecision } {
  // ① server instance 变化检测
  const candidate = readRealtimeServerInstance(event);
  if (candidate !== "" && candidate !== state.serverInstance) {
    state.serverInstance = candidate;
    state.seenSequences.clear();
  }

  // ② 抽取 threadId / cacheVersion
  const threadId = readRealtimeThreadId(event);
  const cacheVersion = readRealtimeCacheVersion(event);

  // ③ 无 sequence → 非去重事件
  if (typeof event.sequence !== "number" || !Number.isFinite(event.sequence)) {
    return {
      accept: true,
      decision: { accepted: true, threadId, cacheVersion },
    };
  }

  // ④ 已见过 → 重复
  if (state.seenSequences.has(event.sequence)) {
    return {
      accept: false,
      decision: { accepted: false, threadId, cacheVersion },
    };
  }

  // ⑤ 接受 + 记录
  recordSequence(state.seenSequences, event.sequence);
  return {
    accept: true,
    decision: { accepted: true, threadId, cacheVersion },
  };
}
