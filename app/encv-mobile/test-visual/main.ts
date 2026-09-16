/**
 * 视觉回归独立挂载壳（绕过项目 vite.config.ts 的 devStartGuard）
 *
 * 与 cypress component 同一思路：最小 Vite 配置，仅挂 IonicVue + router + pinia，
 * 把 AppearanceDetail 挂到 #app，供 Playwright 截图比对（Phase 4 主题迁移 QA）。
 *
 * 注意：此处必须完成三项才能让快照真实：
 *  1. 注册业务 i18n（initEncvI18n）——否则所有 label 都是 [MISSING:...]。
 *  2. 全局注册 Ionic Vue 组件（CE 构建模式下 IonicVue 不会自动注册 <ion-*>）。
 *  3. 加载项目主题 CSS（variables.css + theme-core.css），否则 daisyUI 令牌不生效。
 */
import { createApp, h } from 'vue'
import { IonicVue } from '@ionic/vue'
import { createRouter, createWebHashHistory } from 'vue-router'
import { createPinia } from 'pinia'
import { registerIonicComponents } from '@encv/shared-components/composables/useIonicAutoRegister'
import { initEncvI18n } from '@/i18n/init'
import AppearanceDetail from '@/views/AppearanceDetail.vue'

import '@ionic/vue/css/core.css'
import '@ionic/vue/css/normalize.css'
import '@ionic/vue/css/structure.css'
import '@ionic/vue/css/typography.css'
import '@encv/shared-components/theme/variables.css'
import '@encv/shared-components/styles/theme-core.css'
import '@encv/shared-components/styles/timeline-tokens.css'
import '@encv/shared-components/styles/timeline-utilities.css'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [{ path: '/', component: AppearanceDetail }],
})

const app = createApp({ render: () => h(AppearanceDetail) })
  .use(IonicVue)
  .use(router)
  .use(createPinia())

registerIonicComponents(app)
initEncvI18n()

router.isReady().then(() => app.mount('#app'))
