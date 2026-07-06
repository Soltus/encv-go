<template>
  <ion-item
    :class="['radio-item', { 'radio-item--disabled': disabled, 'radio-item--selected': isSelected }]"
    :disabled="disabled"
    button
    :detail="false"
    lines="none"
    role="radio"
    :aria-checked="isSelected"
    @click="handleClick"
    @keydown.enter.prevent="handleClick"
    @keydown.space.prevent="handleClick"
  >
    <ion-radio
      :value="value"
      :disabled="disabled"
      slot="start"
      :aria-hidden="true"
      tabindex="-1"
      :checked="isSelected"
      @ionFocus="onRadioFocus"
    />
    <ion-label class="ion-text-wrap">
      <slot :selected="isSelected" />
    </ion-label>
  </ion-item>
</template>

<script setup lang="ts">
import { computed } from "vue";

/**
 * RadioItem —— 优雅的单选项组件（统一所有"整行点击即可切换"的 radio item）
 *
 * 背景：
 *   - 在 Ionic 8 里，<ion-radio-group> 的 @ionChange 事件只在 <ion-radio> 圆点本身被点击时触发。
 *   - 点击 <ion-label> / 空白区域 / badge 都不会冒泡到 radio，用户体验差（"只能点小圆点"）。
 *   - 本组件：把整行 ion-item 变成 button + 主动 emit 'select'，让上层 group 接收。
 *   - 同时禁用 ion-radio 的 tabindex（避免双重 tab 焦点），整行 keyboard 可达。
 *
 * 用法（在父组件里）：
 *   <ion-radio-group :value="selected" @ionChange="onChange">
 *     <RadioItem :value="0" @select="onSelect" :selected="selected">
 *       <span>Option A</span>
 *     </RadioItem>
 *     <RadioItem :value="1" @select="onSelect" :selected="selected">
 *       <span>Option B</span>
 *     </RadioItem>
 *   </ion-radio-group>
 *
 * 或者更简单的方式（推荐）：
 *   <RadioGroup :model-value="selected" @update:model-value="onUpdate" :options="[...]" />
 *
 * 两种方式都支持，自动通过 v-model 风格传递。
 */

interface Props {
  value: string | number;
  selected?: string | number;
  disabled?: boolean;
}

const props = withDefaults(defineProps<Props>(), {
  selected: undefined,
  disabled: false,
});

const emit = defineEmits<(e: "select", value: string | number) => void>();

const isSelected = computed(() => props.value === props.selected);

/**
 * 整行点击：阻止默认后 emit 'select'，由父 group 接管更新。
 *   - 通过 emit 而不是直接修改 prop，是 Vue 单向数据流的最佳实践。
 *   - 不调用 radio.click() 避免双重事件触发（radio 也会 emit ionChange）。
 */
function _handleClick(event: MouseEvent) {
  if (props.disabled) {
    event.preventDefault();
    event.stopPropagation();
    return;
  }
  // 避免点击 radio 圆点本身时重复触发（虽然逻辑上不会有问题，但可以减少一次 emit）
  const target = event.target as HTMLElement;
  if (target.closest("ion-radio") && !target.closest("ion-radio")?.matches(":host(:not([disabled]))")) {
    // radio 内部点击仍然走 ionChange 路径，handler 在父级统一处理
    return;
  }
  if (!isSelected.value) {
    emit("select", props.value);
  }
}

/**
 * 内部 radio focus 时不抢焦点（item 已经是 button，整行 keyboard 可达）
 * 这是 Ionic 8 推荐的模式：button > radio 的可访问性树。
 */
function _onRadioFocus() {
  // 故意不 focus，让 button 保留焦点
}
</script>

<style scoped>
.radio-item {
  --padding-start: 8px;
  --inner-padding-end: 12px;
  --min-height: 56px;
  cursor: pointer;
  transition: background-color 0.15s ease;
}

.radio-item.radio-item--disabled {
  cursor: not-allowed;
  opacity: 0.55;
  pointer-events: auto;
}

.radio-item.radio-item--selected {
  --background: rgba(var(--ion-color-primary-rgb), 0.06);
}

.radio-item:hover:not(.radio-item--disabled) {
  --background: rgba(var(--ion-color-primary-rgb), 0.04);
}

.radio-item:active:not(.radio-item--disabled) {
  --background: rgba(var(--ion-color-primary-rgb), 0.10);
}
</style>
