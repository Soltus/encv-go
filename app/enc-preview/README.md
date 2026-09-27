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
| `open(bytes, password)` | 打开容器（BytesSource，不碰文件系统） |
| `info(handle)` | 容器类型 / segment 数 / 是否分层密钥 / KVI / **明文长度** |
| `readRange(handle, off, len)` | 按**明文偏移**随机读 —— 流式与拖动的基础 |
| `encryptBytes(plain, password, opts)` | 加密成 v4 容器（分层密钥 + 主线 writer） |
| `close(handle)` | 释放 |

`encryptBytes` 写的是主线同一条路径（`crypto.EncryptSegment` + `writer.WriteV4ContainerTo`），
并补齐主线 reader 需要的元数据（manifest 的 `Playlists`、文件名），
实测**主线 `internal/v2/reader` 能原样解开 wasm 产出的容器**（逐字节一致）。

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

## 已知边界

- wasm 加密走**writer 层**，不跑插件层：产物是单段、无压缩、无插件 index
  的合法 v4 容器。主线 `reader` 能解；CLI 的**插件**解密路径需要插件 index
  （text 插件会报 `index missing`，已修掉原先的 panic）
- 视频是一次性解密后交给 `<video>` 播放，尚未做 MSE 分段喂流
