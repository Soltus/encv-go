import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { IonicVue } from '@ionic/vue'
import { installProxiedFetch } from './composables/useProxiedFetch'
import { clearLegacyLocalStorage } from './lib/taskPersistence'

// TDesign Chat 组件库不再做全局注册：
//   早期版本用 <Chatbot> + ChatService 自行消费 SSE 流，与 useAgent
//   共享数据源架构冲突（双消费）。
//   Phase 4 重构后改为 TDesignChatView 按需导入 ChatList / ChatItem /
//   ChatThinking 等具体组件（参见 src/engines/TDesignChatView.vue）。
//   <Chatbot> 全局注册已删除，仅保留 TDesign 通用组件的 CSS（项目其它
//   地方仍可能用 tdesign-vue-next 的 List/Tag 等基础组件）。

import '@ionic/vue/css/core.css'
import '@ionic/vue/css/normalize.css'
import '@ionic/vue/css/structure.css'
import '@ionic/vue/css/typography.css'
import '@ionic/vue/css/padding.css'
import '@ionic/vue/css/flex-utils.css'
import '@ionic/vue/css/display.css'
import './theme/variables.css'
import './styles/timeline-tokens.css'
import './styles/timeline-utilities.css'

// 🆕 v6 2026-06-18：注册 Pinia（任务系统 store）
const pinia = createPinia()
const app = createApp(App).use(IonicVue).use(router).use(pinia)

// Phase X1: 在 native 模式下把 window.fetch 路由到 ApiProxy 插件，
// 绕开 WebView CORS preflight。dev / web 平台 no-op。
installProxiedFetch()

// 🆕 v6 2026-06-18：清理旧 localStorage key（v6 决定：清空迁移，从零开始）
clearLegacyLocalStorage()

router.isReady().then(() => {
  app.mount('#app')
})
