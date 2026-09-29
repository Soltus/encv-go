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
  {
    file: "samples/sample.4pm.sccgv",
    label: "视频 .sccgv",
    kind: "video",
    magic: [0x66, 0x74, 0x79, 0x70],
    magicOffset: 4,
    note: "mp4 (ftyp)",
  },
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
  filestream: el("filestream"),
  streamurl: el("streamurl"),
  streamurlGo: el("streamurl-go"),
  mseurl: el("mseurl"),
  mseGo: el("mse-go"),
  streamResult: el("stream-result"),
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
  if (/\.(docx?|xlsx?|pptx?|wps|et|dps)$/.test(name)) return { ...base, containerType: 4, containerTypeStr: "document", ext: "sccgwps" };
  return { ...base, containerType: 5, containerTypeStr: "text", ext: "sccgt" };
}

const TEXT_META = { containerType: 5, containerTypeStr: "text", originalName: "encrypted.txt", ext: "sccgt" };

// ────────────────────────── 流式加密（大附件） ──────────────────────────
//
// 整块 API `encryptBytes` 要求同时把「完整明文 + 完整密文 + 完整容器」摆在内存里，
// 浏览器里明文一上 GB 就把 wasm 打死（fatal error: out of memory）。
// 流式这套把明文按片喂进去、密文按片取出来：Go 侧只持有当前这一片的缓冲，
// JS 侧把产出的碎块交给 Blob —— 两边都不攒整个文件。

const STREAM_CHUNK = 1 << 20; // 1MB：喂给内核的片大小
// 超过这个体积，页面就不再把容器读回来逐字节比了：open(bytes) 这一侧仍是整块的，
// 大容器会把刚省下来的内存又吃回去。这一段只报体积/段数；逐字节回读由
// pw-enc-stream.ts 在 VERIFY_MAX_MB（默认 128MB）以内完成，方式与 Go 单测同源。
const VERIFY_LIMIT = 64 << 20;

function toU8(v) {
  return v instanceof Uint8Array ? v : new Uint8Array(v ?? []);
}

// fileChunks 把 Web API 的 ReadableStream 转成简单的异步迭代器（不整体落内存）。
async function* fileChunks(file, chunkSize = STREAM_CHUNK) {
  const reader = file.stream().getReader();
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      let buf = toU8(value);
      for (let off = 0; off < buf.length; off += chunkSize) {
        yield buf.subarray(off, Math.min(off + chunkSize, buf.length));
      }
    }
  } finally {
    reader.releaseLock();
  }
}

function* slices(bytes, chunkSize = STREAM_CHUNK) {
  for (let off = 0; off < bytes.length; off += chunkSize) {
    yield bytes.subarray(off, Math.min(off + chunkSize, bytes.length));
  }
}

/**
 * 流式加密成 Blob。
 *
 * 产物的拼装顺序是 **head ‖ 每次 write 取回的碎块 ‖ end 最后一次碎块 ‖ tail**：
 * 容器头要先于数据出现，但头里的 manifest 偏移/CRC 只有写完全部数据才知道，
 * 所以内核把头留到最后一次性交回来，由这里负责拼 —— 这也是前端这层
 * 唯一需要知道的容器格式细节。
 *
 * @param {AsyncIterable<Uint8Array>|Iterable<Uint8Array>} source 明文分片
 */
async function encryptToBlob(source, meta, opts = {}) {
  const started = performance.now();
  const begun = api.encryptBegin(ui.password.value, {
    ...meta,
    ...opts,
    segmentSize: opts.segmentSize ?? STREAM_CHUNK,
  });
  if (!begun.ok) throw new Error(`encryptBegin 失败：${begun.error}`);
  const handle = begun.value.handle;
  const pieces = [];
  let written = 0;
  try {
    for await (const chunk of source) {
      const r = api.encryptWrite(handle, chunk);
      if (!r.ok) throw new Error(`encryptWrite 失败：${r.error}`);
      const produced = toU8(r.data); // 本次调用攒出的、已成型的数据段
      if (produced.length) pieces.push(produced);
      written += chunk.length;
    }
    const end = api.encryptEnd(handle);
    if (!end.ok) throw new Error(`encryptEnd 失败：${end.error}`);
    const v = end.value;
    const blob = new Blob([toU8(v.head), ...pieces, toU8(v.data), toU8(v.tail)], {
      type: "application/octet-stream",
    });
    return { blob, written, value: v, elapsed: Math.round(performance.now() - started) };
  } catch (error) {
    try {
      api.encryptAbort(handle);
    } catch {}
    throw error;
  }
}

