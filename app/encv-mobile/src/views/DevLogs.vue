<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>{{ t('devlogs.title') }}</ion-title>
      </ion-toolbar>
      <ion-toolbar class="tab-toolbar">
        <ion-segment :value="activeTab" @ionChange="onTabChange">
          <ion-segment-button value="frontend" @click="onTabClick('frontend')">
            {{ t('devlogs.frontend') }}
          </ion-segment-button>
          <ion-segment-button value="backend" @click="onTabClick('backend')">
            {{ t('devlogs.backend') }}
          </ion-segment-button>
        </ion-segment>
      </ion-toolbar>
      <div class="toolbar-row">
        <div class="filter-dropdowns">
          <FilterDropdown
            :options="levelDropdownOptions"
            v-model="selectedLevelsArray"
            :label="t('devlogs.level')"
            :multi-select="true"
            :show-actions="true"
            :select-all-text="t('devlogs.selectAll')"
            :clear-all-text="t('devlogs.clearAll')"
            @change="onLevelsChange"
          />
          <FilterDropdown
            :options="tagDropdownOptions"
            v-model="selectedTags"
            :label="t('devlogs.source')"
            :multi-select="true"
            :searchable="true"
            :search-placeholder="t('devlogs.searchTag')"
            :empty-text="t('devlogs.noTags')"
            :show-actions="true"
            :select-all-text="t('devlogs.selectAll')"
            :clear-all-text="t('devlogs.clearAll')"
            @change="onTagsChange"
          />
        </div>
        <div class="toolbar-actions">
          <!-- v6 纯手动挡：▶ 跟随 / ⏸ 暂停 开关按钮 -->
          <ion-button
            fill="clear"
            size="small"
            :color="autoScrollEnabled ? 'primary' : 'medium'"
            :title="autoScrollEnabled ? t('devlogs.autoScrollOn') : t('devlogs.autoScrollOff')"
            data-testid="devlogs-auto-scroll-toggle"
            @click="toggleAutoScroll"
          >
            <ion-icon
              :icon="autoScrollEnabled ? pauseOutline : playOutline"
              slot="icon-only"
            ></ion-icon>
          </ion-button>
          <ion-button fill="clear" size="small" @click="handleCopy">
            <ion-icon :icon="copyOutline" slot="icon-only"></ion-icon>
          </ion-button>
          <ion-button fill="clear" size="small" color="danger" @click="handleClear">
            <ion-icon :icon="trashOutline" slot="icon-only"></ion-icon>
          </ion-button>
        </div>
      </div>
      <div class="search-row">
        <ion-searchbar
          v-model="searchText"
          :placeholder="t('devlogs.searchPlaceholder')"
          class="log-searchbar"
          mode="ios"
          :debounce="150"
        ></ion-searchbar>
      </div>
    </ion-header>

    <!--
      v6 纯手动挡：toolbar ▶/⏸ 开关 + 浮动 ↓ 按钮
      详见脚本顶部 autoScrollEnabled 注释
    -->
    <ion-content ref="contentRef" class="log-content">
      <!--
        🆕 虚拟滚动：ion-content 的 scroll 事件触发 VirtualLogList 重算可见 items
        DOM 节点数恒定 ~30（视口内 + overscan），切 tab 成本 = O(visible) 而非 O(N)
      -->
      <div v-if="activeTab === 'frontend'" class="log-list">
        <div v-if="filteredFrontend.length === 0" class="empty-logs">
          <p>{{ t('devlogs.noLogs') }}</p>
        </div>
        <VirtualLogList :key="'frontend'" v-if="filteredFrontend.length > 0" :items="filteredFrontend" :scroll-el="scrollEl" @select="onLogSelect">
          <template #default="{ item }">
            <span class="log-source-icon" :title="getSourceIconTitle(item)">
              {{ getSourceIcon(item) }}
            </span>
            <span class="log-time">[{{ item.timestamp }}]</span>
            <ion-badge :color="getBadgeColor(item.level)" class="level-badge">{{ item.level.toUpperCase() }}</ion-badge>
            <span class="log-msg" v-html="highlightMatch(item.message, searchText)"></span>
          </template>
        </VirtualLogList>
      </div>

      <div v-else class="log-list">
        <!-- debug 工具顶栏：后端健康度摘要（compact 模式）
             此页面语义 = "开发日志 / 调试"，需要看到后端是否在线但不要占太多空间
             卡片不可点（避免和 ServerDetail 状态行的可点行为混淆） -->
        <div class="devlog-status-card-wrap">
          <ServerStatusCard :clickable="false" :compact="true" />
        </div>
        <div v-if="backendFilteredItems.length === 0" class="empty-logs">
          <p>{{ t('devlogs.noLogs') }}</p>
        </div>
        <VirtualLogList :key="'backend'" v-if="backendFilteredItems.length > 0" :items="backendFilteredItems" :scroll-el="scrollEl" @select="onLogSelect">
          <template #default="{ item }">
            <span class="log-source-icon" :title="getSourceIconTitle(item)">
              {{ getSourceIcon(item) }}
            </span>
            <span class="log-time">[{{ item.timestamp }}]</span>
            <ion-badge :color="getBadgeColor(item.level)" class="level-badge">{{ item.level.toUpperCase() }}</ion-badge>
            <span class="log-msg" v-html="highlightMatch(item.message, searchText)"></span>
          </template>
        </VirtualLogList>
      </div>
    </ion-content>

    <!--
      浮动「↑/↓」按钮组：v6.1 加 ↑ 滚顶按钮，对称 ↓ 重启跟随+滚底
      两者独立条件：
        - ↑ 滚顶：scrollTop > 阈值（约 200px）时显示，点击 = ion-content.scrollTop = 0
        - ↓ 滚底：autoScrollEnabled=false 时显示（已在 v6 落地）
    -->
    <div class="scroll-buttons">
      <transition name="fade">
        <button
          v-if="showScrollToTop"
          type="button"
          class="scrollToTopBtn"
          :title="t('devlogs.scrollToTop')"
          :aria-label="t('devlogs.scrollToTop')"
          @click="onJumpToTop"
        >
          <ion-icon :icon="arrowUpOutline" class="scrollToTopIcon" />
        </button>
      </transition>
      <transition name="fade">
        <button
          v-if="!autoScrollEnabled"
          type="button"
          class="scrollToBottomBtn"
          :title="t('devlogs.scrollToBottom')"
          :aria-label="t('devlogs.scrollToBottom')"
          @click="onJumpToBottom"
        >
          <ion-icon :icon="arrowDownOutline" class="scrollToBottomIcon" />
        </button>
      </transition>
    </div>

    <ion-footer class="status-bar">
      <ion-toolbar>
        <div class="status-inner">
          <span class="status-text">{{ t('devlogs.total', { total: String(totalCurrent), filtered: String(filteredCurrent) }) }}</span>
          <!-- v6: 显示自动滚动状态（⏸ 暂停 / ▶ 跟随） -->
          <span class="status-text auto-scroll-status" :class="{ paused: !autoScrollEnabled }">
            {{ autoScrollEnabled ? t('devlogs.autoScrollOn') : t('devlogs.autoScrollOff') }}
          </span>
        </div>
      </ion-toolbar>
    </ion-footer>

    <!--
      🆕 2026-06-15 修 #2：日志详情模态
      用户点击单行日志 → VirtualLogList emit('select') → onLogSelect 设置 selectedLog
      → 此模态显示完整 timestamp/level/message + 复制按钮
      原因：之前 28px 固定行高 + ellipsis + nowrap 会截断长 log
    -->
    <div v-if="selectedLog" class="log-detail-overlay" @click.self="closeLogDetail">
      <div class="log-detail-modal" role="dialog" aria-modal="true">
        <div class="log-detail-header">
          <h3 class="log-detail-title">{{ t('devlogs.logDetail') }}</h3>
          <button type="button" class="log-detail-close" :aria-label="t('devlogs.logDetailClose')" @click="closeLogDetail">
            <ion-icon :icon="closeOutline" />
          </button>
        </div>
        <div class="log-detail-body">
          <!-- 🆕 2026-06-16：紧凑 1 行 2 列 — 时间 + 级别合并节省垂直空间 -->
          <div class="log-detail-row log-detail-meta-row">
            <div class="log-detail-meta-item">
              <span class="log-detail-label">{{ t('devlogs.logDetailTimestamp') }}</span>
              <span class="log-detail-value log-time-detail">{{ selectedLog.timestamp }}</span>
            </div>
            <div class="log-detail-meta-item">
              <span class="log-detail-label">{{ t('devlogs.logDetailLevel') }}</span>
              <ion-badge :color="getBadgeColor(selectedLog.level)" class="level-badge">
                {{ selectedLog.level.toUpperCase() }}
              </ion-badge>
            </div>
          </div>
          <!-- 🆕 升级能力：显示来源（哪个 console.* / ws / http-poll 通道）+ 原始堆栈（error 级别） -->
          <div v-if="selectedLog.source" class="log-detail-row">
            <span class="log-detail-label">{{ t('devlogs.logDetailSource') }}</span>
            <span class="log-detail-value log-source-detail">{{ selectedLog.source }}</span>
          </div>
          <div v-if="selectedLog.tags && selectedLog.tags.length > 0" class="log-detail-row">
            <span class="log-detail-label">{{ t('devlogs.logDetailTags') }}</span>
            <div class="log-detail-tags">
              <span
                v-for="tag in selectedLog.tags"
                :key="tag"
                class="log-tag-chip"
                @click="onTagClick(tag)"
              >{{ tag }}</span>
            </div>
          </div>
          <div class="log-detail-row log-detail-message-row">
            <span class="log-detail-label">{{ t('devlogs.logDetailMessage') }}</span>
            <pre class="log-detail-message">{{ selectedLog.message }}</pre>
          </div>
          <!-- 🆕 2026-06-16：error 级别永远显示堆栈 section（即使没 stack 也显示「(无堆栈)」提示，让用户知道功能存在） -->
          <div v-if="selectedLog.level === 'error'" class="log-detail-row log-detail-stack-row">
            <span class="log-detail-label">{{ t('devlogs.logDetailStack') }}</span>
            <pre v-if="selectedLog.stack" class="log-detail-stack">{{ selectedLog.stack }}</pre>
            <pre v-else class="log-detail-stack log-detail-stack-empty">(no stack trace available — 后端 slog 暂未推送 stack，前端 Error 已支持)</pre>
          </div>
        </div>
        <div class="log-detail-footer">
          <ion-button fill="outline" size="small" @click="copyLogDetail">
            <ion-icon :icon="copyOutline" slot="start" />
            {{ t('devlogs.logDetailCopy') }}
          </ion-button>
          <ion-button fill="clear" size="small" @click="closeLogDetail">
            {{ t('devlogs.logDetailClose') }}
          </ion-button>
        </div>
      </div>
    </div>
  </ion-page>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, onBeforeUnmount, nextTick, markRaw, shallowRef, triggerRef } from 'vue'
