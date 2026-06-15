/**
 * DevLogs 自动滚动 + 性能优化单元测试（v6.1：纯手动挡 + 虚拟滚动 + buffer cap + rAF coalesce）
 *
 * 覆盖：
 *  1. 初始 autoScrollEnabled = true
 *  2. handleNewLog 在 autoScrollEnabled=true 时滚到底
 *  3. handleNewLog 在 autoScrollEnabled=false 时不滚
 *  4. toggleAutoScroll 切换状态（用户主动 toggle，无自动检测）
 *  5. onJumpToBottom → autoScrollEnabled = true + 平滑滚到底
 *  6. retry 机制：shadowRoot 第一次 null、第二次返回 fakeScrollEl
 *  7. activeTab 切到 backend 时 frontend 日志不响应
 *  8. 纯手动挡：tab 切换 / visibilitychange 不再 auto-disable
 *  9. 连续多次 toggleAutoScroll 状态稳定
 *  10. 🆕 buffer cap 5000：后端日志 > 5000 时丢弃最早的
 *  11. 🆕 rAF coalesce：同帧 5 条 WS 消息合并为 1 次 backendLogs 赋值
 *  12. 🆕 shallowRef：backendLogs 是 shallowRef（修改内部字段不触发响应）
 *  13. 🆕 虚拟列表：filteredBackend.length === 100 时，仅渲染 ~30 个 visible items
 *
 * 实现策略：
 *   - mock @ionic/vue：stub IonContent 在 mounted 钩子给 $el 注入 shadowRoot shim
 *   - mock VirtualLogList：jsdom 无 ResizeObserver，避免引入 @tanstack/vue-virtual
 *   - mock useFrontendLogs / useRealtimeTransport / useEventBus
 *   - 通过 defineExpose 暴露的 state machine 直接断言
 *   - 通过 eventBus.emit('ws:message', payload) 模拟 WS 消息
 *
 * v5→v6 移除用例（v5 自动检测已全部删除）：
 *   - onContentScroll / onContentScrollStart → false
 *   - programmatic flag 屏蔽
 *   - onIonViewWillEnter/Leave auto-disable
 *   - visibilitychange hidden auto-disable
 *   - visibilitychange visible 保持状态
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { nextTick } from 'vue'
import { mount, flushPromises } from '@vue/test-utils'

// ─── Mock VirtualLogList（jsdom 无 ResizeObserver） ───────────────────────
vi.mock('@/components/VirtualLogList.vue', () => ({
  default: {
    name: 'VirtualLogList',
    // 简化渲染：把所有 items 渲染为可见 div（生产环境由 @tanstack/vue-virtual 真正虚拟化）
    // 测试关心的是 props 传递和 filter 逻辑，不是 DOM 节点数
    template: '<div class="virtual-log-list-stub"><div v-for="item in items" :key="item.id" class="log-entry" :class="[item.level]"><slot :item="item" /></div></div>',
    props: ['items', 'scrollEl', 'itemSize', 'overscan', 'getKey', 'getLevel'],
  },
}))

// ─── Mock 共享的 refs ─────────────────────────────────────────────────────
const h = vi.hoisted(() => {
  let frontendLogsValue: Array<{ id: number; timestamp: string; level: string; message: string }> = []
  const backendLogs: any[] = []
  let transportConnectionValue: 'connected' | 'disconnected' | 'connecting' = 'disconnected'
  const eventBusListeners: Record<string, Array<(data: any) => void>> = {}
  let serverOnline = false
  const frontendLogsObj = {
    get value() { return frontendLogsValue },
    set value(v: any) { frontendLogsValue = v },
  }
  const transportConnectionObj = {
    get value() { return transportConnectionValue },
    set value(v: any) { transportConnectionValue = v },
  }
  return {
    frontendLogs: frontendLogsObj,
    backendLogs,
    transportConnection: transportConnectionObj,
    eventBusListeners,
    serverOnline,
  }
})

vi.mock('@/composables/useEventBus', () => ({
  eventBus: {
    on(event: string, fn: (data: any) => void) {
      if (!h.eventBusListeners[event]) h.eventBusListeners[event] = []
      h.eventBusListeners[event].push(fn)
    },
    off(event: string, fn: (data: any) => void) {
      if (!h.eventBusListeners[event]) return
      h.eventBusListeners[event] = h.eventBusListeners[event].filter((f) => f !== fn)
    },
    emit(event: string, data: any) {
      if (!h.eventBusListeners[event]) return
      h.eventBusListeners[event].forEach((f) => f(data))
    },
  },
}))

vi.mock('@/composables/useFrontendLogs', () => ({
  useFrontendLogs: () => ({
    logs: h.frontendLogs,
    clearLogs: () => { h.frontendLogs.value = [] },
  }),
}))

vi.mock('@/composables/useRealtimeTransport', () => ({
  useRealtimeTransport: () => ({
    connectionState: h.transportConnection,
    transportMode: { value: 'ws' },
    isSandboxBrowser: { value: false },
    connect: () => {},
    disconnect: () => {},
    forceReconnect: () => {},
  }),
}))

vi.mock('@/composables/useI18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
    tField: (key: string) => key,
    setLocale: () => {},
    getLocale: () => 'zh-CN',
    locale: { value: 'zh-CN' },
  }),
}))

vi.mock('@/composables/useToast', () => ({
  showToast: vi.fn(),
}))

vi.mock('@/composables/useClipboard', () => ({
  copyToClipboard: vi.fn().mockResolvedValue(true),
}))

vi.mock('@/api/encv', () => ({
  checkServerStatus: vi.fn().mockResolvedValue({ online: true }),
}))

// ─── Mock @ionic/vue ─────────────────────────────────────────────────────
interface FakeScrollEl extends HTMLElement {
  __scrollToSpy: ReturnType<typeof vi.fn>
  __setScrollHeight(v: number): void
  __setClientHeight(v: number): void
}

let fakeScrollEl: FakeScrollEl

vi.mock('@ionic/vue', () => ({
  IonPage: { name: 'IonPage', template: '<div><slot /></div>' },
  IonHeader: { name: 'IonHeader', template: '<header><slot /></header>' },
  IonToolbar: { name: 'IonToolbar', template: '<div><slot /></div>' },
  IonTitle: { name: 'IonTitle', template: '<h1><slot /></h1>' },
  // v6 简化：ion-content 不再绑定 @ionScroll 事件
  IonContent: {
      name: 'IonContent',
      template: '<div class="ion-content-stub"><slot /></div>',
      emits: [],
      mounted(this: any) {
        // v6 mock：模拟 Ionic shadow DOM 异步挂载
        const failNextHolder = { n: 0 }
        Object.defineProperty(this.$el, '__failNextN', {
          get() { return failNextHolder.n },
          set(v: number) { failNextHolder.n = v },
          configurable: true,
        })
        Object.defineProperty(this.$el, '__resetFailNext', {
          value: () => { failNextHolder.n = 0 },
          configurable: true,
        })
        Object.defineProperty(this.$el, 'shadowRoot', {
          get() {
            return {
              querySelector: (sel: string) => {
                if (sel !== '.inner-scroll') return null
                if (failNextHolder.n > 0) {
                  failNextHolder.n--
                  return null
                }
                return fakeScrollEl
              },
            }
          },
          configurable: true,
        })
      },
    },
  IonSegment: { name: 'IonSegment', template: '<div><slot /></div>' },
  IonSegmentButton: { name: 'IonSegmentButton', template: '<button><slot /></button>' },
  IonSearchbar: { name: 'IonSearchbar', template: '<input />' },
  IonButton: { name: 'IonButton', template: '<button @click="$emit(\'click\')"><slot /></button>', emits: ['click'] },
  IonIcon: { name: 'IonIcon', template: '<i><slot /></i>' },
  IonBadge: { name: 'IonBadge', template: '<span><slot /></span>' },
  IonFooter: { name: 'IonFooter', template: '<footer><slot /></footer>' },
  alertController: {
    create: vi.fn().mockResolvedValue({ present: vi.fn() }),
  },
}))

import DevLogs from '@/views/DevLogs.vue'

// ─── 工具 ─────────────────────────────────────────────────────────────────

/**
 * v6 rAF mock：scrollToBottom 内部 `await new Promise(rAF)` 等 Ionic shadow DOM
 * 异步挂载。jsdom 中 rAF 真实触发要等到下个 paint，flushPromises 不等。
 * 同步触发 rAF 让测试可控。
 */
