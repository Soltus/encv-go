#!/usr/bin/env node
/**
 * i18n 字典提取 - 从 TS 文件提取所有语言的键值对
 * 使用 eval 快速提取，比 import 快 3x+
 *
 * 用法: node extract-i18n.mjs <file1> <file2> ...
 * 输出: JSON 到 stdout
 */
import { existsSync } from 'node:fs'
import { resolve } from 'node:path'
import { parseDictFiles } from './dict-parser.mjs'

const files = process.argv.slice(2)
if (files.length === 0) {
  console.error('Usage: node extract-i18n.mjs <file1> [file2 ...]')
  process.exit(1)
}

const existing = files.filter(f => existsSync(resolve(f)))

try {
  const result = await parseDictFiles(existing)
  process.stdout.write(JSON.stringify(result))
} catch (err) {
  console.error(`Error parsing dictionaries: ${err.message}`)
  process.exit(2)
}
