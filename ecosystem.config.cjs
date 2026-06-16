/* eslint-disable */
// =============================================================================
// ecosystem.config.cjs — 方案 C：网关合一
// -----------------------------------------------------------------------------
// pm2 配置：精简为 2 个 app。
//   ① preview-gateway   (:16666) — 唯一对外入口 + 唯一进程管理者
//      内部 child_process.spawn 管理 4 个子进程：
//        - encv-go         (:2025)  SPAWN_GO=1 (default)
//        - encv-mobile-vite(:8100)  SPAWN_VITE=1 (default)
//        - plugin-vite     (:5174)  SPAWN_PLUGIN_VITE=0 (按需)
//        - openlist        (:5244)  SPAWN_OPENLIST=0 (按需)
//   ② openpreview-stub  (:15003) — OpenPreview 工具垫脚石
//      （agent-tool-host 要求 command_id 来自 web_server 类型命令；
//        本服务纯返 200 OK，真实预览仍走 :16666）
//
// 历史变更（2026-06-08 方案 C 大改）：
//   - 删除 start-preview（amalgamated 巨脚本，子进程已下沉到 gateway）
//   - 删除 encv-mobile-vite（端口冲突源；现由 gateway 独占 :8100）
//   - 删除 plugin-openlist-vite / openlist（现由 gateway 按需拉起）
//   - 删除 preview-helper（与 openpreview-stub 功能完全重复）
//   - ENCV_DEV_PREVIEW / ENCV_MOBILE env 注入：start-preview.sh → gateway 的 air 子进程
//   - .air-run.sh 的 `:-1` 兜底保留（防御性深度）
//
// 用法：
//   pm2 start /workspace/ecosystem.config.cjs          # 启全部（2 个）
//   pm2 restart preview-gateway                        # 改 gateway 配置
//   pm2 restart openpreview-stub                       # 改 stub
//   pm2 stop all && pm2 delete all                     # 全清
//   pm2 save && pm2 resurrect                          # 跨会话持久化
//
// 端口决策（spec/unify-sandbox-preview-port §D1-D9）：
//   :16666 = preview-gateway 唯一对外端口
//   :8100  = encv-mobile Vite（gateway 内部子进程）
//   :5174  = plugin-openlist-web Vite（按需）
//   :2025  = encv-go（gateway 内部子进程）
//   :5244  = OpenList fork（按需）
//   :15003 = openpreview-stub（OpenPreview 工具垫脚石）
// =============================================================================

const path = require('path');
const fs = require('fs');

const REPO_ROOT     = '/workspace';
const MOBILE_DIR    = path.join(REPO_ROOT, 'app', 'encv-mobile');
const GATEWAY_DIR   = path.join(REPO_ROOT, 'app', 'preview-gateway');

const GATEWAY_SCRIPT = path.join(GATEWAY_DIR, 'dist', 'server.js');

// 启动前 fail-fast：dist 必须存在
if (!fs.existsSync(GATEWAY_SCRIPT)) {
  throw new Error(
    `preview-gateway dist/server.js 不存在: ${GATEWAY_SCRIPT}\n` +
    `请先运行 setup-sandbox-env.sh（或 cd ${GATEWAY_DIR} && pnpm install && pnpm build）`,
  );
}

module.exports = {
  apps: [
    // ── ① preview-gateway (:16666) ───────────────────────────────────
    // 方案 C 核心：单进程 = 单入口 = 单一管理。
    // gateway 内部 spawn air / vite / plugin-vite / openlist，
    // 任何子进程死 → gateway 退出 → pm2 重启整套。
    {
      name: 'preview-gateway',
      script: GATEWAY_SCRIPT,
      interpreter: 'node',
      cwd: GATEWAY_DIR,
      env: {
        PATH: process.env.PATH,
        PORT: '16666',
        HOST: '0.0.0.0',
        // ── 子进程开关（方案 C） ──
        SPAWN_GO: '1',          // air → encv-go (:2025)，mobile overlay 关键
        SPAWN_VITE: '1',        // encv-mobile Vite (:8100)
        SPAWN_PLUGIN_VITE: '0', // plugin-openlist-web Vite (:5174)，按需 1
        SPAWN_OPENLIST: '0',    // OpenList fork (:5244)，按需 1
        // ── env 注入铁律（L1 pm2 注入） ──
        // 透传给 air 子进程，触发 ApplyMobileOverlay
        // (internal/config/config.go:292-294)
        ENCV_DEV_PREVIEW: '1',
        ENCV_MOBILE: '1',
        // ── mobile-overlay 数据根（service-guard 硬约束）──
        // 🆕 2026-06-14 修复：service-guard (mobile_api.go:210) 硬编码 /storage/emulated/0，
        //   preflight.ensureMockData 必须建这个目录。paths.ts 拆 mobileDir / mobileDataDir：
        //   - mobileDir = app/encv-mobile（vite cwd，必须存在 node_modules）
        //   - mobileDataDir = /storage/emulated/0（mobile 真机/dev preview 标准挂载点）
        MOBILE_DATA_DIR: '/storage/emulated/0',
        MOBILE_DIR: MOBILE_DIR,
        // ── air binary 路径（gateway 子进程要 spawn air 监视 encv-go）──
        // 🆕 2026-06-14 修复：原路径 `/root/.local/share/mise/installs/go/1.25.1/bin/air`
        //   在沙箱里不存在（mise 目录有但 air 没装到那里）。setup-sandbox-env.sh
        //   用 `go install github.com/air-verse/air@latest` 装到 GOPATH/bin（/root/go/bin）。
        //   验证：which air → /root/go/bin/air
        AIR_BIN: '/root/go/bin/air',
      },
      // 网关本体轻量；子进程会跑出来几百 MB（Go 编译 + Vite + node_modules）
      max_memory_restart: '256M',
      listen_timeout: 600_000,  // 沙箱首次冷编 encv-go 需 3-5 分钟（go mod + 全量 build）
      kill_timeout: 10_000,    // stopAll() 给 5s grace + 兜底
      autorestart: true,
      max_restarts: 10,
      min_uptime: '30s',
      out_file: '/tmp/pm2-preview-gateway.log',
      error_file: '/tmp/pm2-preview-gateway.err.log',
      merge_logs: true,
      time: true,
    },

    // ── ② openpreview-stub (:15003) ─────────────────────────────────
    // OpenPreview 工具的"垫脚石"：仅用于让工具拿到 web_server 类型 command_id。
    // 真实预览仍走 :16666 preview-gateway，本服务纯返 200 OK。
    // ⚠️ 不能删除：agent-tool-host 要求 command_id 来自 web_server 命令（详见
    //    .trae/rules/trae_web_sandbox_network.md §八）。
    {
      name: 'openpreview-stub',
      script: path.join(REPO_ROOT, 'scripts', 'openpreview-stub.js'),
      interpreter: 'node',
      cwd: path.join(REPO_ROOT, 'scripts'),
      env: { PATH: process.env.PATH, PORT: '15003' },
      max_memory_restart: '64M',
      listen_timeout: 5_000,
      kill_timeout: 2_000,
      autorestart: true,
      max_restarts: 10,
      out_file: '/tmp/pm2-openpreview-stub.log',
      error_file: '/tmp/pm2-openpreview-stub.err.log',
    },
  ],

  deploy: {},
};
