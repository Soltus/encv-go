# 文件界面功能修复 Spec

## Why

现有 `file-menu-enhance` spec 的实现存在 4 个缺陷：
1. 长按菜单中"重命名/复制/移动/分享/标签管理"5 个操作缺少图标（其他操作都有）
2. 分享功能调用 `Share.share()` 时传入的是内部 stream URL（如 `/stream?path=...`），原生端无法识别为可分享文件，导致分享失败或无效果
3. 标签管理使用普通 Alert 单行输入框，无法查看已有标签、无法删除标签；文件列表项也未展示文件的标签
4. 插件侧边抽屉按钮在 toolbar 左侧 (`slot="start"`），应在右侧；且 `IonMenu` 组件在某些场景下可能未正常工作

## What Changes

### 1. 长按菜单补全图标
- 为"重命名"、"复制"、"移动"、"分享"、"标签管理"5 个按钮添加 `icon` 属性
- 图标选择：重命名→`createOutline`、复制→`copyOutline`、移动→`moveOutline`、分享→`shareOutline`、标签→`pricetagOutline`

### 2. 分享功能修复
- 原因：`handleShare()` 传入 `getExternalStreamUrl(file.path)` 返回的是内部 HTTP URL，原生 Share 无法处理
- 修复方案：
  - **原生环境**：通过 GoProcessPlugin 新增方法获取文件的真实本地路径（nativeLibraryDir 或 filesDir 下的文件），用 `Share.share({ url: fileUrl })` 传递 `file://` URI
  - **Web 环境**：保持当前 clipboard 行为作为降级方案
- 如果文件不在本地可访问路径（远程 WebDAV 等），则提示用户"仅支持本地文件分享"

### 3. 标签管理 UI 升级
- **替换 Alert 输入框** → 使用自定义 Modal 组件（或在页面内嵌套的 tag editor panel）：
  - 上方：已关联标签列表（chip/badge 形式，每个带 × 删除按钮）
  - 下方：输入框 + 添加按钮
  - 支持一次添加多个标签
- **文件列表项增加标签显示**：在 `<ion-label>` 内文件名下方、size 信息上方，渲染该文件的标签 chips（小号 badge/chip）

### 4. 侧边抽屉按钮位置修正 + 抽屉增强
- 将插件入口按钮从 `slot="start"` 移到 toolbar 的 `slot="end"`（右侧）
- 确认 `IonMenu` 在移动端正常工作；若 `IonMenu` 与页面内容冲突（如覆盖问题），改用 `IonModal` 或自定义 slide-over 抽屉组件
- 抽屉内插件列表项图标从 `slot="start"` 保持不变（这是列表项内部布局，不是按钮位置）

## Impact

- Affected specs: 基于 `file-menu-enhance` 的缺陷修复
- Affected code:
  - `src/views/Files.vue` — 菜单图标、分享逻辑、标签 UI、抽屉按钮位置
  - `src/plugins/GoProcess.ts` / `web.ts` / `GoProcessPlugin.kt` — 可能新增获取本地文件路径的方法（如果需要）

## ADDED Requirements

### Requirement: 长按菜单操作图标完整性
所有长按菜单按钮 SHALL 包含对应的 icon 属性。
#### Scenario: 用户打开长按菜单
- **WHEN** 用户长按任意文件弹出 ActionSheet
- **THEN** 每个操作项均显示对应图标（信息/播放/预览/加密/解密/删除/重命名/复制/移动/分享/标签/取消）

### Requirement: 文件分享可用性
原生环境下的分享功能 SHALL 能正确触发系统分享面板并传递有效文件引用。
#### Scenario: 用户分享本地文件
- **WHEN** 用户在原生环境选择"分享"
- **THEN** 系统分享面板打开且能正确传递文件给目标应用
#### Scenario: 用户分享非本地文件
- **WHEN** 待分享文件不在本地文件系统（如远程挂载路径）
- **THEN** 显示提示"仅支持本地文件分享"，不崩溃

### Requirement: 标签管理多标签交互
标签管理界面 SHALL 支持查看已有标签、添加新标签、删除已有标签。
#### Scenario: 用户管理文件标签
- **WHEN** 用户选择"标签管理"
- **THEN** 弹出标签编辑界面，展示该文件已有标签（可点击 × 删除），底部有输入框和添加按钮
#### Scenario: 文件列表展示标签
- **WHEN** 文件列表加载完成且文件有关联标签
- **THEN** 文件项内以小号 chip 形式展示该文件的标签名

### Requirement: 抽屉入口位置正确性
插件分类抽屉的入口按钮 SHALL 位于工具栏右侧。
#### Scenario: 用户寻找插件分类入口
- **WHEN** 用户查看文件页面顶部工具栏
- **THEN** 插件分类按钮位于标题右侧（slot="end"），而非左侧

## MODIFIED Requirements

### Requirement: handleLongPress ActionSheet 按钮
现有的 `handleLongPress` 函数中的 5 个新增操作按钮 SHALL 补充 `icon` 属性。

### Requirement: handleShare 分享函数
现有的 `handleShare` 函数 SHALL 修改为使用有效的本地文件路径进行分享。

### Requirement: showTagDialog 标签对话框
现有的 `showTagDialog`（IonAlert 单输入框） SHALL 替换为支持多标签增删的自定义标签编辑器。

### Requirement: 文件列表项渲染
现有的 `<ion-item>` 文件列表项 SHALL 在 `<ion-label>` 区域内增加标签 chip 渲染区域。

## REMOVED REQUIREMENTS

无
