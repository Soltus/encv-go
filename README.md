# encv-go 怡念汐拂

[![zread](https://img.shields.io/badge/Ask_Zread-_.svg?style=flat&color=00b0aa&labelColor=000000&logo=data%3Aimage%2Fsvg%2Bxml%3Bbase64%2CPHN2ZyB3aWR0aD0iMTYiIGhlaWdodD0iMTYiIHZpZXdCb3g9IjAgMCAxNiAxNiIgZmlsbD0ibm9uZSIgeG1sbnM9Imh0dHA6Ly93d3cudzMub3JnLzIwMDAvc3ZnIj4KPHBhdGggZD0iTTQuOTYxNTYgMS42MDAxSDIuMjQxNTZDMS44ODgxIDEuNjAwMSAxLjYwMTU2IDEuODg2NjQgMS42MDE1NiAyLjI0MDFWNC45NjAxQzEuNjAxNTYgNS4zMTM1NiAxLjg4ODEgNS42MDAxIDIuMjQxNTYgNS42MDAxSDQuOTYxNTZDNS4zMTUwMiA1LjYwMDEgNS42MDE1NiA1LjMxMzU2IDUuNjAxNTYgNC45NjAxVjIuMjQwMUM1LjYwMTU2IDEuODg2NjQgNS4zMTUwMiAxLjYwMDEgNC45NjE1NiAxLjYwMDFaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik00Ljk2MTU2IDEwLjM5OTlIMi4yNDE1NkMxLjg4ODEgMTAuMzk5OSAxLjYwMTU2IDEwLjY4NjQgMS42MDE1NiAxMS4wMzk5VjEzLjc1OTlDMS42MDE1NiAxNC4xMTM0IDEuODg4MSAxNC4zOTk5IDIuMjQxNTYgMTQuMzk5OUg0Ljk2MTU2QzUuMzE1MDIgMTQuMzk5OSA1LjYwMTU2IDE0LjExMzQgNS42MDE1NiAxMy43NTk5VjExLjAzOTlDNS42MDE1NiAxMC42ODY0IDUuMzE1MDIgMTAuMzk5OSA0Ljk2MTU2IDEwLjM5OTlaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik0xMy43NTg0IDEuNjAwMUgxMS4wMzg0QzEwLjY4NSAxLjYwMDEgMTAuMzk4NCAxLjg4NjY0IDEwLjM5ODQgMi4yNDAxVjQuOTYwMUMxMC4zOTg0IDUuMzEzNTYgMTAuNjg1IDUuNjAwMSAxMS4wMzg0IDUuNjAwMUgxMy43NTg0QzE0LjExMTkgNS42MDAxIDE0LjM5ODQgNS4zMTM1NiAxNC4zOTg0IDQuOTYwMVYyLjI0MDFDMTQuMzk4NCAxLjg4NjY0IDE0LjExMTkgMS42MDAxIDEzLjc1ODQgMS42MDAxWiIgZmlsbD0iI2ZmZiIvPgo8cGF0aCBkPSJNNCAxMkwxMiA0TDQgMTJaIiBmaWxsPSIjZmZmIi8%2BCjxwYXRoIGQ9Ik00IDEyTDEyIDQiIHN0cm9rZT0iI2ZmZiIgc3Ryb2tlLXdpZHRoPSIxLjUiIHN0cm9rZS1saW5lY2FwPSJyb3VuZCIvPgo8L3N2Zz4K&logoColor=ffffff)](https://zread.ai/Soltus/encv-go)

一个基于 `AES-256-CTR` 的强大命令行工具集/库，用于多种类型文件的加密、解密、流传输，并提供与 OpenList 的无缝代理集成与通用 HTTP / Webdav 服务。90%代码由AI提供。

> [!WARNING]
>
> - 请确保 `ffmpeg` 已安装并添加到系统的环境变量中。
> - 实际命令以 `encv --help` 为准，文档更新不及时。

---

## 📖 用户使用指南

本部分将指导您如何安装、配置和使用 **怡念汐拂** 的各项功能。

### 🚀 安装

如果没有可执行程序资产，您需要从源代码构建 `encv`  ，通常这不需要。

首先生成构建脚本：

```bash
go run ./cmd/encv-makefile
```

然后执行构建：

- Windows: `./build.bat` 或 `./build.ps1`
- Linux: `make build-all`

### ⚙️ 配置

`encv` 自动读取同目录下的配置文件 `config.user.json`。将其放置在可执行文件同级目录下，可以避免每次手动输入参数。

**`config.user.json` 示例:**

```json
{
  "$schema": "https://raw.githubusercontent.com/Soltus/encv-go/main/config.schema.json",
  "password": "my-encv_key，可以使用中文和标点符号✔",
  "output_path": "./output",
  "server": {
    "port": 2025,
    "dir": "/"
  },
    "admin": {
    "port": 1808,
    "password": "123456"
  },
  "proxy": {
     "sites": {
      "pc": {
        "host": "http://localhost:5244" ,
        "description": "电脑上的openlist"
      },
      "vivo": {
        "host": "http://192.168.31.19:5244" , 
        "description": "手机上的openlist"
      }
    }
  },
  "log": {
    "level": "info",
    "file": ""
  },
  "webdav": {
    "port": 1234,
    "root": "webdav",
    "dir": "./",
    "username": "admin",
    "password": "123456"
  },
  "plugin_settings": {
    "video": {
      "ext": ".sccgv",
      "chunk_size_mb": 100,
      "track_extensions": [".ass", ".srt", ".dm.ass", ".vtt"]
    },
    "image": {"ext": ".sccgi"},
    "audio": {"ext": ".sccga"},
    "text": {"ext": ".sccgt"},
    "wps": {"ext": ".sccgwps"},
    "pdf": {"ext": ".sccgpdf"}
  }
}
```

**配置项说明:**

根据 `config.schema.json` 生成。

#### 🔐 全局设置

| 配置键          | 类型        | 描述                                  | 示例值                                      |
| --------------- | ----------- | ------------------------------------- | ------------------------------------------- |
| `password`    | `string`  | 加密/解密主密码，支持中文和特殊字符   | `"my-encv_key，可以使用中文和标点符号✔"` |
| `output_path` | `string`  | 加密后文件的输出目录（相对/绝对路径） | `"./output"`                              |
| `recover`     | `boolean` | 解密时是否覆盖已存在文件              | `false`                                   |

#### 🧩 插件专属设置

| 配置键                                             | 类型        | 描述                        | 示例值                                  |
| -------------------------------------------------- | ----------- | --------------------------- | --------------------------------------- |
| `plugin_settings.video.ext`                      | `string`  | 视频加密容器扩展名          | `".sccgv"`                            |
| `plugin_settings.video.chunk_size_mb`            | `integer` | 视频分片大小（MB，0=禁用）  | `100`                                 |
| `plugin_settings.video.light_main_chunk_enabled` | `boolean` | 启用轻量主分片模式          | `true`                                |
| `plugin_settings.video.track_extensions`         | `array`   | 需关联的字幕/轨道文件扩展名 | `[".ass", ".srt", ".dm.ass", ".vtt"]` |
| `plugin_settings.image.ext`                      | `string`  | 图像加密容器扩展名          | `".sccgi"`                            |
| `plugin_settings.audio.ext`                      | `string`  | 音频加密容器扩展名          | `".sccga"`                            |
| `plugin_settings.text.ext`                       | `string`  | 文本加密容器扩展名          | `".sccgt"`                            |
| `plugin_settings.pdf.ext`                        | `string`  | PDF加密容器扩展名           | `".sccgpdf"`                          |
| `plugin_settings.wps.ext`                        | `string`  | WPS加密容器扩展名           | `".sccgwps"`                          |

#### 🌐 内置 HTTP 服务器设置

| 配置键          | 类型        | 描述                       | 示例值   |
| --------------- | ----------- | -------------------------- | -------- |
| `server.port` | `integer` | HTTP流媒体服务器监听端口   | `2025` |
| `server.dir`  | `string`  | 文件系统根目录（相对路径） | `"/"`  |

#### 🛠️ 管理后台服务器设置

| 配置键             | 类型        | 描述                        | 示例值       |
| ------------------ | ----------- | --------------------------- | ------------ |
| `admin.port`     | `integer` | 管理后台服务监听端口        | `1808`     |
| `admin.password` | `string`  | 管理员登录密码（留空=禁用） | `"123456"` |

#### 🔄 OpenList 代理服务器设置

| 配置键                                   | 类型                | 描述                                                          | 示例值    |
| ---------------------------------------- | ------------------- | ------------------------------------------------------------- | --------- |
| `proxy.sites`                          | `ProxySiteConfig` | 需要代理的OpenList站点（**Token需要访问管理后台设置**） | `""`    |
| `proxy.disable_signature_verification` | `boolean`         | 是否禁用URL签名验证                                           | `false` |

#### 📁 WebDAV 服务器设置

| 配置键              | 类型        | 描述                       | 示例值       |
| ------------------- | ----------- | -------------------------- | ------------ |
| `webdav.port`     | `integer` | WebDAV服务监听端口         | `1234`     |
| `webdav.root`     | `string`  | WebDAV访问路径前缀（路由） | `"webdav"` |
| `webdav.dir`      | `string`  | 映射的文件系统根目录       | `"./"`     |
| `webdav.username` | `string`  | 基础认证用户名             | `"admin"`  |
| `webdav.password` | `string`  | 基础认证密码               | `"123456"` |

#### 📝 日志设置

| 配置键        | 类型     | 描述                                                              | 示例值                |
| ------------- | -------- | ----------------------------------------------------------------- | --------------------- |
| `log.level` | `string` | 日志级别: `debug`, `info`, `warn`, `error`                      | `"info"`            |
| `log.file`  | `string` | 日志文件路径，为空只输出到控制台；指定后文件输出 JSON 格式        | `"encv.log"`        |
| `log.console` | `boolean` | 是否输出到控制台（当前仅做标记，控制台输出始终开启）         | `true`              |

**日志文件说明**：当指定 `log.file` 时，控制台保持带颜色的文本格式输出，日志文件输出结构化的 JSON 格式，便于使用工具（如 `jq`、`logstash`）进行后续分析。

### 关键说明

1. **端口冲突**：所有服务端口（`server.port`/`admin.port`/`proxy.port`/`webdav.port`）必须唯一且未被占用
2. **安全建议**：
   - `proxy.token` 建议通过命令行参数传递而非明文存储
   - `admin.password` 留空时将完全禁用管理后台登录
3. **路径规范**：
   - `server.dir`/`webdav.dir` 支持相对路径（相对于可执行文件）
   - `output_path` 支持绝对路径（如 `C:\encrypted`）或相对路径
4. **插件扩展名**：所有 `plugin_settings.*.ext` 必须以点开头（如 `.sccgv`）

### 💻 核心用法

#### 加密视频

将指定目录下的所有视频文件加密，并处理相关字幕文件。

```bash
# 将 _videos 目录下的所有视频加密到 output 目录
./encv encrypt -o ./output ./_videos
```

#### 解密视频

将加密后的文件恢复为原始格式。

> [!NOTE]
> Go 标准库 `flag` 包的行为：所有 `-` 开头的标志参数（如 `-o`）必须放在位置参数（如文件路径）**之前**。

```bash
# 解密单个文件
./encv decrypt -o ./_decrypt/ "./output/sample.sccgv"
# 解密整个目录下的所有 .sccgv 文件
./encv decrypt -o ./_decrypt/ ./output
```

#### 查看 kvi 信息

> [!NOTE]
> Go 标准库 `flag` 包的行为：所有 `-` 开头的标志参数（如 `-s`）必须放在位置参数（如文件路径）**之前**。

```bash
# 基础用法
./encv kvi "./output/movie.sccgv"
# 保存到文件
./encv kvi -s kvi.json "./output/movie.sccgv"
```

#### HTTP服务

启动一个 HTTP 服务器，支持集成 Webdav 服务。

```bash
# 在 1999 端口启动服务，提供 ./output 目录下的文件
./encv start -port 1999 ./output
```

启用 HTTP 服务后，可以通过 URL 播放服务的路径和磁盘任意位置的加密视频。

> [!WARNING]
> 使用 `mpv` 播放器观看时，需要手动指定字幕文件

```bash
mpv http://localhost:1999/321.sccgv --sub-files=http://localhost:1999/321.ass
```

播放任意位置的视频：

```bash
mpv http://127.0.0.1:1999/stream?file=A%3A%5CLocal%5CCol-Study%5Cgo%5Cencv%5Coutput%5C321.4pm.sccgv
```

### 🔗 OpenList 集成

`encv proxy` 命令启动一个为 OpenList 设计的代理服务，它能透明地解密 encv 加密容器，让 OpenList 可以直接播放加密内容。

> [!WARNING]
> 由于远程服务的特性，OpenList 集成不支持真正的视频寻址，只能发起 Range 请求，但是文件由 OpenList 提供，因此需要先加载对应的物理分片。

**配置步骤:**

1. **构建 `encv`** (见上文安装部分)。
2. **在 OpenList 中配置 WebDAV**:

   * 进入管理页面，找到存储设置。
   * 在【WebDAV 策略】中，选择 **使用代理地址**。如果是第三方网盘还需要勾选上方的 **Web代理**
   * URL 地址填入 `http://localhost:${port}/openlist/sites/${siteid}` (根据你的实际配置修改)。
3. **获取 OpenList 令牌**:

   * 在 OpenList 管理页面的【设置】->【其他】中，滚动到最底部，复制管理员令牌。在 `/openlist/sites` 提交令牌（**令牌会以加密二进制文件的形式存储在 `config.user.json` 文件所在路径**）。
4. **为加密文件添加预览**:

   * 在 OpenList 管理页面的【设置】->【预览】中。
   * 根据配置按类别添加后缀名。
5. **通过 iframe 预览文件**：

   * 在 Openlist 管理页面的【设置】->【预览】中，iframe 预览追加以下配置：

     ```json
     "sccgpdf": {
       "ENCV PDF": "http://localhost:${port}/openlist/sites/${siteid}/_preview/pdf.html?file=$e_url",
     },
     "sccgt": {
        "ENCV Text": "http://localhost:${port}/openlist/sites/${siteid}/_preview/text.html?file=$e_url"
      }
     ```
   * 保存后即可在 OpenList 中预览加密后的 PDF 和文本文件。
   * 文本文件为什么需要 iframe 预览？因为测试过程中发现 OpenList 无法预览 50MB 的大型文本文件，而通过 encv 的 iframe 预览则没有问题。

OpenList Webdav 代理是很好的方式，只需要定义预览后缀名，不影响其他操作。但假如希望在其他平台通过 Webdav 预览加密容器，encv-go 也提供了支持，缺点是仅支持只读模式，而且性能可能远不如 OpenList 。通用 Webdav 服务将显示解密后的原始文件名作为“不存在”的文件，当请求打开时反查真实的加密容器并进行解密。

调试通用 webdav

```bash
curl -v -X PROPFIND http://localhost:2025/webdav/ -H "Depth: 1" -H "Content-Type: application/xml" -d '<?xml version="1.0" encoding="utf-8"?><D:propfind xmlns:D="DAV:"><D:prop><D:displayname/></D:prop></D:propfind>'
```

### Windows 上使用

`register` 命令可以注册加密容器后缀名的右键菜单，`unregister` 反注册。`openas` 命令注册容器后缀名打开方式（双击打开）

### Linux 上使用

#### 安装 ffmpeg

如果还没安装 ffmpeg

1. 下载FFmpeg压缩包

```bash
wget https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linux64-gpl.tar.xz
```

2. 解压文件

```bash
tar -xf ffmpeg-master-latest-linux64-gpl.tar.xz
```

3. 移动到系统目录

```bash
sudo mv ffmpeg-master-latest-linux64-gpl /opt/ffmpeg
```

4. 添加到PATH环境变量

```bash
echo 'export PATH="/opt/ffmpeg/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc  # 立即生效
```

WSL2 已经打通了 localhost

如果 localhost 不行，尝试在 WSL 终端中运行命令 `hostname -I`，获取 WSL 虚拟机的 IP 地址

### 🐛 已知问题

- 非 Openlist Webdav 客户端无法播放加密音频

---

## 🛠️ 开发者指南

本部分面向希望参与项目开发或进行调试的开发者。

### 📁 项目结构

```md
encv-go/
├── cmd/                              # 程序入口
│   ├── encv/                         # encv 主程序
│   ├── encv-makefile/                # 构建脚本生成器
│   ├── encv-schema/                  # 仅用于生成 config.schema.json
│   └── bench-report/                 # 基准测试报告生成器
├── internal/                         # 内部实现
│   ├── admin/                        # 管理后台
│   ├── config/                       # 配置管理
│   ├── container/                    # 加密容器相关
│   ├── middleware/                   # 中间件
│   ├── proxy/                        # OpenList 代理服务核心逻辑
│   ├── register/                     # 服务注册
│   ├── server/                       # 服务器实现
│   ├── service/                      # 业务服务
│   ├── utils/                        # 通用工具
│   ├── v2/                           # v2 版本实现
│   │   ├── chunker/                  # 分块处理
│   │   ├── container/                # 容器实现
│   │   │   ├── block/                # 块处理
│   │   │   ├── detector/             # 检测器
│   │   │   ├── envelope/             # 信封
│   │   │   ├── fragment/             # 片段
│   │   │   └── manifest/             # 清单
│   │   ├── crypto/                   # 加密解密
│   │   ├── handler/                  # 处理器
│   │   ├── namer/                    # 命名管理
│   │   ├── openlist/                 # OpenList 适配
│   │   ├── physical/                 # 物理存储
│   │   ├── plugins/                  # 插件系统
│   │   │   ├── audio/                # 音频插件
│   │   │   ├── image/                # 图片插件
│   │   │   ├── interfaces/           # 插件接口
│   │   │   ├── pdf/                  # PDF 插件
│   │   │   ├── text/                 # 文本插件
│   │   │   ├── video/                # 视频插件
│   │   │   ├── wps/                  # WPS 插件
│   │   │   └── registry.go           # 插件注册
│   │   ├── provider/                 # 提供者
│   │   ├── reader/                   # 读取器
│   │   ├── service/                  # 服务层
│   │   ├── types/                    # 共享数据结构
│   │   └── writer/                   # 写入器
│   ├── web/                          # Web 服务
│   │   ├── static/                   # 静态资源
│   │   │   └── preview/              # iframe 预览页面
│   │   │       ├── pdf.html          # PDF 预览
│   │   │       └── text.html         # 文本预览
│   │   └── preview.go                # iframe 预览服务
│   └── webdav/                       # WebDAV 服务核心逻辑
├── tests/                            # 测试产物（gitignore）
│   └── bench/                        # 基准测试报告与历史缓存
├── pkg/                              # 对外暴露的公共包
│   └── encv/                         # 作为库调用
├── go.mod                            # Go 模块定义
├── README.md                         # 项目说明
├── config.user.json                  # 用户配置文件
└── config.schema.json                # 配置文件模式定义
```

### 🔨 构建

首先生成构建脚本（只需执行一次，或当构建配置更改时）：

```bash
go run ./cmd/encv-makefile
```

然后执行构建：

```bash
# Windows
./build.bat
# 或
./build.ps1

# Linux
make build-all
```

#### 构建 Android AAR (占位)

> [!NOTE]
> 此功能待后续实现。

```bash
gomobile bind -target=android -o encv.aar ./pkg/encv
```

### 🧪 测试

#### 功能测试

```cmd
go run ./cmd/encv start 
```

单元测试

```cmd
# ✅ 推荐：通过统一入口（默认 -short + -failfast）
bash scripts/test-go.sh ./internal/service -v

# ✅ 只跑特定测试
bash scripts/test-go.sh -run "TestContinuousRead|TestRandomSeek" ./internal/service -v

# ✅ 模块化编排：自动遍历所有包 + 单包 -short 跑
bash scripts/test-all-go.sh

# ⚠️ 严禁 go test ./... / ./internal/...（沙箱内 380s+ 必断网）
# 详见 .trae/rules/test-orchestration.md
```

一个简单的测试流程是加密一个示例视频目录：

```bash
# 假设你有一个 _videos 目录
./encv encrypt -o ./_test_output ./_videos
```

然后检查 `_test_output` 目录下的 `.sccgv`, `.kvi` 和字幕文件是否符合预期。

#### 基准测试与性能报告

`bench-report` 工具自动运行基准测试并生成可视化 HTML 报告，包含评分仪表盘、雷达图、功能分类卡片和历史对比。

```bash
# 编译
go build -o bench-report.exe ./cmd/bench-report/

# 运行（跳过集成测试，约 1-2 分钟）
./bench-report.exe --skip-integration

# 运行（包含集成测试，需要 FFmpeg 和 _videos 目录）
./bench-report.exe

# 自定义输出路径
./bench-report.exe -o ./my-report.html

# 自定义测试时长
./bench-report.exe --benchtime 5s --skip-integration

# 不保存历史缓存
./bench-report.exe --no-save --skip-integration
```

**命令行参数：**

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-o` | `tests/bench/bench-report.html` | 输出 HTML 文件路径 |
| `--benchtime` | `2s` | 每项基准测试运行时长 |
| `--skip-integration` | `false` | 跳过 L4/L5 集成测试 |
| `--no-save` | `false` | 不保存本次结果到历史缓存 |
| `--open` | `true` | 生成后自动在浏览器中打开 |

**测试分层：**

| 层级 | 范围 | 说明 |
|------|------|------|
| L1 | 密码学原语 | AES-256-CTR 加解密、PBKDF2 密钥派生、CTR IV 推导 |
| L2 | 容器 I/O | Block 读写、分片管理、临时文件加密 |
| L3 | 解密读取器 | 顺序读取、Seek 跳转、批量解密 |
| L4/L5 | 端到端集成 | 完整加密→解密→播放流程（需 FFmpeg） |

**报告功能：**

- 综合评分（0-100）与 SVG 环形仪表盘
- 5 大功能分类雷达图（密钥派生 / AES-CTR / 容器 I/O / 分片管理 / 解密读取器）
- 自动生成关键发现（最强项、偏慢项、历史提升）
- 评级参考标准（优秀 / 及格 / 偏慢）
- 历史数据缓存与可视化对比
- 产物自动输出到 `tests/bench/` 目录

生成 `config.schema.json` ：

```cmd
go run ./cmd/encv-schema
```

在 `config.user.json` 中测试：

```json
"$schema": "./config.schema.json"
```

### 🐛 调试与技巧

* **依赖检查**: 确保系统已安装 `ffmpeg` 和 `mpv`，并且它们在 `PATH` 环境变量中。程序依赖它们进行视频处理和播放。
* **日志输出**: 程序使用标准的 `log` 包输出信息，可以通过环境变量 `GODEBUG` 或重定向标准输出来进行更详细的调试。
* **Flag 解析顺序**: 再次强调，所有 `-flag` 必须放在命令的最前面，否则 `flag.Parse()` 会提前停止解析，导致后续参数失效。
