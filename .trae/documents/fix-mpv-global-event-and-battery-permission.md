# 修复计划：MPV 播放 + 电池优化权限 + 防重犯

## 问题 1（最关键）：MPV 播放卡在 loading

### 根因分析（从 logcat 发现两个独立 bug）

**Bug A：GlobalEventEmitter 不可用**

logcat 第 190 行：`"GlobalEventEmitter not available"`

当前代码使用 `globalThis.lynx.getJSModule('GlobalEventEmitter')` 注册事件监听器，但在 ReactLynx 后台线程中返回 null。原生端通过 `lynxContext.sendGlobalEvent()` 发送了 `mpv:state-change` 事件（logcat 364-378 行确认发送成功），但 JS 端无法接收。

**正确做法**：ReactLynx 提供了官方 hook `useLynxGlobalEventListener`（位于 `@lynx-js/react/runtime/lib/core/hooks/useLynxGlobalEventListener.js`），内部使用 `lynx.getJSModule('GlobalEventEmitter')`（注意是 `lynx` 全局变量，不是 `globalThis.lynx`），并标记 `'background only'`，使用 `useMemo` 尽早注册监听器。

```javascript
// useLynxGlobalEventListener 源码关键部分
export function useLynxGlobalEventListener(eventName, listener) {
    'background only';
    useMemo(() => {
        lynx.getJSModule('GlobalEventEmitter').addListener(eventName, listener);
    }, [eventName, listener]);
    useEffect(() => {
        return () => {
            lynx.getJSModule('GlobalEventEmitter').removeListener(eventName, listener);
        };
    }, []);
}
```

**Bug B：`lynx_player_root not found`**

logcat 第 362/424 行：`init auto-attach: lynx_player_root not found`

MpvPlayerModule 的 `init` 块在主线程查找 `R.id.lynx_player_root`（来自 `lynx_player_activity.xml`），但 PlayerOverlayManager 的 overlay 布局使用 `R.id.player_overlay_root`（来自 `ids.xml`）。ID 不匹配导致 MPV SurfaceView 无法附加到布局。

同时存在时序问题：`tryAttachMpvModule()` 在 `onRuntimeReady` 时调用，但此时 MpvPlayerModule 尚未创建（logcat 354 行：`mpvModule not yet created`）。模块在 JS 首次调用 `NativeModules.MpvPlayerModule.play()` 时才创建，此时 `init` 块查找错误的 ID。

### 修复步骤

#### 1.1 PlayerApp.tsx：使用 `useLynxGlobalEventListener` 替代手动 GlobalEventEmitter 访问

```typescript
import { useLynxGlobalEventListener } from '@lynx-js/react'

// 在组件内：
useLynxGlobalEventListener('mpv:state-change', useCallback((event: any) => {
  const state = event?.state
  const error = event?.error
  // ... 处理状态变化
}, [filePath, setError]))

useLynxGlobalEventListener('mpv:position-update', useCallback((event: any) => {
  setPosition(event?.position ?? 0)
  setDuration(event?.duration ?? 0)
}, []))
```

删除整个手动 GlobalEventEmitter useEffect 块（第 238-308 行）。

#### 1.2 MpvPlayerModule.kt：修复 auto-attach ID 查找

```kotlin
val root = act.findViewById<android.widget.FrameLayout>(R.id.lynx_player_root)
    ?: act.findViewById<android.widget.FrameLayout>(R.id.player_overlay_root)
```

同时添加 PlayerOverlayManager 的 `tryAttachMpvModule` 重试机制：在 MpvPlayerModule 创建后（`init` 块末尾），通知 PlayerOverlayManager 重试附加。

---

## 问题 2：电池优化权限添加到设置权限管理组

### 当前架构

- 权限管理 UI 在 `ServerDetail.vue`（第 75-99 行）
- 权限检查/请求通过 `GoProcessPlugin.kt` Capacitor 插件
- 前端通过 `GoProcess.ts` 调用
- i18n 在 `useI18n.ts`

### 修复步骤

#### 2.1 GoProcessPlugin.kt：添加电池优化权限方法

```kotlin
@PluginMethod
fun requestBatteryOptimization(call: PluginCall) {
    val result = JSObject()
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
        val pm = context.getSystemService(Context.POWER_SERVICE) as PowerManager
        if (pm.isIgnoringBatteryOptimizations(context.packageName)) {
            result.put("granted", true)
        } else {
            try {
                val intent = Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS)
                intent.data = Uri.parse("package:${context.packageName}")
                activity.startActivity(intent)
            } catch (e: Exception) {
                Log.w(TAG, "requestBatteryOptimization failed", e)
            }
            result.put("granted", false)
            result.put("requiresSettings", true)
        }
    } else {
        result.put("granted", true)
    }
    call.resolve(result)
}
```

同时在 `checkPermissions` 中添加 `batteryOptimization` 字段。

#### 2.2 web.ts：扩展接口

`PermissionCheckResult` 添加 `batteryOptimization: boolean`
`GoProcessPlugin` 添加 `requestBatteryOptimization(): Promise<PermissionResult>`

#### 2.3 GoProcess.ts：添加导出函数

```typescript
export async function requestBatteryOptimization(): Promise<PermissionResult> { ... }
```

#### 2.4 ServerDetail.vue：添加电池优化权限 UI 项

在权限管理列表中添加电池优化权限项（在通知权限和存储权限之后）。

#### 2.5 useI18n.ts：添加 i18n 键

- `settings.batteryOptimization`: '忽略耗电优化' / 'Battery Optimization'

---

## 问题 3：完善防重犯机制

### 当前防重犯

- `package.json` build 脚本检查 `globalThis.NativeModules`
- `project_rules.md` 添加了 Lynx NativeModules 规则

### 需要增强

#### 3.1 添加 `globalThis.lynx.getJSModule` 检查

在 lynx-player build 脚本中同时检查 `globalThis.lynx.getJSModule`，因为应该使用 `useLynxGlobalEventListener` 而非手动访问。

#### 3.2 更新 project_rules.md

添加规则：
- 禁止使用 `globalThis.lynx.getJSModule('GlobalEventEmitter')`，应使用 `useLynxGlobalEventListener` hook
- NativeModules 模块名必须与 Android 端 `registerModule()` 第一个参数一致

---

## 修改文件清单

| 文件 | 修改内容 |
|------|----------|
| `lynx-player/src/player/PlayerApp.tsx` | 使用 `useLynxGlobalEventListener` 替代手动 GlobalEventEmitter |
| `android/.../MpvPlayerModule.kt` | auto-attach 添加 `player_overlay_root` fallback |
| `android/.../GoProcessPlugin.kt` | 添加 `requestBatteryOptimization` 方法，扩展 `checkPermissions` |
| `src/plugins/web.ts` | 扩展 `PermissionCheckResult` 和 `GoProcessPlugin` 接口 |
| `src/plugins/GoProcess.ts` | 添加 `requestBatteryOptimization` 导出函数 |
| `src/views/ServerDetail.vue` | 添加电池优化权限 UI 项 |
| `src/composables/useI18n.ts` | 添加电池优化 i18n 键 |
| `lynx-player/package.json` | build 脚本添加 `globalThis.lynx.getJSModule` 检查 |
| `.trae/rules/project_rules.md` | 添加 GlobalEventEmitter 和模块名规则 |
