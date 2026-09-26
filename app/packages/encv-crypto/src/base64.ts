/**
 * base64.ts —— 纯编码工具（不涉及任何密码学）。
 *
 * 笔记类插件（Obsidian / 思源 / VuePress）常常需要把信封塞进 Markdown 文本里，
 * 所以提供 base64 互转。实现只用 btoa/atob，浏览器与 Node 18+ 均可用。
 */

/** Uint8Array → base64 字符串。 */
export function toBase64(bytes: Uint8Array): string {
  let binary = "";
  const chunkSize = 0x8000; // 分块避免 apply 参数过多爆栈
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunkSize));
  }
  return btoa(binary);
}

/** base64 字符串 → Uint8Array。 */
export function fromBase64(text: string): Uint8Array {
  const binary = atob(text);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    out[i] = binary.charCodeAt(i);
  }
  return out;
}

/** UTF-8 字符串 → Uint8Array（浏览器/Node 都支持 TextEncoder）。 */
export function utf8Encode(text: string): Uint8Array {
  return new TextEncoder().encode(text);
}

/** Uint8Array → UTF-8 字符串。 */
export function utf8Decode(bytes: Uint8Array): string {
  return new TextDecoder().decode(bytes);
}
