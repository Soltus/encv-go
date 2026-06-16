#!/usr/bin/env node
/**
 * dev-start-guard.js — 开发环境启动守卫
 *
 * ⚠️ 防御机制：禁止直接 `vite` / `npm run dev` / `node vite` 等裸启动方式。
 *
 * 本项目必须通过 PM2 + preview-gateway 启动，原因：
 *   1. preview-gateway(:16666) 是唯一对外入口，负责子进程管理（vite:8100, air:2025 等）
 *   2. 直接 vite 启动不会加载 ENCV_DEV_PREVIEW / ENCV_MOBILE 等 env 注入
 *   3. Vite 会自动扫描 plugin-openlist/web/index.html 导致找不到 src/views/ 下的文件
 *   4. HMR 在沙箱环境需要 gateway 的 dynamicHmrHostPlugin 透传
 *
 * 正确用法：
 *   pm2 start /workspace/ecosystem.config.cjs          # 启动全部
 *   pm2 restart preview-gateway                        # 重启网关
 *   pm2 logs preview-gateway --lines 50                 # 查看日志
 */

const { spawn } = require('child_process')
const path = require('path')
const fs = require('fs')

// ==================== 检测非法启动方式 ====================

const ILLEGAL_PATTERNS = [
  // 直接调用 vite 二进制
  /(?:^|\/)vite(?:\.js)?$/,
  // npm run dev (会调用 vite)
  /npm(?:\.cmd)?\s+run\s+dev\b/,
  // npx vite
  /npx(?:\.cmd)?\s+(?:--yes\s+)?vite\b/,
  // node_modules/.bin/vite
  /node_modules[\\/].bin[\\/]vite/,
]

function detectIllegalLaunch() {
  const rawArgv = process.argv.slice(1).join(' ')
  const execName = path.basename(process.argv[1] || '').toLowerCase()

  for (const pattern of ILLEGAL_PATTERNS) {
    if (pattern.test(execName) || pattern.test(rawArgv)) {
      return {
        illegal: true,
        reason: `检测到非法启动方式: "${process.argv.slice(2).join(' ') || 'vite'}"`,
        detail: [
          '',
          '═══════════════════════════════════════════════',
          '  本项目禁止直接使用 vite / npm run dev 启动！',
          '═══════════════════════════════════════════════',
          '',
          '原因：',
          '  ① preview-gateway(:16666) 是唯一对外入口，内部管理子进程',
          '  ② 直接 vite 不会注入 ENCV_DEV_PREVIEW / ENCV_MOBILE env',
          '  ③ Vite 自动扫描 plugin-openlist/index.html → 找不到文件报错',
          '  ④ HMR 需要 gateway 的 dynamicHmrHostPlugin 透传 Host 头',
          '',
          '正确启动方式：',
          '  pm2 start /workspace/ecosystem.config.cjs',
          '',
          '或（如果只需重启）：',
          '  pm2 restart preview-gateway && pm2 logs preview-gateway --lines 20',
          '',
          '预览地址：http://localhost:16666/',
          '═══════════════════════════════════════════════',
        ].join('\n'),
      }
    }
  }

  // 额外检查：是否从 ecosystem.config.cjs 的子进程链路启动
  // SPAWN_VITE=1 表示是 gateway spawn 出来的，合法
  if (process.env.SPAWN_VITE === '1') {
    return { illegal: false }
  }

  // PM2 进程树在管 = 合法（agent-tool-host 或 pm2 daemon 设 PM2_HOME）
  if (process.env.PM2_HOME) {
    return { illegal: false }
  }

  // 唯一权威 = PM2 进程树。其他一切（含 PPA_SPAWNED）一律视为非法
  // 2026-06-15 收紧：原版"只警告不阻断"→ 改为 exit 1
  return {
    illegal: true,
    reason: '未检测到 PM2_HOME 或 SPAWN_VITE 环境标记',
    detail: [
      '',
      '═══════════════════════════════════════════════',
      '  本项目禁止绕过 PM2 → preview-gateway 链路启动！',
      '═══════════════════════════════════════════════',
      '',
      '原因：',
      '  ① preview-gateway(:16666) 是唯一对外入口，内部管理子进程',
      '  ② 直接 vite 不会注入 ENCV_DEV_PREVIEW / ENCV_MOBILE env',
      '  ③ Vite 自动扫描 plugin-openlist/index.html → 找不到文件报错',
      '  ④ HMR 需要 gateway 的 dynamicHmrHostPlugin 透传 Host 头',
      '',
      '❌ 非法绕过方式（2026-06-15 收紧）：',
      '  - CI=true 绕开 dev-start-guard',
      '  - PPA_SPAWNED=1 包装后再 vite',
      '  - nohup / bash -c / tmux 包装',
      '  - 直接 go run ./cmd/encv/ start 启后端',
      '',
      '✅ 正确启动方式：',
      '  pm2 start /workspace/ecosystem.config.cjs',
      '',
      '预览地址：http://localhost:16666/',
      '═══════════════════════════════════════════════',
    ].join('\n'),
  }
}

// ==================== 主逻辑 ====================

const result = detectIllegalLaunch()

if (result.illegal) {
  console.error('\n' + result.detail + '\n')
  process.exit(1)
}

if (result.warning) {
  console.warn(`[dev-start-guard] ${result.warning}`)
}

// 合法启动：什么都不做，让原始命令继续执行
