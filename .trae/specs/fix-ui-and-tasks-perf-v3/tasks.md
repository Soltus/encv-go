# 第三轮修复 Task 清单

> 创建：2026-06-18
> 共 11 个 Task，按依赖顺序排列

---

## Task 1: 虚拟滚动白屏修复

**状态**：待实施
**文件**：`app/encv-mobile/src/components/tasks/TaskVirtualList.vue`

**修改**：
- 移除 `.task-virtual-item` 的 `content-visibility: auto` + `contain-intrinsic-size: 80px`
- `estimateSize` 从 80 改为 120
- `overscan` 从 20 降到 10
- 启用 `useAnimationFrameWithResizeObserver: true`

**验收**：快速滚动不白屏；展开/折叠 group card 高度正确测量

---

## Task 2: 扩展 timeline-tokens.css

**状态**：待实施
**文件**：`app/encv-mobile/src/styles/timeline-tokens.css`

**新增 token**：
- 间距：`--tl-space-xs/sm/md/lg/xl/2xl/3xl`
- 阴影：`--tl-shadow-card/card-elevated/icon-bubble`
- icon-bubble 尺寸：`--tl-bubble-size-lg/md/sm` + `--tl-bubble-radius-circle/rounded`
- 触发器 tone 色：`--tl-trigger-automation/ai-agent` + `-rgb`
- section dimension tone 色：`--tl-dim-plugin/type/category/none` + `-rgb`
- 进度条高度：`--tl-progress-height-lg/sm`
- ion-item 集成：`--tl-item-padding-start/group/subsection` + `--tl-item-min-height/group`

**验收**：light/dark 模式都覆盖；无重复 token

---

## Task 3: 抽象 utility class

**状态**：待实施
**文件**：`app/encv-mobile/src/styles/timeline-utilities.css`（新建）

**新增**：
- `.tl-item-card` — ion-item 卡片基类（position: relative + transparent background + token 化）
- `.tl-item-card--group` / `.tl-item-card--subsection` — 变体
- `.tl-bubble` + `--lg/md/sm` + `--tone-*` — 统一 icon-bubble
- `.tl-status-badge` — 统一 status badge
- `.tl-progress` + `--lg/sm` + `__fill` — 统一进度条

**引入**：`main.ts` 在 `timeline-tokens.css` 之后引入

**验收**：utility class 屏蔽 ion-item 内部变量冲突

---

## Task 4: Tasks.vue 改用 utility class + 修 15 处 CSS 冲突

**状态**：待实施
**文件**：`app/encv-mobile/src/views/Tasks.vue`

**修改**：
- group/sub_section/task card 改用 `.tl-item-card` utility class
- icon-bubble 改用 `.tl-bubble`
- status badge 改用 `.tl-status-badge`
- progress-track 改用 `.tl-progress`
- 修 15 处 CSS 冲突（见 spec §一.3）
- 硬编码颜色 → `--tl-*` token
- 字体统一 `var(--tl-card-font-mono)`

**验收**：light/dark 模式三种 item kind 视觉一致，无冲突，进度条位置正确

---

## Task 5: 时间线样式修复

**状态**：待实施
**文件**：`app/encv-mobile/src/components/TaskTimeline.vue`

**修改**：
- `background: var(--tl-card-border)` → `background: rgba(var(--tl-state-*-rgb), 0.04)`
- 自定义 #detail slot 内包 grid 容器（`display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 8px`）
- 加 `border-left: var(--tl-detail-border-left)`
- 加进入动画 `utc-detail-enter`

**验收**：展开详情排列整齐，背景不"脏"，有左侧边线

---

## Task 6: 后端 phase detail 填充 + task:completed WS 增加 outputPath

**状态**：待实施
**文件**：
- `internal/service/task_manager.go`
- `internal/server/mobile_api.go`

**修改**：
- `updateProgress` phase 切换时填充有意义的 Detail（analyzing/initializing/preprocessing/encrypting/decrypting/packing/verifying 各有描述）
- `task:completed` WS 事件 payload 增加 `outputPath` 字段
- 保持 `step.detail = outputPath` 现有逻辑

