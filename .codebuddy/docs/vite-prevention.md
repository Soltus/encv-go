# Vite 开发服务器防暂停指南

## 问题
Ctrl+Z 会暂停（挂起）Vite 进程，导致：
- 端口仍被监听，但请求无响应
- 需要手动 `kill` 或 `fg` 恢复

## 🛡️ 永久解决方案：使用 pm2（推荐）

### 1. 安装 pm2
```bash
pnpm add -g pm2
```

### 2. 创建 pm2 配置文件
项目根目录创建 `ecosystem.config.cjs`：
```javascript
module.exports = {
  apps: [
    {
      name: 'simverse-frontend',
      script: 'pnpm',
      args: 'dev',
      cwd: '/path/to/simverse-frontend',
      interpreter: 'none',
      exec_mode: 'fork',
      watch: false,
      autorestart: true,
      max_restarts: 10,
      min_uptime: 5000,
      max_memory_restart: '1G',
      env: { NODE_ENV: 'development' },
      error_file: '/tmp/simverse-vite-error.log',
      out_file: '/tmp/simverse-vite-out.log',
    },
  ],
};
```

### 3. 启动/管理命令
```bash
# 启动
pm2 start ecosystem.config.cjs

# 查看状态
pm2 status

# 查看日志
pm2 logs simverse-frontend

# 重启
pm2 restart simverse-frontend

# 停止
pm2 stop simverse-frontend

# 删除
pm2 delete simverse-frontend

# 开机自启（可选）
pm2 startup  # 需要先运行此命令生成初始化脚本
pm2 save     # 保存当前进程列表
```

## ⚠️ 不推荐方案（容易出问题）

### ❌ tmux/screen
```bash
tmux new -s dev
pnpm dev
# 按 Ctrl+B 然后 D 分离
```
**问题**：tmux 本身可能崩溃、内存泄漏、session 丢失。

### ❌ nohup / &
```bash
nohup pnpm dev > /tmp/vite.log 2>&1 &
```
**问题**：无法方便地重启、查看日志、管理进程。

## 紧急恢复命令（仅限非 pm2 管理时）
```bash
# 查看暂停的进程
jobs -l

# 恢复暂停的进程到前台
fg

# 或放到后台
bg
```

## 警告标志
- 进程状态显示 `T` = 已暂停
- 端口监听正常但请求超时 = 很可能被暂停了
- 先尝试 `kill -SIGCONT <pid>` 恢复，不要直接 kill