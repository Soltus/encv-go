<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings/devtools"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('devtools.webdavTests') }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="showHistory = !showHistory" fill="clear" :title="t('devtools.viewHistory')">
            <ion-icon :icon="timeOutline" slot="icon-only"></ion-icon>
          </ion-button>
          <ion-button @click="refreshManifest" fill="clear" :title="t('devtools.webdav.refreshManifest')" :disabled="manifest.loading.value">
            <ion-icon :icon="sync" slot="icon-only" :class="{ spin: manifest.loading.value }"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!-- WebDAV 启用状态 banner -->
      <div v-if="webDavEnabled === false" class="webdav-status-banner webdav-status-disabled">
        <ion-icon :icon="warningOutline" class="banner-icon"></ion-icon>
        <div class="banner-text">
          <strong>{{ t('devtools.webdavAuth.disabledTitle') }}</strong>
          <p>{{ t('devtools.webdavAuth.disabledHint') }}</p>
        </div>
        <ion-button fill="clear" size="small" @click="checkWebDavHealth">
          <ion-icon :icon="sync" slot="icon-only"></ion-icon>
        </ion-button>
      </div>
      <div v-else-if="webDavEnabled === true" class="webdav-status-banner webdav-status-enabled">
        <ion-icon :icon="cloudDoneOutline" class="banner-icon"></ion-icon>
        <div class="banner-text">
          <strong>{{ t('devtools.webdavAuth.enabledTitle') }}</strong>
          <p>endpoint: <code>{{ baseUrl }}{{ webdavPath }}</code></p>
        </div>
      </div>

      <!-- 顶部 Manifest 状态卡（多 mount 选择） -->
      <section class="manifest-card" :class="manifestTone">
        <div class="manifest-card-header">
          <ion-icon :icon="serverOutline" class="manifest-icon"></ion-icon>
          <div class="manifest-meta">
            <strong>{{ t('devtools.webdav.manifestStatus') }}</strong>
            <p v-if="manifest.error.value" class="manifest-error">
              {{ t('devtools.webdav.manifestError') }}: {{ manifest.error.value.message }}
            </p>
            <p v-else-if="manifest.loading.value" class="manifest-loading">
              {{ t('devtools.webdav.manifestLoading') }}
            </p>
            <p v-else-if="availableMounts.length === 0" class="manifest-empty">
              {{ t('devtools.webdav.manifestEmpty') }}
              <span class="manifest-empty-hint">· {{ t('devtools.webdav.manifestEmptyHint') }}</span>
            </p>
            <p v-else class="manifest-ready">
              <span class="ready-pill">{{ t('devtools.webdav.manifestReady') }}</span>
              <span class="mount-count">{{ availableMounts.length }} mounts</span>
              <span class="attack-tagged" v-if="attackCountTotal > 0">
                · {{ t('devtools.webdav.attackTagged', { count: String(attackCountTotal) }) }}
              </span>
            </p>
            <!-- 🆕 2026-06-17 当前 activeMount 的注册容器扩展名（权威显示） -->
            <!-- 目的：用户一眼看出 manifest 实际注册的 container 扩展名是哪些 -->
            <!-- 任何 .encv 硬编码或期望 .sccg* 都会与这里对比立即发现 -->
            <div
              v-if="manifest.activeMount.value && (manifest.activeMount.value.manifest.registered_container_exts?.length ?? 0) > 0"
              class="manifest-container-exts"
            >
              <span class="exts-label">已注册容器扩展名:</span>
              <code
                v-for="ext in manifest.activeMount.value.manifest.registered_container_exts"
                :key="ext"
                class="ext-pill"
              >{{ ext }}</code>
            </div>
          </div>
        </div>
        <!-- Mount 选择器（仅多 mount 时显示） -->
        <div v-if="availableMounts.length > 1" class="mount-selector">
          <ion-label class="mount-selector-label">{{ t('devtools.webdav.mountSelector') }}</ion-label>
          <div class="mount-chips">
            <button
              v-for="m in availableMounts"
              :key="m.name"
              class="mount-chip"
              :class="{ active: m.name === manifest.activeMountName.value, 'is-default': m.is_default }"
              @click="manifest.setActiveMount(m.name)"
              type="button"
            >
              <ion-icon :icon="m.is_default ? starOutline : folderOutline" class="chip-icon"></ion-icon>
              <span class="chip-name">{{ m.name }}</span>
              <code class="chip-path">{{ m.webdav_path }}</code>
            </button>
          </div>
        </div>
      </section>

      <!-- 账号配置面板（折叠） -->
      <ion-list>
        <ion-item button @click="showAuthPanel = !showAuthPanel" :detail="false">
          <ion-icon :icon="keyOutline" slot="start" color="medium"></ion-icon>
          <ion-label>
            <h3>{{ t('devtools.webdavAuth.title') }}</h3>
            <p>
              <span v-if="authRequired" class="auth-status-required">{{ t('devtools.webdavAuth.required') }}</span>
              <span v-else class="auth-status-optional">{{ t('devtools.webdavAuth.optional') }}</span>
              ·
              <span class="creds-summary">{{ maskedUsername }}</span>
            </p>
          </ion-label>
          <ion-icon :icon="showAuthPanel ? chevronUp : chevronDown" slot="end"></ion-icon>
        </ion-item>
        <div v-if="showAuthPanel" class="auth-panel-body">
          <ion-item>
            <ion-label position="stacked">{{ t('devtools.webdavAuth.username') }}</ion-label>
            <ion-input
              v-model="credsUsername"
              :placeholder="t('devtools.webdavAuth.usernamePlaceholder')"
              autocapitalize="off"
              autocorrect="off"
              :spellcheck="false"
            ></ion-input>
          </ion-item>
          <ion-item>
            <ion-label position="stacked">{{ t('devtools.webdavAuth.password') }}</ion-label>
            <ion-input
              v-model="credsPassword"
              :placeholder="t('devtools.webdavAuth.passwordPlaceholder')"
              type="password"
            ></ion-input>
          </ion-item>
          <ion-item button @click="saveCreds">
            <ion-icon :icon="checkmarkCircle" slot="start" color="primary"></ion-icon>
            <ion-label>
              <h3>{{ t('devtools.webdavAuth.save') }}</h3>
              <p>{{ t('devtools.webdavAuth.saveHint') }}</p>
            </ion-label>
          </ion-item>
          <ion-item button @click="resetToBackend" :disabled="!backendUsername">
            <ion-icon :icon="cloudDownloadOutline" slot="start" color="medium"></ion-icon>
            <ion-label>
              <h3>{{ t('devtools.webdavAuth.resetToBackend') }}</h3>
              <p v-if="backendUsername">{{ t('devtools.webdavAuth.backendHas') }}: <code>{{ backendUsername }}</code></p>
              <p v-else>{{ t('devtools.webdavAuth.backendNoAuth') }}</p>
            </ion-label>
          </ion-item>
        </div>
      </ion-list>

      <!-- 批量运行控制条 -->
      <section class="bulk-controls">
        <ion-button
          expand="block"
          color="primary"
          :disabled="isAnyRunning || !manifest.isReady.value"
          @click="handleRunAllModules"
        >
          <ion-icon :icon="playCircle" slot="start"></ion-icon>
          {{ t('devtools.webdav.runAllModules') }}
          <span class="bulk-meta" v-if="modules.length > 0">· {{ modules.length }} modules · {{ totalCases }} cases</span>
        </ion-button>
        <ion-button
          v-if="isAnyRunning"
          expand="block"
          fill="outline"
          color="danger"
          @click="handleCancelAll"
        >
          <ion-icon :icon="stopCircle" slot="start"></ion-icon>
          {{ t('devtools.webdav.cancelAll') }}
        </ion-button>
      </section>

      <!-- 7 Module Card 网格 -->
      <section class="module-grid-section">
        <ion-list-header class="module-grid-header">
          <ion-label>{{ t('devtools.webdavTests') }}</ion-label>
          <span class="header-meta">{{ t('devtools.testCases') }} · {{ totalCases }}</span>
        </ion-list-header>
        <p class="section-hint">{{ t('devtools.webdavTestsHint') }}</p>
        <div class="module-grid">
          <article
            v-for="m in modules"
            :key="m.id"
            class="module-card"
            :class="[
              `module-tone-${m.color}`,
              { 'is-running': isModuleRunning(m.id), 'is-done': isModuleDone(m.id) }
            ]"
          >
            <header class="module-card-header" @click="toggleModule(m.id)">
              <div class="module-icon-bubble" :class="`tone-${m.color}`">
                <ion-icon :icon="resolveIcon(m.icon)"></ion-icon>
              </div>
              <div class="module-title-block">
                <h3 class="module-title">{{ t(m.nameI18n) }}</h3>
                <p class="module-desc">{{ t(m.descI18n) }}</p>
              </div>
              <div class="module-stats">
                <div v-if="getModuleState(m.id).status === 'idle'" class="module-stat idle">
                  {{ m.cases.length }} {{ t('devtools.cases') }}
                </div>
                <div v-else class="module-stat progress" :class="getModuleState(m.id).status">
                  <span class="stat-passed">{{ getModulePassed(m.id) }}</span>
                  <span class="stat-sep">/</span>
                  <span class="stat-total">{{ m.cases.length }}</span>
                </div>
                <div v-if="getAttackCount(m.id) > 0" class="module-attack-badge">
                  <ion-icon :icon="shieldCheckmarkOutline"></ion-icon>
                  {{ t('devtools.webdav.moduleAttackBadge', { count: String(getAttackCount(m.id)) }) }}
                </div>
              </div>
              <ion-icon
                :icon="expandedModules.has(m.id) ? chevronUp : chevronDown"
                class="module-chevron"
              ></ion-icon>
            </header>

            <!-- 展开：用例列表 -->
            <div v-if="expandedModules.has(m.id)" class="module-card-body">
              <div class="module-actions">
                <ion-button
                  size="small"
                  fill="outline"
                  color="primary"
                  :disabled="isModuleRunning(m.id) || isAnyRunning || !manifest.isReady.value"
                  @click="runSingleModule(m.id)"
                >
                  <ion-icon :icon="playCircle" slot="start"></ion-icon>
                  Run
                </ion-button>
                <ion-button
                  v-if="isModuleRunning(m.id)"
                  size="small"
                  fill="outline"
                  color="danger"
                  @click="cancelSingleModule(m.id)"
                >
                  <ion-icon :icon="stopCircle" slot="start"></ion-icon>
                  {{ t('common.cancel') }}
                </ion-button>
              </div>
              <ul class="case-list">
                <li
                  v-for="c in m.cases"
                  :key="c.id"
                  class="case-row"
                  :class="`status-${getCaseStatus(m.id, c.id)}`"
                >
                  <span class="case-method" :class="`method-${c.method.toLowerCase()}`">{{ c.method }}</span>
                  <div class="case-body">
                    <div class="case-name">
                      {{ t(c.nameI18n) }}
                      <span v-if="c.attackType" class="case-attack-tag" :class="`attack-${c.attackType}`">
                        {{ t(`devtools.webdav.attackTypes.${c.attackType}`) }}
                      </span>
                    </div>
                    <p class="case-desc">{{ t(c.descI18n) }}</p>
                    <p v-if="getCaseResult(m.id, c.id)?.error" class="case-error">
                      <ion-icon :icon="warningOutline"></ion-icon>
                      {{ getCaseResult(m.id, c.id)?.error }}
                    </p>
                    <p v-if="getCaseResult(m.id, c.id)?.httpStatus" class="case-meta">
                      <ion-badge size="small" :color="getCaseStatusColor(m.id, c.id)">
                        {{ getCaseResult(m.id, c.id)?.httpStatus }}
                      </ion-badge>
                      <span class="case-duration">{{ getCaseResult(m.id, c.id)?.durationMs }}ms</span>
                    </p>
                  </div>
                  <div class="case-status-icon">
                    <ion-icon
                      v-if="getCaseStatus(m.id, c.id) === 'success'"
                      :icon="checkmarkCircle"
                      class="status-passed"
                    ></ion-icon>
                    <ion-icon
                      v-else-if="getCaseStatus(m.id, c.id) === 'failure' || getCaseStatus(m.id, c.id) === 'timed_out'"
                      :icon="closeCircle"
                      class="status-failed"
                    ></ion-icon>
                    <ion-icon
                      v-else-if="getCaseStatus(m.id, c.id) === 'skipped'"
                      :icon="removeCircle"
                      class="status-skipped"
                    ></ion-icon>
                    <ion-icon
                      v-else
                      :icon="ellipseOutline"
                      class="status-pending"
                    ></ion-icon>
                  </div>
                </li>
              </ul>
            </div>
          </article>
        </div>
      </section>

      <!-- 历史报告弹窗 -->
      <ion-modal :is-open="showHistory" @did-dismiss="showHistory = false">
        <ion-header>
          <ion-toolbar>
            <ion-title>{{ t('devtools.testReports') }}</ion-title>
            <ion-buttons slot="end">
              <ion-button @click="showHistory = false">{{ t('common.close') }}</ion-button>
            </ion-buttons>
          </ion-toolbar>
        </ion-header>
        <ion-content>
          <div class="history-header">
            <ion-button fill="clear" size="small" @click="refreshHistory">
              <ion-icon :icon="sync" slot="start"></ion-icon>
              {{ t('common.refresh') }}
            </ion-button>
            <ion-button
              fill="clear"
              size="small"
              color="danger"
              @click="handleClearHistory"
              v-if="historyRuns.length > 0"
            >
              <ion-icon :icon="trashOutline" slot="start"></ion-icon>
              {{ t('devtools.clearHistory') }}
            </ion-button>
          </div>
          <ion-list v-if="historyRuns.length > 0">
            <ion-item
              v-for="run in historyRuns"
              :key="run.id"
              button
              detail
              @click="openRunDetail(run)"
            >
              <ion-icon
                :icon="cloudDoneOutline"
                slot="start"
                :color="run.failed === 0 ? 'success' : 'danger'"
              ></ion-icon>
              <ion-label>
                <h3>
                  {{ formatTime(run.startedAt) }}
                  <ion-badge :color="run.failed === 0 ? 'success' : 'danger'">
                    {{ run.passed }}/{{ run.totalCases }} passed
                  </ion-badge>
                </h3>
                <p>
                  <span v-if="run.failed > 0" class="fail">{{ run.failed }} failed</span>
                  <span v-else class="ok">all passed</span>
                  · {{ formatDuration(run.startedAt, run.completedAt) }}
                  · #{{ run.id.slice(0, 16) }}
                </p>
                <p class="history-base-url">module: <code>{{ run.module }}</code></p>
              </ion-label>
            </ion-item>
          </ion-list>
          <div v-else class="empty-history">
            <ion-icon :icon="archiveOutline" class="empty-icon"></ion-icon>
            <h3>{{ t('devtools.noHistory') }}</h3>
            <p>{{ t('devtools.noHistoryHint') }}</p>
          </div>
        </ion-content>
      </ion-modal>

      <!-- 单个 run 详情弹窗 -->
      <ion-modal :is-open="!!detailRun" @did-dismiss="detailRun = null">
        <ion-header>
          <ion-toolbar>
            <ion-title>{{ detailRun ? formatTime(detailRun.startedAt) : '' }}</ion-title>
            <ion-buttons slot="end">
              <ion-button @click="detailRun = null">{{ t('common.close') }}</ion-button>
            </ion-buttons>
          </ion-toolbar>
        </ion-header>
        <ion-content v-if="detailRun">
          <div class="run-detail-summary">
            <div class="summary-stat ok">
              <div class="stat-value">{{ detailRun.passed }}</div>
              <div class="stat-label">passed</div>
            </div>
            <div class="summary-stat fail">
              <div class="stat-value">{{ detailRun.failed }}</div>
              <div class="stat-label">failed</div>
            </div>
            <div class="summary-stat skip">
              <div class="stat-value">{{ detailRun.skipped }}</div>
              <div class="stat-label">skipped</div>
            </div>
            <div class="summary-stat total">
              <div class="stat-value">{{ detailRun.totalCases }}</div>
              <div class="stat-label">total</div>
            </div>
          </div>
          <div class="run-detail-meta">
            <div><strong>Run ID:</strong> <code>{{ detailRun.id }}</code></div>
            <div><strong>Module:</strong> <code>{{ detailRun.module }}</code></div>
            <div><strong>Duration:</strong> {{ formatDuration(detailRun.startedAt, detailRun.completedAt) }}</div>
          </div>
          <ion-list>
            <ion-list-header>
              <ion-label>用例详情</ion-label>
            </ion-list-header>
            <ion-item
              v-for="r in detailRun.results"
              :key="r.id"
              :class="['run-detail-row', `status-${r.status}`]"
            >
              <ion-icon
                :icon="r.status === 'success' ? checkmarkCircle : r.status === 'failure' || r.status === 'timed_out' ? closeCircle : r.status === 'skipped' ? removeCircle : ellipseOutline"
                slot="start"
                :color="r.status === 'success' ? 'success' : (r.status === 'failure' || r.status === 'timed_out') ? 'danger' : 'medium'"
              ></ion-icon>
              <ion-label>
                <h3>{{ r.name }}</h3>
                <p>
                  <ion-badge size="small" :color="r.status === 'success' ? 'success' : (r.status === 'failure' || r.status === 'timed_out') ? 'danger' : 'medium'">
                    {{ r.status }}
                  </ion-badge>
                  <ion-badge v-if="r.httpStatus" size="small" color="medium">{{ r.httpStatus }}</ion-badge>
                  <ion-badge v-if="r.durationMs" size="small" color="medium">{{ r.durationMs }}ms</ion-badge>
                </p>
                <p v-if="r.error" class="run-detail-error">{{ r.error }}</p>
              </ion-label>
            </ion-item>
          </ion-list>
        </ion-content>
      </ion-modal>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { fetchWebDavLocalInfo, type WebDavLocalInfo } from "@encv/shared-components/api/encv";
