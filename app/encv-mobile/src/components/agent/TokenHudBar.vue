<script setup lang="ts">
/**
 * TokenHudBar — 7 行 token 实时用量 HUD
 *
 * 借鉴自 nuclear-boy ui-chat/TokenHudBar.kt L166-198。
 *
 * 显示项（7 行）：
 *   1. 输入 token  (PromptTokensThisRequest)
 *   2. 输出 token  (CompletionTokensThisRequest)
 *   3. 缓存命中    (CacheHitRate%)
 *   4. 思考 token  (ReasoningTokensTotal)
 *   5. 上下文占用  (ContextUsagePercent)
 *   6. 输出速度    (TokensPerSecond)
 *   7. 平均延迟    (AverageLatencyMs)
 *
 * 颜色规则（nuclear-boy L166-198）：
 *   - < 80%  绿色
 *   - 80-95% 黄色
 *   - > 95%  红色
 */
import { computed } from "vue";
import type { TokenSnapshot } from "../../types/tokenSnapshot";

const props = withDefaults(
  defineProps<{
    snapshot: TokenSnapshot;
    /** DeepSeek context window size (default 1M) */
    contextWindow?: number;
  }>(),
  { contextWindow: 1_000_000 }
);

const usagePercent = computed(() => props.snapshot.contextUsagePercent);
const warningLevel = computed<"ok" | "green" | "yellow" | "red" | "force">(() => {
  const pct = usagePercent.value;
  if (pct >= 0.98) return "force";
  if (pct >= 0.95) return "red";
  if (pct >= 0.8) return "yellow";
  if (pct >= 0.3) return "green";
  return "ok";
});

const levelColor = computed(() => {
  switch (warningLevel.value) {
    case "ok":
      return "#4caf50";
    case "green":
      return "#4caf50";
    case "yellow":
      return "#ffc107";
    case "red":
      return "#ff5252";
    case "force":
      return "#b71c1c";
  }
});

const formatTokens = (n: number): string => {
  if (n < 1000) return `${n}`;
  if (n < 1_000_000) return `${(n / 1000).toFixed(1)}K`;
  return `${(n / 1_000_000).toFixed(2)}M`;
};

const formatPercent = (n: number): string => `${(n * 100).toFixed(1)}%`;
const formatSpeed = (n: number): string => `${n.toFixed(1)} t/s`;
const formatLatency = (ms: number): string => (ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(2)}s`);
</script>

<template>
  <div class="token-hud-bar" :style="{ borderLeftColor: levelColor }">
    <div class="hud-header">
      <span class="hud-title">🪙 Token HUD</span>
      <span class="hud-warning" :style="{ color: levelColor }">
        {{ warningLevel.toUpperCase() }}
      </span>
    </div>
    <div class="hud-row">
      <span class="hud-label">输入</span>
      <span class="hud-value">{{ formatTokens(snapshot.promptTokensThisRequest) }}</span>
    </div>
    <div class="hud-row">
      <span class="hud-label">输出</span>
      <span class="hud-value">{{ formatTokens(snapshot.completionTokensThisRequest) }}</span>
    </div>
    <div class="hud-row">
      <span class="hud-label">缓存命中</span>
      <span class="hud-value">{{ formatPercent(snapshot.cacheHitRate) }}</span>
    </div>
    <div class="hud-row">
      <span class="hud-label">思考</span>
      <span class="hud-value">{{ formatTokens(snapshot.reasoningTokensTotal) }}</span>
    </div>
    <div class="hud-row">
      <span class="hud-label">上下文</span>
      <span class="hud-value" :style="{ color: levelColor }">
        {{ formatPercent(usagePercent) }} / {{ formatTokens(contextWindow) }}
      </span>
    </div>
    <div class="hud-row">
      <span class="hud-label">速度</span>
      <span class="hud-value">{{ formatSpeed(snapshot.tokensPerSecond) }}</span>
    </div>
    <div class="hud-row">
      <span class="hud-label">延迟</span>
      <span class="hud-value">{{ formatLatency(snapshot.averageLatencyMs) }}</span>
    </div>
    <div v-if="snapshot.requestCount > 0" class="hud-meta">
      请求 #{{ snapshot.requestCount }} · 累计 {{ formatTokens(snapshot.promptTokensTotal + snapshot.completionTokensTotal) }}
    </div>
  </div>
</template>

<style scoped>
.token-hud-bar {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 10px;
  background: var(--color-base-100);
  border-left: 3px solid #4caf50;
  border-radius: 4px;
  font-family: var(--ion-font-family-monospace, monospace);
  font-size: 11px;
  line-height: 1.3;
}
.hud-header {
  display: flex;
  justify-content: space-between;
  font-weight: 600;
  margin-bottom: 2px;
}
.hud-warning {
  letter-spacing: 0.5px;
  font-size: 10px;
}
.hud-row {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}
.hud-label {
  color: color-mix(in srgb, var(--color-base-content) 57%, var(--color-base-100));
}
.hud-value {
  color: color-mix(in srgb, var(--color-base-content) 92%, transparent);
  font-weight: 500;
}
.hud-meta {
  margin-top: 2px;
  font-size: 9px;
  color: color-mix(in srgb, var(--color-base-content) 43%, var(--color-base-100));
  text-align: right;
}
</style>
