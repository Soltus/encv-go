# enc-preview —— ENCV 容器的 WASM 纯前端预览

**这一页不请求任何后端。** 容器字节来自静态样例或用户选择的文件，
加解密全在浏览器里由 `encv-container.wasm` 完成；页面里若出现
`:2025`、`/stream`、`/api/` 之类的请求，就是回归。

## 起服务

```bash
make wasm                        # 构建 wasm 内核（产出到 app/enc-preview/wasm/）
python3 app/enc-preview/serve.py 5179   # 纯静态托管，无代理
```

访问：`http://localhost:5179/`
（有 `:16666` 预览网关时也可走 `http://localhost:16666/enc-ui/`。）

## wasm 内核

`cmd/encv-wasm-container`（Go → WASM），编译的是**同一份 `internal/v2` 代码**，
所以与 CLI / 主应用完全同质 —— 主线改了，重跑 `make wasm` 即跟随。

导出（全部返回 `{ok, ...}` 或 `{ok:false, error}`）：

| 函数 | 作用 |
|---|---|
| `open(bytes, password)` | 打开容器（BytesSource，不碰文件系统；要求容器整体在内存） |
| `openStream(password, {size})` | **流式**打开：返回 `{handle, ready, need}`，按 need 补齐字节即可 |
| `streamFeed(handle, offset, bytes)` | 把内核声明缺的那段字节喂进去（喂完会自动推进打开流程） |
| `info(handle)` | 容器类型 / segment 数 / 是否分层密钥 / KVI / **明文长度** |
| `readRange(handle, off, len)` | 按**明文偏移**随机读 —— 流式与拖动的基础 |
| `encryptBytes(plain, password, opts)` | 整块加密成 v4 容器（明文/libcipher/容器三份同时在内存里） |
| `encryptBegin(password, opts)` | **流式**加密开始：`{handle}`；`opts.segmentSize` 决定常驻内存上限 |
| `encryptWrite(handle, chunk)` | 喂一片明文，取回已成型密文段（**必须**按顺序追加，不要攒着） |
| `encryptEnd(handle)` | 收尾，交回 `{head, data, tail, containerBytes, plainSize, segments}` |
| `encryptAbort(handle)` | 丢弃会话（取消上传/加密时用） |
| `close(handle)` | 释放 |

`encryptBytes` 写的是主线同一条路径（`crypto.EncryptSegment` + `writer.WriteV4ContainerTo`），
并补齐主线 reader 需要的元数据（manifest 的 `Playlists`、文件名），
实测**主线 `internal/v2/reader` 能原样解开 wasm 产出的容器**（逐字节一致）。

### 流式加密（大附件）

整块 API 的内存账是按**文件大小的倍数**计的：明文 + 密文 + 容器 + 跨 JS↔Go 边界的拷贝，
浏览器里明文约 1GB 就把 wasm 打死（真实报错：
`runtime: out of memory: cannot allocate … (3229417472 in use)` / `fatal error: out of memory`）。
流式这套把明文分片喂进去、密文分片取出来，Go 侧只持有当前一片的缓冲 —— 详见
`internal/v2/writer/stream_v4.go`（段布局复用整块路径同一个 `writeOneSegment`，产出格式与之完全同构）。

前端拼装顺序**恒为**：

```
new Blob([head, ...每一次 encryptWrite 取回的碎块, encryptEnd 的最后一块 data, tail])
```

之所以这么绕：容器头必须先于数据出现，但头里的 manifest 偏移/长度/CRC 只有写完全部数据才知道。

验证脚本 `app/encv-mobile/pw-enc-stream.ts`（真实浏览器，需先 `python3 app/enc-preview/serve.py 5179`）：

```bash
cd app/encv-mobile && SIZES=512,1024 bun pw-enc-stream.ts   # 每个档位分别跑整块与流式
cd app/encv-mobile && SELFTEST=1 bun pw-enc-stream.ts       # 跑页面自带的一键自检
```

实测结论（本机 12GB）：512MB 两条路都能过；**1GB 起整块路径必炸，流式照常产出**。

内容正确性怎么验：

1. 浏览器内 ≤128MB：`readRange` 分窗逐字节回读（`verified: byte-identical`）
2. 浏览器外：`node app/enc-preview/verify-container.mjs <容器> <口令> --pattern`
   —— 加载的是**同一份** `encv-container.wasm`，走的是同一条主线 reader，
   不需要浏览器内存。实测 512MB 逐字节通过；1GB 会因为 wasm 线性内存上限
   在 `info()` 那一步被打死（脚本会明确报出来，不会假装通过）
3. **1GB 级**：把产物落盘后用 `encv decrypt-v2` 解（不受 wasm 内存上限），
   实测 `MATCH 1073741824` —— 这也顺带证明了浏览器产出的容器在 CLI/主线上同样能开

密钥走与主实现同一条分层路径：`encrypt_salt → KEK → UnwrapDEK → DEK`。

## 页面能力

- **浏览器内加密**：输入文本或选文件 → wasm 加密成容器 → 立刻解密回读比对字节
  （"加密成功但没人能解开"这类问题在这里就会暴露），容器可下载
- **样例容器**：视频 / 音频 / 图片 / PDF / 文本 / WPS 六类，点击即解密并渲染
  （视频走 `<video>`，可播放可拖动）
- **自带容器**：选择或拖入 `.sccg*` 文件，填口令即可解密
- **一键自检（11 项）**：wasm 就绪、加解密往返、错口令必拒、各类容器解密结果的 magic 正确、
  随机读一致 —— 全部在浏览器内完成，不依赖后端

## 样例容器（二进制，不入库）

`samples/*.sccg*` 是 464KB 的二进制示例，已在 `.gitignore` 中排除。需要时自己生成：

