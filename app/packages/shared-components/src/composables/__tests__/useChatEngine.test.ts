/**
 * useChatEngine.test.ts
 *
 * 测试 useChatEngine 的懒初始化机制（修复"刷新页面引擎加载失败"bug）
 *
 * 历史 bug 根因：
 * - useChatEngine.ts 旧版在模块顶层调用 `ensureEngine(activeEngineId.value)`
 * - 但 import 顺序：`useChatEngine` 在 `import '@/engines/defaultEngine'` 之前被解析
 * - 导致首次 ensureEngine 时 registry 还没工厂函数 → currentEngine = null
 * - 用户看到 "引擎加载失败，请刷新页面" 提示
 *
 * 新版修复：
 * - 移除模块顶层 init
 * - useChatEngine() 首次调用时懒初始化（此时所有 import 已执行）
 * - 加 engineInitRetry 重试机制（最多 3 次），覆盖极端 import 顺序场景
 */

import type { ChatEngine } from "@encv/shared-components/composables/chatEngine";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

describe("useChatEngine lazy init (engine load fix)", () => {
  beforeEach(() => {
    // 清理 localStorage（避免上一个测试的引擎选择影响）
    try {
      localStorage.clear();
    } catch {
      /* ignore */
    }
    // 清理动态 import 缓存，强制重新执行模块顶层
    vi.resetModules();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("TestUseChatEngine_DoesNotInitializeAtModuleLoad: 模块加载时不会立即调用 ensureEngine", async () => {
    // 模拟注册表为空的状态（极端 import 顺序）
    await import("@encv/shared-components/composables/chatEngine");
    const factorySpy = vi.spyOn(await import("@encv/shared-components/composables/chatEngine"), "createEngineInstance");

    // 重新加载 useChatEngine —— 此时还未 import 任何引擎
    await import("@encv/shared-components/composables/useChatEngine");

    // 关键：模块加载时不应触发 createEngineInstance
    expect(factorySpy).not.toHaveBeenCalled();
  });

  it("TestUseChatEngine_InitializesOnFirstCall: useChatEngine() 调用时才创建实例", async () => {
    // 模拟正常 import 顺序：先注册 default + tdesign 引擎
    const { registerEngine } = await import("@encv/shared-components/composables/chatEngine");

    const stubEngine: ChatEngine = {
      id: "default",
      name: "Default",
      supportsA2UI: false,
      renderMessages: () => ({}) as ReturnType<ChatEngine["renderMessages"]>,
      destroy: () => {},
    };
    registerEngine("default", () => stubEngine);

    // 现在再 import useChatEngine
    const { useChatEngine } = await import("@encv/shared-components/composables/useChatEngine");

    // 调用 useChatEngine 应触发懒初始化
    const result = useChatEngine();

    // 验证 currentEngine 已被填充
    expect(result.currentEngine.value).not.toBeNull();
    expect(result.currentEngine.value?.id).toBe("default");
  });

  it("TestUseChatEngine_RetriesOnEmptyRegistry: registry 为空时最多重试 3 次", async () => {
    // 第一次：registry 为空
    const { useChatEngine } = await import("@encv/shared-components/composables/useChatEngine");

    // 第一次调用 —— 失败（registry 仍空）
    const r1 = useChatEngine();
    expect(r1.currentEngine.value).toBeNull();

    // 第二次 —— 仍然失败（registry 还是空）
    const r2 = useChatEngine();
    expect(r2.currentEngine.value).toBeNull();

    // 第三次 —— 仍然失败
    const r3 = useChatEngine();
    expect(r3.currentEngine.value).toBeNull();

    // 第四次 —— 应停止重试（达到 3 次上限）
    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const r4 = useChatEngine();
    expect(r4.currentEngine.value).toBeNull();
    // 不再打错误日志（因为已经放弃重试）
    expect(consoleErrorSpy).not.toHaveBeenCalledWith(expect.stringContaining("懒初始化失败"));
    consoleErrorSpy.mockRestore();
  });

  it("TestUseChatEngine_RecoversWhenRegistryFillsLater: registry 后续填充后能自愈", async () => {
    const { useChatEngine } = await import("@encv/shared-components/composables/useChatEngine");
    const { registerEngine } = await import("@encv/shared-components/composables/chatEngine");

    // 第一次：registry 为空，失败
    const r1 = useChatEngine();
    expect(r1.currentEngine.value).toBeNull();

    // 模拟后续动态 import 注册引擎
    const stubEngine: ChatEngine = {
      id: "default",
      name: "Default",
      supportsA2UI: false,
      renderMessages: () => ({}) as ReturnType<ChatEngine["renderMessages"]>,
      destroy: () => {},
    };
    registerEngine("default", () => stubEngine);

    // 第二次调用 —— 这次应该成功了
    const r2 = useChatEngine();
    expect(r2.currentEngine.value).not.toBeNull();
    expect(r2.currentEngine.value?.id).toBe("default");
  });

  it("TestUseChatEngine_SwitchEngine_GoesThroughEnsureEngine: 切换引擎走统一 ensureEngine 路径", async () => {
    const { useChatEngine } = await import("@encv/shared-components/composables/useChatEngine");
    const { registerEngine } = await import("@encv/shared-components/composables/chatEngine");

    const mkStub = (id: string): ChatEngine => ({
      id,
      name: id,
      supportsA2UI: false,
      renderMessages: () => ({}) as ReturnType<ChatEngine["renderMessages"]>,
      destroy: () => {},
    });
    registerEngine("default", () => mkStub("default"));
    registerEngine("tdesign", () => mkStub("tdesign"));

    const r1 = useChatEngine();
    expect(r1.currentEngine.value?.id).toBe("default");

    // 切换到 tdesign
    const ok = r1.switchEngine("tdesign");
    expect(ok).toBe(true);
    expect(r1.currentEngine.value?.id).toBe("tdesign");
  });
});
