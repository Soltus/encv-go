/**
 * TaskDebugPanel 组件测试
 *
 * 覆盖场景：
 * - 调试面板展开/收起
 * - 显示统计信息（tasks 总数 / group 数 / displayedItems）
 * - 伪 group 告警
 * - 自我诊断功能
 */
import { defineComponent } from 'vue/dist/vue.esm-bundler.js'
import TaskDebugPanel from '../../src/components/tasks/TaskDebugPanel.vue'
import { taskFixtures } from '../support/task-test-helpers'

describe('TaskDebugPanel', () => {
  function mountWithTasks(count: number, opts: any = {}) {
    const tasks = taskFixtures.generateBatch(count, opts)
    const displayedItems = tasks.map((t, i) => ({ kind: 'task', id: t.id, index: i, task: t }))
    const groupedTasksByRunId = opts.runId
      ? [{ key: opts.runId, runId: opts.runId, tasks, startedAt: tasks[0]?.createdAt || new Date().toISOString() }]
      : tasks.map((t) => ({
          key: t.runId,
          runId: t.runId,
          tasks: [t],
          startedAt: t.createdAt,
        }))

    return cy.mount(TaskDebugPanel as any, {
      props: {
        tasks,
        displayedItems,
        groupedTasksByRunId,
        viewMode: 'group',
        sortBy: 'activity',
        searchQuery: '',
        filterPlugins: [],
        filterTypes: [],
        filterStatuses: [],
        filterTriggeredBy: [],
        filterDatePreset: 'all',
        pinnedRunIds: new Set(),
        defaultOpen: true,
      },
    })
  }

  it('渲染调试面板标题', () => {
    mountWithTasks(10)
    cy.contains('任务诊断面板').should('exist')
  })

  it('显示 tasks 总数', () => {
    mountWithTasks(42)
    cy.contains('42 task').should('exist')
  })

  it('显示 group 数量', () => {
    mountWithTasks(5, { runId: 'run-single' })
    cy.contains('1 真 group').should('exist')
  })

  it('没有伪 group 时显示正常状态', () => {
    mountWithTasks(10, { runId: 'run-test' })
    cy.contains('✅ 无逃逸 task').should('exist')
  })

  it('有伪 group 时显示告警', () => {
    // 每个 task 独立 runId → 全部是伪 group？不，__manual__ 才是伪 group
    // 让我们构造一些 __manual__ runId 的任务
    const tasks = taskFixtures.generateBatch(5, { runId: '__manual__test' })
    const displayedItems = tasks.map((t, i) => ({ kind: 'task', id: t.id, index: i, task: t }))
    const groupedTasksByRunId = [
      { key: '__manual__test', runId: '__manual__test', tasks, startedAt: tasks[0].createdAt },
    ]

    cy.mount(TaskDebugPanel as any, {
      props: {
        tasks,
        displayedItems,
        groupedTasksByRunId,
        viewMode: 'group',
        sortBy: 'activity',
        searchQuery: '',
        filterPlugins: [],
        filterTypes: [],
        filterStatuses: [],
        filterTriggeredBy: [],
        filterDatePreset: 'all',
        pinnedRunIds: new Set(),
        defaultOpen: true,
      },
    })

    cy.contains('伪 group').should('exist')
    cy.contains('5 个 task 失去 runId').should('exist')
  })

  it('显示 displayedItems 数量', () => {
    mountWithTasks(20)
    cy.contains('displayedItems: 20').should('exist')
  })
})
