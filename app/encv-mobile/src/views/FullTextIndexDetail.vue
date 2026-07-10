<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>{{ t('settings.fullTextIndex') || '全文索引' }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="loadStats" :disabled="loading">
            <ion-icon :icon="refreshOutline" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div v-if="renderError" class="unavailable-container render-error">
        <ion-icon :icon="bugOutline" class="unavailable-icon" color="danger"></ion-icon>
        <h2>{{ t('settings.renderErrorTitle') || '组件渲染错误' }}</h2>
        <p class="error-reason">{{ renderError.message || String(renderError) }}</p>
        <pre v-if="renderError.stack" class="render-error-stack">{{ renderError.stack }}</pre>
        <p class="hint">
          {{ t('settings.renderErrorHint') || '此错误已被自动上报到错误捕获系统。点击下方按钮重新加载页面。' }}
        </p>
        <ion-button @click="reloadPage" fill="outline" color="primary" class="render-error-reload">
          {{ t('common.reload') || '重新加载' }}
        </ion-button>
      </div>

      <div v-else-if="loading" class="loading-container">
        <ion-spinner name="crescent"></ion-spinner>
        <p>{{ t('settings.loading') || '加载中…' }}</p>
      </div>

      <div v-else-if="!available" class="unavailable-container">
        <ion-icon :icon="warningOutline" class="unavailable-icon"></ion-icon>
        <h2>{{ t('settings.fullTextIndexUnavailable') || '全文索引不可用' }}</h2>
        <p class="error-reason">{{ error || 'FTS5 not initialized' }}</p>
        <p class="hint">
          {{ t('settings.fullTextIndexHint') || '请确保 SQLite FTS5 模块已编译，索引将在 server 启动时自动初始化。' }}
        </p>
      </div>

      <template v-else>
        <ion-list>
          <ion-list-header>
            <ion-label>{{ t('settings.basicInfo') || '基础信息' }}</ion-label>
          </ion-list-header>
          <ion-item>
            <ion-label>{{ t('settings.dbPath') || '数据库路径' }}</ion-label>
            <ion-note slot="end" class="tokenizer-text">{{ stats?.dbPath || '-' }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t('settings.fts5Enabled') || 'FTS5 已启用' }}</ion-label>
            <ion-note slot="end">
              <ion-badge :color="stats?.fts5Enabled ? 'success' : 'danger'">
                {{ stats?.fts5Enabled ? 'YES' : 'NO' }}
              </ion-badge>
            </ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t('settings.tokenizer') || '分词器' }}</ion-label>
            <ion-note slot="end" class="tokenizer-text">{{ stats?.tokenizer || '-' }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t('settings.indexVersion') || '索引版本' }}</ion-label>
            <ion-note slot="end">v{{ stats?.indexVersion ?? 0 }}</ion-note>
          </ion-item>
        </ion-list>

        <!-- 🆕 2026-07-03：FTS 索引重建任务卡片（spec fts-rebuild-task）-->
        <ion-list>
          <ion-list-header>
            <ion-label>{{ t('settings.rebuildIndex') || '索引重建' }}</ion-label>
          </ion-list-header>
          <ion-item v-if="!rebuildTask" data-testid="rebuild-idle-item" class="rebuild-idle">
            <ion-label class="ion-text-wrap">
              <h3>{{ t('settings.rebuildAction') || '重建全文索引' }}</h3>
              <p class="rebuild-hint">
                {{ t('settings.rebuildHint') || '扫描所有文件并重新构建 FTS5 索引。任务走系统队列，自带进度和耗时。' }}
              </p>
            </ion-label>
            <ion-button
              slot="end"
              fill="solid"
              color="primary"
              data-testid="rebuild-trigger-btn"
              :disabled="rebuildSubmitting"
              @click="triggerRebuild"
            >
              <ion-icon :icon="refreshOutline" slot="start"></ion-icon>
              {{ t('common.rebuild') || '重建' }}
            </ion-button>
          </ion-item>

          <!-- 任务进行中卡片 -->
          <ion-item v-else data-testid="rebuild-active-item" class="rebuild-active">
            <ion-label class="ion-text-wrap">
              <h3>
                <ion-spinner v-if="rebuildTask.status === 'running' || rebuildTask.status === 'queued'" name="crescent" class="rebuild-spinner"></ion-spinner>
                <span data-testid="rebuild-status-label">{{ rebuildStatusLabel }}</span>
              </h3>
              <p v-if="rebuildTask.phase" data-testid="rebuild-phase" class="rebuild-phase">{{ rebuildTask.phase }}</p>
              <ion-progress-bar
                data-testid="rebuild-progress-bar"
                :value="(rebuildTask.progress || 0) / 100"
                :color="rebuildTask.status === 'failed' ? 'danger' : 'primary'"
                class="rebuild-progress"
              ></ion-progress-bar>
              <p class="rebuild-meta">
                <span v-if="rebuildTask.progress !== undefined" data-testid="rebuild-progress-text">{{ rebuildTask.progress }}%</span>
                <span v-if="rebuildTask.speed" class="rebuild-speed">{{ rebuildTask.speed }}</span>
                <span v-if="rebuildTask.eta" class="rebuild-eta">ETA: {{ rebuildTask.eta }}</span>
              </p>
              <p v-if="rebuildTask.error" data-testid="rebuild-error" class="rebuild-error">{{ rebuildTask.error }}</p>
            </ion-label>
            <ion-button
              v-if="rebuildTask.status === 'running' || rebuildTask.status === 'queued'"
              slot="end"
              fill="clear"
              color="danger"
              data-testid="rebuild-cancel-btn"
              @click="cancelRebuild"
            >
              {{ t('common.cancel') || '取消' }}
            </ion-button>
            <ion-button
              v-else
              slot="end"
              fill="clear"
              color="medium"
              data-testid="rebuild-dismiss-btn"
              @click="dismissRebuildCard"
            >
              {{ t('common.dismiss') || '关闭' }}
            </ion-button>
          </ion-item>
        </ion-list>

        <ion-list>
          <ion-list-header>
            <ion-label>{{ t('settings.indexStats') || '索引统计' }}</ion-label>
          </ion-list-header>
          <ion-item>
            <ion-label>{{ t('settings.totalFiles') || '文件总数' }}</ion-label>
            <ion-note slot="end" class="stat-number">{{ formatNumber(stats?.totalFiles ?? 0) }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t('settings.totalDirs') || '目录总数' }}</ion-label>
            <ion-note slot="end" class="stat-number">{{ formatNumber(stats?.totalDirs ?? 0) }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t('settings.totalSize') || '索引大小' }}</ion-label>
            <ion-note slot="end" class="stat-number">{{ formatBytes(stats?.totalSize ?? 0) }}</ion-note>
          </ion-item>
          <ion-item v-if="stats?.lastBuildMs && stats.lastBuildMs > 0">
            <ion-label>{{ t('settings.lastBuildTime') || '上次构建耗时' }}</ion-label>
            <ion-note slot="end">{{ stats.lastBuildMs }} ms</ion-note>
          </ion-item>
          <ion-item v-if="stats?.indexedAt">
            <ion-label>{{ t('settings.indexedAt') || '索引时间' }}</ion-label>
            <ion-note slot="end" class="time-text">{{ formatDateTime(stats.indexedAt) }}</ion-note>
          </ion-item>
          <ion-item v-if="stats?.isIndexing">
            <ion-label>{{ t('settings.isIndexing') || '正在索引' }}</ion-label>
            <ion-note slot="end">
              <ion-spinner name="dots" class="indexing-spinner"></ion-spinner>
            </ion-note>
          </ion-item>
        </ion-list>

        <ion-list>
          <ion-list-header>
            <ion-label>{{ t('settings.performanceBench') || '性能基准' }}</ion-label>
          </ion-list-header>
          <ion-item>
            <ion-label class="ion-text-wrap">
              <h3>100w 条目搜索 1000 结果</h3>
              <p class="bench-desc">SQLite FTS5 + bm25 + CJK bigram</p>
            </ion-label>
            <ion-note slot="end" class="bench-number">≤ 500ms</ion-note>
          </ion-item>
          <ion-item>
            <ion-label class="ion-text-wrap">
              <h3>1w 条目 BulkInsert</h3>
              <p class="bench-desc">事务批量 + prepared stmt</p>
            </ion-label>
            <ion-note slot="end" class="bench-number">≤ 1s</ion-note>
          </ion-item>
          <ion-item>
            <ion-label class="ion-text-wrap">
              <h3>100w 条目 BulkInsert</h3>
              <p class="bench-desc">WAL 模式 + 索引同步</p>
            </ion-label>
            <ion-note slot="end" class="bench-number">≤ 90s</ion-note>
          </ion-item>
        </ion-list>

        <ion-list>
          <ion-list-header>
            <ion-label>{{ t('settings.supportedSyntax') || '支持的查询语法' }}</ion-label>
          </ion-list-header>
          <ion-item>
            <ion-label class="ion-text-wrap">
              <code>word</code> 空格分隔（隐式 AND）
            </ion-label>
          </ion-item>
          <ion-item>
            <ion-label class="ion-text-wrap">
              <code>word1 AND word2</code> 必须大写
            </ion-label>
          </ion-item>
          <ion-item>
            <ion-label class="ion-text-wrap">
              <code>word1 OR word2</code> 任一匹配
            </ion-label>
          </ion-item>
          <ion-item>
            <ion-label class="ion-text-wrap">
              <code>word1 NOT word2</code> 排除（Go 端 substring 过滤）
            </ion-label>
          </ion-item>
          <ion-item>
            <ion-label class="ion-text-wrap">
              <code>"exact phrase"</code> 双引号短语
            </ion-label>
          </ion-item>
          <ion-item>
            <ion-label class="ion-text-wrap">
              <code>regex:^pattern</code> 或 <code>regex:/^p/</code> 正则
            </ion-label>
          </ion-item>
          <ion-item>
            <ion-label class="ion-text-wrap">
              <code>\\</code> 转义下一个特殊字符
            </ion-label>
          </ion-item>
        </ion-list>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { bugOutline, refreshOutline, warningOutline } from "ionicons/icons";