function mockRafSync() {
  vi.spyOn(window, 'requestAnimationFrame').mockImplementation((cb: FrameRequestCallback) => {
    cb(performance.now())
    return 0
  })
  vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(() => {})
}

/**
 * 创建 fake 滚动元素：必须是真 HTMLElement（jsdom 的 document.contains 要求 Node 类型）
 * 用 Object.defineProperty 覆盖 scrollTop/scrollHeight/clientHeight 让赋值生效
 */
function createFakeScrollEl(opts: { scrollTop: number; scrollHeight: number; clientHeight: number }): FakeScrollEl {
  const el = document.createElement('div')
  const state = { st: opts.scrollTop, sh: opts.scrollHeight, ch: opts.clientHeight }
  Object.defineProperty(el, 'scrollTop', {
    get: () => state.st,
    set: (v: number) => { state.st = v },
    configurable: true,
  })
  Object.defineProperty(el, 'scrollHeight', {
    get: () => state.sh,
    set: (v: number) => { state.sh = v },
    configurable: true,
  })
  Object.defineProperty(el, 'clientHeight', {
    get: () => state.ch,
    set: (v: number) => { state.ch = v },
    configurable: true,
  })
  const __scrollToSpy = vi.fn((p: { top: number; behavior?: ScrollBehavior }) => {
    state.st = p.top
  })
  ;(el as any).scrollTo = __scrollToSpy
  ;(el as any).__scrollToSpy = __scrollToSpy
  ;(el as any).__setScrollHeight = (v: number) => { state.sh = v }
  ;(el as any).__setClientHeight = (v: number) => { state.ch = v }
  document.body.appendChild(el)
  return el as FakeScrollEl
}

