<!--
  AgentDebugPanel - Mock 模式自动开启的调试面板
  显示 SSE 事件解析后的实际状态：
   - messages 数量、role 分布、tool_calls/tool_results 数量
   - renderedItems 类型分布（关键：看 GroupedOperationMessage 到底有没有实例化）
   - 最近 N 条 message 的 tool_calls + tool_results 详情（看配对是否成功）
   - 是否有"伪造"（result 字符串与真实 list_files 路径是否匹配）

  触发条件（AgentChat 父组件控制）：
   - 默认：isMockMode 时自动显示
   - 任意模式：URL 加 ?debug=agent 时强制显示
  用 <details> 折叠默认收起，避免遮挡正常对话。
-->
<template>
  <details class="agentDebugPanel" :open="defaultOpen">
    <summary class="agentDebugSummary">
      <ion-icon :icon="bugIcon" class="agentDebugSummaryIcon" />
      <span>Agent 调试面板</span>
      <span class="agentDebugBadge">{{ messages.length }} msg · {{ renderedItems.length }} rendered</span>
    </summary>
    <div class="agentDebugBody">
      <!-- ① messages 分布 -->
      <section class="agentDebugSection">
        <h4>① messages ({{ messages.length }})</h4>
        <div class="agentDebugStats">
          <span v-for="(c, role) in roleCounts" :key="role" class="agentDebugChip">
            {{ role }}: {{ c }}
          </span>
        </div>
        <div class="agentDebugStats">
          <span class="agentDebugChip">tool_calls: {{ totalToolCalls }}</span>
          <span class="agentDebugChip">tool_results: {{ totalToolResults }}</span>
          <span class="agentDebugChip">pairing: {{ pairRateText }}</span>
        </div>
      </section>

      <!-- ② renderedItems 类型分布 -->
      <section class="agentDebugSection">
        <h4>② renderedItems ({{ renderedItems.length }})</h4>
        <div class="agentDebugStats">
          <span
            v-for="(c, t) in renderedTypeCounts"
            :key="t"
            class="agentDebugChip"
            :class="{ agentDebugChip_emphasis: t === 'operationGroup' && c > 0 }"
          >
            {{ t }}: {{ c }}
          </span>
        </div>
      </section>

      <!-- ③ 最近 N 条 message 的 tool_calls + tool_results 详情 -->
      <section class="agentDebugSection">
        <h4>③ 最近 {{ recentMessages.length }} 条 message 的 tool_calls ↔ tool_results</h4>
        <div v-for="(m, i) in recentMessages" :key="i" class="agentDebugMsg">
          <div class="agentDebugMsgHead">
            <span class="agentDebugChip">{{ m.role }}</span>
            <span class="agentDebugMsgId">#{{ i }}</span>
            <span class="agentDebugChip">tool_calls: {{ m.tool_calls.length }}</span>
            <span class="agentDebugChip">tool_results: {{ m.tool_results.length }}</span>
          </div>
          <ul v-if="m.tool_calls.length > 0" class="agentDebugList">
            <li v-for="tc in m.tool_calls" :key="tc.id" class="agentDebugListItem">
              <div class="agentDebugListHead">
                <span class="agentDebugName">{{ tc.name }}</span>
                <span class="agentDebugId">{{ tc.id }}</span>
                <span class="agentDebugChip" :class="`agentDebugStatus_${tc.status}`">{{ tc.status }}</span>
                <span class="agentDebugChip">kind: {{ tc.kind }}</span>
              </div>
              <div v-if="findResult(m, tc.id)" class="agentDebugResult">
                <span class="agentDebugResultTag">↳ result</span>
                <code class="agentDebugResultJson">{{ truncate(findResult(m, tc.id), 200) }}</code>
              </div>
              <div v-else class="agentDebugResult agentDebugResult_missing">
                <span class="agentDebugResultTag">↳ 缺 result</span>
                <span class="agentDebugHint">{{ resultStatusHint(tc.status) }}</span>
              </div>
            </li>
          </ul>
        </div>
      </section>

      <!-- ④ operationGroup 实际渲染预览（关键：看 tool_result 卡片是不是真的渲染） -->
      <section v-if="operationGroups.length > 0" class="agentDebugSection">
        <h4>④ operationGroup 渲染预览（{{ operationGroups.length }} 组）</h4>
        <div v-for="(g, gi) in operationGroups" :key="gi" class="agentDebugGroup">
          <div class="agentDebugGroupHead">
            <span>{{ g.type }}</span>
            <span class="agentDebugChip">toolCallIds: {{ g.toolCallIds.length }}</span>
          </div>
          <div class="agentDebugGroupHint">
            👉 看下面正式 chat 流里的 GroupedOperationMessage 是否真的展开并显示了 MountListCard/FileListCard/FileContentCard
          </div>
        </div>
      </section>

      <!-- ⑤ 自我诊断 -->
      <section class="agentDebugSection">
        <h4>⑤ 自我诊断</h4>
        <ul class="agentDebugDiag">
          <li v-for="(line, i) in diagnostics" :key="i" :class="`agentDebugDiag_${line.level}`">
            <span class="agentDebugDiagLevel">{{ line.level }}</span>
            {{ line.text }}
          </li>
          <li v-if="diagnostics.length === 0" class="agentDebugDiag_empty">
            <span class="agentDebugDiagLevel">empty</span>
            诊断数组为空 = messages / renderedItems 都是 0（数据流没过来）或所有 key 都不触发诊断
          </li>
        </ul>
      </section>

      <!--
        ⑥ 可复制 dump：完整状态文本（messages 详情 + renderedItems type 分布）
        调试按钮：点 Copy 写剪贴板 / 点 Select 选中让用户手动 ⌘+C
        即便 console.error 不可读（移动端 / 自动收起）也能从 UI 直接复制
      -->
      <section class="agentDebugSection">
        <h4>⑥ 可复制 dump</h4>
        <div class="agentDebugActions">
          <button type="button" class="agentDebugBtn" @click="copyDump">
            <ion-icon :icon="copyOutline" />
            <span>{{ copyStatus === 'copied' ? '已复制 ✓' : copyStatus === 'failed' ? '复制失败' : '复制全部' }}</span>
          </button>
          <button type="button" class="agentDebugBtn" @click="selectAllDump">
            <ion-icon :icon="refreshOutline" />
            <span>全选文本（手动 ⌘+C）</span>
          </button>
        </div>
        <textarea
          id="agentDebugDumpTextarea"
          ref="dumpTextarea"
          class="agentDebugDumpText"
          :value="dumpText"
          readonly
          rows="10"
          spellcheck="false"
          @click="selectAllDump"
        ></textarea>
        <p class="agentDebugDumpHint">
          ⓘ 复制后粘给我（包括 agentStatus / messages / renderedItems 状态）；移动端可点「全选文本」后长按复制
        </p>
      </section>

      <!--
        ⑦ 原始 SSE 事件流（实时捕获，不需要复制 console）
        每行：时间戳 | event.type | data 摘要（前 200 字符）
        关键判断：
          - 有 type=tool_call / tool_result → 后端推了（看前端为什么没 append 到 messages）
          - 只有 type=text_delta / stream_start / stream_end → 后端没推 tool_call
      -->
      <section class="agentDebugSection">
        <h4>⑦ 原始 SSE 事件流 ({{ (rawSSEEvents || []).length }} 条)</h4>
        <div class="agentDebugSseStats">
          <span v-for="(c, t) in sseTypeCounts" :key="t" class="agentDebugChip" :class="{ agentDebugChip_emphasis: t === 'tool_call' || t === 'tool_result' }">
            {{ t }}: {{ c }}
          </span>
        </div>
        <textarea
          id="agentDebugSseTextarea"
          ref="sseTextarea"
          class="agentDebugDumpText"
          :value="sseEventText"
          readonly
          rows="12"
          spellcheck="false"
          @click="selectAllSse"
        ></textarea>
        <div class="agentDebugActions">
          <button type="button" class="agentDebugBtn" @click="copySse">
            <ion-icon :icon="copyOutline" />
            <span>{{ sseCopyStatus === 'copied' ? '已复制 ✓' : '复制 SSE 事件流' }}</span>
          </button>
        </div>
        <p class="agentDebugDumpHint">
          ⓘ 这就是后端推给前端的所有 SSE 事件——直接粘给我就能看到有没有 tool_call/tool_result
        </p>
      </section>
    </div>
  </details>
