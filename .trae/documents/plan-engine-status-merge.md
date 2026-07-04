# 引擎状态并入联机设置组 + 二级页面

## 目标

1. **修复上一轮丢失的修改**（plan-artplayer-fix-container-preview.md）
2. 将 Settings.vue 中独立的"引擎状态"分组移除，其入口并入"联机"设置组
3. 新建 `EngineDetail.vue` 二级页面，提供引擎运行时状态和构建配置细节
4. AboutDetail.vue 保留并**扩充**第三方库展示，移除构建配置细节（迁移到 EngineDetail）

## 步骤 0：修复上一轮丢失的修改

经检查，以下 4 处修改已丢失，需重新应用：

### 0a. ArtPlayerView.vue — 竖屏修复

**当前状态（错误）**：
- `initArtPlayer()` 中无 `minHeight`/`maxHeight` 设置
- `loadedmetadata` 中竖屏视频缩窄 width + margin（用户否决的方案）

**需修复为**：
1. `initArtPlayer()` 中恢复：
   ```typescript
   artContainer.value.style.minHeight = '200px'
   artContainer.value.style.maxHeight = `${window.innerHeight - 56}px`
   ```
2. `loadedmetadata` 回调改为只设 height，不动 width：
   ```typescript
   art.on('video:loadedmetadata', () => {
     const video = art?.video
     if (video) {
       if (video.videoWidth && video.videoHeight) {
         mediaInfo.value.resolution = `${video.videoWidth}×${video.videoHeight}`
         const ratio = video.videoHeight / video.videoWidth
         const containerWidth = artContainer.value?.clientWidth || window.innerWidth
         const naturalHeight = Math.round(containerWidth * ratio)
         const maxHeight = window.innerHeight - 56
         const finalHeight = Math.min(naturalHeight, maxHeight)
         if (artContainer.value) {
           artContainer.value.style.height = `${finalHeight}px`
         }
       }
       if (video.duration && isFinite(video.duration)) {
         mediaInfo.value.duration = formatDuration(video.duration)
       }
     }
     hideNativeControls()
   })
   ```

### 0b. Files.vue — encrypted 分类点击走 preview

**当前状态（错误）**：L394 `category === 'encrypted'` 仍走 `playMedia()`

**需修复**：
```typescript
// L394: 移除 || category === 'encrypted'
if (category === 'video' || category === 'audio') {
    playMedia(file, category)
} else {
    router.push({
      path: '/tabs/preview',
      query: { path: file.path, name: file.name, isEncrypted: String(!!file.isEncrypted) },
    })
}
```

同时 L180 `playMedia` 中 `category === 'encrypted'` 逻辑保留（长按菜单"播放"按钮仍需走此路径）

### 0c. FilePreview.vue — 容器信息空值保护

**当前状态（错误）**：L61/65/69/83 无 `??` 保护

**需修复**：
```
L61: containerInfo.version → containerInfo.version ?? '?'
L65: containerInfo.container_id → containerInfo.container_id ?? '-'
L69: containerInfo.container_type → containerInfo.container_type ?? '-'
L83: containerInfo.segments?.length || 0 → containerInfo.segment_count ?? 0
```

### 0d. FileInfo.vue — 容器信息空值保护

**当前状态（错误）**：L73/77/81 无 `??` 保护

**需修复**：
```
L73: containerData.version → containerData.version ?? '?'
L77: containerData.container_id → containerData.container_id ?? '-'
L81: containerData.container_type → containerData.container_type ?? '-'
```

---

## 信息分层

| 信息 | 关于页面 | 引擎详情 |
|------|---------|---------|
| FFmpeg 版本 + license | ✅ | ✅ |
| x264 版本 + license | ✅ | ✅ |
| Go Runtime 版本 + license | ✅ | ❌ |
| Gin / Cobra / Capacitor / Ionic / Vue / Artplayer 等其他库 | ✅ | ❌ |
| FFmpeg NDK/API Level/ABI/链接方式/构建日期/CFLAGS | ❌ | ✅ |
| FFmpeg 编解码器/封装器/解封装器/解析器/协议/滤镜/静态库列表 | ❌ | ✅ |
| x264 配置选项 | ❌ | ✅ |
| FFmpeg/FFprobe 运行时可用性 + 错误详情 | ❌ | ✅ |

