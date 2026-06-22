/**
 * 真机"任务逃逸"诊断 — 完整自动化插件测试入口 + 真实 HTML UI 渲染
 *
 * 设计目标：
 * 1. 用真机 7 个 plugin 数据（从后端 registry.go 真值构造）
 * 2. 跑真实 buildDynamicWorkflow 逻辑（cartesianExpand + plugin.taskOptions.extraFields 派生）
 * 3. mock 后端 createTask API + WS 推送
 * 4. mount 真实 TaskListDiag 组件（ion-card 风格 HTML 模拟 group card）
 * 5. 输出 wrapper.html() → 写文件 → console.log
 * 6. 多个状态对比：注入后 / fetchTasks 丢 runId 后 / 重启后
 *
 * 关键诊断：自动化测试入口 1 次 submitRun 应产生 1 个 run (group)，
 *          但真机 1+1+365 拆 3 个 group — 意味着逃逸是渲染层面 / 状态累积导致。
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import * as fs from 'fs'
import * as path from 'path'

// ==================== Setup mocks ====================
const testStorage = new Map<string, string>()
const mockLocalStorage: Storage = {
  get length() { return testStorage.size },
  key: (index: number) => Array.from(testStorage.keys())[index] ?? null,
  getItem: (key: string) => testStorage.get(key) ?? null,
  setItem: (key: string, value: string) => { testStorage.set(key, value) },
  removeItem: (key: string) => { testStorage.delete(key) },
  clear: () => { testStorage.clear() },
} as unknown as Storage
vi.stubGlobal('localStorage', mockLocalStorage)

import { setActivePinia, createPinia } from 'pinia'
import { useTaskStore } from '@/stores/taskStore'
import { useTasksList, _resetTasksListSingletonForTests } from '@/composables/useTasksList'
import { extToRelativePath } from '@/lib/mockDataGenerator'
import type { EncvTask, PluginMeta, TaskStatus } from '@/api/encv'

vi.mock('@/composables/useTaskEventBridge', () => ({
  useTaskEventBridge: () => {},
}))
vi.mock('@/lib/taskPersistence', () => ({
  loadAllTasks: vi.fn().mockResolvedValue([]),
  bulkPutTasks: vi.fn().mockResolvedValue(undefined),
  putTask: vi.fn().mockResolvedValue(undefined),
  deleteTask: vi.fn().mockResolvedValue(undefined),
  clearPutThrottle: vi.fn(),
  ensureLRUCache: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/composables/useWorkflowTaskService', () => ({
  useWorkflowTaskService: () => ({
    currentRun: { value: null },
    isRunning: { value: false },
    currentDef: { value: null },
    submitRun: vi.fn().mockResolvedValue({ id: 'mock-run', jobs: [] }),
    cancelRun: vi.fn(),
    listRuns: () => [],
    getRun: () => undefined,
  }),
  __resetServiceForTests: () => {},
}))
vi.mock('@/composables/useI18n', () => ({
  useI18n: () => ({ t: (_k: string, opts?: any) => opts?.defaultValue ?? _k }),
}))
vi.mock('@/composables/useDateFormat', () => ({
  formatDateTime: (d: string) => d,
}))

// ==================== 真机 7 个 plugin 数据（从后端 registry.go 真值构造） ====================
function buildRealPlugins(): PluginMeta[] {
  const SUPPORTED_VERSIONS = [3, 4]
  const DEFAULT_VERSION = 4
  const CONTAINER_EXT: Record<string, string> = {
    video: '.encv',
    audio: '.enca',
    image: '.enci',
    wps: '.encw',
    pdf: '.encp',
    text: '.enct',
    alistencrypt: '.ae',
  }
  return [
    {
      name: 'video',
      supportedExtensions: ['mp4', 'mkv', 'avi', 'mov', 'rmvb', 'webm', 'flv', 'm3u8'],
      supportedMimePrefixes: ['video/'],
      containerExtension: CONTAINER_EXT.video,
      taskOptions: {
        passwordStrategy: 'global',
        supportVersionSelect: true,
        supportedVersions: SUPPORTED_VERSIONS,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [
          { key: 'stream_preset', label: 'streamPreset', type: 'select', required: false, defaultValue: 'balanced',
            options: ['balanced', 'quality', 'high_quality'], optionLabels: { balanced: 'Balanced', quality: 'Quality', high_quality: 'High Quality' }, condition: 'encrypt' },
          { key: 'encrypt_filename', label: 'encryptFilename', type: 'bool', required: false, defaultValue: 'false', condition: 'encrypt' },
          { key: 'fn_rounds', label: 'fnRounds', type: 'select', required: false, defaultValue: '8',
            options: ['4', '8', '12', '16'], optionLabels: { '4': '4', '8': '8 (Recommended)', '12': '12', '16': '16' }, condition: 'encrypt' },
          { key: 'fn_charset', label: 'fnCharset', type: 'select', required: false, defaultValue: 'alphanumeric',
            options: ['alphanumeric', 'hex'], optionLabels: { alphanumeric: 'Alphanumeric', hex: 'Hex' }, condition: 'encrypt' },
        ],
      },
    },
    {
      name: 'audio',
      supportedExtensions: ['mp3', 'flac', 'ogg', 'm4a', 'wav', 'aac', 'opus'],
      supportedMimePrefixes: ['audio/'],
      containerExtension: CONTAINER_EXT.audio,
      taskOptions: {
        passwordStrategy: 'global',
        supportVersionSelect: true,
        supportedVersions: SUPPORTED_VERSIONS,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [
          { key: 'fn_charset', label: 'fnCharset', type: 'select', required: false, defaultValue: 'alphanumeric',
            options: ['alphanumeric', 'hex'], optionLabels: { alphanumeric: 'Alphanumeric', hex: 'Hex' }, condition: 'encrypt' },
        ],
      },
    },
    {
      name: 'image',
      supportedExtensions: ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'tiff'],
      supportedMimePrefixes: ['image/'],
      containerExtension: CONTAINER_EXT.image,
      taskOptions: {
        passwordStrategy: 'global',
        supportVersionSelect: true,
        supportedVersions: SUPPORTED_VERSIONS,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [
          { key: 'fn_charset', label: 'fnCharset', type: 'select', required: false, defaultValue: 'alphanumeric',
            options: ['alphanumeric', 'hex'], optionLabels: { alphanumeric: 'Alphanumeric', hex: 'Hex' }, condition: 'encrypt' },
        ],
      },
    },
    {
      name: 'wps',
      supportedExtensions: ['doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx'],
      supportedMimePrefixes: ['application/vnd.ms-', 'application/vnd.openxmlformats-'],
      containerExtension: CONTAINER_EXT.wps,
      taskOptions: {
        passwordStrategy: 'global',
        supportVersionSelect: false,
        supportedVersions: null,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [],
      },
    },
    {
      name: 'pdf',
      supportedExtensions: ['pdf'],
      supportedMimePrefixes: ['application/pdf'],
      containerExtension: CONTAINER_EXT.pdf,
      taskOptions: {
        passwordStrategy: 'global',
        supportVersionSelect: true,
        supportedVersions: SUPPORTED_VERSIONS,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [],
      },
    },
    {
      name: 'text',
      supportedExtensions: ['txt', 'md', 'rtf', 'log'],
      supportedMimePrefixes: ['text/'],
      containerExtension: CONTAINER_EXT.text,
      taskOptions: {
        passwordStrategy: 'global',
        supportVersionSelect: true,
        supportedVersions: SUPPORTED_VERSIONS,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [],
      },
    },
    {
      name: 'alistencrypt',
      supportedExtensions: [], // 故意空（"处理所有文件"语义）
      supportedMimePrefixes: [],
      containerExtension: CONTAINER_EXT.alistencrypt,
      taskOptions: {
        passwordStrategy: 'global',
        supportVersionSelect: false,
        supportedVersions: null,
        defaultVersion: DEFAULT_VERSION,
        extraFields: [],
      },
    },
  ] as PluginMeta[]
}

function categoryForExt(ext: string): string {
  const e = ext.toLowerCase().replace(/^\./, '')
  if (['mp4', 'mkv', 'avi', 'mov', 'webm', 'flv', 'wmv'].includes(e)) return 'video'
  if (['mp3', 'flac', 'ogg', 'm4a', 'wav', 'aac', 'opus'].includes(e)) return 'audio'
  if (['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'tiff'].includes(e)) return 'image'
  if (['pdf'].includes(e)) return 'pdf'
  if (['doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx'].includes(e)) return 'wps'
  if (['txt', 'md', 'rtf', 'log'].includes(e)) return 'text'
  if (['encv', 'ae'].includes(e)) return 'alist-encrypted'
  return 'misc'
}

function cartesianExpand(arrays: string[][]): string[][] {
  if (arrays.length === 0) return [[]]
  if (arrays.some((a) => a.length === 0)) return [[]]
  return arrays.reduce<string[][]>(
    (acc, curr) => acc.flatMap((a) => curr.map((c) => [...a, c])),
    [[]],
  )
}

/**
 * 真实 buildDynamicWorkflow 核心逻辑（从 PluginTestsDetail.vue 移植）
 * 输入 plugins → 输出 step 列表
 */