```bash
go run ./cmd/encv encrypt-v2 <明文文件> --password my-encv_key --output <目录>
# 把产出的 .sccg* 放进 app/enc-preview/samples/ 即可（页面按扩展名识别类型）
```

没有样例时样例按钮会报错，其余能力（浏览器内加密、上传容器解密、自检）不受影响。

### 流式打开（大容器不必整体进内存）

`open(bytes)` 要求容器整体落在 wasm 的线性内存里。实测同一个 **1GB** 容器：

```
流式 openStream   : ok, 明文 1073741824 字节, JS 堆增量    4 MB
整块 open(bytes)  : ok, 明文 1073741824 字节, JS 堆增量 1012 MB
```

协议是**同步**的「声明缺口 → 喂字节 → 重试」，不是回调：

```js
let r = api.openStream(pw, { size: file.size });          // {handle, ready:false, need:[{offset,length}]}
while (r.need?.length) {
  for (const { offset, length } of r.need) {
    const bytes = new Uint8Array(await file.slice(offset, offset + length).arrayBuffer());
    r = api.streamFeed(handle, offset, bytes);            // 喂完就地推进
    if (r.value?.ready) break;
  }
}
```

⚠️ **为什么不让内核直接调 JS 的 `read(offset,length)` 拿 Promise**：Go/wasm 是单线程的，
导出给 JS 的**同步**函数无法在调用帧里挂起等 Promise —— 实测那样做会让 Go 程序直接退出
（之后每个调用都是 `Go program has already exited`）。所以方向反过来：内核只**声明**缺哪些字节。

页面里「容器 URL + Range 流式打开」就是这条路。1GB 容器实测：

```
✓ Range 流式打开：1024 段 / 明文 1024.0MB / 只下载 3.1MB（10 个 Range 请求）· 138ms
```

即**网络流量只有文件的 0.3%**。

请求数从最初的 **1031 降到 10**，靠两处（都是同一类根因）：

1. 内核求"明文总长"原本要读遍段头 → 改成抽样首段+末段（开销一致就统一推算）。
2. 主线 `SegmentSeekableReader` 构造时逐段读段头算各段明文长度 → 加 `samplePlainSizes`
   抽样首/中/末三段推统一开销，不适用时退回原路径（行为不变）。

⚠️ **两处都卡在同一个坑上**：把"字节还没供给"当成普通失败、退回兜底路径。
抽样时缺字节就放弃 → 每次重试只多命中一个段头 → O(n²)：实测 1GB 容器触发
**13 万次段头读取**。正确做法是把它**原样上抛**（主线新增 `reader.NeedBytesError`
约定，内核的 `errNeedBytes` 实现它），让字节补进来后重试。

样例（二进制、已 gitignore，需要自己挂）：

```bash
ln -sf /path/to/1GB.sccgt app/enc-preview/samples/big.sccgt
```

⚠️ 静态服务必须支持 Range，否则 `fetch(Range)` 会拿到整个文件：
`app/enc-preview/serve.py` 已经实现了（206 + Content-Range）。

配套实现要点（都踩过坑）：
- 明文长度不能再靠"解密后累加"求（那会把整个容器读一遍）：无压缩的段按段头算术求长，
  见 `opened.segmentPlainLength`；声明了 zstd 的段才退回完整读取。
- 喂进来的字节缓存按**字节预算**（64MB）淘汰，不能按块数 —— `info()` 会逐段读段头，
  1024 段的容器按块数淘汰会把前面的段头挤掉，于是前端永远喂不完。
- `streamFeed` 拿 `length` 之前必须先确认它是数字：JS 侧忘写 `await` 传进 Promise 的话，
  `Value.Int()` 会 panic，而 wasm 里一次 panic 会打死整个内核实例。

## 已知边界

- wasm 加密走**writer 层**，产物是「v4 segment 栈」的合法容器（每段独立随机 nonce）。
  主线 `reader.OpenV4Container + NewSegmentSeekableReader` 能解（浏览器/`verify-container.mjs`
  走的都是这一条）。
  2026-09-29 起 **CLI `decrypt-v2` 也能解**（实测 16MB / 512MB 逐字节一致）：
  CLI 的插件路径是「fragment 栈」，整条逻辑流只认 KVI 里那一个 iv，算不出每段的 keystream ——
  以前会**不报错**地解出一个长度正确、内容全是乱码的文件。修法是给每个分片带上它自己的
  nonce（`types.Fragment.Nonce` ← `container/handle.AdaptV4ToV2` 从 v4 segment 复制），
  读取端按分片重置 keystream；分片没有 nonce 时行为与过去完全一致。
  回归锁：`internal/v2/reader/factory_nonce_stack_test.go`。
- **流式路径不支持 zstd 压缩**：seekable zstd 要随机访存整段数据，与"一片进一片出"冲突。
  需要压缩时只能走整块路径（CompressionMode 由调用方在整块路径侧决定）
- **回读比对有体积上限**（`VERIFY_LIMIT = 64MB`）：超过之后页面不再把容器整体读进内存做比对，
  只报容器大小/段数。大体积的正确性由 `pw-enc-stream.ts` 在 `VERIFY_MAX_MB` 以内做逐字节回读
- ~~**打开侧仍是整块的**~~：**已解决（2026-09-29）**，见上面「流式打开」：`openStream`
  + `streamFeed` 按区间供给字节，1GB 容器的 JS 堆增量从 1012MB 降到 4MB。
  仍剩的是"边下载边播"的最后一公里（把 `read` 接到 HTTP Range / MSE 上，页面里还没接）
- 视频是一次性解密后交给 `<video>` 播放，尚未做 MSE 分段喂流
- 无样例容器时 `samples/*` 相关自检会失败（`samples/` 是二进制、已 gitignore），其余自检项不受影响
