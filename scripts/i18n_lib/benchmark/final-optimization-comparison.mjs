#!/usr/bin/env node
/**
 * 终极优化方案对比测试
 */
import { readFileSync, existsSync, writeFileSync, mkdirSync } from 'node:fs'
import { performance } from 'node:perf_hooks'
import { join } from 'node:path'
import { createRequire } from 'node:module'

const require = createRequire(import.meta.url)
const { Packr } = require('/tmp/node_modules/msgpackr')
const msgpackr = new Packr({ variableMapSize: true })

const files = process.argv.slice(2).filter(f => existsSync(f))
const tmpDir = '/tmp/i18n-bench-tmp'
mkdirSync(tmpDir, { recursive: true })

console.error('\n🏆 终极优化方案对比 (' + files.length + ' files, ~20万 keys × 20语)')
console.error('='.repeat(60))

// 方案B: eval + JSON + 文件IO
console.error('\n📦 方案B: eval + JSON + 文件IO')
const tB0 = performance.now()
const resultB = {}
for (const file of files) {
  const content = readFileSync(file, 'utf-8')
  const s = content.indexOf('{')
  const e = content.lastIndexOf('}')
  const dict = eval('(' + content.slice(s, e + 1) + ')')
  for (const locale of Object.keys(dict)) {
    if (!resultB[locale]) resultB[locale] = {}
    const ld = dict[locale] || {}
    for (const [k, v] of Object.entries(ld)) {
      resultB[locale][k] = String(v)
    }
  }
}
const jsonB = JSON.stringify(resultB)
writeFileSync(join(tmpDir, 'out-b.json'), jsonB, 'utf-8')
const tB = performance.now() - tB0
const speedB = (32000 / tB).toFixed(1)
const statB = tB < 5000 ? '✅' : '❌'
console.error('  耗时: ' + tB.toFixed(0) + 'ms')
console.error('  加速: ' + speedB + 'x')
console.error('  5秒目标: ' + statB)

// 方案C: eval + msgpack + 文件IO
console.error('\n📦 方案C: eval + msgpack + 文件IO')
const tC0 = performance.now()
const resultC = {}
for (const file of files) {
  const content = readFileSync(file, 'utf-8')
  const s = content.indexOf('{')
  const e = content.lastIndexOf('}')
  const dict = eval('(' + content.slice(s, e + 1) + ')')
  for (const locale of Object.keys(dict)) {
    if (!resultC[locale]) resultC[locale] = {}
    const ld = dict[locale] || {}
    for (const [k, v] of Object.entries(ld)) {
      resultC[locale][k] = String(v)
    }
  }
}
const packed = msgpackr.pack(resultC)
writeFileSync(join(tmpDir, 'out-c.mp'), packed)
const tC = performance.now() - tC0
const speedC = (32000 / tC).toFixed(1)
const statC = tC < 5000 ? '✅' : '❌'
const jsonSizeMB = (Buffer.byteLength(jsonB) / 1024 / 1024).toFixed(1)
const packSizeMB = (packed.length / 1024 / 1024).toFixed(1)
console.error('  耗时: ' + tC.toFixed(0) + 'ms')
console.error('  msgpack大小: ' + packSizeMB + ' MB (JSON: ' + jsonSizeMB + ' MB)')
console.error('  加速: ' + speedC + 'x')
console.error('  5秒目标: ' + statC)

// 方案D: eval + Object.assign 合并 + msgpack
console.error('\n📦 方案D: eval + Object.assign合并 + msgpack')
const tD0 = performance.now()
const allDicts = []
for (const file of files) {
  const content = readFileSync(file, 'utf-8')
  const s = content.indexOf('{')
  const e = content.lastIndexOf('}')
  allDicts.push(eval('(' + content.slice(s, e + 1) + ')'))
}
const allLocales = new Set()
for (const d of allDicts) {
  for (const l of Object.keys(d)) allLocales.add(l)
}
const resultD = {}
for (const locale of allLocales) {
  resultD[locale] = {}
  for (const d of allDicts) {
    if (d[locale]) {
      Object.assign(resultD[locale], d[locale])
    }
  }
}
const packedD = msgpackr.pack(resultD)
writeFileSync(join(tmpDir, 'out-d.mp'), packedD)
const tD = performance.now() - tD0
const speedD = (32000 / tD).toFixed(1)
const statD = tD < 5000 ? '✅' : '❌'
console.error('  耗时: ' + tD.toFixed(0) + 'ms')
console.error('  用 Object.assign 替代 for-of 合并')
console.error('  加速: ' + speedD + 'x')
console.error('  5秒目标: ' + statD)

