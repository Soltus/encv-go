/**
 * TaskTimeline 组件测试
 *
 * 覆盖场景：
 * - 基础渲染：section title + 卡片数量
 * - steps 模式 vs fallback 模式
 * - 当前 step 显示 progress / speed / eta
 * - 已完成 step 显示 duration
 * - 最长耗时 step 高亮
 * - 状态色（running 蓝 / success 绿 / failure 红）
 * - 展开详情卡片（sourcePath / outputPath / crypto / error）
 * - completed 态追加完成事件
 * - failed / cancelled 态错误处理
 */
import { defineComponent } from 'vue/dist/vue.esm-bundler.js'
import TaskTimeline from '../../src/components/TaskTimeline.vue'
import type { EncvTask, TaskStep } from '../../src/api/encv'

function makeStep(overrides: Partial<TaskStep> = {}): TaskStep {
  return {
    phase: 'encrypting',
    startedAt: '2026-06-18T10:00:00Z',
    completedAt: '2026-06-18T10:00:05Z',
    detail: '/storage/emulated/0/output.encv',
    ...overrides,
  }
}

function makeTask(overrides: Partial<EncvTask> = {}): EncvTask {
  return {
    id: 'task-1',
    type: 'encrypt',
    sourcePath: '/storage/emulated/0/sample.mp4',
    status: 'running',
    progress: 50,
    phase: 'encrypting',
    speed: '12.5 MB/s',
    eta: '00:01:30',
    createdAt: '2026-06-18T10:00:00Z',
    steps: [],
    ...overrides,
  }
}

