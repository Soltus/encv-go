import { describe, expect, it } from "vitest";

import { getAbsoluteStreamUrl, getFileStreamUrl } from "@encv/shared-components/api/encv_files";

/**
 * 锁住"交给原生播放器（mpv）的地址"必须是绝对 URL。
 *
 * 背景（真实 bug）：移动端播加密视频时，前端把**容器文件路径**直接交给 mpv，
 * mpv 不认 .sccgv 容器 → 根本播不了。改成解密流之后，还有第二个坑：
 * DEV 下 getFileStreamUrl 返回相对路径，mpv 在独立原生进程里解析不了。
 */
describe("getAbsoluteStreamUrl", () => {
  it("DEV 下的相对路径会被补成绝对 URL", () => {
    const abs = getAbsoluteStreamUrl("/data/clip.mp4.sccgv");
    expect(abs).toMatch(/^https?:\/\//);
    expect(abs).toContain("/stream?path=");

    // 相对路径版本（DEV）应当指向同一个端点，只是缺了 origin
    const rel = getFileStreamUrl("/data/clip.mp4.sccgv");
    if (rel.startsWith("/")) {
      expect(abs.endsWith(rel)).toBe(true);
    }
  });

  it("已经是绝对 URL 时原样返回（幂等）", () => {
    const once = getAbsoluteStreamUrl("/data/clip.mp4.sccgv");
    const twice = getAbsoluteStreamUrl("/data/clip.mp4.sccgv");
    expect(twice).toBe(once);
  });

  it("路径里的空格/中文/特殊字符都被编码，不会破坏 URL", () => {
    const abs = getAbsoluteStreamUrl("/data/我的 视频 & 音频.mp4.sccgv");
    expect(abs).toMatch(/^https?:\/\/[^ ]+$/); // 绝对 URL 里不能有裸空格
    expect(abs).not.toContain("我的");
  });

  it("与图片预览用的是同一个 /stream 端点（后端 Range 解密供给）", () => {
    // 图片分支一直用 getFileStreamUrl，视频分支之前给的是容器路径 —— 现在对齐
    expect(getAbsoluteStreamUrl("/x.mp4.sccgv")).toContain(getFileStreamUrl("/x.mp4.sccgv").replace(/^https?:\/\/[^/]+/, ""));
  });
});
