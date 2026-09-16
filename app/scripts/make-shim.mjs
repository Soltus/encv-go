#!/usr/bin/env node
/**
 * make-shim — 任务系统迁移的「应用层兼容垫片」生成 / 校验工具
 *
 * 背景：Phase 3+ 把模块从 encv-mobile 提升到 @encv/shared-components 后，
 * 原位置要留一个 re-export 垫片。手写垫片最大的坑是「符号名写错」
 *（例如把导出一族 getTaskTypeLabel/getTaskTypeMeta 的模块臆想成单符号 taskTypeLabel），
 * 一旦写错，下游经 @/ 别名解析会静默断链 / typecheck 失败。
 *
 * 本工具从「共享模块（真源）」解析其真实导出符号，自动生成正确垫片，
 * 并提供 check 模式校验已有垫片是否与真源一致，避免人工枚举出错。
 *
 * 用法：
 *   node scripts/make-shim.mjs gen  <sharedModule> [mobileShim] [--dry] [--phase N]
 *   node scripts/make-shim.mjs check <mobileShim> <sharedModule>
 *   node scripts/make-shim.mjs check-all [appSrcDir=encv-mobile/src]
 *
 * 例：
 *   node scripts/make-shim.mjs gen packages/shared-components/src/composables/useRunSummaries.ts
 *   node scripts/make-shim.mjs check encv-mobile/src/composables/useRunSummaries.ts packages/shared-components/src/composables/useRunSummaries.ts
 *
 * 无第三方依赖（纯正则 + 轻量扫描器），可在任意 Node 环境运行。
 */

import { readFileSync, writeFileSync, existsSync, readdirSync, statSync, unlinkSync } from "node:fs";
import { resolve, relative, join, extname } from "node:path";

const SRC_ROOT = resolve(import.meta.dirname, "..");
const SHARED_SRC = resolve(SRC_ROOT, "packages/shared-components/src");

// ── 轻量扫描器：去掉注释与字符串，避免 `// export const x` 之类的误判 ──
function stripNoise(src) {
  let out = "";
  let i = 0;
  const n = src.length;
  while (i < n) {
    const c = src[i];
    const c2 = src[i + 1];
    // 行注释
    if (c === "/" && c2 === "/") {
      while (i < n && src[i] !== "\n") i++;
      continue;
    }
    // 块注释
    if (c === "/" && c2 === "*") {
      i += 2;
      while (i < n && !(src[i] === "*" && src[i + 1] === "/")) i++;
      i += 2;
      continue;
    }
    // 注意：此处**不**吃掉字符串内容。re-export 垫片形如
    //   export * from "@encv/shared-components/...";
    // 说明符必须保留，否则 isShim / parseReExports 的 `from "..."` 正则会
    // 匹配不到，导致 check-all 误报「未扫描到任何垫片」。
    // 仅剥离注释即可（注释里的 `// export const x` 假阳性已被上面处理）。
    out += c;
    i++;
  }
  return out;
}

// 收集文件中声明的类型名（interface / type），用于推断 export { x } 中 x 是否为类型
function collectTypeDecls(code) {
  const types = new Set();
  const re = /\b(?:interface|type)\s+([A-Za-z0-9_$]+)/g;
  let m;
  while ((m = re.exec(code))) types.add(m[1]);
  return types;
}

function parseExportClause(body, typeHints, valueNames, typeNames) {
  // body 形如 "a, b as c, type D, type E as F"
  for (const raw of body.split(",")) {
    const item = raw.trim();
    if (!item) continue;
    const isType = /^\btype\b/.test(item);
    const inner = item.replace(/^\btype\b\s+/, "").trim();
    const asMatch = inner.match(/^(.+?)\s+as\s+(.+)$/);
    let name;
    if (asMatch) {
      name = asMatch[2].trim();
    } else {
      name = inner.trim();
    }
    if (name === "default") continue;
    if (isType) typeNames.add(name);
    else if (typeHints.has(name)) typeNames.add(name);
    else valueNames.add(name);
  }
}

