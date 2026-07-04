/**
 * TaskActionButtons 组件测试
 *
 * 业务价值：任务操作按钮是用户与任务交互的核心入口
 *   - running 状态显示取消按钮
 *   - failed 状态显示重试按钮
 *   - completed/failed/cancelled 状态显示删除按钮
 *   - 按钮点击 emit 正确的事件
 */
import TaskActionButtons from '../../src/components/TaskActionButtons.vue'
import { taskFixtures } from '../support/task-test-helpers'

describe('TaskActionButtons', () => {
  function mountWithStatus(status: string, propsOverrides: Record<string, any> = {}) {
    const task = taskFixtures.one(0, { status: status as any })
    const allProps = { task, ...propsOverrides }
    cy.mount(TaskActionButtons, { props: allProps })
    return cy.wrap(task)
  }

  // 直接用选择器，避免 filter 在 0 个元素时超时
  const cancelBtnSel = 'ion-button[color="warning"]'
  const retryBtnSel = 'ion-button[color="primary"]:not([fill="outline"])'
  const removeBtnSel = 'ion-button[color="danger"]'

  // ==========================================================================
  // 渲染：按状态显示对应按钮
  // ==========================================================================
  describe('按状态渲染按钮', () => {
    it('running 状态 → 只显示取消按钮', () => {
      mountWithStatus('running')
      cy.get(cancelBtnSel).should('have.length', 1)
      cy.get(retryBtnSel).should('not.exist')
      cy.get(removeBtnSel).should('not.exist')
    })

    it('failed 状态 → 显示重试 + 移除按钮', () => {
      mountWithStatus('failed')
      cy.get(cancelBtnSel).should('not.exist')
      cy.get(retryBtnSel).should('have.length', 1)
      cy.get(removeBtnSel).should('have.length', 1)
    })

    it('completed 状态 → 只显示移除按钮（outline）', () => {
      mountWithStatus('completed')
      cy.get(cancelBtnSel).should('not.exist')
      cy.get(retryBtnSel).should('not.exist')
      cy.get(removeBtnSel).should('have.length', 1)
      cy.get(removeBtnSel).should('have.attr', 'fill', 'outline')
    })

    it('cancelled 状态 → 只显示移除按钮', () => {
      mountWithStatus('cancelled')
      cy.get(cancelBtnSel).should('not.exist')
      cy.get(retryBtnSel).should('not.exist')
      cy.get(removeBtnSel).should('have.length', 1)
    })

    it('queued 状态 → 不显示任何按钮', () => {
      mountWithStatus('queued')
      cy.get('ion-button').should('not.exist')
    })

    it('pending 状态 → 不显示任何按钮', () => {
      mountWithStatus('pending')
      cy.get('ion-button').should('not.exist')
    })
  })

  // ==========================================================================
  // 交互：按钮点击 emit 事件
  // ==========================================================================
  describe('按钮点击 emit 事件', () => {
    it('点击取消按钮 → emit cancel 事件', () => {
      const onCancel = cy.spy().as('cancelSpy')
      mountWithStatus('running', { onCancel })
      cy.get(cancelBtnSel).click()
      cy.get('@cancelSpy').should('have.been.calledOnce')
    })

    it('点击重试按钮 → emit retry 事件', () => {
      const onRetry = cy.spy().as('retrySpy')
      mountWithStatus('failed', { onRetry })
      cy.get(retryBtnSel).click()
      cy.get('@retrySpy').should('have.been.calledOnce')
    })

    it('点击移除按钮 → emit remove 事件', () => {
      const onRemove = cy.spy().as('removeSpy')
      mountWithStatus('completed', { onRemove })
      cy.get(removeBtnSel).click()
      cy.get('@removeSpy').should('have.been.calledOnce')
    })
  })

  // ==========================================================================
  // 样式：按钮颜色和变体
  // ==========================================================================
  describe('按钮样式', () => {
    it('取消按钮 → warning 色 + 实心填充', () => {
      mountWithStatus('running')
      cy.get(cancelBtnSel).should('have.attr', 'color', 'warning')
      cy.get(cancelBtnSel).should('not.have.attr', 'fill', 'outline')
    })

    it('重试按钮 → primary 色 + 实心填充', () => {
      mountWithStatus('failed')
      cy.get(retryBtnSel).should('have.attr', 'color', 'primary')
      cy.get(retryBtnSel).should('not.have.attr', 'fill', 'outline')
    })

    it('移除按钮 → danger 色 + outline 边框', () => {
      mountWithStatus('completed')
      cy.get(removeBtnSel).should('have.attr', 'color', 'danger')
      cy.get(removeBtnSel).should('have.attr', 'fill', 'outline')
    })
  })
})
