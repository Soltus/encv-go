import { IonicVue } from "@ionic/vue";
import { createPinia } from "pinia";
import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import { registerIonicComponents } from "./composables/useIonicAutoRegister";
import { useI18n, registerI18nModule } from "@encv/shared-components/composables/useI18n";
import simverseI18n from "./i18n/simverse";

import "@ionic/vue/css/core.css";
import "@ionic/vue/css/normalize.css";
import "@ionic/vue/css/structure.css";
import "@ionic/vue/css/typography.css";
import "./theme/variables.css";
import "./theme/simverse.css";

const pinia = createPinia();
const app = createApp(App).use(IonicVue).use(router).use(pinia);

const { registered } = registerIonicComponents(app);
console.log(`[ionic] Registered ${registered.length} Ionic Vue components`);

registerI18nModule(simverseI18n);

const { t } = useI18n();
app.config.globalProperties.$t = t;

router.isReady().then(() => {
  app.mount("#app");
});
