package ssh

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"vps-dashboard-api/internal/models"
)

// LiveTunnel is a running tunnel connection. The caller starts it with
// ConnectByServer and stops it with Close.
type LiveTunnel struct {
	mu        sync.Mutex
	Type      string
	LocalAddr string
	status    string
	err       string
	listener  net.Listener
	client    *ssh.Client
	cancel    context.CancelFunc
	done      chan struct{}
}

// Status returns the current tunnel state.
func (lt *LiveTunnel) Status() string {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	return lt.status
}

// Error returns the last error (if status == "error").
func (lt *LiveTunnel) Error() string {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	return lt.err
}

func (lt *LiveTunnel) setStatus(s, e string) {
	lt.mu.Lock()
	lt.status = s
	lt.err = e
	lt.mu.Unlock()
}

// Close shuts down the tunnel: stops the listener, closes the SSH
// client, and cancels the accept loop.
func (lt *LiveTunnel) Close() error {
	lt.setStatus("stopped", "")
	if lt.cancel != nil {
		lt.cancel()
	}
	if lt.listener != nil {
		_ = lt.listener.Close()
	}
	if lt.client != nil {
		_ = lt.client.Close()
	}
	if lt.done != nil {
		<-lt.done
	}
	return nil
}

// TunnelManager maintains live SSH tunnels. It is safe for concurrent
// use.
type TunnelManager struct {
	mu      sync.Mutex
	tunnels map[string]*LiveTunnel // tunnel ID → live session
	engine  *Service
}

// NewTunnelManager constructs a TunnelManager bound to the SSH engine.
func NewTunnelManager(engine *Service) *TunnelManager {
	return &TunnelManager{
		tunnels: make(map[string]*LiveTunnel),
		engine:  engine,
	}
}

// Get returns the live tunnel for a tunnel ID, or nil if not running.
func (tm *TunnelManager) Get(tunnelID string) *LiveTunnel {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.tunnels[tunnelID]
}

// ConnectByServer opens a tunnel to a registered server. The tunnel
// runs in the background until Close is called or the SSH connection
// breaks.
func (tm *TunnelManager) ConnectByServer(ctx context.Context, tunnelID string, server models.Server, tunnelType, localAddr, remoteAddr string) (*LiveTunnel, error) {
	// Close any existing tunnel with the same ID.
	if existing := tm.Get(tunnelID); existing != nil {
		_ = existing.Close()
	}

	client, err := tm.engine.DialClient(ctx, server)
	if err != nil {
		return nil, fmt.Errorf("tunnel: dial: %w", err)
	}

	lt := &LiveTunnel{
		Type:      tunnelType,
		LocalAddr: localAddr,
		client:     client,
		done:       make(chan struct{}),
	}

	tunnelCtx, cancel := context.WithCancel(ctx)
	lt.cancel = cancel

	switch tunnelType {
	case models.TunnelTypeLocal:
		if err := tm.startLocal(tunnelCtx, lt, client, localAddr, remoteAddr); err != nil {
			_ = client.Close()
			return nil, err
		}
	case models.TunnelTypeRemote:
		if err := tm.startRemote(tunnelCtx, lt, client, localAddr, remoteAddr); err != nil {
			_ = client.Close()
			return nil, err
		}
	case models.TunnelTypeSocks:
		if err := tm.startSocks(tunnelCtx, lt, client, localAddr); err != nil {
			_ = client.Close()
			return nil, err
		}
	default:
		_ = client.Close()
		return nil, fmt.Errorf("tunnel: unknown type %q", tunnelType)
	}

	tm.mu.Lock()
	tm.tunnels[tunnelID] = lt
	tm.mu.Unlock()

	lt.setStatus("active", "")
	return lt, nil
}

// Disconnect closes a running tunnel by ID.
func (tm *TunnelManager) Disconnect(tunnelID string) error {
	tm.mu.Lock()
	lt, ok := tm.tunnels[tunnelID]
	if ok {
		delete(tm.tunnels, tunnelID)
	}
	tm.mu.Unlock()
	if !ok {
		return fmt.Errorf("tunnel: %s not running", tunnelID)
	}
	return lt.Close()
}

// maxConcurrentTunnels limits the number of concurrent forwarded connections
// per tunnel to prevent unbounded goroutine creation (DoS protection).
const maxConcurrentTunnels = 100

// keepaliveInterval is how often we send a keepalive request to the SSH
// server to detect dead connections and keep the tunnel alive through
// idle timeouts.
const keepaliveInterval = 30 * time.Second

// startLocal creates a local port forward: listen on localAddr,
// forward connections to remoteAddr via the SSH client.
// Uses a bounded semaphore to prevent unbounded goroutine creation.
// Starts a keepalive goroutine to detect dead connections.
func (tm *TunnelManager) startLocal(ctx context.Context, lt *LiveTunnel, client *ssh.Client, localAddr, remoteAddr string) error {
	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("tunnel: local listen %q: %w", localAddr, err)
	}
	lt.listener = listener

	// Start keepalive goroutine
	go tm.keepalive(ctx, lt, client)

	// Bounded semaphore: max 100 concurrent forwarded connections
	sem := make(chan struct{}, maxConcurrentTunnels)

	go func() {
		defer close(lt.done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				lt.setStatus("disconnected", "listener closed")
				return
			}
			// Acquire semaphore (non-blocking if slots available)
			select {
			case sem <- struct{}{}:
			default:
				// At capacity — reject connection
				_ = conn.Close()
				continue
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close(); <-sem }()
				remote, err := client.Dial("tcp", remoteAddr)
				if err != nil {
					lt.setStatus("error", "remote dial: "+err.Error())
					return
				}
				defer func() { _ = remote.Close() }()
				go func() { _, _ = io.Copy(remote, c); _ = remote.Close() }()
				_, _ = io.Copy(c, remote)
			}(conn)
		}
	}()

	return nil
}

