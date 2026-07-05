/**
 * TaskBasicInfo 单元测试
 *
 * 覆盖 Task 18：加解密参数区块展示
 * - crypto params 区块在有 cipherMode/compressionMode/extraFields 时显示
 * - crypto params 区块在旧任务（无 crypto 字段）时不显示
 * - cipherMode badge 显示 AES-128 / AES-256
 * - compressionMode badge 显示 Zstd / 无压缩
 * - extraFields 迭代显示
 * - formatExtraFieldLabel: 已知 key 走 i18n，未知 key 退化 Title Case
 * - formatExtraFieldValue: bool 字符串 → ✓/✗，长字符串截断
 */

import type { EncvTask } from "@encv/shared-components/api/encv";
import TaskBasicInfo from "@encv/shared-components/components/TaskBasicInfo.vue";
import { mount } from "@vue/test-utils";
import { beforeEach, describe, expect, it, vi } from "vitest";

// ion-icon stub：避免 @ionic/vue 全局注册依赖
const IonIconStub = {
  name: "IonIcon",
  props: {
    icon: { type: [String, Object], default: null },
  },
  template: '<span class="ion-icon-stub" :data-icon="String(icon)" />',
};

// ion-badge stub：渲染为 span 保留 text 内容
const IonBadgeStub = {
  name: "IonBadge",
  props: {
    color: { type: String, default: "" },
  },
  template: '<span class="ion-badge-stub" :data-color="color"><slot /></span>',
};

// showToast 在测试环境不需要真正弹 toast（避免 toastController 依赖）
vi.mock("@/composables/useToast", () => ({
  showToast: vi.fn().mockResolvedValue(undefined),
}));

/** 构造测试用 EncvTask（便于覆盖默认值） */
function makeTask(overrides: Partial<EncvTask> = {}): EncvTask {
  return {
    id: "task-001",
    type: "encrypt",
    sourcePath: "/storage/emulated/0/sample.mp4",
    status: "completed",
    progress: 100,
    phase: "completed",
    createdAt: "2026-06-18T10:00:00Z",
    completedAt: "2026-06-18T10:01:00Z",
    pluginName: "mp4-plugin",
    containerVersion: 4,
    ...overrides,
  };
}

function mountBasicInfo(task: EncvTask) {
  return mount(TaskBasicInfo, {
    props: { task },
    global: {
      stubs: {
        "ion-icon": IonIconStub,
        "ion-badge": IonBadgeStub,
      },
    },
  });
}

