/**
 * Cypress 业务测试基础设施 — Task 领域统一 helper
 *
 * 设计目标：
 *   - 消除每个 spec 重复的 mock 数据生成代码
 *   - 统一 store 注入 / 滚动模拟 / DOM 断言模式
 *   - 支持"真实业务量级"测试（1000+ task）
 *   - Cypress 友好：所有操作都是 thenable，可链式调用
 *
 * 用法：
 *   import { taskFixtures, taskStore, taskDom } from '../support/task-test-helpers'
 *
 *   const tasks = taskFixtures.generateBatch(1000, { runId: 'run-1', triggeredBy: 'automation' })
 *   taskStore.injectTasks(tasks)
 *   taskDom.scrollToBottom()
 *   taskDom.assertVisibleTaskCount(20)
 */

import { _getTaskStore } from './store-helpers'
import type { EncvTask, TaskStatus, TaskType } from '../../src/api/encv'

// ============================================================================
// 1. Fixture 生成 — 生成真实感的任务数据
// ============================================================================

export interface GenerateBatchOptions {
  /** 所有 task 共享的 runId（不传则每个 task 独立 runId） */
  runId?: string
  /** triggeredBy 类型 */
  triggeredBy?: 'user' | 'automation' | 'ai_agent'
  /** 起始序号（默认 0） */
  startIndex?: number
  /** 状态分布（默认全部 queued） */
  statusMix?: Partial<Record<TaskStatus, number>>
  /** plugin 名称池 */
  plugins?: string[]
  /** 文件类型 */
  fileTypes?: string[]
  /** 基础时间（ISO 字符串） */
  baseTime?: string
}

function generateTask(
  index: number,
  opts: GenerateBatchOptions = {},
): EncvTask {
  const {
    runId,
    triggeredBy = 'user',
    startIndex = 0,
    plugins = ['mp4-encrypt', 'mkv-encrypt', 'png-encrypt', 'pdf-encrypt'],
    fileTypes = ['mp4', 'mkv', 'png', 'jpg', 'pdf'],
    baseTime = '2026-06-23T10:00:00.000Z',
  } = opts

  const i = startIndex + index
  const plugin = plugins[i % plugins.length]
  const ext = fileTypes[i % fileTypes.length]
  const taskRunId = runId || `run-${i}`
  const createdTs = new Date(baseTime).getTime() + i * 1000 // 每个 task 间隔 1s

  // 状态分布：默认全部 queued
  let status: TaskStatus = 'queued'
  let progress = 0
  let completedAt: string | undefined = undefined

  if (opts.statusMix) {
    const total = Object.values(opts.statusMix).reduce((a, b) => a + (b || 0), 0)
    let acc = 0
    const idx = i % total
    for (const [s, count] of Object.entries(opts.statusMix) as [TaskStatus, number][]) {
      acc += count || 0
      if (idx < acc) {
        status = s
        break
      }
    }
  }

  if (status === 'running') {
    progress = (i % 99) + 1
  } else if (status === 'completed') {
    progress = 100
    completedAt = new Date(createdTs + 30000 + i * 100).toISOString()
  } else if (status === 'failed') {
    progress = Math.floor((i % 50) + 10)
    completedAt = new Date(createdTs + 15000 + i * 100).toISOString()
  }

  return {
    id: `task-${String(i).padStart(5, '0')}`,
    type: 'encrypt' as TaskType,
    sourcePath: `/mock/media/sample-${String(i).padStart(4, '0')}.${ext}`,
    targetPath: `/mock/encrypted/sample-${String(i).padStart(4, '0')}.encv`,
    pluginName: plugin,
    status,
    progress,
    phase: status === 'running' ? 'encrypting' : undefined,
    speed: status === 'running' ? `${(i % 10) + 1}.${i % 10} MB/s` : undefined,
    eta: status === 'running' ? `${(i % 60) + 1}s` : undefined,
    error: status === 'failed' ? 'encryption failed' : undefined,
    errorDetail: status === 'failed' ? 'detail error message' : undefined,
    createdAt: new Date(createdTs).toISOString(),
    completedAt,
    triggeredBy,
    runId: taskRunId,
    cipherMode: i % 2,
    compressionMode: i % 2 === 0 ? 'none' : 'zstd',
    extraFields: {},
  }
}

