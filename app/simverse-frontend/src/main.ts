import { createApp } from 'vue'
import { IonicVue } from '@ionic/vue'
import App from './App.vue'
import router from './router'

import '@ionic/vue/css/core.css'
import '@ionic/vue/css/normalize.css'
import '@ionic/vue/css/structure.css'
import '@ionic/vue/css/typography.css'
import '@ionic/vue/css/padding.css'
import '@ionic/vue/css/flex-utils.css'
import '@ionic/vue/css/display.css'

console.log('[Main] Starting app initialization...')

const app = createApp(App)

// 配置 Ionic
app.use(IonicVue, {
  mode: 'ios',
  rippleEffect: false,
})

console.log('[Main] IonicVue plugin installed')

// 配置路由
app.use(router)

console.log('[Main] Router installed, routes:', router.getRoutes().map(r => r.path))

// 等待路由就绪后再挂载
router.isReady().then(() => {
  console.log('[Main] Router ready, mounting app...')
  try {
    const el = document.getElementById('app')
    if (!el) {
      throw new Error('#app element not found')
    }
    app.mount(el)
    console.log('[Main] App mounted successfully!')
  } catch (err) {
    console.error('[Main] Failed to mount app:', err)
    document.body.innerHTML = `<div style="padding:20px;color:red;"><h1>App Mount Error</h1><pre>${err}</pre></div>`
  }
}).catch(err => {
  console.error('[Main] Router isReady failed:', err)
  document.body.innerHTML = `<div style="padding:20px;color:red;"><h1>Router Error</h1><pre>${err.message}</pre><pre>${err.stack}</pre></div>`
})
