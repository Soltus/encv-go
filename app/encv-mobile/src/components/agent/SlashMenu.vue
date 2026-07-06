<!--
  SlashMenu - "/" 触发命令面板
  接收父级 useSlashMenu 提供的 items + selectedIndex，渲染分组列表。
  设计为受控组件（selectedIndex 由父级 useSlashMenu 持有并响应 keydown）。
  移动端：通过 Teleport 挂到 body，避免 textarea 滚动时被裁剪或遮挡。
-->
<template>
  <Teleport to="body">
    <div
      v-if="items.length > 0 || hasQuery"
      class="slash-menu"
      role="listbox"
      :aria-label="t('agent.slashMenuTitle')"
      @mouseleave="onMouseLeaveList"
    >
      <div class="slash-menu-header">
        <span class="slash-menu-title">{{ t('agent.slashMenuTitle') }}</span>
        <span v-if="query" class="slash-menu-query">/{{ query }}</span>
      </div>

      <!-- 功能分组 -->
      <div v-if="groupedItems.features.length > 0" class="slash-menu-group">
        <div class="slash-menu-group-label">{{ t('agent.slashMenuFeatures') }}</div>
        <div
          v-for="item in groupedItems.features"
          :key="item.id"
          class="slash-menu-item"
          :class="{ 'slash-menu-item-active': flatIndexOf(item.id) === selectedIndex }"
          role="option"
          :aria-selected="flatIndexOf(item.id) === selectedIndex"
          :data-testid="`slash-menu-item-${item.id}`"
          @click="onClickItem(item.id)"
          @mouseenter="onMouseEnterItem(item.id)"
        >
          <span class="slash-menu-item-icon" aria-hidden="true">
            <ion-icon :icon="item.icon" />
          </span>
          <span class="slash-menu-item-text">
            <span class="slash-menu-item-label">{{ item.label }}</span>
            <span v-if="item.description" class="slash-menu-item-desc">{{ item.description }}</span>
          </span>
        </div>
      </div>

      <!-- 技能分组 -->
      <div v-if="groupedItems.skills.length > 0" class="slash-menu-group">
        <div class="slash-menu-group-label">{{ t('agent.slashMenuSkills') }}</div>
        <div
          v-for="item in groupedItems.skills"
          :key="item.id"
          class="slash-menu-item"
          :class="{ 'slash-menu-item-active': flatIndexOf(item.id) === selectedIndex }"
          role="option"
          :aria-selected="flatIndexOf(item.id) === selectedIndex"
          :data-testid="`slash-menu-item-${item.id}`"
          @click="onClickItem(item.id)"
          @mouseenter="onMouseEnterItem(item.id)"
        >
          <span class="slash-menu-item-icon slash-menu-item-icon-skill" aria-hidden="true">
            <ion-icon :icon="item.icon" />
          </span>
          <span class="slash-menu-item-text">
            <span class="slash-menu-item-label">{{ item.label }}</span>
            <span v-if="item.description" class="slash-menu-item-desc">{{ item.description }}</span>
          </span>
        </div>
      </div>

      <!-- 无匹配提示 -->
      <div v-if="items.length === 0" class="slash-menu-empty">
        {{ t('agent.slashMenuNoMatches') }}
      </div>

      <div class="slash-menu-hint">{{ t('agent.slashMenuHint') }}</div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { useI18n } from "@/composables/useI18n";
import type { SlashMenuItem } from "@/composables/useSlashMenu";
import { computed } from "vue";

/**
 * 必填 props：items / query / onApply / onClose（与 spec 一致）
 * 扩展 props：selectedIndex / onSelectedIndexChange
 *   父级 useSlashMenu 持有 selectedIndex，通过这两个 prop 把菜单变成
 *   "受控组件"——键盘事件在 textarea 上发生，状态在 composable 中更新。
 */
const props = defineProps<{
  items: SlashMenuItem[];
  query: string;
  onApply: (id: string) => void;
  onClose: () => void;
  selectedIndex?: number;
  onSelectedIndexChange?: (n: number) => void;
}>();

const { t } = useI18n();

/** 按 group 拆分后渲染（功能在前，技能在后） */
const _groupedItems = computed(() => {
  const features: SlashMenuItem[] = [];
  const skills: SlashMenuItem[] = [];
  for (const it of props.items) {
    if (it.group === "功能") features.push(it);
    else skills.push(it);
  }
  return { features, skills };
});

