/**
 * useRunTasksStore — GroupDetail 页专用 task store（按 runId 独立加载）
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
 * 设计：
 *   - tasks: ref<EncvTask[]> — 当前 runId 的 task 列表
 *   - currentRunId: ref<string> — 当前加载的 runId
 *   - loadRun(runId): 调 GET /api/tasks?runId=xxx 加载
 *   - loadMore(): 分页加载下一页
 *   - applyEvent(type, data): WS 事件处理（只处理 currentRunId 的 task）
 *   - clear(): 清空 store（离开 GroupDetail 时调用）
 */

import { type EncvTask, getTasks } from "@/api/encv";
import { type ComputedRef, computed, type Ref, ref } from "vue";

const PAGE_SIZE = 100;

export interface UseRunTasksStore {
  /** 当前 runId */
  currentRunId: Ref<string>;
  /** 当前 runId 的 task 列表 */
  tasks: Ref<EncvTask[]>;
  /** 是否正在加载 */
  isLoading: Ref<boolean>;
  /** 是否还有更多（分页） */
  hasMore: Ref<boolean>;
  /** 是否正在加载下一页 */
  isLoadingMore: Ref<boolean>;
  /** 当前 runId 的 task 总数（从后端 X-Total-Count 或分页推断） */
  totalCount: ComputedRef<number>;
  /** 加载指定 runId 的 task（进入 GroupDetail 时调用） */
  loadRun(runId: string): Promise<void>;
  /** 加载下一页（ion-infinite-scroll 触发） */
  loadMore(): Promise<void>;
  /** 清空 store（离开 GroupDetail 时调用） */
  clear(): void;
  /** WS 事件处理（只处理 currentRunId 的 task） */
  applyEvent(type: "created" | "update" | "progress" | "completed", data: any): void;
  /** 按 taskId 查找 task */
  getTaskById(id: string): EncvTask | undefined;
  /** 按 taskId patch task（WS update/progress/completed 用） */
  patchTaskById(id: string, patch: Partial<EncvTask>): boolean;
}

export function useRunTasksStore(): UseRunTasksStore {
  const currentRunId = ref<string>("");
  const tasks = ref<EncvTask[]>([]);
  const isLoading = ref(false);
  const hasMore = ref(false);
  const isLoadingMore = ref(false);
  const _totalCount = ref(0);
  const _paginationOffset = ref(0);

  const totalCount = computed(() => _totalCount.value);

  /** 加载指定 runId 的 task（进入 GroupDetail 时调用） */
  async function loadRun(runId: string): Promise<void> {
    if (!runId) return;
    // 如果已经在加载这个 runId，跳过
    if (currentRunId.value === runId && tasks.value.length > 0) return;

    currentRunId.value = runId;
    isLoading.value = true;
    try {
      const list = await getTasks({ runId, offset: 0, limit: PAGE_SIZE });
      tasks.value = list;
      _paginationOffset.value = 0;
      hasMore.value = list.length >= PAGE_SIZE;
      // totalCount 暂用 list.length（后端 X-Total-Count 后续接入）
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

  /** 加载下一页（ion-infinite-scroll 触发） */
  async function loadMore(): Promise<void> {
    if (isLoadingMore.value || !hasMore.value || !currentRunId.value) return;
    isLoadingMore.value = true;
    try {
      const nextOffset = _paginationOffset.value + PAGE_SIZE;
      const list = await getTasks({ runId: currentRunId.value, offset: nextOffset, limit: PAGE_SIZE });
      tasks.value = [...tasks.value, ...list];
      _paginationOffset.value = nextOffset;
      hasMore.value = list.length >= PAGE_SIZE;
      // 更新 totalCount（如果还有更多，说明总数 > 当前已加载数）
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

  /** 清空 store（离开 GroupDetail 时调用） */
  function clear(): void {
    currentRunId.value = "";
    tasks.value = [];
    hasMore.value = false;
    isLoadingMore.value = false;
    _totalCount.value = 0;
    _paginationOffset.value = 0;
  }

  /** WS 事件处理（只处理 currentRunId 的 task） */
  function applyEvent(type: "created" | "update" | "progress" | "completed", data: any): void {
    if (!data?.id) return;
    // 只处理当前 runId 的 task
    //   - created: 如果 data.runId === currentRunId，push 到 tasks
    //   - update/progress/completed: patch 已加载的 task
    if (type === "created") {
      const taskRunId = (data as EncvTask).runId;
      if (taskRunId && taskRunId !== currentRunId.value) return;
      // 避免重复 push
      if (tasks.value.some(t => t.id === data.id)) return;
      tasks.value = [data as EncvTask, ...tasks.value];
      _totalCount.value++;
      return;
    }

    // update/progress/completed: patch 已加载的 task
    patchTaskById(data.id, data);
  }

  /** 按 taskId 查找 task */
  function getTaskById(id: string): EncvTask | undefined {
    return tasks.value.find(t => t.id === id);
  }

  /** 按 taskId patch task（WS update/progress/completed 用） */
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
