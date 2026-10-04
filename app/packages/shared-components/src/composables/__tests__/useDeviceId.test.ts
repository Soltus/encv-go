/**
 * useDeviceId.test.ts —— 落盘设备标识的**坏值自愈**（2026-10-05）
 *
 * 对应 docs/persisted-state-selfhealing.md 的 P1-1。
 *
 * 原实现 `if (stored)` 直接用 ⇒ localStorage 里一旦躺着垃圾值（"null"、
 * "[object Object]"、被别的版本写坏的残留），设备身份就被污染，且**重启无效**
 * （值不会自己变好）——这就是"持久化的坏值被无条件信任"的同一类问题。
 *
 * 契约：
 *   1. 坏值 ⇒ 不采用 + **清除** + 留痕；
 *   2. 合法值（含历史裸 UUID）**必须继续有效**，不误伤（反向锁）。
 */

import { beforeEach, describe, expect, it } from "vitest";
import { clearDeviceIdCache, getDeviceIdSync, isValidDeviceId } from "@encv/shared-components/composables/useDeviceId";

const KEY = "encv-device-id";

beforeEach(() => {
  clearDeviceIdCache();
  try {
    localStorage.clear();
  } catch {
    /* ignore */
  }
});

describe("isValidDeviceId — 判定", () => {
  it("合法值：native:/web: 前缀与历史裸 UUID 都算可用", () => {
    expect(isValidDeviceId("native:abc123xyz")).toBe(true);
    expect(isValidDeviceId("web:9f1c2b3e-1234")).toBe(true);
    expect(isValidDeviceId("a1b2c3d4e5f6a7b8")).toBe(true);
  });

  it("坏值：太短 / 含空白 / JSON 污染", () => {
    expect(isValidDeviceId("")).toBe(false);
    expect(isValidDeviceId("abc")).toBe(false);
    expect(isValidDeviceId("[object Object]")).toBe(false);
    expect(isValidDeviceId("{\"a\":1}")).toBe(false);
    expect(isValidDeviceId("null")).toBe(false);
    expect(isValidDeviceId("has space here")).toBe(false);
  });
});

describe("getDeviceIdSync — 坏值自愈", () => {
  it("落盘坏值 ⇒ 不采用，且被清除（否则下次启动还是它）", () => {
    localStorage.setItem(KEY, "[object Object]");

    expect(getDeviceIdSync()).toBe("");
    expect(localStorage.getItem(KEY)).toBeNull();
  });

  it("反向锁：落盘合法值 ⇒ 原样采用（trim 后），不得被新校验误伤", () => {
    localStorage.setItem(KEY, "native:device-0001");

    expect(getDeviceIdSync()).toBe("native:device-0001");
    expect(localStorage.getItem(KEY)).toBe("native:device-0001");
  });
});
