import { createRouter, createWebHistory } from "@ionic/vue-router";
import type { RouteRecordRaw } from "vue-router";

const routes: RouteRecordRaw[] = [
  {
    path: "/",
    redirect: "/tabs/home",
  },
  {
    path: "/tabs/",
    component: () => import("@/views/Tabs.vue"),
    children: [
      {
        path: "",
        redirect: "/tabs/home",
      },
      {
        path: "home",
        component: () => import("@/views/SimverseHome.vue"),
      },
      {
        path: "world",
        component: () => import("@/views/SimverseWorld.vue"),
      },
      {
        path: "settings",
        component: () => import("@/views/SimverseSettings.vue"),
      },
      {
        path: "devlogs",
        component: () => import("@/views/SimverseDevLogs.vue"),
      },
    ],
  },
  {
    path: "/chronicle/:id",
    component: () => import("@/views/ChronicleDetail.vue"),
  },
];

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
});

export default router;
