// 臻彩显示（vivid / P3）真实渲染回归（先红后绿，铁律：真实浏览器复现）。
// 锁死两个根因：
//  (A) 滤镜选择器 .encv-vivid ion-page 命中不到页面（CE 模式页面是 <div class="ion-page">）
//      → 真实 computed filter 恒为 none。修复：改同时匹配 .ion-page 类。
//  (B) P3 @media 交换被 applyColor 内联的 --color-primary 压过（无 !important）。
//      修复：交换规则加 !important + 非循环回退 --color-primary-srgb。
import { test, expect } from '@playwright/test'

test('vivid 滤镜在真实页面上生效（修复 A）', async ({ page }) => {
  await page.goto('http://localhost:5199/')
  await page.waitForTimeout(4000)
  const pageEl = page.locator('.ion-page').first()
  await expect(pageEl).toBeAttached()

  const before = await page.evaluate(() => getComputedStyle(document.querySelector('.ion-page') as HTMLElement).filter)
  expect(before, '开启前滤镜应为 none').toBe('none')

  // 经真实 UI 路径开启 vivid
  await page.evaluate(() => {
    const toggles = Array.from(document.querySelectorAll('ion-toggle')) as any[]
    const target =
      toggles.find((t) => {
        const item = t.closest('ion-item') || t.parentElement
        const txt = (item?.textContent || '').toLowerCase()
        return txt.includes('vivid') || txt.includes('瑰彩') || txt.includes('臻彩')
      }) || toggles[0]
    target.checked = !target.checked
    target.dispatchEvent(new CustomEvent('ionChange', { detail: { checked: target.checked }, bubbles: true }))
  })
  await page.waitForTimeout(1200)

  const after = await page.evaluate(() => getComputedStyle(document.querySelector('.ion-page') as HTMLElement).filter)
  expect(after, '修复后 .encv-vivid .ion-page 应命中 → 真实滤镜生效').toContain('contrast')
  expect(after).toContain('saturate')

  // 真实 gsap 确实把 vivid 变量写到了 :root
  const sat = await page.evaluate(() =>
    getComputedStyle(document.documentElement).getPropertyValue('--encv-vivid-sat'),
  )
  expect(parseFloat(sat), 'gsap 应写入非零 sat').toBeGreaterThan(0)
})

test('暗色场景下臻彩滤镜对比度收敛、浓度加大（明暗分调优化）', async ({ page }) => {
  await page.goto('http://localhost:5199/')
  await page.waitForTimeout(4000)
  const res = await page.evaluate(() => {
    const root = document.documentElement
    const body = document.body
    root.classList.add('encv-vivid')
    root.style.setProperty('--encv-vivid-sat', '1')
    root.style.setProperty('--encv-vivid-contrast', '1')
    const lightEl = document.querySelector('.ion-page') as HTMLElement | null
    const light = lightEl ? getComputedStyle(lightEl).filter : 'none'
    body.classList.add('dark')
    const darkEl = document.querySelector('.ion-page') as HTMLElement | null
    const dark = darkEl ? getComputedStyle(darkEl).filter : 'none'
    return { light, dark }
  })
  const parseSat = (f: string) => {
    const m = f.match(/saturate\(([\d.]+)\)/)
    return m ? parseFloat(m[1]) : 0
  }
  const parseCon = (f: string) => {
    const m = f.match(/contrast\(([\d.]+)\)/)
    return m ? parseFloat(m[1]) : 0
  }
  expect(res.light, '亮色下滤镜应生效').toContain('saturate')
  expect(parseSat(res.dark), '暗色 saturate 增益应大于亮色').toBeGreaterThan(parseSat(res.light))
  expect(parseCon(res.dark), '暗色 contrast 增益应小于亮色').toBeLessThan(parseCon(res.light))
})

test('P3 交换规则在真实样式表里带 !important 且回退 --color-primary-srgb（修复 B）', async ({ page }) => {
  await page.goto('http://localhost:5199/')
  await page.waitForTimeout(4000)
  const p3Rule = await page.evaluate(() => {
    for (const sheet of Array.from(document.styleSheets)) {
      let rules: any[]
      try {
        rules = Array.from(sheet.cssRules)
      } catch {
        continue
      }
      for (const r of rules) {
        if (r.media && r.media.mediaText.includes('color-gamut: p3') && r.cssRules) {
          for (const inner of Array.from(r.cssRules)) {
            if (inner.selectorText && inner.selectorText.includes('encv-p3')) return inner.cssText
          }
        }
      }
    }
    return null
  })
  expect(p3Rule, '应存在 @media (color-gamut:p3) :root.encv-p3 规则').toBeTruthy()
  expect(p3Rule).toContain('!important')
  expect(p3Rule).toContain('--color-primary-srgb')
})

test('P3 交换经 !important 能覆盖内联 --color-primary（级联复现）', async ({ page }) => {
  await page.goto('http://localhost:5199/')
  await page.waitForTimeout(4000)
  const res = await page.evaluate(() => {
    const root = document.documentElement
    root.style.setProperty('--color-primary', 'rgb(1,2,3)') // 模拟 applyColor 内联
    root.style.setProperty('--color-primary-p3', 'rgb(9,9,9)')
    const s2 = document.createElement('style')
    s2.textContent = `:root.encv-p3-probe2 { --color-primary: var(--color-primary-p3, rgb(0,255,0)); }`
    document.head.appendChild(s2)
    root.classList.add('encv-p3-probe2')
    const withoutImportant = getComputedStyle(root).getPropertyValue('--color-primary')
    root.classList.remove('encv-p3-probe2')
    s2.remove()
    const s1 = document.createElement('style')
    s1.textContent = `:root.encv-p3-probe { --color-primary: var(--color-primary-p3, rgb(0,255,0)) !important; }`
    document.head.appendChild(s1)
    root.classList.add('encv-p3-probe')
    const withImportant = getComputedStyle(root).getPropertyValue('--color-primary')
    return { withImportant, withoutImportant }
  })
  expect(res.withoutImportant, '无 !important：内联压过作者规则（Bug 复现）').toBe('rgb(1,2,3)')
  expect(res.withImportant, '有 !important：修复后覆盖内联').toBe('rgb(9,9,9)')
})
