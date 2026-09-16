// Safety gate rules for app_exec, hot-reloaded by app-dev-mcp.mjs.
// Edit this file and the running MCP server picks up changes automatically
// (fs.watch + dynamic import, ~80ms) — no restart / new conversation needed.
//
// 策略（ESM-only 工具链 / bun 运行）：
//   - 非 kill 类破坏性命令（rm -rf / 危险 git / dd / shred / sudo / curl|sh / 写块设备）
//     仍走同步「命中即拦截」列表 APP_EXEC_DENY。
//   - kill / pkill / killall / fuser 不再 blanket 拦截，改为「先检查再放行」：
//       * 环境连接进程（ssh tunnel、code-server、dev MCP、会话守护、自身/祖先进程树、
//         init）无条件拦截——误杀会令环境失联。
//       * 同一身份（同 uid）启的服务/端口：无条件允许释放
//         （开发期痛快清掉 stale vite / chromium / 占用的端口等）。
//       * 其他身份/未知：先检查——已死(zombie)/无响应(kill -0 不可达)/异常占用端口，
//         且对项目正常服务无风险，才允许释放；否则保守拦截，避免误杀他人进程。
//     以此平衡「安全（不砍环境连接）」与「体验（能痛快释放自己起的资源）」。

import { execFileSync } from 'node:child_process'