/**
 * 流式打开：容器**不整体进内存**，缺哪段字节就让 JS 现取哪段。
 *
 * open(bytes) 要求容器整体在 wasm 的线性内存里，1GB 容器会直接 fatal error。
 * 这里把字节供给反过来交给前端：File.slice / HTTP Range / OPFS 都能当数据源。
 * 打开之后 info/readRange/close 的用法与 open 完全一致。
 */
/**
 * @param {(offset:number,length:number)=>Promise<Uint8Array>} read 按需取字节（File.slice / Range / OPFS…）
 */
async function openStream(read, size, name = "stream") {
  const started = performance.now();
  const opened = api.openStream(ui.password.value, { size, name });
  if (!opened.ok) throw new Error(`openStream 失败：${opened.error}`);
  const handle = opened.value.handle;
  // 内核是**同步**的：缺哪些字节会直接告诉你，喂进去再推进，直到 ready。
  // （Go/wasm 不能在导出函数里挂起等 Promise，所以不能反过来让内核回调 JS。）
  await feedUntilReady(handle, opened.value, read);
  const infoRes = await callWithFeed(handle, () => api.info(handle), read);
  if (!infoRes.ok) throw new Error(`info 失败：${infoRes.error ?? JSON.stringify(infoRes)}`);
  const info = infoRes.value;
  return { handle, info, elapsed: Math.round(performance.now() - started) };
}

// callWithFeed 统一处理「内核说还缺字节」：补上再重试。
// 打开、info、readRange 三处都可能回 need —— 段头/密文都是按需取的。
async function callWithFeed(handle, call, read) {
  // 判定活锁的标准不是"补了多少次"（1GB/1MB 段的容器有 1024 段，info 要逐段读段头，
  // 补一千次是正常的），而是"同一个缺口被反复索要"。
  const seen = new Map();
  for (let attempt = 0; ; attempt++) {
    const r = call();
    if (r.ok || !r.need) return r; // 缺口是**顶层**字段：{ok:false, need:[...]}
    if (attempt > 200000) throw new Error("补字节次数异常，放弃");
    for (const { offset, length } of r.need) {
      const key = `${offset}:${length}`;
      const n = (seen.get(key) ?? 0) + 1;
      seen.set(key, n);
      if (n > 8) {
        throw new Error(`内核反复索要同一段 [${key}]${r.error ? " / 原始错误：" + r.error : ""}`);
      }
      const fed = api.streamFeed(handle, offset, await read(offset, length));
      if (!fed.ok) throw new Error(`streamFeed 失败：${fed.error}`);
    }
  }
}

async function feedUntilReady(handle, state, read) {
  let need = state?.need;
  let guard = 0;
  while (need?.length) {
    if (++guard > 64) throw new Error("喂字节次数异常（内核一直在要字节，多半是 size 不对）");
    for (const { offset, length } of need) {
      const bytes = await read(offset, length);
      const fed = api.streamFeed(handle, offset, bytes);
      if (!fed.ok) throw new Error(`streamFeed 失败：${fed.error}`);
      need = fed.value?.need;
      if (fed.value?.ready) return;
    }
  }
}

// 按偏移读一段明文（供比对用）。
// 流式打开的容器，命中的段可能还没喂进来：内核会回 need，取来喂上再重试。
async function readWindow(handle, offset, length, read) {
  const r = await callWithFeed(handle, () => api.readRange(handle, offset, length), read);
  if (!r.ok) throw new Error(`readRange 失败：${r.error ?? JSON.stringify(r)}`);
  return new Uint8Array(r.data);
}

async function openStreamFile(file) {
  const read = async (offset, length) => new Uint8Array(await file.slice(offset, offset + length).arrayBuffer());
  const { handle, info, elapsed } = await openStream(read, file.size, file.name);
  const total = Number((info.plainLength ?? info.plainLength === 0) ? info.plainLength : 0);
  const windows = [0, Math.floor(total / 2), Math.max(0, total - 65536)];
  const heads = [];
  for (const off of windows) {
    const got = await readWindow(handle, off, Math.min(65536, Math.max(0, total - off)), read);
    heads.push(
      `${off}:${Array.from(got.slice(0, 4))
        .map(b => b.toString(16).padStart(2, "0"))
        .join("")}`
    );
  }
  api.close(handle);
  return { info, elapsed, total, heads };
}

