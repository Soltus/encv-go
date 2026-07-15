/**
 * Appearance 视觉回归（Phase 4 主题迁移截图比对基）
 *
 * 运行：npx playwright test
 *   - 首次：写 baseline 到 cypress/visual/baseline/
 *   - 之后：pixelmatch 比对，mismatch > threshold 写 diff 到 cypress/visual/diff/ 并 fail
 *
 * 阈值为 0.1（10% 像素容差），平衡抗锯齿抖动与回归灵敏度。
 * 每个主题态通过切 documentElement[data-theme] 触发（AppearanceDetail 的 composable 同机制）。
 */
import { test, expect } from '@playwright/test'
import { compareSnapshot } from './compare.ts'
import path from 'node:path'
import { mkdirSync } from 'node:fs'

const CANDIDATE_DIR = path.resolve(process.cwd(), 'cypress', 'visual', 'candidate')
mkdirSync(CANDIDATE_DIR, { recursive: true })

const THEMES = [
  { id: 'appearance-default', dataTheme: null as string | null },
  { id: 'appearance-sunset', dataTheme: 'sunset' },
  { id: 'appearance-mint', dataTheme: 'mint' },
]

for (const { id, dataTheme } of THEMES) {
  test(`视觉回归: ${id}`, async ({ page }) => {
    await page.goto('/')
    await page.waitForSelector('ion-content', { state: 'attached', timeout: 30_000 })
    // 等 Ionic hydrated + 布局
    await page.waitForTimeout(2500)
    // 强制 ion-app 可见（防 ion-content 在 headless 下被判 hidden）
    await page.evaluate(() => {
      const a = document.querySelector('ion-app') || document.getElementById('app')
      if (a) a.style.display = 'block'
      document.body.style.margin = '0'
    })
    await page.evaluate((theme) => {
      const root = document.documentElement
      if (theme) root.setAttribute('data-theme', theme)
      else root.removeAttribute('data-theme')
    }, dataTheme)
    // 等 IonContent + 主题令牌生效
    await page.waitForTimeout(400)

    const candidatePath = path.resolve(CANDIDATE_DIR, `${id}.png`)
    await page.screenshot({ path: candidatePath, fullPage: true })

    const res = compareSnapshot(id, candidatePath, 0.1)
    if (res.firstRun) {
      console.log(`[baseline] 已写入 ${id}`)
    } else {
      console.log(
        `matched=${res.matched} mismatch=${res.mismatchPercent}%${res.diffPath ? ` diff=${res.diffPath}` : ''}`,
      )
      expect(res.matched, `视觉回归失败: ${id}，diff=${res.diffPath}`).toBe(true)
    }
  })
}
