/**
 * TaskDebugPanel 单元测试
 *
 * 覆盖"任务逃逸"真机诊断 UI 的关键派生：
 * - realGroupCount / fakeGroupCount / escapeTaskCount 派生正确
 * - diagnostics 自动跑出关键告警（无逃逸 / 有逃逸 / store 空）
 * - 时间桶分布（today/yesterday/thisWeek/thisMonth/earlier）
 * - sortedGroups 按"真 group 在前 + 伪 group 在后 + 数量降序"排
 *
 * 2026-06-22 新增：嵌在 Tasks 页面顶部，URL ?debug=tasks 启用，
 * 让 user 在真机屏幕上一眼看到逃逸 task 数 + 各 runId 聚合。
 */

import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import type { EncvTask, TaskStatus } from "@encv/shared-components/api/encv";
import TaskDebugPanel from "@encv/shared-components/components/TaskDebugPanel.vue";

function makeTask(
  id: string,
  opts: {
    runId?: string;
    triggeredBy?: "user" | "automation" | "ai_agent";
    status?: TaskStatus;
    createdAt?: string;
    pluginName?: string;
  } = {}
): EncvTask {
  return {
    id,
    type: "encrypt",
    sourcePath: `/mock/${id}.mp4`,
    status: (opts.status ?? "running") as TaskStatus,
    progress: 50,
    createdAt: opts.createdAt ?? new Date().toISOString(),
    runId: opts.runId,
    triggeredBy: opts.triggeredBy,
    pluginName: opts.pluginName,
  } as EncvTask;
}

function makeProps(
  overrides: Partial<{
    tasks: EncvTask[];
    displayedItems: any[];
    groupedTasksByRunId: Array<{ key: string; runId: string; tasks: EncvTask[]; startedAt: string }>;
    viewMode: string;
    sortBy: string;
    searchQuery: string;
    filterPlugins: string[];
    filterTypes: string[];
    filterStatuses: string[];
    filterTriggeredBy: string[];
    filterDatePreset: string;
    pinnedRunIds: Set<string>;
    defaultOpen: boolean;
  }> = {}
) {
  return {
    tasks: overrides.tasks ?? [],
    displayedItems: overrides.displayedItems ?? [],
    groupedTasksByRunId: overrides.groupedTasksByRunId ?? [],
    viewMode: overrides.viewMode ?? "group",
    sortBy: overrides.sortBy ?? "activity",
    searchQuery: overrides.searchQuery ?? "",
    filterPlugins: overrides.filterPlugins ?? [],
    filterTypes: overrides.filterTypes ?? [],
    filterStatuses: overrides.filterStatuses ?? [],
    filterTriggeredBy: overrides.filterTriggeredBy ?? [],
    filterDatePreset: overrides.filterDatePreset ?? "all",
    pinnedRunIds: overrides.pinnedRunIds ?? new Set<string>(),
    defaultOpen: overrides.defaultOpen ?? true,
  };
}

