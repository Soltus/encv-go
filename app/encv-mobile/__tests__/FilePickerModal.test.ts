import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import FilePickerModal from '@/components/FilePickerModal.vue'

const mockT = vi.hoisted(() => vi.fn((key: string) => key))
const mockAlertPresent = vi.hoisted(() => vi.fn())
const mockAlertCreate = vi.hoisted(() => vi.fn().mockResolvedValue({ present: mockAlertPresent }))
const mockModalDismiss = vi.hoisted(() => vi.fn())

vi.mock('@/api/encv', () => ({
  listFiles: vi.fn().mockResolvedValue([]),
  createDirectory: vi.fn().mockResolvedValue(undefined),
  formatFileSize: vi.fn((bytes?: number) => bytes != null ? `${bytes} B` : ''),
  getFileCategory: vi.fn(() => 'document'),
  PermissionDeniedError: class extends Error {
    constructor(msg: string) { super(msg); this.name = 'PermissionDeniedError' }
  },
}))

vi.mock('@/composables/useI18n', () => ({
  useI18n: () => ({
    t: mockT,
    tField: vi.fn((k: string) => k),
    setLocale: vi.fn(),
    getLocale: vi.fn(() => 'zh-CN'),
    locale: ref('zh-CN'),
  }),
}))

vi.mock('@ionic/vue', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@ionic/vue')>()
  return {
    ...actual,
    alertController: {
      create: mockAlertCreate,
    },
    modalController: {
      dismiss: mockModalDismiss,
    },
  }
})

function mountFolderMode(initialPath = '/') {
  return mount(FilePickerModal, {
    props: { mode: 'folder', initialPath },
    global: {
      stubs: {
        'ion-page': { template: '<div><slot /></div>' },
        'ion-header': { template: '<header><slot /></header>' },
        'ion-toolbar': { template: '<div class="toolbar"><slot /></div>' },
        'ion-buttons': { template: '<div class="btn-group"><slot /></div>' },
        'ion-button': { template: '<button class="ion-btn" @click="$emit(\'click\', $event)"><slot /></button>' },
        'ion-title': { template: '<h1><slot /></h1>' },
        'ion-icon': { template: '<span class="icon" />' },
        'ion-content': { template: '<main><slot /></main>' },
        'ion-list': { template: '<ul class="list"><slot /></ul>' },
        'ion-item': { template: '<li class="item" @click="$emit(\'click\', $event)"><slot /></li>' },
        'ion-label': { template: '<label><slot /></label>' },
        'ion-spinner': { template: '<span class="spinner" />' },
        'ion-input': {
          template: '<input class="ion-input" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @keydown="$emit(\'keydown\', $event)" />',
          props: ['modelValue'],
        },
      },
    },
  })
}

function mountFileMode() {
  return mount(FilePickerModal, {
    props: { mode: 'file' },
    global: {
      stubs: {
        'ion-page': { template: '<div><slot /></div>' },
        'ion-header': { template: '<header><slot /></header>' },
        'ion-toolbar': { template: '<div class="toolbar"><slot /></div>' },
        'ion-buttons': { template: '<div class="btn-group"><slot /></div>' },
        'ion-button': { template: '<button class="ion-btn" @click="$emit(\'click\', $event)"><slot /></button>' },
        'ion-title': { template: '<h1><slot /></h1>' },
        'ion-icon': { template: '<span class="icon" />' },
        'ion-content': { template: '<main><slot /></main>' },
        'ion-list': { template: '<ul class="list"><slot /></ul>' },
        'ion-item': { template: '<li class="item" @click="$emit(\'click\', $event)"><slot /></li>' },
        'ion-label': { template: '<label><slot /></label>' },
        'ion-spinner': { template: '<span class="spinner" />' },
        'ion-input': {
          template: '<input class="ion-input" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" @keydown="$emit(\'keydown\', $event)" />',
          props: ['modelValue'],
        },
      },
    },
  })
}

