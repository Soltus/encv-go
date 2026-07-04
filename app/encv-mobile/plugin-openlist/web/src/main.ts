import { createApp } from 'vue'
import { IonicVue } from '@ionic/vue'
import App from './App.vue'
import { router } from './router'

// Ionic 核心样式
import '@ionic/vue/css/core.css'
import '@ionic/vue/css/normalize.css'
import '@ionic/vue/css/structure.css'
import '@ionic/vue/css/typography.css'

// 主题（与主 app 一致）
import './theme/variables.css'

const app = createApp(App)
app.use(IonicVue)
app.use(router)

router.isReady().then(() => {
  app.mount('#app')
})
