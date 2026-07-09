#!/usr/bin/env node
/**
 * Node.js 下各格式解析性能测试
 */
import { readFileSync } from 'node:fs'
import { performance } from 'node:perf_hooks'
import { createRequire } from 'node:module'
import { pathToFileURL } from 'node:url'

const require = createRequire(import.meta.url)
const jsoncParser = require('/tmp/node_modules/jsonc-parser')
const YAML = require('/tmp/node_modules/yamljs')
const toml = require('/tmp/node_modules/@ltd/j-toml')

const dir = process.argv[2] || '/tmp/i18n-formats-50k'

console.log('\n🔬 Node.js 格式解析性能测试')
console.log('='.repeat(60))
console.log('环境: Node.js ' + process.version)
console.log('目录: ' + dir)
console.log()

const results = []

function bench(name, fn, warmup = 2) {
  // 预热
  for (let i = 0; i < warmup; i++) fn()
  // 正式测试 3 次取中位
  const times = []
  for (let i = 0; i < 3; i++) {
    const t0 = performance.now()
    const result = fn()
    const t1 = performance.now()
    times.push(t1 - t0)
    // 验证结果
    if (i === 0 && result && typeof result === 'object') {
      const firstLang = Object.keys(result)[0]
      const keyCount = Object.keys(result[firstLang] || {}).length
      const langCount = Object.keys(result).length
      console.log(`  ${name}: 解析出 ${langCount} 语言 × ${keyCount.toLocaleString()} keys = ${(langCount * keyCount).toLocaleString()} 条目`)
    }
  }
  times.sort((a, b) => a - b)
  const median = times[1]
  results.push({ name, ms: median })
  console.log(`  耗时: ${median.toFixed(0)}ms (三次中位)`)
  console.log()
}

// ====== 1. JSON (内置) ======
const jsonPath = dir + '/dict.json'
const jsonContent = readFileSync(jsonPath, 'utf-8')
console.log(`📄 JSON 文件: ${(Buffer.byteLength(jsonContent, 'utf-8')/1024/1024).toFixed(1)} MB`)
bench('JSON (JSON.parse)', () => JSON.parse(jsonContent))

// ====== 2. JSONC ======
const jsoncPath = dir + '/dict.jsonc'
const jsoncContent = readFileSync(jsoncPath, 'utf-8')
console.log(`📄 JSONC 文件: ${(Buffer.byteLength(jsoncContent, 'utf-8')/1024/1024).toFixed(1)} MB`)
bench('JSONC (jsonc-parser)', () => jsoncParser.parse(jsoncContent))

// ====== 3. YAML ======
const yamlPath = dir + '/dict.yaml'
const yamlContent = readFileSync(yamlPath, 'utf-8')
console.log(`📄 YAML 文件: ${(Buffer.byteLength(yamlContent, 'utf-8')/1024/1024).toFixed(1)} MB`)
bench('YAML (yamljs)', () => YAML.parse(yamlContent))

// ====== 4. TOML ======
const tomlPath = dir + '/dict.toml'
const tomlContent = readFileSync(tomlPath, 'utf-8')
console.log(`📄 TOML 文件: ${(Buffer.byteLength(tomlContent, 'utf-8')/1024/1024).toFixed(1)} MB`)
bench('TOML (@ltd/j-toml)', () => toml.parse(tomlContent, { bigint: false }))

// ====== 5. TS (import) ======
const tsPath = dir + '/dict.ts'
console.log(`📄 TS 文件: ${(require('fs').statSync(tsPath).size/1024/1024).toFixed(1)} MB`)
let tsResult = null
bench('TS (dynamic import)', async () => {
  const mod = await import(pathToFileURL(tsPath).href + '?t=' + Date.now())
  return mod.default
}, 1)

// ====== 6. TS (eval 提取) ======
const tsContent = readFileSync(tsPath, 'utf-8')
const s = tsContent.indexOf('{')
const e = tsContent.lastIndexOf('}')
const objStr = tsContent.slice(s, e + 1)
bench('TS (eval提取)', () => eval('(' + objStr + ')'))

// ====== 汇总 ======
console.log('='.repeat(60))
console.log('📊 性能汇总（从快到慢）')
console.log('='.repeat(60))
results.sort((a, b) => a.ms - b.ms)
const fastest = results[0].ms

function pad(str, len) { while (str.length < len) str += ' '; return str }
function padR(str, len) { while (str.length < len) str = ' ' + str; return str }

console.log()
console.log('  ' + pad('格式', 28) + padR('耗时', 10) + padR('相对速度', 12) + '  注释支持')
console.log('  ' + '-'.repeat(28) + '-'.repeat(10) + '-'.repeat(12) + '  ' + '-'.repeat(10))

const commentSupport = {
  'JSON (JSON.parse)': '❌ 无',
  'JSONC (jsonc-parser)': '✅ 有',
  'YAML (yamljs)': '✅ 有',
  'TOML (@ltd/j-toml)': '✅ 有',
  'TS (dynamic import)': '✅ 有',
  'TS (eval提取)': '✅ 有',
}

for (const r of results) {
  const ratio = (r.ms / fastest).toFixed(1) + 'x'
  const msStr = r.ms.toFixed(0) + 'ms'
  const comment = commentSupport[r.name] || '?'
  console.log('  ' + pad(r.name, 28) + padR(msStr, 10) + padR(ratio, 12) + '  ' + comment)
}

console.log()
console.log('💡 最快: ' + results[0].name + ' (' + results[0].ms.toFixed(0) + 'ms)')
console.log('💡 最慢: ' + results[results.length - 1].name + ' (' + results[results.length - 1].ms.toFixed(0) + 'ms)')
const diff = (results[results.length - 1].ms / results[0].ms).toFixed(1)
console.log('💡 差距: ' + diff + 'x')

console.log('\n' + '='.repeat(60))