import { formatDateTime } from "@/composables/useDateFormat";

// 🆕 2026-07-03 修复 classList 错误：必须显式 import Ionic 组件
//   根因（cypress e2e DOM log 确认）：未显式 import 时，<ion-page> 标签未被 Vue 编译器
//   识别为 Ionic Vue 组件，渲染成原生 <ION-PAGE> 自闭合元素，缺失 .ion-page class
//   和 z-index 样式，导致页面被前一个 CacheDetail（z-index:101）覆盖。
//   对比 ServerDetail.vue / DatabaseDetail.vue / CacheDetail.vue 都显式 import。

import { computed, onErrorCaptured, onMounted, onUnmounted, ref } from "vue";
import { getApiBaseUrl } from "@/api/encv_core";
import { type FullTextIndexStats, getFullTextIndexStats, rebuildFullTextIndex } from "@/api/encv_search";
import { errorStore } from "@/composables/useErrorCapture";
import { useI18n } from "@/composables/useI18n";
import { useTaskEventBridge } from "@/composables/useTaskEventBridge";

const { t } = useI18n();

const loading = ref(false);
const available = ref(false);
const error = ref<string | null>(null);
const stats = ref<FullTextIndexStats | null>(null);

// 🆕 2026-07-03：FTS 索引重建任务状态（spec fts-rebuild-task）
interface RebuildTaskState {
  id: string;
  status: string; // queued / running / cancelling / completed / failed / cancelled
  progress?: number;
  phase?: string;
  speed?: string;
  eta?: string;
  error?: string;
}
const rebuildTask = ref<RebuildTaskState | null>(null);
const rebuildSubmitting = ref(false);

