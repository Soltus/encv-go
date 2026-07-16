import { describe, it, expect } from "vitest";
import * as sass from "sass-embedded";
import fs from "node:fs";
import path from "node:path";

// 编译期契约（续 vivid.css → vivid.scss 重构 + codemogger css-source 溯源）：
//   - P3 宽色域孪生 --color-*-p3 由 _vivid-p3.scss 的 @function p3() + @each 自动派生，
//     不再手写 color(display-p3 ...) 字面量（消除与 palette.css 基色 hex 的漂移）。
//   - 语义与 useTheme.hexToP3Token 完全一致（naive 归一化），保证默认 encv 与
//     自定义主题在 P3 屏视觉统一。
//   - 任何经 @mixin/@each 生成的规则都能被 codemogger css-source 由产物溯源回 .scss 源
//     （此处用同源解码校验 sourcemap 的 line→source 归属，作为回归锁）。

function resolveEntry(): string {
  const candidates = [
    path.resolve(process.cwd(), "..", "packages", "shared-components", "src", "theme", "vivid.scss"),
    path.resolve(process.cwd(), "packages", "shared-components", "src", "theme", "vivid.scss"),
    path.resolve(__dirname, "..", "..", "..", "..", "packages", "shared-components", "src", "theme", "vivid.scss"),
  ];
  for (const c of candidates) if (fs.existsSync(c)) return c;
  return candidates[0]!;
}

const entry = resolveEntry();
const compiled = sass.compile(entry, {
  loadPaths: [path.dirname(entry)],
  sourceMap: true,
  quietDeps: true,
  silenceDeprecations: ["import", "global-builtin", "color-functions", "slash-div"],
});

// 紧凑 VLQ 解码：返回某生成行（0-based）首个带源段的 { srcIdx, origLine }。
function sourceOfLine(map: any, genLine: number): { srcIdx: number; origLine: number } | null {
  const B64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
  const V = new Map([...B64].map((c, i) => [c, i]));
  const decode = (s: string) => {
    const out: number[] = [];
    let shift = 0,
      value = 0;
    for (const ch of s) {
      const v = V.get(ch)!;
      const cont = v & 32;
      value += (v & 31) << shift;
      shift += 5;
      if (!cont) {
        out.push(value & 1 ? -(value >> 1) : value >> 1);
        value = 0;
        shift = 0;
      }
    }
    return out;
  };
  const lines = map.mappings.split(";");
  let prevSrc = 0,
    prevOrigLine = 0;
  let genCol = 0;
  for (let gi = 0; gi <= genLine; gi++) {
    const line = lines[gi] ?? "";
    genCol = 0;
    if (!line.length) continue;
    for (const seg of line.split(",")) {
      if (!seg) continue;
      const f = decode(seg);
      genCol += f[0];
      if (f.length >= 4) {
        prevSrc += f[1];
        prevOrigLine += f[2];
        if (gi === genLine) return { srcIdx: prevSrc, origLine: prevOrigLine };
      }
    }
  }
  return null;
}

function lineOf(css: string, needle: string): number {
  const i = css.split("\n").findIndex(l => l.includes(needle));
  expect(i, `compiled CSS should contain ${needle}`).toBeGreaterThanOrEqual(0);
  return i;
}

