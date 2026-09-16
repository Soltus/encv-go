/**
 * useSlashMenu - Composer textarea "/" 命令面板
 *
 * 触发条件：textarea 内容以 `/` 开头且后跟非空白字符（`/^\s*\/\S*$/`）。
 * 菜单分组两类：
 *   - 功能（静态）：attach / plan-mode / permission-mode 共 3 项
 *   - 技能（动态）：从后端 /api/skills 拉取，组件 mount 时拉一次后缓存
 *
 * 设计要点：
 * 1. 技能数据仅在首次 mount 时拉取一次（spec 约束），不每开菜单都 fetch
 * 2. 拉取失败时静默降级为"仅功能项"——网络/后端不可用不阻塞 UI
 * 3. query 过滤是大小写不敏感的子串匹配，匹配 label / id / description
 * 4. selectedIndex 在 query 变化或菜单关闭时归零（避免错位）
 * 5. apply() 调用后自动 closeMenu()——无论子项是否实现 apply
 *
 * 集成方式：AgentChat 的 textarea @input / @keydown 转发到本 composable；
 * SlashMenu.vue 作为受控组件接收 items + selectedIndex + onApply + onClose。
 */

import { attachOutline, ribbonOutline, shieldCheckmarkOutline, sparklesOutline } from "ionicons/icons";
import { computed, onMounted, ref, watch } from "vue";
import { getAgentApiBase } from "@encv/shared-components/composables/useAgentApiBase";

export type SlashMenuGroup = "功能" | "技能";

export interface SlashMenuItem {
  id: string;
  group: SlashMenuGroup;
  label: string;
  description: string;
  /**
   * ionicons 图标引用（来自 ionicons/icons 的导出变量），如 attachOutline。
   * 这里使用类型 any 是因为 ionicons 的 SVGIcon 内部类型不在 d.ts 中暴露
   * 完整签名，运行时只是 svg path data。
   */
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  icon: any;
  /** 用户选中后触发的副作用；外部不感知返回值，apply 完毕后自动 closeMenu */
  apply: () => void;
}

export interface UseSlashMenuOptions {
  /** "添加附件" 功能项 apply 时触发（外部决定具体行为） */
  onAttach?: () => void;
  /** "Plan 模式" 功能项 apply 时触发 */
  onTogglePlanMode?: () => void;
  /** "权限模式" 功能项 apply 时触发 */
  onTogglePermissionMode?: () => void;
  /** 用户选中某条 skill 时触发（id = skill 的标识） */
  onSelectSkill?: (id: string, label: string) => void;
}

/** 静态功能项配置（声明式，方便测试断言和未来扩展） */
function buildFeatureItems(opts: UseSlashMenuOptions): SlashMenuItem[] {
  return [
    {
      id: "attach",
      group: "功能",
      label: "添加附件",
      description: "附加图片或文件供 AI 引用",
      icon: attachOutline,
      apply: () => {
        opts.onAttach?.();
      },
    },
    {
      id: "plan-mode",
      group: "功能",
      label: "Plan 模式",
      description: "让 AI 先拆解步骤再执行",
      icon: ribbonOutline,
      apply: () => {
        opts.onTogglePlanMode?.();
      },
    },
    {
      id: "permission-mode",
      group: "功能",
      label: "权限模式",
      description: "切换 default / auto-review / full-access",
      icon: shieldCheckmarkOutline,
      apply: () => {
        opts.onTogglePermissionMode?.();
      },
    },
  ];
}

export interface SkillRecord {
  id: string;
  name: string;
  description?: string;
}