const rebuildStatusLabel = computed(() => {
  if (!rebuildTask.value) return "";
  const s = rebuildTask.value.status;
  if (s === "queued") return t("settings.rebuildQueued") || "重建任务排队中";
  if (s === "running") return t("settings.rebuildRunning") || "正在重建索引";
  if (s === "cancelling") return t("settings.rebuildCancelling") || "正在取消";
  if (s === "completed") return t("settings.rebuildCompleted") || "重建完成";
  if (s === "failed") return t("settings.rebuildFailed") || "重建失败";
  if (s === "cancelled") return t("settings.rebuildCancelled") || "已取消";
  return s;
});

// 🆕 2026-07-02：防止卸载后异步回调继续更新状态
//   虽然不会直接导致 classList 错误，但这是防御性编程最佳实践
let isMounted = true;

// 🆕 A5：渲染错误捕获（onErrorCaptured 兜底，把"更底层错误"显式显示给用户）
const renderError = ref<Error | null>(null);
onErrorCaptured((err: unknown) => {
  const e = err instanceof Error ? err : new Error(String(err));
  renderError.value = e;
  errorStore.addError({
    source: "vue",
    message: e.message,
    stack: e.stack,
    componentName: "FullTextIndexDetail",
    url: typeof window !== "undefined" ? window.location.pathname : undefined,
  });
  return false;
});