async function encryptStreamAndVerify(label, source, meta, expected) {
  ui.encMetric.textContent = "流式加密中…";
  const { blob, value, elapsed } = await encryptToBlob(source, meta);

  let same = null; // null = 体积太大，本页不做逐字节回读
  if (expected && expected.length <= VERIFY_LIMIT) {
    const container = new Uint8Array(await blob.arrayBuffer());
    const { plain } = decrypt(container, ui.password.value);
    same = bytesEqual(plain, expected);
  }

  ui.encMetric.textContent = same === false ? "✗ 往返不一致" : same ? "✓ 往返一致" : "✓ 已产出（体积过大，页面内不回读）";
  ui.encDetail.innerHTML = `${label} → 容器 ${blob.size} 字节（明文 ${value.plainSize}）· ${value.segments} 段 · 流式加密 ${elapsed}ms · <a id="enc-download" href="#">下载容器 .${meta.ext}</a>`;
  const a = el("enc-download");
  a.href = URL.createObjectURL(blob);
  a.download = `${meta.originalName}.${meta.ext}`;
  log(
    `${same === false ? "✗" : "✓"} 流式加密 ${label}：明文 ${value.plainSize} → 容器 ${blob.size} 字节 / ${value.segments} 段 / ${elapsed}ms${
      same === null ? "（超过回读上限，未做逐字节比对）" : same ? "，回读一致" : "，回读不一致"
    }`
  );
  return { blob, value, same };
}

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

// 选文件加密**一律走流式**：这是真实的"大附件"路径 —— 明文不整体落内存，
// 交给 <video>/<img> 之前也没人需要它完整地待着。
ui.plainfile.addEventListener("change", () => {
  const f = ui.plainfile.files?.[0];
  if (!f) return;
  const meta = kindOfFile(f);
  // 期望值只在能装下时才会去取得 —— 超限就直接跳过逐字节比对（见 VERIFY_LIMIT 注释）
  const expected = f.size <= VERIFY_LIMIT ? f.arrayBuffer().then(ab => new Uint8Array(ab)) : Promise.resolve(null);
  expected
    .then(buf => encryptStreamAndVerify(f.name, fileChunks(f), meta, buf))
    .catch(error => {
      ui.encMetric.textContent = "✗ 失败";
      log(`✗ 流式加密 ${f.name} 失败：${error.message}`);
    });
});

// ────────────────────────────── 样例按钮 ──────────────────────────────

ui.samples.innerHTML = SAMPLES.map((s, i) => `<button class="btn btn-sample" data-i="${i}">${s.label}</button>`).join("");
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

/**
 * 用 **HTTP Range** 打开一个容器：只取它真正要读的那几个区间，全程不整体下载。
 *
 * 这是"边下载边播"的地基：`read` 接到哪里，字节就从哪里来（Range / OPFS / 本地 File…）。
 * 内核侧完全不知道字节是哪来的，同一套 openStream/streamFeed 协议。
 */
async function openStreamURL(url) {
  const head = await fetch(url, { method: "HEAD" });
  if (!head.ok) throw new Error(`HEAD ${url} 失败：HTTP ${head.status}`);
  const size = Number(head.headers.get("Content-Length"));
  if (!Number.isFinite(size) || size <= 0) throw new Error("服务端没给 Content-Length");

  let requests = 0;
  let bytes = 0;
  const read = async (offset, length) => {
    const res = await fetch(url, { headers: { Range: `bytes=${offset}-${offset + length - 1}` } });
    // 206=部分内容；有些静态服务忽略 Range 直接回 200，那就自己切出来
    if (!res.ok && res.status !== 206 && res.status !== 200) {
      throw new Error(`Range 请求失败：HTTP ${res.status}`);
    }
    requests++;
    bytes += length;
    const all = new Uint8Array(await res.arrayBuffer());
    return res.status === 200 ? all.subarray(0, length) : all;
  };

  const { handle, info, elapsed } = await openStream(read, size, url.split("/").pop());
  const total = Number(info.plainLength);
  const windows = [];
  for (const off of [0, Math.floor(total / 2), Math.max(0, total - 4096)]) {
    const got = await readWindow(handle, off, Math.min(4096, total - off), read);
    windows.push(
      `${off}:${Array.from(got.slice(0, 3))
        .map(b => b.toString(16).padStart(2, "0"))
        .join("")}`
    );
  }
  api.close(handle);
  return { size, total, elapsed, requests, bytes, windows, segments: info.segments };
}

ui.streamurlGo.addEventListener("click", () => {
  const url = ui.streamurl.value.trim();
  if (!url) return;
  ui.streamResult.textContent = `流式打开（Range）中… ${url}`;
  openStreamURL(url)
    .then(r => {
      const mb = n => (n / 1048576).toFixed(1);
      ui.streamResult.textContent = `✓ Range 流式打开：${r.segments} 段 / 明文 ${mb(r.total)}MB / 只取了 ${mb(r.bytes)}MB（${r.requests} 个请求）`;
      log(
        `✓ HTTP Range 流式打开 ${url}（容器 ${mb(r.size)}MB，只下载 ${mb(r.bytes)}MB / ${r.requests} 个 Range 请求）：${r.segments} 段 · 明文 ${mb(r.total)}MB · ${r.elapsed}ms；随机读 ${r.windows.join(" ")}`
      );
    })
    .catch(err => {
      ui.streamResult.textContent = "✗ Range 流式打开失败";
      log(`✗ HTTP Range 流式打开 ${url} 失败：${err.message}`);
    });
});

