<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>{{ t('remote.title') }}</ion-title>
      </ion-toolbar>
      <ion-toolbar>
        <ion-segment v-model="activeTab" @ionChange="onTabChange">
          <ion-segment-button value="webdav">
            <ion-label>WebDAV</ion-label>
          </ion-segment-button>
          <ion-segment-button value="openlist">
            <ion-label>Openlist</ion-label>
          </ion-segment-button>
        </ion-segment>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div v-if="activeTab === 'webdav'">
        <div v-if="webdavConfigs.length === 0 && !builtInWebdav" class="empty-state">
          <ion-icon :icon="cloud" class="empty-icon"></ion-icon>
          <h3>{{ t('webdav.noServers') }}</h3>
          <p>{{ t('webdav.noServersDesc') }}</p>
        </div>

        <ion-list v-else>
          <ion-item v-if="builtInWebdav" class="built-in-item">
            <ion-icon :icon="home" color="primary" slot="start"></ion-icon>
            <ion-label>
              <h2>{{ t('remote.builtInWebdav') }}</h2>
              <p>{{ builtInWebdav.url }}</p>
              <p v-if="builtInWebdav.username">{{ t('webdav.username') }}: {{ builtInWebdav.username }}</p>
            </ion-label>
            <ion-badge :color="builtInWebdav.enabled ? 'success' : 'medium'" slot="end">
              {{ builtInWebdav.enabled ? t('remote.enabled') : t('remote.disabled') }}
            </ion-badge>
          </ion-item>

          <ion-item-sliding v-for="config in webdavConfigs" :key="config.id">
            <ion-item @click="editConfig(config)">
              <ion-icon :icon="cloud" color="primary" slot="start"></ion-icon>
              <ion-label>
                <h2>{{ config.name }}</h2>
                <p>{{ config.url }}</p>
                <p v-if="config.mountPath">{{ t('webdav.mount') }}: {{ config.mountPath }}</p>
              </ion-label>
              <ion-badge :color="config.id === testingId ? 'warning' : 'medium'" slot="end">
                {{ config.id === testingId ? t('webdav.testing') : t('webdav.saved') }}
              </ion-badge>
            </ion-item>

            <div v-if="listTestResults[config.id]" class="list-test-result-area" :class="{ 'result-error': !listTestResults[config.id].success, 'result-ok': listTestResults[config.id].success }">
              <div class="result-items">
                <div class="result-item">
                  <span class="result-label">{{ listTestResults[config.id].reachable ? t('webdav.reachable') : t('webdav.notReachable') }}</span>
                  <ion-badge :color="listTestResults[config.id].reachable ? 'success' : 'danger'" class="mini-badge">
                    {{ listTestResults[config.id].reachable ? 'OK' : 'FAIL' }}
                  </ion-badge>
                </div>
                <div class="result-item">
                  <span class="result-label">{{ listTestResults[config.id].is_webdav ? t('webdav.isWebDAV') : t('webdav.notWebDAV') }}</span>
                  <ion-badge :color="listTestResults[config.id].is_webdav ? 'success' : 'danger'" class="mini-badge">
                    {{ listTestResults[config.id].is_webdav ? 'OK' : 'FAIL' }}
                  </ion-badge>
                </div>
                <div v-if="listTestResults[config.id].error" class="result-error-inline">
                  {{ listTestResults[config.id].error }}
                </div>
              </div>
            </div>
            <ion-item-options side="end">
              <ion-item-option color="primary" @click="testConfig(config)">
                {{ t('webdav.test') }}
              </ion-item-option>
              <ion-item-option color="danger" @click="deleteConfig(config.id)">
                {{ t('webdav.delete') }}
              </ion-item-option>
            </ion-item-options>
          </ion-item-sliding>
        </ion-list>

        <ion-fab vertical="bottom" horizontal="end" slot="fixed">
          <ion-fab-button @click="openNewConfig">
            <ion-icon :icon="add"></ion-icon>
          </ion-fab-button>
        </ion-fab>
      </div>

      <div v-if="activeTab === 'openlist'">
        <LocalOpenListStatusCard />

        <div v-if="openlistSiteKeys.length === 0" class="empty-state">
          <ion-icon :icon="globe" class="empty-icon"></ion-icon>
          <h3>{{ t('remote.noOpenlistSites') }}</h3>
          <p>{{ t('remote.noOpenlistSitesDesc') }}</p>
        </div>

        <ion-list v-else>
          <ion-item-sliding v-for="key in openlistSiteKeys" :key="key">
            <ion-item @click="editSite(key)" :class="{ 'site-disabled': isSiteDisabled(key) }">
              <ion-icon :icon="globe" color="primary" slot="start"></ion-icon>
              <ion-label>
                <h2 class="site-name-row">
                  <ion-chip v-if="isLocalLoopback(key)" color="primary" class="local-chip">{{ t('remote.localBadge') }}</ion-chip>
                  <span>{{ key }}</span>
                </h2>
                <p v-if="openlistSites[key].description">{{ openlistSites[key].description }}</p>
                <p>{{ t('remote.host') }}: {{ openlistSites[key].host }}</p>
                <p class="proxy-url">{{ t('remote.proxyUrl') }}: {{ openlistSites[key].proxyUrl }}</p>
                <ion-note v-if="!isSiteBuiltIn(key)" class="site-toggle-label-row">
                  {{ isSiteDisabled(key) ? t('remote.siteDisabled') : t('remote.enabled') }}
                </ion-note>
              </ion-label>
              <ion-toggle
                v-if="!isSiteBuiltIn(key)"
                slot="end"
                :checked="!isSiteDisabled(key)"
                @ionChange="onSiteToggleChange(key, $event)"
                @click.stop
              ></ion-toggle>
            </ion-item>
            <ion-item-options side="end">
              <ion-item-option color="primary" @click.stop="copyProxyUrl(openlistSites[key].proxyUrl)">
                {{ t('remote.copied') }}
              </ion-item-option>
              <ion-item-option v-if="!isSiteBuiltIn(key)" color="danger" @click.stop="handleDeleteSite(key)">
                {{ t('webdav.delete') }}
              </ion-item-option>
            </ion-item-options>
          </ion-item-sliding>
        </ion-list>

        <ion-fab vertical="bottom" horizontal="end" slot="fixed">
          <ion-fab-button @click="openNewSite">
            <ion-icon :icon="add"></ion-icon>
          </ion-fab-button>
        </ion-fab>
      </div>

      <ion-modal :is-open="showWebdavModal" @didDismiss="showWebdavModal = false">
        <ion-header>
          <ion-toolbar>
            <ion-title>{{ editingId ? t('webdav.edit') : t('webdav.add') }} WebDAV</ion-title>
            <ion-buttons slot="end">
              <ion-button @click="showWebdavModal = false">{{ t('settings.cancel') }}</ion-button>
            </ion-buttons>
          </ion-toolbar>
        </ion-header>
        <ion-content class="ion-padding">
          <ion-list>
            <ion-item>
              <InputWithHistory
                v-model="formName"
                :label="t('webdav.name')"
                placeholder="My WebDAV Server"
                :icon="cloud"
                history-key="webdav.name"
              />
            </ion-item>
            <ion-item>
              <InputWithHistory
                v-model="formUrl"
                :label="t('webdav.serverUrl')"
                placeholder="https://dav.example.com"
                :icon="globe"
                history-key="webdav.url"
              />
            </ion-item>
            <ion-item>
              <InputWithHistory
                v-model="formUsername"
                :label="t('webdav.username')"
                placeholder="user"
                :icon="person"
                history-key="webdav.username"
              />
            </ion-item>
            <ion-item>
              <InputWithHistory
                v-model="formPassword"
                :label="t('webdav.password')"
                placeholder="password"
                :icon="lockClosed"
                input-type="password"
                history-key="webdav.password"
              />
            </ion-item>
            <ion-item>
              <InputWithHistory
                v-model="formMountPath"
                :label="t('webdav.mountPath')"
                placeholder="/webdav"
                :icon="folderOpen"
                history-key="webdav.mountPath"
              />
            </ion-item>
          </ion-list>
          <ion-button expand="block" @click="testConnection" :disabled="testing || !formUrl">
            <ion-icon :icon="flash" slot="start"></ion-icon>
            {{ testing ? t('webdav.testing') : t('webdav.testConnection') }}
          </ion-button>

          <div v-if="testResult" class="test-result-area" :class="{ 'result-error': !testResult.success, 'result-ok': testResult.success }">
            <h4 class="result-title">{{ t('webdav.testResult') }}</h4>

            <div class="result-items">
              <div class="result-item">
                <span class="result-label">{{ testResult.reachable ? t('webdav.reachable') : t('webdav.notReachable') }}</span>
                <ion-badge :color="testResult.reachable ? 'success' : 'danger'">
                  {{ testResult.reachable ? 'OK' : 'FAIL' }}
                </ion-badge>
              </div>

              <div class="result-item">
                <span class="result-label">{{ testResult.is_webdav ? t('webdav.isWebDAV') : t('webdav.notWebDAV') }}</span>
                <ion-badge :color="testResult.is_webdav ? 'success' : 'danger'">
                  {{ testResult.is_webdav ? 'OK' : 'FAIL' }}
                </ion-badge>
              </div>

              <div v-if="testResult.is_webdav" class="result-item">
                <span class="result-label">{{ testResult.auth_ok ? t('webdav.authOK') : t('webdav.authFailed') }}</span>
                <ion-badge :color="testResult.auth_ok ? 'success' : 'danger'">
                  {{ testResult.auth_ok ? 'OK' : 'FAIL' }}
                </ion-badge>
              </div>

              <div v-if="testResult.is_webdav && testResult.status_code === 207" class="result-item">
                <span class="result-label">{{ testResult.dir_readable ? t('webdav.dirReadable') : t('webdav.dirNotReadable') }}</span>
                <ion-badge :color="testResult.dir_readable ? 'success' : 'danger'">
                  {{ testResult.dir_readable ? 'OK' : 'FAIL' }}
                </ion-badge>
              </div>

              <div class="result-item">
                <span class="result-label">{{ t('webdav.statusCode') }}</span>
                <span class="result-value">HTTP {{ testResult.status_code }}</span>
              </div>

              <div v-if="testResult.dav_header" class="result-item">
                <span class="result-label">{{ t('webdav.davHeader') }}</span>
                <span class="result-value code-text">{{ testResult.dav_header }}</span>
              </div>
            </div>

            <div v-if="testResult.error" class="result-error-msg">
              <p>{{ t('webdav.testDetail') }}:</p>
              <p class="error-detail">{{ testResult.error }}</p>
            </div>
          </div>

          <ion-button expand="block" class="ion-margin-top" @click="saveConfig" :disabled="!formName || !formUrl">
            <ion-icon :icon="saveIcon" slot="start"></ion-icon>
            {{ t('webdav.save') }}
          </ion-button>
        </ion-content>
      </ion-modal>

      <ion-modal :is-open="showSiteModal" @didDismiss="showSiteModal = false">
        <ion-header>
          <ion-toolbar>
            <ion-title>{{ editingSiteId ? t('remote.editSite') : t('remote.addSite') }}</ion-title>
            <ion-buttons slot="end">
              <ion-button @click="showSiteModal = false">{{ t('settings.cancel') }}</ion-button>
            </ion-buttons>
          </ion-toolbar>
        </ion-header>
        <ion-content class="ion-padding">
          <ion-list>
            <ion-item>
              <InputWithHistory
                v-model="formSiteId"
                :label="t('remote.siteId')"
                :placeholder="t('remote.siteIdPlaceholder')"
                :disabled="!!editingSiteId"
                :error-text="formSiteIdError"
                :icon="fingerPrint"
                history-key="openlist.siteId"
                @update:model-value="validateSiteId"
              />
            </ion-item>
            <ion-item>
              <InputWithHistory
                v-model="formHost"
                :label="t('remote.host')"
                :placeholder="t('remote.hostPlaceholder')"
                :error-text="formHostError"
                :icon="globe"
                history-key="openlist.host"
                @update:model-value="validateHost"
              />
            </ion-item>
            <ion-item>
              <InputWithHistory
                v-model="formDescription"
                :label="t('remote.description')"
                :placeholder="t('remote.descriptionPlaceholder')"
                :icon="documentText"
                history-key="openlist.description"
              />
            </ion-item>
          </ion-list>
          <ion-button expand="block" @click="saveSite" :disabled="!formSiteId || !formHost || !!formSiteIdError || !!formHostError">
            {{ t('webdav.save') }}
          </ion-button>
        </ion-content>
      </ion-modal>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import {
  cloud,
  documentText,
  flash,
  folderOpen,
  globe,
  home,
  lockClosed,
  person,
} from "ionicons/icons";

