// Cypress 组件测试支持文件
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
  void sharedTestRouter.push('/').catch(() => {})
})

// 暴露 mount 给所有测试
Cypress.Commands.add('mount', (component, options: any = {}) => {
  const pinia = createPinia()
  const { global = {}, ...rest } = options
  
  // 添加全局样式来修复 ion-content 高度问题
  const styleTag = document.createElement('style')
  styleTag.id = 'cypress-global-styles'
  styleTag.textContent = `
    html, body { margin: 0; padding: 0; height: 100%; }
    ion-app { height: 100% !important; min-height: 100vh !important; }
    ion-page { display: flex !important; flex-direction: column !important; height: 100% !important; }
    ion-header { flex-shrink: 0; }
    ion-content { flex: 1 1 auto !important; display: flex !important; flex-direction: column !important; }
  `
  document.head.appendChild(styleTag)
  
  return mount(component, {
    global: {
      plugins: [
        sharedTestRouter,
        pinia,
        [IonicVue, { mode: 'ios', rippleEffect: false }],
        ...(global.plugins || []),
      ],
      ...global,
    },
    ...rest,
  })
})

export * as Vue from 'vue'
