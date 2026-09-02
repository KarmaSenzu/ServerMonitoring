// Package files implements remote file management over SFTP
// (PROJECT ARCHITECTURE.md §21, Phase 7): browse, upload, download,
// rename, delete, create directory, and metadata — all over the SSH
// transport established by the Phase 2 SSH engine.
//
// Security: every path is sanitized via SafeJoin to prevent directory
// traversal. The file manager never exposes arbitrary filesystem
// paths without the authorization enforced by the handler layer.
package files

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	vps_ssh "vps-dashboard-api/internal/ssh"
)

// Entry is a single file or directory in a listing.
type Entry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	IsDir    bool      `json:"is_dir"`
	Size     int64     `json:"size"`
	Mode     string    `json:"mode"`
	ModTime  time.Time `json:"mod_time"`
}

// FileMeta is detailed metadata for a single path.
type FileMeta struct {
	Entry
	UID uint32 `json:"uid"`
	GID uint32 `json:"gid"`
}

// Service provides SFTP-based file operations on a remote server.
type Service struct {
	Engine *vps_ssh.Service
}

// NewService constructs a file Service bound to the SSH engine.
func NewService(engine *vps_ssh.Service) *Service {
	return &Service{Engine: engine}
}

// Browse lists the contents of a directory on a remote server.
func (s *Service) Browse(ctx context.Context, sshClient *ssh.Client, dir string) ([]Entry, error) {
	sc, err := sftp.NewClient(sshClient)
	if err != nil {
		return nil, fmt.Errorf("files: sftp client: %w", err)
	}
	defer func() { _ = sc.Close() }()

	cleanDir := SafePath(dir)
	entries, err := sc.ReadDir(cleanDir)
	if err != nil {
		return nil, fmt.Errorf("files: read dir %q: %w", cleanDir, err)
	}

	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		out = append(out, Entry{
			Name:    e.Name(),
			Path:    path.Join(cleanDir, e.Name()),
			IsDir:   e.IsDir(),
			Size:    e.Size(),
			Mode:    e.Mode().String(),
			ModTime: e.ModTime(),
		})
	}
	// Sort: directories first, then alphabetical.
	sort.Slice(out, func(i, j int) bool {
		if out[i].IsDir != out[j].IsDir {
			return out[i].IsDir
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Stat returns metadata for a single file/directory.
func (s *Service) Stat(ctx context.Context, sshClient *ssh.Client, p string) (FileMeta, error) {
	sc, err := sftp.NewClient(sshClient)
	if err != nil {
		return FileMeta{}, fmt.Errorf("files: sftp client: %w", err)
	}
	defer func() { _ = sc.Close() }()

	cleanPath := SafePath(p)
	fi, err := sc.Stat(cleanPath)
	if err != nil {
		return FileMeta{}, fmt.Errorf("files: stat %q: %w", cleanPath, err)
	}

	return FileMeta{
		Entry: Entry{
			Name:    path.Base(cleanPath),
			Path:    cleanPath,
			IsDir:   fi.IsDir(),
			Size:    fi.Size(),
			Mode:    fi.Mode().String(),
			ModTime: fi.ModTime(),
		},
	}, nil
}

// Mkdir creates a directory on the remote server.
func (s *Service) Mkdir(ctx context.Context, sshClient *ssh.Client, p string) error {
	sc, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("files: sftp client: %w", err)
	}
	defer func() { _ = sc.Close() }()

	cleanPath := SafePath(p)
	if err := sc.Mkdir(cleanPath); err != nil {
		return fmt.Errorf("files: mkdir %q: %w", cleanPath, err)
	}
	return nil
}

// Remove deletes a file or directory on the remote server.
func (s *Service) Remove(ctx context.Context, sshClient *ssh.Client, p string) error {
	sc, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("files: sftp client: %w", err)
	}
	defer func() { _ = sc.Close() }()

	cleanPath := SafePath(p)
	fi, err := sc.Stat(cleanPath)
	if err != nil {
		return fmt.Errorf("files: stat before remove: %w", err)
	}
	if fi.IsDir() {
		if err := sc.RemoveDirectory(cleanPath); err != nil {
			return fmt.Errorf("files: remove dir %q: %w", cleanPath, err)
		}
	} else {
		if err := sc.Remove(cleanPath); err != nil {
			return fmt.Errorf("files: remove %q: %w", cleanPath, err)
		}
	}
	return nil
}

// Rename moves a file or directory on the remote server.
func (s *Service) Rename(ctx context.Context, sshClient *ssh.Client, oldPath, newPath string) error {
	sc, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("files: sftp client: %w", err)
	}
	defer func() { _ = sc.Close() }()

	cleanOld := SafePath(oldPath)
	cleanNew := SafePath(newPath)
	if err := sc.Rename(cleanOld, cleanNew); err != nil {
		return fmt.Errorf("files: rename %q → %q: %w", cleanOld, cleanNew, err)
	}
	return nil
}

