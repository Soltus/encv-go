import { IonicVue } from "@ionic/vue";
import { createPinia } from "pinia";
import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";
import { registerIonicComponents } from "./composables/useIonicAutoRegister";

import "@ionic/vue/css/core.css";
import "@ionic/vue/css/normalize.css";
import "@ionic/vue/css/structure.css";
import "@ionic/vue/css/typography.css";
import "@ionic/vue/css/padding.css";
import "@ionic/vue/css/flex-utils.css";
import "@ionic/vue/css/display.css";
import "./theme/variables.css";

const pinia = createPinia();
const app = createApp(App).use(IonicVue).use(router).use(pinia);

const { registered } = registerIonicComponents(app);
console.log(`[ionic] Registered ${registered.length} Ionic Vue components`);

router.isReady().then(() => {
  app.mount("#app");
});
