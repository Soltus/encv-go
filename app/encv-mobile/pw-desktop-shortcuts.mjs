// Task 1.3 桌面快捷键：真实浏览器端到端（`/` 聚焦搜索、Esc 关浮层）
// 用法：node pw-desktop-shortcuts.mjs   （需 dev server :8100 + 后端 :2025）
import { chromium } from "@playwright/test";

const base = process.env.URL || "http://127.0.0.1:8100";
const results = [];
const check = (name, ok, extra = "") => {
  results.push({ name, ok, extra });
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}${extra ? "  — " + extra : ""}`);
};

const browser = await chromium.launch({
  executablePath: "/usr/bin/chromium",
  args: ["--no-sandbox", "--disable-dev-shm-usage"],
});
const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
await page.addInitScript(() => localStorage.setItem("encv-server-url", "http://127.0.0.1:2025"));
const jsErrors = [];
page.on("pageerror", e => {
  const msg = String(e);
  // 沙箱 dev 环境噪音：vite HMR 连不上 :16666（preview-gateway 未起），与本特性无关
  if (/WebSocket closed without opened|Failed to connect/i.test(msg)) return;
  jsErrors.push(msg.slice(0, 120));
});

// ── Files 页：`/` 聚焦搜索 ──
await page.goto(`${base}/tabs/files`, { waitUntil: "domcontentloaded", timeout: 60000 });
await page.waitForTimeout(4000);

const beforeFocus = await page.evaluate(() => document.activeElement?.tagName ?? "null");
check("前置：进入 Files 页时焦点不在搜索框", beforeFocus !== "SEARCH-INPUT" && beforeFocus !== "DIV");

await page.keyboard.press("/");
const focusedSearch = await page.evaluate(() => {
  const el = document.activeElement;
  return el?.getAttribute?.("data-testid") === "search-input";
});
check("按 `/` 后搜索框获得焦点", focusedSearch);

await page.keyboard.type("automation");
const typed = await page.evaluate(
  () => document.querySelector("[data-testid='search-input']")?.textContent ?? "",
);
check("`/` 后键盘输入进入搜索框", typed.includes("automation"), `text=${JSON.stringify(typed.slice(0, 20))}`);

await page.keyboard.press("Escape");
const searchStillOk = await page.evaluate(
  () => !!document.querySelector("[data-testid='search-input']"),
);
check("Esc（无浮层打开）不报错、页面不异常", searchStillOk);

// ── Settings 页：Esc 关闭 ion-modal ──
await page.click('.desktop-rail a[href="/tabs/settings"]');
await page.waitForTimeout(1500);
await page.getByText("编辑原始配置").first().click();
await page.waitForSelector("ion-modal:not(.overlay-hidden)", { timeout: 8000 });
check("设置页打开 JSON 编辑浮层", true);

await page.keyboard.press("Escape");
await page.waitForTimeout(600);
const modalOpen = await page.evaluate(() => {
  const m = document.querySelector("ion-modal");
  return !!m && !m.classList.contains("overlay-hidden");
});
check("按 Esc 后浮层关闭", !modalOpen);

check("无 JS 运行时错误", jsErrors.length === 0, jsErrors.join(" | "));

const failed = results.filter(r => !r.ok).length;
console.log(`\n${results.length - failed}/${results.length} PASS`);
await browser.close();
process.exit(failed ? 1 : 0);
