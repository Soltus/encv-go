# TapTap Maker MCP 探索报告

> 探索日期：2026-07-09
> 探索目标：了解 TapTap Maker 本地开发模式的 MCP 工具能力，包括美术资产生成、手机预览等功能

## 一、概述

TapTap Maker 是 TapTap 推出的本地游戏开发平台，通过 MCP（Model Context Protocol）协议与 AI 客户端集成，提供游戏开发全流程的工具支持。

### 核心能力

| 能力类别 | 工具名称 | 说明 |
|---------|---------|------|
| **项目状态** | `maker_status_lite` | 检查本地 Maker 项目状态 |
| **构建/预览** | `maker_build_current_directory` | 提交代码 + 远程构建 + 手机预览 |
| **图片生成** | `generate_image` | 单张游戏美术资产生成 |
| **批量图片** | `batch_generate_images` | 批量生成游戏美术资源 |
| **图片编辑** | `edit_image` | 编辑现有项目图片 |
| **视频生成** | `create_video_task` | 创建视频生成任务 |
| **视频查询** | `query_video_task` | 查询视频任务状态 |
| **音乐生成** | `text_to_music` | 游戏音乐/音效生成 |
| **3D模型生成** | `create_3d_model_task` | 3D 模型资产生成 |
| **3D模型查询** | `query_3d_model_task` | 查询 3D 模型任务状态 |

## 二、环境配置

### 2.1 安装 Maker MCP

```bash
npx -y @taptap/maker install --ide codex,cursor,claude
```

### 2.2 初始化项目

```bash
# 交互式初始化（选择已有项目或创建新项目）
npx -y @taptap/maker init

# 直接创建新项目
npx -y @taptap/maker init --create --name "项目名称"
```

### 2.3 授权登录

- CLI 会自动打开浏览器授权页面
- 点击「确认登录」完成授权
- 授权成功后自动获取项目列表

### 2.4 项目结构

```
项目根/
├── .maker-mcp/              # Maker MCP 配置
│   └── config.json          # 项目绑定信息
├── scripts/                 # 游戏脚本（Lua）
│   └── main.lua             # 入口脚本
├── assets/                  # 游戏资源
│   ├── image/               # 图片资源
│   ├── video/               # 视频资源
│   ├── audio/               # 音频资源
│   └── model/               # 3D 模型资源
├── urhox-libs/              # 引擎工具库（只读参考）
├── templates/               # 项目脚手架模板
├── examples/                # 示例代码
└── engine-docs/             # 引擎 API 文档
```

## 三、UI 组件库（urhox-libs/UI）

Maker 内置了一套完整的 UI 组件库，基于 Yoga Flexbox 布局，支持 40+ 组件。

### 3.1 组件列表

| 类别 | 组件 |
|------|------|
| **布局** | `Panel`, `ScrollView`, `SafeAreaView`, `SimpleGrid` |
| **基础** | `Label`, `Button`, `Icon` |
| **数据展示** | `Card`, `Badge`, `Avatar`, `List`, `Table`, `Timeline`, `Tree` |
| **表单** | `TextField`, `Checkbox`, `Toggle`, `Slider`, `Stepper`, `Rating`, `Dropdown`, `ColorPicker`, `DatePicker`, `TimePicker`, `FileUpload` |
| **导航** | `Tabs`, `Breadcrumb`, `Pagination`, `Menu`, `Drawer`, `Accordion` |
| **反馈** | `Modal`, `Alert`, `Toast`, `Popover`, `Tooltip`, `Drawer`, `Skeleton`, `ProgressBar` |
| **其他** | `Divider`, `Chip`, `Carousel`, `RichText`, `Spine`, `EditMenu` |

### 3.2 常用组件示例

#### Button 按钮

```lua
UI.Button {
    text = "点击我",
    height = 44,
    borderRadius = 10,
    backgroundColor = "#06b6d4",
    color = "#ffffff",
    fontSize = 14,
    fontWeight = "bold",
    onClick = function()
        print("按钮被点击了")
    end,
}
```

