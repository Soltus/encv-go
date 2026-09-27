/**
 * main.js —— ENCV 容器解密的**纯前端**预览。
 *
 * 这一页不请求任何后端：容器字节来自静态样例或用户选的文件，
 * 解密全部由浏览器里的 encv-container.wasm（Go 编译）完成。
 * 页面里若出现 /stream、/api 之类的后端请求，就是回归。
 */

const SAMPLES = [
  // mp4 的 box 头长度不固定（这里 remux 后是 0x20 而不是常见的 0x18），
  // 所以按偏移 4 处的 'ftyp' 标识校验，别把 box size 写死。
  { file: "samples/sample.4pm.sccgv", label: "视频 .sccgv", kind: "video", magic: [0x66, 0x74, 0x79, 0x70], magicOffset: 4, note: "mp4 (ftyp)" },
  { file: "samples/sample.3pm.sccga", label: "音频 .sccga", kind: "audio", magic: [0x49, 0x44, 0x33], note: "mp3 (ID3)" },
  { file: "samples/sample.gnp.sccgi", label: "图片 .sccgi", kind: "image", magic: [0x89, 0x50, 0x4e, 0x47], note: "png" },
  { file: "samples/sample.fdp.sccgpdf", label: "PDF .sccgpdf", kind: "pdf", magic: [0x25, 0x50, 0x44, 0x46], note: "pdf" },
  { file: "samples/sample.txt.sccgt", label: "文本 .sccgt", kind: "text", magic: null, note: "utf-8" },
  { file: "samples/sample.xcod.sccgwps", label: "WPS .sccgwps", kind: "wps", magic: [0x50, 0x4b, 0x03, 0x04], note: "docx/zip" },
];

const el = id => document.getElementById(id);

const ui = {
  badge: el("badge"),
  samples: el("samples"),
  file: el("file"),
  password: el("password"),
  ownResult: el("own-result"),
  drop: el("drop"),
  result: el("result"),
  resultTitle: el("result-title"),
  meta: el("meta"),
  selftest: el("btn-selftest"),
  metric: el("metric"),
  log: el("log"),
  plaintext: el("plaintext"),
  plainfile: el("plainfile"),
  btnEncrypt: el("btn-encrypt"),
  encMetric: el("enc-metric"),
  encDetail: el("enc-detail"),
};

let api = null; // globalThis.encvContainer

function log(line) {
  const stamp = new Date().toLocaleTimeString("zh-CN", { hour12: false });
  ui.log.textContent = `[${stamp}] ${line}\n${ui.log.textContent ?? ""}`.slice(0, 20000);
}

// ────────────────────────────── wasm 内核 ──────────────────────────────

async function boot() {
  if (typeof Go === "undefined") throw new Error("wasm_exec.js 没加载（缺少 globalThis.Go）");
  const go = new Go();
  // 不用 instantiateStreaming：静态服务未必给 .wasm 正确的 MIME，
  // 用 arrayBuffer 自己实例化更稳。
  const bytes = await fetch("./wasm/encv-container.wasm").then(r => {
    if (!r.ok) throw new Error(`取 wasm 失败：HTTP ${r.status}`);
    return r.arrayBuffer();
  });
  const res = await WebAssembly.instantiate(bytes, go.importObject);
  go.run(res.instance);
  for (let i = 0; i < 200 && !globalThis.encvContainer; i++) await new Promise(r => setTimeout(r, 10));
  if (!globalThis.encvContainer) throw new Error("wasm 未挂载 globalThis.encvContainer");
  api = globalThis.encvContainer;

  ui.badge.textContent = "wasm 内核就绪（无后端）";
  ui.badge.dataset.state = "ok";
  log("wasm 内核加载完成；本页不会向后端发任何请求");
}

// ────────────────────────────── 解密与渲染 ──────────────────────────────

function plainLength(info) {
  // 用 wasm 给的**明文**长度：容器里记录的 seg.Size 是含段头/nonce/MAC 的密文尺寸，
  // 拿它当读取长度会在空文件等边界上越界。
  return info.plainLength;
}

function decrypt(bytes, password) {
  const opened = api.open(new Uint8Array(bytes), password);
  if (!opened.ok) throw new Error(opened.error);
  const handle = opened.value.handle;
  const info = api.info(handle).value;
  const plain = api.readRange(handle, 0, plainLength(info));
  api.close(handle);
  if (!plain.ok) throw new Error(plain.error);
  return { plain: new Uint8Array(plain.data), info };
}