export function useSlashMenu(opts: UseSlashMenuOptions = {}) {
  const isOpen = ref(false);
  /** 当前过滤关键字（去掉前导 `/` 后的子串） */
  const query = ref("");
  /** 当前高亮项在 filteredItems 中的索引（0-based） */
  const selectedIndex = ref(0);
  /** 后端拉取到的技能列表（mount 时拉一次，失败时为空） */
  const skillItems = ref<SkillRecord[]>([]);
  /** 防止 mount 后重复拉取 /api/skills */
  const skillsLoaded = ref(false);
  /** 标记是否正在拉取中（防并发） */
  const skillsLoading = ref(false);

  // ── 派生数据 ──────────────────────────────────────────────────────

  /**
   * 静态功能项（不变）。
   * 使用 computed 包装是给未来的"动态启用/禁用"功能预留扩展点
   * （例如"权限模式"在某些部署下不可用时整组不显示）。
   */
  const featureItems = computed<SlashMenuItem[]>(() => buildFeatureItems(opts));

  /**
   * 技能项（来自后端 /api/skills）。空数组 = 后端不可用，仅显示功能项。
   */
  const skillMenuItems = computed<SlashMenuItem[]>(() =>
    skillItems.value.map(s => ({
      id: `skill:${s.id}`,
      group: "技能",
      label: s.name,
      description: s.description || "",
      icon: sparklesOutline,
      apply: () => {
        opts.onSelectSkill?.(s.id, s.name);
      },
    }))
  );

  /** 全量项（功能 + 技能） */
  const allItems = computed<SlashMenuItem[]>(() => [...featureItems.value, ...skillMenuItems.value]);

  /**
   * 当前展示项（按 query 过滤）。空 query 时返回全量。
   * 匹配规则：label / id / description 任一字段大小写不敏感子串命中。
   */
  const items = computed<SlashMenuItem[]>(() => {
    const q = query.value.trim().toLowerCase();
    if (!q) return allItems.value;
    return allItems.value.filter(it => {
      return it.label.toLowerCase().includes(q) || it.id.toLowerCase().includes(q) || it.description.toLowerCase().includes(q);
    });
  });

  // 当 items 长度变化时归零 selectedIndex（防止越界）
  watch(
    () => items.value.length,
    len => {
      if (len === 0) {
        selectedIndex.value = 0;
      } else if (selectedIndex.value >= len) {
        selectedIndex.value = len - 1;
      }
    }
  );

  // ── 状态变更 ──────────────────────────────────────────────────────

  function openMenu(q: string) {
    query.value = q;
    isOpen.value = true;
    selectedIndex.value = 0;
  }

  function closeMenu() {
    isOpen.value = false;
    query.value = "";
    selectedIndex.value = 0;
  }

  /**
   * 处理 textarea @input：根据文本是否匹配触发条件决定开关
   * 触发条件：`/^\s*\/\S*$/` —— 允许 `/`、`/foo` 等形式，阻止 `/foo bar`
   * 提取匹配组作为 query（去掉前导 `/`）。
   */
  function handleInput(text: string) {
    if (/^\s*\/\S*$/.test(text)) {
      const m = text.match(/^\s*\/(.*)$/);
      openMenu(m ? m[1] : "");
    } else {
      if (isOpen.value) closeMenu();
    }
  }

  function moveSelection(delta: number) {
    const len = items.value.length;
    if (len === 0) return;
    const next = selectedIndex.value + delta;
    selectedIndex.value = ((next % len) + len) % len;
  }

  function applySelected() {
    const it = items.value[selectedIndex.value];
    if (it) {
      it.apply();
      closeMenu();
    }
  }

  function applyById(id: string) {
    const it = items.value.find(x => x.id === id);
    if (it) {
      it.apply();
      closeMenu();
    }
  }

  /**
   * textarea @keydown 入口。返回 true 表示已处理（调用方可继续其他逻辑），
   * 返回 false 表示未拦截（让 textarea 走默认行为）。
   *
   * 拦截规则：仅在菜单打开时拦截 ArrowUp/Down/Enter/Escape。
   * 其他键（Tab、字符键、Backspace 等）一律放行给 textarea。
   */
  function handleKeydown(e: KeyboardEvent): boolean {
    if (!isOpen.value) return false;
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        moveSelection(1);
        return true;
      case "ArrowUp":
        e.preventDefault();
        moveSelection(-1);
        return true;
      case "Enter":
        e.preventDefault();
        applySelected();
        return true;
      case "Escape":
        e.preventDefault();
        closeMenu();
        return true;
      default:
        return false;
    }
  }

  /**
   * mount 时拉一次 /api/skills。失败时静默降级为仅功能项。
   * 拉取成功 / 失败都会标记 skillsLoaded，确保只拉一次。
   */
  async function loadSkillsOnce() {
    if (skillsLoaded.value || skillsLoading.value) return;
    skillsLoading.value = true;
    try {
      const res = await fetch(`${getAgentApiBase()}/api/skills`);
      if (!res.ok) {
        // 404 / 500 等：不报错，菜单仍可用（仅功能项）
        return;
      }
      const data = await res.json();
      const rawList: unknown[] = Array.isArray(data)
        ? data
        : Array.isArray((data as { skills?: unknown[] })?.skills)
          ? (data as { skills: unknown[] }).skills
          : [];
      const normalized: SkillRecord[] = rawList
        .filter((s): s is Record<string, unknown> => !!s && typeof s === "object")
        .map(s => ({
          id: String((s.id as string | undefined) || (s.name as string | undefined) || ""),
          name: String((s.name as string | undefined) || (s.id as string | undefined) || ""),
          description: typeof s.description === "string" ? s.description : "",
        }))
        .filter(s => !!s.id);
      skillItems.value = normalized;
    } catch (e) {
      // 网络失败 / CORS / JSON 解析失败：静默降级
      console.debug("[useSlashMenu] loadSkillsOnce failed:", e);
    } finally {
      skillsLoading.value = false;
      skillsLoaded.value = true;
    }
  }

  /**
   * 自动在 composable 创建时触发拉取。
   * 留 onMounted 钩子是为了在 SSR / 测试环境跳过副作用——
   * 直接在工厂函数里 await 会污染所有调用方。
   */
  onMounted(() => {
    void loadSkillsOnce();
  });

  return {
    isOpen,
    query,
    items,
    selectedIndex,
    skillItems,
    openMenu,
    closeMenu,
    handleInput,
    handleKeydown,
    applyById,
    applySelected,
    moveSelection,
    loadSkillsOnce,
  };
}
