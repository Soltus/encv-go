> ⚠️ **本文档已过时，请勿按此执行。**
> 当前权威架构文档是 **`app/docs/migration-task-system.md`**（拆分抽象与实现：`@encv/shared-components` = 纯抽象层/库，只依赖 vue/pinia/ionic/通用第三方，**不依赖** `@/config` `@/constants` `@/router`；`encv-mobile` = 应用层，提供共享抽象所需的注入上下文）。
> 本文档的核心理念（“encv 业务全搬回 encv-mobile，shared 只放通用”）与已落地的 **DI 共享抽象层直接相反**；尤其 Phase 1/2/3 要求把 `stores/`、`api/` 搬出 shared，会**摧毁已建好的共享抽象层**（`taskStore`/`runTasksStore` 等已提升进 shared，经依赖注入解耦应用层）。
> 保留本文档仅作历史参考。实际重构进度与边界判定见 `migration-task-system.md`（§2 目标架构、§7 模块地图、§9 执行进度）。

# shared-components 边界规范（SPEC）

> **核心原则**：shared-components 只放「两个项目都真正能用」的公共部分。
> 不是「把 encv-mobile 全搬过去让 simverse 挑着用」，而是「两边都需要的才放进去」。
> ⚠️ 注意：上述“公共部分”包含**经依赖注入解耦后的任务系统抽象**（已在 migration-task-system.md 落地），并非仅限 UI/工具。详见权威文档。

## 一、现状诊断

### 1.1 问题

当前 shared-components 本质是 encv-mobile 的「全量镜像」：
- `api/` 下全是 encv 业务 API（`encv_tasks.ts` / `encv_plugins.ts` 等）
- `views/` 下全是 encv 业务页面（`Tasks.vue` / `Files.vue` / `AgentChat.vue` 等）
- `composables/` 下 80% 是 encv 业务逻辑（任务列表、文件列表、Agent 引擎等）
- `plugins/` 下全是 encv 特有（`GoProcess.ts` / `ApiProxy.ts` 等）
- simverse-frontend 根本用不上这些，完全谈不上「复用」

### 1.2 目标

抽丝剥茧，把 encv-mobile 特有的业务代码从 shared-components 移走，只留下真正可复用的公共部分。

**最终状态**：
- shared-components：纯公共基础库（UI 组件 + 通用 composables + 工具函数 + 主题 + i18n 框架）
- encv-mobile：自己的业务代码 + 从 shared-components 复用公共部分
- simverse-frontend：自己的业务代码 + 从 shared-components 复用公共部分

## 二、模块归属判定表

### 2.1 判定标准

| 类别 | 判定规则 | 举例 |
|------|---------|------|
| ✅ 可复用 | 两边项目都可能用到，不依赖任何一方的业务 API | useTheme、useToast、卡片组件 |
| ❌ encv 特有 | 依赖 encv API、GoProcess、加密解密业务、插件系统 | useTasksList、encv_tasks.ts、Tasks.vue |
| ❌ simverse 特有 | 依赖 simverse API、世界地图、NPC、抽卡 | （目前大部分在 simverse-frontend 里） |

### 2.2 目录级判定

