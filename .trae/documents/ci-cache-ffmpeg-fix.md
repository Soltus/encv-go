# CI 缓存优化 + ffmpeg x264 编译修复

## 问题 1：CI 缓存

当前每次 CI 运行都要重新下载/安装/编译所有依赖，耗时很长。需要缓存以下内容：

### 可缓存项
| 缓存项 | 路径 | 缓存 Key | 预估节省 |
|--------|------|----------|----------|
| npm 依赖 (主项目) | `app/encv-mobile/node_modules` | `npm-${{ hashFiles('app/encv-mobile/package-lock.json') }}` | ~1min |
| npm 依赖 (lynx-player) | `app/encv-mobile/lynx-player/node_modules` | `npm-lynx-${{ hashFiles('app/encv-mobile/lynx-player/package-lock.json') }}` | ~30s |
| Go 模块缓存 | `~/go/pkg/mod` | `go-${{ hashFiles('go.sum') }}` | ~30s |
| Gradle 缓存 | `~/.gradle/caches`, `~/.gradle/wrapper` | `gradle-${{ hashFiles('app/encv-mobile/android/*.gradle', 'app/encv-mobile/android/app/*.gradle') }}` | ~1min |
| ffmpeg/x264 构建产物 | `app/encv-mobile/scripts/.ffmpeg-build` | `ffmpeg-${{ hashFiles('app/encv-mobile/scripts/build-ffmpeg-android.sh') }}` | ~5-10min |
| NDK | `$ANDROID_HOME/ndk/26.1.10909125` | `ndk-26.1.10909125` | ~1min |

### 实施步骤
1. 在 `android.yml` 中添加 `actions/cache@v4` 步骤
2. 修改 `build-ffmpeg-android.sh` 支持增量构建（检测已编译产物跳过）
3. npm install 步骤加 `--prefer-offline` 条件
4. Go build 利用 `setup-go` 自带的缓存机制（已内置）

---

## 问题 2：ffmpeg configure 找不到 x264

### 根因分析

x264.pc 已经被修复（移除了 `-lpthread -ldl`），`pkg-config --exists x264` 也通过了。但 ffmpeg configure 仍然报 "x264 not found"。

ffmpeg configure 检测外部库的流程：
1. `pkg-config --cflags x264` → 获取编译参数
2. `pkg-config --libs x264 --static` → 获取链接参数
3. **编译+链接一个测试程序** → 验证 x264 是否可用

第 3 步失败。原因：ffmpeg configure 使用交叉编译器编译测试程序后，会**尝试执行**它来验证。但 Android ARM64 二进制无法在 x86_64 CI 主机上运行。虽然 `--enable-cross-compile` 会跳过部分运行测试，但链接测试仍然会失败，因为：

- `--pkg-config-flags="--static"` 让 pkg-config 返回静态链接参数
- 静态链接 x264 需要完整的依赖链，而 Android 交叉编译环境下某些依赖可能缺失
- ffmpeg configure 的测试链接可能缺少必要的 `--sysroot` 或其他链接器标志

### 修复方案

**方案 A（推荐）：创建自定义 pkg-config wrapper + 添加 configure 失败诊断**

1. 创建一个 pkg-config wrapper 脚本，确保返回正确的 Android 交叉编译参数
2. 在 configure 失败时输出 `ffbuild/config.log` 的最后 50 行，方便诊断
3. 移除 `--pkg-config-flags="--static"`（不需要静态链接 x264，我们链接的是 .so）

**方案 B（备选）：绕过 pkg-config，手动指定 x264 路径**

使用 `--x264-internal` 或直接通过 `--extra-cflags`/`--extra-ldflags` 指定路径，不依赖 pkg-config。

### 具体实施

#### build-ffmpeg-android.sh 修改

```bash
# 1. 创建自定义 pkg-config wrapper
cat > "${BUILD_DIR}/pkg-config-wrapper" << 'EOF'
#!/bin/bash
SYSROOT="${TOOLCHAIN}/sysroot"
PKG_CONFIG_PATH="${BUILD_DIR}/x264-install/lib/pkgconfig" \
PKG_CONFIG_LIBDIR="${BUILD_DIR}/x264-install/lib/pkgconfig" \
PKG_CONFIG_SYSROOT_DIR="$SYSROOT" \
pkg-config "$@"
EOF
chmod +x "${BUILD_DIR}/pkg-config-wrapper"

# 2. 修改 ffmpeg configure 参数
./configure \
    ... \
    --pkg-config="${BUILD_DIR}/pkg-config-wrapper" \
    --pkg-config-flags="--static" \
    ...

# 3. configure 失败时输出诊断日志
./configure ... || {
    echo "=== ffmpeg configure FAILED ==="
    tail -50 ffbuild/config.log
    exit 1
}
```

#### android.yml 缓存修改

在关键步骤前插入缓存恢复步骤，构建后自动保存。

---

## 实施步骤

### Step 1: 修复 build-ffmpeg-android.sh
- 创建 pkg-config wrapper 脚本
- 用 `--pkg-config=` 指定 wrapper
- configure 失败时输出 `ffbuild/config.log` 诊断
- 增量构建支持（检测已存在的 .so 跳过编译）

### Step 2: 添加 CI 缓存
- npm node_modules 缓存
- lynx-player node_modules 缓存
- Go 模块缓存（setup-go 内置）
- ffmpeg/x264 构建产物缓存
- NDK 缓存
- Gradle 缓存

### Step 3: 优化 CI 步骤
- npm install 加缓存命中判断
- ffmpeg 构建步骤加缓存命中判断
- 移除冗余验证步骤（合并到构建步骤中）
