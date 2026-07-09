/**
 * relativeTime.test.ts
 *
 * 覆盖 5 档边界 + 临界值 + 防御性输入。
 * 注入 `now` 参数，避免依赖 Date.now() 的不确定性。
 */

import { formatRelativeTime } from "@encv/shared-components/composables/relativeTime";
import { describe, expect, it } from "vitest";

// 固定 now 时间戳：2026-06-08T22:00:00.000Z（UTC）
// 本地时区不影响相对时间计算（用毫秒差），但 >= 7d 的绝对日期分支
// 会用 new Date(ts) 走本地时区 → 这里用 ts 落在本地的某个 22:00 即可。
const NOW = new Date("2026-06-08T22:00:00").getTime();

describe("formatRelativeTime - 5 档边界值", () => {
  it("TestRelative_LessThan60s: 30 秒前 → 刚刚", () => {
    expect(formatRelativeTime(NOW - 30 * 1000, NOW)).toBe("刚刚");
  });

  it("TestRelative_LessThan60s_OneSecond: 1 秒前 → 刚刚", () => {
    expect(formatRelativeTime(NOW - 1 * 1000, NOW)).toBe("刚刚");
  });

  it("TestRelative_LessThan60s_FiftyNineSeconds: 59 秒前 → 刚刚（边界内）", () => {
    expect(formatRelativeTime(NOW - 59 * 1000, NOW)).toBe("刚刚");
  });

  it('TestRelative_OneMinute: 1 分钟前 → "1 分钟前"', () => {
    expect(formatRelativeTime(NOW - 60 * 1000, NOW)).toBe("1 分钟前");
  });

  it('TestRelative_FewMinutes: 5 分钟前 → "5 分钟前"', () => {
    expect(formatRelativeTime(NOW - 5 * 60 * 1000, NOW)).toBe("5 分钟前");
  });

  it('TestRelative_FiftyNineMinutes: 59 分钟前 → "59 分钟前"（边界内）', () => {
    expect(formatRelativeTime(NOW - 59 * 60 * 1000, NOW)).toBe("59 分钟前");
  });

  it('TestRelative_OneHour: 60 分钟前 → "1 小时前"', () => {
    expect(formatRelativeTime(NOW - 60 * 60 * 1000, NOW)).toBe("1 小时前");
  });

  it('TestRelative_FewHours: 5 小时前 → "5 小时前"', () => {
    expect(formatRelativeTime(NOW - 5 * 60 * 60 * 1000, NOW)).toBe("5 小时前");
  });

  it('TestRelative_TwentyThreeHours: 23 小时前 → "23 小时前"（边界内）', () => {
    expect(formatRelativeTime(NOW - 23 * 60 * 60 * 1000, NOW)).toBe("23 小时前");
  });

  it('TestRelative_OneDay: 24 小时前 → "1 天前"', () => {
    expect(formatRelativeTime(NOW - 24 * 60 * 60 * 1000, NOW)).toBe("1 天前");
  });

  it('TestRelative_FewDays: 3 天前 → "3 天前"', () => {
    expect(formatRelativeTime(NOW - 3 * 24 * 60 * 60 * 1000, NOW)).toBe("3 天前");
  });

  it('TestRelative_SixDays: 6 天前 → "6 天前"（边界内）', () => {
    expect(formatRelativeTime(NOW - 6 * 24 * 60 * 60 * 1000, NOW)).toBe("6 天前");
  });

  it("TestRelative_ExactlySevenDays: 7 天前 → 绝对日期 YYYY-MM-DD", () => {
    // 边界条件：abs === WEEK_MS 时不进入 < WEEK_MS 分支 → 走绝对日期
    const result = formatRelativeTime(NOW - 7 * 24 * 60 * 60 * 1000, NOW);
    // 7d 前 = 2026-06-01 22:00 本地 → 2026-06-01
    expect(result).toBe("2026-06-01");
  });

  it("TestRelative_TenDays: 10 天前 → 绝对日期", () => {
    const result = formatRelativeTime(NOW - 10 * 24 * 60 * 60 * 1000, NOW);
    expect(result).toBe("2026-05-29");
  });

  it("TestRelative_ManyDays: 100 天前 → 绝对日期", () => {
    const result = formatRelativeTime(NOW - 100 * 24 * 60 * 60 * 1000, NOW);
    // 2026-06-08 减 100 天 = 2026-02-28（2026 非闰年，2 月 28 天）
    expect(result).toBe("2026-02-28");
  });
});

describe("formatRelativeTime - 未来时间（diff < 0）", () => {
  it("TestRelative_Future30s: 30 秒后 → 刚刚（同样阈值）", () => {
    // abs(30s) = 30s < 60s → 刚刚
    expect(formatRelativeTime(NOW + 30 * 1000, NOW)).toBe("刚刚");
  });

  it('TestRelative_Future5min: 5 分钟后 → "5 分钟前"（按 abs 算）', () => {
    // 当前实现用 Math.abs(diff) → 未来时间按同样档位输出
    // 设计取舍：未来时间不会出现（消息时间戳一定是过去的），
    // 但防御性行为：和过去时间使用同一档位避免异常崩溃
    expect(formatRelativeTime(NOW + 5 * 60 * 1000, NOW)).toBe("5 分钟前");
  });
});

describe("formatRelativeTime - 防御性输入", () => {
  it("TestRelative_Zero: ts=0 → 空字符串", () => {
    expect(formatRelativeTime(0, NOW)).toBe("");
  });

  it("TestRelative_NaN: ts=NaN → 空字符串", () => {
    expect(formatRelativeTime(NaN, NOW)).toBe("");
  });

  it("TestRelative_InvalidNow: now=0 → 空字符串", () => {
    expect(formatRelativeTime(NOW, 0)).toBe("");
  });

  it("TestRelative_SameTimestamp: ts == now → 刚刚", () => {
    expect(formatRelativeTime(NOW, NOW)).toBe("刚刚");
  });
});

describe("formatRelativeTime - 默认 now 参数", () => {
  it("TestRelative_DefaultNow: 不传 now 时不抛错", () => {
    // 用刚刚的时间戳
    const ts = Date.now() - 5_000;
    const result = formatRelativeTime(ts);
    // 5s 前 < 60s → 刚刚
    expect(result).toBe("刚刚");
  });
});
