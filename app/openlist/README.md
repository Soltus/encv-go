# app/openlist — Hi-Sillot OpenList Fork 协作入口

> **读到本文件即了解 fork 协作全貌，无需再向用户问任何背景问题。**

---

## 1. 文档目的与适用读者

本目录是 encv-mobile（`app/encv-mobile/`）与 Hi-Sillot/OpenList fork 协作的**总入口**。它不存放可编译代码，只承担三个职责：

1. **fork 维护**：clone / push / 打 tag / 同步 frontend dist
2. **跨会话 context 持久化**：新 AI 会话或新工程师把本文档纳入上下文，即可接手工作
3. **故障速查**：常见 10 类问题的「症状 → 根因 → 修复」一表

适用读者：

- **AI 会话**：把本文档贴入 system prompt 或 context，新会话就能自助
- **新工程师**：clone 仓库后先读本 README，再读 [主 spec](../../.trae/specs/integrate-openlist-as-combolite-plugin/spec.md)
- **运维**：构建 AAR / 推送 fork / 排查 CI 失败

---

## 2. 项目背景

端到端架构（移动端）：

```
[Android 设备]
  ├── encv-mobile WebView (Capacitor + Ionic Vue)
  │     └── UI 调 /openlist/* API
  ├── encv-go 子进程 (端口 2025)
  │     └── 反代 /openlist/* → 127.0.0.1:5244（OpenList）
  └── plugin-openlist ComboLite 插件 (端口 5244)
        ├── OpenListService (前台保活)
        └── OpenListNativeService (Phase 26+) ProcessBuilder → libopenlist.so (arm64-v8a, Phase 27)
              └── OpenList 内部 gin → 解密 ENCV 容器 → 流回
```

> **Phase 27 架构变更**：OpenList 不再走 gomobile bind 嵌入进程模式（`openlist.aar` / `libgojni.so`），改为 fork dev 分支**独立交叉编译**为 `libopenlist.so`（arm64-v8a only），通过 AGP 标准 jniLibs 流程打包进 plugin-openlist.apk，由 `OpenListNativeService` 用 `ProcessBuilder` 启独立进程。详见 §5.4。

**关键路径**（以访问一个 `.sccgv` 加密视频为例）：

```
用户点击文件
  → WebView 发 GET /openlist/local-loopback/d/test.sccgv?sign=xxx
  → encv-go(2025) 反代字节透传
  → OpenList(5244) 收到 → handleEncvPreviewFromLink() 识别 .sccgv
  → 调用 encv.GenerateENCVSettingItems + LoadENCVPluginSettings
  → 透明解密 → 以 video/mp4 流回
  → encv-go 透传 → WebView 播放
```

**结论**：OpenList 必须本地化才有解密能力；fork 是**唯一**具备 ENCV 解密能力的 OpenList 变体。

---

## 3. 五层 Fork 关系图

```mermaid
graph TB
  subgraph 本仓库 [encv-mobile / 本仓库]
    A[app/encv-mobile]
    P[plugin-openlist]
    CI[.github/workflows/android.yml<br/>Phase 27 cross-compile step]
  end

  subgraph HiSillot [Hi-Sillot 个人 fork]
    F1[Hi-Sillot/OpenList<br/>dev 分支 + glebarez/sqlite]
    F2[Hi-Sillot/OpenList-Frontend<br/>i18n 同步源]
  end

  subgraph 上游 [OpenListTeam 上游]
    U1[OpenListTeam/OpenList<br/>main 分支]
    U2[OpenListTeam/OpenList-Frontend<br/>releases dist]
  end

  subgraph 参考 [K-Sillot 参考实现]
    R1[K-Sillot/OpenList-Mobile<br/>gomobile bind 路线 (废弃参考)]
    R2[K-Sillot/OpenList-Desktop<br/>Tauri 桌面端]
  end

  CI -->|go build arm64-v8a| F1
  CI -->|copy to jniLibs/| P
  A --> P
  P -->|ProcessBuilder libopenlist.so| F1
  F1 -.->|定期 rebase| U1
  F2 -.->|参考 i18n| U2
  R1 -.->|历史架构参考| F1
  R2 -.->|参考集成| A
```

| 仓库 | 角色 | 与本项目关系 |
|------|------|-------------|
| **encv-mobile**（本仓库） | 主项目：WebView + encv-go + ComboLite | — |
| **Hi-Sillot/OpenList** | 个人 fork，集成 ENCV 解密 + `openlistlib/` 入口 + **`glebarez/sqlite` 选型**（commit `404daf0`，2026-06-02） | 唯一被 build script clone，**本地路径：`app/openlist/Hi-Sillot-OpenList/`** |
| **OpenListTeam/OpenList** | 上游 OpenList | 周期性 rebase 来源 |
| **OpenListTeam/OpenList-Frontend** | 前端 dist 发布源 | 精确 tag 下载 |
| **Hi-Sillot/OpenList-Frontend** | 前端 fork（i18n 同步源）；手动 clone | **本地路径：`app/openlist/Hi-Sillot-OpenList-Frontend/`** |
| **K-Sillot/OpenList-Desktop** | 桌面端 Tauri 实现（项目已废弃，仓库保留参考） | 手动 clone；**本地路径：`app/openlist/K-Sillot-OpenList-Desktop/`** |
| **K-Sillot/OpenList-Mobile** | gomobile bind 参考实现 | 仅在线浏览，**不参与本地集成** |

---

## 4. Hi-Sillot Fork 维护工作流

### 4.1 分支策略

