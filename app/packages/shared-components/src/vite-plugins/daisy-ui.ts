import tailwindcss from "@tailwindcss/vite";
import type { Plugin } from "vite";

/**
 * 统筹工作区 Tailwind v4 + daisyUI 接入点。
 *
 * 各插件 vite 配置统一引入此函数，确保 daisyUI / Tailwind 配置一致：
 *
 *   import { daisyUiPlugin } from "@encv/shared-components/vite-plugins/daisy-ui";
 *   plugins: [daisyUiPlugin(), ...]
 *
 * 真正的样式入口是 @encv/shared-components/styles/daisyui.css，
 * 需在应用 main.ts 中 import（见该文件注释）。
 */
export function daisyUiPlugin(): Plugin[] {
  return tailwindcss();
}