describe('FilePickerModal - New Folder Feature', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  function getAddButton(wrapper: ReturnType<typeof mount>) {
    const btnGroups = wrapper.findAll('.btn-group')
    const endGroup = btnGroups[btnGroups.length - 1]
    const buttons = endGroup.findAll('.ion-btn')
    const addButton = buttons.find(btn => {
      const hasIcon = btn.find('.icon').exists()
      const hasText = btn.text().trim().length > 0
      return hasIcon && !hasText
    })
    return addButton
  }

  function clickAddButton(wrapper: ReturnType<typeof mount>) {
    const btn = getAddButton(wrapper)
    expect(btn, 'add button should exist in folder mode').toBeDefined()
    return btn!.trigger('click')
  }

  it('shows overlay input when + button clicked (folder mode)', async () => {
    const wrapper = mountFolderMode()
    await wrapper.vm.$nextTick()

    await clickAddButton(wrapper)
    await wrapper.vm.$nextTick()

    const overlay = wrapper.find('.new-folder-input-overlay')
    expect(overlay.exists()).toBe(true)

    const list = wrapper.find('.list')
    expect(list.exists()).toBe(true)
  })

  it('calls createDirectory with correct params on confirm', async () => {
    const { createDirectory } = await import('@/api/encv')
    const wrapper = mountFolderMode('/current')
    await wrapper.vm.$nextTick()

    await clickAddButton(wrapper)
    await wrapper.vm.$nextTick()

    const input = wrapper.find('.new-folder-input-overlay .ion-input')
    expect(input.exists()).toBe(true)
    await input.setValue('test-folder')
    await wrapper.vm.$nextTick()

    const overlayButtons = wrapper.findAll('.new-folder-input-overlay .ion-btn')
    const confirmBtn = overlayButtons[0]
    await confirmBtn.trigger('click')
    await wrapper.vm.$nextTick()

    expect(createDirectory).toHaveBeenCalledWith('/current', 'test-folder')
  })

  it('navigates to new folder after successful creation', async () => {
    const { listFiles } = await import('@/api/encv')
    const wrapper = mountFolderMode('/media')
    await wrapper.vm.$nextTick()

    await clickAddButton(wrapper)
    await wrapper.vm.$nextTick()

    const input = wrapper.find('.new-folder-input-overlay .ion-input')
    await input.setValue('test-folder')

    const overlayButtons = wrapper.findAll('.new-folder-input-overlay .ion-btn')
    await overlayButtons[0].trigger('click')
    await wrapper.vm.$nextTick()

    expect(listFiles).toHaveBeenCalledWith('/media/test-folder')

    const overlay = wrapper.find('.new-folder-input-overlay')
    expect(overlay.exists()).toBe(false)
  })

  it('hides input on cancel and resets name', async () => {
    const wrapper = mountFolderMode()
    await wrapper.vm.$nextTick()

    await clickAddButton(wrapper)
    await wrapper.vm.$nextTick()

    let overlay = wrapper.find('.new-folder-input-overlay')
    expect(overlay.exists()).toBe(true)

    const input = wrapper.find('.new-folder-input-overlay .ion-input')
    await input.setValue('should-be-cleared')

    const overlayButtons = wrapper.findAll('.new-folder-input-overlay .ion-btn')
    const cancelBtn = overlayButtons[1]
    await cancelBtn.trigger('click')
    await wrapper.vm.$nextTick()

    overlay = wrapper.find('.new-folder-input-overlay')
    expect(overlay.exists()).toBe(false)

    const backdrop = wrapper.find('.overlay-backdrop')
    expect(backdrop.exists()).toBe(false)

    expect(wrapper.vm.newFolderName).toBe('')
  })

  it('blocks empty name submission', async () => {
    const { createDirectory } = await import('@/api/encv')
    const wrapper = mountFolderMode()
    await wrapper.vm.$nextTick()

    await clickAddButton(wrapper)
    await wrapper.vm.$nextTick()

    const input = wrapper.find('.new-folder-input-overlay .ion-input')
    await input.setValue('   ')
    await wrapper.vm.$nextTick()

    const overlayButtons = wrapper.findAll('.new-folder-input-overlay .ion-btn')
    await overlayButtons[0].trigger('click')
    await wrapper.vm.$nextTick()

    expect(createDirectory).not.toHaveBeenCalled()

    const overlay = wrapper.find('.new-folder-input-overlay')
    expect(overlay.exists()).toBe(true)
  })

  it('shows alert on API failure', async () => {
    const { createDirectory } = await import('@/api/encv')
    vi.mocked(createDirectory).mockRejectedValueOnce(new Error('Forbidden'))

    const wrapper = mountFolderMode('/')
    await wrapper.vm.$nextTick()

    await clickAddButton(wrapper)
    await wrapper.vm.$nextTick()

    const input = wrapper.find('.new-folder-input-overlay .ion-input')
    await input.setValue('bad-folder')

    const overlayButtons = wrapper.findAll('.new-folder-input-overlay .ion-btn')
    await overlayButtons[0].trigger('click')
    await wrapper.vm.$nextTick()

    expect(mockAlertCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        header: 'files.createFolderFailed',
        message: 'Forbidden',
      }),
    )
  })

  it('+ button not visible in file mode', () => {
    const wrapper = mountFileMode()

    const btnGroups = wrapper.findAll('.btn-group')
    const endGroup = btnGroups[btnGroups.length - 1]
    const buttons = endGroup.findAll('.ion-btn')

    const addBtn = buttons.find(btn => {
      const hasIcon = btn.find('.icon').exists()
      const hasText = btn.text().trim().length > 0
      return hasIcon && !hasText
    })
    expect(addBtn).toBeUndefined()
  })

  it('hides input when backdrop is clicked', async () => {
    const wrapper = mountFolderMode()
    await wrapper.vm.$nextTick()

    await clickAddButton(wrapper)
    await wrapper.vm.$nextTick()

    let backdrop = wrapper.find('.overlay-backdrop')
    expect(backdrop.exists()).toBe(true)

    await backdrop.trigger('click')
    await wrapper.vm.$nextTick()

    backdrop = wrapper.find('.overlay-backdrop')
    expect(backdrop.exists()).toBe(false)

    const overlay = wrapper.find('.new-folder-input-overlay')
    expect(overlay.exists()).toBe(false)
  })

  it('submits on Enter key in input', async () => {
    const { createDirectory } = await import('@/api/encv')
    const wrapper = mountFolderMode('/docs')
    await wrapper.vm.$nextTick()

    await clickAddButton(wrapper)
    await wrapper.vm.$nextTick()

    const input = wrapper.find('.new-folder-input-overlay .ion-input')
    await input.setValue('enter-folder')
    await input.trigger('keydown', { key: 'Enter' })
    await wrapper.vm.$nextTick()

    expect(createDirectory).toHaveBeenCalledWith('/docs', 'enter-folder')
  })

  it('constructs correct new path when currentPath is root "/"', async () => {
    const { listFiles } = await import('@/api/encv')
    const wrapper = mountFolderMode('/')
    await wrapper.vm.$nextTick()

    await clickAddButton(wrapper)
    await wrapper.vm.$nextTick()

    const input = wrapper.find('.new-folder-input-overlay .ion-input')
    await input.setValue('root-subdir')

    const overlayButtons = wrapper.findAll('.new-folder-input-overlay .ion-btn')
    await overlayButtons[0].trigger('click')
    await wrapper.vm.$nextTick()

    expect(listFiles).toHaveBeenCalledWith('/root-subdir')
  })

  it('constructs correct new path when currentPath is nested', async () => {
    const { listFiles } = await import('@/api/encv')
    const wrapper = mountFolderMode('/a/b/c')
    await wrapper.vm.$nextTick()

    await clickAddButton(wrapper)
    await wrapper.vm.$nextTick()

    const input = wrapper.find('.new-folder-input-overlay .ion-input')
    await input.setValue('new-dir')

    const overlayButtons = wrapper.findAll('.new-folder-input-overlay .ion-btn')
    await overlayButtons[0].trigger('click')
    await wrapper.vm.$nextTick()

    expect(listFiles).toHaveBeenCalledWith('/a/b/c/new-dir')
  })
})