</template>

<script setup lang="ts">
import type { Message, ToolCall } from "@encv/shared-components/composables/useAgent";
import { bugOutline } from "ionicons/icons";
import { computed, onMounted, ref, watch } from "vue";

type RenderedItemLike = { type: string; [k: string]: unknown };

const props = defineProps<{
  messages: Message[];
  /** 由 AgentChat 传入的 renderedItems（任意结构，只读 type 字段） */
  renderedItems: RenderedItemLike[];
  /** 默认是否展开（mock 模式自动展开便于诊断） */
  defaultOpen?: boolean;
  /** 当前 agent status（idle / streaming / confirming / error） */
  agentStatus?: string;
  /**
   * 原始 SSE 事件日志（useAgent.pushRawEvent 追加的数组）。
   * 每条含 { ts, type, dataSummary, seq }。
   * AgentDebugPanel ⑦ 区直接渲染——用户不需要复制 console 了。
   */
  rawSSEEvents?: { ts: string; type: string; dataSummary: string; seq?: number | null }[];
}>();

const _bugIcon = bugOutline;

const _roleCounts = computed<Record<string, number>>(() => {
  const counts: Record<string, number> = {};
  for (const m of props.messages) {
    counts[m.role] = (counts[m.role] ?? 0) + 1;
  }
  return counts;
});

