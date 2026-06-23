// Cypress Component Testing 支持文件（2026-06-23 重构）
//   - 导入 Vue / Pinia / Ionic
//   - 暴露 mount 命令（自动装 IonicVue + vue-router + pinia）
//   - 注册自定义命令
//   - 用 store-helpers（module-level pinia）解决 cy.window() 拿不到 win 的问题

import { mount } from 'cypress/vue'
import { IonicVue } from '@ionic/vue'
import { _resetTestPinia, sharedTestRouter } from './store-helpers'

// Ionic 全局 CSS
import '@ionic/vue/css/core.css'
import '@ionic/vue/css/normalize.css'
import '@ionic/vue/css/structure.css'
import '@ionic/vue/css/typography.css'

// 导入自定义命令（cy.dataCy 等）
import './commands'

// 每个测试前重置 pinia + 共享 router
beforeEach(() => {
  _resetTestPinia()
  // 重置 router 到 /（避免上一个测试残留 route state）
  void sharedTestRouter.push('/').catch(() => {})
})

// 暴露 mount 给所有测试（自动装 pinia + router + IonicVue）
//   - pinia 在 store-helpers 里已经 setActivePinia，组件内 useTaskStore() 通过 getActivePinia 拿
//   - router 通过 sharedTestRouter（spec 也能用 _pushTo 切换 route params）
Cypress.Commands.add('mount', (component, options: any = {}) => {
  const { global = {}, ...rest } = options
  return mount(component, {
    global: {
      plugins: [
        // 顺序：router → IonicVue（ion-router 用 vue-router）
        sharedTestRouter,
        IonicVue,
        ...(global.plugins || []),
      ],
      ...global,
    },
    ...rest,
  })
})

// ⚠️ 暴露带 template compiler 的 Vue 给 cypress spec
//   - cypress 15 + Vite 默认用 vue.runtime.esm-bundler.js（runtime-only）
//   - spec 文件 import { defineComponent } from 'vue/dist/vue.esm-bundler.js'
//   - 否则 defineComponent({ template: '...' }) 静默失败（无编译错误，渲染为 <!---->）
export * as VueWithCompiler from 'vue/dist/vue.esm-bundler.js'
