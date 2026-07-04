/**
 * useChatEngine.ts - 运行时引擎切换器
 *
 * 管理当前活跃的 ChatEngine 实例，支持：
 * - localStorage 持久化引擎选择
 * - reactive 响应式切换（无需刷新页面）
 * - 自动 fallback 到 default 引擎
 * - destroy/实例化生命周期管理
 *
 * SPEC: /workspace/.trae/specs/multi-engine-chat-architecture/tasks.md Task 1.2
 */

import { type Ref, ref, type ShallowRef, shallowRef } from "vue";
import type { ChatEngine } from "./chatEngine";
import { createEngineInstance, getRegisteredEngines } from "./chatEngine";

// =============================================================================
// 常量
// =============================================================================

const STORAGE_KEY = "encv-chat-engine";
const DEFAULT_ENGINE_ID = "default";

// =============================================================================
// 引擎状态（模块级单例，响应式）
// =============================================================================

/**
 * 当前活跃引擎实例（shallowRef，切换时触发重新渲染）
 * 模块级单例——所有 useChatEngine() 调用共享同一个 ref
 *
 * **关键**：不在模块加载时初始化！
 * - 模块级 init 会先于 AgentChat.vue 的 `import '@/engines/defaultEngine'`
 *   等注册副作用执行（import 顺序由 vue-loader / vite 决定，模块副作用
 *   与 import 解析顺序有关）
 * - 旧版在模块顶层调用 `ensureEngine(activeEngineId.value)`，导致
 *   registry 还没有任何工厂 → currentEngine = null → 用户看到
 *   "引擎加载失败，请刷新页面"。
 * - 新版改为**懒初始化**：在 useChatEngine() 首次调用时才创建实例，
 *   此时 AgentChat.vue 的 setup() 上下文里所有 import 已完成，
 *   引擎注册副作用已执行。
 */
const currentEngine: ShallowRef<ChatEngine | null> = shallowRef(null);

/** 当前引擎 ID（响应式，用于 select 绑定） */
const activeEngineId = ref(loadSavedEngineId());

/** 所有已注册引擎的元信息列表（响应式） */
const engineList = ref(getRegisteredEngines());

/** 引擎初始化重试计数器（防止极端 import 顺序下注册延迟到后续调用） */
let engineInitRetry = 0;

/**
 * 从 localStorage 加载保存的引擎 ID
 * 如果保存的值无效或引擎不存在，返回 DEFAULT_ENGINE_ID
 */
function loadSavedEngineId(): string {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved && typeof saved === "string" && saved.trim().length > 0) {
      return saved.trim();
    }
  } catch {
    // localStorage 可能不可用（SSR / 隐私模式）
  }
  return DEFAULT_ENGINE_ID;
}

/** 持久化引擎选择 */
function saveEngineId(id: string): void {
  try {
    localStorage.setItem(STORAGE_KEY, id);
  } catch {
    // 静默失败
  }
}

/**
 * 确保引擎可用 —— 如果实例不存在则创建，失败则尝试 default
 */
function ensureEngine(id: string): ChatEngine | null {
  let instance = createEngineInstance(id);
  if (!instance && id !== DEFAULT_ENGINE_ID) {
    console.warn(`[useChatEngine] Engine "${id}" not found, falling back to default`);
    instance = createEngineInstance(DEFAULT_ENGINE_ID);
    if (instance) {
      activeEngineId.value = DEFAULT_ENGINE_ID;
      saveEngineId(DEFAULT_ENGINE_ID);
    }
  }
  return instance;
}

// 初始化：移到 useChatEngine() 内部懒初始化（见下方函数体内的 lazy init 块）
// 旧版这里在模块顶层调用 ensureEngine(...) 会因 import 顺序问题失败。

// =============================================================================
// useChatEngine Composable
// =============================================================================

export interface UseChatEngineReturn {
  /** 当前活跃引擎（shallowRef，切换时触发重新渲染） */
  currentEngine: ShallowRef<ChatEngine | null>;
  /** 当前引擎 ID（响应式，用于 select 绑定） */
  currentEngineId: Ref<string>;
  /** 所有已注册引擎的元信息列表（响应式） */
  engineList: Ref<Array<{ id: string; name: string; description?: string }>>;
  /**
   * 切换到指定引擎
   * @param id 目标引擎 id
   * @returns 是否切换成功
   */
  switchEngine: (id: string) => boolean;
}

/**
 * 获取引擎切换器实例
 *
 * 所有调用方共享同一个模块级响应式状态（currentEngine / activeEngineId / engineList），
 * 切换引擎时所有使用方自动更新。
 */
export function useChatEngine(): UseChatEngineReturn {
  // ── 懒初始化：首次调用时创建引擎实例 ──────────────────────
  // 此时 AgentChat.vue setup() 上下文里所有 import 已执行完毕，
  // `import '@/engines/defaultEngine'` / `import '@/engines/tdesignEngine'`
  // 这类带副作用的 import 都已 registerEngine(...) 完成，registry 不为空。
  //
  // 自愈机制：如果首次 init 时 registry 还为空（极端 import 顺序），
  // 每次调用都重试直到成功（最多 3 次），避免"刷新页面看到加载失败"。
  if (!currentEngine.value && engineInitRetry < 3) {
    engineInitRetry += 1;
    const inst = ensureEngine(activeEngineId.value);
    if (inst) {
      currentEngine.value = inst;
      activeEngineId.value = inst.id;
    } else {
      console.error(
        "[useChatEngine] 懒初始化失败（第 " +
          engineInitRetry +
          " 次）：registry 仍为空。" +
          "请确认 AgentChat.vue 或其父组件已 import @/engines/defaultEngine 和 @/engines/tdesignEngine。"
      );
    }
  }

  // 引擎模块可能在 useChatEngine() 首次调用前已通过 import 注册，
  // 但 engineList 在模块初始化时可能为空（注册还没执行）。
  // 每次调用时刷新列表，确保 UI 能看到所有已注册引擎。
  engineList.value = getRegisteredEngines();

  function switchEngine(id: string): boolean {
    if (id === activeEngineId.value && currentEngine.value) {
      return true; // 已经是目标引擎
    }

    // 销毁旧引擎
    if (currentEngine.value) {
      try {
        currentEngine.value.destroy();
      } catch (err) {
        console.warn(`[useChatEngine] Error destroying previous engine`, err);
      }
    }

    // 创建新引擎
    const newEngine = ensureEngine(id);
    if (!newEngine) {
      // fallback：如果目标引擎创建失败，回退到 default
      console.warn(`[useChatEngine] Engine "${id}" failed to create, falling back to default`);
      const fallback = ensureEngine(DEFAULT_ENGINE_ID);
      currentEngine.value = fallback;
      activeEngineId.value = DEFAULT_ENGINE_ID;
      saveEngineId(DEFAULT_ENGINE_ID);
      return false;
    }

    currentEngine.value = newEngine;
    activeEngineId.value = id;
    saveEngineId(id);

    // 刷新引擎列表（可能有新引擎注册）
    engineList.value = getRegisteredEngines();

    return true;
  }

  return {
    currentEngine,
    currentEngineId: activeEngineId,
    engineList,
    switchEngine,
  };
}

/** 导出常量供外部使用 */
export { DEFAULT_ENGINE_ID, STORAGE_KEY };