import { useI18n } from "@encv/shared-components/composables/useI18n";
import { showToast } from "@encv/shared-components/composables/useToast";
import { useWebDavManifest } from "@encv/shared-components/composables/useWebDavManifest";
import { useWebDavAutomationTests } from "@encv/shared-components/composables/useWebDavWorkflowAdapter";
import type { TestCaseStatus, TestRun } from "@encv/shared-components/types/webdav-test";
import { alertController } from "@ionic/vue";
import {
  cubeOutline,
  flashOutline,
  gitNetworkOutline,
  listOutline,
  lockClosedOutline,
  shieldCheckmarkOutline,
  warningOutline,
} from "ionicons/icons";
import { computed, onMounted, ref, watch } from "vue";

const { t } = useI18n();
const automation = useWebDavAutomationTests();
const { modules, moduleStates, historyRuns, isAnyRunning, runModule, runAll, cancelModule, clearHistory } = automation;

// 🆕 2026-06-17: 8 module grid 使用 manifest composable（独立拉取 + mount 选择）
const manifest = useWebDavManifest();

const totalCases = computed(() => modules.reduce((sum, m) => sum + m.cases.length, 0));

// ============= manifest tone =============
const _manifestTone = computed(() => {
  if (manifest.error.value) return "tone-error";
  if (manifest.loading.value) return "tone-loading";
  if (availableMounts.value.length === 0) return "tone-empty";
  return "tone-ready";
});

