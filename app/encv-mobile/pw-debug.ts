// 用 bun 运行：bun pw-debug.ts
import { chromium } from '@playwright/test'
const b = await chromium.launch({ args: ['--no-sandbox', '--disable-dev-shm-usage'] })
const p = await b.newPage()
const logs: string[] = []
p.on('console', (m) => logs.push('[' + m.type() + '] ' + m.text()))
p.on('pageerror', (e) => logs.push('PAGEERR: ' + e.message + '\n' + (e.stack || '').split('\n').slice(0, 4).join('\n')))
await p.goto('http://localhost:5198/', { waitUntil: 'load', timeout: 30_000 })
await p.waitForTimeout(3000)
const html = await p.evaluate(() => document.getElementById('app')?.innerHTML?.slice(0, 600) || 'NO #app')
console.log('=== #app innerHTML ===')
console.log(html)
console.log('=== console/page errors ===')
console.log(logs.join('\n'))
await b.close()
