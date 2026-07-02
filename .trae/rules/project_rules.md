# 项目规则（Project Rules）

> 11 个跨领域铁律集合：沙箱网络 / FFmpeg / Mobile Overlay / Go Build Tag / GitHub / Compose / 防御性编程 / 沙箱前端访问 / 测试覆盖 / 编译产物 / Skill 目录
>
> **完整内容 + 代码示例 + 反模式实战**：[详情文档](../rule-library/project_rules.md)

## 一、沙箱网络限制（本地构建必读）

- ❌ **沙箱禁止 Java/JVM 进程出站 TCP 连接**（所有 JDK 版本都受影响）
- ✅ 沙箱内 `curl` / `npm`（走 MCP HTTP 代理）可正常联网
- ✅ CI 环境不受此限制，Gradle 构建应在 CI 执行
- ✅ 沙箱内可用 `mise exec -- go build` 编译 Go（Java/Scala/Kotlin 不行）
> 进程级策略矩阵 + DNS 测试数据 + Java ProxySelector 根因链 → [trae_web_sandbox_network.md](./trae_web_sandbox_network.md)（索引）
> 完整诊断 + 401 区分铁律 + mock 浏览器日志规范 → [rule-library/trae_web_sandbox_network.md](../rule-library/trae_web_sandbox_network.md)

## 二、FFmpeg 8.0 备注（仅要点）

