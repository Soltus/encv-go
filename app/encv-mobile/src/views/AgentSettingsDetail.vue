<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.agentSettings') }}</ion-title>
        <ion-buttons slot="end" v-if="dirty">
          <ion-button @click="handleResetConfig" color="medium">{{ t('settings.undo') }}</ion-button>
          <ion-button @click="handleSaveConfig" :disabled="configLoading">
            <ion-icon :icon="saveIcon" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!-- ① 加载中（spinner）—— 仅在尚未加载完且正在拉取时显示 -->
      <div v-if="configLoading && !configLoaded && !configError" class="loading-container">
        <ion-spinner name="crescent"></ion-spinner>
        <p>{{ t('settings.loadingConfig') }}</p>
      </div>

      <!-- ② 错误态 —— 后端挂掉 / 配置拉取失败 / 离线时显示，
              给出明确原因 + 手动重试按钮，避免页面一片空白让用户无所适从 -->
      <div v-else-if="!serverOnline || configError" class="configErrorContainer">
        <!-- 后端健康度摘要：让用户调 Agent 前先看后端是否在线
             此页面语义 = "Agent 行为配置"，不是"后端状态显示"
             卡片自带 version / instance_id / port / latency 等丰富信息，
             视觉效果 + 动态（pulse / 状态切换）远比纯文本好。
             保留下方"诊断文本" details 给高级用户 / bug report 用 -->
        <div class="configErrorCardWrap">
          <ServerStatusCard :clickable="false" />
        </div>
        <h2 class="configErrorTitle">
          {{ serverOnline
              ? (t('agent.configLoadFailed') || '加载 AI 配置失败')
              : (t('agent.backendOffline') || '后端服务未连接') }}
        </h2>
        <p class="configErrorDetail">
          {{ configError
              ? configError
              : (t('agent.backendOfflineHint') || '请确认 encv-go 服务已启动，或检查网络连接。') }}
        </p>
        <!-- 诊断信息：展开查看当前探测结果（forensic / bug report 仍有用） -->
        <details class="configErrorDiag">
          <summary>诊断信息（技术细节）</summary>
          <pre class="configErrorDiagPre">{{ diagInfo }}</pre>
        </details>
        <button type="button" class="configErrorRetryBtn" :disabled="configLoading" @click="retryLoadConfig">
          <ion-icon :icon="refreshIcon"></ion-icon>
          <span>{{ t('common.retry') || '重试' }}</span>
          <span v-if="autoRetryCount > 0" class="retryCount">({{ autoRetryCount }})</span>
        </button>
        <button type="button" class="configErrorSecondaryBtn" @click="handleGoToDevLogs">
          <ion-icon :icon="bugIcon"></ion-icon>
          <span>{{ t('agent.apiKeyViewLogs') || '查看日志' }}</span>
        </button>
      </div>

      <!-- ③ 正常态：schema 驱动的字段列表 -->
      <template v-else-if="configLoaded && agentSection">
        <ion-list>
          <ion-list-header>
            <ion-label>{{ t('settings.agentSettings') }}</ion-label>
            <ion-badge slot="end" color="primary" class="scope-badge scope-synced">
              <ion-icon :icon="cloudOutline" class="scope-badge-icon"></ion-icon>
              <span class="scope-text">{{ t('settings.synced') }}</span>
            </ion-badge>
          </ion-list-header>

          <template v-for="child in agentSection.properties" :key="child.key">
            <template v-if="child.key === 'enabled_tools'">
              <ion-item lines="none" class="config-field">
                <ion-icon :icon="listIcon" slot="start"></ion-icon>
                <ion-input
                  :value="toolsText"
                  :label="fieldLabel(child.key)"
                  label-placement="stacked"
                  :placeholder="'list_files, read_file, delete_file, exec_command'"
                  @ionInput="handleToolsInput($event)"
                ></ion-input>
              </ion-item>
              <ion-item v-if="toolsChips.length > 0" lines="none" class="tools-chip-item">
                <ion-label class="ion-text-wrap">
                  <div class="tools-chip-row">
                    <ion-chip
                      v-for="tool in toolsChips"
                      :key="tool"
                      @click="removeTool(tool)"
                      class="tool-chip"
                    >
                      {{ tool }}
                      <ion-icon :icon="closeIcon"></ion-icon>
                    </ion-chip>
                  </div>
                </ion-label>
              </ion-item>
              <ion-item v-if="child.description" lines="none">
                <ion-note class="field-description">
                  {{ child.description }}
                </ion-note>
              </ion-item>
            </template>

            <!-- openai_api_key：使用通用密码输入框（InputWithHistory），保存时自动加密 -->
            <!-- 用 <template> 把所有 openai_api_key 相关 UI 包成单一条件块，-->
            <!-- 避免 v-if/v-else-if 链被中间插入的 v-if 打断导致 ConfigFieldItem 误命中 -->
            <template v-else-if="child.key === 'openai_api_key'">
              <div class="api-key-input-wrap" :class="{ 'api-key-broken': apiKeyStatus === 'decrypt-failed' }">
                <InputWithHistory
                  :model-value="apiKeyPlainValue"
                  :label="fieldLabel(child.key, child.required)"
                  :placeholder="apiKeyInputPlaceholder"
                  :icon="key"
                  input-type="password"
                  :history-key="'config.agent_api_key'"
                  :is-customized="isApiKeyCustomized"
                  @update:model-value="handleApiKeyInput($event)"
                  @reset="handleApiKeyReset"
                  @keyup-enter="handleApiKeyEnter"
                  @blur="handleApiKeyBlur"
                />
                <p v-if="apiKeyStatus === 'decrypt-failed' && !apiKeyPlainValue" class="api-key-mask-hint">
                  <ion-icon :icon="lockIcon" class="api-key-mask-icon"></ion-icon>
                  {{ t('agent.apiKeyMaskHint') || '已存储加密值但当前无法解密显示，请重新输入后保存以覆盖损坏值' }}
                </p>
              </div>

              <!-- API Key 状态徽标 + 后端 base + 测试按钮（紧跟 input 显示） -->
              <ion-item lines="none" class="apiKeyStatusItem">
                <ion-icon :icon="bugIcon" slot="start" class="apiKeyStatusIcon"></ion-icon>
                <ion-label class="ion-text-wrap">
                  <div class="apiKeyStatusRow">
                    <ion-badge :color="apiKeyStatusBadge.color" class="apiKeyStatusBadge">
                      <ion-icon
                        v-if="apiKeyStatusBadge.spinning"
                        :icon="apiKeyStatusBadge.icon"
                        class="apiKeyStatusBadgeIcon"
                      ></ion-icon>
                      <ion-icon
                        v-else
                        :icon="apiKeyStatusBadge.icon"
                        class="apiKeyStatusBadgeIcon"
                      ></ion-icon>
                      <span class="apiKeyStatusBadgeText">{{ apiKeyStatusBadge.label }}</span>
                    </ion-badge>
                    <ion-spinner v-if="roundtripRunning" name="crescent" class="apiKeySpinner"></ion-spinner>
                  </div>
                  <p v-if="apiKeyStatusDetail" class="apiKeyStatusDetail">{{ apiKeyStatusDetail }}</p>
                  <p class="apiKeyBackendLine">
                    <span class="apiKeyBackendLabel">{{ t('agent.apiKeyBackendLabel') }}:</span>
                    <code class="apiKeyBackendBase">{{ agentApiBaseCtx.base }}</code>
                    <span class="apiKeyBackendSource">({{ agentApiBaseLabel }})</span>
                  </p>
                </ion-label>
                <ion-button
                  slot="end"
                  size="small"
                  fill="outline"
                  :disabled="roundtripRunning"
                  @click="handleRoundtripTest"
                >
                  <ion-icon :icon="refreshIcon" slot="start"></ion-icon>
                  {{ t('agent.apiKeyActionRoundtrip') }}
                </ion-button>
              </ion-item>
              <ion-item v-if="apiKeyStatusDetail" lines="none">
                <ion-button slot="end" size="small" fill="clear" color="medium" @click="goToDevLogs">
                  <ion-icon :icon="bugIcon" slot="start"></ion-icon>
                  {{ t('agent.apiKeyViewLogs') }}
                </ion-button>
              </ion-item>
            </template>

            <ConfigFieldItem
              v-else-if="child.key !== 'openai_model'"
              :field="child"
              :model-value="getValue(['agent_settings', child.key])"
              :label="fieldLabel(child.key, child.required)"
              :placeholder="child.description || fieldLabel(child.key)"
              :icon="getFieldIcon(child.key, child.type)"
              @update:model-value="setValue(['agent_settings', child.key], $event)"
              @input="handleInput(['agent_settings', child.key], child, $event)"
              @reset="resetFieldToDefault(['agent_settings', child.key], child)"
            />

            <!-- openai_model：动态模型选择器（从供应商 API 获取） -->
            <div v-else-if="child.key === 'openai_model'" class="config-field config-field-card">
              <div class="field-label-row">
                <ion-icon :icon="sparklesOutline" class="field-icon"></ion-icon>
                <span class="field-label-text">{{ fieldLabel(child.key, child.required) }}</span>
                <ion-icon :icon="cloudOutline" class="sync-indicator" :title="t('settings.synced')"></ion-icon>
              </div>
              <p v-if="child.description" class="field-description-text">{{ child.description }}</p>

              <!-- 加载中 -->
              <div v-if="settingsModelsLoading" class="model-loading">
                <ion-spinner name="crescent" class="model-spinner"></ion-spinner>
                <span>{{ t('agent.loadingModels') }}...</span>
              </div>

              <!-- 加载失败：显示错误 + 手动输入回退 + 跳转到 API Key 设置（依赖关系暴露） -->
              <div v-else-if="settingsModelsError" class="model-error-state">
                <p class="model-error-text">{{ settingsModelsError }}</p>
                <div class="model-error-actions">
                  <!-- 当错误根因是 API Key 状态问题时，提供"一键跳转"入口 -->
                  <!-- 用原生 <button> 避免 ion-button 内部 Shadow DOM 偶发拦截 @click 冒泡 -->
                  <button
                    v-if="isModelErrorCausedByApiKey"
                    type="button"
                    class="model-error-fix-btn"
                    @click="scrollToApiKey"
                  >
                    <ion-icon :icon="key" class="model-error-fix-icon"></ion-icon>
                    {{ t('agent.modelErrorFixApiKey') || '↑ 跳转到 API Key 设置' }}
                  </button>
                  <ion-button
                    size="small"
                    fill="clear"
                    color="medium"
                    @click="goToDevLogs"
                  >
                    <ion-icon :icon="bugIcon" slot="start"></ion-icon>
                    {{ t('agent.apiKeyViewLogs') }}
                  </ion-button>
                </div>
                <input
                  type="text"
                  class="model-fallback-input"
                  :value="String(getValue(['agent_settings', 'openai_model']) ?? '')"
                  :placeholder="t('agent.modelFallbackPlaceholder') || '输入模型名称'"
                  @input="handleModelManualInput($event)"
                />
              </div>

              <!-- 正常：动态 preset-cards -->
              <div v-else-if="settingsModelOptions.length > 0" class="preset-cards">
                <div
                  v-for="opt in settingsModelOptions"
                  :key="opt.id"
                  class="preset-card"
                  :class="{ 'preset-card-active': String(getValue(['agent_settings', 'openai_model'])) === opt.id }"
                  @click="setValue(['agent_settings', 'openai_model'], opt.id)"
                >
                  <div class="preset-card-title">{{ opt.name || opt.id }}</div>
                  <div v-if="opt.provider && opt.provider !== 'unknown'" class="preset-card-desc">{{ opt.provider }}</div>
                </div>
              </div>

              <!-- 空列表 -->
              <div v-else class="model-empty">
                <p>{{ t('agent.noModelsAvailable') || '无可用模型' }}</p>
                <input
                  type="text"
                  class="model-fallback-input"
                  :value="String(getValue(['agent_settings', 'openai_model']) ?? '')"
                  :placeholder="t('agent.modelFallbackPlaceholder') || '输入模型名称'"
                  @input="handleModelManualInput($event)"
                />
              </div>
            </div>
          </template>
        </ion-list>

        <ion-list>
          <ion-item button :disabled="testing" @click="handleTestConnection">
            <ion-icon :icon="flashIcon" slot="start"></ion-icon>
            <ion-label>
              <h3>{{ t('settings.testConnection') }}</h3>
              <p v-if="testResult" :class="testResultClass">
                {{ testResult }}
              </p>
            </ion-label>
            <ion-spinner v-if="testing" slot="end" name="crescent"></ion-spinner>
          </ion-item>
        </ion-list>

        <!-- Task 25: Sync Doctor — 脱敏诊断报告 -->
        <ion-list>
          <ion-item button :disabled="doctorRunning" @click="handleRunSyncDoctor">
            <ion-icon :icon="medkitIcon" slot="start"></ion-icon>
            <ion-label>
              <h3>{{ t('agent.syncDoctor') }}</h3>
              <p>{{ doctorIssuesCount > 0 ? t('agent.syncDoctorResult') + ' (' + doctorIssuesCount + ')' : t('agent.syncDoctorEmpty') }}</p>
            </ion-label>
            <ion-spinner v-if="doctorRunning" slot="end" name="crescent"></ion-spinner>
          </ion-item>
          <ion-item v-if="doctorReportJson" lines="none" class="doctor-result-item">
            <div class="doctor-result-wrap">
              <pre class="doctor-result-pre">{{ doctorReportJson }}</pre>
              <div class="doctor-result-actions">
                <ion-button size="small" fill="outline" @click="handleCopyDoctorJson">
                  <ion-icon :icon="copyIcon" slot="start"></ion-icon>
                  {{ t('agent.syncDoctorCopy') }}
                </ion-button>
              </div>
            </div>
          </ion-item>
        </ion-list>
      </template>

      <ion-list v-if="configLoaded && agentSection">
        <ion-item button @click="openJsonEditor">
          <ion-icon :icon="documentText" slot="start"></ion-icon>
          <ion-label>
            <h3>{{ t('settings.editRawConfig') }}</h3>
          </ion-label>
        </ion-item>
      </ion-list>

      <ion-modal :is-open="showJsonEditor" @didDismiss="showJsonEditor = false">
        <ion-header>
          <ion-toolbar>
            <ion-title>{{ t('settings.editRawConfig') }}</ion-title>
            <ion-buttons slot="end">
              <ion-button @click="showJsonEditor = false">{{ t('settings.cancel') }}</ion-button>
              <ion-button @click="handleSaveJson" :disabled="!!jsonError" color="primary">{{ t('settings.saveConfig') }}</ion-button>
            </ion-buttons>
          </ion-toolbar>
        </ion-header>
        <ion-content class="json-editor-content">
          <textarea
            v-model="jsonText"
            class="json-textarea"
            spellcheck="false"
            @input="validateJson"
          ></textarea>
          <div v-if="jsonError" class="json-error">
            {{ t('settings.jsonError') }}: {{ jsonError }}
          </div>
        </ion-content>
      </ion-modal>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { fetchConfig, getApiBaseUrl, updateConfig } from "@/api/encv";
