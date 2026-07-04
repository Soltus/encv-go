<template>
  <!--
    **重要（铁律：capacitor.md §1.1 反模式）**：
    本组件由 modalController.create({ component: PwdEditDialog }) 加载时，
    modalController 已经为它创建了全局 modal overlay（挂载在 <body> 根节点）。
    **不能再内嵌 <ion-modal :is-open>**，否则会双重 wrapper 导致白屏。
    此处直接渲染内容（header + content）即可。
  -->
  <div class="pwd-edit-dialog">
    <ion-header>
      <ion-toolbar>
        <ion-title>设置管理员密码</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="onDismiss">取消</ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>
    <ion-content class="ion-padding">
      <ion-list>
        <ion-item>
          <ion-label position="stacked">新密码</ion-label>
          <ion-input
            v-model="password"
            type="password"
            placeholder="请输入新密码"
            :clear-input="true"
          ></ion-input>
        </ion-item>
        <ion-item>
          <ion-label position="stacked">确认密码</ion-label>
          <ion-input
            v-model="confirmPassword"
            type="password"
            placeholder="再次输入新密码"
            :clear-input="true"
          ></ion-input>
        </ion-item>
      </ion-list>
      <div v-if="error" class="error-text">{{ error }}</div>
      <ion-button
        expand="block"
        :disabled="!canConfirm"
        @click="onConfirm"
        class="ion-margin-top"
      >
        确认
      </ion-button>
    </ion-content>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  IonHeader,
  IonToolbar,
  IonTitle,
  IonButtons,
  IonButton,
  IonContent,
  IonList,
  IonItem,
  IonLabel,
  IonInput,
  modalController,
} from '@ionic/vue'

const props = defineProps<{
  onConfirm: (password: string) => void | Promise<void>
}>()

const password = ref('')
const confirmPassword = ref('')
const error = ref('')

const canConfirm = computed(() => {
  return password.value.length >= 4 && password.value === confirmPassword.value
})

// modalController.create 加载的子组件无法 emit 关闭自己，
// 必须调 modalController.dismiss() 才会真正 dismiss 当前 modal overlay。
async function onConfirm() {
  if (!canConfirm.value) {
    error.value = '密码不一致或太短（至少 4 位）'
    return
  }
  error.value = ''
  try {
    await props.onConfirm(password.value)
  } catch (e: any) {
    error.value = `设置失败：${e?.message || e}`
    return
  }
  await modalController.dismiss()
}

async function onDismiss() {
  await modalController.dismiss()
}
</script>

<style scoped>
.pwd-edit-dialog {
  display: flex;
  flex-direction: column;
  height: 100%;
  background: var(--ion-background-color);
}
.error-text {
  color: var(--ion-color-danger);
  font-size: 12px;
  margin: 8px 0;
}
</style>
