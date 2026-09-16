/**
 * themeStorage —— 主题【存储端口】（续43 修订：框架与后端解耦，本地优先由 Go 后端落地）。
 *
 * 初版 themeStore 把「云拉取下载 + 主题字节落地」直接写进 shared-components 框架，
 * 且错用 @capacitor/filesystem（本工程后端是 Go，主题「本地同一目录」= Go 后端托管的
 * 同源 /themes/<id>/，不是设备文件系统）；同时把【应用层职责】——「字节下载到哪、怎么存」——
 * 耦合进了框架，违背「避免框架与应用耦合」。
 *
 * 修订：框架只定义【端口（抽象）ThemeStorage】，具体实现由【应用】注入
 * （encv-mobile 提供 Go 后端适配器 registerSharedThemeStorage，走 fetch + getAgentApiBase）。
 *
 * 本地优先语义（Go 后端视角，前端永远从同源 /themes/<id>/ 加载）：
 *   - 内置主题：已在 Go 后端本地 themes/ 目录，同源 /themes/<id>/theme.css 直出。
 *   - 云拉取主题：前端安装时调 pullToLocal → Go 后端把远程主题下载到【同一目录】
 *     themes/<id>/（与内置同形态、同加载机制）。前端不热链远程 CDN —— 同源即本地优先、
 *     离线可用、与内置零差异。
 *
 * 框架默认提供 sameOriginThemeStorage：不下载、不存字节（假定后端已就绪 / 单测用），
 * 保证 shared-components 在缺省与测试环境下自洽运行，零后端依赖、零 Capacitor 依赖。
 */
import type { ThemeManifest } from "./themeLoader";

/** pullToLocal 入参：告诉后端去哪拉、拉成什么形态。 */
export interface ThemePullRequest {
  /** 目标主题 id（落盘到 themes/<id>/）。 */
  id: string;
  /** 远程来源（文件夹 / theme.json / 裸 .css 直链）—— 后端据此下载。 */
  sourceUrl: string;
  /** 已解析的清单（含 id / name / css / js 元信息，供后端镜像目录结构）。 */
  manifest: ThemeManifest;
  /** 裸 .css 直链回退：仅下载单个 css，无 assets / theme.js。 */
  cssOnly?: boolean;
}

/**
 * 主题存储端口：框架只依赖此抽象，绝不感知后端是 Go 还是别的、字节存哪。
 * 由应用（encv-mobile）在启动期 setThemeStorage(...) 注入具体实现。
 */
export interface ThemeStorage {
  /** 把远程主题拉取到后端【本地同一目录】themes/<id>/（云拉取落地本地，本地优先）。 */
  pullToLocal(req: ThemePullRequest): Promise<void>;
  /** 从后端本地目录删除（卸载主题）。 */
  removeLocal(id: string): Promise<void>;
}

/** 默认实现：同源假定，不下载（单测 / 后端已就绪场景）。 */
export const sameOriginThemeStorage: ThemeStorage = {
  async pullToLocal() {},
  async removeLocal() {},
};

let active: ThemeStorage = sameOriginThemeStorage;

/** 应用启动期注入具体 ThemeStorage（如 Go 后端适配器）。可多次部分覆盖（同 setApiProxy 范式）。 */
export function setThemeStorage(storage: ThemeStorage): void {
  active = storage;
}

/** 框架内部取用当前 ThemeStorage（默认同源无下载）。 */
export function getThemeStorage(): ThemeStorage {
  return active;
}
