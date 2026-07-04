# 实施计划：Artplayer 竖屏修复 + 加密容器预览/信息修复

## 问题 1：Artplayer 竖屏视频 384x0 直接看不到

### 根因

**上一次改动的错误**：移除了 `minHeight: '200px'` 后，容器在没有初始高度的情况下，`loadedmetadata` 中设置的 height 可能因时机问题未生效（`384x0` 说明高度为 0）。更根本的错误是：**不应该在 JS 中手动修改 artContainer 的 width/height**，这与 Artplayer 的 `autoSize: true` 内部管理逻辑冲突，导致尺寸计算紊乱。

### 正确方案

**原则：CSS 控制布局，JS 不干预容器尺寸，让 Artplayer autoSize 自行管理。**

#### 步骤 1：恢复 artContainer 初始尺寸保证可见性
```typescript
// initArtPlayer 中恢复：
const containerWidth = artContainer.value.clientWidth || window.innerWidth
artContainer.value.style.minHeight = '200px'
artContainer.value.style.maxHeight = `${window.innerHeight - 56}px`
```

#### 步骤 2：删除 loadedmetadata 中的所有手动尺寸设置代码
```typescript
// 删除以下整个 if (video.videoWidth && video.videoHeight) { ... } 块：
//   - isPortrait / w / h 计算
//   - artContainer.value.style.width / height / margin 设置
// 只保留 resolution 和 duration 的赋值
```

#### 步骤 3：CSS 层面优化竖屏视频显示
`.video-player` 保持 `width: 100%`，不设固定高度。关键是通过 CSS 控制 video 元素在容器内的填充方式：

```css
.video-player {
  width: 100%;
  min-height: 200px;
  background: #000;
  position: relative;
  overflow: hidden;
}

/* 确保 Artplayer 内部 video 元素正确填充 */
:deep(.art-video-player .art-video) {
  width: 100% !important;
  height: 100% !important;
  object-fit: contain;
}
```

**关于竖屏黑边的说明**：竖屏视频（如 9:16）在横屏设备（如 16:9）上，`object-fit: contain` 必然会在左右产生黑边——这是正常的几何约束。消除黑边的唯一方式是让容器变窄（按视频比例），但这会导致左右两侧大面积空白，体验并不更好。当前方案保证视频完整可见且不失真，是合理的默认行为。如果未来确实需要"竖屏满屏"效果，应使用全屏旋转（`autoOrientation` 已开启）而非修改容器尺寸。

#### 涉及文件
| 文件 | 改动 |
|------|------|
| `app/encv-mobile/src/views/ArtPlayerView.vue` | 恢复 minHeight/maxHeight + 删除 loadedmetadata 手动尺寸 + CSS 微调 |

---

## 问题 2a：加密容器全部当成视频播放（老 bug）

### 根因

[Files.vue:394](file:///workspace/app/encv-mobile/src/views/Files.vue#L394)：
```typescript
if (category === 'video' || category === 'audio' || category === 'encrypted') {
    playMedia(file, category)  // ← encrypted 一律走播放器
}
```

[Files.vue:180](file:///workspace/app/encv-mobile/src/views/Files.vue#L180)：
```typescript
const isVideo = category === 'video' || category === 'encrypted'  // ← encrypted 当视频处理
```

加密容器（无论实际内容是图片/文本/PDF/视频）都被当作视频丢给 Artplayer/MPV 播放。

### 修复

**加密容器统一走 FilePreview（container 类型显示容器信息），不走播放器。**

#### Files.vue handleFileClick（第 394 行）
```typescript
// 修改前：
if (category === 'video' || category === 'audio' || category === 'encrypted') {
    playMedia(file, category)
}

// 修改后：
if (category === 'video' || category === 'audio') {
    playMedia(file, category)
} else if (category === 'encrypted') {
    // 加密容器走预览（显示容器信息和清单）
    router.push({
      path: '/tabs/preview',
      query: { path: file.path, name: file.name, isEncrypted: 'true' },
    })
}
```

#### Files.vue 长按菜单（第 488 行 encrypted 分支）
当前长按菜单中 encrypted 分类只有"播放"和"解密"。应改为：
- 如果是视频类型加密容器 → 保留"播放"
- 所有加密容器 → 增加"预览"选项（走 FilePreview 显示容器信息）

但由于前端不知道容器具体类型（需要调用 API），**简化方案**：加密容器长按菜单统一提供"信息"按钮（已实现的 FileInfo 页）+ "解密"，移除笼统的"播放"。用户想播放视频容器可以从 FileInfo 页操作。

或者更简：**加密容器点击行为统一改为走 FilePreview（container 类型）**，和上面 handleFileClick 的改动一致。长按菜单保持现状（播放/解密），播放功能保留给确实想尝试的用户。

#### 涉及文件
| 文件 | 改动 |
|------|------|
| `app/encv-mobile/src/views/Files.vue` | handleFileClick 中 encrypted 分支走 preview |

---

## 问题 2b：sccgi 图片容器信息显示异常

### 可能原因

1. **`OpenV4Container` 对图片容器读取成功，但字段值不符合前端预期** — 比如 `original_duration` 为 0（图片无时长）、`segment_count` 结构不同等
2. **`container_type` 映射错误** — 图片插件 ContainerType=3 应映射为 `"image"`，需确认
3. **manifest 序列化问题** — 图片容器的 manifest 可能含大字段（如缩略图数据），序列化到 JSON 后数据异常

### 排查方向

1. 检查 `GetFileInfo` 返回的原始 JSON（特别是 image 类型容器的 container 字段）
2. 检查 `FileInfo.vue` 模板中对 `containerInfo` 各字段的访问是否有空值保护
3. 确认 `containerInfo.version` 是否存在（模板中 `V{{ containerInfo.version }}` 如果 version 为 undefined 会显示 "Vundefined"）

### 修复

在 `FileInfo.vue` 和 `FilePreview.vue` 的容器信息模板中增加防御性判断：

```html
<!-- 修改前 -->
<span class="info-value">V{{ containerInfo.version }}</span>

<!-- 修改后 -->
<span class="info-value">V{{ containerInfo.version ?? '?' }}</span>
```

对所有 containerInfo 字段（`container_id`, `container_type`, `is_seekable`, `segment_count` 等）增加类似的空值保护。

#### 涉及文件
| 文件 | 改动 |
|------|------|
| `app/encv-mobile/src/views/FileInfo.vue` | 容器信息字段空值保护 |
| `app/encv-mobile/src/views/FilePreview.vue` | 容器信息字段空值保护 |

---

## 执行顺序

1. **ArtPlayerView.vue** — 恢复 minHeight + 删除手动尺寸 + CSS 修复
2. **Files.vue** — encrypted 点击走 preview
3. **FileInfo.vue + FilePreview.vue** — 容器信息空值保护
4. **构建验证** — go vet + vue-tsc + vite build