describe("vivid.scss — P3 孪生由 @function/@each 编译期派生（溯源友好）", () => {
  it("primary/secondary/accent 的 -p3 孪生均被生成，且值匹配 hexToP3Token(naive)", () => {
    // #8b5cf6 -> (139,92,246)/255 = 0.5451 0.3608 0.9647
    expect(compiled.css).toContain("--color-primary-p3: color(display-p3 0.5451 0.3608 0.9647)");
    // #06b6d4 -> (6,182,212)/255 = 0.0235 0.7137 0.8314
    expect(compiled.css).toContain("--color-secondary-p3: color(display-p3 0.0235 0.7137 0.8314)");
    // #ec4899 -> (236,72,153)/255 = 0.9255 0.2824 0.6000
    expect(compiled.css).toContain("--color-accent-p3: color(display-p3 0.9255 0.2824 0.6)");
  });

  it("palette.css 里的手写 -p3 字面量已移除（单一事实源迁到 SCSS，无漂移）", () => {
    const palette = fs.readFileSync(path.resolve(path.dirname(entry), "palette.css"), "utf8");
    expect(palette).not.toContain("--color-secondary-p3: color(display-p3");
    expect(palette).not.toContain("--color-accent-p3: color(display-p3");
  });

  it("生成的 -p3 规则可经 sourcemap 精确溯源到 _vivid-p3.scss（css-source 同源校验）", () => {
    const map = compiled.sourceMap;
    expect(map, "sass should emit a source map").toBeTruthy();
    const gi = lineOf(compiled.css, "--color-secondary-p3:");
    const hit = sourceOfLine(map, gi);
    expect(hit, "secondary-p3 line should have a source mapping").not.toBeNull();
    const src = map!.sources[hit!.srcIdx];
    expect(src, `secondary-p3 should trace to _vivid-p3.scss, got ${src}`).toContain("_vivid-p3.scss");
  });
});

describe("vivid.scss — 臻彩显示真实生效修复（2026-07-17，先红后绿）", () => {
  it('滤镜选择器同时匹配 .ion-page 类（CE 模式页面是 <div class="ion-page">，纯 ion-page TAG 命中不到 → 实测滤镜恒 none）', () => {
    // 修复前 compiled.css 只有 `.encv-vivid ion-page`，div.ion-page 永远不命中；
    // 修复后必须含 `.encv-vivid .ion-page`（类选择器）。
    expect(compiled.css).toContain(".encv-vivid .ion-page");
  });

  it("P3 交换带 !important 且 --color-primary 回退非循环 --color-primary-srgb（越过 applyColor 内联覆盖）", () => {
    // 修复前：@media 块无 !important，被 applyColor 写在内联的 --color-primary 压过 → P3 恒不生效；
    // 且回退写 var(--color-primary) 自引用（无效声明）。修复后：加 !important + 回退 --color-primary-srgb。
    expect(compiled.css).toContain("!important");
    expect(compiled.css).toContain("--color-primary-srgb");
    const p3Block = compiled.css.split("@media (color-gamut: p3)")[1] ?? "";
    expect(p3Block, "P3 块不应再自引用 var(--color-primary)（会导致无效声明丢失令牌）").not.toContain("var(--color-primary)");
  });
});

describe("vivid.scss — 滤镜拆分 + 明暗分调（2026-07-17 优化）", () => {
  it("滤镜拆分为色彩浓度(saturate) / 对比度(contrast) 两条独立变量", () => {
    expect(compiled.css).toContain("--encv-vivid-sat");
    expect(compiled.css).toContain("--encv-vivid-contrast");
    expect(compiled.css).toMatch(/saturate\(calc\(1 \+ var\(--encv-vivid-sat/);
    expect(compiled.css).toMatch(/contrast\(calc\(1 \+ var\(--encv-vivid-contrast/);
  });

  it("明暗场景分别调优：暗色(body.dark)对比度增益收敛、浓度增益加大", () => {
    // 亮色规则：saturate*0.45、contrast*0.25
    expect(compiled.css).toContain("0.45");
    expect(compiled.css).toContain("0.25");
    // 暗色专属规则（:root.encv-vivid body.dark ...）
    const darkIdx = compiled.css.indexOf("body.dark");
    expect(darkIdx, "应存在 :root.encv-vivid body.dark 暗色专属滤镜规则").toBeGreaterThan(-1);
    const darkBlock = compiled.css.slice(darkIdx);
    // 暗色 saturate*0.72（> 亮色 0.45）、contrast*0.12（< 亮色 0.25）
    expect(darkBlock).toContain("0.72");
    expect(darkBlock).toContain("0.12");
  });
});
