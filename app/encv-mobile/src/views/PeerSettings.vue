<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('peers.title') }}</ion-title>
      </ion-toolbar>
    </ion-header>

    <ion-content class="peersContent">
      <PeerScanPanel />

      <PeerPairingPanel />

      <!-- 🆕 P3：互联搜索（本端 + 已配对设备，带来源徽章） -->
      <div class="fedSearch">
        <div class="fedSearchRow">
          <input
            v-model="fedQuery"
            class="fedInput"
            type="text"
            data-testid="fed-input"
            :placeholder="t('peers.searchPlaceholder') || '搜索本机与已配对设备…'"
            spellcheck="false"
            @keyup.enter="runFedSearch"
          />
          <ion-button size="small" :disabled="fedBusy" data-testid="fed-run" @click="runFedSearch">
            {{ t('peers.search') || '搜索' }}
          </ion-button>
        </div>
        <p class="fedHint">{{ t('peers.federatedHint') || '远端结果只作为跨端引用展示，不会挂载为本地路径' }}</p>

        <div v-if="peerStatuses.length > 0" class="fedStatusRow" data-testid="fed-status">
          <span v-for="st in peerStatuses" :key="st.peerId" class="fedStatusChip" :class="`fedStatus_${st.state}`">
            {{ st.name || st.peerId }}：{{ fedStateLabel(st.state) }}
          </span>
        </div>

        <div v-if="fedItems.length > 0" class="fedResults" data-testid="fed-results">
          <div v-for="hit in fedItems" :key="hit.key" class="fedItem">
            <PeerSourceBadge :source="hit.source" :peer-name="hit.peerName" :peer-platform="hit.raw?.platform as string" />
            <span class="fedPath">{{ hit.path }}</span>
            <!-- P3.4：远端命中**只能**在线打开/取回，且请求始终带 peerId + 远端 path（不产生本地路径语义） -->
            <a
              v-if="hit.source === 'peer'"
              class="fedOpen"
              data-testid="fed-open"
              target="_blank"
              rel="noopener"
              :href="peerFileUrl(hit)"
            >{{ t('peers.openOnline') || '在线打开' }}</a>
          </div>
        </div>
        <p v-else-if="fedRan" class="fedEmpty">{{ t('files.noSearchResults') || '没有匹配结果' }}</p>
      </div>

      <ion-list>
        <ion-list-header>
          <ion-label>{{ t('peers.title') }}</ion-label>
        </ion-list-header>

        <ion-item v-if="peers.length === 0">
          <ion-label class="peersEmpty">{{ t('peers.empty') }}</ion-label>
        </ion-item>

        <ion-item v-for="p in peers" :key="p.id">
          <ion-label>
            <h3>{{ p.name || p.deviceId }}</h3>
            <p>{{ p.platform }} · {{ p.id }}</p>
            <p>
              <ion-badge :color="p.online ? 'success' : 'medium'">
                {{ p.online ? t('settings.online') : t('settings.offline') }}
              </ion-badge>
            </p>
          </ion-label>
          <ion-button slot="end" size="small" color="danger" fill="outline" @click="handleUnpair(p.id)">
            {{ t('peers.unpair') }}
          </ion-button>
        </ion-item>
      </ion-list>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import {
  IonBackButton,
  IonBadge,
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonItem,
  IonLabel,
  IonList,
  IonListHeader,
  IonPage,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { onMounted, onUnmounted, ref } from "vue";
import PeerPairingPanel from "@/components/PeerPairingPanel.vue";
import PeerScanPanel from "@/components/PeerScanPanel.vue";
import PeerSourceBadge from "@/components/PeerSourceBadge.vue";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { fetchPeers, unpairPeer, usePeerLink } from "@encv/shared-components/composables/usePeerLink";
import { type FederatedHit, type PeerSearchStatus, searchFederated } from "@encv/shared-components/composables/useFederatedSearch";

const { t } = useI18n();
const { peers } = usePeerLink();

const fedQuery = ref("");
const fedItems = ref<FederatedHit[]>([]);
const peerStatuses = ref<PeerSearchStatus[]>([]);
const fedBusy = ref(false);
const fedRan = ref(false);

function fedStateLabel(state: PeerSearchStatus["state"]): string {
  switch (state) {
    case "ok":
      return "在线";
    case "offline":
      return "离线";
    case "timeout":
      return "超时";
    default:
      return "异常";
  }
}

/** P3.4：远端文件的在线打开链接（始终带 peerId + 远端 path，绝不伪装成本地路径） */
function peerFileUrl(hit: FederatedHit): string {
  const qs = new URLSearchParams({ peerId: hit.peerId, path: hit.path });
  return `/api/peerlink/file?${qs.toString()}`;
}

async function runFedSearch() {
  if (!fedQuery.value.trim()) return;
  fedBusy.value = true;
  try {
    const res = await searchFederated(fedQuery.value, { peers: peers.value, timeoutMs: 2000 });
    fedItems.value = res.items;
    peerStatuses.value = res.peerStatuses;
    fedRan.value = true;
  } finally {
    fedBusy.value = false;
  }
}

// ⚠️ 在线状态必须持续刷新（2026-10-04）：
//   旧实现只在 onMounted 拉一次 peers，之后**再也不更新** ——
//   于是"点完信任后对端掉线/压根没连上"时，桌面端 UI 永远停在那一刻的状态，
//   用户看到的就是"信任了却还是连不上"（实际是连上又断了 / 从未连上）。
const PEER_POLL_MS = 5000;
let peerPollTimer: ReturnType<typeof setInterval> | null = null;

onMounted(() => {
  void fetchPeers().catch(() => undefined);
  peerPollTimer = setInterval(() => {
    // 页面不可见时不打搅后端（切后台/息屏）
    if (typeof document !== "undefined" && document.visibilityState === "hidden") return;
    void fetchPeers().catch(() => undefined);
  }, PEER_POLL_MS);
});

onUnmounted(() => {
  if (peerPollTimer) {
    clearInterval(peerPollTimer);
    peerPollTimer = null;
  }
});

async function handleUnpair(peerId: string) {
  await unpairPeer(peerId);
  await fetchPeers().catch(() => undefined);
}
</script>

<style scoped>
.peersContent {
  --padding-start: 12px;
  --padding-end: 12px;
}

.peersEmpty {
  opacity: 0.7;
}

/* ── P3 互联搜索区 ── */
.fedSearch {
  padding: 8px 0 4px;
}

.fedSearchRow {
  display: flex;
  gap: 8px;
  align-items: center;
}

.fedInput {
  flex: 1;
  min-width: 0;
  padding: 8px 10px;
  border: 1px solid var(--color-base-300);
  border-radius: var(--radius-field, 0.5rem);
  background: var(--color-base-100);
  color: var(--color-base-content);
}

.fedHint {
  font-size: 0.75rem;
  opacity: 0.7;
  margin: 6px 0;
}

.fedStatusRow {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 6px 0;
}

.fedStatusChip {
  font-size: 0.7rem;
  padding: 2px 8px;
  border-radius: 999px;
  background: var(--color-base-200);
}

.fedStatus_offline {
  background: var(--color-warning, #f59e0b);
  color: var(--color-warning-content, #1f2937);
}

.fedStatus_timeout {
  background: var(--color-error, #b91c1c);
  color: #fff;
}

.fedResults {
  border: 1px solid var(--color-base-300);
  border-radius: var(--radius-box, 0.75rem);
  margin-top: 6px;
}

.fedItem {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-bottom: 1px solid var(--color-base-300);
}

.fedItem:last-child {
  border-bottom: none;
}

.fedPath {
  flex: 1;
  min-width: 0;
  font-size: 0.8rem;
  word-break: break-all;
}

.fedOpen {
  flex-shrink: 0;
  font-size: 0.75rem;
  color: var(--color-primary);
  text-decoration: none;
  font-weight: 600;
}

.fedEmpty {
  font-size: 0.8rem;
  opacity: 0.7;
  margin: 8px 0;
}
</style>
