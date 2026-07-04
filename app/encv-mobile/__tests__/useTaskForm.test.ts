import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useTaskForm } from '@/composables/useTaskForm'

vi.mock('@/api/encv', () => ({
  predictPlugin: vi.fn().mockResolvedValue({
    candidates: [
      {
        name: 'text',
        matchType: 'extension' as const,
        priority: 0,
        taskOptions: {
          passwordStrategy: 'global' as const,
          supportVersionSelect: true,
          supportedVersions: [2, 3, 4],
          defaultVersion: 4,
        },
      },
      {
        name: 'alist_encrypt',
        matchType: 'general' as const,
        priority: 1,
        taskOptions: {
          passwordStrategy: 'independent' as const,
          supportVersionSelect: false,
          extraFields: [
            {
              key: 'plugin_password',
              label: 'task.pluginPassword',
              type: 'password',
              required: false,
              help: 'task.pluginPasswordHelp',
            },
          ],
        },
      },
    ],
  }),
}))

describe('useTaskForm - initFromQuery (Files→Tasks 跳转初始化)', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('initFromQuery should reset state then call predictPlugin', async () => {
    const { initFromQuery, candidates, selectedPluginIndex, extraValues, primaryOverride, secondaryPassword } = useTaskForm()

    candidates.value = [{ name: 'old', matchType: 'mime', priority: 0, taskOptions: null }]
    selectedPluginIndex.value = 5
    extraValues.value = { old_key: 'old_val' }
    primaryOverride.value = 'old_override'
    secondaryPassword.value = 'old_secondary'

    const promise = initFromQuery({ sourcePath: '/test.md', taskType: 'encrypt' })

    await vi.runAllTimersAsync()
    await promise

    expect(candidates.value.length).toBeGreaterThan(0)
    expect(selectedPluginIndex.value).toBe(0)
    expect(extraValues.value).toEqual({})
    expect(primaryOverride.value).toBe('')
    expect(secondaryPassword.value).toBe('')
  })

  it('initFromQuery with independent plugin should populate extraFields defaults', async () => {
    const { initFromQuery, candidates, selectedPluginIndex, extraValues, taskOptions } = useTaskForm()

    const promise = initFromQuery({ sourcePath: '/test.md', taskType: 'encrypt' })
    await vi.runAllTimersAsync()
    await promise

    expect(candidates.value.length).toBeGreaterThan(1)

    selectedPluginIndex.value = 1

    expect(taskOptions.value?.passwordStrategy).toBe('independent')
  })

  it('reset should clear all form state including pending predict timer', () => {
    const { reset, candidates, selectedPluginIndex, extraValues, primaryOverride, secondaryPassword } = useTaskForm()

    candidates.value = [{ name: 'x', matchType: 'mime', priority: 0, taskOptions: null }]
    selectedPluginIndex.value = 3
    extraValues.value = { a: 'b' }
    primaryOverride.value = 'pw'
    secondaryPassword.value = 'sec'

    reset()

    expect(candidates.value).toEqual([])
    expect(selectedPluginIndex.value).toBe(0)
    expect(extraValues.value).toEqual({})
    expect(primaryOverride.value).toBe('')
    expect(secondaryPassword.value).toBe('')
  })

  it('predictedPlugin derives from candidates[selectedPluginIndex]', () => {
    const { predictedPlugin, candidates, selectedPluginIndex } = useTaskForm()

    expect(predictedPlugin.value).toBeNull()

    candidates.value = [
      { name: 'video', matchType: 'mime', priority: 0, taskOptions: null },
      { name: 'alist_encrypt', matchType: 'general', priority: 1, taskOptions: null },
    ]

    expect(predictedPlugin.value).toBe('video')

    selectedPluginIndex.value = 1
    expect(predictedPlugin.value).toBe('alist_encrypt')

    candidates.value = []
    expect(predictedPlugin.value).toBeNull()
  })

  it('taskOptions derives from candidates[selectedPluginIndex].taskOptions', () => {
    const { taskOptions, candidates } = useTaskForm()

    expect(taskOptions.value).toBeNull()

    const mockOpts = {
      passwordStrategy: 'global' as const,
      supportVersionSelect: true,
      supportedVersions: [2, 3, 4],
      defaultVersion: 4,
    }

    candidates.value = [
      { name: 'video', matchType: 'mime', priority: 0, taskOptions: mockOpts },
    ]

    expect(taskOptions.value).toEqual(mockOpts)

    candidates.value = []
    expect(taskOptions.value).toBeNull()
  })

  it('selecting different plugin resets extraValues to new defaults', async () => {
    const { candidates, selectedPluginIndex, extraValues, taskOptions } = useTaskForm()

    candidates.value = [
      {
        name: 'video',
        matchType: 'mime',
        priority: 0,
        taskOptions: {
          passwordStrategy: 'global' as const,
          supportVersionSelect: true,
          supportedVersions: [2, 3, 4],
          defaultVersion: 4,
        },
      },
      {
        name: 'alist_encrypt',
        matchType: 'general',
        priority: 1,
        taskOptions: {
          passwordStrategy: 'independent' as const,
          supportVersionSelect: false,
          extraFields: [
            { key: 'plugin_password', label: 'PW', type: 'password', required: false, defaultValue: '', help: '' },
          ],
        },
      },
    ]

    selectedPluginIndex.value = 0
    expect(extraValues.value).toEqual({})

    selectedPluginIndex.value = 1
    expect(taskOptions.value?.extraFields).toBeDefined()
  })

  it('getExtraPayload returns only non-empty values', () => {
    const { getExtraPayload, extraValues } = useTaskForm()

    extraValues.value = { plugin_password: '', note: 'hello' }
    expect(getExtraPayload()).toEqual({ note: 'hello' })

    extraValues.value = {}
    expect(getExtraPayload()).toEqual({})

    extraValues.value = { plugin_password: 'secret123' }
    expect(getExtraPayload()).toEqual({ plugin_password: 'secret123' })
  })
})
