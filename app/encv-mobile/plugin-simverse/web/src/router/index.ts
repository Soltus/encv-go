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
        component: () => import("@/views/WorldMapView.vue"),
      },
      {
        path: "npcs",
        component: () => import("@/views/NPCList.vue"),
      },
      {
        path: "orgs",
        component: () => import("@/views/OrgList.vue"),
      },
      {
        path: "regions",
        component: () => import("@/views/RegionList.vue"),
      },
      {
        path: "chronicles",
        component: () => import("@/views/ChronicleList.vue"),
      },
      {
        path: "economy",
        component: () => import("@/views/EconomyOverview.vue"),
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
    path: "/npc/:id",
    component: () => import("@/views/NPCDetail.vue"),
  },
  {
    path: "/npc/:id/relations",
    component: () => import("@/views/NPCRelations.vue"),
  },
  {
    path: "/npc/:id/timeline",
    component: () => import("@/views/NPCTimeline.vue"),
  },
  {
    path: "/npc/:id/inventory",
    component: () => import("@/views/NPCInventory.vue"),
  },
  {
    path: "/org/:id",
    component: () => import("@/views/OrgDetail.vue"),
  },
  {
    path: "/org/:id/members",
    component: () => import("@/views/OrgMembers.vue"),
  },
  {
    path: "/org/:id/territory",
    component: () => import("@/views/OrgTerritory.vue"),
  },
  {
    path: "/region/:id",
    component: () => import("@/views/RegionDetail.vue"),
  },
  {
    path: "/chronicle/:id",
    component: () => import("@/views/ChronicleDetail.vue"),
  },
  {
    path: "/era/:id",
    component: () => import("@/views/EraOverview.vue"),
  },
  {
    path: "/chronicle/:id/causal",
    component: () => import("@/views/ChronicleCausal.vue"),
  },
  {
    path: "/economy/prices",
    component: () => import("@/views/EconomyPrices.vue"),
  },
  {
    path: "/economy/trade",
    component: () => import("@/views/EconomyTrade.vue"),
  },
  {
    path: "/settings/performance",
    component: () => import("@/views/PerformanceSettings.vue"),
  },
  {
    path: "/settings/simulation",
    component: () => import("@/views/SimulationSettings.vue"),
  },
  {
    path: "/settings/saves",
    component: () => import("@/views/SaveManagement.vue"),
  },
  {
    path: "/settings/about",
    component: () => import("@/views/AboutSimverse.vue"),
  },
  {
    path: "/world",
    component: () => import("@/views/SimverseWorld.vue"),
  },
  {
    path: "/world/npc/:id",
    component: () => import("@/views/WorldNPCDetail.vue"),
  },
  {
    path: "/world/org/:id",
    component: () => import("@/views/WorldOrgDetail.vue"),
  },
  {
    path: "/world/chronicles",
    component: () => import("@/views/WorldChronicles.vue"),
  },
  {
    path: "/world/economy",
    component: () => import("@/views/WorldEconomy.vue"),
  },
  {
    path: "/world/intervention",
    component: () => import("@/views/WorldIntervention.vue"),
  },
  {
    path: "/world/debug/perf",
    component: () => import("@/views/WorldDebugPerf.vue"),
  },
  {
    path: "/world/debug/entities",
    component: () => import("@/views/WorldDebugEntities.vue"),
  },
];

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
});

export default router;