**验收**：后端单测覆盖；WS 事件含 outputPath

---

## Task 7: 前端时间线步骤细节补充

**状态**：待实施
**文件**：
- `app/encv-mobile/src/components/TaskTimeline.vue`
- `app/encv-mobile/src/composables/useTasksList.ts`

**修改**：
- `created` 条目展开显示源文件路径（`task.sourcePath`）
- `encrypting`/`decrypting` step 展开显示加密参数摘要
- `completed` 条目展开显示 outputPath（用 `task.outputPath`）
- 每个 step 展开显示 `step.detail`（phase 描述）
- `applyTaskCompleted` 用 WS 推送的 outputPath 补写到最后一个 step

**验收**：时间线展开有细节信息；任务完成即可看到 outputPath

---

## Task 8: 后端统一虚拟路径 + 前端 Files.vue 读取 route.query

**状态**：待实施
**文件**：
- `internal/service/task_manager.go`
- `internal/server/mobile_api.go`
- `app/encv-mobile/src/views/Files.vue`
- `app/encv-mobile/src/components/TaskDetailModal.vue`

**修改**：
- 后端 `task.outputPath` / `step.detail` 统一返回虚拟路径（如 `/d/primary/...`）
- 前端 Files.vue `import useRoute` + `onIonViewWillEnter` 读取 `route.query.path/highlight`
- 新增 `highlightFile(name)` 函数（scrollIntoView + 临时高亮 class）
- TaskDetailModal `locateOutput` 不再做路径转换

**验收**：点击"定位"跳转 Files tab，自动导航到目录并高亮文件

---

## Task 9: FFMPEG 日志时间格式化

**状态**：待实施
**文件**：
- `app/encv-mobile/src/components/developer/MockGenLogCard.vue`
- `app/encv-mobile/src/components/shared/UnifiedTimelineCard.vue`

**修改**：
- `MockGenLogCard.vue` `time: entry.at` → `time: formatDateTime(entry.at)` 或 `HH:mm:ss`
- `UnifiedTimelineCard.vue` `.utc__time` 加 `max-width: 120px; overflow: hidden; text-overflow: ellipsis;`

**验收**：时间显示 `HH:mm:ss`，窄屏不溢出

---

## Task 10: 插件测试报告 group card 跳转按钮

**状态**：待实施
**文件**：
- `app/encv-mobile/src/views/Tasks.vue`
- `app/encv-mobile/src/views/PluginTestsDetail.vue`

**修改**：
- Tasks.vue group card 加「查看报告」按钮
- 点击跳转 `/tabs/settings/devtools/plugin-tests?runId=xxx`
- PluginTestsDetail.vue 读取 `route.query.runId` 自动 selectHistoryRun

**验收**：group card 点击「查看报告」跳转并自动选中 run

---

## Task 11: 回归测试

**状态**：待实施

**验证项**：
- TypeScript 编译通过（`vue-tsc --noEmit`）
- 前端单测全通过（`pnpm run test:run`）
- 后端 Go 测试通过（`bash scripts/test-go.sh ./internal/service/`）
- Go vet/build 通过
- 新增单测覆盖：TaskVirtualList 白屏修复 / TaskTimeline 细节 / Files.vue route.query / PluginTestsDetail query

---

## 依赖关系图

```
Task 1 (虚拟滚动) ──────────────────────────────→ Task 11
Task 2 (token) → Task 3 (utility) → Task 4 (Tasks.vue) → Task 11
                → Task 5 (时间线样式) → Task 11
Task 6 (后端 detail) → Task 7 (前端细节) → Task 11
Task 8 (虚拟路径) → Task 11
Task 9 (FFMPEG 日志) → Task 11
Task 10 (报告跳转) → Task 11
```

**可并行**：Task 1 / Task 6 / Task 8 / Task 9 / Task 10 互相独立
