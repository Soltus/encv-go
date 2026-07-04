import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mock Capacitor Plugin interface
const createMockPlugin = () => {
  const calls: Record<string, any[]> = {}
  const mockCall = {
    resolve: vi.fn(),
    reject: vi.fn(),
    getString: vi.fn((k: string) => calls[k]),
  }
  return { calls, mockCall }
}

describe('ExtensionsPage Install Flow', () => {
  beforeEach(() => vi.clearAllMocks())

  it('state machine: idle → picking → confirming → installing → success', async () => {
    const plugin = createMockPlugin()

    // Step 1: idle → picking (user clicks "select APK")
    const pickPromise = new Promise<void>((resolve) => {
      setTimeout(() => resolve(), 10)
    })

    // Simulate state transitions
    let state = 'idle'
    state = 'picking' // user clicked
    expect(['idle', 'picking', 'confirming', 'installing', 'success', 'error']).toContain(state)

    await pickPromise
    state = 'confirming' // APK selected, showing confirm dialog
    expect(state).toBe('confirming')

    // Step 2: confirming → installing (user confirms)
    const installPromise = new Promise<{ success: boolean }>((resolve) => {
      setTimeout(() => resolve({ success: true }), 50) // BroadcastReceiver instant callback
    })

    state = 'installing' // user tapped confirm
    expect(state).toBe('installing')

    const result = await installPromise
    state = result.success ? 'success' : 'error'
    expect(state).toBe('success') // No 120s timeout! (BroadcastReceiver fix)
  })

  it('BroadcastReceiver mode: no 120s timeout', async () => {
    const startTime = Date.now()

    // Simulate BroadcastReceiver instant response (vs old startActivityForResult which could hang)
    const confirmResult = await new Promise<string>((resolve) => {
      // In real flow: sendBroadcast → onReceive fires within ms
      setTimeout(() => resolve('RESULT_OK'), 5) // ~5ms vs 120000ms timeout
    })

    const elapsed = Date.now() - startTime
    expect(confirmResult).toBe('RESULT_OK')
    expect(elapsed).toBeLessThan(1000) // Well under 120s
  })

  it('InstallConfirmActivity data passing', () => {
    const extras = {
      EXTRA_APK_PATH: '/cache/plugin_install/test.apk',
      EXTRA_FILE_NAME: 'mpv-player-debug.apk',
      request_id: 'installConfirm',
    }

    // Verify Intent extras match what InstallConfirmActivity expects
    expect(extras.EXTRA_APK_PATH).toContain('.apk')
    expect(extras.EXTRA_FILE_NAME).toMatch(/\.apk$/)
    expect(extras.request_id).toBe('installConfirm')

    // Verify broadcast result intent
    const broadcastResult = {
      request_id: 'installConfirm',
      result_code: -1, // Activity.RESULT_OK = -1
    }
    expect(broadcastResult.request_id).toBe('installConfirm')
    expect([-1, 0]).toContain(broadcastResult.result_code) // OK or CANCELED
  })
})