## 步骤 1：Settings.vue — 合并引擎状态到联机组

1. 在"联机"组的服务器入口下方，新增一个"引擎状态"入口项：
   - 图标：`filmOutline`（复用现有 import）
   - 标题：`t('settings.engineStatus')`
   - 副标题：简要状态摘要
     - Native 平台：显示 FFmpeg/FFprobe 各自的 ✅/❌ badge
     - 非 Native 平台：不显示此项（`v-if="isNative()"`）
   - 点击跳转 `/tabs/settings/engine`
   - 带 `detail` 箭头
2. 删除原来独立的"引擎状态"分组（L102-129 的整个 `<ion-list>`）
3. 删除不再需要的 CSS 类：`.engine-error-inline`、`.engine-detail-text`
4. `engineStatus` ref 和 `fetchFFmpegStatus` 调用保留（仍需在 Settings 概览中显示摘要 badge）

## 步骤 2：新建 EngineDetail.vue

路径：`/workspace/app/encv-mobile/src/views/EngineDetail.vue`

页面结构：

```
<ion-page>
  <ion-header>
    <ion-back-button default-href="/tabs/settings">
    <ion-title>引擎详情</ion-title>
  </ion-header>

  <ion-content>
    <!-- 运行时状态 -->
    <ion-list header="运行时状态">
      <ion-item> FFmpeg 可用性 badge + 错误详情 </ion-item>
      <ion-item> FFprobe 可用性 badge + 错误详情 </ion-item>
      <ion-item> 刷新按钮 </ion-item>
    </ion-list>

    <!-- 构建信息 -->
    <ion-list header="构建信息" v-if="buildInfo">
      <ion-item> FFmpeg 版本 + codename + license badge </ion-item>
      <ion-item> x264 版本 + license badge </ion-item>
      <ion-item> NDK 版本 </ion-item>
      <ion-item> API Level </ion-item>
      <ion-item> ABI </ion-item>
      <ion-item> 链接方式 </ion-item>
      <ion-item> 构建日期 </ion-item>
      <ion-item> CFLAGS（折叠展示） </ion-item>
    </ion-list>

    <!-- 组件列表（手风琴） -->
    <ion-list header="组件" v-if="buildInfo">
      <ion-accordion-group>
        <ion-accordion> 解码器列表 </ion-accordion>
        <ion-accordion> 编码器列表 </ion-accordion>
        <ion-accordion> 封装器列表 </ion-accordion>
        <ion-accordion> 解封装器列表 </ion-accordion>
        <ion-accordion> 解析器列表 </ion-accordion>
        <ion-accordion> 协议列表 </ion-accordion>
        <ion-accordion> 滤镜列表 </ion-accordion>
        <ion-accordion> 静态库列表 </ion-accordion>
      </ion-accordion-group>
    </ion-list>
  </ion-content>
</ion-page>
```

数据来源：
- 运行时状态：`fetchFFmpegStatus()` → `/api/ffmpeg-status`
- 构建信息：`fetchBuildInfo()` → `/api/build-info`
- 页面 `onMounted` 时并行请求两个 API
- 刷新按钮重新调用 `fetchFFmpegStatus()`

样式：从 AboutDetail.vue 迁移手风琴 + tag-list 相关 CSS

## 步骤 3：AboutDetail.vue — 扩充第三方库展示

将第三方库区块从手风琴改为简洁列表，并扩充更多库：