function buildDynamicSteps(plugins: PluginMeta[], mockRoot: string): any[] {
  const steps: any[] = []
  for (const plugin of plugins) {
    const opts = plugin.taskOptions
    if (!opts) continue
    const supportedExts = plugin.supportedExtensions ?? []
    if (supportedExts.length === 0) continue
    const sourceExt = supportedExts[0]
    const specRelPath = extToRelativePath(sourceExt)
    const sourcePath = specRelPath
      ? `${mockRoot}${specRelPath}`
      : `${mockRoot}01-plain-media/${categoryForExt(sourceExt)}/sample.${sourceExt}`

    const versions: number[] = opts.supportVersionSelect && opts.supportedVersions
      ? opts.supportedVersions
      : [opts.defaultVersion]

    const selectFields: { field: any; values: string[] }[] = []
    const boolFields: { field: any }[] = []
    for (const f of opts.extraFields ?? []) {
      if (f.type === 'select' && Array.isArray(f.options) && f.options.length > 1) {
        selectFields.push({ field: f, values: f.options })
      } else if (f.type === 'bool') {
        boolFields.push({ field: f })
      }
    }

    for (const version of versions) {
      const encryptSelectFields = selectFields.filter((sf) => !sf.field.condition || sf.field.condition === 'encrypt')
      const encryptBoolFields = boolFields.filter((bf) => !bf.field.condition || bf.field.condition === 'encrypt')
      const encryptSelectCombos = cartesianExpand(encryptSelectFields.map((sf) => sf.values))
      const encryptBoolCombos: boolean[][] = []
      if (encryptBoolFields.length === 0) {
        encryptBoolCombos.push([])
      } else {
        const n = encryptBoolFields.length
        for (let mask = 0; mask < 1 << n; mask++) {
          encryptBoolCombos.push(Array.from({ length: n }, (_, i) => Boolean(mask & (1 << i))))
        }
      }

      for (const selectCombo of encryptSelectCombos) {
        for (const boolCombo of encryptBoolCombos) {
          const extraFields: Record<string, string> = {}
          encryptSelectFields.forEach((sf, i) => {
            const val = selectCombo[i]
            if (val !== undefined) extraFields[sf.field.key] = val
          })
          encryptBoolFields.forEach((bf, i) => {
            extraFields[bf.field.key] = boolCombo[i] ? 'true' : 'false'
          })
          steps.push({
            id: `enc_${plugin.name}_v${version}_${sourceExt}_${JSON.stringify(extraFields)}`,
            pluginName: plugin.name,
            taskType: 'encrypt',
            sourcePath,
            sourceExt,
            version,
            extraFields,
          })
        }
      }
    }
  }
  return steps
}

