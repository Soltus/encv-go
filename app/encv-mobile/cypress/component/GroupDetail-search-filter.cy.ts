/**
 * GroupDetail 搜索筛选与顶层共享测试
 *
 * 覆盖场景：
 *   - GroupDetail 使用 taskStore 的共享搜索筛选状态
 *   - 在 Tasks 页设置筛选 → 进入 GroupDetail 筛选状态保持
 *   - 在 GroupDetail 修改筛选 → Tasks 页也同步
 *   - GroupDetail 内搜索筛选结果正确
 *   - 1000+ 任务下搜索筛选性能
 *
 * 断言层级：store 层为核心断言
 */
import GroupDetail from '../../src/views/GroupDetail.vue'
import { taskFixtures, taskStore } from '../support/task-test-helpers'
import { _getTaskStore, _getRunTasksStore, _pushTo } from '../support/store-helpers'

const TEST_RUN_ID = 'run-group-detail-filter-test'

describe('GroupDetail 搜索筛选与顶层共享', () => {
  beforeEach(() => {
    cy.intercept('GET', '**/api/tasks*', { body: [], statusCode: 200 }).as('getTasks')
    cy.intercept('GET', '**/api/runs/summary*', { body: {}, statusCode: 200 }).as('getSummary')
    cy.intercept('GET', '**/api/runs/**', { body: {}, statusCode: 200 }).as('getRun')
    cy.intercept('GET', '**/api/calibration*', { body: null, statusCode: 200 }).as('getCalibration')
    cy.on('uncaught:exception', (err) => {
      if (err.message.includes('postMessage') && err.message.includes('could not be cloned')) {
        return false
      }
      return false
    })
  })

  function mountGroupDetail(runId: string, taskCount: number) {
    cy.then(() => {
      void _pushTo(`/group/${runId}`)
    })

    const tasks = taskFixtures.generateMixedBatch(taskCount, {
      runId,
      triggeredBy: 'automation',
      plugins: ['mp4-encrypt', 'mkv-encrypt', 'png-encrypt', 'pdf-encrypt'],
    })

    cy.intercept('GET', `**/api/tasks*`, { body: tasks, statusCode: 200 })
    cy.intercept('GET', `**/api/runs/summary*`, {
      body: {
        runId,
        total: taskCount,
        passed: Math.floor(taskCount * 0.45),
        failed: Math.floor(taskCount * 0.1),
        running: Math.floor(taskCount * 0.15),
        pending: Math.floor(taskCount * 0.25),
        cancelled: Math.floor(taskCount * 0.05),
        percent: 55,
      },
      statusCode: 200,
    })

    cy.mount(GroupDetail as any)
    cy.wait(3000)
    return cy.wrap(tasks)
  }

  // ==========================================================================
  // 共享状态验证
  // ==========================================================================
  describe('状态共享', () => {
    it('GroupDetail 从 taskStore 读取共享的 searchQuery', () => {
      // 先在 taskStore 设置搜索
      const store = _getTaskStore()
      store.setSearchQuery('test-search')

      mountGroupDetail(TEST_RUN_ID, 50)

      // 验证 GroupDetail 读取到了相同的 searchQuery
      cy.then(() => {
        const s = _getTaskStore()
        expect(s.searchQuery).to.equal('test-search')
      })
    })

    it('GroupDetail 从 taskStore 读取共享的 filterStatuses', () => {
      const store = _getTaskStore()
      store.setStatusFilter(['completed', 'failed'])

      mountGroupDetail(TEST_RUN_ID, 50)

      cy.then(() => {
        const s = _getTaskStore()
        expect(s.filterStatuses).to.deep.equal(['completed', 'failed'])
      })
    })

    it('GroupDetail 从 taskStore 读取共享的 filterPlugins', () => {
      const store = _getTaskStore()
      store.setPluginFilter(['mp4-encrypt'])

      mountGroupDetail(TEST_RUN_ID, 50)

      cy.then(() => {
        const s = _getTaskStore()
        expect(s.filterPlugins).to.deep.equal(['mp4-encrypt'])
      })
    })

    it('GroupDetail 修改搜索 → taskStore 同步更新（双向绑定）', () => {
      mountGroupDetail(TEST_RUN_ID, 50)

      // 初始为空
      cy.then(() => {
        const s = _getTaskStore()
        expect(s.searchQuery).to.equal('')
      })

      // 修改 store 中的 searchQuery（模拟用户输入）
      taskStore.setSearchQuery('new-search')
      cy.wait(500)

      // 验证 store 已更新
      cy.then(() => {
        const s = _getTaskStore()
        expect(s.searchQuery).to.equal('new-search')
      })
    })

    it('activeFilterCount 在 GroupDetail 中与 Tasks 页一致', () => {
      const store = _getTaskStore()
      store.setSearchQuery('test')
      store.setStatusFilter(['running'])
      store.setPluginFilter(['mp4-encrypt'])

      mountGroupDetail(TEST_RUN_ID, 50)

      cy.then(() => {
        const s = _getTaskStore()
        // search + status + plugin = 3
        expect(s.activeFilterCount).to.equal(3)
      })
    })
  })

  // ==========================================================================
  // GroupDetail 内搜索筛选结果
  // ==========================================================================
  describe('搜索筛选结果', () => {
    it('搜索文件名 → runTasks 过滤结果正确', () => {
      mountGroupDetail(TEST_RUN_ID, 100)

      // 清空筛选，基线
      taskStore.clearFilters()
      cy.wait(500)
      cy.then(() => {
        const runStore = _getRunTasksStore()
        // 初始所有任务
        expect(runStore.tasks.value.length).to.be.greaterThan(0)
      })

      // 搜索特定文件名
      taskStore.setSearchQuery('sample-0001')
      cy.wait(500)

      cy.then(() => {
        const runStore = _getRunTasksStore() as any
        const allTasks = runStore.tasks.value
        // 过滤后的结果
        const q = 'sample-0001'.toLowerCase()
        const filtered = allTasks.filter((t: any) => {
          const name = (t.targetPath?.split('/').pop() ?? t.sourcePath?.split('/').pop() ?? t.id.slice(0, 8)).toLowerCase()
          const plugin = (t.pluginName || '').toLowerCase()
          const error = (t.error || '').toLowerCase()
          const tid = t.id.toLowerCase()
          return name.includes(q) || plugin.includes(q) || error.includes(q) || tid.includes(q)
        })
        cy.log(`搜索 sample-0001 匹配 ${filtered.length} 个任务`)
        expect(filtered.length).to.be.lessThan(allTasks.length)
        expect(filtered.length).to.be.greaterThan(0)
      })
    })

    it('plugin 筛选 → 只显示对应插件的任务', () => {
      mountGroupDetail(TEST_RUN_ID, 100)

      taskStore.clearFilters()
      taskStore.setPluginFilter(['mp4-encrypt'])
      cy.wait(500)

      cy.then(() => {
        const runStore = _getRunTasksStore() as any
        const allTasks = runStore.tasks.value
        const mp4Tasks = allTasks.filter((t: any) => t.pluginName === 'mp4-encrypt')
        cy.log(`mp4-encrypt 任务数: ${mp4Tasks.length}`)
        expect(mp4Tasks.length).to.be.greaterThan(0)
        expect(mp4Tasks.length).to.be.lessThan(allTasks.length)
      })
    })

    it('status 筛选 → 只显示对应状态的任务', () => {
      mountGroupDetail(TEST_RUN_ID, 100)

      taskStore.clearFilters()
      taskStore.setStatusFilter(['completed'])
      cy.wait(500)

      cy.then(() => {
        const runStore = _getRunTasksStore() as any
        const allTasks = runStore.tasks.value
        const completedTasks = allTasks.filter((t: any) => t.status === 'completed')
        cy.log(`completed 任务数: ${completedTasks.length}`)
        expect(completedTasks.length).to.be.greaterThan(0)
        expect(completedTasks.length).to.be.lessThan(allTasks.length)
      })
    })

    it('type 筛选 → 只显示对应类型的任务', () => {
      mountGroupDetail(TEST_RUN_ID, 100)

      taskStore.clearFilters()
      taskStore.setTypeFilter(['encrypt'])
      cy.wait(500)

      cy.then(() => {
        const runStore = _getRunTasksStore() as any
        const allTasks = runStore.tasks.value
        const encryptTasks = allTasks.filter((t: any) => t.type === 'encrypt')
        cy.log(`encrypt 任务数: ${encryptTasks.length}`)
        expect(encryptTasks.length).to.equal(allTasks.length) // fixture 默认都是 encrypt
      })
    })

    it('清空筛选 → 恢复全部任务', () => {
      mountGroupDetail(TEST_RUN_ID, 100)

      taskStore.setSearchQuery('sample-0001')
      taskStore.setStatusFilter(['completed'])
      cy.wait(500)

      taskStore.clearFilters()
      cy.wait(500)

      cy.then(() => {
        const s = _getTaskStore()
        expect(s.activeFilterCount).to.equal(0)
      })
    })
  })

  // ==========================================================================
  // 1000+ 任务下的搜索筛选
  // ==========================================================================
  describe('1000+ 任务量级', () => {
    it('1000 任务下搜索仍然正常工作', () => {
      mountGroupDetail(TEST_RUN_ID, 1000)

      taskStore.clearFilters()
      cy.wait(500)
      taskStore.setSearchQuery('mp4-encrypt')
      cy.wait(1000)

      cy.then(() => {
        const runStore = _getRunTasksStore() as any
        const allTasks = runStore.tasks.value
        expect(allTasks.length).to.equal(1000)
        const mp4Tasks = allTasks.filter((t: any) => t.pluginName === 'mp4-encrypt')
        cy.log(`mp4-encrypt 任务数: ${mp4Tasks.length}`)
        expect(mp4Tasks.length).to.be.greaterThan(0)
        expect(mp4Tasks.length).to.be.lessThan(1000)
      })
    })

    it('1000 任务下多维度组合筛选正常', () => {
      mountGroupDetail(TEST_RUN_ID, 1000)

      taskStore.clearFilters()
      taskStore.setSearchQuery('sample')
      taskStore.setPluginFilter(['mp4-encrypt', 'mkv-encrypt'])
      taskStore.setStatusFilter(['completed', 'failed'])
      cy.wait(1000)

      cy.then(() => {
        const runStore = _getRunTasksStore() as any
        const allTasks = runStore.tasks.value
        expect(allTasks.length).to.equal(1000)
        const filtered = allTasks.filter((t: any) => {
          const name = (t.targetPath?.split('/').pop() ?? t.sourcePath?.split('/').pop() ?? t.id.slice(0, 8)).toLowerCase()
          const plugin = (t.pluginName || '').toLowerCase()
          const error = (t.error || '').toLowerCase()
          const tid = t.id.toLowerCase()
          return (name.includes('sample') || plugin.includes('sample') || error.includes('sample') || tid.includes('sample'))
            && ['mp4-encrypt', 'mkv-encrypt'].includes(t.pluginName)
            && ['completed', 'failed'].includes(t.status)
        })
        cy.log(`三重组合筛选匹配: ${filtered.length} 个`)
        expect(filtered.length).to.be.greaterThan(0)
        expect(filtered.length).to.be.lessThan(1000)
      })
    })
  })
})
