import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  registerFileFeature,
  unregisterFileFeature,
  useFileFeatures,
} from '@/composables/useFileFeatures'
import type { FileFeature, FileBadge, FileSubtitle, FileAction } from '@/types/file-feature'

const mockFile = { name: 'test.bin', path: '/test.bin', isDirectory: false }

describe('useFileFeatures - registry lifecycle', () => {
  beforeEach(() => {
    const { allFeatures } = useFileFeatures()
    allFeatures.value.forEach((f) => unregisterFileFeature(f.id))
  })

  it('register → version increments → feature appears in allFeatures', () => {
    const { version, allFeatures } = useFileFeatures()
    const v0 = version.value

    const feature: FileFeature = {
      id: 'test-feat',
      isActive: () => true,
      onActivate: vi.fn(),
      onDeactivate: vi.fn(),
    }
    registerFileFeature(feature)

    expect(version.value).toBe(v0 + 1)
    expect(allFeatures.value).toHaveLength(1)
    expect(allFeatures.value[0].id).toBe('test-feat')
    expect(feature.onActivate).toHaveBeenCalledOnce()
  })

  it('unregister → feature removed → onDeactivate called', () => {
    const feature: FileFeature = {
      id: 'to-remove',
      isActive: () => true,
      onDeactivate: vi.fn(),
    }
    registerFileFeature(feature)

    const { allFeatures } = useFileFeatures()
    unregisterFileFeature('to-remove')

    expect(allFeatures.value.find((f) => f.id === 'to-remove')).toBeUndefined()
    expect(feature.onDeactivate).toHaveBeenCalledOnce()
  })

  it('duplicate registration is skipped with warning', () => {
    const warnSpy = vi.spyOn(console, 'debug').mockImplementation(() => {})
    const feature: FileFeature = { id: 'dup', isActive: () => true }
    registerFileFeature(feature)
    registerFileFeature(feature)

    expect(warnSpy).toHaveBeenCalledWith(
      expect.stringContaining('dup'),
    )
    warnSpy.mockRestore()
  })
})

describe('useFileFeatures - getBadges', () => {
  beforeEach(() => {
    const { allFeatures } = useFileFeatures()
    allFeatures.value.forEach((f) => unregisterFileFeature(f.id))
  })

  it('collects badges from active features with getBadge', async () => {
    const badgeA: FileBadge = { text: 'AE', color: 'danger' }
    registerFileFeature({
      id: 'badge-a',
      isActive: () => true,
      getBadge: () => badgeA,
    })
    registerFileFeature({
      id: 'no-badge',
      isActive: () => true,
      getBadge: () => null,
    })

    const { getBadges } = useFileFeatures()
    const badges = await getBadges(mockFile as any)

    expect(badges).toHaveLength(1)
    expect(badges[0]).toEqual(badgeA)
  })

  it('skips inactive features', async () => {
    registerFileFeature({
      id: 'inactive',
      isActive: () => false,
      getBadge: () => ({ text: 'X', color: 'warn' }),
    })

    const { getBadges } = useFileFeatures()
    const badges = await getBadges(mockFile as any)

    expect(badges).toHaveLength(0)
  })

  it('isolates errors from individual features', async () => {
    const warnSpy = vi.spyOn(console, 'debug').mockImplementation(() => {})
    registerFileFeature({
      id: 'exploding',
      isActive: () => true,
      getBadge: () => {
        throw new Error('boom')
      },
    })

    const { getBadges } = useFileFeatures()
    const badges = await getBadges(mockFile as any)

    expect(badges).toHaveLength(0)
    expect(warnSpy).toHaveBeenCalled()
    warnSpy.mockRestore()
  })
})

describe('useFileFeatures - getSubtitles', () => {
  beforeEach(() => {
    const { allFeatures } = useFileFeatures()
    allFeatures.value.forEach((f) => unregisterFileFeature(f.id))
  })

  it('collects subtitles from active features', async () => {
    registerFileFeature({
      id: 'sub-feat',
      isActive: () => true,
      getSubtitle: () => ({ text: 'real: video.mp4', color: 'red' }),
    })

    const { getSubtitles } = useFileFeatures()
    const subs = await getSubtitles(mockFile as any)

    expect(subs).toHaveLength(1)
    expect(subs[0].text).toContain('video.mp4')
  })
})

describe('useFileFeatures - getAllActions', () => {
  beforeEach(() => {
    const { allFeatures } = useFileFeatures()
    allFeatures.value.forEach((f) => unregisterFileFeature(f.id))
  })

  it('filters out actions where visible returns false', async () => {
    const actionVisible: FileAction = {
      id: 'visible-action',
      text: () => 'Show',
      icon: 'eye',
      visible: () => true,
      handler: vi.fn(),
    }
    const actionHidden: FileAction = {
      id: 'hidden-action',
      text: () => 'Hide',
      icon: 'eye-off',
      visible: () => false,
      handler: vi.fn(),
    }
    registerFileFeature({
      id: 'actions-feat',
      isActive: () => true,
      getFileActions: () => [actionVisible, actionHidden],
    })

    const { getAllActions } = useFileFeatures()
    const actions = await getAllActions(mockFile as any)

    expect(actions).toHaveLength(1)
    expect(actions[0].id).toBe('visible-action')
  })

  it('handles getFileActions throwing error gracefully', async () => {
    const warnSpy = vi.spyOn(console, 'debug').mockImplementation(() => {})
    registerFileFeature({
      id: 'action-error',
      isActive: () => true,
      getFileActions: () => {
        throw new Error('actions boom')
      },
    })

    const { getAllActions } = useFileFeatures()
    const actions = await getAllActions(mockFile as any)

    expect(actions).toHaveLength(0)
    expect(warnSpy).toHaveBeenCalled()
    warnSpy.mockRestore()
  })
})