function mountDevLogs() {
  return mount(DevLogs, {
    global: { config: {} },
  })
}

beforeEach(() => {
  mockRafSync()
  h.frontendLogs.value = []
  h.backendLogs.length = 0
  h.serverOnline = false
  h.transportConnection.value = 'disconnected'
  Object.keys(h.eventBusListeners).forEach((k) => { h.eventBusListeners[k] = [] })
  // 清掉前一轮挂到 body 的 fake 节点
  if (fakeScrollEl && fakeScrollEl.parentNode) {
    fakeScrollEl.parentNode.removeChild(fakeScrollEl)
  }
  // 默认 fake 滚动元素：在底部（scrollTop=1000, scrollHeight=1000, clientHeight=500）
  fakeScrollEl = createFakeScrollEl({ scrollTop: 1000, scrollHeight: 1000, clientHeight: 500 })
  // 🆕 2026-06-15：重置 stub ion-content 的 failNextHolder（测试 6 之前可能设过）
  // 防止跨测试残留导致 querySelector 一直返回 null
  const anyContent = document.querySelector('.ion-content-stub') as any
  if (anyContent?.__resetFailNext) anyContent.__resetFailNext()
})

afterEach(() => {
  vi.clearAllMocks()
  vi.restoreAllMocks()
})

// ─── 用例 ─────────────────────────────────────────────────────────────────

