// Cypress Component Testing 支持文件
//   - 导入 Vue / Pinia / Ionic
//   - 暴露 mount 命令
//   - 注册自定义命令

import { mount } from 'cypress/vue'
import { createPinia, setActivePinia } from 'pinia'
import { IonicVue } from '@ionic/vue'

// Ionic 全局 CSS
import '@ionic/vue/css/core.css'
import '@ionic/vue/css/normalize.css'
import '@ionic/vue/css/structure.css'
import '@ionic/vue/css/typography.css'

// 导入自定义命令（cy.dataCy 等）
import './commands'

// 每个测试前重置 Pinia
beforeEach(() => {
  setActivePinia(createPinia())
})

// 暴露 mount 给所有测试（已带 IonicVue 插件）
Cypress.Commands.add('mount', (component, options = {}) => {
  const { global = {}, ...rest } = options
  return mount(component, {
    global: {
      plugins: [IonicVue, ...(global.plugins || [])],
      ...global,
    },
    ...rest,
  })
})

// ⚠️ 暴露带 template compiler 的 Vue 给 cypress spec
//   - cypress 15 + Vite 默认用 vue.runtime.esm-bundler.js（runtime-only）
//   - spec 文件 import { defineComponent } from '@test/vue' 走带 compiler 的 esm-bundler.js
//   - 否则 defineComponent({ template: '...' }) 静默失败（无编译错误，渲染为 <!---->）
export * as VueWithCompiler from 'vue/dist/vue.esm-bundler.js'