import { devlogApiError, devlogApiInfo } from "@/composables/devlogApiError";
import { type DoctorReport, runSyncDoctor } from "@/composables/useAgent";
import { getAgentApiBase, getAgentApiBaseContext } from "@/composables/useAgentApiBase";
import { useConfig } from "@/composables/useConfig";
import { getDeviceId } from "@/composables/useDeviceId";
import { useI18n } from "@/composables/useI18n";
import { useServerStatus } from "@/composables/useServerStatus";
import { showToast } from "@/composables/useToast";
import type { FieldDef } from "@/config/schemaParser";
import {
  alertCircleOutline,
  bugOutline,
  checkmarkCircle,
  closeCircleOutline,
  copyOutline,
  documentText,
  flashOutline,
  globeOutline,
  key,
  listOutline,
  lockClosed,
  lockOpenOutline,
  medkitOutline,
  optionsOutline,
  refreshOutline,
  settingsOutline,
  sparklesOutline,
  speedometerOutline,
} from "ionicons/icons";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";

/** 健壮的错误序列化 — 处理 TypeError/DOMException/AbortError/普通 Error 等所有情况 */
function serializeError(e: unknown): string {
  if (!e) return "(null error)";
  if (e instanceof Error) {
    // TypeError: Failed to fetch / DOMException: NetworkError 等
    const parts: string[] = [e.name || "Error", e.message || "(no message)"];
    if ((e as any).cause) parts.push(`cause=${serializeError((e as any).cause)}`);
    return parts.filter(Boolean).join(": ");
  }
  if (typeof e === "string") return e;
  try {
    return JSON.stringify(e);
  } catch {
    return String(e);
  }
}

const { isOnline: serverOnline, checkStatus: checkServerStatusNow } = useServerStatus();
const {
  schemaFields,
  loading: configLoading,
  dirty,
  loadConfig,
  saveConfig,
  resetConfig,
  getFieldValue,
  setFieldValue,
  resetFieldToDefault,
} = useConfig();
const { t, tField } = useI18n();
const router = useRouter();

const configLoaded = ref(false);
const configError = ref(""); // 配置加载失败原因（用于错误态 UI 展示）
const autoRetryCount = ref(0); // 自动重试次数（显示在"重试"按钮上）
const testing = ref(false);
const testResult = ref("");
const testResultSuccess = ref(false);

const _listIcon = listOutline;
const _flashIcon = flashOutline;
const _closeIcon = closeCircleOutline;
const lockOpenIcon = lockOpenOutline;
const lockIcon = lockClosed;
const checkmarkIcon = checkmarkCircle;
const alertIcon = alertCircleOutline;
const refreshIcon = refreshOutline;
const _bugIcon = bugOutline;
const _medkitIcon = medkitOutline;
const _copyIcon = copyOutline;

