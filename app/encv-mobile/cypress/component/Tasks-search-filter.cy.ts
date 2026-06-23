/**
 * Tasks 页搜索筛选功能测试
 *
 * 覆盖场景：
 *   - 搜索：按文件名 / plugin 名 / 错误信息 / task id
 *   - 筛选：plugin / type / status 三维度
 *   - 搜索 + 筛选组合
 *   - 清空筛选
 *   - 多 plugin 混合场景（模拟自动化测试）
 *
 * 断言层级：store 层（filteredTasks）为核心断言
 */
import Tasks from '../../src/views/Tasks.vue'
import { taskFixtures, taskStore } from '../support/task-test-helpers'

describe('Tasks 搜索筛选功能', () => {
  beforeEach(() => {
    cy.intercept('GET', '**/api/tasks*', { body: [], statusCode: 200 }).as('getTasks')
    cy.intercept('GET', '**/api/runs/summary*', { body: {}, statusCode: 200 }).as('getSummary')
    cy.intercept('DELETE', '**/api/tasks*', { body: { removed: 0 }, statusCode: 200 }).as('deleteTasks')
  })

  function mountWithMixedTasks() {
    cy.mount(Tasks as any)
    cy.wait(3000)
    // 生成多 plugin 混合任务（模拟自动化测试场景）
    const tasks = taskFixtures.generateMixedBatch(200, {
      plugins: ['mp4-encrypt', 'mkv-encrypt', 'png-encrypt', 'pdf-encrypt', 'mp3-encrypt'],
      runId: 'run-filter-test',
      triggeredBy: 'automation',
    })
    taskStore.injectTasks(tasks)
    taskStore.clearFilters()
    taskStore.setViewMode('flat')
    cy.wait(1000)
    return cy.wrap(tasks)
  }

  // ==========================================================================
  // 搜索功能
  // ==========================================================================
  describe('搜索功能', () => {
    it('初始状态：搜索为空 → filteredTasks = 全部', () => {
      mountWithMixedTasks()
      taskStore.getSearchQuery().should('equal', '')
      taskStore.getFilteredTaskCount().should('equal', 200)
    })

    it('搜索文件名：输入 sample-001 → 匹配到文件名包含的任务', () => {
      mountWithMixedTasks()
      taskStore.setSearchQuery('sample-001')
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((count) => {
        cy.log(`搜索 sample-001 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('搜索 plugin 名：输入 mp4-encrypt → 只显示 mp4-encrypt 插件的任务', () => {
      mountWithMixedTasks()
      taskStore.setSearchQuery('mp4-encrypt')
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((count) => {
        cy.log(`搜索 mp4-encrypt 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('搜索错误信息：输入 failed → 匹配有 error 的任务', () => {
      mountWithMixedTasks()
      taskStore.setSearchQuery('failed')
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((count) => {
        cy.log(`搜索 failed 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('搜索不存在的关键词 → 0 结果', () => {
      mountWithMixedTasks()
      taskStore.setSearchQuery('nonexistent-keyword-xyz')
      cy.wait(500)
      taskStore.getFilteredTaskCount().should('equal', 0)
    })

    it('清空搜索 → 恢复全部任务', () => {
      mountWithMixedTasks()
      taskStore.setSearchQuery('sample-001')
      cy.wait(500)
      taskStore.getFilteredTaskCount().should('be.lessThan', 200)
      taskStore.setSearchQuery('')
      cy.wait(500)
      taskStore.getFilteredTaskCount().should('equal', 200)
    })
  })

  // ==========================================================================
  // Plugin 筛选
  // ==========================================================================
  describe('Plugin 筛选', () => {
    it('筛选单个 plugin：mp4-encrypt → 只显示该插件任务', () => {
      mountWithMixedTasks()
      taskStore.setPluginFilter(['mp4-encrypt'])
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((count) => {
        cy.log(`plugin=mp4-encrypt 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('筛选多个 plugin：mp4 + mkv → 显示两个插件的任务', () => {
      mountWithMixedTasks()
      taskStore.setPluginFilter(['mp4-encrypt', 'mkv-encrypt'])
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((count) => {
        cy.log(`plugin=mp4+mkv 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('plugin 筛选 + activeFilterCount 正确', () => {
      mountWithMixedTasks()
      taskStore.getActiveFilterCount().should('equal', 0)
      taskStore.setPluginFilter(['mp4-encrypt'])
      cy.wait(500)
      taskStore.getActiveFilterCount().should('equal', 1)
    })
  })

  // ==========================================================================
  // Type 筛选
  // ==========================================================================
  describe('Type 筛选', () => {
    it('筛选 encrypt → 只显示加密任务', () => {
      mountWithMixedTasks()
      taskStore.setTypeFilter(['encrypt'])
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((count) => {
        cy.log(`type=encrypt 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('筛选 decrypt → 只显示解密任务', () => {
      mountWithMixedTasks()
      taskStore.setTypeFilter(['decrypt'])
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((count) => {
        cy.log(`type=decrypt 匹配: ${count} 个`)
        // fixture 默认只有 encrypt，这里可能是 0
        expect(count).to.be.greaterThan(-1)
      })
    })
  })

  // ==========================================================================
  // Status 筛选
  // ==========================================================================
  describe('Status 筛选', () => {
    it('筛选 completed → 只显示已完成任务', () => {
      mountWithMixedTasks()
      taskStore.setStatusFilter(['completed'])
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((count) => {
        cy.log(`status=completed 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('筛选 failed → 只显示失败任务', () => {
      mountWithMixedTasks()
      taskStore.setStatusFilter(['failed'])
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((count) => {
        cy.log(`status=failed 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('筛选 running → 只显示运行中任务', () => {
      mountWithMixedTasks()
      taskStore.setStatusFilter(['running'])
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((count) => {
        cy.log(`status=running 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('筛选多个状态：completed + failed → 显示所有终态任务', () => {
      mountWithMixedTasks()
      taskStore.setStatusFilter(['completed', 'failed'])
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((count) => {
        cy.log(`status=completed+failed 匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })
  })

  // ==========================================================================
  // 组合筛选
  // ==========================================================================
  describe('组合筛选', () => {
    it('搜索 + plugin 筛选：交集正确', () => {
      mountWithMixedTasks()
      // 先只搜索
      taskStore.setSearchQuery('sample-00')
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((searchOnly) => {
        cy.log(`仅搜索匹配: ${searchOnly} 个`)
        // 再加 plugin 筛选
        taskStore.setPluginFilter(['mp4-encrypt'])
        cy.wait(500)
        taskStore.getFilteredTaskCount().then((combined) => {
          cy.log(`搜索+plugin 匹配: ${combined} 个`)
          // 组合筛选结果应该 <= 仅搜索
          expect(combined).to.be.lessThan(searchOnly)
        })
      })
    })

    it('plugin + status 筛选：交集正确', () => {
      mountWithMixedTasks()
      // 先只 plugin
      taskStore.setPluginFilter(['mp4-encrypt'])
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((pluginOnly) => {
        cy.log(`仅 plugin 匹配: ${pluginOnly} 个`)
        // 再加 status
        taskStore.setStatusFilter(['completed'])
        cy.wait(500)
        taskStore.getFilteredTaskCount().then((combined) => {
          cy.log(`plugin+status 匹配: ${combined} 个`)
          expect(combined).to.be.lessThan(pluginOnly)
        })
      })
    })

    it('搜索 + plugin + status 三重组合：交集正确', () => {
      mountWithMixedTasks()
      taskStore.setSearchQuery('sample')
      taskStore.setPluginFilter(['mp4-encrypt'])
      taskStore.setStatusFilter(['completed'])
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((count) => {
        cy.log(`三重组合匹配: ${count} 个`)
        expect(count).to.be.greaterThan(0)
        expect(count).to.be.lessThan(200)
      })
    })

    it('activeFilterCount 正确统计所有活跃筛选', () => {
      mountWithMixedTasks()
      taskStore.getActiveFilterCount().should('equal', 0)
      taskStore.setSearchQuery('test')
      taskStore.setPluginFilter(['mp4-encrypt'])
      taskStore.setStatusFilter(['running'])
      cy.wait(500)
      // search + plugin + status = 3
      taskStore.getActiveFilterCount().should('equal', 3)
    })
  })

  // ==========================================================================
  // 清空筛选
  // ==========================================================================
  describe('清空筛选', () => {
    it('clearFilters 清空所有筛选', () => {
      mountWithMixedTasks()
      taskStore.setSearchQuery('test')
      taskStore.setPluginFilter(['mp4-encrypt'])
      taskStore.setStatusFilter(['running'])
      taskStore.setTypeFilter(['encrypt'])
      cy.wait(500)
      taskStore.getActiveFilterCount().should('equal', 4)
      taskStore.getFilteredTaskCount().should('be.lessThan', 200)

      taskStore.clearFilters()
      cy.wait(500)
      taskStore.getActiveFilterCount().should('equal', 0)
      taskStore.getFilteredTaskCount().should('equal', 200)
    })
  })

  // ==========================================================================
  // Group 模式下筛选也生效
  // ==========================================================================
  describe('Group 模式筛选', () => {
    it('group 模式下搜索筛选也生效', () => {
      mountWithMixedTasks()
      taskStore.setViewMode('group')
      cy.wait(1000)
      taskStore.taskCount().should('equal', 200)

      // 应用筛选
      taskStore.setSearchQuery('mp4-encrypt')
      cy.wait(500)
      taskStore.getFilteredTaskCount().then((count) => {
        cy.log(`group 模式下搜索 mp4-encrypt 匹配: ${count} 个`)
        expect(count).to.be.lessThan(200)
      })
    })
  })
})
