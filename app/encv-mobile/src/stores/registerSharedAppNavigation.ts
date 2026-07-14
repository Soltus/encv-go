// registerSharedAppNavigation.ts - 在应用启动时把 vue-router 实例桥接为
// 共享抽象层所需的导航能力（@encv/shared-components/runtime/appNavigation）。
// 必须早于任何使用导航能力的运行时调用（Tasks.vue / useTasksView 挂载前）。
import { ref, type Ref } from "vue";
import router from "@/router";
import { setAppNavigation } from "@encv/shared-components/runtime/appNavigation";

export function registerSharedAppNavigation(): void {
  // 初始 query / params：router 创建时即解析了初始位置，currentRoute.value 已可用。
  const query: Ref<Record<string, string | undefined>> = ref((router.currentRoute.value.query as Record<string, string | undefined>) ?? {});
  const params: Ref<Record<string, string | string[] | undefined>> = ref(
    (router.currentRoute.value.params as Record<string, string | string[] | undefined>) ?? {}
  );

  // 导航后同步 query / params，保持响应式。
  router.afterEach(to => {
    query.value = to.query as Record<string, string | undefined>;
    params.value = to.params as Record<string, string | string[] | undefined>;
  });

  setAppNavigation({
    query,
    params,
    navigate: (path: string) => {
      void router.push(path);
    },
    replace: (path: string, q?: Record<string, string>) => {
      void router.replace({ path, query: q });
    },
  });
}
