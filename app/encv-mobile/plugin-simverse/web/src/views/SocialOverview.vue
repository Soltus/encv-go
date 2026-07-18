<template>
  <ion-page>
    <ion-header :translucent="true">
      <ion-toolbar>
        <ion-buttons slot="start">
          <ion-back-button default-href="/tabs/world" />
        </ion-buttons>
        <ion-title>{{ t("simverse.socialOverview") }}</ion-title>
        <ion-buttons slot="end">
          <ion-button @click="reload">
            <ion-icon :icon="refreshOutline" slot="icon-only" />
          </ion-button>
        </ion-buttons>
      </ion-toolbar>
    </ion-header>

    <ion-content>
      <div class="p-4 space-y-4">
        <div v-if="loading" class="state-box">
          <ion-spinner name="crescent" />
          <p>{{ t("settings.loading") }}</p>
        </div>

        <div v-else-if="error" class="state-box">
          <ion-icon :icon="alertCircleOutline" color="danger" size="large" />
          <p>{{ error }}</p>
          <button type="button" class="ui-button" @click="reload">{{ t("settings.check") }}</button>
        </div>

        <template v-else-if="stats">
          <div class="grid grid-cols-3 gap-3">
            <div class="ui-card p-3 text-center">
              <div class="text-xl font-semibold text-primary font-mono">{{ stats.total_relations }}</div>
              <div class="text-xs text-base-content/70 mt-1">{{ t("simverse.socialTotal") }}</div>
            </div>
            <div class="ui-card p-3 text-center">
              <div class="text-xl font-semibold text-primary font-mono">{{ stats.sampled_npcs }}</div>
              <div class="text-xs text-base-content/70 mt-1">{{ t("simverse.socialSampled") }}</div>
            </div>
            <div class="ui-card p-3 text-center">
              <div class="text-xl font-semibold text-primary font-mono">{{ typeCount }}</div>
              <div class="text-xs text-base-content/70 mt-1">{{ t("simverse.socialByType") }}</div>
            </div>
          </div>

          <div class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.socialByType") }}</div>
              <div class="space-y-1">
                <div
                  v-for="(cnt, type) in stats.by_type"
                  :key="type"
                  class="flex items-center gap-3 p-3 rounded-lg hover:bg-base-200 transition-colors"
                >
                  <span class="text-sm font-medium flex-1">{{ t("simverse.rel." + type) }}</span>
                  <div class="flex items-center gap-2">
                    <div class="w-20 h-2 bg-base-300 rounded-full overflow-hidden">
                      <div
                        class="h-full rounded-full transition-all"
                        :class="barBgColor(type)"
                        :style="{ width: maxCount ? (cnt / maxCount) * 100 + '%' : '0%' }"
                      ></div>
                    </div>
                    <span class="text-xs text-base-content/70 font-mono w-8 text-right">{{ cnt }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-if="regionEntries.length" class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.socialByRegion") }}</div>
              <div class="space-y-1">
                <div
                  v-for="[rid, cnt] in regionEntries"
                  :key="'r' + rid"
                  class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                  @click="goRegion(rid)"
                >
                  <span class="text-sm font-medium">{{ t("simverse.regions") }} #{{ rid }}</span>
                  <div class="flex items-center gap-2">
                    <span class="text-xs text-base-content/70 font-mono">{{ cnt }}</span>
                    <ion-icon :icon="chevronForward" class="text-base-content/40" />
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-if="orgEntries.length" class="ui-card">
            <div class="p-3">
              <div class="ui-header mb-2">{{ t("simverse.socialByOrg") }}</div>
              <div class="space-y-1">
                <div
                  v-for="[oid, cnt] in orgEntries"
                  :key="'o' + oid"
                  class="flex items-center justify-between p-3 rounded-lg hover:bg-base-200 transition-colors cursor-pointer"
                  @click="goOrg(oid)"
                >
                  <span class="text-sm font-medium">{{ t("simverse.orgs") }} #{{ oid }}</span>
                  <div class="flex items-center gap-2">
                    <span class="text-xs text-base-content/70 font-mono">{{ cnt }}</span>
                    <ion-icon :icon="chevronForward" class="text-base-content/40" />
                  </div>
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
  IonButton,
  IonButtons,
  IonContent,
  IonHeader,
  IonIcon,
  IonPage,
  IonSpinner,
  IonTitle,
  IonToolbar,
} from "@ionic/vue";
import { alertCircleOutline, chevronForward, refreshOutline } from "ionicons/icons";
import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { type SimverseSocialStats, useSimverse } from "@/composables/useSimverse";

const { t } = useI18n();
const router = useRouter();
const { loadSocialStats } = useSimverse();

const loading = ref(false);
const error = ref("");
const stats = ref<SimverseSocialStats | null>(null);

const typeCount = computed(() => Object.keys(stats.value?.by_type || {}).length);
const maxCount = computed(() => {
  const vals = Object.values(stats.value?.by_type || {});
  return vals.length ? Math.max(...vals) : 0;
});
const regionEntries = computed(() => Object.entries(stats.value?.by_region || {}).sort((a, b) => b[1] - a[1]));
const orgEntries = computed(() => Object.entries(stats.value?.by_org || {}).sort((a, b) => b[1] - a[1]));

function barBgColor(type: string): string {
  if (type === "enemy" || type === "rival") return "bg-error";
  if (type === "friend" || type === "lover" || type === "spouse") return "bg-success";
  if (type === "parent" || type === "child" || type === "sibling" || type === "master" || type === "apprentice") return "bg-tertiary";
  return "bg-primary";
}

function goRegion(id: string) {
  router.push(`/region/${id}`);
}
function goOrg(id: string) {
  router.push(`/org/${id}`);
}

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    stats.value = await loadSocialStats();
    if (!stats.value) error.value = t("simverse.socialNoRelations");
  } catch (e: any) {
    error.value = e.message || "Failed to load social stats";
  } finally {
    loading.value = false;
  }
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

  p {
    color: var(--color-error);
    margin: 0;
  }
}
</style>
