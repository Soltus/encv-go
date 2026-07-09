#!/usr/bin/env node
/**
 * i18n 字典编译 - 将 TS 字典编译为 Android strings.xml（供 Kotlin 复用）
 * 使用 eval 快速提取，比 import 快 3x+
 *
 * 用法: node compile-android-xml.mjs <output-dir> <file1> [file2 ...]
 * 输出: output-dir/values/strings.xml, output-dir/values-zh-rCN/strings.xml, ...
 *
 * 合并策略: 如果目标 strings.xml 已存在，保留其中非 i18n 生成的固定资源
 * （如 Capacitor 必需的 app_name），i18n 生成的条目以 TS 字典为准覆盖。
 */
import { writeFileSync, existsSync, mkdirSync, readFileSync } from 'node:fs'
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

  function parseExistingStrings(filePath) {
    if (!existsSync(filePath)) return new Map()
    try {
      const content = readFileSync(filePath, 'utf-8')
      const map = new Map()
      const re = /<string\s+name="([^"]+)"\s*>([\s\S]*?)<\/string>/g
      let m
      while ((m = re.exec(content)) !== null) {
        map.set(m[1], m[2])
      }
      return map
    } catch {
      return new Map()
    }
  }

  for (const [locale, dict] of Object.entries(result)) {
    const dirName = localeToAndroidDir(locale)
    const outDir = resolve(outputDir, dirName)
    mkdirSync(outDir, { recursive: true })
    const outPath = resolve(outDir, 'strings.xml')

    const existing = parseExistingStrings(outPath)

    const i18nResNames = new Map()
    const collisions = []

    for (const [key, value] of Object.entries(dict)) {
      const resName = toAndroidResourceName(key)
      if (i18nResNames.has(resName)) {
        collisions.push({ resName, firstKey: i18nResNames.get(resName).key, secondKey: key })
      } else {
        i18nResNames.set(resName, { key, value: escapeXml(value) })
      }
    }

    if (collisions.length > 0) {
      console.error(`❌ ${dirName}/strings.xml: ${collisions.length} 个 Android resource name collision(s):`)
      for (const c of collisions) {
        console.error(`   "${c.firstKey}" and "${c.secondKey}" both map to R.string.${c.resName}`)
      }
      process.exit(3)
    }

    const merged = new Map()
    for (const [name, val] of existing) {
      if (!i18nResNames.has(name)) {
        merged.set(name, val)
      }
    }
    for (const [name, { value }] of i18nResNames) {
      merged.set(name, value)
    }

    const sortedNames = [...merged.keys()].sort()
    const lines = ['<?xml version="1.0" encoding="utf-8"?>', '<resources>']
    for (const name of sortedNames) {
      lines.push(`    <string name="${name}">${merged.get(name)}</string>`)
    }
    lines.push('</resources>', '')
    writeFileSync(outPath, lines.join('\n'), 'utf-8')

    const preservedCount = [...existing.keys()].filter(n => !i18nResNames.has(n)).length
    const i18nCount = i18nResNames.size
    totalKeys += i18nCount
    localeCount++
    console.log(`  ✅ ${dirName}/strings.xml  (${i18nCount} i18n keys` +
      (preservedCount > 0 ? ` + ${preservedCount} preserved` : '') + ')')
  }

  console.log()
  console.log(`📦 编译完成: ${localeCount} 个语言, 共 ${totalKeys} 条翻译`)
  console.log(`📁 输出目录: ${outputDir}`)
} catch (err) {
  console.error(`Error: ${err.message}`)
  process.exit(2)
}
