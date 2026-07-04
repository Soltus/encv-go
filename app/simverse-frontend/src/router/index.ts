import { createRouter, createWebHistory } from "vue-router";
import type { RouteRecordRaw } from "vue-router";
import Tabs from "@self/views/SimverseTabs.vue";

const routes: RouteRecordRaw[] = [
  {
    path: "/",
    redirect: "/tabs/home",
  },
  {
    path: "/world",
    component: () => import("@self/views/SimverseWorld.vue"),
  },
  {
    path: "/tabs/",
    component: Tabs,
    children: [
      {
        path: "",
        redirect: "/tabs/home",
      },
      {
        path: "home",
        component: () => import("@self/views/SimverseHome.vue"),
      },
      {
        path: "settings",
        component: () => import("@self/views/SimverseSettings.vue"),
      },
      {
        path: "devlogs",
        component: () => import("@self/views/SimverseDevLogs.vue"),
      },
    ],
  },
];

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
});

export default router;