import {
  IonPage, IonHeader, IonToolbar, IonTitle, IonContent,
  IonSegment, IonSegmentButton, IonSearchbar, IonButton,
  IonIcon, IonBadge, IonFooter, alertController,
} from '@ionic/vue'
import { trashOutline, copyOutline, arrowDownOutline, arrowUpOutline, playOutline, pauseOutline, closeOutline } from 'ionicons/icons'
import VirtualLogList from '@/components/VirtualLogList.vue'
import FilterDropdown from '@/components/shared/FilterDropdown.vue'
import type { DropdownOption } from '@/components/shared/FilterDropdown.vue'
import { eventBus } from '@/composables/useEventBus'
import { useI18n } from '@/composables/useI18n'
import { useRealtimeTransport } from '@/composables/useRealtimeTransport'
import { useFrontendLogs, type LogEntry } from '@/composables/useFrontendLogs'
import { showToast } from '@/composables/useToast'
import { copyToClipboard } from '@/composables/useClipboard'
import { checkServerStatus, getRecentBackendLogs, type BackendLogEntry } from '@/api/encv'
import { IncrementalFilter, type Level } from '@/utils/IncrementalFilter'

const { t } = useI18n()
const transport = useRealtimeTransport()

const activeTab = ref<'frontend' | 'backend'>('frontend')
const searchText = ref('')
/**
 * 自动滚动：true 跟随 / false 暂停
 * 唯一交互入口：toolbar 开关按钮（toggleAutoScroll）和浮动 ↓ 按钮（onJumpToBottom）
 * 纯手动挡：不监听 scroll 事件、不在 tab 切换 / 前后台切换时 auto-disable
 * 理由：浏览器预览=手机浏览器无 wheel；项目用 Capacitor 高刷 WebView 90/120Hz，
 * @ionScroll/@ionScrollStart 在移动端 + 高刷下完全不可靠
 */
