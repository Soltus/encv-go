/**
 * 工作流存储层 — localStorage CRUD
 *
 * 职责：
 * - WorkflowDefinition 的持久化（localStorage）
 * - 内置模板的注册和加载
 *
 * 2026-06-18 spec unify-workflow-task-service：
 *   - runs 历史持久化已移除（消费者 useWorkflowEngine 已删除）
 *   - runs 相关功能由 useWorkflowTaskService 接管（key = encv_workflow_tasks_v1）
 *   - 本 composable 仅保留 definitions CRUD + 内置模板注册
 *
 * MVP 阶段纯前端存储，预留后端 API 接口签名。
 */

import type { WorkflowDefinition } from "@encv/shared-components/lib/workflow/types";
import { WORKFLOW_STORE_KEY } from "@encv/shared-components/lib/workflow/types";
import { ref } from "vue";

/** 生成简易 UUID（不需要 crypto 库） */
function generateId(): string {
  return "wf-" + Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 8);
}

function loadJSON<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key);
    return raw ? (JSON.parse(raw) as T) : fallback;
  } catch {
    return fallback;
  }
}

function saveJSON<T>(key: string, data: T): void {
  try {
    localStorage.setItem(key, JSON.stringify(data));
  } catch (e) {
    console.warn("[WorkflowStore] Failed to save to localStorage:", e);
  }
}

export function useWorkflowStore() {
  const definitions = ref<WorkflowDefinition[]>(loadJSON(WORKFLOW_STORE_KEY, []));

  // ==================== Definition CRUD ====================

  function createDefinition(partial: Omit<WorkflowDefinition, "id" | "createdAt" | "updatedAt"> & { id?: string }): WorkflowDefinition {
    const now = new Date().toISOString();
    // 尊重 partial.id（PluginTestsDetail.vue 的 dynamic-auto-test 硬编码 id 场景）
    // 已有调用方都不传 id，所以只在指定时才使用 partial.id，行为与原版 100% 兼容。
    const def: WorkflowDefinition = {
      ...partial,
      id: partial.id ?? generateId(),
      createdAt: now,
      updatedAt: now,
    };
    definitions.value = [...definitions.value, def];
    persistDefinitions();
    return def;
  }

  function updateDefinition(id: string, patch: Partial<Omit<WorkflowDefinition, "id" | "createdAt">>): void {
    definitions.value = definitions.value.map(d => (d.id === id ? { ...d, ...patch, updatedAt: new Date().toISOString() } : d));
    persistDefinitions();
  }

  function deleteDefinition(id: string): void {
    // 内置模板不可删除
    const target = definitions.value.find(d => d.id === id);
    if (target?.builtin) return;
    definitions.value = definitions.value.filter(d => d.id !== id);
    persistDefinitions();
  }

  function getDefinition(id: string): WorkflowDefinition | undefined {
    return definitions.value.find(d => d.id === id);
  }

  // ==================== 内置模板管理 ====================

  /**
   * 注册内置模板。如果同名模板已存在则跳过。
   */
  function registerBuiltin(template: WorkflowDefinition): void {
    const exists = definitions.value.some(d => d.builtin && d.name === template.name);
    if (!exists) {
      definitions.value = [...definitions.value, template];
      persistDefinitions();
    }
  }

  /** 批量注册内置模板 */
  function registerBuiltinTemplates(templates: WorkflowDefinition[]): void {
    for (const t of templates) {
      registerBuiltin(t);
    }
  }

  // ==================== 内部持久化 ====================

  function persistDefinitions(): void {
    saveJSON(WORKFLOW_STORE_KEY, definitions.value);
  }

  return {
    definitions,
    createDefinition,
    updateDefinition,
    deleteDefinition,
    getDefinition,
    registerBuiltin,
    registerBuiltinTemplates,
  };
}