function matchesMagic(bytes, magic, offset = 0) {
  if (!magic) return true;
  return magic.every((b, i) => bytes[offset + i] === b);
}

function render(title, plain, info, kind) {
  ui.resultTitle.textContent = title;
  const blob = new Blob([plain], { type: mimeOf(kind) });
  const url = URL.createObjectURL(blob);

  ui.result.innerHTML = "";
  if (kind === "video") {
    const v = document.createElement("video");
    v.src = url;
    v.controls = true;
    ui.result.appendChild(v);
  } else if (kind === "audio") {
    const a = document.createElement("audio");
    a.src = url;
    a.controls = true;
    ui.result.appendChild(a);
  } else if (kind === "image") {
    const img = document.createElement("img");
    img.src = url;
    img.className = "preview-img";
    ui.result.appendChild(img);
  } else if (kind === "pdf") {
    const f = document.createElement("iframe");
    f.src = url;
    f.className = "preview-doc";
    ui.result.appendChild(f);
  } else if (kind === "text") {
    const pre = document.createElement("pre");
    pre.className = "preview-text";
    pre.textContent = new TextDecoder().decode(plain);
    ui.result.appendChild(pre);
  } else {
    const p = document.createElement("p");
    p.className = "empty";
    p.textContent = `浏览器无法直接渲染该类型（${kind}）；已解密 ${plain.length} 字节。`;
    const a = document.createElement("a");
    a.href = url;
    a.download = title;
    a.textContent = "下载解密结果";
    ui.result.append(p, a);
  }

  ui.meta.innerHTML = "";
  const rows = [
    ["容器类型", info.containerType],
    ["segment 数", info.segments],
    ["分层密钥", info.hasWrappedDEK ? "WrappedDEK ✓" : "（无）"],
    ["DEK 长度", `${info.keyLen} 字节`],
    ["解密后大小", `${plain.length} 字节`],
  ];
  for (const [k, v] of rows) {
    const dt = document.createElement("dt");
    dt.textContent = k;
    const dd = document.createElement("dd");
    dd.textContent = String(v);
    ui.meta.append(dt, dd);
  }
}

function mimeOf(kind) {
  return (
    {
      video: "video/mp4",
      audio: "audio/mpeg",
      image: "image/png",
      pdf: "application/pdf",
      text: "text/plain; charset=utf-8",
    }[kind] ?? "application/octet-stream"
  );
}

async function loadSample(sample) {
  const bytes = await fetch(sample.file).then(r => r.arrayBuffer());
  const { plain, info } = decrypt(bytes, ui.password.value);
  render(sample.label, plain, info, sample.kind);
  log(`${sample.label} 解密 ${plain.length} 字节（容器 magic 已由 wasm 校验，期望 ${sample.note}）`);
}

// ────────────────────────────── 加密（浏览器内） ──────────────────────────────

function bytesEqual(a, b) {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}

/**
 * 加密并**立刻回读验证**：
 * 加密完就解密一遍比对字节，能证明产出的容器格式自洽，
 * 而不是"看着加密成功了、实际没人能解开"。
 */
/**
 * 按文件类型决定容器类型。
 *
 * ⚠️ 不能一律标成 text：容器类型写错，解密回来就按 text 渲染，
 * 图片/视频会变成一堆乱码 —— 而字节其实是对的，看起来却像"解密失败"。
 */
function kindOfFile(file) {
  const name = (file?.name ?? "").toLowerCase();
  const mime = file?.type ?? "";
  const base = { originalName: file?.name || "encrypted.bin" };
  if (mime.startsWith("video/") || /\.(mp4|mov|mkv|webm|m4v)$/.test(name))
    return { ...base, containerType: 1, containerTypeStr: "video", ext: "sccgv" };
  if (mime.startsWith("audio/") || /\.(mp3|m4a|wav|flac|aac|ogg)$/.test(name))
    return { ...base, containerType: 2, containerTypeStr: "audio", ext: "sccga" };
  if (mime.startsWith("image/") || /\.(png|jpe?g|gif|webp|bmp|avif)$/.test(name))
    return { ...base, containerType: 3, containerTypeStr: "image", ext: "sccgi" };
  if (mime === "application/pdf" || name.endsWith(".pdf"))
    return { ...base, containerType: 4, containerTypeStr: "document", ext: "sccgpdf" };
  if (/\.(docx?|xlsx?|pptx?|wps|et|dps)$/.test(name))
    return { ...base, containerType: 4, containerTypeStr: "document", ext: "sccgwps" };
  return { ...base, containerType: 5, containerTypeStr: "text", ext: "sccgt" };
}

