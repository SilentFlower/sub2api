package antigravityls

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
)

// NewTCPRelay 创建 TCP Relay 实例
// listenAddr: 监听地址（如 "127.0.0.2:443"）
// proxyAddr: SOCKS5 代理地址（如 "127.0.0.1:7890"）
// targetHost: 目标主机（如 "cloudcode-pa.googleapis.com"）
// targetPort: 目标端口（如 443）
func NewTCPRelay(listenAddr, proxyAddr, targetHost string, targetPort int) *TCPRelay {
	return &TCPRelay{
		listenAddr: listenAddr,
		proxyAddr:  proxyAddr,
		targetHost: targetHost,
		targetPort: targetPort,
		done:       make(chan struct{}),
	}
}

// Start 启动 TCP Relay（非阻塞），在后台 goroutine 中接受连接
func (r *TCPRelay) Start() error {
	ln, err := net.Listen("tcp", r.listenAddr)
	if err != nil {
		return fmt.Errorf("TCP Relay 监听 %s 失败: %w", r.listenAddr, err)
	}
	r.listener = ln
	slog.Info("TCP Relay 已启动",
		"listenAddr", r.listenAddr,
		"targetHost", r.targetHost,
		"targetPort", r.targetPort,
		"proxyAddr", r.proxyAddr,
	)

	go r.acceptLoop()
	return nil
}

// Stop 停止 TCP Relay
func (r *TCPRelay) Stop() error {
	close(r.done)
	if r.listener != nil {
		return r.listener.Close()
	}
	return nil
}

// acceptLoop 接受连接的主循环
func (r *TCPRelay) acceptLoop() {
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			select {
			case <-r.done:
				return // 正常关闭
			default:
				slog.Error("TCP Relay Accept 失败", "error", err)
				continue
			}
		}
		go r.handleConn(conn)
	}
}

// handleConn 处理单个客户端连接：通过 SOCKS5 代理连接目标，双向转发数据
func (r *TCPRelay) handleConn(clientConn net.Conn) {
	defer clientConn.Close()

	remoteConn, err := r.socks5Connect(r.targetHost, r.targetPort)
	if err != nil {
		slog.Error("SOCKS5 连接失败",
			"target", fmt.Sprintf("%s:%d", r.targetHost, r.targetPort),
			"error", err,
		)
		return
	}
	defer remoteConn.Close()

	slog.Debug("TCP Relay 转发建立",
		"client", clientConn.RemoteAddr(),
		"target", fmt.Sprintf("%s:%d", r.targetHost, r.targetPort),
	)

	// 双向转发
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(remoteConn, clientConn)
	}()
	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, remoteConn)
	}()

	wg.Wait()
}

// socks5Connect 通过 SOCKS5 代理建立到目标服务器的 TCP 连接
func (r *TCPRelay) socks5Connect(targetHost string, targetPort int) (net.Conn, error) {
	conn, err := net.Dial("tcp", r.proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("连接 SOCKS5 代理 %s 失败: %w", r.proxyAddr, err)
	}

	// SOCKS5 握手 — 无认证模式
	if _, err := conn.Write([]byte{0x05, 0x01, 0x00}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 握手发送失败: %w", err)
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 握手响应读取失败: %w", err)
	}
	if resp[0] != 0x05 || resp[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 握手失败: 响应 %x", resp)
	}

	// SOCKS5 连接请求（域名模式 0x03）
	hostBytes := []byte(targetHost)
	connectReq := make([]byte, 0, 7+len(hostBytes))
	connectReq = append(connectReq, 0x05, 0x01, 0x00, 0x03) // VER, CMD=CONNECT, RSV, ATYP=DOMAINNAME
	connectReq = append(connectReq, byte(len(hostBytes)))    // 域名长度
	connectReq = append(connectReq, hostBytes...)            // 域名
	connectReq = append(connectReq, byte(targetPort>>8), byte(targetPort&0xff)) // 端口（大端序）

	if _, err := conn.Write(connectReq); err != nil {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 连接请求发送失败: %w", err)
	}

	// 读取连接响应（至少 10 字节）
	connectResp := make([]byte, 10)
	if _, err := io.ReadFull(conn, connectResp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 连接响应读取失败: %w", err)
	}
	if connectResp[1] != 0x00 {
		conn.Close()
		return nil, fmt.Errorf("SOCKS5 连接被拒绝: status %d", connectResp[1])
	}

	return conn, nil
}