- 构建脚本: `app/encv-mobile/scripts/build-ffmpeg-android.sh`
- 链接时必须 `-lz`（缺失 `uncompress` 符号导致 `dlopen` 失败）
- CFLAGS 必须 `-I compat/stdbit`（NDK Clang 17 不支持 C23 `<stdbit.h>`）
- CFLAGS 必须 `-DHAVE_SYS_RESOURCE_H=1 -DHAVE_UNISTD_H=1 -DHAVE_SYS_SELECT_H=1`
- configure 必须 `--disable-asm`（ARM64 NEON 汇编与共享库不兼容）
- FFmpeg 8.0 已移除 `libpostproc`，链接列表中不能有 `-lpostproc`
- CFLAGS **禁止** `-I${FFMPEG_SRC}/libavutil`（会遮蔽系统 `<time.h>`）
> 完整 CFLAGS / 链接参数 / `--disable-asm` 根因 → [详情文档 §二](../rule-library/project_rules.md#二ffmpeg-80-构建脚本备注)

## 三、移动端 ffmpeg 调用架构

- Go 后端 cgo + dlopen 直接加载 `libffmpeg.so` / `libffprobe.so`
- 不经过 HTTP/Kotlin/JNI 中间层；stdout/stderr 通过 dup2 重定向捕获
- `ENCV_LIB_DIR` 指向 Android `nativeLibraryDir`
> 文件路径（ffmpeg_dlopen.go / video.go / build_info.go / build-info.json 分发）→ [详情文档 §三](../rule-library/project_rules.md#三移动端-ffmpeg-调用架构)

## 四、前端构建验证

- ❌ `vue-tsc --noEmit` 对 `.vue` 文件 `<script setup>` 中未使用导入存在漏检（TS6133）
- ✅ **必须同时运行 `vite build`**：完整验证流程 `vue-tsc --noEmit && vite build`

## 五、配置模板保护 + Mobile Overlay（核心）

> **原则：`mobile` 段是运行时 overlay（覆盖层），不修改持久化的 `config.user.json`。**

### 字段命名映射

| mobile 路径 | 映射到顶层 | Go 类型 |
|------------|-----------|---------|
| `mobile.server.dir` | `server.dir` | `MobileServerConfig.Dir` |
| `mobile.output.path` | `output_path` | `MobileOutputConfig.Path` |
| `mobile.webdav.dir` | `webdav.dir` | `MobileWebdavConfig.Dir` |

**禁止**扁平命名（`server_dir` / `output_path`）——无法从 JSON 结构推断映射。
### 数据流

```
config.user.json (持久化, 不被修改)
  ├── server: { dir: "/", port: 2025 }     ← 桌面端值
  └── mobile: { server: { dir: "/storage/emulated/0" } }
              ↓
   Load() → finalize() → ApplyMobileOverlay()  ← 仅内存中生效
              ↓
   运行时 Config: server.dir = "/storage/emulated/0"
   (config.user.json 文件内容不变)
```
### 触发条件

| 环境变量 | 场景 | 效果 |
|---------|------|------|
| `ENCV_MOBILE=1` | Android 真机（EncvGoService.kt 设置） | ✅ overlay 生效 |
| `ENCV_DEV_PREVIEW=1` | 桌面端移动预览（Makefile dev-mobile） | ✅ overlay 生效 |
| 均未设置 | 桌面端正常启动 | ❌ mobile 段被忽略 |

> JSON 代码示例 + 反模式 4 条（❌ 隐式覆盖 / ❌ 扁平命名 / ❌ 只在 ENCV_DEV_PREVIEW 生效 / ❌ finalize 顺序错）→ [详情文档 §五.2](../rule-library/project_rules.md#52-禁止的反模式)

## 六、Go Build Tag 平台约束

- **凡是有平台特定 stub 实现的函数，其对应的主实现文件必须添加互斥 build tag**
- 移动端 stub 使用 `//go:build android`，桌面端使用 `//go:build !android`
- **禁止**只给 stub 加 tag 而主文件留空（GOOS 交叉编译会重复声明）
- 每次新增平台 stub 文件对时，**必须**同时验证 `GOOS=android` 和默认平台编译通过
- 正确示例：`internal/utils/ffmpeg_dlopen.go` (android) ↔ `internal/utils/ffmpeg_dlopen_stub.go` (!android)

## 七、GitHub 项目搜索 + 拉取第三方源码铁律

> **核心原则：要拿就一次 `git clone` 拿全；不要 curl 遍历单文件。**

### 工具允许场景

| 工具 | 允许场景 | 禁止场景 |
|------|---------|---------|
| `WebSearch` | 搜概念、官方文档、issue 摘要、API 行为、StackOverflow 答疑 | 搜第三方仓库的源码实现细节 |
| `WebFetch` | 抓 README / 官方文档 / issue / blog / 官方 API 文档 | 抓 `raw.githubusercontent.com/.../*.kt` 等单文件源码 |
| `curl` | 查 maven published artifact metadata / 校验下载 / GitHub API 元数据 | 抓 raw 源码文件 |
| `git clone` | **唯一允许**的源码拉取方式，一次拿全 | — |

### 正确路径（按优先级）

1. **先**看项目内已 clone 的源码（如 `/tmp/combolite-src/` 等）
2. **再**查 `WebSearch` 找官方文档 / issue 摘要
3. **再** `WebFetch` 抓官方 README / 文档页
4. **再** `git clone --depth 1 <url>` 一次拿全 → 本地 read / grep
5. **绝不用** `curl raw.githubusercontent.com` 遍历
### 优先级
- **不确定行为** → 优先查本地 clone + 现有规则 + 现有代码
- **还不确定** → 引用 URL 进规则文件 + 问用户
- **绝对禁止** → `curl raw.githubusercontent.com` / `WebFetch` 拉单文件源码
> 完整反例（2026-06-03 越界案例 6+ 次外部请求） + Gradle/Maven 镜像代码块 → [详情文档 §7.1/§7.2](../rule-library/project_rules.md#71-拉取第三方源码铁律)

## 八、UI 交互铁律 + Compose 编码规范

- **严禁自动 fallback**：用户选择功能不可用时**禁止**静默切换到其他方案
- **严禁 Toast 提示**：状态信息必须通过持久性 UI 元素显示（选项旁状态标签 / 设置页指示器）
- **State `by` 委托必须同时 import** `getValue` + `setValue`（缺一不可）
- **Material Icons Extended** 包路径 `Icons.Outlined.XXX`（**大写 O**）
- 写完任何 .kt Compose 文件后对照 [compose-reference.md](./compose-reference.md) 逐条检查

## 九、防御性编程铁律（4 章核心）

### 一、禁止硬编码动态数据
- ❌ `const FALLBACK = {...}; return data?.extensions ?? FALLBACK`（新增插件后过时）
- ✅ API 未就绪时返回 `UNAVAILABLE` 标记值，触发阻断
### 二、不确定时阻断，不猜测（Fail-Safe）
- API 404/超时/未初始化 → 返回 `[UNAVAILABLE]` → 禁用保存按钮
### 三、三层防御架构

| 层级 | 触发时机 | 行为 |
|------|---------|------|
| L1 前端 | 用户输入即时 | disabled 保存按钮 + 警告文案 |
| L2 API | PUT /api/config | `validateContainerExtensionsInConfig()` 返回 400 |
| L3 启动 | `InitializePlugins()` | slog.Error 日志 + 继续启动不 abort |

> 第四章 preview-gateway UPSTREAMS 完整踩坑（2026-06-07 事故根因链 + 路径清单 + 防御守卫实现 + 排查 checklist）→ [详情文档 §10.4](../rule-library/project_rules.md#104-preview-gateway-upstreams-路由完整性守卫-实战踩坑)

## 十、Trae Web 沙箱前端访问规则

> **铁律：云端沙箱只能通过 agent-tool-host 代理访问前端，严禁混淆端口身份**

| 端口 | 进程 | 身份 |
|------|------|------|
| **5173** (或动态分配) | `agent-tool-host` | **前端 HTTP 代理** — 用户浏览器实际访问的入口 |
| **5174/5175/...** (vite 动态分配) | `node .../vite` | Vite dev server 原始端口 — agent-tool-host 反向代理到此 |
**关键认知**：
- ✅ `lsof -i :5173` 看到 `agent-tool-host` 是**正常**的，不代表"vite 没在运行"
- ❌ 看到 `agent-tool-host` 就断言"这不是 vite / 这不是前端"——是错的
- ❌ 杀掉 `agent-tool-host` 进程——这是沙箱基础设施
- ❌ 让用户访问 vite 原始端口而非 agent-tool-host 代理端口
> 验证代码块 + OpenPreview 激活 → [详情文档 §11.2](../rule-library/project_rules.md#112-验证代码是否生效的正确方法) + [preview-management.md](./preview-management.md)

## 十一、测试覆盖铁律

| 场景类型 | 示例 | 测试方式 |
|---------|------|---------|
| 路由跳转 + Modal 打开 | Files → Tasks 新建任务 | 单元测试 mock router + 断言 modal state |
| API 调用触发时机 | `predictPlugin` 是否被调用 | spy/predictPlugin mock + 断言调用次数 |
| computed 派生状态 | candidates 变化 → predictedPlugin 自动更新 | 设置 candidates → 断言 predictedPlugin 值 |
| 条件渲染 | passwordStrategy=independent 时字段显隐 | 设置不同 strategy → 断言 DOM 元素存在性 |

**测试优先级**：路由/导航（最高）→ API 调用链（高）→ computed 派生（中）→ 样式/CSS（低）

## 十二、编译产物铁律

- **SHALL NOT** 提交 `go build` 产物到仓库根目录或子目录
- **SHALL NOT** 在 `bin/`、`app/*/server-go`、根目录散落 `encv-server`、`server`、`agent-demo` 等可执行文件
- 编译产物使用 `-o bin/encv-server` 等输出路径，**必须**确保目标路径在 `.gitignore` 排除列表中
- 历史已清理（2026-06-08 git-filter-repo：`.git` 145MB → 5.1MB，节省 96%）
### Mock-data 流向（防混淆）

> **仓库内不存在 mock-data 真身**。所有 mock 都在设备运行时路径 `/storage/emulated/0/`，由**后端 API `/api/mock/generate`**（带 `X-Confirm-Mock-Mutation: yes` header）动态生成。

| 真实路径 | 生成器 | 用途 |
|---------|--------|------|
| `/storage/emulated/0/01-plain-media/*` | 后端 `POST /api/mock/generate` | 视频/图片/音频/文档 |
| `/storage/emulated/0/02-alist-encrypt/*` | 后端 `POST /api/mock/generate` | 小型加密夹具 |
| `/storage/emulated/0/03-encv-containers/*` | 后端 `POST /api/mock/generate` | ENCV v4 容器 |
| `/storage/emulated/0/04-boundary-test/*` | 后端 `POST /api/mock/generate` | 边界用例 |

> 完整 `.gitignore` 清单 + 预提交钩子 + 历史教训 → [详情文档 §13](../rule-library/project_rules.md#十三编译产物铁律)

## 十三、Skill 目录归属铁律

> **`.agents/skills/`（含 `.trae/skills/` 与 `app/encv-mobile/.agents/skills/`）与 `plugin-openlist/src/main/assets/` 已被 `.github/linguist.yml` 与 `.gitattributes` 标记为 `linguist-vendored`/`linguist-generated`，提交到这两处的代码不会出现在仓库 Languages 栏。**
- ❌ 不得向 `.agents/skills/**` 提交 first-party 技能定义
- ❌ 不得直接编辑 `plugin-openlist/src/main/assets/openlist/assets/**` 下的 dist 产物
- ❌ 不得向 `app/encv-mobile/.agents/skills/**` 提交 first-party 脚本
- ✅ first-party skill 应放置到 `.trae/skills/` 或新建 `app/encv-mobile/scripts/agents-skills/` 目录
> 完整 awk 验证脚本 + 例外条款（手动 sync Capacitor / Lynx skill）→ [详情文档 §14](../rule-library/project_rules.md#十四skill-目录归属铁律)

## 十四、新功能入口铁律（先全貌勘察，再决定放哪）

> **核心原则：新增任何「设置项 / 子功能 / 二级页面」之前，**必须**先 `grep` 已有页面找最契合的归属位置，**禁止**直接在 `Settings.vue` 等一级页面堆叠 `ion-item button @click="goXxx"`。**

### 14.1 必做调研（不调研 = 擅自堆入口）

新增任何「设置/子页面/详情页」之前，**必须**先做以下查询：

```bash
# 1. 找现有最相关的页面（避免重复造轮子）
grep -r "searchIcon\|cloudIcon\|cacheIcon\|databaseIcon" src --include="*.vue" | head -10

# 2. 找现有二级页面的入口位置
grep -n "ion-item button.*@click=\"go" src/views/Settings.vue | head -10

# 3. 找现有 nav 跳转函数
grep -n "function go.*router.push" src/views/Settings.vue
```

### 14.2 三类归属（按场景）

| 场景 | 归属位置 | 反例（禁止） |
|------|---------|------------|
| **数据库/索引/缓存相关** | `CacheDetail.vue` (cache + index 已有) 或 `DatabaseDetail.vue` | ❌ 单独在 Settings.vue 加「全文索引」ion-item |
| **AI/Agent 相关** | `AgentSettingsDetail.vue` | ❌ 在 Settings.vue 顶层加 Agent 入口 |
| **插件相关** | `PluginSettings.vue` | ❌ 散落到 Settings 列表 |
| **其他独立子页面** | 在 Settings.vue 顶层但**需有明确分组/标题** | ❌ 重复造轮子（已有类似页面） |

### 14.3 必须就近添加的场景（Settings 顶层违规清单）

**严禁在 Settings.vue 顶层加 ion-item 入口**（这些都有专属二级页）：

| 拟添加的功能 | 正确位置 |
|------------|---------|
| 「全文索引」 | `CacheDetail.vue` 加 ion-item button |
| 「数据库引擎详情」 | `DatabaseDetail.vue` 已有 |
| 「Agent 配置」 | `AgentSettingsDetail.vue` 已有 |
| 「插件设置」 | `PluginSettings.vue` 已有 |

### 14.4 违规检测（开发期自检）

每完成一个 Settings.vue 的 ion-item 添加前，**必须**自问：

1. 是否 grep 过现有 page 列表找到归属位置？
2. 是否阅读了至少 3 个类似二级页面的结构？
3. 是否有强语义化理由（而非「找不到地方就放这」）？

> **历史踩坑（2026-07-02）**：assistant 擅自给「全文索引」在 Settings.vue 一级页面加 ion-item 入口，没去 CacheDetail.vue 找归属。已写入本规则强制后续复盘。

## 十五、相关规则

- [trae_web_sandbox_network.md](./trae_web_sandbox_network.md) — 沙箱网络诊断
- [compose-reference.md](./compose-reference.md) — Compose 权威参考
- [verification-discipline.md](./verification-discipline.md) — 验证纪律
- [development.md](./development.md) — 阻塞式启动反模式
- [capacitor.md](./capacitor.md) — Capacitor 架构
- [android.md](./android.md) — Android 构建系统（镜像顺序铁律）
- [preview-management.md](./preview-management.md) — Preview 服务 pm2 监管
- [mock-data-architecture.md](./mock-data-architecture.md) — Mock 架构
- [saturation-debugging.md](./saturation-debugging.md) — 饱和调试

> 拆分：2026-06-11
