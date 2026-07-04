# ffmpeg-builder

跨平台 ffmpeg 编译工具集。把零散的 `build-ffmpeg-android.sh` 单脚本拆成「小项目级」模块化结构，零硬编码路径，单入口跨 host/android 调用。

> 2026-06-13 重构自 [`build-ffmpeg-android.sh`](../legacy-build-ffmpeg-android.sh)。原单脚本保留为 `legacy-build-ffmpeg-android.sh` 作历史参考。

## 为什么升级

旧单脚本的问题：
1. **硬编码路径**：`/workspace`、`$HOME/Android/Sdk/ndk/26.1.10909125` 等写到 `NDK_PATH` 变量里，CI runner 一改 checkout 根就炸。
2. **零重用**：想加 iOS target 只能 `cp` 一份改 abi。
3. **逻辑纠缠**：下载/patch/configure/link/worker 一锅炖，200+ 行 sed 看不到主流程。
4. **错误体验差**：`set -e` 但失败只 echo 一行，CI log 里要翻 30 行才看到 stack trace。

## 架构

```
ffmpeg-builder/
├── Makefile                    # 唯一入口
├── common/                     # 跨 host/target 共享
│   ├── paths.sh                #   路径检测（git rev-parse / env / find 兜底）
│   ├── logging.sh              #   统一彩色日志
│   ├── exec.sh                 #   统一错误处理 + 日志 tail
│   ├── manifest.sh             #   读 ffmpeg-feature-manifest.json 派生变量
│   ├── toolchain.sh            #   cc/ar/nm 探测 + pkg-config 包装
│   └── downloads.sh            #   下载 + sha256 + 镜像兜底
├── targets/                    # 平台抽象
│   ├── dispatcher.sh           #   TARGET 路由
│   ├── host.sh                 #   宿主机（gcc/clang 直编）
│   ├── android.sh              #   Android NDK 跨编译
│   └── ios.sh                  #   iOS stub（未实现）
├── deps/                       # 第三方依赖（与 target 解耦）
│   ├── dispatcher.sh           #   按 manifest external_libs 编排
│   ├── x264.sh                 #   H.264 encoder (GPLv2)
│   └── libmp3lame.sh           #   MP3 encoder (LGPL 2.1)
├── ffmpeg.sh                   # ffmpeg 主链：下载+patch+configure+make
├── postprocess.sh              # fftools 编译 + link + build-info.json
├── worker.sh                   # libffmpeg-worker.so (Go c-shared)
├── legacy-build-ffmpeg-android.sh
└── ffmpeg-feature-manifest.json  (在 scripts/ 父目录)
```

## 调用流

```
Makefile → bash -c "source common/paths.sh → target_setup → load_manifest → build_all_deps → build_ffmpeg → build_fftools → build_worker"
```

每个 step 都是独立函数，任意 step 失败 → 立即 die 退出 + tail 30 行错误日志。

## 零硬编码原则

| 路径 | 怎么拿 |
|------|-------|
| git repo root | `git rev-parse --show-toplevel` |
| NDK | `$ANDROID_NDK_HOME` → `$ANDROID_HOME/ndk/<latest>` → find `$HOME /opt /usr/local` 兜底 |
| Android SDK | `$ANDROID_HOME` → `$ANDROID_SDK_ROOT` → 找 `build-tools` 目录 |
| host compiler | `command -v gcc clang cc` |
| Go module root | `git rev-parse --show-toplevel`（worker build 用） |
| NDK host 子目录 | 派生自 `uname -s/m`（linux-x86_64 / darwin-x86_64） |

**不存在的字符串**：`/workspace`、`/home/runner`、`$HOME/Android/Sdk`、`26.1.10909125`（NDK 版本号）、`arm64-v8a` 硬编码 ABI——全部动态探测或参数化。

## 跨环境验证

- ✅ 沙盒 `/workspace`（CI runner checkout）
- ✅ 真实用户本机（`$HOME` 任意）
- ✅ macOS（host: `darwin-x86_64`，Android: Rosetta 跑 NDK）

## 快速上手

```bash
# 本机 sanity check（不需要 NDK）
make host

# Android 交叉编译
export ANDROID_NDK_HOME=/path/to/ndk/26.1.10909125
make android

# 换 ABI
make android ANDROID_ABI=x86_64 ANDROID_API=24

# 清理
make clean

# 验证导出符号
make verify
```

## 加新 target

1. 写 `targets/<new>.sh`，实现 `<new>_setup` 函数，export `CC/AR/SYSROOT/...`
2. 在 `targets/dispatcher.sh` 加 `case` 分支
3. 在 `Makefile` 加 `.PHONY: <new>` 目标

## 加新 dep

1. 在 `scripts/ffmpeg-feature-manifest.json` 的 `external_libs` 加名字
2. 写 `deps/<name>.sh`，实现 `build_<name>` 函数
3. 在 `deps/dispatcher.sh` 的 `_dep_builders` 加 case