function extractExports(srcPath) {
  const src = readFileSync(srcPath, "utf8");
  const code = stripNoise(src);
  const valueNames = new Set();
  const typeNames = new Set();
  const starFrom = [];

  const typeHints = collectTypeDecls(code);

  // export * from "..."  → 无法枚举，记下来走 export * 兜底
  const starRe = /export\s*\*\s*from\s*["']([^"']+)["']/g;
  let m;
  while ((m = starRe.exec(code))) starFrom.push(m[1]);

  // export type { ... }  /  export { ... } [from "..."]
  const braceRe = /export\s+(?:type\s+)?\{([^}]+)\}\s*(?:from\s*["'][^"']+["'])?/g;
  while ((m = braceRe.exec(code))) {
    parseExportClause(m[1], typeHints, valueNames, typeNames);
  }

  // export (default)? (async)? (function|class|const|let|var|enum|interface|type|abstract class) NAME
  const declRe =
    /export\s+(?:default\s+)?(?:async\s+)?(?:abstract\s+)?(function|class|const|let|var|enum|interface|type)\s+([A-Za-z0-9_$]+)/g;
  while ((m = declRe.exec(code))) {
    const kind = m[1];
    const name = m[2];
    if (kind === "interface" || kind === "type") typeNames.add(name);
    else valueNames.add(name);
  }

  return { valueNames: [...valueNames], typeNames: [...typeNames], starFrom };
}

// 由共享模块路径推导 @encv/shared-components/... 说明符
function sharedSpecifier(srcPath) {
  const abs = resolve(srcPath);
  const idx = abs.indexOf("shared-components/src/");
  if (idx === -1) throw new Error(`无法从路径推导共享说明符：${srcPath}`);
  let rel = abs.slice(idx + "shared-components/src/".length);
  rel = rel.replace(/\.(ts|tsx|vue)$/, "");
  return `@encv/shared-components/${rel}`;
}

// 由 @encv/shared-components/<rel> 说明符解析到 shared 源文件
// （尝试 ts/tsx/vue，以及目录索引 index.ts/tsx/vue —— 兼容 `@.../api/core` 指向 core/index.ts）
function resolveShared(spec) {
  const m = spec.match(/^@encv\/shared-components\/(.+)$/);
  if (!m) return null;
  const rel = m[1].replace(/\.(ts|tsx|vue)$/, "");
  const base = resolve(SRC_ROOT, "packages/shared-components/src", rel);
  const candidates = [];
  for (const ext of ["ts", "tsx", "vue"]) candidates.push(`${base}.${ext}`);
  for (const ext of ["ts", "tsx", "vue"]) candidates.push(resolve(base, `index.${ext}`));
  for (const p of candidates) {
    if (existsSync(p)) return p;
  }
  return null;
}

function buildShim(spec, ex, phase) {
  const phaseNote = phase ? `（Phase ${phase}）` : "";
  const lines = [];
  lines.push("/**");
  lines.push(" * 应用层兼容垫片（由 scripts/make-shim.mjs 生成）");
  lines.push(` * 真实实现已提升至 ${spec}。`);
  lines.push(" * 本文件仅做 re-export 转发，不保留任何实现逻辑，");
  lines.push(" * 防止实现被回贴回应用层。下游经 @/ 别名无感。");
  if (phaseNote) lines.push(` * ${phaseNote}`);
  lines.push(" */");
  lines.push("");

  if (ex.starFrom.length > 0) {
    // 含 export * 兜底：用 export * 覆盖全部（含跨模块再导出符号）
    lines.push(`export * from "${spec}";`);
    return lines.join("\n") + "\n";
  }

  const names = [
    ...ex.valueNames.map((n) => `  ${n},`),
    ...ex.typeNames.map((n) => `  type ${n},`),
  ];
  if (names.length === 0) {
    lines.push(`export {} from "${spec}";`);
    return lines.join("\n") + "\n";
  }
  lines.push("export {");
  lines.push(names.join("\n"));
  lines.push(`} from "${spec}";`);
  return lines.join("\n") + "\n";
}

function parseReExports(shimPath) {
  const code = stripNoise(readFileSync(shimPath, "utf8"));
  const valueNames = new Set();
  const typeNames = new Set();
  const starFrom = []; // export * from "..." 的来源
  const braceFrom = []; // export { ... } from "..." 的来源
  let m;
  const starRe = /export\s*\*\s*from\s*["']([^"']+)["']/g;
  while ((m = starRe.exec(code))) starFrom.push(m[1]);
  const braceRe = /export\s+(?:type\s+)?\{([^}]+)\}\s*from\s*["']([^"']+)["']/g;
  while ((m = braceRe.exec(code))) {
    const from = m[2];
    parseExportClause(m[1], new Set(), valueNames, typeNames);
    // 仅记录目标说明符（用于与真源比对）
    braceFrom.push(from);
  }
  return {
    valueNames: [...valueNames],
    typeNames: [...typeNames],
    starFrom,
    braceFrom,
  };
}

