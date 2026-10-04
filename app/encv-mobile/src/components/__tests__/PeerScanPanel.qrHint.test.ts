/**
 * PeerScanPanel.qrHint.test.ts —— 扫码失败**必须给出可执行的引导**（2026-10-05）
 *
 * 真机问题：手机拿一张过期/已用过的配对码去连，后端只回一句 `pair_rejected:401`，
 *   界面显示"连接失败：…" ⇒ 用户拿着同一张注定失败的码反复扫（真机反馈：不知道该怎么办）。
 *
 * 本用例用**真实 DOM**（@vue/test-utils mount + happy-dom）走完整条路径：
 *   粘贴配对码 → 点连接 → 后端 502 + reason=ticket_expired → 断言界面提示"刷新二维码"。
 * 只测 composable 不够：文案是组件里渲染的，映射漏了它就是一行"连接失败"。
 */

import { mount } from "@vue/test-utils";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import PeerScanPanel from "@/components/PeerScanPanel.vue";
import { registerI18nModule } from "@encv/shared-components/composables/useI18n";
import { setPeerLinkFetchProvider } from "@encv/shared-components/composables/usePeerLink";
import settingsMessages from "@encv/shared-components/i18n/settings";

// ⚠️ 组件文案走 i18n：不注册字典的话 t() 返回 "[MISSING: ...]"，
//    断言就变成在测"缺 key"，而不是在测"用户看到了什么"。
registerI18nModule(settingsMessages);

function jsonLike(status: number, body: unknown) {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
    text: async () => JSON.stringify(body),
  } as unknown as Response;
}

const PAIRING_CODE = JSON.stringify({
  v: 2,
  hub: "https://hub.example/api/peerlink",
  pairingId: "a".repeat(32),
  psk: "f".repeat(64),
  exp: 119,
});

async function submitCode(wrapper: ReturnType<typeof mount>) {
  const input = wrapper.get<HTMLTextAreaElement>('[data-testid="paste-input"]');
  await input.setValue(PAIRING_CODE);
  await wrapper.get('[data-testid="paste-submit"]').trigger("click");
  // 等组件内的异步 pair 走完（真实 microtask + 一次 tick 足够，fetch provider 是同步 Promise）
  await new Promise(r => setTimeout(r, 0));
  await wrapper.vm.$nextTick();
}

beforeEach(() => {
  // /edge/status：未连接，避免 onMounted 的刷新干扰断言
  setPeerLinkFetchProvider((async (url: string) => {
    if (String(url).includes("/edge/status")) return jsonLike(200, { running: false });
    return jsonLike(502, {});
  }) as unknown as typeof fetch);
});

afterEach(() => {
  setPeerLinkFetchProvider(((...args: Parameters<typeof fetch>) => fetch(...args)) as typeof fetch);
});

describe("PeerScanPanel：失败的下一步引导", () => {
  it("票据过期 → 错误文案与提示都指向「刷新二维码」", async () => {
    setPeerLinkFetchProvider((async (url: string) => {
      if (String(url).includes("/edge/status")) return jsonLike(200, { running: false });
      return jsonLike(502, {
        error: "pair_failed",
        reason: "ticket_expired",
        refreshQr: true,
        detail: "pair_rejected:401:ticket_expired",
      });
    }) as unknown as typeof fetch);

    const wrapper = mount(PeerScanPanel, { global: { stubs: { "ion-button": true } } });
    await new Promise(r => setTimeout(r, 0));
    await submitCode(wrapper);

    const err = wrapper.get('[data-testid="scan-error"]').text();
    expect(err).toContain("刷新二维码");
    // 光说失败不够，还要告诉用户**怎么做**
    const hint = wrapper.get('[data-testid="scan-hint"]').text();
    expect(hint).toContain("重新生成");
    expect(wrapper.find('[data-testid="scan-ok"]').exists()).toBe(false);
  });

  it("非票据类失败（连不上会合点）→ 不得误导成刷新二维码", async () => {
    setPeerLinkFetchProvider((async (url: string) => {
      if (String(url).includes("/edge/status")) return jsonLike(200, { running: false });
      return jsonLike(502, { error: "pair_failed", detail: "connection refused" });
    }) as unknown as typeof fetch);

    const wrapper = mount(PeerScanPanel, { global: { stubs: { "ion-button": true } } });
    await new Promise(r => setTimeout(r, 0));
    await submitCode(wrapper);

    const err = wrapper.get('[data-testid="scan-error"]').text();
    expect(err).toContain("连接失败");
    expect(err).not.toContain("刷新二维码");
    // 没有票据类提示（否则就是把"网络不通"当成"码有问题"）
    expect(wrapper.find('[data-testid="scan-hint"]').exists()).toBe(false);
  });
});
