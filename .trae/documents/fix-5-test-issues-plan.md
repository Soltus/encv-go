# 测试问题修复计划

## 问题总览

| # | 问题 | 根因 | 涉及文件 |
|---|------|------|----------|
| 1 | 文件长按加密/解密只跳转不进入新建任务 | Ionic Tab 组件缓存导致 `onMounted` 不重发，query 参数处理被跳过 | `Tasks.vue`, `Files.vue` |
| 2 | 新建任务全局密码只读显示多余 | UI 冗余，用户明确要求移除 | `Tasks.vue` L180-192 |
| 3 | WebDAV 硬编码已启用 | `WebdavServer` 无 Enabled 字段，`DefaultConfig()` 无条件设置 `Root="/webdav/"` | `types.go`, `config.go`, `mobile_api.go` |
| 4 | 扩展从文件安装仅弹窗无实际功能 | `handleInstallFromFile()` 只弹 alert，无文件选择器也无安装逻辑 | `ExtensionsPage.vue` |
| 5 | `ff_graph_css_data` 符号缺失 dlopen 失败 | fftools/graph/*.c 编译可能静默失败或符号未正确链接 | `build-ffmpeg-android.sh` |

---

## Issue #1：文件长按加密解密不进入新建任务

### 根因分析

[Files.vue:557-569](app/encv-mobile/src/views/Files.vue#L557-L569) 中 `handleEncryptFile`/`handleDecryptFile` 使用 `router.push({ path: '/tabs/tasks', query: { action: 'new', ... } })` 跳转。

[Tasks.vue:623-636](app/encv-mobile/src/views/Tasks.vue#L623-L636) 中 `onMounted` 检查 `route.query.action === 'new'` 来打开新建任务模态框。

**关键问题**：在 Ionic Vue 的 tab 架构中（见 [router/index.ts](app/encv-mobile/src/router/index.ts) 和 [Tabs.vue](app/encv-mobile/src/views/Tabs.vue)），所有 tab 子路由都是 Tabs 组件的 children。当用户从 `/tabs/files` 导航到 `/tabs/tasks` 时，**Tasks.vue 组件已经被缓存挂载**，`onMounted` **不会再次执行**。因此 query 参数中的 `action=new` 被完全忽略。

### 修复方案

在 `Tasks.vue` 中添加 `watch(() => route.query, ...)` 监听器，替代（补充）`onMounted` 中的 query 参数处理：

1. 将 query 参数处理逻辑提取为独立函数 `processQueryAction()`
2. 在 `onMounted` 中调用该函数
3. 在 `watch(route.query, ...)` 中也调用该函数
4. 处理完后清除 query 参数（`router.replace({ query: {} })`）避免重复触发

### 修改文件
- [Tasks.vue](app/encv-mobile/src/views/Tasks.vue)
  - 提取 `processQueryAction()` 函数
  - 添加 `watch(route.query, processQueryAction, { immediate: true })`
  - 移除 `onMounted` 中的重复 query 处理代码
  - 处理后执行 `router.replace({ path: '/tabs/tasks', query: {} })`

---

## Issue #2：移除新建任务的全局密码只读显示

### 根因分析

[Tasks.vue:180-192](app/encv-mobile/src/views/Tasks.vue#L180-L192) 包含一个 `readonly` 的 `<ion-input>` 显示全局密码。用户认为这是多余的——密码已在上方输入框中预填为 `newTaskPassword.value = config.value.password`（L624-626），再显示一个只读版本没有意义。

### 修复方案

直接删除 L180-192 的整个 `<ion-item>` 块（包含"全局密码显示（只读）"注释到闭合 `</ion-item>`）。

### 修改文件
- [Tasks.vue](app/encv-mobile/src/views/Tasks.vue)：删除 L180-192 全局密码只读显示块

---

## Issue #3：WebDAV 硬编码已启用 + 默认配置 Bug 链

### 根因分析（多层 Bug 链）

这不是单一问题，而是一条 **4 层 Bug 链**，导致安装 APP 后"大量配置为空"且 WebDAV 永远显示已启用：

#### 第 1 层：构建脚本复制了错误的配置文件

| 环节 | 实际行为 | 正确行为 |
|------|---------|---------|
| [post-cap-sync.mjs:424](app/encv-mobile/scripts/post-cap-sync.mjs#L424) | 复制 `assets/config.mobile.json` → assets | 应复制 `config.user.json` → assets |
| 产物文件 | `android/app/src/main/assets/config.mobile.json`（**错误产物**） | 应为 `config.user.json` |
| [EncvGoService.kt:411](app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt#L411) 从 assets 读取 | 寻找 `config.user.json` | ✅ 名称正确，但文件不存在 |

结果：`copyDefaultConfig()` 找不到 `config.user.json` → 抛异常 → 走 `writeFallbackConfig()` 写出一个**极度残缺的配置**

#### 第 2 层：Fallback 配置严重缺失

[EncvGoService.kt:495-510](app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt#L495-L510) 的 fallback 配置缺少以下字段：
- ❌ `webdav` 整段（无 root/dir/username/password）
- ❌ `admin` 整段
- ❌ `proxy` 整段
- ❌ `default_container_version`
- ❌ `strict_deprecated_version`
- ❌ `recover`
- ❌ `webdav.port`

这就是用户说的"大量配置为空"的直接原因。

#### 第 3 层：DefaultConfig() 强制启用 WebDAV

[config.go:90-93](internal/config/config.go#L90-L93)：`DefaultConfig()` 无条件设置 `Root: "/webdav/"`, `Dir: "./output"`

由于 fallback 没有 webdav 键，Go `Load()` 的 `json.Unmarshal` 不会覆盖 → **DefaultConfig 的值保留** → WebDAV 永远启用。

#### 第 4 层：config.mobile.json 是错误产物（应删除）

assets 中的 `config.mobile.json` 存在两个问题：
1. 它是**多余的**——项目规则明确"不得创建独立的 config.mobile.json 或其他平台特定配置模板"
2. 即使它被正确加载，`"root": "/webdav/"` 同样硬编码启用 WebDAV，且缺少 `"mobile"` 段

### 修复方案（按依赖顺序）

#### Step A：修复 DefaultConfig() — 根源
- [config.go](internal/config/config.go)：`DefaultConfig()` 设置 `Root: ""`, `Dir: ""`（默认禁用）
- 这一步单独就能解决 WebDAV 硬编码启用问题（即使上层配置全错，默认值也是禁用）

#### Step B：移除 config.mobile.json，改用 config.user.json
- **删除** `app/encv-mobile/assets/config.mobile.json`（如果存在）
- **删除** `app/encv-mobile/android/app/src/main/assets/config.mobile.json`
- [post-cap-sync.mjs](app/encv-mobile/scripts/post-cap-sync.mjs) L421-430：将复制目标从 `config.mobile.json` 改为项目根目录的 `config.user.json`，产物名为 `config.user.json`
- Kotlin 侧无需改动（已经读 `config.user.json` ✅）

#### Step C：补全 Fallback 配置 + Merge 逻辑
- [EncvGoService.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt) `writeFallbackConfig()`：补全 `webdav`、`admin`、`proxy`、`default_container_version`、`recover` 字段
- [EncvGoService.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt) `mergeConfigDefaults()`：补充 `webdav`、`admin`、`proxy`、`default_container_version`、`recover` 的合并逻辑

#### Step D：路由冲突检测
- [server.go](internal/server/server.go)：在 WebDAV 初始化前（L103 块开头）添加路由冲突检测
- 冲突时 fatal error（启动失败），不是 silent continue

### 修改文件清单
| 文件 | 操作 |
|------|------|
| [internal/config/config.go](internal/config/config.go) | `DefaultConfig()` Root="", Dir="" |
| [app/encv-mobile/scripts/post-cap-sync.mjs](app/encv-mobile/scripts/post-cap-sync.mjs) | 复制 `config.user.json`（非 config.mobile.json） |
| [app/encv-mobile/android/app/src/main/assets/config.mobile.json](app/encv-mobile/android/app/src/main/assets/config.mobile.json) | **删除** |
| [android/.../EncvGoService.kt](app/encv-mobile/android/app/src/main/java/com/encvgo/app/EncvGoService.kt) | 补全 fallback + merge |
| [internal/server/server.go](internal/server/server.go) | 路由冲突检测 |
| [internal/server/mobile_api.go](internal/server/mobile_api.go) | 无需改动 |

---

## Issue #4：扩展管理从文件安装无实际功能

### 根因分析

[ExtensionsPage.vue:185-204](app/encv-mobile/src/views/ExtensionsPage.vue#L185-L204) 的 `handleInstallFromFile()` 函数：

```typescript
async function handleInstallFromFile() {
    // ...
    const alert = await alertController.create({
        message: '请将 MPV 播放器扩展的 .apk 文件传到手机...',
        buttons: ['确定'],
    })
    await alert.present()  // ← 仅此而已，没有任何文件选择或安装逻辑
}
```

这是一个**空壳实现**——只弹出一个提示文字的对话框，没有：
- 文件选择器（File Picker）
- APK 文件读取
- 调用 ComboLite 插件安装 API

### 修复方案

使用 Capacitor 的原生文件选择能力 + ComboLite PluginManager 安装 API：

1. 引入 `@capacitor/filesystem` 或使用自定义 Plugin API 选择 `.apk` 文件
2. 读取选中的 APK 文件路径
3. 调用 ComboLite 的 `PluginManager.installPlugin(apkPath)` 进行实际安装
4. 安装完成后刷新扩展列表

需要确认项目中 ComboLite 是否暴露了安装插件的 JS/API 接口。如果没有，至少需要实现文件选择器并将 APK 复制到 ComboLite 可访问的目录。

### 修改文件
- [ExtensionsPage.vue](app/encv-mobile/src/views/ExtensionsPage.vue)：重写 `handleInstallFromFile()`
- 可能需要在 Kotlin 端或 Bridge 层暴露插件安装 API

---

## Issue #5：FFmpeg `ff_graph_css_data` 符号缺失

### 根因分析

错误信息：`dlopen failed: cannot locate symbol "ff_graph_css_data" referenced by "libffmpeg.so"`

构建脚本 [build-ffmpeg-android.sh](app/encv-mobile/scripts/build-ffmpeg-android.sh) 分析：

1. **L35-37**：缓存检测时主动检查并**拒绝**含 `ff_graph_css_data` 的缓存（视为废弃符号），强制重建
2. **L199-209**：CFLAGS 已包含 `-I${FFMPEG_SRC}/fftools/graph` 和 `-I${FFMPEG_SRC}/fftools/resources`
3. **L226-233**：可选目录循环中包含 `fftools/graph` 和 `fftools/resources`，会添加 `*.c` 文件到编译列表
4. **L245-268**：编译循环中，**可选目录的文件编译失败时仅 warning 不退出**（L264-266 的 else 分支）
5. **L270-282**：链接步骤使用 `$FFMPEG_OBJS`（包含所有成功编译的 .o 文件）

**核心矛盾**：
- 缓存检测（L35）把 `ff_graph_css_data` 当作"旧版残留符号"，有则删缓存
- 但运行时 dlopen 又**需要**这个符号（FFmpeg 8.0 fftools 依赖它）
- 这说明 `fftools/graph/` 目录下的某个 `.c` 文件定义了此符号，但**编译可能静默失败**（因为是 optional dir），导致该 .o 文件缺失

### 修复方向

需要确认以下两点来制定精确修复：

**A. 定位符号定义源**：`ff_graph_css_data` 定义在哪个具体源文件（很可能是 `fftools/graph/*.c` 中的某个文件，如 `graph_parse.c` 或类似名称的 CSS 数据文件）

**B. 编译失败原因**：graph 目录下哪些 `.c` 文件编译失败、失败原因是什么（查看 `${LOG_DIR}/ffmpeg_*.log`）

### 修复方案（分两步）

**Step 1 — 提升关键文件编译优先级**

将定义 `ff_graph_css_data` 的源文件从"optional dir"（失败仅警告）提升为"core file"（失败则退出）。修改方法：

1. 先通过搜索 FFmpeg 8.0 源码确定哪个文件定义了该符号
2. 将该文件加入 `FFMPEG_CORE_FFTOOLS` 列表（或创建一个 `FFMPEG_GRAPH_FFTOOLS` 必须编译列表）
3. 编译失败时 exit 1 而非 skip

**Step 2 — 修正缓存检测逻辑**

L35-37 的缓存检测逻辑有误：它把 `ff_graph_css_data` 视为"废弃符号"但实际上 FFmpeg 8.0 **需要**此符号。应改为：
- 如果 `ffmpeg_run` 存在但 `ff_graph_css_data` **不存在** → 说明 graph 模块未正确编译，强制重建
- 如果两者都存在 → 缓存有效

### 修改文件
- [build-ffmpeg-android.sh](app/encv-mobile/scripts/build-ffmpeg-android.sh)
  - 修正 L35-37 缓存检测逻辑（反转判断语义）
  - 将 `fftools/graph/` 中定义 `ff_graph_css_data` 的文件提升为必编译项
  - 可选：在链接后验证符号存在性

---

## 实施顺序

按依赖关系和风险排序：

1. **Issue #2**（最简单，纯 UI 删除）→ 立即可做
2. **Issue #1**（中等难度，需理解 Ionic tab 生命周期）
3. **Issue #3**（需同步改 Go 后端 struct + config + API handler）
4. **Issue #5**（需定位具体源文件，可能需要下载 FFmpeg 8.0 源码确认）
5. **Issue #4**（复杂度最高，需要了解 ComboLite 安装 API + 原生文件选择器集成）
