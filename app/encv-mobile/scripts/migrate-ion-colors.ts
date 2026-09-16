// 用 bun 运行：bun scripts/migrate-ion-colors.ts [file...]
// 无参数时全量处理 src 下所有 .vue/.css/.scss（排除入口/逻辑文件）。
// 临时迁移助手：把 color-preserving 的 --ion-color-* 映射到 daisyUI --color-* 令牌。
// 设计原则：桥接 bridge.css 仍兜底所有 --ion-color-* 变体（含 -rgb），故裸 -rgb 残留
//   不会失效；本脚本仅做语义等价迁移，遇见未覆盖形态会报告 [remaining] 供人工复核。
import { existsSync, readFileSync, writeFileSync, readdirSync, statSync } from 'node:fs'
import path from 'node:path'

const ROOT = path.resolve('src')
const EXCLUDE = new Set(['src/main.ts'])
const EXTS = new Set(['.vue', '.css', '.scss'])

const COLOR: Record<string, string> = {
  primary: 'var(--color-primary)',
  secondary: 'var(--color-secondary)',
  tertiary: 'var(--color-accent)',
  success: 'var(--color-success)',
  warning: 'var(--color-warning)',
  danger: 'var(--color-error)',
  medium: 'color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100))',
  light: 'var(--color-base-200)',
}
const NAMES = Object.keys(COLOR)

function walk(dir: string, out: string[]) {
  for (const name of readdirSync(dir)) {
    const full = path.join(dir, name)
    const st = statSync(full)
    if (st.isDirectory()) walk(full, out)
    else if (EXTS.has(path.extname(name)) && !EXCLUDE.has(path.relative(ROOT, full))) out.push(full)
  }
}

const explicit = process.argv.slice(2)
const files = explicit.length > 0 ? explicit.map(f => path.resolve(f)) : (() => { const a: string[] = []; walk(ROOT, a); return a })()

// var(--ion-color-X, fallback?) 匹配；fallback 可含一层嵌套括号（如 rgba(...)）
function v(name: string): RegExp {
  return new RegExp(
    `var\\(${name.replace(/[-[\]{}()*+?.,\\^$|#]/g, '\\$&')}(?:,\\s*[^()]*(?:\\([^()]*\\)[^()]*)*)?\\)`,
    'g',
  )
}

// Ionic 数字色 name-50/100/.../900 → color-mix 浅化/深化（近似，非像素复刻）
function numericIon(name: string, num: number): string {
  const c = COLOR[name]
  if (num <= 100) {
    const white = num === 50 ? 95 : 90
    return `color-mix(in srgb, ${c} ${100 - white}%, #fff)`
  }
  if (num < 500) {
    const white = (500 - num) / 5 // 100→80,200→60,300→40,400→20
    return `color-mix(in srgb, ${c} ${100 - white}%, #fff)`
  }
  if (num === 500) return c
  const black = (900 - num) / 4 // 600→75,700→50,800→25,900→0
  const cpct = num === 900 ? 15 : black
  return `color-mix(in srgb, ${c} ${cpct}%, #000)`
}

// Ionic step-N 灰度阶梯 → base 灰阶
function stepMix(num: number): string {
  if (num <= 100) return 'var(--color-base-100)'
  if (num <= 200) return 'var(--color-base-200)'
  const pct = (num - 200) / 7
  if (num >= 900) return 'color-mix(in srgb, var(--color-base-content) 92%, transparent)'
  return `color-mix(in srgb, var(--color-base-content) ${Math.round(pct)}%, var(--color-base-100))`
}

