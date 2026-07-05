// Cypress 组件测试支持文件
// 设计参考：app/encv-mobile/cypress/support/component.ts

import { mount } from 'cypress/vue'
import { IonicVue } from '@ionic/vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHistory } from 'vue-router'

// Ionic 全局 CSS
import '@ionic/vue/css/core.css'
import '@ionic/vue/css/normalize.css'
import '@ionic/vue/css/structure.css'
import '@ionic/vue/css/typography.css'
import '@ionic/vue/css/padding.css'
import '@ionic/vue/css/flex-utils.css'
import '@ionic/vue/css/display.css'

// 创建共享路由
const sharedTestRouter = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: { template: '<div>Home</div>' } },
    { path: '/world', component: { template: '<div>World</div>' } },
    { path: '/chronicle', component: { template: '<div>Chronicle</div>' } },
    { path: '/tabs/settings', component: { template: '<div>Settings</div>' } },
    { path: '/tabs/devlogs', component: { template: '<div>DevLogs</div>' } },
  ],
})

// 每个测试前重置 pinia + router
beforeEach(() => {
  // 重置 router 到 /
  void sharedTestRouter.push('/').catch(() => {})
})

// 暴露 mount 给所有测试（自动装 IonicVue + vue-router + pinia）
Cypress.Commands.add('mount', (component, options: any = {}) => {
  const pinia = createPinia()
  const { global = {}, ...rest } = options
  return mount(component, {
    global: {
      plugins: [
        sharedTestRouter,
        pinia,
        IonicVue,
        ...(global.plugins || []),
      ],
      ...global,
    },
    ...rest,
  })
})

// ⚠️ 暴露带 template compiler 的 Vue
export * as Vue from 'vue'
