<!--
  V2ScenariosMenu - v2 剧本演示入口（弹窗列表）

  位置：AgentChat 顶部（mock 徽章旁）
  作用：8 个 v2 剧本一键演示（自动切到 builtin mock + 发送 trigger keyword）
  设计：
  - 入口按钮：胶囊状 🎬 v2 剧本
  - 弹窗：ion-modal 形式，分 4 组（搜索 / 读+元数据+shell / 写 / 分支）
  - 每条 entry：场景名 + 描述 + 触发关键词 badge
  - 点击 entry → emit 'pick'，由父组件 setMockMode('builtin') + send(triggerKeyword)
-->
<template>
  <ion-button
    fill="clear"
    size="small"
    class="v2ScenariosBtn"
    :title="t('agent.v2Scenarios.btnTitle')"
    @click="openModal"
  >
    <ion-icon :icon="filmOutline" class="v2ScenariosBtnIcon" />
    <span class="v2ScenariosBtnLabel">{{ t('agent.v2Scenarios.btn') }}</span>
  </ion-button>

  <ion-modal
    :is-open="isOpen"
    @did-dismiss="closeModal"
    class="v2ScenariosModal"
  >
    <ion-header>
      <ion-toolbar>
        <ion-title>{{ t('agent.v2Scenarios.title') }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="closeModal" :aria-label="t('agent.close')">
            <ion-icon :icon="closeIcon" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content class="v2ScenariosContent">
      <p class="v2ScenariosHint">{{ t('agent.v2Scenarios.hint') }}</p>
      <div
        v-for="group in groups"
        :key="group.id"
        class="v2ScenariosGroup"
      >
        <h3 class="v2ScenariosGroupTitle">
          <ion-icon :icon="group.icon" class="v2ScenariosGroupIcon" />
          <span>{{ group.title }}</span>
          <span class="v2ScenariosGroupCount">{{ group.scenarios.length }}</span>
        </h3>
        <button
          v-for="s in group.scenarios"
          :key="s.id"
          type="button"
          class="v2ScenarioCard"
          @click="emitPick(s)"
        >
          <div class="v2ScenarioCardHead">
            <span class="v2ScenarioCardId">{{ s.id }}</span>
            <span class="v2ScenarioCardTag">{{ t('agent.v2Scenarios.mock') }}</span>
          </div>
          <div class="v2ScenarioCardDesc">{{ s.desc }}</div>
          <div class="v2ScenarioCardFoot">
            <span class="v2ScenarioCardKwLabel">{{ t('agent.v2Scenarios.triggerKw') }}</span>
            <code class="v2ScenarioCardKw">{{ s.triggerKeyword }}</code>
          </div>
        </button>
      </div>
    </ion-content>
  </ion-modal>
</template>

<script setup lang="ts">
import { close as closeIcon, documentTextOutline, filmOutline, gitBranchOutline, pricetagOutline, searchOutline } from "ionicons/icons";
import { ref } from "vue";
import { useI18n } from "@/composables/useI18n";

export interface V2ScenarioEntry {
  id: string;
  desc: string;
  triggerKeyword: string;
  groupId: "search" | "read" | "write" | "branch";
}

defineProps<{
  /** disabled 状态（streaming / confirming 时不可触发） */
  disabled?: boolean;
}>();

const emit = defineEmits<{
  pick: [scenario: V2ScenarioEntry];
}>();

const { t } = useI18n();

const isOpen = ref(false);

function openModal(): void {
  isOpen.value = true;
}

function closeModal(): void {
  isOpen.value = false;
}

function emitPick(s: V2ScenarioEntry): void {
  closeModal();
  emit("pick", s);
}

// ─── 8 个 v2 剧本分组 ──────────────────────────────────────
const groups = [
  {
    id: "search",
    title: t("agent.v2Scenarios.groupSearch"),
    icon: searchOutline,
    scenarios: [
      {
        id: "search_recursive_mp4",
        desc: t("agent.v2Scenarios.s.recursiveMp4"),
        triggerKeyword: "search_recursive_mp4",
        groupId: "search" as const,
      },
      {
        id: "search_logical_query",
        desc: t("agent.v2Scenarios.s.logicalQuery"),
        triggerKeyword: "search_logical_query",
        groupId: "search" as const,
      },
      {
        id: "search_content_regex",
        desc: t("agent.v2Scenarios.s.contentRegex"),
        triggerKeyword: "search_content_regex",
        groupId: "search" as const,
      },
    ],
  },
  {
    id: "read",
    title: t("agent.v2Scenarios.groupRead"),
    icon: documentTextOutline,
    scenarios: [
      { id: "read_file_v2", desc: t("agent.v2Scenarios.s.readFileV2"), triggerKeyword: "read_file_v2", groupId: "read" as const },
      { id: "get_metadata", desc: t("agent.v2Scenarios.s.getMetadata"), triggerKeyword: "get_metadata", groupId: "read" as const },
      {
        id: "command_run_ffprobe",
        desc: t("agent.v2Scenarios.s.commandRun"),
        triggerKeyword: "command_run_ffprobe",
        groupId: "read" as const,
      },
    ],
  },
  {
    id: "write",
    title: t("agent.v2Scenarios.groupWrite"),
    icon: pricetagOutline,
    scenarios: [
      {
        id: "edit_metadata_wizard",
        desc: t("agent.v2Scenarios.s.editMetadata"),
        triggerKeyword: "edit_metadata_wizard",
        groupId: "write" as const,
      },
      {
        id: "batch_rename_with_preview",
        desc: t("agent.v2Scenarios.s.batchRename"),
        triggerKeyword: "batch_rename_with_preview",
        groupId: "write" as const,
      },
    ],
  },
  {
    id: "branch",
    title: t("agent.v2Scenarios.groupBranch"),
    icon: gitBranchOutline,
    scenarios: [
      {
        id: "branch_encrypt_or_decrypt",
        desc: t("agent.v2Scenarios.s.branchEncrypt"),
        triggerKeyword: "branch_encrypt_or_decrypt",
        groupId: "branch" as const,
      },
      {
        id: "branch_video_or_audio",
        desc: t("agent.v2Scenarios.s.branchVideo"),
        triggerKeyword: "branch_video_or_audio",
        groupId: "branch" as const,
      },
    ],
  },
];
</script>

<style scoped>
.v2ScenariosBtn {
  --color: var(--ion-color-primary);
  font-size: 11px;
  margin: 0 4px;
  height: 26px;
  border: 1px solid rgba(var(--ion-color-primary-rgb), 0.4);
  border-radius: 13px;
  padding: 0 10px;
}
.v2ScenariosBtnIcon {
  font-size: 13px;
  margin-right: 3px;
}
.v2ScenariosBtnLabel {
  font-weight: 500;
}

.v2ScenariosContent {
  --padding-start: 14px;
  --padding-end: 14px;
  --padding-top: 8px;
  --padding-bottom: 24px;
}
.v2ScenariosHint {
  font-size: 12px;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.7));
  margin: 4px 0 14px;
  padding: 8px 10px;
  background: rgba(var(--ion-color-primary-rgb), 0.08);
  border-left: 3px solid var(--ion-color-primary);
  border-radius: 4px;
  line-height: 1.5;
}