const totalToolCalls = computed(() => props.messages.reduce((sum, m) => sum + m.tool_calls.length, 0));
const totalToolResults = computed(() => props.messages.reduce((sum, m) => sum + m.tool_results.length, 0));

/** 配对率：tool_results 中能找到对应 tool_call id 的比例 */
const pairRateText = computed(() => {
  const calls = new Set<string>();
  for (const m of props.messages) for (const tc of m.tool_calls) calls.add(tc.id);
  if (calls.size === 0) return "n/a";
  const results = new Set<string>();
  for (const m of props.messages) for (const r of m.tool_results) results.add(r.id);
  let paired = 0;
  for (const id of calls) if (results.has(id)) paired++;
  return `${paired}/${calls.size}`;
});

const _renderedTypeCounts = computed<Record<string, number>>(() => {
  const counts: Record<string, number> = {};
  for (const r of props.renderedItems) {
    counts[r.type] = (counts[r.type] ?? 0) + 1;
  }
  return counts;
});

const _recentMessages = computed(() => {
  // 只看含 tool_calls 的最近 3 条 message（核心问题区）
  return props.messages.filter(m => m.tool_calls.length > 0).slice(-3);
});

const operationGroups = computed(() => {
  // narrow：把 type=operationGroup 的元素断言为带 toolCallIds 字段
  return props.renderedItems
    .filter(r => r.type === "operationGroup")
    .map(r => r as unknown as { type: "operationGroup"; toolCallIds: string[] });
});

function _findResult(msg: Message, toolCallId: string): string | null {
  const r = msg.tool_results.find(x => x.id === toolCallId);
  return r ? r.result : null;
}

function _truncate(s: string | null, max: number): string {
  if (!s) return "";
  if (s.length <= max) return s;
  return s.slice(0, max) + `… (+${s.length - max} chars)`;
}

function _resultStatusHint(status: ToolCall["status"]): string {
  if (status === "pending" || status === "running") return "工具还在执行，正常";
  if (status === "success") return "⚠️ 工具声明 success 但 tool_result 事件没到——后端数据丢失";
  if (status === "failed") return "工具失败，等错误回传";
  if (status === "cancelled") return "已取消";
  return "未知状态";
}