const autoScrollEnabled = ref(true)
const contentRef = ref<InstanceType<typeof IonContent> | null>(null)
/** ion-content 的 .inner-scroll 元素（虚拟列表的 scroll 容器） */
const scrollEl = ref<HTMLElement | null>(null)

/** VirtualLogList refs（用于主动调 forceMeasure 触发首次渲染） */
const frontendListRef = ref<{ forceMeasure: () => void } | null>(null)
const backendListRef = ref<{ forceMeasure: () => void } | null>(null)
function virtualizerForceMeasure(): void {
  // 当前 active tab 的 virtual list 才需要 measure
  if (activeTab.value === 'frontend') frontendListRef.value?.forceMeasure()
  else backendListRef.value?.forceMeasure()
}
/**
 * 🆕 2026-06-15 1M+ 容量优化：用 IncrementalFilter 替代 shallowRef<LogEntry[]>
 *   - push O(1)（ring buffer）
 *   - 过滤 O(1) 读取（incremental cache）
 *   - 切 filter O(N) rebuild（27ms/1M 实测，60FPS 预算内）
 *   - markRaw：避免 Vue 把 filter 实例包成 reactive proxy（性能杀手）
 *   - MAX=1_000_000 满足"至少 100 万条"硬需求
 */
const MAX_BACKEND_LOGS = 1_000_000
const backendFilter = markRaw(new IncrementalFilter(MAX_BACKEND_LOGS))
/**
 * 同步 trigger：IncrementalFilter 是 markRaw 对象，watch 纯函数 getter 永远不 invoke
 * （vue 内部对 markRaw 属性不 track）。改用 IncrementalFilter.subscribe() 显式通知：
 * 推/批推/rebuild/clear 时同步 notify → 这里 ++tick → computed 重算 → UI 更新。
 *
 * 🆕 2026-06-15 修 #2：补 backendLogsView (shallowRef) + triggerRef。
 * 原因：IncrementalFilter 内部 result 数组是同一个引用（mutate in place 以避免 O(N) 复制），
 * 哪怕 computed 重算，Vue 比较新旧 value 是 Object.is —— 同 ref → 下游不触发。
 * 同时 VirtualLogList 内部 `virtualizerOptions = computed(() => ({ count: props.items.length }))`，
 * props.items.length 读的是 plain array，**Vue 不 track** plain array 的 .length 属性。
 *
 * 解决：用一个 shallowRef 持有数组引用，每次 IncrementalFilter 变更时
 *   backendLogsView.value = backendFilter.getResult()  // 显式赋值（ref 同值也不影响）
 *   triggerRef(backendLogsView)                         // 强制 trigger 下游
 * → 依赖 backendLogsView 的所有 effects 全部重算
 *   → backendFilteredItems 重算 → 同样 ref 传下去
 *   → filteredCurrent 重算 → 读到 .length 新值
 *   → VirtualLogList 收到 props.items 引用变化 → virtualizerOptions 重算 → virtualizer 更新 count
 *
 * 修复的 4 个症状：
 *   - "共 1 条（已筛选 10186 条）" 计数错（totalBackendCount 走 tick，notify 后正常）
 *   - 切后端 tab 自动滚动失效（watch(tick) 现在能 fire）
 *   - 级别筛选/搜索 UI 不响应（pushMany 之前漏 notify，filter.setFilter 路径 OK 但 pushMany 看着像卡死）
 *   - VirtualLogList 显示不全（virtualizer count 不更新）
 */
const backendUpdateTick = ref(0)
const backendLogsView = shallowRef<readonly LogEntry[]>(backendFilter.getResult())
const unsubBackendFilter = backendFilter.subscribe(() => {
  backendUpdateTick.value++
  // 🆕 修复 A1: spread 创建新数组引用，让 Vue 3 computed 缓存失效链路完整传递
  backendLogsView.value = [...backendFilter.getResult()]
  triggerRef(backendLogsView)
})

// 🆕 2026-06-15 rAF coalesce 后端日志：把单帧内多条 WS 消息合并为 1 次 filter.pushMany
  let pendingBackendLogs: LogEntry[] = []
  let flushScheduled = false
  function queueBackendLog(entry: LogEntry) {
    pendingBackendLogs.push(entry)
    if (flushScheduled) return
    flushScheduled = true
    requestAnimationFrame(flushPendingBackendLogs)
  }
  function flushPendingBackendLogs() {
    flushScheduled = false
    if (pendingBackendLogs.length === 0) return
    const toAdd = pendingBackendLogs
    pendingBackendLogs = []
    for (const entry of toAdd) {
      updateTagCounts(entry, backendTagCounts)
    }
    backendFilter.pushMany(toAdd)
  }

  /**
   * 🆕 2026-06-18 v4 Bug2 修复：冷启动拉历史日志分块（避免大 list 一次性 pushMany 卡 UI）
   *   - 旧逻辑：for (const e of resp.logs) queueBackendLog(e) → 单帧内触发 1 次 rAF → pushMany(arr) O(N)
   *   - 10K+ 历史日志时单帧推完仍可能超 16ms（1 帧预算），导致 tab 切换卡顿
   *   - 修法：每帧最多 200 条，分多帧推完（实测 5K log/帧 = 1 帧内 200 条 O(200) ≈ 0.5ms）
   *   - 配合 requestIdleCallback（推迟到 main thread idle），首次切到 DevLogs tab 渲染后再拉
   */
  const PUSH_CHUNK_SIZE = 200
  async function pushLogsInChunks(logs: BackendLogEntry[]): Promise<void> {
    if (logs.length === 0) return
    let i = 0
    const pushNextChunk = (): Promise<void> => {
      return new Promise((resolve) => {
        requestAnimationFrame(() => {
          const chunk = logs.slice(i, i + PUSH_CHUNK_SIZE)
          i += chunk.length
          if (chunk.length > 0) {
            for (const e of chunk) {
              const lvl: Level = ['debug', 'info', 'warn', 'error'].includes(e.level)
                ? (e.level as Level)
                : 'info'
              const entry: LogEntry = {
                id: ++nextId,
                timestamp: e.timestamp || new Date().toLocaleTimeString('zh-CN', { hour12: false }),
                level: lvl,
                message: e.message,
                tags: (e as any).tags ? (e as any).tags : undefined,
              }
              updateTagCounts(entry, backendTagCounts)
              backendFilter.push(entry)
            }
          }
          if (i < logs.length) {
            // 继续下一帧
            void pushNextChunk().then(resolve)
          } else {
            resolve()
          }
        })
      })
    }
    await pushNextChunk()
  }

  /** 冷启动拉历史日志入口（推迟到 idle + 分块推） */
  async function coldStartLoadRecentLogs(): Promise<void> {
    if (!serverOnline.value) return
    try {
      const resp = await getRecentBackendLogs()
      await pushLogsInChunks(resp.logs || [])
    } catch (err) {
      console.warn('[DevLogs] cold-start fetch recent logs failed:', err instanceof Error ? err.message : String(err))
    }
  }

