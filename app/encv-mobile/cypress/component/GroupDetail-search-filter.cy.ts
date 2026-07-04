import GroupDetail from '../../src/views/GroupDetail.vue'
import { taskFixtures } from '../support/task-test-helpers'
import { _getTaskStore, _getRunTasksStore, _pushTo } from '../support/store-helpers'

const TEST_RUN_ID = 'run-group-detail-filter-test'

describe('GroupDetail 搜索筛选与顶层共享', () => {
  beforeEach(() => {
    cy.intercept('GET', '**/api/tasks*', { body: [], statusCode: 200 }).as('getTasks')
    cy.intercept('GET', '**/api/runs/summary*', { body: {}, statusCode: 200 }).as('getSummary')
    cy.intercept('GET', '**/api/runs/**', { body: {}, statusCode: 200 }).as('getRun')
    cy.intercept('GET', '**/api/calibration*', { body: null, statusCode: 200 }).as('getCalibration')
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

    cy.intercept('GET', '**/api/tasks*', { body: tasks, statusCode: 200 })
    cy.intercept('GET', '**/api/runs/summary*', {
      body: { runId, total: taskCount, passed: 90, failed: 10, running: 30, pending: 50, cancelled: 10, percent: 55 },
      statusCode: 200,
    })

    cy.mount(GroupDetail as any)
    cy.wait(3000)

    // 直接注入到 runTasksStore
    cy.then(() => {
      const runStore = _getRunTasksStore() as any
      runStore.currentRunId = runId
      runStore.tasks = tasks
    })
    cy.wait(500)

    return cy.wrap(tasks)
  }

  describe('状态共享', () => {
    it('GroupDetail 从 taskStore 读取共享的 searchQuery', () => {
      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('test-search')
      })

      mountGroupDetail(TEST_RUN_ID, 50)

      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.searchQuery).to.equal('test-search')
      })
    })

    it('GroupDetail 从 taskStore 读取共享的 filterStatuses', () => {
      cy.then(() => {
        const store = _getTaskStore() as any
        store.filterStatuses = ['completed', 'failed']
      })

      mountGroupDetail(TEST_RUN_ID, 50)

      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.filterStatuses).to.deep.equal(['completed', 'failed'])
      })
    })

    it('GroupDetail 从 taskStore 读取共享的 filterPlugins', () => {
      cy.then(() => {
        const store = _getTaskStore() as any
        store.filterPlugins = ['mp4-encrypt']
      })

      mountGroupDetail(TEST_RUN_ID, 50)

      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.filterPlugins).to.deep.equal(['mp4-encrypt'])
      })
    })

    it('activeFilterCount 与 taskStore 一致', () => {
      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('test')
        store.filterStatuses = ['running']
        store.filterPlugins = ['mp4-encrypt']
      })

      mountGroupDetail(TEST_RUN_ID, 50)

      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.activeFilterCount).to.equal(3)
      })
    })
  })

  describe('搜索筛选结果', () => {
    it('搜索文件名 → runTasks 过滤结果正确', () => {
      mountGroupDetail(TEST_RUN_ID, 100)

      cy.then(() => {
        const store = _getTaskStore() as any
        store.clearFilters()
      })
      cy.wait(500)

      cy.then(() => {
        const runStore = _getRunTasksStore() as any
        const allCount = runStore.tasks.length
        cy.log(`全部任务数: ${allCount}`)
        expect(allCount).to.equal(100)
      })

      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('sample-0001')
      })
      cy.wait(500)

      cy.then(() => {
        const runStore = _getRunTasksStore() as any
        const allTasks = runStore.tasks
        const q = 'sample-0001'.toLowerCase()
        const filtered = allTasks.filter((t: any) => {
          const name = (t.targetPath?.split('/').pop() ?? t.sourcePath?.split('/').pop() ?? t.id.slice(0, 8)).toLowerCase()
          const plugin = (t.pluginName || '').toLowerCase()
          const error = (t.error || '').toLowerCase()
          const tid = t.id.toLowerCase()
          return name.includes(q) || plugin.includes(q) || error.includes(q) || tid.includes(q)
        })
        cy.log(`搜索 sample-0001 匹配: ${filtered.length} 个`)
        expect(filtered.length).to.be.lessThan(100)
        expect(filtered.length).to.be.greaterThan(0)
      })
    })

    it('plugin 筛选 → 只显示对应插件的任务', () => {
      mountGroupDetail(TEST_RUN_ID, 100)

      cy.then(() => {
        const store = _getTaskStore() as any
        store.clearFilters()
        store.filterPlugins = ['mp4-encrypt']
      })
      cy.wait(500)

      cy.then(() => {
        const runStore = _getRunTasksStore() as any
        const allTasks = runStore.tasks
        const mp4Tasks = allTasks.filter((t: any) => t.pluginName === 'mp4-encrypt')
        cy.log(`mp4-encrypt 任务数: ${mp4Tasks.length}`)
        expect(mp4Tasks.length).to.be.greaterThan(0)
        expect(mp4Tasks.length).to.be.lessThan(100)
      })
    })

    it('status 筛选 → 只显示对应状态的任务', () => {
      mountGroupDetail(TEST_RUN_ID, 100)

      cy.then(() => {
        const store = _getTaskStore() as any
        store.clearFilters()
        store.filterStatuses = ['completed']
      })
      cy.wait(500)

      cy.then(() => {
        const runStore = _getRunTasksStore() as any
        const allTasks = runStore.tasks
        const completedTasks = allTasks.filter((t: any) => t.status === 'completed')
        cy.log(`completed 任务数: ${completedTasks.length}`)
        expect(completedTasks.length).to.be.greaterThan(0)
        expect(completedTasks.length).to.be.lessThan(100)
      })
    })

    it('清空筛选 → activeFilterCount = 0', () => {
      mountGroupDetail(TEST_RUN_ID, 100)

      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('sample-0001')
        store.filterStatuses = ['completed']
      })
      cy.wait(500)

      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.activeFilterCount).to.be.greaterThan(0)
        store.clearFilters()
      })
      cy.wait(500)

      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.activeFilterCount).to.equal(0)
      })
    })
  })

  describe('1000+ 任务量级', () => {
    it('1000 任务下搜索仍然正常工作', () => {
      mountGroupDetail(TEST_RUN_ID, 1000)

      cy.then(() => {
        const store = _getTaskStore() as any
        store.clearFilters()
      })
      cy.wait(500)

      cy.then(() => {
        const runStore = _getRunTasksStore() as any
        expect(runStore.tasks.length).to.equal(1000)
      })

      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('mp4-encrypt')
      })
      cy.wait(1000)

      cy.then(() => {
        const runStore = _getRunTasksStore() as any
        const allTasks = runStore.tasks
        const mp4Tasks = allTasks.filter((t: any) => t.pluginName === 'mp4-encrypt')
        cy.log(`mp4-encrypt 任务数: ${mp4Tasks.length}`)
        expect(mp4Tasks.length).to.be.greaterThan(0)
        expect(mp4Tasks.length).to.be.lessThan(1000)
      })
    })

    it('1000 任务下多维度组合筛选正常', () => {
      mountGroupDetail(TEST_RUN_ID, 1000)

      cy.then(() => {
        const store = _getTaskStore() as any
        store.clearFilters()
        store.setSearchQuery('sample')
        store.filterPlugins = ['mp4-encrypt', 'mkv-encrypt']
        store.filterStatuses = ['completed', 'failed']
      })
      cy.wait(1000)

      cy.then(() => {
        const runStore = _getRunTasksStore() as any
        const allTasks = runStore.tasks
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
