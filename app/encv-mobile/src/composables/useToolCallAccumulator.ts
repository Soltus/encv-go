/**
 * useToolCallAccumulator.ts — 工具调用流式累积器
 *
 * Stage 2 (borrow-nuclear-boy-2026q2)：ToolCallAccumulator 模式。
 *
 * 借鉴来源：/tmp/nuclear-boy/agent-core/src/main/java/com/nuclearboy/agent/AgentEngine.kt L82-122
 *   private class ToolCallAccumulator {
 *     private val partialCalls = mutableMapOf<Int, ToolCallBuilder>()
 *     fun feed(id, name, args) { ... }
 *     fun toCompletedCalls(): List<ToolCall> { ... }
 *     fun hasPartialCalls(): Boolean { ... }
 *     fun clear() { ... }
 *   }
 *
 * **关键设计决策（来自 nuclear-boy 实战踩坑 L707）**：
 *   "Clear once per API call, NOT per individual tool call"
 *   即 clear() 在 ReAct 循环**开始**时调用，**不**在每个 TOOL_CALL_START 时清——
 *   避免同一轮 2-3 个 tool call 互相覆盖 args。
 *
 * 用法（useAgent.send() 中）：
 *   const acc = createToolCallAccumulator()
 *   // 收到 TOOL_CALL_START → acc.feedStart({id, name})
 *   // 收到 TOOL_CALL_ARGS  → acc.feedArgs({id, argsDelta})
 *   // 收到 TOOL_CALL_END   → acc.complete(id)
 *   // 收到 TOOL_CALL_RESULT→ acc.setResult({id, result})
 *   // ReAct 循环开始        → acc.clear()  ← 关键：只在循环开始清
 *   // 循环结束              → acc.getCompleted()
 *
 * 设计原则：纯函数 + 单 Map（by id），不依赖 Vue/React reactivity 运行时，
 *           方便在单测中直接调用。
 */

export type ToolCallStatus =
  | "pending" // 已收到 TOOL_CALL_START，args 还在累积
  | "accumulating" // 至少收到 1 个 TOOL_CALL_ARGS
  | "complete" // 收到 TOOL_CALL_END
  | "executed"; // 收到 TOOL_CALL_RESULT（已执行完成）

export interface AccumulatedToolCall {
  id: string;
  name: string;
  args: string; // 累积的 args JSON 字符串
  status: ToolCallStatus;
  result?: string; // tool 执行结果（仅 executed 状态有值）
  error?: string; // tool 执行错误（executed + 失败）
}

export interface ToolCallAccumulator {
  /** 收到 TOOL_CALL_START：初始化 entry */
  feedStart(entry: { id: string; name: string }): void;
  /** 收到 TOOL_CALL_ARGS：累加 args */
  feedArgs(entry: { id: string; argsDelta: string }): void;
  /** 收到 TOOL_CALL_END：标记 complete */
  complete(id: string): void;
  /** 收到 TOOL_CALL_RESULT：标记 executed + 记录 result */
  setResult(entry: { id: string; result: string; error?: string }): void;
  /** 获取当前所有 tool call 快照 */
  getAll(): AccumulatedToolCall[];
  /** 获取所有已 complete 的（可执行队列） */
  getCompleted(): AccumulatedToolCall[];
  /** 获取所有已 executed 的（含结果） */
  getExecuted(): AccumulatedToolCall[];
  /** 是否有未完成（pending/accumulating）的 call */
  hasPartial(): boolean;
  /** 清空所有累积状态 */
  clear(): void;
  /** 当前累积数量（调试用） */
  size(): number;
}

/**
 * 创建工具调用累积器实例。
 * 每次 ReAct 循环**开始**时调用 createToolCallAccumulator() 或显式 clear()。
 */
export function createToolCallAccumulator(): ToolCallAccumulator {
  // Map<id, AccumulatedToolCall> —— nuclear-boy 用 index（OpenAI 流式字段），
  // AG-UI 用 id（已自带），我们跟 AG-UI 协议。
  const map = new Map<string, AccumulatedToolCall>();

  return {
    feedStart({ id, name }) {
      if (!id) return;
      // nuclear-boy 实战：如果 id 已存在，**不**覆盖（保留累积的 args）
      // 这是 "Clear once per API call" 的另一半 — feedStart 不能毁掉累积
      if (map.has(id)) {
        // 仅更新 name（理论上 LLM 不会改 name，但保险起见）
        const existing = map.get(id)!;
        existing.name = name || existing.name;
        return;
      }
      map.set(id, {
        id,
        name: name || "",
        args: "",
        status: "pending",
      });
    },

    feedArgs({ id, argsDelta }) {
      if (!id) return;
      const entry = map.get(id);
      if (!entry) {
        // 收到 args 但没收到 start（protocol 异常），宽容处理
        map.set(id, {
          id,
          name: "",
          args: argsDelta || "",
          status: "accumulating",
        });
        return;
      }
      entry.args += argsDelta || "";
      if (entry.status === "pending") {
        entry.status = "accumulating";
      }
    },

    complete(id) {
      if (!id) return;
      const entry = map.get(id);
      if (!entry) return;
      entry.status = "complete";
    },

    setResult({ id, result, error }) {
      if (!id) return;
      const entry = map.get(id);
      if (!entry) return;
      entry.status = "executed";
      entry.result = result;
      entry.error = error;
    },

    getAll() {
      return Array.from(map.values());
    },

    getCompleted() {
      return Array.from(map.values()).filter(e => e.status === "complete" || e.status === "executed");
    },

    getExecuted() {
      return Array.from(map.values()).filter(e => e.status === "executed");
    },

    hasPartial() {
      for (const e of map.values()) {
        if (e.status === "pending" || e.status === "accumulating") {
          return true;
        }
      }
      return false;
    },

    clear() {
      map.clear();
    },

    size() {
      return map.size;
    },
  };
}

/**
 * 解析累积的 args JSON 字符串为对象。
 * 借鉴 nuclear-boy ToolRegistry.kt L776-798 parseToolParams 容错：
 *   JSON 解析失败 → 返回空对象 {}，不抛错。
 */
export function parseAccumulatedArgs(args: string): Record<string, unknown> {
  if (!args || !args.trim()) return {};
  try {
    const parsed = JSON.parse(args);
    return parsed && typeof parsed === "object" && !Array.isArray(parsed) ? parsed : {};
  } catch {
    return {};
  }
}
