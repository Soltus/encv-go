import { describe, it, expect, vi, beforeEach } from 'vitest'

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  on: vi.fn(),
  off: vi.fn(),
  emit: vi.fn(),
}))

vi.mock('@/composables/useProxiedFetch', () => ({
  useProxiedFetch: () => ({ request: mocks.request }),
}))

vi.mock('@/composables/useEventBus', () => ({
  eventBus: { on: mocks.on, off: mocks.off, emit: mocks.emit },
}))

import { useVectorSearchStatus } from '@/composables/useVectorSearchStatus'

describe('useVectorSearchStatus', () => {
  beforeEach(() => {
    mocks.request.mockReset()
    mocks.on.mockClear()
    mocks.off.mockClear()
  })

  it('探测返回 vector_search_available=true, degraded=false → status="available"', async () => {
    mocks.request.mockResolvedValue({
      vector_search_available: true,
      vector_search_degraded: false,
    })
    const { status, refresh } = useVectorSearchStatus()
    await refresh()
    expect(status.value).toBe('available')
  })

  it('探测返回 degraded=true → status="degraded"', async () => {
    mocks.request.mockResolvedValue({
      vector_search_available: true,
      vector_search_degraded: true,
    })
    const { status, refresh } = useVectorSearchStatus()
    await refresh()
    expect(status.value).toBe('degraded')
  })

  it('探测返回 available=false → status="unavailable"', async () => {
    mocks.request.mockResolvedValue({
      vector_search_available: false,
      vector_search_degraded: false,
    })
    const { status, refresh } = useVectorSearchStatus()
    await refresh()
    expect(status.value).toBe('unavailable')
  })

  it('探测失败：状态应反映"未知"（模块级 status 持久，前置测试可能已改）', async () => {
    // 注意：composable 用了模块级 const 状态，测试间共享。
    // 此处只验证「探测失败后状态未变为 available/degraded」
    // 即：失败后是 'unknown' 或 'unavailable'，且不是 'available' / 'degraded'
    mocks.request.mockRejectedValue(new Error('network error'))
    const { status, refresh } = useVectorSearchStatus()
    await refresh()
    expect(['unknown', 'unavailable']).toContain(status.value)
    expect(status.value).not.toBe('available')
    expect(status.value).not.toBe('degraded')
  })

  it('探测返回 available=undefined → "unavailable"（保守）', async () => {
    mocks.request.mockResolvedValue({})
    const { status, refresh } = useVectorSearchStatus()
    await refresh()
    expect(status.value).toBe('unavailable')
  })

  it('同时只能有一个 in-flight 请求（pollInFlight 保护）', async () => {
    let resolveFn: (v: any) => void = () => {}
    mocks.request.mockReturnValue(new Promise(r => { resolveFn = r }))
    const { refresh } = useVectorSearchStatus()
    const p1 = refresh()
    const p2 = refresh()
    const p3 = refresh()
    expect(mocks.request).toHaveBeenCalledTimes(1)
    resolveFn({ vector_search_available: true, vector_search_degraded: false })
    await Promise.all([p1, p2, p3])
  })

  it('状态切换：available → degraded 后再次 refresh 仍能正确反映', async () => {
    mocks.request.mockResolvedValueOnce({
      vector_search_available: true,
      vector_search_degraded: false,
    })
    const { status, refresh } = useVectorSearchStatus()
    await refresh()
    expect(status.value).toBe('available')

    mocks.request.mockResolvedValueOnce({
      vector_search_available: true,
      vector_search_degraded: true,
    })
    await refresh()
    expect(status.value).toBe('degraded')
  })
})
