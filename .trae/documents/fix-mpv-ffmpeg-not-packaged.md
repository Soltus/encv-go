# 修复：mpv/ffmpeg .so 文件未打包进 APK

## 根因分析

### 问题现象
CI 构建成功，但 APK 中缺少 mpv 和 ffmpeg 的 .so 文件（libmpv.so、libavcodec.so 等），导致运行时 `System.loadLibrary("mpv")` 失败。

### 根因追踪

**直接原因**：`sync-native.mjs` 脚本在 CI 中从未被执行，mpv/ffmpeg .so 文件从 `android-overlay/` 目录从未被复制到 `android/app/src/main/jniLibs/`。

**根本原因**：上一轮优化将 CI 从 `cap sync` 改为 `cap copy`，但 `sync-native.mjs` 注册在 `capacitor:sync:after` 钩子上，而 **`cap copy` 不触发 `capacitor:sync:after`**。

Capacitor 钩子触发规则：
| 命令 | 触发的钩子 |
|------|-----------|
| `cap copy` | `capacitor:copy:after` ✅ |
| `cap update` | `capacitor:update:after` |
| `cap sync` (= copy + update) | `capacitor:copy:after` + `capacitor:update:after` + `capacitor:sync:after` ✅ |
| `cap build` (内部含 sync) | 以上全部 |

### 日志证据

1. **Step 11**：mpv/ffmpeg .so 成功下载到 `android-overlay/app/src/main/jniLibs/arm64-v8a/`（11 个文件）
2. **Step 15**：`cap copy` 执行，但**未触发** `sync-native.mjs`（日志中无 `encv-sync-native` 输出）
3. **Step 16**：`jniLibs/arm64-v8a/` 目录中**只有** `libencv-go.so`（手动复制的 Go 二进制）
4. **Step 21**：APK 中只有 `libencv-go.so` + Lynx .so，**无** mpv/ffmpeg .so

---

## 修复方案

### Step 1：修改 package.json 钩子

将 `capacitor:sync:after` 改为 `capacitor:copy:after`：

```json
"capacitor:copy:after": "node scripts/sync-native.mjs"
```

**为什么用 `copy:after` 而不是 `sync:after`**：
- `cap copy` 触发 `copy:after` ✅
- `cap sync` 内部包含 `cap copy`，也会触发 `copy:after` ✅
- `cap build` 内部包含 `cap sync`，也会触发 `copy:after` ✅
- `sync-native.mjs` 只做文件复制，不依赖 `cap update` 产物，放在 `copy:after` 时机正确

### Step 2：在 CI 中增加 mpv/ffmpeg .so 验证步骤

在 "Copy Go binary to jniLibs" 步骤之后，增加验证：

```yaml
- name: Verify native libraries
  run: |
    echo "=== jniLibs contents ==="
    find app/encv-mobile/android/app/src/main/jniLibs -name "*.so" -type f | sort
    echo "=== Checking mpv/ffmpeg .so files ==="
    JNI_DIR="app/encv-mobile/android/app/src/main/jniLibs/arm64-v8a"
    for lib in libmpv.so libavcodec.so libavformat.so libplayer.so; do
      if [ -f "$JNI_DIR/$lib" ]; then
        echo "✅ $lib found ($(ls -lh "$JNI_DIR/$lib" | awk '{print $5}'))"
      else
        echo "❌ $lib MISSING!"
        exit 1
      fi
    done
```

### Step 3：在 APK 验证步骤中增加 mpv/ffmpeg 检查

在 "Verify APK contents" 步骤中增加：

```bash
echo "=== mpv/ffmpeg .so in APK ==="
unzip -l "$APK_PATH" | grep -E "libmpv|libavcodec|libavformat|libplayer" || echo "❌ mpv/ffmpeg .so NOT in APK!"
```

---

## 影响范围

| 场景 | 修复前 | 修复后 |
|------|--------|--------|
| Debug 构建 (`cap copy`) | ❌ sync-native.mjs 不执行 | ✅ copy:after 触发 |
| Release 构建 (`cap build`) | ✅ sync:after 触发（通过内部 sync） | ✅ copy:after 触发（通过内部 copy） |
| 本地开发 (`cap sync`) | ✅ sync:after 触发 | ✅ copy:after 触发 |

## 修改文件

1. `/workspace/app/encv-mobile/package.json` — 钩子 `sync:after` → `copy:after`
2. `/workspace/.github/workflows/android.yml` — 增加验证步骤