```
<ion-list header="第三方库">
  <!-- Native 引擎库（从 buildInfo 获取版本） -->
  <ion-item> FFmpeg 图标 + "FFmpeg" + 版本 badge + "LGPL-2.1" badge </ion-item>
  <ion-item> x264 图标 + "x264" + 版本 badge + "GPL-2.0" badge </ion-item>

  <!-- Go 后端库（硬编码版本） -->
  <ion-item> Go 图标 + "Go" + 版本 badge + "BSD" badge </ion-item>
  <ion-item> Gin 图标 + "Gin" + 版本 badge + "MIT" badge </ion-item>
  <ion-item> Cobra 图标 + "Cobra" + 版本 badge + "Apache-2.0" badge </ion-item>

  <!-- 前端库（硬编码版本） -->
  <ion-item> Vue 图标 + "Vue" + 版本 badge + "MIT" badge </ion-item>
  <ion-item> Ionic 图标 + "Ionic" + 版本 badge + "MIT" badge </ion-item>
  <ion-item> Capacitor 图标 + "Capacitor" + 版本 badge + "MIT" badge </ion-item>
  <ion-item> Artplayer 图标 + "Artplayer" + 版本 badge + "MIT" badge </ion-item>
</ion-list>
```

第三方库清单（按层级分组）：

**Native 引擎库**（版本从 `buildInfo` 动态获取）：
| 库名 | 版本来源 | 许可证 |
|------|---------|--------|
| FFmpeg | `buildInfo.ffmpeg_version` | LGPL-2.1 |
| x264 | `buildInfo.x264_version` | GPL-2.0 |

**Go 后端库**（版本硬编码，随 go.mod 更新）：
| 库名 | 版本 | 许可证 |
|------|------|--------|
| Go | `goVersion`（从 buildInfo 或 runtime 获取） | BSD-3-Clause |
| Gin | v1.12.0 | MIT |
| Cobra | v1.10.2 | Apache-2.0 |
| go-mp4 | v1.4.1 | MIT |
| go-exif | v3.0.1 | MIT |
| fsnotify | v1.9.0 | BSD-3-Clause |
| gorilla/websocket | v1.5.3 | BSD-2-Clause |
| go-humanize | v1.0.1 | MIT |

**前端库**（版本硬编码，随 package.json 更新）：
| 库名 | 版本 | 许可证 |
|------|------|--------|
| Vue | ^3.5.34 | MIT |
| Ionic | ^8.8.7 | MIT |
| Capacitor | ^8.3.4 | MIT |
| Artplayer | ^5.4.0 | MIT |
| vue-router | ^4.6.4 | MIT |

删除内容：
1. FFmpeg 手风琴展开内容（构建配置 + 组件列表）→ 迁移到 EngineDetail
2. x264 手风琴展开内容（配置选项）→ 迁移到 EngineDetail
3. Go 手风琴展开内容（版本详情）→ 改为列表项
4. `ion-accordion-group` / `ion-accordion` → 改为普通 `ion-item` 列表
5. 所有手风琴/tag-list 相关 CSS
6. `formatDate` 函数（不再需要）

保留内容：
1. `buildInfo` ref + `fetchBuildInfo` 调用（仍需 FFmpeg/x264 版本号）
2. `buildInfoLoading` / `buildInfoError` 状态
3. 应用版本、GitHub、危险区

## 步骤 4：路由注册

在 `/workspace/app/encv-mobile/src/router/index.ts` 的 tabs children 中添加：

```typescript
{
  path: 'settings/engine',
  component: () => import('@/views/EngineDetail.vue'),
},
```

## 步骤 5：i18n 新增 key

**引擎详情页 key：**

