#!/usr/bin/env node
/**
 * skill-manager.mjs — project skill registry + lifecycle for the app-dev MCP.
 *
 * A "skill" is a directory containing a SKILL.md file (with YAML frontmatter:
 * name / description / metadata). Skills may live under MULTIPLE base paths
 * (e.g. .codebuddy/skills, .agents/skills, .trae/skills). This module:
 *
 *   - keeps an editable list of skill base paths (skillPaths)
 *   - discovers skills by scanning those paths
 *   - persists a registry (.codebuddy/skill-registry.json) with per-skill
 *     metadata + sha256 hash (so edits are detectable)
 *   - watches the paths and skill dirs and re-scans on change (monitoring)
 *   - exposes CRUD: add (import | npx | git), get, update, remove, plus
 *     add/remove/list skill paths and forced rescan.
 *
 * add methods:
 *   import : copy a local dir/file into a skill path
 *   npx    : `npm install <pkg> --prefix <tmp>` then copy the first SKILL.md found
 *   git    : `git clone --depth 1 <url>` (optional subdir/ref) then copy skill(s)
 *
 * Pure Node, no external deps. Core functions are exported for unit testing;
 * the stdio MCP server lives in app-dev-mcp.mjs (which imports this module).
 */

import { promises as fs, watch, existsSync, statSync } from "node:fs";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { dirname, resolve, relative, join, basename, isAbsolute } from "node:path";
import { fileURLToPath } from "node:url";
import { tmpdir } from "node:os";

const __dirname = dirname(fileURLToPath(import.meta.url));
const REPO_ROOT = resolve(__dirname, ".."); // /workspace
const REGISTRY_PATH = resolve(REPO_ROOT, ".codebuddy/skill-registry.json");
const SKILL_FILENAME = "SKILL.md";

const DEFAULT_SKILL_PATHS = [".codebuddy/skills", ".agents/skills", ".trae/skills"];

// --- in-memory state -------------------------------------------------------
let state = {
  skillPaths: [...DEFAULT_SKILL_PATHS],
  skills: {}, // name -> { path, source, sourceType, hash, displayName, description, updatedAt }
};
const watchers = new Map(); // absDir -> { handle, isSkillDir }
let rescanTimers = new Map(); // absDir -> timeout handle (debounce)
let started = false;

// --- low-level helpers ------------------------------------------------------
function runCmd(cmd, args, cwd, timeoutMs = 120000) {
  return new Promise((resolve) => {
    const child = spawn(cmd, args, { cwd, env: process.env });
    let stdout = "";
    let stderr = "";
    let done = false;
    const finish = (code, signal, timedOut = false) => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      resolve({ stdout, stderr, code: code ?? (signal ? 1 : 0), timedOut });
    };
    const timer = setTimeout(() => {
      child.kill("SIGKILL");
      finish(null, "timeout", true);
    }, timeoutMs);
    child.stdout.on("data", (d) => (stdout += d));
    child.stderr.on("data", (d) => (stderr += d));
    child.on("error", (e) => {
      stderr += `\n[spawn error] ${e.message}`;
      finish(1, null);
    });
    child.on("close", (code, signal) => finish(code, signal));
  });
}

function sha256(text) {
  return createHash("sha256").update(text).digest("hex");
}

/** Parse a minimal YAML frontmatter (name / description) from SKILL.md text. */
function parseFrontmatter(text) {
  const m = text.match(/^---\s*\n([\s\S]*?)\n---/);
  if (!m) return {};
  const out = {};
  for (const line of m[1].split("\n")) {
    const mm = line.match(/^([A-Za-z0-9_-]+)\s*:\s*(.*)$/);
    if (mm) {
      let v = mm[2].trim();
      if ((v.startsWith('"') && v.endsWith('"')) || (v.startsWith("'") && v.endsWith("'"))) {
        v = v.slice(1, -1);
      }
      out[mm[1]] = v;
    }
  }
  return out;
}

async function hashSkillFile(absDir) {
  try {
    const buf = await fs.readFile(join(absDir, SKILL_FILENAME), "utf8");
    return sha256(buf);
  } catch {
    return null;
  }
}