// 方案E: 不合并，按文件存
console.error('\n📦 方案E: 不合并(按文件存msgpack)')
const tE0 = performance.now()
let totalPackedSize = 0
for (let i = 0; i < files.length; i++) {
  const file = files[i]
  const content = readFileSync(file, 'utf-8')
  const s = content.indexOf('{')
  const e = content.lastIndexOf('}')
  const dict = eval('(' + content.slice(s, e + 1) + ')')
  const p = msgpackr.pack(dict)
  totalPackedSize += p.length
  writeFileSync(join(tmpDir, 'e-' + i + '.mp'), p)
}
const tE = performance.now() - tE0
const speedE = (32000 / tE).toFixed(1)
const statE = tE < 5000 ? '✅' : '❌'
console.error('  耗时: ' + tE.toFixed(0) + 'ms')
console.error('  总msgpack大小: ' + (totalPackedSize / 1024 / 1024).toFixed(1) + ' MB')
console.error('  加速: ' + speedE + 'x')
console.error('  5秒目标: ' + statE)
console.error('  注: 需要改查询逻辑，遍历所有文件找key')

// 方案F: +4 Worker 并行（理论估算）
console.error('\n📦 方案F: +4 Worker并行(理论估算)')
const evalOnlyTime = tD * 0.45
const mergeTime = tD * 0.35
const serializeTime = tD * 0.20
const parallelEvalTime = evalOnlyTime / 3.5
const tF = parallelEvalTime + mergeTime + serializeTime
const speedF = (32000 / tF).toFixed(1)
const statF = tF < 5000 ? '✅' : '❌'
console.error('  估算耗时: ' + tF.toFixed(0) + 'ms')
console.error('  加速: ' + speedF + 'x')
console.error('  5秒目标: ' + statF)

// 方案G: 极限优化（理论）
console.error('\n📦 方案G: 极限优化(理论)')
const tG = parallelEvalTime + (mergeTime * 0.7) + (serializeTime * 0.6)
const speedG = (32000 / tG).toFixed(1)
const statG = tG < 5000 ? '✅' : '❌'
console.error('  估算耗时: ' + tG.toFixed(0) + 'ms')
console.error('  加速: ' + speedG + 'x')
console.error('  5秒目标: ' + statG)

// 汇总表
console.error('\n' + '='.repeat(60))
console.error('📊 方案汇总（20万key × 20语言，400万总翻译条目）')
console.error('='.repeat(60))
console.error('')

function pad(str, len) {
  while (str.length < len) str += ' '
  return str
}
function padR(str, len) {
  while (str.length < len) str = ' ' + str
  return str
}

const header = pad('方案', 30) + padR('耗时', 10) + padR('状态', 6) + padR('成本', 6) + '  说明'
const sep = '-'.repeat(30) + '-'.repeat(10) + '-'.repeat(6) + '-'.repeat(6) + '  ' + '-'.repeat(20)
console.error('  ' + header)
console.error('  ' + sep)

const scenarios = [
  { name: 'A: 当前(import+JSON+stdout)', ms: 32000, status: '❌', cost: '低', note: '已实现/32秒' },
  { name: 'B: eval + JSON + 文件IO', ms: tB, status: statB, cost: '低', note: '改动最小' },
  { name: 'C: eval + msgpack + 文件IO', ms: tC, status: statC, cost: '低', note: '+msgpack依赖' },
  { name: 'D: eval+Object.assign+msgpack', ms: tD, status: statD, cost: '低', note: '纯代码优化' },
  { name: 'E: 不合并(按文件存)', ms: tE, status: statE, cost: '高', note: '架构大改' },
  { name: 'F: D + 4 Worker并行(估)', ms: tF, status: statF, cost: '中', note: '复杂度上升' },
  { name: 'G: 极限优化(理论估)', ms: tG, status: statG, cost: '高', note: '全部堆上去' },
]

for (const s of scenarios) {
  const msStr = Math.round(s.ms).toLocaleString() + 'ms'
  const line = pad(s.name, 30) + padR(msStr, 10) + padR(s.status, 6) + padR(s.cost, 6) + '  ' + s.note
  console.error('  ' + line)
}

console.error(`
🎯 关键结论:
1. 当前 import 方案 32秒，离 5秒目标差 6.4x
2. 方案D 投入产出比最高：~${tD.toFixed(0)}ms，改动最小，纯代码优化
3. 要稳进 5秒 选方案F：+Worker并行，估算 ${tF.toFixed(0)}ms
4. 有缓存场景 < 2秒，日常开发主要靠缓存
5. 20万key×20语是极端规模，100倍于当前项目（1791 keys）
`)

console.log('{}')
