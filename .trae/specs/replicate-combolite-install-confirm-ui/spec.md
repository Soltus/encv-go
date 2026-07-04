# 复刻 ComboLite 安装确认界面 Spec

## Why

当前应用的插件安装流程：
1. 用户在 ExtensionsPage 点击「选择 APK」
2. GoProcessPlugin 调用 `PluginManager.installerManager.installPlugin()`（已修复为直接 API）
3. 如果签名不匹配 + ValidationStrategy != Insecure → ComboLite 内部会启动 `AuthorizationActivity` → 显示 `InstallPermissionScreen`

但当前 EncvApplication 使用的是 `ValidationStrategy.Insecure`，**跳过了授权确认界面**。用户要求复刻 ComboLite 官方的安装确认界面，让用户在安装前看到插件信息并确认。

**目标**：创建一个与 ComboLite `InstallPermissionScreen` 视觉一致的安装确认 Activity，在 `installPlugin()` 调用**之前**展示给用户，用户确认后才执行实际安装。

## What Changes

- 新建 `InstallConfirmActivity.kt`（Compose Material3 Activity），复刻官方 `InstallPermissionScreen` 布局
- 修改 `GoProcessPlugin.kt` 的安装流程：先启动 InstallConfirmActivity，用户确认后再调用 `installPlugin()`
- 从 APK 文件中提取图标、名称、版本、包名等元数据展示给用户

## Impact

- Affected code:
  - `app/encv-mobile/android/app/src/main/java/com/encvgo/app/GoProcessPlugin.kt` — 安装流程增加确认步骤
  - 新建 `app/encv-mobile/android/app/src/main/java/com/encvgo/app/InstallConfirmActivity.kt`
  - `app/encv-mobile/android/app/src/main/AndroidManifest.xml` — 注册新 Activity
- Affected specs: `fix-all-combolite-integration-defects`（在上一个 spec 基础上增强）

---

## ADDED Requirements

### Requirement 1: InstallConfirmActivity（Compose 界面）

系统 SHALL 提供一个 Compose Material3 Activity，视觉布局对齐 ComboLite 官方 `InstallPermissionScreen`：

```
┌──────────────────────────────────────────────┐
│  ←  (TopAppBar, transparent bg)             │
├──────────────────────────────────────────────┤
│                                              │
│  ┌──────┐  ┌─────────────────────────────┐  │
│  │ ICON │  │ Plugin Name                  │  │
│  │ 56dp │  │ 版本 x.x.x                   │  │
│  │圆角12│  └─────────────────────────────┘  │
│  └──────┘                                    │
│                                              │
│         (Spacer 24dp)                         │
│  ┌── ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─┐   │
│  │ ⚠️ 即将安装以下插件到本应用          │   │ ← tertiaryContainer 圆角10
│  ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─┘   │
│                                              │
│         (Spacer 24dp)                         │
│  ┌ InfoRow: 文件名    xxx.apk ────────────┐ │
│  ├ InfoRow: 包名      com.example.xxx ────┤ │
│  └ InfoRow: 大小      xx.x MB ────────────┘ │
│                                              │
│              (Spacer flex=1)                 │
│  ┌──────────────────────────────────────┐   │
│  │           [ 确认安装  ]               │   │ ← PrimaryButton fillMaxWidth
│  └──────────────────────────────────────┘   │
│            [ 取消 ]                        │   ← TextButton
│                                              │
└──────────────────────────────────────────────┘
```

#### Scenario 1.1: 正常确认流程
- **WHEN** 用户选择 APK 后，系统解析 APK 元数据并启动 InstallConfirmActivity
- **THEN** 用户看到：插件图标（从 APK 提取或默认）、文件名、包名、文件大小
- **AND** 点击「确认安装」→ Activity 返回 RESULT_OK → GoProcessPlugin 执行 `installPlugin()`
- **AND** 点击「取消」→ Activity 返回 RESULT_CANCELED → 安装中止

#### Scenario 1.2: APK 元数据提取失败
- **WHEN** APK 文件无法读取或解析失败
- **THEN** 显示默认图标和文件名（不显示包名/版本）
- **AND** 仍允许用户确认安装（降级体验，不阻塞）

### Requirement 2: GoProcessPlugin 安装流程改造

当前流程：
```
pickAndInstallPlugin() → 复制APK → installFromPath() → 直接调用 installPlugin()
```

改为：
```
pickAndInstallPlugin() → 复制APK → 启动 InstallConfirmActivity(携带apkPath)
  → 用户点击"确认安装" → onActivityResult(RESULT_OK) → installFromPath() → installPlugin()
  → 用户点击"取消"     → onActivityResult(RESULT_CANCELED) → call.reject("用户取消")
```

`installPlugin(call: PluginCall)` 方法（直接传 apkPath 的路径）同样需要经过确认。

#### Scenario 2.1: 通过 call.apkPath 触发的安装
- **WHEN** 前端调用 `installPlugin({apkPath: "/path/to/file.apk"})`
- **THEN** 先启动 InstallConfirmActivity 展示 APK 信息
- **AND** 用户确认后执行安装

---

## MODIFIED Requirements

### Requirement 3: Intent 数据传递

InstallConfirmActivity 通过 Intent Extra 接收数据：

| Extra Key | 类型 | 说明 |
|-----------|------|------|
| `EXTRA_APK_PATH` | String | APK 文件绝对路径 |
| `EXTRA_FILE_NAME` | String | 显示用的文件名 |

返回结果：
- `RESULT_OK` (+ 无需 extra data) — 用户确认
- `RESULT_CANCELED` — 用户取消

### Requirement 4: 与现有 Insecure 模式共存

当前 `ValidationStrategy.Insecure` 保持不变。InstallConfirmActivity 是**应用层**的用户确认（展示信息），不是 ComboLite 框架层的签名校验。两者互补：
- ComboLite Insecure = 不校验签名（技术层面）
- InstallConfirmActivity = 让用户看到要装什么（用户体验层面）

---

## 技术细节

### InstallConfirmActivity 关键实现点

1. **APK 图标提取**：使用 `PackageManager.getPackageArchiveInfo(apkPath, 0)` + `ResourcesCompat.getDrawable()` （与官方一致）
2. **文件大小**：`File(apkPath).length()` 格式化为 MB
3. **包名提取**：同上 `packageInfo.packageName`
4. **Compose 组件**：Scaffold + TopAppBar + Image + Text + PrimaryButton + TextButton（全部 Material3）
5. **主题**：使用项目已有的 `AppTheme.NoActionBar` 或继承 `ComponentActivity`
6. **InfoRowStyled**：简单的键值对行（Label + Value 两列布局）

### 需要与官方一致的样式细节

- 图标：56dp 圆角 RoundedCornerShape(12.dp)
- 警告横幅：tertiaryContainer 背景 + 10dp 圆角 + Info 图标 20dp + tertiary 色
- 信息行：label 用 onSurfaceVariant 色、bodySmall；value 用 onSurface 色、bodyMedium
- 主按钮：fillMaxWidth、PrimaryButton 样式
- 整体 padding：horizontal 24dp、vertical 16dp
