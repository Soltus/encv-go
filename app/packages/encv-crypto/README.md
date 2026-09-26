# @encv/crypto

ENCV 加密层的**前端唯一入口**：`Worker + Go→WASM`。

给 VuePress 插件、Obsidian 插件、思源笔记插件用同一套加解密，**不需要、也不允许各写一套**。

## 为什么是 Go→WASM，而不是用 WebCrypto 重写

| 方案 | 后果 |
| --- | --- |
| 每个插件用 WebCrypto 自己实现一遍 | 迭代次数、盐长、IV 推导、口令校验迟早漂移；一端改了另一端读不出历史数据 |
| **把 Go 加密层编译成 WASM**（本方案） | 前端与 CLI / 服务端跑的是**同一份 `internal/v2/crypto` 代码**，字节级对等是「结构上必然」而不是「靠测试保证」 |

具体链路：

```
internal/v2/crypto        （PBKDF2-SHA256 / AES-CTR / PasswordHint，唯一实现）
        │
        ├── Go 原生（CLI、服务端）
        └── cmd/encv-wasm  ── GOOS=js GOARCH=wasm ──▶ encv.wasm ──▶ Worker / 主线程
                    │
                    └── internal/v2/crypto/simple（ENCVS1 轻量信封）
```

再加一道**契约锁**：`internal/v2/crypto/simple/testdata/vectors.json`
由 `go run ./cmd/encv-crypto-vectors` 生成，Go 测试与前端测试读同一个文件，
两端必须逐字节一致（见 `test/parity.test.mjs`）。

## 构建

```bash
# 仓库根目录（需要 Go 1.24+）
make wasm            # = bash scripts/build-wasm.sh
```

产物（`app/packages/encv-crypto/wasm/`，已 gitignore）：

- `encv.wasm`（3.3MB，gzip 后约 950KB）
- `wasm_exec.js`（浏览器/Worker 用）
- `wasm_exec_node.js`（Node 测试用）

## 信封格式 ENCVS1

完整容器（`.sccg*`）带 2048 字节信封头 + Manifest + Segment + CRC，对「一条笔记」过重，
且依赖 os 文件语义、无法在浏览器直接复用。ENCVS1 只留加密层本身（小端）：

| Offset | Size | 字段 | 说明 |
| --- | --- | --- | --- |
| 0 | 4 | Magic | `ENCS`（刻意区别于容器的 `ENCV`，容器检测器不会误判） |
| 4 | 1 | FormatVersion | `1` |
| 5 | 2 | CipherMode | 0=AES-128-CTR，1=AES-256-CTR（同 `crypto.CipherMode_v4`） |
| 7 | 1 | KDF | 0=PBKDF2-HMAC-SHA256 |
| 8 | 4 | Iterations | 10000（= `crypto.Iterations_v2`，不可配置） |
| 12 | 1 | SaltLen | 默认 16（= `types.SaltSize_v2`） |
| 13 | 1 | IVLen | 16 |
| 14 | 1 | HintLen | 16 |
| 15 | 16 | PasswordHint | `crypto.CalculatePasswordHint`，与 v4 容器同一算法 |
| 31 | S | Salt | |
| 31+S | 16 | IV | |
| 31+S+16 | rest | Ciphertext | AES-CTR，无填充，长度 == 明文 |

性质：

- **无认证**：CTR 不防篡改（与 ENCV 容器一致）。口令错误由 `PasswordHint` 检出，返回明确错误而非乱码。
- **迭代次数不可配**：显式传入非 10000 会直接报错，绝不静默降级成 10000（避免安全陷阱）。

## API

```ts
import { createCryptoClient } from "@encv/crypto";

const encv = await createCryptoClient({
  loadWasm: () => fetch("/encv-crypto/encv.wasm").then((r) => r.arrayBuffer()),
  loadExecSource: () => fetch("/encv-crypto/wasm_exec.js").then((r) => r.text()),
  createWorker: () => new EncvWorker(), // 可选；不给则主线程模式
});

const blob = await encv.encryptText("# 机密笔记", password);   // Uint8Array
const text = await encv.decryptText(blob, password);           // string
const b64  = await encv.encryptTextToBase64("正文", password); // 适合塞进 Markdown
await encv.verifyPassword(blob, password);                     // 只判密码
await encv.deriveKey(password, salt, 32);                      // PBKDF2，与 Go 同一实现
await encv.parseHeader(blob);                                  // 元信息
encv.dispose();
```

`createWorker` 不给时退化为主线程模式：PBKDF2 10000 迭代处理一条笔记约 10ms，UI 无感。

## 三种宿主的接入

### VuePress / Vite（`@encv/crypto/adapters/vuepress`）

```ts
import EncvWorker from "@encv/crypto/worker?worker";
import { createVuePressCrypto } from "@encv/crypto/adapters/vuepress";

const encv = await createVuePressCrypto({
  base: "/encv-crypto/",               // 把 wasm/ 下的产物拷进 .vuepress/public/encv-crypto/
  createWorker: () => new EncvWorker(),
});
```

> 若插件跑在 Node（SSR / 构建期），不要传 `createWorker`：主线程模式无需 Worker/DOM 之外的能力。

### Obsidian（`@encv/crypto/adapters/obsidian`）

Obsidian 插件目录里的文件不能随便 `fetch`，所以由调用方注入读取函数：

```ts
import { createObsidianCrypto } from "@encv/crypto/adapters/obsidian";

const encv = await createObsidianCrypto({
  dir: `${this.manifest.dir}/wasm`,
  readBinary: (p) => this.app.vault.adapter.readBinary(p),
  readText: (p) => this.app.vault.adapter.read(p),
});

const cipher = await encv.encryptText(editor.getValue(), password);
await this.app.vault.createBinary("notes/secret.encs", cipher);
```

（默认主线程模式：esbuild 单文件产物拆 Worker 需要额外配置；需要后台线程就自己打包后通过 `createWorker` 注入。）

### 思源笔记（`@opencv/crypto/adapters/siyuan`）

```ts
import { createSiYuanCrypto } from "@encv/crypto/adapters/siyuan";

const encv = await createSiYuanCrypto({ pluginName: "encv-crypto" }); // /plugins/<name>/encv.wasm
```

## 测试

```bash
make wasm                                   # 构建 wasm + 生成黄金向量
cd app/packages/encv-crypto && pnpm test    # node --test，验证 wasm 与 Go 逐字节一致
bash scripts/test-go.sh ./internal/v2/crypto/simple/   # Go 侧读同一份向量
```

8 个用例覆盖：常量一致、密钥派生、信封字节、解密还原、中文/emoji 往返、密码错误、
两种 CipherMode、坏参数不打死 wasm 实例。

## 已知取舍 / TODO

- `encv.wasm` 3.3MB：其中约 1MB 来自 `internal/v2/crypto` 间接依赖的 zstd/cbor
  （`internal/v2/types` 引入）。后续可把 PBKDF2/AES/HMAC 抽成叶子包 `crypto/primitive`
  再让 `crypto` 委托，预计降到约 2.3MB。**不建议**为了瘦身而另写一份 TS 实现。
- 目前只做「整块加解密」。笔记场景够用；大附件流式加解密留待后续。
