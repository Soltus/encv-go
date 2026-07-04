#!/usr/bin/env node
/**
 * biome-stats.mjs — Biome lint 统计工具
 *
 * 用法：
 *   node scripts/biome-stats.mjs [src]       统计指定目录（默认 src）
 *
 * 输出：
 *   - 控制台打印 Top N 规则统计
 *   - 完整 JSON 输出到 /tmp/biome-stats.json
 *
 * 示例：
 *   pnpm biome:stats          # 统计 src
 *   pnpm biome:stats src/composables  # 只统计 composables 目录
 */

import { execSync } from 'node:child_process'
import { writeFileSync } from 'node:fs'
import { resolve } from 'node:path'

const target = process.argv[2] || 'src'
const projectRoot = resolve(new URL('.', import.meta.url).pathname, '..')
const outFile = '/tmp/biome-stats.json'

console.log(`\n🔍 Biome Stats — 统计目录: ${target}\n`)

try {
  const cmd = `npx biome lint ${target} --reporter=json`
  let raw
  try {
    raw = execSync(cmd, { cwd: projectRoot, encoding: 'utf8', maxBuffer: 50 * 1024 * 1024, stdio: ['pipe', 'pipe', 'pipe'] })
  } catch (e) {
    // biome lint 有错误时退出码非 0，但 stdout 里还是有 JSON
    raw = e.stdout || ''
  }
  const data = JSON.parse(raw.trim())

  const ruleCounts = {}
  const sevCounts = {}
  const fileCounts = {}

  for (const d of data.diagnostics) {
    const cat = d.category || 'unknown'
    const sev = d.severity || 'unknown'
    const file = d.location?.path || 'unknown'

    ruleCounts[cat] = (ruleCounts[cat] || 0) + 1
    sevCounts[sev] = (sevCounts[sev] || 0) + 1

    const baseFile = file.split('/').slice(0, 2).join('/')
    fileCounts[baseFile] = (fileCounts[baseFile] || 0) + 1
  }

  const sortedRules = Object.entries(ruleCounts).sort((a, b) => b[1] - a[1])
  const sortedFiles = Object.entries(fileCounts).sort((a, b) => b[1] - a[1])

  console.log('=== 严重程度分布')
  console.log('─────────────────────────────')
  for (const [sev, count] of Object.entries(sevCounts).sort((a, b) => b[1] - a[1])) {
    const bar = '█'.repeat(Math.min(Math.round(count / 50), 50))
    console.log(`${count.toString().padStart(6)}  ${sev.padEnd(10)} ${bar}`)
  }
  console.log()

  console.log('=== Top 20 规则')
  console.log('─────────────────────────────')
  for (const [rule, count] of sortedRules.slice(0, 20)) {
    const pct = ((count / data.diagnostics.length) * 100).toFixed(1)
    console.log(`${count.toString().padStart(6)}  ${pct.padStart(5)}%  ${rule}`)
  }
  console.log()

  console.log('=== Top 10 目录')
  console.log('─────────────────────────────')
  for (const [file, count] of sortedFiles.slice(0, 10)) {
    console.log(`${count.toString().padStart(6)}  ${file}`)
  }
  console.log()

  console.log(`总计: ${data.diagnostics.length} 个诊断`)
  console.log(`文件数: ${Object.keys(fileCounts).length}\n`)

  const fullReport = {
    target,
    total: data.diagnostics.length,
    bySeverity: sevCounts,
    byRule: ruleCounts,
    byFilePrefix: fileCounts,
  }
  writeFileSync(outFile, JSON.stringify(fullReport, null, 2))
  console.log(`📄 完整报告已写入: ${outFile}\n`)

  const hasErrors = sevCounts['error'] && sevCounts['error'] > 0
  process.exit(hasErrors ? 1 : 0)
} catch (e) {
  console.error('❌ 执行失败:', e.message)
  process.exit(1)
}
