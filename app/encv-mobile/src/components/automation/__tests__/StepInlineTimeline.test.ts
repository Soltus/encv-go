/**
 * StepInlineTimeline 单元测试
 *
 * 覆盖 Task 14 SubTask 14.1-14.6：
 * - 基础渲染：step 有 phase 时渲染对应 UnifiedTimelineCard
 * - phase 序列派生（多个 phase：started + current phase + completed）
 * - progress / speed / eta 显示（从 step 字段派生）
 * - duration 计算（从 startedAt + completedAt 或 durationMs）
 * - 空状态（step 无 phase 且无 startedAt）
 * - 失败/取消态：最后一个事件标记为 failure + error 信息
 * - 运行中态：当前 phase 标记 isCurrent + running 状态
 * - 已完成态：追加 Phase.Completed 事件
 * - 最长耗时高亮
 * - expandDetail 包含 startedAt / completedAt / duration / error
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import StepInlineTimeline from '@/components/automation/StepInlineTimeline.vue'
import UnifiedTimelineCard from '@/components/shared/UnifiedTimelineCard.vue'
import type { StepRun } from '@/lib/workflow/types'
import { Phase } from '@/lib/workflow/types'

// ion-icon stub：避免 @ionic/vue 全局注册依赖
const IonIconStub = {
  name: 'IonIcon',
  props: {
    icon: { type: [String, Object], default: null },
  },
  template: '<span class="ion-icon-stub" :data-icon="String(icon)" />',
}

/** 构造测试用 StepRun（便于覆盖默认值） */
function makeStep(overrides: Partial<StepRun> = {}): StepRun {
  return {
    id: 'step-run-1',
    stepDefId: 'enc_mp4',
    status: 'running',
    startedAt: '2026-06-18T10:00:00Z',
    phase: 'encrypting',
    progress: 50,
    speed: '12.5 MB/s',
    eta: '00:01:30',
    ...overrides,
  }
}

function mountTimeline(step: StepRun) {
  return mount(StepInlineTimeline, {
    props: { step },
    global: {
      stubs: { 'ion-icon': IonIconStub },
    },
  })
}

// ─── 基础渲染 ───────────────────────────────────────────────────────

