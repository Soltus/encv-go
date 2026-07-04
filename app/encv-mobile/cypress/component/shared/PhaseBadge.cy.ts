/**
 * PhaseBadge Cypress Component 真实组件测试（2026-06-23 替换 jsdom 版本）
 *
 * 覆盖：
 * - 9 个 Phase 值渲染对应 label
 * - 2 个终态 status 值（failed / cancelled）渲染对应 label
 * - 自定义 label 覆盖默认
 * - class 包含 phase 标识
 * - 包含 PhaseIcon 子组件
 *
 * 与 jsdom 版本的本质区别：
 *   - 真实 ion-icon web component
 *   - 真实 CSS 样式计算（background-color 可验证）
 *   - 真实子组件渲染（PhaseIcon 不是 stub）
 */
import PhaseBadge from '../../../src/components/shared/PhaseBadge.vue'
import { Phase, ALL_PHASES } from '../../../src/lib/workflow/types'

const EXPECTED_LABEL_MAP: Record<string, string> = {
  [Phase.Created]: '已创建',
  [Phase.Analyzing]: '分析中',
  [Phase.Initializing]: '初始化',
  [Phase.Preprocessing]: '预处理',
  [Phase.Encrypting]: '加密中',
  [Phase.Decrypting]: '解密中',
  [Phase.Packing]: '打包中',
  [Phase.Verifying]: '校验中',
  [Phase.Completed]: '已完成',
  failed: '失败',
  cancelled: '已取消',
}

describe('PhaseBadge - 真实组件 (Cypress Component)', () => {
  it('9 个 Phase 值渲染对应默认 label', () => {
    for (const phase of ALL_PHASES) {
      cy.mount(PhaseBadge as any, { props: { phase } })
      cy.get('.phase-badge')
        .should('have.class', `phase-badge--${phase}`)
        .find('.phase-badge__label')
        .should('have.text', EXPECTED_LABEL_MAP[phase])
    }
  })

  it('failed 终态渲染默认 label "失败"', () => {
    cy.mount(PhaseBadge as any, { props: { phase: 'failed' } })
    cy.get('.phase-badge--failed .phase-badge__label')
      .should('have.text', '失败')
  })

  it('cancelled 终态渲染默认 label "已取消"', () => {
    cy.mount(PhaseBadge as any, { props: { phase: 'cancelled' } })
    cy.get('.phase-badge--cancelled .phase-badge__label')
      .should('have.text', '已取消')
  })

  it('自定义 label 覆盖默认', () => {
    cy.mount(PhaseBadge as any, { props: { phase: Phase.Encrypting, label: '自定义加密标签' } })
    cy.get('.phase-badge__label')
      .should('have.text', '自定义加密标签')
  })

  it('传入空字符串 label 时显示空字符串', () => {
    cy.mount(PhaseBadge as any, { props: { phase: Phase.Created, label: '' } })
    cy.get('.phase-badge__label')
      .should('have.text', '')
  })

  it('包含 PhaseIcon 子组件', () => {
    cy.mount(PhaseBadge as any, { props: { phase: Phase.Created } })
    cy.get('.phase-badge ion-icon.phase-icon')
      .should('exist')
      .and('be.visible')
  })

  it('PhaseIcon 接收正确的 phase class', () => {
    cy.mount(PhaseBadge as any, { props: { phase: Phase.Encrypting } })
    cy.get('.phase-badge ion-icon.phase-icon--encrypting')
      .should('exist')
  })

  it('渲染单个 phase-badge 根元素', () => {
    cy.mount(PhaseBadge as any, { props: { phase: Phase.Created } })
    cy.get('.phase-badge').should('have.length', 1)
  })

  it('渲染单个 phase-badge__label 元素', () => {
    cy.mount(PhaseBadge as any, { props: { phase: Phase.Created } })
    cy.get('.phase-badge__label').should('have.length', 1)
  })
})
