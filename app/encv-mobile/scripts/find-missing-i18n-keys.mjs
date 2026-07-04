#!/usr/bin/env node
/**
 * 扫描项目中所有 t() 调用，对照 i18n 字典，找出缺失的 key
 *
 * 用法：node scripts/find-missing-i18n-keys.mjs
 */
import fs from 'node:fs'
import path from 'node:path'

const PROJECT_ROOT = path.resolve(import.meta.dirname, '..')
const SRC_DIR = path.join(PROJECT_ROOT, 'src')
const I18N_DIR = path.join(SRC_DIR, 'i18n')

// 收集所有源码中的 t('key') / t(`key`) 调用
function collectUsedKeys(dir) {
  const keys = new Set()
  const files = fs.readdirSync(dir, { withFileTypes: true })

  for (const file of files) {
    const fullPath = path.join(dir, file.name)
    if (file.isDirectory()) {
      if (file.name === 'node_modules' || file.name.startsWith('.')) continue
      collectUsedKeys(fullPath).forEach(k => keys.add(k))
    } else if (/\.(vue|ts|tsx|js|jsx)$/.test(file.name)) {
      const content = fs.readFileSync(fullPath, 'utf-8')
      // 匹配 t('xxx') / t("xxx") / t(`xxx`)
      const re = /\bt\(\s*['"`]([^'"`]+)['"`]/g
      let m
      while ((m = re.exec(content)) !== null) {
        keys.add(m[1])
      }
    }
  }
  return keys
}

// 解析 i18n 字典文件（简单的 key-value 提取，只找 'x.y.z': 'value' 格式）
function collectDictKeys(filePath) {
  const keys = new Set()
  const content = fs.readFileSync(filePath, 'utf-8')
  // 匹配  'some.key':  或  "some.key":
  const re = /^\s*['"]([^'"]+)['"]\s*:\s*['"`]/gm
  let m
  while ((m = re.exec(content)) !== null) {
    keys.add(m[1])
  }
  return keys
}

console.log('🔍 扫描源码中使用的 i18n key...')
const usedKeys = collectUsedKeys(SRC_DIR)
console.log(`   共找到 ${usedKeys.size} 个使用中的 key`)

console.log('\n📚 读取 i18n 字典...')
const dictFiles = fs.readdirSync(I18N_DIR).filter(f => f.endsWith('.ts'))
const allDictKeys = new Set()
for (const f of dictFiles) {
  const keys = collectDictKeys(path.join(I18N_DIR, f))
  console.log(`   ${f}: ${keys.size} 个 key`)
  keys.forEach(k => allDictKeys.add(k))
}

console.log(`\n   合并后字典共 ${allDictKeys.size} 个 key`)

// 找出缺失的
console.log('\n❌ 缺失的 key（使用了但字典中没有）：')
const missing = []
for (const key of usedKeys) {
  // 跳过动态 key（包含变量的，如 template string 带 ${} 的）
  if (key.includes('${') || key.includes('+')) continue
  // 跳过明显不是 i18n key 的（太短的、包含空格的）
  if (key.length < 3 || /\s/.test(key)) continue
  // 跳过带花括号的（可能是模板）
  if (key.includes('{')) continue

  if (!allDictKeys.has(key)) {
    missing.push(key)
  }
}

if (missing.length === 0) {
  console.log('   ✅ 没有缺失的 key！')
} else {
  missing.sort()
  missing.forEach((k, i) => {
    console.log(`   ${i + 1}. ${k}`)
  })
  console.log(`\n   共 ${missing.length} 个缺失的 key`)
}

// 找出未使用的（可选，注释掉，太多了没意义）
// console.log('\n⚠️  未使用的 key（字典中有但源码没用到）：')
// const unused = []
// for (const key of allDictKeys) {
//   if (!usedKeys.has(key)) {
//     unused.push(key)
//   }
// }
// unused.sort().forEach(k => console.log(`   - ${k}`))
// console.log(`   共 ${unused.length} 个未使用的 key`)

process.exit(missing.length > 0 ? 1 : 0)
