/**
 * 本文件为「任务系统模块提升（lift）」从 encv-mobile/src/lib/mockDataGenerator.ts
 * 迁来的真实实现，encv-mobile 原位留 re-export 垫片。
 */

/**
 * Mock 数据生成器（纯函数模块）
 *
 * 这是纯函数版本，不依赖 fs / child_process，可被前端 / 后端任何环境 import。
 *
 * - 前端：通过 POST /api/mock/generate 把 Uint8Array 发给后端写盘
 * - 后端：直接调用本模块的函数写文件
 * - 单元测试：直接调 create*() 验证字节
 *
 * 2026-06-10 note：Node CLI 脚本 scripts/generate-mock-files.ts 已废弃（重复入口被砍）。
 *               本模块继续被前端 / 后端 / 单元测试用，作为唯一 mock 字节定义源。
 *
 * 设计原则：
 * - 所有 create*() 函数返回 Uint8Array（pure，无副作用）
 * - IO 通过 generateMockFiles() 的 writeToDisk 回调抽象
 * - 不在模块顶层调 execSync / fs
 */

import { normalizeExt } from "@encv/shared-components/lib/string";

// ==================== 类型定义 ====================

export type MockFileType = "all" | "plain" | "ae" | "container" | "boundary";

export interface MockFileSpec {
  relativePath: string;
  data: Uint8Array;
  size: number;
}

export interface GenerateOptions {
  root: string;
  type?: MockFileType;
  writeToDisk?: (path: string, data: Uint8Array) => Promise<void> | void;
  onProgress?: (spec: MockFileSpec) => void;
}

export interface GenerateResult {
  count: number;
  totalSize: number;
  specs: MockFileSpec[];
}

// ==================== 工具函数 ====================

function randomBytes(n: number): Uint8Array {
  const buf = new Uint8Array(n);
  if (typeof crypto !== "undefined" && crypto.getRandomValues) {
    // crypto.getRandomValues 单次最多 65536 字节（Web Crypto 硬限制）
    // 超过必须分块调用，否则抛 QuotaExceededError
    const CHUNK = 65536;
    let offset = 0;
    while (offset < n) {
      const len = Math.min(CHUNK, n - offset);
      crypto.getRandomValues(buf.subarray(offset, offset + len));
      offset += len;
    }
  } else {
    for (let i = 0; i < n; i++) buf[i] = Math.floor(Math.random() * 256);
  }
  return buf;
}

function padToSize(data: Uint8Array, targetSize: number): Uint8Array {
  if (data.length >= targetSize) return data;
  const padded = new Uint8Array(targetSize);
  padded.set(data);
  padded.set(randomBytes(targetSize - data.length), data.length);
  return padded;
}

function crc32(buf: Uint8Array): number {
  let c = 0xffffffff;
  for (let i = 0; i < buf.length; i++) {
    c ^= buf[i];
    for (let j = 0; j < 8; j++) {
      c = (c >>> 1) ^ (c & 1 ? 0xedb88320 : 0);
    }
  }
  return (c ^ 0xffffffff) >>> 0;
}

function makeChunk(type: string, data: Uint8Array): Uint8Array {
  const len = new Uint8Array(4);
  new DataView(len.buffer).setUint32(0, data.length, false);
  const typeB = new TextEncoder().encode(type);
  const crcData = new Uint8Array(typeB.length + data.length);
  crcData.set(typeB, 0);
  crcData.set(data, typeB.length);
  const crc = new Uint8Array(4);
  new DataView(crc.buffer).setUint32(0, crc32(crcData), false);
  const out = new Uint8Array(4 + typeB.length + data.length + 4);
  out.set(len, 0);
  out.set(typeB, 4);
  out.set(data, 4 + typeB.length);
  out.set(crc, 4 + typeB.length + data.length);
  return out;
}

function joinPath(...parts: string[]): string {
  return parts.join("/").replace(/\/+/g, "/");
}

// 注：mockDataGenerator 自身是纯算法 + 可选 writeToDisk 回调的库
// 父目录建在调用方（后端 mock_generator.go 走 os.MkdirAll）
// 这里不放 Node 专属 fs 代码（避免污染前端 bundle + 避开 vue-tsc TS2580: require 找不到）

// ==================== JPEG ====================

