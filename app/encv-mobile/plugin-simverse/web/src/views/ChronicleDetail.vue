<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/settings"></ion-back-button>
        </ion-buttons>
        <ion-title>编年史</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="loadData" :disabled="loading">
            <ion-icon :icon="refreshIcon" slot="icon-only"></ion-icon>
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div v-if="loading && !loaded" class="loading-container">
        <ion-spinner name="crescent"></ion-spinner>
        <p>加载编年史...</p>
      </div>

      <template v-else-if="loaded">
        <ion-list>
          <ion-list-header>
            <ion-label>世界状态</ion-label>
          </ion-list-header>
          <ion-item>
            <ion-label>当前时代</ion-label>
            <ion-note slot="end">第 {{ worldData?.era || 0 }} 纪元</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>事件总数</ion-label>
            <ion-note slot="end">{{ worldData?.total_events || 0 }}</ion-note>
          </ion-item>
          <ion-item>
            <ion-label>世界时间</ion-label>
            <ion-note slot="end">tick {{ currentTick }}</ion-note>
          </ion-item>
        </ion-list>

        <ion-list>
          <ion-list-header>
            <ion-label>筛选</ion-label>
          </ion-list-header>
          <ion-item>
            <ion-label>最低重要度</ion-label>
            <ion-select
              v-model="minImportance"
              slot="end"
              interface="action-sheet"
              @ionChange="loadData"
            >
              <ion-select-option :value="0">全部</ion-select-option>
              <ion-select-option :value="1">次要以上</ion-select-option>
              <ion-select-option :value="2">中等以上</ion-select-option>
              <ion-select-option :value="3">重要以上</ion-select-option>
              <ion-select-option :value="4">重大以上</ion-select-option>
              <ion-select-option :value="5">史诗</ion-select-option>
            </ion-select>
          </ion-item>
          <ion-item>
            <ion-label>显示数量</ion-label>
            <ion-select
              v-model="displayLimit"
              slot="end"
              interface="action-sheet"
              @ionChange="loadData"
            >
              <ion-select-option :value="20">20 条</ion-select-option>
              <ion-select-option :value="50">50 条</ion-select-option>
              <ion-select-option :value="100">100 条</ion-select-option>
            </ion-select>
          </ion-item>
        </ion-list>

        <ion-list>
          <ion-list-header>
            <ion-label>世界编年史</ion-label>
            <ion-note slot="end">{{ worldData?.count || 0 }} 条</ion-note>
          </ion-list-header>

          <ion-item
            v-for="evt in worldData?.items || []"
            :key="evt.id"
            class="chronicle-item"
            :class="`imp-${evt.importance}`"
            button
            @click="openEventDetail(evt)"
          >
            <ion-avatar slot="start" class="level-avatar" :class="`level-${evt.level}`">
              <span class="avatar-text">{{ levelIcon(evt.level) }}</span>
            </ion-avatar>
            <ion-label class="ion-text-wrap">
              <h3>{{ evt.type_cn }}</h3>
              <p>
                <ion-badge :color="impBadgeColor(evt.importance)" class="imp-badge">
                  {{ evt.imp_cn }}
                </ion-badge>
                <span class="tick-info">tick {{ evt.tick }}</span>
              </p>
            </ion-label>
            <ion-icon :icon="chevronForward" slot="end"></ion-icon>
          </ion-item>

          <ion-item v-if="!worldData?.items?.length" class="empty-item">
            <ion-label class="ion-text-center">
              <p>暂无世界事件</p>
            </ion-label>
          </ion-item>
        </ion-list>
      </template>

      <template v-if="selectedEvent">
        <ion-modal :is-open="showEventModal" @will-dismiss="showEventModal = false">
          <ion-header>
            <ion-toolbar>
              <ion-buttons slot="start">
                <ion-button @click="showEventModal = false">关闭</ion-button>
              </ion-buttons>
              <ion-title>事件详情</ion-title>
            </ion-toolbar>
          </ion-header>
          <ion-content>
            <ion-list>
              <ion-list-header>
                <ion-label>基本信息</ion-label>
              </ion-list-header>
              <ion-item>
                <ion-label>事件类型</ion-label>
                <ion-note slot="end">{{ selectedEvent.type_cn }}</ion-note>
              </ion-item>
              <ion-item>
                <ion-label>层级</ion-label>
                <ion-note slot="end">{{ selectedEvent.level_cn }}</ion-note>
              </ion-item>
              <ion-item>
                <ion-label>重要度</ion-label>
                <ion-note slot="end">
                  <ion-badge :color="impBadgeColor(selectedEvent.importance)">
                    {{ selectedEvent.imp_cn }}
                  </ion-badge>
                </ion-note>
              </ion-item>
              <ion-item>
                <ion-label>发生时间</ion-label>
                <ion-note slot="end">tick {{ selectedEvent.tick }}</ion-note>
              </ion-item>
              <ion-item v-if="selectedEvent.entity_id">
                <ion-label>关联实体</ion-label>
                <ion-note slot="end">#{{ selectedEvent.entity_id }}</ion-note>
              </ion-item>
            </ion-list>

            <ion-list v-if="selectedEvent.causes?.length">
              <ion-list-header>
                <ion-label>起因事件（{{ selectedEvent.causes.length }}）</ion-label>
              </ion-list-header>
              <ion-item
                v-for="cause in selectedEvent.causes"
                :key="cause.id"
                button
                @click="loadAndShowEvent(cause.id)"
              >
                <ion-label class="ion-text-wrap">
                  <h3>{{ cause.type_cn }}</h3>
                  <p>tick {{ cause.tick }} · {{ cause.level_cn }}</p>
                </ion-label>
                <ion-icon :icon="chevronForward" slot="end"></ion-icon>
              </ion-item>
            </ion-list>

            <ion-list v-if="selectedEvent.effects?.length">
              <ion-list-header>
                <ion-label>后续影响（{{ selectedEvent.effects.length }}）</ion-label>
              </ion-list-header>
              <ion-item
                v-for="eff in selectedEvent.effects"
                :key="eff.id"
                button
                @click="loadAndShowEvent(eff.id)"
              >
                <ion-label class="ion-text-wrap">
                  <h3>{{ eff.type_cn }}</h3>
                  <p>tick {{ eff.tick }} · {{ eff.level_cn }}</p>
                </ion-label>
                <ion-icon :icon="chevronForward" slot="end"></ion-icon>
              </ion-item>
            </ion-list>
          </ion-content>
        </ion-modal>
      </template>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { chevronForward as chevronForwardIcon, refresh } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { type SimverseChronicleEvent, type SimverseChronicleWorldResponse, useSimverse } from "../composables/useSimverse";

