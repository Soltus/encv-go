/**
 * 1000+ 任务量级测试 — 验证大批量任务下的渲染正确性
 *
 * 背景（真机 bug 修复）：
 *   - 用户反馈：安卓真机运行自动化插件测试 1000+ 任务，只显示 100 个
 *   - 根因：MAX_LOADED_TASKS=100 的 WS 守卫拦截了批量创建的新任务
 *   - 修复：提高到 10000（虚拟滚动 + Worker 计算保证性能）
 *
 * 测试策略：
 *   - 注入 1000 个任务到 store（绕过后端分页，直接测前端渲染链路）
 *   - 分层断言：store 层 → computed 层 → 虚拟列表层 → DOM 层
 *   - 验证 viewMode=group 和 viewMode=flat 两种模式
 *   - 验证虚拟滚动模式（Cypress component 下 ion-content shadow DOM 完整）
 *
 * 关键断言：
 *   - store.tasks.length = 1000（数据没丢）
 *   - 虚拟列表总高度与任务数成正比（没有被截断到 100）
 *   - DOM 中只有 ~14 个可见 item（虚拟滚动正常工作）
 */
import { defineComponent } from 'vue/dist/vue.esm-bundler.js'
import Tasks from '../../src/views/Tasks.vue'
import { taskFixtures, taskStore, taskDom } from '../support/task-test-helpers'
import { _getTaskStore } from '../support/store-helpers'

