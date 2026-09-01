// Package ssh implements the SSH engine for the Infrastructure
// Platform (Phase 2): connecting to registered servers, testing
// connectivity, and executing bounded commands using the Go
// golang.org/x/crypto/ssh package.
//
// Design notes (PROJECT ARCHITECTURE.md §9-§11):
//   - Agentless: the backend speaks SSH directly to remote servers.
//   - Credentials are stored by reference. Private key material is
//     persisted in a dedicated key store directory (0600) and never
//     returned through the API — only metadata (name, fingerprint).
//   - Every operation is bounded by a context deadline.
package ssh

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Key store errors.
var (
	// ErrKeyNotFound is returned when a key with the given name does
	// not exist in the store.
	ErrKeyNotFound = errors.New("ssh key: not found")

	// ErrKeyExists is returned when adding a key whose name is already
	// present in the store.
	ErrKeyExists = errors.New("ssh key: already exists")

	// ErrInvalidKey is returned when the supplied PEM payload is not a
	// parseable private key.
	ErrInvalidKey = errors.New("ssh key: invalid PEM private key")
)

// keyNameRE constrains store entry names (also used as file names).
var keyNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.\-]{0,63}$`)

// KeyMeta is the safe metadata shape exposed over the API. It never
// contains private key material.
type KeyMeta struct {
	Name        string    `json:"name"`
	Type        string    `json:"type"`
	Fingerprint string    `json:"fingerprint"`
	Comment     string    `json:"comment"`
	CreatedAt   time.Time `json:"created_at"`
}

// KeyStore persists SSH private keys on the local filesystem, one PEM
// file per key under a dedicated directory. Files are created with
// 0600 permissions and the directory is expected to be owned by the
// service user. This is the "secure credential store" referenced by
// the Server Registry's credential_ref column.
type KeyStore struct {
	mu  sync.Mutex
	dir string
}

// NewKeyStore constructs a KeyStore rooted at dir. The directory is
// created (0700) when missing.
func NewKeyStore(dir string) (*KeyStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("ssh keystore: dir is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ssh keystore: mkdir %q: %w", dir, err)
	}
	return &KeyStore{dir: dir}, nil
}

// path returns the on-disk path for a key name.
func (ks *KeyStore) path(name string) string {
	return filepath.Join(ks.dir, name)
}

// Add persists a private key under the given name. The PEM payload is
// parsed first so invalid keys are rejected before being written.
// Returns the metadata of the stored key. Fingerprints use the
// SHA256:base64 form (same as OpenSSH).
func (ks *KeyStore) Add(name, pemData string) (KeyMeta, error) {
	if !keyNameRE.MatchString(name) {
		return KeyMeta{}, fmt.Errorf("ssh keystore: invalid key name %q", name)
	}

	signer, comment, err := parsePrivateKey([]byte(pemData))
	if err != nil {
		return KeyMeta{}, fmt.Errorf("%w: %s", ErrInvalidKey, err.Error())
	}
	if c := strings.TrimSpace(comment); c != "" {
		comment = c
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()

	if _, err := os.Stat(ks.path(name)); err == nil {
		return KeyMeta{}, ErrKeyExists
	}

	meta := KeyMeta{
		Name:        name,
		Type:        signer.PublicKey().Type(),
		Fingerprint: ssh.FingerprintSHA256(signer.PublicKey()),
		Comment:     comment,
		CreatedAt:   time.Now().UTC(),
	}

	if err := os.WriteFile(ks.path(name), []byte(pemData), 0o600); err != nil {
		return KeyMeta{}, fmt.Errorf("ssh keystore: write %q: %w", name, err)
	}
	return meta, nil
}

// Generate creates a new Ed25519 key pair and stores the private half
// under the given name. Returns the metadata plus the OpenSSH-format
// public key line that can be distributed to remote servers
// (authorized_keys).
func (ks *KeyStore) Generate(name, comment string) (KeyMeta, string, error) {
	if !keyNameRE.MatchString(name) {
		return KeyMeta{}, "", fmt.Errorf("ssh keystore: invalid key name %q", name)
	}

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return KeyMeta{}, "", fmt.Errorf("ssh keystore: generate: %w", err)
	}

	block, err := ssh.MarshalPrivateKey(priv, strings.TrimSpace(comment))
	if err != nil {
		return KeyMeta{}, "", fmt.Errorf("ssh keystore: marshal: %w", err)
	}
	pemData := pem.EncodeToMemory(block)

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return KeyMeta{}, "", fmt.Errorf("ssh keystore: signer: %w", err)
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()

	if _, err := os.Stat(ks.path(name)); err == nil {
		return KeyMeta{}, "", ErrKeyExists
	}

	meta := KeyMeta{
		Name:        name,
		Type:        signer.PublicKey().Type(),
		Fingerprint: ssh.FingerprintSHA256(signer.PublicKey()),
		Comment:     strings.TrimSpace(comment),
		CreatedAt:   time.Now().UTC(),
	}

	if err := os.WriteFile(ks.path(name), pemData, 0o600); err != nil {
		return KeyMeta{}, "", fmt.Errorf("ssh keystore: write %q: %w", name, err)
	}
	return meta, string(pubKeyLine(signer, strings.TrimSpace(comment))), nil
}

// List returns the metadata of every stored key, sorted by name.
// Unparseable entries are skipped with no error (log-worthy but not
// fatal) — a corrupt file must not take the whole listing down.
func (ks *KeyStore) List() ([]KeyMeta, error) {
	entries, err := os.ReadDir(ks.dir)
	if err != nil {
		return nil, fmt.Errorf("ssh keystore: list: %w", err)
	}

	out := make([]KeyMeta, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !keyNameRE.MatchString(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(ks.path(name))
		if err != nil {
			continue
		}
		signer, comment, err := parsePrivateKey(raw)
		if err != nil {
			// Skip unparseable; the file may be a leftover artifact.
			continue
		}
		out = append(out, KeyMeta{
			Name:        name,
			Type:        signer.PublicKey().Type(),
			Fingerprint: ssh.FingerprintSHA256(signer.PublicKey()),
			Comment:     comment,
			CreatedAt:   info.ModTime().UTC(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get loads a stored key and returns its metadata.
func (ks *KeyStore) Get(name string) (KeyMeta, error) {
	signer, err := ks.signer(name)
	if err != nil {
		return KeyMeta{}, err
	}
	return KeyMeta{
		Name:        name,
		Type:        signer.PublicKey().Type(),
		Fingerprint: ssh.FingerprintSHA256(signer.PublicKey()),
	}, nil
}

// GetPublic returns the OpenSSH-format public key line for a stored
// key. This is safe to expose to admins — it is public material.
func (ks *KeyStore) GetPublic(name string) (string, error) {
	signer, err := ks.signer(name)
	if err != nil {
		return "", err
	}
	return pubKeyLine(signer, ""), nil
}

// Remove deletes a key from the store.
func (ks *KeyStore) Remove(name string) error {
	if !keyNameRE.MatchString(name) {
		return fmt.Errorf("ssh keystore: invalid key name %q", name)
	}
	ks.mu.Lock()
	defer ks.mu.Unlock()

	if err := os.Remove(ks.path(name)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrKeyNotFound
		}
		return fmt.Errorf("ssh keystore: remove %q: %w", name, err)
	}
	return nil
}

// signer loads and parses a stored private key.
func (ks *KeyStore) signer(name string) (ssh.Signer, error) {
	if !keyNameRE.MatchString(name) {
		return nil, fmt.Errorf("ssh keystore: invalid key name %q", name)
	}
	raw, err := os.ReadFile(ks.path(name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("ssh keystore: read %q: %w", name, err)
	}
	signer, _, err := parsePrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrInvalidKey, err.Error())
	}
	return signer, nil
}

// parsePrivateKey accepts an OpenSSH or PEM (PKCS#1/PKCS#8) private
// key payload and returns the signer plus the embedded comment when
// available. Passphrase-protected keys are rejected: an unattended
// control plane cannot prompt for passphrases.
func parsePrivateKey(raw []byte) (ssh.Signer, string, error) {
	signer, err := ssh.ParsePrivateKey(raw)
	if err != nil {
		if errors.Is(err, &ssh.PassphraseMissingError{}) {
			return nil, "", fmt.Errorf("passphrase-protected keys are not supported")
		}
		return nil, "", fmt.Errorf("unsupported private key format: %w", err)
	}
	return signer, openSSHComment(raw), nil
}

// openSSHComment extracts the comment from an OpenSSH-format private
// key PEM block by walking the openssh-key-v1 wire format. Returns ""
// for non-OpenSSH PEM (PKCS#1/PKCS#8) and for encrypted sections
// (their layout differs and they are rejected at parse time anyway).
func openSSHComment(raw []byte) string {
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "OPENSSH PRIVATE KEY" {
		return ""
	}
	b := block.Bytes
	magic := []byte("openssh-key-v1\x00")
	if !bytes.HasPrefix(b, magic) {
		return ""
	}
	b = b[len(magic):]

	// ciphername, kdfname, kdfoptions — all length-prefixed strings.
	for i := 0; i < 3; i++ {
		_, rest, ok := readSSHString(b)
		if !ok {
			return ""
		}
		b = rest
	}
	if len(b) < 4 { // numkeys
		return ""
	}
	b = b[4:]
	if _, rest, ok := readSSHString(b); ok { // pubkey blob
		b = rest
	} else {
		return ""
	}
	priv, _, ok := readSSHBytes(b) // private section
	if !ok {
		return ""
	}
	if len(priv) < 8 { // checkint1 + checkint2
		return ""
	}
	p := priv[8:]

	keyType, rest, ok := readSSHString(p)
	if !ok {
		return ""
	}
	p = rest

	// Skip key material; every field is a length-prefixed string for
	// the types we support.
	fieldCounts := map[string]int{
		"ssh-ed25519":         2, // pubkey + private key
		"ssh-rsa":             6, // n, e, d, iqmp, p, q
		"ecdsa-sha2-nistp256": 3, // curve, pubkey, privkey
		"ecdsa-sha2-nistp384": 3,
		"ecdsa-sha2-nistp521": 3,
	}
	n, supported := fieldCounts[keyType]
	if !supported {
		return ""
	}
	for i := 0; i < n; i++ {
		_, rest, ok := readSSHString(p)
		if !ok {
			return ""
		}
		p = rest
	}

	comment, _, ok := readSSHString(p)
	if !ok {
		return ""
	}
	return comment
}

// readSSHString reads one SSH wire-format (length-prefixed) string.
func readSSHString(b []byte) (string, []byte, bool) {
	s, rest, ok := readSSHBytes(b)
	return string(s), rest, ok
}

// readSSHBytes reads one length-prefixed payload without copying it
// to a string.
func readSSHBytes(b []byte) ([]byte, []byte, bool) {
	if len(b) < 4 {
		return nil, nil, false
	}
	n := int(binary.BigEndian.Uint32(b[:4]))
	if n < 0 || len(b) < 4+n {
		return nil, nil, false
	}
	return b[4 : 4+n], b[4+n:], true
}

// pubKeyLine renders the OpenSSH one-line public key format
// ("ssh-ed25519 AAAA... comment").
func pubKeyLine(signer ssh.Signer, comment string) string {
	line := fmt.Sprintf("%s %s", signer.PublicKey().Type(), base64.StdEncoding.EncodeToString(signer.PublicKey().Marshal()))
	if comment != "" {
		line += " " + comment
	}
	return line
}

// fingerprintHostKey returns the SHA256:base64 fingerprint of a host
// key, used by the known_hosts-equivalent TOFU store.
func fingerprintHostKey(key ssh.PublicKey) string {
	sum := sha256.Sum256(key.Marshal())
	return "SHA256:" + base64.StdEncoding.EncodeToString(sum[:])
}
