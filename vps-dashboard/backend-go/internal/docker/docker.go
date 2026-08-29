// Package docker wraps the Docker CLI in a small service that lists
// containers and starts/stops/restarts them, all without going through
// a shell. Every external call is mediated by safeexec so arguments are
// never interpreted as shell tokens.
package docker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/safeexec"
)

// ErrDockerUnavailable is returned by List when the docker binary is
// not on PATH (or otherwise not reachable). Handlers translate this to
// HTTP 503.
var ErrDockerUnavailable = errors.New("docker: not installed or not reachable")

// Container is the JSON-friendly shape returned by the service.
type Container struct {
	ID        string    `json:"id"`
	ShortID   string    `json:"shortId"`
	Name      string    `json:"name"`
	Image     string    `json:"image"`
	State     string    `json:"state"`
	Status    string    `json:"status"`
	Ports     string    `json:"ports"`
	CreatedAt time.Time `json:"createdAt"`
}

// Service exposes the higher-level docker operations used by the API.
type Service struct {
	Logger zerolog.Logger
}

// NewService constructs a Service. Logger is used for warning-level
// diagnostics; nothing in this package fails because of logging.
func NewService(l zerolog.Logger) *Service {
	return &Service{Logger: l}
}

// List returns the full set of local containers (running and stopped).
// It invokes `docker ps -a --format '{{json .}}' --no-trunc` and parses
// each line as a separate JSON document.
func (s *Service) List(ctx context.Context) ([]Container, error) {
	stdout, stderr, err := safeexec.Run(ctx, "docker", "ps", "-a", "--format", "{{json .}}", "--no-trunc")
	if err != nil {
		if isDockerMissing(err) || strings.Contains(stderr, "Cannot connect to the Docker daemon") {
			return nil, ErrDockerUnavailable
		}
		return nil, fmt.Errorf("docker ps: %w (stderr=%q)", err, stderr)
	}
	return parseListJSON(strings.NewReader(stdout))
}

// Start starts the container with the given name. The name is validated
// against the Docker naming rules before being passed to exec.
func (s *Service) Start(ctx context.Context, name string) error {
	if err := safeexec.ValidateContainerName(name); err != nil {
		return err
	}
	_, stderr, err := safeexec.Run(ctx, "docker", "start", name)
	if err != nil {
		if isDockerMissing(err) {
			return ErrDockerUnavailable
		}
		return fmt.Errorf("docker start %s: %w (stderr=%q)", name, err, stderr)
	}
	return nil
}

// Stop stops the container with a graceful timeout (in seconds). A
// timeoutSec of 0 falls back to docker's default of 10s. Negative or
// excessive timeouts are rejected.
func (s *Service) Stop(ctx context.Context, name string, timeoutSec int) error {
	if err := safeexec.ValidateContainerName(name); err != nil {
		return err
	}
	t, err := normalizeStopTimeout(timeoutSec)
	if err != nil {
		return err
	}
	_, stderr, err := safeexec.Run(ctx, "docker", "stop", "-t", fmt.Sprintf("%d", t), name)
	if err != nil {
		if isDockerMissing(err) {
			return ErrDockerUnavailable
		}
		return fmt.Errorf("docker stop %s: %w (stderr=%q)", name, err, stderr)
	}
	return nil
}

// Restart restarts the container. Same timeout semantics as Stop.
func (s *Service) Restart(ctx context.Context, name string, timeoutSec int) error {
	if err := safeexec.ValidateContainerName(name); err != nil {
		return err
	}
	t, err := normalizeStopTimeout(timeoutSec)
	if err != nil {
		return err
	}
	_, stderr, err := safeexec.Run(ctx, "docker", "restart", "-t", fmt.Sprintf("%d", t), name)
	if err != nil {
		if isDockerMissing(err) {
			return ErrDockerUnavailable
		}
		return fmt.Errorf("docker restart %s: %w (stderr=%q)", name, err, stderr)
	}
	return nil
}

func normalizeStopTimeout(t int) (int, error) {
	switch {
	case t == 0:
		return 10, nil
	case t < 0:
		return 0, fmt.Errorf("invalid timeout: must be >= 0")
	case t > 3600:
		return 0, fmt.Errorf("invalid timeout: must be <= 3600")
	default:
		return t, nil
	}
}

// rawContainer mirrors the JSON shape emitted by `docker ps --format '{{json .}}'`.
// We accept a superset and ignore unknown fields.
type rawContainer struct {
	ID        string `json:"ID"`
	Names     string `json:"Names"`
	Image     string `json:"Image"`
	State     string `json:"State"`
	Status    string `json:"Status"`
	Ports     string `json:"Ports"`
	CreatedAt string `json:"CreatedAt"`
}

