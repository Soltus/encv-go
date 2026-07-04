import { execSync } from 'child_process'
import { existsSync } from 'fs'
import { join, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const ANDROID_DIR = join(__dirname, '..', 'android')

console.log('=== Kotlin Type Check ===\n')

if (!existsSync(join(ANDROID_DIR, 'gradlew'))) {
  console.error('❌ android/gradlew not found. Run "npx cap sync android" first.')
  process.exit(1)
}

console.log('\n[1/1] Running compileDebugKotlin...')
try {
  execSync('./gradlew compileDebugKotlin --no-daemon --stacktrace', {
    cwd: ANDROID_DIR,
    stdio: 'inherit',
    env: { ...process.env }
  })
} catch (e) {
  console.error('\n❌ Kotlin compilation failed — see errors above')
  process.exit(1)
}

console.log('\n✅ All Kotlin files type-checked successfully')