// ─── 自我诊断 ────────────────────────────────────────
const _diagnostics = computed<{ level: "ok" | "warn" | "error"; text: string }[]>(() => {
  const out: { level: "ok" | "warn" | "error"; text: string }[] = [];
  if (totalToolCalls.value > 0 && totalToolResults.value === 0) {
    out.push({
      level: "error",
      text: `有 ${totalToolCalls.value} 个 tool_call 但 0 个 tool_result——剧本可能没推 tool_result 事件，或前端没收到`,
    });
  }
  if (totalToolResults.value > 0) {
    const fakeCheck = checkForFakeData();
    if (fakeCheck.length > 0) {
      out.push({
        level: "warn",
        text: `检测到疑似硬编码假数据：${fakeCheck.join("; ")}`,
      });
    } else {
      out.push({ level: "ok", text: "tool_result 数据看起来是真实数据（非 {FAKE:true} / 已知硬编码文件名）" });
    }
  }
  if (operationGroups.value.length === 0 && totalToolCalls.value > 0) {
    out.push({
      level: "error",
      text: "有 tool_call 但 renderedItems 里 0 个 operationGroup——renderTurnItems 没把它们聚成 group，看下面的纯 markdown 渲染就是它导致的",
    });
  }
  if (operationGroups.value.length > 0) {
    out.push({ level: "ok", text: `renderedItems 含 ${operationGroups.value.length} 个 operationGroup` });
  }
  if (pairRateText.value !== "n/a" && pairRateText.value !== "0/0") {
    const [p, t] = pairRateText.value.split("/").map(Number);
    if (p < t) {
      out.push({
        level: "warn",
        text: `配对率 ${p}/${t}——部分 tool_call 没收到 result（可能是分组时漏了）`,
      });
    } else if (p === t && t > 0) {
      out.push({ level: "ok", text: `所有 ${t} 个 tool_call 都配对到 tool_result` });
    }
  }
  return out;
});

function checkForFakeData(): string[] {
  const suspicious: string[] = [];
  for (const m of props.messages) {
    for (const r of m.tool_results) {
      if (r.result.includes('"FAKE":true') || r.result.includes('"FAKE": true')) {
        suspicious.push(`${r.id}: 含 FAKE:true 标记`);
      }
      if (r.result.includes("studio_video_")) {
        suspicious.push(`${r.id}: 含老剧本硬编码 studio_video_* 假文件名`);
      }
    }
  }
  return suspicious;
}

// ─── 实时状态 dump（可复制） ──────────────────────────────
function getMsgText(m: Message): string {
  // content 可能是 string | MessageContentPart[]，dump 时统一转字符串
  if (typeof m.content === "string") return m.content;
  if (Array.isArray(m.content)) {
    return m.content.map(p => (typeof p === "string" ? p : ((p as { text?: string })?.text ?? ""))).join("");
  }
  return "";
}

const dumpText = computed(() => {
  const lines: string[] = [];
  const ts = new Date().toISOString();
  lines.push(`# AgentDebugPanel dump @ ${ts}`);
  lines.push(`agentStatus: ${props.agentStatus ?? "(unset)"}`);
  lines.push(`messages.length: ${props.messages.length}`);
  lines.push(`renderedItems.length: ${props.renderedItems.length}`);
  lines.push("");
  lines.push("--- messages ---");
  props.messages.forEach((m, i) => {
    const text = getMsgText(m).slice(0, 80).replace(/\n/g, "⏎");
    lines.push(`[${i}] role=${m.role} content="${text}" tool_calls=${m.tool_calls.length} tool_results=${m.tool_results.length}`);
    m.tool_calls.forEach(tc => {
      lines.push(`  call  id=${tc.id} name=${tc.name} kind=${tc.kind} status=${tc.status} args=${(tc.args || "").slice(0, 60)}`);
    });
    m.tool_results.forEach(tr => {
      lines.push(
        `  result id=${tr.id} name=${tr.name} status=${tr.status} result="${(tr.result || "").slice(0, 200).replace(/\n/g, "⏎")}"`
      );
    });
  });
  lines.push("");
  lines.push("--- renderedItems (type 分布 + 关键字段) ---");
  const typeCounts: Record<string, number> = {};
  for (const r of props.renderedItems) typeCounts[r.type] = (typeCounts[r.type] ?? 0) + 1;
  lines.push(`typeCounts: ${JSON.stringify(typeCounts)}`);
  props.renderedItems.forEach((r, i) => {
    const keys = Object.keys(r).filter(k => k !== "type");
    const summary = keys
      .map(k => {
        const v = r[k];
        if (Array.isArray(v)) return `${k}=[${v.length}]`;
        if (typeof v === "string") return `${k}="${v.slice(0, 40).replace(/\n/g, "⏎")}"`;
        return `${k}=${JSON.stringify(v)?.slice(0, 40)}`;
      })
      .join(" ");
    lines.push(`[${i}] ${r.type} ${summary}`);
  });
  return lines.join("\n");
});

