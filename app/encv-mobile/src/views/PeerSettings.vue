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

      <!-- 🆕 2026-10-05：热更新（待办 ②）
           移动端：本端装了哪版 / 能不能退回去；桌面端：云控视角的记录与一键回滚。 -->
      <div class="bundleBox" data-testid="bundle-box">
        <h3 class="bundleTitle">{{ t('peers.bundleTitle') }}</h3>

        <p class="bundleSub">{{ t('peers.bundleLocal') }}</p>
        <p v-if="localBundles.length === 0" class="bundleEmpty" data-testid="bundle-local-empty">
          {{ t('peers.bundleLocalNone') }}
        </p>
        <div v-for="b in localBundles" :key="b.name" class="bundleRow" data-testid="bundle-local-item">
          <span class="bundleName">{{ b.name }}</span>
          <span class="bundleVer" data-testid="bundle-local-version">{{ b.version || t('peers.bundleNotInstalled') }}</span>
          <ion-button
            size="small"
            fill="outline"
            data-testid="bundle-local-rollback"
            :disabled="bundleBusy || !b.rollable"
            @click="handleLocalRollback(b.name)"
          >{{ t('peers.bundleRollback') }}</ion-button>
        </div>
        <p v-if="!localBundles.some(b => b.rollable)" class="bundleEmpty">{{ t('peers.bundleNoBackup') }}</p>

        <p class="bundleSub">{{ t('peers.bundleCloud') }}</p>
        <p class="bundleMeta" data-testid="bundle-available">
          {{
            bundleStatus && bundleStatus.available.length
              ? String(t('peers.bundleAvailable')).replace('{list}', bundleStatus.available.map(a => `${a.name}@${a.version}`).join('、'))
              : t('peers.bundleAvailableNone')
          }}
        </p>
        <div v-for="(ver, pid) in (bundleStatus?.deviceVer || {})" :key="pid" class="bundleRow" data-testid="bundle-device">
          <span class="bundleName">{{ pid }}</span>
          <span class="bundleVer">{{ ver }}</span>
          <ion-button
            size="small"
            fill="outline"
            data-testid="bundle-cloud-rollback"
            :disabled="bundleBusy"
            @click="handleCloudRollback(pid as string, ver as string)"
          >{{ t('peers.bundleRollback') }}</ion-button>
        </div>

        <p class="bundleSub">{{ t('peers.bundleReports') }}</p>
        <p v-if="!bundleStatus?.reports?.length" class="bundleEmpty" data-testid="bundle-reports-empty">
          {{ t('peers.bundleReportsNone') }}
        </p>
        <div v-for="(r, i) in (bundleStatus?.reports || []).slice(-8).reverse()" :key="i" class="bundleReport" data-testid="bundle-report">
          <span class="bundleName">{{ r.name }}@{{ r.version || '-' }}</span>
          <span class="bundleVer">{{ r.peerId }}</span>
          <span class="bundleTag" :class="r.ok ? 'bundleTag_ok' : 'bundleTag_fail'">
            {{ r.ok ? t('peers.bundleReportOk') : t('peers.bundleReportFail') }}
          </span>
          <span v-if="r.rolledBack" class="bundleTag bundleTag_rollback">{{ t('peers.bundleReportRolledBack') }}</span>
        </div>

        <p v-if="bundleMsg" class="bundleMsg" data-testid="bundle-msg">{{ bundleMsg }}</p>
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
import {
  fetchBundleStatus,
  fetchLocalBundles,
  fetchPeers,
  type BundleStatus,
  type LocalBundle,
  rollbackBundle,
  rollbackLocalBundle,
  unpairPeer,
  usePeerLink,
} from "@encv/shared-components/composables/usePeerLink";
import { type FederatedHit, type PeerSearchStatus, searchFederated } from "@encv/shared-components/composables/useFederatedSearch";

const { t } = useI18n();
const { peers } = usePeerLink();

const fedQuery = ref("");
const fedItems = ref<FederatedHit[]>([]);
const peerStatuses = ref<PeerSearchStatus[]>([]);
const fedBusy = ref(false);
const fedRan = ref(false);

