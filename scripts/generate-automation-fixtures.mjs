#!/usr/bin/env node
/**
 * 自动化插件测试数据生成器
 *
 * 生成两种格式的 fixture：
 *   1. tasks.sql  — SQLite INSERT 语句，可直接导入数据库
 *   2. tasks.json  — 前端 Cypress 测试用的 mock 数据
 *   3. run-summary.json  — run summary 数据（后端 SQL 权威计数）
 *
 * 场景：7 个 plugin × encrypt+decrypt × 多种状态 = 1000+ task
 *
 * 用法：
 *   node scripts/generate-automation-fixtures.mjs
 *   # 输出到 fixtures/ 目录
 *
 *   # 自定义任务数
 *   TASKS_PER_PLUGIN=200 node scripts/generate-automation-fixtures.mjs
 */

import fs from 'fs'
import path from 'path'
import { fileURLToPath } from 'url'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const ROOT = path.resolve(__dirname, '..')

// ============================================================================
// 配置
// ============================================================================

const TASKS_PER_PLUGIN = parseInt(process.env.TASKS_PER_PLUGIN || '150', 10)
const RUN_ID = process.env.RUN_ID || 'run-automation-20260623'
const TRIGGERED_BY = 'automation'
const BASE_TIME = new Date('2026-06-23T10:00:00.000Z').getTime()

// 7 个 plugin（模拟真实自动化测试场景）
const PLUGINS = [
  { name: 'mp4-encrypt', ext: 'mp4', type: 'video' },
  { name: 'mkv-encrypt', ext: 'mkv', type: 'video' },
  { name: 'png-encrypt', ext: 'png', type: 'image' },
  { name: 'pdf-encrypt', ext: 'pdf', type: 'pdf' },
  { name: 'mp3-encrypt', ext: 'mp3', type: 'audio' },
  { name: 'docx-encrypt', ext: 'docx', type: 'wps' },
  { name: 'zip-encrypt', ext: 'zip', type: 'archive' },
]

// 状态分布（模拟真实测试运行中：部分完成、部分失败、部分运行中、部分排队）
const STATUS_MIX = [
  { status: 'completed', ratio: 0.45, hasError: false },
  { status: 'failed', ratio: 0.08, hasError: true },
  { status: 'running', ratio: 0.15, hasError: false },
  { status: 'queued', ratio: 0.25, hasError: false },
  { status: 'cancelled', ratio: 0.07, hasError: false },
]

// 任务类型：encrypt + decrypt
const TASK_TYPES = ['encrypt', 'decrypt']

// ============================================================================
// 生成函数
// ============================================================================

function pickStatus(index) {
  const totalRatio = STATUS_MIX.reduce((a, b) => a + b.ratio, 0)
  let acc = 0
  const pos = (index % 1000) / 1000 * totalRatio
  for (const s of STATUS_MIX) {
    acc += s.ratio
    if (pos < acc) return s
  }
  return STATUS_MIX[0]
}

