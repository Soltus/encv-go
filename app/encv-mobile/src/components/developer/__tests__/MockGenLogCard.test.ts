/**
 * MockGenLogCard 单元测试
 *
 * 覆盖 Task 13 SubTask 13.5：
 * - 基础渲染：log 数组渲染为 UnifiedTimelineCard 列表
 * - summary 渲染：total / ok / failed / skipped / disconnected
 * - copied 状态显示
 * - toggle emit：点击条目触发 toggle 事件
 * - copy emit：点击复制按钮触发 copy 事件
 * - MockGenLogEntry → UnifiedTimelineEntry 转换正确（runner 图标 / status 映射 / meta 拼接 / expandDetail.extra 填充）
 * - 空日志状态
 * - detail slot 渲染（ffmpeg args / stderr / context 卡片）
 */

import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import MockGenLogCard from "@/components/developer/MockGenLogCard.vue";
import UnifiedTimelineCard from "@encv/shared-components/components/shared/UnifiedTimelineCard.vue";
import type { MockGenLogEntry, MockGenLogSummary } from "@encv/shared-components/composables/useMockGenLog";

// ion-icon stub：避免 @ionic/vue 全局注册依赖
const IonIconStub = {
  name: "IonIcon",
  props: {
    icon: { type: [String, Object], default: null },
  },
  template: '<span class="ion-icon-stub" :data-icon="String(icon)" />',
};

/** 构造测试用 MockGenLogEntry（便于覆盖默认值） */
function makeEntry(overrides: Partial<MockGenLogEntry> = {}): MockGenLogEntry {
  return {
    key: "1-sample.mp4-ok",
    index: 1,
    total: 3,
    relativePath: "01-plain-media/video/sample.mp4",
    status: "ok",
    encoder: "h264+aac (-c copy)",
    runner: "ffmpeg",
    ffmpegArgs: ["-i", "input.mp4", "-c", "copy", "output.mp4"],
    exitCode: 0,
    stderr: "",
    at: "2026-06-18T14:30:00.000Z",
    expanded: false,
    ...overrides,
  };
}

/** 构造测试用 MockGenLogSummary */
function makeSummary(overrides: Partial<MockGenLogSummary> = {}): MockGenLogSummary {
  return {
    total: 3,
    ok: 2,
    failed: 1,
    skipped: 0,
    disconnected: false,
    ...overrides,
  };
}

function mountCard(props: { log: MockGenLogEntry[]; summary?: MockGenLogSummary | null; copied?: boolean }) {
  return mount(MockGenLogCard, {
    props: {
      summary: null,
      copied: false,
      ...props,
    },
    global: {
      stubs: { "ion-icon": IonIconStub },
    },
  });
}

describe("MockGenLogCard - 基础渲染", () => {
  it("log 为空时不渲染卡片", () => {
    const wrapper = mountCard({ log: [] });
    expect(wrapper.find(".mock-gen-log-card").exists()).toBe(false);
  });

  it("log 有条目时渲染卡片", () => {
    const wrapper = mountCard({ log: [makeEntry()] });
    expect(wrapper.find(".mock-gen-log-card").exists()).toBe(true);
  });

  it("渲染标题 + 计数", () => {
    const wrapper = mountCard({
      log: [makeEntry(), makeEntry({ key: "2-sample.mp3-ok", index: 2 })],
      summary: makeSummary({ total: 5 }),
    });
    expect(wrapper.find(".mock-gen-log-title").text()).toContain("FFMPEG 流程日志");
    expect(wrapper.find(".mock-gen-log-count").text()).toBe("2 / 5");
  });

  it("summary 为 null 时计数用 log.length 兜底", () => {
    const wrapper = mountCard({
      log: [makeEntry(), makeEntry({ key: "2-sample.mp3-ok", index: 2 })],
      summary: null,
    });
    expect(wrapper.find(".mock-gen-log-count").text()).toBe("2 / 2");
  });

  it("每个 log 条目渲染为一个 UnifiedTimelineCard 子组件", () => {
    const wrapper = mountCard({
      log: [makeEntry({ key: "1" }), makeEntry({ key: "2", index: 2 }), makeEntry({ key: "3", index: 3 })],
    });
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards).toHaveLength(3);
  });
});

