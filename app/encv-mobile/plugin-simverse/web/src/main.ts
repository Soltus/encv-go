import { registerI18nModule, useI18n } from "@encv/shared-components/composables/useI18n";
import { initSharedI18n } from "@encv/shared-components/i18n";
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

const pinia = createPinia();
const app = createApp(App).use(IonicVue).use(router).use(pinia);

const { registered } = registerIonicComponents(app);
console.log(`[ionic] Registered ${registered.length} Ionic Vue components`);

initSharedI18n();
registerI18nModule(simverseI18n);

const { t } = useI18n();
app.config.globalProperties.$t = t;

router.isReady().then(() => {
  app.mount("#app");
});