.v2ScenariosGroup {
  margin-bottom: 16px;
}
.v2ScenariosGroupTitle {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 0 0 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.85));
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.v2ScenariosGroupIcon {
  font-size: 14px;
  color: var(--ion-color-primary);
}
.v2ScenariosGroupCount {
  margin-left: auto;
  background: var(--encv-bg-elevated, rgba(127, 127, 127, 0.12));
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.7));
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 8px;
  font-weight: 500;
  letter-spacing: 0;
  text-transform: none;
}

.v2ScenarioCard {
  display: block;
  width: 100%;
  margin-bottom: 8px;
  padding: 10px 12px;
  background: var(--encv-bg-elevated, rgba(127, 127, 127, 0.06));
  border: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.14));
  border-radius: 8px;
  text-align: left;
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s, transform 0.1s;
}
.v2ScenarioCard:hover {
  background: rgba(var(--ion-color-primary-rgb), 0.08);
  border-color: rgba(var(--ion-color-primary-rgb), 0.35);
}
.v2ScenarioCard:active {
  transform: scale(0.99);
}
.v2ScenarioCardHead {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}
.v2ScenarioCardId {
  font-size: 12.5px;
  font-weight: 600;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  color: var(--ion-text-color);
}
.v2ScenarioCardTag {
  font-size: 9px;
  padding: 1px 5px;
  background: rgba(var(--ion-color-primary-rgb), 0.18);
  color: var(--ion-color-primary);
  border-radius: 4px;
  font-weight: 600;
  letter-spacing: 0.04em;
  line-height: 1.3;
}
.v2ScenarioCardDesc {
  font-size: 12px;
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.8));
  line-height: 1.45;
  margin-bottom: 6px;
}
.v2ScenarioCardFoot {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 10.5px;
}
.v2ScenarioCardKwLabel {
  color: var(--encv-text-secondary, rgba(127, 127, 127, 0.6));
}
.v2ScenarioCardKw {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 10.5px;
  padding: 1px 5px;
  background: rgba(var(--ion-color-medium-rgb), 0.12);
  color: var(--ion-text-color);
  border-radius: 3px;
  border: 1px solid var(--encv-border-color, rgba(127, 127, 127, 0.1));
}
</style>