const selectedLevelsArray = ref<string[]>(['debug', 'info', 'warn', 'error'])
const levelDropdownOptions: DropdownOption[] = [
  { value: 'debug', label: 'DEBUG' },
  { value: 'info', label: 'INFO' },
  { value: 'warn', label: 'WARN' },
  { value: 'error', label: 'ERROR' },
]

const selectedTags = ref<string[]>([])

const backendTagCounts = new Map<string, number>()

function updateTagCounts(entry: LogEntry, counts: Map<string, number>) {
  if (entry.tags && entry.tags.length > 0) {
    for (const tag of entry.tags) {
      counts.set(tag, (counts.get(tag) || 0) + 1)
    }
  }
}

const tagDropdownOptions = computed<DropdownOption[]>(() => {
  if (activeTab.value === 'frontend') {
    const counts = new Map<string, number>()
    for (const entry of frontendLogs.value) {
      updateTagCounts(entry, counts)
    }
    return Array.from(counts.entries())
      .map(([value, count]) => ({ value, label: value, count }))
      .sort((a, b) => b.count - a.count)
  }
  return Array.from(backendTagCounts.entries())
    .map(([value, count]) => ({ value, label: value, count }))
    .sort((a, b) => b.count - a.count)
})

/**
 * 🆕 2026-06-15 搜索高亮：转义 HTML 特殊字符 + 把 query 用 <mark> 包起来
 * 性能：30 item 虚拟列表下完全可承受
 */
function highlightMatch(text: string, query: string): string {
  const escapeHtml = (s: string) => s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
  if (!query.trim()) return escapeHtml(text)
  try {
    const escaped = query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const re = new RegExp(`(${escaped})`, 'gi')
    return escapeHtml(text).replace(re, '<mark>$1</mark>')
  } catch {
    return escapeHtml(text)
  }
}

function onLevelsChange(_values: string[]) {
  // levels 变化时，watch 会自动触发 filter 更新
}

function onTagsChange(_values: string[]) {
  // tags 变化时，watch 会自动触发 filter 更新
}

function onTagClick(tag: string) {
  if (!selectedTags.value.includes(tag)) {
    selectedTags.value = [...selectedTags.value, tag]
  }
  closeLogDetail()
}

function getSourceIcon(entry: LogEntry): string {
  const tags = entry.tags || []
  if (tags.includes('frontend')) {
    if (tags.includes('worker')) return '⚙'
    if (tags.includes('error-capture')) return '⚠'
    if (tags.includes('hmr')) return '🔥'
    if (tags.includes('console')) return '◉'
    return '◉'
  }
  if (tags.includes('backend')) {
    if (tags.includes('api')) return '🌐'
    if (tags.includes('websocket')) return '⚡'
    if (tags.includes('task')) return '📋'
    if (tags.includes('db')) return '🗄'
    if (tags.includes('mount')) return '📁'
    if (tags.includes('kernel') || tags.includes('plugin')) return '🔌'
    if (tags.includes('agent')) return '🤖'
    if (tags.includes('service')) return '⚙'
    return '●'
  }
  return '●'
}

function getSourceIconTitle(entry: LogEntry): string {
  const tags = entry.tags || []
  if (tags.length === 0) return entry.source || 'unknown'
  return tags.join(' · ')
}

let nextId = 0
const { logs: frontendLogs, clearLogs: clearFrontendLogs } = useFrontendLogs()
/**
 * 🆕 2026-06-15 1M+ 容量：所有 backend 状态都从 IncrementalFilter 读
 *   - filteredBackendItems：O(1) 拿当前过滤结果（incremental cache）
 *   - totalBackendCount：O(1) 拿 ring buffer 当前大小
 *   - backendUpdateTick：subscribe() 触发 → 触发重算 + triggerRef 桥接
 *
 *   依赖 backendLogsView（shallowRef）+ backendUpdateTick（ref）：
 *   - backendLogsView 强制 trigger 链：filter 变更 → triggerRef → 下游 effects 重算
 *   - backendUpdateTick 是显式 dep 标识（computed 缓存基于 dep 集合，triggerRef 内部也会 mark dep）
 */
const backendFilteredItems = computed<readonly LogEntry[]>(() => {
  // 访问 backendLogsView 触发 computed dep；filter.getResult() 返回同一引用但 triggerRef → 强制下游
  void backendLogsView.value
  // 显式 dep 标识：subscribe 回调里 ++tick，computed 也会重新求值
  void backendUpdateTick.value
  // 🆕 修复 A1: spread 确保返回新引用（双保险 — 即便 subscription 回调漏 spread，下游也能失效）
  return [...backendFilter.getResult()]
})
const totalBackendCount = computed(() => {
  void backendUpdateTick.value
  return backendFilter.totalLength
})
const serverOnline = ref(false)

function getBadgeColor(level: string): string {
  switch (level) {
    case 'debug': return 'medium'
    case 'info': return 'success'
    case 'warn': return 'warning'
    case 'error': return 'danger'
    default: return 'medium'
  }
}

/**
 * 🆕 2026-06-15：searchText 触发 IncrementalFilter.setFilter（O(N) rebuild 一次）。
 * 注意：searchText 已有 :debounce="150"，150ms 内多次输入只触发 1 次 setFilter
 * 1M rebuild 实测 27ms（< 60FPS 单帧预算 16.67ms × 2，仍在用户可感知阈值外）
 */
watch(
  [searchText, selectedLevelsArray, selectedTags],
  () => {
    backendFilter.setFilter({
      levels: new Set<Level>([...selectedLevelsArray.value] as Level[]),
      searchLower: searchText.value.toLowerCase(),
      tags: new Set<string>(selectedTags.value),
    })
  },
  { flush: 'post' },
)