function cmdGen(sharedArg, mobileArg, opts) {
  const sharedPath = resolve(SRC_ROOT, sharedArg);
  if (!existsSync(sharedPath)) {
    console.error(`✖ 共享模块不存在: ${sharedPath}`);
    process.exit(1);
  }
  const ex = extractExports(sharedPath);
  const spec = sharedSpecifier(sharedPath);
  const shim = buildShim(spec, ex, opts.phase);

  if (!mobileArg) {
    console.log(shim);
    return;
  }
  const mobilePath = resolve(SRC_ROOT, mobileArg);
  if (opts.dry) {
    console.log(`# 将写入 ${relative(SRC_ROOT, mobilePath)}:`);
    console.log(shim);
    return;
  }
  writeFileSync(mobilePath, shim, "utf8");
  console.log(`✔ 已生成垫片: ${relative(SRC_ROOT, mobilePath)}`);
  console.log(`  导出 ${ex.valueNames.length} 值 / ${ex.typeNames.length} 类型` + (ex.starFrom.length ? `（含 export * 兜底）` : ""));
}

function cmdCheck(shimArg, sharedArg) {
  const shimPath = resolve(SRC_ROOT, shimArg);
  const sharedPath = resolve(SRC_ROOT, sharedArg);
  if (!existsSync(shimPath) || !existsSync(sharedPath)) {
    console.error("✖ 文件不存在");
    process.exit(1);
  }
  const truth = extractExports(sharedPath);
  const truthAll = new Set([...truth.valueNames, ...truth.typeNames]);
  const shim = parseReExports(shimPath);
  const shimAll = new Set([...shim.valueNames, ...shim.typeNames]);

  const missing = [...truthAll].filter((n) => !shimAll.has(n));
  const extra = [...shimAll].filter((n) => !truthAll.has(n));

  if (missing.length === 0 && extra.length === 0) {
    console.log(`✔ 垫片与真源一致（${truthAll.size} 个导出）`);
    return;
  }
  if (missing.length) console.log(`⚠ 垫片缺失导出: ${missing.join(", ")}`);
  if (extra.length) console.log(`⚠ 垫片多出（真源无）: ${extra.join(", ")}`);
  process.exit(1);
}

// 递归收集目录下所有 .ts/.tsx/.vue（跳过 node_modules/dist/.git）
function walk(dir, acc = []) {
  for (const name of readdirSync(dir)) {
    if (name === "node_modules" || name === "dist" || name === ".git") continue;
    const p = join(dir, name);
    const st = statSync(p);
    if (st.isDirectory()) walk(p, acc);
    else if ([".ts", ".tsx", ".vue"].includes(extname(p))) acc.push(p);
  }
  return acc;
}

// 判断文件是否为「纯 re-export 垫片」：所有 export 都来自 @encv/shared-components
//
// 两种垫片形态都要认（否则 check-all 会静默漏掉一半垫片）：
//   ① export * from "@encv/shared-components/..."
//   ② export { a, b, type C } from "@encv/shared-components/..."
// 坑：形态②的 `export { type C } from` 里的 `type C` 会被「本地声明」正则
//     误判成本地 type 声明；且旧 specs 正则 `export\s+(?:\*\s*)?from` 根本
//     匹配不到中间夹着 `{...}` 的 brace re-export。两者叠加导致所有 brace
//     垫片被判为「非垫片」而跳过一致性校验。修法：先收集/剥离 re-export 语句，
//     再在「剩余代码」上探测本地声明。
const RE_STAR = /export\s*\*\s*(?:as\s+[A-Za-z0-9_$]+\s+)?from\s*["']([^"']+)["']\s*;?/g;
const RE_BRACE = /export\s+(?:type\s+)?\{[^}]*\}\s*from\s*["']([^"']+)["']\s*;?/g;
function isShim(code) {
  const stripped = stripNoise(code);
  const specs = [
    ...stripped.matchAll(RE_STAR),
    ...stripped.matchAll(RE_BRACE),
  ].map((m) => m[1]);
  if (specs.length === 0) return false;
  // 去掉所有 re-export 语句后，剩余代码若仍有本地声明 → 非纯垫片
  const rest = stripped.replace(RE_STAR, "").replace(RE_BRACE, "");
  if (/\b(?:export\s+)?(?:async\s+)?(?:abstract\s+)?(?:function|class|const|let|var|enum|interface|type)\s+[A-Za-z0-9_$]/.test(rest)) {
    return false;
  }
  return specs.every((s) => s.startsWith("@encv/shared-components/"));
}

