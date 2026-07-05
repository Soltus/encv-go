<!--
  ServerStatusCard.vue — 后端连接状态可视化卡片

  单一职责：把后端进程的健康度（online/offline/checking）+ 身份（version / instance_id /
  port / transport / latency / last check）可视化成一张可交互的卡片。**不承载任何设置项
  配置 / 表单 / 列表项 / 跳转入口**。需要这些的请用 ion-item / form。

  使用边界（避免与"设置项"卡片混淆）：
    ✅ 可用位置
      · ServerDetail.vue        详情页状态行
      · DevLogs.vue             debug 工具顶栏（compact 模式）
    ❌ 不可用位置（这些是"设置"语义，放这里会让用户误以为是"系统设置建议"）
      · Settings.vue            设置首页（配置项 + 主题 + 权限）
      · ServerSettings.vue      URL 配置页（属于"配置 backend 怎么连"，不是"显示连上没"）
      · AgentSettingsDetail.vue Agent 设置详情（"AI 行为配置"，不是"后端健康度"）

  视觉：3D 实体化（perspective + 厚度底面 + 表面光泽 + 倒角 + hover 抬起 translateZ/rotateX）
  + 双面翻转（Grid 叠放 + backface-visibility，告别 absolute 残留 + 高度跳变）。
  高度自适应：JS 测两个面 offsetHeight → 设容器 min-height + transition → 平滑伸缩。
  主题：100% CSS variables（--ion-color-* / --ion-background-color / --ion-text-color），
        0 硬编码颜色。深色模式自动适配。

  使用：
    <ServerStatusCard
      :clickable="true"
      :compact="false"
      @click="goServerDetail"
      @check="checkStatus"
      @restart="restartBackend"
      @stop="stopBackend"
    />
