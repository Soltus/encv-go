/**
 * WS 批量任务推送测试 — 验证 MAX_LOADED_TASKS 守卫行为
 *
 * 背景（真机 bug 修复）：
 *   - 用户反馈：安卓真机运行自动化插件测试 1000+ 任务，只显示 100 个
 *   - 根因：MAX_LOADED_TASKS=100 的 WS 守卫拦截了批量创建的新任务
 *   - 修复：提高到 10000（虚拟滚动 + Worker 计算保证性能，1000+ 任务场景全覆盖）
 *
 * 验证逻辑：
 *   1. 先注入 N 个任务（达到/超过 MAX_LOADED_TASKS）
 *   2. 模拟 WS task:created 事件推送 M 个新任务
 *   3. 检查 store 中最终任务数
 */
import { defineComponent } from 'vue/dist/vue.esm-bundler.js'
import Tasks from '../../src/views/Tasks.vue'
import { taskFixtures, taskStore } from '../support/task-test-helpers'
import { _getTaskStore } from '../support/store-helpers'
import { MAX_LOADED_TASKS } from '../../src/stores/taskStore'

describe('WS 批量任务推送 — MAX_LOADED_TASKS 守卫测试', () => {
  beforeEach(() => {
    cy.intercept('GET', '**/api/tasks*', { body: [], statusCode: 200 }).as('getTasks')
    cy.intercept('GET', '**/api/runs/summary*', { body: {}, statusCode: 200 }).as('getSummary')
    cy.intercept('DELETE', '**/api/tasks*', { body: { removed: 0 }, statusCode: 200 }).as('deleteTasks')
  })

  function mountAndInject(count: number) {
    cy.mount(Tasks as any)
    cy.wait(3000) // 等 hydrate + runSummaries
    const tasks = taskFixtures.generateBatch(count)
    taskStore.injectTasks(tasks)
    cy.wait(500)
    return cy.wrap(tasks)
  }

  /**
   * 模拟 WS 批量推送 N 个 task:created 事件
   */
  function simulateWsCreatedBatch(startIndex: number, count: number, runId?: string) {
    return cy.then(() => {
      const store = _getTaskStore()
      const batch = taskFixtures.generateBatch(count, {
        startIndex,
        runId,
        triggeredBy: runId ? 'automation' : 'user',
      })
      for (const task of batch) {
        store.applyEvent('created', task)
      }
      return batch.length
    })
  }

  // ==========================================================================
  // 验证 MAX_LOADED_TASKS 常量值（修复后应为 10000）
  // ==========================================================================
  it('MAX_LOADED_TASKS = 10000（覆盖 1000+ 任务场景）', () => {
    expect(MAX_LOADED_TASKS).to.equal(10000)
  })

  // ==========================================================================
  // 场景 1：store 未满时，WS created 事件正常添加
  // ==========================================================================
  describe('场景 1：store 未满时 WS 推送', () => {
    it('store 有 50 个任务 → 推送 10 个新任务 → store 有 60 个', () => {
      mountAndInject(50)
      taskStore.taskCount().should('equal', 50)

      simulateWsCreatedBatch(1000, 10)
      taskStore.taskCount().should('equal', 60)
    })
  })

  // ==========================================================================
  // 场景 2：自动化批量创建 1000 个任务场景（核心回归测试）
  // ==========================================================================
  describe('场景 2：自动化批量创建 1000 个任务（核心回归测试）', () => {
    it('store 有 100 个任务 → 推送 900 个 → store 有 1000 个（全部正常添加）', () => {
      mountAndInject(100)
      taskStore.taskCount().should('equal', 100)

      // 模拟 WS 批量推送 900 个任务（自动化测试场景）
      simulateWsCreatedBatch(100, 900, 'run-automation-1000')

      // 修复后：应该有 1000 个任务（全部正常添加）
      taskStore.taskCount().should('equal', 1000)
    })

    it('store 有 200 个任务 → 推送 100 个 → store 有 300 个（全部正常添加）', () => {
      mountAndInject(200)
      taskStore.taskCount().should('equal', 200)

      simulateWsCreatedBatch(1000, 100, 'run-batch-test')

      taskStore.taskCount().should('equal', 300)
    })

    it('1000 个任务共享同一个 runId → tasksByRunId 只有 1 个 group（无逃逸）', () => {
      mountAndInject(0)

      // 推送 1000 个共享 runId 的任务
      simulateWsCreatedBatch(0, 1000, 'run-automation-single-group')

      taskStore.taskCount().should('equal', 1000)
      cy.then(() => {
        const store = _getTaskStore()
        // 验证只有 1 个 runId group
        expect(store.tasksByRunId.size).to.equal(1)
        expect(store.tasksByRunId.get('run-automation-single-group')?.length).to.equal(1000)
      })
    })
  })

  // ==========================================================================
  // 场景 3：已存在的任务的 update/progress/completed 事件正常处理
  // ==========================================================================
  describe('场景 3：已存在任务的状态更新不受影响', () => {
    it('store 有 100 个任务 → 第 50 个任务 running → 状态正常更新', () => {
      mountAndInject(100)
      cy.then(() => {
        const store = _getTaskStore()
        const task50 = store.tasks[50]
        expect(task50.status).to.equal('queued')
        expect(task50.progress).to.equal(0)
      })

      // 模拟 WS task:update 事件
      cy.then(() => {
        const store = _getTaskStore()
        const taskId = store.tasks[50].id
        store.applyEvent('update', {
          id: taskId,
          status: 'running',
          progress: 50,
        })
      })

      // 验证状态更新了
      cy.then(() => {
        const store = _getTaskStore()
        const task50 = store.tasks[50]
        expect(task50.status).to.equal('running')
        expect(task50.progress).to.equal(50)
      })
    })
  })
})