// 🆕 2026-07-03：订阅任务 4 件套 WS 事件（automation-workflow 规则 §二）
//   只关心 rebuild_fts_index 任务的进度，过滤其他任务的事件
useTaskEventBridge({
  onProgress: data => {
    if (!rebuildTask.value || data.id !== rebuildTask.value.id) return;
    if (!isMounted) return;
    rebuildTask.value = {
      ...rebuildTask.value,
      progress: data.progress,
      phase: data.phase,
      speed: data.speed,
      eta: data.eta,
      status: "running",
    };
  },
  onUpdate: data => {
    if (!rebuildTask.value || data.id !== rebuildTask.value.id) return;
    if (!isMounted) return;
    rebuildTask.value = {
      ...rebuildTask.value,
      status: data.status,
      progress: data.progress,
    };
  },
  onComplete: data => {
    if (!rebuildTask.value || data.id !== rebuildTask.value.id) return;
    if (!isMounted) return;
    const status = data.error ? "failed" : "completed";
    rebuildTask.value = {
      ...rebuildTask.value,
      status,
      error: data.error,
      progress: 100,
    };
    // 重建完成后刷新 stats（显示新的 totalFiles 等）
    if (!data.error) {
      setTimeout(() => loadStats(), 500);
    }
  },
});

function reloadPage() {
  if (typeof window !== "undefined") {
    window.location.reload();
  }
}

async function loadStats() {
  loading.value = true;
  try {
    const result = await getFullTextIndexStats();
    // 🆕 2026-07-02：组件卸载后不再更新状态（防止竞态条件）
    if (!isMounted) return;
    available.value = result.available;
    if (result.available && result.stats) {
      stats.value = result.stats;
    } else {
      error.value = result.error || "unknown";
    }
  } catch (e) {
    if (!isMounted) return;
    available.value = false;
    error.value = e instanceof Error ? e.message : String(e);
    // 抛到全局 errorStore（让 A5 浮窗也显示）
    errorStore.addError({
      source: "console",
      message: `FullTextIndexDetail: ${error.value}`,
      stack: e instanceof Error ? e.stack : undefined,
      url: typeof window !== "undefined" ? window.location.pathname : undefined,
    });
  } finally {
    if (isMounted) {
      loading.value = false;
    }
  }
}

// 🆕 2026-07-03：触发 FTS 索引重建
async function triggerRebuild() {
  if (rebuildSubmitting.value) return;
  rebuildSubmitting.value = true;
  try {
    const resp = await rebuildFullTextIndex();
    if (!isMounted) return;
    rebuildTask.value = {
      id: resp.taskId,
      status: resp.status,
      progress: 0,
    };
  } catch (e: any) {
    if (!isMounted) return;
    const errMsg = e?.message || String(e);
    // 409 Conflict：已有重建任务在跑，复用其 taskId
    if (e?.code === "REBUILD_IN_PROGRESS" && e?.taskId) {
      rebuildTask.value = {
        id: e.taskId,
        status: e.status || "running",
        progress: 0,
      };
    } else {
      errorStore.addError({
        source: "console",
        message: `FTS rebuild trigger failed: ${errMsg}`,
        url: typeof window !== "undefined" ? window.location.pathname : undefined,
      });
    }
  } finally {
    if (isMounted) rebuildSubmitting.value = false;
  }
}