| key | 中文 | English |
|-----|------|---------|
| `settings.engineDetail` | 引擎详情 | Engine Detail |
| `engine.runtimeStatus` | 运行时状态 | Runtime Status |
| `engine.buildInfo` | 构建信息 | Build Info |
| `engine.components` | 组件 | Components |
| `engine.available` | 可用 | Available |
| `engine.unavailable` | 不可用 | Unavailable |
| `engine.ffmpegVersion` | FFmpeg 版本 | FFmpeg Version |
| `engine.x264Version` | x264 版本 | x264 Version |
| `engine.ndkVersion` | NDK 版本 | NDK Version |
| `engine.apiLevel` | API Level | API Level |
| `engine.abi` | 架构 | Architecture |
| `engine.linking` | 链接方式 | Linking |
| `engine.buildDate` | 构建日期 | Build Date |
| `engine.cflags` | 编译标志 | CFLAGS |
| `engine.staticLinking` | 静态链接 | Static Linking |
| `engine.decoders` | 解码器 | Decoders |
| `engine.encoders` | 编码器 | Encoders |
| `engine.muxers` | 封装器 | Muxers |
| `engine.demuxers` | 解封装器 | Demuxers |
| `engine.parsers` | 解析器 | Parsers |
| `engine.protocols` | 协议 | Protocols |
| `engine.filters` | 滤镜 | Filters |
| `engine.staticLibs` | 静态库 | Static Libraries |
| `engine.refresh` | 刷新 | Refresh |
| `engine.loadFailed` | 加载失败 | Failed to load |
| `engine.configureOpts` | 配置选项 | Configure Options |

**关于页面新增第三方库 key：**

| key | 中文 | English |
|-----|------|---------|
| `about.nativeEngine` | 原生引擎 | Native Engine |
| `about.backendLibs` | 后端库 | Backend Libraries |
| `about.frontendLibs` | 前端库 | Frontend Libraries |
| `about.ginDesc` | HTTP 框架 | HTTP Framework |
| `about.cobraDesc` | CLI 框架 | CLI Framework |
| `about.goMp4Desc` | MP4 解析器 | MP4 Parser |
| `about.goExifDesc` | EXIF 解析器 | EXIF Parser |
| `about.fsnotifyDesc` | 文件系统监控 | File System Watcher |
| `about.websocketDesc` | WebSocket 库 | WebSocket Library |
| `about.humanizeDesc` | 人类可读格式化 | Human-readable Formatting |
| `about.vueDesc` | 前端框架 | Frontend Framework |
| `about.ionicDesc` | UI 组件库 | UI Component Library |
| `about.capacitorDesc` | 跨平台运行时 | Cross-platform Runtime |
| `about.artplayerDesc` | 视频播放器 | Video Player |
| `about.vueRouterDesc` | 路由库 | Router Library |

## 步骤 6：构建验证

```bash
cd /workspace && go vet ./internal/...
cd /workspace/app/encv-mobile && npx vue-tsc --noEmit && npx vite build
```

## 文件变更清单

| 文件 | 操作 |
|------|------|
| `app/encv-mobile/src/views/ArtPlayerView.vue` | 修复：恢复 minHeight/maxHeight + loadedmetadata 只设 height |
| `app/encv-mobile/src/views/Files.vue` | 修复：encrypted 点击走 preview |
| `app/encv-mobile/src/views/FilePreview.vue` | 修复：容器信息空值保护 |
| `app/encv-mobile/src/views/FileInfo.vue` | 修复：容器信息空值保护 |
| `app/encv-mobile/src/views/Settings.vue` | 编辑：引擎状态并入联机组，删除独立分组 |
| `app/encv-mobile/src/views/EngineDetail.vue` | 新建：引擎详情二级页面 |
| `app/encv-mobile/src/views/AboutDetail.vue` | 编辑：扩充第三方库展示，移除构建配置细节 |
| `app/encv-mobile/src/router/index.ts` | 编辑：添加 settings/engine 路由 |
| `app/encv-mobile/src/composables/useI18n.ts` | 编辑：添加 i18n key |

## 设计考量

1. **先修复丢失修改**：步骤 0 优先于新功能，确保基础功能正确
2. **关于页面扩充第三方库**：分为三组（原生引擎/后端库/前端库），满足开源合规需求
3. **构建配置细节迁移到引擎详情**：NDK/ABI/CFLAGS/编解码器列表等属于引擎详情
4. **运行时状态只在引擎详情**：FFmpeg/FFprobe 可用性是动态信息，放在引擎详情语义正确
5. **Go/Capacitor/Ionic 等非引擎库**只在关于页面展示，不在引擎详情页