import type { OpenlistSiteInfo, RemoteWebDAVInfo, WebDAVConfig, WebDAVTestResult } from "@/api/encv";
import {
  addOpenlistSite,
  deleteOpenlistSite,
  fetchRemoteInfo,
  getWebDAVConfigs,
  saveWebDAVConfigs,
  testWebDAVConnection,
  updateOpenlistSite,
} from "@/api/encv";
import { copyToClipboard as clipboardWrite } from "@/composables/useClipboard";
import { useI18n } from "@/composables/useI18n";
import { showToast } from "@/composables/useToast";
import { computed, onMounted, ref } from "vue";

const { t } = useI18n();

const activeTab = ref<"webdav" | "openlist">("webdav");
const webdavConfigs = ref<WebDAVConfig[]>([]);
const builtInWebdav = ref<RemoteWebDAVInfo | null>(null);
const openlistSites = ref<Record<string, OpenlistSiteInfo>>({});
const disabledSites = ref<Set<string>>(new Set());
const _openlistSiteKeys = computed(() => {
  const all = Object.keys(openlistSites.value);
  return [...all.filter(k => k === "local-loopback"), ...all.filter(k => k !== "local-loopback")];
});

function isLocalLoopback(key: string): boolean {
  return key === "local-loopback";
}

function isSiteBuiltIn(key: string): boolean {
  return !!openlistSites.value[key]?.isBuiltIn || isLocalLoopback(key);
}

