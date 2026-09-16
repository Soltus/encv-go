<template>
  <!--
    v2 多轮/分支剧本的 chip 选择条
    参考 .trae/specs/agent-tools-scenarios-v2/spec.md §MockBranchChoiceBar
    视觉风格与 MockPresetBar 一致（半透明 primary tint + backdrop-filter blur），
    区别：
      1. 多了 header 行（scenario badge + round 进度）
      2. 多了 prompt 行（后端推的当前 step 提问文案）
      3. chip 内容更详细（icon + label + description 纵向三行）
      4. chips 区支持横向滚动（chip 多时不被压缩）
  -->
  <div v-if="paused" class="mock-branch-bar">
    <div class="mock-branch-header">
      <span class="mock-branch-scenario">🧪 {{ scenario || 'mock-scenario' }}</span>
      <span v-if="round !== undefined && total !== undefined" class="mock-branch-round">
        {{ t('agent.roundProgress', { round: String((round ?? 0) + 1), total: String(total ?? 1) }) }}
      </span>
    </div>
    <div v-if="prompt" class="mock-branch-prompt">{{ prompt }}</div>
    <div class="mock-branch-chips">
      <button
        v-for="b in branches"
        :key="b.id"
        type="button"
        class="mock-branch-chip"
        @click="onPick(b)"
      >
        <span v-if="b.icon" class="mock-branch-icon">{{ b.icon }}</span>
        <span class="mock-branch-label">{{ b.label }}</span>
        <span v-if="b.description" class="mock-branch-desc">{{ b.description }}</span>
      </button>
    </div>
    <div class="mock-branch-hint">{{ t('agent.roundPausedHint') }}</div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";

defineProps<{
  /** 父组件用 mockScenarioPaused 控制显隐；这里再做一次防御（v-if 双向） */
  paused: boolean;
  /** 当前激活的 scenario ID（来自 useAgent.currentMockScenario） */
  scenario?: string;
  /** 当前 round 下标（0-based），来自 mockRoundState.roundIdx */
  round?: number;
  /** 剧本总轮数，来自 mockRoundState.totalRounds */
  total?: number;
  /** 当前 step 的 prompt 文案（来自 useAgent.mockBranchPrompt） */
  prompt?: string;
  /** 分支选项列表（来自 useAgent.mockBranchChoices） */
  branches?: Array<{
    id: string;
    label: string;
    icon?: string;
    description?: string;
  }>;
  /** 当前 phase 字符串，调试用（'awaiting_branch_choice' / 'awaiting_user_input'） */
  phase?: string;
}>();

const emit = defineEmits<{
  /** 点 chip：把整个 branch 对象回传，父组件用 branch.id 调 pickMockBranch */
  (e: "pick", branch: { id: string; label: string; icon?: string; description?: string }): void;
  /** 用户在输入框键入文本时由父组件转发到 useAgent.sendMockRoundResponse */
  (e: "type", text: string): void;
}>();

const { t } = useI18n();

function onPick(branch: { id: string; label: string; icon?: string; description?: string }): void {
  emit("pick", branch);
}
</script>

<style scoped>
/* =========================================================================
 * 容器
 * 与 MockPresetBar 风格保持一致：半透明 primary tint + backdrop-filter blur
 * + rounded corners + 内边距。深色模式用 color-mix(in srgb, var(--color-primary) 12%, transparent)
 * ========================================================================= */
.mock-branch-bar {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  margin: 6px 8px 4px 8px;
  border-radius: 12px;
  background: color-mix(in srgb, var(--color-primary) 10%, transparent);
  border: 1px solid color-mix(in srgb, var(--color-primary) 25%, transparent);
  backdrop-filter: blur(var(--material-blur, 12px));
  -webkit-backdrop-filter: blur(var(--material-blur, 12px));
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
  /* 防止被 IonFooter 截断时溢出的 chip 抢占横向滚动条 */
  overflow: hidden;
}

/* =========================================================================
 * Header 行：scenario badge + round 进度
 * ========================================================================= */
.mock-branch-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
  opacity: 0.85;
}

.mock-branch-scenario {
  font-weight: 600;
  color: var(--color-primary);
  letter-spacing: 0.2px;
  /* 长 scenario 名时截断 */
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 60%;
}