function cmdCheckAll(appArg = "encv-mobile/src") {
  const appDir = resolve(SRC_ROOT, appArg);
  if (!existsSync(appDir)) {
    console.error(`✖ 应用层目录不存在: ${appDir}`);
    process.exit(1);
  }
  const files = walk(appDir);
  let shimCount = 0;
  let fail = 0;
  const mismatches = [];

  for (const f of files) {
    const code = readFileSync(f, "utf8");
    if (!isShim(code)) continue;
    shimCount++;
    const shim = parseReExports(f);
    // 收集该垫片指向的所有 shared 说明符（brace 来源）
    const specs = [...new Set(shim.braceFrom)];
    const shimAll = new Set([...shim.valueNames, ...shim.typeNames]);

    // export * 垫片：转发全部，无法比对完整性，仅校验目标存在
    if (shim.starFrom.length > 0) {
      for (const s of shim.starFrom) {
        if (!resolveShared(s)) {
          fail++;
          mismatches.push(`${relative(SRC_ROOT, f)}: export * 目标不存在 ${s}`);
        }
      }
      continue;
    }

    // 多个说明符时取并集；单说明符时直接比对
    const union = new Set();
    const perTarget = [];
    for (const s of specs) {
      const target = resolveShared(s);
      if (!target) {
        fail++;
        mismatches.push(`${relative(SRC_ROOT, f)}: 目标不存在 ${s}`);
        continue;
      }
      const truth = extractExports(target);
      const all = new Set([...truth.valueNames, ...truth.typeNames]);
      perTarget.push({ s, all, truth });
      for (const n of all) union.add(n);
    }
    if (specs.length === 0) continue;

    const missing = [...shimAll].filter((n) => !union.has(n));
    const extra = specs.length === 1 && perTarget[0]
      ? [...perTarget[0].truth.valueNames, ...perTarget[0].truth.typeNames].filter((n) => !shimAll.has(n))
      : [];

    if (missing.length || extra.length) {
      fail++;
      if (missing.length) mismatches.push(`${relative(SRC_ROOT, f)}: 垫片多导出(真源无) ${missing.join(", ")}`);
      if (extra.length) mismatches.push(`${relative(SRC_ROOT, f)}: 垫片漏导出(真源有) ${extra.join(", ")}`);
    }
  }

  if (shimCount === 0) {
    console.log("✔ 无残留垫片（应用层已纯化，全部经 @/ 别名直连 shared）");
    return;
  }
  if (fail === 0) {
    console.log(`✔ 全部 ${shimCount} 个垫片与 shared 真源一致`);
    return;
  }
  console.log(`✖ ${fail} 个垫片不一致（共 ${shimCount} 个）：`);
  for (const m of mismatches) console.log(`  - ${m}`);
  process.exit(1);
}

