# 多问题修复计划（v3 - 最终版）

## 问题清单

| # | 问题 | 严重程度 | 根因 |
|---|------|----------|------|
| 1 | video.go Android 编译失败 | 🔴 CI 失败 | 引用 Android 特有类型/函数 |
| 2 | 加密入口分散维护 | 🟡 架构问题 | Files.vue 独立维护加密/解密 modal，应委托给统一入口 |
| 3 | v4 容器图片信息乱码 | 🔴 数据错误 | ContainerID 为空；KVI 原始 JSON 直接暴露前端 |
| 4 | config.user.json 内容丢失 | 🔴 数据丢失 | PUT /api/config 全量替换 |

---

## 问题 1: video.go Android 编译失败

### 修复
[video.go](file:///workspace/internal/utils/video.go) 添加 `//go:build !android`

Android 上视频处理走 native ffmpeg 直接调用路径（不经过 Go 层 video.go）。

---

## 问题 2: 加密入口统一委托（核心架构调整）

### 用户意图
- **Tasks.vue 是唯一的新建任务入口**，负责维护完整的创建 UI
- **Files.vue 长按加密/解密** 不再独立维护 modal → 直接**委托**给 Tasks 的创建流程
- 使用全局密码（正确），不需要每任务输入密码
- 二级密码：先写 UI 但显示"计划中/不可用"

### 当前状态 vs 目标状态

**当前（两套独立逻辑）**:
```
Files.vue 长按 → handleEncryptFile() → showEncryptModal (ion-modal, 含 ContainerVersionSelector)
             → handleDecryptFile() → alertController (旧风格)

Tasks.vue FAB → showNewTaskModal (ion-modal, 无版本选择, 无密码)
```

**目标（单一入口）**:
```
Files.vue 长按 → handleEncryptFile() → 打开 Tasks 的新建任务 modal（预填 type=encrypt, sourcePath）
             → handleDecryptFile() → 打开 Tasks 的新建任务 modal（预填 type=decrypt, sourcePath）

Tasks.vue FAB → showNewTaskModal（唯一入口，功能完整）
```

### 具体实现方案

#### Step 1: 增强 Tasks.vue 的 `showNewTaskModal`

修改 `/workspace/app/encv-mobile/src/views/Tasks.vue`：

1. 新增状态变量:
   - `newTaskPassword` (ref<string>) — 从全局 config.password 预填充（只读显示）
   - `newTaskVersion` (ref<number>) — 默认 4
   - `newTaskSecondaryPassword` (ref<string>) — 二级密码（UI 存在但 disabled + 提示"计划中"）

2. Modal 模板增强:
   ```
   ┌─ 新建任务 ─────────────────┐
   │ 任务类型: [加密 ▼]          │
   │                             │
   │ 源路径:   [________] [📁]    │
   │ 目标路径: [________] [📁]    │
   │                             │
   │ 密码:     [从全局配置读取]🔒 │  ← 只读显示，说明"使用全局密码"
   │                             │
   │ 容器版本:                     │  ← 仅 encrypt 时显示
   │   ○ V2 (已弃用)            │
   │   ○ V3                      │
   │   ● V4 (推荐)               │
   │                             │
   │ 二级密码: [____________]     │  ← disabled + "计划中" badge
   │                             │
   │      [ 创建任务 ]           │
   └─────────────────────────────┘
   ```

3. `handleCreateTask()` 更新:
   ```typescript
   await createTask(
       newTaskType.value,
       newTaskPath.value,
       newTaskTargetPath.value || undefined,
       newTaskType.value === 'encrypt' ? newTaskVersion.value : undefined
       // password 不传 → 后端使用全局配置密码
       // secondaryPassword 不传 → 功能未启用
   )
   ```

#### Step 2: Files.vue 委托给 Tasks

修改 `/workspace/app/encv-mobile/src/views/Files.vue`：

1. **删除** `showEncryptModal`, `handleEncryptSubmit`, `encryptSourcePath`, `encryptTargetPath`, `selectedVersion`, `encryptFileName` 等变量和模板
2. **删除** `showDecryptModal`, `decryptSourcePath`, `decryptTargetPath`, `decryptPassword` 等变量和模板
3. **修改** `handleEncryptFile(file)`:
   ```typescript
   // 方案 A: 导航到 Tasks 页面并触发新建任务（推荐）
   router.push({ name: 'Tasks', query: { action: 'new', type: 'encrypt', source: file.path } })

   // 或方案 B: 通过事件总线 / store 触发 Tasks 页面的 modal（需要共享状态）
   ```

4. **修改** `handleDecryptFile(file)`:
   ```typescript
   router.push({ name: 'Tasks', query: { action: 'new', type: 'decrypt', source: file.path } })
   ```

5. **Tasks.vue 接收预填参数**:
   - 在 `onMounted` 或 watcher 中检查 route.query
   - 如果有 `action=new` → 自动打开 modal 并预填 type 和 sourcePath

#### Step 3: 二级密码 UI（占位）

- 在 Tasks modal 中添加二级密码 input
- 设置 `disabled` 属性
- 显示 badge "计划中" (coming soon)
- i18n 键: `tasks.secondaryPassword`, `tasks.comingSoon`

---

## 问题 3: v4 容器图片信息乱码

### 根因
- ContainerID 为空：所有插件传 `SpecialID:nil` → `writeManifestV4` 中 `ContainerID=""`
- KVI "乱码"：后端 GetFileInfo 将整个 Manifest_v4（含 base64 KVI）返回前端，JSON.stringify 后显示为长串

### 修复

**Step 1: 后端 GetFileInfo 清理** ([mobile_service.go](file:///workspace/internal/service/mobile_service.go))
- 返回前删除 KVI 字段（敏感+冗长）
- ContainerID 为空时标注 `(auto)`

**Step 2: 前端 FileInfo 容错**
- 空值友好显示

---

## 问题 4: config.user.json 内容丢失

### 根因
PUT `/api/config` 全量替换，如果前端 saveConfig 发送不完整则丢失字段。

### 修复方向
- 排查 useConfig composable saveConfig 是否发送完整配置
- PUT API 改为 merge 策略（防御性）

---

## 实施顺序

### Phase 1: 编译修复 ⚡
1. [ ] Fix 1 - video.go: `//go:build !android`

### Phase 2: 架构统一（核心工作量）
2. [ ] Fix 2a - 增强 Tasks.vue modal: 密码只读显示 + 版本选择 + 二级密码占位
3. [ ] Fix 2b - Tasks.vue 支持路由预填参数 (query: action, type, source)
4. [ ] Fix 2c - Files.vue 删除独立 modal，改为 router.push 委托
5. [ ] Fix 2d - i18n 更新（新键: secondaryPassword, comingSoon, 使用全局密码等）

### Phase 3: 数据修复
6. [ ] Fix 3a - 后端 GetFileInfo 清理 KVI + ContainerID 容错
7. [ ] Fix 3b - 前端 FileInfo 容错
8. [ ] Fix 4 - 排查 config 丢失 + 防御性修复

### Phase 4: 验证
9. [ ] Android 交叉编译验证
10. [ ] 桌面端编译 + 测试
11. [ ] 前端构建验证
