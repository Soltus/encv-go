# TRAE Solo Web 全局 MCP 伪装方案 — 项目级 MCP 配置的动态注册

## 适用范围

> ⚠️ **本方案仅适用于 TRAE Solo Web 环境**，不适用于 TRAE IDE / TRAE CLI 等其他 TRAE 产品形态。
>
> 在 TRAE Solo Web 中，`agent-tool-host` 运行在沙箱容器内，不支持项目级 `.trae/mcp.json` 的自动加载。
> 本方案通过逆向工程发现的 gRPC 接口实现等效效果。

## 背景

TRAE Solo Web 当前版本**不支持项目级 MCP 配置**（即 `.trae/mcp.json` 不会被自动加载）。

但 TRAE Solo Web 的 `agent-tool-host` 提供了通过 gRPC 动态管理 MCP 服务器的能力。本方案利用这个能力，将项目级 MCP 配置"伪装"成全局用户级配置，实现项目级 MCP 的等效效果。

## 方案原理

### 核心发现

通过逆向工程 `agent-tool-host` 二进制，定位到以下 gRPC 接口：

| 项 | 值 |
|----|----|
| **gRPC 地址** | `http://127.0.0.1:9000` |
| **服务名** | `command.CommandService` |
| **方法名** | `Execute` |
| **命令名** | `icube.common.commands.tooling.manageMcpServers` |

### 请求参数

```json
{
  "action": "add",
  "mcpServers": {
    "server-name": {
      "command": "node",
      "args": ["/path/to/server.mjs"],
      "env": {
        "KEY": "value"
      }
    }
  }
}
```

- `action`: `"add"` 或 `"remove"`
- `mcpServers`: MCP 服务器配置对象，格式与标准 `mcp.json` 的 `mcpServers` 字段一致

### 响应格式

```json
{
  "code": 0,
  "errorMessage": null,
  "data": null
}
```

`code === 0` 表示成功。

## 使用方法

### 脚本位置

[trae_solo_web_register_mcp.mjs](file:///workspace/scripts/trae_solo_web_register_mcp.mjs)

### 基本用法

```bash
# 注册 .trae/mcp.json 中的所有 MCP 服务器
node scripts/trae_solo_web_register_mcp.mjs

# 显式指定 add 操作
node scripts/trae_solo_web_register_mcp.mjs add

# 移除所有项目级 MCP 服务器
node scripts/trae_solo_web_register_mcp.mjs remove

# 使用指定配置文件
node scripts/trae_solo_web_register_mcp.mjs --config /path/to/mcp.json

# 查看帮助
node scripts/trae_solo_web_register_mcp.mjs --help
```

### 配置文件格式

与标准 MCP 配置文件格式一致：

```json
{
  "mcpServers": {
    "app-dev": {
      "command": "node",
      "args": ["/workspace/scripts/app-dev-mcp.mjs"]
    },
    "web-fetch": {
      "command": "node",
      "args": ["/workspace/scripts/web-fetch-mcp.mjs"]
    },
    "codemogger": {
      "command": "node",
      "args": ["/workspace/app/codemogger-patch/mcp-server.mjs"],
      "env": {
        "CODEMOGGER_ROOT": "/workspace/app/encv-mobile"
      }
    }
  }
}
```

本项目的配置文件位于 [.trae/mcp.json](file:///workspace/.trae/mcp.json)。

## 验证方法

### 1. 检查进程

```bash
ps aux | grep -E "app-dev|web-fetch|codemogger" | grep -v grep
```

成功注册后应能看到对应的 node 进程。

### 2. 检查日志

```bash
grep "MCP server startup committed" /var/log/tool/agent-tool-host.stdout.log | tail -5
```

成功启动的服务器会打印类似日志：
```
INFO mcp_client::server_group::lifecycle: MCP server startup committed source=user server_name=app-dev generation=6 tool_count=15 transport=Stdio pid=Some(2218)
```

### 3. 调用 MCP 工具

通过 `run_mcp` 工具调用（在 AI 对话中）：

```
run_mcp server_name=app-dev tool_name=app_check_all args={"noTests": true}
```

## 注意事项

### 1. 非持久化

通过 gRPC 动态添加的 MCP 服务器**不会持久化**，`agent-tool-host` 进程重启后会丢失。

**应对方案**：每次环境启动后手动运行 `node scripts/trae_solo_web_register_mcp.mjs` 重新注册。

### 2. 原子性

`manageMcpServers` 命令是**原子操作**——所有服务器要么全部成功，要么全部回滚。

如果其中某一个服务器启动失败（比如命令不存在），所有服务器都会被回滚移除。

**应对方案**：确保配置中的所有服务器都能正常启动。如果有不确定的服务器，可以分批注册。

### 3. 幂等性

- 重复 `add` 同一台服务器：会替换（先移除旧的，再启动新的）
- `remove` 不存在的服务器：静默成功

### 4. 与用户级配置的关系

`/data/user/mcp/mcp-servers.json` 是用户级 MCP 配置文件，但当前版本的 TRAE Solo Web 不会自动加载它。必须通过 `MCP_CONFIG_PATH` 环境变量或 gRPC 命令手动触发加载。

本方案选择 gRPC 动态注册的方式，因为它：
- 不依赖环境变量注入
- 不需要修改系统文件
- 可以随时添加/移除
- 对正在运行的会话无侵入

## 排查问题

### 问题：调用 gRPC 失败 / 连接被拒绝

**可能原因**：`agent-tool-host` 未启动或端口不对。

**排查**：
```bash
ps aux | grep agent-tool-host | grep -v grep
ss -tlnp | grep 9000
```

### 问题：服务器启动失败，全部被回滚

**可能原因**：配置中的某一个服务器启动失败。

**排查**：查看日志中的错误详情：
```bash
grep -A 5 "MCP server startup failed" /var/log/tool/agent-tool-host.stdout.log | tail -20
```

**解决**：移除有问题的服务器，或修复其启动命令。

### 问题：工具调用超时

**可能原因**：
- MCP 服务器自身响应慢
- 工具名不正确

**排查**：
```bash
grep "tool_count" /var/log/tool/agent-tool-host.stdout.log | tail -5
```

确认服务器启动时报告的工具数量和名称。

## 技术细节

### Protobuf 编码

请求消息使用 Protobuf 编码，字段定义：

| 字段号 | 字段名 | 类型 | 说明 |
|--------|--------|------|------|
| 1 | command | string | 命令名 |
| 2 | params | string | JSON 字符串格式的参数 |

> 注：params 采用 JSON 字符串而非嵌套 Protobuf message，这是 TRAE Solo Web 内部的设计选择。

### gRPC 帧格式

标准 gRPC over HTTP/2 格式：
```
+------------------------------------+
| 压缩标志 (1 byte) | 长度 (4 bytes, BE) |
+------------------------------------+
|            Protobuf 消息体          |
+------------------------------------+
```

本方案中压缩标志始终为 0（不压缩）。

## 相关文件

- [trae_solo_web_register_mcp.mjs](file:///workspace/scripts/trae_solo_web_register_mcp.mjs) — 注册脚本
- [.trae/mcp.json](file:///workspace/.trae/mcp.json) — 项目级 MCP 配置
- [app-dev-mcp.mjs](file:///workspace/scripts/app-dev-mcp.mjs) — app-dev MCP 服务器
- [web-fetch-mcp.mjs](file:///workspace/scripts/web-fetch-mcp.mjs) — web-fetch MCP 服务器
