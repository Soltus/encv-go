/**
 * useRunSummaries — 管理 run summary 数据（后端 SQL 权威）
 *
 * 2026-06-23 spec backend-sql-authority-view-pagination Task 4.2：
 *   - 后端是唯一权威，聚合计数由 SQL COUNT + GROUP BY status 出
 *   - 前端 group card 显示 summary.total/passed/failed（不靠 store.tasks 算）
 *   - store 只持有"当前视图需要的"task（视图分页），不是所有 task
 *   - WS task:completed 时刷新对应 runId 的 summary（调 GET /api/runs/:runId/summary）
 *
 * 设计：
 *   - summaries: ref<Map<runId, RunSummary>> — 内存缓存，按需刷新
 *   - fetchAll(): 一次拉取所有 run 的 summary（GET /api/runs，带 summary）
 *   - fetchOne(runId): 拉取单个 run 的 summary（GET /api/runs/:runId/summary）
 *   - refreshOnTaskCompleted(runId): WS task:completed 时刷新对应 runId
 *   - getSummary(runId): 同步获取缓存的 summary（未加载时返回 undefined）
 *
 * 降级策略：
 *   - API 失败时保留旧数据 + warn 日志（不阻塞 UI）
 *   - 首次加载失败时返回空 Map（group card 显示 loading 或 0）
 */
import { type ComputedRef, computed, type Ref, ref } from "vue";
import { getRunSummary, listRuns, type RunInfo, type RunSummary } from "@encv/shared-components/api/encv";

export interface UseRunSummaries {
  /** 所有 run 的 summary 缓存（按 runId 索引） */
  summaries: Ref<Map<string, RunSummary>>;
  /** 所有 run 的基本信息（runId + startedAt + triggeredBy） */
  runs: Ref<RunInfo[]>;
  /** 是否正在加载 */
  isLoading: ComputedRef<boolean>;
  /** 最后一次加载错误 */
  error: Ref<Error | null>;
  /** 拉取所有 run 的 summary（GET /api/runs） */
  fetchAll(): Promise<void>;
  /** 拉取单个 run 的 summary（GET /api/runs/:runId/summary） */
  fetchOne(runId: string): Promise<void>;
  /** WS task:completed 时刷新对应 runId 的 summary */
  refreshOnTaskCompleted(runId: string): Promise<void>;
  /** 同步获取缓存的 summary（未加载时返回 undefined） */
  getSummary(runId: string): RunSummary | undefined;
}

export function useRunSummaries(): UseRunSummaries {
  const summaries = ref<Map<string, RunSummary>>(new Map());
  const runs = ref<RunInfo[]>([]);
  const error = ref<Error | null>(null);
  const _isLoading = ref(false);

  const isLoading = computed(() => _isLoading.value);

  /** 拉取所有 run 的 summary（GET /api/runs，带 summary） */
  async function fetchAll(): Promise<void> {
    _isLoading.value = true;
    try {
      const list = await listRuns();
      runs.value = list;
      const map = new Map<string, RunSummary>();
      for (const r of list) {
        map.set(r.runId, r.summary);
      }
      summaries.value = map;
      error.value = null;
    } catch (e) {
      console.warn("[useRunSummaries.fetchAll] failed:", e);
      error.value = e as Error;
    } finally {
      _isLoading.value = false;
    }
  }

  /** 拉取单个 run 的 summary（GET /api/runs/:runId/summary） */
  async function fetchOne(runId: string): Promise<void> {
    if (!runId) return;
    try {
      const summary = await getRunSummary(runId);
      const map = new Map(summaries.value);
      map.set(runId, summary);
      summaries.value = map;
    } catch (e) {
      console.warn("[useRunSummaries.fetchOne] failed:", runId, e);
      // 保留旧数据，不设置 error（单个失败不阻塞整体）
    }
  }

  /** WS task:completed 时刷新对应 runId 的 summary */
  async function refreshOnTaskCompleted(runId: string): Promise<void> {
    if (!runId) return;
    // debounce 不需要：WS 事件频率不高，直接刷新
    await fetchOne(runId);
  }

  /** 同步获取缓存的 summary（未加载时返回 undefined） */
  function getSummary(runId: string): RunSummary | undefined {
    return summaries.value.get(runId);
  }

  return {
    summaries,
    runs,
    isLoading,
    error,
    fetchAll,
    fetchOne,
    refreshOnTaskCompleted,
    getSummary,
  };
}

/** 模块级单例（跨组件共享 run summary 数据） */
let _cachedInstance: UseRunSummaries | null = null;

export function useRunSummariesSingleton(): UseRunSummaries {
  if (_cachedInstance) return _cachedInstance;
  _cachedInstance = useRunSummaries();
  return _cachedInstance;
}

/** 测试用：重置单例 */
export function __resetRunSummariesForTests(): void {
  _cachedInstance = null;
}
