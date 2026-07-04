# Combolite 扩展安装修复 — 验证与收尾计划

## 问题回顾

用户报告 3 个问题：
1. **点击安装没有使用系统选择器** — 原来用 FilePickerModal（自定义组件），内部文件路由是相对路径
2. **选择文件安装失败** — Go 后端返回的相对路径在 Android 原生层无法解析
3. **手动安装后没有识别为已安装** — `installed: false` 硬编码

## 已完成的代码修改

### 1. GoProcessPlugin.kt — 新增 4 个方法/重写

| 方法 | 行号 | 功能 |
|------|------|------|
| `pickAndInstallPlugin()` | L414-428 | 调用系统 `ACTION_GET_CONTENT` 文件选择器，过滤 APK |
| `checkInstalledPlugins()` | L431-482 | 反射调用 ComboLite PluginManager，fallback 扫描插件目录 |
| `handleOnActivityResult()` | L502-537 | 处理文件选择器回调，复制 content URI 到 cacheDir |
| `installFromPath()` | L539-582 | 从复制后的绝对路径安装 APK |

**关键设计决策：**
- 系统文件选择器返回 `content://` URI → 必须复制到 `cacheDir/plugin_install/temp.apk` 变成真实文件路径
- 安装优先走 ComboLite `PluginManager.installPlugin(File)`，不可用时 fallback 到 `ACTION_INSTALL_PACKAGE`
- 检测优先走 ComboLite 反射 API，fallback 扫描 `app_plugins/`、`assets/plugins/` 目录下的 .apk 文件

### 2. GoProcess.ts — 新增 2 个导出函数

- `pickAndInstallPlugin(): Promise<PickAndInstallResult>`
- `checkInstalledPlugins(): Promise<Record<string, boolean>>`

### 3. web.ts — 新增接口定义 + Web stub

- 接口新增 `pickAndInstallPlugin()` 和 `checkInstalledPlugins()` 签名
- `GoProcessWeb` 新增对应空实现

### 4. ExtensionsPage.vue — 重写安装流程和检测逻辑

**变更前：**
```typescript
// 旧代码使用 FilePickerModal + 相对路径 + installed: false 硬编码
import { installExtensionApk, FilePickerModal, modalController } from '...'
installed: false  // 硬编码
```

**变更后：**
```typescript
// 新代码使用系统选择器 + API 检测
import { pickAndInstallPlugin, checkInstalledPlugins } from '@/plugins/GoProcess'
const installedMap = await checkInstalledPlugins()
installed: !!installedMap['mpv-player']  // 动态检测
handleInstallFromFile() → pickAndInstallPlugin() → 系统文件选择器
```

**已验证：**
- ✅ 无残留旧引用（FilePickerModal/modalController/installExtensionApk）
- ✅ FileProvider 已配置（`${applicationId}.fileprovider` + @xml/file_paths）

## 待执行步骤

### Step 1: TypeScript / Vue 构建验证
运行前端构建确认 ExtensionsPage.vue 编译无报错：
```bash
cd /workspace/app/encv-mobile && npx vue-tsc --noEmit && npx vite build
```
预期：无 TS 错误，Rollup 打包成功

### Step 2: 清理旧日志文件（如有残留）
检查并删除之前调试遗留的 job_logs.zip：
```bash
rm -f /workspace/job_logs.zip
ls /workspace/job_logs* 2>/dev/null || echo "已清理"
```

### Step 3: 通知用户完成
所有 3 个问题的修复代码已完成，等待 CI 构建验证。
