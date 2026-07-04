import { createRouter, createWebHistory } from "vue-router";
import type { RouteRecordRaw } from "vue-router";
import SimverseTabs from "@self/views/SimverseTabs.vue";

const routes: RouteRecordRaw[] = [
  { path: "/", redirect: "/tabs/home" },
  {
    path: "/tabs/",
    component: SimverseTabs,
    children: [
      { path: "", redirect: "/tabs/home" },
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
  {
    path: "/world",
    component: () => import("@self/views/SimverseWorld.vue"),
  },
  {
    path: "/chronicle",
    component: () => import("@self/views/ChronicleDetail.vue"),
  },
];

export default createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
});
