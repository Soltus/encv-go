import Tasks from '../../src/views/Tasks.vue'
import { taskFixtures, taskStore } from '../support/task-test-helpers'
import { _getTaskStore } from '../support/store-helpers'

function mountAndInject(count: number, opts?: any) {
  cy.intercept('GET', '**/api/tasks*', { body: [], statusCode: 200 }).as('getTasks')
  cy.intercept('GET', '**/api/runs/summary*', { body: {}, statusCode: 200 }).as('getSummary')
  cy.intercept('DELETE', '**/api/tasks*', { body: { removed: 0 }, statusCode: 200 }).as('deleteTasks')
  cy.mount(Tasks as any)
  cy.wait(2000)
  const tasks = taskFixtures.generateMixedBatch(count, {
    plugins: ['mp4-encrypt', 'mkv-encrypt', 'png-encrypt', 'pdf-encrypt', 'mp3-encrypt'],
    runId: 'run-filter-test',
    triggeredBy: 'automation',
    ...opts,
  })
  taskStore.injectTasks(tasks)
  cy.wait(200)
  cy.then(() => {
    const store = _getTaskStore() as any
    store.viewMode = 'flat'
    store.clearFilters()
  })
  cy.wait(100)
}

describe('Tasks 搜索筛选功能', () => {
  describe('搜索功能', () => {
    it('初始状态 + 搜索文件名 + 搜索不存在关键词', () => {
      mountAndInject(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.searchQuery).to.equal('')
        expect(store.filteredTasks.length).to.equal(200)
        store.setSearchQuery('sample-001')
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        const count = store.filteredTasks.length
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
        store.setSearchQuery('nonexistent-keyword-xyz-123')
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.filteredTasks.length).to.equal(0)
      })
    })

    it('搜索 plugin 名 + 清空搜索恢复', () => {
      mountAndInject(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('mp4-encrypt')
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        const count = store.filteredTasks.length
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
        store.setSearchQuery('')
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.filteredTasks.length).to.equal(200)
      })
    })
  })

  describe('筛选功能', () => {
    it('Plugin 筛选：单个 + 多个 + activeFilterCount', () => {
      mountAndInject(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.activeFilterCount).to.equal(0)
        store.filterPlugins = ['mp4-encrypt']
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.activeFilterCount).to.equal(1)
        const single = store.filteredTasks.length
        expect(single).to.be.greaterThan(0)
        expect(single).to.be.lessThan(200)
        store.filterPlugins = ['mp4-encrypt', 'mkv-encrypt']
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        const multi = store.filteredTasks.length
        expect(multi).to.be.greaterThan(0)
        expect(multi).to.be.lessThan(200)
      })
    })

    it('Status 筛选：completed + failed + 多状态', () => {
      mountAndInject(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.filterStatuses = ['completed']
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        const completed = store.filteredTasks.length
        expect(completed).to.be.greaterThan(0)
        expect(completed).to.be.lessThan(200)
        store.filterStatuses = ['failed']
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        const failed = store.filteredTasks.length
        expect(failed).to.be.greaterThan(0)
        expect(failed).to.be.lessThan(200)
        store.filterStatuses = ['completed', 'failed']
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        const both = store.filteredTasks.length
        expect(both).to.be.greaterThan(0)
        expect(both).to.be.lessThan(200)
      })
    })

    it('Type 筛选 + 清空筛选', () => {
      mountAndInject(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.filterTypes = ['encrypt']
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.filteredTasks.length).to.be.greaterThan(0)
        store.setSearchQuery('test')
        store.filterPlugins = ['mp4-encrypt']
        store.filterStatuses = ['running']
        store.filterTypes = ['encrypt']
        expect(store.activeFilterCount).to.equal(4)
        store.clearFilters()
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.activeFilterCount).to.equal(0)
        expect(store.filteredTasks.length).to.equal(200)
      })
    })
  })

  describe('组合筛选', () => {
    it('搜索 + plugin + status 三重组合', () => {
      mountAndInject(200)
      let searchOnly = 0
      let pluginOnly = 0
      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('sample-01')
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        searchOnly = store.filteredTasks.length
        store.filterPlugins = ['mp4-encrypt']
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        pluginOnly = store.filteredTasks.length
        expect(pluginOnly).to.be.lessThan(searchOnly)
        store.filterStatuses = ['completed']
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        const combined = store.filteredTasks.length
        expect(combined).to.be.lessThan(pluginOnly)
        expect(combined).to.be.greaterThan(0)
      })
    })
  })

  describe('Group 模式', () => {
    it('group 模式下搜索筛选也生效', () => {
      mountAndInject(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.viewMode = 'group'
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.tasks.length).to.equal(200)
        store.setSearchQuery('mp4-encrypt')
      })
      cy.wait(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        const count = store.filteredTasks.length
        expect(count).to.be.lessThan(200)
      })
    })
  })
})
