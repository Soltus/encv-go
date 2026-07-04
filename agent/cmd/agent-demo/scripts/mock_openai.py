#!/usr/bin/env python3
"""Mock OpenAI server for end-to-end smoke test of agent-demo."""
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer


def make_sse_body():
    return (
        "data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\"hello\"},\"index\":0}]}\n"
        "data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"delta\":{\"content\":\" world\"},\"index\":0}]}\n"
        "data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"finish_reason\":\"stop\",\"delta\":{}}]}\n"
        "data: [DONE]\n"
    )


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        if self.path != "/v1/chat/completions":
            self.send_response(404)
            self.end_headers()
            return
        body = make_sse_body()
        self.send_response(200)
        self.send_header("Content-Type", "text/event-stream")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body.encode())

    def do_GET(self):
        if self.path == "/v1/models":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"data":[{"id":"gpt-4o"}]}')
            return
        self.send_response(404)
        self.end_headers()

    def log_message(self, fmt, *args):
        pass


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 9999
    HTTPServer(("127.0.0.1", port), Handler).serve_forever()
