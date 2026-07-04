# 第三轮修复：UI 美化 + 虚拟滚动白屏 + 时间线细节 + 产物定位 + 报告合并

> 创建：2026-06-18
> 状态：待用户批准
> 前置：fix-ui-and-tasks-perf-v2（第二轮修复已完成，但存在遗留问题）

---

## 一、问题根因汇总（7 个修复点）

### 1. 虚拟滚动白屏

**根因**：`content-visibility: auto` + `contain-intrinsic-size: 80px` 与 `measureElement` 动态高度测量冲突。

链路：
1. `.task-virtual-item` 设了 `content-visibility: auto`，浏览器对不在视口的元素跳过渲染，用 80px 占位
2. ResizeObserver 测量时读到 80px 占位值（而非实际高度），写入 `itemSizeCache`
3. 后续 `measureElement` 同步路径发现缓存有值就直接返回 80px，不再读 DOM → **缓存中毒自我强化**
4. `getTotalSize()` = Σ(被污染的 80px) → 容器高度错误 → `calculateRange()` 计算错误窗口 → 漏渲染可见 item → 白屏

**"空白高度不固定"的原因**：group card ~120px、sub_section ~52px、task card 80-200px，与 80px 占位的差值正负不一；且污染的缓存条目随滚动动态变化。

### 2. 时间线丑

**根因**：
- `background: var(--tl-card-border)` — 边框色 token（半透明灰 `rgba(0,0,0,0.08)`）当背景色，卡片看起来"脏"
- 自定义 `#detail` slot 覆盖了 UnifiedTimelineCard 默认 grid 布局
- `.timeline` 容器没有左侧竖线，不像时间线，像散乱卡片
- `grid-column: 1 / -1` 失效（父容器非 grid）

### 3. Tasks.vue 显示冲突（15 处 CSS 冲突）

**P0 bug**：
- `sub-section-progress-track` absolute 定位但父级无 `position: relative` → 进度条跑到错误位置
- `--background`（shadow DOM 内部）与 `background-color`（host）双层叠加 → backdrop-filter 失效
- 暗黑模式多处硬编码 `#e65100` / `#666` / `#f0f0f0` 不可见

**P1 视觉**：
- `--tl-*` token 完全未使用（与 UnifiedTimelineCard 视觉脱节）
- 8 种圆角 / 15+ 种硬编码颜色 / 3 种等宽字体声明方式
- ion-item `button detail` 自动 chevron 与自定义 chevron-btn 冲突

### 4. 时间线步骤缺少细节

**根因**：
- `step.detail` 只在最后一个 step（encrypting/decrypting）有值（= outputPath），其他 phase 为空
- WS `task:completed` 事件不推送 steps 数组，前端任务刚完成时看不到 outputPath
- 展开后只有 startedAt/completedAt/duration/outputPath，缺加密参数/源文件路径

### 5. 产物定位跳转失效

**根因**：
- `Files.vue` **完全不读取 `route.query`**（不 import useRoute，onIonViewWillEnter 不解析 query）
- `outputPath` 是绝对路径（`/storage/emulated/0/...`），与前端 mount 虚拟路径（`/d/primary/...`）不匹配
- 用户要求："前后端全链路统一使用虚拟路径，不该关心路径细节的不要关心耦合"

### 6. FFMPEG 日志时间溢出

**根因**：
- `entry.at` 是原始 ISO 字符串（24 字符 `2026-06-18T10:23:45.123Z`）未格式化
- `.utc__time` `white-space: nowrap` + `.utc__header-right` `flex-shrink: 0` + `.utc__card` `overflow: hidden` 裁剪

### 7. 插件测试报告未合并

**根因**：Tasks.vue group card 只展示扁平任务卡，缺报告可视化入口。

---

## 二、修复方案（11 个 Task，按依赖顺序）

### Task 1: 虚拟滚动白屏修复

**文件**：`app/encv-mobile/src/components/tasks/TaskVirtualList.vue`

**修改**：
1. 移除 `.task-virtual-item` 的 `content-visibility: auto` + `contain-intrinsic-size: 80px`
2. `estimateSize` 从 80 改为 120（接近 task card 实际高度）
3. `overscan` 从 20 降到 10
4. 启用 `useAnimationFrameWithResizeObserver: true`（避免 RO 回调与 Vue patch 竞争）

**验收**：快速滚动任务列表（含 group/sub_section/task 混合）不出现白屏；展开/折叠 group card 高度变化被正确测量。

