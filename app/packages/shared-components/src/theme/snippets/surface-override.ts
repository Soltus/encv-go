/* =============================================================================
 * Snippet: 表面类任意覆写（续27 · SiYuan 式自由度演示）
 *
 * 这个片段用【极简选择器】把主色胶囊（.ui-chip）改成「方形 + 红边 + 橙底 +
 * 加粗 + 更大内距」，以证明用户无需改任何组件、无需 !important 即可任意改写外观：
 *
 *   - .ui-chip 是 surface.css 的全局语义类（无 scoped [data-v-x]）；
 *   - 本片段经 useSnippets 在运行时 appendChild 到 <head>，【晚于】打包 CSS；
 *   - 与原规则 specificity 相同（0,1,0）时后加载者胜出 → 直接生效。
 *
 * 这正是思源笔记主题的能力：稳定语义类名 + 普通 CSS 覆盖。我们的令牌层
 * （--text-sm / --pad-chip-y / --density …）还让「只换字/密度」更省事，故等同且更好。
 * ========================================================================== */

export const surfaceOverrideCss = `
/* 演示：将主色胶囊改成方形橙调粗体（任意覆写） */
.ui-chip {
  border-radius: 2px;
  border-color: #ef4444;
  background: #fff7ed;
  color: #9a3412;
  font-weight: 700;
  padding: 6px 14px;
}

/* 演示：徽章也一并改写，证明整组表面类均可达 */
.ui-badge {
  border-radius: 4px;
  letter-spacing: 0.04em;
}
`;
