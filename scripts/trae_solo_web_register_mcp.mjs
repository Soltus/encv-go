#!/usr/bin/env node
/**
 * TRAE Solo Web 全局 MCP 伪装注册脚本
 *
 * 通过 gRPC 动态调用 agent-tool-host 的 manageMcpServers 命令，
 * 将项目级 .trae/mcp.json 中的 MCP 服务器注册为用户级 MCP 服务器。
 *
 * 背景：TRAE Solo Web 当前不支持项目级 .trae/mcp.json，但支持通过 gRPC
 *       动态管理 MCP 服务器。本脚本封装了这个调用，使得项目级
 *       MCP 配置可以被"伪装"成全局用户级配置。
 *
 * 用法：
 *   node scripts/trae_solo_web_register_mcp.mjs            # 注册 .trae/mcp.json 中的所有服务器
 *   node scripts/trae_solo_web_register_mcp.mjs add        # 同上，显式指定 add
 *   node scripts/trae_solo_web_register_mcp.mjs remove     # 移除 .trae/mcp.json 中的所有服务器
 *   node scripts/trae_solo_web_register_mcp.mjs --config /path/to/mcp.json  # 指定配置文件
 *
 * 原理：
 *   - gRPC 服务: command.CommandService / Execute
 *   - 命令: icube.common.commands.tooling.manageMcpServers
 *   - 参数: { action: "add"|"remove", mcpServers: {...} }
 *   - 地址: http://127.0.0.1:9000
 */

import http2 from "node:http2";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const WORKSPACE_ROOT = path.resolve(__dirname, "..");
const DEFAULT_CONFIG = path.join(WORKSPACE_ROOT, ".trae", "mcp.json");
const GRPC_ENDPOINT = "http://127.0.0.1:9000";
const SERVICE = "command.CommandService";
const METHOD = "Execute";
const COMMAND_NAME = "icube.common.commands.tooling.manageMcpServers";

// ---------- Protobuf 编码（最小实现，仅支持 string + 嵌套 message） ----------

function encodeVarint(value) {
  const buf = [];
  let v = value;
  while (v > 127) {
    buf.push((v & 0x7f) | 0x80);
    v >>= 7;
  }
  buf.push(v & 0x7f);
  return Buffer.from(buf);
}

function encodeString(fieldNum, value) {
  const strBuf = Buffer.from(value, "utf8");
  const tag = (fieldNum << 3) | 2; // wire type 2 = length-delimited
  return Buffer.concat([encodeVarint(tag), encodeVarint(strBuf.length), strBuf]);
}

function buildRequest(command, params) {
  const commandBuf = encodeString(1, command);
  const paramsBuf = encodeString(2, JSON.stringify(params));
  const body = Buffer.concat([commandBuf, paramsBuf]);

  // gRPC 帧头: 1 byte compression flag + 4 bytes length (big-endian)
  const header = Buffer.alloc(5);
  header.writeUInt8(0, 0);
  header.writeUInt32BE(body.length, 1);

  return Buffer.concat([header, body]);
}

// ---------- gRPC 调用 ----------

function callGrpc(service, method, requestBuf) {
  return new Promise((resolve, reject) => {
    const client = http2.connect(GRPC_ENDPOINT);

    client.on("error", (err) => {
      reject(new Error(`gRPC connection error: ${err.message}`));
    });

    client.on("connect", () => {
      const req = client.request({
        ":method": "POST",
        ":path": `/${service}/${method}`,
        "content-type": "application/grpc",
        te: "trailers",
      });

      let data = Buffer.alloc(0);
      let responseHeaders = null;

      req.on("response", (headers) => {
        responseHeaders = headers;
      });

      req.on("data", (chunk) => {
        data = Buffer.concat([data, chunk]);
      });

      req.on("end", () => {
        client.close();

        const status = responseHeaders?.["grpc-status"];
        const message = responseHeaders?.["grpc-message"];

        if (status && status !== "0") {
          const decoded = decodeURIComponent(message || "");
          reject(new Error(`gRPC error ${status}: ${decoded}`));
          return;
        }

        // 跳过 5 字节 gRPC 头
        const payload = data.length > 5 ? data.slice(5) : Buffer.alloc(0);
        resolve({ headers: responseHeaders, payload });
      });

      req.on("error", (err) => {
        client.close();
        reject(new Error(`gRPC request error: ${err.message}`));
      });

      req.write(requestBuf);
      req.end();
    });
  });
}