---

### Task 2: 扩展 timeline-tokens.css

**文件**：`app/encv-mobile/src/styles/timeline-tokens.css`

**新增 token**：
```css
:root {
  /* 间距 */
  --tl-space-xs: 2px;
  --tl-space-sm: 4px;
  --tl-space-md: 8px;
  --tl-space-lg: 12px;
  --tl-space-xl: 16px;
  --tl-space-2xl: 20px;
  --tl-space-3xl: 24px;

  /* 阴影 */
  --tl-shadow-card: 0 1px 3px rgba(0, 0, 0, 0.04);
  --tl-shadow-card-elevated: 0 1px 0 rgba(0, 0, 0, 0.04), 0 4px 12px -4px rgba(0, 0, 0, 0.06);
  --tl-shadow-icon-bubble: 0 1px 2px rgba(0, 0, 0, 0.1);

  /* icon-bubble 尺寸 */
  --tl-bubble-size-lg: 40px;
  --tl-bubble-size-md: 28px;
  --tl-bubble-size-sm: 20px;
  --tl-bubble-radius-circle: 50%;
  --tl-bubble-radius-rounded: 8px;

  /* 触发器 tone 色 */
  --tl-trigger-automation: var(--tl-state-analyzing);
  --tl-trigger-automation-rgb: var(--tl-state-analyzing-rgb);
  --tl-trigger-ai-agent: #8b5cf6;
  --tl-trigger-ai-agent-rgb: 139, 92, 246;

  /* section dimension tone 色 */
  --tl-dim-plugin: var(--tl-state-analyzing);
  --tl-dim-plugin-rgb: var(--tl-state-analyzing-rgb);
  --tl-dim-type: #ffa726;
  --tl-dim-type-rgb: 255, 167, 38;
  --tl-dim-category: #36af6e;
  --tl-dim-category-rgb: 54, 175, 110;
  --tl-dim-none: #9e9e9e;
  --tl-dim-none-rgb: 158, 158, 158;

  /* 进度条高度扩展 */
  --tl-progress-height-lg: 6px;
  --tl-progress-height-sm: 2px;

  /* ion-item 集成 */
  --tl-item-padding-start: 16px;
  --tl-item-padding-start-group: 20px;
  --tl-item-padding-start-subsection: 56px;
  --tl-item-min-height: 52px;
  --tl-item-min-height-group: 64px;
}

body.dark {
  --tl-trigger-ai-agent: #a78bfa;
  --tl-trigger-ai-agent-rgb: 167, 139, 250;
  --tl-dim-type: #ffb74d;
  --tl-dim-type-rgb: 255, 183, 77;
  --tl-dim-category: #66bb6a;
  --tl-dim-category-rgb: 102, 187, 106;
  --tl-dim-none: #bdbdbd;
  --tl-dim-none-rgb: 189, 189, 189;
}
```

**验收**：token 定义无重复，light/dark 模式都覆盖。

---

### Task 3: 抽象 utility class

**文件**：`app/encv-mobile/src/styles/timeline-utilities.css`（新建）

**新增 utility class**：
- `.tl-item-card` — ion-item 卡片基类（position: relative + transparent background + token 化 border/radius/shadow）
- `.tl-item-card--group` / `.tl-item-card--subsection` — 变体
- `.tl-bubble` + `.tl-bubble--lg/md/sm` + `.tl-bubble--tone-*` — 统一 icon-bubble
- `.tl-status-badge` — 统一 status badge
- `.tl-progress` + `.tl-progress--lg/sm` + `.tl-progress__fill` — 统一进度条

**引入位置**：`app/encv-mobile/src/main.ts` 在 `timeline-tokens.css` 之后引入。

**验收**：utility class 不依赖 ion-item 内部变量，屏蔽 `--background` / `--padding-start` 冲突。

---

### Task 4: Tasks.vue 三种 item kind 改用 utility class + 修 15 处 CSS 冲突

**文件**：`app/encv-mobile/src/views/Tasks.vue`

**修改**：
1. group card / sub_section_header / task card 改用 `.tl-item-card` utility class
2. icon-bubble 改用 `.tl-bubble` utility class
3. status badge 改用 `.tl-status-badge` utility class
4. progress-track 改用 `.tl-progress` utility class
5. 修复 P0 bug：
   - sub-section-progress-track 父级加 `position: relative`
   - sub-section-header `--background` 与 `background-color` 二选一
   - 暗黑模式硬编码颜色改用 token