**注意：** `backgroundColor` 必须使用字符串格式（`"#RRGGBB"` 或 `"rgba(...)"`），**不能用数字**（如 `0xFF06b6d4`），否则会导致 `Style.Lighten` 报错。

#### Card 卡片

```lua
UI.Card {
    padding = 16,
    borderRadius = 12,
    backgroundColor = "#1e293b",
    borderWidth = 1,
    borderColor = "#334155",
    
    UI.Label { text = "卡片标题", fontSize = 16, fontWeight = "bold" },
}
```

#### Tabs 标签页

```lua
UI.Tabs {
    tabs = {
        { key = "overview", label = "总览" },
        { key = "buildings", label = "建筑" },
    },
    activeTab = "overview",
    onTabChange = function(tabKey)
        print("切换到:", tabKey)
    end,
}
```

#### Slider 滑块

```lua
UI.Slider {
    min = 0,
    max = 100,
    value = 50,
    height = 36,
    activeColor = "#8b5cf6",
    trackColor = "#334155",
    thumbColor = "#a78bfa",
    onValueChange = function(val)
        print("当前值:", val)
    end,
}
```

#### ProgressBar 进度条

```lua
UI.ProgressBar {
    width = "100%",
    height = 8,
    value = 0.75,  -- 0-1 之间
    color = "#22c55e",
    backgroundColor = "#334155",
    borderRadius = 4,
}
```

#### Toggle 开关

```lua
UI.Toggle {
    checked = true,
    activeColor = "#22c55e",
    inactiveColor = "#475569",
    thumbColor = "#ffffff",
    onValueChange = function(val)
        print("开关状态:", val)
    end,
}
```

#### Stepper 步进器

```lua
UI.Stepper {
    value = 1,
    min = 1,
    max = 5,
    step = 1,
    onValueChange = function(val)
        print("当前值:", val)
    end,
}
```

### 3.3 布局系统

基于 Yoga Flexbox，支持以下属性：

| 属性 | 说明 | 示例 |
|------|------|------|
| `flexDirection` | 主轴方向 | `"row"` / `"column"` |
| `justifyContent` | 主轴对齐 | `"flex-start"` / `"center"` / `"space-between"` |
| `alignItems` | 交叉轴对齐 | `"flex-start"` / `"center"` / `"stretch"` |
| `flex` | 弹性比例 | `1`, `2` |
| `width` / `height` | 尺寸 | `100`, `"100%"` |
| `padding` / `margin` | 内边距/外边距 | `16` 或 `{ left = 8, right = 8 }` |
| `gap` | 子元素间距 | `12` |
| `flexWrap` | 换行 | `"wrap"` / `"nowrap"` |
| `display` | 显示 | `"flex"` / `"none"` |

### 3.4 组件查找与动态更新

#### 查找组件

```lua
-- 按 id 递归查找（推荐）
local widget = root:FindById("myButton")
```

#### 常用更新方法

```lua
-- Label
label:SetText("新文本")

-- ProgressBar
progress:SetValue(0.75)  -- 0-1

-- Slider
slider:SetValue(50)

-- Toggle
toggle:SetValue(true)

-- 通用：设置单个属性
widget:SetProp("backgroundColor", "#ff0000")

-- 通用：设置多个样式
widget:SetStyle({
    width = 200,
    height = 100,
    backgroundColor = "#1e293b",
})
```

## 四、构建与手机预览

### 4.1 构建流程

`maker_build_current_directory` 工具执行以下操作：

1. **检查远程同步状态** - 确保本地 main 分支与远程同步
2. **提交本地变更** - 自动 commit + push
3. **触发远程构建** - 在 Maker 云端构建游戏
4. **刷新预览** - 更新预览版本
5. **启动日志监听** - 本地 CLI watcher 持续拉取运行日志

### 4.2 构建参数