/**
 * 边下边播：容器字节经 HTTP Range 按需取、解密后**边解边 append 给 MediaSource**。
 *
 * 前提：容器里的视频是**分片 MP4（fMP4，含 moof）**。实测源用
 * `-movflags +frag_keyframe+empty_moov+default_base_moof` 产出时，
 * 加密→解密后 fMP4 结构完整保留（插件按大小切片，不重新 remux），
 * 所以这条路不需要改插件。
 *
 * 不是 fMP4（ftyp+moov+mdat）时无法分段喂 —— 直接报错并提示走整体解密播放。
 */
async function playViaMSE(url) {
  if (typeof MediaSource === "undefined") throw new Error("该浏览器不支持 MediaSource");

  const head = await fetch(url, { method: "HEAD" });
  if (!head.ok) throw new Error(`HEAD ${url} 失败：HTTP ${head.status}`);
  const size = Number(head.headers.get("Content-Length"));
  const read = async (offset, length) => {
    const res = await fetch(url, { headers: { Range: `bytes=${offset}-${offset + length - 1}` } });
    if (!res.ok && res.status !== 206 && res.status !== 200) throw new Error(`Range 失败：HTTP ${res.status}`);
    const all = new Uint8Array(await res.arrayBuffer());
    return res.status === 200 ? all.subarray(0, length) : all;
  };

  const { handle, info } = await openStream(read, size, url.split("/").pop());
  const total = Number(info.plainLength);

  // 1) 取初始化段：ftyp+moov（到第一个 moof 为止）
  const probeLen = Math.min(131072, total);
  const headPlain = await readWindow(handle, 0, probeLen, read);
  const moofAt = indexOfBox(headPlain, "moof", 0);
  if (moofAt < 0) {
    api.close(handle);
    throw new Error("这个容器里的视频不是分片 MP4（没找到 moof），无法分段喂播；请走整体解密后播放");
  }
  const init = headPlain.subarray(0, moofAt);
  const picked = pickMime(init);
  if (!picked) {
    api.close(handle);
    throw new Error("浏览器不认这个视频的编码格式（MediaSource.isTypeSupported 全否）");
  }
  const { mime, codecs } = picked;

  // 2) 建 MediaSource，先 append 初始化段
  const ms = new MediaSource();
  const video = document.createElement("video");
  video.controls = true;
  video.src = URL.createObjectURL(ms);
  await new Promise(r => ms.addEventListener("sourceopen", r, { once: true }));
  const sb = ms.addSourceBuffer(mime);
  sb.mode = "sequence";
  const append = buf =>
    new Promise((resolve, reject) => {
      sb.addEventListener("updateend", resolve, { once: true });
      sb.addEventListener("error", () => reject(new Error("SourceBuffer 出错（容器编码不匹配？）")), { once: true });
      sb.appendBuffer(buf);
    });

  await append(init);

  // 3) 边读边 append：窗口大小 256KB，只有真正要播的那一段才会被下载
  const WINDOW = 256 * 1024;
  for (let off = moofAt; off < total; off += WINDOW) {
    const chunk = await readWindow(handle, off, Math.min(WINDOW, total - off), read);
    await append(chunk);
  }
  ms.endOfStream();
  // ⚠️ codecs 与容器里的流不匹配时，appendBuffer 全程不报错、'updateend' 照常触发，
  // 错误只挂在 **video.error.message** 上（例如 "audio object type 0x40 does not match..."）。
  // 不显式查就等于"看着绿、其实没画面" —— 这里给它留出 demuxer 报错的时间窗口。
  await new Promise(r => setTimeout(r, 300));
  if (video.error) throw new Error(`播放管线报错：${video.error.message || `code=${video.error.code}`}`);
  api.close(handle);
  return { video, total, initBytes: init.length, mime };
}

// 在明文里找某个 box 的起始偏移（从 from 开始）
function indexOfBox(buf, name, from = 0) {
  for (let i = from; i + 8 <= buf.length; ) {
    const size = (buf[i] << 24) | (buf[i + 1] << 16) | (buf[i + 2] << 8) | buf[i + 3];
    const tag = String.fromCharCode(buf[i + 4], buf[i + 5], buf[i + 6], buf[i + 7]);
    if (tag === name) return i;
    if (!size) break;
    i += size;
  }
  return -1;
}

