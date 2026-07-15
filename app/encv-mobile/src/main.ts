import { vLongpress } from "@encv/shared-components/directives/longpress";
import { IonicVue } from "@ionic/vue";
import { createPinia } from "pinia";
import { createApp, watch } from "vue";
// 🆕 2026-07-02 A5：三管齐下错误捕获
//   用户强反馈："ion-page 警告 = 更底层错误没有捕获，比如不支持安卓端的调用"
//   三管齐下：Vue errorHandler + window.onerror/unhandledrejection + console.error 重定向
import { bindVueErrorHandler, errorStore, installErrorCapture } from "@encv/shared-components/composables/useErrorCapture";
// 🆕 2026-07-02：DevLogs 前端日志（错误捕获系统的错误同步写入这里）
import { addFrontendLog } from "@encv/shared-components/composables/useFrontendLogs";
// 🆕 2026-07-06：Ionic 组件全局注册
//   根因：@ionic/vue 的 IonicVue 插件在 CE 构建模式下不全局注册 Vue 组件，
//   只初始化 Web Components，导致模板里的 <ion-xxx> 报 "Failed to resolve component"
//   页面空白。解决方案：手动扫描 @ionic/vue 导出的所有 IonXxx 组件并全局注册。
import { registerIonicComponents } from "@encv/shared-components/composables/useIonicAutoRegister";
import { installProxiedFetch } from "@encv/shared-components/composables/useProxiedFetch";
import { initEncvI18n } from "@/i18n/init";
import { clearLegacyLocalStorage } from "@encv/shared-components/lib/taskPersistence";
import App from "./App.vue";
import router from "./router";

// TDesign Chat 组件库不再做全局注册：
//   早期版本用 <Chatbot> + ChatService 自行消费 SSE 流，与 useAgent
//   共享数据源架构冲突（双消费）。
//   Phase 4 重构后改为 TDesignChatView 按需导入 ChatList / ChatItem /
//   ChatThinking 等具体组件（参见 src/engines/TDesignChatView.vue）。
//   <Chatbot> 全局注册已删除，仅保留 TDesign 通用组件的 CSS（项目其它
//   地方仍可能用 tdesign-vue-next 的 List/Tag 等基础组件）。

import "@ionic/vue/css/core.css";
import "@ionic/vue/css/normalize.css";
import "@ionic/vue/css/structure.css";
import "@ionic/vue/css/typography.css";
import "@encv/shared-components/theme/variables.css";
// 🆕 Phase 1：引入共享主题核心（调色板 + Ionic←daisyUI 桥接令牌，纯 CSS 不依赖 Tailwind）。
// 必须在 variables.css 之后，桥接的 --ion-color-* 才能覆盖其字面量，
// 让 Ionic 组件跟随与插件一致的 daisyUI encv / encv-dark 单一调色板。
import "@encv/shared-components/styles/theme-core.css";
// 🆕 续27：全局语义「表面」类（.ui-chip / .ui-badge / ...），无 scoped，
// 供用户主题以极简选择器任意覆写（SiYuan 式自由度 + 令牌易用路径）。
import "@encv/shared-components/theme/surface.css";
import "@encv/shared-components/styles/timeline-tokens.css";
import "@encv/shared-components/styles/timeline-utilities.css";

// 🆕 v6 2026-06-18：注册 Pinia（任务系统 store）
const pinia = createPinia();
const app = createApp(App).use(IonicVue).use(router).use(pinia);

// 🆕 2026-07-10 Phase 2：将任务 store 所需的应用层能力（向量搜索 / 分页拉取 / IndexedDB）
// 注入共享抽象层（@encv/shared-components/stores/*）。必须早于任何 useTaskStore() 调用。
import { registerSharedTaskServices } from "@/stores/registerSharedTaskServices";
import { registerSharedAppCapabilities } from "@/stores/registerSharedAppCapabilities";
import { registerSharedAppNavigation } from "@/stores/registerSharedAppNavigation";
import { registerSharedNativeBridge } from "@/stores/registerSharedNativeBridge";
import { registerSharedAppAssets } from "@/stores/registerSharedAppAssets";
import { registerSharedApiProxy } from "@/stores/registerSharedApiProxy";

registerSharedTaskServices();
registerSharedAppCapabilities();
registerSharedAppNavigation();
registerSharedNativeBridge();
registerSharedAppAssets();
registerSharedApiProxy();

// 🆕 2026-07-06：全局注册所有 Ionic Vue 组件
//   必须在 .use(IonicVue) 之后调用，确保 Web Components 初始化完成
const { registered: ionicRegistered } = registerIonicComponents(app);
console.log(`[ionic] Registered ${ionicRegistered.length} Ionic Vue components`);

// 注册长按指令
app.directive("longpress", vLongpress);

// 🆕 2026-07-06：注册 encv 业务 i18n 字典
initEncvI18n();

// 🆕 2026-07-02 A5：在 Vue app 创建后挂 errorHandler
// 类型签名差异：Vue 的 errorHandler 第 2 参数是 ComponentPublicInstance 类型，
// 我们只需要 err/info → 用 any cast 简化（实际语义不影响）
bindVueErrorHandler(app as unknown as { config: { errorHandler?: (err: unknown, instance: unknown, info: string) => void } });

// Phase X1: 在 native 模式下把 window.fetch 路由到 ApiProxy 插件，
// 绕开 WebView CORS preflight。dev / web 平台 no-op。
installProxiedFetch();

// 🆕 2026-07-02 A5：安装 window.onerror / unhandledrejection / console.error 三件套
//   必须在 app 创建后调，确保覆盖完整
installErrorCapture();

// 🆕 2026-07-02：错误捕获系统 ↔ DevLogs 前端日志 桥接
//   - 错误捕获系统抓到的异常 → 写入 DevLogs 前端日志（带原始堆栈）
//   - 用户要求："devlogs 支持原始堆栈，应当补充发送"
watch(
  () => errorStore.latestError.value,
  err => {
    if (err) {
      addFrontendLog("error", `[${err.source.toUpperCase()}] ${err.message}`, {
        source: `error_capture:${err.source}`,
        stack: err.stack,
      });
    }
  }
);

// 🆕 v6 2026-06-18：清理旧 localStorage key（v6 决定：清空迁移，从零开始）
clearLegacyLocalStorage();

router.isReady().then(() => {
  app.mount("#app");
});
