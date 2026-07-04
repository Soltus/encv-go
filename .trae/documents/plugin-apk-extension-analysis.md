# 插件文件扩展名合理性分析

## 结论：.apk 扩展名合理，但需在 UI 层面做好区分

### ComboLite 对文件扩展名的要求

1. **`InstallerManager.installPlugin(File)` 不检查扩展名**：内部通过 `PackageManager.getPackageArchiveInfo()` 和 `ZipFile` 解析文件内容，只关心文件是否是有效的 APK 格式（ZIP + AndroidManifest.xml + classes.dex），不关心文件扩展名

2. **安装后源文件被重命名为 `base.apk`**：`copyPluginApk()` 将源文件复制到 `plugins/{pluginId}/base.apk`，原始文件名和扩展名完全无关

3. **aar2apk 构建工具硬编码输出 `.apk`**：`ConvertAarToApkTask.kt` L199 输出 `${pluginName}-${buildType}.apk`

4. **ComboLite 官方示例全部使用 `.apk`**：`updates/plugins/` 目录下的所有插件文件都是 `.apk` 扩展名

### 为什么 .apk 是正确的扩展名

**插件文件本质上就是 APK**：
- 它是标准的 Android APK 格式（ZIP 容器 + AndroidManifest.xml + DEX + resources.arsc + so 库）
- 它有合法的 Android 签名
- `PackageManager.getPackageArchiveInfo()` 能正常解析它
- 改成其他扩展名（如 `.plugin`、`.cpk`）不会改变文件内容，只是伪装

**改扩展名的坏处**：
- 破坏与 ComboLite 官方工具链的兼容性（aar2apk 输出 `.apk`）
- 文件选择器需要额外处理自定义 MIME 类型
- 用户无法通过扩展名理解文件本质
- 增加维护成本（每次 aar2apk 升级可能需要适配）

### 真正的问题和解决方案

**问题不是扩展名，而是用户可能误用系统安装器打开插件 APK**。之前代码中 `installPlugin()` 的 `ACTION_INSTALL_PACKAGE` 兜底逻辑就是这种混淆的产物。

**正确的解决方案**：保持 `.apk` 扩展名，但在 UI 和交互层面明确区分：

1. ✅ **已修复**：移除了 `installPlugin()` 的系统安装兜底（上一步已完成）
2. ✅ **已修复**：移除了前端的 `systemInstallerHint` 提示（上一步已完成）
3. **可选增强**：在 `InstallConfirmActivity` 的 UI 中更明确地说明"这是应用内插件，不是独立应用"
4. **可选增强**：在文件选择器中过滤 `.apk` 文件时，提示用户选择 ComboLite 插件 APK

### 不需要修改的内容

- 文件扩展名保持 `.apk`
- aar2apk 构建输出保持 `.apk`
- 文件选择器 MIME 类型保持 `application/vnd.android.package-archive`
- CI 构建流程不变
