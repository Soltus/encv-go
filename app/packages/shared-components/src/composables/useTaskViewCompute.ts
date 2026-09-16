/**
 * useTaskViewCompute — Web Worker 委托视图计算（主线程封装，shared 版）
 *
 * 2026-07-12 Phase 3 提升（从 `app/encv-mobile/src/composables/useTaskViewCompute.ts`）：
 *   - 纯计算核心已抽到 `@/lib/taskViewComputeCore`（无 Worker / 无 DOM），
 *     本模块 `computeSync` 只做 i18n date label 映射，不再内联 120 行重复逻辑。
 *   - **shared 不实例化 Worker**：`?worker` import 与 `new Worker()` 留在应用层。
 *     通过 DI 参数 `options.workerFactory` 注入；不提供则纯同步降级。
 *   - 沿用 Phase 2 的 DI 模式（如 taskServices），使 shared 保持可测试、无环境耦合。
 *
 * 设计（与 mobile 原版一致）：
 *   - watch tasks + filter/sort/view 状态变化
 *   - debounce 16ms（1 帧）→ postMessage 给 worker
 *   - 接收 worker 结果 → 更新 displayedItems ref（触发虚拟滚动重渲染）
 *   - date section label：worker 返回 dateKey，主线程映射为 i18n label
 *
 * 降级策略：
 *   - workerFactory 未提供 / Worker 构造失败 / 测试环境 → 主线程同步计算
 *   - Worker 计算出错 → 保留旧值 + warn 日志
 */
import { type ComputedRef, computed, type Ref, ref, toRaw, watch } from "vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import type { ComputeInput, ComputeOutput } from "@encv/shared-components/lib/taskViewComputeCore";
import { buildDisplayedItems } from "@encv/shared-components/lib/taskViewComputeCore";
import type { EncvTask } from "@encv/shared-components/types/task";

export interface UseTaskViewComputeOptions {
  /** 任务列表（响应式） */
  tasks: Ref<EncvTask[]> | ComputedRef<EncvTask[]>;
  /** 视图模式 */
  viewMode: Ref<"group" | "flat"> | ComputedRef<"group" | "flat">;
  /** 排序模式 */
  sortBy: Ref<"activity" | "created"> | ComputedRef<"activity" | "created">;
  /** 搜索关键词 */
  searchQuery: Ref<string> | ComputedRef<string>;
  /** 插件筛选 */
  filterPlugins: Ref<string[]> | ComputedRef<string[]>;
  /** 类型筛选 */
  filterTypes: Ref<string[]> | ComputedRef<string[]>;
  /** 状态筛选 */
  filterStatuses: Ref<string[]> | ComputedRef<string[]>;
  /** 触发者筛选 */
  filterTriggeredBy: Ref<string[]> | ComputedRef<string[]>;
  /** 日期范围筛选 */
  filterDateRange: Ref<{ from?: string; to?: string }> | ComputedRef<{ from?: string; to?: string }>;
  /** 置顶 runId 集合 */
  pinnedRunIds: Ref<Set<string>> | ComputedRef<Set<string>>;
  /**
   * Worker 工厂（应用层注入；shared 自身不实例化 Worker）。
   * 不提供则走纯同步降级（SSR / 测试 / 未注入场景）。
   */
  workerFactory?: () => Worker | null;
}

export interface UseTaskViewCompute {
  /** 计算后的 displayedItems（date section label 已映射为 i18n） */
  displayedItems: Ref<any[]>;
  /** 是否正在计算（worker 异步） */
  isComputing: Ref<boolean>;
  /** 是否降级为主线程同步计算 */
  isFallback: Ref<boolean>;
}

/**
 * 主线程同步计算（降级用）
 *   - 复用 shared 纯计算核心 buildDisplayedItems，仅在此做 i18n date label 映射
 *   - 与 worker 内 buildDisplayedItems 逻辑一致（同一份核心）
 */