| 分支类型 | 命名 | 用途 | 生命周期 |
|---------|------|------|---------|
| `main` | 上游 main 同步 | 长期 | 永久 |
| `dev` | ENCV 补丁开发 | **默认 checkout 分支** | 永久 |
| `feature/*` | 单次功能开发 | 短期 | 合并后删除 |
| `encv-v*.*.*` | 稳定发布 tag | CI 切到固定版本 | 永久 |

### 4.2 沙箱推送命令模板

参见 §10，此处仅列最常用命令：

```bash
# 1. 克隆到固定目录（影响 build script 路径解析——fork 的 go.mod 相对路径 ../../../ 依赖此布局）
cd /workspace/app/openlist
git clone --branch dev \
    https://x-access-token:${GITHUB_TOKEN}@github.com/Hi-Sillot/OpenList.git \
    Hi-Sillot-OpenList

# 2. 修改 + 提交
cd Hi-Sillot-OpenList
echo "v4.0.0" > frontend-pinned.txt
git add frontend-pinned.txt
git commit -m "bump frontend pin to v4.0.0"

# 3. 推送（必须用 URL 注入，extraHeader 在 push 时被拒）
git push \
    https://x-access-token:${GITHUB_TOKEN}@github.com/Hi-Sillot/OpenList.git dev
```

### 4.3 Tag 流程（切到稳定版本）

```bash
# 在 fork 上打 tag
cd OpenList
git tag encv-v0.1.0
git push origin encv-v0.1.0

# 在 encv-mobile 仓库切到该 tag
# 编辑 scripts/openlist-fork.env:
#   OPENLIST_FORK_PINNED_TAG=encv-v0.1.0
# 重新跑 build-openlist-aar.sh 即可
```

### 4.4 三个 fork 的本地布局

> **设计铁律**：所有本地参与维护的 fork 都要 clone 到 `app/openlist/` 下固定子目录，目录名与 GitHub 仓库名同形（含 org 前缀），并由 `app/openlist/.gitignore` 整体忽略。`build-openlist-aar.sh` 默认把主 fork clone 到 `app/openlist/Hi-Sillot-OpenList/`，fork 的 go.mod 里的 `replace github.com/Soltus/encv-go => ../../../` 这个相对路径**依赖此布局**才能解析到 encv-go 根（`/workspace`）。

```bash
# 三层 fork 各自的固定路径（手动 clone，不需要时跳过）
cd /workspace/app/openlist

# ① 主 fork：build script 依赖（脚本会自动 clone/更新）
#    如手动 clone 务必指定目录名为 Hi-Sillot-OpenList
git clone --branch dev \
    https://github.com/Hi-Sillot/OpenList.git \
    Hi-Sillot-OpenList

# ② 前端 fork：i18n 同步源（手动维护 ENCV overlay 翻译补丁时用到）
git clone --branch main \
    https://github.com/Hi-Sillot/OpenList-Frontend.git \
    Hi-Sillot-OpenList-Frontend

# ③ 桌面 fork：Tauri 实现参考（项目已废弃，仓库保留用作代码考古）
git clone --branch main \
    https://github.com/K-Sillot/OpenList-Desktop.git \
    K-Sillot-OpenList-Desktop

# ④ 在线参考（不需要本地 clone）：
#    K-Sillot/OpenList-Mobile — 纯参考，https://github.com/K-Sillot/OpenList-Mobile
```

**环境变量覆盖**：`build-openlist-aar.sh` 支持 `OPENLIST_FORK_WORK_DIR` 环境变量，CI runner 想把 fork clone 到独立卷（如 `/cache/fork`、`D:\cache\fork`）复用时可设此变量，绕过默认布局。

```bash
# 例：CI runner 缓存策略
export OPENLIST_FORK_WORK_DIR=/cache/fork
bash scripts/build-openlist-aar.sh --output app/encv-mobile/plugin-openlist/libs
```

> **注意**：若用 `OPENLIST_FORK_WORK_DIR` 改了 fork 路径，go.mod 的相对 `replace` 不再成立；脚本会自动 `sed` 改回绝对路径兜底（见 D4）。

---

## 5. Phase 27: libopenlist.so 交叉编译架构

> **Phase 27 取代了 gomobile bind 嵌入进程模式（Phase 26 之前）**。本节描述当前生产架构。
> 历史 gomobile bind 路线已废弃，仅作参考（见 `integrate-openlist-as-combolite-plugin/spec.md`）。

### 5.1 架构对比

| 维度 | ~~gomobile AAR（废弃）~~ | **sidecar binary（当前, Phase 27）** |
|------|------------------------|--------------------------------------|
| 进程数 | ~~1（共享 JVM）~~ | **2（host + OpenList 子进程）** |
| 启动 | ~~~100ms in-process~~ | ~3-5s 冷启（OpenList 子进程） |
| Go runtime 数量 | ~~1 份（OpenList 嵌入）~~ | **2 份（encv-go + OpenList）** |
| 产物 | ~~`openlist.aar` + `libgojni.so`~30-50MB~~ | **`libopenlist.so`（ELF 可执行,非 shared）~30-50MB** |
| 打包流程 | aar2apk classes.jar 注入（易坏） | **AGP 标准 jniLibs 流程（稳）** |
| ABI 范围 | 4 ABI (arm64-v8a / armeabi-v7a / x86 / x86_64) | **1 ABI (arm64-v8a only, Phase 27)** |
| 调试 | ~~统一 Logcat tag=`OpenList`~~ | 子进程日志走 `adb logcat -s OpenListNativeService` |
| 复杂度 | gomobile bind NDK 链 + AAR dex 注入（多次踩坑） | **go build + cc=NdkClang（一次配置）** |

