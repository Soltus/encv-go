/**
 * ChildrenManager — preview-gateway 的子进程编排器
 * =====================================================
 *
 * 为什么：方案 C「网关合一」要求 preview-gateway 内部管理 encv-go (air)、
 * encv-mobile-vite、plugin-openlist-vite、openlist 四个子进程，外部 pm2 只
 * 监管 preview-gateway + openpreview-stub 两个 app。
 *
 * 设计原则：
 *   1. **失败即崩**：子进程死掉 → 整个 gateway 退出 → pm2 重启整套（避免
 *      出现「vite 死、Go 活、gateway 200、用户看到白屏」的鬼状态）
 *   2. **就绪再标 OK**：每个子进程通过 HTTP 探活确认 ready，才向用户暴露
 *   3. **优雅停机**：SIGINT/SIGTERM 时先 SIGTERM 子进程，5s 后 SIGKILL 兜底
 *   4. **env 全开**：所有路径/参数可通过 env 覆盖，默认走仓库结构
 *
 * 不要在此文件写任何 vite/air 业务配置 — 配置来自 caller（server.ts）。
 */

import { type ChildProcess, type StdioOptions, spawn } from "node:child_process";
import { existsSync } from "node:fs";
import http from "node:http";

const LOG_PREFIX = "[children]";

function log(...args: unknown[]): void {
  console.log(LOG_PREFIX, ...args);
}

function logChild(name: string, data: Buffer): void {
  // 子进程 stdout/stderr 加上 [name] 前缀便于 DevLogs 过滤
  const lines = data.toString("utf-8").split("\n");
  for (const line of lines) {
    if (line.length === 0) continue;
    console.log(`[child:${name}] ${line}`);
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms));
}

/**
 * 子进程探活：发 GET 到 url，2xx/3xx/4xx 算 alive（5xx 算 dead）。
 * timeout 严格控制 1500ms，避免 health 拖死整个 readiness 循环。
 */
function pingHttp(url: string, timeoutMs = 1500): Promise<boolean> {
  return new Promise(resolve => {
    let settled = false;
    const finish = (v: boolean): void => {
      if (settled) return;
      settled = true;
      resolve(v);
    };
    try {
      const req = http.request(url, { method: "GET", timeout: timeoutMs }, res => {
        // 必须 drain，否则 socket 不会释放
        res.resume();
        const code = res.statusCode ?? 0;
        finish(code < 500);
      });
      req.on("error", () => finish(false));
      req.on("timeout", () => {
        req.destroy();
        finish(false);
      });
      req.end();
    } catch {
      finish(false);
    }
  });
}

export interface ChildSpec {
  /** 人类可读名称，用于日志/状态/health 端点 */
  name: string;
  /** 可执行文件绝对路径（不要依赖 PATH，env 注入式） */
  cmd: string;
  /** 参数列表 */
  args: string[];
  /** 子进程 env（merge 到 process.env 还是完全替换由 caller 决定） */
  env: NodeJS.ProcessEnv;
  /** 子进程 cwd */
  cwd: string;
  /** 就绪探活 URL（不设 = spawn 后立即返回，由 caller 自己 sleep） */
  readyUrl?: string;
  /** 就绪超时（默认 60s，encv-go 首次 go build 可能 90s） */
  readyTimeoutMs?: number;
  /** 健康检查间隔（默认 500ms） */
  readyPollMs?: number;
  /** 子进程死掉是否让 gateway 退出（默认 true） */
  exitOnDeath?: boolean;
}

export interface ChildStatus {
  name: string;
  pid?: number;
  alive: boolean;
  ready: boolean;
  readyUrl?: string;
  exitCode?: number | null;
  signal?: NodeJS.Signals | null;
  spawnedAt: number;
}

export class ChildrenManager {
  private readonly children = new Map<string, ChildProcess>();
  private readonly statuses = new Map<string, ChildStatus>();
  private stopping = false;

  /**
   * 按顺序启动所有子进程（串行）。每个就绪后才启下一个 — 减少并发压力，
   * 也避免 :2025 还没起就被 :8100 抢资源。
   */
  async startAll(specs: ChildSpec[]): Promise<void> {
    log(`starting ${specs.length} child(ren) in order: ${specs.map(s => s.name).join(", ") || "(none)"}`);
    for (const spec of specs) {
      await this.startOne(spec);
    }
    log(`all ${specs.length} child(ren) ready`);
  }