// ---------- 响应解析 ----------

function parseResponse(payload) {
  // 响应格式: { code, errorMessage, data }
  // 尝试从 payload 中提取 JSON
  const str = payload.toString("utf8");
  const match = str.match(/\{[\s\S]*\}/);
  if (match) {
    try {
      return JSON.parse(match[0]);
    } catch {
      // fall through
    }
  }
  return { raw: str };
}

// ---------- 主逻辑 ----------

function parseArgs() {
  const args = process.argv.slice(2);
  const opts = {
    action: "add",
    config: DEFAULT_CONFIG,
  };

  for (let i = 0; i < args.length; i++) {
    const arg = args[i];
    if (arg === "add" || arg === "remove") {
      opts.action = arg;
    } else if (arg === "--config" && args[i + 1]) {
      opts.config = path.resolve(args[++i]);
    } else if (arg === "-h" || arg === "--help") {
      opts.help = true;
    }
  }

  return opts;
}

function printHelp() {
  console.log(`
TRAE Solo Web 全局 MCP 伪装注册脚本

用法:
  node scripts/trae_solo_web_register_mcp.mjs [action] [--config <path>]

参数:
  action              add (默认) 或 remove
  --config <path>     MCP 配置文件路径（默认: .trae/mcp.json）
  -h, --help          显示帮助

示例:
  node scripts/trae_solo_web_register_mcp.mjs                        # 注册项目级 MCP
  node scripts/trae_solo_web_register_mcp.mjs remove                 # 移除项目级 MCP
  node scripts/trae_solo_web_register_mcp.mjs --config /path/to.json # 使用指定配置
`);
}

function loadConfig(configPath) {
  if (!fs.existsSync(configPath)) {
    throw new Error(`配置文件不存在: ${configPath}`);
  }
  const raw = fs.readFileSync(configPath, "utf8");
  const config = JSON.parse(raw);
  if (!config.mcpServers || typeof config.mcpServers !== "object") {
    throw new Error("配置文件格式错误：缺少 mcpServers 字段");
  }
  return config.mcpServers;
}

async function main() {
  const opts = parseArgs();

  if (opts.help) {
    printHelp();
    return;
  }

  const mcpServers = loadConfig(opts.config);
  const serverNames = Object.keys(mcpServers);

  console.log(`[MCP] 配置文件: ${opts.config}`);
  console.log(`[MCP] 操作: ${opts.action}`);
  console.log(`[MCP] 服务器: ${serverNames.join(", ")}`);
  console.log("");

  const params = {
    action: opts.action,
    mcpServers,
  };

  const requestBuf = buildRequest(COMMAND_NAME, params);

  console.log("[MCP] 发送 gRPC 请求...");

  try {
    const { payload } = await callGrpc(SERVICE, METHOD, requestBuf);
    const result = parseResponse(payload);

    if (result.code === 0) {
      console.log("[MCP] ✅ 操作成功");
      console.log("");
      console.log(`已${opts.action === "add" ? "注册" : "移除"} ${serverNames.length} 个 MCP 服务器:`);
      for (const name of serverNames) {
        console.log(`  - ${name}`);
      }
    } else {
      console.log("[MCP] ❌ 操作失败");
      console.log("");
      console.log(JSON.stringify(result, null, 2));
      process.exitCode = 1;
    }
  } catch (err) {
    console.log("[MCP] ❌ 调用失败");
    console.log("");
    console.error(err.message);
    process.exitCode = 1;
  }
}

main();