// ==================== 真实 group card UI 组件（ion-card 风格） ====================
const TaskListDiag = defineComponent({
  name: 'TaskListDiag',
  setup() {
    const list = useTasksList()
    return { list, store: useTaskStore() }
  },
  template: `
    <div class="task-list-diag">
      <header class="diag-header">
        <h1>自动化测试任务列表</h1>
        <div class="diag-stats" data-testid="stats">
          <span class="stat"><b data-testid="stat-store">{{ store.tasks.length }}</b> task</span>
          <span class="stat"><b data-testid="stat-groups">{{ list.groupedTasksByRunId.value.length }}</b> group card</span>
          <span class="stat"><b data-testid="stat-items">{{ list.displayedItems.value.length }}</b> items</span>
        </div>
      </header>
      <ul class="task-list">
        <template v-for="it in list.displayedItems.value" :key="it.key">
          <li v-if="it.kind === 'date'" class="date-header" :data-testid="'date-' + it.label">
            <span class="date-label">{{ it.label }}</span>
          </li>
          <li
            v-else-if="it.kind === 'group'"
            class="group-card"
            :data-testid="'group-' + it.runId"
            :data-run-id="it.runId"
          >
            <div class="group-header">
              <div class="group-icon">📦</div>
              <div class="group-info">
                <h3 class="group-title">自动化 · {{ it.tasks.length }} 个任务</h3>
                <div class="group-meta">
                  <span class="badge badge-run">runId: <code>{{ it.runId }}</code></span>
                  <span v-if="it.tasks[0]?.triggeredBy" class="badge badge-trigger">{{ it.tasks[0].triggeredBy }}</span>
                </div>
              </div>
            </div>
            <div class="group-tasks" :data-testid="'tasks-' + it.runId">
              <span v-for="t in it.tasks.slice(0, 3)" :key="t.id" class="task-chip">{{ t.id }}</span>
              <span v-if="it.tasks.length > 3" class="task-chip-more">+{{ it.tasks.length - 3 }} more</span>
            </div>
          </li>
          <li v-else-if="it.kind === 'task'" class="task-row" :data-testid="'row-' + it.task.id">
            <span class="task-id">{{ it.task.id }}</span>
            <span class="task-runid">runId: {{ it.task.runId || '⚠️ UNDEFINED' }}</span>
            <span v-if="it.task.triggeredBy" class="task-trigger">{{ it.task.triggeredBy }}</span>
          </li>
        </template>
      </ul>
    </div>
  `,
})

