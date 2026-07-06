<template>
  <ion-page>
    <ion-header>
      <ion-toolbar>
        <ion-title>{{ t('tasks.taskDetail') }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="modalController.dismiss()" fill="clear" size="small" color="medium">
            {{ t('tasks.close') }}
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content class="ion-padding">
      <TaskBasicInfo :task="task" />
      <TaskTimeline :task="task" />

      <div class="detail-section" v-if="task.status === 'running' || task.status === 'cancelling'">
        <div class="section-title">{{ t('tasks.progress') }}</div>
        <ion-progress-bar :value="task.progress / 100"></ion-progress-bar>
        <div class="progress-stats">
          <span>{{ task.progress }}%</span>
          <span v-if="task.speed">{{ task.speed }}</span>
          <span v-if="task.eta">ETA: {{ task.eta }}</span>
        </div>
      </div>

      <TaskPerformanceSection :task="task" />
      <TaskOutputInfo :task="task" @open="openOutput" @locate="locateOutput" />
      <TaskErrorSection :task="task" />
      <TaskWarningSection :task="task" />
      <TaskActionButtons :task="task" @cancel="dismiss('cancel')" @retry="dismiss('retry')" @remove="dismiss('remove')" />

      <ion-button
        v-if="canRollback"
        expand="block"
        color="warning"
        fill="outline"
        @click="showRollbackConfirm = true"
      >
        <ion-icon :icon="arrowUndoOutline" slot="start"></ion-icon>
        {{ t('tasks.rollbackButton') }}
      </ion-button>
    </ion-content>

    <ion-alert
      :is-open="showRollbackConfirm"
      :header="t('tasks.rollbackConfirm')"
      :message="t('tasks.rollbackConfirmMessage', { taskId: props.task?.id?.slice(0, 8) })"
      :buttons="[
        { text: t('common.cancel'), role: 'cancel' },
        { text: t('tasks.rollbackConfirm'), handler: doRollback }
      ]"
      @did-dismiss="showRollbackConfirm = false"
    ></ion-alert>
  </ion-page>
</template>

<script setup lang="ts">
import {
  arrowUndoOutline,
} from "ionicons/icons";

import { type EncvTask, rollbackTask } from "@/api/encv";
import { useI18n } from "@/composables/useI18n";
import { showToast } from "@/composables/useToast";
import { modalController } from "@ionic/vue";
import { computed, ref } from "vue";
import { useRouter } from "vue-router";

const props = defineProps<{ task: EncvTask }>();
const emit = defineEmits<(e: "rollback", taskId: string) => void>();
const { t } = useI18n();
const router = useRouter();

const showRollbackConfirm = ref(false);

const canRollback = computed(() => {
  const task = props.task;
  if (!task) return false;
  // 必须是 completed 状态
  if (task.status !== "completed") return false;
  // 必须是可回滚类型（非 rollback_*）
  const type = task.type;
  if (type.startsWith("rollback_")) return false;
  // 必须是 encrypt/decrypt/move/copy/rename/delete 之一
  const rollbackableTypes = ["encrypt", "decrypt", "move", "copy", "rename", "delete"];
  if (!rollbackableTypes.includes(type)) return false;
  // 不能是已被回滚过的任务（rollbackOf 为空）
  if (task.rollbackOf) return false;
  return true;
});

async function doRollback() {
  if (!props.task) return;
  try {
    const result = await rollbackTask(props.task.id);
    showToast({ message: t("tasks.rollbackSuccess"), duration: 2000, color: "success" });
    showRollbackConfirm.value = false;
    // 关闭 modal 或刷新任务详情
    emit("rollback", result.taskId);
  } catch (err: any) {
    showToast({ message: err.message || t("tasks.rollbackFailed"), duration: 3000, color: "danger" });
    showRollbackConfirm.value = false;
  }
}

function dismiss(action: "cancel" | "retry" | "remove") {
  return modalController.dismiss({ action, id: props.task.id });
}

function openOutput(outputPath: string) {
  const name = outputPath.split("/").pop() || outputPath;
  router.push({ path: "/player", query: { path: outputPath, name } });
  modalController.dismiss({ action: "opened", id: props.task.id, outputPath });
}

// 🆕 v3 2026-06-18 Task 8：locateOutput 不再做路径转换
//   - 后端 task.outputPath 已统一为虚拟路径 /d/<mount>/<sub>（task_manager.absToVirtualPath）
//   - 前端直接拆 dir + name 塞 route.query，Files.vue onIonViewWillEnter 消费
//   - 旧版逻辑（物理绝对路径 → 前端无法解析）已废弃
function locateOutput(outputPath: string) {
  const trimmed = outputPath.replace(/\/+$/, "");
  const lastSlash = trimmed.lastIndexOf("/");
  const name = lastSlash >= 0 ? trimmed.substring(lastSlash + 1) : trimmed;
  const dir = lastSlash >= 0 ? trimmed.substring(0, lastSlash) : "/d";
  router.push({ path: "/tabs/files", query: { path: dir, highlight: name } });
  modalController.dismiss({ action: "located", id: props.task.id, outputPath });
}
</script>

<style scoped>
.detail-section { margin-bottom: 20px; }
.section-title {
  font-size: 14px; font-weight: 700; color: var(--ion-text-color);
  margin-bottom: 10px; display: flex; align-items: center; gap: 6px;
}
.progress-stats {
  display: flex; gap: 12px; margin-top: 6px;
  font-size: 12px; color: var(--ion-color-medium); font-weight: 500;
}
</style>
