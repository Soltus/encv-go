#!/usr/bin/env node
/**
 * 大规模 i18n 测试数据生成器
 * 生成指定数量的 key 和语言，用于性能基准测试
 */
import { writeFileSync, mkdirSync } from 'node:fs'
import { join } from 'node:path'

const args = process.argv.slice(2)
const keyCount = parseInt(args[0] || '200000', 10)
const langCount = parseInt(args[1] || '20', 10)
const outDir = args[2] || '/tmp/i18n-bench'

const languages = [
  'zh-CN', 'en', 'ja', 'ko', 'fr', 'de', 'es', 'pt', 'it', 'ru',
  'ar', 'hi', 'bn', 'tr', 'vi', 'th', 'id', 'ms', 'pl', 'nl',
].slice(0, langCount)

mkdirSync(outDir, { recursive: true })

console.log(`生成 ${keyCount} 个 key × ${languages.length} 种语言的测试数据...`)

const wordLists = {
  'zh-CN': ['设置', '任务', '文件', '用户', '系统', '数据', '配置', '管理', '信息', '列表', '详情', '编辑', '删除', '新增', '搜索', '导出', '导入', '刷新', '保存', '取消'],
  'en': ['settings', 'task', 'file', 'user', 'system', 'data', 'config', 'admin', 'info', 'list', 'detail', 'edit', 'delete', 'create', 'search', 'export', 'import', 'refresh', 'save', 'cancel'],
  'ja': ['設定', 'タスク', 'ファイル', 'ユーザー', 'システム', 'データ', '設定', '管理', '情報', 'リスト', '詳細', '編集', '削除', '新規', '検索', 'エクスポート', 'インポート', '更新', '保存', 'キャンセル'],
  'ko': ['설정', '작업', '파일', '사용자', '시스템', '데이터', '구성', '관리', '정보', '목록', '상세', '편집', '삭제', '생성', '검색', '내보내기', '가져오기', '새로고침', '저장', '취소'],
  'fr': ['paramètres', 'tâche', 'fichier', 'utilisateur', 'système', 'données', 'configuration', 'gestion', 'informations', 'liste', 'détails', 'modifier', 'supprimer', 'créer', 'rechercher', 'exporter', 'importer', 'actualiser', 'enregistrer', 'annuler'],
  'de': ['einstellungen', 'aufgabe', 'datei', 'benutzer', 'system', 'daten', 'konfiguration', 'verwaltung', 'informationen', 'liste', 'details', 'bearbeiten', 'löschen', 'erstellen', 'suchen', 'exportieren', 'importieren', 'aktualisieren', 'speichern', 'abbrechen'],
  'es': ['configuración', 'tarea', 'archivo', 'usuario', 'sistema', 'datos', 'configuración', 'gestión', 'información', 'lista', 'detalles', 'editar', 'eliminar', 'crear', 'buscar', 'exportar', 'importar', 'actualizar', 'guardar', 'cancelar'],
  'pt': ['configurações', 'tarefa', 'arquivo', 'usuário', 'sistema', 'dados', 'configuração', 'gerenciamento', 'informações', 'lista', 'detalhes', 'editar', 'excluir', 'criar', 'pesquisar', 'exportar', 'importar', 'atualizar', 'salvar', 'cancelar'],
  'it': ['impostazioni', 'attività', 'file', 'utente', 'sistema', 'dati', 'configurazione', 'gestione', 'informazioni', 'elenco', 'dettagli', 'modifica', 'elimina', 'crea', 'cerca', 'esporta', 'importa', 'aggiorna', 'salva', 'annulla'],
  'ru': ['настройки', 'задача', 'файл', 'пользователь', 'система', 'данные', 'конфигурация', 'управление', 'информация', 'список', 'детали', 'редактировать', 'удалить', 'создать', 'поиск', 'экспорт', 'импорт', 'обновить', 'сохранить', 'отмена'],
}

function generateValue(lang, keyIndex, category) {
  const words = wordLists[lang] || wordLists['en']
  const w1 = words[keyIndex % words.length]
  const w2 = words[(keyIndex * 7 + 3) % words.length]
  const w3 = words[(keyIndex * 13 + 9) % words.length]
  return `${w1} ${w2} ${w3} ${keyIndex}`
}

function generateKey(i) {
  const categories = ['settings', 'tasks', 'files', 'users', 'system', 'data', 'config', 'admin', 'common', 'errors', 'warnings', 'info', 'forms', 'tables', 'modals', 'buttons', 'menu', 'sidebar', 'header', 'footer']
  const cat = categories[Math.floor(i / 10000) % categories.length]
  const subcat = Math.floor(i / 100) % 100
  const name = `item_${i}`
  return `${cat}.${subcat}.${name}`
}

const perFile = 20000
const fileCount = Math.ceil(keyCount / perFile)

console.log(`拆分成 ${fileCount} 个文件，每个文件约 ${perFile} 个 key`)

const fileList = []

for (let f = 0; f < fileCount; f++) {
  const obj = {}
  for (const lang of languages) {
    obj[lang] = {}
  }

  const startIdx = f * perFile
  const endIdx = Math.min(startIdx + perFile, keyCount)

  for (let i = startIdx; i < endIdx; i++) {
    const key = generateKey(i)
    for (const lang of languages) {
      obj[lang][key] = generateValue(lang, i, '')
    }
  }

  const content = `// 测试数据文件 ${f + 1}/${fileCount} - ${endIdx - startIdx} keys
export default ${JSON.stringify(obj, null, 2)}
`

  const filename = `bench-${f.toString().padStart(3, '0')}.ts`
  const filepath = join(outDir, filename)
  writeFileSync(filepath, content, 'utf-8')
  fileList.push(filepath)

  if ((f + 1) % 5 === 0 || f === fileCount - 1) {
    console.log(`  已生成 ${f + 1}/${fileCount} 个文件`)
  }
}

console.log()
console.log(`完成！共生成 ${fileCount} 个文件`)
console.log(`总 key 数: ${keyCount}`)
console.log(`语言数: ${languages.length}`)
console.log(`输出目录: ${outDir}`)
console.log(`文件列表:`)
for (const f of fileList.slice(0, 5)) {
  console.log(`  ${f}`)
}
if (fileList.length > 5) {
  console.log(`  ... 还有 ${fileList.length - 5} 个文件`)
}
