import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

// 契约锁（2026-10-03，真实浏览器复现后固化）：
//   v-page-transition 曾用 from() 把「当前计算值」当终态 —— 挂在 <ion-page> 上时，
//   Ionic 转场开始前会给页面写内联 opacity:0，from() 采集到 0 ⇒ 动画 0→0，
//   y 正常归位但透明度永久卡 0 ⇒ **整页空白但可点击**（Files / AgentChat 在桌面壳下全白）。
//   契约：必须 fromTo 显式终态 opacity:1，且结束后 clearProps；绝不能再退回 from()。
//
// ⚠️ 为什么这里是源码扫描而不是 mock 引擎：
//   FAST 项目 isolate:false，模块级 mock 跨文件共享 —— 任何额外 import
//   `@encv/shared-components/motion/internal` 的测试文件都会让
//   directive-reveal.test.ts 的同模块 vi.mock 失效（实测两文件交替假红）。
//   功能级锁见 ISOLATED 的 page-transition-directive.test.ts（ENCV_TEST_FULL=1 跑）。

// vitest 下 import.meta.url 不是 file: scheme（会 ERR_INVALID_URL_SCHEME），
// 所以用 cwd（= encv-mobile 包根）+ 相对路径定位真源。
const SRC = resolve(process.cwd(), "../packages/shared-components/src/directives/motion.ts");

function vPageTransitionBlock(): string {
  const src = readFileSync(SRC, "utf8");
  const start = src.indexOf("export const vPageTransition");
  if (start < 0) throw new Error("directives/motion.ts 里找不到 vPageTransition");
  const end = src.indexOf("export const v", start + 10);
  return src.slice(start, end > 0 ? end : src.length);
}

describe("v-page-transition 终态契约（防「整页空白但可点击」复发）", () => {
  it("必须用 motion.fromTo 显式终态，不得用 motion.from", () => {
    const block = vPageTransitionBlock();
    expect(block).toContain("motion.fromTo(");
    expect(block).not.toMatch(/\bmotion\.from\(/);
  });

  it("终态 opacity 显式为 1（不依赖采集到的当前值）", () => {
    expect(vPageTransitionBlock()).toMatch(/opacity:\s*1/);
  });

  it("结束后 clearProps 释放内联 opacity/transform", () => {
    const block = vPageTransitionBlock();
    expect(block).toMatch(/clearProps:\s*"[^"]*opacity/);
    expect(block).toMatch(/clearProps:\s*"[^"]*transform/);
  });
});