export function createJPEG(): Uint8Array {
  return new Uint8Array([
    0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xff, 0xdb,
    0x00, 0x43, 0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09, 0x09, 0x08, 0x0a, 0x0c, 0x14, 0x0d, 0x0c, 0x0b,
    0x0b, 0x0c, 0x19, 0x12, 0x13, 0x0f, 0x14, 0x1d, 0x1a, 0x1f, 0x1e, 0x1d, 0x1a, 0x1c, 0x1c, 0x20, 0x24, 0x2e, 0x27, 0x20, 0x22, 0x2c,
    0x23, 0x1c, 0x1c, 0x28, 0x37, 0x29, 0x2c, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1f, 0x27, 0x39, 0x3d, 0x38, 0x32, 0x3c, 0x2e, 0x33, 0x34,
    0x32, 0xff, 0xc0, 0x00, 0x0b, 0x08, 0x00, 0x01, 0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xff, 0xc4, 0x00, 0x1f, 0x00, 0x00, 0x01, 0x05,
    0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
    0x09, 0x0a, 0x0b, 0xff, 0xc4, 0x00, 0xb5, 0x10, 0x00, 0x02, 0x01, 0x03, 0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00,
    0x01, 0x7d, 0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06, 0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32,
    0x81, 0x91, 0xa1, 0x08, 0x23, 0x42, 0xb1, 0xc1, 0x15, 0x52, 0xd1, 0xf0, 0x24, 0x33, 0x62, 0x72, 0x82, 0x09, 0x0a, 0x16, 0x17, 0x18,
    0x19, 0x1a, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2a, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3a, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48, 0x49,
    0x4a, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59, 0x5a, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6a, 0x73, 0x74, 0x75, 0x76, 0x77,
    0x78, 0x79, 0x7a, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89, 0x8a, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9a, 0xa2, 0xa3,
    0xa4, 0xa5, 0xa6, 0xa7, 0xa8, 0xa9, 0xaa, 0xb2, 0xb3, 0xb4, 0xb5, 0xb6, 0xb7, 0xb8, 0xb9, 0xba, 0xc2, 0xc3, 0xc4, 0xc5, 0xc6, 0xc7,
    0xc8, 0xc9, 0xca, 0xd2, 0xd3, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8, 0xd9, 0xda, 0xe1, 0xe2, 0xe3, 0xe4, 0xe5, 0xe6, 0xe7, 0xe8, 0xe9, 0xea,
    0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7, 0xf8, 0xf9, 0xfa, 0xff, 0xda, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3f, 0x00, 0x7b, 0x94,
    0x40, 0x18, 0x19, 0x1f, 0x81, 0x17, 0x38, 0x41, 0x91, 0x82, 0x83, 0x84, 0x88, 0x89, 0x8c, 0x8d, 0x90, 0x92, 0x93, 0x96, 0x97, 0x98,
    0x99, 0x9a, 0x9c, 0x9e, 0x9f, 0xa0, 0xa1, 0xa2, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8, 0xa9, 0xaa, 0xac, 0xad, 0xae, 0xaf, 0xb0, 0xb1, 0xb2,
    0xb3, 0xb4, 0xb5, 0xb6, 0xb7, 0xb8, 0xb9, 0xba, 0xbc, 0xbd, 0xbe, 0xbf, 0xc0, 0xc1, 0xc2, 0xc3, 0xc4, 0xc5, 0xc6, 0xc7, 0xc8, 0xc9,
    0xca, 0xcc, 0xcd, 0xce, 0xcf, 0xd0, 0xd1, 0xd2, 0xd3, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8, 0xd9, 0xda, 0xdc, 0xde, 0xdf, 0xe0, 0xe1, 0xe2,
    0xe3, 0xe4, 0xe5, 0xe6, 0xe7, 0xe8, 0xe9, 0xea, 0xec, 0xed, 0xee, 0xef, 0xf0, 0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7, 0xf8, 0xf9,
    0xea, 0xfb, 0xfd, 0xfe, 0xff, 0xd9,
  ]);
}

// ==================== PNG ====================

export function createPNG(): Uint8Array {
  const signature = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

  const ihdrData = new Uint8Array([0, 0, 0, 4, 0, 0, 0, 4, 8, 6, 0, 0, 0]);
  const ihdr = makeChunk("IHDR", ihdrData);

  const pixels: number[] = [];
  for (let y = 0; y < 4; y++) {
    pixels.push(0);
    for (let x = 0; x < 4; x++) {
      const r = Math.floor((x / 3) * 255);
      const g = Math.floor((y / 3) * 255);
      const b = 128;
      const a = 255;
      pixels.push(r, g, b, a);
    }
  }
  const idatRaw = new Uint8Array(pixels);
  // 纯前端环境用 store-only deflate（zlib 0x78 0x01 header），
  // 任何合规 PNG 解码器都接受。Node 端 import 脚本优先用 ffmpeg 重写，无需 zlib。
  const idatCompressed = deflateRaw(idatRaw);
  const idat = makeChunk("IDAT", idatCompressed);

  const iend = makeChunk("IEND", new Uint8Array(0));

  const out = new Uint8Array(signature.length + ihdr.length + idat.length + iend.length);
  out.set(signature, 0);
  out.set(ihdr, signature.length);
  out.set(idat, signature.length + ihdr.length);
  out.set(iend, signature.length + ihdr.length + idat.length);
  return out;
}

// 极简 deflate（store-only blocks）+ zlib wrapper
// 用于纯前端环境（无 zlib 模块）。Node 端会被 require('zlib').deflateSync 覆盖。
function deflateRaw(input: Uint8Array): Uint8Array {
  // zlib header: 0x78 0x01 (no compression hint)
  const header = new Uint8Array([0x78, 0x01]);
  // adler32
  let a = 1,
    b = 0;
  for (let i = 0; i < input.length; i++) {
    a = (a + input[i]) % 65521;
    b = (b + a) % 65521;
  }
  const adler = new Uint8Array(4);
  new DataView(adler.buffer).setUint32(0, (b << 16) | a, false);

  // 单个 non-final stored block (BTYPE=00)
  const blockType = 0x00;
  const lastBlock = 0x01;
  const lenBuf = new Uint8Array(4);
  new DataView(lenBuf.buffer).setUint16(0, input.length, true);
  new DataView(lenBuf.buffer).setUint16(2, ~input.length & 0xffff, true);

  const total = 2 + 1 + 4 + input.length + 4;
  const out = new Uint8Array(total);
  out.set(header, 0);
  out[2] = lastBlock | blockType;
  out.set(lenBuf, 3);
  out.set(input, 7);
  out.set(adler, 7 + input.length);
  return out;
}