// ── 热更新（2026-10-05）──
const localBundles = ref<LocalBundle[]>([]);
const bundleStatus = ref<BundleStatus | null>(null);
const bundleBusy = ref(false);
const bundleMsg = ref("");

async function refreshBundles() {
  try {
    localBundles.value = await fetchLocalBundles();
  } catch {
    localBundles.value = []; // 本端状态查不到不该让整页红（桌面端没有设备侧目标属正常）
  }
  try {
    bundleStatus.value = await fetchBundleStatus();
  } catch {
    bundleStatus.value = null;
  }
}

/** 设备**自己**退回上一版（不经过云端；用户就在设备跟前，没理由绕一圈） */
async function handleLocalRollback(name: string) {
  bundleBusy.value = true;
  bundleMsg.value = "";
  try {
    const out = await rollbackLocalBundle(name);
    bundleMsg.value = String(t("peers.bundleRollbackDone") || "已回滚到：{version}").replace("{version}", out.version || "");
    await refreshBundles();
  } catch (e) {
    bundleMsg.value = String(t("peers.bundleRollbackFailed") || "回滚失败：{detail}").replace(
      "{detail}",
      e instanceof Error ? e.message : String(e),
    );
  } finally {
    bundleBusy.value = false;
  }
}

/** 云控一键回滚：让某台已配对设备退回它的上一版 */
async function handleCloudRollback(peerId: string, ver: string) {
  // deviceVer 形如 "web@v0.0.10" ⇒ 包名是最后一个 '@' 之前的部分
  const name = ver.includes("@") ? ver.slice(0, ver.lastIndexOf("@")) : ver;
  bundleBusy.value = true;
  bundleMsg.value = "";
  try {
    const out = await rollbackBundle(peerId, name);
    bundleMsg.value = String(t("peers.bundleRollbackDone") || "已回滚到：{version}").replace("{version}", out.version || "");
    await refreshBundles();
  } catch (e) {
    bundleMsg.value = String(t("peers.bundleRollbackFailed") || "回滚失败：{detail}").replace(
      "{detail}",
      e instanceof Error ? e.message : String(e),
    );
  } finally {
    bundleBusy.value = false;
  }
}

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
  void refreshBundles();
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

/* ── 2026-10-05 热更新区块 ── */
.bundleBox {
  margin: 12px 0;
  padding: 10px 12px;
  border: 1px solid var(--color-base-300);
  border-radius: var(--radius-box, 0.75rem);
}

.bundleTitle {
  font-size: 0.95rem;
  font-weight: 600;
  margin: 0 0 6px;
}

.bundleSub {
  font-size: 0.75rem;
  font-weight: 600;
  opacity: 0.85;
  margin: 10px 0 4px;
}

.bundleMeta,
.bundleEmpty {
  font-size: 0.75rem;
  opacity: 0.75;
  margin: 4px 0;
  word-break: break-all;
}

.bundleRow,
.bundleReport {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
  border-bottom: 1px solid var(--color-base-300);
}

.bundleReport:last-child {
  border-bottom: none;
}

.bundleName {
  font-size: 0.78rem;
  font-weight: 600;
  min-width: 0;
  word-break: break-all;
}

.bundleVer {
  flex: 1;
  min-width: 0;
  font-size: 0.72rem;
  opacity: 0.8;
  font-family: ui-monospace, monospace;
  word-break: break-all;
}

.bundleTag {
  font-size: 0.68rem;
  padding: 1px 6px;
  border-radius: 999px;
  background: var(--color-base-200);
}

.bundleTag_ok {
  background: var(--color-success, #16a34a);
  color: #fff;
}

.bundleTag_fail {
  background: var(--color-error, #b91c1c);
  color: #fff;
}

.bundleTag_rollback {
  background: var(--color-warning, #f59e0b);
  color: #1f2937;
}

.bundleMsg {
  font-size: 0.75rem;
  margin: 8px 0 0;
  color: var(--color-base-content);
  opacity: 0.85;
  word-break: break-all;
}
</style>
