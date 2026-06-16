/**
 * preflight.ts — gateway 启动前的预检
 * =====================================================
 *
 * 2026-06-14 修复：上次 2026-06-10 改 noop 是"删过头"——
 *   - mock 数据生成职责：删（确实由用户主动调 /api/mock/generate，带 X-Confirm-Mock-Mutation header）
 *   - 空目录占位职责：必须保留！service-guard 硬约束 servingDir=/storage/emulated/0
 *     （mobile_api.go:209-263），目录不存在 → os.Stat 失败 → service-guard BLOCK。
 *     mobile 真机不需要这个目录（设备自带），但 dev preview 沙箱里必须建。
 *   - 当前职责：mkdir -p /storage/emulated/0（空目录，service-guard 不查内容）
 */

const LOG_PREFIX = '[preflight]'

function log(...args: unknown[]): void {
  console.log(LOG_PREFIX, ...args)
}

import { mkdir } from 'node:fs/promises'
import { existsSync } from 'node:fs'

/**
 * 2026-06-14 修复版：建空 /storage/emulated/0（service-guard 要求 servingDir 是这个路径）。
 * mock 数据生成职责已迁至用户主动调后端 /api/mock/generate（带 X-Confirm-Mock-Mutation header），
 * 不在本 preflight 范围。
 */
export async function ensureMockData(mobileDir: string): Promise<void> {
  if (existsSync(mobileDir)) {
    log(`(skip) ${mobileDir} already exists`)
    return
  }
  try {
    await mkdir(mobileDir, { recursive: true })
    log(`(created) ${mobileDir}`)
  } catch (err) {
    log(`(warn) failed to create ${mobileDir}:`, err)
  }
}