function apply(content: string): string {
  let out = content
  // 1) rgba(var(--ion-color-X-rgb), N) 半透明 → color-mix
  out = out.replace(
    /rgba\(var\(--ion-color-(primary|secondary|tertiary|success|warning|danger|medium|light)-rgb\)\s*,\s*([0-9.]+)\)/g,
    (_m, color: string, alpha: string) => {
      const c = COLOR[color]
      const pct = Math.round(parseFloat(alpha) * 100)
      return `color-mix(in srgb, ${c} ${pct}%, transparent)`
    },
  )
  // 2) Ionic 数字色 name-NNN（50/100/.../900），fallback 支持一层括号
  out = out.replace(
    /var\(--ion-color-(primary|secondary|tertiary|success|warning|danger|medium|light)-(\d{2,3})(?:\s*,\s*[^()]*(?:\([^()]*\)[^()]*)*)?\)/g,
    (_m, name: string, num: string) => numericIon(name, parseInt(num, 10)),
  )
  // 3) step-N 灰度，fallback 支持一层括号
  out = out.replace(
    /var\(--ion-color-step-(\d{2,3})(?:\s*,\s*[^()]*(?:\([^()]*\)[^()]*)*)?\)/g,
    (_m, num: string) => stepMix(parseInt(num, 10)),
  )
  // 4) dark（Ionic 暗色专用，bridge 未定义，需兜底）
  out = out.replace(v('--ion-color-dark'), 'var(--color-base-300)')
  out = out.replace(v('--ion-color-dark-shade'), 'color-mix(in srgb, var(--color-base-300) 85%, #000)')
  out = out.replace(v('--ion-color-dark-tint'), 'color-mix(in srgb, var(--color-base-300) 85%, #fff)')
  out = out.replace(v('--ion-color-dark-contrast'), 'var(--color-base-content)')
  // 5) tint / shade / contrast
  for (const name of NAMES) {
    out = out.replace(v(`--ion-color-${name}-tint`), `color-mix(in srgb, ${COLOR[name]} 85%, #fff)`)
    out = out.replace(v(`--ion-color-${name}-shade`), `color-mix(in srgb, ${COLOR[name]} 85%, #000)`)
  }
  out = out.replace(v('--ion-color-primary-contrast'), 'var(--color-primary-content)')
  out = out.replace(v('--ion-color-secondary-contrast'), 'var(--color-secondary-content)')
  out = out.replace(v('--ion-color-tertiary-contrast'), 'var(--color-accent-content)')
  out = out.replace(v('--ion-color-success-contrast'), 'var(--color-success-content)')
  out = out.replace(v('--ion-color-warning-contrast'), '#000000')
  out = out.replace(v('--ion-color-danger-contrast'), 'var(--color-error-content)')
  out = out.replace(v('--ion-color-medium-contrast'), '#ffffff')
  out = out.replace(v('--ion-color-light-contrast'), 'var(--color-base-content)')
  // 6) 基色
  for (const name of NAMES) {
    out = out.replace(v(`--ion-color-${name}`), COLOR[name])
  }
  return out
}

let totalChanged = 0
let totalRemaining = 0
const safeRgb = /-rgb$|-contrast-rgb$/
for (const file of files) {
  if (!existsSync(file)) { console.error(`[skip] 不存在: ${file}`); continue }
  const original = readFileSync(file, 'utf8')
  const content = apply(original)
  if (content !== original) {
    writeFileSync(file, content, 'utf8')
    totalChanged++
    console.log(`[rewrite] ${path.relative(ROOT, file)}`)
  }
  const left = [...content.matchAll(/(--ion-color-[a-z0-9-]+)/g)].map(m => m[1])
  if (left.length > 0) {
    const unique = [...new Set(left)]
    const unsafe = unique.filter(t => !safeRgb.test(t))
    if (unsafe.length > 0) {
      totalRemaining++
      console.log(`[remaining] ${path.relative(ROOT, file)}: ${unsafe.join(', ')}`)
    } else {
      console.log(`[bridge-safe] ${path.relative(ROOT, file)}: ${unique.join(', ')}`)
    }
  }
}
console.log(`\n# changed=${totalChanged} unsafe-remaining-files=${totalRemaining}`)
