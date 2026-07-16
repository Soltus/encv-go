import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'
import path from 'path'
import { fileURLToPath } from 'node:url'
import { dirname } from 'node:path'
import { encvAliasFallback } from './vite-plugins/encv-alias-fallback'

const __dirname = dirname(fileURLToPath(import.meta.url))

// vitest 4.x: use test.alias (not resolve.alias) for tsconfig paths to be
// resolved correctly via vite's resolver. resolve.alias is silently ignored
// in some vitest 4 + vite 8 combinations, leading to "Cannot find package
// '@/...'" errors even though vue-tsc compiles fine.
const SRC_DIR = path.resolve(__dirname, './src')
const TDESIGN_STUB = path.resolve(__dirname, './src/engines/__tests__/__mocks__/tdesign-chat.mjs')
// 与 vite.config.ts 对齐：让 `@encv/shared-components`（含子路径）在测试运行时可解析。
// 否则 vite 不读 tsconfig 的 paths 映射，shared 子路径/裸包导入在测试里解析不到。
const SHARED_SRC = path.resolve(__dirname, '../packages/shared-components/src')

// ⚠️ 关键：vitest projects 模式下，根配置的 plugins【不会】自动继承到各 project。
// 每个 project 是独立的 Vite 配置，必须各自带 plugins，否则 `@/...` 无人解析
// （连本地 src 下的文件都 Failed to resolve）。因此抽成 BASE_PLUGINS，根配置
// 与各 project 都引用同一份。
// 2026-07-14 批 9：移除 shared 兜底分支（roots 仅留本地 src）。
// 此前 encv-alias-fallback 是 app 唯一 @/ 解析器，且「本地优先、shared 次之」；
// 批 9 已把全部 136 处落到 shared 的 @/x 改写为显式 @encv/shared-components/x
// （_measure-fallback.mjs 归零），故 shared 兜底已成死代码，摘除后 @/ 严格只解析本地。
const BASE_PLUGINS = [vue(), encvAliasFallback({ roots: [SRC_DIR] })]

// ═══════════════════════════════════════════════════════════════════
// 2026-07-02 性能优化：和 Go 对齐的"分层 + 守卫"模式
// ═══════════════════════════════════════════════════════════════════
//
// Go 的 test-go.sh 思路（本项目已验证成功）：
//   默认 = 单包 + short（1-2s）
//   全量 = ENCV_TEST_FULL=1（CI 专用，~几分钟）
//
// 对应到 vitest：
//   默认 vitest run          → FAST 子集（纯函数/纯逻辑，isolate:false 加速）
//   ENCV_TEST_FULL=1 vitest → 全部测试（含重型集成，isolate:true 保正确）
//   日常改完继续             → vitest run src/path/to/file.test.ts（2s 内）
//
// 性能对比（当前 1015 个测试）：
//   旧版（isolate:true + 全量）→ ~60s
//   FAST 子集（isolate:false + 40 个文件）→ ~8-12s
//   单文件 → ~1-2s
//   ⚠️ 95%+ 提升是"日常改完继续"场景（60s → 2s），不是全量
//
// 为什么不用单个 isolate:false：
//   本项目大量 composable 用模块级 reactive/ref（useTaskTrigger / usePluginExtensions
//   / useTheme / useConfig / useChatEngine / mockDataGenerator 等）
//   isolate:false 会导致 10+ 个测试文件互相污染，不可接受。
//
// 方案：用 projects 分层
//   - "fast" project：isolate:false + 纯函数/纯数据测试（无模块级状态副作用）
//   - "isolated" project：isolate:true + 有状态/集成测试（默认 exclude，FULL=1 才跑）
// ═══════════════════════════════════════════════════════════════════

const IS_FULL = process.env.ENCV_TEST_FULL === '1'

