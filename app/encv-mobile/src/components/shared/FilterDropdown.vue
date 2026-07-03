<template>
  <div class="filter-dropdown" ref="dropdownRef">
    <button
      class="dropdown-trigger"
      @click="toggleOpen"
      :class="{ 'is-open': isOpen }"
    >
      <span class="trigger-label">{{ triggerLabel }}</span>
      <span v-if="selectedCount > 0" class="trigger-count">{{ selectedCount }}</span>
      <span class="trigger-arrow">▾</span>
    </button>

    <Transition name="dropdown-fade">
      <div v-if="isOpen" class="dropdown-panel" @click.stop>
        <div v-if="searchable" class="dropdown-search">
          <input
            ref="searchInputRef"
            v-model="searchQuery"
            type="text"
            class="search-input"
            :placeholder="searchPlaceholder"
            @input="onSearchInput"
          />
        </div>
        <div class="dropdown-list" ref="listRef">
          <div
            v-for="option in filteredOptions"
            :key="option.value"
            class="dropdown-item"
            :class="{ 'is-selected': isSelected(option.value) }"
            @click="toggleOption(option.value)"
          >
            <span v-if="multiSelect" class="item-checkbox">
              <span v-if="isSelected(option.value)" class="check-mark">✓</span>
            </span>
            <span class="item-label">{{ option.label }}</span>
            <span v-if="option.count != null" class="item-count">{{ option.count }}</span>
          </div>
          <div v-if="filteredOptions.length === 0" class="dropdown-empty">
            {{ emptyText }}
          </div>
        </div>
        <div v-if="multiSelect && showActions" class="dropdown-actions">
          <button class="action-btn" @click="selectAll">{{ selectAllText }}</button>
          <button class="action-btn" @click="clearAll">{{ clearAllText }}</button>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount, nextTick } from 'vue'

export interface DropdownOption {
  value: string
  label: string
  count?: number
}

interface Props {
  options: DropdownOption[]
  modelValue: string[]
  label: string
  multiSelect?: boolean
  searchable?: boolean
  searchPlaceholder?: string
  emptyText?: string
  selectAllText?: string
  clearAllText?: string
  showActions?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  multiSelect: true,
  searchable: false,
  searchPlaceholder: '搜索...',
  emptyText: '无匹配项',
  selectAllText: '全选',
  clearAllText: '清空',
  showActions: true,
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string[]): void
  (e: 'change', value: string[]): void
}>()

const isOpen = ref(false)
const searchQuery = ref('')
const dropdownRef = ref<HTMLElement | null>(null)
const searchInputRef = ref<HTMLInputElement | null>(null)
const listRef = ref<HTMLElement | null>(null)

const selectedSet = computed(() => new Set(props.modelValue))
const selectedCount = computed(() => props.modelValue.length)

const triggerLabel = computed(() => {
  if (props.modelValue.length === 0) return props.label
  if (props.modelValue.length === 1) {
    const opt = props.options.find((o) => o.value === props.modelValue[0])
    return opt?.label ?? props.label
  }
  return `${props.label} (${props.modelValue.length})`
})

const filteredOptions = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return props.options
  return props.options.filter((o) => o.label.toLowerCase().includes(q) || o.value.toLowerCase().includes(q))
})

function isSelected(value: string): boolean {
  return selectedSet.value.has(value)
}

function toggleOption(value: string) {
  if (props.multiSelect) {
    const next = new Set(props.modelValue)
    if (next.has(value)) next.delete(value)
    else next.add(value)
    const arr = Array.from(next)
    emit('update:modelValue', arr)
    emit('change', arr)
  } else {
    emit('update:modelValue', [value])
    emit('change', [value])
    isOpen.value = false
  }
}

function selectAll() {
  const all = filteredOptions.value.map((o) => o.value)
  emit('update:modelValue', all)
  emit('change', all)
}

function clearAll() {
  emit('update:modelValue', [])
  emit('change', [])
}

function toggleOpen() {
  isOpen.value = !isOpen.value
  if (isOpen.value) {
    nextTick(() => {
      if (props.searchable && searchInputRef.value) {
        searchInputRef.value.focus()
      }
    })
  }
}

function onSearchInput() {
  // 搜索时不需要特殊处理，computed 自动更新
}

function handleClickOutside(e: MouseEvent) {
  if (!dropdownRef.value) return
  if (!dropdownRef.value.contains(e.target as Node)) {
    isOpen.value = false
  }
}

function handleEscape(e: KeyboardEvent) {
  if (e.key === 'Escape' && isOpen.value) {
    isOpen.value = false
  }
}

onMounted(() => {
  document.addEventListener('mousedown', handleClickOutside)
  document.addEventListener('keydown', handleEscape)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', handleClickOutside)
  document.removeEventListener('keydown', handleEscape)
})

watch(isOpen, (open) => {
  if (!open) searchQuery.value = ''
})
</script>

<style scoped>
.filter-dropdown {
  position: relative;
  display: inline-block;
}

