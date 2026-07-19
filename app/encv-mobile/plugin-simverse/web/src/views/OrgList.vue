<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.orgs") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="p-4 space-y-3">
        <div v-if="loading" class="state-box">
          <ion-spinner name="crescent" />
          <p>{{ t("settings.loading") }}</p>
        </div>
        <div v-else-if="error" class="state-box">
          <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
          <p>{{ error }}</p>
          <button type="button" class="ui-button" @click="reload">{{ t("settings.check") }}</button>
        </div>
        <div v-else-if="!orgs.length" class="state-box">
          <ion-icon :icon="business" size="large" color="medium" />
          <p>{{ t("simverse.noOrgs") }}</p>
        </div>

        <template v-else>
          <div class="ui-header justify-between">
            <span>{{ t("simverse.total") }}: {{ total }}</span>
          </div>

          <div class="space-y-2">
            <div
              v-for="org in orgs"
              :key="org.org_id"
              class="ui-card cursor-pointer hover:scale-[0.98] active:scale-[0.97] transition-transform"
              @click="goDetail(org.org_id)"
            >
              <div class="p-3 flex items-center gap-3">
                <div class="ui-bubble !p-0 !w-10 !h-10 flex items-center justify-center flex-shrink-0 !bg-primary/15 !border-primary/30">
                  <ion-icon :icon="gitNetwork" class="text-primary" />
                </div>
                <div class="flex-1 min-w-0">
                  <h3 class="text-base font-semibold m-0 mb-1 truncate">{{ org.name }}</h3>
                  <div class="flex items-center gap-2 flex-wrap">
                    <span class="ui-chip !text-xs !py-0.5 !bg-tertiary/15 !text-tertiary !border-tertiary/30">
                      {{ org.org_type }}
                    </span>
                    <span class="text-xs text-base-content/60">
                      {{ t("simverse.memberCount") }}: {{ org.member_count }}
                    </span>
                  </div>
                </div>
                <div class="flex-shrink-0 text-xs font-mono text-base-content/70">
                  Lv.{{ org.avg_level.toFixed(1) }}
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>
    </ion-content>
  </ion-page>
</template>

<script setup lang="ts">
import { useI18n } from "@encv/shared-components/composables/useI18n";
import {
  IonBackButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonPage,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, business, gitNetwork, refreshOutline } from "ionicons/icons";
import { onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { type SimverseOrg, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const router = useRouter();
const { loadOrgList } = useSimverse();

const loading = ref(false);
const error = ref("");
const orgs = ref<SimverseOrg[]>([]);
const total = ref(0);

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const data = await loadOrgList();
    if (data) {
      orgs.value = data.items;
      total.value = data.count;
    }
  } catch (e: any) {
    error.value = e.message || "Failed to load orgs";
  } finally {
    loading.value = false;
  }
}

function goDetail(id: number) {
  router.push(`/org/${id}`);
}

onMounted(reload);
</script>

<style scoped lang="scss">
.state-box {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
}
</style>
