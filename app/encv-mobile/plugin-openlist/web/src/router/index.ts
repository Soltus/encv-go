import { createRouter, createWebHashHistory } from "@ionic/vue-router";
import type { RouteRecordRaw } from "vue-router";
import BackToMain from "@/views/BackToMain.vue";
import NotFoundView from "@/views/NotFoundView.vue";
import OpenListConfigEditor from "@/views/OpenListConfigEditor.vue";
import OpenListHome from "@/views/OpenListHome.vue";
import OpenListSettings from "@/views/OpenListSettings.vue";
import OpenListWebView from "@/views/OpenListWebView.vue";

export const routes: RouteRecordRaw[] = [
  { path: "/", redirect: "/home" },
  { path: "/home", component: OpenListHome },
  { path: "/config", component: OpenListConfigEditor },
  { path: "/settings", component: OpenListSettings },
  { path: "/webview", component: OpenListWebView },
  // "返回 ENCV 主页面"视图：内嵌全屏 iframe 加载 encv-mobile :5173，
  // 绕过 Trae 沙箱 OpenPreview 工具「单 port 限制」(trae_web_sandbox_network.md §8.4)
  { path: "/back-to-main", component: BackToMain },
  // Catch-all 404 路由（防御性 UI：开发期任何路径不匹配都显示清晰提示而不是空白）
  {
    path: "/:pathMatch(.*)*",
    name: "not-found",
    component: NotFoundView,
  },
];

/**
 * 必须用 hash 模式（createWebHashHistory）！
 * 原因：Android WebView 通过 file:///android_asset/openlist/index.html 加载
 * - file:// 协议不支持 history.pushState
 * - 即使支持，刷新非根路径也会 404（无服务端路由）
 * - hash 模式（#/home）天然兼容 file:// + 刷新友好
 *
 * 必须传 base='/openlist-ui/'：
 * - 沙箱 dev 链路：浏览器访问 http://localhost:16666/openlist-ui/
 *   → preview-gateway 透传到 http://127.0.0.1:5174/openlist-ui/
 *   → vite (VITE_BASE=/openlist-ui/) 输出 <base href="/openlist-ui/">
 *   → 资源路径正确解析到 :5174/openlist-ui/src/main.ts
 * - vue-router 也需要感知 base：否则 location.pathname='/openlist-ui/' 被
 *   解析为 path='/openlist-ui/'，路由表无匹配 → 空白
 * - 设 base='/openlist-ui/' 后，vue-router 剥前缀 → path='/' → redirect → /home
 *
 * 生产模式：Android WebView 加载 file:///android_asset/openlist/index.html
 * - base='/openlist-ui/' 在 file:// 下也无害（hash 路由不依赖 path 解析）
 * - 生产环境 vite.config.ts 的 base 默认 './' 仍然控制 HTML 资源路径，与 router base 解耦
 */
export const router = createRouter({
  history: createWebHashHistory("/openlist-ui/"),
  routes,
});