function isSiteDisabled(key: string): boolean {
  return disabledSites.value.has(key);
}

function onSiteToggleChange(key: string, event: CustomEvent) {
  const checked = !!event.detail.checked;
  const next = new Set(disabledSites.value);
  if (checked) {
    next.delete(key);
  } else {
    next.add(key);
  }
  disabledSites.value = next;
}

const showWebdavModal = ref(false);
const editingId = ref("");
const testing = ref(false);
const testingId = ref("");
const testResult = ref<WebDAVTestResult | null>(null);
const listTestResults = ref<Record<string, WebDAVTestResult>>({});
const formName = ref("");
const formUrl = ref("");
const formUsername = ref("");
const formPassword = ref("");
const formMountPath = ref("");

const showSiteModal = ref(false);
const editingSiteId = ref("");
const formSiteId = ref("");
const formHost = ref("");
const formDescription = ref("");
const formSiteIdError = ref("");
const formHostError = ref("");

function onTabChange() {
  if (activeTab.value === "openlist") {
    loadRemoteInfo();
  }
}

async function loadRemoteInfo() {
  try {
    const info = await fetchRemoteInfo();
    if (info.webdav?.enabled) {
      builtInWebdav.value = info.webdav;
    } else {
      builtInWebdav.value = null;
    }
    openlistSites.value = info.openlistSites || {};
  } catch {
    // silent
  }
}

