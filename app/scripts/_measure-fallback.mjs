import { readFileSync, existsSync, readdirSync, statSync } from "node:fs";
import { resolve, relative, join, extname } from "node:path";

const SRC = resolve(import.meta.dirname, "../encv-mobile/src");
const SHARED = resolve(import.meta.dirname, "../packages/shared-components/src");
const EXTS = ["ts", "tsx", "vue", "js", "mjs", "cjs"];

function walk(dir) {
  const out = [];
  for (const e of readdirSync(dir)) {
    if (e === "node_modules" || e === ".git") continue;
    const p = join(dir, e);
    const s = statSync(p);
    if (s.isDirectory()) out.push(...walk(p));
    else if (/\.(ts|tsx|vue|js|mjs)$/.test(e)) out.push(p);
  }
  return out;
}
function existsAt(base, rel) {
  const base2 = join(base, rel);
  for (const ext of EXTS) if (existsSync(base2 + "." + ext)) return true;
  for (const ext of EXTS) if (existsSync(join(base2, "index." + ext))) return true;
  return false;
}

const files = walk(SRC);
const reSpec = /from\s*["']@\/([^"']+)["']|import\s*\(\s*["']@\/([^"']+)["']\s*\)/g;
const hits = []; // { file, spec, localExists, sharedExists }
for (const f of files) {
  const code = readFileSync(f, "utf8");
  let m;
  while ((m = reSpec.exec(code))) {
    const spec = m[1] || m[2];
    const rel = spec.replace(/\.(ts|tsx|vue|js|mjs)$/, "");
    const localExists = existsAt(SRC, rel);
    const sharedExists = existsAt(SHARED, rel);
    // 仅关心「app 里没有、靠回退落到 shared」的导入 —— 这才是改革要改写的
    if (!localExists && sharedExists) hits.push({ file: relative(SRC, f), spec: "@/" + rel, sharedExists });
  }
}
// 去重（同文件同 spec 多次）
const seen = new Set();
const uniq = hits.filter((h) => {
  const k = h.file + "|" + h.spec;
  if (seen.has(k)) return false;
  seen.add(k);
  return true;
});

const byDir = {};
for (const h of uniq) {
  const d = h.spec.split("/").slice(0, 2).join("/"); // e.g. composables/useX or api/encv
  byDir[d] = (byDir[d] || 0) + 1;
}
console.log("=== 实际落到 shared 的 @/ 导入（app 无本地文件，靠回退解析）===");
console.log("总数（去重）:", uniq.length);
console.log("\n--- 按目标目录分布 ---");
for (const [d, c] of Object.entries(byDir).sort((a, b) => b[1] - a[1])) {
  console.log(`  ${String(c).padStart(3)}  ${d}`);
}
console.log("\n--- 完整清单（file -> @/spec）---");
for (const h of uniq.sort((a, b) => a.spec.localeCompare(b.spec))) {
  console.log(`  ${h.file}  ->  ${h.spec}`);
}