-->
<template>
  <div
    ref="wrapperRef"
    class="server-status-card"
    :class="[
      `state-${state}`,
      { [flipClass]: true, 'is-pulse': pulsing, 'is-compact': compact, 'is-clickable': clickable },
    ]"
    role="status"
    :aria-label="ariaLabel"
    @click="onCardClick"
  >
    <div ref="innerRef" class="card-3d-inner">
      <!-- ============ 正面：状态概览 + 操作按钮 ============ -->
      <div ref="frontRef" class="card-face card-face-front">
        <!-- 状态行：左 = dot + label，中 = flex:1，右 = 操作按钮 -->
        <div class="status-row">
          <div class="status-indicator">
            <span class="pulse-dot" :class="`pulse-${state}`">
              <span class="pulse-dot-inner" />
            </span>
            <div class="status-text">
              <span class="status-label">{{ statusLabel }}</span>
            </div>
          </div>

          <!-- 操作按钮内嵌卡片内（refresh / restart / stop） -->
          <div v-if="!hideActions" class="status-actions" @click.stop>
            <ion-button
              fill="outline"
              size="small"
              :title="t('serverStatus.refresh')"
              :disabled="checking"
              @click.stop="emit('check')"
            >
              <ion-spinner v-if="checking" slot="icon-only" name="crescent" />
              <ion-icon v-else :icon="refreshIcon" slot="icon-only" />
            </ion-button>
            <ion-button
              v-if="checking"
              fill="outline"
              size="small"
              color="medium"
              disabled
            >
              <ion-spinner slot="icon-only" name="crescent" />
            </ion-button>
            <ion-button
              v-else-if="isOnline"
              fill="outline"
              size="small"
              color="danger"
              :disabled="stopping"
              :title="t('serverStatus.stop')"
              @click.stop="emit('stop')"
            >
              <ion-spinner v-if="stopping" slot="icon-only" name="crescent" />
              <ion-icon v-else :icon="stopIcon" slot="icon-only" />
            </ion-button>
            <ion-button
              v-else
              fill="outline"
              size="small"
              color="warning"
              :title="t('serverStatus.start')"
              @click.stop="emit('restart')"
            >
              <ion-icon :icon="playIcon" slot="icon-only" />
            </ion-button>
          </div>
        </div>

        <!-- meta pills（latency + transport wifi） -->
        <div v-if="latencyPillVisible || transport" class="meta-row">
          <span
            v-if="latencyPillVisible"
            class="meta-pill"
            :class="`latency-${latencyQuality}`"
            :title="t('serverStatus.latencyHint')"
          >
            <ion-icon :icon="speedometerOutline" class="meta-pill-icon" />
            {{ latencyText }}
          </span>
          <span
            v-if="transport"
            class="meta-pill transport-pill"
            :title="t('serverStatus.transportHint')"
          >
            <ion-icon :icon="transportIcon" class="meta-pill-icon" />
            {{ transport }}
          </span>
        </div>

        <!-- instance-changed banner（4s 自动消失；不进 lastError，不阻塞状态） -->
        <div v-if="instanceChangedBanner" class="instance-changed-banner" role="status">
          <ion-icon :icon="refreshCircleIcon" class="banner-icon" />
          <span class="banner-text">
            {{ t('serverStatus.instanceChangedBanner') }}
            <code class="banner-prev">{{ instanceChangedBanner.previous.slice(0, 6) }}</code>
            <ion-icon :icon="arrowForwardIcon" class="banner-arrow" />
            <code class="banner-curr">{{ instanceChangedBanner.current.slice(0, 6) }}</code>
          </span>
          <button class="banner-close" :aria-label="t('common.close')" @click.stop="instanceChangedBanner = null">×</button>
        </div>

        <!-- 详细字段网格（仅 online 状态显示） -->
        <div v-if="state === 'online'" class="detail-grid">
          <div class="detail-item">
            <span class="detail-label">{{ t('serverStatus.version') }}</span>
            <span class="detail-value version-value">v{{ version || '—' }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-label">{{ t('serverStatus.instanceId') }}</span>
            <span
              class="detail-value monospace"
              :class="{ 'instance-changed': instanceChanged }"
              :title="backendInstanceId || ''"
            >
              {{ shortInstanceId || '—' }}
            </span>
          </div>
          <div class="detail-item">
            <span class="detail-label">{{ t('serverStatus.port') }}</span>
            <span class="detail-value port-value">:{{ port || '—' }}</span>
          </div>
          <div class="detail-item">
            <span class="detail-label">{{ t('serverStatus.lastCheck') }}</span>
            <span class="detail-value last-check-value">
              <span :key="lastCheckKey" class="time-roll">{{ lastCheckText }}</span>
            </span>
          </div>
        </div>

        <!-- 错误态 -->
        <div v-else-if="state === 'offline'" class="error-body">
          <ion-icon :icon="cloudOfflineOutline" class="error-icon" />
          <div class="error-text">
            <span class="error-title">{{ t('serverStatus.backendOffline') }}</span>
            <span v-if="error" class="error-detail">{{ error }}</span>
          </div>
        </div>

        <!-- 检查态 -->
        <div v-else-if="state === 'checking'" class="error-body checking-body">
          <ion-spinner name="crescent" class="checking-spinner" />
          <span class="error-title">{{ t('serverStatus.checking') }}</span>
        </div>

        <!-- 翻转提示 -->
        <div class="flip-hint" aria-hidden="true">
          <ion-icon :icon="refreshIcon" class="flip-hint-icon" />
          <span class="flip-hint-text">{{ t('serverStatus.flipHint') }}</span>
        </div>
      </div>

      <!-- ============ 反面：诊断 / 操作历史 ============ -->
      <div ref="backRef" class="card-face card-face-back">
        <div class="back-header">
          <ion-icon :icon="pulseIcon" class="back-header-icon" />
          <span class="back-header-title">{{ t('serverStatus.diagnosticsTitle') }}</span>
        </div>

        <!-- 完整 instance_id（不再截断） -->
        <div class="back-section">
          <div class="back-label">{{ t('serverStatus.fullInstanceId') }}</div>
          <div
            class="back-value monospace"
            :class="{ 'instance-changed': instanceChanged }"
            :title="backendInstanceId || ''"
          >
            {{ backendInstanceId || '—' }}
          </div>
        </div>

        <!-- 完整 lastError 详情（仅 offline 时显示） -->
        <div v-if="state === 'offline' && error" class="back-section">
          <div class="back-label">{{ t('serverStatus.fullError') }}</div>
          <div class="back-value error-text-mono">{{ error }}</div>
        </div>

        <!-- 完整 transport 描述 -->
        <div class="back-section">
          <div class="back-label">{{ t('serverStatus.transportDesc') }}</div>
          <div class="back-value">
            <ion-icon :icon="transportIcon" class="back-transport-icon" />
            {{ transportFullLabel }}
          </div>
        </div>

        <!-- 时间戳详情 -->
        <div v-if="!compact" class="back-section">
          <div class="back-label">{{ t('serverStatus.timestamp') }}</div>
          <div class="back-value monospace">{{ lastCheckAbsolute }}</div>
        </div>

        <div class="flip-hint" aria-hidden="true">
          <ion-icon :icon="flipBackIcon" class="flip-hint-icon" />
          <span class="flip-hint-text">{{ t('serverStatus.flipBackHint') }}</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { layersOutline, wifiOutline } from "ionicons/icons";
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { formatRelativeTime } from "@/composables/relativeTime";
import { eventBus } from "@/composables/useEventBus";
import { useI18n } from "@/composables/useI18n";
import { useServerStatus } from "@/composables/useServerStatus";

interface Props {
  /** 紧凑模式：省略反面时间戳详情 */
  compact?: boolean;
  /** 卡片可点击 → 触发外部 click 事件（注意：内部翻转也走 click，但 emit 仍 fire） */
  clickable?: boolean;
  /** 隐藏内嵌操作按钮（让父级自己渲染） */
  hideActions?: boolean;
}
const props = withDefaults(defineProps<Props>(), {
  compact: false,
  clickable: false,
  hideActions: true, // 默认隐藏操作按钮 — 只有 ServerDetail 这种"健康度 + 操作"场景显式启用
});
const emit = defineEmits<{
  (e: "click"): void;
  (e: "check"): void;
  (e: "restart"): void;
  (e: "stop"): void;
}>();

const { t } = useI18n();
const {
  isOnline,
  lastError,
  backendPort,
  backendInstanceId,
  backendVersion,
  latencyMs,
  lastCheckedAt,
  transportMode,
  checkStatus,
  restartBackend,
  stopBackend,
  isRestarting,
  isStopping,
} = useServerStatus();

// —— refs ——
const wrapperRef = ref<HTMLElement | null>(null);
const _innerRef = ref<HTMLElement | null>(null);
const frontRef = ref<HTMLElement | null>(null);
const backRef = ref<HTMLElement | null>(null);

// —— state machine: online | offline | checking ——
const checking = computed(() => isRestarting.value); // 重启中 ≡ 检查中
const state = computed<"online" | "offline" | "checking">(() => {
  if (checking.value) return "checking";
  return isOnline.value ? "online" : "offline";
});
const _stopping = computed(() => isStopping.value);

// —— 3D 翻转（Android 兼容） ——
const isFlipped = ref(false);
watch(state, () => {
  // 状态切换时自动回正面（避免误导）
  isFlipped.value = false;
});

// —— 3D 翻转（统一逻辑） ——
//
// 🆕 2026-06-16 恢复统一 3D rotateY 翻转
//   历史 Android 上用过 is-flip-fade（opacity fade）当 fallback —
//     当时症状是点击卡片跳转 /tabs/settings/server/status（被旧 @click 残留影响）
//   真相：跳转的不是「翻转泄露」，是 ServerDetail.vue 旧 @click 残留
//     旧残留已删（详情页文件 + router 路由 + 组件 click 绑定），Android 上点击恢复正常翻转
//   现在：所有平台统一用 is-flipped 3D rotateY
//
// 防御性仍保留（防 Android WebView 极端情况）：
//   - isolation: isolate（独立 stacking context，防 backdrop-filter 抓合成层）
//   - .error-detail max-height（防离线错误文本撑爆卡片）
//   - .back-value.monospace max-height（防长 instance_id 撑爆）
const _flipClass = computed(() => (isFlipped.value ? "is-flipped" : ""));

// —— 脉冲 / 光泽动画 ——
const pulsing = ref(false);
watch(state, () => {
  pulsing.value = false;
  requestAnimationFrame(() => {
    pulsing.value = true;
  });
  setTimeout(() => {
    pulsing.value = false;
  }, 1200);
});

// —— 标签文案 ——
const statusLabel = computed(() => {
  switch (state.value) {
    case "online":
      return t("serverStatus.online");
    case "offline":
      return t("serverStatus.offline");
    case "checking":
      return t("serverStatus.checking");
  }
});

// —— aria ——
const _ariaLabel = computed(() => {
  const bits: string[] = [statusLabel.value];
  if (state.value === "online") {
    if (version.value) bits.push(`v${version.value}`);
    if (port.value) bits.push(`port ${port.value}`);
  } else if (state.value === "offline" && lastError.value) {
    bits.push(lastError.value);
  }
  return bits.join(", ");
});

// —— detail 字段 ——
const version = computed(() => backendVersion.value);
const port = computed(() => backendPort.value);
const _shortInstanceId = computed(() => {
  return backendInstanceId.value ? backendInstanceId.value.slice(0, 8) : "";
});
const _error = computed(() => lastError.value);

// —— instance_id 变化检测 → 闪烁 1.5s ——
const instanceChanged = ref(false);
const prevInstanceId = ref("");
watch(backendInstanceId, newId => {
  if (prevInstanceId.value && newId && prevInstanceId.value !== newId) {
    instanceChanged.value = true;
    setTimeout(() => {
      instanceChanged.value = false;
    }, 1500);
  }
  prevInstanceId.value = newId;
});

// —— 监听 useServerStatus 发的 'backend:instance-changed' 事件（4s banner） ——
let bannerTimer: ReturnType<typeof setTimeout> | null = null;
const instanceChangedBanner = ref<{ previous: string; current: string } | null>(null);
function onInstanceChanged(data: { previous: string; current: string }) {
  instanceChangedBanner.value = data;
  if (bannerTimer) clearTimeout(bannerTimer);
  bannerTimer = setTimeout(() => {
    instanceChangedBanner.value = null;
    bannerTimer = null;
  }, 4000);
}

// —— latency 分类 / 显示 ——
const _latencyText = computed(() => {
  if (latencyMs.value <= 0) return "—";
  if (latencyMs.value < 1000) return `${latencyMs.value}ms`;
  return `${(latencyMs.value / 1000).toFixed(2)}s`;
});
const _latencyQuality = computed<"fast" | "normal" | "slow" | "unknown">(() => {
  if (latencyMs.value <= 0) return "unknown";
  if (latencyMs.value < 100) return "fast";
  if (latencyMs.value < 500) return "normal";
  return "slow";
});
const _latencyPillVisible = computed(() => state.value === "online" && latencyMs.value > 0);

// —— transport 显示 ——
const _transport = computed(() => {
  const m = transportMode.value;
  return m && m !== "unknown" ? m.toUpperCase() : "";
});
const _transportFullLabel = computed(() => {
  const m = transportMode.value;
  switch (m) {
    case "ws":
      return t("serverStatus.transportWs");
    case "http-poll":
      return t("serverStatus.transportHttpPoll");
    case "native-bridge":
      return t("serverStatus.transportNativeBridge");
    default:
      return t("serverStatus.transportUnknown");
  }
});
const _transportIcon = computed(() => {
  const m = transportMode.value;
  // 用户要求：HTTP Polling 旁必须有 wifi 图标
  if (m === "ws" || m === "http-poll" || m === "native-bridge") return wifiOutline;
  return layersOutline;
});

// —— last check 时间（30s 滚动刷新）——
const _lastCheckText = computed(() => {
  if (!lastCheckedAt.value) return t("serverStatus.never");
  return formatRelativeTime(lastCheckedAt.value.getTime());
});
const _lastCheckAbsolute = computed(() => {
  if (!lastCheckedAt.value) return "—";
  const d = lastCheckedAt.value;
  const pad = (n: number) => n.toString().padStart(2, "0");
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
});
const lastCheckKey = ref(0);
const now = ref(Date.now());
let tickHandle: ReturnType<typeof setInterval> | null = null;

// —— 高度自适应 + 平滑伸缩 ——
async function syncHeight() {
  await nextTick();
  if (!wrapperRef.value || !frontRef.value || !backRef.value) return;
  const fh = frontRef.value.offsetHeight;
  const bh = backRef.value.offsetHeight;
  const max = Math.max(fh, bh, 160); // 兜底 min-height
  wrapperRef.value.style.minHeight = `${max}px`;
}

onMounted(() => {
  // 初始高度
  syncHeight();
  // 监听窗口尺寸变化
  window.addEventListener("resize", syncHeight);
  // 30s 滚动刷新
  tickHandle = setInterval(() => {
    now.value = Date.now();
    lastCheckKey.value++;
  }, 30_000);
  eventBus.on("backend:instance-changed", onInstanceChanged);
});
onUnmounted(() => {
  if (tickHandle) clearInterval(tickHandle);
  if (bannerTimer) clearTimeout(bannerTimer);
  window.removeEventListener("resize", syncHeight);
  eventBus.off("backend:instance-changed", onInstanceChanged);
});

// 翻转时 / 状态切换时 / 关键数据变化时重测高度
watch([isFlipped, state, isOnline, transportMode, lastError, instanceChangedBanner], () => {
  syncHeight();
});

// —— 点击卡片主体翻转 ——
function _onCardClick(event: MouseEvent) {
  const target = event.target as HTMLElement;
  // 阻止子元素点击冒泡时翻转（按钮 / pill / 链接 / flip-hint）
  if (target.closest("button, a, .meta-pill, .flip-hint, .status-actions, ion-button")) {
    return;
  }
  isFlipped.value = !isFlipped.value;
  if (props.clickable) emit("click");
}

// —— expose to parent ——
defineExpose({ checkStatus, restartBackend, stopBackend });
</script>

<style scoped>
/* ============================================================
   ServerStatusCard — 3D 实体化 + 双面翻转 + 高度自适应
   100% CSS variables — 0 硬编码颜色。深色模式自动适配。
   ============================================================ */

/* ============ 外层 3D 上下文（拟物 + 金属拉丝 + 倒角厚度） ============
   关键兼容性：Android WebView 不可靠 backface-visibility，因此使用
   visibility + opacity + 自身 rotateY(180) 抵消外层旋转。
   见 L596-628 配套规则。
   */
.server-status-card {
  --card-bg: var(--ion-background-color, #fff);
  --card-border: var(--ion-color-medium, #92949c);
  --card-text: var(--ion-text-color, #000);
  --card-text-muted: color-mix(in srgb, var(--ion-text-color, #000) 60%, transparent);
  --card-accent: var(--ion-color-primary, #3880ff);
  --card-radius: 14px;
  --transition-3d: 0.6s cubic-bezier(0.4, 0.0, 0.2, 1);
  --transition-fast: 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  --transition-height: 0.45s cubic-bezier(0.4, 0, 0.2, 1);

  position: relative;
  display: grid;
  grid-template-areas: 'stack';
  perspective: 1500px;
  perspective-origin: 50% 30%;
  width: 100%;
  min-height: 160px; /* JS 同步后会覆盖 */
  transition: min-height var(--transition-height);

  /* 🆕 2026-06-16 修复 Android 翻转"幽灵内容"
     根因链：
     1. ion-tab-bar 的 backdrop-filter: blur(20px) 在 Android WebView 上抓取
        屏幕所有 GPU 合成层的内容作为模糊输入
     2. ServerStatusCard 翻转时 transform 触发 GPU 合成层
     3. 合成层被 tab bar 的 backdrop-filter "贴" 到 tab bar 区域显示
        → 背面"应用信息"等内容在 tab bar 左下角"穿透"出来
        → 翻转回来也不消失（合成层仍在）
     修复：isolation: isolate 创建独立 stacking context，backdrop-filter 抓不到 */
  isolation: isolate;

  /* 拟物：外层 drop-shadow（卡片浮起阴影） */
  filter:
    drop-shadow(0 1px 1px rgba(0, 0, 0, 0.10))
    drop-shadow(0 4px 8px rgba(0, 0, 0, 0.08))
    drop-shadow(0 10px 20px rgba(0, 0, 0, 0.06))
    drop-shadow(0 24px 48px rgba(0, 0, 0, 0.04));
}
.server-status-card.is-clickable { cursor: pointer; }

/* hover 抬起 + 微旋转（3D 凸起效果） */
.server-status-card.is-clickable:hover {
  filter:
    drop-shadow(0 2px 2px rgba(0, 0, 0, 0.12))
    drop-shadow(0 8px 16px rgba(0, 0, 0, 0.12))
    drop-shadow(0 16px 32px rgba(0, 0, 0, 0.10))
    drop-shadow(0 32px 64px rgba(0, 0, 0, 0.06));
  transform: translateY(-2px);
}

/* ============ 内层翻转元素 ============ */
.card-3d-inner {
  grid-area: stack;
  transform-style: preserve-3d;
  transition: transform var(--transition-3d);
  display: grid;
  grid-template-areas: 'stack';
}
.server-status-card.is-flipped .card-3d-inner {
  transform: rotateY(180deg);
}

/* ============ 双面通用：Grid 叠放 + 自然高度 + 拟物表面 ============
   拟物三层叠加：
   1) `linear-gradient` 顶部高光 + 底部内阴影（玻璃/塑料光泽）
   2) `repeating-linear-gradient` 45° 极细斜纹（金属拉丝/磨砂质感）
   3) `box-shadow` inset 多层（顶部高光 1px、底部 1px、左侧 1px 倒角、右侧 1px 倒角）
   */
.card-face {
  grid-area: stack;
  /* 关键：不再 position: absolute → 高度由内容自然撑开，Grid 取 max */
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 16px;

  /* 表面层：拉丝 + 顶部高光 + 底部内阴影 */
  background:
    /* 顶部高光带（玻璃效果） */
    linear-gradient(180deg,
      rgba(255, 255, 255, 0.65) 0%,
      rgba(255, 255, 255, 0.20) 4%,
      transparent 14%,
      transparent 86%,
      rgba(0, 0, 0, 0.05) 100%),
    /* 金属拉丝（45° 极细斜纹） */
    repeating-linear-gradient(135deg,
      rgba(255, 255, 255, 0.025) 0px,
      rgba(255, 255, 255, 0.025) 1px,
      transparent 1px,
      transparent 3px),
    /* 深色模式下拉丝反向（黑丝纹理） */
    repeating-linear-gradient(45deg,
      rgba(0, 0, 0, 0.04) 0px,
      rgba(0, 0, 0, 0.04) 1px,
      transparent 1px,
      transparent 4px),
    /* 底色（主题色感知） */
    var(--card-bg);

  color: var(--card-text);
  border: 1px solid var(--card-border);
  border-left-width: 4px;
  border-radius: var(--card-radius);

  /* 倒角 + 多层玻璃高光 inset（顶 / 底 / 左 / 右倒角） */
  box-shadow:
    /* 顶部高光（白色 1px） */
    inset 0 1px 0 rgba(255, 255, 255, 0.5),
    /* 底部 1px 暗边（倒角） */
    inset 0 -1px 0 rgba(0, 0, 0, 0.10),
    /* 左侧 1px 高光（倒角） */
    inset 1px 0 0 rgba(255, 255, 255, 0.18),
    /* 右侧 1px 暗边（倒角） */
    inset -1px 0 0 rgba(0, 0, 0, 0.06),
    /* 中间下沉的柔和阴影（"凸起"感） */
    inset 0 2px 6px rgba(0, 0, 0, 0.04);

  transition:
    border-color var(--transition-fast),
    background-color var(--transition-fast),
    transform var(--transition-3d),
    opacity var(--transition-3d),
    visibility 0s linear var(--transition-3d);

  min-height: inherit; /* 让面继承 wrapper 的 min-height */
  overflow: hidden;
  /* 显式设置 backface-visibility 作为双保险（iOS Safari 必需要） */
  backface-visibility: hidden;
  -webkit-backface-visibility: hidden;
}
.server-status-card.is-clickable .card-face { cursor: pointer; }

/* 反面：自身 rotateY(180) 抵消外层翻转（防止文字镜像）
   + visibility/opacity 严格控制"未翻转时不可见"（Android WebView 兼容） */
.card-face-back {
  transform: rotateY(180deg);
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
}
/* 翻转后：正面隐藏，反面显示 */
.server-status-card.is-flipped .card-face-front {
  opacity: 0;
  visibility: hidden;
  pointer-events: none;
  transition:
    border-color var(--transition-fast),
    background-color var(--transition-fast),
    transform var(--transition-3d),
    opacity var(--transition-3d),
    visibility 0s linear var(--transition-3d);
}
.server-status-card.is-flipped .card-face-back {
  opacity: 1;
  visibility: visible;
  pointer-events: auto;
  transition:
    border-color var(--transition-fast),
    background-color var(--transition-fast),
    transform var(--transition-3d),
    opacity var(--transition-3d),
    visibility 0s linear 0s; /* 翻转开始即恢复可见（不是延迟到末） */
}

/* ============ 状态变体：边框 + 主题色 ============ */
.server-status-card.state-online {
  --card-accent: var(--ion-color-success, #2dd55b);
  --card-border: color-mix(in srgb, var(--ion-color-success, #2dd55b) 30%, var(--ion-color-medium, #92949c));
}
.server-status-card.state-offline {
  --card-accent: var(--ion-color-danger, #eb445a);
  --card-border: color-mix(in srgb, var(--ion-color-danger, #eb445a) 35%, var(--ion-color-medium, #92949c));
}
.server-status-card.state-checking {
  --card-accent: var(--ion-color-warning, #ffc409);
  --card-border: color-mix(in srgb, var(--ion-color-warning, #ffc409) 35%, var(--ion-color-medium, #92949c));
}
.card-face { border-left-color: var(--card-accent); }

/* ============ 3D 实体化关键：底面 + 表面光泽 ============ */

/* 底面（::before）：在卡片正下方 4px 处，模拟卡片厚度 */
.server-status-card::before {
  content: '';
  position: absolute;
  left: 6px;
  right: 6px;
  bottom: -3px;
  height: 6px;
  background: linear-gradient(180deg, rgba(0, 0, 0, 0.08), rgba(0, 0, 0, 0.02));
  border-radius: 0 0 var(--card-radius) var(--card-radius);
  filter: blur(2px);
  z-index: -1;
  opacity: 0.5;
  transition: opacity var(--transition-fast);
}
.server-status-card:hover::before { opacity: 0.8; }

/* 表面光泽（::after）：对角线渐变 + 状态色高光 */
.card-face::after {
  content: '';
  position: absolute;
  inset: 0;
  background:
    /* 对角线斜光（高光从左上扫到右下） */
    linear-gradient(135deg,
      rgba(255, 255, 255, 0.10) 0%,
      transparent 35%,
      transparent 65%,
      rgba(0, 0, 0, 0.04) 100%);
  pointer-events: none;
  border-radius: inherit;
  opacity: 0.85;
  transition: opacity var(--transition-fast);
}
/* 状态色高光带（顶部 1px 用 accent 色） */
.card-face::before {
  content: '';
  position: absolute;
  inset: 0 0 auto 0;
  height: 1px;
  background: linear-gradient(90deg,
    transparent 0%,
    var(--card-accent) 20%,
    var(--card-accent) 80%,
    transparent 100%);
  opacity: 0.4;
  pointer-events: none;
  border-radius: var(--card-radius) var(--card-radius) 0 0;
}
.server-status-card.is-pulse .card-face-front::after {
  animation: ssc-sheen 1.2s ease-out;
}
@keyframes ssc-sheen {
  0%   { opacity: 0.85; }
  50%  { opacity: 1; }
  100% { opacity: 0.85; }
}

/* 状态色底部柔光（accent 色内阴影，让卡片"发光"） */
.server-status-card.state-online .card-face {
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.5),
    inset 0 -1px 0 rgba(0, 0, 0, 0.10),
    inset 1px 0 0 rgba(255, 255, 255, 0.18),
    inset -1px 0 0 rgba(0, 0, 0, 0.06),
    inset 0 2px 6px rgba(0, 0, 0, 0.04),
    inset 0 -2px 4px color-mix(in srgb, var(--ion-color-success, #2dd55b) 8%, transparent);
}
.server-status-card.state-offline .card-face {
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.5),
    inset 0 -1px 0 rgba(0, 0, 0, 0.10),
    inset 1px 0 0 rgba(255, 255, 255, 0.18),
    inset -1px 0 0 rgba(0, 0, 0, 0.06),
    inset 0 2px 6px rgba(0, 0, 0, 0.04),
    inset 0 -2px 4px color-mix(in srgb, var(--ion-color-danger, #eb445a) 8%, transparent);
}
.server-status-card.state-checking .card-face {
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.5),
    inset 0 -1px 0 rgba(0, 0, 0, 0.10),
    inset 1px 0 0 rgba(255, 255, 255, 0.18),
    inset -1px 0 0 rgba(0, 0, 0, 0.06),
    inset 0 2px 6px rgba(0, 0, 0, 0.04),
    inset 0 -2px 4px color-mix(in srgb, var(--ion-color-warning, #ffc409) 8%, transparent);
}

/* ============ 状态行（正面） ============ */
.status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 36px;
}
.status-indicator {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  flex: 1;
}
.status-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.status-label {
  font-size: 15px;
  font-weight: 600;
  color: var(--card-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* ============ 操作按钮（内嵌卡片内右侧） ============ */
.status-actions {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}
/* 让卡片内 ion-button 紧凑 + 不抢空间 */
.status-actions ion-button {
  --padding-start: 6px;
  --padding-end: 6px;
  margin: 0;
  height: 30px;
  min-height: 30px;
}
.status-actions ion-button::part(native) {
  padding: 0 6px;
  min-height: 28px;
}

/* ============ Pulse dot ============ */
.pulse-dot {
  position: relative;
  display: inline-flex;
  width: 14px;
  height: 14px;
  border-radius: 50%;
  flex-shrink: 0;
}
.pulse-dot-inner {
  position: absolute;
  inset: 0;
  border-radius: 50%;
  background: currentColor;
}
.pulse-dot::after {
  content: '';
  position: absolute;
  inset: -4px;
  border-radius: 50%;
  background: currentColor;
  opacity: 0;
  animation: ssc-pulse 2s ease-out infinite;
  z-index: -1;
}
.pulse-online { color: var(--ion-color-success, #2dd55b); }
.pulse-online::after { animation-name: ssc-pulse-success; }
.pulse-offline { color: var(--ion-color-danger, #eb445a); }
.pulse-offline::after { animation-name: ssc-pulse-danger; }
.pulse-checking { color: var(--ion-color-warning, #ffc409); }
.pulse-checking::after { animation-name: ssc-pulse-warning; }
@keyframes ssc-pulse-success {
  0% { transform: scale(0.8); opacity: 0.7; }
  100% { transform: scale(2); opacity: 0; }
}
@keyframes ssc-pulse-danger {
  0% { transform: scale(0.8); opacity: 0.7; }
  100% { transform: scale(2); opacity: 0; }
}
@keyframes ssc-pulse-warning {
  0% { transform: scale(0.8); opacity: 0.5; }
  100% { transform: scale(1.8); opacity: 0; }
}

/* ============ meta-row（latency + transport） ============ */
.meta-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.meta-pill {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 3px 9px;
  font-size: 11px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  border-radius: 999px;
  background: color-mix(in srgb, var(--ion-color-medium, #92949c) 18%, transparent);
  color: var(--card-text);
  transition: background-color var(--transition-fast);
}
.meta-pill-icon {
  font-size: 12px;
  width: 12px;
  height: 12px;
}
.meta-pill.latency-fast {
  background: color-mix(in srgb, var(--ion-color-success, #2dd55b) 22%, transparent);
  color: var(--ion-color-success, #2dd55b);
}
.meta-pill.latency-normal {
  background: color-mix(in srgb, var(--ion-color-primary, #3880ff) 18%, transparent);
  color: var(--ion-color-primary, #3880ff);
}
.meta-pill.latency-slow {
  background: color-mix(in srgb, var(--ion-color-warning, #ffc409) 24%, transparent);
  color: var(--ion-color-warning-shade, #cc8a00);
}
.meta-pill.latency-unknown {
  background: color-mix(in srgb, var(--ion-color-medium, #92949c) 18%, transparent);
  color: var(--card-text-muted);
}
.meta-pill.transport-pill {
  background: color-mix(in srgb, var(--ion-color-primary, #3880ff) 18%, transparent);
  color: var(--ion-color-primary, #3880ff);
}

/* ============ instance-changed banner ============ */
.instance-changed-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  margin: 4px 0 0;
  padding: 7px 10px;
  background: color-mix(in srgb, var(--ion-color-warning, #ffc409) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--ion-color-warning, #ffc409) 30%, transparent);
  border-radius: 6px;
  font-size: 12px;
  color: var(--ion-color-warning-shade, #cc8a00);
  animation: banner-in 0.3s ease-out;
}
@keyframes banner-in {
  from { opacity: 0; transform: translateY(-4px); }
  to { opacity: 1; transform: translateY(0); }
}
.banner-icon { font-size: 16px; flex-shrink: 0; }
.banner-text { flex: 1; line-height: 1.4; }
.banner-prev, .banner-curr {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  padding: 0 4px;
  background: var(--ion-color-light, #f4f5f8);
  border-radius: 3px;
  color: var(--ion-text-color, #000);
}
.banner-curr {
  background: color-mix(in srgb, var(--ion-color-warning, #ffc409) 20%, transparent);
  color: var(--ion-color-warning-shade, #cc8a00);
}
.banner-arrow { font-size: 12px; opacity: 0.6; vertical-align: middle; }
.banner-close {
  background: transparent;
  border: 0;
  color: inherit;
  font-size: 18px;
  line-height: 1;
  padding: 0 4px;
  cursor: pointer;
  opacity: 0.6;
}
.banner-close:hover { opacity: 1; }

/* ============ Detail grid（正面） ============ */
.detail-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px 14px;
  padding-top: 8px;
  border-top: 1px solid color-mix(in srgb, var(--card-border) 50%, transparent);
}
.detail-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}
.detail-label {
  font-size: 11px;
  color: var(--card-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 500;
}
.detail-value {
  font-size: 13px;
  color: var(--card-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.detail-value.monospace {
  font-family: var(--ion-font-family-monospace, 'SF Mono', Menlo, Consolas, monospace);
  font-size: 12px;
  background: color-mix(in srgb, var(--ion-color-medium, #92949c) 12%, transparent);
  padding: 1px 5px;
  border-radius: 3px;
  display: inline-block;
  max-width: 100%;
  transition: background-color var(--transition-fast);
}
.detail-value.instance-changed {
  animation: ssc-instance-change 1.5s ease-out;
}
@keyframes ssc-instance-change {
  0%   { background: color-mix(in srgb, var(--ion-color-warning, #ffc409) 50%, transparent); transform: scale(1.04); }
  100% { background: color-mix(in srgb, var(--ion-color-medium, #92949c) 12%, transparent); transform: scale(1); }
}
.detail-value.port-value {
  font-family: var(--ion-font-family-monospace, monospace);
  color: var(--ion-color-primary, #3880ff);
  font-weight: 600;
}
.detail-value.version-value {
  color: var(--ion-color-primary, #3880ff);
  font-weight: 500;
}
.time-roll {
  display: inline-block;
  animation: ssc-time-roll 0.3s ease-out;
}
@keyframes ssc-time-roll {
  0% { transform: translateY(2px); opacity: 0.4; }
  100% { transform: translateY(0); opacity: 1; }
}

/* ============ Error body（正面） ============ */
.error-body {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 8px 0;
  border-top: 1px solid color-mix(in srgb, var(--card-border) 50%, transparent);
}
.error-body.checking-body { align-items: center; }
.error-icon { font-size: 22px; color: var(--ion-color-danger, #eb445a); flex-shrink: 0; }
.checking-spinner { width: 22px; height: 22px; color: var(--ion-color-warning, #ffc409); }
.error-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
  min-height: 0;
}
.error-title { font-size: 13px; font-weight: 600; color: var(--ion-color-danger, #eb445a); }
/* 🆕 2026-06-16 限制 error-detail 高度 — 错误文本可能很长（如 stack trace / multiline log），
   不限 max-height 会把卡片撑到 1 屏高度，物理越界到 ion-tab-bar 区域 */
.error-detail {
  font-size: 12px;
  color: var(--card-text-muted);
  word-break: break-word;
  max-height: 60px;
  overflow-y: auto;
}

/* ============ 反面：诊断 / 操作历史 ============ */
.back-header {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-bottom: 8px;
  border-bottom: 1px solid color-mix(in srgb, var(--card-border) 50%, transparent);
}
.back-header-icon {
  font-size: 18px;
  color: var(--ion-color-primary, #3880ff);
}
.back-header-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--card-text);
}
.back-section {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
}
.back-label {
  font-size: 10px;
  color: var(--card-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.04em;
  font-weight: 500;
}
.back-value {
  font-size: 12px;
  color: var(--card-text);
  word-break: break-all;
  display: flex;
  align-items: center;
  gap: 6px;
}
.back-value.monospace {
  font-family: var(--ion-font-family-monospace, 'SF Mono', Menlo, Consolas, monospace);
  background: color-mix(in srgb, var(--ion-color-medium, #92949c) 12%, transparent);
  padding: 2px 6px;
  border-radius: 3px;
  display: inline-block;
  max-width: 100%;
  word-break: break-all;
  /* 🆕 2026-06-16 限制 monospace 文本最多 2 行 + scroll — instance_id 长时（如 32 字符 uuid）撑高卡片 */
  max-height: 2.8em;
  overflow-y: auto;
  line-height: 1.4;
}
.back-value.monospace.instance-changed {
  animation: ssc-instance-change 1.5s ease-out;
}
.back-value.error-text-mono {
  font-family: var(--ion-font-family-monospace, monospace);
  color: var(--ion-color-danger, #eb445a);
  background: color-mix(in srgb, var(--ion-color-danger, #eb445a) 8%, transparent);
  padding: 4px 8px;
  border-radius: 4px;
  border-left: 2px solid var(--ion-color-danger, #eb445a);
  white-space: pre-wrap;
  max-height: 80px;
  overflow-y: auto;
}
.back-transport-icon {
  font-size: 14px;
  width: 14px;
  height: 14px;
  color: var(--ion-color-primary, #3880ff);
}

/* ============ 翻转提示 ============ */
.flip-hint {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 4px;
  margin-top: auto;
  padding-top: 6px;
  font-size: 10px;
  color: var(--card-text-muted);
  opacity: 0.5;
  transition: opacity var(--transition-fast);
}
.server-status-card:hover .flip-hint { opacity: 0.9; }
.flip-hint-icon {
  font-size: 11px;
  width: 11px;
  height: 11px;
  animation: flip-hint-spin 2s linear infinite;
}
@keyframes flip-hint-spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}
.server-status-card.is-flipped .flip-hint-icon {
  animation: flip-hint-spin 2s linear infinite reverse;
}

/* ============ Compact mode ============ */
.server-status-card.is-compact .card-face {
  padding: 10px 12px;
  gap: 6px;
}
.server-status-card.is-compact .status-label { font-size: 13px; }
.server-status-card.is-compact .pulse-dot { width: 12px; height: 12px; }
.server-status-card.is-compact .detail-grid { display: none; }
.server-status-card.is-compact .status-actions { display: none; }

/* ============ 响应式 ============ */
@media (max-width: 380px) {
  .detail-grid { grid-template-columns: 1fr; }
  .meta-row { flex-direction: column; align-items: flex-start; }
  .status-actions ion-button { height: 28px; min-height: 28px; }
}

/* ============ 减弱动画（无障碍） ============ */
@media (prefers-reduced-motion: reduce) {
  .card-3d-inner { transition: transform 0.3s ease-out; }
  .pulse-dot::after { animation: none; }
  .detail-value.instance-changed,
  .back-value.monospace.instance-changed { animation: none; }
  .time-roll { animation: none; }
  .flip-hint-icon { animation: none; }
  .server-status-card { transition: none; }
}
</style>
