
# ENCV v2 Writer 架构设计

ENCV v2 的 `writer` 包旨在为不同的打包需求提供一套灵活、健壮且高效的工具。它遵循明确的职责分离原则，确保无论是简单的单文件生成还是复杂的多文件分片，都能得到正确和可靠的实现。

### 核心设计原则

#### 1. 策略模式：应对多样的打包需求

容器并非只有一种形态。我们通过 `PhysicalPacker` 接口定义了打包行为的抽象，并提供了两种核心实现：
*   **`SinglePhysicalPacker`**：将所有数据（KVI、Data、Manifest）顺序写入一个单一的 `.sccgv` 文件。这是最简单、最常见的容器形式。
*   **`FileChunkerPhysicalPacker`**：将数据流拆分成多个物理文件（一个主 `.sccgv` 文件 + N 个 `.part` 文件）。这适用于超大文件或需要分布式存储的场景。
这种设计使得上层调用者（如 `Encrypt` 插件）可以根据配置轻松切换打包策略，而无需关心具体实现。

#### 2. 构建器模式：简化单文件容器的创建

对于最常见的单文件容器，手动管理块头、CRC、偏移量和 Footer 是繁琐且易错的。`SingleFileContainerWriter` 提供了一个流畅的、类似构建器的 API，将这一切复杂性封装起来。

```go
writer := NewSingleFileContainerWriter("output.sccgv")
writer.WriteKVI(kviData)
writer.WriteFragment(frag, data)
writer.WriteManifest(manifest)
writer.Close() // 在这里自动写入 Footer
```

它内部自动维护了数据流偏移、全局 CRC 计算，并确保了最终生成的文件结构完全符合规范。

#### 3. 组合优于继承：实现灵活的分片写入

物理分片打包的逻辑比单文件复杂得多，它需要将数据写入不同的文件句柄。直接复用 `SingleFileContainerWriter` 会导致状态管理冲突（我们修复过的那个 Bug）。
因此，我们采用了组合模式，将能力拆分为更小的、可复用的组件：

* **`ChunkedContainerWriter`**：一个无状态的工具集，提供 `WriteDataChunk` 和 `WriteManifestAndFooter` 等高级方法。它负责“如何写一个块”和“如何写结尾”，但不关心“写到哪里”。
* **`FileChunkerPhysicalPacker`**：作为“导演”，它控制整个流程。它决定哪个数据块写入主文件，哪个写入 `.part` 文件，并调用 `ChunkedContainerWriter` 来完成实际的写入和哈希更新。
  这种设计赋予了 `FileChunkerPhysicalPacker` 完全的控制权，同时复用了底层的写入逻辑，既灵活又高效。

### 健壮性与原子性

#### a. 端到端的数据完整性

无论是 `SingleFileContainerWriter` 还是 `ChunkedContainerWriter`，它们都会为每个数据块计算 CRC32，并将其记录在 Manifest 中。此外，它们还会计算整个容器（除 Footer 外）的全局 CRC32 并写入 Footer。这提供了双重校验，确保了从单个块到整个文件的完整性。

#### b. 原子文件操作

为了防止在打包过程中因程序崩溃或断电而产生损坏的文件，我们采用了“写入临时文件 + 原子重命名”的模式。

* `SingleFileContainerWriter` 写入到 `output.sccgv.tmp`，在 `Close` 时重命名为 `output.sccgv`。
* `FileChunkerPhysicalPacker` 对主文件和所有 `.part` 文件都采用此策略。
  这确保了在任何时刻，磁盘上要么是一个完整的旧文件，要么是一个完整的新文件，绝不存在“半成品”。

### 三位一体的写入器：清晰定位，各司其职

为了更好地理解它们的关系，我们可以用以下比喻来定位这三个核心组件：

#### **`SingleFileContainerWriter` (专用工具)**

* **目标**：将所有内容**一次性地**、**顺序地**写入一个自包含的文件。
* **核心**：**简单与封装**。它隐藏了所有内部细节，提供一个“傻瓜式”的接口。
* **使用场景**：生成标准的、独立的加密容器文件。
* **接口**：`ContainerWriter_v2` (`WriteKVI`, `WriteFragment`, `WriteManifest`, `Close`)。

#### **`ChunkedContainerWriter` (通用工具集)**

* **目标**：为复杂的打包任务提供**可复用的、无状态的**写入原语。
* **核心**：**灵活与组合**。它不控制流程，只提供高质量的“工具函数”。
* **使用场景**：被 `FileChunkerPhysicalPacker` 等高级编排器所使用。
* **接口**：`WriteDataChunk`, `WriteManifestAndFooter`。

#### **`FileChunkerPhysicalPacker` (高级编排器)**

* **目标**：**协调**一个复杂的多文件打包过程，决定数据的最终去向。
* **核心**：**控制与决策**。它掌握着打包的业务逻辑。
* **使用场景**：需要将大文件拆分为多个物理分片时。
* **接口**：`PhysicalPacker.Pack`。

```plaintext
高层应用 (如 Encrypt Plugin)
↓ 调用
PhysicalPacker 接口
↓ 由…实现
SinglePhysicalPacker / FileChunkerPhysicalPacker
↓ 依赖
SingleFileContainerWriter / ChunkedContainerWriter
↓ 依赖
os.File / block.WriteBlock_v2 (系统资源与底层实现)
```

**它们三者共同构成了一个既能处理简单场景、又能驾驭复杂需求的、强大且可靠的写入解决方案。**

