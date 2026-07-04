# 计划：修复 ndk-build 编译缺少 mpv/client.h 头文件

## 错误

```
jni/property.cpp:3:10: fatal error: 'mpv/client.h' file not found
jni/render.cpp:2:10: fatal error: 'mpv/client.h' file not found
jni/main.cpp:7:10:  fatal error: 'mpv/client.h' file not found
jni/event.cpp:2:10: fatal error: 'mpv/client.h' file not found
```

**根因**：AAR 只包含预编译的 `.so` 文件（动态库），不包含 C/C++ 头文件。编译 `libplayer.so`（JNI wrapper）时需要 `<mpv/client.h>`、`<libavcodec/jni.h>`、`<libswscale/swscale.h>` 等头文件。

## 方案：从 AAR 提取 + 下载缺失的头文件

### Step 1：检查 AAR 是否包含 headers 目录

AAR 可能包含 `headers/` 或 `include/` 目录。需要在 `setup-mpv-libs.sh` 的 Phase 1 中额外提取。

### Step 2：下载 mpv 开发头文件

如果 AAR 没有头文件，需要从以下来源获取：

**方案 A（推荐）：从 mpv-android release assets 获取**
- mpv-android CI 构建时会打包头文件到 AAR 的 `headers/` 目录
- 但 abdallahmehiz 的 fork 可能没包含

**方案 B：直接下载 mpv 最小头文件集**
- 只需要 `client.h` + 几个核心头文件（~20KB）
- 来源：mpv 官方仓库或 mpv-android 的 `app/src/main/jni/` 目录

**方案 C：从 AAR 解压出 include/ 目录**
- 先用 `unzip -l` 列出 AAR 内容确认

### Step 3：Android.mk 添加 include 路径

```makefile
LOCAL_C_INCLUDES := $(PREBUILT_DIR)/../include $(PREBUILT_DIR)/../../jni/include
```

### 具体实施

#### 修改 setup-mpv-libs.sh

在 Phase 1 提取 .so 后增加：
```bash
# Extract headers from AAR if present
mkdir -p "$JNI_DIR/$abi/include"
unzip -o -j "$AAR_TMP" "headers/*.h" -d "$JNI_DIR/$abi/include" 2>/dev/null || true
unzip -o -j "$AAR_TMP" "include/*.h" -d "$JNI_DIR/$abi/include" 2>/dev/null || true

# If no headers in AAR, download minimal set
if [ ! -f "$JNI_DIR/$abi/include/mpv/client.h" ]; then
    echo "  downloading mpv headers..."
    # Download from mpv-android master branch
fi
```

#### 下载最小头文件集

需要的头文件列表：
```
mpv/client.h          — mpv 核心 API（必需）
mpv/opengl_cb.h       — 可选
libavcodec/jni.h      — ffmpeg JNI bridge（thumbnail.cpp 需要）
libswscale/swscale.h  — swscale API（thumbnail.cpp 需要）
```

#### 修改 Android.mk

在 libplayer module 定义中添加：
```makefile
LOCAL_C_INCLUDES := $(PREBUILT_DIR)/include
```

## 预期结果

ndk-build 能找到所有 `#include` 的头文件，成功编译出 `libplayer.so`。
