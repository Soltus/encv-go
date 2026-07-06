/**
 * chatEngine.ts - 多渲染引擎架构：统一接口定义
 *
 * 定义 ChatEngine 抽象接口、EngineContext 共享状态、EngineRegistry 注册表。
 * 所有聊天渲染引擎（Default / TDesign）都实现此接口，
 * AgentChat.vue 作为宿主容器通过 useChatEngine() 动态切换引擎实例。
 *
 * SPEC: /workspace/.trae/specs/multi-engine-chat-architecture/spec.md
 */

import type { VNode } from "vue";
import type { AgentStatus, Message, MockPreset } from "./useAgent";

// =============================================================================
// 引擎渲染 Props（从宿主传入引擎）
// =============================================================================

/** 消息列表渲染所需的全部上下文 */
export interface EngineRenderProps {
  /** 完整消息列表（共享数据源，来自 useAgent） */
  messages: readonly Message[];
  /** 当前 agent 状态 */
  status: AgentStatus;
  /** 发送消息 */
  onSend: (text: string) => Promise<void>;
  /** 停止生成 */
  onStop: () => void;
  /** 确认工具调用 */
  onConfirmTool: (toolCallId: string, decision: string) => Promise<void>;
  /** 复制消息内容 */
  onCopyMessage: (messageId: string) => Promise<void>;
  /** Mock 预设按钮点击 */
  onPresetClick: (userText: string) => void;
  /** 当前是否正在流式输出 */
  streaming: boolean;
}

/** 输入区域渲染 Props */
export interface EngineInputProps {
  /** 当前输入框文本 */
  inputValue: string;
  /** 更新输入框文本 */
  onInputUpdate: (value: string) => void;
  /** 发送处理 */
  onSend: (text: string) => Promise<void>;
  /** 停止生成 */
  onStop: () => void;
  /** 是否可发送 */
  canSend: boolean;
  /** 是否正在流式输出 */
  streaming: boolean;
  /** Mock 预设列表 */
  presets: readonly MockPreset[];
  /** 预设点击回调 */
  onPresetClick: (userText: string) => void;
}

// =============================================================================
// ChatEngine 接口
// =============================================================================

/**
 * 聊天渲染引擎 —— 统一抽象接口
 *
 * 每种 UI 实现（Ionic 默认 / TDesign Chat）都实现此接口。
 * AgentChat.vue 通过 <component :is="engine.renderMessages(props)" /> 动态渲染。
 */
export interface ChatEngine {
  /** 引擎唯一标识（如 'default' / 'tdesign'） */
  readonly id: string;
  /** 显示名称（用于切换器 UI） */
  readonly name: string;
  /** 简短描述（用于切换器 tooltip） */
  readonly description?: string;
  /** 是否支持 A2UI Surface 渲染（本轮所有引擎 = false，预留扩展点） */
  readonly supportsA2UI: boolean;

  /**
   * 渲染消息列表区域 —— 核心方法
   * 返回 VNode（可以是单个组件或 Fragment 包含多个子组件）
   */
  renderMessages(props: EngineRenderProps): VNode;

  /**
   * 渲染输入区域（可选 —— 引擎可以不提供，使用宿主默认输入框）
   * 如果返回 null/undefined，宿主使用内置输入框
   */
  renderInput?(props: EngineInputProps): VNode | null;

  /**
   * 用户发送文本时的额外钩子（引擎可拦截或增强发送行为）
   * 如果返回 false，阻止默认发送流程
   */
  onSend?(text: string): Promise<boolean> | undefined;

  /**
   * 停止生成时的额外钩子
   */
  onStop?(): void;

  /**
   * A2UI Surface 渲染（可选 —— 仅 supportsA2UI=true 的引擎实现）
   * 由 agent JSONL 描述的声明式 UI，渲染为 VNode
   */
  renderSurface?(surfaceId: string, payload: unknown): VNode;

  /**
   * 引擎销毁清理（切换引擎时调用）
   * 清理定时器、事件监听器、订阅等资源
   */
  destroy(): void;
}

/**
 * A2UI 扩展接口（预留，本轮不实现）
 * 当 supportsA2UI=true 时，引擎还需实现此接口以支持 A2UI Surface 渲染
 */
export interface A2UICapableEngine extends ChatEngine {
  supportsA2UI: true;
  /** 渲染 A2UI Surface（由 agent JSONL 描述的声明式 UI） */
  renderSurface(surfaceId: string, payload: unknown): VNode;
}

// =============================================================================
// EngineRegistry（引擎注册表）
// =============================================================================

type EngineFactory = () => ChatEngine;

/** 全局引擎注册表：id → 工厂函数 */
const registry = new Map<string, EngineFactory>();

/** 注册一个引擎 */
export function registerEngine(id: string, factory: EngineFactory): void {
  if (registry.has(id)) {
    console.warn(`[chatEngine] Engine "${id}" already registered, overwriting`);
  }
  registry.set(id, factory);
}

/** 创建引擎实例（通过工厂函数） */
export function createEngineInstance(id: string): ChatEngine | null {
  const factory = registry.get(id);
  if (!factory) {
    console.error(`[chatEngine] Unknown engine id: "${id}"`);
    return null;
  }
  try {
    return factory();
  } catch (err) {
    console.error(`[chatEngine] Failed to instantiate engine "${id}"`, err);
    return null;
  }
}

/** 获取所有已注册引擎的元信息列表（用于切换器 UI） */
export function getRegisteredEngines(): Array<{ id: string; name: string; description?: string }> {
  return Array.from(registry.keys()).map(id => {
    // 创建临时实例读取元信息后立即销毁
    try {
      const instance = registry.get(id)!();
      const info = { id, name: instance.name, description: instance.description };
      instance.destroy();
      return info;
    } catch {
      return { id, name: id, description: undefined };
    }
  });
}
