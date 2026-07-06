import { describe, expect, it } from "vitest";
import { i18nKeyFor, normalizeReasoningEffort } from "./reasoningEffort";

describe("normalizeReasoningEffort", () => {
  describe("5 个核心 bucket 直接输入", () => {
    it.each([
      ["low", "low"],
      ["medium", "medium"],
      ["high", "high"],
      ["xhigh", "xhigh"],
      ["default", "default"],
    ])("%s → %s", (input, expected) => {
      expect(normalizeReasoningEffort(input)).toBe(expected);
    });
  });

  describe("6 个值 × normalize × i18n（spec 主覆盖）", () => {
    // 6 = 5 归一化 bucket + 1 未知兜底
    // 每个值同时验证 normalize 输出 + 对应 i18n 键名
    it("low: normalize + i18n", () => {
      const v = normalizeReasoningEffort("low");
      expect(v).toBe("low");
      expect(i18nKeyFor(v)).toBe("agent.reasoningEffort.low");
    });

    it("medium: normalize + i18n", () => {
      const v = normalizeReasoningEffort("medium");
      expect(v).toBe("medium");
      expect(i18nKeyFor(v)).toBe("agent.reasoningEffort.medium");
    });

    it("high: normalize + i18n", () => {
      const v = normalizeReasoningEffort("high");
      expect(v).toBe("high");
      expect(i18nKeyFor(v)).toBe("agent.reasoningEffort.high");
    });

    it("xhigh: normalize + i18n", () => {
      const v = normalizeReasoningEffort("xhigh");
      expect(v).toBe("xhigh");
      expect(i18nKeyFor(v)).toBe("agent.reasoningEffort.xhigh");
    });

    it("default: normalize + i18n", () => {
      const v = normalizeReasoningEffort("default");
      expect(v).toBe("default");
      expect(i18nKeyFor(v)).toBe("agent.reasoningEffort.default");
    });

    it("unknown 字符串: normalize + i18n", () => {
      const v = normalizeReasoningEffort("something-else");
      expect(v).toBe("default");
      expect(i18nKeyFor(v)).toBe("agent.reasoningEffort.default");
    });
  });

  describe("low 集合别名", () => {
    it.each(["low", "LOW", "Low", "minimal", "MINIMAL", "small", "Small"])('%s → "low"', input => {
      expect(normalizeReasoningEffort(input)).toBe("low");
    });
  });

  describe("medium 集合别名", () => {
    it.each(["medium", "MEDIUM", "med", "Med", "normal", "Normal"])('%s → "medium"', input => {
      expect(normalizeReasoningEffort(input)).toBe("medium");
    });
  });

  describe("high 集合别名", () => {
    it.each(["high", "HIGH", "High", "large", "LARGE", "Large"])('%s → "high"', input => {
      expect(normalizeReasoningEffort(input)).toBe("high");
    });
  });

  describe("xhigh 集合别名", () => {
    it.each([
      "xhigh",
      "XHIGH",
      "xHigh",
      "extra",
      "EXTRA",
      "Extra",
      "extra max",
      "extra-max",
      "extra_max",
      "Extra Max",
      "max",
      "MAX",
    ])('%s → "xhigh"', input => {
      expect(normalizeReasoningEffort(input)).toBe("xhigh");
    });
  });

  describe("分隔符处理", () => {
    it("空格被吃", () => {
      expect(normalizeReasoningEffort("extra max")).toBe("xhigh");
      expect(normalizeReasoningEffort("  extra  max  ")).toBe("xhigh");
    });

    it("下划线被吃", () => {
      expect(normalizeReasoningEffort("extra_max")).toBe("xhigh");
      expect(normalizeReasoningEffort("x_high")).toBe("xhigh");
    });

    it("连字符被吃", () => {
      expect(normalizeReasoningEffort("extra-max")).toBe("xhigh");
      expect(normalizeReasoningEffort("x-high")).toBe("xhigh");
    });

    it("混合分隔符", () => {
      expect(normalizeReasoningEffort("  extra _- max  ")).toBe("xhigh");
    });

    it("大小写 + 分隔符组合", () => {
      expect(normalizeReasoningEffort("EXTRA-MAX")).toBe("xhigh");
      expect(normalizeReasoningEffort("Extra_Max")).toBe("xhigh");
    });
  });

  describe("边界输入（永抛错 = false）", () => {
    it('null → "default"', () => {
      expect(normalizeReasoningEffort(null)).toBe("default");
    });

    it('undefined → "default"', () => {
      expect(normalizeReasoningEffort(undefined)).toBe("default");
    });

    it('空字符串 → "default"', () => {
      expect(normalizeReasoningEffort("")).toBe("default");
    });

    it('数字 → "default"', () => {
      expect(normalizeReasoningEffort(0)).toBe("default");
      expect(normalizeReasoningEffort(42)).toBe("default");
      expect(normalizeReasoningEffort(1.5)).toBe("default");
    });

    it('布尔 → "default"', () => {
      expect(normalizeReasoningEffort(true)).toBe("default");
      expect(normalizeReasoningEffort(false)).toBe("default");
    });

    it('对象 → "default"（不会把 type/status 字段当 effort）', () => {
      expect(normalizeReasoningEffort({ type: "low" })).toBe("default");
      expect(normalizeReasoningEffort({ effort: "high" })).toBe("default");
    });

    it('数组 → "default"', () => {
      expect(normalizeReasoningEffort(["low"])).toBe("default");
    });

    it('未知字符串 → "default"', () => {
      expect(normalizeReasoningEffort("idle")).toBe("default");
      expect(normalizeReasoningEffort("auto")).toBe("default");
      expect(normalizeReasoningEffort("thinking")).toBe("default");
    });
  });

  describe("类型契约", () => {
    it("返回值始终是 string（不会抛错）", () => {
      const values: unknown[] = [
        "low",
        "medium",
        "high",
        "xhigh",
        "default",
        "minimal",
        "med",
        "large",
        "extra",
        "max",
        "LOW",
        "Extra Max",
        "extra-max",
        "extra_max",
        "",
        null,
        undefined,
        0,
        1,
        true,
        false,
        [],
        {},
        "unknown",
        "idle",
        "thinking",
      ];
      for (const v of values) {
        const r = normalizeReasoningEffort(v);
        expect(typeof r).toBe("string");
        expect(["low", "medium", "high", "xhigh", "default"]).toContain(r);
      }
    });
  });
});

