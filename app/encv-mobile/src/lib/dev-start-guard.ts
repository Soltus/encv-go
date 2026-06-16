/**
 * dev-start-guard.ts
 *
 * Vite plugin：dev 模式启动守卫，强制走 PM2 → preview-gateway 链路。
 *
 * ⚠️ 触发条件（必须**同时**满足才抛错）：
 *   ① env.command === 'serve'  （build 模式永远不抛 — 产线打包任何时候都应可执行）
 *   ② SPAWN_VITE !== '1'        （非 preview-gateway spawn）
 *   ③ !PM2_HOME                （非 PM2 进程树）
 *   ④ !PPA_SPAWNED              （非 PPA 子进程）
 *
 * 唯一合法链路：
 *   pm2 start ecosystem.config.cjs
 *     → preview-gateway (spawn vite with SPAWN_VITE=1 env)
 *       → vite 正常启动
 *
 * 任何绕过方式（CI=true、nohup vite、pnpm exec vite 等）一律视为非法启动。
 *
 * 历史（2026-06-15）收编：原版有 5 个条件（含 CI / PPA_SPAWNED），
 *   出现"CI 跑 dev"和"用户 PPA 包装后直接 vite"两类绕过 → 收紧为 3 个。
 *   唯一权威 = PM2 进程树。CI 永远不应跑 vite dev（应跑 build/lint/test）。
 */

import type { Plugin } from 'vite'

export interface DevStartGuardOptions {
  /** 自定义错误信息（测试可注入） */
  errorMessage?: string
}

export function devStartGuard(opts: DevStartGuardOptions = {}): Plugin {
  return {
    name: 'dev-start-guard',
    config(_config, env) {
      // ① build 模式直接跳过 — 产线打包任何时候都应可执行
      if (env?.command !== 'serve') return

      // ② preview-gateway spawn 合法
      if (process.env.SPAWN_VITE === '1') return

      // ③ PM2 管理下合法（PM2_HOME 由 agent-tool-host 或 pm2 daemon 设）
      const isPm2 = !!process.env.PM2_HOME

      // ④ 唯一权威 = PM2 进程树。其他一切（CI / PPA_SPAWNED / nohup / bash -c）一律拒绝
      if (!isPm2) {
        const msg = opts.errorMessage ?? DEFAULT_ERROR_MESSAGE
        throw new Error(msg)
      }
    },
  }
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
`.trim()
