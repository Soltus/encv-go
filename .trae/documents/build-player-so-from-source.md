# 计划：从源码编译 libplayer.so（JNI wrapper）解决 JNI 不匹配

## 问题分析

### 错误链路演进

| 阶段 | 错误 | 根因 |
|------|------|------|
| 第 1 次 | `dlopen failed: "libavcodec.so" not found` | ffmpeg 动态库缺失 |
| **第 2 次（当前）** | `No implementation found for void MPVLib.create(Context)` | **JNI 函数签名不匹配** |

ffmpeg 问题已修复 ✅。当前错误说明：
- ✅ `System.loadLibrary("mpv")` 成功 — libmpv.so 加载了
- ✅ `System.loadLibrary("player")` 也成功了 — libplayer.so 加载了
- ❌ 但 `libplayer.so` 内注册的 JNI 函数名与我们的 `MPVLib.kt` 的 `external fun` 声明**不匹配**

### 根因：预编译 AAR 的 libplayer.so 和我们的 MPVLib.kt 不是同一版本

```
abdallahmehiz 的 AAR 构建流程：
  某个版本的 MPVLib.java → javac → javah → 生成 JNI 头 → 编译 C++ → libplayer.so

我们的代码：
  我们手写的 MPVLib.kt → Kotlin 编译器生成不同的 JNI 名称 → 运行时找不到
```

具体差异可能来自：
- 原版是 `class MPVLib`（Java），我们的是 `object MPVLib`（Kotlin object = static methods）
- 方法参数注解不同（`@NonNull` vs 无注解）
- Kotlin 编译器的 JNI name mangling 与 javah 不同
- 版本差异导致方法列表增减

**用户说得对：抄别人预编译的二进制文件不可靠，必须从源码构建。**

## 方案：从源码编译 libplayer.so

### 架构思路

```
┌─────────────────────────────────────────────┐
│              我们的代码                       │
│  MPVLib.kt (external fun 声明)               │
│  MpvPlayerModule.kt (调用层)                 │
└───────────┬─────────────────────────────────┘
            │ JNI 调用
            ▼
┌─────────────────────────────────────────────┐
│        libplayer.so ← 从源码编译 ✅           │
│  main.cpp / event.cpp / property.cpp         │
│  (来自 mpv-android 开源项目)                  │
└───────────┬─────────────────────────────────┘
            │ C API 调用
            ▼
┌─────────────────────────────────────────────┐
│     libmpv.so + ffmpeg .so                   │
│     (从 AAR 提取，预编译好的)                  │
└─────────────────────────────────────────────┘
```

**关键原则**：只编译 JNI wrapper 层（C++，~500行），mpv 核心 + ffmpeg 继续用预编译的 .so。

### 为什么不全量编译 mpv？

- mpv 从源码编译需要 ~30 分钟 + 完整 toolchain（meson, python, nasm 等）
- ffmpeg 更复杂（需要 yasm, 各种 codec 库）
- mpv-android 的 CI 用专用 runner + 大量缓存
- **我们只需要让 JNI 函数名匹配**，核心播放引擎不需要改

### 数据来源

