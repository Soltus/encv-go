## 插件系统

### 新的、清晰的架构

1. **`internal/v2/plugins/interface.go`** : 定义所有插件必须实现的统一接口。
2. **`internal/v2/plugins/registry.go`** : 插件注册表，负责加载和管理所有插件。
3. **`internal/v2/plugins/generic/`** : 通用插件，处理所有非特殊文件。
4. **`internal/v2/plugins/video/`** : 视频插件，处理视频和字幕。


## 2. 性能优化策略：零拷贝与 I/O

处理大文件（特别是 GB 级别的视频）时，内存管理和 I/O 效率至关重要。

### 2.1 零拷贝原则

 **目标** ：避免将整个文件加载到内存中。

 **实现** ：

* 利用 `io.SectionReader` 对文件句柄进行封装，只保留文件的起始偏移和长度，不持有数据。
* `VirtualSeekableDecryptReader` 实现了 `io.ReaderAt` 和 `io.Seeker`，这允许上层（如 `http.ServeContent`）直接发起 `Seek` 和 `ReadAt` 操作，而无需将整个流预加载。

### 2.2 接口保留陷阱

在 Go 中，将一个多接口对象（如 `io.SectionReader`）转换为 `io.Reader` 往往会丢失其高级特性（如 `ReaderAt`）。

 **反例** ：

```
// ❌ 错误：使用 io.NopCloser 会丢失 ReaderAt 接口
section := io.NewSectionReader(file, 0, 100)
return io.NopCloser(section) 
// 上层代码无法再将其断言为 io.ReaderAt，导致回退到低效路径
```

 **正例** ：

```
// ✅ 正确：显式保留接口
type PooledReader struct {
    io.Reader
    io.ReaderAt
    io.Seeker
    // ...
}
// 构造时显式传入各接口
```

### 2.3 智能缓存策略

并非所有文件都适合流式传输。对于不可随机访问的流或极小文件，内存缓存能提升性能。

 **决策逻辑** ：

1. **检查 `IsSeekable()`** ：如果底层解密器支持 `Seek`（如视频流），则 **直接流式传输** 。
2. **文件大小阈值** ：

* 如果支持 Seek 且文件很大（>1MB），不缓存。
* 如果不支持 Seek 且文件较小（<150MB），缓存以支持 Range 请求。
* 如果文件极大，禁止缓存，防止 OOM。

internal/v2/
├─ provider/                     # Adapter Layer (适配层，对外提供统一接口)
│  ├─ provider.go                # 定义 FileContentProvider 接口
│  ├─ standard_provider.go       # 普通文件适配器 (简单封装 os.Open)
│  ├─ local_provider.go          # 本地容器适配器 (封装 reader.Open)
│  └─ remote_provider.go         # 远程容器适配器 (封装 reader.Open)
│
├─ service/                      # Service Layer (服务层，管理生命周期与配置)
│  ├─ container_manager.go       # (保留) 物理容器重建、分片管理
│  └─ reader_service.go          # (重构) 负责加载 Patch、创建 Source、管理缓存
│
└─ reader/                       # Core Layer (核心层，三层架构实现)
   ├─ open.go                    # (新增/重构) 组装三层的入口 (替代 factory)
   ├─ bulk_decryptor.go          # 工具：基于 Source 批量解密
   ├─ pool.go                    # 工具：文件句柄池 (原 file_handle_pool)
   ├─ temp_file.go               # 工具：临时文件处理
   │
   ├─ source/                    # Layer 1: Physical (物理层 - 数据在哪)
   │  ├─ interfaces.go           # 定义 ContainerSource 接口
   │  ├─ local_source.go         # 实现：本地文件、物理分片映射
   │  ├─ remote_source.go        # 实现：HTTP Range 请求
   │  └─ patch.go                # (新增) 实现：Header/Manifest 补丁合并逻辑
   │
   ├─ crypto/                    # Layer 2: Transform (变换层 - 如何解密)
   │  ├─ interfaces.go           # 定义 FragmentTransformer 接口
   │  └─ transformer.go           # 实现：AES-CTR 流式解密、IV 推导
   │
   └─ strategy/                  # Layer 3: Logic (逻辑层 - 如何消费)
      ├─ interfaces.go           # 定义 DecryptStream 接口
      ├─ seekable_stream.go      # 实现：可寻址策略 (支持 io.Seeker，用于视频)
      └─ concat_stream.go        # 实现：顺序拼接策略 (用于原子文件包)
