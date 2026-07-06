#!/usr/bin/env node
/**
 * i18n 字典编译 - 将 TS 字典编译为纯 JSON 文件（供 Go/Python 等其他语言复用）
 *
 * 用法: node compile-json.mjs <output-dir> <file1> <file2> ...
 * 输出: output-dir/zh-CN.json, output-dir/en.json, ...
 */
import { readFileSync, writeFileSync, existsSync, mkdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const args = process.argv.slice(2)
if (args.length < 2) {
  console.error('Usage: node compile-json.mjs <output-dir> <file1> [file2 ...]')
  process.exit(1)
}

const outputDir = resolve(args[0])
const files = args.slice(1)

if (!existsSync(outputDir)) {
  mkdirSync(outputDir, { recursive: true })
}

const result = {}

for (const file of files) {
  const absFile = resolve(file)
  if (!existsSync(absFile)) {
    console.error(`Warning: file not found: ${absFile}`)
    continue
  }

  try {
    const mod = await import(pathToFileURL(absFile).href + '?t=' + Date.now())
    const dict = mod.default || {}

    for (const locale of Object.keys(dict)) {
      if (!result[locale]) {
        result[locale] = {}
      }
      const localeDict = dict[locale] || {}
      for (const [key, value] of Object.entries(localeDict)) {
        result[locale][key] = String(value)
      }
    }
  } catch (err) {
    console.error(`Error parsing ${absFile}: ${err.message}`)
    process.exit(2)
  }
}

let totalKeys = 0
let localeCount = 0

for (const [locale, dict] of Object.entries(result)) {
  const outPath = resolve(outputDir, `${locale}.json`)
  writeFileSync(outPath, JSON.stringify(dict, null, 2), 'utf-8')
  const keyCount = Object.keys(dict).length
  totalKeys += keyCount
  localeCount++
  console.log(`  ✅ ${locale}.json  (${keyCount} keys, ${Math.round(Buffer.byteLength(JSON.stringify(dict), 'utf-8') / 1024)} KB)`)
}

console.log()
console.log(`📦 编译完成: ${localeCount} 个语言文件, 共 ${totalKeys} 条翻译`)
console.log(`📁 输出目录: ${outputDir}`)
