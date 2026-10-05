<!--
  AgentMockBadge —— Mock 模式切换徽章（从 AgentChat.vue 拆出）

  为什么拆出来：
    AgentChat.vue 超过 2000 行触发 file-size-limit 构建门禁 ⇒ 必须拆分。
    徽章是**自包含**的一块（自己的模板 + 自己的样式 + 自己的图标），
    拆分后父组件只留一行声明，是最干净的一刀。

  门禁语义（vNext Round 2，不要回退）：
    - profile=production（默认）⇒ mockAllowed=false ⇒ **整个徽章不渲染**
    - 只有 ENCV_RUNTIME_PROFILE=test 才显示
    ⇒ 演示入口不得冒充真实能力。

  ⚠️ 样式随组件一起搬过来了（原来是父组件 scoped 样式的一部分），
     不要再用父组件的 .mockBadge 类名去改这里。
-->
<template>
  <button
    v-if="mockAllowed"
    type="button"
    class="mockBadge"
    :class="{
      mockBadge_active: active,
      mockBadge_clickable: true,
    }"
    :title="title"
    data-testid="agent-mock-badge"
    @click="emit('toggle')"
  >
    <ion-icon :icon="flaskIcon" class="mockBadgeIcon" />
    <span class="mockBadgeText">{{ text }}</span>
    <ion-icon :icon="chevronDownIcon" class="mockBadgeChevron" />
  </button>
</template>

<script setup lang="ts">
import { IonIcon } from "@ionic/vue";
import { chevronDown, flask } from "ionicons/icons";

const flaskIcon = flask;
const chevronDownIcon = chevronDown;

defineProps<{
  /** 运行 profile 是否允许 mock（production ⇒ false ⇒ 不渲染） */
  mockAllowed: boolean;
  /** 当前 mock 模式是否非 off（强调色） */
  active: boolean;
  /** 徽章文案 */
  text: string;
  /** 徽章 tooltip */
  title: string;
}>();

const emit = defineEmits<{ toggle: [] }>();
</script>

<style scoped>
/* ── Mock 模式切换器（仅在 test profile 下可见） ── */
.mockBadge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  border: 0;
  border-radius: 12px;
  background: color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 12%, transparent);
  color: color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100));
  font-size: 11px;
  font-weight: 500;
  line-height: 1.4;
  user-select: none;
  cursor: pointer;
  font-family: inherit;
  transition: background 0.15s ease, color 0.15s ease, transform 0.1s ease;
}

.mockBadge:hover {
  background: color-mix(in srgb, color-mix(in srgb, var(--color-base-content) 50%, var(--color-base-100)) 22%, transparent);
}

.mockBadge:active {
  transform: scale(0.96);
}

.mockBadge:focus-visible {
  outline: 2px solid var(--color-primary);
  outline-offset: 1px;
}

/* 启用 mock（builtin / custom）时的强调色 */
.mockBadge_active {
  background: color-mix(in srgb, var(--color-primary) 16%, transparent);
  color: var(--color-primary);
}

.mockBadge_active:hover {
  background: color-mix(in srgb, var(--color-primary) 24%, transparent);
}

.mockBadgeIcon {
  font-size: 12px;
  color: inherit;
}

.mockBadgeText {
  letter-spacing: 0.02em;
}

.mockBadgeChevron {
  font-size: 10px;
  color: inherit;
  opacity: 0.7;
}
</style>
