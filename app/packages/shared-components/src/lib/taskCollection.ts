/**
 * createTaskCollection — 任务集合核心归约逻辑的单一真源
 *
 * 收敛 taskStore（Tasks 页）与 runTasksStore（GroupDetail 页）重复实现的：
 *   - tasks 容器 + O(1) id→index 索引
 *   - getTaskById / patchTaskById（含 IDENTITY_FIELDS 守卫 + triggerRef）
 *   - appendTask（prepend + 去重 patch + 重建索引）
 *   - applyEvent（WS 4 件套 reducer：created/update/progress/completed，
 *     completed 统一经 normalizeTaskEvent 归一化）
 *
 * 两 store 的差异（taskStore 的 MAX_LOADED 守卫 + 持久化 + hasAnyTask；
 * runTasksStore 的 runId 守卫 + totalCount++）通过 hooks 参数化，
 * 不写进共享核心，避免错误抽象（呼应 K22/K43 纪律）。
 */

import { shallowRef, triggerRef, type ShallowRef } from "vue";
import type { EncvTask } from "@encv/shared-components/types/task";
import { normalizeTaskEvent, type TaskEventType } from "./taskEvent";

/** 归约时绝不覆盖的「实体标识字段」：空/null/"" 视为「未提供」，保留原值 */
const IDENTITY_FIELDS: ReadonlySet<keyof EncvTask> = new Set(["runId", "triggeredBy", "pluginName", "type"]);

export interface TaskCollection {
  /** 任务数组（shallowRef，归约通过 triggerRef 触发响应式） */
  tasks: ShallowRef<EncvTask[]>;
  getTaskById(id: string): EncvTask | undefined;
  patchTaskById(id: string, partial: Partial<EncvTask>): boolean;
  /** 重建 O(1) id→index 索引（在批量替换 tasks.value 后必须调用） */
  rebuildIndex(): void;
  /** 插入任务：已存在则按 patch 合并，否则 prepend + 重建索引（不含 view-state / 持久化） */
  appendTask(task: EncvTask): void;
  /** WS 事件归约入口（created/update/progress/completed） */
  applyEvent(type: TaskEventType, data: any): void;
}

export interface TaskCollectionHooks {
  /** created 事件守卫：返回 false 则丢弃该事件（如 MAX_LOADED 超限 / runId 不匹配） */
  acceptCreated?(task: EncvTask): boolean;
  /** created 事件被接受并插入后触发（持久化 / totalCount++ / view-state） */
  onCreated?(task: EncvTask): void;
  /** update/progress/completed 事件 patch 后触发（持久化等副作用） */
  onPatched?(type: TaskEventType, data: EncvTask): void;
}

export function createTaskCollection(hooks: TaskCollectionHooks = {}): TaskCollection {
  const tasks = shallowRef<EncvTask[]>([]);
  const _taskIndex = new Map<string, number>();

  function rebuildIndex(): void {
    _taskIndex.clear();
    const arr = tasks.value;
    for (let i = 0; i < arr.length; i++) {
      _taskIndex.set(arr[i].id, i);
    }
  }

  function getTaskById(id: string): EncvTask | undefined {
    const idx = _taskIndex.get(id);
    if (idx === undefined) return undefined;
    return tasks.value[idx];
  }

  function patchTaskById(id: string, partial: Partial<EncvTask>): boolean {
    const idx = _taskIndex.get(id);
    if (idx === undefined) return false;
    const prev = tasks.value[idx];
    const merged: EncvTask = { ...prev };
    for (const k of Object.keys(partial) as (keyof EncvTask)[]) {
      const v = partial[k];
      if (v === undefined) continue;
      if (IDENTITY_FIELDS.has(k) && (v === null || v === "")) continue;
      (merged as any)[k] = v;
    }
    tasks.value[idx] = merged;
    triggerRef(tasks);
    return true;
  }

  function appendTask(task: EncvTask): void {
    const existing = _taskIndex.get(task.id);
    if (existing !== undefined) {
      patchTaskById(task.id, task);
      return;
    }
    tasks.value = [task, ...tasks.value];
    rebuildIndex();
  }

  function applyEvent(type: TaskEventType, data: any): void {
    if (!data?.id) return;
    const id = data.id;
    if (type === "created") {
      if (hooks.acceptCreated && hooks.acceptCreated(data as EncvTask) === false) return;
      appendTask(data as EncvTask);
      hooks.onCreated?.(data as EncvTask);
    } else if (type === "update" || type === "progress") {
      patchTaskById(id, data);
      hooks.onPatched?.(type, data as EncvTask);
    } else if (type === "completed") {
      patchTaskById(id, normalizeTaskEvent("completed", data));
      hooks.onPatched?.("completed", data as EncvTask);
    }
  }

  return {
    tasks,
    getTaskById,
    patchTaskById,
    rebuildIndex,
    appendTask,
    applyEvent,
  };
}