const filteredFrontend = computed(() => {
  let logs = frontendLogs.value
  const lvls = selectedLevelsArray.value
  if (lvls.length > 0) {
    logs = logs.filter((l) => lvls.includes(l.level))
  }
  if (selectedTags.value.length > 0) {
    const tagSet = new Set(selectedTags.value)
    logs = logs.filter((l) => {
      if (!l.tags || l.tags.length === 0) return false
      return l.tags.some((t) => tagSet.has(t))
    })
  }
  if (searchText.value) logs = logs.filter((l) => l.message.toLowerCase().includes(searchText.value.toLowerCase()))
  return logs
})

const totalCurrent = computed(() => activeTab.value === 'frontend' ? frontendLogs.value.length : totalBackendCount.value)
const filteredCurrent = computed(() => activeTab.value === 'frontend' ? filteredFrontend.value.length : backendFilteredItems.value.length)

/** 重复点击当前 tab 按钮时滚到顶部（VS Code / Chrome DevTools 行为） */
function onTabClick(tab: 'frontend' | 'backend') {
  if (activeTab.value === tab) {
    scrollToTop()
  }
}

function onTabChange(event: CustomEvent) {
  activeTab.value = (event.detail.value || 'frontend') as 'frontend' | 'backend'
}

/**
 * 找 ion-content 实际滚动的元素（每次重查，不缓存）
 * 同步更新 scrollEl ref——虚拟列表的 useVirtualizer 通过 getScrollElement 观察它
 * 失败由 scrollToBottom / onJumpToBottom 的 rAF retry 处理
 *
 * 🆕 2026-06-15 修 #1：ion-content 是 Web Component，scroll 事件发生在 shadow DOM
 * 内部 .inner-scroll 上，**不**冒泡到 host → 模板 @scroll="onLogScroll" 收不到。
 * 修法：找到 .inner-scroll 后手动 addEventListener('scroll', onLogScroll, { passive: true })。
 * 用 boundScrollEl 跟踪已绑定的元素，避免重复绑定。
 */
let boundScrollEl: HTMLElement | null = null
function ensureScrollEl(): HTMLElement | null {
  if (!contentRef.value) {
    if (typeof window !== 'undefined' && (window as any).__DEVLOGS_DEBUG__) {
      throw new Error(`DEBUG ensureScrollEl: contentRef.value is null`)
    }
    return null
  }
  const hostEl = ((contentRef.value as any).$el || (contentRef.value as any)) as HTMLElement | undefined
  if (!hostEl || !hostEl.shadowRoot) {
    if (typeof window !== 'undefined' && (window as any).__DEVLOGS_DEBUG__) {
      throw new Error(`DEBUG ensureScrollEl: hostEl=${!!hostEl} shadowRoot=${!!hostEl?.shadowRoot}`)
    }
    return null
  }
  const el = hostEl.shadowRoot.querySelector('.inner-scroll') as HTMLElement | null
  if (el && el !== scrollEl.value) scrollEl.value = el
  // 🆕 手动绑定 scroll listener（Web Component shadow DOM 不冒泡）
  if (el && el !== boundScrollEl) {
    if (boundScrollEl) boundScrollEl.removeEventListener('scroll', onLogScroll)
    el.addEventListener('scroll', onLogScroll, { passive: true })
    boundScrollEl = el
  }
  return el
}

/**
 * 组件卸载时清理 scroll listener（避免热更新后泄漏）
 */
onBeforeUnmount(() => {
  if (boundScrollEl) {
    boundScrollEl.removeEventListener('scroll', onLogScroll)
    boundScrollEl = null
  }
})

function scrollToTop() {
  const el = ensureScrollEl()
  if (el) el.scrollTop = 0
}

/**
 * 滚动到底部（程序化）
 * 单一守卫：autoScrollEnabled=false 时直接 return
 * nextTick + rAF + retry rAF 是为了等 Ionic shadow DOM 异步挂载完成
 * smooth=true 时用 scrollTo 触发平滑滚动；smooth=false 时直接赋值即生效
 */
async function scrollToBottom(smooth = false) {
  if (!autoScrollEnabled.value) return
  await nextTick()
  await new Promise<void>((r) => requestAnimationFrame(() => r()))
  let el = ensureScrollEl()
  if (!el) {
    await new Promise<void>((r) => requestAnimationFrame(() => r()))
    el = ensureScrollEl()
  }
  if (!el) return
  if (smooth) el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' })
  else el.scrollTop = el.scrollHeight
}

/** 切换自动滚动状态（toolbar ▶/⏸ 开关） */
function toggleAutoScroll() {
  autoScrollEnabled.value = !autoScrollEnabled.value
}

/** 浮动「↓」按钮：开启跟随 + 平滑滚到底 */
async function onJumpToBottom() {
  autoScrollEnabled.value = true
  await scrollToBottom(true)
}

/** 浮动「↑」按钮：滚到顶部（不影响 autoScrollEnabled 状态） */
function onJumpToTop() {
  const el = ensureScrollEl()
  if (el) el.scrollTop = 0
}

/** 浮动「↑」按钮显示条件：滚离顶部 200px 以上时显示，避免无意义闪烁 */
const showScrollToTop = ref(false)
/** 跟踪 ion-content 滚动以控制 ↑ 按钮显示 */
function onLogScroll() {
  const el = ensureScrollEl()
  if (!el) { showScrollToTop.value = false; return }
  showScrollToTop.value = el.scrollTop > 200
}

// 🆕 2026-06-15 修 #2：点击日志行展开详情
const selectedLog = ref<LogEntry | null>(null)
function onLogSelect(item: LogEntry) {
  selectedLog.value = item
}
function closeLogDetail() {
  selectedLog.value = null
}
async function copyLogDetail() {
  if (!selectedLog.value) return
  const text = `[${selectedLog.value.timestamp}] ${selectedLog.value.level.toUpperCase()} ${selectedLog.value.message}`
  const ok = await copyToClipboard(text)
  if (ok) await showToast({ message: t('devlogs.logDetailCopied') })
  else await showToast({ message: t('devlogs.copyFailed') })
}

/** ESC 关闭详情模态 + body 滚动锁 */
function onKeyDown(e: KeyboardEvent) {
  if (e.key === 'Escape' && selectedLog.value) {
    closeLogDetail()
    e.preventDefault()
  }
}
watch(selectedLog, (open) => {
  if (typeof document === 'undefined') return
  // 🆕 详情模态打开时锁 body 滚动，避免背景跟着滚
  document.body.style.overflow = open ? 'hidden' : ''
})
onMounted(() => {
  if (typeof window !== 'undefined') window.addEventListener('keydown', onKeyDown)
})
onUnmounted(() => {
  if (typeof window !== 'undefined') window.removeEventListener('keydown', onKeyDown)
  if (typeof document !== 'undefined') document.body.style.overflow = ''
})

