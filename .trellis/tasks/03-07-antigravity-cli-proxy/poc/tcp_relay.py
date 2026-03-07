#!/usr/bin/env python3
"""
TCP Relay — 通过 SOCKS5 代理转发 gRPC 连接。

用途：
  LS 的 gRPC 连接（ApiServerClientV2）不走 HTTP 代理，直连 Google IP。
  通过 DNS 劫持将域名解析到 127.0.0.2，再由本脚本通过 SOCKS5 代理转发。

前置条件：
  1. /etc/hosts 中添加：127.0.0.2 cloudcode-pa.googleapis.com
  2. SOCKS5 代理已在 PROXY_HOST:PROXY_PORT 监听

启动：
  python3 tcp_relay.py
"""
import socket
import select
import threading
import sys

# 监听地址 — 对应 /etc/hosts 中劫持的 IP
LISTEN_ADDR = "127.0.0.2"
LISTEN_PORT = 443

# SOCKS5 代理地址
PROXY_HOST = "127.0.0.1"
PROXY_PORT = 7890

# 目标地址 — 真实的 Google API
TARGET_HOST = "cloudcode-pa.googleapis.com"
TARGET_PORT = 443


def socks5_connect(proxy_host, proxy_port, target_host, target_port):
    """通过 SOCKS5 代理连接目标服务器。"""
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.connect((proxy_host, proxy_port))

    # SOCKS5 握手 — 无认证
    sock.send(b'\x05\x01\x00')
    resp = sock.recv(2)
    if resp != b'\x05\x00':
        raise Exception(f"SOCKS5 握手失败: {resp}")

    # SOCKS5 连接请求（域名模式）
    target_bytes = target_host.encode()
    req = (b'\x05\x01\x00\x03'
           + bytes([len(target_bytes)])
           + target_bytes
           + target_port.to_bytes(2, 'big'))
    sock.send(req)

    resp = sock.recv(10)
    if resp[1] != 0:
        raise Exception(f"SOCKS5 连接失败: status {resp[1]}")

    return sock


def relay(client_sock, remote_sock):
    """在两个 socket 之间双向转发数据。"""
    socks = [client_sock, remote_sock]
    while True:
        readable, _, exceptional = select.select(socks, [], socks, 30)
        if exceptional:
            break
        for s in readable:
            data = s.recv(65536)
            if not data:
                return
            other = remote_sock if s is client_sock else client_sock
            other.sendall(data)


def handle_client(client_sock, addr):
    """处理单个客户端连接。"""
    try:
        remote = socks5_connect(PROXY_HOST, PROXY_PORT, TARGET_HOST, TARGET_PORT)
        print(f"[+] Relay {addr} <-> {TARGET_HOST}:{TARGET_PORT} via SOCKS5", flush=True)
        relay(client_sock, remote)
    except Exception as e:
        print(f"[-] Error for {addr}: {e}", flush=True)
    finally:
        client_sock.close()


def main():
    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    server.bind((LISTEN_ADDR, LISTEN_PORT))
    server.listen(64)
    print(f"[*] TCP Relay 监听 {LISTEN_ADDR}:{LISTEN_PORT}", flush=True)
    print(f"[*] 转发至 {TARGET_HOST}:{TARGET_PORT} via {PROXY_HOST}:{PROXY_PORT}", flush=True)

    while True:
        client_sock, addr = server.accept()
        t = threading.Thread(target=handle_client, args=(client_sock, addr), daemon=True)
        t.start()


if __name__ == "__main__":
    main()
