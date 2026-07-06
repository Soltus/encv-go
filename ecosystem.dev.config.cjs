/* eslint-disable */
/* =============================================================================
 * ecosystem.dev.config.cjs — 开发调试专用配置
 * -----------------------------------------------------------------------------
 * ⚠️ 重要警告：禁止使用 pm2 delete all / pkill -9 杀死进程！
 *
 * 本配置启动 preview-gateway (:16666)，它会通过 ChildrenManager
 * 自动管理子进程（encv-go :2025、encv-mobile-vite :8100、
 * simverse-frontend :8200）。
 *
 * 杀死父进程会导致子进程变成僵尸进程（defunct），占用端口资源
 * 造成后续无法重启。
 *
 * 正确操作：
 *   ✅ pm2 restart preview-gateway  (重启网关及子进程)
 *   ✅ pm2 restart all              (重启所有服务)
 *   ❌ pm2 delete all             (会杀不死子进程，造成僵尸)
 *   ❌ pkill -9                   (强制杀死导致僵尸)
 *
 * 如果遇到端口占用：
 *   1. 检查僵尸进程: ps aux | grep defunct
 *   2. 杀死僵尸父进程: kill -9 <父PID>
 *   3. 然后重启: pm2 restart preview-gateway
 *
 * 使用方法:
 *   pm2 start ecosystem.dev.config.cjs
 *   然后在 VSCode 中选择 "PM2: preview-gateway" 进行调试
 * ============================================================================= */

const path = require('path');
const fs = require('fs');

const REPO_ROOT = '/workspace';
const MOBILE_DIR = path.join(REPO_ROOT, 'app', 'encv-mobile');
const GATEWAY_DIR = path.join(REPO_ROOT, 'app', 'preview-gateway');

const GATEWAY_SCRIPT = path.join(GATEWAY_DIR, 'dist', 'server.js');

// 如果 node_modules 不存在，先安装依赖
const WORKSPACE_ROOT = path.join(REPO_ROOT, 'app');
if (!fs.existsSync(path.join(GATEWAY_DIR, 'node_modules'))) {
  console.log('[PM2] preview-gateway node_modules 不存在，先安装依赖...');
  const { execSync } = require('child_process');
  try {
    execSync('pnpm install', { cwd: WORKSPACE_ROOT, stdio: 'inherit' });
    console.log('[PM2] pnpm workspace 依赖安装完成');
  } catch (e) {
    console.error('[PM2] pnpm install 失败:', e.message);
  }
}

// 启动前 fail-fast（dist 必须存在）
if (!fs.existsSync(GATEWAY_SCRIPT)) {
  throw new Error(
    `preview-gateway dist/server.js 不存在: ${GATEWAY_SCRIPT}\n` +
    `请先运行: cd ${GATEWAY_DIR} && pnpm install && pnpm build`
  );
}

// 环境变量合并函数
function mergeEnv(base = {}, overrides = {}) {
  return { ...base, ...overrides };
}

// 构建基础 PATH - 确保包含 Go 路径
function getBasePath() {
  let paths = [
    process.env.PATH || '',
    '/root/go/bin',
    '/root/.local/share/mise/installs/go/1.25.1/bin',
    '/usr/local/go/bin',
    '/root/.local/share/mise/installs/go/latest/bin',
  ];
  // 去重
  let seen = new Set();
  let result = [];
  for (let p of paths.join(':').split(':')) {
    if (p && !seen.has(p)) {
      seen.add(p);
      result.push(p);
    }
  }
  return result.join(':');
}

const basePath = getBasePath();

module.exports = {
  apps: [
    // ── ① preview-gateway (:16666) — 开发调试模式 ─────────────────────
    {
      name: 'preview-gateway',
      // 使用 node --inspect 启动，支持 VSCode 附加调试
      script: GATEWAY_SCRIPT,
      interpreter: 'none',  // 不使用解释器，直接用 node 运行
      node_args: '--inspect=9229',  // 调试端口 9229
      cwd: GATEWAY_DIR,
      env: mergeEnv(process.env, {
        PATH: basePath,
        PORT: '16666',
        HOST: '0.0.0.0',
        // 子进程开关
        SPAWN_GO: '1',
        SPAWN_VITE: '1',
        SPAWN_PLUGIN_VITE: '0',
        SPAWN_OPENLIST: '0',
        // 环境变量
        ENCV_DEV_PREVIEW: '1',
        ENCV_MOBILE: '1',
        MOBILE_DATA_DIR: '/storage/emulated/0',
        MOBILE_DIR: MOBILE_DIR,
        AIR_BIN: '/root/go/bin/air',
      }),
      max_memory_restart: '256M',
      listen_timeout: 600_000,
      kill_timeout: 10_000,
      autorestart: true,
      max_restarts: 10,
      min_uptime: '30s',
      out_file: '/tmp/pm2-preview-gateway.log',
      error_file: '/tmp/pm2-preview-gateway.err.log',
      merge_logs: true,
      time: true,
    },

    // ── ② openpreview-stub (:15003) — 开发调试模式 ───────────────────
    {
      name: 'openpreview-stub',
      script: path.join(REPO_ROOT, 'scripts', 'openpreview-stub.js'),
      interpreter: 'none',
      node_args: '--inspect=9230',  // 调试端口 9230
      cwd: path.join(REPO_ROOT, 'scripts'),
      env: mergeEnv(process.env, {
        PATH: basePath,
        PORT: '15003'
      }),
      max_memory_restart: '64M',
      listen_timeout: 5_000,
      kill_timeout: 2_000,
      autorestart: true,
      max_restarts: 10,
      out_file: '/tmp/pm2-openpreview-stub.log',
      error_file: '/tmp/pm2-openpreview-stub.err.log',
      merge_logs: true,
      time: true,
    },
  ],

  deploy: {},
};