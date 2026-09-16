import type { SimverseNPCDetail } from "@/composables/useSimverse";

// P14 流派系统：基于 NPC 既有属性（职业/大五人格/价值观/兴趣/阶层/等级）
// 确定性派生"流派定位"。结果稳定可复现（同输入同输出），无需后端即可在端上计算。

export type ArchetypeKey =
  | "warrior"
  | "guardian"
  | "scholar"
  | "merchant"
  | "artisan"
  | "healer"
  | "leader"
  | "hermit"
  | "rogue"
  | "artist";

export interface SimverseBuild {
  primary: ArchetypeKey;
  tags: ArchetypeKey[]; // primary + 最多 2 个次要流派
  synergy: number; // 流派契合度 1-5（主流派得分映射）
  scores: Record<ArchetypeKey, number>;
}

export const ARCHETYPES: ArchetypeKey[] = [
  "warrior",
  "guardian",
  "scholar",
  "merchant",
  "artisan",
  "healer",
  "leader",
  "hermit",
  "rogue",
  "artist",
];

// 兼容 0-1 与 0-100 两种量纲
function norm(v: number | undefined): number {
  if (v === undefined || v === null) return 0;
  return v > 1 ? v / 100 : v;
}

// 职业 → 基础流派权重
const PROF_MAP: Record<string, Partial<Record<ArchetypeKey, number>>> = {
  warrior: { warrior: 3, guardian: 1 },
  soldier: { warrior: 3 },
  guard: { guardian: 3 },
  knight: { guardian: 2, warrior: 1 },
  mage: { scholar: 2, artisan: 1 },
  wizard: { scholar: 3 },
  scholar: { scholar: 3 },
  merchant: { merchant: 3 },
  trader: { merchant: 3 },
  priest: { healer: 2, leader: 1 },
  cleric: { healer: 3 },
  farmer: { artisan: 2, hermit: 1 },
  craftsman: { artisan: 3 },
  noble: { leader: 3, merchant: 1 },
  lord: { leader: 3 },
  artist: { artist: 3 },
  rogue: { rogue: 3, warrior: 1 },
  thief: { rogue: 3 },
  hermit: { hermit: 3 },
};

// 文本关键词 → 流派（用于 values / interests）
const VALUE_KEYWORDS: [RegExp, ArchetypeKey, number][] = [
  [/knowledge|learn|wisdom|study|research|书|学|智/i, "scholar", 2],
  [/art|beauty|music|create|paint|艺|美|创/i, "artist", 2],
  [/wealth|money|trade|commerce|gold|商|富|金/i, "merchant", 2],
  [/power|lead|command|rule|权|领|统/i, "leader", 2],
  [/family|love|community|heal|care|家|爱|愈|护/i, "healer", 2],
  [/duty|order|law|protect|guard|责|序|守|律/i, "guardian", 2],
  [/craft|build|make|forge|匠|造|工/i, "artisan", 2],
  [/freedom|solitude|lonely|peace|隐|独|静|由/i, "hermit", 2],
  [/fight|war|battle|combat|战|斗|武/i, "warrior", 2],
  [/adventure|explore|risk|wand|险|探|游/i, "rogue", 2],
];

export function deriveNPCBuild(npc: SimverseNPCDetail): SimverseBuild {
  const scores = {} as Record<ArchetypeKey, number>;
  ARCHETYPES.forEach(a => (scores[a] = 0));
  const add = (k: ArchetypeKey, p: number) => {
    scores[k] += p;
  };

  // 1) 职业基础
  const prof = (npc.profession || "").toLowerCase();
  const pm = PROF_MAP[prof];
  if (pm) {
    for (const [k, v] of Object.entries(pm)) add(k as ArchetypeKey, v as number);
  } else {
    add("rogue", 1); // 未知职业的兜底
  }

  // 2) 大五人格
  const bf = npc.big_five || {};
  add("scholar", norm(bf.openness) * 2);
  add("artist", norm(bf.openness) * 1);
  add("guardian", norm(bf.conscientiousness) * 2);
  add("artisan", norm(bf.conscientiousness) * 1);
  add("leader", norm(bf.extraversion) * 2);
  add("merchant", norm(bf.extraversion) * 1);
  add("healer", norm(bf.agreeableness) * 2);
  add("leader", norm(bf.agreeableness) * 0.5);
  add("hermit", norm(bf.neuroticism) * 1);
  add("rogue", norm(bf.neuroticism) * 1);

  // 3) 价值观 / 兴趣 关键词
  const texts = [
    ...(npc.top_values || []),
    ...(npc.top_interests || []),
    ...Object.keys(npc.values || {}),
    ...Object.keys(npc.interests || {}),
  ];
  for (const txt of texts) {
    for (const [re, key, pts] of VALUE_KEYWORDS) {
      if (re.test(String(txt))) add(key, pts);
    }
  }

  // 4) 阶层 / 等级 微调
  add("merchant", (npc.wealth_tier || 0) * 0.3);
  add("leader", (npc.social_tier || 0) * 0.3);
  add("warrior", (npc.level || 0) / 40);
  add("guardian", (npc.level || 0) / 60);

  // 5) 排序（得分降序；并列时按流派名稳定排序，保证可复现）
  const ranked = [...ARCHETYPES].sort((a, b) => {
    const d = scores[b] - scores[a];
    if (Math.abs(d) > 1e-6) return d;
    return a.localeCompare(b);
  });

  const primary = ranked[0];
  const tags: ArchetypeKey[] = [primary];
  for (const a of ranked.slice(1, 3)) {
    if (scores[a] > 0) tags.push(a);
  }
  const synergy = Math.max(1, Math.min(5, Math.round(scores[primary] / 2.5)));

  return { primary, tags, synergy, scores };
}

