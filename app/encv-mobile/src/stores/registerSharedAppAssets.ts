/**
 * 注册共享层「构建期静态资产」：注入 frontend-deps.json
 *
 * 该 JSON 由 vite plugin 在构建期生成于 src/generated/frontend-deps.json，
 * 属应用层资源；通过 setAppAssets 注入共享层 appAssets DI，避免 shared 反向依赖 @/generated。
 */
import frontendDeps from "@/generated/frontend-deps.json";
import { setAppAssets } from "@encv/shared-components/runtime/appAssets";

export function registerSharedAppAssets(): void {
  setAppAssets({ frontendDeps: frontendDeps as unknown as import("@encv/shared-components/runtime/appAssets").FrontendDepsManifest });
}
