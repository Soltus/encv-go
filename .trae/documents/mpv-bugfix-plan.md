# 修复 MPV 播放器两致命 Bug

## Bug 诊断

### Bug 1: 三种 MPV 模式静默回滚到 Artplayer（严重违反铁律）

**根因链**：
```
用户在 Settings 选择 "MPV (Activity)"
  → Settings.vue 存 localStorage('encv_player_video', 'mpv-activity')  ✅
  → Files.vue getPlayMode() 读取 'mpv-activity'
    → Line 474: if (stored === ARTPLAYER || MPV_PLUGIN || EXTERNAL) return stored
    → 'mpv-activity' 不匹配任何一项！→ 跳到 else → 返回 VIDEO_DEFAULT = 'artplayer'  ❌
  → switch(mode) 进入 case ARTPLAYER → router.push('/player')  ← 静默回滚！
```

**3 个断点全部断裂**：

| 断点 | 位置 | 问题 |
|------|------|------|
| `getPlayMode()` 白名单 | [Files.vue L474](app/encv-mobile/src/views/Files.vue#L474) | 只认 `ARTPLAYER/MPV_PLUGIN/EXTERNAL`，不认 `MPV_ACTIVITY/MPV_FRAGMENT/MPV_COMPOSE` |
| `switch(mode)` 分发 | [Files.vue L497](app/encv-mobile/src/views/Files.vue#L497) | 只有 `case MPV_PLUGIN`，缺少 3 个新 sub-mode 的 case |
| `openPlayer()` 参数 | [Files.vue L499](app/encv-mobile/src/views/Files.vue#L499) | 硬编码传 `PLAY_MODE.MPV_PLUGIN`，应传实际 `mode` 变量 |

---

### Bug 2: 安装后启用不正确 + 切换选项不刷新

**双重根因**：

#### 根因 2a：Kotlin 安装流程缺少 enable 步骤

```
PluginLifecycleEngine.installPlugin():
  installPlugin(apkFile, true)     → APK 安装到文件系统 + XML 持久化 ✅
  loadEnabledPlugins()             → 只加载 enabled=true 的插件 ⚠️
  返回 pi.enabled                  → ComboLite 安装后默认 disabled → false ❌

缺失步骤：从未调用 setPluginEnabled(pluginId, true) ！
```

结果：安装完成后插件处于 **disabled** 状态，`loadEnabledPlugins()` 跳过它，`getPluginInfo()` 返回 null。

#### 根因 2b：Settings 切换播放器选项时不刷新 MPV 徽章

```
Settings.vue handleVideoPlayerChange() (L425):
  videoPlayerMode.value = value        ← 更新 UI 选中项
  localStorage.setItem(...)              ← 持久化
  ❌ 缺少 refreshMpvPluginStatus() 调用！

用户切换到 "MPV (Activity)" → 徽章仍显示旧状态（或无状态）→ 不知道 MPV 实际是否可用
```

#### 根因 2c：ExtensionsPage 操作后 Settings 不感知

安装/启用/卸载成功后 ExtensionsPage 只刷新自身列表，未通知 Settings。

---

## 修复方案

### Task 1: 修复 Files.vue — mode 识别 + 分发 + 传参（Bug 1 核心修复）

#### SubTask 1.1: `getPlayMode()` 白名单扩展

```typescript
// Before (L474):
if (stored === PLAY_MODE.ARTPLAYER || stored === PLAY_MODE.MPV_PLUGIN || stored === PLAY_MODE.EXTERNAL)

// After:
if (isValidPlayMode(stored))
```

新增工具函数：
```typescript
import { PLAY_MODE } from '@/constants/player'

function isValidPlayMode(value: string): value is PlayMode {
  const allModes = [
    PLAY_MODE.ARTPLAYER,
    PLAY_MODE.MPV_PLUGIN,      // 兼容旧值
    PLAY_MODE.MPV_ACTIVITY,
    PLAY_MODE.MPV_FRAGMENT,
    PLAY_MODE.MPV_COMPOSE,
    PLAY_MODE.EXTERNAL,
  ]
  return allModes.includes(value as PlayMode)
}
```

#### SubTask 1.2: `switch(mode)` 分发覆盖所有 MPV 子模式

```typescript
switch (mode) {
  case PLAY_MODE.ARTPLAYER:
    router.push({ path: '/player', query: { path: file.path, name: file.name } })
    break
  // 所有 MPV 子模式统一走 native openPlayer
  case PLAY_MODE.MPV_PLUGIN:
  case PLAY_MODE.MPV_ACTIVITY:
  case PLAY_MODE.MPV_FRAGMENT:
  case PLAY_MODE.MPV_COMPOSE:
    if (isNative()) {
      const result = await openPlayer(file.path, file.name, mimeType, mode)
      if (!result.success) { /* 显示错误 banner */ }
    } else { router.push({ path: '/player', query: { path: file.path, name: file.name } }) }
    break
  case PLAY_MODE.EXTERNAL:
    // ... 保持不变 ...
    break
  default:
    console.warn('[Files] Unknown play mode:', mode, '— falling back to artplayer')
    router.push({ path: '/player', query: { path: file.path, name: file.name } })
    break
}
```

#### SubTask 1.3: `openPlayer()` 传参修正

```typescript
// Before: 硬编码 MPV_PLUGIN
const result = await openPlayer(file.path, file.name, mimeType, PLAY_MODE.MPV_PLUGIN)

// After: 传递用户选择的实际 mode
const result = await openPlayer(file.path, file.name, mimeType, mode)
```

---

### Task 2: 修复 Kotlin 安装流程 — 安装后自动 enable（Bug 2a 核心修复）

**修改文件**: [PluginLifecycleEngine.kt](app/encv-mobile/android/combolite-host/src/main/java/com/encvgo/combolite/engine/PluginLifecycleEngine.kt) L80-91

```kotlin
is InstallResult.Success -> {
    try { PluginManager.loadEnabledPlugins() } catch (_: Exception) {}
    val pluginId = pi.id
    if (!pi.enabled) {
        Log.i(TAG, "installPlugin: plugin $pluginId installed but disabled, enabling...")
        try { PluginManager.setPluginEnabled(pluginId, true) } catch (_: Exception) {}
        try { PluginManager.loadEnabledPlugins() } catch (_: Exception) {}
    }
    OperationResult.Success(PluginState(
        id = pi.id, name = pi.name, versionName = pi.versionName,
        versionCode = pi.versionCode, enabled = true,   // ← 安装后保证 enabled=true
        installed = true, entryClass = pi.entryClass, description = pi.description
    ))
}
```

**关键逻辑**：检查 `pi.enabled`，如果为 false 则自动调用 `setPluginEnabled(pluginId, true)` + 重新 `loadEnabledPlugins()`。

---

### Task 3: 修复 Settings — 切换选项时刷新 + 监听事件（Bug 2b + 2c）

#### SubTask 3.1: `handleVideoPlayerChange` / `handleAudioPlayerChange` 触发刷新

```typescript
async function handleVideoPlayerChange(event: CustomEvent) {
  const value = event.detail.value
  videoPlayerMode.value = value
  localStorage.setItem('encv_player_video', value)
  if (isMpvMode(value)) await refreshMpvPluginStatus()  // ← 新增！
}

async function handleAudioPlayerChange(event: CustomEvent) {
  const value = event.detail.value
  audioPlayerMode.value = value
  localStorage.setItem('encv_player_audio', value)
  if (isMpvMode(value)) await refreshMpvPluginStatus()  // ← 新增！
}
```

#### SubTask 3.2: ExtensionsPage 所有状态变更操作后广播事件

```typescript
// 安装成功后 (L208-209 之后)
window.dispatchEvent(new CustomEvent('plugin-state-changed'))

// 启用/禁用成功后 (L244 之后)
window.dispatchEvent(new CustomEvent('plugin-state-changed'))

// 卸载成功后 (L273 之后)
window.dispatchEvent(new CustomEvent('plugin-state-changed'))
```

#### SubTask 3.3: Settings.vue 监听事件

```typescript
onMounted(() => {
  window.addEventListener('plugin-state-changed', refreshMpvPluginStatus)
})
onUnmounted(() => {
  window.removeEventListener('plugin-state-changed', refreshMpvPluginStatus)
})
```

---

## 改动文件清单

| 文件 | 改动内容 |
|------|---------|
| [Files.vue](app/encv-mobile/src/views/Files.vue) | `isValidPlayMode()` + switch 覆盖子模式 + openPlayer 传实际 mode |
| [PluginLifecycleEngine.kt](app/encv-mobile/android/combolite-host/src/main/java/com/encvgo/combolite/engine/PluginLifecycleEngine.kt) | 安装后自动 `setPluginEnabled(true)` + 重载 |
| [Settings.vue](app/encv-mobile/src/views/Settings.vue) | ionChange 触发 refresh + 监听 `plugin-state-changed` |
| [ExtensionsPage.vue](app/encv-mobile/src/views/ExtensionsPage.vue) | 安装/启用/卸载后 dispatch 事件 |

## 铁律合规检查

| 铁律 | 合规方式 |
|------|---------|
| **严禁自动 fallback** | unknown mode 进 default 分支打 warning 日志，不再静默切 Artplayer |
| **严禁 Toast** | 错误通过现有 error banner 展示 |
| **饱和调试** | `[Files] Unknown play mode:` warning + Kotlin `installPlugin: plugin X installed but disabled, enabling...` |

## 验证清单

- [ ] **Bug 1**: Settings 选 "MPV (Activity)" → Files 打开视频 → Kotlin 端 `[ModeC-Activity]` 日志出现（非 Artplayer）
- [ ] **Bug 1**: 选 Fragment/Compose 同理，对应 `[ModeB-Fragment]` / `[ModeA-Compose]` 日志
- [ ] **Bug 2a**: 安装 MPV 插件 → 无需手动启用 → Settings 直接显示 ✓ ready
- [ ] **Bug 2b**: 切换播放器选项到任一 MPV 子模式 → 徽章立即刷新显示当前状态
- [ ] **Bug 2c**: 在 ExtensionsPage 启用/禁用/卸载 MPV → 切回 Settings → 徽章已自动更新
