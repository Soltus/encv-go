// 一次性脚本：把各主题的调色板（palette）镜像写入所有 theme.json
// （public/themes、dist/themes、android 打包副本），与 useUserThemes.ts 的 TS 声明保持忠实一致。
// 运行：node scripts/patch-theme-palettes.mjs
import { readFileSync, writeFileSync, existsSync } from "node:fs";
import { join, resolve } from "node:path";

const PALETTES = {
  encv: { primary: "#8b5cf6", bg: "#ffffff", presets: { primary: ["#8b5cf6", "#06b6d4", "#ec4899", "#6366f1", "#0ea5e9", "#a855f7"], bg: ["#ffffff", "#f3f4f6", "#faf5ff", "#eef2ff", "#f5f3ff"] } },
  "encv-dark": { primary: "#9c6df7", bg: "#0f172a", presets: { primary: ["#9c6df7", "#1fbdd8", "#ee5aa3", "#818cf8", "#38bdf8", "#c084fc"], bg: ["#0f172a", "#1e293b", "#0b1120", "#171923", "#121218"] } },
  rose: { primary: "#e11d48", bg: "#fffafb", presets: { primary: ["#e11d48", "#0ea5e9", "#db2777", "#f43f5e", "#be123c", "#fb7185"], bg: ["#fffafb", "#ffeef2", "#fff1f2", "#fce7ef", "#fff0f5"] } },
  ocean: { primary: "#22d3ee", bg: "#0b1120", presets: { primary: ["#22d3ee", "#2dd4bf", "#818cf8", "#38bdf8", "#0ea5e9", "#34d399"], bg: ["#0b1120", "#111c33", "#0a0e14", "#0f172a", "#101a2e"] } },
  forest: { primary: "#16a34a", bg: "#f3faf5", presets: { primary: ["#16a34a", "#0d9488", "#65a30d", "#22c55e", "#10b981", "#84cc16"], bg: ["#f3faf5", "#dcfce7", "#ecfdf5", "#f0fdf4", "#effaf1"] } },
  midnight: { primary: "#818cf8", bg: "#0a0a14", presets: { primary: ["#818cf8", "#a78bfa", "#c084fc", "#6366f1", "#8b5cf6", "#a855f7"], bg: ["#0a0a14", "#13131f", "#0a0e14", "#121218", "#1a1a2e"] } },
  sunset: { primary: "#f97316", bg: "#fffaf5", presets: { primary: ["#f97316", "#fb923c", "#f43f5e", "#f59e0b", "#ea580c", "#fb7185"], bg: ["#fffaf5", "#fdeede", "#fff7ed", "#ffedd5", "#fff1e6"] } },
  mint: { primary: "#10b981", bg: "#f3faf7", presets: { primary: ["#10b981", "#06b6d4", "#14b8a6", "#22c55e", "#0ea5e9", "#2dd4bf"], bg: ["#f3faf7", "#d9f2ea", "#ecfdf5", "#effaf7", "#e6f7f1"] } },
  kitty: { primary: "#ff5fa2", bg: "#fff5fa", presets: { primary: ["#ff5fa2", "#ffb3d1", "#ff8fab", "#fb7185", "#f472b6", "#f9a8d4"], bg: ["#fff5fa", "#ffe6f0", "#fff0f6", "#ffeef5", "#fff1f7"] } },
  neon: { primary: "#39ff14", bg: "#0a0e14", presets: { primary: ["#39ff14", "#00e5ff", "#ff2bd6", "#38bdf8", "#a3e635", "#22d3ee"], bg: ["#0a0e14", "#121823", "#0b1120", "#0d1117", "#0a0e14"] } },
  paper: { primary: "#a0785a", bg: "#f7f1e6", presets: { primary: ["#a0785a", "#7d9b76", "#c08552", "#b08968", "#8a7a66", "#9c7a52"], bg: ["#f7f1e6", "#efe6d6", "#faf6ee", "#f3ebdc", "#f9f3ea"] } },
};

const ROOTS = [
  resolve(process.cwd(), "encv-mobile/public/themes"),
  resolve(process.cwd(), "encv-mobile/dist/themes"),
  resolve(process.cwd(), "encv-mobile/android/app/src/main/assets/public/themes"),
];

let touched = 0;
for (const root of ROOTS) {
  if (!existsSync(root)) continue;
  for (const id of Object.keys(PALETTES)) {
    const file = join(root, id, "theme.json");
    if (!existsSync(file)) continue;
    const json = JSON.parse(readFileSync(file, "utf8"));
    json.palette = PALETTES[id];
    writeFileSync(file, JSON.stringify(json, null, 2) + "\n");
    touched++;
  }
}
console.log(`patched ${touched} theme.json files`);