describe('1000+ 任务量级渲染测试', () => {
  beforeEach(() => {
    cy.intercept('GET', '**/api/tasks*', { body: [], statusCode: 200 }).as('getTasks')
    cy.intercept('GET', '**/api/runs/summary*', { body: {}, statusCode: 200 }).as('getSummary')
    cy.intercept('DELETE', '**/api/tasks*', { body: { removed: 0 }, statusCode: 200 }).as('deleteTasks')
  })

  function mountWithTasks(count: number, runId?: string) {
    cy.mount(Tasks as any)
    cy.wait(3000) // 等 hydrate + runSummaries
    const tasks = taskFixtures.generateBatch(count, {
      runId,
      triggeredBy: runId ? 'automation' : 'user',
    })
    taskStore.injectTasks(tasks)
    cy.wait(1000) // 等 reactive update
    return cy.wrap(tasks)
  }

  // ==========================================================================
  // Layer 1: Store 数据层
  // ==========================================================================
  describe('Layer 1: Store 数据层', () => {
    it('1000 task 注入 → store.tasks.length = 1000', () => {
      mountWithTasks(1000)
      taskStore.taskCount().should('equal', 1000)
    })

    it('5000 task 注入 → store.tasks.length = 5000', () => {
      mountWithTasks(5000)
      taskStore.taskCount().should('equal', 5000)
    })

    it('100 task 注入 → store.tasks.length = 100（基线）', () => {
      mountWithTasks(100)
      taskStore.taskCount().should('equal', 100)
    })
  })

  // ==========================================================================
  // Layer 2: computed 计算层（displayedItems）
  // ==========================================================================
  describe('Layer 2: displayedItems 计算层', () => {
    it('flat 模式 1000 task → store 数据完整（无截断）', () => {
      mountWithTasks(1000)
      taskStore.setViewMode('flat')
      cy.wait(1000)
      cy.then(() => {
        const store = _getTaskStore()
        expect(store.tasks.length).to.equal(1000)
      })
    })

    it('group 模式 1000 task（同 runId）→ 只有 1 个 runId group', () => {
      mountWithTasks(1000, 'run-single')
      taskStore.setViewMode('group')
      cy.wait(1000)
      cy.then(() => {
        const store = _getTaskStore()
        // 验证 tasksByRunId 只有 1 个
        const runIdCount = store.tasksByRunId?.size || Object.keys(store.tasksByRunId || {}).length
        expect(runIdCount).to.equal(1)
      })
    })
  })

  // ==========================================================================
  // Layer 3: 虚拟列表配置层
  // ==========================================================================
  describe('Layer 3: 虚拟列表配置层', () => {
    it('1000 task → 虚拟列表存在且 item 数远小于 1000（虚拟滚动正常工作）', () => {
      mountWithTasks(1000)
      taskStore.setViewMode('flat')
      cy.wait(1000)

      // 验证虚拟列表存在
      taskDom.getVirtualList().should('exist')

      // 虚拟滚动模式下，DOM 中只有可见的 ~14 个 item（视口容量 + overscan）
      // 这远小于 1000，证明虚拟滚动在正常工作
      taskDom.getVirtualItemCount().should('be.lessThan', 50)
      taskDom.getVirtualItemCount().should('be.greaterThan', 5)
    })

    it('1000 task → 虚拟列表 scrollHeight 足够大（数据未被截断到 100）', () => {
      mountWithTasks(1000)
      taskStore.setViewMode('flat')
      cy.wait(1500)

      taskDom.getVirtualList().then(($list) => {
        const scrollHeight = $list[0].scrollHeight
        cy.log(`scrollHeight: ${scrollHeight}px`)

        // 核心断言：如果被截断到 100，scrollHeight 会是 ~12000px（100 * ~120px per item）
        // 1000 个任务的 scrollHeight 应该远大于 100 个任务的情况
        // 用 50000px 作为阈值（保守估计，每个 item 至少 50px * 1000 = 50000）
        expect(scrollHeight).to.be.greaterThan(50000)
      })
    })
  })

  // ==========================================================================
  // Layer 4: DOM 渲染层（虚拟滚动模式）
  // ==========================================================================
  describe('Layer 4: DOM 渲染层（虚拟滚动模式）', () => {
    it('虚拟滚动模式 1000 task → DOM 中只有 ~14 个可见 item（不是 1000）', () => {
      mountWithTasks(1000)
      taskStore.setViewMode('flat')
      cy.wait(1000)

      // 验证虚拟滚动模式（不是 fallback 模式）
      cy.get('.task-virtual-list').should('exist')
      cy.get('.task-virtual-list--fallback').should('not.exist')

      // 虚拟滚动下 DOM 节点数 = 视口容量 + 2 * overscan
      // 视口约 ~-4 个（因为 cypress headless 视口可能较小）
      // overscan 默认 10
      // 所以总共约 14 个左右
      taskDom.getVirtualItemCount().then((count) => {
        cy.log(`Visible virtual items: ${count}`)
        expect(count).to.be.lessThan(50)
        expect(count).to.be.greaterThan(5)
      })
    })

    it('第一个可见 item 的 data-index = 0（初始位置）', () => {
      mountWithTasks(1000)
      taskStore.setViewMode('flat')
      cy.wait(1000)
      taskDom.getVirtualItems().first().then(($item) => {
        const dataIndex = $item.attr('data-index')
        expect(Number(dataIndex)).to.equal(0)
      })
    })

    it('50 task → 虚拟列表正常渲染', () => {
      mountWithTasks(50)
      taskStore.setViewMode('flat')
      cy.wait(1000)
      taskDom.getVirtualList().should('exist')
      taskDom.getVirtualItemCount().should('be.greaterThan', 0)
    })

    it('20 task → 虚拟列表正常渲染（少量数据也工作）', () => {
      mountWithTasks(20)
      taskStore.setViewMode('flat')
      cy.wait(1000)
      taskDom.getVirtualList().should('exist')
      taskDom.getVirtualItemCount().should('be.greaterThan', 0)
    })
  })

  // ==========================================================================
  // Layer 5: 关键场景验证 — 同 runId 的 1000 task 聚合显示
  // ==========================================================================
  describe('Layer 5: 同 runId 1000 task 聚合显示', () => {
    it('1000 task 共享 runId → store 中所有任务 runId 都相同（无逃逸）', () => {
      mountWithTasks(1000, 'run-automation-batch')
      cy.wait(1000)
      cy.then(() => {
        const store = _getTaskStore()
        const tasks = store.tasks
        expect(tasks.length).to.equal(1000)
        // 所有任务的 runId 都应该是 'run-automation-batch'
        const uniqueRunIds = new Set(tasks.map((t: any) => t.runId))
        expect(uniqueRunIds.size).to.equal(1)
        expect(uniqueRunIds.has('run-automation-batch')).to.be.true
      })
    })

    it('1000 task 共享 runId → tasksByRunId 只有 1 个 group', () => {
      mountWithTasks(1000, 'run-automation-batch')
      taskStore.setViewMode('group')
      cy.wait(2000)

      cy.then(() => {
        const store = _getTaskStore()
        // 检查 tasksByRunId 的数量
        const runIdCount = store.tasksByRunId?.size || Object.keys(store.tasksByRunId || {}).length
        expect(runIdCount).to.equal(1)
      })
    })
  })
})
