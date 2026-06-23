import Tasks from '../../src/views/Tasks.vue'
import { taskFixtures } from '../support/task-test-helpers'
import { _getTaskStore } from '../support/store-helpers'

function mountWithTasks(count = 200) {
  cy.mount(Tasks as any)
  cy.wait(3000)
  const tasks = taskFixtures.generateMixedBatch(count, {
    plugins: ['mp4-encrypt', 'mkv-encrypt', 'png-encrypt', 'pdf-encrypt', 'mp3-encrypt'],
    runId: 'run-filter-test',
    triggeredBy: 'automation',
  })
  cy.then(() => {
    const store = _getTaskStore() as any
    store.bulkSetTasks(tasks)
    store.viewMode = 'flat'
    store.clearFilters()
  })
  cy.wait(1000)
}

describe('Tasks 搜索筛选功能', () => {
  beforeEach(() => {
    cy.intercept('GET', '**/api/tasks*', { body: [], statusCode: 200 }).as('getTasks')
    cy.intercept('GET', '**/api/runs/summary*', { body: {}, statusCode: 200 }).as('getSummary')
    cy.intercept('DELETE', '**/api/tasks*', { body: { removed: 0 }, statusCode: 200 }).as('deleteTasks')
  })

  describe('搜索功能', () => {
    it('初始状态：搜索为空，显示全部任务', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.searchQuery).to.equal('')
        expect(store.filteredTasks.length).to.equal(200)
      })
    })

    it('搜索文件名：sample-001 匹配部分任务', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('sample-001')
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        const count = store.filteredTasks.length
        cy.log(`搜索 sample-001 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('搜索 plugin 名：mp4-encrypt 只显示该插件', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('mp4-encrypt')
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        const count = store.filteredTasks.length
        cy.log(`搜索 mp4-encrypt 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('搜索不存在的关键词 → 0 结果', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('nonexistent-keyword-xyz-123')
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.filteredTasks.length).to.equal(0)
      })
    })

    it('清空搜索 → 恢复全部任务', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('sample-001')
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.filteredTasks.length).to.be.lessThan(200)
        store.setSearchQuery('')
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.filteredTasks.length).to.equal(200)
      })
    })
  })

  describe('Plugin 筛选', () => {
    it('筛选单个 plugin：mp4-encrypt', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.filterPlugins = ['mp4-encrypt']
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        const count = store.filteredTasks.length
        cy.log(`plugin=mp4-encrypt 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('筛选多个 plugin：mp4 + mkv', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.filterPlugins = ['mp4-encrypt', 'mkv-encrypt']
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        const count = store.filteredTasks.length
        cy.log(`plugin=mp4+mkv 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('plugin 筛选 + activeFilterCount 正确', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.activeFilterCount).to.equal(0)
        store.filterPlugins = ['mp4-encrypt']
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.activeFilterCount).to.equal(1)
      })
    })
  })

  describe('Type 筛选', () => {
    it('筛选 encrypt 类型', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.filterTypes = ['encrypt']
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        const count = store.filteredTasks.length
        cy.log(`type=encrypt 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
      })
    })
  })

  describe('Status 筛选', () => {
    it('筛选 completed 状态', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.filterStatuses = ['completed']
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        const count = store.filteredTasks.length
        cy.log(`status=completed 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('筛选 failed 状态', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.filterStatuses = ['failed']
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        const count = store.filteredTasks.length
        cy.log(`status=failed 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('筛选多个状态：completed + failed', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.filterStatuses = ['completed', 'failed']
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        const count = store.filteredTasks.length
        cy.log(`status=completed+failed 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })
  })

  describe('组合筛选', () => {
    it('搜索 + plugin 筛选：交集正确', () => {
      mountWithTasks(200)
      let searchOnly = 0
      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('sample-00')
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        searchOnly = store.filteredTasks.length
        cy.log(`仅搜索匹配: ${searchOnly} 个`)
        store.filterPlugins = ['mp4-encrypt']
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        const combined = store.filteredTasks.length
        cy.log(`搜索+plugin 匹配: ${combined} 个`)
        expect(combined).to.be.lessThan(searchOnly)
      })
    })

    it('plugin + status 筛选：交集正确', () => {
      mountWithTasks(200)
      let pluginOnly = 0
      cy.then(() => {
        const store = _getTaskStore() as any
        store.filterPlugins = ['mp4-encrypt']
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        pluginOnly = store.filteredTasks.length
        cy.log(`仅 plugin 匹配: ${pluginOnly} 个`)
        store.filterStatuses = ['completed']
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        const combined = store.filteredTasks.length
        cy.log(`plugin+status 匹配: ${combined} 个`)
        expect(combined).to.be.lessThan(pluginOnly)
      })
    })

    it('三重组合：搜索 + plugin + status', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('sample')
        store.filterPlugins = ['mp4-encrypt']
        store.filterStatuses = ['completed']
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        const count = store.filteredTasks.length
        cy.log(`三重组合匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('activeFilterCount 正确统计所有活跃筛选', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.activeFilterCount).to.equal(0)
        store.setSearchQuery('test')
        store.filterPlugins = ['mp4-encrypt']
        store.filterStatuses = ['running']
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.activeFilterCount).to.equal(3)
      })
    })
  })

  describe('清空筛选', () => {
    it('clearFilters 清空所有筛选', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.setSearchQuery('test')
        store.filterPlugins = ['mp4-encrypt']
        store.filterStatuses = ['running']
        store.filterTypes = ['encrypt']
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.activeFilterCount).to.equal(4)
        expect(store.filteredTasks.length).to.be.lessThan(200)
        store.clearFilters()
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.activeFilterCount).to.equal(0)
        expect(store.filteredTasks.length).to.equal(200)
      })
    })
  })

  describe('Group 模式筛选', () => {
    it('group 模式下搜索筛选也生效', () => {
      mountWithTasks(200)
      cy.then(() => {
        const store = _getTaskStore() as any
        store.viewMode = 'group'
      })
      cy.wait(1000)
      cy.then(() => {
        const store = _getTaskStore() as any
        expect(store.tasks.length).to.equal(200)
        store.setSearchQuery('mp4-encrypt')
      })
      cy.wait(300)
      cy.then(() => {
        const store = _getTaskStore() as any
        const count = store.filteredTasks.length
        cy.log(`group 模式下搜索 mp4-encrypt 匹配: ${count} 个`)
        expect(count).to.be.lessThan(200)
      })
    })
  })
})
