/**
 * tdesignEngine.ts - 腾讯 TDesign 视觉风格渲染引擎
 *
 * 数据源：通过 useAgent 共享的 AG-UI 解析后的 Message[]（不自行消费 SSE）
 * 渲染层：本引擎用 TDesign 视觉组件渲染 Message[]
 *
 * 协议：AG-UI（与 Default 引擎共享同一份数据）
 *
 * 演化历史：
 *  - v1：直接使用 @tdesign-vue-next/chat 的 <Chatbot> 组件 + chatServiceConfig，
 *        <Chatbot> 内置独立 ChatService 实例再消费一份 SSE → 与 useAgent 数据源冲突
 *  - v2：改为纯渲染层，用 <ChatList :data="Message[]"> 展示——但 ChatList
 *        把整条 m.content 当一个 ChatItem 渲染 → 文本累积成单块 markdown；
 *        tool_calls 排在底部 → 用户痛点（"渲染排版不对"）
 *  - v3：改用 useRenderTurnItems(messages, status, compactionText) 拿
 *        RenderedItem[]，按 eventLog 时间轴逐项渲染（与 Default 引擎
 *        同源数据，TDesign 仅提供差异化视觉）→ 文本段和工具调用交错
 *
 * SPEC: /workspace/.trae/specs/agui-real-llm-path-completion/ Phase 4
 */

import type { VNode } from "vue";
import { h } from "vue";
import type { ChatEngine, EngineRenderProps } from "@encv/shared-components/composables/chatEngine";
import { registerEngine } from "@encv/shared-components/composables/chatEngine";
import TDesignChatView from "./TDesignChatView.vue";

export function createTDesignEngine(): ChatEngine {
  return {
    id: "tdesign",
    name: "TDesign 风格",
    description: "腾讯 TDesign 视觉风格的聊天渲染",
    supportsA2UI: false,

    /**
     * 渲染消息列表区域 —— 纯 VNode 透传
     * 把 EngineRenderProps 全部 props 传给 TDesignChatView
     * （消息 / 状态 / 回调等所有上下文都从宿主来）
     */
    renderMessages(props: EngineRenderProps): VNode {
      return h(TDesignChatView, { ...props });
    },

    destroy(): void {
      // 无需清理：TDesignChatView 是无状态纯组件
    },
  };
}

registerEngine("tdesign", createTDesignEngine);