// ─── API Key 状态机（spec F.3 状态反馈 UI）────────────────────
type ApiKeyStatus =
  | "empty" // 未配置
  | "plaintext" // 加载到明文（明文储存格式或刚解密回填）
  | "encrypted" // 已加密储存（enc:xxx）
  | "decrypting" // 解密中
  | "encrypting" // 加密中
  | "decrypt-failed" // 解密失败
  | "encrypt-failed" // 加密失败
  | "test-failed" // 连通性测试失败
  | "roundtrip-ok" // 往返测试成功
  | "roundtrip-mismatch"; // 往返测试不一致

const apiKeyStatus = ref<ApiKeyStatus>("empty");
const apiKeyStatusDetail = ref(""); // 详细错误信息（用于展开）
const roundtripRunning = ref(false);

const apiKeyPlainValue = ref(""); // 用户正在编辑的明文（内存中，password input 自动掩码）

// Template ref：用于"跳转到 API Key"按钮的滚动定位 + 焦点
// 注意：原本用 ref="apiKeyInputRef" 绑 <div>，但 <div> 在 <template v-for> 内
// 会被 Vue 收集为数组（[div]），调 scrollIntoView 报 "is not a function"。
// 改用 scrollToApiKey() 内部的 document.querySelector('.api-key-input-wrap') 抓取。
// 保留 ref 声明以备未来其他需要（暂未使用，加 :ref 也不绑，删了避免误导）。
// const apiKeyInputRef = ref<HTMLElement | null>(null)

// Template ref：用于"跳转到 API Key"错误暴露按钮的 click 绑定
// 用 addEventListener 而非 @click：ion-button 内部 Shadow DOM 偶发会拦截 @click 冒泡
// addEventListener 直接挂到外层 div 上，不经过 Shadow DOM 中转
//
// 不使用 ref="..." 绑定：v-for 内部 ref 会被 Vue 自动收集为数组，
// 函数 ref 也不能稳定拿到 DOM 元素。
// 改为 onMounted + onActivated 后用 querySelector 抓取，更可靠。

// 输入框 placeholder 根据 API Key 状态切换
// 当 decrypt-failed 时明确告诉用户"这里有损坏的密文"，避免被误认为输入框是空的
const _apiKeyInputPlaceholder = computed(() => {
  if (apiKeyStatus.value === "decrypt-failed") {
    return t("agent.apiKeyPlaceholderBroken") || "已存储加密值但无法解密，请重新输入";
  }
  if (apiKeyStatus.value === "encrypted" || apiKeyStatus.value === "plaintext") {
    return t("agent.apiKeyPlaceholderKeep") || "已配置（输入新值将覆盖）";
  }
  return t("agent.apiKeyPlaceholder") || "sk-...";
});

// model-error 是否由 API Key 状态问题引起——决定是否显示"一键跳转"按钮
// 关键：让依赖关系可被发现。模型列表不能孤立地报告"未配置 API Key"，
// 必须在 UI 上提供回到根因的入口
const _isModelErrorCausedByApiKey = computed(() => {
  return (
    apiKeyStatus.value === "empty" ||
    apiKeyStatus.value === "decrypt-failed" ||
    apiKeyStatus.value === "encrypt-failed" ||
    apiKeyStatus.value === "test-failed"
  );
});

// 滚动到 API Key 输入框并聚焦
// 这是"错误暴露机制"的关键：让用户从下游错误直接跳到根因
// 关键技术点：ion-input 会把原生 <input> 放在 light DOM 里（id=ion-input-X, name=ion-input-X），
// 所以 querySelector('input') 在 light DOM 就能拿到，不必穿透 Shadow DOM。
// 但聚焦后需要保持：ion-input 的 ionFocusable 行为可能再次夺焦，所以聚焦后必须保留。
//
// 重要：不能用 apiKeyInputRef.value 拿 DOM —— 模板里 <div ref="apiKeyInputRef"> 在
// <template v-for> 内，Vue 收集的是数组，ref.value 是 [div]，调 scrollIntoView
// 必报 "el.scrollIntoView is not a function"。统一改用 querySelector 抓取。
function scrollToApiKey() {
  nextTick(() => {
    // 防御 1：找 .api-key-input-wrap —— 页面里只有一个（openai_api_key 字段专属）
    const el = document.querySelector<HTMLElement>(".api-key-input-wrap");
    if (!el) {
      console.error("[scrollToApiKey] no .api-key-input-wrap found");
      return;
    }
    el.scrollIntoView({ behavior: "smooth", block: "center" });
    // 调试：打印 DOM 结构，确认 input 在哪一层
    const wrap = el.querySelector(".input-with-history") || el;
    const lightInputs = wrap.querySelectorAll("input");
    const ionInputEl = el.querySelector("ion-input");
    const shadowInput = ionInputEl?.shadowRoot?.querySelector("input") as HTMLInputElement | null;
    console.debug("[scrollToApiKey] DOM tree", {
      lightInputs: lightInputs.length,
      shadowInput: !!shadowInput,
      hasIonInputSetFocus: ionInputEl && typeof (ionInputEl as any).setFocus === "function",
    });
    // 优先用 ion-input 的官方 setFocus（最稳定，会处理 Shadow DOM 焦点 + 滚动定位）
    if (ionInputEl && typeof (ionInputEl as any).setFocus === "function") {
      setTimeout(() => {
        try {
          (ionInputEl as any).setFocus();
          // 再在 light DOM 上把 input 选中，方便用户直接覆盖
          const realInput = lightInputs[0] || shadowInput;
          if (realInput) {
            // 再延迟一点 select，等 focus 真正生效
            setTimeout(() => {
              try {
                (realInput as HTMLInputElement).select();
              } catch {
                /* ignore */
              }
            }, 50);
          }
        } catch (e) {
          console.error("[scrollToApiKey] setFocus failed:", e instanceof Error ? `${e.name}: ${e.message}` : String(e));
        }
      }, 400);
      return;
    }
    // 兜底：直接 focus light DOM 上的 input
    if (lightInputs[0]) {
      setTimeout(() => {
        try {
          lightInputs[0].focus({ preventScroll: true });
          lightInputs[0].select();
        } catch (e) {
          console.error("[scrollToApiKey] light focus failed:", e instanceof Error ? `${e.name}: ${e.message}` : String(e));
        }
      }, 400);
    }
  });
}

// 是否已自定义（非默认空值）
// 注意：不能用 apiKeyPlainValue 作为判断源——页面加载时 decryptAndLoadApiKey 会把解密后的
// 明文回填到 apiKeyPlainValue，导致"已保存的密文"被误判为"用户修改中"。
// 正确语义：当前显示内容是否与"默认空值"不同 → 存储值非空即视为已自定义。
const _isApiKeyCustomized = computed(() => {
  const stored = getFieldValue(["agent_settings", "openai_api_key"]);
  return typeof stored === "string" && stored.length > 0;
});

// 状态徽标展示
const _apiKeyStatusBadge = computed(() => {
  switch (apiKeyStatus.value) {
    case "empty":
      return { color: "medium" as const, icon: lockOpenIcon, label: t("agent.apiKeyStatusEmpty") };
    case "plaintext":
      return { color: "warning" as const, icon: lockOpenIcon, label: t("agent.apiKeyStatusPlaintext") };
    case "encrypted":
      return { color: "success" as const, icon: lockIcon, label: t("agent.apiKeyStatusEncrypted") };
    case "decrypting":
      return { color: "primary" as const, icon: refreshIcon, label: t("agent.apiKeyStatusDecrypting"), spinning: true };
    case "encrypting":
      return { color: "primary" as const, icon: refreshIcon, label: t("agent.apiKeyStatusEncrypting"), spinning: true };
    case "decrypt-failed":
      return { color: "danger" as const, icon: alertIcon, label: t("agent.apiKeyStatusDecryptFailed") };
    case "encrypt-failed":
      return { color: "danger" as const, icon: alertIcon, label: t("agent.apiKeyStatusEncryptFailed") };
    case "test-failed":
      return { color: "danger" as const, icon: alertIcon, label: t("agent.apiKeyStatusTestFailed") };
    case "roundtrip-ok":
      return { color: "success" as const, icon: checkmarkIcon, label: t("agent.apiKeyStatusRoundtripOk") };
    case "roundtrip-mismatch":
      return { color: "danger" as const, icon: alertIcon, label: t("agent.apiKeyStatusRoundtripMismatch") };
    default:
      return { color: "medium" as const, icon: lockOpenIcon, label: "" };
  }
});

// Agent API base 当前解析（用于 UI 展示"实际打到哪里"）
const agentApiBaseCtx = computed(() => getAgentApiBaseContext());
const _agentApiBaseLabel = computed(() => {
  switch (agentApiBaseCtx.value.source) {
    case "dev-gateway":
      return t("agent.apiKeyBackendDev");
    case "native-default":
      return t("agent.apiKeyBackendNative");
    case "user-configured":
      return t("agent.apiKeyBackendUser");
    case "web-fallback":
      return t("agent.apiKeyBackendFallback");
  }
});

/**
 * 加载配置后自动解密 API Key（如果存储的是加密格式）
 * 解密后的明文存入 apiKeyPlainValue，由 InputWithHistory(password) 的 type="password" 自动掩码显示
 *
 * 状态机驱动：
 *   - 'empty'      → 没存
 *   - 'plaintext'  → 存的是明文（旧数据或 dev 模式手动）
 *   - 'decrypting' → 正在调 /api/decrypt-key
 *   - 'encrypted'  → 解密成功（明文已回填到 input 内存）
 *   - 'decrypt-failed' → /api/decrypt-key 返回非 2xx
 */