describe("MockGenLogCard - summary 渲染", () => {
  it("summary 存在时渲染汇总行", () => {
    const wrapper = mountCard({
      log: [makeEntry()],
      summary: makeSummary(),
    });
    expect(wrapper.find(".mock-gen-log-summary").exists()).toBe(true);
  });

  it("summary 为 null 时不渲染汇总行", () => {
    const wrapper = mountCard({
      log: [makeEntry()],
      summary: null,
    });
    expect(wrapper.find(".mock-gen-log-summary").exists()).toBe(false);
  });

  it("summary 文本格式：ok ✓ / failed ✗ / skipped ◌", () => {
    const wrapper = mountCard({
      log: [makeEntry()],
      summary: makeSummary({ ok: 2, failed: 1, skipped: 0 }),
    });
    expect(wrapper.find(".mock-gen-log-summary").text()).toContain("2 ✓ / 1 ✗ / 0 ◌");
  });

  it("summary.disconnected=true 时显示中断提示", () => {
    const wrapper = mountCard({
      log: [makeEntry()],
      summary: makeSummary({ disconnected: true, total: 5 }),
    });
    expect(wrapper.find(".mock-gen-log-disconnect").exists()).toBe(true);
    expect(wrapper.find(".mock-gen-log-disconnect").text()).toContain("后端连接已断开");
  });

  it("summary.disconnected=true 时汇总文本包含流中断信息", () => {
    const wrapper = mountCard({
      log: [makeEntry()],
      summary: makeSummary({ disconnected: true, total: 5, ok: 1, failed: 0, skipped: 0 }),
    });
    const text = wrapper.find(".mock-gen-log-summary").text();
    expect(text).toContain("流中断于 1/5");
  });

  it("summary.failed > 0 时显示 warning 图标", () => {
    const wrapper = mountCard({
      log: [makeEntry()],
      summary: makeSummary({ failed: 1 }),
    });
    const icon = wrapper.find(".mock-gen-log-summary .ion-icon-stub");
    expect(icon.exists()).toBe(true);
  });
});

describe("MockGenLogCard - copied 状态", () => {
  it("copied=false 时显示「复制全部」", () => {
    const wrapper = mountCard({
      log: [makeEntry()],
      copied: false,
    });
    expect(wrapper.find(".mock-gen-log-copy").text()).toContain("复制全部");
    expect(wrapper.find(".mock-gen-log-copy").classes()).not.toContain("mock-gen-log-copy--copied");
  });

  it("copied=true 时显示「已复制」+ copied class", () => {
    const wrapper = mountCard({
      log: [makeEntry()],
      copied: true,
    });
    expect(wrapper.find(".mock-gen-log-copy").text()).toContain("已复制");
    expect(wrapper.find(".mock-gen-log-copy").classes()).toContain("mock-gen-log-copy--copied");
  });
});

describe("MockGenLogCard - toggle / copy emit", () => {
  it("点击复制按钮触发 copy 事件", async () => {
    const wrapper = mountCard({
      log: [makeEntry()],
    });
    await wrapper.find(".mock-gen-log-copy").trigger("click");
    expect(wrapper.emitted("copy")).toHaveLength(1);
  });

  it("UnifiedTimelineCard 触发 toggle 时转发为 toggle(key) 事件", async () => {
    const entry = makeEntry({ key: "test-key-1" });
    const wrapper = mountCard({
      log: [entry],
    });
    // 找到 UnifiedTimelineCard 子组件，触发 toggle 事件
    const card = wrapper.findComponent(UnifiedTimelineCard);
    expect(card.exists()).toBe(true);
    await card.vm.$emit("toggle", true);
    expect(wrapper.emitted("toggle")).toBeDefined();
    expect(wrapper.emitted("toggle")![0]).toEqual(["test-key-1"]);
  });
});

