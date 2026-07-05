import { createApp } from "vue";
import App from "./App.vue";
import router from "./router";

const app = createApp(App);
app.use(router);
app.mount("#app");

window.onerror = function(msg, src, line, col, err) {
  document.body.innerHTML = '<div style="color:red;padding:20px;background:#fee;">错误: ' + (err?.stack || msg) + '</div>';
  return true;
};