// ==================== MP4 ====================
//
// sample.mp4 fixture: 1s 64x64 H.264 Constrained Baseline L1.0 + AAC LC 16k mono.
// 之前手写的 mp4 box 构造有严重错误（hdlr='vide' 配 mp4aEntry / stsz 谎报 /
// tkhd duration 33.5s 与 mdhd 150ms 不一致），ffprobe 报 Invalid data。
// 现改为 base64 内嵌 ffmpeg 生成的合法 mp4，保留同步接口避免连锁 async 改造。
//
// ffprobe 验证：nb_streams=2, duration=1.000000, bit_rate=38256

const MP4_B64_P1 =
  "AAAAIGZ0eXBpc29tAAACAGlzb21pc28yYXZjMW1wNDEAAAbbbW9vdgAAAGxtdmhkAAAAAAAAAAAAAAAAAAAD6AAAA+gAAQAAAQAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAwAAApx0cmFrAAAAXHRraGQAAAADAAAAAAAAAAAAAAABAAAAAAAAA+gAAAAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAABAAAAAAEAAAABAAAAAAAAkZWR0cwAAABxlbHN0AAAAAAAAAAEAAAPoAAAAAAABAAAAAAIUbWRpYQAAACBtZGhkAAAAAAAAAAAAAAAAAAAoAAAAKABVxAAAAAAALWhkbHIAAAAAAAAAAHZpZGUAAAAAAAAAAAAAAABWaWRlb0hhbmRsZXIAAAABv21pbmYAAAAUdm1oZAAAAAEAAAAAAAAAAAAAACRkaW5mAAAAHGRyZWYAAAAAAAAAAQAAAAx1cmwgAAAAAQAAAX9zdGJsAAAAu3N0c2QAAAAAAAAAAQAAAKthdmMxAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAEAAQABIAAAASAAAAAAAAAABFUxhdmM2MC4zMS4xMDIgbGlieDI2NAAAAAAAAAAAAAAAGP//AAAAMWF2Y0MBQsAK/+EAGGdCwAqmERCbARAAAAMAEAAAAwFA8SJhGAEABmjIQgRLIAAAABBwYXNwAAAAAQAAAAEAAAAUYnRydAAAAAAAABfQAAAX0AAAABhzdHRzAAAAAAAAAAEAAAAKAAAEAAAAABRzdHNzAAAAAAAAAAEAAAABAAAAHHN0c2MAAAAAAAAAAQAAAAEAAAABAAAAAQAAADxzdHN6AAAAAAAAAAAAAAAKAAACmgAAAAoAAAAKAAAACgAAAAsAAAALAAAACwAAAAsAAAALAAAACwAAADhzdGNvAAAAAAAAAAoAAAdQAAALBgAAC9IAAAyWAAANlAAADl8AAA8qAAAQHwAAEO4AABG/AAADaXRyYWsAAABcdGtoZAAAAAMAAAAAAAAAAAAAAAIAAAAAAAAD6AAAAAAAAAAAAAAAAQEAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAACRlZHRzAAAAHGVsc3QAAAAAAAAAAQAAA+gAAAQAAAEAAAAAAuFtZGlhAAAAIG1kaGQAAAAAAAAAAAAAAAAAAKxEAACwRFXEAAAAAAAtaGRscgAAAAAAAAAAc291bgAAAAAAAAAAAAAAAFNvdW5kSGFuZGxlcgAAAAKMbWluZgAAABBzbWhkAAAAAAAAAAAAAAAkZGluZgAAABxkcmVmAAAAAAAAAAEAAAAMdXJsIAAAAAEAAAJQc3RibAAAAH5zdHNkAAAAAAAAAAEAAABu";