### 5.2 链路

```
plugin-openlist.apk
  └── lib/arm64-v8a/libopenlist.so   ← Phase 27 产物
        ├── Go runtime (goroutine scheduler, GC, net)
        ├── OpenList 全部业务代码
        ├── glebarez/sqlite (pure-Go, CGO-free)
        └── ENCV 集成（fork 注入 internal/encv/）

加载链（plugin-openlist 启动）:
  OpenListNativeService.start()
    → findExecutableBinary() = applicationInfo.nativeLibraryDir + "/libopenlist.so"
    → binary.setExecutable(true) + setReadable(true)  ← jniLibs 默认非可执行
    → ProcessBuilder("/data/app/.../lib/arm64-v8a/libopenlist.so", "server", "--port", "5244", "--data", dataDir)
    → 子进程: 调 cmd/server.go::Start()
        → GenerateENCVSettingItems() + LoadENCVPluginSettings() + InitializeWithSettings()
        → bootstrap.InitOfflineDownloadTools() + LoadStorages() + InitTaskManager()
        → gin.Run("127.0.0.1:5244")
    → OpenListEmbedWebView 加载 http://127.0.0.1:5244/
```

### 5.3 5244 端口

OpenList 内部 gin 仍绑 `127.0.0.1:5244`（与桌面版一致）。encv-go 通过 loopback 访问，**无需特殊代码**。

### 5.4 ABI 范围决策（arm64-v8a only）

| 决策 | 理由 |
|------|------|
| **只编 arm64-v8a** | 现代 Android 设备（2018 年后）几乎都是 arm64-v8a；Google Play 2019 起强制 64-bit |
| 编译时间 | 从 4 ABI 的 5-8 min 降到 **~2 min**（含 cgo+NDK 编译） |
| APK 体积 | 从 4 ABI 的 ~120-200MB 降到 **~30-50MB** |
| 简化 | 少 3 个 ABI 的 build script、jniLibs 目录、aar2apk addNativeLibs 复杂度 |
| 风险 | armeabi-v7a / x86 / x86_64 设备**无法运行** plugin-openlist |

`plugin-openlist/build.gradle.kts` 配置（Phase 27）：
```kotlin
ndk {
    abiFilters += listOf("arm64-v8a")  // Phase 27: 限制为仅 arm64-v8a
}
jniLibs.srcDirs("src/main/jniLibs")
```

`src/main/jniLibs/` 目录结构：
```
src/main/jniLibs/
└── arm64-v8a/
    └── libopenlist.so   ← CI 编译产物（git ignore,每次 build 覆盖）
```

### 5.5 fork 编译命令（CGO=1 + NDK 兜底）

> **重要认知**：尽管 fork 切到 `glebarez/sqlite`（pure-Go），fork 通过 `replace` 链入 encv-go monorepo，encv-go 的 `internal/utils/ffmpeg/ffmpeg_dlopen.go` 是 **`//go:build android` + `import "C"` cgo-only**（用 `dlopen` 调 native libffmpeg.so）。所以 fork 编译**必须 CGO=1**——但因为纯 Go 依赖，不需要复杂的 C 编译，**只需 NDK clang 工具链**（不调 NDK C 编译器，仅用其 ld）。

```bash
# 关键:虽然 glebarez/sqlite 是 pure-Go,但 ffmpeg_dlopen.go 强制 cgo
# 详见 .trae/rules/android.md §五 5.5 兜底方案
cd app/openlist/Hi-Sillot-OpenList
go mod download

CGO_ENABLED=1 GOOS=android GOARCH=arm64 \
CC=/usr/local/lib/android/sdk/ndk/26.1.10909125/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android24-clang \
  go build -o /tmp/libopenlist-arm64.so ./
```

**产物**：
- 文件名 `libopenlist.so` 仅是**命名约定**（过 AGP jniLibs 打包，AGP 只打 `*.so`）
- 实际是 **ELF executable**（非 shared library）—— `file` 验证：`ELF 64-bit LSB executable, ARM aarch64`
- 运行时由 Kotlin `binary.setExecutable(true)` 启用执行位

**为什么不走 c-shared**：
- `-buildmode=c-shared` 产 JNI/FFI .so 库，需 cgo=1 编译
- 但 EncvGoService 模式跑的是**独立进程**（`ProcessBuilder`），不需要 C ABI 接口
- 默认 buildmode = `exe` 才是正确选择

### 5.6 Phase 27 编译流程（CI 端到端）

```
.github/workflows/android.yml (3 个新 step)
  │
  ① Setup Go 1.25.1
  │   - actions/setup-go@v5
  │   - go-version-file: app/openlist/Hi-Sillot-OpenList/go.mod
  │   - cache-dependency-path: app/openlist/Hi-Sillot-OpenList/go.sum
  │
  ② Build OpenList native libs (arm64-v8a, CGO+NDK)
  │   - CGO_ENABLED=1 GOOS=android GOARCH=arm64
  │   - CC=<NDK>/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android24-clang
  │   - go build -o /tmp/libopenlist-arm64.so ./
  │   - 验证 file output: "ARM aarch64" + "executable"
  │
  ③ Copy libopenlist.so to plugin-openlist jniLibs
  │   - JNI_DIR=app/encv-mobile/plugin-openlist/src/main/jniLibs/arm64-v8a
  │   - cp /tmp/libopenlist-arm64.so $JNI_DIR/libopenlist.so
  │
  ↓
AGP jniLibs 流程（plugin-openlist/build.gradle.kts）
  │
  - abiFilters = listOf("arm64-v8a")  → 仅 arm64-v8a 进入 APK
  - jniLibs.srcDirs("src/main/jniLibs")  → 找 libopenlist.so
  - aar2apk 任务 [ConvertAarToApkTask.kt:179-193] addNativeLibs 无条件打包
  │
  ↓
plugin-openlist-debug.apk
  └── lib/arm64-v8a/libopenlist.so  (Phase 27 产物)
```

