import { ref, onMounted, onUnmounted } from 'vue'
import { eventBus } from './useEventBus'
import { getApiBaseUrl } from '@/api/encv'

/**
 * 向量搜索可用性状态。
 *
 * 状态机（3 态）：
 *   - 'unknown'     尚未探测完成（首屏 / 启动瞬间）
 *   - 'available'   searchSvc != nil（原生 vector_distance_cos 可用）
 *   - 'degraded'    searchSvc 存在但运行时降级（libSQL 不支持，向量搜索退化为 Go 层）
 *   - 'unavailable' searchSvc == nil（纯关键词模式）
 *
 * 三态在前端的视觉区分（见 Files.vue / Tasks.vue）：
 *   - 'available'   → 搜索中显示 spinner（正常旋转，代表「正在搜索 + 索引可用」）
 *   - 'degraded'    → 搜索中显示 dots（安静等待，代表「索引部分可用」）
 *   - 'unavailable' → 搜索中显示 pulse（脉冲闪动，代表「仅纯关键词」）
 *
 * 数据源：后端 GET /api/runtime（见 internal/server/runtime_api.go）
 *   - vector_search_available: bool
 *   - vector_search_degraded:  bool
 *
 * 轮询策略：App 启动时探测一次；后端 server:status 事件触发重探测。
 */
export type VectorSearchStatus = 'unknown' | 'available' | 'degraded' | 'unavailable'

const status = ref<VectorSearchStatus>('unknown')
const lastCheckedAt = ref<Date | null>(null)
let pollInFlight = false
let pollTimer: ReturnType<typeof setInterval> | null = null

async function probeOnce(): Promise<void> {
  if (pollInFlight) return
  pollInFlight = true
  try {
    // 用全局 fetch（native 模式下 useProxiedFetch.installProxiedFetch() 已自动覆盖）
    const url = getApiBaseUrl() + '/api/runtime'
    const resp = await fetch(url, { method: 'GET' })
    if (!resp.ok) {
      return
    }
    const data = (await resp.json()) as {
      vector_search_available?: boolean
      vector_search_degraded?: boolean
    }
    const available = data.vector_search_available === true
    const degraded = data.vector_search_degraded === true
    if (!available) {
      status.value = 'unavailable'
    } else if (degraded) {
      status.value = 'degraded'
    } else {
      status.value = 'available'
    }
    lastCheckedAt.value = new Date()
  } catch {
    // 探测失败（后端未就绪）→ 保持 unknown，不报错
  } finally {
    pollInFlight = false
  }
}

/**
 * useVectorSearchStatus 暴露向量搜索可用性的可观察 ref。
 *
 * 用法：
 *   const { status, lastCheckedAt, refresh } = useVectorSearchStatus()
 *   // status.value === 'available' | 'degraded' | 'unavailable' | 'unknown'
 *
 * 自动行为：
 *   - onMounted 探测一次
 *   - 监听 'server:status' 事件（后端 ready 时）触发刷新
 *   - 每 60s 兜底轮询（防止后端状态切换遗漏）
 */
export function useVectorSearchStatus() {
  function start(): void {
    if (pollTimer) return
    void probeOnce()
    pollTimer = setInterval(() => void probeOnce(), 60_000)
  }

  function stop(): void {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  }

  function refresh(): Promise<void> {
    return probeOnce()
  }

  onMounted(() => {
    start()
    eventBus.on('server:status', refresh)
  })
  onUnmounted(() => {
    stop()
    eventBus.off('server:status', refresh)
  })

  return {
    status,
    lastCheckedAt,
    refresh,
  }
}