/**
 * 从初始化段读出 **视频 + 音频** 两组 codecs。
 *
 * 层级：视频是 stsd → avc1 → avcC；音频是 stsd → mp4a → esds。
 *
 * ⚠️ 音频**必须**一并读出：MSE 的 SourceBuffer 会把 mime 里的 codecs 与容器里的流
 * **逐项**核对。只写 `video/mp4; codecs="avc1.…"` 而容器里有 AAC 音轨时，
 * Chromium 的报错是：
 *   PipelineStatus::CHUNK_DEMUXER_ERROR_APPEND_FAILED:
 *   audio object type 0x40 does not match what is specified in the mimetype
 * 而这段报错**只出现在 video.error.message 里** —— appendBuffer 本身不抛错、
 * 'updateend' 正常触发，只看"append 成功"会把失败误判成"边下边播已就绪"。
 * （2026-09-29 实测：带音轨的 fMP4 样例在这套判断下必经此坑。）
 */
function codecsFromInit(init) {
  return { video: videoCodecFromInit(init), audio: audioCodecFromInit(init) };
}

// ⚠️ 别按"固定偏移"读：avcC 之前可能有 pasp/btrt 等可选 box，写死 86 字节偏移会读到无关字段
// （实测就偏成 avc1.e10019，于是被当成"浏览器不认"，白白退到兜底 codecs）。
// 正确做法是先找到 'avc1'，再在它后面找 'avcC'，取其负载里的三个字节。
function videoCodecFromInit(init) {
  const at = indexOfBytes(init, [0x61, 0x76, 0x63, 0x31]); // 'avc1'
  if (at < 0) return null;
  const avcC = indexOfBytes(init, [0x61, 0x76, 0x63, 0x43], at); // 'avcC' 在 avc1 之后
  if (avcC < 0 || avcC + 8 > init.length) return null;
  // 'avcC' box：size(4) + type(4) + configurationVersion(1) + profile + compat + level
  const [profile, compat, level] = [init[avcC + 5], init[avcC + 6], init[avcC + 7]];
  const hex = n => n.toString(16).padStart(2, "0");
  return `avc1.${hex(profile)}${hex(compat)}${hex(level)}`;
}

function audioCodecFromInit(init) {
  const at = indexOfBytes(init, [0x65, 0x73, 0x64, 0x73]); // 'esds'
  if (at < 0) return null;

  // 描述符从 esds 载荷的 version+flags 之后开始（type 起点 +8）
  let cur = at + 8;
  for (let guard = 0; guard < 8 && cur + 1 < init.length; guard++) {
    const tag = init[cur];
    if (tag === 0x00) return null;
    const head = descriptorLength(init, cur + 1);
    if (head.value < 0) return null;

    if (tag === 0x03) {
      // ES_Descriptor：ES_ID(2) + flags(1) 之后才是子描述符
      cur = head.next + 3;
      continue;
    }
    if (tag === 0x04) {
      // DecoderConfigDescriptor：objectType(1) streamType(1) bufferSize(3)
      // maxBitrate(4) avgBitrate(4) 之后是 DecoderSpecificInfo(tag 0x05)
      const body = head.next;
      const dsi = body + 13;
      for (let t = dsi; t + 1 < Math.min(body + head.value, init.length); t++) {
        if (init[t] !== 0x05) continue;
        const len = descriptorLength(init, t + 1);
        const aot = init[len.next] >> 3; // 首字节高 5 位 = AudioObjectType（2 = AAC-LC）
        if (aot >= 1 && aot <= 4) return `mp4a.40.${aot}`;
      }
      return null;
    }
    cur = head.next + head.value;
  }
  return null;
}

/**
 * ISO/IEC 14496-1 的描述符长度是**可变长**：每字节低 7 位累加，最高位为 continuation。
 * ffmpeg 在某些选项组合下会写成 `80 80 80 17` 这样的 4 字节形式 —— 只当它是单字节
 * 就会把 objectTypeIndication 的位置算错，进而"读不出音频 codecs"（实测）。
 */
function descriptorLength(buf, i) {
  let value = 0;
  for (let n = 0; n < 4 && i + n < buf.length; n++) {
    value = (value << 7) | (buf[i + n] & 0x7f);
    if ((buf[i + n] & 0x80) === 0) return { value, next: i + n + 1 };
  }
  return { value: -1, next: i };
}

function indexOfBytes(buf, needle, from = 0) {
  outer: for (let i = from; i + needle.length <= buf.length; i++) {
    for (let k = 0; k < needle.length; k++) if (buf[i + k] !== needle[k]) continue outer;
    return i;
  }
  return -1;
}

// 挑一个浏览器认的 mime：**优先带完整音视轨声明**，再逐级退化。
function pickMime(init) {
  const { video, audio } = codecsFromInit(init);
  const exact = video && audio ? `${video},${audio}` : null;
  const candidates = [exact, video, "avc1.4d401f", "avc1.42e01e", "avc1.64001f", "avc1.640028"].filter(Boolean);
  for (const c of candidates) {
    const mime = `video/mp4; codecs="${c}"`;
    if (MediaSource.isTypeSupported(mime)) return { mime, codecs: c, video, audio, fallback: c !== exact };
  }
  return null;
}

