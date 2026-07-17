import { registerI18nModule, useI18n } from "@encv/shared-components/composables/useI18n";
import { initSharedI18n } from "@encv/shared-components/i18n";
import { hijackConsole } from "@encv/shared-components/composables/useFrontendLogs";
import { IonicVue } from "@ionic/vue";
import { createPinia } from "pinia";
import { createApp } from "vue";
import App from "./App.vue";
import { registerIonicComponents } from "./composables/useIonicAutoRegister";
import simverseI18n from "./i18n/simverse";
import router from "./router";

import "@ionic/vue/css/core.css";
import "@ionic/vue/css/normalize.css";
import "@ionic/vue/css/structure.css";
import "@ionic/vue/css/typography.css";
import "./theme/variables.css";
import "./theme/simverse.css";
import "@encv/shared-components/styles/daisyui.css";
// 🆕 表面语义类（.ui-chip / .ui-badge / ...）+ daisyUI 桥接令牌，让插件 UI 与主应用
// 使用同一套调色板词汇；实际颜色由宿主注入的 window.__ENCV_THEME__ 覆盖。
import "@encv/shared-components/theme/surface.scss";

const pinia = createPinia();
const app = createApp(App).use(IonicVue).use(router).use(pinia);

const { registered } = registerIonicComponents(app);
console.log(`[ionic] Registered ${registered.length} Ionic Vue components`);

initSharedI18n();
registerI18nModule(simverseI18n);

const { t } = useI18n();
app.config.globalProperties.$t = t;

// 🆕 前端日志捕获：把 console.* 镜像到共享 DevLogs 前端日志缓冲（主应用 App.vue 同款）。
// 插件此前从未调用，导致 DevLogs「前端」tab 永远为空。idempotent，重复调用安全。
hijackConsole();

// 🆕 主应用外观设置桥接（难题：插件是独立 WebView，不共享主应用 window / CSS 变量）。
// 宿主（SimVerseWebViewClient）在 onPageFinished 注入 window.__ENCV_THEME__（形如 {css?: string}），
// 其 css 为主应用选中主题的【已解析 CSS 变量块】。这里把它写进 <style id="encv-host-theme">，
// 覆盖插件自带的静态 variables.css，使主应用外观设置在插件内生效。
// 未注入时回落到插件自带 variables.css + 系统 prefers-color-scheme（不报错）。
function applyHostTheme(): void {
  const raw = (window as any).__ENCV_THEME__;
  if (!raw) return;
  const css = typeof raw === "string" ? raw : raw.css;
  if (!css) return;
  let el = document.getElementById("encv-host-theme") as HTMLStyleElement | null;
  if (!el) {
    el = document.createElement("style");
    el.id = "encv-host-theme";
    document.head.appendChild(el);
  }
  el.textContent = css;
}
// 启动期先尝试一次（宿主可能在 JS 执行前已注入），并监听运行时变更（换肤实时生效）。
applyHostTheme();
window.addEventListener("encv:theme-change", applyHostTheme);

router.isReady().then(() => {
  // 挂载后宿主可能刚刚注入（onPageFinished 时序不确定），再补一次。
  applyHostTheme();
  app.mount("#app");
});
