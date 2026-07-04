# 第三轮修复验证清单

> 创建：2026-06-18
> 共 11 个 Task 的验证项

---

## Task 1: 虚拟滚动白屏修复

- [ ] `.task-virtual-item` 的 `content-visibility: auto` + `contain-intrinsic-size: 80px` 已移除
- [ ] `estimateSize` 改为 120
- [ ] `overscan` 改为 10
- [ ] `useAnimationFrameWithResizeObserver: true` 已启用
- [ ] 快速滚动任务列表（含 group/sub_section/task 混合）不出现白屏
- [ ] 展开/折叠 group card 高度变化被正确测量
- [ ] 切 tab 回 Tasks 首屏渲染正确
- [ ] `totalSize` 稳定不"呼吸"

## Task 2: 扩展 timeline-tokens.css

- [ ] 间距 token（`--tl-space-xs/sm/md/lg/xl/2xl/3xl`）已定义
- [ ] 阴影 token（`--tl-shadow-card/card-elevated/icon-bubble`）已定义
- [ ] icon-bubble 尺寸 token（`--tl-bubble-size-lg/md/sm` + radius）已定义
- [ ] 触发器 tone 色 token（`--tl-trigger-automation/ai-agent` + `-rgb`）已定义
- [ ] section dimension tone 色 token（`--tl-dim-plugin/type/category/none` + `-rgb`）已定义
- [ ] 进度条高度 token（`--tl-progress-height-lg/sm`）已定义
- [ ] ion-item 集成 token（`--tl-item-padding-start/group/subsection` + min-height）已定义
- [ ] light/dark 模式都覆盖
- [ ] 无重复 token

## Task 3: 抽象 utility class

- [ ] `.tl-item-card` 基类已定义（position: relative + transparent background + token 化）
- [ ] `.tl-item-card--group` / `.tl-item-card--subsection` 变体已定义
- [ ] `.tl-bubble` + 尺寸变体 + tone 变体已定义
- [ ] `.tl-status-badge` 已定义
- [ ] `.tl-progress` + 高度变体 + `__fill` 已定义
- [ ] `main.ts` 在 `timeline-tokens.css` 之后引入 `timeline-utilities.css`
- [ ] utility class 不依赖 ion-item 内部变量

## Task 4: Tasks.vue 改用 utility class + 修 15 处 CSS 冲突

- [ ] group card 改用 `.tl-item-card--group`
- [ ] sub_section_header 改用 `.tl-item-card--subsection`
- [ ] task card 改用 `.tl-item-card`
- [ ] icon-bubble 改用 `.tl-bubble`
- [ ] status badge 改用 `.tl-status-badge`
- [ ] progress-track 改用 `.tl-progress`
- [ ] P0: sub-section-progress-track 父级有 `position: relative`
- [ ] P0: sub-section-header `--background` 与 `background-color` 二选一
- [ ] P0: 暗黑模式硬编码颜色（`#e65100` / `#666` / `#f0f0f0`）改用 token
- [ ] 硬编码 `rgba(79,140,255,...)` → `var(--tl-trigger-automation-rgb)`
- [ ] 硬编码 icon-bubble 渐变 → 纯色 `var(--tl-*)`
- [ ] 字体声明统一为 `var(--tl-card-font-mono)`
- [ ] ion-item `button detail` 与自定义 chevron-btn 二选一
- [ ] light/dark 模式三种 item kind 视觉一致
- [ ] 进度条位置正确（在 item 底部，不在容器底部）

## Task 5: 时间线样式修复

- [ ] `background: var(--tl-card-border)` 已改为 `rgba(var(--tl-state-*-rgb), 0.04)`
- [ ] 自定义 #detail slot 内有 grid 容器
- [ ] `grid-column: 1 / -1` 在 grid 容器内生效
- [ ] 有 `border-left: var(--tl-detail-border-left)`
- [ ] 有进入动画
- [ ] 展开详情排列整齐
- [ ] 背景色不再"脏"

## Task 6: 后端 phase detail 填充 + task:completed WS 增加 outputPath

- [ ] `updateProgress` 在 phase 切换时填充有意义的 Detail
- [ ] analyzing → "分析源文件格式与流信息"
- [ ] initializing → "初始化加密引擎"
- [ ] preprocessing → "预处理源数据"
- [ ] encrypting → "加密数据流"
- [ ] decrypting → "解密数据流"
- [ ] packing → "打包加密产物"
- [ ] verifying → "校验产物完整性"
- [ ] `task:completed` WS 事件 payload 含 `outputPath`
- [ ] 保持 `step.detail = outputPath` 现有逻辑
- [ ] 后端单测覆盖 phase detail 填充

## Task 7: 前端时间线步骤细节补充

- [ ] `created` 条目展开显示源文件路径
- [ ] `encrypting`/`decrypting` step 展开显示加密参数摘要
- [ ] `completed` 条目展开显示 outputPath（用 `task.outputPath`）
- [ ] 每个 step 展开显示 `step.detail`（phase 描述）
- [ ] `applyTaskCompleted` 用 WS 推送的 outputPath 补写到最后一个 step
- [ ] 任务完成即可看到 outputPath（无需下拉刷新）

## Task 8: 后端统一虚拟路径 + 前端 Files.vue 读取 route.query

- [ ] 后端 `task.outputPath` / `step.detail` 返回虚拟路径
- [ ] 前端 Files.vue `import useRoute`
- [ ] `onIonViewWillEnter` 读取 `route.query.path` / `route.query.highlight`
- [ ] 新增 `highlightFile(name)` 函数（scrollIntoView + 临时高亮 class）
- [ ] TaskDetailModal `locateOutput` 不做路径转换
- [ ] 点击"定位"跳转 Files tab，自动导航到目录
- [ ] 目标文件被高亮（2 秒后移除）

## Task 9: FFMPEG 日志时间格式化

- [ ] `MockGenLogCard.vue` `time: entry.at` → `time: formatDateTime(entry.at)` 或 `HH:mm:ss`
- [ ] `UnifiedTimelineCard.vue` `.utc__time` 加 `max-width: 120px`
- [ ] `.utc__time` 加 `overflow: hidden; text-overflow: ellipsis;`
- [ ] 时间显示为 `HH:mm:ss` 格式
- [ ] 窄屏下不溢出

## Task 10: 插件测试报告 group card 跳转按钮

- [ ] Tasks.vue group card 有「查看报告」按钮
- [ ] 点击跳转 `/tabs/settings/devtools/plugin-tests?runId=xxx`
- [ ] PluginTestsDetail.vue 读取 `route.query.runId`
- [ ] 自动 `selectHistoryRun(runId)`
- [ ] group card 点击「查看报告」跳转并自动选中 run

## Task 11: 回归测试

- [ ] TypeScript 编译通过（`vue-tsc --noEmit`）
- [ ] 前端单测全通过（`pnpm run test:run`）
- [ ] 后端 Go 测试通过（`bash scripts/test-go.sh ./internal/service/`）
- [ ] Go vet/build 通过
- [ ] 新增单测：TaskVirtualList 白屏修复
- [ ] 新增单测：TaskTimeline 时间线细节
- [ ] 新增单测：Files.vue route.query 读取
- [ ] 新增单测：PluginTestsDetail query 接收