| 参数 | 类型 | 说明 |
|------|------|------|
| `target_dir` | string | 项目目录（可选，默认 MCP cwd） |
| `entry` | string | 入口 Lua 文件，如 "main.lua" |
| `scriptsPath` | string | 脚本目录，如 "scripts" |
| `message` | string | 提交信息 |
| `files` | string[] | 指定提交的文件（默认全部） |
| `confirm_remote_build_without_submit` | boolean | 仅构建远程版本，不提交本地更改 |
| `timeout_ms` | number | 构建超时时间（默认 10 分钟） |

### 4.3 构建结果

成功返回包含：

```
- maker_url: 预览页面 URL
- preview_refresh_url: 预览刷新 API
- runtime_logs: 运行时日志配置
  - local_file: 本地日志文件路径
  - watch_command: 日志监听命令
- remote_result: 远程构建结果详情
```

### 4.4 手机预览

构建成功后，通过以下方式在手机上预览：

1. 在手机上打开 TapTap Maker App
2. 进入对应项目
3. 点击「预览」按钮
4. 扫码或直接在 App 内打开

## 五、美术资产生成

### 5.1 图片生成 (generate_image)

用于生成单张游戏美术资源。生成的图片自动保存到 `assets/image/` 目录。

**输出格式**：PNG

#### 参数说明

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `prompt` | string | ✅ | 图片描述（中文，建议简短） |
| `name` | string | ✅ | 文件名（不含扩展名） |
| `target_size` | string | ✅ | 最终尺寸，如 "256x256"、"512x512" |
| `aspect_ratio` | string | ❌ | 宽高比，默认 "1:1" |
| `transparent` | boolean | ❌ | 是否透明背景 |
| `reference_images` | string[] | ❌ | 参考图片列表（路径或 data URL） |
| `seed` | number | ❌ | 随机种子，用于复现结果 |
| `thinking_level` | string | ❌ | "minimal" 或 "high" |
| `resolution` | string | ❌ | "0.5K" / "1K" / "2K" / "4K"，默认 "1K" |
| `model` | string | ❌ | "nanobanana" 或 "gpt"（不建议手动设置） |

#### 宽高比与生成尺寸

| 比例 | AI 生成尺寸 | 用途 |
|------|------------|------|
| 1:1 | 1024×1024 | 图标、头像、方形资源（默认） |
| 2:3 | 832×1248 | 竖版海报、角色立绘 |
| 3:2 | 1248×832 | 横版场景、风景图 |
| 16:9 | 1344×768 | 宽屏、游戏场景、视频封面 |
| 9:16 | 768×1344 | 全屏竖版、短视频封面 |

#### 常见游戏资源尺寸

- 图标：64x64 / 128x128 / 256x256
- UI 元素：256x256 / 512x512
- 角色精灵：256x512 / 512x1024
- 纹理贴图：512x512 / 1024x1024

#### 测试结果

**测试用例**：像素风格骑士角色
- prompt: "可爱的像素风格游戏角色，中世纪骑士，正面视角，头盔和盾牌"
- size: 256x256, 1:1, 透明背景
- 耗时：约 30-60 秒
- 输出：`assets/image/knight-pixel_20260708163212.png`

生成结果：可爱的 Q 版像素骑士，带有狮鹫盾牌和红色披风，透明背景，符合游戏美术风格。

### 5.2 批量图片生成 (batch_generate_images)

批量生成多张游戏美术资源，适用于需要一组风格统一的资源时。

### 5.3 图片编辑 (edit_image)

基于现有图片进行修改，支持：
- 本地项目图片路径（`assets/image/` 下）
- 远程 CDN URL
- Data URL

### 5.4 视频生成

- `create_video_task` - 创建视频生成任务
- `query_video_task` - 轮询任务状态
- 视频保存到 `assets/video/` 目录（MP4 格式）

### 5.5 音乐生成 (text_to_music)

生成游戏背景音乐或音效，保存到 `assets/audio/` 目录（MP3/WAV 格式）。

### 5.6 3D 模型生成

