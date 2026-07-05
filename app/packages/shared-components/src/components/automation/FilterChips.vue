<template>
  <div class="filter-bar">
    <div class="filter-bar__row">
      <span class="filter-bar__label">FILTER</span>
      <div class="filter-bar__chips">
        <!-- 状态 chip -->
        <button
          v-for="s in statusOptions"
          :key="s.value"
          class="chip chip--status"
          :class="{
            'chip--active': activeStatuses.has(s.value),
            [`chip--status-${s.value}`]: true,
          }"
          @click="toggleStatus(s.value)"
        >
          <span class="chip__count">{{ s.count }}</span>
          <span class="chip__label">{{ s.label }}</span>
        </button>

        <span class="filter-bar__sep"></span>

        <!-- 错误类别 chip（仅 failed 有意义） -->
        <button
          v-for="c in categoryOptions"
          :key="c.value"
          class="chip chip--cat"
          :class="{
            'chip--active': activeCategories.has(c.value),
          }"
          :style="activeCategories.has(c.value) ? { borderColor: c.color, color: c.color } : {}"
          @click="toggleCategory(c.value)"
        >
          <span class="chip__count">{{ c.count }}</span>
          <span class="chip__label">{{ c.label }}</span>
        </button>

        <button
          v-if="hasAnyFilter"
          class="chip chip--clear"
          @click="clearAll"
        >
          ✕ CLEAR
        </button>
      </div>
    </div>
    <div v-if="hasAnyFilter" class="filter-bar__summary">
      Showing <strong>{{ filteredCount }}</strong> of {{ totalCount }} cases
    </div>
  </div>
</template>

<script setup lang="ts">
import { CATEGORY_META, type ErrorCategory } from "@encv/shared-components/composables/useErrorAnalyzer";
import type { TestCaseResult } from "@encv/shared-components/lib/workflow/types";
import { computed } from "vue";

const props = defineProps<{
  results: TestCaseResult[];
  activeStatuses: Set<string>;
  activeCategories: Set<string>;
  filteredCount: number;
  totalCount: number;
}>();

const emit = defineEmits<{
  "update:activeStatuses": [Set<string>];
  "update:activeCategories": [Set<string>];
}>();

const _statusOptions = computed(() => {
  const counts: Record<string, number> = { passed: 0, failed: 0, skipped: 0, running: 0, pending: 0 };
  for (const r of props.results) counts[r.status] = (counts[r.status] ?? 0) + 1;
  return [
    { value: "failed", label: "FAILED", count: counts.failed },
    { value: "passed", label: "PASSED", count: counts.passed },
    { value: "skipped", label: "SKIPPED", count: counts.skipped },
    { value: "running", label: "RUNNING", count: counts.running },
    { value: "pending", label: "PENDING", count: counts.pending },
  ].filter(opt => opt.count > 0);
});

const _categoryOptions = computed(() => {
  const counts: Partial<Record<ErrorCategory, number>> = {};
  for (const r of props.results) {
    if (r.status !== "failed") continue;
    const cat = r.errorAnalysis?.category ?? "unknown";
    counts[cat] = (counts[cat] ?? 0) + 1;
  }
  const entries = Object.entries(counts) as [ErrorCategory, number][];
  return entries
    .map(([cat, count]) => ({
      value: cat,
      count,
      label: CATEGORY_META[cat].label,
      color: CATEGORY_META[cat].color,
    }))
    .sort((a, b) => b.count - a.count);
});

const _hasAnyFilter = computed(() => {
  return props.activeStatuses.size > 0 || props.activeCategories.size > 0;
});

function _toggleStatus(value: string) {
  const next = new Set(props.activeStatuses);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  emit("update:activeStatuses", next);
}

function _toggleCategory(value: string) {
  const next = new Set(props.activeCategories);
  if (next.has(value)) next.delete(value);
  else next.add(value);
  emit("update:activeCategories", next);
}

function _clearAll() {
  emit("update:activeStatuses", new Set());
  emit("update:activeCategories", new Set());
}
</script>

<style scoped>
.filter-bar {
  background: #FAF6EE;
  border: 1px solid #D4C9B5;
  border-radius: 4px;
  padding: 12px 16px;
  margin: 0 16px 12px 16px;
  font-family: 'Times New Roman', Georgia, serif;
}

.filter-bar__row {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  flex-wrap: wrap;
}

.filter-bar__label {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  letter-spacing: 0.2em;
  color: #6B5D4C;
  padding-top: 5px;
  flex-shrink: 0;
}

.filter-bar__chips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.filter-bar__sep {
  width: 1px;
  height: 16px;
  background: #C9BBA1;
  margin: 0 4px;
}

.filter-bar__summary {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 11px;
  color: #6B5D4C;
  margin-top: 8px;
  padding-top: 6px;
  border-top: 1px dashed #C9BBA1;
}

.chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 8px;
  background: #F4EFE6;
  border: 1.5px solid #C9BBA1;
  border-radius: 2px;
  font-family: 'Times New Roman', Georgia, serif;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.06em;
  color: #4A3F2E;
  cursor: pointer;
  transition: all 0.15s ease;
}
.chip:hover {
  background: #EDE5D2;
}
.chip--active {
  background: #1A1A1A;
  color: #F4EFE6;
  border-color: #1A1A1A;
}

.chip__count {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 10px;
  opacity: 0.85;
}
.chip__label {
  font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  font-size: 9px;
  letter-spacing: 0.18em;
}

/* status-specific active colors */
.chip--active.chip--status-failed { background: #8B1E3F; border-color: #8B1E3F; }
.chip--active.chip--status-passed { background: #1B4332; border-color: #1B4332; }
.chip--active.chip--status-skipped { background: #6B5D4C; border-color: #6B5D4C; }
.chip--active.chip--status-running { background: #B8860B; border-color: #B8860B; }
.chip--active.chip--status-pending { background: #8B7355; border-color: #8B7355; }

.chip--clear {
  background: transparent;
  border-style: dashed;
  color: #8B1E3F;
  border-color: #8B1E3F;
}
.chip--clear:hover {
  background: rgba(139, 30, 63, 0.08);
}
</style>
