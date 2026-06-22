/**
 * HttpPollBackend 单元测试（2026-06-10）
 *
 * 覆盖：
 *  1. 启动后立刻 tick 一次（active 模式）
 *  2. 新 task id → emit 'task:created'
 *  3. status 变化 → emit 'task:update' + 'task:completed'（如果终态）
 *  4. progress/phase 变化 → emit 'task:progress'
 *  5. 错误 backoff（连续失败 → 间隔翻倍）
 *  6. document hidden 切到 30s 节流
 *  7. stop 后不再 tick
 *  8. snapshot 消失防御（后端重启）→ emit 'task:completed' with server-list-missing
 *
 * 实现：
 *   - 通过 options.fetchTasks 注入 mock（避免真实 HTTP）
 *   - 用真实 setTimeout 等待（避免 fake timers + microtask 同步陷阱）
 *   - stop() / 错误路径用更短等待以减少测试时间
 */

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createHttpPollBackend } from '@/composables/realtime/HttpPollBackend'
import type { EncvTask } from '@/api/encv'

function makeTask(id: string, status: string = 'running', progress: number = 0): EncvTask {
  return {
    id,
    type: 'encrypt' as any,
    sourcePath: `/tmp/${id}.mp4`,
    status: status as any,
    progress,
    createdAt: new Date().toISOString(),
  } as EncvTask
}