const availableMounts = computed(() => manifest.availableMounts.value);
const baseUrl = computed(() => manifest.serverBaseUrl.value || (typeof window !== "undefined" ? window.location.origin : ""));
const _webdavPath = computed(() => manifest.webdavPath.value);

const _attackCountTotal = computed(() => modules.reduce((sum, m) => sum + m.cases.filter(c => !!c.attackType).length, 0));

function _getAttackCount(moduleId: string): number {
  const m = modules.find(mm => mm.id === moduleId);
  if (!m) return 0;
  return m.cases.filter(c => !!c.attackType).length;
}

// ============= module helpers =============
function _isModuleRunning(moduleId: string): boolean {
  const s = moduleStates[moduleId]?.value;
  return s?.status === "running" || s?.status === "cancelling";
}
function _isModuleDone(moduleId: string): boolean {
  const s = moduleStates[moduleId]?.value;
  return s?.status === "done" || s?.status === "cancelled" || s?.status === "error";
}
function getModuleState(moduleId: string) {
  return moduleStates[moduleId]?.value ?? { status: "idle" as const, results: [] };
}
function _getModulePassed(moduleId: string): number {
  return getModuleState(moduleId).results.filter(r => r.status === "success").length;
}

function getCaseResult(moduleId: string, caseId: string) {
  return getModuleState(moduleId).results.find(r => r.id === caseId);
}
function getCaseStatus(moduleId: string, caseId: string): TestCaseStatus {
  return getCaseResult(moduleId, caseId)?.status ?? "pending";
}
function _getCaseStatusColor(moduleId: string, caseId: string): string {
  const st = getCaseStatus(moduleId, caseId);
  if (st === "success") return "success";
  if (st === "failure" || st === "timed_out") return "danger";
  return "medium";
}