| 目录 | 归属 | 说明 |
|------|------|------|
| `api/` | ❌ encv 特有 | 全部是 encv_*.ts，无任何通用 API |
| `components/agent/` | ❌ encv 特有 | Agent 聊天 UI，依赖 Agent 业务 |
| `components/automation/` | ❌ encv 特有 | 自动化测试 UI，依赖 workflow 业务 |
| `components/developer/` | ❌ encv 特有 | Mock 生成日志，encv 开发工具 |
| `components/group-detail/` | ❌ encv 特有 | 任务组详情，encv 业务 |
| `components/tasks/` | ❌ encv 特有 | 任务虚拟列表等，encv 业务 |
| `components/shared/` | ✅ 可复用 | 通用卡片、徽章、时间线 |
| `components/` 根文件 | ❌ encv 特有 | NewTaskModal、FilePickerModal 等都是 encv 业务 |
| `components-shared/` | ❌ encv 特有 | OpenList 相关，encv 业务 |
| `composables/` | ⚠️ 混合 | 需逐个判定（见下表） |
| `config/` | ❌ encv 特有 | encv 配置 schema |
| `constants/` | ❌ encv 特有 | 容器版本、播放器常量 |
| `directives/` | ✅ 可复用 | 通用 Vue 指令（longpress 等） |
| `engines/` | ❌ encv 特有 | TDesign Chat 引擎，Agent 业务 |
| `features/` | ❌ encv 特有 | alist-encrypt 业务功能 |
| `i18n/` | ⚠️ 混合 | 框架通用，字典分模块（见下表） |
| `lib/workflow/` | ❌ encv 特有 | 工作流引擎，encv 自动化测试 |
| `lib/` 其他 | ⚠️ 混合 | 需逐个判定 |
| `plugins/` | ❌ encv 特有 | GoProcess、ApiProxy 等 |
| `stores/` | ❌ encv 特有 | 任务 store，encv 业务 |
| `styles/` | ✅ 可复用 | 通用样式 tokens、timeline 样式 |
| `theme/` | ✅ 可复用 | 主题变量 |
| `types/` | ⚠️ 混合 | 部分通用部分特有 |
| `utils/` | ✅ 可复用 | RingBuffer、IncrementalFilter 等通用工具 |
| `views/` | ❌ 基本全是 encv | 只有 Settings/DevLogs/NotFound 可考虑通用 |
| `workers/` | ❌ encv 特有 | 任务视图计算 worker |

### 2.3 composables 逐个判定