async function decryptAndLoadApiKey() {
  const stored = String(getFieldValue(["agent_settings", "openai_api_key"]) ?? "");

  // 1. 决定初始状态（不依赖 network）
  if (!stored) {
    apiKeyPlainValue.value = "";
    apiKeyStatus.value = "empty";
    apiKeyStatusDetail.value = "";
    return;
  }
  if (!stored.startsWith("enc:")) {
    // 明文存储 → 不需要网络，但提示"未加密"以防误以为是加密的
    apiKeyPlainValue.value = stored;
    apiKeyStatus.value = "plaintext";
    apiKeyStatusDetail.value = "Stored without enc: prefix; encryption skipped on save";
    return;
  }

  // 2. encrypted 格式 → 调 /api/decrypt-key
  apiKeyStatus.value = "decrypting";
  apiKeyStatusDetail.value = "";
  let deviceId = "";
  try {
    deviceId = await getDeviceId();
    const res = await fetch(`${getAgentApiBase()}/api/decrypt-key`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ encrypted: stored, deviceId }),
    });
    if (res.ok) {
      const data = await res.json().catch(() => ({}) as any);
      const decrypted = typeof data?.decrypted === "string" ? data.decrypted : "";
      if (decrypted) {
        // 真正解密成功：明文回填到 input，状态机切换到 encrypted
        apiKeyPlainValue.value = decrypted;
        apiKeyStatus.value = "encrypted";
        apiKeyStatusDetail.value = "";
        devlogApiInfo(`decrypt-key OK (${decrypted.length} chars)`, { kind: "decrypt" });
      } else {
        // HTTP 200 但 decrypted 为空：后端所有格式都解不出（key/salt 不匹配/数据被截断等）
        // 2026-06 修复：自动破坏性迁移 → 调 /api/agent/reset-key 把 config 里的 openai_api_key
        // 字段清空，让用户重新输入一次。
        //
        // 为什么是自动而不是提示用户？
        //   旧密文（Node.js agent-stub 加密 / deviceId-bound）永久解不出。
        //   提示用户"重新输入"需要用户主动操作，但用户已经在"4 条 decrypt failed 日志"中愤怒
        //   ——再让他手动清空 + 手动输入 + 手动保存，他会骂人。
        //   改成：自动清空 + 自动 focus 输入框 + 大红色 banner 解释原因 + 用户只输一次即可。
        apiKeyPlainValue.value = "";
        apiKeyStatus.value = "decrypt-failed";
        apiKeyStatusDetail.value =
          t("agent.apiKeyStatusDecryptFailedEmpty") ||
          "Stored API key cannot be decrypted. The encryption key may have rotated or the stored value is corrupted. Auto-clearing now — please re-enter your key.";
        devlogApiError(new Error("decrypt-key returned empty decrypted"), {
          kind: "decrypt",
          endpoint: "/api/decrypt-key",
          status: res.status,
          deviceId,
          body: JSON.stringify(data).slice(0, 200),
        });
        // 自动迁移：清空 config 里的密文 + 状态机 + 自动跳到 input
        await autoResetBrokenApiKey();
      }
    } else {
      let body = "";
      try {
        body = await res.text();
      } catch {
        /* ignore */
      }
      const detail = `HTTP ${res.status}${body ? `: ${body.slice(0, 200)}` : ""}`;
      apiKeyPlainValue.value = "";
      apiKeyStatus.value = "decrypt-failed";
      apiKeyStatusDetail.value = detail;
      devlogApiError(new Error(`decrypt-key ${detail}`), {
        kind: "decrypt",
        endpoint: "/api/decrypt-key",
        status: res.status,
        body,
        deviceId,
      });
    }
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    apiKeyPlainValue.value = "";
    apiKeyStatus.value = "decrypt-failed";
    apiKeyStatusDetail.value = detail;
    devlogApiError(e, {
      kind: "decrypt",
      endpoint: "/api/decrypt-key",
      deviceId,
    });
  }
}

function _handleApiKeyInput(val: string) {
  apiKeyPlainValue.value = val;
  setFieldValue(["agent_settings", "openai_api_key"], val);
  // 用户开始编辑：状态机切到 'plaintext'（避免仍显示"已加密"绿徽标误导用户）
  // 保留 encrypted/decrypt-failed 等已发生的错误状态：仅在用户实际"从干净状态开始输入"时切换
  if (apiKeyStatus.value === "empty" || apiKeyStatus.value === "encrypted" || apiKeyStatus.value === "plaintext") {
    if (val) {
      apiKeyStatus.value = "plaintext";
      apiKeyStatusDetail.value = "Unencrypted in memory; will be encrypted on save";
    } else {
      // 用户把内容清空 → 回到 empty
      apiKeyStatus.value = "empty";
      apiKeyStatusDetail.value = "";
    }
  }
}

// 重置 API Key 到默认值（空）
// 之前 InputWithHistory 的 ↺ 按钮 click 后无任何反应——@reset 事件未挂载。
// 这里把存储值清空、明文缓存清空、状态机归位到 empty。
function _handleApiKeyReset() {
  apiKeyPlainValue.value = "";
  setFieldValue(["agent_settings", "openai_api_key"], "");
  apiKeyStatus.value = "empty";
  apiKeyStatusDetail.value = "";
  devlogApiInfo("api_key reset to default (empty)", { kind: "replay", endpoint: "reset-button" });
}

/**
 * API Key 输入框按 Enter 时触发：立即加密 + 保存整个 config。
 *
 * 关键 UX：autoResetBrokenApiKey 后用户**只需输入一次 + 按 Enter** 即可完成全流程，
 * 不必再点顶部的"保存"按钮。这是从"用户愤怒不愿操作"到"零操作可用"的关键。
 */
async function _handleApiKeyEnter() {
  // 1. 基本校验：必须有内容
  const raw = apiKeyPlainValue.value.trim();
  if (!raw) {
    showToast({ message: t("agent.apiKeyEmpty") || "请输入 API Key", duration: 2000, color: "warning" });
    return;
  }
  // 2. 必须看起来像 OpenAI key（避免误存其他内容）
  if (!/^sk[-_]/i.test(raw) && raw.length < 20) {
    showToast({
      message: t("agent.apiKeyInvalid") || "API Key 格式看起来不对，应以 sk- 开头",
      duration: 3000,
      color: "warning",
    });
    return;
  }
  devlogApiInfo("api_key enter pressed → auto save", { kind: "encrypt" });
  // 3. 复用 handleSaveConfig 逻辑：加密 + 持久化整个 config
  await handleSaveConfig();
}

/**
 * 离开 API Key 输入框时自动保存。
 *
 * 为什么 blur 也要保存？
 *   - 用户可能从解密成功的"sk-placeholder-..."全选删掉 → 重新输入真 key → 直接关掉页面
 *   - 这时候 Enter 没被按过，但真 key 已经输入好了——blur 时自动保存。
 *   - 只有"用户改动了内容"才保存，避免空 blur 时也触发 save。
 */
async function _handleApiKeyBlur() {
  const raw = apiKeyPlainValue.value.trim();
  if (!raw) return; // 空白不保存
  if (raw.startsWith("enc:")) return; // 已经是密文格式（用户没改）
  // 只有当值跟初始"解密后的值"不同时才保存
  // 简化：直接保存（重复保存无害，PUT /api/config 是幂等的）
  devlogApiInfo("api_key blur → auto save", { kind: "encrypt" });
  try {
    await handleSaveConfig();
  } catch (e) {
    // 静默失败——用户没主动操作，不需要打扰
    console.debug("[handleApiKeyBlur] auto-save on blur failed:", e);
  }
}

/**
 * 自动破坏性迁移：解密失败时（典型场景：deviceId 变了 / Node.js 旧密文
 * 与 Go 端 scrypt 不兼容），调用后端 /api/agent/reset-key 把 config 里的
 * openai_api_key 字段置空，状态机归位到 empty，并自动滚动 + 聚焦输入框。
 *
 * 关键 UX：用户**无需任何操作**——只需看到红 banner + 已被聚焦的输入框，
 * 直接打字输入新 key 即可，blur 时自动保存。
 */