export const APP_EXEC_DENY = [
  {
    label: "递归/强制删除（rm -r/-f、rmdir、shred）",
    re: /\brm\b[^|;&]*\s-[rf]/,
  },
  { label: "递归目录删除（rmdir）", re: /\brmdir\b/ },
  {
    label: "磁盘擦除（shred / mkfs / dd if=）",
    re: /\b(shred|mkfs)\b|\bdd\b\s+if=/,
  },
  {
    label: "危险 git 操作（reset --hard / clean -f / checkout -- . / push --force / branch -D）",
    re: /\bgit\b[^|;&]*?(--hard|clean\s+[^|;&]*--?f|push\s+[^|;&]*--force(?:-with-lease)?|checkout\s+[^|;&]*--\s+\.|branch\s+[^|;&]*-D)/,
  },
  { label: "提权（sudo / su）", re: /\b(sudo|su)\b/ },
  {
    label: "网络下载直灌 shell（curl|sh、wget|sh）",
    re: /\b(curl|wget)\b[^|;&]*\|\s*(sudo\s+)?(ba)?sh\b/,
  },
  {
    // 只拦真正危险的块设备/磁盘写入；放行 /dev/null、/dev/stdout、/dev/stderr、
    // /dev/tty、/dev/fd/* 等极常见且无害的重定向（如 `2>/dev/null`）。
    label: "写入块设备/磁盘（> /dev/sd*、/dev/nvme* 等）",
    re: />\s*\/dev\/(?!null\b|stdout\b|stderr\b|tty\b|fd\/|std)/,
  },
  { label: "fork 炸弹（:(){:|:&};:）", re: /:\(\)\s*\{/ },
]

// 环境连接进程：一旦被 kill 会令远程环境失联，永远拦截。
const CRITICAL_RE = [
  /\bsshd\b/,
  /\bssh\b(?:\s.*)?\s-[a-zA-Z]*[LRD]\b/, // ssh -L/-R/-D 隧道
  /\bssh\b.*ProxyJump/,
  /code-server/,
  /codebuddy/,
  /vscode-server/,
  /app-dev-mcp/,
  /\bmcp\b[^\n]*server/i,
  /\bclaude\b/,
  /cursor/,
  /session[^\n]*daemon/i,
  /sessiond/,
  /\.codebuddy[^\n]*session/i,
]

function sh(args, timeout = 4000) {
  try {
    return execFileSync(args[0], args.slice(1), {
      encoding: "utf8",
      stdio: ["ignore", "pipe", "ignore"],
      timeout,
    })
  } catch {
    return ""
  }
}

// 当前进程及其全部祖先（向上回溯到 init）——自身/祖先进程树绝不释放。
function ancestorPids() {
  const set = new Set([process.pid])
  let pid = process.pid
  for (let i = 0; i < 24; i++) {
    const out = sh(["ps", "-o", "ppid=", "-p", String(pid)]).trim()
    const ppid = parseInt(out, 10)
    if (!Number.isFinite(ppid) || ppid <= 1) break
    set.add(ppid)
    pid = ppid
  }
  return set
}

function procInfo(pid) {
  const out = sh(["ps", "-o", "uid=,state=,ppid=,args=", "-p", String(pid)]).trim()
  if (!out) return null
  const m = out.match(/^(\d+)\s+(\S+)\s+(\d+)\s+([\s\S]+)$/)
  if (!m) return null
  return { uid: Number(m[1]), state: m[2], ppid: Number(m[3]), cmd: m[4] }
}

function pidsForPort(port) {
  const list = new Set(
    sh(["lsof", "-t", "-i", `:${port}`])
      .split("\n")
      .map((s) => parseInt(s, 10))
      .filter((n) => Number.isInteger(n) && n > 0),
  )
  if (list.size === 0) {
    const ss = sh(["ss", "-ltnp"])
    const re = new RegExp(`:${port}\\b[\\s\\S]*?pid=(\\d+)`, "g")
    let mm
    while ((mm = re.exec(ss))) list.add(parseInt(mm[1], 10))
  }
  return [...list]
}

// 解析 kill/pkill/killall/fuser 命令指向的 PID 集合（pid -> 来源描述）。
function resolveKillPids(cmd) {
  const pids = new Map()
  const add = (found, how) => found.forEach((p) => pids.set(p, how))

  // 1) kill [-signal] pid...
  const killRe = /\bkill\b((?:\s+-\S+)*)\s+((?:-?\d+\s*)+)/g
  let m
  while ((m = killRe.exec(cmd))) {
    const sig = m[1] || ""
    if (/\B-l\b|\B-L\b/.test(sig)) continue // kill -l 仅列信号，无目标
    m[2]
      .trim()
      .split(/\s+/)
      .map(Number)
      .forEach((n) => {
        if (n > 0) pids.set(n, `kill pid ${n}`)
      })
  }

  // 2) killall [-signal] [-u user] name
  const kaRe = /\bkillall\b((?:\s+-\S+)*)\s+(\S+)/g
  while ((m = kaRe.exec(cmd))) {
    const name = m[2]
    const found = sh(["pgrep", "-x", name])
      .split("\n")
      .map((s) => parseInt(s, 10))
      .filter((n) => Number.isInteger(n) && n > 0)
    add(found, `killall ${name}`)
  }

  // 3) pkill [-f|-x|-u ...] pattern
  const pkRe = /\bpkill\b((?:\s+-\S+)*)\s+(\S+)/g
  while ((m = pkRe.exec(cmd))) {
    const pat = m[2]
    const opts = []
    if (/-f/.test(m[1] || "")) opts.push("-f")
    if (/-x/.test(m[1] || "")) opts.push("-x")
    const found = sh(["pgrep", ...opts, pat])
      .split("\n")
      .map((s) => parseInt(s, 10))
      .filter((n) => Number.isInteger(n) && n > 0)
    add(found, `pkill ${pat}`)
  }

  // 4) 按端口释放：lsof -i:PORT / fuser PORT/tcp / kill $(lsof ...)
  const portRe = /(?:lsof[^|;&]*?-i[:\s]+(\d{2,5})|fuser[^|;&]*?(\d{2,5})(?:\/(?:tcp|udp))?)/g
  const ports = new Set()
  let pm
  while ((pm = portRe.exec(cmd))) {
    const port = Number(pm[1] || pm[2])
    if (port > 0) ports.add(port)
  }
  for (const port of ports) add(pidsForPort(port), `port ${port}`)

  return pids
}

async function evaluateKill(cmd) {
  const targets = resolveKillPids(cmd)
  if (targets.size === 0) return null // 无明确目标（如 kill -l）→ 放行

  // 大范围模式（如 pkill node 命中数百）：保守拦截，避免误杀 / 慢检查
  if (targets.size > 200) {
    return `kill 目标过多（${targets.size} 个），疑似大范围模式，已拦截。请缩小范围或确认 PID。`
  }

  const ancestors = ancestorPids()
  let selfUid = -1
  try {
    selfUid = process.getuid()
  } catch {
    /* 无 getuid（如 Windows）→ 视为未知，走保守分支 */
  }

  const criticalHits = []
  const otherHits = []

  for (const [pid, how] of targets) {
    if (pid <= 1) {
      criticalHits.push(`${pid}(init/系统)`)
      continue
    }
    if (ancestors.has(pid)) {
      criticalHits.push(`${pid}(自身/祖先进程)`)
      continue
    }
    const info = procInfo(pid)
    if (!info) continue // 已死 → 无需要保护的对象
    if (CRITICAL_RE.some((re) => re.test(info.cmd))) {
      criticalHits.push(`${pid}(环境连接: ${info.cmd.slice(0, 70)})`)
      continue
    }
    if (info.uid === selfUid) {
      // 同一身份启的服务/端口：无条件允许释放
      continue
    }
    otherHits.push({ pid, info })
  }

  if (criticalHits.length) {
    return `触及环境连接进程，已拦截: ${criticalHits.join(", ")}`
  }

  // 其他身份/未知：先检查再决定
  for (const { pid, info } of otherHits) {
    if (info.state === "Z" || info.state === "X" || info.state === "x") {
      continue // 已死(zombie)→ 允许回收
    }
    let responsive = true
    try {
      process.kill(pid, 0) // 仅探活，不真正发信号
    } catch {
      responsive = false // 无响应/无权限/已死 → 允许释放
    }
    if (!responsive) continue
    // 仍存活且非本身份：无法确定与项目服务的关系，保守拦截，避免误杀他人进程
    return `目标 ${pid} 为其他身份存活进程(${info.cmd.slice(0, 60)})，存在误杀风险，已拦截。请确认 PID 或用更精确命令。`
  }

  return null // 放行
}

export async function guardAppExec(cmd) {
  for (const { label, re } of APP_EXEC_DENY) {
    if (re.test(cmd)) return label
  }
  if (/\b(kill|pkill|killall|fuser)\b/.test(cmd)) {
    const blocked = await evaluateKill(cmd)
    if (blocked) return blocked
  }
  return null
}
