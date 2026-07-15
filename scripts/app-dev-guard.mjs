// Safety gate rules for app_exec, hot-reloaded by app-dev-mcp.mjs.
// Edit this file and the running MCP server picks up changes automatically
// (fs.watch + dynamic import, ~80ms) — no restart / new conversation needed.
//
// Each rule: { label, re }. The regex is matched against the WHOLE command
// string (chained/piped commands included). A match blocks execution.

export const APP_EXEC_DENY = [
  {
    label: "递归/强制删除（rm -r/-f、rmdir、shred）",
    re: /\brm\b[^|;&]*\s-[rf]/,
  },
  { label: "递归目录删除（rmdir）", re: /\brmdir\b/ },
  {
    label: "磁盘擦除（shred / mkfs / dd if=）",
    re: /\b(shred|mkfs)\b|\bdd\b\s+if=/,
  },
  {
    label: "危险 git 操作（reset --hard / clean -f / checkout -- . / push --force / branch -D）",
    re: /\bgit\b[^|;&]*?(--hard|clean\s+[^|;&]*--?f|push\s+[^|;&]*--force(?:-with-lease)?|checkout\s+[^|;&]*--\s+\.|branch\s+[^|;&]*-D)/,
  },
  {
    label: "进程强杀（kill/pkill/killall）——可能切断环境连接",
    re: /\b(kill|pkill|killall)\b/,
  },
  { label: "提权（sudo / su）", re: /\b(sudo|su)\b/ },
  {
    label: "网络下载直灌 shell（curl|sh、wget|sh）",
    re: /\b(curl|wget)\b[^|;&]*\|\s*(sudo\s+)?(ba)?sh\b/,
  },
  {
    // 只拦真正危险的块设备/磁盘写入；放行 /dev/null、/dev/stdout、/dev/stderr、
    // /dev/tty、/dev/fd/* 等极常见且无害的重定向（如 `2>/dev/null`）。
    label: "写入块设备/磁盘（> /dev/sd*、/dev/nvme* 等）",
    re: />\s*\/dev\/(?!null\b|stdout\b|stderr\b|tty\b|fd\/|std)/,
  },
  { label: "fork 炸弹（:(){:|:&};:）", re: /:\(\)\s*\{/ },
];

export function guardAppExec(cmd) {
  for (const { label, re } of APP_EXEC_DENY) {
    if (re.test(cmd)) return label;
  }
  return null;
}