describe("i18nKeyFor", () => {
  describe("5 个核心 bucket", () => {
    it.each([
      ["low", "agent.reasoningEffort.low"],
      ["medium", "agent.reasoningEffort.medium"],
      ["high", "agent.reasoningEffort.high"],
      ["xhigh", "agent.reasoningEffort.xhigh"],
      ["default", "agent.reasoningEffort.default"],
    ])("%s → %s", (input, expected) => {
      expect(i18nKeyFor(input)).toBe(expected);
    });
  });

  describe("透传任意字符串", () => {
    it("任意字符串都拼到 agent.reasoningEffort.*", () => {
      expect(i18nKeyFor("custom-bucket")).toBe("agent.reasoningEffort.custom-bucket");
      expect(i18nKeyFor("whatever")).toBe("agent.reasoningEffort.whatever");
    });

    it("空字符串也能用（虽然不推荐）", () => {
      expect(i18nKeyFor("")).toBe("agent.reasoningEffort.");
    });
  });

  describe("与 normalize 联用", () => {
    it("normalize → i18nKeyFor 链路对所有别名都正确", () => {
      const cases: Array<[unknown, string]> = [
        ["low", "agent.reasoningEffort.low"],
        ["LOW", "agent.reasoningEffort.low"],
        ["minimal", "agent.reasoningEffort.low"],
        ["medium", "agent.reasoningEffort.medium"],
        ["med", "agent.reasoningEffort.medium"],
        ["high", "agent.reasoningEffort.high"],
        ["large", "agent.reasoningEffort.high"],
        ["xhigh", "agent.reasoningEffort.xhigh"],
        ["extra", "agent.reasoningEffort.xhigh"],
        ["extra max", "agent.reasoningEffort.xhigh"],
        ["extra-max", "agent.reasoningEffort.xhigh"],
        ["max", "agent.reasoningEffort.xhigh"],
        ["default", "agent.reasoningEffort.default"],
        ["something-unknown", "agent.reasoningEffort.default"],
        ["", "agent.reasoningEffort.default"],
        [null, "agent.reasoningEffort.default"],
        [undefined, "agent.reasoningEffort.default"],
        [42, "agent.reasoningEffort.default"],
      ];
      for (const [input, expected] of cases) {
        expect(i18nKeyFor(normalizeReasoningEffort(input))).toBe(expected);
      }
    });
  });
});