.dropdown-trigger {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border: 1px solid var(--ion-color-medium-tint, #ccc);
  border-radius: 8px;
  background: var(--ion-color-light, #f5f5f5);
  color: var(--ion-color-dark, #333);
  font-size: 13px;
  cursor: pointer;
  transition: all 0.15s ease;
  user-select: none;
}

.dropdown-trigger:hover {
  background: var(--ion-color-light-shade, #e8e8e8);
}

.dropdown-trigger.is-open {
  border-color: var(--ion-color-primary, #3880ff);
  background: var(--ion-color-primary-tint, #e8f0fe);
}

.trigger-label {
  font-weight: 500;
}

.trigger-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: 9px;
  background: var(--ion-color-primary, #3880ff);
  color: #fff;
  font-size: 11px;
  font-weight: 600;
}

.trigger-arrow {
  font-size: 10px;
  opacity: 0.7;
  transition: transform 0.15s ease;
}

.dropdown-trigger.is-open .trigger-arrow {
  transform: rotate(180deg);
}

.dropdown-panel {
  position: absolute;
  top: calc(100% + 4px);
  left: 0;
  min-width: 220px;
  max-width: 320px;
  max-height: 360px;
  background: var(--ion-color-light, #fff);
  border: 1px solid var(--ion-color-medium-tint, #ddd);
  border-radius: 10px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.15);
  z-index: 1000;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.dropdown-search {
  padding: 8px;
  border-bottom: 1px solid var(--ion-color-light-shade, #eee);
}

.search-input {
  width: 100%;
  padding: 6px 10px;
  border: 1px solid var(--ion-color-medium-tint, #ddd);
  border-radius: 6px;
  font-size: 13px;
  background: var(--ion-color-light, #fafafa);
  color: var(--ion-color-dark, #333);
  outline: none;
  box-sizing: border-box;
}

.search-input:focus {
  border-color: var(--ion-color-primary, #3880ff);
}

.dropdown-list {
  flex: 1;
  overflow-y: auto;
  max-height: 280px;
}

.dropdown-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  cursor: pointer;
  font-size: 13px;
  color: var(--ion-color-dark, #333);
  transition: background 0.1s ease;
}

.dropdown-item:hover {
  background: var(--ion-color-light-shade, #f0f0f0);
}

.dropdown-item.is-selected {
  background: var(--ion-color-primary-tint, #e8f0fe);
  color: var(--ion-color-primary, #3880ff);
  font-weight: 500;
}

.item-checkbox {
  width: 16px;
  height: 16px;
  border: 1.5px solid var(--ion-color-medium, #999);
  border-radius: 3px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.dropdown-item.is-selected .item-checkbox {
  background: var(--ion-color-primary, #3880ff);
  border-color: var(--ion-color-primary, #3880ff);
}

.check-mark {
  color: #fff;
  font-size: 11px;
  font-weight: bold;
  line-height: 1;
}

.item-label {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.item-count {
  font-size: 11px;
  color: var(--ion-color-medium, #999);
  flex-shrink: 0;
}

.dropdown-empty {
  padding: 24px 12px;
  text-align: center;
  color: var(--ion-color-medium, #999);
  font-size: 13px;
}

.dropdown-actions {
  display: flex;
  gap: 8px;
  padding: 8px 12px;
  border-top: 1px solid var(--ion-color-light-shade, #eee);
  background: var(--ion-color-light, #fafafa);
}

.action-btn {
  flex: 1;
  padding: 6px 8px;
  border: 1px solid var(--ion-color-medium-tint, #ddd);
  border-radius: 6px;
  background: var(--ion-color-light, #fff);
  color: var(--ion-color-primary, #3880ff);
  font-size: 12px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.15s ease;
}

.action-btn:hover {
  background: var(--ion-color-primary-tint, #e8f0fe);
  border-color: var(--ion-color-primary, #3880ff);
}

.dropdown-fade-enter-active,
.dropdown-fade-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.dropdown-fade-enter-from,
.dropdown-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

@media (prefers-color-scheme: dark) {
  .dropdown-trigger {
    border-color: var(--ion-color-medium-shade, #444);
    background: var(--ion-color-dark-shade, #222);
    color: var(--ion-color-light, #eee);
  }

  .dropdown-trigger:hover {
    background: var(--ion-color-dark, #333);
  }

  .dropdown-trigger.is-open {
    border-color: var(--ion-color-primary, #3880ff);
    background: rgba(56, 128, 255, 0.15);
  }

  .dropdown-panel {
    background: var(--ion-color-dark, #1e1e1e);
    border-color: var(--ion-color-medium-shade, #444);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
  }

  .dropdown-search {
    border-bottom-color: var(--ion-color-dark-shade, #2a2a2a);
  }

  .search-input {
    border-color: var(--ion-color-medium-shade, #444);
    background: var(--ion-color-dark-shade, #2a2a2a);
    color: var(--ion-color-light, #eee);
  }

  .dropdown-item {
    color: var(--ion-color-light, #eee);
  }

  .dropdown-item:hover {
    background: var(--ion-color-dark-shade, #2a2a2a);
  }

  .dropdown-item.is-selected {
    background: rgba(56, 128, 255, 0.15);
    color: var(--ion-color-primary, #6ba3ff);
  }

  .item-checkbox {
    border-color: var(--ion-color-medium, #666);
  }

  .item-count {
    color: var(--ion-color-medium, #888);
  }

  .dropdown-empty {
    color: var(--ion-color-medium, #888);
  }

  .dropdown-actions {
    border-top-color: var(--ion-color-dark-shade, #2a2a2a);
    background: var(--ion-color-dark-shade, #1a1a1a);
  }

  .action-btn {
    border-color: var(--ion-color-medium-shade, #444);
    background: var(--ion-color-dark, #2a2a2a);
    color: var(--ion-color-primary, #6ba3ff);
  }

  .action-btn:hover {
    background: rgba(56, 128, 255, 0.15);
    border-color: var(--ion-color-primary, #3880ff);
  }
}
</style>
