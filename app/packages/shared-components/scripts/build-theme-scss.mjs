#!/usr/bin/env node
/**
 * 主题 SCSS → CSS 产物编译（带 source map），供 codemogger css-source 溯源。
 *
 * Vite 8(rolldown) 的 build.cssSourcemap 在本环境不实际产出 .css.map，故这里用
 * sass-embedded 直接编译主题入口 scss 为「CSS 产物 + .css.map」，作为稳定的可溯源
 * 中间产物：codemogger css-source <产物.css> 即可由生成的 CSS 规则精确回到 .scss
 * 源（含 @mixin/@function/@each 生成的规则，它们从不在 scss 中以字面量出现）。
 *
 * 用法：node scripts/build-theme-scss.mjs
 * 产物：src/theme/.dist/<entry>.css  +  <entry>.css.map
 *   （.dist 已 gitignore；运行时仍由 Vite 直接编译 .scss 入口，此脚本仅用于溯源/校验）
 */
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

// sass-embedded 是 encv-mobile 的 devDependency（本工作区 pnpm 严格隔离，
// shared-components 自身未直接依赖），故从相邻 encv-mobile 的 node_modules 解析，
// 避免给 shared-components 强加依赖。
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const require = createRequire(import.meta.url);
function loadSass() {
  const candidates = [
    "sass-embedded",
    path.resolve(__dirname, "..", "..", "..", "encv-mobile", "node_modules", "sass-embedded"),
    path.resolve(__dirname, "..", "..", "node_modules", "sass-embedded"),
    "/workspace/app/node_modules/sass-embedded",
  ];
  for (const c of candidates) {
    try {
      return require(c);
    } catch {
      /* try next */
    }
  }
  return require("sass-embedded");
}
const sass = loadSass();

const THEME_DIR = path.resolve(__dirname, "..", "src", "theme");
const OUT_DIR = path.join(THEME_DIR, ".dist");

// 主题入口（scss）。新增入口在此追加即可。
const ENTRIES = ["surface.scss", "vivid.scss"];

fs.mkdirSync(OUT_DIR, { recursive: true });

for (const entry of ENTRIES) {
  const entryPath = path.join(THEME_DIR, entry);
  if (!fs.existsSync(entryPath)) {
    console.warn(`[skip] entry not found: ${entryPath}`);
    continue;
  }
  const outFile = path.join(OUT_DIR, entry.replace(/\.scss$/, ".css"));
  const result = sass.compile(entryPath, {
    loadPaths: [THEME_DIR],
    sourceMap: true,
    sourceMapIncludeSources: true,
    outFile,
    quietDeps: true,
    silenceDeprecations: ["import", "global-builtin", "color-functions", "slash-div"],
  });
  fs.writeFileSync(outFile, `${result.css}\n/*# sourceMappingURL=${path.basename(outFile)}.map */`);
  fs.writeFileSync(`${outFile}.map`, JSON.stringify(result.sourceMap));
  const nSrc = (result.sourceMap.sources || []).length;
  console.log(`[ok] ${entry} -> ${path.relative(process.cwd(), outFile)} (${result.css.length} bytes, ${nSrc} scss sources)`);
}
console.log("Done. Trace with: codemogger css-source <产物.css>");
