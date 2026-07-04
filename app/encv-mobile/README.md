# ENCV-go Mobile

ENCV-go 的移动端前端，基于 Ionic Vue + Capacitor 构建，通过 HTTP API 与本地 ENCV-go Daemon 通信。

## 技术栈

| 层 | 技术 |
|---|---|
| UI 框架 | Ionic Vue 8 |
| 移动端框架 | Capacitor 8 |
| 前端构建 | Vite 8 + TypeScript 5 |
| 后端服务 | ENCV-go Daemon（本地运行） |

## 架构

```
Ionic Vue UI (Capacitor WebView)
        ↓ localhost HTTP
ENCV-go Daemon
  ├── /stream     加密文件流
  ├── /api/config 配置读写
  └── /ping       健康检查
```

## 功能模块

| 模块 | 文件 | 说明 |
|------|------|------|
| 文件浏览 | `Files.vue` | 浏览目录、打开加密文件 |
| 媒体播放 | `Player.vue` | 视频/音频流式播放 |
| 任务管理 | `Tasks.vue` | 加密/解密任务队列 |
| WebDAV | `WebDAV.vue` | WebDAV 服务器配置与连接测试 |
| 设置 | `Settings.vue` | Schema 驱动的配置管理、外观、语言 |

## 项目结构

```
src/
├── api/
│   └── encv.ts              # API 层（文件、任务、配置、WebSocket）
├── composables/
│   ├── useConfig.ts         # Schema 驱动的配置管理
│   ├── useEventBus.ts       # 全局事件总线
│   ├── useI18n.ts           # 国际化（简体中文 / English）
│   ├── useServerStatus.ts   # 服务器状态检测
│   ├── useTheme.ts          # 深色/浅色主题切换
│   └── useWebSocket.ts      # WebSocket 实时通信
├── config/
│   ├── schema.json          # 配置 Schema（从 config.schema.json 同步）
│   └── schemaParser.ts      # JSON Schema 解析器
├── router/
│   └── index.ts             # Vue Router 路由定义
├── theme/
│   └── variables.css        # Ionic CSS 变量（深色/浅色主题）
├── views/
│   ├── Files.vue            # 文件浏览
│   ├── Player.vue           # 媒体播放
│   ├── Tasks.vue            # 任务管理
│   ├── WebDAV.vue           # WebDAV 配置
│   ├── Settings.vue         # 设置页面
│   └── Tabs.vue             # 底部标签栏
├── App.vue                  # 根组件
└── main.ts                  # 入口文件
```

## 开发

```bash
# 安装依赖
npm install

# 启动开发服务器
npm run dev

# 类型检查 + 构建
npm run build

# 预览构建产物
npm run preview
```

## Android 构建

项目通过 GitHub Actions 手动触发构建 APK：

1. 进入仓库 Actions 页面
2. 选择 "Build Android APK" workflow
3. 点击 "Run workflow"
4. 选择分支（默认 `main`）和版本号（可选）
5. 构建完成后在 Artifacts 中下载 APK

本地构建 Android：

```bash
# 构建前端
npm run build

# 同步 web 资源到 Android 项目（android/ 已在 git 中，无需 cap add）
npx cap copy android

# 用 Android Studio 打开
npx cap open android
```

## 国际化

应用默认使用简体中文，可在 **设置 → 外观 → 语言** 中切换。

翻译文件位于 `src/composables/useI18n.ts`，新增语言需：

1. 在 `messages` 对象中添加对应语言的翻译
2. 在 `Locale` 类型中添加语言代码
3. 在 Settings.vue 的 `ion-select` 中添加选项

## 配置管理

设置页面采用 Schema 驱动的方式，所有 ENCV-go 配置字段从 `config.schema.json` 动态渲染：

- Schema 文件：`src/config/schema.json`
- 解析器：`src/config/schemaParser.ts`
- 配置 API：`GET/PUT /api/config`、`GET /api/config/schema`

新增配置字段时无需修改前端代码，只需更新 Schema 即可自动渲染。