// ── FAST 子集：纯函数 / 纯数据 / 无模块级可变状态 ──
// 这些测试 isolate:false 也不会互相污染，是日常开发的主力
const FAST_INCLUDE = [
  // 纯数据解析/转换（无状态）
  'src/__tests__/appResult.test.ts',
  'src/__tests__/messageStatus.test.ts',
  'src/__tests__/tokenSnapshot.test.ts',
  'src/__tests__/renderTurnItems.test.ts',
  'src/__tests__/renderTurnItems.agentTask.test.ts',
  // composables: 纯函数式（无模块级 state）
  'src/composables/__tests__/parseContentDelta.test.ts',
  'src/composables/__tests__/parseToolResultData.test.ts',
  'src/composables/__tests__/relativeTime.test.ts',
  'src/composables/__tests__/useAGUIParser.test.ts',
  'src/composables/__tests__/useSearchInput.test.ts',
  'src/composables/__tests__/useSectionDerivation.test.ts',
  'src/composables/__tests__/useToolCallAccumulator.test.ts',
  '../packages/shared-components/src/composables/__tests__/workflow-core.test.ts',
  'src/composables/activeStatus.test.ts',
  'src/composables/appServerRealtimeReducer.test.ts',
  'src/composables/inlineFileReference.test.ts',
  'src/composables/reasoningEffort.test.ts',
  // lib: 纯数据生成/状态机
  'src/lib/workflow/__tests__/state-machine.test.ts',
  'src/lib/workflow/__tests__/unified-types.test.ts',
  // shared: 纯逻辑（SSE 解析 + composable 状态机）
  '../packages/shared-components/src/api/__tests__/mockGenerator.test.ts',
  '../packages/shared-components/src/composables/__tests__/useMockGenLog.test.ts',
  '../packages/shared-components/src/composables/__tests__/useDisclosure.test.ts',
  '../packages/shared-components/src/composables/__tests__/useClickOutside.test.ts',
  '../packages/shared-components/src/composables/__tests__/useModal.test.ts',
  '../packages/shared-components/src/lib/__tests__/taskEvent.test.ts',
  // utils: RingBuffer bench（纯算法）
  'src/utils/RingBuffer.bench.test.ts',
  // view 层纯逻辑（无模块级状态）
  'src/views/__tests__/useFilesView.searchTokens.test.ts',
  // theme: SCSS 编译期契约快照（纯 sass 编译，无模块级状态）
  'src/theme/__tests__/surface.test.ts',
  // theme: vivid.scss P3 孪生由 @function/@each 派生 + sourcemap 溯源（css-source 同源校验）
  'src/theme/__tests__/vividScss.test.ts',
  // theme: 运行时资源包 + themeLoader 指标/优化（happy-dom，resetThemeLoaderForTest 防污染）
  'src/theme/__tests__/official-themes.test.ts',
  // motion: 动效 ACL 闸门契约（guard 行为 + 设计令牌导出），happy-dom
  'src/motion/__tests__/motion-guard.test.ts',
  // motion: 滚动揭示 IntersectionObserver 修复（复现「Ionic 内滚整页空白」），happy-dom
  'src/motion/__tests__/scroll-reveal.test.ts',
  // motion: v-reveal 指令同根因修复（Ionic 内滚空白），happy-dom
  'src/motion/__tests__/directive-reveal.test.ts',
  // theme: 臻彩显示（vivid / P3）真实生效回归（先红后绿），happy-dom
  'src/motion/__tests__/vivid.test.ts',
  // theme: 表面材质模糊令牌 --material-blur 契约（先红后绿），happy-dom
  'src/theme/__tests__/surfaceMaterial.test.ts',
  // theme: per-theme 主色/背景色定制（2026-07-17 优化），happy-dom
  'src/theme/__tests__/themeCustomization.test.ts',
]