async function autoResetBrokenApiKey() {
  try {
    const res = await fetch(`${getAgentApiBase()}/api/agent/reset-key`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
    if (res.ok) {
      const data = await res.json().catch(() => ({}) as any);
      devlogApiInfo(`reset-key OK (prev_len=${data?.prevLen ?? "n/a"})`, { kind: "decrypt" });
    } else {
      devlogApiError(new Error(`reset-key HTTP ${res.status}`), {
        kind: "decrypt",
        endpoint: "/api/agent/reset-key",
        status: res.status,
      });
    }
  } catch (e) {
    devlogApiError(e, { kind: "decrypt", endpoint: "/api/agent/reset-key" });
  }
  // 不论后端成功与否，都清掉前端 store + 状态机
  setFieldValue(["agent_settings", "openai_api_key"], "");
  apiKeyPlainValue.value = "";
  apiKeyStatus.value = "empty";
  apiKeyStatusDetail.value =
    t("agent.apiKeyAutoReset") ||
    "Old API key was unreadable and has been auto-cleared. Please enter your key below — it will save automatically on blur.";
  // 自动滚动 + 聚焦：让用户无缝衔接
  scrollToApiKey();
  devlogApiInfo("api_key auto-reset, scrolling to input", { kind: "decrypt" });
}

// ─── 动态模型选择（openai_model 字段） ──────────────────────
interface SettingsModelOption {
  id: string;
  name: string;
  provider: string;
}
const settingsModelOptions = ref<SettingsModelOption[]>([]);
const settingsModelsLoading = ref(true);
const settingsModelsError = ref("");

async function fetchSettingsModels() {
  // 依赖关卡：API Key 状态不健康时直接走"已知的失败原因"分支，
  // 不向后端发无意义的请求，同时给用户精准的错误文案（不显示误导性的"未配置"）
  if (apiKeyStatus.value === "empty") {
    settingsModelsLoading.value = false;
    settingsModelsError.value =
      t("agent.modelErrorNoApiKey") || "未配置 API Key：模型列表需要有效的 OpenAI API Key 才能拉取。请在上方填写 API Key 后保存。";
    return;
  }
  if (apiKeyStatus.value === "decrypt-failed") {
    settingsModelsLoading.value = false;
    settingsModelsError.value =
      t("agent.modelErrorDecryptFailed") ||
      "API Key 已存储但无法解密：加密密钥可能已轮换或存储值已损坏。请在上方重新输入 API Key 后保存以覆盖。";
    return;
  }
  if (apiKeyStatus.value === "encrypting" || apiKeyStatus.value === "decrypting") {
    // 异步状态未稳定，等 watcher 触发重试
    settingsModelsLoading.value = true;
    settingsModelsError.value = "";
    return;
  }

  settingsModelsLoading.value = true;
  settingsModelsError.value = "";
  let url = "";
  try {
    // 关键：必须传 deviceId 给后端！
    // 后端 readAgentConfig(deviceId) 用 deviceId 派生 AES 解密 key，
    // 不传 deviceId 会用错的 key 派生，永远解不出设备绑定的密文。
    const did = await getDeviceId();
    url = `${getAgentApiBase()}/api/models?deviceId=${encodeURIComponent(did)}`;
    const res = await fetch(url);
    if (!res.ok) throw new Error(`HTTP ${res.status} ${res.statusText}`);
    const data = await res.json();
    if (data.error && data.error === "no_api_key") {
      // 罕见路径：API Key 状态显示正常但后端拿不到（并发竞态 / 配置 reload 中）
      // 同样把"一键跳转"露出
      settingsModelsError.value =
        t("agent.modelErrorNoApiKey") || "未配置 API Key：模型列表需要有效的 OpenAI API Key 才能拉取。请在上方填写 API Key 后保存。";
    } else if (data.error) {
      settingsModelsError.value = data.note || data.error;
    } else {
      settingsModelOptions.value = (data.models || []).map((m: any) => ({
        id: m.id,
        name: m.name || m.id,
        provider: m.provider || "unknown",
      }));
    }
  } catch (e: any) {
    const errInfo = serializeError(e);
    console.error(`[AgentSettings] fetchModels failed: url=${url} error=${errInfo}`);
    settingsModelsError.value = `网络错误 (${errInfo})`;
  } finally {
    settingsModelsLoading.value = false;
  }
}

// API Key 状态机变化时自动同步模型列表请求
// 这是"依赖追踪"的关键——之前模型列表只在 onMounted 拉一次，
// 用户在 API Key 输入框修改后看不到模型列表的同步刷新
watch(apiKeyStatus, (newStatus, oldStatus) => {
  if (newStatus === oldStatus) return;
  // 只有从"不可用"切到"可用"时才主动重试
  const wasBroken = oldStatus === "empty" || oldStatus === "decrypt-failed" || oldStatus === "encrypt-failed";
  const isReady = newStatus === "encrypted" || newStatus === "plaintext";
  if (wasBroken && isReady) {
    fetchSettingsModels();
  } else if (newStatus === "empty" || newStatus === "decrypt-failed") {
    // 状态机进入"不可用"，立刻同步给模型列表（不等待 fetch 返回）
    fetchSettingsModels();
  }
});

function _handleModelManualInput(event: Event) {
  const val = (event.target as HTMLInputElement).value;
  setValue(["agent_settings", "openai_model"], val);
}

const showJsonEditor = ref(false);
const jsonText = ref("");
const jsonError = ref("");

// 诊断信息：错误态 UI 展示的完整状态快照
// 目的：让用户（和开发者）一眼看到"到底哪一步失败了"，而不是只看到"后端服务未连接"
const _diagInfo = computed(() => {
  const lines: string[] = [];
  lines.push(`serverOnline   = ${serverOnline.value}`);
  lines.push(`configLoaded    = ${configLoaded.value}`);
  lines.push(`configError     = ${JSON.stringify(configError.value).slice(0, 200)}`);
  lines.push(`autoRetryCount  = ${autoRetryCount.value}`);
  lines.push(`configLoading   = ${configLoading.value}`);
  const now = new Date().toLocaleTimeString();
  lines.push(`timestamp       = ${now}`);
  try {
    const base = getAgentApiBaseContext();
    lines.push(`agentApiBase     = ${base.base} (${base.source})`);
  } catch {
    /* ignore */
  }
  try {
    const apiBase = getApiBaseUrl();
    lines.push(`apiBaseUrl       = ${apiBase || "(empty/dev mode)"}`);
  } catch {
    /* ignore */
  }
  return lines.join("\n");
});

const _agentSection = computed<FieldDef | undefined>(() => {
  return schemaFields.value.find(s => s.key === "agent_settings");
});

const toolsChips = computed<string[]>(() => {
  const raw = getFieldValue(["agent_settings", "enabled_tools"]);
  if (Array.isArray(raw)) return raw.filter((s): s is string => typeof s === "string");
  // 兼容老数据：值是 string 时（如 "list_files,read_file"），解析为数组。
  // 否则用户看到的 chip 列表与实际存储值完全不一致——会以为是 bug。
  if (typeof raw === "string" && raw.length > 0) {
    return raw
      .split(",")
      .map(s => s.trim())
      .filter(s => s.length > 0);
  }
  return [];
});

const _toolsText = computed(() => toolsChips.value.join(", "));

const _testResultClass = computed(() => (testResultSuccess.value ? "test-result-success" : "test-result-failed"));

function _getValue(path: string[]): unknown {
  return getFieldValue(path);
}

function setValue(path: string[], value: unknown) {
  setFieldValue(path, value);
}

function _handleInput(path: string[], field: FieldDef, event: CustomEvent) {
  // ion-input 的 ionInput 事件是 CustomEvent，值在 event.detail.value
  // 不能用 event.target.value：event.target 是 ion-input 元素（不是原生 input）
  // 直接读 .detail.value 才是稳定可靠的（兼容 number / string / 未知类型）
  const detail: any = (event as any)?.detail;
  const raw = typeof detail?.value === "string" || typeof detail?.value === "number" ? detail.value : "";
  if (field.type === "integer") {
    setFieldValue(path, raw !== "" && raw !== null && raw !== undefined ? Number(raw) : 0);
  } else {
    setFieldValue(path, String(raw));
  }
}

function _handleToolsInput(event: CustomEvent) {
  const raw = (event.target as HTMLInputElement).value || "";
  const list = raw
    .split(",")
    .map(s => s.trim())
    .filter(s => s.length > 0);
  setFieldValue(["agent_settings", "enabled_tools"], list);
}

function _removeTool(name: string) {
  const next = toolsChips.value.filter(t => t !== name);
  setFieldValue(["agent_settings", "enabled_tools"], next);
}

function _fieldLabel(key: string, _required?: boolean): string {
  return tField(key);
}

const fieldIconMap: Record<string, string> = {
  openai_api_key: key,
  openai_base_url: globeOutline,
  openai_model: sparklesOutline,
  openlist_base_url: globeOutline,
  openlist_token: lockClosed,
  default_container_version: speedometerOutline,
  enabled_tools: optionsOutline,
  system_prompt: documentText,
  max_tool_calls_per_turn: speedometerOutline,
};

function _getFieldIcon(fieldKey: string, fieldType: string): string {
  if (fieldIconMap[fieldKey]) return fieldIconMap[fieldKey];
  if (fieldType === "boolean") return settingsOutline;
  if (fieldType === "integer") return speedometerOutline;
  if (fieldKey.includes("password")) return lockClosed;
  return settingsOutline;
}

async function handleSaveConfig() {
  try {
    // 保存前加密 API Key（防止明文写入 config.user.json）
    const rawKey = String(getFieldValue(["agent_settings", "openai_api_key"]) ?? "");
    if (rawKey && !rawKey.startsWith("enc:")) {
      // 状态：开始加密
      apiKeyStatus.value = "encrypting";
      apiKeyStatusDetail.value = "";
      let deviceId = "";
      try {
        deviceId = await getDeviceId();
        const encRes = await fetch(`${getAgentApiBase()}/api/encrypt-key`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ key: rawKey, deviceId }),
        });
        if (encRes.ok) {
          const { encrypted } = await encRes.json();
          setFieldValue(["agent_settings", "openai_api_key"], encrypted);
          // 关键：保留 apiKeyPlainValue = rawKey，让密码框继续显示掩码的明文，
          // 否则用户保存后看到"已加密"绿徽标但输入框空白，会以为 key 丢了。
          // 显式保留可以避免 reload 时还要重新解密才能看到掩码值。
          apiKeyStatus.value = "encrypted";
          apiKeyStatusDetail.value = "";
          devlogApiInfo(`encrypt-key OK (${encrypted.length} chars)`, { kind: "encrypt" });
        } else {
          let body = "";
          try {
            body = await encRes.text();
          } catch {
            /* ignore */
          }
          const detail = `HTTP ${encRes.status}${body ? `: ${body.slice(0, 200)}` : ""}`;
          apiKeyStatus.value = "encrypt-failed";
          apiKeyStatusDetail.value = detail;
          devlogApiError(new Error(`encrypt-key ${detail}`), {
            kind: "encrypt",
            endpoint: "/api/encrypt-key",
            status: encRes.status,
            body,
            deviceId,
          });
          // 关键修复：加密失败 → 中止保存！否则 saveConfig() 会把 config.value 里的
          // 明文 API Key 写入磁盘，下次启动变成"明文存储的 API Key"，彻底破坏
          // 加密存储的设计目标。
          showToast({
            message: t("agent.apiKeyEncryptFailedSaveAborted") || `API Key 加密失败，已中止保存：${detail}`,
            duration: 4000,
            color: "danger",
          });
          return;
        }
      } catch (e) {
        const detail = e instanceof Error ? e.message : String(e);
        apiKeyStatus.value = "encrypt-failed";
        apiKeyStatusDetail.value = detail;
        devlogApiError(e, {
          kind: "encrypt",
          endpoint: "/api/encrypt-key",
          deviceId,
        });
        // 同样：网络异常时中止保存，防止明文泄漏
        showToast({
          message: t("agent.apiKeyEncryptFailedSaveAborted") || `API Key 加密失败，已中止保存：${detail}`,
          duration: 4000,
          color: "danger",
        });
        return;
      }
    }
    await saveConfig();
    showToast({ message: t("settings.configSaved"), duration: 1500, color: "success" });
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    showToast({ message: t("settings.configSaveFailed") + ": " + detail, duration: 3000, color: "danger" });
  }
}