function generateTask(index, plugin, taskType) {
  const statusInfo = pickStatus(index)
  const status = statusInfo.status
  const i = index
  const createdTs = BASE_TIME + i * 800 // 每个 task 间隔 800ms
  const taskId = `task-automation-${String(i).padStart(6, '0')}`

  let progress = 0
  let completedAt = null
  let phase = null
  let speed = null
  let eta = null
  let error = null
  let errorDetail = null

  if (status === 'running') {
    progress = (i % 80) + 10
    phase = taskType === 'encrypt' ? 'encrypting' : 'decrypting'
    speed = `${(i % 20) + 5}.${i % 10} MB/s`
    eta = `${(i % 120) + 10}s`
  } else if (status === 'completed') {
    progress = 100
    completedAt = new Date(createdTs + 30000 + i * 200).toISOString()
  } else if (status === 'failed') {
    progress = Math.floor((i % 60) + 10)
    completedAt = new Date(createdTs + 15000 + i * 150).toISOString()
    error = `${taskType} failed: invalid password`
    errorDetail = `Detailed error message for task ${taskId}\nstack trace line 1\nstack trace line 2`
  } else if (status === 'cancelled') {
    progress = Math.floor((i % 40) + 5)
    completedAt = new Date(createdTs + 8000 + i * 100).toISOString()
  }

  const cipherMode = i % 2
  const compressionMode = i % 3 === 0 ? 'none' : i % 3 === 1 ? 'zstd' : 'gzip'

  const sourcePath = `/storage/emulated/0/encv-test-media/${plugin.type}/sample-${String(i).padStart(4, '0')}.${plugin.ext}`
  const targetPath = taskType === 'encrypt'
    ? `/storage/emulated/0/encv-test-results/sample-${String(i).padStart(4, '0')}.${plugin.ext}.encv`
    : `/storage/emulated/0/encv-test-results/sample-${String(i).padStart(4, '0')}.${plugin.ext}`

  return {
    id: taskId,
    type: taskType,
    status,
    source_path: sourcePath,
    target_path: targetPath,
    output_path: targetPath,
    plugin_name: plugin.name,
    triggered_by: TRIGGERED_BY,
    run_id: RUN_ID,
    progress,
    phase,
    error,
    error_detail: errorDetail,
    warning: null,
    warning_detail: null,
    container_version: 4,
    cipher_mode: cipherMode,
    compression_mode: compressionMode,
    extra_fields: JSON.stringify({
      passwordHint: 'test123',
      testSuite: `suite-${i % 5}`,
      testCase: `case-${i % 20}`,
    }),
    steps: null,
    mount_id: null,
    mount_sub_path: null,
    target_mount_id: null,
    target_mount_sub_path: null,
    password: null,
    secondary_password: null,
    created_at: new Date(createdTs).toISOString(),
    completed_at: completedAt,
    rollback_of: null,
    original_path: null,
    // 前端额外字段（不在 SQL 表中）
    speed,
    eta,
  }
}

function generateAllTasks() {
  const tasks = []
  let index = 0

  for (const plugin of PLUGINS) {
    for (const taskType of TASK_TYPES) {
      for (let i = 0; i < TASKS_PER_PLUGIN; i++) {
        tasks.push(generateTask(index, plugin, taskType))
        index++
      }
    }
  }

  return tasks
}

function computeRunSummary(tasks) {
  const total = tasks.length
  let passed = 0
  let failed = 0
  let running = 0
  let pending = 0
  let cancelled = 0

  for (const t of tasks) {
    switch (t.status) {
      case 'completed':
        passed++
        break
      case 'failed':
        failed++
        break
      case 'running':
        running++
        break
      case 'queued':
      case 'pending':
        pending++
        break
      case 'cancelled':
        cancelled++
        break
    }
  }

  const finished = passed + failed
  const percent = total > 0 ? Math.round((finished / total) * 100) : 0

  return {
    runId: RUN_ID,
    total,
    passed,
    failed,
    running,
    pending,
    cancelled,
    percent,
  }
}

// ============================================================================
// 输出生成
// ============================================================================

function generateSQL(tasks) {
  const lines = [
    '-- ============================================================',
    '-- 自动化插件测试 fixture (1000+ tasks)',
    `-- 生成时间: ${new Date().toISOString()}`,
    `-- Run ID: ${RUN_ID}`,
    `-- 总任务数: ${tasks.length}`,
    '-- ============================================================',
    '',
    'BEGIN TRANSACTION;',
    '',
  ]

  for (const t of tasks) {
    const values = [
      `'${t.id}'`,                                    // id
      `'${t.type}'`,                                  // type
      `'${t.status}'`,                                // status
      t.source_path ? `'${escapeSql(t.source_path)}'` : 'NULL',    // source_path
      t.target_path ? `'${escapeSql(t.target_path)}'` : 'NULL',    // target_path
      t.output_path ? `'${escapeSql(t.output_path)}'` : 'NULL',    // output_path
      t.plugin_name ? `'${t.plugin_name}'` : 'NULL',               // plugin_name
      `'${t.triggered_by}'`,                                      // triggered_by
      t.run_id ? `'${t.run_id}'` : 'NULL',                        // run_id
      String(t.progress || 0),                                    // progress
      t.phase ? `'${t.phase}'` : 'NULL',                          // phase
      t.error ? `'${escapeSql(t.error)}'` : 'NULL',               // error
      t.error_detail ? `'${escapeSql(t.error_detail)}'` : 'NULL', // error_detail
      'NULL',  // warning
      'NULL',  // warning_detail
      String(t.container_version || 4),                           // container_version
      String(t.cipher_mode ?? 0),                                 // cipher_mode
      t.compression_mode ? `'${t.compression_mode}'` : 'NULL',    // compression_mode
      t.extra_fields ? `'${escapeSql(t.extra_fields)}'` : 'NULL', // extra_fields
      'NULL',  // steps
      'NULL',  // mount_id
      'NULL',  // mount_sub_path
      'NULL',  // target_mount_id
      'NULL',  // target_mount_sub_path
      'NULL',  // password
      'NULL',  // secondary_password
      `'${t.created_at}'`,                                        // created_at
      t.completed_at ? `'${t.completed_at}'` : 'NULL',            // completed_at
      'NULL',  // rollback_of
      'NULL',  // original_path
    ].join(', ')

    lines.push(`INSERT INTO tasks VALUES (${values});`)
  }

  lines.push('', 'COMMIT;', '')
  return lines.join('\n')
}

