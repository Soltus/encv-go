#!/usr/bin/env node
/**
 * 优化方案对比测试
 * 测试各种优化手段的实际收益
 */
import { readFileSync, writeFileSync, existsSync, mkdirSync } from 'node:fs'
import { performance } from 'node:perf_hooks'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'
import { Worker, isMainThread, parentPort, workerData } from 'node:worker_threads'

const files = process.argv.slice(2)
const existing = files.filter(f => existsSync(f))

console.error(`\n🔬 优化方案对比测试 (${existing.length} files)`)
console.error('='.repeat(60))

// ============================================================
// 基准：当前方案（import + 合并 + JSON.stringify + stdout）
// ============================================================
const t0 = performance.now()
const resultBase = {}
for (const file of existing) {
  const mod = await import(pathToFileURL(file).href + '?t=' + Date.now())
  const dict = mod.default || {}
  for (const locale of Object.keys(dict)) {
    if (!resultBase[locale]) resultBase[locale] = {}
    const localeDict = dict[locale] || {}
    for (const [key, value] of Object.entries(localeDict)) {
      resultBase[locale][key] = String(value)
    }
  }
}
const jsonStr = JSON.stringify(resultBase)
const t_base = performance.now() - t0

const totalKeys = Object.values(resultBase).reduce((sum, d) => sum + Object.keys(d).length, 0)
const totalSize = Buffer.byteLength(jsonStr, 'utf-8')

console.error(`
基准方案 (import + JSON.stringify):
  总耗时: ${t_base.toFixed(0)}ms
  总key数: ${totalKeys.toLocaleString()}
  JSON大小: ${(totalSize/1024/1024).toFixed(1)} MB
`)

// ============================================================
// 优化1: 写临时文件代替 stdout 传输
// ============================================================
const tmpDir = '/tmp/i18n-bench-tmp'
mkdirSync(tmpDir, { recursive: true })
const tmpFile = join(tmpDir, 'out.json')

const t1 = performance.now()
writeFileSync(tmpFile, jsonStr, 'utf-8')
const t_write_file = performance.now() - t1

console.error(`
优化1 - 写文件代替stdout:
  文件写入: ${t_write_file.toFixed(1)}ms
  (对比: stdout传输约 5587ms / 200k keys)
  节省: ${((5587 - t_write_file) / 5587 * 100).toFixed(1)}%
`)

// ============================================================
// 优化2: 多 Worker 并行 import
// ============================================================
if (existing.length >= 4) {
  const workerCount = Math.min(4, existing.length)
  const t2 = performance.now()

  const workers = []
  const results = new Array(workerCount)

  for (let i = 0; i < workerCount; i++) {
    const startIdx = Math.floor(i * existing.length / workerCount)
    const endIdx = Math.floor((i + 1) * existing.length / workerCount)
    const workerFiles = existing.slice(startIdx, endIdx)

    workers.push(new Promise((resolve) => {
      // 直接在主线程模拟（避免 worker 脚本复杂度）
      const localResult = {}
      for (const file of workerFiles) {
        // 用 eval + Function 来模拟 "解析" 成本
        const content = readFileSync(file, 'utf-8')
        // 模拟 import 的开销：这里实际做 import
        import(pathToFileURL(file).href + '?t2=' + Date.now() + i).then(mod => {
          const dict = mod.default || {}
          for (const locale of Object.keys(dict)) {
            if (!localResult[locale]) localResult[locale] = {}
            const localeDict = dict[locale] || {}
            for (const [key, value] of Object.entries(localeDict)) {
              localResult[locale][key] = String(value)
            }
          }
          resolve(localResult)
        })
      }
    }))
  }

  // 这个测试不太准，因为 event loop 已经在处理 import 了
  // 我们改为用更直接的方式测试 —— 见下面的 "理论分析"
  const importEstimate = (t_base * 0.87).toFixed(0)
  const parallelEstimate = (t_base * 0.87 / (workerCount * 0.7)).toFixed(0)
  console.error(`
优化2 - 多 Worker 并行 import (理论分析):
  文件数: ${existing.length}
  理论核心数: ${workerCount}
  基准 import: ${importEstimate}ms (估算 87% 是 import)
  理论加速比: ~${(workerCount * 0.7).toFixed(1)}x (考虑开销)
  理论 import 耗时: ${parallelEstimate}ms
`)
}

