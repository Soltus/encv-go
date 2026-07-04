/**
 * TaskOutputInfo 组件测试
 *
 * 业务价值：任务完成后展示输出文件信息，是用户查看加密结果的核心组件
 *   - completed 状态显示输出文件信息
 *   - 非 completed 状态不显示
 *   - 可预览的视频格式显示"打开"按钮
 *   - 不可预览格式点击打开显示 toast 提示
 *   - "定位到文件"按钮 emit locate 事件
 */
import TaskOutputInfo from '../../src/components/TaskOutputInfo.vue'
import { taskFixtures } from '../support/task-test-helpers'
import type { EncvTask } from '../../src/api/encv'

describe('TaskOutputInfo', () => {
  function makeTask(overrides: Partial<EncvTask> = {}): EncvTask {
    const task = taskFixtures.one(0, { status: 'completed' })
    return { ...task, ...overrides }
  }

  function mountCompleted(outputPath: string, propsOverrides: Record<string, any> = {}) {
    const task = makeTask({
      outputPath,
      createdAt: '2025-01-01T00:00:00Z',
      completedAt: '2025-01-01T00:05:00Z',
    })
    const allProps = { task, ...propsOverrides }
    cy.mount(TaskOutputInfo, { props: allProps })
    return cy.wrap(task)
  }

  // 选择器
  const openBtnSel = '.output-actions ion-button[color="primary"]:not([fill="outline"])'
  const locateBtnSel = '.output-actions ion-button[color="medium"]'

  // ==========================================================================
  // 渲染：按状态显示
  // ==========================================================================
  describe('按状态渲染', () => {
    it('completed 状态 + 有 outputPath → 显示输出信息块', () => {
      mountCompleted('/storage/emulated/0/encrypted/sample.encv')
      cy.get('.output-block').should('exist')
      cy.get('.output-name').should('contain.text', 'sample.encv')
    })

    it('completed 状态 + 无 outputPath → 不显示输出块', () => {
      const task = makeTask({
        status: 'completed',
        outputPath: undefined as any,
      })
      cy.mount(TaskOutputInfo, { props: { task } })
      cy.get('.output-block').should('not.exist')
    })

    it('running 状态 → 不显示 detail-section', () => {
      const task = makeTask({ status: 'running' })
      cy.mount(TaskOutputInfo, { props: { task } })
      cy.get('.detail-section').should('not.exist')
    })

    it('failed 状态 → 不显示 detail-section', () => {
      const task = makeTask({ status: 'failed' })
      cy.mount(TaskOutputInfo, { props: { task } })
      cy.get('.detail-section').should('not.exist')
    })

    it('queued 状态 → 不显示 detail-section', () => {
      const task = makeTask({ status: 'queued' })
      cy.mount(TaskOutputInfo, { props: { task } })
      cy.get('.detail-section').should('not.exist')
    })
  })

  // ==========================================================================
  // 输出信息展示
  // ==========================================================================
  describe('输出信息展示', () => {
    it('显示文件名（从路径提取）', () => {
      mountCompleted('/storage/emulated/0/encrypted/my-video.mp4.encv')
      cy.get('.output-name').should('contain.text', 'my-video.mp4.encv')
    })

    it('显示目录路径（去掉文件名）', () => {
      mountCompleted('/storage/emulated/0/encrypted/sample.encv')
      cy.get('.output-meta').should('contain.text', '/storage/emulated/0/encrypted')
    })

    it('根目录文件 → 目录显示 /', () => {
      mountCompleted('/sample.encv')
      cy.get('.output-meta').should('contain.text', '/')
    })

    it('无路径分隔符 → 目录显示 /', () => {
      mountCompleted('sample.encv')
      cy.get('.output-meta').should('contain.text', '/')
    })
  })

  // ==========================================================================
  // 耗时显示
  // ==========================================================================
  describe('耗时显示', () => {
    it('有 createdAt 和 completedAt → 显示耗时', () => {
      mountCompleted('/storage/sample.encv')
      cy.get('.completed-duration').should('exist')
    })

    it('无 completedAt → 不显示耗时', () => {
      const task = makeTask({
        status: 'completed',
        outputPath: '/sample.encv',
        createdAt: '2025-01-01T00:00:00Z',
        completedAt: undefined as any,
      })
      cy.mount(TaskOutputInfo, { props: { task } })
      cy.get('.completed-duration').should('not.exist')
    })
  })

  // ==========================================================================
  // 打开按钮：可预览格式（mp4/webm/mov/m4v/mkv）
  // ==========================================================================
  describe('打开按钮 - 可预览格式', () => {
    const previewableExts = ['mp4', 'webm', 'mov', 'm4v', 'mkv']

    previewableExts.forEach((ext) => {
      it(`.${ext} 文件 → 显示打开按钮`, () => {
        mountCompleted(`/storage/sample.${ext}`)
        cy.get(openBtnSel).should('have.length', 1)
      })
    })

    it('点击打开按钮 → emit open 事件，参数为输出路径', () => {
      const onOpen = cy.spy().as('openSpy')
      mountCompleted('/storage/video.mp4', { onOpen })
      cy.get(openBtnSel).click()
      cy.get('@openSpy').should('have.been.calledOnce')
      cy.get('@openSpy').should('have.been.calledWith', '/storage/video.mp4')
    })
  })

  // ==========================================================================
  // 打开按钮：不可预览格式（encv/jpg/png/pdf 等）
  // ==========================================================================
  describe('打开按钮 - 不可预览格式', () => {
    it('.encv 文件 → 不显示打开按钮', () => {
      mountCompleted('/storage/sample.encv')
      cy.get(openBtnSel).should('not.exist')
    })

    it('.jpg 文件 → 不显示打开按钮', () => {
      mountCompleted('/storage/photo.jpg')
      cy.get(openBtnSel).should('not.exist')
    })

    it('.pdf 文件 → 不显示打开按钮', () => {
      mountCompleted('/storage/doc.pdf')
      cy.get(openBtnSel).should('not.exist')
    })
  })

  // ==========================================================================
  // 定位按钮
  // ==========================================================================
  describe('定位按钮', () => {
    it('有 outputPath → 显示定位按钮', () => {
      mountCompleted('/storage/sample.encv')
      cy.get(locateBtnSel).should('have.length', 1)
    })

    it('点击定位按钮 → emit locate 事件，参数为输出路径', () => {
      const onLocate = cy.spy().as('locateSpy')
      mountCompleted('/storage/folder/sample.encv', { onLocate })
      cy.get(locateBtnSel).click()
      cy.get('@locateSpy').should('have.been.calledOnce')
      cy.get('@locateSpy').should('have.been.calledWith', '/storage/folder/sample.encv')
    })
  })
})