// ── ISOLATED：有模块级状态 / 用 vi.resetModules / 依赖 localStorage ──
// 默认不跑（FAST 子集不含这些），ENCV_TEST_FULL=1 才跑
const ISOLATED_INCLUDE = [
  'src/__tests__/source-extension-delegation.test.ts',
  'src/__tests__/usePluginExtensions.test.ts',
  'src/api/__tests__/getApiBaseUrl.test.ts',
  'src/api/encv.test.ts',
  'src/components/__tests__/TaskBasicInfo.test.ts',
  'src/components/__tests__/TaskTimeline.test.ts',
  'src/components/automation/__tests__/StepInlineTimeline.test.ts',
  'src/components/automation/__tests__/TreeView.test.ts',
  'src/components/developer/__tests__/MockGenLogCard.test.ts',
  'src/components/shared/__tests__/PhaseBadge.test.ts',
  'src/components/shared/__tests__/PhaseIcon.test.ts',
  'src/components/shared/__tests__/RelevanceBadge.test.ts',
  'src/components/shared/__tests__/UnifiedTimelineCard.test.ts',
  '../packages/shared-components/src/components/__tests__/TaskDebugPanel.test.ts',
  '../packages/shared-components/src/components/__tests__/TaskVirtualList.test.ts',
  'src/composables/__tests__/dev-start-guard.test.ts',
  'src/composables/__tests__/path-chain-e2e.test.ts',
  'src/composables/__tests__/realtime/HttpPollBackend.test.ts',
  'src/composables/__tests__/useApiBaseProbe.test.ts',
  'src/composables/__tests__/useChatEngine.test.ts',
  'src/composables/__tests__/useErrorAnalyzer.test.ts',
  'src/composables/__tests__/useFileList.test.ts',
  'src/composables/__tests__/useFileList.clientFilter.test.ts',
  'src/composables/__tests__/usePathResolver.test.ts',
  'src/composables/__tests__/usePinchZoom.test.ts',
  '../packages/shared-components/src/composables/__tests__/useProxiedFetch.test.ts',
  'src/composables/__tests__/useRealtimeTransport.test.ts',
  'src/composables/__tests__/useTaskTrigger.test.ts',
  'src/composables/__tests__/useTaskViewCompute.test.ts',
  'src/composables/__tests__/useTasksList.aggregation.test.ts',
  'src/composables/__tests__/useTasksList.automation-escape.test.ts',
  'src/composables/__tests__/useTasksList.dom.test.ts',
  'src/composables/__tests__/useTasksList.escape.test.ts',
  'src/composables/__tests__/useTasksList.escape-reverse.test.ts',
  'src/composables/__tests__/useTasksList.grouping.test.ts',
  'src/composables/__tests__/useTestCaseGeneration.test.ts',
  '../packages/shared-components/src/composables/__tests__/useVectorSearchStatus.test.ts',
  '../packages/shared-components/src/composables/__tests__/useWebDavWorkflowAdapter.test.ts',
  'src/composables/__tests__/useWorkflowStore.test.ts',
  'src/composables/__tests__/useWorkflowTaskService.test.ts',
  'src/composables/useAttachments.test.ts',
  'src/engines/__tests__/tdesignEngine.test.ts',
  'src/engines/__tests__/TDesignChatView.test.ts',
  'src/lib/__tests__/mockDataGenerator.test.ts',
  'src/lib/workflow/__tests__/buildDynamicWorkflow.pre-population.test.ts',
  'src/lib/workflow/__tests__/buildDynamicWorkflow.real-e2e.test.ts',
  'src/views/__tests__/AgentChat.history.test.ts',
]