| composable | 归属 | 理由 |
|------------|------|------|
| `useTheme.ts` | ✅ 通用 | 主题切换，两边都用 |
| `useI18n.ts` | ✅ 通用 | 国际化，两边都用 |
| `useToast.ts` | ✅ 通用 | Toast 提示，两边都用 |
| `useClipboard.ts` | ✅ 通用 | 剪贴板，两边都用 |
| `useDateFormat.ts` | ✅ 通用 | 日期格式化，两边都用 |
| `relativeTime.ts` | ✅ 通用 | 相对时间格式化 |
| `useErrorCapture.ts` | ✅ 通用 | 错误捕获，两边都用 |
| `useFrontendLogs.ts` | ✅ 通用 | 前端日志，两边都用 |
| `useEventBus.ts` | ✅ 通用 | 事件总线，两边都用 |
| `useDevTools.ts` | ✅ 通用 | vConsole 等开发工具 |
| `useHighRefreshRate.ts` | ✅ 通用 | 高刷适配，移动端都用 |
| `useIonicAutoRegister.ts` | ✅ 通用 | Ionic 组件自动注册 |
| `usePinchZoom.ts` | ✅ 通用 | 捏合缩放手势 |
| `useSearchInput.ts` | ✅ 通用 | 搜索输入框交互 |
| `activeStatus.ts` | ✅ 通用 | 页面活跃状态 |
| `devlogApiError.ts` | ❌ encv | 依赖 encv API 错误处理 |
| `useApiBaseProbe.ts` | ❌ encv | 探测 encv 服务端 |
| `useProxiedFetch.ts` | ❌ encv | 依赖 GoProcess 代理 |
| `useRealtimeTransport.ts` | ❌ encv | 依赖 encv 实时推送 |
| `useServerStatus.ts` | ❌ encv | 依赖 encv 服务端状态 |
| `useTasksList.ts` | ❌ encv | 任务列表业务 |
| `useTaskForm.ts` | ❌ encv | 任务表单业务 |
| `useTaskTrigger.ts` | ❌ encv | 任务触发业务 |
| `useTaskEventBridge.ts` | ❌ encv | 任务事件桥接 |
| `useTaskCancel.ts` | ❌ encv | 任务取消 |
| `useTaskViewCompute.ts` | ❌ encv | 任务视图计算 |
| `useNewTaskModal.ts` | ❌ encv | 新建任务弹窗 |
| `useFileList.ts` | ❌ encv | 文件列表业务 |
| `useFileFeatures.ts` | ❌ encv | 文件功能注册 |
| `usePathResolver.ts` | ❌ encv | 路径解析业务 |
| `useLibraries.ts` | ❌ encv | 插件库业务 |
| `usePluginExtensions.ts` | ❌ encv | 插件扩展 |
| `useOpenListBridge.ts` | ❌ encv | OpenList 桥接 |
| `useMockGenLog.ts` | ❌ encv | Mock 生成日志 |
| `useChatEngine.ts` | ❌ encv | Agent 聊天引擎 |
| `useAgent.ts` | ❌ encv | Agent 业务 |
| `useAgentApiBase.ts` | ❌ encv | Agent API 基地址 |
| `useAGUIParser.ts` | ❌ encv | AGUI 解析 |
| `useAgent_helpers.ts` | ❌ encv | Agent 辅助函数 |
| `useBatchOperations.ts` | ❌ encv | 批量操作（任务/文件） |
| `useContextUsage.ts` | ❌ encv | Agent 上下文用量 |
| `useErrorAnalyzer.ts` | ❌ encv | 错误分析（依赖 encv 任务） |
| `useSectionDerivation.ts` | ❌ encv | 章节推导（Agent 业务） |
| `useSlashMenu.ts` | ❌ encv | 斜杠菜单（Agent 业务） |
| `useToolCallAccumulator.ts` | ❌ encv | 工具调用累加器 |
| `useVectorSearchStatus.ts` | ❌ encv | 向量搜索状态 |
| `useWebDavManifest.ts` | ❌ encv | WebDAV 清单 |
| `useWebDavTestModules.ts` | ❌ encv | WebDAV 测试模块 |
| `useWebDavTestRunner.ts` | ❌ encv | WebDAV 测试运行器 |
| `useWebDavWorkflowAdapter.ts` | ❌ encv | WebDAV 工作流适配器 |
| `useWorkflowStore.ts` | ❌ encv | 工作流 store |
| `useWorkflowTaskService.ts` | ❌ encv | 工作流任务服务 |
| `useTestCaseGeneration.ts` | ❌ encv | 测试用例生成 |
| `useRunSummaries.ts` | ❌ encv | 运行摘要 |
| `useConfig.ts` | ❌ encv | encv 配置 |
| `useInputHistory.ts` | ❌ encv | 输入历史（Agent 相关） |
| `useDeviceId.ts` | ❌ encv | 设备 ID（encv 业务） |
| `useAttachments.ts` | ❌ encv | 附件（Agent 相关） |
| `useThumbnailCache.ts` | ❌ encv | 缩略图缓存（文件相关） |
| `useFileSystemTests.ts` | ❌ encv | 文件系统测试 |
| `useTestBackdoor.ts` | ❌ encv | 测试后门 |
| `chatEngine.ts` | ❌ encv | Chat 引擎 |
| `reasoningEffort.ts` | ❌ encv | 推理深度（Agent 相关） |
| `renderTurnItems.ts` | ❌ encv | 回合项渲染（Agent 相关） |
| `inlineFileReference.ts` | ❌ encv | 行内文件引用（Agent 相关） |
| `appServerRealtimeReducer.ts` | ❌ encv | 服务端实时状态 reducer |
| `realtime/` | ❌ encv | 全部实时传输后端 |

### 2.4 i18n 字典模块判定

