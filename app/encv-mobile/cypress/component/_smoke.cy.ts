/**
 * Cypress Component Testing 基础设施验证测试
 *   - 验证 cypress 能在 Electron 下挂载真 Vue 组件
 *   - 强制 import vue.esm-bundler.js（带 template compiler），避免 runtime-only 问题
 */
import { defineComponent } from 'vue/dist/vue.esm-bundler.js'

describe('Cypress Component 基础设施验证', () => {
  it('cy.mount 挂载真组件', () => {
    const HelloComponent = defineComponent({
      name: 'HelloComponent',
      template: `<h1 data-cy="hello">Hello Cypress</h1>`,
    })
    cy.mount(HelloComponent as any).dataCy('hello').should('contain.text', 'Hello Cypress')
  })
})
