#!/usr/bin/env python3
"""shim.py: a reverse proxy with a health route, standard library only.

GET /health    200 if the upstream accepts TCP connections, else 503.
anything else  forwarded to the upstream unchanged (method, path, headers, body).
"""

from __future__ import annotations

import argparse
import http.client
import socket
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

HOP = {"connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te",
       "trailers", "transfer-encoding", "upgrade", "host", "content-length"}


def make_handler(host: str, port: int, timeout: float):
    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def log_message(self, fmt, *args):  # one line per request on stderr
            super().log_message(fmt, *args)

        def _alive(self) -> bool:
            try:
                with socket.create_connection((host, port), timeout=2):
                    return True
            except OSError:
                return False

        def _reply(self, code: int, body: bytes, ctype: str = "text/plain") -> None:
            self.send_response(code)
            self.send_header("Content-Type", ctype)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def _forward(self) -> None:
            if self.path == "/health":
                ok = self._alive()
                self._reply(200 if ok else 503, b"ok\n" if ok else b"upstream down\n")
                return
            n = int(self.headers.get("Content-Length") or 0)
            body = self.rfile.read(n) if n else None
            headers = {k: v for k, v in self.headers.items() if k.lower() not in HOP}
            conn = http.client.HTTPConnection(host, port, timeout=timeout)
            try:
                conn.request(self.command, self.path, body=body, headers=headers)
                resp = conn.getresponse()
                data = resp.read()
            except OSError as e:
                self._reply(502, f"upstream error: {e}\n".encode())
                return
            finally:
                conn.close()
            self.send_response(resp.status)
            for k, v in resp.getheaders():
                if k.lower() not in HOP:
                    self.send_header(k, v)
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        do_GET = do_POST = do_PUT = do_DELETE = do_PATCH = _forward

    return Handler


def main(argv: list[str] | None = None) -> None:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--listen", default="0.0.0.0:8080")
    p.add_argument("--upstream", default="127.0.0.1:5000")
    p.add_argument("--timeout", type=float, default=170.0)
    a = p.parse_args(argv)
    lh, lp = a.listen.rsplit(":", 1)
    uh, up = a.upstream.rsplit(":", 1)
    ThreadingHTTPServer((lh, int(lp)), make_handler(uh, int(up), a.timeout)).serve_forever()


if __name__ == "__main__":
    main()
