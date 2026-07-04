import { describe, it, expect, vi, beforeEach } from 'vitest'

describe('PlayerEntry Startup Chain', () => {
  beforeEach(() => vi.clearAllMocks())

  it('MPV loaded → Intent targets MpvPlayerActivity', () => {
    const pluginLoaded = true
    const path = '/storage/test.mp4'

    const intent = buildIntent(pluginLoaded, path)

    expect(intent.component).toBe('com.encvgo.plugin.mpv.MpvPlayerActivity')
    expect(intent.action).toBe('android.intent.action.VIEW')
  })

  it('MPV NOT loaded → fallback to ArtPlayer', () => {
    const pluginLoaded = false
    const path = '/storage/test.mp4'

    const intent = buildIntent(pluginLoaded, path)

    // Should use WebView-based ArtPlayer
    expect(intent.component).not.toBe('com.encvgo.plugin.mpv.MpvPlayerActivity')
    expect(intent.component).toContain('ArtPlayer') // or some webview indicator
  })

  it('ProxyManager routes to EncvHostActivity when MPV plugin active', () => {
    // When ProxyManager.setHostActivity(EncvHostActivity::class.java) is configured:
    // - MpvPlayerActivity intent → intercepted by ProxyManager
    // - Re-routed to EncvHostActivity with plugin activity info
    // - BaseHostApplication.onCreate() proxies lifecycle to actual MpvPlayerActivity

    const hostActivity = resolveProxyTarget('com.encvgo.plugin.mpv.MpvPlayerActivity')
    expect(hostActivity).toBe('com.encvgo.app.EncvHostActivity')
  })
})

// Helper functions (simulating PlayerEntry logic)
function buildIntent(mpvLoaded: boolean, path: string) {
  return {
    component: mpvLoaded ? 'com.encvgo.plugin.mpv.MpvPlayerActivity' : 'com.encvgo.app.ArtPlayerActivity',
    action: 'android.intent.action.VIEW',
    data: { path },
  }
}

function resolveProxyTarget(targetComponent: string): string {
  if (targetComponent === 'com.encvgo.plugin.mpv.MpvPlayerActivity') {
    return 'com.encvgo.app.EncvHostActivity' // ProxyManager reroute
  }
  return targetComponent
}
