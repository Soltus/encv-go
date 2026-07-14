/**
 * useContextUsage - 周期拉取 /api/agent/context-usage
 *
 * 设计原则：
 * 1. 由 useAgent 在 status 变化时启停不同频率的轮询（streaming 5s / idle 30s）
 * 2. 错误静默：拉取失败仅 console.debug，不影响 UI
 * 3. 拉取结果存 reactive 状态，供 ContextIcon / ContextPopover 读取
 * 4. 卸载时清 timer，避免内存泄漏
 *
 * SPEC: /workspace/.trae/specs/go-in-process-agent/spec.md
 *   - §Requirement: Context 图标
 */
import { onUnmounted, type Ref, ref, watch } from "vue";
import { getAgentApiBase } from "@encv/shared-components/composables/useAgentApiBase";

const AGENT_API_BASE = getAgentApiBase();

// ─── 类型契约 ───────────────────────────────────────────────

export interface ContextUsage {
  tokens: number;
  window: number;
  percent: number;
}

export interface ContextTodo {
  content: string;
  status: "pending" | "in_progress" | "completed";
}

export interface ContextReferencedFile {
  path: string;
  mountId: string;
  viaTool: string;
  lastRefAt: number;
}

export interface ContextUsageResponse {
  sessionId: string;
  model: string;
  usage: ContextUsage;
  todos: ContextTodo[];
  referencedFiles: ContextReferencedFile[];
  compactions: number;
  updatedAt: number;
}

export type AgentStatusLite = "idle" | "streaming" | "confirming" | "error";

// ─── Composable ─────────────────────────────────────────────

export interface UseContextUsageOptions {
  /** 当前 session id（默认 'default'） */
  sessionId: Ref<string>;
  /** useAgent 的 status ref（决定轮询频率） */
  status: Ref<AgentStatusLite>;
}

/**
 * 创建一个 context-usage 拉取器，返回 reactive 状态 + 控制函数
 */
export function useContextUsage(opts: UseContextUsageOptions) {
  const data = ref<ContextUsageResponse | null>(null);
  const loading = ref(false);
  const lastFetchedAt = ref(0);
  let timer: ReturnType<typeof setTimeout> | null = null;

  const STREAMING_INTERVAL = 5_000;
  const IDLE_INTERVAL = 30_000;

  function getInterval(): number {
    return opts.status.value === "streaming" || opts.status.value === "confirming" ? STREAMING_INTERVAL : IDLE_INTERVAL;
  }

  async function fetchOnce() {
    const sessionId = opts.sessionId.value || "default";
    loading.value = true;
    try {
      const url = `${AGENT_API_BASE}/api/agent/context-usage?sessionId=${encodeURIComponent(sessionId)}`;
      const resp = await fetch(url, { method: "GET" });
      if (!resp.ok) {
        console.debug("[useContextUsage] fetch failed:", resp.status);
        return;
      }
      const body = (await resp.json()) as ContextUsageResponse;
      data.value = body;
      lastFetchedAt.value = Date.now();
    } catch (e) {
      console.debug("[useContextUsage] fetch error:", e);
    } finally {
      loading.value = false;
    }
  }

  function schedule() {
    if (timer) clearTimeout(timer);
    const ms = getInterval();
    timer = setTimeout(async () => {
      await fetchOnce();
      schedule();
    }, ms);
  }

  function start() {
    if (timer) return;
    // 立即拉一次
    void fetchOnce();
    schedule();
  }

  function stop() {
    if (timer) {
      clearTimeout(timer);
      timer = null;
    }
  }

  // status 变化时重排（切换频率）
  const stopWatch = watch(
    () => opts.status.value,
    () => {
      if (timer) {
        clearTimeout(timer);
        schedule();
      }
    }
  );

  // sessionId 变化时立即拉一次（仅在轮询已启动后）
  // - useAgent() 内部 ref 初始化时也会触发 watch，但不应当偷偷发请求
  // - AgentChat 视图会在 onMounted 中调用 start()，由 start() 触发首次拉取
  const watchSession = watch(
    () => opts.sessionId.value,
    () => {
      if (timer) {
        void fetchOnce();
      }
    }
  );

  onUnmounted(() => {
    stop();
    stopWatch();
    watchSession();
  });

  return {
    data,
    loading,
    lastFetchedAt,
    start,
    stop,
    refresh: fetchOnce,
  };
}