// startRemote creates a remote port forward: the SSH server listens on
// remoteAddr and forwards connections back to localAddr.
// Uses a bounded semaphore to prevent unbounded goroutine creation.
func (tm *TunnelManager) startRemote(ctx context.Context, lt *LiveTunnel, client *ssh.Client, localAddr, remoteAddr string) error {
	listener, err := client.Listen("tcp", remoteAddr)
	if err != nil {
		return fmt.Errorf("tunnel: remote listen %q: %w", remoteAddr, err)
	}
	lt.listener = listener

	// Start keepalive goroutine
	go tm.keepalive(ctx, lt, client)

	// Bounded semaphore
	sem := make(chan struct{}, maxConcurrentTunnels)

	go func() {
		defer close(lt.done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				lt.setStatus("disconnected", "listener closed")
				return
			}
			select {
			case sem <- struct{}{}:
			default:
				_ = conn.Close()
				continue
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close(); <-sem }()
				local, err := net.Dial("tcp", localAddr)
				if err != nil {
					lt.setStatus("error", "local dial: "+err.Error())
					return
				}
				defer func() { _ = local.Close() }()
				go func() { _, _ = io.Copy(local, c); _ = local.Close() }()
				_, _ = io.Copy(c, local)
			}(conn)
		}
	}()

	return nil
}

// startSocks creates a dynamic SOCKS5 proxy: listen on localAddr,
// negotiate SOCKS5 per connection, then dial the requested target via
// the SSH client. Uses a bounded semaphore.
func (tm *TunnelManager) startSocks(ctx context.Context, lt *LiveTunnel, client *ssh.Client, localAddr string) error {
	// Enforce loopback-only binding for SOCKS to prevent open proxy
	if !strings.HasPrefix(localAddr, "127.0.0.1") && !strings.HasPrefix(localAddr, "localhost") && !strings.HasPrefix(localAddr, "::1") {
		return fmt.Errorf("tunnel: SOCKS proxy must bind to loopback (127.0.0.1), got %q", localAddr)
	}

	listener, err := net.Listen("tcp", localAddr)
	if err != nil {
		return fmt.Errorf("tunnel: socks listen %q: %w", localAddr, err)
	}
	lt.listener = listener

	// Start keepalive goroutine
	go tm.keepalive(ctx, lt, client)

	// Bounded semaphore
	sem := make(chan struct{}, maxConcurrentTunnels)

	go func() {
		defer close(lt.done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				lt.setStatus("disconnected", "listener closed")
				return
			}
			select {
			case sem <- struct{}{}:
			default:
				_ = conn.Close()
				continue
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close(); <-sem }()
				handleSocksConnection(client, conn)
			}(conn)
		}
	}()

	return nil
}

// keepalive sends periodic keepalive requests to the SSH server to:
// 1. Keep the connection alive through idle timeouts
// 2. Detect dead connections (network drop, server restart)
// On failure, sets tunnel status to "disconnected" and closes the tunnel.
func (tm *TunnelManager) keepalive(ctx context.Context, lt *LiveTunnel, client *ssh.Client) {
	ticker := time.NewTicker(keepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Send keepalive request — if it fails, the connection is dead
			_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
			if err != nil {
				lt.setStatus("disconnected", "keepalive failed: "+err.Error())
				// Close the tunnel — the listener will unblock and the accept
				// loop will exit, closing lt.done
				if lt.listener != nil {
					_ = lt.listener.Close()
				}
				_ = client.Close()
				return
			}
		}
	}
}

// handleSocksConnection serves a single SOCKS5 client.
func handleSocksConnection(sshClient *ssh.Client, client net.Conn) {
	defer func() { _ = client.Close() }()

	// SOCKS5 greeting: version, nmethods, methods.
	buf := make([]byte, 2)
	if _, err := io.ReadFull(client, buf); err != nil {
		return
	}
	if buf[0] != 5 {
		return
	}
	nMethods := int(buf[1])
	if _, err := io.ReadFull(client, make([]byte, nMethods)); err != nil {
		return
	}
	// Reply: no auth.
	_, _ = client.Write([]byte{5, 0})

	// Request: version, cmd, rsv, atyp, addr, port.
	header := make([]byte, 4)
	if _, err := io.ReadFull(client, header); err != nil {
		return
	}
	if header[0] != 5 || header[1] != 1 { // only CONNECT
		_, _ = client.Write([]byte{5, 0x07, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}

	var host string
	switch header[3] {
	case 1: // IPv4
		ip := make([]byte, 4)
		if _, err := io.ReadFull(client, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	case 3: // Domain
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(client, lenBuf); err != nil {
			return
		}
		domain := make([]byte, int(lenBuf[0]))
		if _, err := io.ReadFull(client, domain); err != nil {
			return
		}
		host = string(domain)
	case 4: // IPv6
		ip := make([]byte, 16)
		if _, err := io.ReadFull(client, ip); err != nil {
			return
		}
		host = net.IP(ip).String()
	default:
		return
	}
	portBuf := make([]byte, 2)
	if _, err := io.ReadFull(client, portBuf); err != nil {
		return
	}
	port := binary.BigEndian.Uint16(portBuf)
	target := fmt.Sprintf("%s:%d", host, port)

	// Dial via SSH.
	remote, err := sshClient.Dial("tcp", target)
	if err != nil {
		_, _ = client.Write([]byte{5, 0x01, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer func() { _ = remote.Close() }()

	// Success reply.
	_, _ = client.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0})

	// Bidirectional pipe.
	go func() { _, _ = io.Copy(remote, client); _ = remote.Close() }()
	_, _ = io.Copy(client, remote)
}