describe("TaskBasicInfo - crypto params 区块 (Task 18)", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("有 cipherMode + compressionMode 时显示 crypto params 区块", () => {
    const task = makeTask({
      cipherMode: 1,
      compressionMode: "zstd",
    });
    const wrapper = mountBasicInfo(task);
    // crypto params 区块标题
    const titles = wrapper.findAll(".section-title");
    const cryptoTitle = titles.find(t => t.text().includes("加解密参数"));
    expect(cryptoTitle).toBeDefined();
  });

  it("旧任务无 crypto 字段时不显示 crypto params 区块", () => {
    const task = makeTask();
    const wrapper = mountBasicInfo(task);
    const titles = wrapper.findAll(".section-title");
    const cryptoTitle = titles.find(t => t.text().includes("加解密参数"));
    expect(cryptoTitle).toBeUndefined();
  });

  it("仅 extraFields 存在时也显示 crypto params 区块", () => {
    const task = makeTask({
      extraFields: { customParam: "value123" },
    });
    const wrapper = mountBasicInfo(task);
    const titles = wrapper.findAll(".section-title");
    const cryptoTitle = titles.find(t => t.text().includes("加解密参数"));
    expect(cryptoTitle).toBeDefined();
  });

  it("cipherMode=1 显示 AES-256 badge", () => {
    const task = makeTask({ cipherMode: 1 });
    const wrapper = mountBasicInfo(task);
    const badges = wrapper.findAll(".ion-badge-stub");
    const aes256Badge = badges.find(b => b.text().includes("AES-256"));
    expect(aes256Badge).toBeDefined();
  });

  it("cipherMode=0 显示 AES-128 badge", () => {
    const task = makeTask({ cipherMode: 0 });
    const wrapper = mountBasicInfo(task);
    const badges = wrapper.findAll(".ion-badge-stub");
    const aes128Badge = badges.find(b => b.text().includes("AES-128"));
    expect(aes128Badge).toBeDefined();
  });

  it("compressionMode=zstd 显示 Zstd badge", () => {
    const task = makeTask({ compressionMode: "zstd" });
    const wrapper = mountBasicInfo(task);
    const badges = wrapper.findAll(".ion-badge-stub");
    const zstdBadge = badges.find(b => b.text().includes("Zstd"));
    expect(zstdBadge).toBeDefined();
  });

  it('compressionMode=none 显示"无压缩" badge', () => {
    const task = makeTask({ compressionMode: "none" });
    const wrapper = mountBasicInfo(task);
    const badges = wrapper.findAll(".ion-badge-stub");
    const noneBadge = badges.find(b => b.text().includes("无压缩"));
    expect(noneBadge).toBeDefined();
  });

  it("extraFields 迭代显示每个 key-value 对", () => {
    const task = makeTask({
      cipherMode: 1,
      extraFields: {
        fnRounds: "8",
        fnCharset: "base64",
      },
    });
    const wrapper = mountBasicInfo(task);
    // extraFields 的值会渲染在 .extra-field-value 中
    const extraValues = wrapper.findAll(".extra-field-value");
    expect(extraValues.length).toBeGreaterThanOrEqual(2);
    const texts = extraValues.map(v => v.text());
    expect(texts).toContain("8");
    expect(texts).toContain("base64");
  });

  it("extraFields 已知 key (fnRounds) 走 i18n 显示中文标签", () => {
    const task = makeTask({
      extraFields: { fnRounds: "8" },
    });
    const wrapper = mountBasicInfo(task);
    // fnRounds → tasks.fnRounds → "Feistel 轮数"
    const labels = wrapper.findAll(".info-label");
    const feistelLabel = labels.find(l => l.text().includes("Feistel"));
    expect(feistelLabel).toBeDefined();
  });

  it("extraFields 未知 key 退化到 Title Case", () => {
    const task = makeTask({
      extraFields: { some_custom_field: "abc" },
    });
    const wrapper = mountBasicInfo(task);
    const labels = wrapper.findAll(".info-label");
    // some_custom_field → "Some Custom Field"
    const titleLabel = labels.find(l => l.text().includes("Some Custom Field"));
    expect(titleLabel).toBeDefined();
  });

  it('extraFields bool 值 "true" → ✓', () => {
    const task = makeTask({
      extraFields: { encryptFilename: "true" },
    });
    const wrapper = mountBasicInfo(task);
    const extraValues = wrapper.findAll(".extra-field-value");
    const checkValue = extraValues.find(v => v.text() === "✓");
    expect(checkValue).toBeDefined();
  });

  it('extraFields bool 值 "false" → ✗', () => {
    const task = makeTask({
      extraFields: { encryptFilename: "false" },
    });
    const wrapper = mountBasicInfo(task);
    const extraValues = wrapper.findAll(".extra-field-value");
    const crossValue = extraValues.find(v => v.text() === "✗");
    expect(crossValue).toBeDefined();
  });

  it("extraFields 长字符串（>32 字符）截断显示", () => {
    const longValue = "a".repeat(50);
    const task = makeTask({
      extraFields: { longParam: longValue },
    });
    const wrapper = mountBasicInfo(task);
    const extraValues = wrapper.findAll(".extra-field-value");
    const longValueEl = extraValues.find(v => v.text().includes("…"));
    expect(longValueEl).toBeDefined();
    // 截断格式：前 8 + … + 后 4
    expect(longValueEl?.text()).toContain("aaaaaaaa…aaaa");
  });
});