function loadConfigs() {
  webdavConfigs.value = getWebDAVConfigs();
}

function openNewConfig() {
  editingId.value = "";
  formName.value = "";
  formUrl.value = "";
  formUsername.value = "";
  formPassword.value = "";
  formMountPath.value = "/webdav";
  testResult.value = null;
  showWebdavModal.value = true;
}

function editConfig(config: WebDAVConfig) {
  editingId.value = config.id;
  formName.value = config.name;
  formUrl.value = config.url;
  formUsername.value = config.username;
  formPassword.value = config.password;
  formMountPath.value = config.mountPath;
  testResult.value = null;
  showWebdavModal.value = true;
}

function saveConfig() {
  if (!formName.value || !formUrl.value) return;
  let updated: WebDAVConfig[];
  if (editingId.value) {
    updated = webdavConfigs.value.map(c =>
      c.id === editingId.value
        ? {
            ...c,
            name: formName.value,
            url: formUrl.value,
            username: formUsername.value,
            password: formPassword.value,
            mountPath: formMountPath.value,
          }
        : c
    );
  } else {
    const newConfig: WebDAVConfig = {
      id: Date.now().toString(),
      name: formName.value,
      url: formUrl.value,
      username: formUsername.value,
      password: formPassword.value,
      mountPath: formMountPath.value,
    };
    updated = [...webdavConfigs.value, newConfig];
  }
  saveWebDAVConfigs(updated);
  webdavConfigs.value = updated;
  showWebdavModal.value = false;
  showToast({ message: t("webdav.configSaved"), duration: 1500, color: "success" });
}