const TEXT_META = { containerType: 5, containerTypeStr: "text", originalName: "encrypted.txt", ext: "sccgt" };

function encryptAndVerify(plain, label, meta = TEXT_META) {
  ui.encMetric.textContent = "加密中…";
  const started = performance.now();
  const enc = api.encryptBytes(plain, ui.password.value, meta);
  if (!enc.ok) throw new Error(`加密失败：${enc.error}`);
  const container = new Uint8Array(enc.data);
  const elapsed = Math.round(performance.now() - started);

  const { plain: back, info } = decrypt(container, ui.password.value);
  const same = bytesEqual(plain, back);

  ui.encMetric.textContent = same ? "✓ 往返一致" : "✗ 往返不一致";
  ui.encDetail.innerHTML = `${label} → 容器 ${container.length} 字节（明文 ${plain.length}）· 加密 ${elapsed}ms · 分层密钥 ${
    info.hasWrappedDEK ? "✓" : "无"
  } · <a id="enc-download" href="#">下载容器 .${meta.ext}</a>`;
  const a = el("enc-download");
  a.href = URL.createObjectURL(new Blob([container], { type: "application/octet-stream" }));
  a.download = `${meta.originalName}.${meta.ext}`;
  log(`${same ? "✓" : "✗"} 加密 ${label}：明文 ${plain.length} → 容器 ${container.length} 字节，回读${same ? "一致" : "不一致"}`);
  return { container, same };
}

ui.btnEncrypt.addEventListener("click", () => {
  try {
    encryptAndVerify(new TextEncoder().encode(ui.plaintext.value), "文本");
  } catch (error) {
    ui.encMetric.textContent = "✗ 失败";
    log(`✗ 加密失败：${error.message}`);
  }
});

ui.plainfile.addEventListener("change", () => {
  const f = ui.plainfile.files?.[0];
  if (!f) return;
  f.arrayBuffer()
    .then(buf => encryptAndVerify(new Uint8Array(buf), f.name, kindOfFile(f)))
    .catch(error => {
      ui.encMetric.textContent = "✗ 失败";
      log(`✗ 加密 ${f.name} 失败：${error.message}`);
    });
});

// ────────────────────────────── 样例按钮 ──────────────────────────────

ui.samples.innerHTML = SAMPLES.map(
  (s, i) => `<button class="btn btn-sample" data-i="${i}">${s.label}</button>`
).join("");
for (const btn of ui.samples.querySelectorAll("button")) {
  btn.addEventListener("click", () => {
    loadSample(SAMPLES[Number(btn.dataset.i)]).catch(e => log(`✗ ${e.message}`));
  });
}

// ────────────────────────────── 自带容器 ──────────────────────────────

// decryptFile：选文件与拖放共用同一条解密路径。
// （之前只有 drop 会解密，用文件选择框选完只改一行文案，看着像"没反应"。）
function decryptFile(file) {
  return file
    .arrayBuffer()
    .then(buf => {
      const { plain, info } = decrypt(new Uint8Array(buf), ui.password.value);
      render(file.name, plain, info, guessKind(info, file.name));
      log(`自带容器 ${file.name} 解密 ${plain.length} 字节（类型 ${info.containerType}）`);
    })
    .catch(err => log(`✗ ${file.name}：${err.message}`));
}

ui.file.addEventListener("change", () => {
  const f = ui.file.files?.[0];
  ui.ownResult.textContent = f ? `已选择：${f.name}（${f.size} 字节）` : "未选择文件";
  if (f) decryptFile(f);
});

for (const type of ["dragenter", "dragover"]) {
  ui.drop.addEventListener(type, e => {
    e.preventDefault();
    ui.drop.dataset.drag = "1";
  });
}
for (const type of ["dragleave", "drop"]) {
  ui.drop.addEventListener(type, e => {
    e.preventDefault();
    ui.drop.dataset.drag = "";
  });
}
ui.drop.addEventListener("drop", e => {
  const file = e.dataTransfer?.files?.[0];
  if (!file) return;
  const sink = new DataTransfer();
  sink.items.add(file);
  ui.file.files = sink.files;
  ui.file.dispatchEvent(new Event("change"));
  decryptFile(file);
});