const expandedModules = ref<Set<string>>(new Set());
function _toggleModule(id: string) {
  if (expandedModules.value.has(id)) expandedModules.value.delete(id);
  else expandedModules.value.add(id);
  // 触发响应式更新
  expandedModules.value = new Set(expandedModules.value);
}

// ============= icon resolver =============
const ICON_MAP: Record<string, string> = {
  "lock-closed-outline": lockClosedOutline,
  "list-outline": listOutline,
  "git-network-outline": gitNetworkOutline,
  "flash-outline": flashOutline,
  "cube-outline": cubeOutline,
  "warning-outline": warningOutline,
  "shield-checkmark-outline": shieldCheckmarkOutline,
};
function _resolveIcon(iconName: string): string {
  return ICON_MAP[iconName] ?? warningOutline;
}

// ============= 顶部 WebDAV 健康检查 =============
const webDavEnabled = ref<boolean | null>(null);
async function checkWebDavHealth() {
  try {
    const path = manifest.webdavPath.value || "/webdav/";
    const url = `${baseUrl.value}${path}`.replace(/\/+$/, "/");
    const res = await fetch(url, { method: "OPTIONS" });
    webDavEnabled.value = res.status < 500;
  } catch {
    webDavEnabled.value = false;
  }
}

// ============= 账号配置 =============
const _showAuthPanel = ref(false);
const credsUsername = ref("");
const credsPassword = ref("");
const backendUsername = ref("");
const authRequired = ref(false);
const localInfo = ref<WebDavLocalInfo | null>(null);