ui.mseGo.addEventListener("click", () => {
  const url = ui.mseurl.value.trim();
  if (!url) return;
  ui.streamResult.textContent = `边下边播中… ${url}`;
  playViaMSE(url)
    .then(({ video, total, initBytes, mime }) => {
      ui.resultTitle.textContent = `${url}（边下边播）`;
      ui.result.innerHTML = "";
      ui.result.appendChild(video);
      ui.streamResult.textContent = `✓ 边下边播就绪：${(total / 1048576).toFixed(2)}MB / ${mime}`;
      log(`✓ 边下边播 ${url}：初始化段 ${initBytes} 字节 · 明文 ${(total / 1048576).toFixed(2)}MB · ${mime}`);
    })
    .catch(err => {
      ui.streamResult.textContent = "✗ 边下边播失败";
      log(`✗ 边下边播 ${url} 失败：${err.message}`);
    });
});

// 流式打开（大容器）：选中文件即按需取字节打开，不把容器整体读进内存
ui.filestream.addEventListener("change", () => {
  const f = ui.filestream.files?.[0];
  if (!f) {
    ui.streamResult.textContent = "流式打开：未选择";
    return;
  }
  ui.streamResult.textContent = `流式打开中…（${f.size} 字节，按需取字节）`;
  openStreamFile(f)
    .then(({ info, elapsed, total, heads }) => {
      ui.streamResult.textContent = `✓ 流式打开：${info.segments} 段 · 明文 ${total} 字节 · ${elapsed}ms`;
      log(
        `✓ 流式打开 ${f.name}（${f.size} 字节容器，未整体入内存）：${info.segments} 段 / 明文 ${total} 字节 / ${elapsed}ms；随机读三处首字节 ${heads.join(" ")}`
      );
    })
    .catch(err => {
      ui.streamResult.textContent = "✗ 流式打开失败";
      log(`✗ 流式打开 ${f.name} 失败：${err.message}`);
    });
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

/**
 * 用例返回值的一种：**没验到**（缺样例之类的前置条件不满足）。
 * 它既不是 true 也不是 false —— 混进"通过"里就是假绿，见文件末尾的统计逻辑。
 */
const SELFTEST_SKIP = "skip";

/**
 * 声明"这条用例的前置条件不满足（通常是缺样例），本次没验到"。
 * samples/ 是 gitignore 的二进制目录，本机有没有样例不该决定自检的颜色 ——
 * 缺样例必须显式跳过，混进"通过"是假绿，混进"失败"又把环境噪声当成回归。
 */
const skipSignal = why => Object.assign(new Error(why), { selfTestSkip: true });

/** 取样例字节；样例不存在就抛 skip 信号。 */
async function fetchSampleOrSkip(url) {
  const head = await fetch(url, { method: "HEAD" });
  if (!head.ok) throw skipSignal(`没有样例容器 ${url}（samples/ 是二进制、已 gitignore）`);
  return new Uint8Array(await (await fetch(url)).arrayBuffer());
}

async function selfTest() {
  const cases = [];
  const push = async (name, fn) => {
    try {
      cases.push([name, await fn()]);
    } catch (error) {
      // 用例可以 throw skipSignal() 主动声明"前置条件不满足"：既不是通过也不是失败。
      if (error && error.selfTestSkip) {
        log(`  ↳ 跳过：${error.message}`);
        cases.push([name, SELFTEST_SKIP]);
        return;
      }
      log(`  ↳ ${name} 抛错：${error.message}`);
      cases.push([name, false]);
    }
  };

  await push("wasm 内核已就绪", () => Boolean(api));

  await push("加密→解密往返一致（中文/emoji）", () => {
    const text = new TextEncoder().encode("往返 🔐 中文 test 123");
    return encryptAndVerify(text, "自检文本").same;
  });

  await push("流式加密：多段产物能被 open/readRange 原样解回（逐字节）", async () => {
    const plain = new Uint8Array(3 * 1024 * 1024 + 7); // 故意不是整数字节，最后一段必须是不满的
    for (let i = 0; i < plain.length; i++) plain[i] = (i * 31 + 7) & 0xff;
    const meta = { ...TEXT_META, originalName: "stream-test.bin" };
    const { blob, value, same } = await encryptStreamAndVerify("自检流式", slices(plain), meta, plain);
    if (value.segments < 4) log(`  段数只有 ${value.segments}（3MB 明文按 1MB 段切，期望 ≥4 段）`);
    return same === true && value.segments >= 4 && Number(value.plainSize) === plain.length;
  });

  await push("流式加密：不规则分片（含空片/1 字节）同样能解回", async () => {
    // 空片与 1 字节片最容易踩到"段边界正好填满"的分支：段被封存后又要开新段。
    const pieces = [new Uint8Array(0), new Uint8Array([0x2a]), new Uint8Array(STREAM_CHUNK), new Uint8Array([0xff])];
    const plain = new Uint8Array(STREAM_CHUNK + 2);
    let at = 0;
    for (const piece of pieces.slice(1)) {
      plain.set(piece, at);
      at += piece.length;
    }
    const { value, same } = await encryptStreamAndVerify("自检流式-不规则分片", pieces[Symbol.iterator](), TEXT_META, plain);
    return same === true && Number(value.plainSize) === plain.length;
  });

  await push("流式打开：与整块打开逐字节一致（含中段/末尾随机读）", async () => {
    // 造一个容器 → 转成 File → 用 openStream 打开（字节按需从 File.slice 取），
    // 与 open(bytes) 的结果逐字节比对。大容器能开、小容器结果一致，两边都要成立。
    const plain = new Uint8Array(2 * 1024 * 1024 + 123);
    for (let i = 0; i < plain.length; i++) plain[i] = (i * 17 + 3) & 0xff;
    const { blob } = await encryptToBlob(slices(plain), { ...TEXT_META, originalName: "stream-open.bin" });
    const file = new File([blob], "stream-open.bin.sccgt", { type: "application/octet-stream" });

    const fileRead = async (offset, length) => new Uint8Array(await file.slice(offset, offset + length).arrayBuffer());
    const { handle, info } = await openStream(fileRead, file.size, file.name);
    const total = Number(info.plainLength);
    let allSame = total === plain.length;
    for (let off = 0; off < total && allSame; off += 512 * 1024) {
      const want = plain.subarray(off, Math.min(off + 512 * 1024, total));
      const got = await readWindow(handle, off, want.length, fileRead);
      allSame = bytesEqual(got, want);
    }
    // 末尾一段最容易错（最后一段不满、段内偏移换算）
    const tail = await readWindow(handle, total - 37, 37, fileRead);
    allSame = allSame && bytesEqual(tail, plain.subarray(total - 37));
    api.close(handle);
    return allSame;
  });

  await push("HTTP Range 流式打开：只取需要的字节（有样例才跑）", async () => {
    // 这条验的是"字节供给可以来自网络"：内核完全不知道字节是哪来的，
    // 同一套 openStream/streamFeed 协议，read 换成 fetch(Range) 即可。
    const url = "samples/stream16.txt.sccgt";
    const head = await fetch(url, { method: "HEAD" });
    if (!head.ok) {
      // ⚠️ 缺样例时**必须返回 skip**：早先这里 return true，于是"样例不存在"
      // 被当成"这条用例通过"计入分子 —— 自检 18/18 里有两项是这么来的假绿。
      log("  ↳ 跳过：没有样例容器（samples/ 是二进制、已 gitignore）");
      return SELFTEST_SKIP;
    }
    const r = await openStreamURL(url);
    // 该样例的明文由 pw-enc-stream.ts 的生成器规则产出：片内每 997 字节写一个 (全局偏移 & 0xff)
    const CH = 1 << 20;
    const expected = g => ((g - Math.floor(g / CH) * CH) % 997 === 0 ? g & 0xff : 0);
    const read = async (offset, length) => {
      const res = await fetch(url, { headers: { Range: `bytes=${offset}-${offset + length - 1}` } });
      return new Uint8Array(await res.arrayBuffer());
    };
    const { handle } = await openStream(read, Number(head.headers.get("Content-Length")), url);
    let same = true;
    for (const off of [0, Math.floor(r.total / 2), Math.max(0, r.total - 4096)]) {
      const got = await readWindow(handle, off, Math.min(4096, r.total - off), read);
      for (let i = 0; i < got.length; i++) {
        if (got[i] !== expected(off + i)) same = false;
      }
    }
    api.close(handle);
    log(`  ↳ Range 打开 ${url}：容器 ${(r.size / 1048576).toFixed(1)}MB，只取 ${(r.bytes / 1048576).toFixed(1)}MB / ${r.requests} 个请求`);
    return same;
  });

  await push("边下边播：分片 MP4 容器经 MSE 起播（有样例才跑）", async () => {
    const url = "samples/sample.4pm.sccgv";
    const head = await fetch(url, { method: "HEAD" });
    if (!head.ok) {
      // 同上：缺样例是"没验到"，不是"验过了"。
      log("  ↳ 跳过：没有分片 MP4 样例容器（samples/ 已 gitignore）");
      return SELFTEST_SKIP;
    }
    const { video } = await playViaMSE(url);
    video.style.display = "none";
    document.body.appendChild(video);
    try {
      await video.play().catch(() => {}); // 自动播放可能被策略拦，下面只看 readyState/进度
      await new Promise(r => setTimeout(r, 800));
      const ok = video.readyState >= 2 && video.currentTime > 0;
      log(`  ↳ 播放状态 readyState=${video.readyState} currentTime=${video.currentTime.toFixed(2)}s`);
      return ok;
    } finally {
      video.remove();
    }
  });

  await push("流式加密的产物能被错口令拒绝", async () => {
    const begun = api.encryptBegin(ui.password.value, TEXT_META);
    if (!begun.ok) return false;
    const handle = begun.value.handle;
    api.encryptWrite(handle, new TextEncoder().encode("secret"));
    const end = api.encryptEnd(handle);
    if (!end.ok) return false;
    const container = new Uint8Array(await new Blob([toU8(end.value.head), toU8(end.value.data), toU8(end.value.tail)]).arrayBuffer());
    const wrong = api.open(container, "definitely-wrong-password");
    if (wrong.ok) api.close(wrong.value.handle);
    return wrong.ok === false;
  });

  await push("整块加密仍可用（向后兼容）", () => {
    const text = new TextEncoder().encode("整块路径仍然要能用");
    const enc = api.encryptBytes(text, ui.password.value, TEXT_META);
    if (!enc.ok) return false;
    return decrypt(new Uint8Array(enc.data), ui.password.value).plain.length === text.length;
  });

  await push("请求压缩必须被明确拒绝（不是静默忽略）", () => {
    // compose 编排层两条路都不支持压缩：传 compression:"zstd" 必须拿到明确报错。
    // 静默忽略的后果是调用方以为自己得到了压缩容器，实际没有 —— 不失败、不告警，
    // 只有体积不对，等发现时数据已经按"压缩过"的预期流转了。
    const begun = api.encryptBegin(ui.password.value, { ...TEXT_META, compression: "zstd" });
    if (begun.ok) {
      // 真给了 handle 说明它接受了压缩请求：那也必须真的压了，否则就是静默忽略
      api.encryptAbort(begun.value.handle);
      return false;
    }
    return /不支持压缩/.test(String(begun.error ?? ""));
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
      const bytes = await fetchSampleOrSkip(s.file);
      const { plain, info } = decrypt(bytes, ui.password.value);
      const ok = matchesMagic(plain, s.magic, s.magicOffset ?? 0);
      if (!ok)
        log(
          `  ${s.label} 前 8 字节：${Array.from(plain.slice(0, 8))
            .map(b => b.toString(16).padStart(2, "0"))
            .join(" ")}`
        );
      return ok && plain.length > 0 && Number(info.segments) >= 1;
    });
  }

  await push("随机读：中段偏移与整体解密一致", async () => {
    const bytes = await fetchSampleOrSkip("samples/sample.4pm.sccgv");
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
    const bytes = await fetchSampleOrSkip("samples/sample.txt.sccgt");
    const opened = api.open(new Uint8Array(bytes), "definitely-wrong-password");
    if (opened.ok) {
      api.close(opened.value.handle);
      return false;
    }
    log(`  错口令返回：${opened.error}`);
    return true;
  });

  // skip 是"条件不满足（缺样例），没验到"，必须与"通过"分开统计：
  // 混进分子就是**假绿**——样例目录是 gitignore 的，本机没有样例时自检会报 18/18，
  // 看上去全绿，其实边下边播和 Range 流式打开一次都没跑过。
  for (const [name, ok] of cases) log(`${ok === SELFTEST_SKIP ? "⏭" : ok ? "✓" : "✗"} ${name}`);
  const failed = cases.filter(([, ok]) => ok !== true && ok !== SELFTEST_SKIP);
  const skipped = cases.filter(([, ok]) => ok === SELFTEST_SKIP);
  const ran = cases.length - skipped.length;
  if (failed.length === 0) log(`自检通过（${ran} 项${skipped.length ? `，另有 ${skipped.length} 项缺样例跳过` : ""}，全部在浏览器内完成）`);
  else log(`自检失败：${failed.map(([n]) => n).join("、")}`);
  ui.metric.textContent = `${ran - failed.length}/${ran} 通过` + (skipped.length ? ` · ${skipped.length} 项缺样例未验` : "");
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

window.encvPreview = {
  get api() {
    return api;
  },
  selfTest,
  encryptToBlob,
  openStream,
  readWindow,
  slices,
  STREAM_CHUNK,
  VERIFY_LIMIT,
  // 诊断用：MSE 起播失败的根因几乎都在 codecs 解析上（2026-09-29 的坑），
  // 把这几个纯函数暴露出来，浏览器里可以直接对某个 init 段试算，不必改代码重跑。
  pickMime,
  codecsFromInit,
  indexOfBytes,
};
