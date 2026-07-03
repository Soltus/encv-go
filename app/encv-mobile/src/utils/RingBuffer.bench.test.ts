/**
 * RingBuffer + IncrementalFilter 压力测试
 * =====================================================
 * 实测极限能力：找出 1M / 5M / 10M 下的性能断点
 * 输出：每秒 push 数 / 过滤 / slice / 内存估算
 *
 * 用法（vitest）：
 *   npm test -- RingBuffer.bench
 *
 * 用法（手测 dev）：
 *   node --experimental-vm-modules ./src/utils/RingBuffer.bench.ts
 *
 * 2026-06-15 创建
 */

import { describe, it, expect } from 'vitest'
import { RingBuffer } from './RingBuffer'
import { IncrementalFilter, type LogEntry } from './IncrementalFilter'

function makeLogEntry(i: number): LogEntry {
  const levels: LogEntry['level'][] = ['debug', 'info', 'warn', 'error']
  return {
    id: i,
    timestamp: '12:34:56',
    level: levels[i % 4],
    message: `log entry ${i}: some sample message with random text ${Math.random()}`,
  }
}

function measure<T>(label: string, fn: () => T): { label: string; ms: number; result?: T } {
  const t0 = performance.now()
  const result = fn()
  const ms = performance.now() - t0
  return { label, ms, result }
}

describe('RingBuffer 边界 / 正确性', () => {
  it('push + toArray 顺序正确', () => {
    const rb = new RingBuffer<number>(5)
    rb.push(1); rb.push(2); rb.push(3)
    expect(rb.toArray()).toEqual([1, 2, 3])
    expect(rb.size).toBe(3)
  })

  it('满了之后 push 覆盖最旧', () => {
    const rb = new RingBuffer<number>(3)
    rb.push(1); rb.push(2); rb.push(3); rb.push(4)
    expect(rb.toArray()).toEqual([2, 3, 4])
    expect(rb.size).toBe(3)
  })

  it('wrap-around 顺序正确（capacity=4, push 6 次）', () => {
    const rb = new RingBuffer<number>(4)
    for (let i = 1; i <= 6; i++) rb.push(i)
    expect(rb.toArray()).toEqual([3, 4, 5, 6])
  })

  it('clear 后从头开始', () => {
    const rb = new RingBuffer<number>(3)
    rb.push(1); rb.push(2); rb.push(3)
    rb.clear()
    expect(rb.size).toBe(0)
    expect(rb.toArray()).toEqual([])
    rb.push(99)
    expect(rb.toArray()).toEqual([99])
  })

  it('at 索引访问', () => {
    const rb = new RingBuffer<string>(3)
    rb.push('a'); rb.push('b'); rb.push('c')
    expect(rb.at(0)).toBe('a')
    expect(rb.at(2)).toBe('c')
    expect(rb.at(3)).toBeUndefined()
    expect(rb.at(-1)).toBeUndefined()
  })

  it('forEach 可提前终止', () => {
    const rb = new RingBuffer<number>(100)
    for (let i = 0; i < 100; i++) rb.push(i)
    const seen: number[] = []
    rb.forEach((v) => { seen.push(v); if (v >= 5) return false })
    expect(seen).toEqual([0, 1, 2, 3, 4, 5])
  })
})

