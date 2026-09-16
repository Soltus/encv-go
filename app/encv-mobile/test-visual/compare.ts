// 用 bun 运行：bun test-visual/compare.ts
// 视觉回归对比工具（ESM: pixelmatch@7 + pngjs@7），也可被 Playwright spec 直接 import。
//
// 首次运行（baseline 不存在）：写入 baseline，返回 { matched:true, firstRun:true }
// 之后运行：pixelmatch 比对 candidate 与 baseline，
//   mismatch 比例 > threshold 则写出 diff 图并返回 matched:false。
import { readFileSync, writeFileSync, existsSync, mkdirSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { dirname } from 'node:path'
import pixelmatch from 'pixelmatch'
import { PNG } from 'pngjs'

const __dirname = dirname(fileURLToPath(import.meta.url))
export const VISUAL_DIR = path.resolve(__dirname, '..', 'cypress', 'visual')

export interface CompareResult {
  matched: boolean
  firstRun?: boolean
  mismatchPixels?: number
  mismatchPercent?: number
  diffPath?: string
}

export function compareSnapshot(
  name: string,
  candidatePath: string,
  threshold = 0.1,
): CompareResult {
  const baselinePath = path.resolve(VISUAL_DIR, 'baseline', `${name}.png`)

  if (!existsSync(candidatePath)) {
    throw new Error(`[compareSnapshot] 缺少候选截图: ${candidatePath}`)
  }

  if (!existsSync(baselinePath)) {
    mkdirSync(path.dirname(baselinePath), { recursive: true })
    writeFileSync(baselinePath, readFileSync(candidatePath))
    return { matched: true, firstRun: true }
  }

  const imgA = PNG.sync.read(readFileSync(baselinePath))
  const imgB = PNG.sync.read(readFileSync(candidatePath))
  if (imgA.width !== imgB.width || imgA.height !== imgB.height) {
    throw new Error(
      `[compareSnapshot] 尺寸不一致 baseline ${imgA.width}x${imgA.height} vs candidate ${imgB.width}x${imgB.height}`,
    )
  }
  const { width, height } = imgA
  const diff = new PNG({ width, height })
  const mismatchPixels = pixelmatch(imgA.data, imgB.data, diff.data, width, height, {
    threshold,
  })
  const mismatchPercent = Number(((mismatchPixels / (width * height)) * 100).toFixed(2))
  const matched = mismatchPixels / (width * height) <= threshold

  if (!matched) {
    const diffPath = path.resolve(VISUAL_DIR, 'diff', `${name}.png`)
    mkdirSync(path.dirname(diffPath), { recursive: true })
    writeFileSync(diffPath, PNG.sync.write(diff))
    return { matched, mismatchPixels, mismatchPercent, diffPath }
  }

  return { matched, mismatchPixels, mismatchPercent }
}
