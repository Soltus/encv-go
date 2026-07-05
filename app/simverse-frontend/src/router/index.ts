import type { RouteRecordRaw } from "vue-router";
import { createRouter, createWebHistory } from "vue-router";
import SimverseHome from "../views/SimverseHome.vue";
import SimverseWorld from "../views/SimverseWorld.vue";
import ChronicleDetail from "../views/ChronicleDetail.vue";
import SimverseSettings from "../views/SimverseSettings.vue";
import SimverseDevLogs from "../views/SimverseDevLogs.vue";

const routes: RouteRecordRaw[] = [
  { path: "/", redirect: "/simverse-home" },
  { path: "/simverse-home", component: SimverseHome },
  { path: "/world", component: SimverseWorld },
  { path: "/chronicle", component: ChronicleDetail },
  { path: "/tabs/settings", component: SimverseSettings },
  { path: "/tabs/devlogs", component: SimverseDevLogs },
];

export default createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
});
