/* =============================================================================
 * Snippet: 舒适对比度（Phase 5 · 局部 CSS 覆盖示例）
 *
 * 这是一个「CSS 片段」的样例。片段是可热开关的局部覆盖：运行时注入到 <head>
 * 的 <style data-encv-snippet="id"> 中，关闭时移除。
 *
 * 真生产环境的片段应来自 `user-themes/<id>/theme.css` 或远程清单（见
 * THEME_DEV.md），此处内联为字符串以便类型安全、无需 ?raw 模块声明，
 * 但仍经 useSnippets.ts 的同一注入/持久化通道生效。
 * ========================================================================== */

export const comfyContrastCss = `
/* 舒适对比度：提升正文/背景对比，缓解刺眼 */
:root {
  --color-base-content: #111827;
  --color-base-100: #ffffff;
  --color-base-200: #f1f5f9;
  --color-base-300: #cbd5e1;
}
[data-theme="encv-dark"] {
  --color-base-content: #f8fafc;
}
`;
