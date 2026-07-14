import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

const SRC = join(process.cwd(), "packages/shared-components/src");
const SKIP = new Set(["core/request.ts", "core/baseUrl.ts", "core/context.ts"]); // 抽象层自身

function walk(d) {
  const out = [];
  for (const e of readdirSync(d)) {
    const p = join(d, e);
    const s = statSync(p);
    if (s.isDirectory()) out.push(...walk(p));
    else if (/\.(ts|vue)$/.test(e)) out.push(p);
  }
  return out;
}

const files = walk(SRC);
const hits = [];
for (const f of files) {
  const rel = f.replace(SRC + "/", "");
  if (SKIP.has(rel)) continue;
  const code = readFileSync(f, "utf8");
  const lines = code.split("\n");
  lines.forEach((ln, i) => {
    if (/\bfetch\(/.test(ln)) {
      // 向上找函数名
      let fn = "(top-level/script)";
      for (let j = i; j >= 0; j--) {
        const m = lines[j].match(/^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z0-9_]+)/);
        if (m) { fn = m[1]; break; }
        const m2 = lines[j].match(/^\s*(?:export\s+)?const\s+([A-Za-z0-9_]+)\s*[:=]/);
        if (m2) { fn = m2[1]; break; }
      }
      hits.push({ rel, line: i + 1, fn, text: ln.trim().slice(0, 90) });
    }
  });
}

// 按函数分组
const byFn = {};
for (const h of hits) {
  const key = `${h.rel} :: ${h.fn}`;
  (byFn[key] ||= []).push(h);
}
console.log(`=== shared 内裸 fetch 调用点（除 core 抽象层）共 ${hits.length} 处，分 ${Object.keys(byFn).length} 个函数 ===\n`);
for (const [k, arr] of Object.entries(byFn)) {
  console.log(`● ${k}  (${arr.length} 处)`);
  for (const h of arr) console.log(`    L${h.line}: ${h.text}`);
}