describe('TaskTimeline', () => {
  // ==========================================================================
  // 基础渲染
  // ==========================================================================
  describe('基础渲染', () => {
    it('渲染 section-title 为"时间线"', () => {
      cy.mount(TaskTimeline as any, { props: { task: makeTask() } })
      cy.get('.section-title').should('contain.text', '时间线')
    })

    it('task 有 steps 时渲染对应数量卡片（created + steps + completed）', () => {
      const task = makeTask({
        status: 'completed',
        phase: 'completed',
        completedAt: '2026-06-18T10:01:00Z',
        steps: [
          makeStep({ phase: 'analyzing', startedAt: '2026-06-18T10:00:00Z', completedAt: '2026-06-18T10:00:01Z' }),
          makeStep({ phase: 'encrypting', startedAt: '2026-06-18T10:00:01Z', completedAt: '2026-06-18T10:00:05Z' }),
        ],
      })
      cy.mount(TaskTimeline as any, { props: { task } })
      // created + 2 steps + completed = 4
      cy.get('.utc').should('have.length', 4)
    })

    it('task.steps 为空时 fallback 到 phase 序列', () => {
      const task = makeTask({ steps: undefined, phase: 'encrypting' })
      cy.mount(TaskTimeline as any, { props: { task } })
      // fallback phase 数 + created
      cy.get('.utc').should('have.length.greaterThan', 5)
    })
  })

  // ==========================================================================
  // 进度 / 速率 / ETA
  // ==========================================================================
  describe('进度 / 速率 / ETA', () => {
    it('当前 running step 显示进度条', () => {
      const task = makeTask({
        status: 'running',
        phase: 'encrypting',
        progress: 65,
        steps: [
          makeStep({
            phase: 'encrypting',
            startedAt: '2026-06-18T10:00:00Z',
            completedAt: undefined,
          }),
        ],
      })
      cy.mount(TaskTimeline as any, { props: { task } })
      cy.get('.utc').eq(1).find('.utc__progress-bar').should('exist')
    })

    it('当前 step 显示 speed', () => {
      const task = makeTask({
        status: 'running',
        phase: 'encrypting',
        speed: '12.5 MB/s',
        steps: [
          makeStep({
            phase: 'encrypting',
            startedAt: '2026-06-18T10:00:00Z',
            completedAt: undefined,
          }),
        ],
      })
      cy.mount(TaskTimeline as any, { props: { task } })
      cy.get('.utc').eq(1).should('contain.text', '12.5 MB/s')
    })
  })

  // ==========================================================================
  // 耗时跨度 + 最长耗时高亮
  // ==========================================================================
  describe('耗时 + 高亮', () => {
    it('已完成 step 在 header 右侧显示 duration', () => {
      const task = makeTask({
        status: 'running',
        phase: 'encrypting',
        steps: [
          makeStep({
            phase: 'analyzing',
            startedAt: '2026-06-18T10:00:00Z',
            completedAt: '2026-06-18T10:00:05Z',
          }),
        ],
      })
      cy.mount(TaskTimeline as any, { props: { task } })
      cy.get('.utc').eq(1).find('.utc__duration').should('contain.text', '5s')
    })

    it('最长耗时的 step 有 utc--highlight class', () => {
      const task = makeTask({
        status: 'completed',
        phase: 'completed',
        completedAt: '2026-06-18T10:00:30Z',
        steps: [
          makeStep({
            phase: 'analyzing',
            startedAt: '2026-06-18T10:00:00Z',
            completedAt: '2026-06-18T10:00:02Z', // 2s
          }),
          makeStep({
            phase: 'encrypting',
            startedAt: '2026-06-18T10:00:02Z',
            completedAt: '2026-06-18T10:00:30Z', // 28s ← 最长
          }),
        ],
      })
      cy.mount(TaskTimeline as any, { props: { task } })
      // 第二个 step（encrypting）应该有 highlight class
      cy.get('.utc').eq(2).should('have.class', 'utc--highlight')
    })
  })

  // ==========================================================================
  // 状态色
  // ==========================================================================
  describe('状态色', () => {
    it('running 状态有 utc--running class', () => {
      const task = makeTask({
        status: 'running',
        phase: 'encrypting',
        steps: [
          makeStep({
            phase: 'encrypting',
            startedAt: '2026-06-18T10:00:00Z',
            completedAt: undefined,
          }),
        ],
      })
      cy.mount(TaskTimeline as any, { props: { task } })
      cy.get('.utc').eq(1).should('have.class', 'utc--running')
    })

    it('已完成 step 有 utc--success class', () => {
      const task = makeTask({
        status: 'running',
        phase: 'encrypting',
        steps: [
          makeStep({
            phase: 'analyzing',
            startedAt: '2026-06-18T10:00:00Z',
            completedAt: '2026-06-18T10:00:01Z',
          }),
        ],
      })
      cy.mount(TaskTimeline as any, { props: { task } })
      cy.get('.utc').eq(1).should('have.class', 'utc--success')
    })

    it('失败任务最后一个 step 有 utc--failure class', () => {
      const task = makeTask({
        status: 'failed',
        phase: 'encrypting',
        error: 'FFMPEG exited with code 1',
        steps: [
          makeStep({
            phase: 'encrypting',
            startedAt: '2026-06-18T10:00:00Z',
            completedAt: undefined,
          }),
        ],
      })
      cy.mount(TaskTimeline as any, { props: { task } })
      cy.get('.utc').last().should('have.class', 'utc--failure')
    })
  })

  // ==========================================================================
  // 展开详情
  // ==========================================================================
  describe('展开详情', () => {
    it('点击可展开的 header 能展开详情内容', () => {
      const task = makeTask({
        status: 'completed',
        phase: 'completed',
        outputPath: '/d/primary/output.encv',
        completedAt: '2026-06-18T10:00:30Z',
        steps: [
          makeStep({
            phase: 'encrypting',
            startedAt: '2026-06-18T10:00:00Z',
            completedAt: '2026-06-18T10:00:30Z',
            detail: '/d/primary/output.encv',
          }),
        ],
      })
      cy.mount(TaskTimeline as any, { props: { task } })
      // 初始未展开
      cy.get('.utc__detail').should('not.exist')
      // 点击第二个卡片（encrypting step）的 header 展开
      cy.get('.utc').eq(1).find('.utc__header').click()
      // 展开后显示详情
      cy.get('.utc__detail').should('exist')
    })

    it('展开后显示 outputPath 路径卡片', () => {
      const task = makeTask({
        status: 'completed',
        phase: 'completed',
        outputPath: '/d/primary/output.encv',
        completedAt: '2026-06-18T10:00:30Z',
        steps: [
          makeStep({
            phase: 'encrypting',
            startedAt: '2026-06-18T10:00:00Z',
            completedAt: '2026-06-18T10:00:30Z',
            detail: '/d/primary/output.encv',
          }),
        ],
      })
      cy.mount(TaskTimeline as any, { props: { task } })
      cy.get('.utc').eq(1).find('.utc__header').click()
      cy.get('.timeline-detail-card--path').should('contain.text', '/d/primary/output.encv')
    })

    it('失败任务展开显示错误信息', () => {
      const task = makeTask({
        status: 'failed',
        phase: 'encrypting',
        error: 'FFMPEG exited with code 1',
        steps: [
          makeStep({
            phase: 'encrypting',
            startedAt: '2026-06-18T10:00:00Z',
            completedAt: undefined,
          }),
        ],
      })
      cy.mount(TaskTimeline as any, { props: { task } })
      cy.get('.utc').last().find('.utc__header').click()
      cy.get('.timeline-detail-card--error').should('contain.text', 'FFMPEG exited with code 1')
    })
  })

  // ==========================================================================
  // crypto params
  // ==========================================================================
  describe('crypto params 摘要', () => {
    it('cipherMode=1 + compressionMode=zstd → created 卡片显示加密摘要', () => {
      const task = makeTask({
        cipherMode: 1,
        compressionMode: 'zstd',
      })
      cy.mount(TaskTimeline as any, { props: { task } })
      // created 卡片的 meta
      cy.get('.utc').first().should('contain.text', 'AES-256')
      cy.get('.utc').first().should('contain.text', 'Zstd')
    })
  })
})
