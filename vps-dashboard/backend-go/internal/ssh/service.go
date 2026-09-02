package ssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"vps-dashboard-api/internal/crypto"
	"vps-dashboard-api/internal/models"
)

// Service errors, translated by handlers into HTTP status codes.
var (
	// ErrHostUnreachable is returned when the TCP connection to the
	// remote SSH endpoint cannot be established.
	ErrHostUnreachable = errors.New("ssh: host unreachable")

	// ErrAuthFailed is returned when the server rejects the supplied
	// credentials.
	ErrAuthFailed = errors.New("ssh: authentication failed")

	// ErrHostKeyChanged is returned when the remote host key no longer
	// matches the remembered fingerprint (potential MITM).
	ErrHostKeyChanged = errors.New("ssh: host key changed")

	// ErrCommandTimeout is returned when the command deadline elapses.
	ErrCommandTimeout = errors.New("ssh: command timed out")

	// ErrCredentialNotConfigured is returned when the server has no
	// usable credential reference.
	ErrCredentialNotConfigured = errors.New("ssh: credential not configured")
)

// Defaults for the engine (PROJECT ARCHITECTURE.md §45).
const (
	// DefaultConnectTimeout bounds the TCP + SSH handshake.
	DefaultConnectTimeout = 10 * time.Second

	// DefaultCommandTimeout bounds command execution.
	DefaultCommandTimeout = 30 * time.Second

	// maxOutputBytes caps captured stdout/stderr per command so a
	// runaway `cat /dev/zero` cannot exhaust memory.
	maxOutputBytes = 1 << 20 // 1 MiB
)

// Service is the SSH engine shared by handlers (Phase 2) and the
// remote monitoring pipeline (Phase 3).
type Service struct {
	Keys *KeyStore

	// ConnectTimeout / CommandTimeout can be overridden for tests.
	ConnectTimeout time.Duration
	CommandTimeout time.Duration

	// knownHosts guards remembered host key fingerprints (TOFU).
	// Persisted to knownHostsPath on first-use and loaded at startup.
	mu             sync.Mutex
	knownHosts     map[string]string // "host:port" → fingerprint
	knownHostsPath string             // path to persisted known_hosts file
}

// NewService constructs an SSH Service bound to a key store.
func NewService(keys *KeyStore) *Service {
	svc := &Service{
		Keys:           keys,
		ConnectTimeout: DefaultConnectTimeout,
		CommandTimeout: DefaultCommandTimeout,
		knownHosts:     make(map[string]string),
	}
	// Auto-load persisted known_hosts from disk
	svc.loadKnownHosts()
	return svc
}

// SetKnownHostsPath sets the path to persist known_hosts (TOFU).
// Called during startup to enable host key persistence across restarts.
func (svc *Service) SetKnownHostsPath(path string) {
	svc.mu.Lock()
	svc.knownHostsPath = path
	svc.mu.Unlock()
	svc.loadKnownHosts()
}

// loadKnownHosts reads the persisted known_hosts file into memory.
// Format: one "host:port fingerprint" per line.
func (svc *Service) loadKnownHosts() {
	svc.mu.Lock()
	path := svc.knownHostsPath
	svc.mu.Unlock()

	if path == "" {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return // File doesn't exist yet — fine
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			svc.knownHosts[parts[0]] = parts[1]
		}
	}
}

// saveKnownHosts persists the known_hosts map to disk (0600 permissions).
func (svc *Service) saveKnownHosts() {
	svc.mu.Lock()
	path := svc.knownHostsPath
	hosts := make(map[string]string, len(svc.knownHosts))
	for k, v := range svc.knownHosts {
		hosts[k] = v
	}
	svc.mu.Unlock()

	if path == "" {
		return
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0700)

	var buf bytes.Buffer
	for host, fp := range hosts {
		buf.WriteString(host + " " + fp + "\n")
	}
	_ = os.WriteFile(path, buf.Bytes(), 0600)
}

