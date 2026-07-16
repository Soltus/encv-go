/**
 * Kitty 主题运行时装饰钩子（可选 theme.js）——「主题能挂运行时行为/装饰」的证明。
 *
 * themeLoader 激活带 jsUrl 的主题时会动态 import 本模块并调用 mount()；
 * 卸载（unloadTheme / LRU 回收）时调用 unmount()。这让主题不仅能改样式，
 * 还能往界面「贴」DOM 装饰、挂动画、注水印等——Hello Kitty 式皮肤的最后一块拼图。
 *
 * 约束：必须是 ES module，导出 mount()/unmount()；幂等；无 DOM 时安全降级。
 */

const DECOR_ID = "kitty-theme-decor";

export function mount() {
  if (typeof document === "undefined") return;
  if (document.getElementById(DECOR_ID)) return; // 幂等
  const el = document.createElement("div");
  el.id = DECOR_ID;
  el.setAttribute("aria-hidden", "true");
  el.style.cssText = [
    "position:fixed",
    "right:12px",
    "bottom:12px",
    "width:36px",
    "height:31px",
    "z-index:9999",
    "pointer-events:none",
    "background:url('/themes/kitty/assets/sticker.svg') center / contain no-repeat",
    "filter:drop-shadow(0 2px 4px rgba(255,95,162,0.4))",
    "opacity:0.9",
  ].join(";");
  document.body.appendChild(el);
}

export function unmount() {
  if (typeof document === "undefined") return;
  document.getElementById(DECOR_ID)?.remove();
}
