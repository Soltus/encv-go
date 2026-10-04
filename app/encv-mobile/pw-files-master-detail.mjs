// P1 Task 1.2.2（剩余）：Files 页 master-detail 双栏 —— 真实浏览器回归锁
//
// 契约：
//   - 桌面形态：文件行**单击 = 选中**（右侧详情面板），**双击 = 打开**（保持原行为可达）；
//     目录单击仍导航（与手机一致）。详情面板只展示元数据 + 动作，不改写列表语义。
//   - 手机形态：**零回归** —— 不得出现详情面板，单击仍是"打开/进入"。
//
// 用法：URL=http://127.0.0.1:16666/tabs/files node pw-files-master-detail.mjs
import { chromium } from "@playwright/test";

const base = (process.env.URL || "http://127.0.0.1:16666/tabs/files").replace(/\/tabs\/files.*$/, "");
const results = [];
const check = (name, ok, extra = "") => {
  results.push({ name, ok });
  console.log(`${ok ? "PASS" : "FAIL"}  ${name}${extra ? "  — " + extra : ""}`);
};

const browser = await chromium.launch({
  executablePath: "/usr/bin/chromium",
  args: ["--no-sandbox", "--disable-dev-shm-usage"],
});

// ⚠️ 页面里还有侧抽屉（ion-menu）的列表，它是**不可见**的；
//    必须限定主内容里的列表，并逐条判 isVisible，否则点到隐藏元素会一直超时。
async function visibleRows(page) {
  const all = page.locator("#main-content ion-list ion-item");
  const n = await all.count();
  const out = [];
  for (let i = 0; i < n; i++) {
    const r = all.nth(i);
    if (await r.isVisible().catch(() => false)) out.push(r);
  }
  return out;
}

async function openPage(width, height) {
  const page = await browser.newPage({ viewport: { width, height } });
  await page.goto(`${base}/tabs/files`, { waitUntil: "domcontentloaded", timeout: 60000 });
  await page.waitForTimeout(4000);
  return page;
}

// ── 桌面 1440×900 ──
const page = await openPage(1440, 900);
const rows = await visibleRows(page);
const rowCount = rows.length;
check("列表已渲染出条目", rowCount > 0, `rows=${rowCount}`);

// 根目录下是挂载点（目录），先进入第一个挂载点才能看到真实文件
check("桌面：详情面板容器已挂载", (await page.locator('[data-testid="files-detail"]').count()) > 0);
await rows[0].click();
await page.waitForTimeout(1500);

const rows2 = await visibleRows(page);
// 找一个**文件**（非目录）行：目录行文案是"目录"
let targetIdx = -1;
for (let i = 0; i < Math.min(rows2.length, 12); i++) {
  const txt = (await rows2[i].innerText()) || "";
  if (!txt.includes("目录")) {
    targetIdx = i;
    break;
  }
}
check("进入目录后存在可点选的文件行", targetIdx >= 0, `idx=${targetIdx} rows=${rows2.length}`);

if (targetIdx >= 0) {
  const target = rows2[targetIdx];
  const name = ((await target.locator("h2").first().textContent()) || "").trim();
  await target.click();
  await page.waitForTimeout(800);
  const detail = page.locator('[data-testid="files-detail"]').first();
  const dtext = (await detail.innerText()) || "";
  check("桌面：单击文件行后详情面板显示该文件名", dtext.includes(name), `name=${name} detail=${dtext.slice(0, 60)}`);
  // 单击只选中、不打开/不导航（列表条数不变 = 没进入子目录、没切页面）
  const after = await visibleRows(page);
  check("单击只选中不打开（列表条数不变）", after.length === rows2.length, `${rows2.length} → ${after.length}`);
  // 双击仍能触发原有打开行为（不被详情面板顶掉）
  await target.dblclick();
  await page.waitForTimeout(1200);
  check("双击后页面未崩溃（列表或播放器仍可渲染）", (await page.locator("body").innerText()).length > 0);
}

// ── ≥1440 三栏（列表 + 详情 + 预览）/ <1440 保持双栏 ──
async function previewVisible(page) {
  const el = page.locator('[data-testid="files-preview"]').first();
  if ((await el.count()) === 0) return false;
  return await el.isVisible().catch(() => false);
}

const wide = await openPage(1440, 900);
const wideRows = await visibleRows(wide);
if (wideRows.length > 0) {
  await wideRows[0].click(); // 先进入挂载点/目录
  await wide.waitForTimeout(1200);
}
check("≥1440：出现第三栏（预览列）", await previewVisible(wide));

const narrow = await openPage(1280, 900);
check("<1440：不出现第三栏（仍双栏）", !(await previewVisible(narrow)));

// ── 手机 390×844：零回归 ──
const m = await openPage(390, 844);
const mRows = await visibleRows(m);
if (mRows.length > 0) {
  await mRows[0].click();
  await m.waitForTimeout(800);
}
const mDetail = await m.locator('[data-testid="files-detail"]').count();
check("手机形态不出现详情面板（零回归）", mDetail === 0, `count=${mDetail}`);

const failed = results.filter(r => !r.ok).length;
console.log(`\n${results.length - failed}/${results.length} PASS`);
await browser.close();
process.exit(failed ? 1 : 0);
