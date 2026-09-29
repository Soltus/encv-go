# encv-preview —— ENCV 容器的 WASM 纯前端预览

**这一页不请求任何后端。** 容器字节来自静态样例或用户选择的文件，
加解密全在浏览器里由 `encv-container.wasm` 完成；页面里若出现
`:2025`、`/stream`、`/api/` 之类的请求，就是回归。

## 起服务

```bash
make wasm                        # 构建 wasm 内核（产出到 app/encv-preview/wasm/）
bun app/encv-preview/serve.ts 5179   # 纯静态托管，无代理
```

访问：`http://localhost:5179/`
（有 `:16666` 预览网关时也可走 `http://localhost:16666/encv-ui/`。）

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

验证脚本 `app/encv-mobile/pw-enc-stream.ts`（真实浏览器，需先 `bun app/encv-preview/serve.ts 5179`）：

```bash
cd app/encv-mobile && SIZES=512,1024 bun pw-enc-stream.ts   # 每个档位分别跑整块与流式
cd app/encv-mobile && SELFTEST=1 bun pw-enc-stream.ts       # 跑页面自带的一键自检
```

实测结论（本机 12GB）：512MB 两条路都能过；**1GB 起整块路径必炸，流式照常产出**。

内容正确性怎么验：

1. 浏览器内 ≤128MB：`readRange` 分窗逐字节回读（`verified: byte-identical`）
2. 浏览器外：`node app/encv-preview/verify-container.mjs <容器> <口令> --pattern [--stream]`
   —— 加载的是**同一份** `encv-container.wasm`，走的是同一条主线 reader。
   不加 `--stream` 时容器整体进 wasm，实测上限约 512MB（1GB 会在 `info()` 被打死，
   脚本会明说，不假装通过）；**加 `--stream` 走按需读字节，实测 1GB 逐字节通过**。
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
# 把产出的 .sccg* 放进 app/encv-preview/samples/ 即可（页面按扩展名识别类型）
```

没有样例时样例按钮会报错，其余能力（浏览器内加密、上传容器解密、自检）不受影响。

### 边下边播（MSE）

容器字节经 HTTP Range 按需取、解密后**边解边 append 给 MediaSource**，不整体下载。

前提：容器里的视频必须是**分片 MP4（fMP4，含 moof）**。实测源用
`ffmpeg -movflags +frag_keyframe+empty_moov+default_base_moof` 产出时，
加密→解密后 fMP4 结构完整保留（插件按大小切片、不重新 remux），
所以**不需要改插件**。不是 fMP4（ftyp+moov+mdat）时页面会明确报错，
提示改走"整体解密后播放"。

页面入口：「分片 MP4 容器（边下边播 / MSE）」+「边下边播」按钮。
自检里有一条用例锁它（readyState>=2 且 currentTime 前进）。

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
ln -sf /path/to/1GB.sccgt app/encv-preview/samples/big.sccgt
```

⚠️ 静态服务必须支持 Range，否则 `fetch(Range)` 会拿到整个文件：
`app/encv-preview/serve.ts` 已经实现了（206 + Content-Range）。

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
- **MSE 的 mime 必须把音轨写出来**（2026-09-29 修）：容器里有 AAC 音轨时，只声明
  `video/mp4; codecs="avc1.…"` 会让 SourceBuffer 直接报错，而这条报错**只落在
  `video.error.message`** 上（`audio object type 0x40 does not match what is specified
  in the mimetype`）—— `appendBuffer` 不抛错、`updateend` 照常触发，
  所以**只看 append 成功与否**会把失败误判成「边下边播已就绪」。
  页面现在从 init 段解析 `avcC` 与 `esds` 拼成 `avc1.X,mp4a.40.Y`；
  ⚠️ 两个易错点：avcC 之前可能有 `pasp`/`btrt` 等可选 box（不能写死偏移 86），
  esds 的描述符长度是 ISO/IEC 14496-1 **可变长**（ffmpeg 实测写成 `80 80 80 17`），
  当单字节读会把 `objectTypeIndication` 的位置算错 → 读不出音频 codecs。
