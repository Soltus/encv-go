#!/usr/bin/env node
/**
 * eval 方案大规模性能测试
 * 模拟完整的解析流程：读文件 + 提取对象 + eval + 合并 + 输出
 */
import { readFileSync, existsSync, writeFileSync } from 'node:fs'
import { performance } from 'node:perf_hooks'
import { join } from 'node:path'

const files = process.argv.slice(2).filter(f => existsSync(f))

console.error(`\n🚀 eval 方案大规模测试 (${files.length} files)`)
console.error('='.repeat(60))

// ============================================================
// 完整 eval 方案
// ============================================================
const t0 = performance.now()

const result = {}
let totalKeys = 0

// 阶段1: 读文件 + 提取对象 + eval
const t_read_start = performance.now()
const dicts = []
for (const file of files) {
  const content = readFileSync(file, 'utf-8')
  const start = content.indexOf('{')
  const end = content.lastIndexOf('}')
  if (start >= 0 && end > start) {
    const objStr = content.slice(start, end + 1)
    const dict = eval('(' + objStr + ')')
    dicts.push(dict)
  }
}
const t_read_eval = performance.now() - t_read_start

// 阶段2: 合并
const t_merge_start = performance.now()
for (const dict of dicts) {
  for (const locale of Object.keys(dict)) {
    if (!result[locale]) result[locale] = {}
    const localeDict = dict[locale] || {}
    for (const [key, value] of Object.entries(localeDict)) {
      result[locale][key] = String(value)
      totalKeys++
    }
  }
}
const t_merge = performance.now() - t_merge_start

// 阶段3: JSON 序列化（紧凑）
const t_json_start = performance.now()
const jsonStr = JSON.stringify(result)
const t_json = performance.now() - t_json_start

// 阶段4: 写文件（替代 stdout）
const t_write_start = performance.now()
const outFile = '/tmp/i18n-bench-tmp/eval-out.json'
writeFileSync(outFile, jsonStr, 'utf-8')
const t_write = performance.now() - t_write_start

const total = performance.now() - t0
const jsonSize = Buffer.byteLength(jsonStr, 'utf-8')

console.error(`
完整流程（读文件+eval+合并+JSON+写文件）:
  读文件+eval:    ${t_read_eval.toFixed(0)}ms
  合并遍历:       ${t_merge.toFixed(0)}ms
  JSON序列化:     ${t_json.toFixed(0)}ms
  写文件:         ${t_write.toFixed(0)}ms
  ─────────────────────────
  总计:           ${total.toFixed(0)}ms
  总key数:        ${totalKeys.toLocaleString()}
  JSON大小:       ${(jsonSize/1024/1024).toFixed(1)} MB
  吞吐量:         ${(totalKeys/total*1000).toFixed(0)} keys/秒
`)

// 对比 import 方案的基准
const importEstimate = total * 8 // 经验值：import 约慢 8x
console.error(`
对比 import 方案估算:
  import方案估算:  ${importEstimate.toFixed(0)}ms
  eval 方案加速:   ${(importEstimate/total).toFixed(1)}x
  5秒目标:         ${total < 5000 ? '✅ 达成' : '❌ 未达成'}
`)

// 再加上 Python 侧读取 + 解析开销估算
const pyJsonParseMs = jsonSize / 1024 / 1024 * 6.6 // 1536ms / 233MB ≈ 6.6ms/MB
const fullTotal = total + pyJsonParseMs + 50 // + Python 启动等
console.error(`
加上 Python 侧开销（估算）:
  Python JSON解析:  ${pyJsonParseMs.toFixed(0)}ms
  全链路估算:      ${fullTotal.toFixed(0)}ms
  5秒目标:         ${fullTotal < 5000 ? '✅ 达成' : '❌ 未达成'}
`)

console.log(jsonStr)