// ── prune：删除「同名垫片」，列出「名称错位垫片」──
//
// 范式约束（结构性改革）：模块提升进 shared 后，应用层不得再留 re-export 垫片——
// 那只是「迁移谎言」（假装是实现的文件，实则转发）。正确结构是：shared 是唯一真源，
// 应用层经 `@/` 别名的二级回退（tsconfig `@/*` + vite/vitest 的 encv-alias-fallback）
// 直接解析到 shared。
//
// 安全判定：
//   • 同名垫片：shim 相对路径 == shared 真源相对路径（如 app/composables/useX.ts →
//     shared/composables/useX.ts）。删除后 `@/composables/useX` 自动落到 shared，零风险。
//   • 名称错位垫片：shim 路径与 shared 真源路径不同（如 app/api/encv_core →
//     shared/api/core）。删除前必须先改写其 importer（把 `@/api/encv_core` 改成
//     `@/api/core`），否则会断链。故 prune 默认【只删同名】，把错位项列出待人工处理；
//     `--apply` 同样只删同名，错位项保持不动（避免静默断链）。
function shimTargets(code) {
  const stripped = stripNoise(code);
  const specs = [...stripped.matchAll(RE_STAR), ...stripped.matchAll(RE_BRACE)].map((m) => m[1]);
  return [...new Set(specs)];
}
function sharedRelFor(spec) {
  const m = spec.match(/^@encv\/shared-components\/(.+)$/);
  if (!m) return null;
  const rel = m[1].replace(/\.(ts|tsx|vue)$/, "");
  const base = resolve(SHARED_SRC, rel);
  for (const ext of ["ts", "tsx", "vue"]) if (existsSync(`${base}.${ext}`)) return rel;
  for (const ext of ["ts", "tsx", "vue"]) if (existsSync(resolve(base, `index.${ext}`))) return `${rel}/index`;
  return null;
}
function cmdPrune(opts) {
  const appDir = resolve(SRC_ROOT, "encv-mobile/src");
  if (!existsSync(appDir)) {
    console.error(`✖ 应用层目录不存在: ${appDir}`);
    process.exit(1);
  }
  const only = opts.only ? opts.only.replace(/^\/+|\/+$/g, "") : null; // 形如 "api" / "composables/useX"
  const files = walk(appDir);
  const sameName = [];
  const mismatched = [];
  for (const f of files) {
    const code = readFileSync(f, "utf8");
    if (!isShim(code)) continue;
    const shimRel = relative(appDir, f).replace(/\.(ts|tsx|vue)$/, "");
    if (only && !shimRel.startsWith(only + "/") && shimRel !== only) continue; // 按前缀分批
    const targets = shimTargets(code);
    const rels = targets.map(sharedRelFor);
    const allSame = targets.length > 0 && rels.every((r) => r && r === shimRel);
    if (allSame) sameName.push({ f, shimRel });
    else mismatched.push({ shimRel, targets, rels });
  }
  const scope = only ? ` (范围: ${only}/)` : "";
  if (!opts.apply) {
    console.log(`[dry-run${scope}] 同名垫片（可安全删除）: ${sameName.length}`);
    for (const d of sameName) console.log(`    - ${d.shimRel}`);
    console.log(`[dry-run${scope}] 名称错位垫片（需先改写 importer，未删除）: ${mismatched.length}`);
    for (const m of mismatched) console.log(`    - ${m.shimRel}  ->  ${m.targets.join(", ")}`);
    return;
  }
  for (const d of sameName) {
    unlinkSync(d.f);
    console.log(`✔ 已删除垫片: ${d.shimRel}`);
  }
  console.log(
    `✔ 删除 ${sameName.length} 个同名垫片${scope}；${mismatched.length} 个错位垫片未动（先改写其 importer 指向 shared 真源路径后再删）。`,
  );
}

function main() {
  const args = process.argv.slice(2);
  const cmd = args[0];
  if (cmd === "gen" || cmd === undefined) {
    const positional = args.slice(1).filter((a) => !a.startsWith("--"));
    const opts = {
      dry: args.includes("--dry"),
      phase: (args.find((a) => a.startsWith("--phase=")) || "").split("=")[1],
    };
    cmdGen(positional[0], positional[1], opts);
  } else if (cmd === "check") {
    cmdCheck(args[1], args[2]);
  } else if (cmd === "check-all") {
    cmdCheckAll(args[1]);
  } else if (cmd === "prune") {
    const onlyArg = args.find((a) => a.startsWith("--only="));
    cmdPrune({ apply: args.includes("--apply"), only: onlyArg ? onlyArg.slice("--only=".length) : null });
  } else {
    console.error("用法: make-shim.mjs gen <sharedModule> [mobileShim] [--dry] [--phase=N]");
    console.error("      make-shim.mjs check <mobileShim> <sharedModule>");
    console.error("      make-shim.mjs check-all [appSrcDir]");
    console.error("      make-shim.mjs prune [--apply] [--only=<前缀>]   # 删除同名垫片（默认 dry-run 仅列出）");
    process.exit(1);
  }
}

main();