async function testConfig(config: WebDAVConfig) {
  testingId.value = config.id;
  listTestResults.value[config.id] = {
    success: false,
    reachable: false,
    is_webdav: false,
    auth_ok: false,
    dir_readable: false,
    status_code: 0,
  };
  try {
    const result = await testWebDAVConnection({
      name: config.name,
      url: config.url,
      username: config.username,
      password: config.password,
      mountPath: config.mountPath,
    });
    testingId.value = "";
    listTestResults.value[config.id] = result;
  } catch (e) {
    testingId.value = "";
    listTestResults.value[config.id] = {
      success: false,
      reachable: false,
      is_webdav: false,
      auth_ok: false,
      dir_readable: false,
      status_code: 0,
      error: e instanceof Error ? e.message : String(e),
    };
  }
}

async function testConnection() {
  if (!formUrl.value) return;
  testing.value = true;
  testResult.value = null;
  try {
    const result = await testWebDAVConnection({
      name: formName.value,
      url: formUrl.value,
      username: formUsername.value,
      password: formPassword.value,
      mountPath: formMountPath.value,
    });
    testing.value = false;
    testResult.value = result;
  } catch (e) {
    testing.value = false;
    testResult.value = {
      success: false,
      reachable: false,
      is_webdav: false,
      auth_ok: false,
      dir_readable: false,
      status_code: 0,
      error: e instanceof Error ? e.message : String(e),
    };
  }
}

function deleteConfig(id: string) {
  const updated = webdavConfigs.value.filter(c => c.id !== id);
  saveWebDAVConfigs(updated);
  webdavConfigs.value = updated;
}

function validateSiteId() {
  const val = formSiteId.value.trim();
  if (!val) {
    formSiteIdError.value = t("tasks.pathRequired");
  } else if (!/^[a-zA-Z0-9_]+$/.test(val)) {
    formSiteIdError.value = t("remote.siteIdInvalid");
  } else {
    formSiteIdError.value = "";
  }
}

function validateHost() {
  const val = formHost.value.trim();
  if (!val) {
    formHostError.value = t("tasks.pathRequired");
  } else {
    formHostError.value = "";
  }
}

function openNewSite() {
  editingSiteId.value = "";
  formSiteId.value = "";
  formHost.value = "";
  formDescription.value = "";
  formSiteIdError.value = "";
  formHostError.value = "";
  showSiteModal.value = true;
}