function guessKind(info, name = "") {
  // 先看文件名：容器类型只有 "document" 这一档，分不出 pdf 和 docx。
  // 下载回来的容器叫 "原文件名.原扩展名.sccg*"，所以扩展名后面可能还有容器后缀。
  const n = String(name).toLowerCase();
  const ext = re => new RegExp(`\\.${re}(\\.|$)`).test(n);
  if (ext("(mp4|mov|mkv|webm|m4v)")) return "video";
  if (ext("(mp3|m4a|wav|flac|aac|ogg)")) return "audio";
  if (ext("(png|jpe?g|gif|webp|bmp|avif)")) return "image";
  if (ext("pdf")) return "pdf";
  if (ext("(docx?|xlsx?|pptx?|wps|et|dps)")) return "wps";
  if (ext("(txt|md|csv|json|log|ya?ml)")) return "text";

  const t = String(info?.containerType ?? "").toLowerCase();
  if (t.includes("video")) return "video";
  if (t.includes("audio")) return "audio";
  if (t.includes("image")) return "image";
  if (t.includes("text")) return "text";
  if (t.includes("document")) return "wps";
  return "unknown";
}

// ────────────────────────────── 自检（纯前端可验证） ──────────────────────────────

async function selfTest() {
  const cases = [];
  const push = async (name, fn) => {
    try {
      cases.push([name, await fn()]);
    } catch (error) {
      log(`  ↳ ${name} 抛错：${error.message}`);
      cases.push([name, false]);
    }
  };

  await push("wasm 内核已就绪", () => Boolean(api));

  await push("加密→解密往返一致（中文/emoji）", () => {
    const text = new TextEncoder().encode("往返 🔐 中文 test 123");
    return encryptAndVerify(text, "自检文本").same;
  });

  await push("wasm 加密的容器不能被错口令打开", () => {
    const enc = api.encryptBytes(new TextEncoder().encode("secret"), "good-pw", {
      containerType: 5,
      containerTypeStr: "text",
    });
    if (!enc.ok) return false;
    const wrong = api.open(enc.data, "bad-pw");
    if (wrong.ok) api.close(wrong.value.handle);
    return wrong.ok === false;
  });

  for (const s of SAMPLES) {
    await push(`${s.label} 解密结果的 magic 正确`, async () => {
      const bytes = await fetch(s.file).then(r => r.arrayBuffer());
      const { plain, info } = decrypt(bytes, ui.password.value);
      const ok = matchesMagic(plain, s.magic, s.magicOffset ?? 0);
      if (!ok) log(`  ${s.label} 前 8 字节：${Array.from(plain.slice(0, 8)).map(b => b.toString(16).padStart(2, "0")).join(" ")}`);
      return ok && plain.length > 0 && Number(info.segments) >= 1;
    });
  }

  await push("随机读：中段偏移与整体解密一致", async () => {
    const bytes = await fetch("samples/sample.4pm.sccgv").then(r => r.arrayBuffer());
    const opened = api.open(new Uint8Array(bytes), ui.password.value);
    const handle = opened.value.handle;
    const info = api.info(handle).value;
    const total = plainLength(info);
    const off = Math.floor(total / 2);
    const a = new Uint8Array(api.readRange(handle, off, 32).data);
    const b = new Uint8Array(api.readRange(handle, 0, total).data).subarray(off, off + 32);
    api.close(handle);
    const same = a.length === b.length && a.every((x, i) => x === b[i]);
    log(`  随机读 off=${off}：${same ? "一致" : "不一致"}`);
    return same;
  });

  await push("错口令必须被拒绝（不能解出内容）", async () => {
    const bytes = await fetch("samples/sample.txt.sccgt").then(r => r.arrayBuffer());
    const opened = api.open(new Uint8Array(bytes), "definitely-wrong-password");
    if (opened.ok) {
      api.close(opened.value.handle);
      return false;
    }
    log(`  错口令返回：${opened.error}`);
    return true;
  });

  for (const [name, ok] of cases) log(`${ok ? "✓" : "✗"} ${name}`);
  const failed = cases.filter(([, ok]) => !ok);
  if (failed.length === 0) log(`自检通过（${cases.length} 项，全部在浏览器内完成）`);
  else log(`自检失败：${failed.map(([n]) => n).join("、")}`);
  ui.metric.textContent = `${cases.length - failed.length}/${cases.length} 通过`;
}

ui.selftest.addEventListener("click", () => {
  ui.selftest.disabled = true;
  ui.metric.textContent = "执行中…";
  selfTest()
    .catch(e => log(`✗ 自检抛错：${e.message}`))
    .finally(() => (ui.selftest.disabled = false));
});

boot().catch(error => {
  ui.badge.textContent = "wasm 内核加载失败";
  ui.badge.dataset.state = "error";
  log(`✗ 启动失败：${error.message}`);
});

window.encPreview = { get api() { return api; }, selfTest };
