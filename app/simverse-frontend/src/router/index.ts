import { createRouter, createWebHistory } from "vue-router";
import type { RouteRecordRaw } from "vue-router";

const routes: RouteRecordRaw[] = [
  { path: "/", redirect: "/simverse-home" },
  {
    path: "/simverse-home",
    component: () => import("@self/views/SimverseHome.vue"),
  },
  {
    path: "/world",
    component: () => import("@self/views/SimverseWorld.vue"),
  },
  {
    path: "/chronicle",
    component: () => import("@self/views/ChronicleDetail.vue"),
  },
  {
    path: "/tabs/settings",
    component: () => import("@self/views/SimverseSettings.vue"),
  },
  {
    path: "/tabs/devlogs",
    component: () => import("@self/views/SimverseDevLogs.vue"),
  },
];

export default createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
});
