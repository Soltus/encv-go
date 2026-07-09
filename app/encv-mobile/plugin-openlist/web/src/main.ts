import { registerI18nModule, useI18n } from "@encv/shared-components/composables/useI18n";
import { initSharedI18n } from "@encv/shared-components/i18n";
import { registerIonicComponents } from "@encv/shared-components/composables/useIonicAutoRegister";
import { IonicVue } from "@ionic/vue";
import { createApp } from "vue";
import App from "./App.vue";
import pluginI18n from "./i18n/openlist";
import { router } from "./router";

import "@ionic/vue/css/core.css";
import "@ionic/vue/css/normalize.css";
import "@ionic/vue/css/structure.css";
import "@ionic/vue/css/typography.css";

import "./theme/variables.css";

const app = createApp(App);
app.use(IonicVue);
app.use(router);

const { registered: ionicRegistered } = registerIonicComponents(app);
console.log(`[ionic] Registered ${ionicRegistered.length} Ionic Vue components`);

initSharedI18n();
registerI18nModule(pluginI18n);

const { t } = useI18n();
app.config.globalProperties.$t = t;

router.isReady().then(() => {
  app.mount("#app");
});