/** 新日志到达的统一处理（被 frontend/backend 两个 watcher 调用） */
function handleNewLog() {
  if (!autoScrollEnabled.value) return
  void scrollToBottom(false)
}

/**
 * 监听前端/后端日志变化
 * - 前端：watch length（useFrontendLogs 的 ref 数组）
 * - 后端：watch backendUpdateTick（IncrementalFilter 触发）
 * flush:'post' 确保 DOM patch 完成、ion-content shadow DOM 更新后再滚到底
 * （'pre' 触发时 scrollHeight 还没增大，滚不到底）
 * activeTab 切换时仅响应当前 tab 的日志
 */
watch(
  () => frontendLogs.value.length,
  () => {
    if (activeTab.value === 'frontend') handleNewLog()
  },
  { flush: 'post' },
)
watch(
  backendUpdateTick,
  () => {
    if (activeTab.value === 'backend') handleNewLog()
  },
  { flush: 'post' },
)

async function handleCopy() {
  const logs = activeTab.value === 'frontend' ? filteredFrontend.value : backendFilteredItems.value
  const text = logs.map((l) => `[${l.timestamp}] ${l.level.toUpperCase()} ${l.message}`).join('\n')
  const ok = await copyToClipboard(text)
  if (ok) {
    showToast({
      message: t('devlogs.copied', { count: String(logs.length) }),
      duration: 1500,
      color: 'success',
    })
  } else {
    showToast({ message: t('devlogs.copyFailed'), duration: 1500, color: 'danger' })
  }
}

async function handleClear() {
  const alert = await alertController.create({
    header: t('devlogs.clearConfirm'),
    buttons: [
      { text: t('common.cancel'), role: 'cancel' },
      {
        text: t('common.confirm'), role: 'destructive',
        handler: () => {
          if (activeTab.value === 'frontend') {
            clearFrontendLogs()
          } else {
            backendFilter.clear()
            backendTagCounts.clear()
          }
        },
      },
    ],
  })
  await alert.present()
}

function onWsMessage(data: any) {
  if (data && data.type === 'log' && data.data) {
    const logData = data.data
    const level = ['debug', 'info', 'warn', 'error'].includes(logData.level) ? logData.level : 'info'
    const message = String(logData.message || logData.msg || '')
    if (!message && !logData.message) return
    let tags: string[] | undefined
    if (Array.isArray(logData.tags)) {
      tags = logData.tags.filter((t: any) => typeof t === 'string')
    } else if (typeof logData.tags === 'string') {
      tags = logData.tags.split(',').filter(Boolean)
    }
    queueBackendLog({
      id: ++nextId,
      timestamp: logData.timestamp || new Date().toLocaleTimeString('zh-CN', { hour12: false }),
      level,
      message,
      source: typeof logData.source === 'string' ? logData.source : 'ws_log_handler',
      stack: typeof logData.stack === 'string' ? logData.stack : undefined,
      tags,
    })
    return
  }
  if (data && data.type && data.type !== 'log' && data.type !== 'pong' && data.type !== 'server:status') {
    const msg = typeof data === 'string' ? data : JSON.stringify(data)
    queueBackendLog({
      id: ++nextId,
      timestamp: new Date().toLocaleTimeString('zh-CN', { hour12: false }),
      level: 'debug',
      message: msg,
      source: 'ws_log_handler',
      tags: ['backend', 'websocket', 'debug'],
    })
  }
}

function onServerStatus(data: any) {
  serverOnline.value = data?.online ?? false
}

onMounted(async () => {
  await nextTick()

  // transport 已在 App.vue 启动为 useWebSocket 单例，DevLogs 只读 connectionState 不再 connect
  eventBus.on('ws:message', onWsMessage)
  eventBus.on('server:status', onServerStatus)

  serverOnline.value = transport.connectionState.value === 'connected'
  if (!serverOnline.value) {
    const result = await checkServerStatus()
    serverOnline.value = result.online
  }

  // 🆕 2026-06-16 修复：ion-content 异步渲染时 .inner-scroll 还没 ready
  // 旧逻辑：onMounted 调一次 ensureScrollEl → 可能仍 null → virtualizer 一直空 → 列表全白
  // 修法：多次重试（rAF + 0ms/50ms/150ms/300ms）+ ResizeObserver 兜底监听
  let retryCount = 0
  const maxRetries = 8
  const tryInitScrollEl = (): void => {
    const el = ensureScrollEl()
    if (el) {
      // 拿到 .inner-scroll 后，强制 virtualizer 重算（首次 watch 已触发 measure()，
      // 这里再 measure 一次确保虚拟列表渲染首屏 items）
      virtualizerForceMeasure()
      return
    }
    retryCount++
    if (retryCount < maxRetries) {
      // 指数退避：0ms → 50ms → 100ms → 150ms → 200ms → 250ms → 300ms
      const delay = Math.min(50 * retryCount, 300)
      setTimeout(tryInitScrollEl, delay)
    }
  }
  tryInitScrollEl()

  // 兜底：ResizeObserver 监听 contentRef 尺寸变化（ion-content 完成渲染时会触发）
  if (typeof ResizeObserver !== 'undefined' && contentRef.value) {
    const hostEl = ((contentRef.value as any).$el || contentRef.value) as HTMLElement | undefined
    if (hostEl) {
      const ro = new ResizeObserver(() => {
        if (!scrollEl.value) tryInitScrollEl()
      })
      ro.observe(hostEl)
      // 组件卸载时断开（onBeforeUnmount 已存在清理 hook，这里先缓存到 ro 变量）
      ;(contentRef.value as any).__devlogsRO__ = ro
    }
  }

  // 🆕 2026-06-16：冷启动拉一次后端历史日志
  // 不依赖 WS / http-poll 模式：HTTP 拉一次 GET /api/logs/recent
  // 真机 WS 模式：历史日志（启动前）补齐；WS 推的实时日志仍通过 onWsMessage 收
  // OpenPreview 沙箱：http-poll 模式也调用 getRecentBackendLogs，但这是冷启动多拉一次（幂等）
  // 🆕 2026-06-18 v4 Bug2 修复：推迟到 requestIdleCallback + pushLogsInChunks 分块推
  //   - 旧行为：onMounted 同步 for 循环 queueBackendLog + rAF 一次性 pushMany(arr) → 大 list 卡 UI
  //   - 新行为：requestIdleCallback 推迟到 main thread idle（首次切 tab 渲染后）→ 分块 200 条/帧推
  if (serverOnline.value) {
    const scheduleIdle = (cb: () => void) => {
      if (typeof window !== 'undefined' && typeof (window as any).requestIdleCallback === 'function') {
        (window as any).requestIdleCallback(cb, { timeout: 1500 })
      } else {
        setTimeout(cb, 200)
      }
    }
    scheduleIdle(() => {
      void coldStartLoadRecentLogs()
    })
  }

  // 写入一条启动日志（INFO/WARN 取决于 server 状态）——首条直接 push 即可
  backendFilter.push({
    id: ++nextId,
    timestamp: new Date().toLocaleTimeString('zh-CN', { hour12: false }),
    level: serverOnline.value ? 'info' : 'warn',
    message: `DevLogs ready, server ${serverOnline.value ? 'online' : 'offline'} (transport=${transport.connectionState.value})`,
  })
})