/**
 * 往返测试：明文 → /encrypt-key → /decrypt-key → 比对原文
 *
 * 用于诊断"加密看似成功但解密时数据丢失 / 哈希被改" 等问题。
 * 与 handleSaveConfig 完全独立：不修改任何持久化数据。
 */
async function _handleRoundtripTest() {
  const rawKey = apiKeyPlainValue.value || String(getFieldValue(["agent_settings", "openai_api_key"]) ?? "").replace(/^enc:/, "");
  if (!rawKey) {
    showToast({ message: t("agent.apiKeyStatusEmpty"), duration: 1500, color: "warning" });
    return;
  }
  roundtripRunning.value = true;
  apiKeyStatus.value = "encrypting";
  apiKeyStatusDetail.value = "";

  let deviceId = "";
  try {
    deviceId = await getDeviceId();
    // 1. encrypt
    const encRes = await fetch(`${getAgentApiBase()}/api/encrypt-key`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key: rawKey, deviceId }),
    });
    if (!encRes.ok) {
      let body = "";
      try {
        body = await encRes.text();
      } catch {
        /* ignore */
      }
      const detail = `encrypt HTTP ${encRes.status}${body ? `: ${body.slice(0, 200)}` : ""}`;
      apiKeyStatus.value = "encrypt-failed";
      apiKeyStatusDetail.value = detail;
      devlogApiError(new Error(detail), {
        kind: "roundtrip",
        endpoint: "/api/encrypt-key",
        status: encRes.status,
        body,
        deviceId,
      });
      return;
    }
    const { encrypted } = await encRes.json();
    devlogApiInfo(`roundtrip encrypt → ${encrypted.length} chars`, { kind: "roundtrip" });

    // 2. decrypt
    apiKeyStatus.value = "decrypting";
    const decRes = await fetch(`${getAgentApiBase()}/api/decrypt-key`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ encrypted, deviceId }),
    });
    if (!decRes.ok) {
      let body = "";
      try {
        body = await decRes.text();
      } catch {
        /* ignore */
      }
      const detail = `decrypt HTTP ${decRes.status}${body ? `: ${body.slice(0, 200)}` : ""}`;
      apiKeyStatus.value = "decrypt-failed";
      apiKeyStatusDetail.value = detail;
      devlogApiError(new Error(detail), {
        kind: "roundtrip",
        endpoint: "/api/decrypt-key",
        status: decRes.status,
        body,
        deviceId,
      });
      return;
    }
    const { decrypted } = await decRes.json();

    // 3. 比对
    if (decrypted === rawKey) {
      apiKeyStatus.value = "roundtrip-ok";
      apiKeyStatusDetail.value = `${rawKey.length} chars match`;
      devlogApiInfo("roundtrip OK", { kind: "roundtrip" });
    } else {
      apiKeyStatus.value = "roundtrip-mismatch";
      apiKeyStatusDetail.value = `original=${rawKey.length} chars, decrypted=${(decrypted || "").length} chars`;
      devlogApiError(new Error("roundtrip mismatch"), {
        kind: "roundtrip",
        endpoint: "/api/decrypt-key",
        deviceId,
        extra: { originalLen: rawKey.length, decryptedLen: (decrypted || "").length },
      });
    }
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    apiKeyStatus.value = "test-failed";
    apiKeyStatusDetail.value = detail;
    devlogApiError(e, { kind: "roundtrip", endpoint: "/api/encrypt-key", deviceId });
  } finally {
    roundtripRunning.value = false;
  }
}

function _goToDevLogs() {
  router.push("/tabs/devlogs");
}

function _handleResetConfig() {
  resetConfig();
}

async function _handleTestConnection() {
  testing.value = true;
  testResult.value = "";
  testResultSuccess.value = false;
  try {
    // 走 /agent-api/* 命名空间转发到 agent 后端 :5245
    // 不走 encv-go 的 /api/agent/test（encv-go 当前没这端点，会 404）
    // 关键：传 deviceId，后端用 deviceId 派生 AES 解密 key
    const did = await getDeviceId();
    const response = await fetch(`/agent-api/test?deviceId=${encodeURIComponent(did)}`, {
      method: "GET",
      headers: { "Content-Type": "application/json", "X-Device-Id": did },
    });
    if (!response.ok) {
      let detail = `HTTP ${response.status}`;
      try {
        const body = await response.text();
        if (body) detail += `: ${body}`;
      } catch {}
      throw new Error(detail);
    }
    const data = await response.json().catch(() => ({}));
    const openaiOk = data.openai === true || data.openai === "ok";
    const openlistOk = data.openlist === true || data.openlist === "ok";
    if (openaiOk && openlistOk) {
      testResultSuccess.value = true;
      testResult.value = t("settings.testConnectionSuccess");
      showToast({ message: t("settings.testConnectionSuccess"), duration: 2000, color: "success" });
    } else {
      const failed: string[] = [];
      if (!openaiOk) failed.push("OpenAI");
      if (!openlistOk) failed.push("OpenList");
      const detail = failed.join(", ") + (data.detail ? `: ${data.detail}` : "");
      testResultSuccess.value = false;
      testResult.value = t("settings.testConnectionFailed", { detail });
      showToast({ message: testResult.value, duration: 3000, color: "danger" });
    }
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    testResultSuccess.value = false;
    testResult.value = t("settings.testConnectionFailed", { detail });
    showToast({ message: testResult.value, duration: 3000, color: "danger" });
  } finally {
    testing.value = false;
  }
}

function _openJsonEditor() {
  const agentVal = getFieldValue(["agent_settings"]);
  jsonText.value = JSON.stringify(agentVal ?? {}, null, 2);
  jsonError.value = "";
  showJsonEditor.value = true;
}

function _validateJson() {
  try {
    JSON.parse(jsonText.value);
    jsonError.value = "";
  } catch (e) {
    jsonError.value = e instanceof Error ? e.message : String(e);
  }
}

async function _handleSaveJson() {
  try {
    const parsed = JSON.parse(jsonText.value);
    const cfg = await fetchConfig();
    (cfg as Record<string, unknown>).agent_settings = parsed;
    await updateConfig(cfg);
    showJsonEditor.value = false;
    showToast({ message: t("settings.configSaved"), duration: 1500, color: "success" });
    await loadConfig();
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    showToast({ message: t("settings.configSaveFailed") + ": " + detail, duration: 3000, color: "danger" });
  }
}