const CRED_STORAGE_KEY = "encv_webdav_creds_v1";

async function loadWebDavLocalInfo() {
  try {
    const info = await fetchWebDavLocalInfo();
    localInfo.value = info;
    backendUsername.value = info.username;
    authRequired.value = info.authRequired;
    const stored = localStorage.getItem(CRED_STORAGE_KEY);
    if (!stored && info.authRequired) {
      credsUsername.value = info.username;
      credsPassword.value = info.password;
    } else if (stored) {
      try {
        const parsed = JSON.parse(stored) as { username?: string; password?: string };
        if (parsed.username) credsUsername.value = parsed.username;
        if (parsed.password) credsPassword.value = parsed.password;
      } catch {
        // ignore
      }
    }
  } catch (e) {
    console.debug("[webdav] loadWebDavLocalInfo failed", e);
  }
}

const _maskedUsername = computed(() => {
  if (credsUsername.value) return credsUsername.value;
  if (backendUsername.value) return `${backendUsername.value} (${t("devtools.webdavAuth.fromBackend")})`;
  return t("devtools.webdavAuth.notSet");
});

function _saveCreds() {
  localStorage.setItem(CRED_STORAGE_KEY, JSON.stringify({ username: credsUsername.value, password: credsPassword.value }));
  showToast({ message: t("devtools.webdavAuth.saved"), color: "success" });
}

function _resetToBackend() {
  if (!localInfo.value) return;
  credsUsername.value = localInfo.value.username;
  credsPassword.value = localInfo.value.password;
  showToast({ message: t("devtools.webdavAuth.resetToBackendHint"), color: "medium" });
}

// ============= 批量运行 =============
async function _handleRunAllModules() {
  if (isAnyRunning.value) return;
  try {
    await runAll();
    showToast({
      message: `WebDAV 全部 module 完成: ${totalCases.value} cases`,
      duration: 3000,
      color: "success",
    });
  } catch (e) {
    showToast({
      message: `WebDAV 测试失败: ${e instanceof Error ? e.message : String(e)}`,
      duration: 3000,
      color: "danger",
    });
  }
}

function _handleCancelAll() {
  for (const m of modules) {
    cancelModule(m.id);
  }
  showToast({ message: "已请求取消所有 module", duration: 1500, color: "medium" });
}

async function _runSingleModule(moduleId: string) {
  if (isAnyRunning.value) return;
  try {
    await runModule(moduleId);
  } catch (e) {
    showToast({
      message: `${moduleId} 失败: ${e instanceof Error ? e.message : String(e)}`,
      color: "danger",
    });
  }
}

function _cancelSingleModule(moduleId: string) {
  cancelModule(moduleId);
}

// ============= manifest refresh =============
async function _refreshManifest() {
  await manifest.refresh();
  if (availableMounts.value.length === 0) {
    showToast({ message: t("devtools.webdav.manifestEmpty"), color: "warning", duration: 2000 });
  }
}

// ============= 历史 =============
const _showHistory = ref(false);
const detailRun = ref<TestRun | null>(null);

function _refreshHistory() {
  // 触发响应式更新：useWebDavWorkflowAdapter 内部 historyRuns 是 ref
  automation.historyRuns.value = [...automation.historyRuns.value];
}

async function _handleClearHistory() {
  const alert = await alertController.create({
    header: t("devtools.confirmClearHistory"),
    message: t("devtools.confirmClearHistoryMsg"),
    buttons: [
      { text: t("common.cancel"), role: "cancel" },
      {
        text: t("common.confirm"),
        role: "confirm",
        handler: () => {
          clearHistory();
          showToast({ message: t("devtools.historyCleared"), duration: 1500, color: "success" });
        },
      },
    ],
  });
  await alert.present();
}

function _openRunDetail(run: TestRun) {
  detailRun.value = run;
}