// parseListJSON consumes the line-delimited JSON output of `docker ps` and
// turns it into Container structs. Malformed lines are skipped so a single
// bad row does not collapse the entire response.
func parseListJSON(r io.Reader) ([]Container, error) {
	scanner := bufio.NewScanner(r)
	// Some container metadata can be longer than the default 64KB scanner buffer.
	const maxLine = 1024 * 1024
	scanner.Buffer(make([]byte, 0, 64*1024), maxLine)

	out := make([]Container, 0, 16)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var raw rawContainer
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			// Skip malformed line; do not fail the whole list.
			continue
		}
		c := Container{
			ID:     raw.ID,
			Name:   raw.Names,
			Image:  raw.Image,
			State:  raw.State,
			Status: raw.Status,
			Ports:  raw.Ports,
		}
		if len(raw.ID) >= 12 {
			c.ShortID = raw.ID[:12]
		} else {
			c.ShortID = raw.ID
		}
		if t, err := parseDockerTime(raw.CreatedAt); err == nil {
			c.CreatedAt = t
		}
		out = append(out, c)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("docker list scan: %w", err)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// parseDockerTime accepts the format Docker uses for CreatedAt in JSON:
// "2006-01-02 15:04:05 -0700 MST". Returns the zero value on failure.
func parseDockerTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	layouts := []string{
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05 -0700",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q", s)
}

func isDockerMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "no such file or directory")
}

// logsTailMax bounds the --tail value accepted from the API. Higher values
// are rejected at the handler layer; the package keeps the same ceiling.
const logsTailMax = 5000

// logsBudget is the maximum bytes returned per stream from Logs.
// If a stream exceeds this it is truncated to the trailing budget bytes.
const logsBudget = 1 * 1024 * 1024

// Logs returns the most recent container logs as separate stdout/stderr
// strings. tail must be 1..5000. since, when non-empty, must match the
// safeexec.ValidateDuration grammar (e.g. "10s", "5m", "1h"). When the
// combined output exceeds 1 MiB per stream, only the trailing 1 MiB of
// each stream is returned and truncated is set to true.
func (s *Service) Logs(ctx context.Context, name string, tail int, since string) (string, string, bool, error) {
	if err := safeexec.ValidateContainerName(name); err != nil {
		return "", "", false, err
	}
	if tail < 1 || tail > logsTailMax {
		return "", "", false, fmt.Errorf("invalid tail: must be 1..%d", logsTailMax)
	}
	if since != "" {
		if err := safeexec.ValidateDuration(since); err != nil {
			return "", "", false, err
		}
	}

	args := []string{"logs", "--tail", fmt.Sprintf("%d", tail)}
	if since != "" {
		args = append(args, "--since", since)
	}
	args = append(args, name)

	var outBuf, errBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", "", false, fmt.Errorf("docker logs %s: %w", name, safeexec.ErrTimeout)
		}
		if isDockerMissing(runErr) || strings.Contains(errBuf.String(), "Cannot connect to the Docker daemon") {
			return "", "", false, ErrDockerUnavailable
		}
		return "", "", false, fmt.Errorf("docker logs %s: %w (stderr=%q)", name, runErr, errBuf.String())
	}

	stdout, truncOut := truncateTail(outBuf.Bytes(), logsBudget)
	stderr, truncErr := truncateTail(errBuf.Bytes(), logsBudget)
	return stdout, stderr, truncOut || truncErr, nil
}

// truncateTail returns the trailing budget bytes of in. The truncated
// flag indicates whether trimming occurred.
func truncateTail(in []byte, budget int) (string, bool) {
	if len(in) <= budget {
		return string(in), false
	}
	return string(in[len(in)-budget:]), true
}

// StreamLogs starts `docker logs --tail N [-f] <name>` and returns the
// stdout/stderr pipes plus the underlying *exec.Cmd. The caller MUST
// invoke cmd.Wait() and close the pipes when done; killing the process
// (e.g. on client disconnect) is also the caller's responsibility.
//
// The returned pipes carry only what cloudflared/docker writes. The
// command is started with cmd.Start so the caller controls the
// lifecycle.
func (s *Service) StreamLogs(ctx context.Context, name string, tail int, follow bool) (io.ReadCloser, io.ReadCloser, *exec.Cmd, error) {
	if err := safeexec.ValidateContainerName(name); err != nil {
		return nil, nil, nil, err
	}
	if tail < 1 || tail > logsTailMax {
		return nil, nil, nil, fmt.Errorf("invalid tail: must be 1..%d", logsTailMax)
	}

	args := []string{"logs", "--tail", fmt.Sprintf("%d", tail)}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, name)

	cmd := exec.CommandContext(ctx, "docker", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("docker logs stream: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		return nil, nil, nil, fmt.Errorf("docker logs stream: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		if isDockerMissing(err) {
			return nil, nil, nil, ErrDockerUnavailable
		}
		return nil, nil, nil, fmt.Errorf("docker logs stream: start: %w", err)
	}
	return stdout, stderr, cmd, nil
}
