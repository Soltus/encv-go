#!/usr/bin/env node
/**
 * Node.js 解析器细粒度性能分析
 * 测试各阶段耗时：import / 遍历合并 / JSON序列化 / stdout传输
 */
import { readFileSync, existsSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { performance } from 'node:perf_hooks'

const files = process.argv.slice(2)
if (files.length === 0) {
  console.error('Usage: node profile-node-parser.mjs <file1> <file2> ...')
  process.exit(1)
}

const existing = files.filter(f => existsSync(f))
console.error(`\n📊 Node.js 解析器细粒度分析 (${existing.length} files)`)
console.error('='.repeat(60))

const result = {}
let totalKeys = 0
let totalLangs = 0

// ---- 阶段1: import 所有文件 ----
const t0 = performance.now()
const modules = []
for (const file of existing) {
  const mod = await import(pathToFileURL(file).href + '?t=' + Date.now())
  modules.push(mod.default || {})
}
const t_import = performance.now() - t0

// ---- 阶段2: 遍历合并 ----
const t1 = performance.now()
for (const dict of modules) {
  for (const locale of Object.keys(dict)) {
    if (!result[locale]) {
      result[locale] = {}
      totalLangs++
    }
    const localeDict = dict[locale] || {}
    const entries = Object.entries(localeDict)
    for (const [key, value] of entries) {
      result[locale][key] = String(value)
    }
    totalKeys += Object.keys(localeDict).length
  }
}
const t_merge = performance.now() - t1

// ---- 阶段3: JSON.stringify（无缩进） ----
const t2 = performance.now()
const jsonCompact = JSON.stringify(result)
const t_json_compact = performance.now() - t2

// ---- 阶段4: JSON.stringify（有缩进） ----
const t3 = performance.now()
const jsonPretty = JSON.stringify(result, null, 2)
const t_json_pretty = performance.now() - t3

// ---- 阶段5: 计算输出大小 ----
const sizeCompact = Buffer.byteLength(jsonCompact, 'utf-8')
const sizePretty = Buffer.byteLength(jsonPretty, 'utf-8')

console.error(`
  总 key 数: ${(totalKeys).toLocaleString()}
  语言数: ${totalLangs}
  
  阶段1 - import: ${t_import.toFixed(1)}ms
  阶段2 - 遍历合并: ${t_merge.toFixed(1)}ms
  阶段3 - JSON序列化(紧凑): ${t_json_compact.toFixed(1)}ms (${(sizeCompact/1024/1024).toFixed(1)} MB)
  阶段4 - JSON序列化(美化): ${t_json_pretty.toFixed(1)}ms (${(sizePretty/1024/1024).toFixed(1)} MB)
  
  纯解析总耗时: ${(t_import + t_merge + t_json_compact).toFixed(1)}ms
`)

// 只输出紧凑 JSON 到 stdout（减少传输量）
console.log(jsonCompact)