describe('IncrementalFilter 正确性', () => {
  it('默认全过', () => {
    const f = new IncrementalFilter(10)
    f.push(makeLogEntry(1))
    f.push(makeLogEntry(2))
    expect(f.length).toBe(2)
  })

  it('等级过滤（仅 info）', () => {
    const f = new IncrementalFilter(100)
    for (let i = 0; i < 10; i++) f.push(makeLogEntry(i))
    // entries 0,4,8 是 info（i%4 === 0）
    f.setFilter({ levels: new Set(['info']), searchLower: '', tags: new Set() })
    expect(f.length).toBe(3)
  })

  it('搜索过滤', () => {
    const f = new IncrementalFilter(100)
    f.push({ id: 1, timestamp: '', level: 'info', message: 'hello world' })
    f.push({ id: 2, timestamp: '', level: 'info', message: 'goodbye' })
    f.setFilter({ levels: new Set(['all']), searchLower: 'hello', tags: new Set() })
    expect(f.length).toBe(1)
  })

  it('增量：新 item 自动按当前 filter 评估', () => {
    const f = new IncrementalFilter(100)
    f.setFilter({ levels: new Set(['error']), searchLower: '', tags: new Set() })
    f.push({ id: 1, timestamp: '', level: 'info', message: 'x' })
    f.push({ id: 2, timestamp: '', level: 'error', message: 'x' })
    expect(f.length).toBe(1)
  })

  it('wrap-around 后仍能增量（processed 落后 < capacity）', () => {
    const f = new IncrementalFilter(5)
    for (let i = 0; i < 8; i++) f.push(makeLogEntry(i))
    // source 满后丢了最早的；processed 跟随 push
    expect(f.totalLength).toBe(5)
    // 4/4=0 (info), 5/4=1 (info), 6/4=2 (warn), 7/4=3 (error) → 8 个中 4 个过默认
    expect(f.length).toBeGreaterThan(0)
  })
})