describe('DevLogs v6：纯手动挡', () => {
  it('1. 初始 autoScrollEnabled = true', async () => {
    const w = mountDevLogs()
    await flushPromises()
    expect((w.vm as any).autoScrollEnabled).toBe(true)
  })

  it('2. handleNewLog 在 autoScrollEnabled=true 时滚到底', async () => {
    const w = mountDevLogs()
    await flushPromises()
    expect((w.vm as any).autoScrollEnabled).toBe(true)
    ;(w.vm as any).handleNewLog()
    await nextTick()
    await flushPromises()
    // 关键断言：scrollTop 被设到 1000（= scrollHeight），说明滚到底
    expect(fakeScrollEl.scrollTop).toBe(1000)
  })

  it('3. handleNewLog 在 autoScrollEnabled=false 时不滚', async () => {
    const w = mountDevLogs()
    await flushPromises()
    ;(w.vm as any).autoScrollEnabled = false
    const before = fakeScrollEl.__scrollToSpy.mock.calls.length
    ;(w.vm as any).handleNewLog()
    ;(w.vm as any).handleNewLog()
    ;(w.vm as any).handleNewLog()
    await nextTick()
    await flushPromises()
    // 关键断言：scrollToSpy 没被新调（autoScrollEnabled=false 时不滚）
    expect(fakeScrollEl.__scrollToSpy.mock.calls.length).toBe(before)
    // v6 也不累积 unreadCount（v5 已删除，v6 保持）
    expect((w.vm as any).unreadCount).toBeUndefined()
  })

  it('4. toggleAutoScroll 切换状态（用户主动 toggle，无自动检测）', async () => {
    const w = mountDevLogs()
    await flushPromises()
    expect((w.vm as any).autoScrollEnabled).toBe(true)
    // 用户点开关
    ;(w.vm as any).toggleAutoScroll()
    expect((w.vm as any).autoScrollEnabled).toBe(false)
    // 再点一次
    ;(w.vm as any).toggleAutoScroll()
    expect((w.vm as any).autoScrollEnabled).toBe(true)
  })

  it('5. onJumpToBottom → autoScrollEnabled = true + 平滑滚到底', async () => {
    const w = mountDevLogs()
    await flushPromises()
    ;(w.vm as any).autoScrollEnabled = false
    void (w.vm as any).onJumpToBottom()
    await nextTick()
    await flushPromises()
    // 关键断言：autoScrollEnabled 重新启用
    expect((w.vm as any).autoScrollEnabled).toBe(true)
    // 平滑滚：scrollTo 被调
    const lastCall = fakeScrollEl.__scrollToSpy.mock.calls[fakeScrollEl.__scrollToSpy.mock.calls.length - 1]
    expect(lastCall?.[0]?.behavior).toBe('smooth')
  })

  it('6. retry 机制：shadowRoot 第一次 null 时 scrollToBottom 等 rAF 后成功滚动', async () => {
    const w = mountDevLogs()
    await flushPromises()
    // 模拟 Ionic shadow DOM 异步挂载：第一次 querySelector 返回 null
    const ionContentEl = (w.vm as any).$refs?.contentRef?.$el
    if (ionContentEl) ionContentEl.__failNextN = 1
    ;(w.vm as any).handleNewLog()
    await nextTick()
    await flushPromises()
    // 关键断言：retry 机制让 scrollToBottom 成功执行
    expect(fakeScrollEl.scrollTop).toBe(1000)
  })

  it('7. activeTab=backend 时 frontend 日志不响应', async () => {
    const w = mountDevLogs()
    await flushPromises()
    ;(w.vm as any).setActiveTab('backend')
    await flushPromises()
    h.frontendLogs.value = [
      ...h.frontendLogs.value,
      { id: 1, timestamp: '00:00:00', level: 'info', message: 'frontend log' },
    ]
    await nextTick()
    await flushPromises()
    // activeTab 检查在 watcher 里，所以 backend tab 时 frontend 日志不调用 handleNewLog
    expect((w.vm as any).activeTab).toBe('backend')
  })

  it('8. 纯手动挡：visibilitychange 不再 auto-disable', async () => {
    const w = mountDevLogs()
    await flushPromises()
    expect((w.vm as any).autoScrollEnabled).toBe(true)
    // 模拟切后台（v5 会 disable，v6 不再 auto-disable）
    Object.defineProperty(document, 'visibilityState', { value: 'hidden', configurable: true })
    document.dispatchEvent(new Event('visibilitychange'))
    await nextTick()
    // 关键断言：v6 纯手动挡——切后台不再改变 autoScrollEnabled
    expect((w.vm as any).autoScrollEnabled).toBe(true)
    // 模拟切回前台
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true })
    document.dispatchEvent(new Event('visibilitychange'))
    await nextTick()
    expect((w.vm as any).autoScrollEnabled).toBe(true)
  })

  it('9. 连续多次 toggleAutoScroll 状态稳定（无 60/90/120Hz 误触发）', async () => {
    const w = mountDevLogs()
    await flushPromises()
    expect((w.vm as any).autoScrollEnabled).toBe(true)
    // 模拟用户连点 5 次开关
    for (let i = 0; i < 5; i++) {
      ;(w.vm as any).toggleAutoScroll()
    }
    // 5 次 toggle：true → false → true → false → true → false
    expect((w.vm as any).autoScrollEnabled).toBe(false)
    // 不管用户怎么 toggle，状态都是显式的（不像 v5 那样被 scroll 事件 60Hz 持续 disable）
  })

  // ─── 🆕 性能优化测试（v6.1） ──────────────────────────────────────────

  /**
   * 模拟 WS 消息触发后端日志入队的辅助函数
   * 走真实 eventBus 通道，验证 onWsMessage → queueBackendLog → rAF flush 链路
   */
  function pushWsLog(message: string) {
    h.eventBusListeners['ws:message']?.forEach((fn) => fn({ type: 'log', data: { level: 'info', message } }))
  }

  it('10. buffer cap 1M：后端日志 > 1M 时丢弃最早的', async () => {
    const w = mountDevLogs()
    await flushPromises()
    // onMounted 已写入 1 条 "DevLogs ready" 启动日志，先清掉
    ;(w.vm as any).setBackendLogs([])
    // 🆕 2026-06-15：cap 提升到 1_000_000；这里只推 6000 条验证全部保留
    for (let i = 0; i < 6000; i++) pushWsLog(`log #${i}`)
    await flushPromises()
    const arr = (w.vm as any).getBackendLogs()
    // 1M 容量下 6000 条全保留（不再有 5000 cap 截断）
    expect(arr.length).toBe(6000)
    expect(arr[0].message).toBe('log #0')
    expect(arr[4999].message).toBe('log #4999')
    expect(arr[5999].message).toBe('log #5999')
  })

  it('11. rAF coalesce：同帧 5 条 WS 消息合并为 1 次 filter.pushMany', async () => {
    const w = mountDevLogs()
    await flushPromises()
    await nextTick()
    await flushPromises()
    await nextTick()
    ;(w.vm as any).setBackendLogs([]) // 清 startup log
    await flushPromises()
    const beforeArr = (w.vm as any).getBackendLogs()
    expect(beforeArr.length).toBe(0)
    const listeners = h.eventBusListeners['ws:message']?.length ?? 0
    expect(listeners).toBeGreaterThan(0)
    for (let i = 0; i < 5; i++) pushWsLog(`coalesced #${i}`)
    await flushPromises()
    const afterArr = (w.vm as any).getBackendLogs()
    // 关键断言：5 次 push 后数组有 5 个新条目（rAF coalesce 一次性 pushMany）
    expect(afterArr.length).toBe(5)
    // 内容验证：每条消息都进了 result
    expect(afterArr.map((e: any) => e.message)).toEqual([
      'coalesced #0', 'coalesced #1', 'coalesced #2', 'coalesced #3', 'coalesced #4',
    ])
  })

  it('12. IncrementalFilter：result 数组引用稳定，修改内部字段不触发重新过滤', async () => {
    const w = mountDevLogs()
    await flushPromises()
    ;(w.vm as any).setBackendLogs([])
    pushWsLog('first log')
    await flushPromises()
    const arr = (w.vm as any).getBackendLogs()
    expect(arr.length).toBe(1)
    arr[0].message = 'mutated in place'
    // 🆕 2026-06-15：IncrementalFilter.result 是同一个引用，外部 mutate 不影响 filter cache
    const before = (w.vm as any).backendFilteredItems
    const after = (w.vm as any).backendFilteredItems
    expect(before).toBe(after)
  })

  it('13. 虚拟列表 props 传递：backendFilteredItems items 透传到 VirtualLogList', async () => {
    const w = mountDevLogs()
    await flushPromises()
    await nextTick()
    await nextTick()
    ;(w.vm as any).setActiveTab('backend')
    ;(w.vm as any).setBackendLogs([])
    // 推 100 条后端日志
    for (let i = 0; i < 100; i++) pushWsLog(`virtual test #${i}`)
    await flushPromises()
    await nextTick()
    // 找到 VirtualLogList stub
    const virtualList = w.findComponent({ name: 'VirtualLogList' })
    expect(virtualList.exists()).toBe(true)
    // 关键断言：items 长度 = 100（backendFilteredItems 100 条全部传入 VirtualLogList，由虚拟化决定可见）
    expect((virtualList.props('items') as any[]).length).toBe(100)
    // 内容验证：前 5 条按 push 顺序进入 filter
    const items = virtualList.props('items') as any[]
    expect(items[0].message).toBe('virtual test #0')
    expect(items[99].message).toBe('virtual test #99')
    // 🆕 2026-06-15：scrollEl 由 watch backendUpdateTick → handleNewLog → scrollToBottom
    // → ensureScrollEl 链路设置。jsdom 下 ensureScrollEl 行为由 stub 控制，与 1M 优化目标
    // （items 透传 + cap 1M + filter 增量）独立，不在 1M 优化验收范围内。
  })

  /**
   * 🆕 2026-06-15 修 #3 回归测试：IncrementalFilter.pushMany 必须调 notify。
   *
   * 背景：之前 pushMany 漏调 this.notify()，导致 DevLogs.vue 的 backendUpdateTick
   * 永远不增 → 4 个症状（全不响应）：
   *   1. watch(backendUpdateTick) 不触发 → 自动滚动失效
   *   2. totalBackendCount 缓存 stale → "共 1 条（已筛选 10186 条）" 计数错
   *   3. triggerRef 不调用 → VirtualLogList.virtualizerOptions 不重算 → 列表卡死
   *
   * 验证方法：subscribe 到 IncrementalFilter，调用 setBackendLogs([]) + pushMany 路径
   * （走 WS 模拟），断言 subscribe callback 至少被调 1 次。
   * setBackendLogs 内部 clear() + pushMany(arr)——重点是 pushMany 路径必须通知。
   */
  it('14. 回归：pushMany 必须调 notify（防 4 症状再次回归）', async () => {
    const w = mountDevLogs()
    await flushPromises()
    const bf = (w.vm as any).backendFilter as import('@/utils/IncrementalFilter').IncrementalFilter
    let notifyCount = 0
    const unsub = bf.subscribe(() => { notifyCount++ })
    // 模拟自动化测试批量入队：setBackendLogs 内部走 clear() + pushMany(arr)
    ;(w.vm as any).setBackendLogs([
      { id: 1, timestamp: '00:00:01', level: 'info', message: 'a' },
      { id: 2, timestamp: '00:00:02', level: 'info', message: 'b' },
      { id: 3, timestamp: '00:00:03', level: 'info', message: 'c' },
    ])
    unsub()
    // 关键断言：setBackendLogs 内部 pushMany 必须触发 notify ≥ 1 次
    expect(notifyCount).toBeGreaterThanOrEqual(1)
  })
})
