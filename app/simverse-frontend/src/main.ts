import { IonicVue } from "@ionic/vue";
import { createPinia } from "pinia";
import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import { registerIonicComponents } from "@encv/shared-components/composables/useIonicAutoRegister";
import { initI18n } from "@encv/shared-components/composables/useI18n";

import simverseI18n from "./i18n/simverse";

import "@ionic/vue/css/core.css";
import "@ionic/vue/css/normalize.css";
import "@ionic/vue/css/structure.css";
import "@ionic/vue/css/typography.css";
import "@ionic/vue/css/padding.css";
import "@ionic/vue/css/flex-utils.css";
import "@ionic/vue/css/display.css";
import "./theme/variables.css";

initI18n({
  modules: [simverseI18n],
  storageKey: "simverse-locale",
  defaultLocale: "zh-CN",
});

const pinia = createPinia();
const app = createApp(App).use(IonicVue).use(router).use(pinia);

registerIonicComponents(app);

router.isReady().then(() => {
  app.mount("#app");
});