---

## 6. encv-Go 集成点

Hi-Sillot/OpenList fork 侧的 3 个 ENCV 集成点：

| 路径 | 符号 | 调用时机 |
|------|------|---------|
| `internal/encv/init.go` | `GenerateENCVSettingItems()` | `cmd/server.go::Start()` 开头 |
| `internal/encv/init.go` | `LoadENCVPluginSettings()` | 同上，紧随其后 |
| `server/handles/down_ext.go` | `handleEncvPreviewFromLink()` | 拦截 `/d/*` 下载 + `.sccgv` 后缀 |

调用顺序（在 `openlistlib/server.go::Start()` 中封装）：

```go
func Start() {
    // 1. 注册 ENCV 设置项
    encv.GenerateENCVSettingItems()
    encv.LoadENCVPluginSettings()
    encvPlugins.InitializeWithSettings()

    // 2. 上游 bootstrap
    bootstrap.InitOfflineDownloadTools()
    bootstrap.LoadStorages()
    bootstrap.InitTaskManager()

    // 3. 起 server
    r := gin.New()
    server.Init(r)
    r.Run("127.0.0.1:5244")
}
```

**关键不变量**：ENCV 容器解密**只在** OpenList fork 侧发生；encv-go **不**对 `/openlist/*` 路径下的 ENCV 容器做二次解密。

---

## 7. Frontend-pinned.txt 同步机制

### 7.1 为什么不能用 `releases/latest`

Hi-Sillot fork 在 `internal/conf/const.go` 中新增了 ENCV 设置项（`EncvDecryptPassword` / `EncvTextExt` / `EncvAudioExt` / `EncvVideoExt` / `EncvImageExt`）。若用 `releases/latest` 拉前端 dist：

- 上游最新版 frontend **没有**这些 key → Web UI 显示空白 / 跳 404
- i18n 翻译条目版本不匹配 → 切语言后部分页面回退英文
- 上游新增路由 → fork backend 未实现 → 跳 404

### 7.2 4 级 Pin 优先级（高 → 低）

1. `--frontend-version` CLI 入参（**最高，CI 显式传入**）
2. `OPENLIST_FRONTEND_VERSION` 环境变量（开发者本地配置 / `openlist-fork.env`）
3. `${SRC_DIR}/frontend-pinned.txt`（fork 提交时 pin，仅当前两者均未指定时生效）
4. fallback `releases/latest` + stderr warning

> **单一事实来源范式**：CI 必须用 `--frontend-version` 显式指定版本；本地开发可在 `openlist-fork.env` 中设 `OPENLIST_FRONTEND_VERSION`；fork 的 `frontend-pinned.txt` 仅作为默认兜底。

`build-openlist-aar.sh` 内部：

```bash
# 优先级：CLI → env → pinned.txt → latest
if [[ -n "${FRONTEND_VERSION_CLI}" ]]; then
  FRONTEND_VERSION="${FRONTEND_VERSION_CLI}"
elif [[ -n "${OPENLIST_FRONTEND_VERSION:-}" ]]; then
  FRONTEND_VERSION="${OPENLIST_FRONTEND_VERSION}"
elif [[ -f "${SRC_DIR}/frontend-pinned.txt" ]]; then
  FRONTEND_VERSION="$(grep -oE 'v[0-9]+\.[0-9]+' frontend-pinned.txt)"
fi
curl "https://api.github.com/repos/OpenListTeam/OpenList-Frontend/releases/tags/${FRONTEND_VERSION}"
```

### 7.3 VERSION 文件

下载 + i18n overlay 合并后写 `public/dist/VERSION`（内容 `${WEB_VERSION}-encv`，如 `v4.0.0-encv`）。OpenList 后端 `Bootstrap()` 读取并存为 `conf.FrontendVersion`。

ldflags 同步注入：

```
-X 'github.com/OpenListTeam/OpenList/v4/internal/conf.WebVersion=${WEB_VERSION}'
```

---

## 8. i18n Overlay 机制

### 8.1 目录约定

若 fork 在 `public/dist/i18n-overlay/<lang>/translation.json` 放置 ENCV 专用翻译补丁，构建脚本会用 `jq` 合并：

```
public/dist/i18n-overlay/
├── zh-CN/
│   └── translation.json   # ENCV 设置项的中文翻译补丁
└── en/
    └── translation.json
```

### 8.2 合并命令

```bash
jq -s '.[0] * .[1]' \
  public/dist/assets/zh-CN.json \
  public/dist/i18n-overlay/zh-CN/translation.json \
  > public/dist/assets/zh-CN.json.tmp && \
  mv public/dist/assets/zh-CN.json.tmp public/dist/assets/zh-CN.json
```

overlay key **覆盖**原 key（jq `.[0] * .[1]` 语义）。

### 8.3 示例