function computeSync(input: Omit<ComputeInput, "type" | "requestId">, t: (key: string, opts?: any) => string): any[] {
  const items = buildDisplayedItems(input as ComputeInput);
  return items.map((item: any) => {
    if (item.kind === "date" && item.dateKey) {
      return { ...item, label: t(`tasks.date.${item.dateKey}`, { defaultValue: item.dateKey }) };
    }
    return item;
  });
}

export function useTaskViewCompute(options: UseTaskViewComputeOptions): UseTaskViewCompute {
  const { t } = useI18n();
  const isComputing = ref(false);
  const isFallback = ref(false);

  // 🆕 检测测试环境（vitest 设置 import.meta.env.VITEST = true）
  //   - jsdom 不支持真正的 Web Worker（new Worker() 可能不抛错但永不响应）
  //   - 测试环境直接用同步计算，避免异步等待 worker 响应
  const isTestEnv = !!(import.meta as any).env?.VITEST;

  // 尝试通过注入的 workerFactory 初始化 Worker（shared 自身不实例化 Worker）
  let worker: Worker | null = null;
  let requestId = 0;
  let pendingTimer: ReturnType<typeof setTimeout> | null = null;
  let lastInput: Omit<ComputeInput, "type" | "requestId"> | null = null;

  const workerFactory = options.workerFactory;
  if (!isTestEnv) {
    if (workerFactory) {
      try {
        worker = workerFactory();
      } catch (e) {
        console.warn("[useTaskViewCompute] Worker init failed, fallback to sync:", e);
        worker = null;
        isFallback.value = true;
      }
    } else {
      // 无 workerFactory（如 SSR / 未注入）→ 纯同步降级
      isFallback.value = true;
    }
  } else {
    isFallback.value = true;
  }

  // ============ 降级路径：同步 computed（测试环境 + Worker 不可用 + Worker 未就绪兜底） ============
  //   - 用 computed 保持同步语义（与原 displayedItems computed 行为一致）
  //   - 🆕 2026-06-23 修复真机空白：worker 异步计算期间用 sync 兜底
  //     - worker 首次计算前 workerDisplayedItems=[] → 页面空白
  //     - worker onerror 后 isFallback=true 但旧逻辑不切回 sync
  //     - 修法：displayedItems 用 computed，worker 未就绪/fallback 时用 sync
  const syncDisplayedItems = computed<any[]>(() => {
    const input = {
      tasks: options.tasks.value,
      viewMode: options.viewMode.value,
      sortBy: options.sortBy.value,
      searchQuery: options.searchQuery.value,
      filterPlugins: options.filterPlugins.value,
      filterTypes: options.filterTypes.value,
      filterStatuses: options.filterStatuses.value,
      filterTriggeredBy: options.filterTriggeredBy.value,
      filterDateRange: options.filterDateRange.value,
      pinnedRunIds: Array.from(options.pinnedRunIds.value),
    };
    return computeSync(input, t);
  });

  // ============ Worker 路径：异步 ref（debounce 16ms） ============
  const workerDisplayedItems = ref<any[]>([]);
  // 🆕 worker 是否已返回过有效结果（首次计算前为 false → 用 sync 兜底）
  //   - 必须是 ref：displayedItems computed 依赖此值，worker 返回结果后切到 workerDisplayedItems
  const workerHasResult = ref(false);

  if (worker) {
    worker.onmessage = (e: MessageEvent<ComputeOutput>) => {
      const output = e.data;
      if (output?.type !== "result") return;
      isComputing.value = false;
      workerHasResult.value = true;
      // date section: dateKey → i18n label
      const items = output.items.map((item: any) => {
        if (item.kind === "date" && item.dateKey) {
          return { ...item, label: t(`tasks.date.${item.dateKey}`, { defaultValue: item.dateKey }) };
        }
        return item;
      });
      workerDisplayedItems.value = items;
    };
    worker.onerror = e => {
      console.warn("[useTaskViewCompute] Worker error, fallback to sync:", e);
      isFallback.value = true;
      workerHasResult.value = false;
    };
  }

  /**
   * 触发 Worker 计算（debounce 16ms = 1 帧）
   *   - 连续 filter 变化时只发最后一次（避免 worker 队列积压）
   *   - 16ms 对应 60fps，用户感知不到延迟
   */
  function scheduleWorkerCompute(): void {
    if (!worker) return;
    // 收集当前输入
    // ⚠️ 关键：tasks 用 toRaw 剥掉 Pinia reactive Proxy
    //   - worker.postMessage 走 structured clone 算法，不能 clone Proxy
    //   - 否则抛 DataCloneError: Failed to execute 'postMessage' on 'Worker'
    //   - 生产环境会导致页面空白（worker 永远不返回结果）
    lastInput = {
      tasks: toRaw(options.tasks.value),
      viewMode: options.viewMode.value,
      sortBy: options.sortBy.value,
      searchQuery: options.searchQuery.value,
      filterPlugins: toRaw(options.filterPlugins.value),
      filterTypes: toRaw(options.filterTypes.value),
      filterStatuses: toRaw(options.filterStatuses.value),
      filterTriggeredBy: toRaw(options.filterTriggeredBy.value),
      filterDateRange: toRaw(options.filterDateRange.value),
      pinnedRunIds: Array.from(options.pinnedRunIds.value),
    };

    // debounce
    if (pendingTimer) clearTimeout(pendingTimer);
    isComputing.value = true;
    pendingTimer = setTimeout(() => {
      if (!worker || !lastInput) return;
      requestId++;
      const input: ComputeInput = {
        type: "compute",
        ...lastInput,
        requestId,
      };
      worker.postMessage(input);
    }, 16);
  }

  // Worker 模式：watch 所有依赖 → debounce → postMessage
  //
  // 🆕 2026-07-02 性能修复：拆分 watch，避免 searchQuery 变化时深度遍历 tasks 数组
  //   - 原问题：watch([tasks, searchQuery, ...], cb, { deep: true }) 会在 searchQuery
  //     变化时对 tasks（可能几千个任务）做深度遍历，阻塞 UI 主线程
  //   - 修复：tasks 单独 deep watch（任务对象会被局部 patch，需 deep 检测）
  //           其他 filter/search 状态单独 watch（数组/字符串，deep 遍历成本低）
  //   - 效果：搜索输入时只触发 filter watch（浅遍历），不遍历 tasks 数组
  if (worker) {
    // tasks 变化：deep watch（检测任务对象的局部 patch，如 status/progress 变化）
    watch(options.tasks, () => scheduleWorkerCompute(), { deep: true, immediate: true });
    // filter/search/view 状态变化：这些是短数组或字符串，deep 遍历成本低
    watch(
      [
        options.viewMode,
        options.sortBy,
        options.searchQuery,
        options.filterPlugins,
        options.filterTypes,
        options.filterStatuses,
        options.filterTriggeredBy,
        options.filterDateRange,
        options.pinnedRunIds,
      ],
      () => scheduleWorkerCompute(),
      { deep: true }
    );
  }

  // 🆕 2026-06-23 修复真机空白：统一出口改为 computed
  //   - isFallback=true（测试/worker 不可用/worker onerror）→ syncDisplayedItems
  //   - isFallback=false 但 worker 尚未返回结果（workerHasResult=false）→ syncDisplayedItems 兜底
  //   - isFallback=false 且 worker 已返回结果 → workerDisplayedItems
  //   - 这样 worker 异步计算期间不会空白（sync 立即给出结果），worker 返回后切换
  const displayedItems = computed<any[]>(() => {
    if (isFallback.value || !workerHasResult.value) {
      return syncDisplayedItems.value;
    }
    return workerDisplayedItems.value;
  });

  return {
    displayedItems,
    isComputing,
    isFallback,
  };
}