/** Find all skill directories under a base path (direct children with SKILL.md). */
async function findSkillsUnder(baseAbs) {
  const found = [];
  if (!existsSync(baseAbs)) return found;
  let entries;
  try {
    entries = await fs.readdir(baseAbs, { withFileTypes: true });
  } catch {
    return found;
  }
  for (const e of entries) {
    // follow symlinks: .codebuddy/skills/* are symlinks to .agents/.trae, and
    // CodeBuddy discovers skills through them — so the manager must too.
    const dir = join(baseAbs, e.name);
    let isDir = false;
    try {
      isDir = statSync(dir).isDirectory();
    } catch {
      continue;
    }
    if (!isDir) continue;
    if (existsSync(join(dir, SKILL_FILENAME))) {
      const text = await fs.readFile(join(dir, SKILL_FILENAME), "utf8").catch(() => "");
      const fm = parseFrontmatter(text);
      found.push({
        name: e.name,
        absDir: dir,
        displayName: fm.name || e.name,
        description: fm.description || "",
        hash: sha256(text),
      });
    }
  }
  return found;
}

// --- registry persistence ---------------------------------------------------
export async function loadRegistry() {
  try {
    const raw = await fs.readFile(REGISTRY_PATH, "utf8");
    const data = JSON.parse(raw);
    state.skillPaths = Array.isArray(data.skillPaths) && data.skillPaths.length
      ? data.skillPaths
      : [...DEFAULT_SKILL_PATHS];
    state.skills = data.skills && typeof data.skills === "object" ? data.skills : {};
  } catch {
    state.skillPaths = [...DEFAULT_SKILL_PATHS];
    state.skills = {};
  }
  await scanAndSync();
  return state;
}

export async function saveRegistry() {
  await fs.mkdir(dirname(REGISTRY_PATH), { recursive: true });
  await fs.writeFile(REGISTRY_PATH, JSON.stringify(state, null, 2) + "\n", "utf8");
}

// --- scanning / discovery ---------------------------------------------------
/** Walk all skill paths, compute hashes, and merge into the registry. */
export async function scanAndSync() {
  const discovered = {};
  for (const rel of state.skillPaths) {
    const baseAbs = resolve(REPO_ROOT, rel);
    const skills = await findSkillsUnder(baseAbs);
    for (const s of skills) {
      const relPath = relative(REPO_ROOT, s.absDir);
      const prev = state.skills[s.name];
      discovered[s.name] = {
        path: relPath,
        displayName: s.displayName,
        description: s.description,
        hash: s.hash,
        // preserve known origin if we already tracked this skill
        source: prev?.source || relPath,
        sourceType: prev?.sourceType || "discovered",
        updatedAt: new Date().toISOString(),
      };
    }
  }
  // keep entries whose underlying dir still exists; update hash/display
  const next = {};
  for (const [name, info] of Object.entries(discovered)) {
    next[name] = info;
  }
  state.skills = next;
  await saveRegistry();
  ensureWatchers();
  return next;
}

// --- monitoring (watching) --------------------------------------------------
function scheduleRescan(absDir) {
  clearTimeout(rescanTimers.get(absDir));
  rescanTimers.set(
    absDir,
    setTimeout(() => {
      rescanTimers.delete(absDir);
      scanAndSync().catch((e) => process.stderr.write(`[skill-mgr] rescan err: ${e.message}\n`));
    }, 120)
  );
}

function watchDir(absDir, isSkillDir) {
  if (watchers.has(absDir) || !existsSync(absDir)) return;
  try {
    const handle = watch(absDir, (event, filename) => {
      if (!filename) return;
      // on Linux fs.watch only fires for direct children; filter noise
      if (isSkillDir && filename !== SKILL_FILENAME && !filename.endsWith(".md")) return;
      scheduleRescan(absDir);
    });
    watchers.set(absDir, { handle, isSkillDir });
  } catch (e) {
    process.stderr.write(`[skill-mgr] watch FAILED ${absDir}: ${e.message}\n`);
  }
}

/** Ensure every skill path + every discovered skill dir is watched. */
export function ensureWatchers() {
  for (const rel of state.skillPaths) {
    watchDir(resolve(REPO_ROOT, rel), false);
  }
  for (const info of Object.values(state.skills)) {
    watchDir(resolve(REPO_ROOT, info.path), true);
  }
}

export function startWatching() {
  if (started) return;
  started = true;
  ensureWatchers();
  process.stderr.write(`[skill-mgr] watching ${state.skillPaths.length} skill path(s)\n`);
}

// --- public API (CRUD) ------------------------------------------------------
export function listSkillPaths() {
  return [...state.skillPaths];
}

export function addSkillPath(rel) {
  if (!rel || typeof rel !== "string") return { ok: false, error: "path required" };
  const norm = rel.replace(/^\/+/, "").replace(/\/+$/, "");
  if (!norm) return { ok: false, error: "invalid path" };
  if (state.skillPaths.includes(norm)) return { ok: true, path: norm, existed: true };
  state.skillPaths.push(norm);
  saveRegistry();
  ensureWatchers();
  return { ok: true, path: norm };
}