function editSite(key: string) {
  editingSiteId.value = key;
  formSiteId.value = key;
  formHost.value = openlistSites.value[key]?.host || "";
  formDescription.value = openlistSites.value[key]?.description || "";
  formSiteIdError.value = "";
  formHostError.value = "";
  showSiteModal.value = true;
}

async function saveSite() {
  if (!formSiteId.value || !formHost.value) return;
  try {
    if (editingSiteId.value) {
      await updateOpenlistSite(editingSiteId.value, formHost.value.trim(), formDescription.value.trim());
    } else {
      await addOpenlistSite(formSiteId.value.trim(), formHost.value.trim(), formDescription.value.trim());
    }
    showSiteModal.value = false;
    showToast({ message: t("webdav.configSaved"), duration: 1500, color: "success" });
    await loadRemoteInfo();
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    showToast({ message: detail, duration: 3000, color: "danger" });
  }
}

async function handleDeleteSite(key: string) {
  try {
    await deleteOpenlistSite(key);
    showToast({ message: t("webdav.configSaved"), duration: 1500, color: "success" });
    await loadRemoteInfo();
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    showToast({ message: detail, duration: 3000, color: "danger" });
  }
}

async function copyProxyUrl(url: string) {
  const ok = await clipboardWrite(url);
  if (ok) {
    showToast({ message: t("remote.copied"), duration: 1500, color: "success" });
  } else {
    showToast({ message: url, duration: 3000, color: "medium" });
  }
}

onMounted(() => {
  loadConfigs();
  loadRemoteInfo();
});
</script>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 50%;
  padding: 24px;
  text-align: center;
  color: var(--encv-text-secondary);
}

.empty-icon {
  font-size: 64px;
  margin-bottom: 16px;
  opacity: 0.5;
}

.built-in-item {
  --background: rgba(var(--ion-color-primary-rgb), 0.05);
}

.proxy-url {
  font-size: 12px;
  color: var(--ion-color-primary);
}

.test-result-area {
  margin-top: 12px;
  padding: 14px;
  border-radius: 8px;
  background: var(--ion-background-color);
}

.result-ok {
  border-left: 3px solid var(--ion-color-success);
}

.result-error {
  border-left: 3px solid var(--ion-color-danger);
}

.result-title {
  margin: 0 0 10px;
  font-size: 15px;
  font-weight: 600;
  color: var(--ion-text-color);
}

.result-items {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.result-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 13px;
}

.result-label {
  color: var(--ion-text-color);
  font-weight: 500;
}

.result-value {
  color: var(--ion-text-secondary);
  font-size: 13px;
  font-weight: 400;
}

.code-text {
  font-family: monospace;
  font-size: 12px;
}

.result-error-msg {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px solid rgba(var(--ion-color-danger-rgb), 0.15);
}

.result-error-msg p:first-child {
  margin: 0 0 4px;
  font-size: 13px;
  font-weight: 600;
  color: var(--ion-color-danger);
}

.error-detail {
  margin: 0;
  font-size: 13px;
  color: var(--ion-color-medium);
  line-height: 1.5;
  word-break: break-word;
}

.list-test-result-area {
  padding: 10px 14px;
  background: var(--ion-background-color);
}

.list-test-result-area.result-ok {
  border-left: 3px solid var(--ion-color-success);
}

.list-test-result-area.result-error {
  border-left: 3px solid var(--ion-color-danger);
}

.mini-badge {
  font-size: 11px;
  --padding-start: 6px;
  --padding-end: 6px;
}

.result-error-inline {
  color: var(--ion-color-danger);
  font-size: 12px;
  margin-top: 4px;
  word-break: break-word;
}

.site-name-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}

.local-chip {
  font-size: 10px;
  font-weight: 600;
  height: 22px;
  margin: 0;
  padding: 0 8px;
}

.site-toggle-label-row {
  display: block;
  font-size: 11px;
  color: var(--ion-color-medium);
  margin-top: 4px;
}

.site-disabled {
  opacity: 0.5;
}

.site-disabled .proxy-url {
  color: var(--ion-color-medium);
}
</style>
