#!/usr/bin/env node
// 一次性分析：把 encv-mobile/src 下所有「纯 re-export 垫片」分类为
//  - SAME   : 垫片路径 <rel> 与 shared 目标 <rel> 相同 → 可直接删除（@/ 别名回退到 shared）
//  - MISMATCH: 垫片路径 <relA> 与 shared 目标 <relB> 不同 → 需显式别名条目或重命名，不能直接删
import { readFileSync, existsSync, readdirSync, statSync } from "node:fs";
import { resolve, relative, join } from "node:path";

const SRC_ROOT = resolve(import.meta.dirname, "..");
const APP_SRC = resolve(SRC_ROOT, "encv-mobile/src");
const SHARED_SRC = resolve(SRC_ROOT, "packages/shared-components/src");

function stripNoise(src) {
  let out = ""; let i = 0; const n = src.length;
  while (i < n) {
    const c = src[i], c2 = src[i + 1];
    if (c === "/" && c2 === "/") { while (i < n && src[i] !== "\n") i++; continue; }
    if (c === "/" && c2 === "*") { i += 2; while (i < n && !(src[i] === "*" && src[i + 1] === "/")) i++; i += 2; continue; }
    out += c; i++;
  }
  return out;
}
const RE_STAR = /export\s*\*\s*(?:as\s+[A-Za-z0-9_$]+\s+)?from\s*["']([^"']+)["']\s*;?/g;
const RE_BRACE = /export\s+(?:type\s+)?\{[^}]*\}\s*from\s*["']([^"']+)["']\s*;?/g;
function isShim(code) {
  const stripped = stripNoise(code);
  const specs = [...stripped.matchAll(RE_STAR), ...stripped.matchAll(RE_BRACE)].map((m) => m[1]);
  if (specs.length === 0) return false;
  const rest = stripped.replace(RE_STAR, "").replace(RE_BRACE, "");
  if (/\b(?:export\s+)?(?:async\s+)?(?:abstract\s+)?(?:function|class|const|let|var|enum|interface|type)\s+[A-Za-z0-9_$]/.test(rest)) return false;
  return specs.every((s) => s.startsWith("@encv/shared-components/"));
}
function shimTargets(code) {
  const stripped = stripNoise(code);
  const specs = [...stripped.matchAll(RE_STAR), ...stripped.matchAll(RE_BRACE)].map((m) => m[1]);
  return specs.filter((s) => s.startsWith("@encv/shared-components/"));
}
function walk(dir, acc = []) {
  for (const name of readdirSync(dir)) {
    if (name === "node_modules" || name === "dist" || name === ".git") continue;
    const p = join(dir, name);
    const st = statSync(p);
    if (st.isDirectory()) walk(p, acc);
    else if ([".ts", ".tsx", ".vue"].includes(p.slice(p.lastIndexOf(".")))) acc.push(p);
  }
  return acc;
}
function sharedRelOf(spec) {
  const m = spec.match(/^@encv\/shared-components\/(.+)$/);
  if (!m) return null;
  return m[1].replace(/\.(ts|tsx|vue)$/, "");
}
function existsShared(rel) {
  const base = resolve(SHARED_SRC, rel);
  for (const ext of ["ts", "tsx", "vue"]) if (existsSync(`${base}.${ext}`)) return true;
  for (const ext of ["ts", "tsx", "vue"]) if (existsSync(join(base, `index.${ext}`))) return true;
  return false;
}

const files = walk(APP_SRC);
const same = [], mismatch = [];
for (const f of files) {
  const code = readFileSync(f, "utf8");
  if (!isShim(code)) continue;
  const appRel = relative(APP_SRC, f).replace(/\.(ts|tsx|vue)$/, "");
  const targets = shimTargets(code);
  if (targets.length === 0) continue;
  // 取第一个目标判定（多数垫片单目标；多目标若都同/都异分别处理）
  const sharedRels = targets.map(sharedRelOf).filter(Boolean);
  const allSame = sharedRels.length > 0 && sharedRels.every((r) => r === appRel);
  const allMismatch = sharedRels.length > 0 && sharedRels.every((r) => r !== appRel);
  const exists = sharedRels.every((r) => existsShared(r));
  const entry = { appRel, targets, sharedRels, exists };
  if (allSame) same.push(entry);
  else if (allMismatch) mismatch.push(entry);
  else mismatch.push({ ...entry, _mixed: true });
}

console.log(`\n=== 垫片总数: ${same.length + mismatch.length} ===`);
console.log(`\n--- SAME (路径同 shared，可直接删，靠别名回退): ${same.length} ---`);
for (const e of same) console.log(`  ${e.appRel}`);
console.log(`\n--- MISMATCH (路径异 shared，需别名条目/重命名，不能直接删): ${mismatch.length} ---`);
for (const e of mismatch) console.log(`  ${e.appRel}  ->  ${e.sharedRels.join(", ")}${e.exists ? "" : "  [!TARGET MISSING]"}`);