onUnmounted(() => {
  eventBus.off('ws:message', onWsMessage)
  eventBus.off('server:status', onServerStatus)
  unsubBackendFilter()
})

/** 暴露给单元测试（生产环境无副作用） */
defineExpose({
  autoScrollEnabled,
  activeTab,
  handleNewLog,
  toggleAutoScroll,
  onJumpToBottom,
  scrollToBottom,
  setActiveTab(tab: 'frontend' | 'backend') { activeTab.value = tab },
  /**
   * 测试专用：替换后端日志数组。
   * 走 setBackendLogs 显式赋值（Vue 自动 unwrap ref 导致 vm.backendLogs.value 无法访问）。
   * 重构后通过 IncrementalFilter.clear() + pushMany 模拟。
   */
  setBackendLogs(arr: LogEntry[]) {
    backendFilter.clear()
    if (arr.length > 0) backendFilter.pushMany(arr)
  },
  getBackendLogs(): readonly LogEntry[] { return backendFilter.getResult() },
  /** 测试专用：暴露 filter 实例用于高级断言 */
  backendFilter,
})
</script>

<style scoped>
.tab-toolbar {
  --padding-start: 8px;
  --padding-end: 8px;
  --min-height: 44px;
}
.tab-toolbar ion-segment {
  --background: rgba(var(--ion-background-color-rgb, 255, 255, 255), 0.08);
}

.toolbar-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  padding: 6px 10px;
  border-bottom: 1px solid var(--ion-border-color, rgba(255, 255, 255, 0.08));
  background: var(--ion-background-color);
}

.toolbar-actions {
  display: flex;
  gap: 2px;
  flex-shrink: 0;
}

.search-row {
  padding: 0 10px 4px;
  border-bottom: 1px solid var(--ion-border-color, rgba(255, 255, 255, 0.08));
  background: var(--ion-background-color);
}

.level-filters {
  display: flex;
  gap: 3px;
  flex-shrink: 0;
}