describe("TaskDebugPanel — 任务逃逸真机诊断 UI", () => {
  it("① 默认折叠：<details> 默认收起，summary 可见", () => {
    const wrapper = mount(TaskDebugPanel, { props: makeProps({ defaultOpen: false }) });
    const panel = wrapper.find("details");
    expect(panel.exists()).toBe(true);
    expect(panel.attributes("open")).toBeUndefined();
    expect(wrapper.find("summary").exists()).toBe(true);
  });

  it("② 默认展开：defaultOpen=true 时所有 section 可见", () => {
    const wrapper = mount(TaskDebugPanel, { props: makeProps({ defaultOpen: true }) });
    expect(wrapper.findAll("section")).toHaveLength(5); // ①-⑤
  });

  it("③ 真 group 计数：3 个真 runId → realGroupCount=3，fakeGroupCount=0", () => {
    const tasks = [
      makeTask("t-1", { runId: "r-1" }),
      makeTask("t-2", { runId: "r-1" }),
      makeTask("t-3", { runId: "r-2" }),
      makeTask("t-4", { runId: "r-3" }),
    ];
    const grouped = [
      { key: "r-1", runId: "r-1", tasks: tasks.slice(0, 2), startedAt: "2026-06-22T10:00:00Z" },
      { key: "r-2", runId: "r-2", tasks: [tasks[2]], startedAt: "2026-06-22T11:00:00Z" },
      { key: "r-3", runId: "r-3", tasks: [tasks[3]], startedAt: "2026-06-22T12:00:00Z" },
    ];
    const wrapper = mount(TaskDebugPanel, {
      props: makeProps({ tasks, groupedTasksByRunId: grouped, defaultOpen: true }),
    });
    expect(wrapper.text()).toContain("真 group: 3");
    expect(wrapper.text()).toContain("伪 group (__manual__): 0");
  });

  it("④ 伪 group 计数：2 个 __manual__ 伪 group → fakeGroupCount=2，escapeTaskCount=3", () => {
    const tasks = [makeTask("t-1"), makeTask("t-2"), makeTask("t-3")];
    const grouped = [
      { key: "__manual__t-1", runId: "__manual__t-1", tasks: [tasks[0]], startedAt: "2026-06-22T10:00:00Z" },
      { key: "__manual__t-2", runId: "__manual__t-2", tasks: [tasks[1]], startedAt: "2026-06-22T10:01:00Z" },
      { key: "__manual__t-3", runId: "__manual__t-3", tasks: [tasks[2]], startedAt: "2026-06-22T10:02:00Z" },
    ];
    const wrapper = mount(TaskDebugPanel, {
      props: makeProps({ tasks, groupedTasksByRunId: grouped, defaultOpen: true }),
    });
    // 3 个伪 group（每个 1 个 task）→ fakeGroupCount=3, escapeTaskCount=3
    const text = wrapper.text();
    expect(text).toMatch(/伪 group \(__manual__\): 3/);
    expect(text).toContain("逃逸 task 数: 3");
  });

  it("⑤ 自我诊断 — 有逃逸 → error 级别 + 显示真因文案", () => {
    const tasks = [makeTask("t-1")];
    const grouped = [{ key: "__manual__t-1", runId: "__manual__t-1", tasks: [tasks[0]], startedAt: "2026-06-22T10:00:00Z" }];
    const wrapper = mount(TaskDebugPanel, {
      props: makeProps({ tasks, groupedTasksByRunId: grouped, defaultOpen: true }),
    });
    const text = wrapper.text();
    expect(text).toContain("error");
    expect(text).toContain("__manual__ 伪 group");
    expect(text).toContain("fetchTasks");
    expect(text).toContain("merge 模式");
  });

  it("⑥ 自我诊断 — 无逃逸 → ok 级别", () => {
    const tasks = [makeTask("t-1", { runId: "r-1" })];
    const grouped = [{ key: "r-1", runId: "r-1", tasks: [tasks[0]], startedAt: "2026-06-22T10:00:00Z" }];
    const wrapper = mount(TaskDebugPanel, {
      props: makeProps({ tasks, groupedTasksByRunId: grouped, defaultOpen: true }),
    });
    expect(wrapper.text()).toContain("ok");
    expect(wrapper.text()).toContain("无逃逸 task");
    expect(wrapper.text()).toContain("merge 模式生效");
  });

  it("⑦ 自我诊断 — store 为空 → info 级别", () => {
    const wrapper = mount(TaskDebugPanel, { props: makeProps({ tasks: [], defaultOpen: true }) });
    expect(wrapper.text()).toContain("info");
    expect(wrapper.text()).toContain("store.tasks 是空");
  });

  it("⑧ 视图状态：viewMode / sortBy / 各 filter / pin 全部展示", () => {
    const wrapper = mount(TaskDebugPanel, {
      props: makeProps({
        viewMode: "flat",
        sortBy: "created",
        searchQuery: "test",
        filterPlugins: ["video", "audio"],
        filterTypes: ["encrypt"],
        filterStatuses: ["running"],
        filterTriggeredBy: ["automation"],
        filterDatePreset: "7d",
        pinnedRunIds: new Set(["r-1", "r-2"]),
        defaultOpen: true,
      }),
    });
    const text = wrapper.text();
    expect(text).toContain("viewMode: flat");
    expect(text).toContain("sortBy: created");
    expect(text).toContain('search: "test"');
    expect(text).toContain("[video, audio]");
    expect(text).toContain("[encrypt]");
    expect(text).toContain("[running]");
    expect(text).toContain("[automation]");
    expect(text).toContain("datePreset: 7d");
    expect(text).toContain("pinned: [r-1, r-2]");
  });

  it("⑨ runId 聚合：3 个真 group + 1 个伪 group → sortedGroups 排前 3 真，1 伪在后", () => {
    const tasks = [
      makeTask("t-1", { runId: "r-1" }),
      makeTask("t-2", { runId: "r-1" }),
      makeTask("t-3", { runId: "r-2" }),
      makeTask("t-4"),
    ];
    const grouped = [
      { key: "r-1", runId: "r-1", tasks: tasks.slice(0, 2), startedAt: "2026-06-22T10:00:00Z" },
      { key: "r-2", runId: "r-2", tasks: [tasks[2]], startedAt: "2026-06-22T10:01:00Z" },
      { key: "__manual__t-4", runId: "__manual__t-4", tasks: [tasks[3]], startedAt: "2026-06-22T10:02:00Z" },
    ];
    const wrapper = mount(TaskDebugPanel, {
      props: makeProps({ tasks, groupedTasksByRunId: grouped, defaultOpen: true }),
    });
    const runItems = wrapper.findAll(".taskDebugRunItem");
    expect(runItems).toHaveLength(3);
    // 真 group 在前：r-1 (2 task) > r-2 (1 task) > 伪 __manual__t-4
    expect(runItems[0].text()).toContain("r-1");
    expect(runItems[0].text()).toContain("2 task");
    expect(runItems[1].text()).toContain("r-2");
    // 伪 group 在最后
    expect(runItems[2].classes()).toContain("taskDebugRunItem_fake");
    expect(runItems[2].text()).toContain("__manual__t-4");
    expect(runItems[2].text()).toContain("__manual__ 伪 group");
  });

  it("⑩ 时间桶分布：5 个 task 跨 5 个时间桶 → 各 1", () => {
    // 用固定时间 mock，避免月初/月末导致测试不稳定
    // 选择月中（15号）作为"今天"，确保 5 个时间桶各有 1 个
    const fakeNow = new Date(2026, 5, 15, 12, 0, 0); // 2026-06-15 中午
    vi.useFakeTimers();
    vi.setSystemTime(fakeNow);

    try {
      const now = fakeNow.getTime();
      const tasks = [
        makeTask("t-1", { createdAt: new Date(now - 3600 * 1000).toISOString() }), // 1小时前 → today
        makeTask("t-2", { createdAt: new Date(now - 86400 * 1000).toISOString() }), // 1天前 → yesterday
        makeTask("t-3", { createdAt: new Date(now - 3 * 86400 * 1000).toISOString() }), // 3天前 → thisWeek
        makeTask("t-4", { createdAt: new Date(now - 10 * 86400 * 1000).toISOString() }), // 10天前 → thisMonth
        makeTask("t-5", { createdAt: new Date(now - 40 * 86400 * 1000).toISOString() }), // 40天前 → earlier
      ];
      const wrapper = mount(TaskDebugPanel, {
        props: makeProps({ tasks, defaultOpen: true }),
      });
      const text = wrapper.text();
      expect(text).toMatch(/today: 1/);
      expect(text).toMatch(/yesterday: 1/);
      expect(text).toMatch(/thisWeek: 1/);
      expect(text).toMatch(/thisMonth: 1/);
      expect(text).toMatch(/earlier: 1/);
    } finally {
      vi.useRealTimers();
    }
  });

  it("⑪ 自我诊断 — 缺 runId task 数 > 0 → warn 级别", () => {
    const tasks = [
      makeTask("t-1", { runId: "r-1" }),
      makeTask("t-2"), // 无 runId
    ];
    const wrapper = mount(TaskDebugPanel, { props: makeProps({ tasks, defaultOpen: true }) });
    const text = wrapper.text();
    expect(text).toContain("warn");
    expect(text).toContain("没有 runId");
  });

  it("⑫ displayedItems 拆解：date=1 + group=3 + task=2 → info 拆解", () => {
    const tasks = [makeTask("t-1", { runId: "r-1" })];
    const displayedItems = [
      { kind: "date", key: "date-today", label: "today" },
      { kind: "group", key: "r-1", runId: "r-1", tasks: [tasks[0]] },
      { kind: "group", key: "r-2", runId: "r-2", tasks: [] },
      { kind: "group", key: "r-3", runId: "r-3", tasks: [] },
      { kind: "task", key: "t-1", task: tasks[0] },
      { kind: "task", key: "t-2", task: tasks[0] },
    ];
    const wrapper = mount(TaskDebugPanel, {
      props: makeProps({ tasks, displayedItems, defaultOpen: true }),
    });
    expect(wrapper.text()).toContain("date=1 / group=3 / task=2");
  });

  it("⑬ pinned 标记：pinnedRunIds 里的 run 显示 📌 pin 标签", () => {
    const tasks = [makeTask("t-1", { runId: "r-1" })];
    const grouped = [{ key: "r-1", runId: "r-1", tasks: [tasks[0]], startedAt: "2026-06-22T10:00:00Z" }];
    const wrapper = mount(TaskDebugPanel, {
      props: makeProps({ tasks, groupedTasksByRunId: grouped, pinnedRunIds: new Set(["r-1"]), defaultOpen: true }),
    });
    expect(wrapper.text()).toContain("📌 pin");
  });
});
