/**
 * dev-start-guard.ts
 *
 * Vite plugin：dev 模式启动守卫，强制走 PM2 → preview-gateway 链路。
 *
 * ⚠️ 触发条件（必须**同时**满足才抛错）：
 *   ① env.command === 'serve'  （build 模式永远不抛 — 产线打包任何时候都应可执行）
 *   ② SPAWN_VITE !== '1'        （非 preview-gateway spawn）
 *   ③ !PM2_HOME                （非 PM2 进程树）
 *   ④ !ENCV_STANDALONE_VITE    （非"独立 vite dev"显式授权）
 *
 * 合法链路（两条）：
 *   A. 沙箱（trae / OpenPreview）：
 *        pm2 start ecosystem.config.cjs
 *          → preview-gateway(:16666) → spawn vite(:8100) with SPAWN_VITE=1
 *   B. 非沙箱（CNB / 本机 / 任何没有 preview-gateway 的环境，2026-10-04 新增）：
 *        ENCV_STANDALONE_VITE=1 vite        ← 必须显式声明，仍不允许"裸 vite"
 *
 * 任何绕过方式（CI=true、nohup vite、pnpm exec vite 等）一律视为非法启动。
 *
 * 历史（2026-06-15）收编：原版有 5 个条件（含 CI / PPA_SPAWNED），
 *   出现"CI 跑 dev"和"用户 PPA 包装后直接 vite"两类绕过 → 收紧为 3 个。
 *   唯一权威 = PM2 进程树。CI 永远不应跑 vite dev（应跑 build/lint/test）。
 * 历史（2026-10-04）：preview-gateway 是**沙箱专用**链路（:16666 是沙箱统一入口）。
 *   在 CNB 这类非 trae 环境里根本没有网关，前端 dev 态默认把 API 打到 :16666
 *   ⇒ 一定断联。故新增 ENCV_STANDALONE_VITE=1 授权位（仍要求显式声明，
 *   不是放开裸 vite），由 vite.config.ts 在该模式下启用 /api 同源反代。
 */

import type { Plugin } from "vite";

export interface DevStartGuardOptions {
  /** 自定义错误信息（测试可注入） */
  errorMessage?: string;
}

export function devStartGuard(opts: DevStartGuardOptions = {}): Plugin {
  return {
    name: "dev-start-guard",
    config(_config, env) {
      // ① build 模式直接跳过 — 产线打包任何时候都应可执行
      if (env?.command !== "serve") return;

      // ② preview-gateway spawn 合法（沙箱链路）
      if (process.env.SPAWN_VITE === "1") return;

      // ②' 独立 vite dev 合法（非沙箱：无 preview-gateway，2026-10-04）
      //     仍需显式声明，避免退化成"谁都能裸跑 vite"
      if (process.env.ENCV_STANDALONE_VITE === "1") return;

      // ③ PM2 管理下合法（PM2_HOME 由 agent-tool-host 或 pm2 daemon 设）
      const isPm2 = !!process.env.PM2_HOME;

      // ④ 唯一权威 = PM2 进程树。其他一切（CI / PPA_SPAWNED / nohup / bash -c）一律拒绝
      if (!isPm2) {
        const msg = opts.errorMessage ?? DEFAULT_ERROR_MESSAGE;
        throw new Error(msg);
      }
    },
  };
}

const DEFAULT_ERROR_MESSAGE = `
╔══════════════════════════════════════════════════════════╗
║  [dev-start-guard] 检测到非法启动方式！立即终止。        ║
╠══════════════════════════════════════════════════════════╣
║                                                          ║
║  ❌ 你正在直接运行 vite / npm run dev / pnpm exec vite   ║
║     这在本项目中是非法的。                               ║
║                                                          ║
║  唯一合法链路：                                            ║
║    pm2 start /workspace/ecosystem.config.cjs              ║
║      → preview-gateway(:16666)                            ║
║        → spawn vite(:8100) with SPAWN_VITE=1             ║
║                                                          ║
║  ❌ 非法绕过方式（已收紧，2026-06-15）：                  ║
║    - CI=true vite / pnpm exec vite                       ║
║    - nohup vite / bash -c 'vite'                         ║
║    - PPA_SPAWNED=1 包装后再 vite                          ║
║    - 直接 go run ./cmd/encv/ start 启后端                ║
║                                                          ║
║  没有 pm2 / air → 装；不要绕过本守卫。                   ║
║                                                          ║
║  预览地址：http://localhost:16666/                        ║
╚══════════════════════════════════════════════════════════╝
`.trim();