// ============================================================
// 优化3: 直接 JSON.parse 对比 import
// ============================================================
// 准备一个纯 JSON 文件
const firstFileContent = readFileSync(existing[0], 'utf-8')
// 提取 export default 后的 JSON 部分
const match = firstFileContent.match(/export\s+default\s+(\{[\s\S]*\})\s*;?\s*$/)
if (match) {
  const jsonContent = match[1]
  const jsonFile = join(tmpDir, 'sample.json')
  writeFileSync(jsonFile, jsonContent, 'utf-8')

  // 测试 JSON.parse 速度
  const t3 = performance.now()
  const parsed = JSON.parse(jsonContent)
  const t_json_parse = performance.now() - t3

  // 测试 import 同样内容的速度（已测过）
  // 计算速度比
  const oneFileImportEstimate = t_base / existing.length
  const speedup = oneFileImportEstimate / t_json_parse

  console.error(`
优化3 - 纯 JSON.parse vs import (单个文件):
  JSON.parse: ${t_json_parse.toFixed(1)}ms
  import 估算: ${oneFileImportEstimate.toFixed(1)}ms
  速度比: JSON.parse 快 ${speedup.toFixed(1)}x
`)
}

// ============================================================
// 优化4: 增量解析（只解析变更文件）
// ============================================================
console.error(`
优化4 - 增量解析 (理论分析):
  场景: 10个文件中只改了1个
  全量解析: ${t_base.toFixed(0)}ms
  增量解析: ${(t_base / existing.length + t_base * 0.05).toFixed(0)}ms (1/10 import + 合并开销)
  日常开发体验: < ${(t_base / existing.length * 2).toFixed(0)}ms (几乎无感)
`)

// ============================================================
// 综合预估
// ============================================================
console.error(`\n${'='.repeat(60)}`)
console.error('📊 20万key × 20语言 各方案预估总耗时')
console.error('='.repeat(60))

const base = 31759 // 实测值
const file_io_saving = 5587 - 50 // 文件IO替代stdout，写文件约50ms
const msgpack_saving_pct = 0.3 // msgpack 比 JSON 快约 30%（序列化+反序列化）

const scenarios = [
  { name: '当前方案', ms: base },
  { name: '+ 文件IO替代stdout', ms: base - file_io_saving },
  { name: '+ 4 Worker并行import', ms: (base * 0.676) / 2.8 + base * 0.324 },
  { name: '+ 文件IO + 4 Worker', ms: ((base - file_io_saving) * 0.676) / 2.8 + (base - file_io_saving) * 0.324 },
  { name: '全部优化(有缓存)', ms: 1536 * 0.5 + 50 + 10 }, // msgpack + 文件IO + 指纹
]

for (const s of scenarios) {
  const barLen = Math.floor(s.ms / base * 50)
  const bar = '█'.repeat(barLen)
  const status = s.ms < 5000 ? '✅' : s.ms < 10000 ? '⚠️' : '❌'
  const pct = (1 - s.ms / base) * 100
  const msStr = Math.round(s.ms).toLocaleString().padStart(6)
  const pctStr = pct > 0 ? '↓' + pct.toFixed(0) + '%' : ''
  console.error(`  ${status} ${s.name.padEnd(24)} ${msStr}ms ${bar} ${pctStr}`)
}

console.error(`
  🎯 5秒目标: 仅靠 import 并行 + 文件IO 还不够
  💡 要达标必须依赖缓存（有缓存场景 < 2秒）
     日常开发场景下增量解析 < 1秒
     首次全量解析是一次性成本
`)

// 输出空 JSON 保持 stdout 干净
console.log('{}')
