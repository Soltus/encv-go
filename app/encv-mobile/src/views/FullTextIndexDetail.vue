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
            <ion-icon :icon="refreshIcon" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <!-- 加载中 -->
      <div v-if="loading && !stats" class="loading-container">
        <ion-spinner name="crescent"></ion-spinner>
        <p>{{ t('settings.loading') || '加载中…' }}</p>
      </div>

      <!-- 索引不可用 -->
      <div v-else-if="!available" class="unavailable-container">
        <ion-icon :icon="warningIcon" class="unavailable-icon"></ion-icon>
        <h2>{{ t('settings.fullTextIndexUnavailable') || '全文索引不可用' }}</h2>
        <p class="error-reason">{{ error || 'FTS5 not initialized' }}</p>
        <p class="hint">
          {{ t('settings.fullTextIndexHint') || '请确保 SQLite FTS5 模块已编译，索引将在 server 启动时自动初始化。' }}
        </p>
      </div>

      <!-- 索引统计 -->
      <template v-else-if="stats">
        <ion-list>
          <ion-list-header>
            <ion-label>{{ t('settings.basicInfo') || '基础信息' }}</ion-label>
          </ion-list-header>
          <ion-item>
            <ion-label>{{ t('settings.dbPath') || '数据库路径' }}</ion-label>
            <ion-note slot="end" class="tokenizer-text">{{ stats.dbPath }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t('settings.fts5Enabled') || 'FTS5 已启用' }}</ion-label>
            <ion-note slot="end">
              <ion-badge :color="stats.fts5Enabled ? 'success' : 'danger'">
                {{ stats.fts5Enabled ? 'YES' : 'NO' }}
              </ion-badge>
            </ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t('settings.tokenizer') || '分词器' }}</ion-label>
            <ion-note slot="end" class="tokenizer-text">{{ stats.tokenizer }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t('settings.indexVersion') || '索引版本' }}</ion-label>
            <ion-note slot="end">v{{ stats.indexVersion }}</ion-note>
          </ion-item>
        </ion-list>

        <ion-list>
          <ion-list-header>
            <ion-label>{{ t('settings.indexStats') || '索引统计' }}</ion-label>
          </ion-list-header>
          <ion-item>
            <ion-label>{{ t('settings.totalFiles') || '文件总数' }}</ion-label>
            <ion-note slot="end" class="stat-number">{{ formatNumber(stats.totalFiles) }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t('settings.totalDirs') || '目录总数' }}</ion-label>
            <ion-note slot="end" class="stat-number">{{ formatNumber(stats.totalDirs) }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>{{ t('settings.totalSize') || '索引大小' }}</ion-label>
            <ion-note slot="end" class="stat-number">{{ formatBytes(stats.totalSize) }}</ion-note>
          </ion-item>
          <ion-item v-if="stats.lastBuildMs > 0">
            <ion-label>{{ t('settings.lastBuildTime') || '上次构建耗时' }}</ion-label>
            <ion-note slot="end">{{ stats.lastBuildMs }} ms</ion-note>
          </ion-item>
          <ion-item v-if="stats.indexedAt">
            <ion-label>{{ t('settings.indexedAt') || '索引时间' }}</ion-label>
            <ion-note slot="end" class="time-text">{{ formatDateTime(stats.indexedAt) }}</ion-note>
          </ion-item>
          <ion-item v-if="stats.isIndexing">
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
import { ref, onMounted } from 'vue'
import { useI18n } from '@/composables/useI18n'
import { refreshOutline, warningOutline } from 'ionicons/icons'
import { getFullTextIndexStats, type FullTextIndexStats } from '@/api/encv_search'
import { formatDateTime } from '@/composables/useDateFormat'

const { t } = useI18n()
const refreshIcon = ref(refreshOutline)
const warningIcon = ref(warningOutline)

const loading = ref(false)
const available = ref(false)
const error = ref<string | null>(null)
const stats = ref<FullTextIndexStats | null>(null)

async function loadStats() {
  loading.value = true
  try {
    const result = await getFullTextIndexStats()
    available.value = result.available
    if (result.available && result.stats) {
      stats.value = result.stats
    } else {
      error.value = result.error || 'unknown'
    }
  } catch (e) {
    available.value = false
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    loading.value = false
  }
}

function formatNumber(n: number): string {
  return n.toLocaleString('en-US')
}

function formatBytes(b: number): string {
  if (b < 1024) return `${b} B`
  if (b < 1024 * 1024) return `${(b / 1024).toFixed(1)} KB`
  if (b < 1024 * 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)} MB`
  return `${(b / 1024 / 1024 / 1024).toFixed(2)} GB`
}

onMounted(() => {
  loadStats()
})
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
</style>