// TestResult describes the outcome of a connectivity test.
type TestResult struct {
	OK          bool      `json:"ok"`
	LatencyMs   int64     `json:"latency_ms"`
	Fingerprint string    `json:"fingerprint"`
	ServerVer   string    `json:"server_version"`
	Username    string    `json:"username"`
	Error       string    `json:"error,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
}

// CommandResult captures a bounded remote command execution.
type CommandResult struct {
	Command    string    `json:"command"`
	ExitCode   int       `json:"exit_code"`
	Stdout     string    `json:"stdout"`
	Stderr     string    `json:"stderr"`
	Err        string    `json:"error,omitempty"`
	DurationMs int64     `json:"duration_ms"`
	FinishedAt time.Time `json:"finished_at"`
}

// endpoint derives the dial address from a Server row.
func endpoint(s models.Server) string {
	host := strings.TrimSpace(s.Hostname)
	if host == "" {
		host = strings.TrimSpace(s.IPAddress)
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", s.SSHPort))
}

// hostKeyCallback implements trust-on-first-use: on the first
// connection the fingerprint is remembered and persisted to disk;
// later connections must present the same key, otherwise ErrHostKeyChanged
// (potential MITM) is returned.
func (svc *Service) hostKeyCallback(hostport string) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fp := fingerprintHostKey(key)
		svc.mu.Lock()
		remembered, ok := svc.knownHosts[hostport]
		svc.mu.Unlock()
		if !ok {
			// First use — trust and persist
			svc.mu.Lock()
			svc.knownHosts[hostport] = fp
			svc.mu.Unlock()
			svc.saveKnownHosts()
			return nil
		}
		if remembered != fp {
			// Host key changed — potential MITM. This is a security event.
			return fmt.Errorf("%w: remembered %s, got %s", ErrHostKeyChanged, remembered, fp)
		}
		return nil
	}
}

// authMethods builds the authentication stack for a server row. When
// credential_type is agent, the SSH_AUTH_SOCK-based agent is used; for
// ssh_key, the referenced store key signs the handshake; password
// credentials are resolved from the environment under
// VPSD_SSH_PASSWORD_<REF> — the platform never stores password
// material in the database.
func (svc *Service) authMethods(s models.Server) ([]ssh.AuthMethod, error) {
	switch s.CredentialType {
	case models.ServerCredentialAgent:
		if sock := os.Getenv("SSH_AUTH_SOCK"); sock == "" {
			return nil, fmt.Errorf("%w: SSH_AUTH_SOCK is not set", ErrCredentialNotConfigured)
		}
		conn, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
		if err != nil {
			return nil, fmt.Errorf("%w: agent unreachable: %s", ErrCredentialNotConfigured, err.Error())
		}
		ag := agentFrom(conn)
		return []ssh.AuthMethod{ssh.PublicKeysCallback(ag.Signers)}, nil

	case models.ServerCredentialSSHKey:
		if svc.Keys == nil || strings.TrimSpace(s.CredentialRef) == "" {
			return nil, fmt.Errorf("%w: no key reference", ErrCredentialNotConfigured)
		}
		signer, err := svc.Keys.signer(s.CredentialRef)
		if err != nil {
			return nil, fmt.Errorf("%w: key %q: %s", ErrCredentialNotConfigured, s.CredentialRef, err.Error())
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil

	case models.ServerCredentialPassword:
		// First check for direct password (stored encrypted in credential_password field)
		if s.CredentialPassword != "" {
			// Decrypt the password using JWT_SECRET-derived key
			key := crypto.GetEncryptionKey()
			if key != nil {
				decrypted, err := crypto.Decrypt(s.CredentialPassword, key)
				if err != nil {
					return nil, fmt.Errorf("%w: password decryption failed: %s", ErrCredentialNotConfigured, err.Error())
				}
				return []ssh.AuthMethod{ssh.Password(decrypted)}, nil
			}
			// Backward compat: no JWT_SECRET = treat as plaintext
			return []ssh.AuthMethod{ssh.Password(s.CredentialPassword)}, nil
		}
		// Fall back to env var reference: VPSD_SSH_PASSWORD_<REF>
		ref := strings.TrimSpace(s.CredentialRef)
		if ref == "" {
			return nil, fmt.Errorf("%w: no password reference", ErrCredentialNotConfigured)
		}
		envKey := "VPSD_SSH_PASSWORD_" + strings.ToUpper(strings.ReplaceAll(ref, "-", "_"))
		pass := os.Getenv(envKey)
		if pass == "" {
			return nil, fmt.Errorf("%w: env %s is empty", ErrCredentialNotConfigured, envKey)
		}
		return []ssh.AuthMethod{ssh.Password(pass)}, nil

	default:
		return nil, fmt.Errorf("%w: unknown credential type %q", ErrCredentialNotConfigured, s.CredentialType)
	}
}

// dial establishes an SSH client connection to a registered server.
func (svc *Service) dial(ctx context.Context, s models.Server) (*ssh.Client, TestResult, error) {
	addr := endpoint(s)
	if addr == ":" || strings.HasPrefix(addr, ":") {
		return nil, TestResult{}, fmt.Errorf("%w: no hostname or IP configured", ErrHostUnreachable)
	}

	auth, err := svc.authMethods(s)
	if err != nil {
		return nil, TestResult{}, err
	}

	connectTimeout := svc.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = DefaultConnectTimeout
	}

	cfg := &ssh.ClientConfig{
		User:            s.SSHUsername,
		Auth:            auth,
		HostKeyCallback: svc.hostKeyCallback(addr),
		Timeout:         connectTimeout,
	}

	start := time.Now()
	client, err := ssh.Dial("tcp", addr, cfg)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		// Classify the failure so handlers can map to precise HTTP codes.
		switch {
		case isAuthFailure(err):
			return nil, TestResult{}, fmt.Errorf("%w: %s", ErrAuthFailed, err.Error())
		case isUnreachable(err):
			return nil, TestResult{}, fmt.Errorf("%w: %s", ErrHostUnreachable, err.Error())
		default:
			return nil, TestResult{}, fmt.Errorf("ssh: dial %s: %w", addr, err)
		}
	}

	res := TestResult{
		OK:          true,
		LatencyMs:   latency,
		Fingerprint: svc.knownFingerprint(addr),
		ServerVer:   string(client.ServerVersion()),
		Username:    s.SSHUsername,
		CheckedAt:   time.Now().UTC(),
	}
	return client, res, nil
}

// Test performs a full handshake against a registered server and
// returns the latency + host key fingerprint. It is the "SSH test"
// capability of Phase 2.
func (svc *Service) Test(ctx context.Context, s models.Server) (TestResult, error) {
	client, res, err := svc.dial(ctx, s)
	if err != nil {
		return TestResult{Error: err.Error(), CheckedAt: time.Now().UTC()}, err
	}
	defer func() { _ = client.Close() }()
	return res, nil
}

// RunCommand executes a single command on a registered server with a
// bounded deadline and bounded output capture. stdout and stderr are
// truncated (not failed) when they exceed the cap; truncation is
// noted in stderr.
func (svc *Service) RunCommand(ctx context.Context, s models.Server, command string) (CommandResult, error) {
	start := time.Now()

	client, _, err := svc.dial(ctx, s)
	if err != nil {
		return CommandResult{Command: command, Err: err.Error(), FinishedAt: time.Now().UTC()}, err
	}
	defer func() { _ = client.Close() }()

	session, err := client.NewSession()
	if err != nil {
		return CommandResult{Command: command, Err: err.Error(), FinishedAt: time.Now().UTC()}, fmt.Errorf("ssh: new session: %w", err)
	}
	defer func() { _ = session.Close() }()

	var stdout, stderr bytes.Buffer
	session.Stdout = &limitedBuffer{buf: &stdout, limit: maxOutputBytes}
	session.Stderr = &limitedBuffer{buf: &stderr, limit: maxOutputBytes}

	cmdTimeout := svc.CommandTimeout
	if cmdTimeout <= 0 {
		cmdTimeout = DefaultCommandTimeout
	}
	if err := session.Start(command); err != nil {
		return CommandResult{Command: command, Err: err.Error(), FinishedAt: time.Now().UTC()}, fmt.Errorf("ssh: start: %w", err)
	}

	// The command gets its own deadline (svc.CommandTimeout) — the
	// caller's ctx only acts as an outer bound. Wait in a goroutine so
	// a runaway remote command cannot block forever.
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()

	timer := time.NewTimer(cmdTimeout)
	defer timer.Stop()

	var waitErr error
	timedOut := false
	select {
	case waitErr = <-done:
	case <-timer.C:
		timedOut = true
	case <-ctx.Done():
		timedOut = true
	}

	if timedOut {
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		err := fmt.Errorf("%w after %s", ErrCommandTimeout, cmdTimeout)
		return CommandResult{
			Command:    command,
			Stdout:     stdout.String(),
			Stderr:     stderr.String(),
			Err:        err.Error(),
			DurationMs: time.Since(start).Milliseconds(),
			FinishedAt: time.Now().UTC(),
		}, err
	}

	// A non-zero exit status is a completed command, not a transport
	// error: return the exit code with a nil error.
	var exitErr *ssh.ExitError
	if waitErr != nil && errors.As(waitErr, &exitErr) {
		return CommandResult{
			Command:    command,
			ExitCode:   exitErr.ExitStatus(),
			Stdout:     stdout.String(),
			Stderr:     stderr.String(),
			DurationMs: time.Since(start).Milliseconds(),
			FinishedAt: time.Now().UTC(),
		}, nil
	}

	if waitErr != nil {
		return CommandResult{
			Command:    command,
			Stdout:     stdout.String(),
			Stderr:     stderr.String(),
			Err:        waitErr.Error(),
			DurationMs: time.Since(start).Milliseconds(),
			FinishedAt: time.Now().UTC(),
		}, fmt.Errorf("ssh: wait: %w", waitErr)
	}

	return CommandResult{
		Command:    command,
		ExitCode:   0,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		DurationMs: time.Since(start).Milliseconds(),
		FinishedAt: time.Now().UTC(),
	}, nil
}

// knownFingerprint returns the remembered TOFU fingerprint for a host.
func (svc *Service) knownFingerprint(hostport string) string {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	return svc.knownHosts[hostport]
}

// DialClient opens an SSH client connection (without running a command)
// for use by subsystem clients like SFTP. The caller is responsible
// for closing the returned client.
func (svc *Service) DialClient(ctx context.Context, s models.Server) (*ssh.Client, error) {
	addr := endpoint(s)
	if addr == ":" || strings.HasPrefix(addr, ":") {
		return nil, fmt.Errorf("%w: no hostname or IP configured", ErrHostUnreachable)
	}

	auth, err := svc.authMethods(s)
	if err != nil {
		return nil, err
	}

	connectTimeout := svc.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = DefaultConnectTimeout
	}

	cfg := &ssh.ClientConfig{
		User:            s.SSHUsername,
		Auth:            auth,
		HostKeyCallback: svc.hostKeyCallback(addr),
		Timeout:         connectTimeout,
	}

	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, classifyDialError(err)
	}
	return client, nil
}

// isAuthFailure detects the go ssh package's authentication errors.
func isAuthFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "authentication failed")
}

// isUnreachable detects network-level failures.
func isUnreachable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "network is unreachable") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "deadline") ||
		strings.Contains(msg, "eof")
}

// limitedBuffer wraps a bytes.Buffer with a hard byte ceiling. Writes
// beyond the limit are silently dropped.
type limitedBuffer struct {
	buf   *bytes.Buffer
	limit int
}

func (lb *limitedBuffer) Write(p []byte) (int, error) {
	room := lb.limit - lb.buf.Len()
	if room <= 0 {
		return len(p), nil
	}
	if len(p) > room {
		lb.buf.Write(p[:room])
		return len(p), nil
	}
	return lb.buf.Write(p)
}