describe('🔥 极限能力实测（关键验收）', () => {
  it('100K: push + 默认过滤 + slice 的总耗时', () => {
    const N = 100_000
    const f = new IncrementalFilter(N)
    const m1 = measure(`push ${N}`, () => {
      for (let i = 0; i < N; i++) f.push(makeLogEntry(i))
    })
    const m2 = measure('read result length', () => f.length)
    const m3 = measure('toArray()', () => f.source.toArray())

    // eslint-disable-next-line no-console
    console.log(`\n=== 100K ===`)
    // eslint-disable-next-line no-console
    console.log(`  ${m1.label}: ${m1.ms.toFixed(1)}ms (${(N / m1.ms * 1000).toFixed(0)} ops/sec)`)
    // eslint-disable-next-line no-console
    console.log(`  ${m2.label}: ${m2.ms.toFixed(3)}ms`)
    // eslint-disable-next-line no-console
    console.log(`  ${m3.label}: ${m3.ms.toFixed(1)}ms (${(N / m3.ms * 1000).toFixed(0)} ops/sec)`)
    // eslint-disable-next-line no-console
    console.log(`  est memory: ${(f.source.estimateBytes() / 1024 / 1024).toFixed(1)}MB`)

    expect(f.totalLength).toBe(N)
    expect(m1.ms).toBeLessThan(1000) // 100K push < 1s
  })

  it('1M: push 性能 + 内存估算', () => {
    const N = 1_000_000
    const f = new IncrementalFilter(N)
    const m1 = measure(`push ${N}`, () => {
      for (let i = 0; i < N; i++) f.push(makeLogEntry(i))
    })
    const memMB = f.source.estimateBytes() / 1024 / 1024

    // eslint-disable-next-line no-console
    console.log(`\n=== 1M (用户目标) ===`)
    // eslint-disable-next-line no-console
    console.log(`  ${m1.label}: ${m1.ms.toFixed(0)}ms (${(N / m1.ms * 1000).toFixed(0)} ops/sec)`)
    // eslint-disable-next-line no-console
    console.log(`  est memory: ${memMB.toFixed(1)}MB`)
    // eslint-disable-next-line no-console
    console.log(`  filtered length: ${f.length}`)

    expect(f.totalLength).toBe(N)
    // 1M push < 5s（容许）
    expect(m1.ms).toBeLessThan(5000)
    // 内存估算 < 200MB（用户设备基本能撑住）
    expect(memMB).toBeLessThan(200)
  })

  it('1M: filter 切换成本（一次性 O(N)）', () => {
    const N = 1_000_000
    const f = new IncrementalFilter(N)
    for (let i = 0; i < N; i++) f.push(makeLogEntry(i))

    const m1 = measure('rebuild filter to error-only', () => {
      f.setFilter({ levels: new Set(['error']), searchLower: '', tags: new Set() })
    })
    // eslint-disable-next-line no-console
    console.log(`\n=== 1M filter rebuild ===`)
    // eslint-disable-next-line no-console
    console.log(`  ${m1.label}: ${m1.ms.toFixed(0)}ms`)
    // eslint-disable-next-line no-console
    console.log(`  filtered: ${f.length} (25% expected)`)

    expect(m1.ms).toBeLessThan(2000) // 1M filter < 2s
    // 4 个 level 均匀分布 → 1M × 0.25 = 250K
    expect(f.length).toBeGreaterThan(200_000)
    expect(f.length).toBeLessThan(300_000)
  })

  it('10M: RingBuffer 自身容量极限（绕过 IncrementalFilter 不保留 result）', () => {
    // IncrementalFilter 会保留 10M 项 result（300MB+jsdom → 4GB 堆爆）
    // 这里只测 RingBuffer 自身（只占 capacity × 8 bytes 指针）能否支撑 10M
    const N = 10_000_000
    const rb = new RingBuffer<number>(N)

    const CHUNK = 100_000
    const startT = performance.now()
    for (let c = 0; c < N; c += CHUNK) {
      const m = measure(`chunk ${c}`, () => {
        for (let i = 0; i < CHUNK && c + i < N; i++) rb.push(c + i)
      })
      void m
    }
    const totalMs = performance.now() - startT

    // eslint-disable-next-line no-console
    console.log(`\n=== 10M RingBuffer only (no result) ===`)
    // eslint-disable-next-line no-console
    console.log(`  total: ${totalMs.toFixed(0)}ms (${(N / totalMs * 1000).toFixed(0)} ops/sec)`)
    // eslint-disable-next-line no-console
    console.log(`  est memory: ${(rb.estimateBytes() / 1024 / 1024).toFixed(1)}MB`)
    // eslint-disable-next-line no-console
    console.log(`  size: ${rb.size}`)

    expect(rb.size).toBe(N)
    // 10M push < 15s（容许；用户 1M 目标下 ~1s 已验证）
    expect(totalMs).toBeLessThan(15_000)
  }, 90_000) // 90s timeout

  it('FPS: 100 条/秒 × 10 分钟 = 60K 条；最终过滤 + 读取耗时', () => {
    const f = new IncrementalFilter(1_000_000) // 用 1M 容量
    const FRAME_MS = 1000 / 60 // 60 FPS → 16.67ms/frame

    // 模拟 60K 条以 100 条/秒（每帧 ~1.67 条）推入
    const startT = performance.now()
    const FRAMES = 600
    for (let frame = 0; frame < FRAMES; frame++) {
      // 每帧 push 100 条（6 条/帧 × ~1 帧是 100 条/帧的近似）
      // 实际场景：100 条/秒 @ 60fps = 1.67 条/帧，但 batch 在 WS 收到时是 burst
      // 用 100 条/帧压测
      for (let i = 0; i < 100; i++) {
        f.push(makeLogEntry(frame * 100 + i))
      }
      // 模拟一帧的其它工作
    }
    const totalMs = performance.now() - startT

    // 模拟过滤切换（用户点击 level 按钮）
    const filterStart = performance.now()
    f.setFilter({ levels: new Set(['info', 'warn']), searchLower: '', tags: new Set() })
    const filterMs = performance.now() - filterStart

    // 模拟 60 帧的 read + 渲染
    const renderStart = performance.now()
    for (let frame = 0; frame < 60; frame++) {
      void f.length
      void f.getResult()
    }
    const renderMs = (performance.now() - renderStart) / 60

    // eslint-disable-next-line no-console
    console.log(`\n=== 60K 持续流式 + 切 filter ===`)
    // eslint-disable-next-line no-console
    console.log(`  total push (600 frames × 100): ${totalMs.toFixed(0)}ms`)
    // eslint-disable-next-line no-console
    console.log(`  filter switch: ${filterMs.toFixed(0)}ms`)
    // eslint-disable-next-line no-console
    console.log(`  per-frame read+render: ${renderMs.toFixed(2)}ms (budget ${FRAME_MS.toFixed(2)}ms)`)
    // eslint-disable-next-line no-console
    console.log(`  filtered: ${f.length}`)

    // 单帧读取必须 < 16.67ms（60FPS 预算）
    expect(renderMs).toBeLessThan(FRAME_MS)
  })
})
