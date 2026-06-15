import { createRouter, createWebHistory } from '@ionic/vue-router'
import type { RouteRecordRaw } from 'vue-router'
import Tabs from '@/views/Tabs.vue'
import NotFoundView from '@/views/NotFoundView.vue'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    redirect: '/tabs/home',
  },
  {
    path: '/player',
    component: () => import('@/views/ArtPlayerView.vue'),
  },
  {
    path: '/tabs/',
    component: Tabs,
    children: [
      {
        path: '',
        redirect: '/tabs/home',
      },
      {
        path: 'home',
        component: () => import('@/views/HomePage.vue'),
      },
      {
        path: 'files',
        component: () => import('@/views/Files.vue'),
      },
      {
        path: 'tasks',
        component: () => import('@/views/Tasks.vue'),
      },
      {
        path: 'remote',
        component: () => import('@/views/Remote.vue'),
      },
      {
        path: 'settings',
        component: () => import('@/views/Settings.vue'),
      },
      {
        path: 'extensions',
        component: () => import('@/views/ExtensionsPage.vue'),
        meta: { title: 'extensions.title' },
      },
      {
        path: 'openlist',
        component: () => import('@/views/OpenListView.vue'),
        meta: { title: 'openlist.title' },
      },
      {
        path: 'settings/server',
        component: () => import('@/views/ServerDetail.vue'),
      },
      {
        path: 'settings/server/http',
        component: () => import('@/views/HttpServerDetail.vue'),
      },
      {
        path: 'settings/server/admin',
        component: () => import('@/views/AdminServerDetail.vue'),
      },
      {
        path: 'settings/server/webdav',
        component: () => import('@/views/WebdavServerDetail.vue'),
      },
      {
        path: 'settings/engine',
        component: () => import('@/views/EngineDetail.vue'),
      },
      {
        path: 'settings/about',
        component: () => import('@/views/AboutDetail.vue'),
      },
      {
        path: 'settings/cache',
        component: () => import('@/views/CacheDetail.vue'),
      },
      {
        path: 'settings/plugins',
        component: () => import('@/views/PluginSettings.vue'),
      },
      {
        path: 'settings/agent',
        component: () => import('@/views/AgentSettingsDetail.vue'),
      },
      {
        // 服务器地址管理页（手动配置 baseUrl 兜底）
        path: 'settings/server',
        component: () => import('@/views/ServerSettings.vue'),
      },
      {
        path: 'settings/devtools',
        component: () => import('@/views/DevToolsDetail.vue'),
      },
      {
        path: 'settings/devtools/automation',
        component: () => import('@/views/AutomationTestsDetail.vue'),
      },
      {
        // 🆕 2026-06-11 v6：webdav 服务自动化测试入口
        path: 'settings/devtools/webdav-tests',
        component: () => import('@/views/WebDavAutomationTestsDetail.vue'),
      },
      {
        // 🆕 2026-06-11：ECv4 容量边界测试（100×128GB sparse 虚拟容器）
        path: 'settings/devtools/sparse-container-test',
        component: () => import('@/views/SparseContainerTestDetail.vue'),
      },
      {
        path: 'settings/devtools/prototype/:id',
        component: () => import('@/views/PrototypeSandbox.vue'),
      },
      {
        path: 'settings/appearance',
        component: () => import('@/views/AppearanceDetail.vue'),
      },
      {
        // 🆕 2026-06-15 multi-mount (spec Phase E)：挂载点管理
        path: 'settings/mounts',
        component: () => import('@/views/MountsDetail.vue'),
      },
      {
        path: 'devlogs',
        component: () => import('@/views/DevLogs.vue'),
      },
      {
        path: 'preview',
        component: () => import('@/views/FilePreview.vue'),
      },
      {
        path: 'file-info',
        component: () => import('@/views/FileInfo.vue'),
      },
      // Catch-all 404 路由（防御性 UI：开发期任何路径不匹配都显示清晰提示而不是空白）
      {
        path: ':pathMatch(.*)*',
        name: 'not-found',
        component: NotFoundView,
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
})

export default router
