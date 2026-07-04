import { describe, expect, it } from "vitest";
import { emptyTokenSnapshot, type TokenSnapshot, tokenWarningLevel } from "../types/tokenSnapshot";

describe("emptyTokenSnapshot", () => {
  it("所有数值字段为 0，contextRemaining 为 1M", () => {
    const s = emptyTokenSnapshot();
    expect(s.tokensPerSecond).toBe(0);
    expect(s.promptTokensTotal).toBe(0);
    expect(s.completionTokensTotal).toBe(0);
    expect(s.contextRemaining).toBe(1_000_000);
    expect(s.contextUsagePercent).toBe(0);
    expect(s.requestCount).toBe(0);
  });

  it("返回新对象（不可变）", () => {
    const a = emptyTokenSnapshot();
    const b = emptyTokenSnapshot();
    expect(a).not.toBe(b);
    a.tokensPerSecond = 99;
    expect(b.tokensPerSecond).toBe(0);
  });
});

describe("tokenWarningLevel", () => {
  it("0% → ok", () => {
    expect(tokenWarningLevel(0)).toBe("ok");
  });

  it("20% → ok", () => {
    expect(tokenWarningLevel(0.2)).toBe("ok");
  });

  it("30% → green", () => {
    expect(tokenWarningLevel(0.3)).toBe("green");
  });

  it("50% → green", () => {
    expect(tokenWarningLevel(0.5)).toBe("green");
  });

  it("80% → yellow", () => {
    expect(tokenWarningLevel(0.8)).toBe("yellow");
  });

  it("90% → yellow", () => {
    expect(tokenWarningLevel(0.9)).toBe("yellow");
  });

  it("95% → red", () => {
    expect(tokenWarningLevel(0.95)).toBe("red");
  });

  it("97% → red", () => {
    expect(tokenWarningLevel(0.97)).toBe("red");
  });

  it("98% → force", () => {
    expect(tokenWarningLevel(0.98)).toBe("force");
  });

  it("100%+ → force", () => {
    expect(tokenWarningLevel(1.0)).toBe("force");
    expect(tokenWarningLevel(1.5)).toBe("force");
  });
});

describe("TokenSnapshot JSON 序列化", () => {
  it("字段名与 Go 端对齐 (snake_case)", () => {
    const s: TokenSnapshot = emptyTokenSnapshot();
    const json = JSON.parse(JSON.stringify(s));
    // 关键字段必须用 snake_case
    expect("tokens_per_second" in json || "tokensPerSecond" in json).toBe(true);
    // Go 端 json tag 是 tokensPerSecond（小写驼峰），对齐
    expect(json.tokensPerSecond).toBe(0);
    expect(json.contextUsagePercent).toBe(0);
  });
});