- **Range 流式打开省下的流量取决于容器的段结构**：多段（segment 栈）容器只取需要的区间；
  单段 / fragment 栈的容器只能**顺序解到目标偏移**，实测 16MB 的文本容器做三次随机读
  就把 **16MB 全取了**（20 个 Range 请求）—— 这条路径上「按需取字节」不成立，
  别拿它当省流量的证据。
- **流式路径不支持 zstd 压缩**：seekable zstd 要随机访存整段数据，与"一片进一片出"冲突。
  需要压缩时只能走整块路径（CompressionMode 由调用方在整块路径侧决定）
- **回读比对有体积上限**（`VERIFY_LIMIT = 64MB`）：超过之后页面不再把容器整体读进内存做比对，
  只报容器大小/段数。大体积的正确性由 `pw-enc-stream.ts` 在 `VERIFY_MAX_MB` 以内做逐字节回读
- ~~**打开侧仍是整块的**~~：**已解决（2026-09-29）**，见上面「流式打开」：`openStream`
  + `streamFeed` 按区间供给字节，1GB 容器的 JS 堆增量从 1012MB 降到 4MB。
- ~~**边下载边播未接通 / 未做 MSE 分段喂流**~~：**已解决（2026-09-29，`a12acd4`）**：
  容器内视频符合要求时 `read` 走 HTTP Range、`MediaSource` 边解边 append 起播
  （页面自检 18/18，新增一条用例锁 MSE 起播）。
  ⚠️ 前提是**容器里的视频必须是分片 MP4（含 `moof`）**：源要用
  `ffmpeg -movflags +frag_keyframe+empty_moov+default_base_moof` 产出；插件按大小切片、
  不重新 remux，fMP4 结构因此能穿过加解密保留下来。不是 fMP4 时页面**明确报错**，
  并提示改走整体解密播放。
  仍剩：**非 fMP4 容器没有自动转封装兜底** —— 唯一出路是先用 ffmpeg 把源 remux 成 fMP4 再加密
  （别指望 mkvmerge：v92 的 `--cues` 只认数字轨道 ID，见 memory 2026-09-28）。
- **自检的「跳过」不是「通过」**（2026-09-29 修）：`samples/` 是二进制、已 gitignore，
  没有样例时依赖样例的用例显示 ⏭ 跳过，**不计入分子** —— 报告形如
  `8/8 通过 · 10 项缺样例未验`。
  ⚠️ 此前这些用例在缺样例时 `return true`，自检照样报 18/18，其中 10 项一次都没跑；
  正是这个假绿，把下面这些缺陷一直藏到 2026-09-29 补齐样例后才暴露出来。
- 补齐样例（本机已装 ffmpeg 7.1，六类源可全量现造）：

  ```bash
  # 1) 视频**必须**是分片 MP4，否则「边下边播」那条用例跑不了（见「已知边界」）
  ffmpeg -y -f lavfi -i testsrc=size=320x240:rate=25:duration=3 \
         -f lavfi -i sine=frequency=440:duration=3 -c:v libx264 -pix_fmt yuv420p -c:a aac \
         -movflags +frag_keyframe+empty_moov+default_base_moov -shortest /tmp/src/sample.mp4
  ffmpeg -y -f lavfi -i sine=frequency=440:duration=3 -write_id3v2 1 -c:a libmp3lame /tmp/src/sample.mp3
  # 2) txt / 最小 PNG / 最小 PDF / 最小 docx 用 python 造（或拿任意真实文件替代）
  # 3) 加密成容器 —— CLI 按「扩展名反转」给产物命名，正好是页面 SAMPLES 里的名字：
  #    sample.mp4 → sample.4pm.sccgv，sample.mp3 → sample.3pm.sccga，sample.png → sample.gnp.sccgi …
  go run ./cmd/encv encrypt-v2 /tmp/src -p my-encv_key -o app/encv-preview/samples
  # 4) 「HTTP Range 流式打开」要的 16MB 文本容器（内容按 pw-enc-stream.ts 的生成器规则）同理，
  #    加密后落到 samples/stream16.txt.sccgt
  ```

  补齐后自检应为 18/18，其中「边下边播」会打出真实播放状态（`readyState=4 currentTime>0`）。