```json
// public/dist/i18n-overlay/zh-CN/translation.json
{
  "encv": {
    "decryptPassword": "解密密码",
    "textExt": "文本扩展名",
    "audioExt": "音频扩展名",
    "videoExt": "视频扩展名",
    "imageExt": "图片扩展名"
  }
}
```

合并后 OpenList Web UI 切到 zh-CN 显示中文；不切则保持英文 fallback。

---

## 9. 构建脚本索引

| 路径 | 用途 | 读者 |
|------|------|------|
| [app/openlist/Hi-Sillot-OpenList/](./Hi-Sillot-OpenList/) | Hi-Sillot/OpenList fork clone（dev 分支） | CI |
| [.github/workflows/android.yml](../../.github/workflows/android.yml) | CI：Go 1.25.1 setup + fork arm64-v8a 交叉编译 + jniLibs 拷贝（Phase 27） | CI / 维护 |
| [scripts/openlist-fork.env](../../scripts/openlist-fork.env) | ~~fork URL / branch / pin 配置（gomobile 时代）~~ **Phase 27 后本文件未被引用** | 历史 |
| [scripts/README.md](../../scripts/README.md) | ~~构建脚本详细文档（gomobile 时代）~~ | 历史 |
| [app/encv-mobile/plugin-openlist/README.md](../encv-mobile/plugin-openlist/README.md) | 插件模块说明（Phase 26+ OpenListNativeService 架构） | 移动端开发者 |
| [app/openlist/build-encv-desktop.ps1](./build-encv-desktop.ps1) | 桌面端 Tauri 构建（**已废弃**，仅留作历史） | 维护参考 |

> **Phase 27 删脚本**：`scripts/build-openlist-aar.sh` 已在 commit `60f5891`（Phase 26）删除——本 spec 正式宣告其废弃。
> 后续不再需要任何本地构建脚本，**所有交叉编译 + 拷贝都在 CI 里完成**。

### 9.1 快速开始（Phase 27）

> **以下命令均在仓库根目录（`/workspace`）执行。**
> **本地无需手动编 libopenlist.so**——CI 跑一次即可。如需本地 dev 模式跑 OpenList，用 §11 的 `dev-openlist.sh`（不依赖 APK）。

```bash
# 1. CI 触发:push 到 trae/solo-agent-WAmQzy 分支即可,无需本地操作
git push origin trae/solo-agent-WAmQzy

# 2. CI 完成后产物
#   app/encv-mobile/android/build/outputs/plugin-apks/release/plugin-openlist-release.apk
#   ├── AndroidManifest.xml
#   ├── classes.dex
#   ├── lib/arm64-v8a/libopenlist.so   ← Phase 27 编译产物
#   └── ...

# 3. 安装到设备
adb install -r app/encv-mobile/android/build/outputs/plugin-apks/release/plugin-openlist-release.apk
```

---

## 10. 沙箱 GITHUB_TOKEN 推送工作流

> **这是本节最重要的内容，新会话最容易踩坑。**

### 10.1 根因

`GITHUB_TOKEN` 已 export 在 shell env 中，但 **`git` 不会自动把环境变量当凭证使用**。沙箱既无 `~/.netrc` 也无 `~/.gitconfig`：

```bash
$ env | grep GITHUB
GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx  # ← 你的 PAT（示例，非真实值）

$ git push origin dev
fatal: could not read Username for 'https://github.com': terminal prompts disabled
```

### 10.2 解决方案对比

| 方案 | 命令 | 优点 | 缺点 |
|------|------|------|------|
| **1. URL 注入（推荐）** | `git push https://x-access-token:${GITHUB_TOKEN}@github.com/Hi-Sillot/OpenList.git dev` | git 命令统一；clone / push / fetch 都通过 | token 会进 shell history（用 `unset HISTFILE` 或 `set +o history` 缓解） |
| **2. extraHeader** | `git -c http.extraHeader="Authorization: Bearer ${GITHUB_TOKEN}" clone <url>` | clone 可用 | **push 时报 `invalid credentials`**（GitHub 不接受 Bearer 头做写操作），**不可靠** |
| **3. 临时 netrc** | `cat > ~/.netrc <<EOF<br>machine github.com login x password ${GITHUB_TOKEN}<br>EOF`<br>`chmod 600 ~/.netrc` | git 命令无需修饰 | 沙箱每次重启失效；token 写盘 |
| **4. SSH 协议** | `git@github.com:Hi-Sillot/OpenList.git` 配 SSH key | 一劳永逸 | 沙箱无 ssh-agent，**不可行** |

> **macOS Keychain 不适用沙箱**：`git credential-osxkeychain` 仅在交互式 macOS 桌面有效。
>
> **URL 注入的 token 隔离**：`https://x-access-token:TOKEN@github.com/...` 是 GitHub 官方推荐的 PAT 传递方式（触发 HTTP Basic Auth，GitHub PAT 接受）。`https://user:TOKEN@github.com/...` 也可但**user 部分会被 GitHub 忽略**。`x-access-token` 是占位 user name。

### 10.3 失败 / 成功对比

```bash
# ❌ 失败 1（裸命令，触发 username 提示）
git push origin dev
# fatal: could not read Username for 'https://github.com': terminal prompts disabled

# ❌ 失败 2（extraHeader 在 push 时被拒，GitHub 不接受 Bearer 头做写）
git -c http.extraHeader="Authorization: Bearer ${GITHUB_TOKEN}" push origin dev
# remote: invalid credentials
# fatal: Authentication failed for 'https://github.com/Hi-Sillot/OpenList.git/'

# ✅ 成功（URL 注入 — GitHub 接受 x-access-token user + PAT password 走 Basic Auth）
git push https://x-access-token:${GITHUB_TOKEN}@github.com/Hi-Sillot/OpenList.git dev
# To https://github.com/Hi-Sillot/OpenList.git
#    abc1234..def5678  dev -> dev
```

