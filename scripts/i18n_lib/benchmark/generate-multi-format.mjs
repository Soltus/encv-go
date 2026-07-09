#!/usr/bin/env node
/**
 * 多格式 i18n 字典数据生成器
 * 生成 JSON / JSONC / YAML / TOML / TS 五种格式的测试数据
 */
import { writeFileSync, mkdirSync } from 'node:fs'
import { join } from 'node:path'

const args = process.argv.slice(2)
const keyCount = parseInt(args[0] || '50000', 10)
const langCount = parseInt(args[1] || '20', 10)
const outBase = args[2] || '/tmp/i18n-formats'

const languages = [
  'zh-CN', 'en', 'ja', 'ko', 'fr', 'de', 'es', 'pt', 'it', 'ru',
  'ar', 'hi', 'bn', 'tr', 'vi', 'th', 'id', 'ms', 'pl', 'nl',
].slice(0, langCount)

const wordLists = {
  'zh-CN': ['设置', '任务', '文件', '用户', '系统', '数据', '配置', '管理', '信息', '列表', '详情', '编辑', '删除', '新增', '搜索', '导出', '导入', '刷新', '保存', '取消'],
  'en': ['settings', 'task', 'file', 'user', 'system', 'data', 'config', 'admin', 'info', 'list', 'detail', 'edit', 'delete', 'create', 'search', 'export', 'import', 'refresh', 'save', 'cancel'],
  'ja': ['設定', 'タスク', 'ファイル', 'ユーザー', 'システム', 'データ', '設定', '管理', '情報', 'リスト', '詳細', '編集', '削除', '新規', '検索', 'エクスポート', 'インポート', '更新', '保存', 'キャンセル'],
  'ko': ['설정', '작업', '파일', '사용자', '시스템', '데이터', '구성', '관리', '정보', '목록', '상세', '편집', '삭제', '생성', '검색', '내보내기', '가져오기', '새로고침', '저장', '취소'],
}

function generateValue(lang, idx) {
  const words = wordLists[lang] || wordLists['en']
  const w1 = words[idx % words.length]
  const w2 = words[(idx * 7 + 3) % words.length]
  const w3 = words[(idx * 13 + 9) % words.length]
  return `${w1} ${w2} ${w3} ${idx}`
}

function generateKey(i) {
  const cats = ['settings', 'tasks', 'files', 'users', 'system', 'data', 'config', 'admin', 'common', 'errors']
  const cat = cats[Math.floor(i / 5000) % cats.length]
  const sub = Math.floor(i / 100) % 100
  return `${cat}.${sub}.item_${i}`
}

// 生成数据对象
const data = {}
for (const lang of languages) {
  data[lang] = {}
  for (let i = 0; i < keyCount; i++) {
    const key = generateKey(i)
    data[lang][key] = generateValue(lang, i)
  }
}

const totalKeys = keyCount * langCount
console.log(`生成 ${keyCount} keys × ${langCount} langs = ${totalKeys.toLocaleString()} 总翻译条目`)
console.log(`输出目录: ${outBase}`)
console.log()

mkdirSync(outBase, { recursive: true })

// ====== 1. 纯 JSON ======
{
  const t0 = Date.now()
  const jsonStr = JSON.stringify(data, null, 2)
  const t1 = Date.now()
  const path = join(outBase, 'dict.json')
  writeFileSync(path, jsonStr, 'utf-8')
  const size = Buffer.byteLength(jsonStr, 'utf-8')
  console.log(`✅ JSON: ${(size/1024/1024).toFixed(1)} MB (序列化 ${t1-t0}ms)`)
}