| 字典文件 | 归属 | 说明 |
|----------|------|------|
| `common.ts` | ✅ 通用 | 通用文案（确认、取消、加载中等） |
| `errors.ts` | ✅ 通用 | 通用错误文案 |
| `settings.ts` | ⚠️ 混合 | 外观/语言/关于通用，服务器/插件等是 encv 特有 |
| `devlogs.ts` | ✅ 通用 | 开发者日志通用 |
| `simverse.ts` | ❌ simverse | simverse 特有文案 |
| `tasks.ts` | ❌ encv | 任务相关 |
| `files.ts` | ❌ encv | 文件相关 |
| `agent.ts` | ❌ encv | Agent 相关 |
| `player.ts` | ❌ encv | 播放器相关 |
| `modals.ts` | ❌ encv | 弹窗相关（新建任务等） |
| `extensions.ts` | ❌ encv | 插件扩展相关 |

### 2.5 types 判定

| 类型文件 | 归属 | 说明 |
|----------|------|------|
| `appError.ts` | ✅ 通用 | 应用错误类型 |
| `appResult.ts` | ✅ 通用 | 结果包装类型 |
| `messageStatus.ts` | ❌ encv | Agent 消息状态 |
| `phase.ts` | ⚠️ 混合 | 阶段类型，encv 任务在用，但概念通用 |
| `tokenSnapshot.ts` | ❌ encv | Token 用量快照 |
| `simverse.ts` | ❌ simverse | simverse 类型 |
| `file-feature.ts` | ❌ encv | 文件功能类型 |
| `webdav-test.ts` | ❌ encv | WebDAV 测试类型 |

### 2.6 lib 判定

| 文件 | 归属 | 说明 |
|------|------|------|
| `dev-start-guard.ts` | ✅ 通用 | 开发启动守卫 |
| `workflow/` | ❌ encv | 工作流引擎全套 |
| `mockDataGenerator.ts` | ❌ encv | Mock 数据生成（encv 业务） |
| `mockConstants.ts` | ❌ encv | Mock 常量 |
| `mountPath.ts` | ❌ encv | 挂载路径（encv 业务） |
| `taskPersistence.ts` | ❌ encv | 任务持久化 |
| `taskTypeLabel.ts` | ❌ encv | 任务类型标签 |
| `buildReportZip.ts` | ❌ encv | 构建报告 zip |

### 2.7 views 判定

| 页面 | 归属 | 说明 |
|------|------|------|
| `Settings.vue` | ⚠️ 半通用 | 框架通用，但具体菜单项 encv 特有 |
| `AppearanceDetail.vue` | ✅ 通用 | 外观设置 |
| `AboutDetail.vue` | ✅ 通用 | 关于页 |
| `DevLogs.vue` | ✅ 通用 | 开发者日志 |
| `NotFoundView.vue` | ✅ 通用 | 404 页 |
| `DevToolsDetail.vue` | ✅ 通用 | 开发工具 |
| `LogSettingsDetail.vue` | ⚠️ 半通用 | 日志设置概念通用，但选项可能不同 |
| 其他所有 views | ❌ encv | 全部是 encv 业务页面 |

## 三、重构实施计划

### Phase 1：搬走 encv 特有 API + plugins + features + stores

**目标**：把最明确的 encv 业务层先移走

**操作**：
1. `api/` 全部移回 encv-mobile/src/api/
2. `plugins/` 全部移回 encv-mobile/src/plugins/
3. `features/` 全部移回 encv-mobile/src/features/
4. `stores/` 全部移回 encv-mobile/src/stores/
5. `config/` 全部移回 encv-mobile/src/config/
6. `constants/` 全部移回 encv-mobile/src/constants/
7. `workers/` 全部移回 encv-mobile/src/workers/
8. `generated/` 移回 encv-mobile/src/generated/

**验证**：
- encv-mobile 构建通过
- shared-components 不再包含上述目录

### Phase 2：搬走 encv 特有业务 views + components

**目标**：把业务 UI 层移走