// 🆕 2026-07-03：取消重建任务
async function cancelRebuild() {
  if (!rebuildTask.value) return;
  try {
    const baseUrl = getApiBaseUrl();
    await fetch(`${baseUrl}/api/tasks/${rebuildTask.value.id}/cancel`, { method: "POST" });
    if (!isMounted) return;
    rebuildTask.value = {
      ...rebuildTask.value,
      status: "cancelling",
    };
  } catch (_e) {
    // 取消失败不阻断，用户可重试
  }
}

// 🆕 2026-07-03：关闭重建卡片（终态后）
function dismissRebuildCard() {
  rebuildTask.value = null;
}

function formatNumber(n: number): string {
  return n.toLocaleString("en-US");
}

function formatBytes(b: number): string {
  if (b < 1024) return `${b} B`;
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`;
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`;
  return `${(b / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

onMounted(() => {
  loadStats();
});

onUnmounted(() => {
  isMounted = false;
});
</script>

<style scoped>
.loading-container,
.unavailable-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  text-align: center;
}

.unavailable-icon {
  font-size: 64px;
  color: var(--ion-color-warning);
  margin-bottom: 16px;
}

.error-reason {
  color: var(--ion-color-danger);
  font-family: monospace;
  font-size: 0.9em;
  margin: 8px 0;
}

.hint {
  color: var(--ion-color-medium);
  font-size: 0.9em;
  max-width: 360px;
}

.stat-number {
  font-family: 'SFMono-Regular', Consolas, monospace;
  font-weight: 600;
}

.bench-number {
  font-family: 'SFMono-Regular', Consolas, monospace;
  font-weight: 700;
  color: var(--ion-color-success);
}

.bench-desc {
  font-size: 0.8em;
  color: var(--ion-color-medium);
}

.secondary-note {
  color: var(--ion-color-medium);
  font-size: 0.85em;
  margin-left: 4px;
}

.tokenizer-text,
.time-text {
  font-family: 'SFMono-Regular', Consolas, monospace;
  font-size: 0.85em;
}

.indexing-spinner {
  width: 20px;
  height: 20px;
}

/* 🆕 2026-07-03：FTS 索引重建任务卡片样式 */
.rebuild-idle .rebuild-hint {
  font-size: 0.85em;
  color: var(--ion-color-medium);
  margin-top: 4px;
}

.rebuild-active h3 {
  display: flex;
  align-items: center;
  gap: 8px;
}

.rebuild-spinner {
  width: 16px;
  height: 16px;
}

.rebuild-phase {
  font-size: 0.85em;
  color: var(--ion-color-primary);
  margin: 4px 0;
  font-family: 'SFMono-Regular', Consolas, monospace;
}

.rebuild-progress {
  margin: 8px 0;
  height: 6px;
  border-radius: 3px;
}

.rebuild-meta {
  display: flex;
  gap: 12px;
  font-size: 0.8em;
  color: var(--ion-color-medium);
  margin: 4px 0;
}

.rebuild-meta > span:first-child {
  font-weight: 600;
  color: var(--ion-color-dark);
}

.rebuild-speed,
.rebuild-eta {
  font-family: 'SFMono-Regular', Consolas, monospace;
}

.rebuild-error {
  color: var(--ion-color-danger);
  font-size: 0.85em;
  margin: 4px 0;
  word-break: break-word;
}

/* 🆕 2026-07-02 A5：渲染错误卡片样式 */
.render-error .error-reason {
  color: var(--ion-color-danger);
  font-family: monospace;
  font-size: 0.95em;
  margin: 8px 0;
  word-break: break-all;
  white-space: pre-wrap;
  max-width: 90vw;
}

.render-error-stack {
  max-width: 90vw;
  max-height: 30vh;
  overflow-y: auto;
  font-family: monospace;
  font-size: 0.75em;
  background: rgba(var(--ion-color-danger-rgb), 0.08);
  border: 1px solid rgba(var(--ion-color-danger-rgb), 0.2);
  border-radius: 6px;
  padding: 8px 12px;
  text-align: left;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--ion-color-danger-shade, #b00020);
}

.render-error-reload {
  margin-top: 12px;
}
</style>
