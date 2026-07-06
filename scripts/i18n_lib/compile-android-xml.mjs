#!/usr/bin/env node
/**
 * i18n 字典编译 - 将 TS 字典编译为 Android strings.xml（供 Kotlin 复用）
 * 使用 eval 快速提取，比 import 快 3x+
 *
 * 用法: node compile-android-xml.mjs <output-dir> <file1> [file2 ...]
 * 输出: output-dir/values/strings.xml, output-dir/values-zh-rCN/strings.xml, ...
 */
import { writeFileSync, existsSync, mkdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { parseDictFiles } from './dict-parser.mjs'

const args = process.argv.slice(2)
if (args.length < 2) {
  console.error('Usage: node compile-android-xml.mjs <output-dir> <file1> [file2 ...]')
  process.exit(1)
}

const outputDir = resolve(args[0])
const files = args.slice(1)

const existing = files.filter(f => existsSync(resolve(f)))

try {
  const result = await parseDictFiles(existing)

  function toAndroidResourceName(key) {
    return key.replace(/[^a-zA-Z0-9_]/g, '_').replace(/^_+|_+$/g, '')
  }

  function escapeXml(str) {
    return str
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, "\\'")
      .replace(/\n/g, '\\n')
  }

  function localeToAndroidDir(locale) {
    if (locale === 'zh-CN') return 'values-zh-rCN'
    if (locale === 'en') return 'values'
    const parts = locale.split('-')
    if (parts.length === 1) return `values-${parts[0].toLowerCase()}`
    return `values-${parts[0].toLowerCase()}-r${parts[1].toUpperCase()}`
  }

  let totalKeys = 0
  let localeCount = 0

  for (const [locale, dict] of Object.entries(result)) {
    const dirName = localeToAndroidDir(locale)
    const outDir = resolve(outputDir, dirName)
    mkdirSync(outDir, { recursive: true })
    const outPath = resolve(outDir, 'strings.xml')

    const keys = Object.keys(dict).sort()
    const lines = ['<?xml version="1.0" encoding="utf-8"?>', '<resources>']

    for (const key of keys) {
      const resName = toAndroidResourceName(key)
      const value = escapeXml(dict[key])
      lines.push(`    <string name="${resName}">${value}</string>`)
    }

    lines.push('</resources>', '')
    writeFileSync(outPath, lines.join('\n'), 'utf-8')

    totalKeys += keys.length
    localeCount++
    console.log(`  ✅ ${dirName}/strings.xml  (${keys.length} keys)`)
  }

  console.log()
  console.log(`📦 编译完成: ${localeCount} 个语言, 共 ${totalKeys} 条翻译`)
  console.log(`📁 输出目录: ${outputDir}`)
} catch (err) {
  console.error(`Error: ${err.message}`)
  process.exit(2)
}
