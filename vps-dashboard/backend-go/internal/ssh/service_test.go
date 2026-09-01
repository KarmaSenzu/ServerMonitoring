package ssh_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	vssh "vps-dashboard-api/internal/models"
	vps_ssh "vps-dashboard-api/internal/ssh"
)

// fakeSSHServer is an in-process SSH server that authenticates with
// an authorized public key and returns fixed results per command.
// The host key is generated once per server instance so TOFU
// semantics can be exercised; tests may swap it to simulate MITM.
type fakeSSHServer struct {
	listener   net.Listener
	addr       string
	mu         sync.Mutex
	hostSigner ssh.Signer
	authorized map[string]bool   // base64 public key payload → allowed
	commands   map[string]string // command → stdout
}

func newFakeSSHServer(t *testing.T) *fakeSSHServer {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	hostSigner, err := newHostSigner()
	if err != nil {
		t.Fatalf("host key: %v", err)
	}
	f := &fakeSSHServer{
		listener:   l,
		addr:       l.Addr().String(),
		hostSigner: hostSigner,
		authorized: make(map[string]bool),
		commands:   make(map[string]string),
	}
	t.Cleanup(func() { _ = l.Close() })
	go f.serve()
	return f
}

// setHostSigner atomically swaps the host key (MITM simulation).
func (f *fakeSSHServer) setHostSigner(s ssh.Signer) {
	f.mu.Lock()
	f.hostSigner = s
	f.mu.Unlock()
}

// currentHostSigner returns the active host key signer.
func (f *fakeSSHServer) currentHostSigner() ssh.Signer {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hostSigner
}

func (f *fakeSSHServer) serve() {
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeSSHServer) handle(conn net.Conn) {
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if f.authorized[string(key.Marshal())] {
				return &ssh.Permissions{}, nil
			}
			return nil, errors.New("unknown public key")
		},
	}
	cfg.AddHostKey(f.currentHostSigner())

	sconn, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer func() { _ = sconn.Close() }()
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		chanConn, chanReqs, _ := newChan.Accept()
		go f.handleSession(chanConn, chanReqs)
	}
}

func (f *fakeSSHServer) handleSession(chanConn ssh.Channel, reqs <-chan *ssh.Request) {
	defer func() { _ = chanConn.Close() }()
	for req := range reqs {
		if req.Type == "exec" {
			cmd := parseExecPayload(req.Payload)
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			_, _ = chanConn.Write([]byte(f.commands[cmd]))
			// exit-status 0.
			_, _ = chanConn.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
			_ = chanConn.Close()
			return
		}
		if req.WantReply {
			_ = req.Reply(true, nil)
		}
	}
}

// parseExecPayload decodes the exec request payload: uint32 length
// followed by the command bytes.
func parseExecPayload(payload []byte) string {
	if len(payload) < 4 {
		return ""
	}
	n := int(payload[0])<<24 | int(payload[1])<<16 | int(payload[2])<<8 | int(payload[3])
	if len(payload) < 4+n {
		return ""
	}
	return string(payload[4 : 4+n])
}

// newHostSigner creates a fresh Ed25519 host key signer.
func newHostSigner() (ssh.Signer, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return ssh.NewSignerFromKey(priv)
}

// startServer spins a fake SSH server authorized for the given public
// key and returns a vssh.Server row pointing at it.
func startServer(t *testing.T, public ssh.PublicKey, results map[string]string) (vssh.Server, *fakeSSHServer) {
	t.Helper()

	f := newFakeSSHServer(t)
	f.authorized[string(public.Marshal())] = true
	f.commands = results

	host, portStr, _ := net.SplitHostPort(f.addr)

	s := vssh.Server{
		Name:           "fake-01",
		Hostname:       host,
		IPAddress:      host,
		SSHPort:        mustAtoi(portStr),
		SSHUsername:    "deploy",
		CredentialType: vssh.ServerCredentialSSHKey,
		CredentialRef:  "test-key",
		Environment:    vssh.ServerEnvProduction,
		Enabled:        true,
		Status:         vssh.ServerStatusUnknown,
	}
	return s, f
}

func mustAtoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return n
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// newTestService builds a Service with one stored key ("test-key")
// and returns the service plus the key's public half.
func newTestService(t *testing.T) (*vps_ssh.Service, ssh.PublicKey) {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "keys")
	ks, err := vps_ssh.NewKeyStore(dir)
	if err != nil {
		t.Fatalf("NewKeyStore: %v", err)
	}
	if _, _, err := ks.Generate("test-key", ""); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	pubLine, err := ks.GetPublic("test-key")
	if err != nil {
		t.Fatalf("GetPublic: %v", err)
	}
	parts := strings.SplitN(pubLine, " ", 3)
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.Join(parts[:2], " ")))
	if err != nil {
		t.Fatalf("ParseAuthorizedKey: %v", err)
	}

	svc := vps_ssh.NewService(ks)
	svc.ConnectTimeout = 3 * time.Second
	svc.CommandTimeout = 3 * time.Second
	return svc, pub
}

