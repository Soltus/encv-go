# ENCVS1 轻量信封与前端 WASM 加密

> 适用范围：VuePress 插件 / Obsidian 插件 / 思源笔记插件等前端宿主。
> 目标：三个插件**共用一套加解密实现**，不允许各自用 WebCrypto 重写一遍。

## 1. 背景与问题

完整容器（v2/v3/v4，`.sccg*`）带 2048 字节信封头、Manifest、Segment、CRC 等结构，
且依赖 `os` 文件语义，无法在浏览器里直接复用；对「一条笔记 / 一段文本」也过重。

但若让每个前端插件用 WebCrypto 自己实现「PBKDF2 + AES-CTR」，迟早出现：

- 迭代次数、盐长度、IV 用法、口令校验算法各自漂移；
- 一端改了参数，另一端读不出历史数据；
- 三份代码三份 bug，修一处漏两处。

## 2. 方案：把 Go 加密层编译成 WASM

```
internal/v2/crypto            ← 唯一实现（PBKDF2-SHA256 / AES-CTR / PasswordHint）
        │
        ├── Go 原生（CLI / 服务端）
        └── internal/v2/crypto/simple（ENCVS1 信封）
                 │
                 └── cmd/encv-wasm ── GOOS=js GOARCH=wasm ──▶ encv.wasm
                                                                  │
                          app/packages/encv-crypto（@encv/crypto） │
                            ├── worker.ts（后台线程调度）          │
                            ├── client.ts（主线程门面）            │
                            └── adapters/{vuepress,obsidian,siyuan}.ts
```

**字节级对等是结构上必然的**：浏览器里跑的就是同一份 `internal/v2/crypto` 编译产物，
不是「照着写一遍」，也不靠「跨语言对齐测试」兜底（测试只是防止有人绕过这条路）。

再加一道**契约锁**：

- `cmd/encv-crypto-vectors` 生成 `internal/v2/crypto/simple/testdata/vectors.json`；
- Go 测试（`internal/v2/crypto/simple/simple_test.go`）读它断言本仓实现；
- 前端测试（`app/packages/encv-crypto/test/parity.test.mjs`）读**同一个文件**断言 wasm 产出。

任一侧改了算法参数，两边同时变红。

## 3. ENCVS1 信封格式（v1，小端）

| Offset | Size | 字段 | 说明 |
| --- | --- | --- | --- |
| 0 | 4 | Magic | `ENCS`（刻意区别于容器的 `ENCV`，`detector` 不会误判） |
| 4 | 1 | FormatVersion | 1 |
| 5 | 2 | CipherMode | 0=AES-128-CTR，1=AES-256-CTR（同 `crypto.CipherMode_v4`） |
| 7 | 1 | KDF | 0=PBKDF2-HMAC-SHA256 |
| 8 | 4 | Iterations | 10000（= `crypto.Iterations_v2`） |
| 12 | 1 | SaltLen | 默认 16（= `types.SaltSize_v2`） |
| 13 | 1 | IVLen | 16（= `types.IVSize_v2`） |
| 14 | 1 | HintLen | 16 |
| 15 | 16 | PasswordHint | `crypto.CalculatePasswordHint`，与 v4 容器同一算法 |
| 31 | S | Salt | |
| 31+S | 16 | IV | |
| 31+S+16 | rest | Ciphertext | AES-CTR，无填充，长度 == 明文 |

性质与约束：

- **无认证**：CTR 不防篡改（与 ENCV 容器一致）。口令错误由 PasswordHint 检出，
  返回 `types.ErrWrongPassword`，不返回乱码。
- **迭代次数不可配**：显式传入非 10000 一律报错（`ErrUnsupportedIter`），
  绝不静默降级成 10000——否则调用方以为用了高迭代、实际拿到 10000，是安全陷阱。
- 默认 AES-256-CTR（密钥 32 字节，与 `crypto.GenerateKey` 默认一致）。

## 4. 前端包：@encv/crypto

位置：`app/packages/encv-crypto`（pnpm workspace 已含 `packages/*`）。

- `src/core.ts`：加载 wasm + `wasm_exec.js`，把 `globalThis.encvCrypto` 包成同步 API。
  **不含任何密码学代码**，只有加载与转发。
- `src/worker.ts`：Worker 入口。刻意不 `import` 任何资源，wasm 与胶水由主线程经 `init`
  消息送入，因此同一份 worker 源码能被 Vite / esbuild / rollup 任意打包器处理。
- `src/client.ts`：`createCryptoClient()`，Worker 模式与主线程模式共用同一套 API。
- `src/adapters/*`：三种宿主只负责「怎么把 wasm 读进来」，加解密路径完全一致。

WASM 导出函数统一返回**结果对象** `{ ok, data|text|value|error }` 而不是抛 JS 异常：
异常跨越 `syscall/js` 边界会打死整个 wasm 实例，Worker 里所有后续调用都会失效。
Go 侧对入参做了类型防御，坏参数只让本次调用失败（见测试「坏参数不打死实例」）。

## 5. 构建与验证

```bash
make wasm                                              # 构建 wasm + 生成黄金向量
cd app/packages/encv-crypto && pnpm test                # wasm ↔ Go 逐字节对等（8 用例）
bash scripts/test-go.sh ./internal/v2/crypto/simple/    # Go 侧读同一份向量
```

产物 `app/packages/encv-crypto/wasm/`（3.3MB，gzip 约 950KB）已 gitignore，
`check:all` 中的对等套件在未构建时自动跳过。

## 6. 已知取舍 / TODO

- **体积**：约 1MB 来自 `internal/v2/crypto` 间接依赖的 zstd/cbor（`internal/v2/types` 引入）。
  可把 PBKDF2/AES/HMAC 抽成叶子包 `crypto/primitive` 再让 `crypto` 委托，预计降到约 2.3MB。
  **不要**为了瘦身而在 TS 侧另写一份实现——那正是本方案要消灭的东西。
- **只做整块加解密**：笔记场景够用；大附件流式加解密留待后续。