  private async startOne(spec: ChildSpec): Promise<void> {
    if (this.children.has(spec.name)) {
      throw new Error(`child '${spec.name}' already started`);
    }
    if (!existsSync(spec.cmd)) {
      throw new Error(`child '${spec.name}': cmd not found: ${spec.cmd}`);
    }
    log(`[${spec.name}] spawn: ${spec.cmd} ${spec.args.join(" ")} (cwd=${spec.cwd})`);

    const stdio: StdioOptions = ["ignore", "pipe", "pipe"];
    const child = spawn(spec.cmd, spec.args, {
      env: spec.env,
      cwd: spec.cwd,
      stdio,
      // detached: false — 让子进程与 gateway 同进程组，SIGINT/SIGTERM 一并收到
    });

    this.children.set(spec.name, child);
    this.statuses.set(spec.name, {
      name: spec.name,
      pid: child.pid,
      alive: true,
      ready: spec.readyUrl === undefined, // 没 readyUrl 默认算 ready
      readyUrl: spec.readyUrl,
      spawnedAt: Date.now(),
    });

    child.stdout?.on("data", data => logChild(spec.name, data as Buffer));
    child.stderr?.on("data", data => logChild(spec.name, data as Buffer));

    child.on("exit", (code, signal) => {
      const status = this.statuses.get(spec.name);
      if (status) {
        status.alive = false;
        status.exitCode = code;
        status.signal = signal;
      }
      log(`[${spec.name}] exited code=${code ?? "null"} signal=${signal ?? "null"}`);
      if (this.stopping) return; // graceful shutdown 不触发
      if ((spec.exitOnDeath ?? true) && child === this.children.get(spec.name)) {
        log(`[${spec.name}] died unexpectedly — gateway exiting (pm2 will restart the whole stack)`);
        // 给 stderr/stdout flush 一点时间
        setTimeout(() => process.exit(1), 200);
      }
    });

    child.on("error", err => {
      log(`[${spec.name}] spawn error: ${err.message}`);
    });

    if (spec.readyUrl) {
      await this.waitForReady(spec.name, spec.readyUrl, spec.readyTimeoutMs ?? 60000, spec.readyPollMs ?? 500);
    }
  }

  private async waitForReady(name: string, url: string, timeoutMs: number, pollMs: number): Promise<void> {
    const start = Date.now();
    let attempt = 0;
    while (Date.now() - start < timeoutMs) {
      attempt += 1;
      const status = this.statuses.get(name);
      if (!status?.alive) {
        throw new Error(`[${name}] died before becoming ready`);
      }
      const ok = await pingHttp(url);
      if (ok) {
        if (status) status.ready = true;
        log(`[${name}] ready at ${url} (${Date.now() - start}ms, ${attempt} attempts)`);
        return;
      }
      await sleep(pollMs);
    }
    throw new Error(`[${name}] not ready after ${timeoutMs}ms: ${url}`);
  }

  /** 当前所有子进程状态（health 端点用） */
  getStatuses(): ChildStatus[] {
    return Array.from(this.statuses.values()).map(s => ({ ...s }));
  }

  /** 优雅停机：SIGTERM → 5s 后 SIGKILL → 退出 */
  async stopAll(): Promise<void> {
    if (this.stopping) return;
    this.stopping = true;
    log(`stopping ${this.children.size} child(ren) (SIGTERM, 5s grace)`);
    for (const [name, child] of this.children) {
      try {
        child.kill("SIGTERM");
        log(`[${name}] SIGTERM sent (pid=${child.pid})`);
      } catch (err) {
        log(`[${name}] SIGTERM failed: ${(err as Error).message}`);
      }
    }
    // 等 5s 让子进程优雅退出
    const deadline = Date.now() + 5000;
    while (Date.now() < deadline) {
      const anyAlive = Array.from(this.children.values()).some(c => c.exitCode === null && c.signalCode === null);
      if (!anyAlive) break;
      await sleep(100);
    }
    // 兜底 SIGKILL
    for (const [name, child] of this.children) {
      if (child.exitCode === null && child.signalCode === null) {
        try {
          child.kill("SIGKILL");
          log(`[${name}] SIGKILL (grace expired)`);
        } catch (err) {
          log(`[${name}] SIGKILL failed: ${(err as Error).message}`);
        }
      }
    }
    log(`all children stopped`);
  }
}
