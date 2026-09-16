<!--
  MockPresetBar - 模拟模式下覆盖在输入框上方的预设输入控件
  参照 codex_web 的 followup chip 模式 + Ionic design tokens。
  数据来源：useAgent().mockPresets（由后端 mock_presets 事件驱动）
  交互：点击 chip → emit('pick', preset) → 父组件调 useAgent().pickMockPreset()

  设计要点：
  1. 只在 mock 模式 + 有 presets 时显示（v-if 控制由 AgentChat 处理）
  2. header 显示 🧪 + scenario 名 + 阶段提示（点击直接发送）
  3. chip 列表用 scroll-x 横向滚动，宽度不足时允许滚动
  4. 暗黑模式适配：与 ion-content 背景融合，chip 用半透明 primary
  5. 流式进行中（status.value !== 'idle'）时禁用 chip，避免重复触发
-->
<template>
  <div v-if="presets.length > 0" class="mock-preset-bar" role="region" :aria-label="t('agent.mockPresetBarAria')">
    <div class="mock-preset-bar-header">
      <span class="mock-preset-bar-badge" aria-hidden="true">🧪</span>
      <span class="mock-preset-bar-title">
        <span class="mock-preset-bar-scenario">{{ scenario || t('agent.mockPresetBarDefaultScenario') }}</span>
        <span v-if="phase" class="mock-preset-bar-phase">· {{ phase }}</span>
      </span>
      <span class="mock-preset-bar-hint">{{ t('agent.mockPresetBarHint') }}</span>
    </div>
    <div class="mock-preset-bar-chips" role="list">
      <button
        v-for="preset in presets"
        :key="preset.id"
        type="button"
        class="mockPresetChip ui-chip"
        :disabled="disabled"
        :title="preset.tooltip || preset.label"
        :data-testid="`mock-preset-chip-${preset.id}`"
        :aria-label="preset.tooltip || preset.label"
        role="listitem"
        @click="onPick(preset)"
      >
        <span v-if="preset.icon" class="mock-preset-chip-icon" aria-hidden="true">{{ preset.icon }}</span>
        <span class="mock-preset-chip-label">{{ preset.label }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import type { MockPreset } from "@/composables/useAgent";
import { useI18n } from "@encv/shared-components/composables/useI18n";

const { t } = useI18n();

defineProps<{
  /** 当前阶段预设列表（mock_presets 事件覆盖更新） */
  presets: MockPreset[];
  /** 当前 scenario ID（调试可见） */
  scenario?: string;
  /** 当前阶段标识（initial / after_round_2 / ...） */
  phase?: string;
  /** 流式进行中时禁用 chip（防止重复触发） */
  disabled?: boolean;
}>();

const emit = defineEmits<(e: "pick", preset: MockPreset) => void>();

function onPick(preset: MockPreset): void {
  emit("pick", preset);
}
</script>

<style scoped>
.mock-preset-bar {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 12px 6px 12px;
  background: linear-gradient(
    180deg,
    rgba(79, 140, 255, 0.06) 0%,
    rgba(79, 140, 255, 0.02) 100%
  );
  border-top: 1px solid rgba(79, 140, 255, 0.18);
  border-bottom: 1px solid rgba(0, 0, 0, 0.04);
  font-size: 12px;
  animation: mockPresetBarIn 240ms ease-out;
}

@keyframes mockPresetBarIn {
  from {
    opacity: 0;
    transform: translateY(4px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.mock-preset-bar-header {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.mock-preset-bar-badge {
  font-size: 13px;
  line-height: 1;
}

.mock-preset-bar-title {
  display: inline-flex;
  align-items: baseline;
  gap: 4px;
  min-width: 0;
  font-weight: 600;
  color: color-mix(in srgb, var(--color-primary) 75%, var(--color-base-content));
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mock-preset-bar-scenario {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.mock-preset-bar-phase {
  font-size: 10px;
  font-weight: 500;
  opacity: 0.7;
  font-family: var(--ion-font-family-monospace, ui-monospace, SFMono-Regular, monospace);
}

.mock-preset-bar-hint {
  margin-left: auto;
  font-size: 10px;
  opacity: 0.55;
  white-space: nowrap;
  flex-shrink: 0;
}

.mock-preset-bar-chips {
  display: flex;
  gap: 6px;
  overflow-x: auto;
  overflow-y: hidden;
  padding: 2px 0 4px 0;
  scrollbar-width: thin;
  -webkit-overflow-scrolling: touch;
}

.mock-preset-bar-chips::-webkit-scrollbar {
  height: 4px;
}

.mock-preset-bar-chips::-webkit-scrollbar-thumb {
  background: rgba(0, 0, 0, 0.18);
  border-radius: 2px;
}

.mock-preset-chip-icon {
  font-size: 13px;
  line-height: 1;
}

.mock-preset-chip-label {
  display: inline-block;
}

/* chip 外观（含主色 tint）已上提到全局语义层 surface.css 的 .ui-chip：
   无 scoped [data-v-x]，故用户主题 .ui-chip { ... } 运行时注入更晚 → 直接胜出，
   无需 !important（SiYuan 式自由度）。本组件 scoped 仅保留结构/交互差异。 */
.mockPresetChip {
  /* 结构性钩子保留（如需组件级微调可在此，但视觉统一由 .ui-chip 控制） */
}
/* 暗黑模式适配 */
body.dark .mock-preset-bar {
  background: linear-gradient(
    180deg,
    rgba(79, 140, 255, 0.10) 0%,
    rgba(79, 140, 255, 0.04) 100%
  );
  border-top-color: rgba(79, 140, 255, 0.30);
  border-bottom-color: rgba(255, 255, 255, 0.05);
}

body.dark .mock-preset-bar-title {
  color: color-mix(in srgb, var(--color-primary) 85%, var(--color-white));
}

body.dark .mock-preset-bar-chips::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.18);
}
</style>