6. 硬编码颜色改用 `--tl-*` token
7. 字体统一为 `var(--tl-card-font-mono)`
8. ion-item `button detail` 与自定义 chevron-btn 二选一

**15 处 CSS 冲突清单**：
1. sub-section-header `--background` vs `background-color` 双层叠加
2. ion-item `--padding-start: 56px` vs icon-bubble `margin-left: -40px` 负 margin hack
3. sub-section-progress-track absolute 定位父级无 relative
4. ion-item `button detail` 自动 chevron vs 自定义 chevron-btn
5. ion-item-sliding 内 ion-item `--background` 不生效
6. group card `border-left: 4px` vs ion-item padding 对齐
7. group card `overflow: hidden` vs ion-item-sliding
8. 硬编码颜色 vs design token 不对齐
9. 字体声明方式不一致（3 种）
10. 圆角不成体系（8 种值）
11. ion-label 内嵌混合块级元素
12. ion-item `--inner-padding-end: 0` vs slot="end" 内容
13. ion-item `--min-height: 52px` vs 实际内容高度
14. task-warning 暗黑模式不适配
15. task-warning-detail pre 文字硬编码

**验收**：light/dark 模式下三种 item kind 视觉一致，无显示冲突，进度条位置正确。

---

### Task 5: 时间线样式修复

**文件**：`app/encv-mobile/src/components/TaskTimeline.vue`

**修改**：
1. 修复 token 误用：`background: var(--tl-card-border)` → `background: rgba(var(--tl-state-*-rgb), 0.04)`
2. 自定义 #detail slot 内包一层 grid 容器：
   ```css
   .timeline-detail-grid {
     display: grid;
     grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
     gap: 8px;
     border-left: var(--tl-detail-border-left);
     padding: var(--tl-detail-padding);
     animation: utc-detail-enter 0.2s ease;
   }
   ```
3. `grid-column: 1 / -1` 在 grid 容器内生效
4. `.timeline` 容器加左侧竖线（可选，如果 UnifiedTimelineCard 自带连线则不需要）

**验收**：时间线展开详情排列整齐，背景色不再"脏"，有左侧边线视觉锚点。

---

### Task 6: 后端 phase detail 填充 + task:completed WS 增加 outputPath

**文件**：
- `internal/service/task_manager.go`
- `internal/server/mobile_api.go`（WS 推送）

**修改**：
1. `updateProgress` 在 phase 切换时为新 TaskStep 填充有意义的 Detail：
   - `analyzing` → "分析源文件格式与流信息"
   - `initializing` → "初始化加密引擎"
   - `preprocessing` → "预处理源数据"
   - `encrypting` → "加密数据流"
   - `decrypting` → "解密数据流"
   - `packing` → "打包加密产物"
   - `verifying` → "校验产物完整性"
2. `task:completed` WS 事件 payload 增加 `outputPath` 字段（从 task.outputPath 取值）
3. 保持 `step.detail = outputPath` 的现有逻辑（任务完成时附加到最后一个 step）

**验收**：后端单测覆盖 phase detail 填充；WS task:completed 事件含 outputPath。

---

### Task 7: 前端时间线步骤细节补充

**文件**：`app/encv-mobile/src/components/TaskTimeline.vue`

**修改**：
1. `created` 条目展开详情显示源文件路径（`task.sourcePath`）
2. `encrypting` / `decrypting` step 展开详情显示加密参数摘要（cipherMode / compressionMode）
3. `completed` 条目展开详情显示 outputPath（用 `task.outputPath`，不依赖 step.detail）
4. 每个 step 展开详情显示 `step.detail`（phase 描述，Task 6 填充）
5. `applyTaskCompleted`（useTasksList.ts）用 WS 推送的 outputPath 补写到最后一个 step

**验收**：时间线展开后每个步骤有有意义的细节信息，任务完成即可看到 outputPath（无需下拉刷新）。

---

### Task 8: 后端统一虚拟路径 + 前端 Files.vue 读取 route.query

**文件**：
- `internal/service/task_manager.go`（outputPath 改为虚拟路径）
- `internal/server/mobile_api.go`（如有路径转换）
- `app/encv-mobile/src/views/Files.vue`
- `app/encv-mobile/src/components/TaskDetailModal.vue`

**修改**：
1. **后端**：`task.outputPath` 和 `step.detail` 统一存储/返回虚拟路径（如 `/d/primary/...`），不返回绝对路径
   - 如果后端无法直接生成虚拟路径，则在 API 层做绝对路径 → 虚拟路径的转换