func TestServiceTestSuccess(t *testing.T) {
	svc, pub := newTestService(t)
	srv, _ := startServer(t, pub, nil)

	res, err := svc.Test(context.Background(), srv)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !res.OK {
		t.Errorf("res.OK: %+v", res)
	}
	if res.LatencyMs < 0 {
		t.Errorf("latency: %d", res.LatencyMs)
	}
	if res.Fingerprint == "" {
		t.Errorf("fingerprint empty")
	}
	if res.ServerVer == "" {
		t.Errorf("server version empty")
	}
}

func TestServiceTestAuthFailure(t *testing.T) {
	svc, _ := newTestService(t)

	// Authorize a *different* key so authentication fails.
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}
	otherPub, err := ssh.NewPublicKey(otherPriv.Public())
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}

	srv, _ := startServer(t, otherPub, nil)

	_, err = svc.Test(context.Background(), srv)
	if !errors.Is(err, vps_ssh.ErrAuthFailed) {
		t.Fatalf("expected ErrAuthFailed, got %v", err)
	}
}

func TestServiceTestUnreachable(t *testing.T) {
	svc, _ := newTestService(t)

	srv := vssh.Server{
		Name:           "dead-01",
		Hostname:       "127.0.0.1",
		SSHPort:        1, // nothing listens here
		SSHUsername:    "deploy",
		CredentialType: vssh.ServerCredentialSSHKey,
		CredentialRef:  "test-key",
	}
	_, err := svc.Test(context.Background(), srv)
	if !errors.Is(err, vps_ssh.ErrHostUnreachable) {
		t.Fatalf("expected ErrHostUnreachable, got %v", err)
	}
}

func TestServiceTestMissingCredential(t *testing.T) {
	svc, _ := newTestService(t)

	srv := vssh.Server{
		Name:           "nokey-01",
		Hostname:       "127.0.0.1",
		SSHPort:        1,
		SSHUsername:    "deploy",
		CredentialType: vssh.ServerCredentialSSHKey,
		CredentialRef:  "missing-key", // not in the store
	}
	_, err := svc.Test(context.Background(), srv)
	if !errors.Is(err, vps_ssh.ErrCredentialNotConfigured) {
		t.Fatalf("expected ErrCredentialNotConfigured, got %v", err)
	}
}

func TestServiceRunCommand(t *testing.T) {
	svc, pub := newTestService(t)
	results := map[string]string{
		"uptime":     "up 42 days",
		"echo hello": "hello",
	}
	srv, _ := startServer(t, pub, results)

	res, err := svc.RunCommand(context.Background(), srv, "uptime")
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if res.Stdout != "up 42 days" {
		t.Errorf("stdout: %q", res.Stdout)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code: %d", res.ExitCode)
	}
	if res.DurationMs < 0 {
		t.Errorf("duration: %d", res.DurationMs)
	}
}

func TestServiceRunCommandUnknown(t *testing.T) {
	svc, pub := newTestService(t)
	srv, _ := startServer(t, pub, map[string]string{})

	res, err := svc.RunCommand(context.Background(), srv, "whatever")
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if res.Stdout != "" {
		t.Errorf("stdout: %q", res.Stdout)
	}
}

func TestServiceRunCommandUnreachable(t *testing.T) {
	svc, _ := newTestService(t)

	srv := vssh.Server{
		Name:           "dead-01",
		Hostname:       "127.0.0.1",
		SSHPort:        1,
		SSHUsername:    "deploy",
		CredentialType: vssh.ServerCredentialSSHKey,
		CredentialRef:  "test-key",
	}
	_, err := svc.RunCommand(context.Background(), srv, "uptime")
	if !errors.Is(err, vps_ssh.ErrHostUnreachable) {
		t.Fatalf("expected ErrHostUnreachable, got %v", err)
	}
}

func TestServiceHostKeyTOFU(t *testing.T) {
	svc, pub := newTestService(t)

	// First connection learns the host key.
	srv, f := startServer(t, pub, nil)
	if _, err := svc.Test(context.Background(), srv); err != nil {
		t.Fatalf("first Test: %v", err)
	}

	// Second connection to the same endpoint must succeed with the
	// same host key.
	res, err := svc.Test(context.Background(), srv)
	if err != nil {
		t.Fatalf("second Test: %v", err)
	}
	if res.Fingerprint == "" {
		t.Fatal("fingerprint missing on second connect")
	}

	// Simulate a MITM: swap in a different host key on the same
	// listener. The service must reject with ErrHostKeyChanged.
	rogue, err := newHostSigner()
	if err != nil {
		t.Fatalf("rogue host key: %v", err)
	}
	f.setHostSigner(rogue)

	_, err = svc.Test(context.Background(), srv)
	if !errors.Is(err, vps_ssh.ErrHostKeyChanged) {
		t.Fatalf("expected ErrHostKeyChanged, got %v", err)
	}
}
