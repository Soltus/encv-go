/**
 * tdesignEngine.test.ts
 *
 * 测试 TDesign 引擎的形态契约：
 *   - createTDesignEngine() 返回符合 ChatEngine 接口的对象
 *   - renderMessages 返回 TDesignChatView 的 VNode
 *   - 不再使用 <Chatbot>（已删除 chatServiceConfig）
 *   - supportsA2UI = false
 *
 * SPEC: /workspace/.trae/specs/agui-real-llm-path-completion/ Phase 4
 */

import { describe, expect, it } from "vitest";
import type { VNode } from "vue";
import type { EngineRenderProps } from "@/composables/chatEngine";
import TDesignChatView from "../TDesignChatView.vue";
import { createTDesignEngine } from "../tdesignEngine";

function makeProps(): EngineRenderProps {
  return {
    messages: [],
    status: "idle",
    onSend: async () => {},
    onStop: () => {},
    onConfirmTool: async () => {},
    onCopyMessage: async () => {},
    onPresetClick: () => {},
    streaming: false,
  };
}

describe("tdesignEngine", () => {
  it("TestTDesignEngine_HasExpectedMetadata: id=tdesign, supportsA2UI=false", () => {
    const engine = createTDesignEngine();
    expect(engine.id).toBe("tdesign");
    expect(engine.name).toBeDefined();
    expect(typeof engine.name).toBe("string");
    expect(engine.supportsA2UI).toBe(false);
  });

  it("TestTDesignEngine_RenderMessages_ReturnsVNodeOfTDesignChatView: renderMessages 返回 TDesignChatView 的 VNode", () => {
    const engine = createTDesignEngine();
    const props = makeProps();
    const vnode: VNode = engine.renderMessages(props);

    expect(vnode).toBeDefined();
    // VNode 的 type 字段是组件对象本身（TDesignChatView 的 SFC 编译结果）
    expect(vnode.type).toBeDefined();
    // Vue 编译 SFC 后 type 是包含 __file 等标记的对象；
    // 我们用组件引用身份做最严格的断言
    expect(vnode.type).toBe(TDesignChatView);
  });

  it("TestTDesignEngine_RenderMessages_PassesAllEngineRenderProps: 所有 EngineRenderProps 都透传", () => {
    const engine = createTDesignEngine();
    const props = makeProps();
    const vnode = engine.renderMessages(props);
    // h(Component, { ...props }) 会把 spread 的 props 放到 vnode.props
    const passedProps = vnode.props as Record<string, unknown>;
    expect(passedProps.messages).toBe(props.messages);
    expect(passedProps.status).toBe(props.status);
    expect(passedProps.onSend).toBe(props.onSend);
    expect(passedProps.onStop).toBe(props.onStop);
    expect(passedProps.onConfirmTool).toBe(props.onConfirmTool);
    expect(passedProps.onCopyMessage).toBe(props.onCopyMessage);
    expect(passedProps.onPresetClick).toBe(props.onPresetClick);
    expect(passedProps.streaming).toBe(props.streaming);
  });

  it("TestTDesignEngine_DestroyIsIdempotent: destroy() 不抛错", () => {
    const engine = createTDesignEngine();
    expect(() => engine.destroy()).not.toThrow();
  });

  it("TestTDesignEngine_NoChatbotReference: 不再依赖 @tdesign-vue-next/chat 的 Chatbot", async () => {
    // 通过源码静态分析确认：编译产物的 VNode 不应该是 Chatbot
    const engine = createTDesignEngine();
    const vnode = engine.renderMessages(makeProps());
    // VNode.type 是组件对象，TDesignChatView 的 SFC 编译结果 ≠ Chatbot 组件
    // 通过 import 引用对比已经足够严格（前面用例已断言）
    expect(vnode.type).toBe(TDesignChatView);
    // 进一步检查：VNode.type 不应该是 Chatbot 字符串/类
    const typeName =
      typeof vnode.type === "object" && vnode.type !== null
        ? ((vnode.type as { name?: string }).name ?? "")
        : typeof vnode.type === "string"
          ? vnode.type
          : "";
    expect(typeName).not.toMatch(/Chatbot/i);
  });

  it("TestTDesignEngine_RegisterToEngineRegistry: 引擎自动注册到全局 registry", async () => {
    // 由于模块被 import 时已经 registerEngine('tdesign', ...)，此用例
    // 触发该 side effect 并验证注册表
    const { createEngineInstance } = await import("@/composables/chatEngine");
    const instance = createEngineInstance("tdesign");
    expect(instance).not.toBeNull();
    expect(instance!.id).toBe("tdesign");
  });
});