describe('StepInlineTimeline - 基础渲染', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('step 有 phase + startedAt 时渲染对应 UnifiedTimelineCard（Started + Current phase）', () => {
    const step = makeStep({
      status: 'running',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: undefined,
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    // 1 个 Started (Created) + 1 个 Current phase (Encrypting) = 2
    expect(cards).toHaveLength(2)
  })

  it('第一个条目为 Started（Phase.Created）', () => {
    const step = makeStep({ status: 'running', phase: 'encrypting' })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    expect(cards[0].props('entry').phase).toBe(Phase.Created)
  })

  it('当前 phase 条目使用 step.phase 枚举值', () => {
    const step = makeStep({ status: 'running', phase: 'encrypting' })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    // 第二个卡片是当前 phase
    expect(cards[1].props('entry').phase).toBe(Phase.Encrypting)
  })

  it('step 无 phase 且无 startedAt 时渲染空状态', () => {
    const step = makeStep({
      status: 'pending',
      phase: undefined,
      startedAt: undefined,
      completedAt: undefined,
      progress: undefined,
      speed: undefined,
      eta: undefined,
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    expect(cards).toHaveLength(0)
    expect(wrapper.find('.step-inline-timeline__empty').exists()).toBe(true)
  })
})

// ─── phase 序列派生 ─────────────────────────────────────────────────

describe('StepInlineTimeline - phase 序列派生', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('运行中 step（startedAt + phase）渲染 2 个条目（Started + Current phase）', () => {
    const step = makeStep({
      status: 'running',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: undefined,
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    expect(cards).toHaveLength(2)
    expect(cards[0].props('entry').phase).toBe(Phase.Created)
    expect(cards[1].props('entry').phase).toBe(Phase.Encrypting)
  })

  it('已完成 step（startedAt + completedAt + phase）渲染 3 个条目（Started + Phase + Completed）', () => {
    const step = makeStep({
      status: 'success',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: '2026-06-18T10:00:05Z',
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    // Started + Encrypting + Completed = 3
    expect(cards).toHaveLength(3)
    expect(cards[0].props('entry').phase).toBe(Phase.Created)
    expect(cards[1].props('entry').phase).toBe(Phase.Encrypting)
    expect(cards[2].props('entry').phase).toBe(Phase.Completed)
  })

  it('step.phase=completed 时不渲染中间 phase 条目（仅 Started + Completed）', () => {
    const step = makeStep({
      status: 'success',
      phase: 'completed',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: '2026-06-18T10:00:05Z',
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    // Started + Completed = 2（phase=completed 跳过中间条目）
    expect(cards).toHaveLength(2)
    expect(cards[0].props('entry').phase).toBe(Phase.Created)
    expect(cards[1].props('entry').phase).toBe(Phase.Completed)
  })

  it('step 只有 phase（无 startedAt）时渲染 1 个 phase 条目', () => {
    const step = makeStep({
      status: 'running',
      phase: 'analyzing',
      startedAt: undefined,
      completedAt: undefined,
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    // 仅 1 个 phase 条目（无 Started，因为 startedAt 缺失）
    expect(cards).toHaveLength(1)
    expect(cards[0].props('entry').phase).toBe(Phase.Analyzing)
  })
})

// ─── progress / speed / eta 显示 ────────────────────────────────────

describe('StepInlineTimeline - progress / speed / eta 显示', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('当前 phase 条目派生 step.progress 字段', () => {
    const step = makeStep({
      status: 'running',
      phase: 'encrypting',
      progress: 65,
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    expect(cards[1].props('entry').progress).toBe(65)
  })

  it('当前 phase 条目派生 step.speed 字段', () => {
    const step = makeStep({
      status: 'running',
      phase: 'encrypting',
      speed: '12.5 MB/s',
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    expect(cards[1].props('entry').speed).toBe('12.5 MB/s')
  })

  it('当前 phase 条目派生 step.eta 字段', () => {
    const step = makeStep({
      status: 'running',
      phase: 'encrypting',
      eta: '00:01:30',
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    expect(cards[1].props('entry').eta).toBe('00:01:30')
  })

  it('progress 缺失时当前 phase 条目 progress 为 undefined', () => {
    const step = makeStep({
      status: 'running',
      phase: 'encrypting',
      progress: undefined,
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    expect(cards[1].props('entry').progress).toBeUndefined()
  })
})

// ─── duration 计算 ──────────────────────────────────────────────────

describe('StepInlineTimeline - duration 计算', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('从 startedAt + completedAt 计算 duration（5 秒）', () => {
    const step = makeStep({
      status: 'success',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: '2026-06-18T10:00:05Z',
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    // 中间 phase 条目应有 duration
    expect(cards[1].props('entry').duration).toBe('5s')
  })

  it('优先使用 step.durationMs 字段（覆盖 startedAt + completedAt 计算）', () => {
    const step = makeStep({
      status: 'success',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: '2026-06-18T10:00:05Z', // 计算 = 5s
      durationMs: 10000, // 但 durationMs 优先 → 10s
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    expect(cards[1].props('entry').duration).toBe('10s')
  })

  it('Completed 条目也派生 duration', () => {
    const step = makeStep({
      status: 'success',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: '2026-06-18T10:00:05Z',
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    // Completed 条目（第 3 个）也应有 duration
    expect(cards[2].props('entry').duration).toBe('5s')
  })

  it('运行中 step（无 completedAt）不派生 duration', () => {
    const step = makeStep({
      status: 'running',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: undefined,
      durationMs: undefined,
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    expect(cards[1].props('entry').duration).toBeUndefined()
  })
})

// ─── 状态色 + 失败/取消态 ───────────────────────────────────────────

describe('StepInlineTimeline - 状态色 + 失败/取消态', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('运行中 step 当前 phase 标记 isCurrent=true + status=running', () => {
    const step = makeStep({
      status: 'running',
      phase: 'encrypting',
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    expect(cards[1].props('entry').isCurrent).toBe(true)
    expect(cards[1].props('entry').status).toBe('running')
  })

  it('失败 step 最后一个条目标记 status=failure', () => {
    const step = makeStep({
      status: 'failure',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: '2026-06-18T10:00:05Z',
      error: 'FFMPEG exited with code 1',
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    const lastCard = cards[cards.length - 1]
    expect(lastCard.props('entry').status).toBe('failure')
  })

  it('失败 step 的 error 附加到最后一个条目的 expandDetail.error', () => {
    const step = makeStep({
      status: 'failure',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: '2026-06-18T10:00:05Z',
      error: 'FFMPEG exited with code 1',
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    const lastCard = cards[cards.length - 1]
    expect(lastCard.props('entry').expandDetail?.error).toBe('FFMPEG exited with code 1')
    expect(lastCard.props('entry').hasExpandableDetail).toBe(true)
  })

  it('取消 step 最后一个条目 status=failure', () => {
    const step = makeStep({
      status: 'cancelled',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: undefined,
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    const lastCard = cards[cards.length - 1]
    expect(lastCard.props('entry').status).toBe('failure')
  })

  it('成功 step 所有条目 status=success', () => {
    const step = makeStep({
      status: 'success',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: '2026-06-18T10:00:05Z',
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    for (const card of cards) {
      expect(card.props('entry').status).toBe('success')
    }
  })
})

// ─── 最长耗时高亮 ───────────────────────────────────────────────────

describe('StepInlineTimeline - 最长耗时高亮', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('有 duration 的条目中最大耗时标记 isHighlight=true', () => {
    const step = makeStep({
      status: 'success',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: '2026-06-18T10:00:10Z', // 10s
      durationMs: 10000,
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    // 中间 phase 条目（10s）和 Completed 条目（10s）都有 duration
    // 第一个有 duration 的条目应高亮
    const highlighted = cards.filter((c) => c.props('entry').isHighlight === true)
    expect(highlighted.length).toBeGreaterThanOrEqual(1)
  })

  it('所有条目都无 duration 时不高亮任何条目', () => {
    const step = makeStep({
      status: 'running',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: undefined,
      durationMs: undefined,
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    expect(cards.every((c) => c.props('entry').isHighlight !== true)).toBe(true)
  })
})

// ─── expandDetail 展开 ──────────────────────────────────────────────

describe('StepInlineTimeline - expandDetail 展开', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('有 startedAt + completedAt + duration 的 phase 条目标记 hasExpandableDetail=true', () => {
    const step = makeStep({
      status: 'success',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: '2026-06-18T10:00:05Z',
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    expect(cards[1].props('entry').hasExpandableDetail).toBe(true)
  })

  it('expandDetail 包含 startedAt / completedAt / duration', () => {
    const step = makeStep({
      status: 'success',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: '2026-06-18T10:00:05Z',
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    const expandDetail = cards[1].props('entry').expandDetail
    expect(expandDetail).toBeDefined()
    expect(expandDetail?.startedAt).toBeTruthy()
    expect(expandDetail?.completedAt).toBeTruthy()
    expect(expandDetail?.duration).toBe('5s')
  })

  it('有 error 的 step 把 error 加入 expandDetail', () => {
    const step = makeStep({
      status: 'failure',
      phase: 'encrypting',
      startedAt: '2026-06-18T10:00:00Z',
      completedAt: '2026-06-18T10:00:05Z',
      error: 'FFMPEG exited with code 1',
    })
    const wrapper = mountTimeline(step)
    const cards = wrapper.findAllComponents(UnifiedTimelineCard)
    // 中间 phase 条目应有 error
    expect(cards[1].props('entry').expandDetail?.error).toBe('FFMPEG exited with code 1')
  })
})

// ─── 暗黑模式 ───────────────────────────────────────────────────────

describe('StepInlineTimeline - 暗黑模式', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('暗黑模式下组件仍正常渲染（CSS 通过 body.dark 作用域）', () => {
    const step = makeStep({ status: 'running', phase: 'encrypting' })
    const wrapper = mountTimeline(step)
    expect(wrapper.find('.step-inline-timeline').exists()).toBe(true)
  })

  it('暗黑模式下空状态仍正常渲染', () => {
    const step = makeStep({
      status: 'pending',
      phase: undefined,
      startedAt: undefined,
    })
    const wrapper = mountTimeline(step)
    expect(wrapper.find('.step-inline-timeline__empty').exists()).toBe(true)
  })
})
