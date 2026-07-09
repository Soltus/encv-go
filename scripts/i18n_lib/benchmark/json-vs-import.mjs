#!/usr/bin/env node
/**
 * 关键发现验证：JSON.parse vs import
 * 以及探索 "extract JSON + JSON.parse" 方案
 */
import { readFileSync, writeFileSync, existsSync, mkdirSync } from 'node:fs'
import { performance } from 'node:perf_hooks'
import { join } from 'node:path'
import { pathToFileURL } from 'node:url'

const file = process.argv[2]
if (!file || !existsSync(file)) {
  console.error('Usage: node json-vs-import.mjs <file.ts>')
  process.exit(1)
}

const content = readFileSync(file, 'utf-8')
const tmpDir = '/tmp/i18n-bench-tmp'
mkdirSync(tmpDir, { recursive: true })

console.error(`\n🔬 JSON.parse vs import 对比测试`)
console.error('='.repeat(60))
console.error(`文件大小: ${(Buffer.byteLength(content, 'utf-8')/1024/1024).toFixed(1)} MB`)

// 方法1: 完整 import (基准)
const t0 = performance.now()
const mod = await import(pathToFileURL(file).href + '?t1=' + Date.now())
const dict1 = mod.default || {}
const t_import = performance.now() - t0
const totalKeys = Object.values(dict1).reduce((s, d) => s + Object.keys(d).length, 0)
console.error(`\n方法1 - import (基准): ${t_import.toFixed(0)}ms, ${totalKeys.toLocaleString()} keys`)

// 方法2: 正则提取 export default 后的对象 + JSON.parse
// 注意：这只对纯数据字典有效，不能有函数/变量等
const t1 = performance.now()
const match = content.match(/export\s+default\s+(\{[\s\S]*\})\s*;?\s*$/)
let dict2 = null
if (match) {
  try {
    dict2 = JSON.parse(match[1])
  } catch (e) {
    console.error(`  JSON.parse 失败: ${e.message}`)
  }
}
const t_regex_json = performance.now() - t1
console.error(`方法2 - 正则提取 + JSON.parse: ${t_regex_json.toFixed(0)}ms`)

// 方法3: 直接 eval 对象字面量（最接近 import 的语义，但快很多）
const t2 = performance.now()
const match2 = content.match(/export\s+default\s+(\{[\s\S]*\})\s*;?\s*$/)
let dict3 = null
if (match2) {
  try {
    dict3 = eval('(' + match2[1] + ')')
  } catch (e) {
    console.error(`  eval 失败: ${e.message}`)
  }
}
const t_eval = performance.now() - t2
console.error(`方法3 - 正则提取 + eval:     ${t_eval.toFixed(0)}ms`)

// 方法4: new Function()
const t3 = performance.now()
const match3 = content.match(/export\s+default\s+(\{[\s\S]*\})\s*;?\s*$/)
let dict4 = null
if (match3) {
  try {
    dict4 = new Function('return (' + match3[1] + ')')()
  } catch (e) {
    console.error(`  new Function 失败: ${e.message}`)
  }
}
const t_newfunc = performance.now() - t3
console.error(`方法4 - 正则提取 + new Function: ${t_newfunc.toFixed(0)}ms`)

// 方法5: 纯 JSON 文件 + JSON.parse
const jsonFile = join(tmpDir, 'dict.json')
if (match) {
  writeFileSync(jsonFile, match[1], 'utf-8')
}
const t4 = performance.now()
const jsonContent = readFileSync(jsonFile, 'utf-8')
const dict5 = JSON.parse(jsonContent)
const t_readfile_json = performance.now() - t4
console.error(`方法5 - fs.readFile + JSON.parse: ${t_readfile_json.toFixed(0)}ms`)

// 速度对比
console.error(`\n速度对比 (相对于 import):`)
console.error(`  import:           1.0x (基准)`)
console.error(`  正则+JSON.parse:  ${(t_import / t_regex_json).toFixed(1)}x`)
console.error(`  正则+eval:        ${(t_import / t_eval).toFixed(1)}x`)
console.error(`  正则+new Func:    ${(t_import / t_newfunc).toFixed(1)}x`)
console.error(`  读文件+JSON.parse: ${(t_import / t_readfile_json).toFixed(1)}x`)

// 正确性验证
console.error(`\n正确性验证:`)
const sampleKey = Object.keys(dict1['zh-CN'] || dict1[Object.keys(dict1)[0]])[0]
const firstLang = Object.keys(dict1)[0]
console.error(`  样本key: ${sampleKey}`)
console.error(`  import: ${dict1[firstLang][sampleKey]}`)
console.error(`  JSON.parse: ${dict2 ? dict2[firstLang][sampleKey] : 'N/A'}`)
console.error(`  eval: ${dict3 ? dict3[firstLang][sampleKey] : 'N/A'}`)

console.error(`\n💡 关键洞察:`)
console.error(`  import 慢的根本原因: V8 要做完整的模块编译 + 类型检查`)
console.error(`  纯数据字典用 JSON.parse/eval 快得多，因为跳过了编译层`)
console.error(`  风险: 正则提取可能漏掉复杂语法（模板字符串、计算属性等）`)

console.log('{}')