const copyStatus = ref<"idle" | "copied" | "failed">("idle");

async function _copyDump() {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(dumpText.value);
    } else {
      // fallback：选中文本让用户手动复制
      const ta = document.getElementById("agentDebugDumpTextarea") as HTMLTextAreaElement | null;
      if (ta) {
        ta.select();
        document.execCommand("copy");
        ta.blur();
      }
    }
    copyStatus.value = "copied";
    console.error("[AgentDebugPanel] dump copied:\n" + dumpText.value);
    setTimeout(() => (copyStatus.value = "idle"), 1500);
  } catch (e) {
    copyStatus.value = "failed";
    console.error("[AgentDebugPanel] copy failed:", e);
  }
}

const dumpTextarea = ref<HTMLTextAreaElement | null>(null);
function _selectAllDump() {
  dumpTextarea.value?.select();
}

// ─── ⑦ 区：SSE 事件流 ─────────────────────────────────────
const sseTextarea = ref<HTMLTextAreaElement | null>(null);
const sseCopyStatus = ref<"idle" | "copied" | "failed">("idle");

const _sseTypeCounts = computed<Record<string, number>>(() => {
  const counts: Record<string, number> = {};
  if (!props.rawSSEEvents) return counts;
  for (const ev of props.rawSSEEvents) counts[ev.type] = (counts[ev.type] ?? 0) + 1;
  return counts;
});

const sseEventText = computed(() => {
  if (!props.rawSSEEvents || props.rawSSEEvents.length === 0) return "(无 SSE 事件)";
  return props.rawSSEEvents
    .map(ev => {
      const seqTag = ev.seq != null ? ` [seq=${ev.seq}]` : "";
      return `[${ev.ts}] type=${ev.type}${seqTag} data=${ev.dataSummary}`;
    })
    .join("\n");
});

function _selectAllSse() {
  sseTextarea.value?.select();
}

async function _copySse() {
  try {
    await navigator.clipboard?.writeText(sseEventText.value);
    sseCopyStatus.value = "copied";
    console.error("[AgentDebugPanel] SSE events copied:\n" + sseEventText.value);
    setTimeout(() => (sseCopyStatus.value = "idle"), 1500);
  } catch (e) {
    sseCopyStatus.value = "failed";
    console.error("[AgentDebugPanel] copy SSE failed:", e);
  }
}

// ─── 实时监控：变化时立即 console.error 打印 ──────────────
onMounted(() => {
  console.error(
    "[AgentDebugPanel] mounted — messages=",
    props.messages.length,
    "renderedItems=",
    props.renderedItems.length,
    "agentStatus=",
    props.agentStatus
  );
  console.error("[AgentDebugPanel] initial dump:\n" + dumpText.value);
});

watch(
  () => props.messages.length,
  (newLen, oldLen) => {
    console.error(`[AgentDebugPanel] messages.length: ${oldLen} → ${newLen} (Δ=${newLen - (oldLen ?? 0)})`);
    if (newLen > 0) {
      const last = props.messages[props.messages.length - 1];
      console.error(
        `[AgentDebugPanel] last message: role=${last.role} content="${getMsgText(last).slice(0, 120).replace(/\n/g, "⏎")}" tool_calls=${last.tool_calls.length} tool_results=${last.tool_results.length}`
      );
    }
    console.error("[AgentDebugPanel] dump @ messages change:\n" + dumpText.value);
  }
);