const MP4_B64_P2 =
  "bXA0YQAAAAAAAAABAAAAAAAAAAAAAQAQAAAAAKxEAAAAAAA2ZXNkcwAAAAADgICAJQACAASAgIAXQBUAAAAAAEO1AABDtQWAgIAFEghW5QAGgICAAQIAAAAUYnRydAAAAAAAAEO1AABDtQAAACBzdHRzAAAAAAAAAAIAAAAsAAAEAAAAAAEAAABEAAAAcHN0c2MAAAAAAAAACAAAAAEAAAABAAAAAQAAAAIAAAAFAAAAAQAAAAMAAAAEAAAAAQAAAAUAAAAFAAAAAQAAAAYAAAAEAAAAAQAAAAgAAAAFAAAAAQAAAAkAAAAEAAAAAQAAAAsAAAAFAAAAAQAAAMhzdHN6AAAAAAAAAAAAAAAtAAAARQAAAFcAAAAzAAAAMQAAADAAAAAxAAAALgAAADMAAAAwAAAAMQAAAC0AAAAwAAAALQAAADAAAAAwAAAAMgAAADMAAAAyAAAALQAAADEAAAAuAAAAMgAAAC8AAAA0AAAALwAAAC0AAAAwAAAALgAAAC0AAAAuAAAAMQAAADAAAAAxAAAAMgAAADEAAAAwAAAALwAAADYAAAAuAAAAMwAAADMAAAAxAAAANwAAAEQAAAAFAAAAPHN0Y28AAAAAAAAACwAABwsAAAnqAAALEAAAC9wAAAygAAANnwAADmoAAA81AAAQKgAAEPkAABHKAAAAGnNncGQBAAAAcm9sbAAAAAIAAAAB//8AAAAcc2JncAAAAAByb2xsAAAAAQAAAC0AAAABAAAAYnVkdGEAAABabWV0YQAAAAAAAAAhaGRscgAAAAAAAAAAbWRpcmFwcGwAAAAAAAAAAAAAAAAtaWxzdAAAACWpdG9vAAAAHWRhdGEAAAABAAAAAExhdmY2MC4xNi4xMDAAAAAIZnJlZQAAC6ttZGF03gIATGF2YzYwLjMxLjEwMgACoJcVyufWe3WuLnEktcLH0+n0vv31VVbPlNMXHjx+XymyxcePHjly5YuPHjFEREhFUURcAAACcwYF//9v3EXpvebZSLeWLNgg2SPu73gyNjQgLSBjb3JlIDE2NCByMzEwOCAzMWUxOWY5IC0gSC4yNjQvTVBFRy00IEFWQyBjb2RlYyAtIENvcHlsZWZ0IDIwMDMtMjAyMyAtIGh0dHA6Ly93d3cudmlkZW9sYW4ub3JnL3gyNjQuaHRtbCAtIG9wdGlvbnM6IGNhYmFjPTAgcmVmPTE2IGRlYmxvY2s9MTowOjAgYW5hbHlzZT0weDE6MHgxMzEgbWU9dW1oIHN1Ym1lPTEwIHBzeT0xIHBzeV9yZD0xLjAwOjAuMDAgbWl4ZWRfcmVmPTEgbWVfcmFuZ2U9MjQgY2hyb21hX21lPTEgdHJlbGxpcz0yIDh4OGRjdD0wIGNxbT0wIGRlYWR6b25lPTIxLDExIGZhc3RfcHNraXA9MSBjaHJvbWFfcXBfb2Zmc2V0PS0yIHRocmVhZHM9MiBsb29rYWhlYWRfdGhyZWFkcz0xIHNsaWNl";

const MP4_B64_P3 =
  "ZF90aHJlYWRzPTAgbnI9MCBkZWNpbWF0ZT0xIGludGVybGFjZWQ9MCBibHVyYXlfY29tcGF0PTAgY29uc3RyYWluZWRfaW50cmE9MCBiZnJhbWVzPTAgd2VpZ2h0cD0wIGtleWludD0yNTAga2V5aW50X21pbj0xMCBzY2VuZWN1dD00MCBpbnRyYV9yZWZyZXNoPTAgcmNfbG9va2FoZWFkPTYwIHJjPWNyZiBtYnRyZWU9MSBjcmY9MzAuMCBxY29tcD0wLjYwIHFwbWluPTAgcXBtYXg9NjkgcXBzdGVwPTQgaXBfcmF0aW89MS40MCBhcT0xOjEuMDAAgAAAAB9liIIH+IxQABEfjgACCVHAAESO+++uuuuuuuuuuuvAATiM2sjaJWyVslXP89dcf/Wv3641/x4/nzxr+/v/PnjQb/W0dEYxC6BMsIWfkDcWUcRr+Q1/L5AfSiE2Eu/whVhggqJk8XPPF9J1cYvpzscvENRRRQbgATrriu8c9/s/f2+LcOGl6q7gdz++dN2bMbP7kMnwBn9wMnwYZ/eEfHwYe/uQ+Pgw9/cuAQQriaKmV+f/7P8f+v/vd6vRN+fXmu/oa6KEYEVFFFFFFFDFtkbRsJZiUxVcpDb8OAEEK4miplfP/9+//1/XU1rWOtuenPQ+U85zqKeeeeec86m+lL6oF6XCqfleAQpfgAEIK4nCkz7f/3Pz/6/+93fSVOO+fbO/YfL5RLOY0UUUUUWXjx4ogrC1V6xKaalUwo4AAAAGQZocD/CMAQIriYK1V9v/77/1/8tXepK4c8+2b4CXnnUtme0vaWJea2ebdRYy5hGS9mwEOAEIK5Gior5//q9/+v/vd6uSrqZp6+gEhITHapQSEhISEhISEzPVFl6cMPTYuSsmiuEYcAD6K4mCtvf6f/1O//X/1mtWvTnXv8bzgL/VCDkX66OjooooX7W72paJZEQJ2aiSTgEIK5GCsr5//5P/n/vc1cvLX7+1/P3AMDA0FjawYkSJEgbPe92mikIOc5QKMRVMEOAAAAAGQZoqA/wjAQQrigJ1Z+f/7f6f+v/vd61cit5xM0OMUS5DGiiiiiiCBEoiJFRS4qXs2FEnAQQricKWV8//37//8+b1epdST39p6+B9HnkpZc88888l555LtpSPXPX0iSCa4lK3AQYriaKlV+f/7X7f+v/vNXxLUrfWSx9KKEOkVFFFFFFFDcWG1kxLMSCzznQcAQAriaKm6+f/78/9f+93fRxWZnmevInnnfPCU8888/VPOq9F6L71aXFsUL+RjBHgAAAABkGaOwP8IwEGK5GipVfb/+n4/9f/ea1cFa9e1+OgDAwNhYtYMbNgxs2DGwaqiqpWUSgxWTKKOAD4K5FixzX7f/1e//n/3vV3rLvL+fZzwIxYHDatYGBgYHRzVljhKqVY1aZQFap1ewKOAQgrkaKlPf/+/X/z/5Xd3Il5z7V7/cBISEx2sKCQkJCRISVKza1o9JPgUuSn";

