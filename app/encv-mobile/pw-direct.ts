// 用 bun 运行：bun pw-direct.ts
import { chromium } from '@playwright/test'
const b = await chromium.launch({ args: ['--no-sandbox', '--disable-dev-shm-usage'] })
const p = await b.newPage({ viewport: { width: 390, height: 844 } })
await p.goto('http://localhost:5198/', { waitUntil: 'load', timeout: 30_000 })
await p.waitForSelector('ion-content', { state: 'attached', timeout: 30_000 })
// 等 Ionic hydrated + 布局
await p.waitForTimeout(2500)
// 强制让 app 可见（防 ion-content hidden）
await p.evaluate(() => {
  const a = document.querySelector('ion-app') || document.getElementById('app')
  if (a) a.style.display = 'block'
  document.body.style.margin = '0'
})
await p.evaluate(() => document.documentElement.setAttribute('data-theme', 'sunset'))
await p.waitForTimeout(500)
await p.screenshot({ path: '/tmp/appearance-sunset.png', fullPage: true })
const ok = await p.locator('ion-list-header').count()
console.log('OK ion-list-header count=' + ok)
await b.close()