function escapeSql(str) {
  return String(str).replace(/'/g, "''")
}

function generateFrontendTasks(tasks) {
  return tasks.map((t) => ({
    id: t.id,
    type: t.type,
    sourcePath: t.source_path,
    targetPath: t.target_path,
    outputPath: t.output_path,
    pluginName: t.plugin_name,
    triggeredBy: t.triggered_by,
    runId: t.run_id,
    progress: t.progress,
    phase: t.phase,
    speed: t.speed,
    eta: t.eta,
    error: t.error,
    errorDetail: t.error_detail,
    containerVersion: t.container_version,
    cipherMode: t.cipher_mode,
    compressionMode: t.compression_mode,
    extraFields: t.extra_fields ? JSON.parse(t.extra_fields) : {},
    createdAt: t.created_at,
    completedAt: t.completed_at,
    status: t.status,
  }))
}

// ============================================================================
// 主程序
// ============================================================================

function main() {
  console.log('🚀 生成自动化测试 fixture...')
  console.log(`   每个 plugin 任务数: ${TASKS_PER_PLUGIN}`)
  console.log(`   Plugin 数量: ${PLUGINS.length}`)
  console.log(`   任务类型: ${TASK_TYPES.join(', ')}`)
  console.log(`   预计总数: ${PLUGINS.length * TASK_TYPES.length * TASKS_PER_PLUGIN}`)

  const tasks = generateAllTasks()
  const summary = computeRunSummary(tasks)

  console.log(`\n✅ 生成完成，共 ${tasks.length} 个任务`)
  console.log(`   - passed: ${summary.passed}`)
  console.log(`   - failed: ${summary.failed}`)
  console.log(`   - running: ${summary.running}`)
  console.log(`   - pending: ${summary.pending}`)
  console.log(`   - cancelled: ${summary.cancelled}`)
  console.log(`   - percent: ${summary.percent}%`)

  // 输出目录
  const outDir = path.join(ROOT, 'fixtures', 'automation')
  fs.mkdirSync(outDir, { recursive: true })

  // 1. SQL 文件
  const sqlPath = path.join(outDir, 'tasks.sql')
  fs.writeFileSync(sqlPath, generateSQL(tasks))
  console.log(`\n📄 SQL 文件: ${sqlPath}`)

  // 2. 前端 Cypress mock 数据
  const frontendTasks = generateFrontendTasks(tasks)
  const jsonPath = path.join(outDir, 'tasks.json')
  fs.writeFileSync(jsonPath, JSON.stringify(frontendTasks, null, 2))
  console.log(`📄 JSON 文件: ${jsonPath}`)

  // 3. Run summary
  const summaryPath = path.join(outDir, 'run-summary.json')
  fs.writeFileSync(summaryPath, JSON.stringify(summary, null, 2))
  console.log(`📄 Run summary: ${summaryPath}`)

  // 4. Cypress fixture 格式（按 plugin 分类的索引）
  const byPlugin = {}
  for (const plugin of PLUGINS) {
    byPlugin[plugin.name] = frontendTasks.filter((t) => t.pluginName === plugin.name)
  }
  const byPluginPath = path.join(outDir, 'tasks-by-plugin.json')
  fs.writeFileSync(byPluginPath, JSON.stringify(byPlugin, null, 2))
  console.log(`📄 按 plugin 分类: ${byPluginPath}`)

  console.log('\n🎉 全部完成！')
}

main()
