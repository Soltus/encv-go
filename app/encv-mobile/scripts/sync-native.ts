#!/usr/bin/env bun
/**
 * 用 bun 运行：bun scripts/sync-native.ts
 * encv-sync-native — 同步原生库到 Android 工程（ESM-only，bun 直跑 .ts）
 */
import { rmSync, mkdirSync, copyFileSync, cpSync, readdirSync, statSync, existsSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const ANDROID_DIR = join(__dirname, '..', 'android')
const OVERLAY_DIR = join(__dirname, '..', 'android-overlay')

console.log('encv-sync-native: syncing native libs to Android project...')

const overlayJni = join(OVERLAY_DIR, 'app', 'src', 'main', 'jniLibs')
const targetJni = join(ANDROID_DIR, 'app', 'src', 'main', 'jniLibs')
const ALLOWED_ABIS = ['arm64-v8a']
if (existsSync(overlayJni)) {
  for (const abi of readdirSync(overlayJni)) {
    if (!ALLOWED_ABIS.includes(abi)) continue
    const abiDir = join(overlayJni, abi)
    if (statSync(abiDir).isDirectory()) {
      const targetAbi = join(targetJni, abi)
      mkdirSync(targetAbi, { recursive: true })
      for (const so of readdirSync(abiDir)) {
        if (so.endsWith('.so')) {
          copyFileSync(join(abiDir, so), join(targetAbi, so))
          console.log(`  so: ${abi}/${so}`)
        }
      }
    }
  }
} else {
  mkdirSync(join(targetJni, 'arm64-v8a'), { recursive: true })
  console.log('  so: ensured jniLibs/arm64-v8a (no overlay)')
}

const overlayJniSrc = join(OVERLAY_DIR, 'app', 'src', 'main', 'jni')
const targetJniSrc = join(ANDROID_DIR, 'app', 'src', 'main', 'jni')
if (existsSync(overlayJniSrc)) {
  if (existsSync(targetJniSrc)) rmSync(targetJniSrc, { recursive: true })
  cpSync(overlayJniSrc, targetJniSrc, { recursive: true })
  console.log('  jni: synced')
}

const overlayInc = join(OVERLAY_DIR, 'app', 'src', 'main', 'include')
const targetInc = join(ANDROID_DIR, 'app', 'src', 'main', 'include')
if (existsSync(overlayInc)) {
  if (existsSync(targetInc)) rmSync(targetInc, { recursive: true })
  cpSync(overlayInc, targetInc, { recursive: true })
  console.log('  include: synced')
}

// Copy config.user.json to Android assets
const configSrc = join(__dirname, '..', '..', '..', 'config.user.json')
const configDest = join(ANDROID_DIR, 'app', 'src', 'main', 'assets', 'config.user.json')
if (existsSync(configSrc)) {
  mkdirSync(dirname(configDest), { recursive: true })
  copyFileSync(configSrc, configDest)
  console.log('  config: config.user.json → assets')
} else {
  console.warn('  config: config.user.json not found at project root (expected for fresh clones)')
}

console.log('encv-sync-native: done')