describe("MockGenLogCard - MockGenLogEntry → UnifiedTimelineEntry 转换", () => {
  it("status=ok 转换为 UnifiedTimelineCard 的 success 状态", () => {
    const wrapper = mountCard({
      log: [makeEntry({ status: "ok" })],
    });
    const card = wrapper.findComponent(UnifiedTimelineCard);
    expect(card.props("entry").status).toBe("success");
  });

  it("status=failed 转换为 UnifiedTimelineCard 的 failure 状态", () => {
    const wrapper = mountCard({
      log: [makeEntry({ status: "failed", exitCode: 1, stderr: "encoder not found" })],
    });
    const card = wrapper.findComponent(UnifiedTimelineCard);
    expect(card.props("entry").status).toBe("failure");
  });

  it("status=pending 转换为 UnifiedTimelineCard 的 running 状态", () => {
    const wrapper = mountCard({
      log: [makeEntry({ status: "pending" })],
    });
    const card = wrapper.findComponent(UnifiedTimelineCard);
    expect(card.props("entry").status).toBe("running");
  });

  it("label 字段使用 relativePath", () => {
    const wrapper = mountCard({
      log: [makeEntry({ relativePath: "01-plain-media/audio/music.mp3" })],
    });
    const card = wrapper.findComponent(UnifiedTimelineCard);
    expect(card.props("entry").label).toBe("01-plain-media/audio/music.mp3");
  });

  it("time 字段使用 at（v3 Task 9：formatDateTime 格式化）", () => {
    const wrapper = mountCard({
      log: [makeEntry({ at: "2026-06-18T10:00:00Z" })],
    });
    const card = wrapper.findComponent(UnifiedTimelineCard);
    // 🆕 v3 2026-06-18 Task 9：time 现在走 formatDateTime，不再是原始 ISO 字符串
    //   formatDateTime('2026-06-18T10:00:00Z') → '2026/06/18 10:00'（本地时区）
    expect(card.props("entry").time).toBeTruthy();
    expect(card.props("entry").time).not.toBe("2026-06-18T10:00:00Z");
  });

  it("id 字段使用 key", () => {
    const wrapper = mountCard({
      log: [makeEntry({ key: "custom-key-123" })],
    });
    const card = wrapper.findComponent(UnifiedTimelineCard);
    expect(card.props("entry").id).toBe("custom-key-123");
  });

  it("hasExpandableDetail 始终为 true（所有条目都可展开）", () => {
    const wrapper = mountCard({
      log: [makeEntry()],
    });
    const card = wrapper.findComponent(UnifiedTimelineCard);
    expect(card.props("entry").hasExpandableDetail).toBe(true);
  });

  it("expanded prop 传递给 UnifiedTimelineCard（受控模式）", () => {
    const wrapper = mountCard({
      log: [makeEntry({ key: "1", expanded: false }), makeEntry({ key: "2", index: 2, expanded: true })],
    });
    const cards = wrapper.findAllComponents(UnifiedTimelineCard);
    expect(cards[0].props("expanded")).toBe(false);
    expect(cards[1].props("expanded")).toBe(true);
  });

  it("failed 状态 + stderr 时 expandDetail.error 包含 stderr 第一行", () => {
    const wrapper = mountCard({
      log: [
        makeEntry({
          status: "failed",
          stderr: "Unknown encoder libmp3lame\nffmpeg exited",
        }),
      ],
    });
    const card = wrapper.findComponent(UnifiedTimelineCard);
    expect(card.props("entry").expandDetail?.error).toBe("Unknown encoder libmp3lame");
  });

  it("ok 状态时 expandDetail.error 不存在", () => {
    const wrapper = mountCard({
      log: [makeEntry({ status: "ok" })],
    });
    const card = wrapper.findComponent(UnifiedTimelineCard);
    expect(card.props("entry").expandDetail?.error).toBeUndefined();
  });
});

describe("MockGenLogCard - runner 图标渲染", () => {
  it("runner=ffmpeg 时渲染 ion-icon + ffmpeg class", () => {
    const wrapper = mountCard({
      log: [makeEntry({ runner: "ffmpeg" })],
    });
    const runner = wrapper.find(".mock-gen-log-runner");
    expect(runner.exists()).toBe(true);
    expect(runner.find(".ion-icon-stub").exists()).toBe(true);
    expect(runner.classes()).toContain("mock-gen-log-runner--ffmpeg");
  });

  it("runner=mediacodec 时渲染 ion-icon + mediacodec class", () => {
    const wrapper = mountCard({
      log: [makeEntry({ runner: "mediacodec" })],
    });
    const runner = wrapper.find(".mock-gen-log-runner");
    expect(runner.find(".ion-icon-stub").exists()).toBe(true);
    expect(runner.classes()).toContain("mock-gen-log-runner--mediacodec");
  });

  it("runner=static 时渲染 ion-icon + static class", () => {
    const wrapper = mountCard({
      log: [makeEntry({ runner: "static" })],
    });
    const runner = wrapper.find(".mock-gen-log-runner");
    expect(runner.find(".ion-icon-stub").exists()).toBe(true);
    expect(runner.classes()).toContain("mock-gen-log-runner--static");
  });
});

