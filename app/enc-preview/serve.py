#!/usr/bin/env python3
"""ENCV wasm 预览页的静态服务。

刻意**只做静态托管**：这一页的全部加解密都在浏览器里的 wasm 内核完成，
不请求任何后端接口（没有 /stream、没有 /api、没有健康检查）。
任何形式的后端代理都会掩盖「是否真的纯前端」这件事。

唯一比 SimpleHTTPRequestHandler 多做的事是**支持 HTTP Range**：
流式打开（api.openStream）要按区间取字节，服务端不认 Range 就只能整个下载，
那"大容器不必整体进内存"这条就在网络这一环上漏掉了。
"""

import os
import socketserver
import sys
from http.server import SimpleHTTPRequestHandler

ROOT = os.path.dirname(os.path.abspath(__file__))
CHUNK = 64 * 1024


class Handler(SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=ROOT, **kwargs)

    def log_message(self, fmt, *args):
        sys.stderr.write("%s %s\n" % (self.address_string(), fmt % args))

    def do_GET(self):
        path = self.translate_path(self.path)
        if os.path.isdir(path) or not os.path.isfile(path):
            return super().do_GET()

        rng = self.headers.get("Range")
        size = os.path.getsize(path)
        start, end = 0, size - 1
        status = 200
        if rng and rng.startswith("bytes="):
            spec = rng[6:].split(",")[0].strip()
            a, _, b = spec.partition("-")
            try:
                if a:
                    start = int(a)
                if b:
                    end = min(int(b), size - 1)
            except ValueError:
                self.send_error(400, "bad Range")
                return
            if start > end or start >= size:
                self.send_response(416)
                self.send_header("Content-Range", f"bytes */{size}")
                self.send_header("Content-Length", "0")
                self.end_headers()
                return
            status = 206

        length = end - start + 1
        self.send_response(status)
        self.send_header("Content-type", self.guess_type(path))
        self.send_header("Content-Length", str(length))
        self.send_header("Accept-Ranges", "bytes")
        if status == 206:
            self.send_header("Content-Range", f"bytes {start}-{end}/{size}")
        self.end_headers()

        if self.command == "HEAD":
            return
        with open(path, "rb") as f:
            f.seek(start)
            left = length
            while left > 0:
                chunk = f.read(min(CHUNK, left))
                if not chunk:
                    break
                self.wfile.write(chunk)
                left -= len(chunk)


def main():
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 5179
    socketserver.ThreadingTCPServer.allow_reuse_address = True
    with socketserver.ThreadingTCPServer(("0.0.0.0", port), Handler) as httpd:
        print(f"enc-preview serving {ROOT} on :{port} (纯静态，支持 Range)", flush=True)
        httpd.serve_forever()


if __name__ == "__main__":
    main()