**JNI C++ 源码**来自 [mpv-android](https://github.com/mpv-android/mpv-android) 主仓库（sfan5 维护），包含：

| 文件 | 行数 | 功能 |
|------|------|------|
| [main.cpp](https://github.com/mpv-android/mpv-android/blob/master/app/src/main/jni/main.cpp) | ~117行 | create/init/destroy/attachSurface/detachSurface/command/setOptionString |
| [event.cpp](https://github.com/mpv-android/mpv-android/blob/master/app/src/main/jni/event.cpp) | ~200行 | 事件线程、事件回调分发到 Java |
| [property.cpp](https://github.com/mpv-android/mpv-android/blob/master/app/src/main/jni/property.cpp) | ~150行 | getProperty*/setProperty* 实现 |
| [log.cpp](https://github.com/mpv-android/mpv-android/blob/master/app/src/main/jni/log.cpp) | ~50行 | 日志回调 |
| [jni_utils.h](https://github.com/mpv-android/mpv-android/blob/master/app/src/main/jni/jni_utils.h) | ~80行 | JNI 工具宏 |
| [jni_utils.cpp](https://github.com/mpv-android/mpv-android/blob/master/app/src/main/jni/jni_utils.cpp) | ~100行 | JNI 工具函数 |
| [Android.mk](https://github.com/mpv-android/mpv-android/blob/master/app/src/main/jni/Android.mk) | ~100行 | ndk-build 配置（定义所有 PREBUILT_SHARED_LIBRARY） |
| [Application.mk](https://github.com/mpv-android/mpv-android/blob/master/app/src/main/jni/Application.mk) | ~15行 | ABI 选择 |

## 实施步骤

### Step 1：创建 scripts/build-player-so.sh — 从源码编译 libplayer.so

新建脚本，逻辑：

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
JNI_DIR="$PROJECT_DIR/android-overlay/app/src/main/jni"
OUTPUT_DIR="$PROJECT_DIR/android-overlay/app/src/main/jniLibs/arm64-v8a"
NDK_ROOT="${ANDROID_NDK_HOME:-$HOME/Android/Sdk/ndk}"

# 1. 确保 NDK 存在
if [ ! -d "$NDK_ROOT" ]; then
    # 尝试常见路径
    for d in "$HOME/Android/Sdk/ndk"/*/; do
        if [ -d "$d" ]; then NDK_ROOT="$d"; break; fi
    done
fi
if [ ! -d "$NDK_ROOT" ] || [ -z "$(ls -A "$NDK_ROOT" 2>/dev/null)" ]; then
    echo "ERROR: Android NDK not found at $NDK_ROOT"
    echo "Set ANDROID_NDK_HOME or install via sdkmanager"
    exit 1
fi
echo "Using NDK: $NDK_ROOT"

# 2. 创建 jni 目录结构
mkdir -p "$JNI_DIR"
mkdir -p "$OUTPUT_DIR"

# 3. 下载 JNI 源码（如果不存在）
# 来源：mpv-android 主仓库的 app/src/main/jni/
MPV_ANDROID_VER="master"
SRC_DIR="$PROJECT_DIR/.cache/mpv-android-jni"
if [ ! -d "$SRC_DIR" ]; then
    echo "Downloading mpv-android JNI sources..."
    mkdir -p "$SRC_DIR"
    # 只下载需要的 jni 文件
    BASE="https://raw.githubusercontent.com/mpv-android/mpv-android/$MPV_ANDROID_VER/app/src/main/jni"
    for f in main.cpp event.cpp property.cpp log.cpp jni_utils.h jni_utils.cpp log.h Android.mk Application.mk; do
        curl -fSL "$BASE/$f" -o "$SRC_DIR/$f" || { echo "Failed to download $f"; exit 1; }
    done
fi
echo "JNI sources ready in $SRC_DIR"

# 4. 复制源码到 jni 目录
cp "$SRC_DIR"/*.cpp "$SRC_DIR"/*.h "$SRC_DIR"/*.mk "$JNI_DIR"/

# 5. 修改 Android.mk — 使用我们的预编译库路径
# 将 $(PREFIX)/lib/... 改为指向 OUTPUT_DIR 的预编译 .so

# 6. 用 ndk-build 编译
$NDK_ROOT/ndk-build \
    -C "$PROJECT_DIR/android-overlay/app/src/main" \
    APP_ABI=arm64-v8a \
    APP_PLATFORM=android-21 \
    NDK_PROJECT_PATH=. \
    NDK_APPLICATION_MK=jni/Application.mk \
    -j$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)

echo "✅ libplayer.so built successfully"
ls -lh "$OUTPUT_DIR/libplayer.so"
```

### Step 2：修改 Android.mk — 适配我们的预编译库路径

原始 Android.mk 引用 `$(PREFIX64)/lib/libxxx.so`（CI 环境变量）。我们需要改为直接引用 `jniLibs/arm64-v8a/` 下已提取的 .so 文件。

关键修改：
```makefile
# 原始:
LOCAL_SRC_FILES := $(PREFIX64)/lib/libmpv.so

# 改为:
LOCAL_SRC_FILES := ../jniLibs/arm64-v8a/libmpv.so
```

### Step 3：确保 MPVLib.kt 与 JNI 源码完全一致

对比我们的 [MPVLib.kt](file:///workspace/app/encv-mobile/android-overlay/app/src/main/java/is/xyz/mpv/MPVLib.kt) 与 [mpv-android 主仓库的 MPVLib.kt](https://github.com/mpv-android/mpv-android/blob/master/app/src/main/java/is/xyz/mpv/MPVLib.kt)，确保每个 `external fun` 的签名完全匹配。

已知需要关注的点：
- 包名：`` `is`.xyz.mpv `` （已正确）
- 类声明：`object MPVLib`（Kotlin singleton，等价于 Java 的 static methods）
- 方法名和参数类型必须逐字匹配

### Step 4：更新 setup-mpv-libs.sh — 分离两步

改为两阶段：
1. **Phase 1**（现有）：从 AAR 提取 libmpv.so + 全部 ffmpeg .so（预编译核心库）
2. **Phase 2**（新增）：调用 `build-player-so.sh` 编译 libplayer.so（JNI wrapper）

```bash
# Phase 1: 提取预编译库
unzip -o -j "$AAR_TMP" "jni/$abi/*.so" -d "$JNI_DIR/$abi"
# 排除 libplayer.so（我们要自己编译）
rm -f "$JNI_DIR/$abi/libplayer.so"

# Phase 2: 编译 JNI wrapper
bash "$SCRIPT_DIR/build-player-so.sh"
```

### Step 5：CI 适配

在 [android.yml](file:///workspace/.github/workflows/android.yml) 中：
- 安装 NDK（如果尚未安装）
- 在 "Setup mpv native libraries" 步骤后增加 "Build JNI wrapper" 步骤
- 或将 Phase 2 合并到现有脚本中

## 风险评估

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| NDK 版本不兼容 | 低 | 编译失败 | 使用 r21-r29 LTS 版本 |
| JNI 源码和 MPVLib.kt 版本不匹配 | 低 | 同样的 JNI 错误 | 严格对照同一 commit 的两边 |
| mpv-android 的 Android.mk 引用外部 PREFIX 变量 | 高 | 需要改造 | Step 2 已规划适配 |
| 编译耗时 | 中 | CI 时间增加 | 只编译 ~500行 C++，预计 < 2min |

## 不做的事

- ❌ 不从源码编译 mpv 本体（太慢，~30min+，需要 meson/python/nasm）
- ❌ 不从源码编译 ffmpeg（更慢，~1h+）
- ✅ 只编译 JNI wrapper 薄层（快速，且解决了根本问题）
