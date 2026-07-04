/**
 * verify-mp4.ts
 *
 * 🆕 2026-06-10 用户报告"真机生成的 mock 视频是错误的"诊断
 *
 * 跑 createMP4() 生成实际字节，落到 /tmp，然后 ffprobe 看：
 *  - 能不能解析 container
 *  - duration / bit_rate / streams
 *  - 哪个 box 长度不合法
 *
 * 如果 ffprobe 报 Invalid data / moov atom not found / duration=NaN，
 * 说明 mock mp4 在真机上无法播放 — 必须修。
 */

import { writeFileSync, mkdirSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createMP4 } from '../app/encv-mobile/src/lib/mockDataGenerator.ts'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const OUT_DIR = resolve(__dirname, '..', '/tmp/mock-verify')
mkdirSync(OUT_DIR, { recursive: true })
const OUT_FILE = resolve(OUT_DIR, 'sample.mp4')

const data = createMP4()
writeFileSync(OUT_FILE, data)

console.log('=== 生成 mock mp4 ===')
console.log(`  size: ${data.length} bytes`)
console.log(`  path: ${OUT_FILE}`)
console.log()
console.log('=== 前 32 字节 hex（ftyp box）===')
console.log('  ' + Array.from(data.slice(0, 32)).map(b => b.toString(16).padStart(2, '0')).join(' '))
console.log()
console.log('=== 提示：手动跑 ffprobe ===')
console.log(`  ffprobe -v error -show_format -show_streams ${OUT_FILE}`)
console.log(`  ffprobe -v error -show_packets ${OUT_FILE}  # 看 packet 解析`)