export function removeSkillPath(rel) {
  const idx = state.skillPaths.indexOf(rel);
  if (idx < 0) return { ok: false, error: "path not tracked" };
  state.skillPaths.splice(idx, 1);
  saveRegistry();
  return { ok: true, path: rel };
}

export function listSkills(filter) {
  const all = Object.entries(state.skills).map(([name, info]) => ({ name, ...info }));
  if (!filter) return all;
  const f = filter.toLowerCase();
  return all.filter(
    (s) =>
      s.name.toLowerCase().includes(f) ||
      (s.displayName || "").toLowerCase().includes(f) ||
      (s.description || "").toLowerCase().includes(f)
  );
}

export function getSkill(name) {
  return state.skills[name] || null;
}

function pickTargetPath(targetPath) {
  if (targetPath) return targetPath;
  // prefer the first path that exists / is writable; else the first declared
  for (const rel of state.skillPaths) {
    const abs = resolve(REPO_ROOT, rel);
    if (existsSync(abs)) return rel;
  }
  return state.skillPaths[0];
}

function resolveRepoPath(p) {
  if (!p) return null;
  return isAbsolute(p) ? p : resolve(REPO_ROOT, p);
}

/** Copy a local source (file or dir) into <targetPath>/<name>/SKILL.md or dir. */
async function importLocal({ src, name, targetPath }) {
  const srcAbs = resolveRepoPath(src);
  if (!srcAbs || !existsSync(srcAbs)) return { ok: false, error: `src not found: ${src}` };
  const isFile = statSync(srcAbs).isFile();
  const skillName = name || (isFile ? basename(dirname(srcAbs)) : basename(srcAbs));
  if (!skillName) return { ok: false, error: "cannot derive skill name" };
  const destBase = resolve(REPO_ROOT, pickTargetPath(targetPath), skillName);
  await fs.mkdir(destBase, { recursive: true });
  if (isFile) {
    await fs.copyFile(srcAbs, join(destBase, SKILL_FILENAME));
  } else {
    await fs.cp(srcAbs, destBase, { recursive: true });
  }
  return { ok: true, name: skillName, dest: relative(REPO_ROOT, destBase), method: "import" };
}

/** npm install a package to a temp prefix, copy first SKILL.md found. */
async function installNpx({ package: pkg, name, targetPath, bin }) {
  if (!pkg) return { ok: false, error: "package required" };
  const tmp = await fs.mkdtemp(join(tmpdir(), "skill-npx-"));
  try {
    const install = await runCmd("npm", ["install", "--no-audit", "--no-fund", "--prefix", tmp, pkg], tmp, 300000);
    if (install.code !== 0) {
      return { ok: false, error: `npm install failed: ${install.stderr.slice(0, 500)}` };
    }
    if (bin) {
      const r = await runCmd("npx", ["-y", ...bin.split(/\s+/)], tmp, 300000);
      if (r.code !== 0) return { ok: false, error: `installer (${bin}) failed: ${r.stderr.slice(0, 500)}` };
    }
    // search node_modules for SKILL.md
    const found = await findFirstSkillDir(join(tmp, "node_modules"));
    if (!found) return { ok: false, error: `no SKILL.md found in installed ${pkg}` };
    const skillName = name || basename(found);
    const destBase = resolve(REPO_ROOT, pickTargetPath(targetPath), skillName);
    await fs.mkdir(destBase, { recursive: true });
    await fs.cp(found, destBase, { recursive: true });
    return { ok: true, name: skillName, dest: relative(REPO_ROOT, destBase), method: "npx", source: pkg };
  } finally {
    await fs.rm(tmp, { recursive: true, force: true });
  }
}

async function findFirstSkillDir(root) {
  if (!existsSync(root)) return null;
  const stack = [root];
  while (stack.length) {
    const dir = stack.pop();
    let entries;
    try {
      entries = await fs.readdir(dir, { withFileTypes: true });
    } catch {
      continue;
    }
    if (existsSync(join(dir, SKILL_FILENAME))) return dir;
    for (const e of entries) if (e.isDirectory()) stack.push(join(dir, e.name));
  }
  return null;
}

/**
 * Copy skill(s) out of srcDir into destBase, following the agentskills
 * convention used by `npx skills add`:
 *   - case A: srcDir is itself a skill (has SKILL.md)
 *   - case B: srcDir is a directory of skills (immediate children w/ SKILL.md)
 *   - case C: srcDir contains a `skills/` subdir (collection) -> recurse
 */