2. **前端 Files.vue**：
   - `import { useRoute }` 
   - `onIonViewWillEnter` 读取 `route.query.path` / `route.query.highlight`
   - 若 `path` query 存在且与 `currentPath` 不同，设置 `currentPath.value = qPath` 并调用 `loadFiles()`
   - 新增 `highlightFile(name)` 函数：在 `files.value` 中查找匹配项，`scrollIntoView` + 临时高亮 class（2 秒后移除）
3. **TaskDetailModal.vue**：`locateOutput` 不再做路径转换（后端已统一虚拟路径）

**验收**：点击"定位"按钮跳转到 Files tab，自动导航到输出文件所在目录并高亮目标文件。

---

### Task 9: FFMPEG 日志时间格式化

**文件**：
- `app/encv-mobile/src/components/developer/MockGenLogCard.vue`
- `app/encv-mobile/src/components/shared/UnifiedTimelineCard.vue`

**修改**：
1. `MockGenLogCard.vue` L177 `time: entry.at` → `time: formatDateTime(entry.at)` 或提取时分秒（`HH:mm:ss`）
2. `UnifiedTimelineCard.vue` `.utc__time` 加 `max-width: 120px; overflow: hidden; text-overflow: ellipsis;`

**验收**：FFMPEG 日志卡时间显示为 `HH:mm:ss` 格式，窄屏下不溢出。

---

### Task 10: 插件测试报告 group card 跳转按钮

**文件**：
- `app/encv-mobile/src/views/Tasks.vue`
- `app/encv-mobile/src/views/PluginTestsDetail.vue`

**修改**：
1. Tasks.vue group card 加「查看报告」按钮（ion-button slot=end, fill=clear）
2. 点击跳转到 `/tabs/settings/devtools/plugin-tests?runId=xxx`
3. PluginTestsDetail.vue `onMounted` 读取 `route.query.runId`，自动 `selectHistoryRun(runId)`

**验收**：group card 点击「查看报告」跳转到 PluginTestsDetail 并自动选中对应 run。

---

### Task 11: 回归测试

**验证项**：
1. TypeScript 编译通过（`vue-tsc --noEmit`）
2. 前端单测全通过（`pnpm run test:run`）
3. 后端 Go 测试通过（`bash scripts/test-go.sh ./internal/service/`）
4. Go vet/build 通过
5. 新增单测覆盖：
   - TaskVirtualList 白屏修复（content-visibility 移除后测量正确）
   - TaskTimeline 时间线细节（源文件/加密参数/outputPath 显示）
   - TaskBasicInfo crypto params（已有）
   - Files.vue route.query 读取
   - PluginTestsDetail query 接收

---

## 三、实施顺序（依赖关系）

```
Task 1: 虚拟滚动白屏修复（独立，无依赖）
    ↓
Task 2: 扩展 timeline-tokens.css（基础，Task 3/4/5 依赖）
    ↓
Task 3: 抽象 utility class（依赖 Task 2，Task 4 依赖）
    ↓
Task 4: Tasks.vue 改用 utility class + 修 15 处冲突（依赖 Task 3）
    ↓
Task 5: 时间线样式修复（依赖 Task 2 的 token）
    ↓
Task 6: 后端 phase detail + WS outputPath（独立，Task 7 依赖）
    ↓
Task 7: 前端时间线步骤细节补充（依赖 Task 6）
    ↓
Task 8: 后端虚拟路径 + 前端 Files.vue route.query（独立）
    ↓
Task 9: FFMPEG 日志时间格式化（独立）
    ↓
Task 10: 插件测试报告跳转按钮（独立）
    ↓
Task 11: 回归测试
```

**可并行**：Task 1 / Task 6 / Task 8 / Task 9 / Task 10 互相独立，可并行实施。

---

## 四、风险与缓解

| 风险 | 缓解 |
|------|------|
| Task 4 Tasks.vue 重构范围大（1688 行） | 分阶段：先修 P0 bug → 再改 token → 最后改 utility class |
| Task 6 后端 phase detail 填充可能影响现有任务 | detail 字段是可选的，前端兼容空值 |
| Task 8 后端路径统一可能影响现有 API | 保持绝对路径作为 fallback，虚拟路径作为首选 |
| 虚拟滚动移除 content-visibility 后性能 | overscan 降到 10 + useAnimationFrameWithResizeObserver 补偿 |