> **shell history 防护**：URL 注入会把 token 写进 `~/.bash_history`，**先**跑 `set +o history` 再 push；或者用 `GITHUB_TOKEN="$(cat ~/.encv-token)"` 内联避免 env 暴露。

### 10.4 build-openlist-aar.sh 已自动处理

`scripts/build-openlist-aar.sh` 检测到 `GITHUB_TOKEN` 时**自动**把 fork URL 改写为 `https://x-access-token:${GITHUB_TOKEN}@github.com/...` 形式，无需手工操作（脚本内日志 `[INFO] using GITHUB_TOKEN for fork auth (ghp_****)` 仅前 4 字符 + 4 星号）。

---

## 11. 开发模式预览（Dev Sandbox Preview）

> **本节描述如何在沙箱 / 本机跑通 OpenList UI 预览，不依赖 APK 编译。**
> **生产环境（APK 内 webview）走 §13 子路径挂载机制。**

### 11.1 架构概览

```
开发模式（两端口独立）:
  浏览器 ──→ http://localhost:8100/          ← encv-mobile 主应用 (Vite dev)
         ──→ http://localhost:5244/         ← OpenList (go run . server)
         ──→ http://localhost:8100/openlist-ui/ ← 可选：Vite 反代子路径（见 §13）

生产模式（APK 内单端口）:
  WebView ──→ http://127.0.0.1:2025/        ← encv-go
              ├── /*                        ← encv-mobile SPA
              ├── /openlist/*                ← encv-go 反代 → 127.0.0.1:5244
              └── /openlist-ui/*             ← Vite middleware → sirv(assets/dist/)
                  ├── /openlist-ui/api/*     → 127.0.0.1:5244/api/*
                  └── /openlist-ui/assets/*   → plugin assets/dist/assets/*
```

**关键区别**：

| 场景 | 访问方式 | API 路径 | 资源路径 |
|------|---------|---------|---------|
| **Dev 沙箱预览** | `http://localhost:5244/` 直访 | `/api/*` | `/assets/*` |
| **Vite 子路径（可选）** | `http://localhost:8100/openlist-ui/` | `/openlist-ui/api/*` → 反代 | `/openlist-ui/assets/*` |
| **APK 生产** | WebView 单端口 | `/openlist-ui/api/*` → 反代 | `/openlist-ui/assets/*` |

### 11.2 一键启动 Dev 模式

```bash
# Terminal 1: 启动 OpenList Go 后端（端口 5244）
cd app/encv-mobile
bash scripts/dev-openlist.sh --data /tmp/preview-openlist-data

# Terminal 2: 启动 Vite（端口 8100，可选）
pnpm dev

# 浏览器直访 OpenList（推荐，无需子路径）
#   http://localhost:5244/
# 登录: admin / <首次启动生成的随机密码>
```

`dev-openlist.sh` 行为：

| 步骤 | 操作 | 条件 |
|------|------|------|
| 1 | 检测本地 `Hi-Sillot-OpenList-Frontend/dist/` 存在？ | 有 → 复制到 fork 的 `public/dist/`（热更新友好） |
| 2 | 不存在 → 尝试从 GitHub Releases 下载 tarball | 版本默认 `4.1.8`，可用 `--frontend-version` 覆盖 |
| 3 | 写入 `${DATA_DIR}/config.json` | 设置 `dist_dir` 为 fork `public/dist/` 绝对路径 |
| 4 | `exec go run . server --data ${DATA_DIR}` | 首次 ~60s 编译，后续增量 ~2-5s |

### 11.3 Vite 子路径插件 (`openlistUiProxy()`)

位置：[app/encv-mobile/vite.config.ts](../encv-mobile/vite.config.ts) — `openlistUiProxy()` 插件。

**注册两条中间件**（顺序重要，API 代理优先于静态文件）：

```
请求路径                          处理方式
─────────────────────────────────────────────────
/openlist-ui/api/*     →  反代到 OPENLIST_UPSTREAM/api/*
                             （Vite 自动 strip 前缀）
/openlist-ui/*           →  sirv(Hi-Sillot-OpenList/public/dist/)
                             带 SPA fallback (single: true)
其他                         →  Vite 默认处理（encv-mobile 主应用）
```

**依赖**：
- `sirv@^3.0.2` — Vite 6+ 移除内置 static middleware 后的替代品
- `Hi-Sillot-OpenList/public/dist/index.html` 必须存在

### 11.4 绝对路径问题（⚠️ 已知坑）

OpenList-Frontend 构建产物的 `index.html` 使用**绝对路径**引用资源：

```html
<!-- 原始 index.html（问题代码） -->
<script type="module" src="/assets/index-S_YTMOrI.js"></script>
<link rel="stylesheet" href="/assets/index-DYLAoZjS.css" />

<!-- JS 预加载块（动态创建 DOM 元素） -->
var preloads = [{"src":"/assets/index-S_YTMOrI.js"}, {"href":"/assets/index-DYLAoZjS.css"}];
```

当页面部署在 `/openlist-ui/` 子路径下时，浏览器把这些解析为主应用路径：
- `/assets/index-S_YTMOrI.js` → `http://host:8100/assets/...`（**错误**：加载了 encv-mobile 的 JS 或 404）

