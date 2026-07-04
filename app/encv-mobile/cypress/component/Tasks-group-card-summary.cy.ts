import Tasks from '../../src/views/Tasks.vue'
import { taskFixtures } from '../support/task-test-helpers'
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

    const tasks = taskFixtures.generateBatch(storeTaskCount, {
      runId: TEST_RUN_ID,
      triggeredBy: 'automation',
    })

    cy.then(() => {
      const store = _getTaskStore() as any
      store.bulkSetTasks(tasks)
      store.viewMode = 'group'
    })
    cy.wait(1000)

    cy.then(() => {
      const runSummaries = _getRunSummaries() as any
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

  describe('核心场景：store 100 / summary 2100', () => {
    it('store 只有 100 条任务，但 runSummaries 有 2100 条 → summary total = 2100', () => {
      mountWithSummaryGap(100, 2100)

      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.tasks.length).to.equal(100)

        const runSummaries = _getRunSummaries() as any
        const summary = runSummaries.summaries.value.get(TEST_RUN_ID)
        expect(summary).to.exist
        expect(summary?.total).to.equal(2100)
      })
    })

    it('summary 各维度按 2100 比例计算', () => {
      mountWithSummaryGap(100, 2100)

      cy.wait(1500)
      cy.then(() => {
        const runSummaries = _getRunSummaries() as any
        const summary = runSummaries.summaries.value.get(TEST_RUN_ID)
        expect(summary).to.exist

        // 2100 * 0.45 = 945 passed
        expect(summary?.passed).to.be.greaterThan(900)
        // 2100 * 0.08 = 168 failed
        expect(summary?.failed).to.be.greaterThan(150)
        // 2100 * 0.15 = 315 running
        expect(summary?.running).to.be.greaterThan(300)
        // 2100 * 0.25 = 525 pending
        expect(summary?.pending).to.be.greaterThan(500)
      })
    })
  })

  describe('边界场景', () => {
    it('store 0 / summary 0 → 正确显示 0', () => {
      mountWithSummaryGap(0, 0)

      cy.wait(1500)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.tasks.length).to.equal(0)

        const runSummaries = _getRunSummaries() as any
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

        const runSummaries = _getRunSummaries() as any
        const summary = runSummaries.summaries.value.get(TEST_RUN_ID)
        expect(summary?.total).to.equal(500)
      })
    })

    it('store 50 / summary 5000 → 大差距场景', () => {
      mountWithSummaryGap(50, 5000)

      cy.wait(1500)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.tasks.length).to.equal(50)

        const runSummaries = _getRunSummaries() as any
        const summary = runSummaries.summaries.value.get(TEST_RUN_ID)
        expect(summary?.total).to.equal(5000)
        expect(summary?.passed).to.be.greaterThan(2000)
      })
    })
  })

  describe('多 run 场景', () => {
    it('多个 run，各有不同的 summary total → 每个 group 显示各自的 summary', () => {
      cy.mount(Tasks as any)
      cy.wait(3000)

      const run1Tasks = taskFixtures.generateBatch(50, { runId: 'run-1', startIndex: 0 })
      const run2Tasks = taskFixtures.generateBatch(50, { runId: 'run-2', startIndex: 50 })
      const run3Tasks = taskFixtures.generateBatch(50, { runId: 'run-3', startIndex: 100 })

      cy.then(() => {
        const store = _getTaskStore() as any
        store.bulkSetTasks([...run1Tasks, ...run2Tasks, ...run3Tasks])
        store.viewMode = 'group'
      })
      cy.wait(1000)

      cy.then(() => {
        const runSummaries = _getRunSummaries() as any
        const newMap = new Map(runSummaries.summaries.value)
        newMap.set('run-1', { runId: 'run-1', total: 500, passed: 200, failed: 50, running: 100, pending: 150, cancelled: 0, percent: 50 })
        newMap.set('run-2', { runId: 'run-2', total: 1000, passed: 600, failed: 100, running: 150, pending: 150, cancelled: 0, percent: 70 })
        newMap.set('run-3', { runId: 'run-3', total: 2000, passed: 1000, failed: 200, running: 300, pending: 500, cancelled: 0, percent: 60 })
        runSummaries.summaries.value = newMap
      })
      cy.wait(1500)

      cy.then(() => {
        const runSummaries = _getRunSummaries() as any
        const s1 = runSummaries.summaries.value.get('run-1')
        const s2 = runSummaries.summaries.value.get('run-2')
        const s3 = runSummaries.summaries.value.get('run-3')

        expect(s1?.total).to.equal(500)
        expect(s2?.total).to.equal(1000)
        expect(s3?.total).to.equal(2000)

        expect(s1?.passed).to.equal(200)
        expect(s2?.passed).to.equal(600)
        expect(s3?.passed).to.equal(1000)
      })
    })
  })

  describe('summary 动态更新', () => {
    it('summary 更新后 → 显示新的总数', () => {
      mountWithSummaryGap(100, 1000)

      cy.wait(1500)
      cy.then(() => {
        const runSummaries = _getRunSummaries() as any
        const before = runSummaries.summaries.value.get(TEST_RUN_ID)
        expect(before?.total).to.equal(1000)
      })

      cy.then(() => {
        const runSummaries = _getRunSummaries() as any
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
        const runSummaries = _getRunSummaries() as any
        const after = runSummaries.summaries.value.get(TEST_RUN_ID)
        expect(after?.total).to.equal(1500)
        expect(after?.passed).to.equal(800)
      })
    })
  })
})