export const taskFixtures = {
  /** 生成单个 task（可直接指定 status 等简单字段） */
  one(index: number, opts?: GenerateBatchOptions & { status?: TaskStatus }): EncvTask {
    if (opts?.status) {
      return generateTask(index, { ...opts, statusMix: { [opts.status]: 1 } })
    }
    return generateTask(index, opts)
  },

  /** 生成一批任务 */
  generateBatch(count: number, opts: GenerateBatchOptions = {}): EncvTask[] {
    return Array.from({ length: count }, (_, i) => generateTask(i, opts))
  },

  /** 生成混合状态的一批任务（模拟真实场景） */
  generateMixedBatch(count: number, opts: GenerateBatchOptions = {}): EncvTask[] {
    return this.generateBatch(count, {
      ...opts,
      statusMix: {
        queued: Math.floor(count * 0.3),
        running: Math.floor(count * 0.2),
        completed: Math.floor(count * 0.35),
        failed: Math.floor(count * 0.1),
        cancelled: Math.floor(count * 0.05),
      },
    })
  },
}

// ============================================================================
// 2. Store 操作 — 注入数据 / 读取状态 / 模拟事件
// ============================================================================

export const taskStore = {
  /** 获取 task store 实例 */
  get() {
    return _getTaskStore()
  },

  /** 注入一批任务到 store（替换现有） */
  injectTasks(tasks: EncvTask[]): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      store.bulkSetTasks(tasks)
    })
  },

  /** 追加一批任务（保留现有） */
  appendTasks(tasks: EncvTask[]): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      const existing = store.tasks
      store.bulkSetTasks([...existing, ...tasks])
    })
  },

  /** 设置 viewMode */
  setViewMode(mode: 'group' | 'flat'): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      store.viewMode = mode
    })
  },

  /** 设置 sortBy */
  setSortBy(sortBy: 'activity' | 'created' | 'name'): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      store.sortBy = sortBy
    })
  },

  /** 设置搜索关键词 */
  setSearchQuery(q: string): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      store.setSearchQuery(q)
    })
  },

  /** 获取搜索关键词 */
  getSearchQuery(): Cypress.Chainable<string> {
    return cy.then(() => {
      const store = _getTaskStore()
      return store.searchQuery
    })
  },

  /** 切换 plugin 筛选 */
  togglePluginFilter(plugin: string): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      store.togglePluginFilter(plugin)
    })
  },

  /** 设置 plugin 筛选 */
  setPluginFilter(plugins: string[]): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      store.filterPlugins = plugins as any
    })
  },

  /** 切换 type 筛选 */
  toggleTypeFilter(type: string): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      store.toggleTypeFilter(type as any)
    })
  },

  /** 设置 type 筛选 */
  setTypeFilter(types: string[]): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      store.filterTypes = types as any
    })
  },

  /** 切换 status 筛选 */
  toggleStatusFilter(status: string): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      store.toggleStatusFilter(status as any)
    })
  },

  /** 设置 status 筛选 */
  setStatusFilter(statuses: string[]): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      store.filterStatuses = statuses as any
    })
  },

  /** 清空所有筛选 */
  clearFilters(): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      store.clearFilters()
    })
  },

  /** 获取活跃筛选数 */
  getActiveFilterCount(): Cypress.Chainable<number> {
    return cy.then(() => {
      const store = _getTaskStore()
      return store.activeFilterCount
    })
  },

  /** 获取过滤后的任务数 */
  getFilteredTaskCount(): Cypress.Chainable<number> {
    return cy.then(() => {
      const store = _getTaskStore()
      return store.filteredTasks.length
    })
  },

  /** 应用 WS 事件（模拟后端推送） */
  applyEvent(
    type: 'created' | 'update' | 'progress' | 'completed',
    data: Partial<EncvTask> & { id: string },
  ): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      store.applyEvent(type, data)
    })
  },

  /** 清空 store */
  clear(): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      store.bulkSetTasks([])
    })
  },

  /** 读取 tasks 数量 */
  taskCount(): Cypress.Chainable<number> {
    return cy.then(() => {
      const store = _getTaskStore()
      return store.tasks.length
    })
  },
}

