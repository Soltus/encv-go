#!/usr/bin/env node
/**
 * i18n 字典提取器 - 用 Node.js 原生能力解析 TS/JS 字典文件
 *
 * 用法: node extract-i18n.mjs <file1> <file2> ...
 * 输出: JSON 格式，{ "zh-CN": { key: value, ... }, "en": { ... } }
 *
 * 为什么用 Node.js 而不是 Python 正则？
 * - 字典文件本身就是合法的 ESM/TS 模块
 * - 直接 import 后 JSON.stringify，100% 准确，不存在正则匹配的边界问题
 * - Python 侧的正则解析作为 fallback（没有 Node.js 环境时使用）
 */
import { readFileSync, writeFileSync, existsSync } from 'node:fs'
import { pathToFileURL } from 'node:url'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { createHash } from 'node:crypto'

const files = process.argv.slice(2)
if (files.length === 0) {
  console.error('Usage: node extract-i18n.mjs <file1> <file2> ...')
  process.exit(1)
}

const result = {}
const fileSources = {}

for (const file of files) {
  if (!existsSync(file)) {
    console.error(`Warning: file not found: ${file}`)
    continue
  }

  try {
    const content = readFileSync(file, 'utf-8')

    const exportMatch = content.match(/export\s+default\s*\{([\s\S]*)\}\s*;?\s*$/)
    if (!exportMatch) {
      console.error(`Warning: no default export found in: ${file}`)
      continue
    }

    const mod = await import(pathToFileURL(file).href + '?t=' + Date.now())
    const dict = mod.default || {}

    for (const locale of Object.keys(dict)) {
      if (!result[locale]) {
        result[locale] = {}
      }
      const localeDict = dict[locale] || {}
      for (const [key, value] of Object.entries(localeDict)) {
        result[locale][key] = String(value)
        if (!(key in fileSources)) {
          fileSources[key] = file.split('/').pop()
        }
      }
    }
  } catch (err) {
    console.error(`Error parsing ${file}: ${err.message}`)
    process.exit(2)
  }
}

const output = {
  ...result,
  _file_source: fileSources,
}

console.log(JSON.stringify(output, null, 2))
