/**
 * useRunTasksStore — GroupDetail 页专用 task store（按 runId 独立加载）（共享抽象层）
 *
 * 2026-06-23 spec backend-sql-authority-view-pagination Task 5.2：
 *   - GroupDetail 页独立路由加载 `GET /api/tasks?runId=xxx`
 *   - WS 不守卫（当前 runId 的 task 全量 push）
 *   - 离开 GroupDetail 时清空该 runId 的 task（释放内存）
 *
 * 与 useTaskStore 的区别：
 *   - useTaskStore（Tasks 页）：视图分页，WS 守卫保留（store 满后不 push）
 *   - useRunTasksStore（GroupDetail 页）：按 runId 独立加载，WS 不守卫
 *
 * 依赖注入：getTasks 来自应用层 api，经 ./taskServices 注入。
 */

import { type ComputedRef, computed, type Ref, ref } from "vue";
import type { EncvTask } from "@/types/task";
import { getTaskServices } from "./taskServices";

const PAGE_SIZE = 100;

export interface UseRunTasksStore {
  currentRunId: Ref<string>;
  tasks: Ref<EncvTask[]>;
  isLoading: Ref<boolean>;
  hasMore: Ref<boolean>;
  isLoadingMore: Ref<boolean>;
  totalCount: ComputedRef<number>;
  loadRun(runId: string): Promise<void>;
  loadMore(): Promise<void>;
  clear(): void;
  applyEvent(type: "created" | "update" | "progress" | "completed", data: any): void;
  getTaskById(id: string): EncvTask | undefined;
  patchTaskById(id: string, patch: Partial<EncvTask>): boolean;
}

export function useRunTasksStore(): UseRunTasksStore {
  // 应用层服务（启动时已注入）
  const { getTasks } = getTaskServices();

  const currentRunId = ref<string>("");
  const tasks = ref<EncvTask[]>([]);
  const isLoading = ref(false);
  const hasMore = ref(false);
  const isLoadingMore = ref(false);
  const _totalCount = ref(0);
  const _paginationOffset = ref(0);

  const totalCount = computed(() => _totalCount.value);

  async function loadRun(runId: string): Promise<void> {
    if (!runId) return;
    if (currentRunId.value === runId && tasks.value.length > 0) return;

    currentRunId.value = runId;
    isLoading.value = true;
    try {
      const list = await getTasks({ runId, offset: 0, limit: PAGE_SIZE });
      tasks.value = list;
      _paginationOffset.value = 0;
      hasMore.value = list.length >= PAGE_SIZE;
      _totalCount.value = list.length;
    } catch (e) {
      console.warn("[useRunTasksStore.loadRun] failed:", runId, e);
      tasks.value = [];
      hasMore.value = false;
      _totalCount.value = 0;
    } finally {
      isLoading.value = false;
    }
  }

  async function loadMore(): Promise<void> {
    if (isLoadingMore.value || !hasMore.value || !currentRunId.value) return;
    isLoadingMore.value = true;
    try {
      const nextOffset = _paginationOffset.value + PAGE_SIZE;
      const list = await getTasks({ runId: currentRunId.value, offset: nextOffset, limit: PAGE_SIZE });
      tasks.value = [...tasks.value, ...list];
      _paginationOffset.value = nextOffset;
      hasMore.value = list.length >= PAGE_SIZE;
      if (hasMore.value) {
        _totalCount.value = tasks.value.length + list.length;
      } else {
        _totalCount.value = tasks.value.length;
      }
    } catch (e) {
      console.warn("[useRunTasksStore.loadMore] failed:", e);
    } finally {
      isLoadingMore.value = false;
    }
  }

  function clear(): void {
    currentRunId.value = "";
    tasks.value = [];
    hasMore.value = false;
    isLoadingMore.value = false;
    _totalCount.value = 0;
    _paginationOffset.value = 0;
  }

  function applyEvent(type: "created" | "update" | "progress" | "completed", data: any): void {
    if (!data?.id) return;
    if (type === "created") {
      const taskRunId = (data as EncvTask).runId;
      if (taskRunId && taskRunId !== currentRunId.value) return;
      if (tasks.value.some(t => t.id === data.id)) return;
      tasks.value = [data as EncvTask, ...tasks.value];
      _totalCount.value++;
      return;
    }
    patchTaskById(data.id, data);
  }

  function getTaskById(id: string): EncvTask | undefined {
    return tasks.value.find(t => t.id === id);
  }

  function patchTaskById(id: string, patch: Partial<EncvTask>): boolean {
    const idx = tasks.value.findIndex(t => t.id === id);
    if (idx < 0) return false;
    tasks.value = tasks.value.map((t, i) => (i === idx ? { ...t, ...patch } : t));
    return true;
  }

  return {
    currentRunId,
    tasks,
    isLoading,
    hasMore,
    isLoadingMore,
    totalCount,
    loadRun,
    loadMore,
    clear,
    applyEvent,
    getTaskById,
    patchTaskById,
  };
}

/** 模块级单例（GroupDetail 页跨组件共享） */
let _cachedInstance: UseRunTasksStore | null = null;

export function useRunTasksStoreSingleton(): UseRunTasksStore {
  if (_cachedInstance) return _cachedInstance;
  _cachedInstance = useRunTasksStore();
  return _cachedInstance;
}

/** 测试用：重置单例 */
export function __resetRunTasksStoreForTests(): void {
  _cachedInstance = null;
}
