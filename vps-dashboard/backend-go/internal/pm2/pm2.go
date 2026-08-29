// Package pm2 wraps the `pm2 jlist` CLI command into a small typed
// service. It never goes through a shell; safeexec validates and runs
// the binary with a context timeout.
package pm2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/safeexec"
)

// ErrPM2Unavailable is returned by List when the pm2 binary is not on
// PATH. Handlers translate this to a soft "pm2_unavailable" indicator
// rather than a 5xx.
var ErrPM2Unavailable = errors.New("pm2: not installed")

// Process is the JSON-friendly view of a single pm2 application.
type Process struct {
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	ScriptPath  string  `json:"script_path"`
	Cwd         string  `json:"cwd"`
	Interpreter string  `json:"interpreter"`
	PID         int     `json:"pid"`
	Restarts    int     `json:"restarts"`
	Uptime      int64   `json:"uptime_seconds"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryBytes int64   `json:"memory_bytes"`
}

// Service is the higher-level pm2 wrapper used by the API.
type Service struct {
	Logger zerolog.Logger
}

// NewService constructs a Service.
func NewService(l zerolog.Logger) *Service {
	return &Service{Logger: l}
}

// List shells out to `pm2 jlist`, parses the JSON, and returns a
// normalized slice of Process. If pm2 is not on PATH it returns
// ErrPM2Unavailable so callers can skip it without surfacing a 5xx.
func (s *Service) List(ctx context.Context) ([]Process, error) {
	stdout, stderr, err := safeexec.Run(ctx, "pm2", "jlist")
	if err != nil {
		if isPM2Missing(err) {
			return nil, ErrPM2Unavailable
		}
		return nil, fmt.Errorf("pm2 jlist: %w (stderr=%q)", err, stderr)
	}
	return parseJList(strings.NewReader(stdout))
}

// Start invokes `pm2 start <name>`. The name is validated against the
// allowlist before exec.
func (s *Service) Start(ctx context.Context, name string) error {
	return s.runAction(ctx, "start", name)
}

// Stop invokes `pm2 stop <name>`.
func (s *Service) Stop(ctx context.Context, name string) error {
	return s.runAction(ctx, "stop", name)
}

// Restart invokes `pm2 restart <name>`.
func (s *Service) Restart(ctx context.Context, name string) error {
	return s.runAction(ctx, "restart", name)
}

// Reload invokes `pm2 reload <name>` (zero-downtime reload when the
// process is running in cluster mode).
func (s *Service) Reload(ctx context.Context, name string) error {
	return s.runAction(ctx, "reload", name)
}

// Delete invokes `pm2 delete <name>`. Admin-only at the route level.
func (s *Service) Delete(ctx context.Context, name string) error {
	return s.runAction(ctx, "delete", name)
}

func (s *Service) runAction(ctx context.Context, action, name string) error {
	if err := ValidateProcessName(name); err != nil {
		return err
	}
	_, stderr, err := safeexec.Run(ctx, "pm2", action, name)
	if err != nil {
		if isPM2Missing(err) {
			return ErrPM2Unavailable
		}
		return fmt.Errorf("pm2 %s %s: %w (stderr=%q)", action, name, err, stderr)
	}
	return nil
}

// Logs reads recent log lines for the named pm2 process by invoking
// `pm2 logs <name> --nostream --lines <n> --raw`. PM2 prints both stdout
// and stderr interleaved on stdout, so stderr is left empty for now
// (separate stream split is deferred). lines must be 1..5000.
func (s *Service) Logs(ctx context.Context, name string, lines int) (string, string, error) {
	if err := ValidateProcessName(name); err != nil {
		return "", "", err
	}
	if lines < 1 || lines > 5000 {
		return "", "", fmt.Errorf("invalid lines: must be 1..5000")
	}
	stdout, stderr, err := safeexec.Run(ctx, "pm2", "logs", name,
		"--nostream", "--lines", strconv.Itoa(lines), "--raw")
	if err != nil {
		if isPM2Missing(err) {
			return "", "", ErrPM2Unavailable
		}
		return stdout, "", fmt.Errorf("pm2 logs %s: %w (stderr=%q)", name, err, stderr)
	}
	return stdout, "", nil
}

// jlistEntry mirrors the subset of fields we read from pm2's output.
type jlistEntry struct {
	Name   string `json:"name"`
	PID    int    `json:"pid"`
	PM2Env struct {
		Status      string `json:"status"`
		PMUptime    int64  `json:"pm_uptime"`
		RestartTime int    `json:"restart_time"`
		Interpreter string `json:"exec_interpreter"`
		ExecPath    string `json:"pm_exec_path"`
		Cwd         string `json:"pm_cwd"`
	} `json:"pm2_env"`
	Monit struct {
		CPU    float64 `json:"cpu"`
		Memory int64   `json:"memory"`
	} `json:"monit"`
}

// parseJList decodes pm2's `jlist` output. Entries with empty names are
// skipped; uptime is computed only for "online" entries with a positive
// pm_uptime value.
//
// PM2 occasionally prints daemon-spawn banners to stdout before the JSON
// payload (e.g. "[PM2] Spawning PM2 daemon..."). To stay robust we strip
// any leading banner lines (lines that aren't a bare "[" or "{" or a
// JSON-formatted block) before decoding.
func parseJList(r io.Reader) ([]Process, error) {
	all, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("pm2: read jlist: %w", err)
	}

	payload := stripBanner(all)
	if len(payload) == 0 {
		return []Process{}, nil
	}

	var raw []jlistEntry
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("pm2: decode jlist: %w", err)
	}

	nowMs := time.Now().UnixMilli()
	out := make([]Process, 0, len(raw))
	for _, e := range raw {
		if strings.TrimSpace(e.Name) == "" {
			continue
		}
		var uptimeSec int64
		if e.PM2Env.Status == "online" && e.PM2Env.PMUptime > 0 {
			diff := nowMs - e.PM2Env.PMUptime
			if diff > 0 {
				uptimeSec = diff / 1000
			}
		}
		out = append(out, Process{
			Name:        e.Name,
			Status:      e.PM2Env.Status,
			ScriptPath:  e.PM2Env.ExecPath,
			Cwd:         e.PM2Env.Cwd,
			Interpreter: e.PM2Env.Interpreter,
			PID:         e.PID,
			Restarts:    e.PM2Env.RestartTime,
			Uptime:      uptimeSec,
			CPUPercent:  e.Monit.CPU,
			MemoryBytes: e.Monit.Memory,
		})
	}
	return out, nil
}

func isPM2Missing(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "no such file or directory")
}

// stripBanner trims leading non-JSON lines from PM2 stdout. It looks
// for the first line whose trimmed content begins with `[` or `{` AND
// is not a `[PM2]` banner line.
func stripBanner(in []byte) []byte {
	lines := bytes.Split(in, []byte("\n"))
	for i, line := range lines {
		t := bytes.TrimSpace(line)
		if len(t) == 0 {
			continue
		}
		// Skip "[PM2] ..." and "[PM2][...] ..." banner lines.
		if bytes.HasPrefix(t, []byte("[PM2]")) {
			continue
		}
		if t[0] == '[' || t[0] == '{' {
			return bytes.Join(lines[i:], []byte("\n"))
		}
	}
	return nil
}