watch(
  () => props.renderedItems.length,
  (newLen, oldLen) => {
    console.error(`[AgentDebugPanel] renderedItems.length: ${oldLen} → ${newLen} (Δ=${newLen - (oldLen ?? 0)})`);
    const types = props.renderedItems.map(r => r.type);
    console.error(`[AgentDebugPanel] renderedItem types: [${types.join(", ")}]`);
  }
);

watch(
  () => props.renderedItems.map(r => r.type).join("|"),
  (newTypes, oldTypes) => {
    if (newTypes === oldTypes) return;
    console.error(`[AgentDebugPanel] renderedItem type set changed: "${oldTypes}" → "${newTypes}"`);
  }
);

watch(
  () => props.agentStatus,
  (newStatus, oldStatus) => {
    console.error(`[AgentDebugPanel] agentStatus: ${oldStatus ?? "?"} → ${newStatus ?? "?"}`);
  }
);
</script>

<style scoped>
.agentDebugPanel {
  margin: 8px 12px 4px;
  border: 1px dashed rgba(var(--ion-color-warning-rgb), 0.5);
  border-radius: 8px;
  background: rgba(var(--ion-color-warning-rgb), 0.05);
  font-size: 11px;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
}

.agentDebugSummary {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  cursor: pointer;
  user-select: none;
  list-style: none;
  outline: none;
  color: var(--ion-color-warning-shade, #b8761e);
}

.agentDebugSummary::-webkit-details-marker {
  display: none;
}

.agentDebugSummary::marker {
  content: '';
}

.agentDebugSummaryIcon {
  font-size: 14px;
}

.agentDebugBadge {
  margin-inline-start: auto;
  padding: 1px 6px;
  border-radius: 8px;
  background: rgba(var(--ion-color-warning-rgb), 0.18);
  color: var(--ion-color-warning-shade, #b8761e);
  font-size: 10px;
  font-weight: 600;
}

.agentDebugBody {
  padding: 6px 10px 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.agentDebugSection {
  border-top: 1px solid rgba(var(--ion-color-medium-rgb), 0.12);
  padding-top: 6px;
}

.agentDebugSection h4 {
  margin: 0 0 4px;
  font-size: 11px;
  font-weight: 700;
  color: var(--ion-text-color);
  text-transform: uppercase;
  letter-spacing: 0.4px;
}

.agentDebugStats {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 3px;
}

.agentDebugChip {
  display: inline-flex;
  align-items: center;
  padding: 1px 6px;
  border-radius: 6px;
  background: rgba(var(--ion-color-medium-rgb), 0.12);
  color: var(--ion-text-color);
  font-size: 10px;
}

.agentDebugChip_emphasis {
  background: rgba(var(--ion-color-primary-rgb), 0.18);
  color: var(--ion-color-primary);
  font-weight: 600;
}

.agentDebugMsg {
  margin-top: 4px;
  padding: 4px 6px;
  background: rgba(var(--ion-color-medium-rgb), 0.06);
  border-radius: 4px;
}

.agentDebugMsgHead {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}

.agentDebugMsgId {
  font-size: 10px;
  color: var(--encv-text-secondary, #888);
}

.agentDebugList {
  list-style: none;
  margin: 4px 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.agentDebugListItem {
  padding: 3px 6px;
  background: rgba(var(--ion-color-medium-rgb), 0.04);
  border-left: 2px solid var(--ion-color-primary);
  border-radius: 0 4px 4px 0;
  font-size: 10.5px;
}

.agentDebugListHead {
  display: flex;
  align-items: center;
  gap: 4px;
  flex-wrap: wrap;
}

.agentDebugName {
  font-weight: 600;
  color: var(--ion-text-color);
}

.agentDebugId {
  font-size: 9.5px;
  color: var(--encv-text-secondary, #888);
}

.agentDebugStatus_pending,
.agentDebugStatus_running {
  background: rgba(var(--ion-color-primary-rgb), 0.18);
  color: var(--ion-color-primary);
}

.agentDebugStatus_success {
  background: rgba(var(--ion-color-success-rgb, 56, 161, 105), 0.18);
  color: var(--ion-color-success, #38a169);
}

.agentDebugStatus_failed,
.agentDebugStatus_cancelled {
  background: rgba(var(--ion-color-danger-rgb), 0.18);
  color: var(--ion-color-danger);
}

.agentDebugResult {
  margin-top: 3px;
  padding: 3px 6px;
  background: rgba(var(--ion-color-medium-rgb), 0.04);
  border-radius: 3px;
  font-size: 10px;
}

.agentDebugResult_missing {
  background: rgba(var(--ion-color-warning-rgb), 0.08);
}

.agentDebugResultTag {
  display: inline-block;
  margin-inline-end: 4px;
  color: var(--ion-color-primary);
  font-weight: 600;
}

.agentDebugResultJson {
  font-size: 9.5px;
  word-break: break-all;
  white-space: pre-wrap;
  color: var(--ion-text-color);
}

.agentDebugHint {
  font-size: 10px;
  color: var(--encv-text-secondary, #888);
}

.agentDebugGroup {
  margin-top: 4px;
  padding: 4px 6px;
  background: rgba(var(--ion-color-primary-rgb), 0.06);
  border-radius: 4px;
}

.agentDebugGroupHead {
  display: flex;
  align-items: center;
  gap: 4px;
  font-weight: 600;
}

.agentDebugGroupHint {
  margin-top: 3px;
  font-size: 10px;
  color: var(--encv-text-secondary, #888);
}

.agentDebugDiag {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.agentDebugDiag li {
  padding: 3px 6px;
  border-radius: 3px;
  font-size: 10.5px;
}

.agentDebugDiag_ok {
  background: rgba(var(--ion-color-success-rgb, 56, 161, 105), 0.1);
  color: var(--ion-color-success, #38a169);
}

.agentDebugDiag_warn {
  background: rgba(var(--ion-color-warning-rgb), 0.12);
  color: var(--ion-color-warning-shade, #b8761e);
}

.agentDebugDiag_error {
  background: rgba(var(--ion-color-danger-rgb), 0.12);
  color: var(--ion-color-danger);
}

.agentDebugDiagLevel {
  display: inline-block;
  margin-inline-end: 4px;
  padding: 0 4px;
  border-radius: 4px;
  background: rgba(0, 0, 0, 0.1);
  font-size: 9px;
  font-weight: 700;
  text-transform: uppercase;
}

.agentDebugDiag_empty {
  background: rgba(var(--ion-color-medium-rgb), 0.12);
  color: var(--encv-text-secondary, #888);
  font-style: italic;
}

.agentDebugActions {
  display: flex;
  gap: 6px;
  margin-bottom: 4px;
  flex-wrap: wrap;
}

.agentDebugBtn {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.3);
  border-radius: 4px;
  background: rgba(var(--ion-color-medium-rgb), 0.08);
  color: var(--ion-text-color);
  font-size: 11px;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  cursor: pointer;
}

.agentDebugBtn:hover {
  background: rgba(var(--ion-color-medium-rgb), 0.14);
}

.agentDebugBtn ion-icon {
  font-size: 13px;
}

.agentDebugDumpText {
  width: 100%;
  box-sizing: border-box;
  font-family: 'SF Mono', Monaco, 'Cascadia Code', monospace;
  font-size: 10.5px;
  line-height: 1.5;
  padding: 8px 10px;
  border: 1px solid rgba(var(--ion-color-medium-rgb), 0.3);
  border-radius: 4px;
  background: rgba(0, 0, 0, 0.25);
  color: #d4d4d4;
  resize: vertical;
  min-height: 180px;
  white-space: pre;
  overflow-x: auto;
  word-break: normal;
  cursor: text;
}

.agentDebugDumpText:focus {
  outline: 1px solid var(--ion-color-primary);
}

.agentDebugDumpHint {
  margin: 4px 0 0;
  font-size: 10px;
  color: var(--encv-text-secondary, #888);
  font-style: italic;
}
</style>