const MP4_B64_P4 =
  "I/kuSUnwAQAricKW6/T/+79v/X/1mr4nG5l+vjeew10UIORUUUUULoMtfVG0YxkZ4lr1KtQjPdwBCCuJgrU+f/+U//P/F6vUuh6+L+fwPlPOc6jTzzzzzz3pvKUXLoFF7iZSCHAAAAAHQZpJAP8IwAEEK5HCllfn/+z/H/r/73etaXlVz7VvgA0suGK1yyyyym5ZSQNLzohChS5VUxNgohwBBCuG7K+f/+U/+//NzWq1Lm+/NevuPhr1ylhjr169c9c05pTlO6VpYSQSXCTgAQgricKVPt//S9//n/3l3dy6458fE70Pp9PohJF9NevXRRrM2aiVrRylssCUi9FQk4AA9iuJYseH7f/1t/+v/vq7jirrOfbffwJ5/V85F+v1+v1HcuvQj+106XKBJMYpOAAAAAdBmllA/wjAAQgrkaKlPf//lv/5/9ru+C6l+vOZ0AYGBoLFrBgYGBgYGvUEWnTfnq53rWF4lK3KOBnO3AD8K4mCtzn6f/2Pf/1/9ZrV3rIz1+Oe/ocYolpMaKKKKKItSs1UKQVQJLklxSLgAQYriaKmPn//lP/t/ter1KiVnVd/A+nnzpTcTzzzzz3O8mU5pSkKaCQUXKEuAQQrieKGV+f/7P8f+v/vOJrS8N+c30NdFCFkVFFFFFC6NbZvpAjwEsIsjOTGLRjwAAAAB0GaaYD/CMABAiuG7dfP//Kf/p/pd6u5Vy/t8Zz5H89++lLw379++977y+9QpDTeBRcomSUcAQgricKSvz//e+f/X/3u9aRe2cM0Pl8uKzmNFFxyxRcYiRDVCEBS4rRc9gUcAP4riYK2V+f/+Wf+v/nNXxXBvHnfj4DzzzqWzfvT+9LHkpzrZySEsJJc2rlBwAEIK5Gior5//rb/9f/O7vRLy/Xxm/YBISEx2qUEhISEiRJWanMllwrRtaBZROTURIcA/CuJgrbz9P/6fj/1/9ZrU1BXPx47+4oooQci6OjooooNG8bdEXwLU0FGQsmSQcAAAAAHQZp5wP8IwAEIK4mCtT5//5T/5/51c1emVrx5zn2Hy+XyOeM0/NPPPsf86CjEU9DPhK2pC5YJQ4ABBCuKImZX5//s/x/6/+93erjVT7frz6+hliiXIY0UUUUUURKIokHqRDBugQgLLlHvcAEGK4mCtVfP/9+//3/e71drqX7/W+/YfR55KWGeeeeeeenLWklKMra+0TL+ZPOqEnABBiuJwpZX2//ufn/1/97u9TVazOfL3/A+lFCEkVFFFFFBjX7vpFLhInKYZTgmSScAAAAHQZqIgDvCMAD+K4lixy+3/99/6/+WprVdVWq+735zoc08754Sn6p51b17/uipeAtA33FXAEeAAQYriiJlPt//V7/9f/e7tq643XP0+33Hy+XywOY3y+Xy+XGETRcxyRNPxOri0F1tPGC5St3AAPgrhu5z9v/6nf/r/7y5xcXOefv47+Bq/PwWlGP9tUIIE4IK8YUJTNf3uVXCjgEIK5GCsr3/";

const MP4_B64_P5 =
  "/vv/X/yu7tqXl/n449/YBISEx2sKCSoSElQkTJaXSOWfSck4Bad7MYpRwAAAAAdBmpiQN8IwAP4riYK3Nfp//c/P/r/73eru9+a7z678fcfSihByKiiihdFGxaMWv2bBTQbIxKplp0hwAQYricKVV8//37//j+rvVru2+fjPHwPlzTnOqOeeeec6pzqJXFi+IlMhjsndYbe0OAE6S5Lqqf8fn/fjXXE4cVxV3pYDw8PbsWHh4efcQAKceHu4gAdw8PdwAAdx4e24AAKcPD23AOABMI2myVsjbJUM/P8+eL+39vrzd//+v+gDavoyrai9ZeNggqJk8XPPF9JxEX0ZlOpqWTT9mRii/q3m+/4L6rzvPPPPcAEYgbRw";

const MP4_B64 = MP4_B64_P1 + MP4_B64_P2 + MP4_B64_P3 + MP4_B64_P4 + MP4_B64_P5;

let _mp4Cache: Uint8Array | null = null;

