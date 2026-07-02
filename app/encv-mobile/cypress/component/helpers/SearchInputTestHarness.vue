<template>
  <div class="search-test-harness">
    <div
      ref="inputRef"
      contenteditable="true"
      class="search-input-div"
      data-testid="search-input"
      @input="onInput"
      @keydown="onKeydown"
    ></div>

    <div class="test-buttons">
      <button type="button" data-testid="btn-and" @click="() => insertSymbol('AND')">＆</button>
      <button type="button" data-testid="btn-or" @click="() => insertSymbol('OR')">｜</button>
      <button type="button" data-testid="btn-not" @click="() => insertSymbol('NOT')">￢</button>
      <button type="button" data-testid="btn-phrase" @click="() => insertSymbol('__phrase_open__')">「」</button>
      <button type="button" data-testid="btn-regex" @click="() => insertSymbol('__regex_prefix__')">/ /</button>
      <button type="button" data-testid="btn-clear" @click="clearInput">清空</button>
    </div>

    <div class="query-display" data-testid="query-display">{{ queryValue }}</div>
    <div class="error-display" data-testid="error-display">{{ errorMsg }}</div>
    <div class="counter-display">
      <span data-testid="enter-count">enter: {{ enterCount }}</span>
      <span data-testid="escape-count">escape: {{ escapeCount }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import { useSearchInput } from '@/composables/useSearchInput'

const props = defineProps<{
  initialQuery?: string
  onChange?: (query: string) => void
  onEnter?: () => void
  onEscape?: () => void
}>()

const errorMsg = ref('')
const enterCount = ref(0)
const escapeCount = ref(0)

function handleChange(query: string) {
  try {
    props.onChange?.(query)
  } catch (e) {
    errorMsg.value = e instanceof Error ? e.message : String(e)
  }
}

function handleEnter() {
  enterCount.value++
  props.onEnter?.()
}

function handleEscape() {
  escapeCount.value++
  props.onEscape?.()
}

const {
  queryInputRef,
  queryValue,
  onQueryInput,
  onQueryKeydown,
  insertSymbol,
  clearInput,
} = useSearchInput({
  onChange: handleChange,
})

const inputRef = queryInputRef as any

function onInput(e: Event) {
  try {
    onQueryInput(e)
  } catch (err) {
    errorMsg.value = err instanceof Error ? err.message : String(err)
  }
}

function onKeydown(e: KeyboardEvent) {
  try {
    onQueryKeydown(e)
  } catch (err) {
    errorMsg.value = err instanceof Error ? err.message : String(err)
  }
}

onMounted(async () => {
  if (props.initialQuery && queryInputRef.value) {
    // 手动设置初始内容：直接写入 div 然后触发 input 事件
    const div = queryInputRef.value
    const span = document.createElement('span')
    span.dataset.kind = 'text'
    span.classList.add('syntax-text-span')
    span.textContent = props.initialQuery
    div.appendChild(span)
    await nextTick()
    // 手动触发同步
    onQueryInput(new Event('input'))
  }
})
</script>

<style scoped>
.search-test-harness {
  padding: 20px;
  background: white;
  min-height: 300px;
}

.search-input-div {
  min-height: 40px;
  padding: 8px 12px;
  border: 1px solid #ddd;
  border-radius: 8px;
  margin-bottom: 12px;
  outline: none;
}

.search-input-div:focus {
  border-color: #4f8cff;
}

.test-buttons {
  margin-bottom: 12px;
}

.test-buttons button {
  margin-right: 8px;
  padding: 6px 12px;
  border: 1px solid #ccc;
  border-radius: 4px;
  background: #f5f5f5;
  cursor: pointer;
}

.query-display {
  padding: 8px;
  background: #f0f0f0;
  border-radius: 4px;
  font-family: monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}

.error-display {
  margin-top: 8px;
  padding: 8px;
  background: #fee;
  color: #c00;
  border-radius: 4px;
  font-size: 12px;
  min-height: 20px;
}
</style>
