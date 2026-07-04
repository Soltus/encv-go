# 修复 MPV AAR 403 + 架构优化

## 用户需求

1. **新增独立 Action** `build-mpv-lib.yml` — 构建 mpv jniLibs，上传到固定 GitHub Release（重复替换）
2. **主 Android workflow** 从该 Release 消费预构建的 mpv lib（不再从 Maven Central 下载）
3. **Android 手动触发器** 新增"只构建主应用"选项（跳过插件）
4. **工件保留时间** 30 天 → 3 天

## 文件变更总览

| 操作 | 文件 | 说明 |
|------|------|------|
| **新建** | `.github/workflows/build-mpv-lib.yml` | 独立 Action: 构建 mpv lib → 上传到固定 Release |
| **修改** | `.github/workflows/android.yml` | 从 Release 下载 mpv lib; 新增 skip-plugin 输入; retention 改为 3 天 |
| **修改** | `app/encv-mobile/scripts/setup-mpv-libs.sh` | 改为支持两种模式: 本地开发(AAR下载) / CI(Release 下载) |

---

## 一、新建 `build-mpv-lib.yml`

### 触发方式

```yaml
on:
  workflow_dispatch:
    inputs:
      branch:
        description: 'Branch to build'
        required: false
        default: ''
        type: string
```

### 完整流程

```
checkout → setup node → download AAR (curl with retry) → extract .so → ndk-build libplayer.so
→ 打包 jniLibs 为 zip → 上传到固定 Release "mpv-native-libs" (覆盖同名 asset)
```

### 关键设计

**Release 固定名称**: `mpv-native-libs`（不存在则自动创建）

**Asset 命名规则**: `mpv-jniLibs-arm64-v8a-v{MPV_LIB_VERSION}.zip`

**上传逻辑**（使用 `softprops/action-gh-release@v2`）:
- 先删除同名的旧 asset（`--clobber` 或先 list 再 delete）
- 再上传新 asset
- 这样保证始终只有一个最新版本

**完整 YAML 结构**:

```yaml
name: Build MPV Native Libraries

on:
  workflow_dispatch:
    inputs:
      branch:
        description: 'Branch to build (defaults to current)'
        required: false
        default: ''
        type: string

env:
  NDK_VERSION: "26.1.10909125"
  MPV_LIB_VERSION: "0.1.12"

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ inputs.branch || github.ref_name }}

      - name: Setup Android SDK
        uses: android-actions/setup-android@v3

      - name: Cache Android NDK
        id: cache-ndk
        uses: actions/cache@v4
        with:
          path: /usr/local/lib/android/sdk/ndk/${{ env.NDK_VERSION }}
          key: ndk-${{ env.NDK_VERSION }}

      - name: Install Android NDK
        if: steps.cache-ndk.outputs.cache-hit != 'true'
        run: yes | sdkmanager --install "ndk;${{ env.NDK_VERSION }}" > /dev/null 2>&1

      - name: Download & Extract MPV AAR
        run: |
          # ... curl with retry, unzip, extract to jniLibs ...

      - name: Build libplayer.so via ndk-build
        run: |
          # ... ndk-build libplayer.so ...

      - name: Package jniLibs as zip
        run: |
          cd app/encv-mobile/plugin-mpv-player/src/main/jniLibs
          zip -r "${{ runner.temp }}/mpv-jniLibs-arm64-v8a-v${{ env.MPV_LIB_VERSION }}.zip" .

      - name: Upload to GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          tag_name: mpv-native-libs
          name: "MPV Native Libraries (auto-generated)"
          replace_existing_artifacts: true
          files: ${{ runner.temp }}/mpv-jniLibs-arm64-v8a-v${{ env.MPV_LIB_VERSION }}.zip
```

> 注: `softprops/action-gh-release@v2` 的 `replace_existing_artifacts: true` 会自动创建 release（如果不存在）并覆盖同名 asset。

---

## 二、修改 `android.yml`

### 2.1 手动触发器新增输入

```yaml
on:
  workflow_dispatch:
    inputs:
      branch:
        description: 'Branch to build (defaults to current branch)'
        required: false
        default: ''
        type: string
      version:
        description: 'Version number (e.g. 1.0.0). When set, builds release APK.'
        required: false
        default: ''
        type: string
      skip_plugin:
        description: 'Skip MPV plugin build (only build host app)'
        required: false
        default: 'false'
        type: boolean
```

### 2.2 替换 "Setup mpv native libraries" 步骤

**原来** (直接运行脚本下载 AAR):
```yaml
- name: Setup mpv native libraries
  run: cd app/encv-mobile && bash scripts/setup-mpv-libs.sh
```

**改为** (从 GitHub Release 下载预构建包):
```yaml
- name: Download MPV native libraries from Release
  run: |
    cd app/encv-mobile/plugin-mpv-player/src/main
    mkdir -p jniLibs/arm64-v8a
    # 从 mpv-native-libs release 下载最新 asset
    ASSET_URL=$(curl -sf \
      "https://api.github.com/repos/${GITHUB_REPOSITORY}/releases/tags/mpv-native-libs" \
      | jq -r '.assets[] | select(.name | startswith("mpv-jniLibs-arm64")) | .browser_download_url')
    if [ -z "$ASSET_URL" ]; then
      echo "::error::No MPV lib asset found in release 'mpv-native-libs'. Run 'Build MPV Native Libraries' workflow first."
      exit 1
    fi
    echo "Downloading: $ASSET_URL"
    curl -fSL -o /tmp/mpv-jniLibs.zip "$ASSET_URL"
    unzip -o /tmp/mpv-jniLibs.zip -d jniLibs/
    echo "✅ MPV libs extracted:"
    ls -lhR jniLibs/
```

### 2.3 条件化插件相关步骤

以下步骤添加 `if: inputs.skip_plugin != true`:

| 步骤名 | 当前行号范围 |
|--------|------------|
| Build MPV player plugin (JNI + Kotlin) | L228-L241 |
| Package MPV plugin as APK | L243-L261 |
| Verify plugin APK contents | L263-L277 |

### 2.4 验证步骤中插件检查也条件化

"Plugin APK in assets" 部分在 skip_plugin 时改为 warning 而非 error。

### 2.5 工件保留时间

```yaml
# Upload APK artifact step
retention-days: 3   # 原: 30
```

---

## 三、修改 `setup-mpv-libs.sh`

保留脚本但调整用途：

- **本地开发时**: 行为不变（从 Maven Central 下载 AAR），方便开发者本地调试
- **CI 中不再调用此脚本**: 由新 Action 替代
- 添加环境变量检测: 如果检测到 CI 环境变量 (`CI=true`)，打印提示信息引导用户使用独立 Action

```bash
if [ "${CI:-}" = "true" ]; then
    echo "::warning::In CI, mpv libs should come from 'build-mpv-lib' workflow Release."
    echo "::warning::Falling back to direct AAR download..."
fi
```

---

## 四、执行顺序

1. 创建 `.github/workflows/build-mpv-lib.yml`
2. 修改 `app/encv-mobile/scripts/setup-mpv-libs.sh`
3. 修改 `.github/workflows/android.yml`
4. 推送后手动触发 `build-mpv-lib` 生成首次 Release
5. 之后正常触发 `android` workflow 即可消费预构建的 mpv lib