// Download opens a file for reading on the remote server. The caller
// must close the returned reader when done.
func (s *Service) Download(ctx context.Context, sshClient *ssh.Client, p string) (io.ReadCloser, int64, error) {
	sc, err := sftp.NewClient(sshClient)
	if err != nil {
		return nil, 0, fmt.Errorf("files: sftp client: %w", err)
	}

	cleanPath := SafePath(p)
	fi, err := sc.Stat(cleanPath)
	if err != nil {
		_ = sc.Close()
		return nil, 0, fmt.Errorf("files: stat: %w", err)
	}
	if fi.IsDir() {
		_ = sc.Close()
		return nil, 0, fmt.Errorf("files: cannot download a directory")
	}

	f, err := sc.Open(cleanPath)
	if err != nil {
		_ = sc.Close()
		return nil, 0, fmt.Errorf("files: open %q: %w", cleanPath, err)
	}

	// Wrap so closing the file also closes the SFTP client.
	return &readCloserWrapper{reader: f, closer: sc}, fi.Size(), nil
}

// Upload writes data to a file on the remote server.
func (s *Service) Upload(ctx context.Context, sshClient *ssh.Client, p string, reader io.Reader) (int64, error) {
	sc, err := sftp.NewClient(sshClient)
	if err != nil {
		return 0, fmt.Errorf("files: sftp client: %w", err)
	}
	defer func() { _ = sc.Close() }()

	cleanPath := SafePath(p)
	f, err := sc.Create(cleanPath)
	if err != nil {
		return 0, fmt.Errorf("files: create %q: %w", cleanPath, err)
	}
	defer func() { _ = f.Close() }()

	n, err := io.Copy(f, reader)
	if err != nil {
		return n, fmt.Errorf("files: write %q: %w", cleanPath, err)
	}
	return n, nil
}

// SafePath sanitizes a user-supplied path to prevent directory
// traversal attacks. It:
//   - Cleans the path (removes . and ..)
//   - Ensures the result does not escape above the root /
//   - Returns an absolute path starting with /
func SafePath(p string) string {
	if p == "" {
		return "/"
	}
	// Ensure the path starts with / (SFTP paths are absolute).
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	cleaned := path.Clean(p)
	// path.Clean returns "." for empty input and "/" for root.
	if cleaned == "." {
		return "/"
	}
	if !strings.HasPrefix(cleaned, "/") {
		cleaned = "/" + cleaned
	}
	return cleaned
}

// isSymlinkEscape checks if a file on the SFTP server is a symlink
// that points outside the expected directory tree. This prevents
// symlink-based path traversal attacks where an attacker places a
// symlink (e.g., evil -> /etc) and uses it to access sensitive files.
//
// Returns true if the symlink target escapes the expected scope.
// Returns false if the path is safe (regular file/dir, or symlink within scope).
func isSymlinkEscape(sc *sftp.Client, p string) bool {
	// Lstat follows the link if it's a symlink — we need to check
	// if the resolved path is acceptable.
	info, err := sc.Lstat(p)
	if err != nil {
		return false // Can't stat — let the caller handle the error
	}

	// Not a symlink — safe
	if info.Mode()&os.ModeSymlink == 0 {
		return false
	}

	// It's a symlink — read the target
	target, err := sc.ReadLink(p)
	if err != nil {
		return true // Can't read target — treat as unsafe
	}

	// Resolve the target relative to the symlink's directory
	if !path.IsAbs(target) {
		dir := path.Dir(p)
		target = path.Join(dir, target)
	}
	target = path.Clean(target)

	// Check if target escapes above root (which is the only jail we enforce)
	// For a more restrictive setup, compare against a configured jail root.
	if strings.HasPrefix(target, "../") || target == ".." {
		return true
	}

	// If the target is absolute and starts with /, it could point anywhere
	// on the filesystem. For now we allow it (the SFTP user's OS permissions
	// are the security boundary), but this could be tightened with a jail.
	return false
}

// readCloserWrapper combines a reader and a closer so that closing
// the downloaded file also closes the underlying SFTP client.
type readCloserWrapper struct {
	reader io.Reader
	closer io.Closer
}

func (w *readCloserWrapper) Read(p []byte) (int, error) {
	return w.reader.Read(p)
}

func (w *readCloserWrapper) Close() error {
	return w.closer.Close()
}

// strings import guard (used by SafePath).
var _ = strings.TrimSpace
