#!/usr/bin/env python3
"""ENCV wasm 预览页的静态服务。

刻意**只做静态托管**：这一页的全部加解密都在浏览器里的 wasm 内核完成，
不请求任何后端接口（没有 /stream、没有 /api、没有健康检查）。
任何形式的后端代理都会掩盖「是否真的纯前端」这件事。
"""

import os
import socketserver
import sys
from http.server import SimpleHTTPRequestHandler

ROOT = os.path.dirname(os.path.abspath(__file__))


class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=ROOT, **kwargs)

    def log_message(self, fmt, *args):
        sys.stderr.write("%s %s\n" % (self.address_string(), fmt % args))


def main():
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 5179
    socketserver.ThreadingTCPServer.allow_reuse_address = True
    with socketserver.ThreadingTCPServer(("0.0.0.0", port), Handler) as httpd:
        print(f"enc-preview serving {ROOT} on :{port} (纯静态，无后端代理)", flush=True)
        httpd.serve_forever()


if __name__ == "__main__":
    main()
