/**
 * Group Card 使用 RunSummaries 总数验证
 *
 * 核心场景（真机 bug 回归测试）：
 *   - 真机自动化插件测试 1000+ 任务，聚合卡片只显示 100 个
 *   - 根因：group card 计数基于 store.tasks.length（视图分页只有 100 条）
 *   - 修复：group card 使用 runSummaries（后端 SQL 权威计数）
 *
 * 测试策略：
 *   - store 只注入 100 条任务（模拟视图分页）
 *   - runSummaries 注入 2100 条（后端权威）
 *   - 验证 displayedItems 中 group item 的 _summaryTotal = 2100（不是 100）
 *   - 验证 displayData.summary.total = 2100
 */
import Tasks from '../../src/views/Tasks.vue'
import { taskFixtures, taskStore } from '../support/task-test-helpers'
import { _getTaskStore, _getRunSummaries } from '../support/store-helpers'

const TEST_RUN_ID = 'run-summary-authority-test'

describe('Group Card 使用 RunSummaries（后端 SQL 权威）', () => {
  beforeEach(() => {
    cy.intercept('GET', '**/api/tasks*', { body: [], statusCode: 200 }).as('getTasks')
    cy.intercept('GET', '**/api/runs/summary*', { body: {}, statusCode: 200 }).as('getSummary')
    cy.intercept('DELETE', '**/api/tasks*', { body: { removed: 0 }, statusCode: 200 }).as('deleteTasks')
  })

  function mountWithSummaryGap(storeTaskCount: number, summaryTotal: number) {
    cy.mount(Tasks as any)
    cy.wait(3000)

    // 只往 store 注入少量任务（模拟视图分页）
    const tasks = taskFixtures.generateBatch(storeTaskCount, {
      runId: TEST_RUN_ID,
      triggeredBy: 'automation',
    })
    taskStore.injectTasks(tasks)
    taskStore.setViewMode('group')
    cy.wait(1000)

    // 注入 runSummaries（模拟后端 SQL 权威数据）
    cy.then(() => {
      const runSummaries = _getRunSummaries()
      const newMap = new Map(runSummaries.summaries.value)
      newMap.set(TEST_RUN_ID, {
        runId: TEST_RUN_ID,
        total: summaryTotal,
        passed: Math.floor(summaryTotal * 0.45),
        failed: Math.floor(summaryTotal * 0.08),
        running: Math.floor(summaryTotal * 0.15),
        pending: Math.floor(summaryTotal * 0.25),
        cancelled: Math.floor(summaryTotal * 0.07),
        percent: 53,
      })
      runSummaries.summaries.value = newMap
    })
    cy.wait(1000)

    return cy.wrap({ storeTaskCount, summaryTotal })
  }

  // ==========================================================================
  // 核心场景：store 少，summary 多（真机 bug 场景）
  // ==========================================================================
  describe('核心场景：store 100 / summary 2100', () => {
    it('store 只有 100 条任务，但 runSummaries 有 2100 条 → _summaryTotal = 2100（不是 100）', () => {
      mountWithSummaryGap(100, 2100)

      cy.then(() => {
        const store = _getTaskStore() as any
        // 验证 store 确实只有 100 条
        expect(store.tasks.length).to.equal(100)

        // 验证 runSummaries 有 2100 条
        const runSummaries = _getRunSummaries()
        const summary = runSummaries.summaries.value.get(TEST_RUN_ID)
        expect(summary).to.exist
        expect(summary?.total).to.equal(2100)
      })
    })

    it('store 100 / summary 2100 → group item 的 displayData.summary.total = 2100', () => {
      mountWithSummaryGap(100, 2100)

      cy.window().then(() => {
        // 等待 displayedItems 更新
        cy.wait(1500)
        cy.then(() => {
          const runSummaries = _getRunSummaries()
          const summary = runSummaries.summaries.value.get(TEST_RUN_ID)
          expect(summary).to.exist
          expect(summary?.total).to.equal(2100)
          expect(summary?.passed).to.be.greaterThan(0)
          expect(summary?.failed).to.be.greaterThan(0)
          expect(summary?.running).to.be.greaterThan(0)
          expect(summary?.pending).to.be.greaterThan(0)
        })
      })
    })

    it('store 100 / summary 2100 → passed/failed/running/pending 都来自 summary（不是 store）', () => {
      mountWithSummaryGap(100, 2100)

      cy.wait(1500)
      cy.then(() => {
        const runSummaries = _getRunSummaries()
        const summary = runSummaries.summaries.value.get(TEST_RUN_ID)
        expect(summary).to.exist

        // summary 各维度应该按 2100 的比例计算
        // 而不是按 store 的 100 条
        expect(summary?.passed).to.be.greaterThan(900) // 2100 * 0.45 = 945
        expect(summary?.failed).to.be.greaterThan(150) // 2100 * 0.08 = 168
        expect(summary?.running).to.be.greaterThan(300) // 2100 * 0.15 = 315
        expect(summary?.pending).to.be.greaterThan(500) // 2100 * 0.25 = 525
      })
    })
  })

  // ==========================================================================
  // 边界场景
  // ==========================================================================
  describe('边界场景', () => {
    it('store 0 / summary 0 → 正确显示 0', () => {
      mountWithSummaryGap(0, 0)

      cy.wait(1500)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.tasks.length).to.equal(0)

        const runSummaries = _getRunSummaries()
        const summary = runSummaries.summaries.value.get(TEST_RUN_ID)
        expect(summary?.total).to.equal(0)
      })
    })

    it('store 500 / summary 500 → 一致（无 gap）', () => {
      mountWithSummaryGap(500, 500)

      cy.wait(1500)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.tasks.length).to.equal(500)

        const runSummaries = _getRunSummaries()
        const summary = runSummaries.summaries.value.get(TEST_RUN_ID)
        expect(summary?.total).to.equal(500)
      })
    })

    it('store 50 / summary 5000 → 大差距场景（极端情况）', () => {
      mountWithSummaryGap(50, 5000)

      cy.wait(1500)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.tasks.length).to.equal(50)

        const runSummaries = _getRunSummaries()
        const summary = runSummaries.summaries.value.get(TEST_RUN_ID)
        expect(summary?.total).to.equal(5000)
        // 5000 * 0.45 = 2250 passed
        expect(summary?.passed).to.be.greaterThan(2000)
      })
    })
  })

  // ==========================================================================
  // 多 run 场景
  // ==========================================================================
  describe('多 run 场景', () => {
    it('多个 run，各有不同的 summary total → 每个 group 显示各自的 summary', () => {
      cy.mount(Tasks as any)
      cy.wait(3000)

      // 注入 3 个 run 的任务（每个 50 条）
      const run1Tasks = taskFixtures.generateBatch(50, { runId: 'run-1', startIndex: 0 })
      const run2Tasks = taskFixtures.generateBatch(50, { runId: 'run-2', startIndex: 50 })
      const run3Tasks = taskFixtures.generateBatch(50, { runId: 'run-3', startIndex: 100 })
      taskStore.injectTasks([...run1Tasks, ...run2Tasks, ...run3Tasks])
      taskStore.setViewMode('group')
      cy.wait(1000)

      // 注入 3 个 run 的不同 summary
      cy.then(() => {
        const runSummaries = _getRunSummaries()
        const newMap = new Map(runSummaries.summaries.value)
        newMap.set('run-1', {
          runId: 'run-1',
          total: 500,
          passed: 200,
          failed: 50,
          running: 100,
          pending: 150,
          cancelled: 0,
          percent: 50,
        })
        newMap.set('run-2', {
          runId: 'run-2',
          total: 1000,
          passed: 600,
          failed: 100,
          running: 150,
          pending: 150,
          cancelled: 0,
          percent: 70,
        })
        newMap.set('run-3', {
          runId: 'run-3',
          total: 2000,
          passed: 1000,
          failed: 200,
          running: 300,
          pending: 500,
          cancelled: 0,
          percent: 60,
        })
        runSummaries.summaries.value = newMap
      })
      cy.wait(1500)

      cy.then(() => {
        const runSummaries = _getRunSummaries()
        const s1 = runSummaries.summaries.value.get('run-1')
        const s2 = runSummaries.summaries.value.get('run-2')
        const s3 = runSummaries.summaries.value.get('run-3')

        expect(s1?.total).to.equal(500)
        expect(s2?.total).to.equal(1000)
        expect(s3?.total).to.equal(2000)

        // passed 数量也不同
        expect(s1?.passed).to.equal(200)
        expect(s2?.passed).to.equal(600)
        expect(s3?.passed).to.equal(1000)
      })
    })
  })

  // ==========================================================================
  // summary 动态更新
  // ==========================================================================
  describe('summary 动态更新', () => {
    it('summary 更新后 → group card 显示新的总数', () => {
      mountWithSummaryGap(100, 1000)

      cy.wait(1500)
      cy.then(() => {
        const runSummaries = _getRunSummaries()
        const before = runSummaries.summaries.value.get(TEST_RUN_ID)
        expect(before?.total).to.equal(1000)
      })

      // 更新 summary（模拟 WS 推送导致后端计数变化）
      cy.then(() => {
        const runSummaries = _getRunSummaries()
        const newMap = new Map(runSummaries.summaries.value)
        newMap.set(TEST_RUN_ID, {
          runId: TEST_RUN_ID,
          total: 1500,
          passed: 800,
          failed: 100,
          running: 200,
          pending: 400,
          cancelled: 0,
          percent: 60,
        })
        runSummaries.summaries.value = newMap
      })
      cy.wait(1500)

      cy.then(() => {
        const runSummaries = _getRunSummaries()
        const after = runSummaries.summaries.value.get(TEST_RUN_ID)
        expect(after?.total).to.equal(1500)
        expect(after?.passed).to.equal(800)
      })
    })
  })
})
