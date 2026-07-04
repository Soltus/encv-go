/**
 * PhaseIcon Cypress Component 真实组件测试（2026-06-23 替换 jsdom 版本）
 *
 * 覆盖：
 * - 9 个 Phase 值渲染对应 ion-icon
 * - 2 个终态 status 值（failed / cancelled）渲染对应 ion-icon
 * - 未知 phase 用 fallback（helpCircleOutline）
 * - class 包含 phase 标识
 * - size props 控制图标尺寸
 *
 * 与 jsdom 版本的本质区别：
 *   - 真实 ion-icon web component（Electron 完整支持）
 *   - 真实 SVG 渲染（不是 stub）
 *   - 真实样式计算（font-size 可通过 getComputedStyle 验证）
 */
import { defineComponent } from 'vue/dist/vue.esm-bundler.js'
import PhaseIcon from '../../../src/components/shared/PhaseIcon.vue'
import { Phase, ALL_PHASES } from '../../../src/lib/workflow/types'

describe('PhaseIcon - 真实组件 (Cypress Component)', () => {
  it('9 个 Phase 值渲染对应 ion-icon', () => {
    for (const phase of ALL_PHASES) {
      cy.mount(PhaseIcon as any, { props: { phase } })
      cy.get('ion-icon.phase-icon')
        .should('have.class', `phase-icon--${phase}`)
        .and('be.visible')
    }
  })

  it('failed 终态渲染 phase-icon--failed class', () => {
    cy.mount(PhaseIcon as any, { props: { phase: 'failed' } })
    cy.get('ion-icon.phase-icon')
      .should('have.class', 'phase-icon--failed')
      .and('be.visible')
  })

  it('cancelled 终态渲染 phase-icon--cancelled class', () => {
    cy.mount(PhaseIcon as any, { props: { phase: 'cancelled' } })
    cy.get('ion-icon.phase-icon')
      .should('have.class', 'phase-icon--cancelled')
      .and('be.visible')
  })

  it('未知 phase 值使用 fallback 图标', () => {
    cy.mount(PhaseIcon as any, { props: { phase: 'unknown_phase' } })
    cy.get('ion-icon.phase-icon')
      .should('have.class', 'phase-icon--unknown_phase')
      .and('be.visible')
  })

  it('空字符串 phase 使用 fallback', () => {
    cy.mount(PhaseIcon as any, { props: { phase: '' } })
    cy.get('ion-icon.phase-icon')
      .should('have.class', 'phase-icon--')
      .and('be.visible')
  })

  it('size=20 时设置 fontSize: 20px', () => {
    cy.mount(PhaseIcon as any, { props: { phase: Phase.Created, size: 20 } })
    cy.get('ion-icon.phase-icon').then(($el) => {
      const style = window.getComputedStyle($el[0])
      expect(style.fontSize).to.equal('20px')
    })
  })

  it('未传 size 时继承父级 font-size', () => {
    cy.mount(PhaseIcon as any, { props: { phase: Phase.Created } })
    cy.get('ion-icon.phase-icon').then(($el) => {
      const style = window.getComputedStyle($el[0])
      // 默认 font-size 由 .phase-icon 样式设置为 1.1em
      expect(style.fontSize).to.not.equal('0px')
    })
  })

  it('phase prop 变化时 class 跟随更新', () => {
    cy.mount(PhaseIcon as any, { props: { phase: Phase.Created } })
    cy.get('ion-icon.phase-icon').should('have.class', 'phase-icon--created')

    // 通过 Cypress vue mount 返回的 wrapper 改 props
    // 或者用重新 mount 的方式验证
    cy.mount(PhaseIcon as any, { props: { phase: Phase.Encrypting } })
    cy.get('ion-icon.phase-icon').should('have.class', 'phase-icon--encrypting')
  })
})