// ==================== 诊断输出 ====================
function logSection(title: string): void {
  // eslint-disable-next-line no-console
  console.log(`\n${'='.repeat(80)}\n[UI-DIAG] ${title}\n${'='.repeat(80)}`)
}

function saveHtml(wrapper: VueWrapper, filename: string, label: string): void {
  const html = wrapper.html()
  const outDir = '/tmp/escape-diag'
  if (!fs.existsSync(outDir)) fs.mkdirSync(outDir, { recursive: true })
  const filepath = path.join(outDir, filename)
  fs.writeFileSync(filepath, html, 'utf-8')
  // eslint-disable-next-line no-console
  console.log(`[UI-DIAG] ${label}: HTML written to ${filepath} (${html.length} bytes)`)
  // eslint-disable-next-line no-console
  console.log(`[UI-DIAG] ${label}: stats HTML:`)
  const statsHtml = wrapper.find('[data-testid="stats"]').html()
  // eslint-disable-next-line no-console
  console.log(`  ${statsHtml.replace(/\s+/g, ' ').trim()}`)
  const groupCards = wrapper.findAll('[data-testid^="group-"]')
  // eslint-disable-next-line no-console
  console.log(`[UI-DIAG] ${label}: DOM group card count=${groupCards.length}`)
  for (const card of groupCards) {
    const runId = card.attributes('data-run-id') ?? '?'
    const titleEl = card.find('.group-title')
    const triggerEl = card.find('.badge-trigger')
    // eslint-disable-next-line no-console
    console.log(`  - [group] runId=${runId} title="${titleEl.text()}" trigger="${triggerEl.exists() ? triggerEl.text() : 'n/a'}"`)
  }
  const orphanRows = wrapper.findAll('[class="task-row"]')
  // eslint-disable-next-line no-console
  console.log(`[UI-DIAG] ${label}: DOM orphan task row count=${orphanRows.length}`)
}