async function copySkillsFrom(srcDir, destBase, { name, url }) {
  // case A: srcDir is itself a skill
  if (existsSync(join(srcDir, SKILL_FILENAME))) {
    const skillName = name || basename(srcDir);
    const dest = join(destBase, skillName);
    await fs.mkdir(dest, { recursive: true });
    await fs.cp(srcDir, dest, { recursive: true });
    return { ok: true, name: skillName, dest: relative(REPO_ROOT, dest), method: "git", source: url };
  }
  // case B: immediate children that are skills -> copy each
  const entries = await fs.readdir(srcDir, { withFileTypes: true });
  const skillDirs = [];
  for (const e of entries) {
    if (!e.isDirectory()) continue;
    if (existsSync(join(srcDir, e.name, SKILL_FILENAME))) skillDirs.push(e.name);
  }
  if (skillDirs.length) {
    const copied = [];
    for (const sn of skillDirs) {
      const dest = join(destBase, sn);
      await fs.mkdir(dest, { recursive: true });
      await fs.cp(join(srcDir, sn), dest, { recursive: true });
      copied.push(sn);
    }
    return { ok: true, names: copied, dest: relative(REPO_ROOT, destBase), method: "git", source: url };
  }
  // case C: a `skills/` subdir (agentskills collection convention) -> recurse
  if (existsSync(join(srcDir, "skills"))) {
    return copySkillsFrom(join(srcDir, "skills"), destBase, { name, url });
  }
  return { ok: false, error: `no SKILL.md found in ${relative(REPO_ROOT, srcDir) || "(repo root)"}` };
}

/** git clone --depth 1, copy skill(s) from optional subdir. */
async function installGit({ url, subdir, name, targetPath, ref }) {
  if (!url) return { ok: false, error: "url required" };
  const tmp = await fs.mkdtemp(join(tmpdir(), "skill-git-"));
  try {
    const args = ["clone", "--depth", "1"];
    if (ref) args.push("--branch", ref);
    args.push(url, tmp);
    const r = await runCmd("git", args, REPO_ROOT, 300000);
    if (r.code !== 0) return { ok: false, error: `git clone failed: ${r.stderr.slice(0, 500)}` };

    let searchRoot = tmp;
    if (subdir) {
      searchRoot = join(tmp, subdir);
      if (!existsSync(searchRoot)) return { ok: false, error: `subdir not found in repo: ${subdir}` };
    }
    const targetBase = resolve(REPO_ROOT, pickTargetPath(targetPath));
    await fs.mkdir(targetBase, { recursive: true });
    return await copySkillsFrom(searchRoot, targetBase, { name, url });
  } finally {
    await fs.rm(tmp, { recursive: true, force: true });
  }
}

export async function addSkill(opts = {}) {
  const method = opts.method || "import";
  let res;
  if (method === "import") res = await importLocal(opts);
  else if (method === "npx") res = await installNpx(opts);
  else if (method === "git") res = await installGit(opts);
  else return { ok: false, error: `unknown method: ${method}` };
  if (res.ok) await scanAndSync();
  return res;
}

export async function updateSkill(name, opts = {}) {
  const cur = state.skills[name];
  if (!cur) return { ok: false, error: `skill not found: ${name}` };
  if (opts.method) {
    // re-fetch via the supplied method into the same target path
    const targetPath = cur.path ? dirname(cur.path) : undefined;
    const res = await addSkill({ ...opts, name, targetPath });
    return res;
  }
  // no method: just refresh hash from disk
  const abs = resolve(REPO_ROOT, cur.path);
  const text = await fs.readFile(join(abs, SKILL_FILENAME), "utf8").catch(() => "");
  cur.hash = sha256(text);
  cur.updatedAt = new Date().toISOString();
  await saveRegistry();
  return { ok: true, name, hash: cur.hash, refreshed: true };
}

export async function removeSkill(name) {
  const cur = state.skills[name];
  if (!cur) return { ok: false, error: `skill not found: ${name}` };
  const abs = resolve(REPO_ROOT, cur.path);
  try {
    await fs.rm(abs, { recursive: true, force: true });
  } catch (e) {
    return { ok: false, error: `remove failed: ${e.message}` };
  }
  delete state.skills[name];
  await saveRegistry();
  return { ok: true, name, removed: relative(REPO_ROOT, abs) };
}

export function getState() {
  return state;
}

// --- bootstrap when run directly (sanity) -----------------------------------
if (import.meta.url === `file://${process.argv[1]}`) {
  await loadRegistry();
  startWatching();
  console.log(JSON.stringify({ paths: listSkillPaths(), skills: listSkills().length }, null, 2));
}