**症状**：白屏、`<div id="root">` 为空、无 JS 报错（因为加载了错误的 bundle 或根本没加载）。

**修复**（vite.config.ts 中 `configureServer` 启动时一次性执行）：

```typescript
// 1. HTML 属性重写
.replace(/src="\/assets\//g, 'src="/openlist-ui/assets/')
.replace(/href="\/assets\//g, 'href="/openlist-ui/assets/')

// 2. JS 字符串字面量重写（preloads 动态创建的路径）
.replace(/":\"\/assets\//g, '":"/openlist-ui/assets/')

// 3. base_path 注入（让 SPA 知道自己的子路径）
.replace(/base_path:\s*undefined/, 'base_path: "/openlist-ui/"')
```

**注意**：此重写仅在生产子路径场景需要。Dev 模式下直接访问 `http://localhost:5244/` 即可，无需经过 Vite 代理。

---

## 12. 故障排查 Checklist

> **Phase 27 后**：原 gomobile bind 时代的故障（#2 #3 #8 #11 #12）已彻底消失——本表只列 Phase 27 实际相关的故障。新故障请追加到本表末尾。

| # | 症状 | 根因 | 修复 |
|---|------|------|------|
| 1 | **`bind: address already in use` 启动失败** | 5244 端口被占用（alist 旧版、nginx 转发、其他 OpenList） | `lsof -i :5244` 找进程；或在 plugin 设置改端口 |
| 2 | **`undefined: utils.CallFFmpegNative` at fork build** | fork 通过 `replace` 链入 encv-go，`internal/utils/ffmpeg/native_runner.go` 是 `//go:build android`，引用 `ffmpeg_dlopen.go` 的 cgo-only 符号 | **必须 CGO=1 + NDK clang**（CGO=0 会失败）。详见 §5.5 |
| 3 | **`libopenlist.so` 编译成功但 APK 找不到** | `app/encv-mobile/plugin-openlist/src/main/jniLibs/arm64-v8a/libopenlist.so` 文件未生成或路径错 | 跑 §5.6 步骤 ③ 拷贝；确认 `file` 输出 `ARM aarch64 executable` |
| 4 | **OpenList-Frontend tag 404** | `frontend-pinned.txt` 写了 `v9.9.9`（不存在） | 核对 tag 全名：`v4.0.0` 而非 `4.0.0` |
| 5 | **`replace github.com/Soltus/encv-go => ../../../` 解析失败** | fork 不在 `app/openlist/Hi-Sillot-OpenList/`（默认布局），或用户设了 `OPENLIST_FORK_WORK_DIR` 到非标路径 | 默认布局下相对路径天然成立；若仍失败，build script 会自动 sed 兜底为绝对路径；详见 §4.4 |
| 6 | **OpenListNativeService.start() 后 5s 内 5244 不响应** | 端口冲突 / 防火墙 / `libopenlist.so` 缺执行位 | `adb logcat -s OpenListNativeService`；`binary.canExecute()` 检查；详见 §5.2 |
| 7 | **i18n overlay jq 合并失败** | 缺 `jq` 或 overlay JSON 语法错 | `apt install jq`；`jq . public/dist/i18n-overlay/zh-CN/translation.json` |
| 8 | **`go build: buildmode=c-shared requires exactly one main package`** | 用了 `-buildmode=c-shared`（gomobile 时代的肌肉记忆） | **Phase 27 改为默认 buildmode（exe）**：见 §5.5 的 `go build -o /tmp/libopenlist-arm64.so ./`（注意是 `./` 不是 `./cmd`） |
| 9 | **`git push` 报 `terminal prompts disabled`** | 见 §10 | 用 `git push https://x-access-token:${GITHUB_TOKEN}@github.com/...` 替换裸 `git push` |
| 10 | **`frontend-pinned.txt` 不被识别** | 文件编码非 UTF-8 / 多行注释不闭合 | `file frontend-pinned.txt` 应为 ASCII/UTF-8 |
| 11 | **CI step ② 报 `cc1: some warnings being treated as errors`** | NDK clang 默认 `-Werror` 严格模式，encv-go 第三方依赖有 warning | 在 CI step 加 `CGO_CFLAGS="-Wno-error"`；或 `go env -w CGO_ENABLED=1` + 显式 `-gcflags="all=-N -l"` |
| 12 | **`file` 验证 `libopenlist.so` 不是 executable** | 用了 `-buildmode=c-shared` 产物是 shared object；或编译平台错（编了 x86_64） | 确认是默认 buildmode + `GOARCH=arm64`；详见 §5.5 |
| 13 | **`/openlist-ui/` 白屏 / `net::ERR_EMPTY_RESPONSE`** | OpenList-Frontend 构建的 `index.html` 用**绝对路径**引用资源（`src="/assets/xxx.js"`），子路径部署时浏览器解析到主应用端口加载错误 bundle；JS 预加载块中的路径也是绝对路径字面量 | vite.config.ts 的 `openlistUiProxy()` 插件在启动时**预读取 + 重写 index.html**：HTML 属性 + JS 字符串字面量 + `base_path` 注入。详见 §11.4 |
| 14 | **Vite 只监听 `[::1]:8100`（IPv6），外部访问 ERR_EMPTY_RESPONSE** | Vite 默认 `host: 'localhost'` 在某些沙箱只绑 IPv6；OpenPreview / 外部 host 走 IPv4 连不上 | vite.config.ts `server.host = '0.0.0.0'` 绑定所有接口 |
| 15 | **`dev-openlist.sh` 下载 frontend dist 404** | GitHub Releases URL 格式错误或版本号不存在（如 `v4.1.8` 的 tarball 名可能变化） | 优先用本地构建的 dist：`cd Hi-Sillot-OpenList-Frontend && bun run build`，dev-openlist.sh 会自动检测并复制 |
| 16 | **OpenList UI 登录后显示 "Failed fetching settings: 502"** | SPA 的 API baseURL 指向 `/api/...`（主应用路径）而非 `/openlist-ui/api/...`（代理路径）；或 OpenList(5244) 未启动 | Dev 模式直接访问 `http://localhost:5244/` 无此问题；子路径模式需注入 `base_path: "/openlist-ui/"`（§11.4 第 3 步） |
| 17 | **`libopenlist.so` 运行时缺 `libffmpeg.so` (dlopen failed)** | encv-go 的 ffmpeg_dlopen.go 在 android 平台用 `dlopen("libffmpeg.so")`，但 plugin-openlist.apk 没打 libffmpeg.so | fork 启动时 ffmpeg 不可用会 fallback 到 "not available" 错误日志（`CallFFmpegNative` 返回 error，**不 panic**）。如果业务流需要 ffmpeg，需要额外打 `libffmpeg.so` 到 jniLibs（**Phase 27 范围外**，未来工作） |

