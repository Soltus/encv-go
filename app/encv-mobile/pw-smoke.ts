// 用 bun 运行：bun pw-smoke.ts
import { chromium } from '@playwright/test'
const b = await chromium.launch({ args: ['--no-sandbox', '--disable-dev-shm-usage'] })
const p = await b.newPage()
await p.setContent('<h1 id=x>hello</h1>')
console.log('TEXT_OK=' + (await p.textContent('#x')))
await p.screenshot({ path: '/tmp/pw-smoke.png' })
console.log('SHOT_OK')
await b.close()