function _formatTime(iso: string): string {
  if (!iso) return "-";
  const d = new Date(iso);
  return d.toLocaleString("zh-CN", { hour12: false });
}
function _formatDuration(start?: string, end?: string): string {
  if (!start || !end) return "-";
  const ms = new Date(end).getTime() - new Date(start).getTime();
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(2)}s`;
}

// ============= lifecycle =============
onMounted(async () => {
  await loadWebDavLocalInfo();
  await manifest.refresh();
  // 同步 webdavPath → 顶部 banner
  if (localInfo.value && !manifest.webdavPath.value) {
    manifest.webdavPath.value = localInfo.value.webdavPath;
  }
  if (localInfo.value && !manifest.auth.value.username) {
    manifest.auth.value = {
      username: localInfo.value.username,
      password: localInfo.value.password,
    };
  }
  checkWebDavHealth();
});

// 监听 manifest 变化自动重检健康
watch(
  () => manifest.webdavPath.value,
  () => {
    webDavEnabled.value = null;
    checkWebDavHealth();
  }
);
</script>

<style scoped>
.section-hint {
  font-size: 12px;
  color: var(--encv-text-secondary, #999);
  margin: 0 16px 12px;
  line-height: 1.5;
}

/* ============ WebDAV 状态 banner（沿用） ============ */
.webdav-status-banner {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 10px 12px 4px;
  padding: 10px 14px;
  border-radius: 8px;
  font-size: 12px;
  line-height: 1.4;
}
.webdav-status-banner .banner-icon {
  font-size: 24px;
  flex-shrink: 0;
}
.webdav-status-banner .banner-text {
  flex: 1;
  min-width: 0;
}
.webdav-status-banner strong {
  display: block;
  font-size: 13px;
  font-weight: 700;
  margin-bottom: 2px;
  letter-spacing: -0.01em;
}
.webdav-status-banner p {
  margin: 0;
  font-size: 11px;
  color: inherit;
  opacity: 0.9;
}
.webdav-status-banner code {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 10px;
  background: rgba(0, 0, 0, 0.08);
  padding: 1px 4px;
  border-radius: 3px;
}
.webdav-status-disabled {
  background: linear-gradient(135deg, rgba(255, 87, 34, 0.12), rgba(244, 67, 54, 0.08));
  border: 1px solid rgba(244, 67, 54, 0.25);
  color: #c62828;
}
.webdav-status-disabled .banner-icon { color: var(--ion-color-danger); }
.webdav-status-enabled {
  background: linear-gradient(135deg, rgba(76, 175, 80, 0.1), rgba(54, 175, 110, 0.06));
  border: 1px solid rgba(76, 175, 80, 0.22);
  color: #2e7d32;
}
.webdav-status-enabled .banner-icon { color: var(--ion-color-success); }

/* ============ Manifest 状态卡（顶部） ============ */
.manifest-card {
  margin: 12px 12px 8px;
  padding: 14px 16px;
  border-radius: 10px;
  border: 1px solid var(--ion-color-light-shade, #e0e0e0);
  background: var(--ion-color-light, #f7f7f7);
  transition: all 0.2s ease;
}
.manifest-card.tone-ready {
  background: linear-gradient(135deg, rgba(76, 175, 80, 0.06), rgba(54, 175, 110, 0.04));
  border-color: rgba(76, 175, 80, 0.22);
}
.manifest-card.tone-error {
  background: linear-gradient(135deg, rgba(244, 67, 54, 0.08), rgba(229, 57, 53, 0.05));
  border-color: rgba(244, 67, 54, 0.25);
}
.manifest-card.tone-loading {
  background: linear-gradient(135deg, rgba(79, 140, 255, 0.06), rgba(54, 175, 110, 0.04));
  border-color: rgba(79, 140, 255, 0.2);
}
.manifest-card.tone-empty {
  background: linear-gradient(135deg, rgba(255, 152, 0, 0.08), rgba(255, 193, 7, 0.04));
  border-color: rgba(255, 152, 0, 0.22);
}
.manifest-card-header {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}
.manifest-icon {
  font-size: 24px;
  color: var(--ion-color-primary);
  flex-shrink: 0;
  margin-top: 1px;
}
.manifest-meta {
  flex: 1;
  min-width: 0;
}
.manifest-meta strong {
  display: block;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--ion-color-dark);
  margin-bottom: 2px;
}
.manifest-meta p {
  margin: 0;
  font-size: 11px;
  line-height: 1.5;
  color: var(--encv-text-secondary);
}
.manifest-ready {
  display: flex !important;
  flex-wrap: wrap;
  gap: 4px 8px;
  align-items: center;
}
.ready-pill {
  display: inline-block;
  padding: 1px 8px;
  border-radius: 10px;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.04em;
  background: rgba(76, 175, 80, 0.18);
  color: #2e7d32;
}
.mount-count {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  font-weight: 600;
  color: var(--ion-color-dark);
}
.attack-tagged {
  font-size: 10px;
  color: var(--ion-color-danger);
  font-weight: 600;
}

/* ========== 🆕 2026-06-17 已注册容器扩展名（manifest activeMount 权威显示） ========== */
.manifest-container-exts {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  padding-top: 6px;
  border-top: 1px dashed rgba(0, 0, 0, 0.06);
}
.exts-label {
  font-size: 11px;
  color: var(--ion-color-medium-shade);
  font-weight: 500;
}
.ext-pill {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  font-weight: 600;
  background: var(--ion-color-primary-tint, #d6e4ff);
  color: var(--ion-color-primary-shade, #1a3a8e);
  padding: 2px 8px;
  border-radius: 10px;
  border: 1px solid rgba(79, 140, 255, 0.2);
}
.manifest-error { color: var(--ion-color-danger) !important; }
.manifest-empty-hint {
  color: var(--encv-text-secondary);
  opacity: 0.75;
}

/* ============ Mount 选择器（多 mount） ============ */
.mount-selector {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px dashed rgba(0, 0, 0, 0.08);
}
.mount-selector-label {
  display: block;
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--encv-text-secondary);
  margin-bottom: 8px;
}
.mount-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.mount-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border-radius: 16px;
  background: var(--ion-color-light, #fff);
  border: 1px solid var(--ion-color-light-shade, #e0e0e0);
  font-size: 11px;
  font-weight: 600;
  color: var(--ion-color-medium);
  cursor: pointer;
  transition: all 0.15s ease;
}
.mount-chip:hover { border-color: var(--ion-color-primary); }
.mount-chip.active {
  background: linear-gradient(135deg, var(--ion-color-primary), var(--ion-color-primary-shade, #2f7ce0));
  color: #fff;
  border-color: var(--ion-color-primary);
  box-shadow: 0 2px 6px rgba(79, 140, 255, 0.3);
}
.mount-chip.is-default .chip-icon { color: #f5a623; }
.mount-chip.active.is-default .chip-icon { color: #fff8e1; }
.chip-icon { font-size: 12px; }
.chip-path {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  opacity: 0.7;
  font-weight: 500;
}

/* ============ 批量控制条 ============ */
.bulk-controls {
  margin: 16px 12px 8px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.bulk-meta {
  font-size: 11px;
  font-weight: 500;
  opacity: 0.85;
  margin-left: 4px;
}

/* ============ Module 网格 ============ */
.module-grid-section {
  margin-top: 16px;
}
.module-grid-header {
  --background: transparent;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: -0.01em;
  text-transform: uppercase;
  color: var(--encv-text-secondary);
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.header-meta {
  font-size: 10px;
  font-weight: 600;
  color: var(--encv-text-secondary);
}
.module-grid {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 0 12px 24px;
}
.module-card {
  background: var(--ion-color-light, #f7f7f7);
  border: 1px solid var(--ion-color-light-shade, #e0e0e0);
  border-radius: 10px;
  overflow: hidden;
  transition: all 0.2s ease;
}
.module-card.is-running {
  border-color: var(--ion-color-warning);
  box-shadow: 0 0 0 1px rgba(255, 193, 7, 0.2);
}
.module-card.is-done {
  border-color: var(--ion-color-success);
}
.module-card-header {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  cursor: pointer;
  user-select: none;
  -webkit-tap-highlight-color: transparent;
}
.module-icon-bubble {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  color: #fff;
  flex-shrink: 0;
  box-shadow: 0 2px 4px rgba(0, 0, 0, 0.08);
}
.module-icon-bubble.tone-primary { background: linear-gradient(135deg, #5b9dff, #2f7ce0); }
.module-icon-bubble.tone-secondary { background: linear-gradient(135deg, #b388ff, #7c4dff); }
.module-icon-bubble.tone-tertiary { background: linear-gradient(135deg, #66bb6a, #388e3c); }
.module-icon-bubble.tone-warning { background: linear-gradient(135deg, #ffa726, #f57c00); }
.module-icon-bubble.tone-medium { background: linear-gradient(135deg, #90a4ae, #607d8b); }
.module-icon-bubble.tone-danger { background: linear-gradient(135deg, #ef5350, #c62828); }
.module-title-block {
  flex: 1;
  min-width: 0;
}
.module-title {
  font-size: 14px;
  font-weight: 700;
  margin: 0;
  letter-spacing: -0.01em;
  color: var(--ion-color-dark);
}
.module-desc {
  font-size: 11px;
  color: var(--encv-text-secondary);
  margin: 2px 0 0;
  line-height: 1.4;
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}
.module-stats {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
  flex-shrink: 0;
}
.module-stat {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  font-weight: 700;
}
.module-stat.idle {
  font-size: 10px;
  color: var(--encv-text-secondary);
}
.module-stat.progress .stat-passed { color: var(--ion-color-success); }
.module-stat.progress .stat-sep { color: var(--encv-text-secondary); margin: 0 2px; }
.module-stat.progress .stat-total { color: var(--ion-color-dark); }
.module-stat.progress.cancelling { opacity: 0.7; }
.module-attack-badge {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  font-size: 9px;
  font-weight: 700;
  padding: 1px 6px;
  border-radius: 8px;
  background: rgba(244, 67, 54, 0.12);
  color: #c62828;
  letter-spacing: 0.04em;
}
.module-attack-badge ion-icon { font-size: 9px; }
.module-chevron {
  font-size: 18px;
  color: var(--encv-text-secondary);
  flex-shrink: 0;
}

/* ============ Module 展开体 ============ */
.module-card-body {
  padding: 0 14px 14px;
  border-top: 1px dashed rgba(0, 0, 0, 0.08);
  background: rgba(0, 0, 0, 0.015);
}
.module-actions {
  display: flex;
  gap: 8px;
  margin: 10px 0;
}
.case-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.case-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 8px 10px;
  background: var(--ion-color-light, #fff);
  border: 1px solid var(--ion-color-light-shade, #e0e0e0);
  border-left: 3px solid transparent;
  border-radius: 6px;
  font-size: 12px;
  transition: all 0.15s ease;
}
.case-row.status-success { border-left-color: var(--ion-color-success); }
.case-row.status-failure,
.case-row.status-timed_out { border-left-color: var(--ion-color-danger); }
.case-row.status-skipped { border-left-color: var(--ion-color-medium); opacity: 0.7; }
.case-method {
  flex-shrink: 0;
  display: inline-block;
  padding: 2px 6px;
  border-radius: 3px;
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.04em;
  background: var(--ion-color-light-shade, #e0e0e0);
  color: var(--ion-color-medium);
  margin-top: 1px;
}
.case-method.method-get { background: rgba(76, 175, 80, 0.15); color: #2e7d32; }
.case-method.method-head { background: rgba(79, 140, 255, 0.15); color: #1565c0; }
.case-method.method-options { background: rgba(158, 158, 158, 0.15); color: #424242; }
.case-method.method-propfind { background: rgba(139, 92, 246, 0.15); color: #6d28d9; }
.case-method.method-delete { background: rgba(244, 67, 54, 0.15); color: #c62828; }
.case-method.method-mkcol { background: rgba(255, 152, 0, 0.15); color: #ef6c00; }
.case-body { flex: 1; min-width: 0; }
.case-name {
  font-size: 12px;
  font-weight: 600;
  color: var(--ion-color-dark);
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  margin-bottom: 2px;
}
.case-attack-tag {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 8px;
  font-size: 9px;
  font-weight: 700;
  background: rgba(244, 67, 54, 0.1);
  color: #c62828;
  letter-spacing: 0.02em;
}
.case-attack-tag.attack-protocol-consistency {
  background: rgba(158, 158, 158, 0.12);
  color: #616161;
}
.case-attack-tag.attack-concurrency-stress,
.case-attack-tag.attack-resource-exhaustion {
  background: rgba(255, 152, 0, 0.12);
  color: #ef6c00;
}
.case-attack-tag.attack-slow-network,
.case-attack-tag.attack-large-payload {
  background: rgba(139, 92, 246, 0.12);
  color: #6d28d9;
}
.case-attack-tag.attack-cross-mount-escape,
.case-attack-tag.attack-index-rebuild-race {
  background: rgba(244, 67, 54, 0.12);
  color: #c62828;
}
.case-desc {
  font-size: 10px;
  color: var(--encv-text-secondary);
  margin: 0;
  line-height: 1.4;
}
.case-error {
  display: flex;
  align-items: flex-start;
  gap: 4px;
  font-size: 10px;
  color: var(--ion-color-danger);
  margin: 4px 0 0;
  font-family: ui-monospace, monospace;
  background: rgba(244, 67, 54, 0.04);
  padding: 4px 6px;
  border-radius: 3px;
  word-break: break-all;
}
.case-error ion-icon { font-size: 12px; flex-shrink: 0; margin-top: 1px; }
.case-meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 10px;
  color: var(--encv-text-secondary);
  margin: 4px 0 0;
  font-family: ui-monospace, monospace;
}
.case-duration { font-weight: 600; }
.case-status-icon {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
}
.status-passed { color: var(--ion-color-success); }
.status-failed { color: var(--ion-color-danger); }
.status-skipped { color: var(--ion-color-medium); }
.status-pending { color: var(--ion-color-medium); opacity: 0.4; }

/* ============ Auth panel（沿用） ============ */
.auth-panel-body { background: var(--ion-color-light); }
.auth-status-required {
  color: var(--ion-color-danger);
  font-weight: 600;
}
.auth-status-optional {
  color: var(--ion-color-success);
  font-weight: 600;
}
.creds-summary {
  font-family: ui-monospace, monospace;
  color: var(--encv-text-secondary);
}

/* ============ 历史（沿用） ============ */
.history-header {
  display: flex;
  justify-content: space-between;
  padding: 8px 12px;
  border-bottom: 1px solid var(--ion-color-light-shade, #e0e0e0);
}
.empty-history {
  text-align: center;
  padding: 60px 24px;
  color: var(--encv-text-secondary);
}
.empty-icon {
  font-size: 56px;
  opacity: 0.3;
  margin-bottom: 12px;
}
.empty-history h3 {
  margin: 0 0 8px;
  font-size: 16px;
  color: var(--ion-color-medium);
}
.empty-history p {
  margin: 0;
  font-size: 12px;
}
.history-base-url {
  font-size: 10px !important;
  font-family: ui-monospace, monospace;
  color: var(--encv-text-secondary);
  word-break: break-all;
}

/* ============ Run 详情（沿用） ============ */
.run-detail-summary {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 8px;
  padding: 16px 12px;
  border-bottom: 1px solid var(--ion-color-light-shade, #e0e0e0);
}
.summary-stat {
  text-align: center;
  padding: 12px 8px;
  border-radius: 8px;
  background: var(--ion-color-light);
}
.summary-stat.ok { background: rgba(54, 175, 110, 0.12); }
.summary-stat.fail { background: rgba(255, 0, 0, 0.08); }
.summary-stat.skip { background: rgba(158, 158, 158, 0.08); }
.summary-stat.total { background: rgba(79, 140, 255, 0.08); }
.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--ion-color-dark);
  font-family: ui-monospace, monospace;
  line-height: 1;
}
.stat-label {
  font-size: 10px;
  color: var(--encv-text-secondary);
  margin-top: 4px;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}
.run-detail-meta {
  padding: 12px 16px;
  font-size: 11px;
  color: var(--encv-text-secondary);
  font-family: ui-monospace, monospace;
  background: var(--ion-color-light);
  border-bottom: 1px solid var(--ion-color-light-shade, #e0e0e0);
}
.run-detail-meta code { color: var(--ion-color-primary); }
ion-item.run-detail-row {
  --padding-start: 12px;
  --inner-padding-end: 0;
}
.run-detail-error {
  font-family: ui-monospace, monospace;
  color: var(--ion-color-danger);
  font-size: 11px;
  background: rgba(255, 0, 0, 0.04);
  padding: 4px 6px;
  border-radius: 4px;
  border-left: 2px solid var(--ion-color-danger);
  word-break: break-all;
}

.spin { animation: spin 1s linear infinite; }
@keyframes spin { to { transform: rotate(360deg); } }
</style>