// ====== 2. JSONC (带注释的 JSON) ======
{
  const t0 = Date.now()
  let jsoncStr = `// i18n 字典文件 - JSONC 格式（支持注释）
// 生成时间: ${new Date().toISOString()}
// 语言数: ${langCount}
// 每语言key数: ${keyCount}

{
`
  for (const lang of languages) {
    jsoncStr += `\n  // ===== ${lang} =====\n`
    jsoncStr += `  "${lang}": {\n`
    const keys = Object.keys(data[lang])
    for (let i = 0; i < keys.length; i++) {
      const k = keys[i]
      const v = data[lang][k]
      const comma = i < keys.length - 1 ? ',' : ''
      if (i % 1000 === 0) {
        jsoncStr += `    // -- 第 ${Math.floor(i/1000)} 组 --\n`
      }
      jsoncStr += `    "${k}": "${v.replace(/"/g, '\\"')}"${comma}\n`
    }
    jsoncStr += `  },\n`
  }
  jsoncStr += `}\n`
  const t1 = Date.now()
  const path = join(outBase, 'dict.jsonc')
  writeFileSync(path, jsoncStr, 'utf-8')
  const size = Buffer.byteLength(jsoncStr, 'utf-8')
  console.log(`✅ JSONC: ${(size/1024/1024).toFixed(1)} MB (生成 ${t1-t0}ms)`)
}

// ====== 3. YAML ======
{
  const t0 = Date.now()
  let yamlStr = `# i18n 字典文件 - YAML 格式
# 生成时间: ${new Date().toISOString()}
# 语言数: ${langCount}
# 每语言key数: ${keyCount}

`
  for (const lang of languages) {
    yamlStr += `\n# ===== ${lang} =====\n`
    yamlStr += `${lang}:\n`
    const keys = Object.keys(data[lang])
    for (let i = 0; i < keys.length; i++) {
      const k = keys[i]
      const v = data[lang][k]
      if (i % 1000 === 0) {
        yamlStr += `  # -- 第 ${Math.floor(i/1000)} 组 --\n`
      }
      // YAML 中特殊字符需要引号
      const needsQuote = /[:#\[\]{}&*!|>'"%@`\n]/.test(v) || v.startsWith(' ') || v.endsWith(' ')
      if (needsQuote) {
        yamlStr += `  ${k}: "${v.replace(/"/g, '\\"')}"\n`
      } else {
        yamlStr += `  ${k}: ${v}\n`
      }
    }
  }
  const t1 = Date.now()
  const path = join(outBase, 'dict.yaml')
  writeFileSync(path, yamlStr, 'utf-8')
  const size = Buffer.byteLength(yamlStr, 'utf-8')
  console.log(`✅ YAML: ${(size/1024/1024).toFixed(1)} MB (生成 ${t1-t0}ms)`)
}

// ====== 4. TOML ======
{
  const t0 = Date.now()
  let tomlStr = `# i18n 字典文件 - TOML 格式
# 生成时间: ${new Date().toISOString()}
# 语言数: ${langCount}
# 每语言key数: ${keyCount}

`
  for (const lang of languages) {
    tomlStr += `\n# ===== ${lang} =====\n`
    tomlStr += `[${lang}]\n`
    const keys = Object.keys(data[lang])
    for (let i = 0; i < keys.length; i++) {
      const k = keys[i]
      const v = data[lang][k]
      if (i % 1000 === 0) {
        tomlStr += `# -- 第 ${Math.floor(i/1000)} 组 --\n`
      }
      // TOML 中 key 含有点号需要用引号
      const safeKey = /[.\s-]/.test(k) ? `"${k}"` : k
      const safeVal = v.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
      tomlStr += `${safeKey} = "${safeVal}"\n`
    }
  }
  const t1 = Date.now()
  const path = join(outBase, 'dict.toml')
  writeFileSync(path, tomlStr, 'utf-8')
  const size = Buffer.byteLength(tomlStr, 'utf-8')
  console.log(`✅ TOML: ${(size/1024/1024).toFixed(1)} MB (生成 ${t1-t0}ms)`)
}

// ====== 5. TypeScript (当前格式) ======
{
  const t0 = Date.now()
  const jsonStr = JSON.stringify(data, null, 2)
  const tsStr = `// i18n 字典文件 - TypeScript 格式
// 生成时间: ${new Date().toISOString()}
// 语言数: ${langCount}
// 每语言key数: ${keyCount}

export default ${jsonStr}
`
  const t1 = Date.now()
  const path = join(outBase, 'dict.ts')
  writeFileSync(path, tsStr, 'utf-8')
  const size = Buffer.byteLength(tsStr, 'utf-8')
  console.log(`✅ TS: ${(size/1024/1024).toFixed(1)} MB (生成 ${t1-t0}ms)`)
}

console.log()
console.log('完成！所有格式已生成到 ' + outBase)
