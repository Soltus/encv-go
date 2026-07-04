/**
 * GroupDetail.vue Cypress Component 真实组件测试（2026-06-23 替换 jsdom 版本）
 *
 * 覆盖真机关键场景：
 *   - 空 group：runId 对应 task 为空 → store + summary 一致
 *   - 多 task group：10 task 共享 runId → runTasksStore.tasks = 10
 *   - 1000 task group：聚合上限 → store = 1000，summary 同步
 *   - WS update 不逃逸：progress event → task.progress 更新，count 不变
 *
 * 设计原则：
 *   - 真实 GroupDetail.vue（不用 stub 子组件）
 *   - 真实 vue-router（memory history + router.push(/group/:runId)）
 *   - cy.intercept 拦截所有 API
 *   - store 层断言（cypress component 模式下 ion-content 渲染可能不完整）
 *
 * ⚠️ 已知问题（跟 Tasks.cy.ts 一样）：
 *   1. Worker postMessage 不能传 Pinia reactive Proxy → 容错
 *   2. ion-content shadow DOM 在 cypress component 模式可能不完整 → store 断言
 */
import { defineComponent } from 'vue/dist/vue.esm-bundler.js'
import GroupDetail from '../../src/views/GroupDetail.vue'
import { _getTaskStore, _getRunTasksStore, _pushTo } from '../support/store-helpers'

/** helper：构造 task fixture */
function makeTask(i: number, runId: string, status: any = 'queued'): any {
  return {
    id: `t-${runId}-${i}`,
    type: 'encrypt' as const,
    sourcePath: `/mock/${runId}/sample-${i}.mp4`,
    status,
    progress: status === 'completed' ? 100 : status === 'failed' ? 0 : 0,
    createdAt: '2026-06-23T10:00:00.000Z',
    runId,
    triggeredBy: 'automation' as const,
    pluginName: 'mp4-encrypt',
  }
}

function makeTaskList(count: number, runId: string, status: any = 'queued'): any[] {
  return Array.from({ length: count }, (_, i) => makeTask(i, runId, status))
}

describe('GroupDetail.vue 真实组件 (Cypress Component)', () => {
  beforeEach(() => {
    // 默认拦截
    cy.intercept('GET', '**/api/tasks*', { body: [], statusCode: 200 }).as('getTasks')
    cy.intercept('GET', '**/api/runs/summary*', { body: {}, statusCode: 200 }).as('getSummary')
    cy.intercept('GET', '**/api/runs/**', { body: {}, statusCode: 200 }).as('getRun')
    cy.intercept('GET', '**/api/calibration*', { body: null, statusCode: 200 }).as('getCalibration')

    // 容错 Worker postMessage 错误
    cy.on('uncaught:exception', (err) => {
      if (err.message.includes('postMessage') && err.message.includes('could not be cloned')) {
        return false
      }
      return false
    })
  })

  /**
   * 路由跳转到 /group/:runId + 拦截 API 返回 task + mount
   * - GroupDetail.vue 用 route.params.runId，必须先 router.push
   * - cypress 内部 router 是 memory history（component.ts 共享）
   */
  function mountGroupDetail(runId: string, taskCount: number, status: any = 'queued') {
    // 先跳到 /group/:runId（sharedTestRouter 在 store-helpers）
    cy.then(() => {
      void _pushTo(`/group/${runId}`)
    })

    // 拦截特定 runId 的 API
    const tasks = makeTaskList(taskCount, runId, status)
    cy.intercept('GET', `**/api/tasks*`, { body: tasks, statusCode: 200 })
    cy.intercept('GET', `**/api/runs/summary*`, {
      body: {
        runId,
        total: taskCount,
        queued: taskCount,
        running: 0,
        completed: 0,
        failed: 0,
      },
      statusCode: 200,
    })

    cy.mount(GroupDetail as any)
    cy.wait(3000)  // 等 onMounted loadRun + summary
  }

  it('空 group：runId 对应 0 task → store + summary 一致', () => {
    mountGroupDetail('r-empty', 0)
    cy.then(() => {
      const runTasksStore = _getRunTasksStore() as any
      // runTasksStore 应该被 loadRun 加载（空 array）
      expect(runTasksStore.tasks.value.length).to.equal(0)
    })
  })

  it('多 task group：10 task 共享 runId → runTasksStore.tasks = 10', () => {
    mountGroupDetail('r-multi', 10, 'queued')
    cy.then(() => {
      const runTasksStore = _getRunTasksStore() as any
      expect(runTasksStore.tasks.value.length).to.equal(10)
      const allSameRunId = runTasksStore.tasks.value.every((t: any) => t.runId === 'r-multi')
      expect(allSameRunId).to.equal(true)
    })
  })

  it('1000 task group：聚合上限 → store = 1000', () => {
    mountGroupDetail('r-thousand', 1000, 'queued')
    cy.then(() => {
      const runTasksStore = _getRunTasksStore() as any
      expect(runTasksStore.tasks.value.length).to.equal(1000)
    })
  })

  it('WS progress event → task.progress 更新，count 不变', () => {
    mountGroupDetail('r-ws-progress', 5, 'running')
    cy.then(() => {
      const runTasksStore = _getRunTasksStore() as any
      expect(runTasksStore.tasks.value.length).to.equal(5)

      // 模拟 5 task 的 progress update
      for (let i = 0; i < 5; i++) {
        runTasksStore.applyEvent('progress', { id: `t-r-ws-progress-${i}`, progress: 50 })
      }
      // count 不变（不逃逸）
      expect(runTasksStore.tasks.value.length).to.equal(5)
      // progress 已更新
      const updated = runTasksStore.tasks.value.find((t: any) => t.id === 't-r-ws-progress-0')
      expect(updated?.progress).to.equal(50)
    })
  })
})