- `create_3d_model_task` - 创建 3D 模型生成任务
- `query_3d_model_task` - 轮询任务状态
- 支持 `rig=true` 为两足人形角色添加骨骼和动画
- 输出格式：GLB/FBX + MDL
- 自动提取到 `assets/Meshes`、`assets/Materials`、`assets/Textures`、`assets/Prefabs`

## 六、资源管理规范

### 6.1 目录结构

```
assets/
├── image/        # 图片资源（generate_image 等输出）
├── video/        # 视频资源
├── audio/        # 音频资源
└── model/        # 3D 模型原始文件
```

### 6.2 资源引用

Maker MCP 会自动记录远程资源映射，支持：
- 后续编辑同一资源
- 保持版本追踪
- 本地文件与远程 CDN 对应

## 七、开发工作流

### 7.1 标准开发流程

```
1. 编写游戏脚本 (scripts/main.lua)
2. 生成美术资源 (generate_image / text_to_music)
3. 本地验证 Lua 语法 (maker-lua-lsp)
4. 构建并预览 (maker_build_current_directory)
5. 查看运行日志 (taptap-maker logs watch)
6. 迭代优化
```

### 7.2 Git 工作流

**重要**：Maker 项目使用特殊的 Git 工作流：
- 不要手动创建 feature 分支
- 不要手动 PR/MR
- 使用 `maker_build_current_directory` 统一处理提交/推送/构建
- 构建工具会自动检查远程同步状态

## 八、与现有项目的结合点

### 8.1 SimVerse 游戏化 UI 参考

从 Maker 的 UI 设计模式中借鉴：
- 游戏化按钮样式（底部边框 + 按下塌陷）
- Flexbox 布局系统（Yoga）
- 多级圆角规范
- 毛玻璃背景效果

### 8.2 潜在应用场景

1. **SimVerse 美术资源** - 使用 generate_image 生成 NPC 头像、场景背景
2. **游戏音效** - 使用 text_to_music 生成背景音乐和音效
3. **快速原型验证** - 用 Maker 快速验证游戏玩法原型
4. **移动端预览** - 利用 Maker 的手机预览能力快速测试横屏 UI

## 九、常见问题与踩坑

### 9.1 颜色格式错误：attempt to index a number value

**错误信息：**
```
[string "urhox-libs/UI/Core/Style"]:268: attempt to index a number value (local 'color')
stack traceback:
    [string "urhox-libs/UI/Core/Style"]:268: in field 'Lighten'
    [string "urhox-libs/UI/Widgets/Button"]:126: in method 'Init'
```

**根因：**
`Style.Lighten` / `Style.Darken` 等颜色处理函数只接受 **RGBA table**（`{ r, g, b, a }`），不接受数字格式的颜色（如 `0xFF4fd1c5`）。

`Style.NormalizeColorProps` 函数会自动转换颜色格式，但**只支持字符串和 table**，不支持数字类型：

```lua
-- ✅ 正确：字符串格式（自动转换）
backgroundColor = "#4fd1c5"
backgroundColor = "rgba(255, 0, 0, 0.5)"

-- ✅ 正确：RGBA table 格式
backgroundColor = { 79, 209, 197, 255 }

-- ❌ 错误：数字格式（不会被自动转换）
backgroundColor = 0xFF4fd1c5
```

**修复方案：**
将所有颜色值改为**十六进制字符串**格式（推荐）或 RGBA table 格式。

**支持的颜色格式：**
| 格式 | 示例 | 说明 |
|------|------|------|
| `#RGB` | `"#f00"` | 短格式红色 |
| `#RGBA` | `"#f00f"` | 短格式带 alpha |
| `#RRGGBB` | `"#ff0000"` | 标准 6 位十六进制 |
| `#RRGGBBAA` | `"#ff000080"` | 8 位带 alpha |
| `rgb(r,g,b)` | `"rgb(255, 0, 0)"` | CSS rgb 格式 |
| `rgba(r,g,b,a)` | `"rgba(255,0,0,0.5)"` | CSS rgba 格式 |
| `{r,g,b}` | `{ 255, 0, 0 }` | RGB table |
| `{r,g,b,a}` | `{ 255, 0, 0, 128 }` | RGBA table |

