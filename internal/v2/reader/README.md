### HTTP 并发解密场景：**全局文件句柄池**

将文件句柄的打开/关闭行为，从 `DecryptReader` 的生命周期中解耦出来，交给一个全局的、智能的资源池管理。

### 本地解密需求：专用的、高性能解密器

通用的 `DecryptReader` 接口为了灵活性，引入了多层抽象（Seek, Read, Fragment切换），这在顺序、全量解密的场景下是不必要的性能开销。

### 分片复杂性封装：**预建索引 + 二分查找**

当 Fragment 数量巨大时，`Seek` 操作会变得非常慢，无法支持高频的随机访问（如视频进度条拖拽）。在 `VirtualSeekableDecryptReader` 初始化时，建立一个用于快速查找的索引。

### 内存和 I/O 性能：**自适应缓冲区 + 预读**

---

#### a. “自动修复”的 `scanPhysicalOffsets` 逻辑

它没有假设文件总是完美的，而是设计了 **双模式自适应扫描** ：

* **模式 1 (Footer 有效)** ：执行标准的块扫描，这是最高效的。
* **模式 2 (Footer 无效)** ：先尝试标准扫描，如果失败，说明文件结构可能已损坏，它会 **优雅降级** ，直接使用 Manifest 中的 `GlobalStartOffset` 作为物理偏移映射。

 **这赋予了我们的解密器“起死回生”的能力** ，能够处理那些 Footer 损坏、但数据流本身尚存的容器。这是**极致健壮性**的体现。

#### b. 带恢复机制的 `GetFragmentReaderSafely`

这个函数是一个完整的 **数据完整性保障系统** ：

* **`verifyFragmentAt`** ：在返回数据前，先校验文件中的 `BlockHeader`（CRC、Length）是否与 Manifest 中的记录匹配。这从根本上防止了读取损坏或被篡改的数据。
* **`findAndOpenFragmentRecovery`** ：当预期的外部 `.part` 文件丢失或无法访问时，它不会立刻失败，而是会 **扫描整个目录** ，通过 CRC 匹配来寻找“失散”的数据块。

 **这为物理分片容器提供了强大的容错能力** ，即使分片文件被重命名或移动，只要还在同一个目录下，就能被找到。

#### c. 健壮的 `readManifestWithFallback`

这个“快速路径 + 回退路径”的模式是处理不确定性的经典范式。它优先尝试最高效的方式（从 Footer 读取），一旦失败，无缝切换到最可靠但较慢的方式（全文件扫描）。这保证了在任何情况下，只要 Manifest 存在，就一定能被读取出来。

### 三位一体的解密器：清晰定位，各司其职

现在，我将用一个更精确的比喻，来定位这三个解密相关的组件，确保它们的关系一目了然。

#### **`BulkDecryptor` (专用工具，非 `DecryptReader`)**

* **目标** ：将整个容器**最快地**从 A 点（加密文件）运输到 B 点（解密文件）。
* **核心** ： **极致吞吐量** 。它不关心运输过程中的细节，只关心最终结果和总耗时。
* **使用场景** ：CLI 命令 `encv decrypt-to`，后台任务等。
* **接口** ：`DecryptToFile(outputPath string) error`，不实现 `io.Reader`。

#### **`VirtualSeekableDecryptReader` (通用组件，实现了 `DecryptReader`)**

* **目标** ：让用户能**瞬间**到达数据流中的任意位置（如视频拖拽）。
* **核心** ： **极低延迟的 `Seek`** 。所有优化都为随机访问服务。
* **使用场景** ：流媒体服务器、在线视频编辑器等需要随机读写的应用。
* **接口** ：`io.Reader`, `io.Seeker`, `io.Closer`。

#### **`SequentialDecryptReader` (通用组件，实现了 `DecryptReader`)**

* **目标** ：将解密后的数据**稳定地、顺序地**送出。
* **核心** ： **标准的流式接口** 。它不提供随机访问，但可以完美集成到任何 Go I/O 生态。
* **使用场景** ：通过 HTTP 响应流式传输、`pipe` 到其他进程（如 FFmpeg）、只支持顺序播放的媒体服务。
* **接口** ：`io.Reader`, `io.Seeker` (返回错误), `io.Closer`。

```plaintext
高层应用 (如 HTTP Server, CLI Tool)
↓ 调用
DecryptReader 接口
↓ 由…实现
VirtualSeekableDecryptReader / SequentialDecryptReader
↓ 依赖
fileContainerReader (核心实现)
↓ 依赖
os.File / globalFileHandlePool (系统资源)
```

---

1. **`DecryptReaderFactory`** 是一个 **调度中心** 。你给它一个容器路径，它：
   * 创建并持有一个 `fileContainerReader` 实例作为“仓库管理员”。
   * 分析容器类型（是否可寻址）。
   * 根据你的请求，为你派发最合适的“列车”：
     * 如果需要随机访问，给你 `VirtualSeekableDecryptReader` (磁悬浮)。
     * 如果只需要顺序流，给你 `SequentialDecryptReader` (传送带)。
2. **`BulkDecryptor`** 是一个 **独立的特种工具** 。当你有“把整个仓库的货一次性运走”的特殊需求时，你直接使用它。它自己会去找“仓库管理员”（`fileContainerReader`）来获取货物。

**它们三者各司其职，互不替代，共同构成了一个完整、强大、且能应对所有场景的解密解决方案。**
