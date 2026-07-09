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
  "warrior", "guardian", "scholar", "merchant",
  "artisan", "healer", "leader", "hermit", "rogue", "artist",
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
  ARCHETYPES.forEach((a) => (scores[a] = 0));
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