// ==================== 测试 ====================
describe('真机"自动化测试入口"任务逃逸诊断', () => {
  let store: ReturnType<typeof useTaskStore>
  let wrapper: VueWrapper

  beforeEach(() => {
    testStorage.clear()
    setActivePinia(createPinia())
    store = useTaskStore()
    store.$reset()
  })

  afterEach(() => {
    wrapper?.unmount()
    _resetTasksListSingletonForTests()
    testStorage.clear()
  })

  // ==================== E1: 1 次 submitRun 派生 N 个 case → 1 个 run（期望 1 个 group） ====================
  it('E1: 真机 7 个 plugin 派生 case 数量诊断', async () => {
    logSection('E1: 真机 plugin 数据 + buildDynamicWorkflow 派生 case 数量')

    const plugins = buildRealPlugins()
    // eslint-disable-next-line no-console
    console.log(`[UI-DIAG] E1: 真机 plugins = ${plugins.map((p) => p.name).join(', ')} (共 ${plugins.length} 个)`)

    const mockRoot = '/storage/emulated/0/encv-automation/'
    const steps = buildDynamicSteps(plugins, mockRoot)

    // 按 plugin 分组统计
    const byPlugin: Record<string, number> = {}
    for (const s of steps) byPlugin[s.pluginName] = (byPlugin[s.pluginName] ?? 0) + 1
    // eslint-disable-next-line no-console
    console.log(`[UI-DIAG] E1: 共派生 ${steps.length} 个 encrypt step`)
    for (const [k, n] of Object.entries(byPlugin)) {
      // eslint-disable-next-line no-console
      console.log(`  - ${k}: ${n} case`)
    }
    // eslint-disable-next-line no-console
    console.log(`[UI-DIAG] E1: 1 次 submitRun → ${steps.length} 个 step（同一个 run.id）`)
    // eslint-disable-next-line no-console
    console.log(`[UI-DIAG] E1: 期望 → DOM 1 个 group card（${steps.length} 个 task）`)

    expect(steps.length).toBeGreaterThan(0)
  })

  // ==================== E2: 1 次 submitRun 跑完 → DOM 1 个 group ====================
  it('E2: 1 次 submitRun 跑完 N 个 step → 1 个 group card（基线）', async () => {
    logSection('E2: 1 次 submitRun 跑完 → DOM 1 个 group card')

    const plugins = buildRealPlugins()
    const mockRoot = '/storage/emulated/0/encv-automation/'
    const steps = buildDynamicSteps(plugins, mockRoot)

    // 模拟 submitRun：用固定的 RUN_ID 提交所有 step
    const RUN_ID = 'auto-test-run-1'
    let idx = 0
    for (const s of steps) {
      const task: EncvTask = {
        id: `task-${idx++}`,
        type: 'encrypt' as const,
        sourcePath: s.sourcePath,
        targetPath: `/tmp/out/${RUN_ID}/${idx}.enc`,
        status: 'running' as TaskStatus,
        progress: 0,
        createdAt: new Date(Date.now() - 60000).toISOString(),
        runId: RUN_ID,
        triggeredBy: 'automation' as const,
        pluginName: s.pluginName,
      }
      // 模拟 WS task:created → store.appendTask
      store.appendTask(task)
    }

    await nextTick()
    wrapper = mount(TaskListDiag)
    await nextTick()
    saveHtml(wrapper, 'E2-after-submitRun.html', 'E2: submitRun 后')

    // eslint-disable-next-line no-console
    console.log(`[UI-DIAG] E2 关键诊断：${steps.length} 个 step 共享 1 个 runId → DOM group count=${wrapper.findAll('[data-testid^="group-"]').length}`)
    expect(wrapper.findAll('[data-testid^="group-"]').length).toBe(1)
  })

  // ==================== E3: 多次 submitRun 拆 3 个 group（真机 1+1+365 场景） ====================
  it('E3: 真机"1+1+365 拆 3 个 group" — 3 次 submitRun（每个 1+1+365 个 step）', async () => {
    logSection('E3: 3 次 submitRun 拆 3 个 group（真机 1+1+365 场景）')

    const plugins = buildRealPlugins()
    const mockRoot = '/storage/emulated/0/encv-automation/'
    const allSteps = buildDynamicSteps(plugins, mockRoot)
    // eslint-disable-next-line no-console
    console.log(`[UI-DIAG] E3: ${allSteps.length} 个 step 来自 7 个 plugin`)

    // 模拟真机：3 次 submitRun
    const RUNS = [
      { id: 'auto-test-run-1', stepCount: 1 },
      { id: 'auto-test-run-2', stepCount: 1 },
      { id: 'auto-test-run-3', stepCount: 365 },
    ]

    let idx = 0
    for (const run of RUNS) {
      // eslint-disable-next-line no-console
      console.log(`[UI-DIAG] E3: 提交 ${run.id}（${run.stepCount} step）`)
      for (let i = 0; i < run.stepCount; i++) {
        const s = allSteps[idx % allSteps.length]
        const task: EncvTask = {
          id: `task-${idx++}`,
          type: 'encrypt',
          sourcePath: s.sourcePath,
          targetPath: `/tmp/out/${run.id}/${i}.enc`,
          status: 'running',
          progress: 0,
          createdAt: new Date(Date.now() - 60000).toISOString(),
          runId: run.id,
          triggeredBy: 'automation',
          pluginName: s.pluginName,
        }
        store.appendTask(task)
      }
    }
    await nextTick()

    wrapper = mount(TaskListDiag)
    await nextTick()
    saveHtml(wrapper, 'E3-three-runs.html', 'E3: 3 次 submitRun 后')

    const groupCount = wrapper.findAll('[data-testid^="group-"]').length
    // eslint-disable-next-line no-console
    console.log(`[UI-DIAG] E3 关键诊断：3 次 submitRun → DOM group count=${groupCount}`)
    // eslint-disable-next-line no-console
    console.log(`[UI-DIAG] E3 → 期望：3 个 group（1+1+365 = 367 task 拆 3 个 run）`)
    // eslint-disable-next-line no-console
    console.log(`[UI-DIAG] E3 → user 反馈"逃逸"：1+1+365 拆 3 个 group（不是 1 个）= 多次 submitRun 累积？`)
  })

  // ==================== E4: "重启后逃逸消失" — fetchTasks 丢 runId 后 DOM 实际渲染 ====================
  it('E4: 1 次 submitRun 跑完 + fetchTasks 模拟丢 runId + 重启', async () => {
    logSection('E4: 重启后逃逸消失 — 完整 e2e')

    const plugins = buildRealPlugins()
    const mockRoot = '/storage/emulated/0/encv-automation/'
    const steps = buildDynamicSteps(plugins, mockRoot)
    const RUN_ID = 'auto-test-run-1'

    // === session 1: submitRun + WS task:created ===
    let idx = 0
    const initialTasks: EncvTask[] = []
    for (const s of steps) {
      const task: EncvTask = {
        id: `task-${idx++}`,
        type: 'encrypt',
        sourcePath: s.sourcePath,
        targetPath: `/tmp/out/${RUN_ID}/${idx}.enc`,
        status: 'running',
        progress: 0,
        createdAt: new Date().toISOString(),
        runId: RUN_ID,
        triggeredBy: 'automation',
        pluginName: s.pluginName,
      }
      initialTasks.push(task)
      store.appendTask(task)
    }
    await nextTick()
    wrapper = mount(TaskListDiag)
    await nextTick()
    saveHtml(wrapper, 'E4-session1-after-submitRun.html', 'E4-1: session 1 submitRun 后')
    const session1GroupCount = wrapper.findAll('[data-testid^="group-"]').length
    // eslint-disable-next-line no-console
    console.log(`[UI-DIAG] E4-1: ${steps.length} step 提交后 DOM group count=${session1GroupCount}`)

    // === session 1: 模拟 fetchTasks 丢 runId（10% task runId 变 undefined） ===
    // 假设：后端 SQLite 早期 row 缺 run_id → Go omitempty 省略 → 前端 task.runId = undefined
    const fetchTasksData: EncvTask[] = initialTasks.map((t) => ({
      ...t,
      ...(Math.random() < 0.1 ? { runId: undefined } : {}),
    })) as EncvTask[]
    store.bulkSetTasks(fetchTasksData)
    await nextTick()
    saveHtml(wrapper, 'E4-session1-after-fetchTasks-drop-runId.html', 'E4-2: session 1 fetchTasks 丢 10% runId 后')

    const session1FinalGroupCount = wrapper.findAll('[data-testid^="group-"]').length
    const session1OrphanRowCount = wrapper.findAll('[class="task-row"]').length
    // eslint-disable-next-line no-console
    console.log(`[UI-DIAG] E4-2 真因：fetchTasks 丢 10% runId → DOM group count=${session1FinalGroupCount}, orphan row count=${session1OrphanRowCount}`)
    // eslint-disable-next-line no-console
    console.log(`[UI-DIAG] E4-2 关键诊断：每个孤儿 task 在 groupedTasksByRunId 兜底成 __manual__\${id} 伪 group → 视觉"逃逸"`)

    wrapper.unmount()
    _resetTasksListSingletonForTests()

    // === session 2: 模拟"重启" → new Pinia + hydrate 干净数据 ===
    setActivePinia(createPinia())
    const store2 = useTaskStore()
    store2.$reset()
    const hydrateData: EncvTask[] = initialTasks.map((t) => ({ ...t, runId: RUN_ID }))
    store2.bulkSetTasks(hydrateData)
    const wrapper2 = mount(TaskListDiag)
    await nextTick()
    saveHtml(wrapper2, 'E4-session2-after-restart.html', 'E4-3: session 2 重启 + hydrate 后')

    const session2GroupCount = wrapper2.findAll('[data-testid^="group-"]').length
    // eslint-disable-next-line no-console
    console.log(`\n[UI-DIAG] E4 关键诊断结论：`)
    // eslint-disable-next-line no-console
    console.log(`  - session 1 fetchTasks 丢 runId: DOM group count=${session1FinalGroupCount}（逃逸）`)
    // eslint-disable-next-line no-console
    console.log(`  - session 2 重启 + hydrate:     DOM group count=${session2GroupCount}（不逃逸）`)
    // eslint-disable-next-line no-console
    console.log(`  - 修复方向：前端 bulkSetTasks 改成 merge 模式（保留 store 里已有 task 的 runId）`)

    wrapper2.unmount()
  })
})
