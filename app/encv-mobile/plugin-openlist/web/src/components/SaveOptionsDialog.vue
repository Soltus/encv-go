<template>
  <ion-header>
    <ion-toolbar>
      <ion-title>保存选项</ion-title>
    </ion-toolbar>
  </ion-header>
  <ion-content class="ion-padding">
    <p>请选择保存方式：</p>
    <ion-button expand="block" @click="dismiss('saveOnly')">仅保存</ion-button>
    <ion-button expand="block" color="primary" @click="dismiss('saveAndRestart')"
      >保存并重启服务</ion-button
    >
    <ion-button expand="block" color="medium" @click="dismiss('cancel')"
      >取消</ion-button
    >
  </ion-content>
</template>

<script setup lang="ts">
import { IonButton, IonContent, IonHeader, IonTitle, IonToolbar, modalController } from '@ionic/vue'

function dismiss(result: 'saveOnly' | 'saveAndRestart' | 'cancel') {
  modalController.dismiss(result)
}
</script>
