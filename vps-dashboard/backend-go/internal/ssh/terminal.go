package ssh

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"

	"vps-dashboard-api/internal/models"
)

// PTYSize is the initial terminal dimensions requested from the remote
// PTY. The frontend sends rows/cols over WebSocket; a resize message
// updates them mid-session.
type PTYSize struct {
	Rows   int
	Cols   int
	Width  int // pixels (0 = don't care)
	Height int
}

// TerminalSession is a live interactive SSH PTY. Data written to stdin
// is forwarded to the remote shell; bytes from the remote shell are
// available on stdout. Resize updates the PTY window.
type TerminalSession struct {
	mu     sync.Mutex
	client *ssh.Client
	sess   *ssh.Session

	stdin  io.WriteCloser
	stdout io.Reader

	done   chan struct{}
	closed bool
}

// NewTerminalSession dials the server, requests a PTY with the given
// size, and starts an interactive shell. The caller owns the session
// lifecycle: call Close when the WebSocket disconnects.
func (svc *Service) NewTerminalSession(ctx context.Context, server models.Server, size PTYSize) (*TerminalSession, error) {
	if size.Rows <= 0 {
		size.Rows = 24
	}
	if size.Cols <= 0 {
		size.Cols = 80
	}

	addr := endpoint(server)
	if addr == ":" || strings.HasPrefix(addr, ":") {
		return nil, fmt.Errorf("%w: no hostname or IP configured", ErrHostUnreachable)
	}

	auth, err := svc.authMethods(server)
	if err != nil {
		return nil, err
	}

	connectTimeout := svc.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = DefaultConnectTimeout
	}

	cfg := &ssh.ClientConfig{
		User:            server.SSHUsername,
		Auth:            auth,
		HostKeyCallback: svc.hostKeyCallback(addr),
		Timeout:         connectTimeout,
	}

	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, classifyDialError(err)
	}

	sess, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ssh: new session: %w", err)
	}

	// Request a PTY with the terminal type "xterm-256color".
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", size.Rows, size.Cols, modes); err != nil {
		_ = sess.Close()
		_ = client.Close()
		return nil, fmt.Errorf("ssh: request pty: %w", err)
	}

	stdin, err := sess.StdinPipe()
	if err != nil {
		_ = sess.Close()
		_ = client.Close()
		return nil, fmt.Errorf("ssh: stdin pipe: %w", err)
	}
	stdout, _ := sess.StdoutPipe()
	stderr, _ := sess.StderrPipe()

	if err := sess.Shell(); err != nil {
		_ = sess.Close()
		_ = client.Close()
		return nil, fmt.Errorf("ssh: start shell: %w", err)
	}

	// Merge stderr into stdout so the terminal sees everything.
	merged := io.MultiReader(stdout, stderr)

	ts := &TerminalSession{
		client: client,
		sess:   sess,
		stdin:  stdin,
		stdout: merged,
		done:   make(chan struct{}),
	}

	go func() {
		_ = sess.Wait()
		close(ts.done)
	}()

	return ts, nil
}

// Read implements io.Reader — bytes from the remote PTY.
func (ts *TerminalSession) Read(p []byte) (int, error) {
	return ts.stdout.Read(p)
}

// Write implements io.Writer — keystrokes from the WebSocket.
func (ts *TerminalSession) Write(p []byte) (int, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.closed {
		return 0, fmt.Errorf("terminal: session closed")
	}
	return ts.stdin.Write(p)
}

// Resize updates the PTY window size mid-session.
func (ts *TerminalSession) Resize(rows, cols int) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.closed {
		return fmt.Errorf("terminal: session closed")
	}
	if rows <= 0 || cols <= 0 {
		return nil
	}
	ok, err := ts.sess.SendRequest("window-change", true, ssh.Marshal(struct {
		Rows   uint32
		Cols   uint32
		Width  uint32
		Height uint32
	}{uint32(rows), uint32(cols), 0, 0}))
	if err != nil {
		return fmt.Errorf("ssh: window-change: %w", err)
	}
	if !ok {
		return fmt.Errorf("ssh: window-change rejected")
	}
	return nil
}

// Done returns a channel that is closed when the remote shell exits.
func (ts *TerminalSession) Done() <-chan struct{} {
	return ts.done
}

// Close terminates the SSH session and underlying connection.
func (ts *TerminalSession) Close() error {
	ts.mu.Lock()
	if ts.closed {
		ts.mu.Unlock()
		return nil
	}
	ts.closed = true
	ts.mu.Unlock()

	_ = ts.stdin.Close()
	_ = ts.sess.Close()
	return ts.client.Close()
}

// classifyDialError wraps raw SSH dial errors into typed sentinels.
func classifyDialError(err error) error {
	if err == nil {
		return nil
	}
	if isAuthFailure(err) {
		return fmt.Errorf("%w: %s", ErrAuthFailed, err.Error())
	}
	if isUnreachable(err) {
		return fmt.Errorf("%w: %s", ErrHostUnreachable, err.Error())
	}
	// Check for host key mismatch (string match since handshake wraps).
	if isHostKeyMismatch(err) {
		return fmt.Errorf("%w: %s", ErrHostKeyChanged, err.Error())
	}
	return fmt.Errorf("ssh: dial: %w", err)
}

// isHostKeyMismatch detects host key mismatch errors from the known
// hosts callback (wrapped via fmt.Errorf in the handshake).
func isHostKeyMismatch(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() != "" && (contains(err.Error(), "host key changed") ||
		contains(err.Error(), "host key mismatch"))
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(sub) > 0 && indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// strings is used for HasPrefix on the endpoint address.