### 12.1 调试命令速查

```bash
# 5244 端口冲突
lsof -i :5244
nc -z -w2 127.0.0.1 5244 && echo "OCCUPIED" || echo "FREE"

# OpenList 健康
curl -s http://127.0.0.1:5244/api/site/list | jq .

# fork 同步状态
cd app/openlist/Hi-Sillot-OpenList
git fetch origin && git log --oneline HEAD..origin/dev

# 验证 libopenlist.so 是 ARM aarch64 + executable
file app/encv-mobile/plugin-openlist/src/main/jniLibs/arm64-v8a/libopenlist.so
# 期望输出:ELF 64-bit LSB executable, ARM aarch64, ...

# Phase 27 jniLibs 完整性
ls -lh app/encv-mobile/plugin-openlist/src/main/jniLibs/arm64-v8a/

# APK 内 libopenlist.so 验证
unzip -l app/encv-mobile/android/build/outputs/plugin-apks/release/plugin-openlist-release.apk | grep libopenlist
```

---

## 13. 双向链接

### 13.1 本仓库 spec

- [主 spec: integrate-openlist-as-combolite-plugin](../../.trae/specs/integrate-openlist-as-combolite-plugin/spec.md) — gomobile bind 时代（已废弃，仅作历史参考）
- [本 spec: openlist-fork-onboarding-readme](../../.trae/specs/openlist-fork-onboarding-readme/spec.md)
- [Phase 27 spec: build-openlist-fork-as-android-native](../../.trae/specs/build-openlist-fork-as-android-native/spec.md) — fork 交叉编译为 libopenlist.so（当前生产架构）
- [前端提取与沙箱预览 spec: openlist-frontend-extraction-and-sandbox-preview](../../.trae/specs/openlist-frontend-extraction-and-sandbox-preview/spec.md)
- [相关: implement-mobile-backend-api](../../.trae/specs/implement-mobile-backend-api/spec.md)
- [相关: eval-combolite-mkv-ffmpeg-plugins](../../.trae/specs/eval-combolite-mkv-ffmpeg-plugins/spec.md)

### 13.2 本仓库 README / CI

- [scripts/README.md](../../scripts/README.md) — 脚本索引（Phase 27 后大多数为历史）
- [app/encv-mobile/plugin-openlist/README.md](../encv-mobile/plugin-openlist/README.md) — Phase 26+ OpenListNativeService 架构
- [.github/workflows/android.yml](../../.github/workflows/android.yml) — **Phase 27 包含 fork 交叉编译 step（§5.6）**
- [app/encv-mobile/vite.config.ts](../encv-mobile/vite.config.ts) — `openlistUiProxy()` Vite 插件（§11.3）
- [app/encv-mobile/scripts/dev-openlist.sh](../encv-mobile/scripts/dev-openlist.sh) — 一键启动 OpenList dev 模式（§11.2）

### 13.3 外部仓库

- [Hi-Sillot/OpenList](https://github.com/Hi-Sillot/OpenList) — 个人 fork，本项目主依赖（**本地路径**：`app/openlist/Hi-Sillot-OpenList/`）
- [Hi-Sillot/OpenList-Frontend](https://github.com/Hi-Sillot/OpenList-Frontend) — fork 前端
- [OpenListTeam/OpenList](https://github.com/OpenListTeam/OpenList) — 上游
- [OpenListTeam/OpenList-Frontend](https://github.com/OpenListTeam/OpenList-Frontend) — 上游前端
- [K-Sillot/OpenList-Mobile](https://github.com/K-Sillot/OpenList-Mobile) — gomobile bind 路线**已废弃**参考
- [K-Sillot/OpenList-Desktop](https://github.com/K-Sillot/OpenList-Desktop) — 桌面端参考

---

## 维护

- 工作流变更 → 同步更新本 README §4-§9；新 ENCV 设置项 → 同步 fork `internal/conf/const.go` + `frontend-pinned.txt` + `i18n-overlay/<lang>/translation.json`；新 CI 失败场景 → 追加到 §12 故障表末。
- Vite 插件变更（`openlistUiProxy()`）→ 同步更新 §11.3 + §11.4；新增 dev 脚本参数 → 更新 §11.2；新增故障模式 → 追加到 §12 末尾。