// ============================================================================
// 3. DOM 操作 — 滚动模拟 / 元素查找 / 渲染断言
// ============================================================================

export const taskDom = {
  /**
   * 获取虚拟列表容器
   * 注意：TaskVirtualList 有两种模式
   *   - fallback 模式（scrollEl=null）：.task-virtual-list--fallback + 直接渲染子项
   *   - 虚拟模式（scrollEl 存在）：.task-virtual-list + 绝对定位子项
   */
  getVirtualList(): Cypress.Chainable<JQuery<HTMLElement>> {
    return cy.get('.task-virtual-list')
  },

  /** 获取当前渲染的所有虚拟 item（data-index） */
  getVirtualItems(): Cypress.Chainable<JQuery<HTMLElement>> {
    return cy.get('.task-virtual-list .task-virtual-item')
  },

  /** 获取当前渲染的虚拟 item 数量 */
  getVirtualItemCount(): Cypress.Chainable<number> {
    return this.getVirtualItems().then(($items) => $items.length)
  },

  /**
   * 模拟滚动到指定位置（通过修改 scrollTop）
   * 在 Cypress component 模式下，scrollEl 可能不存在（走 fallback）
   * 这时候通过滚动 ion-content 或 window 来模拟
   */
  scrollToY(scrollY: number): Cypress.Chainable<void> {
    return cy.get('ion-content').then(($ionContent) => {
      // 尝试 ion-content 的内部 scroll 元素
      const innerScroll = $ionContent[0].shadowRoot?.querySelector('.inner-scroll')
      if (innerScroll) {
        (innerScroll as HTMLElement).scrollTop = scrollY
        innerScroll.dispatchEvent(new Event('scroll', { bubbles: true }))
      } else {
        // fallback：滚 window
        window.scrollTo(0, scrollY)
      }
    })
  },

  /** 滚动到底部 */
  scrollToBottom(): Cypress.Chainable<void> {
    return cy.get('ion-content').then(($ionContent) => {
      const innerScroll = $ionContent[0].shadowRoot?.querySelector('.inner-scroll')
      if (innerScroll) {
        const el = innerScroll as HTMLElement
        el.scrollTop = el.scrollHeight
        el.dispatchEvent(new Event('scroll', { bubbles: true }))
      } else {
        window.scrollTo(0, document.body.scrollHeight)
      }
    })
  },

  /**
   * 断言 store 中任务数量（数据层）
   */
  assertStoreTaskCount(expected: number): Cypress.Chainable<void> {
    return cy.then(() => {
      const store = _getTaskStore()
      expect(store.tasks.length).to.equal(expected)
    })
  },

  /**
   * 断言当前 DOM 中渲染的虚拟 item 数量
   * 注意：虚拟滚动下这个值远小于总任务数（通常 = 视口容量 + 2*overscan）
   */
  assertVisibleItemCount(expected: number): Cypress.Chainable<void> {
    return this.getVirtualItemCount().then((count) => {
      expect(count).to.equal(expected)
    })
  },

  /**
   * 断言虚拟列表总高度正确（totalSize）
   * 在虚拟模式下，总高度 ≈ count * estimateSize（首次渲染未测量时）
   * 或 = Σ 实际测量高度（测量完成后）
   */
  assertVirtualListHeightGreaterThan(minHeight: number): Cypress.Chainable<void> {
    return this.getVirtualList().then(($list) => {
      const height = $list[0].getBoundingClientRect().height
      expect(height).to.be.greaterThan(minHeight)
    })
  },

  /**
   * 断言第一个可见 item 的 index（检查滚动位置）
   */
  assertFirstVisibleIndex(index: number): Cypress.Chainable<void> {
    return this.getVirtualItems().first().then(($item) => {
      const dataIndex = $item.attr('data-index')
      expect(Number(dataIndex)).to.equal(index)
    })
  },

  /**
   * 等待响应式更新完成（vue nextTick + worker fallback）
   */
  waitForReactive(ms: number = 500): Cypress.Chainable<void> {
    return cy.wait(ms)
  },
}