/** 是否当前有 query——空 query 也要显示完整列表 */
const _hasQuery = computed(() => props.query.length > 0);

/**
 * 通过 id 找当前项在 props.items（扁平数组）中的索引。
 * 用于判断某条 item 是否是当前 selectedIndex。
 */
function flatIndexOf(id: string): number {
  return props.items.findIndex(x => x.id === id);
}

function _onClickItem(id: string) {
  props.onApply(id);
}

function _onMouseEnterItem(id: string) {
  // 鼠标悬停时同步更新 selectedIndex，让"键盘高亮 == 鼠标悬停高亮"
  const idx = flatIndexOf(id);
  if (idx >= 0 && idx !== (props.selectedIndex ?? 0)) {
    props.onSelectedIndexChange?.(idx);
  }
}

function _onMouseLeaveList() {
  // 鼠标离开整个列表时不主动改 selectedIndex——下一次键盘导航从原位继续
  // 如果想要"鼠标离开后回到原 selectedIndex"，可在这里发 onSelectedIndexChange
  // 当前不发送以避免键盘/鼠标竞态
}
</script>

<style scoped>
.slash-menu {
  position: fixed;
  /* 默认靠底——具体 left/bottom 由父级在包裹层覆盖（这里是 fixed 居中策略） */
  left: 50%;
  bottom: 88px;
  transform: translateX(-50%);
  width: min(420px, calc(100vw - 24px));
  max-height: 60vh;
  overflow-y: auto;
  background: var(--ion-background-color, #fff);
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.25);
  border-radius: 12px;
  box-shadow: 0 -4px 20px rgba(0, 0, 0, 0.12);
  z-index: 9999;
  display: flex;
  flex-direction: column;
  font-size: 13px;
  color: var(--ion-text-color, #222);
  -webkit-overflow-scrolling: touch;
}

.slash-menu-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px 6px;
  border-bottom: 1px solid rgba(var(--ion-color-medium-rgb), 0.12);
  flex-shrink: 0;
}

.slash-menu-title {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.6px;
  color: var(--ion-text-color-step-400, #888);
}

.slash-menu-query {
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 12px;
  color: var(--ion-color-primary, #4f8cff);
  font-weight: 600;
}

.slash-menu-group {
  display: flex;
  flex-direction: column;
  padding: 4px 0;
}

.slash-menu-group + .slash-menu-group {
  border-top: 1px solid rgba(var(--ion-color-medium-rgb), 0.08);
}

.slash-menu-group-label {
  padding: 8px 14px 4px;
  font-size: 10px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--ion-text-color-step-400, #888);
}

.slash-menu-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 9px 14px;
  cursor: pointer;
  transition: background 0.12s;
  border: 0;
  background: transparent;
  text-align: left;
  width: 100%;
  color: inherit;
}

.slash-menu-item:hover,
.slash-menu-item-active {
  background: rgba(var(--ion-color-primary-rgb), 0.1);
}

.slash-menu-item-active {
  background: rgba(var(--ion-color-primary-rgb), 0.14);
}

.slash-menu-item-icon {
  flex-shrink: 0;
  width: 22px;
  height: 22px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  background: rgba(var(--ion-color-primary-rgb), 0.1);
  color: var(--ion-color-primary, #4f8cff);
  font-size: 14px;
  margin-top: 1px;
}

.slash-menu-item-icon-skill {
  background: rgba(var(--ion-color-warning-rgb, 255, 206, 0), 0.16);
  color: var(--ion-color-warning-shade, #b45309);
}

.slash-menu-item-text {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.slash-menu-item-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--ion-text-color, #222);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.slash-menu-item-desc {
  font-size: 11px;
  color: var(--ion-text-color-step-400, #888);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.slash-menu-empty {
  padding: 24px 14px;
  text-align: center;
  font-size: 12px;
  color: var(--ion-text-color-step-350, #999);
}

.slash-menu-hint {
  padding: 6px 14px 10px;
  font-size: 10px;
  color: var(--ion-text-color-step-350, #999);
  text-align: center;
  border-top: 1px solid rgba(var(--ion-color-medium-rgb), 0.08);
  flex-shrink: 0;
}
</style>