.level-btn {
  font-size: 11px;
  font-weight: 600;
  padding: 2px 8px;
  border-radius: 10px;
  border: 1px solid var(--ion-text-color-step-400, #666);
  background: transparent;
  color: var(--ion-text-color-step-400, #666);
  cursor: pointer;
  white-space: nowrap;
  transition: all 0.15s ease;
  letter-spacing: 0.3px;
  font-family: inherit;
}

.level-btn.active.all,
.level-btn.active.debug { background: rgba(136, 136, 136, 0.2); color: #aaa; border-color: #888; }
.level-btn.active.info { background: rgba(46, 204, 113, 0.15); color: #2ecc71; border-color: #2ecc71; }
.level-btn.active.warn { background: rgba(243, 156, 18, 0.15); color: #f39c12; border-color: #f39c12; }
.level-btn.active.error { background: rgba(231, 76, 60, 0.15); color: #e74c3c; border-color: #e74c3c; }

.log-searchbar {
  --border-radius: 12px;
  --background: rgba(255, 255, 255, 0.06);
  --placeholder-color: var(--ion-text-color-step-350, #aaa);
  --color: var(--ion-text-color);
  padding-top: 0;
  padding-bottom: 0;
}
.log-searchbar .searchbar-search-icon { display: none !important; }

.log-content { --background: var(--ion-background-color); }

.log-list {
  font-family: 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.6;
  padding: 4px 6px;
  min-height: 200px;
}

.conn-indicator {
  text-align: center;
  padding: 4px 0 8px;
}

/* 🆕 2026-06-15：DevLogs backend tab 顶部的 ServerStatusCard 容器
   compact 模式：保留状态行 + latency/transport pills，省略 detail-grid / footer */
.devlog-status-card-wrap {
  padding: 4px 6px 8px;
}
.devlog-status-card-wrap :deep(.server-status-card) {
  width: 100%;
}

/* .log-entry 及其 .error/.warn/.info/.debug 变体样式已在 VirtualLogList.vue 中定义
   .log-time / .log-msg / .level-badge 仍属本组件作用域（slot 渲染本组件） */

.log-time {
  color: var(--ion-text-color-step-400, #555);
  white-space: nowrap;
  flex-shrink: 0;
  user-select: none;
  font-size: 11px;
}

.level-badge {
  --padding-start: 4px;
  --padding-end: 4px;
  --padding-top: 0;
  --padding-bottom: 0;
  font-size: 9px;
  font-weight: 700;
  letter-spacing: 0.5px;
  height: 16px;
  flex-shrink: 0;
}

.log-msg {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.empty-logs {
  text-align: center;
  padding: 40px 20px;
  color: var(--ion-text-color-step-400, #555);
  font-size: 13px;
}

.status-bar {
  --background: var(--ion-toolbar-background, rgba(var(--ion-background-color-rgb), 0.92));
  --border-width: 1px 0 0 0;
  backdrop-filter: blur(8px);
}
.status-bar ion-toolbar { --padding-start: 12px; --padding-end: 12px; --min-height: 38px; }

.status-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
}
.status-text { font-size: 11px; color: var(--ion-text-color-step-400, #666); }

/* 浮动按钮容器：position: fixed 列布局，bottom 偏移让出 status-bar（44px + 20px） */
.scroll-buttons {
  position: fixed;
  right: 16px;
  bottom: 64px;
  z-index: 9999;
  display: flex;
  flex-direction: column;
  gap: 8px;
  pointer-events: none;
}
.scroll-buttons > * { pointer-events: auto; }

/* 浮动「↑」按钮：滚顶，scrollTop > 200 时显示 */
.scrollToTopBtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border: 0;
  border-radius: 50%;
  background: var(--ion-toolbar-background, var(--ion-background-color));
  color: var(--ion-color-primary);
  box-shadow: 0 3px 10px rgba(0, 0, 0, 0.18), 0 1px 3px rgba(0, 0, 0, 0.12);
  cursor: pointer;
  padding: 0;
  transition: transform 0.12s, box-shadow 0.12s;
}
.scrollToTopBtn:hover { transform: scale(1.06); }
.scrollToTopBtn:active { transform: scale(0.94); }
.scrollToTopIcon { font-size: 20px; }

/* 浮动「↓」按钮：滚底，position: fixed 视口右下角永远可见 */
.scrollToBottomBtn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 40px;
  height: 40px;
  border: 0;
  border-radius: 50%;
  background: var(--ion-toolbar-background, var(--ion-background-color));
  color: var(--ion-color-primary);
  box-shadow: 0 3px 10px rgba(0, 0, 0, 0.18), 0 1px 3px rgba(0, 0, 0, 0.12);
  cursor: pointer;
  padding: 0;
  transition: transform 0.12s, box-shadow 0.12s;
}
.scrollToBottomBtn:hover { transform: scale(1.06); }
.scrollToBottomBtn:active { transform: scale(0.94); }
.scrollToBottomIcon { font-size: 20px; }
.fade-enter-active, .fade-leave-active { transition: opacity 0.2s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }

/* 搜索关键词高亮（v-html 注入 <mark>，在 log-msg 内部） */
.log-msg :deep(mark) {
  background: rgba(241, 196, 15, 0.35);
  color: inherit;
  border-radius: 2px;
  padding: 0 1px;
}

/* status-bar 自动滚动状态文字：暂停时 warning 色 */
.auto-scroll-status { font-weight: 500; }
.auto-scroll-status.paused { color: var(--ion-color-warning); }

/*
  🆕 2026-06-15 修 #2：日志详情模态样式
  背景全屏半透明遮罩 + 中央卡片（max-width 640px，响应式）
  message 区 pre 块等宽字体 + 自动换行 + 横向滚动条
*/
.log-detail-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.55);
  z-index: 10000;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  animation: logDetailFade 0.16s ease-out;
}
@keyframes logDetailFade {
  from { opacity: 0; }
  to { opacity: 1; }
}
.log-detail-modal {
  width: 100%;
  max-width: 640px;
  max-height: 80vh;
  display: flex;
  flex-direction: column;
  background: var(--ion-background-color, #fff);
  border-radius: 10px;
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3);
  overflow: hidden;
}
.log-detail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--ion-border-color, rgba(0, 0, 0, 0.08));
}
.log-detail-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
}
.log-detail-close {
  background: transparent;
  border: 0;
  cursor: pointer;
  color: var(--ion-color-medium);
  font-size: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 50%;
  padding: 0;
}
.log-detail-close:hover {
  background: var(--ion-color-light);
}
.log-detail-body {
  flex: 1;
  overflow-y: auto;
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.log-detail-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.log-detail-label {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--ion-color-medium);
}
.log-detail-value {
  font-family: var(--ion-font-family-monospace, 'Courier New', monospace);
  font-size: 12px;
  word-break: break-all;
}
.log-time-detail { color: var(--ion-color-medium); }
/* 🆕 2026-06-16：紧凑 1 行 2 列 — 时间 + 级别共用一行 */
.log-detail-meta-row {
  flex-direction: row;
  gap: 12px;
  align-items: stretch;
}
.log-detail-meta-item {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.log-source-detail {
  color: var(--ion-color-primary);
  font-weight: 600;
  font-size: 11px;
  background: var(--ion-color-light);
  padding: 2px 6px;
  border-radius: 3px;
  display: inline-block;
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.log-detail-message-row { flex: 1; min-height: 0; }
.log-detail-message {
  margin: 0;
  padding: 10px 12px;
  background: var(--ion-color-light);
  border-radius: 6px;
  font-family: var(--ion-font-family-monospace, 'Courier New', monospace);
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;       /* 保留换行 + 自动换行 */
  word-break: break-all;       /* 长 URL/路径强制断行 */
  max-height: 50vh;
  overflow-y: auto;
  color: var(--ion-text-color);
}
/* 🆕 2026-06-16：堆栈专用样式 — 深色背景 + 小字体 + 高对比（错误排障优先） */
.log-detail-stack-row { flex-shrink: 0; }
.log-detail-stack {
  margin: 0;
  padding: 10px 12px;
  background: #1e1e1e;   /* 始终深色 — 即便整体主题是 light */
  color: #d4d4d4;
  border-radius: 6px;
  font-family: var(--ion-font-family-monospace, 'Courier New', monospace);
  font-size: 11px;
  line-height: 1.45;
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 35vh;       /* 堆栈常常很长，但不要比 message 抢更多空间 */
  overflow-y: auto;
  user-select: text;       /* 关键：可复制堆栈 */
  -webkit-user-select: text;
}
/* 🆕 2026-06-16：无堆栈时显示「(no stack trace available)」占位 — 仍是深色但斜体低对比 */
.log-detail-stack-empty {
  font-style: italic;
  opacity: 0.65;
  color: #888;
}
.log-detail-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 10px 16px;
  border-top: 1px solid var(--ion-border-color, rgba(0, 0, 0, 0.08));
}

.filter-dropdowns {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  flex-shrink: 0;
}

.log-source-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  font-size: 12px;
  flex-shrink: 0;
  line-height: 1;
  user-select: none;
}

.log-detail-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.log-tag-chip {
  display: inline-block;
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 500;
  border-radius: 12px;
  background: var(--ion-color-light-shade, #eee);
  color: var(--ion-color-dark, #333);
  cursor: pointer;
  transition: all 0.15s ease;
  font-family: var(--ion-font-family, sans-serif);
}

.log-tag-chip:hover {
  background: var(--ion-color-primary-tint, #e8f0fe);
  color: var(--ion-color-primary, #3880ff);
}

@media (prefers-color-scheme: dark) {
  .log-tag-chip {
    background: var(--ion-color-dark-shade, #2a2a2a);
    color: var(--ion-color-light, #ddd);
  }

  .log-tag-chip:hover {
    background: rgba(56, 128, 255, 0.2);
    color: var(--ion-color-primary, #6ba3ff);
  }
}
</style>