// ─── Task 25: Sync Doctor ────────────────────────────────
// 后端 /api/sync/doctor 返回的 DoctorReport 在前端只读展示
// （用于 bug 报告 / 自检）。不修改任何持久化数据。
const doctorRunning = ref(false);
const doctorReportJson = ref("");
const doctorIssuesCount = ref(0);

async function _handleRunSyncDoctor() {
  if (doctorRunning.value) return;
  doctorRunning.value = true;
  try {
    const report: DoctorReport = await runSyncDoctor();
    // 2 空格缩进，方便用户 / 客服直接复制到 issue
    doctorReportJson.value = JSON.stringify(report, null, 2);
    doctorIssuesCount.value = Array.isArray(report?.issues) ? report.issues.length : 0;
    devlogApiInfo(`sync doctor OK (${doctorIssuesCount.value} issues)`, {
      kind: "sync-doctor",
      endpoint: "/api/sync/doctor",
      extra: { version: report?.version },
    });
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    showToast({ message: t("agent.syncDoctorFailed", { msg: detail }), duration: 3000, color: "danger" });
    devlogApiError(e, { kind: "sync-doctor", endpoint: "/api/sync/doctor" });
  } finally {
    doctorRunning.value = false;
  }
}

async function _handleCopyDoctorJson() {
  if (!doctorReportJson.value) return;
  try {
    if (navigator?.clipboard?.writeText) {
      await navigator.clipboard.writeText(doctorReportJson.value);
    } else {
      // Fallback: 用一个隐藏 textarea + execCommand
      const ta = document.createElement("textarea");
      ta.value = doctorReportJson.value;
      ta.style.position = "fixed";
      ta.style.opacity = "0";
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      document.body.removeChild(ta);
    }
    showToast({ message: t("agent.syncDoctorCopied"), duration: 1500, color: "success" });
  } catch (e) {
    const detail = e instanceof Error ? e.message : String(e);
    showToast({ message: t("agent.syncDoctorCopyFailed") + ": " + detail, duration: 2000, color: "danger" });
    devlogApiError(e, { kind: "sync-doctor-copy", endpoint: "clipboard" });
  }
}

onMounted(async () => {
  // 关键：之前 `if (serverOnline.value)` 同步判断 ref 当时的值，但
  // useServerStatus.checkStatus() 是异步的——onMounted 执行时 checkStatus 还没返回，
  // serverOnline 仍是初始 false → 永远不调 loadConfigSafely → 错误态 UI 永远显示。
  //
  // 修复：主动 await checkStatus 探测一次（即使 useServerStatus 模块单例已 init），
  // 然后才根据探测结果决定是否加载 config。
  try {
    const probe = await checkServerStatusNow();
    if (probe.online) {
      await loadConfigSafely();
    }
    // offline 时错误态 UI 接管，并依赖 watch(serverOnline) 在后续 online 时重试
  } catch (e) {
    console.error("[AgentSettingsDetail] onMounted probe failed:", e instanceof Error ? `${e.name}: ${e.message}` : String(e));
  }
});

// 监听后端连接状态：从未连接 → 已连接时自动重新拉配置，
// 避免"切到 AI 设置时后端刚好没起来 → 页面永远空白"的状态机卡死。
//
// ⚠️ watcher 只在 serverOnline 值变化时触发。如果 serverOnline 已经是 true
//    但 loadConfigSafely 内部 fetchConfig 失败（网络抖动 / Trae proxy 超时），
//    watcher 不会再次触发 → 组件永久卡在错误态。
//    因此需要配合下面的 autoRetryInterval 作为兜底。
watch(serverOnline, async online => {
  if (online && !configLoaded.value) {
    await loadConfigSafely();
  }
});

// ─── 防卡死：定时探测 + 自动重试 ─────────────────────────────
//
// 场景：serverOnline=true 但 fetchConfig 因网络/proxy 问题失败 →
//       watcher 不再触发（值没变）→ 错误态 UI 永久显示。
//       用 5s 定时器兜底：只要还在错误态且未加载成功，就持续重试。
//
let autoRetryTimer: ReturnType<typeof setInterval> | null = null;

function startAutoRetry() {
  stopAutoRetry();
  autoRetryTimer = setInterval(async () => {
    // 只在"已确认后端在线但 config 没加载成功"时重试
    if (serverOnline.value && !configLoaded.value) {
      autoRetryCount.value++;
      console.debug(`[AgentSettingsDetail] auto-retry #${autoRetryCount.value}: serverOnline=true but configLoaded=false, retrying...`);
      await loadConfigSafely();
    }
    // 如果已经加载成功，停止重试
    if (configLoaded.value) {
      stopAutoRetry();
    }
  }, 5000);
}

function stopAutoRetry() {
  if (autoRetryTimer) {
    clearInterval(autoRetryTimer);
    autoRetryTimer = null;
  }
}

// 组件卸载时清理
onBeforeUnmount(() => {
  stopAutoRetry();
});

// 首次进入错误态或加载完成时，启动/停止自动重试
watch(configLoaded, loaded => {
  if (loaded) {
    stopAutoRetry();
  } else if (serverOnline.value) {
    startAutoRetry();
  }
});
watch(configError, err => {
  if (err && serverOnline.value && !configLoaded.value) {
    startAutoRetry();
  }
});

/**
 * 安全的 loadConfig 包装：捕获异常 → 写入 configError → 错误态 UI 自动接管。
 * 之前 loadConfig 抛错会直接冒泡到 onMounted 的 unhandledrejection，
 * configLoaded / configError 都保持初值 → 模板三态全 false → 页面一片空白。
 */
async function loadConfigSafely() {
  configError.value = "";
  try {
    await loadConfig();
    configLoaded.value = true;
    // 解密 API Key 回填到密码框（加密存储 → 解密明文 → password input 自动掩码）
    await decryptAndLoadApiKey();
    // 动态获取模型列表（不阻塞页面渲染）
    fetchSettingsModels();
  } catch (e: any) {
    const detail = e?.message || String(e);
    console.error("[AgentSettingsDetail] loadConfig failed:", detail);
    configError.value = detail;
    configLoaded.value = false;
  }
}

/**
 * 手动重试：用户点击错误态 UI 的"重试"按钮时调用。
 *
 * 之前的实现是个空壳：try/catch 里啥也没有，注释说"useServerStatus 内部已封装好
 * checkStatus"，但根本没调。结果是用户点"重试"按钮后页面纹丝不动 → 误以为崩溃。
 *
 * 修复：先主动触发 useServerStatus.checkStatus() 重新探测（不走缓存），
 * 再根据探测结果决定 loadConfig 或更新错误文案。
 */
async function _retryLoadConfig() {
  // 重试前先主动探测一次后端（避免 serverOnline 缓存还是 false）
  let onlineNow = serverOnline.value;
  try {
    const result = await checkServerStatusNow();
    onlineNow = result?.online === true;
  } catch (e) {
    console.debug("[AgentSettingsDetail] retryLoadConfig checkStatus failed:", e);
    // 探测失败时不改变 onlineNow，沿用缓存值
  }
  if (onlineNow) {
    await loadConfigSafely();
  } else {
    // 后端仍离线，只更新错误文案，等 watch(serverOnline) 自动重试
    configError.value = t("agent.backendOfflineHint") || "请确认 encv-go 服务已启动";
  }
}

function _handleGoToDevLogs() {
  // 跳到 DevLogs tab 方便用户贴日志给客服
  router.push("/tabs/devlogs");
}
</script>

<style scoped>
.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 24px;
  color: var(--encv-text-secondary);
}

/* 🆕 2026-06-15：错误态顶部 ServerStatusCard 容器
   让卡片横向填满但保留最大宽度（避免在 600+px 屏宽时卡片无限拉伸） */
.configErrorCardWrap {
  width: 100%;
  max-width: 480px;
  margin-bottom: 16px;
}

