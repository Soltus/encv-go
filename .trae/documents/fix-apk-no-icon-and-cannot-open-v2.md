# 修复 APK 无 Icon 和无法打开问题（深入分析）

## 问题现状

上一轮只添加了 MAIN/LAUNCHER intent-filter，但问题未解决。需要更全面地分析。

## 根因分析

### 根因 1：overlay AndroidManifest.xml 完全替换了 Capacitor 生成的 manifest

CI 流程中 `post-cap-sync.mjs` 执行 `copyFileSync(overlayManifestSrc, appManifestDest)`，**整体替换**了 Capacitor 默认生成的 AndroidManifest.xml。

Capacitor 默认生成的 AndroidManifest.xml 包含：
```xml
<application
    android:allowBackup="true"
    android:icon="@mipmap/ic_launcher"          ← 应用图标
    android:label="@string/app_name"            ← 应用名称
    android:roundIcon="@mipmap/ic_launcher_round" ← 圆形图标
    android:supportsRtl="true"
    android:theme="@style/AppTheme">
```

而我们的 overlay manifest 只有：
```xml
<application>   ← 空标签！缺少 icon、label、roundIcon、theme 等关键属性
```

**缺少 `android:icon`** → 安装后没有 icon
**缺少 `android:label`** → 应用名显示为包名
**缺少 `android:theme`** → Activity 可能无法正确渲染

### 根因 2：Python ElementTree 补丁可能破坏 manifest 结构

CI step 14 用 Python `xml.etree.ElementTree` 解析并重写 manifest。`ElementTree` 的 `write()` 方法在处理带命名空间的 XML 时有已知问题：
- 可能改变 `android:` 前缀为 `ns0:` 等非标准前缀
- 可能丢失自闭合标签格式
- 可能改变属性顺序

虽然代码中调用了 `ET.register_namespace('android', ...)` 来保留前缀，但 `ElementTree` 对子元素的命名空间处理仍然不可靠。

### 根因 3：缺少 app icon 资源文件

项目中没有任何 `resources/` 目录或 icon 源文件。Capacitor 默认生成的 `mipmap-*` 目录包含的是 Capacitor 默认图标。如果项目没有自定义图标，至少需要保留 Capacitor 默认的。

但 `post-cap-sync.mjs` 不删除 mipmap 资源，所以 Capacitor 默认图标应该保留。**只要 AndroidManifest.xml 中正确引用 `@mipmap/ic_launcher`，图标就能显示。**

## 修复计划

### Step 1：修复 overlay AndroidManifest.xml — 补全 `<application>` 关键属性

在 `<application>` 标签上添加 Capacitor 标准属性：

```xml
<application
    android:allowBackup="true"
    android:icon="@mipmap/ic_launcher"
    android:label="@string/app_name"
    android:roundIcon="@mipmap/ic_launcher_round"
    android:supportsRtl="true"
    android:theme="@style/AppTheme">
```

这样 overlay 替换后，manifest 仍保留所有关键属性。

### Step 2：改进 Python manifest 补丁 — 改用 sed 替代 ElementTree

将 CI workflow 中的 Python `xml.etree.ElementTree` 补丁替换为 `sed` 命令。原因：
- `ElementTree` 解析再写回可能破坏 XML 结构（命名空间、自闭合标签等）
- `sed` 做简单的属性注入更安全，不会改变文件的其他部分

具体改动：
1. 用 `sed` 在 `<application` 标签上注入 `android:networkSecurityConfig` 和 `android:usesCleartextTraffic` 属性
2. 用 `sed` 在 `</manifest>` 前追加缺失的 `<uses-permission>` 标签
3. 删除 Python 代码

### Step 3：在 CI 中添加 manifest 验证步骤

在 Python/sed 补丁之后、构建之前，添加验证：
- 确认 AndroidManifest.xml 中包含 `MAIN` 和 `LAUNCHER`
- 确认包含 `android:icon`
- 确认包含 `android:label`
- 确认 mipmap 资源目录存在

### Step 4：清理临时文件
- 删除 `/tmp/job_logs2` 等临时解压目录