const { loadChronicleWorld, loadChronicleEvent, currentTick } = useSimverse();

const loading = ref(false);
const loaded = ref(false);
const worldData = ref<SimverseChronicleWorldResponse | null>(null);
const minImportance = ref(2);
const displayLimit = ref(50);

const refreshIcon = refresh;
const chevronForward = chevronForwardIcon;

const showEventModal = ref(false);
const selectedEvent = ref<SimverseChronicleEvent | null>(null);

function levelIcon(level: string): string {
  const map: Record<string, string> = {
    Personal: "个",
    Family: "家",
    Organization: "组",
    Regional: "区",
    World: "世",
  };
  return map[level] || "?";
}

function impBadgeColor(imp: number): string {
  const colors = ["medium", "tertiary", "primary", "success", "warning", "danger"];
  return colors[imp] || "medium";
}

async function loadData() {
  loading.value = true;
  try {
    const data = await loadChronicleWorld(minImportance.value, displayLimit.value);
    worldData.value = data;
    loaded.value = true;
  } catch (e) {
    console.warn("Failed to load chronicle:", e);
  } finally {
    loading.value = false;
  }
}

async function openEventDetail(evt: SimverseChronicleEvent) {
  const detail = await loadChronicleEvent(evt.id);
  if (detail) {
    selectedEvent.value = detail;
    showEventModal.value = true;
  }
}

async function loadAndShowEvent(id: number) {
  const detail = await loadChronicleEvent(id);
  if (detail) {
    selectedEvent.value = detail;
  }
}

onMounted(() => {
  loadData();
});
</script>

<style scoped lang="scss">
.loading-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px;
  gap: 12px;
  color: var(--color-base-content);
  opacity: 0.7;
}

.level-avatar {
  width: 36px;
  height: 36px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  font-weight: 600;
  color: #fff;
  background: var(--color-base-content);
}

.level-avatar.level-Personal {
  background: var(--color-accent);
}
.level-avatar.level-Family {
  background: var(--color-success);
}
.level-avatar.level-Organization {
  background: var(--color-primary);
}
.level-avatar.level-Regional {
  background: var(--color-warning);
}
.level-avatar.level-World {
  background: var(--color-error);
}

.avatar-text {
  font-size: 12px;
}

.chronicle-item {
  --padding-start: 12px;
  --inner-padding-end: 8px;
}

.imp-badge {
  margin-right: 8px;
}

.tick-info {
  color: var(--color-base-content);
  opacity: 0.7;
  font-size: 12px;
}

.empty-item {
  --padding-start: 0;
  --inner-padding-end: 0;
  justify-content: center;
  color: var(--color-base-content);
  opacity: 0.7;
  padding: 30px 0;
}
</style>
