import { createRouter, createWebHistory } from "@ionic/vue-router";
import type { RouteRecordRaw } from "vue-router";
import NotFoundView from "@/views/NotFoundView.vue";
import Tabs from "@/views/Tabs.vue";

const routes: RouteRecordRaw[] = [
  {
    path: "/",
    redirect: "/tabs/home",
  },
  {
    path: "/player",
    component: () => import("@/views/ArtPlayerView.vue"),
  },
  {
    path: "/tabs/",
    component: Tabs,
    children: [
      {
        path: "",
        redirect: "/tabs/home",
      },
      {
        path: "home",
        component: () => import("@/views/HomePage.vue"),
      },
      {
        path: "files",
        component: () => import("@/views/Files.vue"),
      },
      {
        path: "tasks",
        component: () => import("@/views/Tasks.vue"),
      },
      {
        // 🆕 2026-06-18 v5-bug3fix：L2 GroupDetail 页面
        //   - Tasks L1 group card 整张 clickable → push 到此页
        //   - ion-segment 3 tab：Pipeline / Tasks / Diagnostics
        //   - 不与 PluginTestsDetail 耦合（插件测试在设置 tab 独立）
        //   - 设计理由：100+ 任务的 group 不应在一级页面直接展开
        path: "tasks/group/:runId",
        component: () => import("@/views/GroupDetail.vue"),
        props: true,
      },
      {
        path: "remote",
        component: () => import("@/views/Remote.vue"),
      },
      {
        path: "settings",
        component: () => import("@/views/Settings.vue"),
      },
      {
        path: "extensions",
        component: () => import("@/views/ExtensionsPage.vue"),
        meta: { title: "extensions.title" },
      },
      {
        path: "openlist",
        component: () => import("@/views/OpenListView.vue"),
        meta: { title: "openlist.title" },
      },
      {
        path: "settings/server",
        component: () => import("@/views/ServerDetail.vue"),
      },
      {
        path: "settings/server/http",
        component: () => import("@/views/HttpServerDetail.vue"),
      },
      {
        path: "settings/server/admin",
        component: () => import("@/views/AdminServerDetail.vue"),
      },
      {
        path: "settings/server/webdav",
        component: () => import("@/views/WebdavServerDetail.vue"),
      },
      {
        // 🆕 2026-06-17：FFmpeg 引擎详情从 Settings 一级迁移到 About 二级
        // 路径层级 /tabs/settings/about/engine（三级）反映导航层级
        // 旧路径 /tabs/settings/engine 已删除（外部 deep link 会 404，不需要兼容）
        path: "settings/about/engine",
        component: () => import("@/views/FfmpegEngineDetail.vue"),
      },
      {
        path: "settings/about",
        component: () => import("@/views/AboutDetail.vue"),
      },
      {
        path: "settings/cache",
        component: () => import("@/views/CacheDetail.vue"),
      },
      {
        path: "settings/database",
        component: () => import("@/views/DatabaseDetail.vue"),
      },
      {
        path: "settings/chronicle",
        component: () => import("@/views/ChronicleDetail.vue"),
      },
      {
        path: "settings/fulltext-index",
        component: () => import("@/views/FullTextIndexDetail.vue"),
      },
      {
        path: "settings/plugins",
        component: () => import("@/views/PluginSettings.vue"),
      },
      {
        path: "settings/agent",
        component: () => import("@/views/AgentSettingsDetail.vue"),
      },
      {
        // 🆕 2026-06-16：删除「服务器状态详情页」独立路由
        // 旧版 ServerStatusDetail.vue 是「点卡片跳独立页」设计，已被 ServerStatusCard 翻转取代
        // （卡片翻转到背面 = 自带诊断/操作历史/进程 ID/transport 详情）
        // 入口：原 ServerDetail 顶部的 ServerStatusCard @click="goServerStatusDetail"
        // 现状：ServerDetail.vue 不再调 goServerStatusDetail；本路由删除防「打开 app 看到残留 page」{
        path: "settings/devtools",
        component: () => import("@/views/DevToolsDetail.vue"),
      },
      {
        // 🆕 2026-06-17：日志设置三级页面（vConsole 之外的日志相关设置 + 导出/清空）
        path: "settings/devtools/log-settings",
        component: () => import("@/views/LogSettingsDetail.vue"),
      },
      {
        // 🆕 2026-06-17：自动化测试总览（Hub）— 包含 plugin / webdav / sparse 3 个子入口
        path: "settings/devtools/automation-hub",
        component: () => import("@/views/AutomationTestsHub.vue"),
      },
      {
        // 🆕 2026-06-11 v6：插件测试运行/管理页面（Mock 数据 + 触发测试 + Pipeline/Tree 视图）
        // 历史：1730866 commit 曾错误删除（误以为是"测试报告"页），2026-06-18 v5-bug3fix 恢复
        // 测试报告 section 已并入任务系统 group card 的 exportGroupReport（zip 导出）
        path: "settings/devtools/plugin-tests",
        component: () => import("@/views/PluginTestsDetail.vue"),
      },
      {
        // 🆕 2026-06-11 v6：webdav 服务自动化测试入口
        // 历史：v4 commit 1730866 错误改为 redirect 到 automation-hub + 单数 path 'webdav-auto'
        //       与 AutomationTestsHub.vue:67 的 'webdav-tests' 跳转不一致
        // 2026-06-18 v5-bug3fix 恢复成 v4 之前的可用状态
        path: "settings/devtools/webdav-tests",
        component: () => import("@/views/WebDavAutomationTestsDetail.vue"),
      },
      {
        // 🆕 2026-06-11：ECv4 容量边界测试（100×128GB sparse 虚拟容器）
        path: "settings/devtools/sparse-container-test",
        component: () => import("@/views/SparseContainerTestDetail.vue"),
      },
      {
        // 🆕 2026-06-22：文件系统任务测试（move/copy/rename/delete + rollback + trash 边界）
        path: "settings/devtools/fs-tests",
        component: () => import("@/views/FileSystemTestsDetail.vue"),
      },
      {
        // 🆕 2026-07-03：数据库自动化测试（CRUD/批量/查询/并发/导出导入）
        path: "settings/devtools/database-tests",
        component: () => import("@/views/DatabaseTests.vue"),
      },
      {
        // 🆕 2026-06-17：Compose UI 原型总览（Hub）— 卡片循环从 DevToolsDetail 迁移
        path: "settings/devtools/compose-prototypes-hub",
        component: () => import("@/views/ComposePrototypesHub.vue"),
      },
      {
        path: "settings/devtools/prototype/:id",
        component: () => import("@/views/PrototypeSandbox.vue"),
      },
      {
        path: "settings/appearance",
        component: () => import("@/views/AppearanceDetail.vue"),
      },
      {
        // 🆕 2026-06-15 multi-mount (spec Phase E)：挂载点管理
        path: "settings/mounts",
        component: () => import("@/views/MountsDetail.vue"),
      },
      {
        path: "devlogs",
        component: () => import("@/views/DevLogs.vue"),
      },
      {
        path: "preview",
        component: () => import("@/views/FilePreview.vue"),
      },
      {
        path: "file-info",
        component: () => import("@/views/FileInfo.vue"),
      },
      // Catch-all 404 路由（防御性 UI：开发期任何路径不匹配都显示清晰提示而不是空白）
      {
        path: ":pathMatch(.*)*",
        name: "not-found",
        component: NotFoundView,
      },
    ],
  },
];

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
});

export default router;
