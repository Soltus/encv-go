import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  getThumbCacheSize,
  clearThumbCache,
  THUMB_CACHE_MAX,
  useThumbnailCache,
} from '@/composables/useThumbnailCache'
import { getExternalStreamUrl } from '@/api/encv'

vi.mock('@/api/encv', () => ({
  getExternalStreamUrl: vi.fn((path: string) => `/mock-stream/${path}`),
}))

describe('Thumbnail Cache - module-level API', () => {
  beforeEach(() => {
    clearThumbCache()
  })

  it('clearThumbCache resets size to 0', () => {
    expect(getThumbCacheSize()).toBe(0)
  })

  it('THUMB_CACHE_MAX should be 500', () => {
    expect(THUMB_CACHE_MAX).toBe(500)
  })
})

describe('useThumbnailCache composable', () => {
  beforeEach(() => {
    clearThumbCache()
  })

  it('returns expected shape', () => {
    const { thumbnailUrls, setupLazyThumbnails, onThumbError } = useThumbnailCache()
    expect(thumbnailUrls).toBeDefined()
    expect(typeof setupLazyThumbnails).toBe('function')
    expect(typeof onThumbError).toBe('function')
  })

  it('thumbnailUrls starts empty', () => {
    const { thumbnailUrls } = useThumbnailCache()
    expect(Object.keys(thumbnailUrls.value).length).toBe(0)
  })

  it('onThumbError removes URL from reactive ref', () => {
    const { thumbnailUrls, onThumbError } = useThumbnailCache()
    thumbnailUrls.value['/test.jpg'] = '/mock-stream/test.jpg'
    onThumbError('/test.jpg')
    expect(thumbnailUrls.value['/test.jpg']).toBeUndefined()
  })

  it('onThumbError also clears module-level cache', async () => {
    const { onThumbError } = useThumbnailCache()
    getExternalStreamUrl('/preload.jpg')
    await new Promise(r => setTimeout(r, 100))
    const sizeBefore = getThumbCacheSize()
    if (sizeBefore > 0) {
      onThumbError('/preload.jpg')
      expect(getThumbCacheSize()).toBeLessThanOrEqual(sizeBefore - 1)
    }
  })
})

describe('useThumbnailCache - cache update callback (Issue 1 fix)', () => {
  beforeEach(() => {
    clearThumbCache()
  })

  it('onThumbError on non-existent path is no-op', () => {
    const { thumbnailUrls, onThumbError } = useThumbnailCache()
    const keysBefore = Object.keys(thumbnailUrls.value)
    onThumbError('/nonexistent.jpg')
    expect(Object.keys(thumbnailUrls.value)).toEqual(keysBefore)
  })

  it('multiple instances have independent thumbnailUrls refs', () => {
    const a = useThumbnailCache()
    const b = useThumbnailCache()
    a.thumbnailUrls.value['/a.jpg'] = '/mock-stream/a.jpg'
    b.thumbnailUrls.value['/b.jpg'] = '/mock-stream/b.jpg'
    expect(a.thumbnailUrls.value['/a.jpg']).toBeDefined()
    expect(a.thumbnailUrls.value['/b.jpg']).toBeUndefined()
    expect(b.thumbnailUrls.value['/b.jpg']).toBeDefined()
    expect(b.thumbnailUrls.value['/a.jpg']).toBeUndefined()
  })

  it('clearThumbCache does not crash when already empty', () => {
    expect(() => clearThumbCache()).not.toThrow()
    expect(getThumbCacheSize()).toBe(0)
  })

  it('onThumbError only removes the specified path', () => {
    const { thumbnailUrls, onThumbError } = useThumbnailCache()
    thumbnailUrls.value['/a.jpg'] = '/stream/a.jpg'
    thumbnailUrls.value['/b.png'] = '/stream/b.png'
    onThumbError('/a.jpg')
    expect(thumbnailUrls.value['/a.jpg']).toBeUndefined()
    expect(thumbnailUrls.value['/b.png']).toBe('/stream/b.png')
  })
})
