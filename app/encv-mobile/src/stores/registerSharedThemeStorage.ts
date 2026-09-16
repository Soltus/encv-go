/**
 * registerSharedThemeStorage —— 把【主题存储端口】的具体实现注入共享层。
 *
 * 续43 修订：主题「本地同一目录」由 Go 后端托管（同源 /themes/<id>/），云端主题由后端
 * 拉取到该目录。这里提供 ThemeStorage 的 Go 后端适配器，沿用 getAgentApiBase() 三态拼装
 * （dev 网关 /agent-api、native 相对路径、web SPA 绝对 URL），与项目既有的 apiProxy
 * 解耦范式一致。shared-components 不感知 Go，只依赖注入的端口。
 *
 * 注意：本文件属【应用层】，可 import @/... 与 @capacitor/*；shared-components 内禁止反向依赖。
 */
import { setThemeStorage, type ThemeStorage } from "@encv/shared-components/theme/themeStorage";
import { getAgentApiBase } from "@encv/shared-components/composables/useAgentApiBase";

const goThemeStorage: ThemeStorage = {
  async pullToLocal(req) {
    const base = getAgentApiBase();
    const res = await fetch(`${base}/api/themes/pull`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        id: req.id,
        sourceUrl: req.sourceUrl,
        manifest: req.manifest,
        cssOnly: req.cssOnly ?? false,
      }),
    });
    if (!res.ok) {
      throw new Error(`[themeStorage] 云端主题拉取失败（${res.status}）：${req.sourceUrl}`);
    }
  },
  async removeLocal(id) {
    const base = getAgentApiBase();
    const res = await fetch(`${base}/api/themes/${encodeURIComponent(id)}`, { method: "DELETE" });
    if (!res.ok) {
      throw new Error(`[themeStorage] 本地主题删除失败（${res.status}）：${id}`);
    }
  },
};

/** 在 app 启动期调用：把 Go 后端适配器注入共享层主题存储端口。 */
export function registerSharedThemeStorage(): void {
  setThemeStorage(goThemeStorage);
}