.mock-branch-round {
  font-variant-numeric: tabular-nums;
  font-weight: 500;
  color: color-mix(in srgb, var(--color-primary) 85%, var(--color-black));
  /* 半透明胶囊背景增强识别度 */
  padding: 1px 8px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--color-primary) 8%, transparent);
}

/* =========================================================================
 * Prompt 行：当前 step 的提问文案
 * ========================================================================= */
.mock-branch-prompt {
  font-size: 13px;
  line-height: 1.45;
  color: var(--ion-text-color, #222);
  word-break: break-word;
  /* prompt 可能较长，最多 4 行后省略，避免撑爆 footer */
  display: -webkit-box;
  -webkit-line-clamp: 4;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* =========================================================================
 * Chips 区：横向 flex + 横向滚动（chip 数量大时不换行不压缩）
 * ========================================================================= */
.mock-branch-chips {
  display: flex;
  flex-direction: row;
  gap: 8px;
  /* chip 多时横向滚动，触摸友好 */
  overflow-x: auto;
  overflow-y: hidden;
  /* 滚动条不喧宾夺主 */
  scrollbar-width: thin;
  -webkit-overflow-scrolling: touch;
  /* 给滚动条留 2px 视觉缓冲 */
  padding: 2px 0;
  /* 强制不换行（重要：防止 chip 被压缩到无法点击） */
  flex-wrap: nowrap;
  /* 内部元素超出时不产生回绕 */
  white-space: nowrap;
}

.mock-branch-chips::-webkit-scrollbar {
  height: 4px;
}

.mock-branch-chips::-webkit-scrollbar-thumb {
  background: color-mix(in srgb, var(--color-primary) 30%, transparent);
  border-radius: 2px;
}

/* =========================================================================
 * Chip 按钮：icon + label + description 纵向三行
 * 视觉：白底 primary 边框，hover/focus 时 primary tint
 * ========================================================================= */
.mock-branch-chip {
  /* 关键：不收缩！flex 横向滚动时子项不能被压缩 */
  flex: 0 0 auto;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  min-width: 96px;
  max-width: 200px;
  padding: 8px 12px;
  border-radius: 10px;
  border: 1px solid color-mix(in srgb, var(--color-primary) 35%, transparent);
  background: rgba(255, 255, 255, 0.85);
  color: color-mix(in srgb, var(--color-primary) 85%, var(--color-black));
  font-size: 12px;
  font-weight: 500;
  line-height: 1.3;
  text-align: left;
  cursor: pointer;
  transition: background 0.15s ease, transform 0.15s ease, border-color 0.15s ease;
  /* 长 label 可在内部多行，但 chip 自身不缩 */
  white-space: normal;
}

/* 暗黑模式：白底 → 半透明深色（与 app 整体暗黑风格一致） */
@media (prefers-color-scheme: dark) {
  .mock-branch-chip {
    background: color-mix(in srgb, var(--color-primary) 18%, transparent);
    color: color-mix(in srgb, var(--color-primary) 85%, var(--color-white));
  }
}

.mock-branch-chip:hover,
.mock-branch-chip:focus-visible {
  background: color-mix(in srgb, var(--color-primary) 20%, transparent);
  border-color: var(--color-primary);
  transform: translateY(-1px);
  outline: none;
}

.mock-branch-chip:active {
  transform: translateY(0);
  background: color-mix(in srgb, var(--color-primary) 28%, transparent);
}

.mock-branch-icon {
  font-size: 18px;
  line-height: 1;
  margin-bottom: 2px;
}

.mock-branch-label {
  font-size: 13px;
  font-weight: 600;
  color: inherit;
  word-break: break-word;
}

.mock-branch-desc {
  font-size: 11px;
  font-weight: 400;
  opacity: 0.78;
  color: inherit;
  word-break: break-word;
  /* 描述最多 2 行 */
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* =========================================================================
 * Hint 行：底部小字提示"点击 chip 继续或键入文本"
 * ========================================================================= */
.mock-branch-hint {
  font-size: 11px;
  opacity: 0.65;
  text-align: center;
  margin-top: 2px;
  font-style: italic;
}
</style>
