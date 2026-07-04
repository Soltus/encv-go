/**
 * TaskBasicInfo 组件测试
 *
 * 覆盖场景：
 * - 层级面包屑渲染（root / trigger / run / section / task）
 * - user 触发的任务不显示 trigger 层
 * - automation / ai_agent 触发的任务显示 trigger 层
 * - 有 runId 的任务显示 workflow run 层
 * - 插件/section 信息展示
 * - crypto params 区块（cipherMode / compressionMode / extraFields）
 * - extraFields 显示（已知 key i18n / 未知 key Title Case / bool 转换 / 长字符串截断）
 */
import { defineComponent } from 'vue/dist/vue.esm-bundler.js'
import TaskBasicInfo from '../../src/components/TaskBasicInfo.vue'
import type { EncvTask } from '../../src/api/encv'

function makeTask(overrides: Partial<EncvTask> = {}): EncvTask {
  return {
    id: 'task-001',
    type: 'encrypt',
    sourcePath: '/storage/emulated/0/sample.mp4',
    status: 'completed',
    progress: 100,
    phase: 'completed',
    createdAt: '2026-06-18T10:00:00Z',
    completedAt: '2026-06-18T10:01:00Z',
    pluginName: 'mp4-plugin',
    containerVersion: 4,
    triggeredBy: 'user',
    ...overrides,
  }
}

describe('TaskBasicInfo', () => {
  // ==========================================================================
  // 层级面包屑
  // ==========================================================================
  describe('层级面包屑', () => {
    it('渲染根层（任务中心）', () => {
      cy.mount(TaskBasicInfo as any, { props: { task: makeTask() } })
      cy.contains('任务中心').should('exist')
    })

    it('user 触发的任务不显示 trigger 层', () => {
      cy.mount(TaskBasicInfo as any, { props: { task: makeTask({ triggeredBy: 'user' }) } })
      cy.get('.breadcrumb-trigger').should('not.exist')
    })

    it('automation 触发的任务显示自动化触发层', () => {
      cy.mount(TaskBasicInfo as any, { props: { task: makeTask({ triggeredBy: 'automation' }) } })
      cy.get('.breadcrumb-trigger').should('exist')
      cy.contains('自动化').should('exist')
    })

    it('ai_agent 触发的任务显示 AI 代理层', () => {
      cy.mount(TaskBasicInfo as any, { props: { task: makeTask({ triggeredBy: 'ai_agent' }) } })
      cy.get('.breadcrumb-trigger').should('exist')
      cy.contains('AI').should('exist')
    })

    it('有 runId 的任务显示 workflow run 层', () => {
      cy.mount(TaskBasicInfo as any, { props: { task: makeTask({ runId: 'run-test-123456' }) } })
      cy.get('.breadcrumb-run').should('exist')
      cy.contains('#run-test').should('exist')
    })

    it('显示 section 层（插件信息）', () => {
      cy.mount(TaskBasicInfo as any, { props: { task: makeTask({ pluginName: 'mp4-plugin' }) } })
      cy.get('.breadcrumb-section').should('exist')
    })

    it('显示 task 层（任务 ID）', () => {
      cy.mount(TaskBasicInfo as any, { props: { task: makeTask({ id: 'task-abcdef1234' }) } })
      cy.get('.breadcrumb-task').should('exist')
      cy.contains('#task-abc').should('exist')
    })
  })

  // ==========================================================================
  // crypto params 区块
  // ==========================================================================
  describe('crypto params 区块', () => {
    it('有 cipherMode + compressionMode 时显示加解密参数区块', () => {
      const task = makeTask({
        cipherMode: 1,
        compressionMode: 'zstd',
      })
      cy.mount(TaskBasicInfo as any, { props: { task } })
      cy.contains('加解密参数').should('exist')
    })

    it('旧任务无 crypto 字段时不显示加解密参数区块', () => {
      cy.mount(TaskBasicInfo as any, { props: { task: makeTask() } })
      cy.contains('加解密参数').should('not.exist')
    })

    it('仅 extraFields 存在时也显示加解密参数区块', () => {
      const task = makeTask({
        extraFields: { customParam: 'value123' },
      })
      cy.mount(TaskBasicInfo as any, { props: { task } })
      cy.contains('加解密参数').should('exist')
    })

    it('cipherMode=1 显示 AES-256 badge', () => {
      const task = makeTask({ cipherMode: 1 })
      cy.mount(TaskBasicInfo as any, { props: { task } })
      cy.contains('AES-256').should('exist')
    })

    it('cipherMode=0 显示 AES-128 badge', () => {
      const task = makeTask({ cipherMode: 0 })
      cy.mount(TaskBasicInfo as any, { props: { task } })
      cy.contains('AES-128').should('exist')
    })

    it('compressionMode=zstd 显示 Zstd badge', () => {
      const task = makeTask({ compressionMode: 'zstd' })
      cy.mount(TaskBasicInfo as any, { props: { task } })
      cy.contains('Zstd').should('exist')
    })

    it('compressionMode=none 显示"无压缩" badge', () => {
      const task = makeTask({ compressionMode: 'none' })
      cy.mount(TaskBasicInfo as any, { props: { task } })
      cy.contains('无压缩').should('exist')
    })
  })

  // ==========================================================================
  // extraFields
  // ==========================================================================
  describe('extraFields', () => {
    it('extraFields 迭代显示每个 key-value 对', () => {
      const task = makeTask({
        cipherMode: 1,
        extraFields: {
          fnRounds: '8',
          fnCharset: 'base64',
        },
      })
      cy.mount(TaskBasicInfo as any, { props: { task } })
      cy.contains('8').should('exist')
      cy.contains('base64').should('exist')
    })

    it('extraFields 已知 key (fnRounds) 走 i18n 显示中文标签', () => {
      const task = makeTask({
        extraFields: { fnRounds: '8' },
      })
      cy.mount(TaskBasicInfo as any, { props: { task } })
      cy.contains('Feistel').should('exist')
    })

    it('extraFields 未知 key 退化到 Title Case', () => {
      const task = makeTask({
        extraFields: { some_custom_field: 'abc' },
      })
      cy.mount(TaskBasicInfo as any, { props: { task } })
      cy.contains('Some Custom Field').should('exist')
    })

    it('extraFields bool 值 "true" → ✓', () => {
      const task = makeTask({
        extraFields: { encryptFilename: 'true' },
      })
      cy.mount(TaskBasicInfo as any, { props: { task } })
      cy.contains('✓').should('exist')
    })

    it('extraFields bool 值 "false" → ✗', () => {
      const task = makeTask({
        extraFields: { encryptFilename: 'false' },
      })
      cy.mount(TaskBasicInfo as any, { props: { task } })
      cy.contains('✗').should('exist')
    })

    it('extraFields 长字符串（>32 字符）截断显示', () => {
      const longValue = 'a'.repeat(50)
      const task = makeTask({
        extraFields: { longParam: longValue },
      })
      cy.mount(TaskBasicInfo as any, { props: { task } })
      // 截断格式：前 8 + … + 后 4
      cy.contains('aaaaaaaa…aaaa').should('exist')
    })
  })
})