**操作**：
1. `views/` 下 encv 特有的页面移回 encv-mobile/src/views/
2. `components/agent/` 移回 encv-mobile/src/components/agent/
3. `components/automation/` 移回 encv-mobile/src/components/automation/
4. `components/developer/` 移回 encv-mobile/src/components/developer/
5. `components/group-detail/` 移回 encv-mobile/src/components/group-detail/
6. `components/tasks/` 移回 encv-mobile/src/components/tasks/
7. `components/` 根下 encv 特有组件移回 encv-mobile/src/components/
8. `components-shared/` 移回 encv-mobile/src/components-shared/
9. `engines/` 移回 encv-mobile/src/engines/
10. `views/prototypes/` 移回 encv-mobile/src/views/prototypes/

**保留在 shared-components 的 views**：
- `AppearanceDetail.vue`
- `AboutDetail.vue`
- `DevLogs.vue`
- `DevToolsDetail.vue`
- `NotFoundView.vue`

**保留在 shared-components 的 components**：
- `components/shared/` 全部

### Phase 3：搬走 encv 特有 composables + lib + 部分 types + 部分 i18n

**目标**：把业务逻辑层移走

**操作**：
1. composables 中 encv 特有的移回 encv-mobile/src/composables/
2. `lib/workflow/` 移回 encv-mobile/src/lib/workflow/
3. `lib/` 下 encv 特有文件移回 encv-mobile/src/lib/
4. `types/` 下 encv 特有文件移回 encv-mobile/src/types/
5. `i18n/` 下 encv 特有字典移回 encv-mobile/src/i18n/

### Phase 4：清理 + 验证

**操作**：
1. 整理 shared-components 的 index.ts 导出，只导出公共部分
2. 整理 shared-components 的 package.json dependencies，只保留公共依赖
3. encv-mobile 构建验证通过
4. simverse-frontend 构建验证通过
5. 两个项目 typecheck 都通过

## 四、最终 shared-components 目录结构（目标态）

```
packages/shared-components/src/
├── components/
│   └── shared/           ← 通用 UI 组件（卡片、徽章、时间线等）
├── composables/          ← 通用 composables（主题、i18n、toast、错误捕获等）
├── directives/           ← 通用 Vue 指令
├── i18n/                 ← i18n 框架 + 通用字典（common、errors 等）
├── styles/               ← 通用样式（timeline tokens 等）
├── theme/                ← 主题变量
├── types/                ← 通用类型定义
├── utils/                ← 通用工具函数（RingBuffer 等）
├── views/                ← 通用页面（外观设置、关于、DevLogs、404 等）
├── index.ts              ← 公共 API 导出
└── env.d.ts              ← 类型声明
```

## 五、代码量预期

| 项目 | 重构前 | 重构后（目标） | 说明 |
|------|--------|-------------|------|
| encv-mobile | ~1k 行（当前空壳状态） | ~50k 行 | 业务代码搬回来，但不再是 shared-components 的镜像 |
| shared-components | ~80k 行（全是 encv 业务） | ~10k 行 | 只留真正通用的公共部分 |
| simverse-frontend | ~5k 行（各搞一套） | ~3k 行 | 复用 shared-components 公共部分后减少重复 |

**复用价值**：simverse-frontend 不需要再写一套主题、i18n、通用组件、DevLogs、设置页框架，直接从 shared-components 复用。

## 六、风险与注意事项

1. **import 路径兼容**：encv-mobile 原来用 `@/xxx`，搬回后路径不变，不影响
2. **shared-components 导出收缩**：index.ts 需要重新整理，只导出公共部分
3. **依赖清理**：shared-components 的 package.json 里要删掉 encv 业务特有的依赖（artplayer、@capacitor/filesystem 等）
4. **i18n 模块拆分**：settings.ts 要拆成通用部分和 encv 特有部分
5. **逐步推进**：按 Phase 1→2→3→4 顺序，每个 Phase 都验证构建通过后再推进下一个

