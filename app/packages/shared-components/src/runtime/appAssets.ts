/**
 * appAssets — 应用层 → 共享层的「构建期静态资产」注入 DI 注册点
 *
 * 背景：shared 作为共享层，不得反向依赖应用层（@/...）内部资源，尤其是
 * 构建期由 vite plugin 生成的静态 JSON 资产（如 frontend-deps.json）。但部分
 * 通用模块（如依赖清单 useLibraries）运行时需要这些资产数据。
 *
 * 约定（镜像 nativeBridge 范式）：
 *   - shared 内部一律通过 getAppAssets() / getFrontendDeps() 取用这些资产，
 *     绝不 import @/generated/...。
 *   - app 在启动期（stores/registerSharedAppAssets）调用 setAppAssets(...) 注入具体数据。
 *   - 未注入时返回空结构（web SPA 安全降级，useLibraries 退化为仅 backend 数据）。
 */

export interface FrontendDepsManifest {
  items: Array<{
    name: string;
    version?: string;
    version_range?: string;
    source?: string;
    kind?: string;
    importance?: string;
    description?: string;
    icon?: string;
    license?: string;
  }>;
}

export interface AppAssets {
  /** 构建期生成的 frontend 依赖清单（vite plugin 产物）。 */
  frontendDeps: FrontendDepsManifest;
}

const defaults: AppAssets = {
  frontendDeps: { items: [] },
};

let assets: AppAssets = { ...defaults };

/** app 启动期调用：注入 / 覆盖静态资产。可多次部分覆盖。 */
export function setAppAssets(partial: Partial<AppAssets>): void {
  assets = { ...assets, ...partial };
}

/** shared 内部取用静态资产。 */
export function getAppAssets(): AppAssets {
  return assets;
}

/** 便捷取用 frontend 依赖清单（未注入时返回空 items）。 */
export function getFrontendDeps(): FrontendDepsManifest {
  return assets.frontendDeps;
}
