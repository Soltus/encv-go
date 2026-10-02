<template>
  <ion-page>
    <!--
      手机 / 平板形态：ion-tabs + 底部 tab 栏（零回归，一切照旧）
      桌面形态（spec desktop-web-android-pairing P1.2）：
        侧边导航 rail + ion-router-outlet 直接承载内容（不用 ion-tabs ——
        它是 shadow DOM，内部布局从外部够不到；路由匹配本就不依赖 ion-tabs）。
      代价：跨越断点时两种壳会切换重挂载（形态极少变化，可接受，见 progress 记录）。
    -->
    <ion-tabs
      v-if="!isDesktop"
      @ionTabsWillChange="onTabsWillChange"
      @ionTabsDidChange="onTabsDidChange"
    >
      <ion-router-outlet></ion-router-outlet>
      <ion-tab-bar slot="bottom">
        <ion-tab-button tab="home" href="/tabs/home">
          <ion-icon :icon="home"></ion-icon>
          <ion-label>{{ t('tabs.home') }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="files" href="/tabs/files">
          <ion-icon :icon="folder"></ion-icon>
          <ion-label>{{ t('tabs.files') }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="tasks" href="/tabs/tasks">
          <ion-icon :icon="list"></ion-icon>
          <ion-label>{{ t('tabs.tasks') }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="remote" href="/tabs/remote">
          <ion-icon :icon="globe"></ion-icon>
          <ion-label>{{ t('tabs.remote') }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="settings" href="/tabs/settings">
          <ion-icon :icon="settings"></ion-icon>
          <ion-label>{{ t('tabs.settings') }}</ion-label>
        </ion-tab-button>

        <ion-tab-button tab="devlogs" href="/tabs/devlogs">
          <ion-icon :icon="bug"></ion-icon>
          <ion-label>DevLogs</ion-label>
        </ion-tab-button>
      </ion-tab-bar>
    </ion-tabs>

    <!-- 桌面壳：左侧导航 + 内容出口（light DOM，令牌驱动，可被用户主题覆写） -->
    <div v-else class="desktop-shell">
      <nav class="desktop-rail" aria-label="Primary">
        <router-link
          v-for="item in railItems"
          :key="item.href"
          :to="item.href"
          class="rail-item"
        >
          <ion-icon :icon="item.icon" class="rail-icon"></ion-icon>
          <span class="rail-label">{{ item.label }}</span>
        </router-link>
      </nav>
      <div class="desktop-content">
        <ion-router-outlet></ion-router-outlet>
      </div>
    </div>
    <!-- P5：降级矩阵持久性条幅（非 Toast）+ P4：执行端审批弹窗 -->
    <PeerDegradedNotice />
    <RemoteApprovalPrompt />
  </ion-page>
</template>

<script setup lang="ts">
import { bug, folder, globe, home, list, settings } from "ionicons/icons";
import { computed, onMounted, onUnmounted } from "vue";
import { useFormFactor } from "@encv/shared-components/composables/useFormFactor";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import RemoteApprovalPrompt from "@/components/RemoteApprovalPrompt.vue";
import PeerDegradedNotice from "@/components/PeerDegradedNotice.vue";
import { startRemoteApprovalPolling, stopRemoteApprovalPolling } from "@/composables/useRemoteApproval";
import { startPeerDegradationPolling, stopPeerDegradationPolling } from "@/composables/usePeerDegradation";

const { t } = useI18n();
const { isDesktop } = useFormFactor();

// 执行端：轮询本端挂起的远程调用请求（未启用/未配对时静默降级）
onMounted(() => {
  startRemoteApprovalPolling();
  startPeerDegradationPolling(); // P5：降级矩阵常驻条幅
});
onUnmounted(() => {
  stopRemoteApprovalPolling();
  stopPeerDegradationPolling();
});

// 与底部 ion-tab-bar 同一份导航语义（桌面 rail 是它的形态变体，不是第二套信息架构）
const railItems = computed(() => [
  { href: "/tabs/home", icon: home, label: t("tabs.home") },
  { href: "/tabs/files", icon: folder, label: t("tabs.files") },
  { href: "/tabs/tasks", icon: list, label: t("tabs.tasks") },
  { href: "/tabs/remote", icon: globe, label: t("tabs.remote") },
  { href: "/tabs/settings", icon: settings, label: t("tabs.settings") },
  { href: "/tabs/devlogs", icon: bug, label: "DevLogs" },
]);

function onTabsWillChange(event: CustomEvent) {
  void event?.detail?.tab;
}

function onTabsDidChange(event: CustomEvent) {
  void event?.detail?.tab;
}
</script>

<style scoped>
/* ── 桌面壳 ── */
.desktop-shell {
  --desktop-rail-width: 224px;
  display: flex;
  width: 100%;
  height: 100%;
}

.desktop-rail {
  width: var(--desktop-rail-width);
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px 8px;
  background: var(--color-base-100);
  border-right: 1px solid var(--color-base-300);
}

.rail-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: var(--radius-field, 0.5rem);
  color: var(--color-base-content);
  text-decoration: none;
  font-size: var(--text-sm, 0.875rem);
  transition: background-color 0.16s ease;
}

.rail-item:hover {
  background: var(--color-base-200);
}

.rail-item.router-link-active {
  background: var(--color-primary);
  color: var(--color-primary-content);
  font-weight: 600;
}

.rail-icon {
  font-size: 1.25rem;
  flex-shrink: 0;
}

.rail-label {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 内容区：router-outlet 由 Ionic 自带 absolute 全填充，这里只提供定位容器 */
.desktop-content {
  flex: 1;
  min-width: 0;
  position: relative;
  overflow: hidden;
  background: var(--color-base-100);
}
</style>
