import { registerIonicComponents } from "@encv/shared-components/composables/useIonicAutoRegister";
import { IonicVue } from "@ionic/vue";
import { createApp } from "vue";
import App from "./App.vue";
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

router.isReady().then(() => {
  app.mount("#app");
});