### 9.2 手机预览调试

**错误报告入口：**
手机上运行出错时，TapTap Maker 会自动生成错误报告，包含：
- 错误时间和版本号
- 浏览器/User-Agent 信息
- 完整的 Lua 调用栈

**调试技巧：**
1. 优先检查 `main.lua` 中 CreateUI 函数的按钮颜色配置
2. 确认所有颜色属性使用字符串格式
3. 检查是否有未定义的变量或函数调用

### 9.3 组件查找 API 错误：FindChild 不存在

**错误信息：**
```
attempt to call a nil value (method 'FindChild')
```

**根因：**
UI 组件的查找方法是 `FindById`，不是 `FindChild`。

**正确 API：**
```lua
-- ✅ 正确：按 id 递归查找子组件
local label = uiRoot_:FindById("popLabel")

-- ❌ 错误：FindChild 方法不存在
local label = uiRoot_:FindChild("popLabel")
```

### 9.4 属性更新 API：不要直接赋值 .text / .value

**错误模式：**
```lua
-- ❌ 错误：直接赋值可能不会触发布局更新
label.text = "新文本"
progress.value = 0.5
```

**正确 API 对照表：**

| 组件 | 操作 | 正确写法 |
|------|------|---------|
| **Label** | 设置文本 | `label:SetText("新文本")` |
| **ProgressBar** | 设置进度 | `progress:SetValue(0.75)` |
| **Slider** | 设置值 | `slider:SetValue(50)` |
| **Toggle** | 设置开关 | `toggle:SetValue(true)` |
| **通用** | 设置任意属性 | `widget:SetProp("key", value)` |
| **通用** | 设置多个样式 | `widget:SetStyle({ key1 = val1, key2 = val2 })` |

**说明：** 虽然直接赋值 `widget.props.text = "xxx"` 可能也能工作，但推荐使用官方的 `SetXXX` 方法，因为它们会正确触发布局更新和过渡动画。

### 9.5 Web 端预览 vs 手机端预览

| 环境 | 状态 | 说明 |
|------|------|------|
| **手机 App 预览** | ✅ 正常 | 真机环境，完整功能 |
| **Web 浏览器预览** | ⚠️ 可能卡住 | 可能缺少某些原生能力，UrhoX 引擎加载可能失败 |

**建议：**
- 优先使用手机 App 预览测试
- Web 端仅用于快速查看布局，不作为功能验证标准
- 错误报告以手机端为准

## 十、已知限制

1. **资产生成工具可用性** - 代理工具需要远程 MCP 服务器支持
2. **构建需要联网** - 远程构建依赖 Maker 云端服务
3. **Lua 语言** - Maker 使用 Lua 作为脚本语言，与项目的 TypeScript/Vue 栈不同
4. **UrhoX 引擎** - 基于 UrhoX 引擎，与现有 WebView 架构不同

## 十二、后续探索方向

- [x] 实际调用 generate_image 生成游戏资源
- [x] 在手机上实际预览效果（已验证，修复了颜色格式 bug）
- [x] 收集 UI 库常见问题与踩坑（颜色格式 + API 差异）
- [x] 梳理 UI 组件库（40+ 组件，含常用组件示例）
- [x] 整理组件查找与动态更新 API（FindById, SetText, SetValue 等）
- [x] 记录 Web 端 vs 手机端预览差异
- [x] 简化 demo（精简稳定版，修复 17 个错误）
- [ ] 测试 text_to_music 音乐生成
- [ ] 探索 3D 模型生成能力
- [ ] 研究资源编辑和版本管理
- [ ] 测试 batch_generate_images 批量生成
- [ ] 测试 edit_image 图片编辑功能
- [ ] 研究多参考图风格控制