// 公共基础配置（所有 project 共享）
function sharedTestConfig() {
  return {
    environment: 'happy-dom',
    globals: true,
    threads: true,
    fileParallelism: true,
    testTimeout: 60_000,
    passWithNoTests: false,
    slowTestThreshold: 30,
    reporters: ['default'],
    bail: 0,
    alias: {
      // ⚠️ 故意不设置 '@' 别名：所有 `@/...` 统一由 encvAliasFallback 插件解析
      // （本地 src 优先、shared 次之）。若这里设 '@': SRC_DIR，Vite 会把
      // `@/composables/useToast` 先解析成本地绝对路径（已删除→不存在），
      // 导致插件来不及回退 shared 就报 "Failed to resolve"。这与 vite.config.ts
      // （无 '@' 别名、仅靠插件）保持一致。
      '@tdesign-vue-next/chat': TDESIGN_STUB,
      '@encv/shared-components': SHARED_SRC,
      '@encv/shared-components/': SHARED_SRC + '/',
    },
    exclude: [
      '**/node_modules/**',
      '**/dist/**',
      '**/cypress/**',
      '**/.{idea,git,cache,output,temp}/**',
      // 复杂 component test → cypress.component
      'src/views/__tests__/**/*.component.test.ts',
      'src/components/agent/**',
      'src/engines/__tests__/**',
      // 旧的根目录集成测试 → cypress
      '__tests__/ApprovalCard.test.ts',
      '__tests__/DevLogs.autoScroll.test.ts',
      '__tests__/FilePickerModal.test.ts',
      '__tests__/MessageVirtualList.test.ts',
      '__tests__/useAgent.test.ts',
      '__tests__/useAgentApiBase.test.ts',
      '__tests__/useNewTaskModal.test.ts',
      '__tests__/useTaskForm.test.ts',
      '__tests__/files.logic.test.ts',
      '__tests__/tasks-regression.test.ts',
      '__tests__/api.mock.test.ts',
    ],
  }
}

export default defineConfig({
  // ⚠️ 根配置也带 BASE_PLUGINS（覆盖单文件 `vitest run xxx.test.ts` 走默认 project 的场景）。
  // 各 project（fast/isolated）在下方各自也引用 BASE_PLUGINS，否则 `@/...` 无人解析。
  plugins: BASE_PLUGINS,
  root: __dirname,
  cacheDir: 'node_modules/.vite',
  resolve: {
    alias: {
      // ⚠️ 故意不设置 '@' 别名：所有 `@/...` 统一由 encvAliasFallback 插件解析
      // （本地 src 优先、shared 次之）。若这里设 '@': SRC_DIR，Vite 会把
      // `@/composables/useToast` 先解析成本地绝对路径（已删除→不存在），
      // 导致插件来不及回退 shared 就报 "Failed to resolve"。这与 vite.config.ts
      // （无 '@' 别名、仅靠插件）保持一致。
      '@tdesign-vue-next/chat': TDESIGN_STUB,
      '@encv/shared-components': SHARED_SRC,
      '@encv/shared-components/': SHARED_SRC + '/',
    },
  },
  test: {
    // ⚠️ vitest 4 的 projects 模式：顶层 test.* 只是默认值，每个 project 可覆盖
    // 为了清晰，我们把所有配置都放到 project 级别，顶层只留 name
    name: 'default',
    // 默认 project 配置（单文件 vitest run xxx 时走这个）
    ...sharedTestConfig(),
    isolate: true,
    // 默认 include = FAST + ISOLATED（兼容单文件指定路径）
    // 但 FAST project 会先跑（isolate:false 更快），ISOLATED 只在 FULL=1 跑
    include: IS_FULL
      ? [...FAST_INCLUDE, ...ISOLATED_INCLUDE]
      : FAST_INCLUDE,

    // ── Projects：分层测试（和 Go test-go.sh 对齐）──
    projects: IS_FULL
      ? [
          // Project 1: FAST（isolate:false，~30 个文件，~8-12s）
          {
            plugins: BASE_PLUGINS,
            test: {
              name: 'fast',
              ...sharedTestConfig(),
              isolate: false,
              include: FAST_INCLUDE,
            },
          },
          // Project 2: ISOLATED（isolate:true，~35 个文件，~40-50s）
          {
            plugins: BASE_PLUGINS,
            test: {
              name: 'isolated',
              ...sharedTestConfig(),
              isolate: true,
              include: ISOLATED_INCLUDE,
            },
          },
        ]
      : [
          // 默认只有 FAST project（日常开发用）
          {
            plugins: BASE_PLUGINS,
            test: {
              name: 'fast',
              ...sharedTestConfig(),
              isolate: false,
              include: FAST_INCLUDE,
            },
          },
        ],
  },
})