describe("MockGenLogCard - meta slot 渲染", () => {
  it("meta slot 渲染 [index/total] + encoder", () => {
    const wrapper = mountCard({
      log: [makeEntry({ index: 2, total: 5, encoder: "libx264" })],
    });
    const meta = wrapper.find(".mock-gen-log-idx");
    expect(meta.exists()).toBe(true);
    expect(meta.text()).toBe("[2/5]");
    const encoder = wrapper.find(".mock-gen-log-encoder");
    expect(encoder.text()).toBe("libx264");
  });

  it("status=failed 时 meta slot 渲染 exit code", () => {
    const wrapper = mountCard({
      log: [makeEntry({ status: "failed", exitCode: 1 })],
    });
    const exitcode = wrapper.find(".mock-gen-log-exitcode");
    expect(exitcode.exists()).toBe(true);
    expect(exitcode.text()).toBe("exit=1");
  });

  it("status=ok 时 meta slot 不渲染 exit code", () => {
    const wrapper = mountCard({
      log: [makeEntry({ status: "ok", exitCode: 0 })],
    });
    expect(wrapper.find(".mock-gen-log-exitcode").exists()).toBe(false);
  });
});

describe("MockGenLogCard - detail slot 渲染", () => {
  it("展开时渲染 FFMPEG Args 卡片", async () => {
    const wrapper = mountCard({
      log: [
        makeEntry({
          expanded: true,
          ffmpegArgs: ["-i", "input.mp4", "-c:v", "libx264"],
        }),
      ],
    });
    const argsCard = wrapper.find(".mock-gen-log-detail-card");
    expect(argsCard.exists()).toBe(true);
    expect(wrapper.find(".mock-gen-log-detail-label").text()).toBe("FFMPEG Args");
    expect(wrapper.find(".mock-gen-log-detail-value").text()).toContain("-i input.mp4 -c:v libx264");
  });

  it("ffmpegArgs 为空时显示「静态字节」提示", async () => {
    const wrapper = mountCard({
      log: [
        makeEntry({
          expanded: true,
          ffmpegArgs: [],
          runner: "static",
        }),
      ],
    });
    expect(wrapper.find(".mock-gen-log-detail-value").text()).toContain("静态字节 - 无 ffmpeg 调用");
  });

  it("展开时渲染 STDERR 卡片（stderr 存在时）", async () => {
    const wrapper = mountCard({
      log: [
        makeEntry({
          expanded: true,
          stderr: "Unknown encoder libmp3lame",
        }),
      ],
    });
    const stderrCards = wrapper.findAll(".mock-gen-log-detail-card--stderr");
    expect(stderrCards.length).toBeGreaterThanOrEqual(1);
  });

  it("展开时渲染 Worker Tmp Dir 卡片（workerTmpDir 存在时）", async () => {
    const wrapper = mountCard({
      log: [
        makeEntry({
          expanded: true,
          workerTmpDir: "/tmp/ffmpeg-worker-123",
        }),
      ],
    });
    expect(wrapper.text()).toContain("/tmp/ffmpeg-worker-123");
  });

  it("展开时渲染 Worker Error 卡片（workerError 存在时）", async () => {
    const wrapper = mountCard({
      log: [
        makeEntry({
          expanded: true,
          workerError: "[ENGINE_LOAD_FAILED] dlopen failed",
        }),
      ],
    });
    const errorCard = wrapper.find(".mock-gen-log-detail-card--error");
    expect(errorCard.exists()).toBe(true);
    expect(errorCard.text()).toContain("[ENGINE_LOAD_FAILED] dlopen failed");
  });

  it("展开时渲染 File Sizes 卡片（srcSize/dstSize 存在时）", async () => {
    const wrapper = mountCard({
      log: [
        makeEntry({
          expanded: true,
          srcSize: 1024,
          dstSize: 2048,
        }),
      ],
    });
    expect(wrapper.text()).toContain("src=1024 bytes");
    expect(wrapper.text()).toContain("dst=2048 bytes");
  });

  it("展开时渲染 Context 卡片（contextInfo 存在时）", async () => {
    const wrapper = mountCard({
      log: [
        makeEntry({
          expanded: true,
          contextInfo: "worker_tmp_dir=/tmp lib_dir=/lib pid=123",
        }),
      ],
    });
    expect(wrapper.text()).toContain("worker_tmp_dir=/tmp lib_dir=/lib pid=123");
  });
});
