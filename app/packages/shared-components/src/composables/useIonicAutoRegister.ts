import type { App } from "vue";
import * as IonicVueComponents from "@ionic/vue";

const ION_PREFIX = "Ion";

export interface IonicRegisterOptions {
  /**
   * 额外要注册的组件（来自其他包）
   */
  extraComponents?: Record<string, any>;
  /**
   * 是否跳过某些组件（按名称，不含 Ion 前缀）
   */
  skip?: string[];
}

function isIonComponent(name: string): boolean {
  return name.startsWith(ION_PREFIX) && name.length > ION_PREFIX.length;
}

function kebabToPascal(kebab: string): string {
  return kebab
    .split("-")
    .map(part => part.charAt(0).toUpperCase() + part.slice(1))
    .join("");
}

function pascalToKebab(pascal: string): string {
  return pascal
    .replace(/([A-Z])/g, "-$1")
    .toLowerCase()
    .replace(/^-/, "");
}

/**
 * 自动注册所有 @ionic/vue 导出的 Vue 组件。
 *
 * 为什么需要这个？
 * =================
 * @ionic/vue 的 IonicVue 插件在 CE（Custom Elements）构建模式下，
 * install() 只做两件事：
 *   1. 给 document.documentElement 加 ion-ce class
 *   2. 调用 initialize() 初始化 Stencil Web Components
 *
 * 它**不全局注册**任何 Vue 组件（IonApp、IonTabs、IonRouterOutlet 等）。
 * 这会导致：
 *   - Vue 模板里的 <ion-app> 等标签被当作"未知组件"处理
 *   - 报 "Failed to resolve component: ion-xxx" 警告
 *   - 页面空白（组件没有正确渲染为 Vue 受控组件）
 *
 * 历史背景：
 *   - Ionic Vue v4/v5 时代：IonicVue 插件会全局注册所有组件
 *   - Ionic Vue v6+：引入 CE 模式，组件改为按需导入
 *   - 但很多项目（包括本项目）依赖全局可用的 Ionic 组件
 *
 * 本函数扫描 @ionic/vue 的所有导出，找出所有以 Ion 开头的组件，
 * 并将它们全局注册到 Vue app 实例上。
 */
export function registerIonicComponents(
  app: App,
  options: IonicRegisterOptions = {},
): { registered: string[]; skipped: string[] } {
  const { extraComponents = {}, skip = [] } = options;

  const registered: string[] = [];
  const skipped: string[] = [];

  const skipSet = new Set(skip.map(name => {
    if (name.startsWith(ION_PREFIX)) return name;
    return ION_PREFIX + name.charAt(0).toUpperCase() + name.slice(1);
  }));

  for (const [name, component] of Object.entries(IonicVueComponents)) {
    if (!isIonComponent(name)) continue;
    if (skipSet.has(name)) {
      skipped.push(name);
      continue;
    }
    if (typeof component !== "object" && typeof component !== "function") {
      continue;
    }
    app.component(name, component as any);
    registered.push(name);
  }

  for (const [name, component] of Object.entries(extraComponents)) {
    const displayName = isIonComponent(name) ? name : ION_PREFIX + name;
    app.component(displayName, component);
    registered.push(displayName);
  }

  return { registered, skipped };
}

export function getIonicComponentNames(): string[] {
  return Object.keys(IonicVueComponents).filter(isIonComponent).sort();
}

export { pascalToKebab, kebabToPascal };

export default registerIonicComponents;