/* 错误态：后端离线 / 配置拉取失败 —— 给用户明确的可执行信息 */
.configErrorContainer {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 48px 24px;
  text-align: center;
  color: var(--encv-text-secondary);
  min-height: 50vh;
}
.configErrorTitle {
  margin: 0 0 8px;
  font-size: 18px;
  font-weight: 600;
  color: var(--ion-text-color, #1f1f1f);
}
.configErrorDetail {
  margin: 0 0 24px;
  font-size: 13px;
  line-height: 1.5;
  max-width: 320px;
  color: var(--ion-color-medium-shade, #6b6c70);
  word-break: break-word;
}
.configErrorRetryBtn,
.configErrorSecondaryBtn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 18px;
  font-size: 13px;
  font-family: inherit;
  font-weight: 500;
  border-radius: 8px;
  cursor: pointer;
  margin-bottom: 8px;
  min-width: 140px;
  justify-content: center;
  transition: background 0.15s, color 0.15s;
}
.configErrorRetryBtn {
  background: var(--ion-color-primary, #4f8cff);
  color: #fff;
  border: 1px solid var(--ion-color-primary, #4f8cff);
}
.configErrorRetryBtn:hover:not(:disabled) {
  background: var(--ion-color-primary-shade, #3a6fd8);
}
.configErrorRetryBtn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.configErrorSecondaryBtn {
  background: transparent;
  color: var(--ion-color-medium-shade, #6b6c70);
  border: 1px solid var(--ion-color-medium, #c8c8cc);
}
.configErrorSecondaryBtn:hover {
  background: rgba(var(--ion-color-medium-rgb, 146, 148, 156), 0.1);
}

.scope-badge {
  font-size: 10px;
  --padding-start: 6px;
  --padding-end: 8px;
  --padding-top: 3px;
  --padding-bottom: 3px;
  border-radius: 10px;
  display: inline-flex;
  align-items: center;
  gap: 3px;
}
.scope-badge-icon {
  font-size: 12px;
}
.scope-synced {
  --background: rgba(var(--ion-color-primary-rgb), 0.12);
  --color: var(--ion-color-primary);
}
@media (max-width: 599px) {
  .scope-badge {
    --padding-start: 5px;
    --padding-end: 5px;
    --padding-top: 2px;
    --padding-bottom: 2px;
  }
  .scope-badge .scope-text {
    display: none;
  }
}

.tools-chip-item {
  --min-height: 0;
  --padding-start: 16px;
}
.tools-chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}
.tool-chip {
  --background: rgba(var(--ion-color-primary-rgb), 0.12);
  --color: var(--ion-color-primary);
  font-size: 12px;
  cursor: pointer;
}
.tool-chip ion-icon {
  margin-left: 4px;
  font-size: 14px;
  opacity: 0.7;
}

.field-description {
  font-size: 12px;
  color: var(--ion-color-medium);
  white-space: normal;
  line-height: 1.4;
}

.test-result-success {
  color: var(--ion-color-success);
  font-size: 12px;
}
.test-result-failed {
  color: var(--ion-color-danger);
  font-size: 12px;
}

.json-editor-content {
  --background: var(--ion-background-color);
}
.json-textarea {
  width: 100%;
  min-height: 400px;
  padding: 12px 16px;
  font-family: 'Courier New', Courier, monospace;
  font-size: 13px;
  line-height: 1.5;
  background: var(--ion-background-color);
  color: var(--ion-text-color);
  border: none;
  outline: none;
  resize: none;
  box-sizing: border-box;
}
.json-error {
  padding: 8px 16px;
  background: rgba(var(--ion-color-danger-rgb), 0.1);
  color: var(--ion-color-danger);
  font-size: 12px;
  font-family: monospace;
}

/* ── 动态模型选择器 ─────────────────────────────────────── */
.model-loading {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 0;
  color: var(--ion-color-medium);
  font-size: 13px;
}
.model-spinner {
  width: 18px;
  height: 18px;
}
.model-error-state,
.model-empty {
  padding: 8px 0;
}
.model-error-text {
  color: var(--ion-color-danger, #eb445a);
  font-size: 12px;
  margin: 0 0 6px;
}
.model-error-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 4px 0 8px;
}
/* "↑ 跳转到 API Key 设置" 按钮（用原生 button 避免 ion-button Shadow DOM
   偶发拦截 @click 冒泡）—— 视觉对齐 Ionic outline 小按钮 */
.model-error-fix-btn {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 5px 10px;
  font-size: 12px;
  font-family: inherit;
  font-weight: 500;
  line-height: 1.4;
  color: var(--ion-color-primary, #4f8cff);
  background: transparent;
  border: 1px solid var(--ion-color-primary, #4f8cff);
  border-radius: 6px;
  cursor: pointer;
  transition: background 0.15s, color 0.15s;
}
.model-error-fix-btn:hover {
  background: rgba(var(--ion-color-primary-rgb), 0.08);
}
.model-error-fix-btn:active {
  background: rgba(var(--ion-color-primary-rgb), 0.16);
}
.model-error-fix-btn:focus-visible {
  outline: 2px solid var(--ion-color-primary, #4f8cff);
  outline-offset: 1px;
}
.model-error-fix-icon {
  font-size: 14px;
  flex-shrink: 0;
}
.api-key-input-wrap {
  position: relative;
}
.api-key-input-wrap.api-key-broken {
  /* 当 API Key 损坏时，输入框周围加红色高亮，提示用户这里有"事故点" */
  outline: 1px solid var(--ion-color-danger, #eb445a);
  outline-offset: -1px;
  border-radius: 6px;
  animation: api-key-broken-pulse 2.4s ease-in-out infinite;
}
@keyframes api-key-broken-pulse {
  0%, 100% { outline-color: var(--ion-color-danger, #eb445a); }
  50% { outline-color: transparent; }
}
.api-key-mask-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 4px 12px 8px;
  color: var(--ion-color-danger, #eb445a);
  font-size: 12px;
  line-height: 1.4;
}
.api-key-mask-icon {
  font-size: 14px;
  flex-shrink: 0;
}
.model-fallback-input {
  width: 100%;
  padding: 8px 10px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.3);
  border-radius: 8px;
  background: var(--ion-background-color);
  color: var(--ion-text-color);
  font-size: 13px;
  font-family: inherit;
  outline: none;
  box-sizing: border-box;
}
.model-fallback-input:focus {
  border-color: var(--ion-color-primary);
}
.model-empty p {
  color: var(--ion-color-medium);
  font-size: 12px;
  margin: 0 0 6px;
}

/* 动态 preset-cards（与 ConfigFieldItem 保持一致，限高滚动） */
.preset-cards {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(80px, 1fr));
  gap: 8px;
  margin-top: 10px;
  width: 100%;
  max-height: 280px;
  overflow-y: auto;
  padding-right: 4px;
}
.preset-card {
  padding: 10px 8px;
  border: 2px solid var(--ion-color-light-shade, #e0e0e0);
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.2s;
  text-align: center;
  background: var(--ion-background-color, transparent);
}
.preset-card-active {
  border-color: var(--ion-color-primary);
  background: rgba(var(--ion-color-primary-rgb), 0.08);
}
.preset-card-title {
  font-weight: 600;
  font-size: 13px;
}
.preset-card-desc {
  font-size: 11px;
  color: var(--ion-color-medium);
  margin-top: 3px;
  line-height: 1.3;
}

/* ── 模型选择器样式（见上方 .preset-cards） ──────────────── */

/* ── API Key 状态徽标 ─────────────────────────── */
.apiKeyStatusItem {
  --min-height: 0;
  --padding-start: 16px;
  --padding-end: 16px;
  --inner-padding-end: 0;
  margin-top: 4px;
}
.apiKeyStatusIcon {
  color: var(--ion-color-medium);
  font-size: 18px;
  align-self: flex-start;
  margin-top: 4px;
}
.apiKeyStatusRow {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.apiKeyStatusBadge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  padding: 4px 8px;
  border-radius: 10px;
}
.apiKeyStatusBadgeIcon {
  font-size: 12px;
}
.apiKeyStatusBadgeText {
  font-weight: 500;
  letter-spacing: 0.2px;
}
.apiKeySpinner {
  width: 14px;
  height: 14px;
}
.apiKeyStatusDetail {
  font-size: 11px;
  color: var(--ion-color-medium);
  font-family: monospace;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 6px 0 0;
  line-height: 1.4;
}
.apiKeyBackendLine {
  font-size: 11px;
  color: var(--ion-color-medium);
  margin: 8px 0 0;
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 4px;
}
.apiKeyBackendLabel {
  font-weight: 500;
}
.apiKeyBackendBase {
  font-family: monospace;
  background: rgba(var(--ion-color-medium-rgb), 0.1);
  padding: 1px 4px;
  border-radius: 3px;
  color: var(--ion-text-color);
}
.apiKeyBackendSource {
  color: var(--ion-color-medium);
  font-style: italic;
}

/* ── Task 25: Sync Doctor 结果块 ───────────────────── */
.doctor-result-item {
  --min-height: 0;
  --padding-start: 16px;
  --padding-end: 16px;
  --inner-padding-end: 0;
}
.doctor-result-wrap {
  width: 100%;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.doctor-result-pre {
  width: 100%;
  max-height: 320px;
  overflow: auto;
  margin: 0;
  padding: 10px 12px;
  border-radius: 8px;
  background: rgba(var(--ion-color-medium-rgb), 0.08);
  color: var(--ion-text-color);
  font-family: 'SF Mono', Menlo, Consolas, 'Courier New', monospace;
  font-size: 11px;
  line-height: 1.5;
  white-space: pre;
  word-break: normal;
  box-sizing: border-box;
}
.doctor-result-actions {
  display: flex;
  justify-content: flex-end;
}

/* ── 诊断面板 ─────────────────────────────────────────── */
.configErrorDiag {
  margin: 12px 0;
  max-width: 400px;
  width: 100%;
  text-align: left;
}
.configErrorDiag summary {
  cursor: pointer;
  font-size: 12px;
  color: var(--ion-color-medium);
  user-select: none;
}
.configErrorDiagPre {
  background: rgba(0, 0, 0, 0.04);
  border-radius: 6px;
  padding: 8px 12px;
  font-size: 11px;
  font-family: monospace;
  white-space: pre-wrap;
  word-break: break-all;
  margin-top: 6px;
  color: var(--ion-color-medium-shade);
}
.retryCount {
  font-size: 11px;
  opacity: 0.7;
  margin-left: 4px;
}
</style>
