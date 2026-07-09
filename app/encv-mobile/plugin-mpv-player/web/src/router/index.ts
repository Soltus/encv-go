import { createRouter, createWebHashHistory } from "@ionic/vue-router";
import type { RouteRecordRaw } from "vue-router";
import MpvAbout from "@/views/MpvAbout.vue";
import MpvDevLogs from "@/views/MpvDevLogs.vue";
import MpvHome from "@/views/MpvHome.vue";
import MpvSettings from "@/views/MpvSettings.vue";
import NotFoundView from "@/views/NotFoundView.vue";

export const routes: RouteRecordRaw[] = [
  { path: "/", redirect: "/home" },
  { path: "/home", component: MpvHome },
  { path: "/settings", component: MpvSettings },
  { path: "/settings/about", component: MpvAbout },
  { path: "/devlogs", component: MpvDevLogs },
  {
    path: "/:pathMatch(.*)*",
    name: "not-found",
    component: NotFoundView,
  },
];

export const router = createRouter({
  history: createWebHashHistory("/mpv-ui/"),
  routes,
});