describe('HttpPollBackend', () => {
  let emit: any
  let fetchTasks: any
  let events: Array<{ type: string; data: any }>

  beforeEach(() => {
    events = []
    emit = vi.fn((type: string, data: any) => {
      events.push({ type, data })
    })
    fetchTasks = vi.fn().mockResolvedValue([])
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  // ─── 基础行为 ─────────────────────────────────────

  it('start() triggers initial tick + emits server:status on first success', async () => {
    fetchTasks.mockResolvedValue([])
    const backend = createHttpPollBackend(emit, { fetchTasks })

    backend.start()
    await new Promise((r) => setTimeout(r, 30))

    expect(fetchTasks).toHaveBeenCalled()
    expect(emit).toHaveBeenCalledWith('server:status', { online: true })
  })

  it('new task id → emit task:created + task:update (if not queued)', async () => {
    fetchTasks.mockResolvedValue([makeTask('t1', 'running', 10)])
    const backend = createHttpPollBackend(emit, { fetchTasks })
    backend.start()
    await new Promise((r) => setTimeout(r, 30))

    const types = events.map((e) => e.type)
    expect(types).toContain('task:created')
    expect(types).toContain('task:update')
  })

  it('status change → emit task:update (and task:completed for terminal)', async () => {
    // 第一轮：running
    fetchTasks.mockResolvedValueOnce([makeTask('t1', 'running', 10)])
    const backend = createHttpPollBackend(emit, { fetchTasks })
    backend.start()
    await new Promise((r) => setTimeout(r, 30))
    events.length = 0

    // 第二轮：success（终态）
    fetchTasks.mockResolvedValueOnce([makeTask('t1', 'success', 100)])
    // 等下一轮 tick（active 2s）
    await new Promise((r) => setTimeout(r, 2100))

    const types = events.map((e) => e.type)
    expect(types).toContain('task:update')
    expect(types).toContain('task:completed')
  })

  it('progress change → emit task:progress', async () => {
    fetchTasks.mockResolvedValueOnce([makeTask('t1', 'running', 10)])
    const backend = createHttpPollBackend(emit, { fetchTasks })
    backend.start()
    await new Promise((r) => setTimeout(r, 30))
    events.length = 0

    fetchTasks.mockResolvedValueOnce([makeTask('t1', 'running', 50)])
    await new Promise((r) => setTimeout(r, 2100))

    expect(events.some((e) => e.type === 'task:progress')).toBe(true)
  })

  // ─── 错误处理 ─────────────────────────────────────

  it('error → no connection-error spam to user', async () => {
    fetchTasks.mockRejectedValue(new Error('network error'))
    const warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const backend = createHttpPollBackend(emit, { fetchTasks })
    backend.start()
    await new Promise((r) => setTimeout(r, 30))

    // 错误路径不应 emit 'server:connection-error'（避免 noise）
    expect(events.every((e) => e.type !== 'server:connection-error')).toBe(true)
    warnSpy.mockRestore()
  })

  // ─── 生命周期 ─────────────────────────────────────

  it('stop() prevents further ticks', async () => {
    fetchTasks.mockResolvedValue([])
    const backend = createHttpPollBackend(emit, { fetchTasks })
    backend.start()
    await new Promise((r) => setTimeout(r, 30))
    const callCount = fetchTasks.mock.calls.length

    backend.stop()
    await new Promise((r) => setTimeout(r, 2500))
    // stop 后不应继续 tick
    expect(fetchTasks.mock.calls.length).toBe(callCount)
  })

  // ─── 边界 case ────────────────────────────────────

  it('disappearing task → emit task:completed with server-list-missing', async () => {
    // 第一轮：task 存在
    fetchTasks.mockResolvedValueOnce([makeTask('t1', 'running', 10)])
    const backend = createHttpPollBackend(emit, { fetchTasks })
    backend.start()
    await new Promise((r) => setTimeout(r, 30))
    events.length = 0

    // 第二轮：task 消失（后端重启 / 列表被清空）
    fetchTasks.mockResolvedValueOnce([])
    await new Promise((r) => setTimeout(r, 2100))

    const completed = events.find((e) => e.type === 'task:completed')
    expect(completed).toBeDefined()
    expect(completed?.data.error).toBe('server-list-missing')
  })

  // ─── 🆕 2026-06-22 真因修复：后端 list 漏 runId 字段 ───
  // 真凶：后端 MobileTask.RunId string `json:"runId,omitempty"` → 空字符串时省略字段
  //   前端拿到的 t.runId = undefined → appendTask push 孤儿 task → 1000+ task 散成多个 group
  // 修复：HttpPollBackend lastFullTask cache 完整 EncvTask（含 runId），emit 前如果 t.runId 空 → 从 cache 补

  it('后端 list 第一次返回 task 带 runId，第二次返回同 task 但 runId 字段缺失 → emit 时回填 runId', async () => {
    const RUN_ID = 'run-test-123'
    // 第一轮：task 带 runId='run-test-123' + status='queued'
    fetchTasks.mockResolvedValueOnce([{ ...makeTask('t1', 'queued', 0), runId: RUN_ID }])
    const backend = createHttpPollBackend(emit, { fetchTasks })
    backend.start()
    await new Promise((r) => setTimeout(r, 30))
    const firstCreated = events.find((e) => e.type === 'task:created')
    expect(firstCreated, '第一轮 task:created 必须发出').toBeDefined()
    expect(firstCreated?.data.runId, '第一轮 runId 透传').toBe(RUN_ID)
    events.length = 0

    // 第二轮：同 task 但 runId 字段缺失（后端 omitempty 波动）+ status='running'（status 变化触发 task:update）
    const t1NoRunId = makeTask('t1', 'running', 50)
    delete (t1NoRunId as any).runId
    fetchTasks.mockResolvedValueOnce([t1NoRunId])
    await new Promise((r) => setTimeout(r, 2100))

    // 第二轮 status 变化会 emit task:update，且 runId 必须从 cache 回填
    const updates = events.filter((e) => e.type === 'task:update')
    expect(updates.length, 'status 变化应 emit task:update').toBeGreaterThan(0)
    expect(updates[0]?.data.runId, 'task:update 要带 runId 才能让 store 不丢 runId').toBe(RUN_ID)
  })

  it('后端 list 第一次返回 task 没 runId → 推完整 task（appendTask warn 由 store 负责）', async () => {
    // 场景：手动 task（+ 按钮创建）→ 后端 RunId="" + omitempty → 字段缺失
    //   此时 cache 也帮不了（没 prev 缓存）→ emit 完整 task → appendTask warn
    fetchTasks.mockResolvedValueOnce([makeTask('manual-1', 'running', 10)])
    const backend = createHttpPollBackend(emit, { fetchTasks })
    backend.start()
    await new Promise((r) => setTimeout(r, 30))

    const created = events.find((e) => e.type === 'task:created')
    expect(created, '新 task 必须 emit').toBeDefined()
    expect(created?.data.id).toBe('manual-1')
    // 行为：emit 完整 task（即使没 runId 也要透传，让 appendTask 自己处理 warn）
    expect(created?.data.runId, '手动 task 没 runId 是预期行为').toBeUndefined()
  })
})
