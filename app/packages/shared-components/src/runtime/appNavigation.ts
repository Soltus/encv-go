/**
 * appNavigation — 应用层 → 共享层的「导航能力」注入 DI 注册点
 *
 * 背景：shared 作为共享层，不得反向依赖应用层（@/...）内部实现，尤其是
 * vue-router（页面路由是应用层专属上下文）。但部分通用视图（如任务列表页
 * Tasks.vue / useTasksView）运行时需要读取路由 query（debug / action / source /
 * type）与编程式导航（跳转到组详情、清空 query）。
 *
 * 约定：
 *   - shared 内部一律通过 getAppNavigation() 取用导航能力，绝不 import vue-router / @/。
 *   - app 在启动期（main.ts）调用 setAppNavigation(...) 注入具体实现（基于 vue-router 实例）。
 *   - 未注入时使用安全默认：navigate / replace 抛清晰错误，便于尽早暴露「忘记注入」的问题。
 */

import { ref, type Ref } from "vue";

export interface AppNavigation {
  /** 当前路由 query（响应式，随导航更新）。供 computed / watch 使用。 */
  query: Readonly<Ref<Record<string, string | undefined>>>;
  /** 当前路由 params（响应式，随导航更新）。供 computed / watch 使用（如 runId）。 */
  params: Readonly<Ref<Record<string, string | string[] | undefined>>>;
  /** 编程式导航（push）。未注入时抛错提示。 */
  navigate: (path: string) => void;
  /** 替换当前路由（replace），可带新 query。未注入时抛错提示。 */
  replace: (path: string, query?: Record<string, string>) => void;
}

const defaultQuery = ref<Record<string, string | undefined>>({});
const defaultParams = ref<Record<string, string | string[] | undefined>>({});

const defaults: AppNavigation = {
  query: defaultQuery,
  params: defaultParams,
  navigate: () => {
    throw new Error("[appNavigation] navigate 未注入（需在 app 启动期调用 setAppNavigation）");
  },
  replace: () => {
    throw new Error("[appNavigation] replace 未注入（需在 app 启动期调用 setAppNavigation）");
  },
};

let navigation: AppNavigation = { ...defaults };

/** app 启动期调用：注入 / 覆盖导航能力。可多次部分覆盖。 */
export function setAppNavigation(partial: Partial<AppNavigation>): void {
  navigation = { ...navigation, ...partial };
}

/** shared 内部取用导航能力。 */
export function getAppNavigation(): AppNavigation {
  return navigation;
}