// 轻量流派派生：仅用列表级字段（职业/等级/阶层），用于编队候选快速判定，
// 与 deriveNPCBuild 共享同一套职业权重与流派枚举，结果方向一致。
export interface NPCSummary {
  profession?: string;
  level?: number;
  wealth_tier?: number;
  social_tier?: number;
}

// 流派展示元数据：供 Phaser 横屏世界与 Ionic 数据视图共用，保证配色/图标一致。
export interface ArchetypeMeta {
  key: ArchetypeKey;
  labelKey: string; // i18n key: simverse.build.<key>
  name: string; // 中文展示名（Phaser 图例用，免依赖 i18n）
  color: number; // Phaser 数值色
  colorCss: string; // CSS 十六进制
  emoji: string;
}

export const ARCH_META: Record<ArchetypeKey, ArchetypeMeta> = {
  warrior: { key: "warrior", labelKey: "simverse.build.warrior", name: "战士", color: 0xef4444, colorCss: "#ef4444", emoji: "🗡️" },
  guardian: { key: "guardian", labelKey: "simverse.build.guardian", name: "守护", color: 0xf59e0b, colorCss: "#f59e0b", emoji: "🛡️" },
  scholar: { key: "scholar", labelKey: "simverse.build.scholar", name: "学者", color: 0x3b82f6, colorCss: "#3b82f6", emoji: "📜" },
  merchant: { key: "merchant", labelKey: "simverse.build.merchant", name: "商人", color: 0x22c55e, colorCss: "#22c55e", emoji: "💰" },
  artisan: { key: "artisan", labelKey: "simverse.build.artisan", name: "工匠", color: 0xa855f7, colorCss: "#a855f7", emoji: "🔨" },
  healer: { key: "healer", labelKey: "simverse.build.healer", name: "治疗", color: 0xec4899, colorCss: "#ec4899", emoji: "⚕️" },
  leader: { key: "leader", labelKey: "simverse.build.leader", name: "领袖", color: 0x6366f1, colorCss: "#6366f1", emoji: "👑" },
  hermit: { key: "hermit", labelKey: "simverse.build.hermit", name: "隐士", color: 0x64748b, colorCss: "#64748b", emoji: "🌙" },
  rogue: { key: "rogue", labelKey: "simverse.build.rogue", name: "游侠", color: 0x1f2937, colorCss: "#1f2937", emoji: "🥷" },
  artist: { key: "artist", labelKey: "simverse.build.artist", name: "艺术家", color: 0x14b8a6, colorCss: "#14b8a6", emoji: "🎨" },
};

export function deriveBuildFromNPC(npc: NPCSummary): { primary: ArchetypeKey; tags: ArchetypeKey[]; synergy: number } {
  const scores = {} as Record<ArchetypeKey, number>;
  ARCHETYPES.forEach(a => (scores[a] = 0));
  const add = (k: ArchetypeKey, p: number) => {
    scores[k] += p;
  };

  const prof = (npc.profession || "").toLowerCase();
  const pm = PROF_MAP[prof];
  if (pm) {
    for (const [k, v] of Object.entries(pm)) add(k as ArchetypeKey, v as number);
  } else {
    add("rogue", 1);
  }
  add("merchant", (npc.wealth_tier || 0) * 0.3);
  add("leader", (npc.social_tier || 0) * 0.3);
  add("warrior", (npc.level || 0) / 40);
  add("guardian", (npc.level || 0) / 60);

  const ranked = [...ARCHETYPES].sort((a, b) => {
    const d = scores[b] - scores[a];
    if (Math.abs(d) > 1e-6) return d;
    return a.localeCompare(b);
  });
  const primary = ranked[0];
  const tags: ArchetypeKey[] = [primary];
  for (const a of ranked.slice(1, 3)) {
    if (scores[a] > 0) tags.push(a);
  }
  const synergy = Math.max(1, Math.min(5, Math.round(scores[primary] / 2.5)));
  return { primary, tags, synergy };
}