export function createMP4(): Uint8Array {
  if (_mp4Cache) return _mp4Cache;
  const binary = atob(MP4_B64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
  _mp4Cache = bytes;
  return bytes;
}

// ==================== MKV/WebM ====================

function ebmlVInt(val: number): Uint8Array {
  if (val < 127) return new Uint8Array([val]);
  if (val < 16383) return new Uint8Array([0x40 | (val >> 8), val & 0xff]);
  if (val < 2097151) return new Uint8Array([0x20 | (val >> 16), (val >> 8) & 0xff, val & 0xff]);
  return new Uint8Array([0x10 | (val >> 24), (val >> 16) & 0xff, (val >> 8) & 0xff, val & 0xff]);
}

function ebmlElement(id: Uint8Array, value: Uint8Array | Uint8Array[]): Uint8Array {
  const v = Array.isArray(value) ? concatBytes(value) : value;
  const size = ebmlVInt(v.length);
  const result = new Uint8Array(id.length + size.length + v.length);
  result.set(id, 0);
  result.set(size, id.length);
  result.set(v, id.length + size.length);
  return result;
}

function ebmlUint(id: Uint8Array, val: number): Uint8Array {
  if (val === 0) return ebmlElement(id, new Uint8Array([0]));
  const bytes: number[] = [];
  let v = val;
  while (v > 0) {
    bytes.unshift(v & 0xff);
    v >>>= 8;
  }
  return ebmlElement(id, new Uint8Array(bytes));
}

function ebmlString(id: Uint8Array, str: string): Uint8Array {
  return ebmlElement(id, new TextEncoder().encode(str));
}

function ebmlFloat(id: Uint8Array, val: number): Uint8Array {
  const buf = new ArrayBuffer(8);
  const view = new DataView(buf);
  view.setFloat64(0, val, false);
  return ebmlElement(id, new Uint8Array(new Int8Array(buf)));
}

export function createMKV(): Uint8Array {
  const EBML = new Uint8Array([0x1a, 0x45, 0xdf, 0xa3]);
  const Segment = new Uint8Array([0x18, 0x53, 0x80, 0x67]);
  const Info = new Uint8Array([0x15, 0x49, 0xa9, 0x66]);
  const Tracks = new Uint8Array([0x16, 0x54, 0xae, 0x6b]);
  const TrackEntry = new Uint8Array([0xae]);
  const TrackNumber = new Uint8Array([0xd7]);
  const TrackUID = new Uint8Array([0x73, 0xc5]);
  const TrackType = new Uint8Array([0x83]);
  const CodecID = new Uint8Array([0x86]);
  const CodecPrivate = new Uint8Array([0x63, 0xa2]);
  const TimecodeScale = new Uint8Array([0x2a, 0xd7, 0xb1]);
  const Duration = new Uint8Array([0x44, 0x89]);
  const Cluster = new Uint8Array([0x1f, 0x43, 0xb6, 0x75]);
  const Timecode = new Uint8Array([0xe7]);
  const SimpleBlock = new Uint8Array([0xa3]);
  const DocType = new Uint8Array([0x42, 0x82]);
  const DocTypeVersion = new Uint8Array([0x42, 0x87]);
  const DocTypeReadVersion = new Uint8Array([0x42, 0x85]);

  const ebmlHeader = ebmlElement(
    EBML,
    concatBytes([ebmlUint(DocTypeVersion, 4), ebmlUint(DocTypeReadVersion, 4), ebmlString(DocType, "matroska")])
  );

  const audioTrack = ebmlElement(
    TrackEntry,
    concatBytes([
      ebmlUint(TrackNumber, 1),
      ebmlUint(TrackUID, 12345),
      ebmlUint(TrackType, 2),
      ebmlString(CodecID, "A_VORBIS"),
      ebmlElement(CodecPrivate, new Uint8Array([0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00])),
    ])
  );

  const info = ebmlElement(Info, concatBytes([ebmlUint(TimecodeScale, 1000000), ebmlFloat(Duration, 2.5)]));

  const tracks = ebmlElement(Tracks, [audioTrack]);

  const silentVorbisData = new Uint8Array(64).fill(0);
  const blockHeader = new Uint8Array([0x01, 0x00, 0x00, 0x00]);
  const simpleBlock = ebmlElement(SimpleBlock, concatBytes([blockHeader, silentVorbisData]));

  const cluster = ebmlElement(Cluster, concatBytes([ebmlUint(Timecode, 0), simpleBlock]));

  const segmentContent = concatBytes([info, tracks, cluster]);
  const segment = ebmlElement(Segment, segmentContent);

  return concatBytes([ebmlHeader, segment]);
}

function concatBytes(parts: Uint8Array[]): Uint8Array {
  const total = parts.reduce((s, p) => s + p.length, 0);
  const out = new Uint8Array(total);
  let off = 0;
  for (const p of parts) {
    out.set(p, off);
    off += p.length;
  }
  return out;
}

// ==================== MP3 ====================

export function createMP3(): Uint8Array {
  const parts: Uint8Array[] = [];
  const id3v2Header = new Uint8Array([0x49, 0x44, 0x33, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x41]);

  function makeId3Frame(id: string, text: string): Uint8Array {
    const enc = new TextEncoder().encode(text);
    const size = enc.length + 1;
    const buf = new Uint8Array(10 + size);
    for (let i = 0; i < id.length; i++) buf[i] = id.charCodeAt(i);
    new DataView(buf.buffer).setUint32(4, size, false);
    buf[10] = 0;
    for (let i = 0; i < enc.length; i++) buf[11 + i] = enc[i];
    return buf;
  }

  parts.push(id3v2Header);
  parts.push(makeId3Frame("TIT2", "Mock Music"));
  parts.push(makeId3Frame("TPE2", "Test Artist"));

  for (let i = 0; i < 108; i++) {
    const frame = new Uint8Array(418);
    frame[0] = 0xff;
    frame[1] = 0xfb;
    frame[2] = 0x90;
    frame[3] = 0x00;
    parts.push(frame);
  }

  return concatBytes(parts);
}

// ==================== FLAC ====================

export function createFLAC(): Uint8Array {
  const signature = new TextEncoder().encode("fLaC");

  const streamInfo = new Uint8Array(38);
  streamInfo[0] = 0x00;
  streamInfo[1] = 0x22;
  streamInfo[2] = 0x00;
  const siView = new DataView(streamInfo.buffer);
  siView.setUint16(3, 44100, false);
  siView.setUint8(5, 1);
  siView.setUint8(6, 16);
  siView.setUint32(7, 100000, false);
  siView.setUint32(11, 500000, false);

  const paddingBlock = new Uint8Array([0x01, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]);

  const frameHeader = new Uint8Array([
    0xf8, 0xe8, 0x1f, 0xfe, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
  ]);

  return concatBytes([signature, streamInfo, paddingBlock, frameHeader]);
}

// ==================== PDF ====================

export function createPDF(): Uint8Array {
  const pdf = `%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj

2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj

3 0 obj
<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>
endobj

xref
0 4
0000000000 65535 f
0000000009 00000 n
0000000058 00000 n
0000000115 00000 n

trailer
<< /Size 4 /Root 1 0 R >>
startxref
190
%%EOF`;
  return new TextEncoder().encode(pdf);
}

// ==================== AList Encrypt (.ae) ====================

export function createAEFile(name: string, targetSize: number): Uint8Array {
  const magic = new TextEncoder().encode("AENC");
  const nameBytes = new TextEncoder().encode(name);
  const nameLen = Math.min(nameBytes.length, 255);
  const headerLen = 8 + nameLen + 1;
  const header = new Uint8Array(headerLen);
  header.set(magic, 0);
  header[4] = 0x01;
  header[5] = 0x00;
  header[6] = nameLen;
  header.set(nameBytes.slice(0, nameLen), 7);
  header[7 + nameLen] = 0x00;
  return padToSize(header, targetSize);
}

// ==================== ENCV ECv4 Container ====================

export function createSCCVFile(name: string, ext: string, targetSize: number): Uint8Array {
  const magic = new TextEncoder().encode("SCCV");
  const manifest = JSON.stringify({
    version: "4.0",
    originalName: name,
    originalExt: ext,
    algorithm: "aes-256-gcm",
    createdAt: new Date().toISOString(),
    entries: [{ type: "file", name: name, size: targetSize - 256 }],
  });

  const manifestBytes = new TextEncoder().encode(manifest);
  const header = new Uint8Array(32);
  header.set(magic, 0);
  header[4] = 0x04;
  header[5] = 0x01;
  const hv = new DataView(header.buffer);
  hv.setUint32(8, 32, false);
  hv.setUint32(12, manifestBytes.length, false);

  const body = new Uint8Array(32 + manifestBytes.length + 64);
  body.set(header, 0);
  body.set(manifestBytes, 32);
  return padToSize(body, targetSize);
}

// ==================== Boundary file specs ====================

const NOTES_CONTENT = `ENCV Mock Data Notes
======================

This is a multi-language UTF-8 test file.
中文测试文件 — 包含中文内容
日本語テスト — 日本語の内容
한국어 테스트 — 한국어 내용
العربية اختبار — محتوى عربي
עברית בדיקה — תוכן עברית
ไทยทดสอบ — เนื้อหาภาษาไทย
Ελληνικά δοκιμή — περιεχόμενο ελληνικά
Русский тест — содержание на русском
Deutsch Test — deutscher Inhalt
Français test — contenu français
Española prueba — contenido en español
Português teste — conteúdo em português
Italiano prova — contenuto italiano
Nederlands test — Nederlandse inhoud
Polski test — treść polska
Český test — český obsah
Magyar teszt — magyar tartalom
Türkçe deneme — Türkçe içerik
Việt Nam kiểm tra — nội dung tiếng Việt
Tiếng Việt kiểm tra — nội dung tiếng Việt
ไทยทดสอบ — เนื้อหาภาษาไทย

Special characters: !@#$%^&*()_+-=[]{}|;':",./<>?~\\\`
Numbers: 0123456789
Emoji: 😀🎉🚀🔥💯✨🎵📝🔒
`;

const CSV_CONTENT = `id,name,category,size,encrypted
1,photo.jpg,image,107,false
2,screenshot.png,image,512,false
3,sample.mp4,video,45056,false
4,comedy.mkv,video,2048,false
5,music.mp3,audio,45184,false
6,podcast.flac,audio,1024,false
7,report.pdf,document,512,false
8,notes.txt,text,1024,false
9,data.csv,csv,256,false
10,secret.ae,encrypt,4096,true
11,document.ae,encrypt,8192,true
12,hidden-gem.ae,encrypt,16384,true
13,container.sccgv,container,8192,true
14,archive.scext,container,16384,true
15,bundle.scepkg,container,32768,true
`;

// ==================== Spec 收集 ====================

function spec(relPath: string, data: Uint8Array | string): MockFileSpec {
  const bytes = typeof data === "string" ? new TextEncoder().encode(data) : data;
  return { relativePath: relPath, data: bytes, size: bytes.length };
}

/**
 * 收集指定 type 的所有 MockFileSpec（不写盘，纯函数）。
 * 用于前端预览 / 后端按 spec 写盘 / CLI 落盘。
 */
export function collectSpecs(type: MockFileType): MockFileSpec[] {
  const specs: MockFileSpec[] = [];

  if (type === "all" || type === "plain") {
    specs.push(
      spec("01-plain-media/image/photo.jpg", createJPEG()),
      spec("01-plain-media/image/screenshot.png", createPNG()),
      spec("01-plain-media/video/sample.mp4", createMP4()),
      spec("01-plain-media/video/comedy.mkv", createMKV()),
      spec("01-plain-media/audio/music.mp3", createMP3()),
      spec("01-plain-media/audio/podcast.flac", createFLAC()),
      spec("01-plain-media/document/report.pdf", createPDF()),
      spec("01-plain-media/document/notes.txt", NOTES_CONTENT),
      spec("01-plain-media/document/data.csv", CSV_CONTENT)
    );
  }

  if (type === "all" || type === "ae") {
    specs.push(
      spec("02-alist-encrypt/secret.ae", createAEFile("secret.ae", 4096)),
      spec("02-alist-encrypt/document.ae", createAEFile("document.ae", 8192)),
      spec("02-alist-encrypt/hidden-gem.ae", createAEFile("hidden-gem.ae", 16384))
    );
  }

  if (type === "all" || type === "container") {
    specs.push(
      spec("03-encv-containers/container.sccgv", createSCCVFile("container", "sccgv", 8192)),
      spec("03-encv-containers/archive.scext", createSCCVFile("archive", "scext", 16384)),
      spec("03-encv-containers/bundle.scepkg", createSCCVFile("bundle", "scepkg", 32768))
    );
  }

  if (type === "all" || type === "boundary") {
    specs.push(
      // 后端 mock_generator.go boundarySpecs 5 个基础文件，前端必须全部覆盖
      spec("04-boundary-test/normal.txt", "Normal baseline content"),
      spec("04-boundary-test/long-unicode-filename-中文-日本語-한국어-العربية-עברית-ไทย-ελληνικά.txt", "Unicode filename test"),
      spec("04-boundary-test/.hidden-file", randomBytes(256)),
      spec("04-boundary-test/spaces   in   name.txt", "Spaces in filename"),
      spec("04-boundary-test/special-chars-!@#$%^&*()_+.txt", "Special characters"),
      spec("04-boundary-test/zero-byte-file.bin", new Uint8Array(0)),
      spec("04-boundary-test/single-byte.bin", new Uint8Array([0x42])),
      spec("04-boundary-test/exactly-1kb.bin", padToSize(new Uint8Array([0x41]), 1024)),
      spec("04-boundary-test/large-1mb.dat", padToSize(new Uint8Array([0x58, 0x59, 0x5a]), 1024 * 1024)),
      spec("04-boundary-test/control-chars-\x01\x02\x03.txt", "Control character filename"),
      spec("04-boundary-test/‫אבג-rtl-filename.txt", "RTL filename test"),
      spec("04-boundary-test/emoji-test-😀🎉🚀🔥.txt", "Emoji filename test"),
      spec("04-boundary-test/trailing-space.txt ", "Trailing space"),
      spec("04-boundary-test/..dotfile", "Dot-start filename"),
      spec("04-boundary-test/normal-dir/subdir/deep-nested.txt", "Deep nested file content"),
      spec("04-boundary-test/MiXeD-CaSe-FiLe.TxT", "Mixed case filename")
    );
  }

  return specs;
}

// ==================== Orchestration ====================

/**
 * 生成所有 Mock 文件。
 *
 * - writeToDisk 回调：每个 spec 调用一次，path 形如 `${root}/01-plain-media/image/photo.jpg`
 * - onProgress 回调：每个 spec 触发，可用于 UI 进度条
 *
 * 如果未传 writeToDisk，则只 collect 不写盘（用于前端预览 / 单元测试）。
 */
export async function generateMockFiles(opts: GenerateOptions): Promise<GenerateResult> {
  const type = opts.type ?? "all";
  const specs = collectSpecs(type);
  let count = 0;
  let totalSize = 0;
  for (const s of specs) {
    if (opts.writeToDisk) {
      const fullPath = joinPath(opts.root, s.relativePath);
      // 父目录由调用方保证（后端 mock_generator.go 走 os.MkdirAll）
      await opts.writeToDisk(fullPath, s.data);
    }
    count++;
    totalSize += s.size;
    opts.onProgress?.(s);
  }
  return { count, totalSize, specs };
}

/**
 * 列出所有可能生成的相对路径（不含 root）。用于 reset 操作前清空目录。
 */
export function listAllRelativePaths(): string[] {
  return collectSpecs("all").map(s => s.relativePath);
}

/**
 * 🆕 2026-06-15 按 ext 查找 plain 类别中第一个匹配 spec 的 relativePath。
 *
 * 用途：自动化测试 (PluginTestsDetail.vue) 按 plugin.supportedExtensions[0] 派生 sourcePath，
 *       必须跟 mock 后端实际生成的文件名一致（mock 是唯一真相源）。
 *
 * 安全性：每个 plugin 的 supportedExtensions[0] 唯一（mp4/mp3/jpg/txt/pdf/docx），
 *         不会触发 m4a vs m4a-lossless 这种同 ext 多 spec 歧义。
 *
 * @returns spec.relativePath 如 '01-plain-media/audio/music.mp3'；找不到返回 null
 */
export function extToRelativePath(ext: string): string | null {
  const e = normalizeExt(ext);
  const specs = collectSpecs("plain");
  const spec = specs.find(s => s.relativePath.toLowerCase().endsWith("." + e));
  return spec?.relativePath ?? null;
}
