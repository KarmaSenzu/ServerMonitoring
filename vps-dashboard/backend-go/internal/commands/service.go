// Package commands implements the multi-host command engine
// (PROJECT ARCHITECTURE.md §17-§20, Phase 6): reusable snippets,
// blast-radius preview, bounded parallel execution with per-host
// independence, and audit recording.
package commands

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/ssh"
)

// MaxParallel caps concurrent SSH command executions (§18 §46).
const MaxParallel = 4

// HostResult captures the outcome of executing a command on one server.
type HostResult struct {
	ServerID   string `json:"server_id"`
	ServerName string `json:"server_name"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	Status     string `json:"status"` // success|failed|timeout|error
	Error      string `json:"error,omitempty"`
	DurationMs int64  `json:"duration_ms"`
}

// ExecuteResult is the aggregate response of a multi-host execution.
type ExecuteResult struct {
	Command   string       `json:"command"`
	Danger    string       `json:"danger_level"`
	Total     int          `json:"total_hosts"`
	Success   int          `json:"success_count"`
	Failed    int          `json:"failed_count"`
	Results   []HostResult `json:"results"`
}

// BlastPreview is the preview shown before confirmation (§19).
type BlastPreview struct {
	Command     string   `json:"command"`
	DangerLevel string   `json:"danger_level"`
	Targets     []string `json:"targets"`
	TargetCount int      `json:"target_count"`
	Warning     string   `json:"warning,omitempty"`
}

// Service orchestrates multi-host command execution.
type Service struct {
	Logger  zerolog.Logger
	SSH     *ssh.Service
	Runs    *models.CommandRunRepo
}

// NewService constructs a command Service.
func NewService(logger zerolog.Logger, engine *ssh.Service, runs *models.CommandRunRepo) *Service {
	return &Service{Logger: logger, SSH: engine, Runs: runs}
}

// Preview builds a blast-radius preview for a pending execution. The
// caller passes the command and the list of target servers; the
// preview classifies the danger level and surfaces a warning for
// high-risk patterns (§19).
func (s *Service) Preview(command string, servers []models.Server) BlastPreview {
	danger := ClassifyDanger(command)
	preview := BlastPreview{
		Command:     command,
		DangerLevel: danger,
		TargetCount: len(servers),
	}
	for _, srv := range servers {
		preview.Targets = append(preview.Targets, srv.Name)
	}
	if danger == models.DangerDangerous {
		preview.Warning = fmt.Sprintf("This command is classified as DANGEROUS and will affect %d hosts. Confirm carefully.", len(servers))
	}
	return preview
}

// Execute runs a command on multiple servers with bounded concurrency
// (§18: "A failure on one server must not automatically terminate
// execution on other servers"). Results are collected per-host and
// persisted as audit records.
func (s *Service) Execute(ctx context.Context, command string, servers []models.Server, userID, snippetID string) ExecuteResult {
	sem := make(chan struct{}, MaxParallel)
	var wg sync.WaitGroup

	results := make([]HostResult, len(servers))
	successCount := 0
	failedCount := 0

	for i, srv := range servers {
		wg.Add(1)
		go func(idx int, server models.Server) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results[idx] = s.executeOnHost(ctx, command, server, userID, snippetID)
			if results[idx].Status == "success" {
				successCount++
			} else {
				failedCount++
			}
		}(i, srv)
	}

	wg.Wait()

	return ExecuteResult{
		Command: command,
		Danger:  ClassifyDanger(command),
		Total:   len(servers),
		Success: successCount,
		Failed:  failedCount,
		Results: results,
	}
}

// executeOnHost runs a single command on one server and records the
// audit trail. Returns a HostResult with the outcome.
func (s *Service) executeOnHost(ctx context.Context, command string, server models.Server, userID, snippetID string) HostResult {
	result, err := s.SSH.RunCommand(ctx, server, command)

	hostResult := HostResult{
		ServerID:   server.ID,
		ServerName: server.Name,
		DurationMs: result.DurationMs,
	}

	status := "success"
	exitCode := result.ExitCode
	stdout := result.Stdout
	stderr := result.Stderr
	errMsg := ""

	if err != nil {
		errMsg = err.Error()
		switch {
		case isTimeout(err):
			status = "timeout"
		case isTransport(err):
			status = "error"
		default:
			status = "failed"
		}
	} else if exitCode != 0 {
		status = "failed"
	}

	hostResult.Status = status
	hostResult.ExitCode = exitCode
	hostResult.Stdout = stdout
	hostResult.Stderr = stderr
	hostResult.Error = errMsg

	// Persist audit record (best-effort).
	if s.Runs != nil {
		runCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		auditStatus := status
		_, _ = s.Runs.Append(runCtx, models.CommandRun{
			SnippetID:  snippetID,
			ServerID:    server.ID,
			ServerName:  server.Name,
			UserID:      userID,
			Command:     command,
			ExitCode:    exitCode,
			Stdout:      truncateOutput(stdout, 64*1024),
			Stderr:      truncateOutput(stderr, 64*1024),
			Status:      auditStatus,
			DurationMs:  result.DurationMs,
		})
		cancel()
	}

	return hostResult
}

// ClassifyDanger inspects a command string and returns its danger
// level (§19 Blast Radius Protection). Dangerous patterns trigger
// explicit confirmation in the UI.
func ClassifyDanger(command string) string {
	lower := strings.ToLower(strings.TrimSpace(command))
	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(lower) {
			return models.DangerDangerous
		}
	}
	for _, pattern := range cautionPatterns {
		if pattern.MatchString(lower) {
			return models.DangerCaution
		}
	}
	return models.DangerSafe
}

// dangerousPatterns are commands that can cause irreversible damage.
var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+(-\w*r\w*\s+)?/`),          // rm -rf /
	regexp.MustCompile(`\bshutdown\b`),                      // shutdown
	regexp.MustCompile(`\breboot\b`),                        // reboot
	regexp.MustCompile(`\bsystemctl\s+(stop|disable)\b`),    // systemctl stop
	regexp.MustCompile(`\bdocker\s+rm\b`),                   // docker rm
	regexp.MustCompile(`\bdocker\s+system\s+prune\b`),       // docker system prune
	regexp.MustCompile(`\bmkfs\b`),                           // mkfs
	regexp.MustCompile(`\bdd\s+.*of=/dev/`),                 // dd to device
	regexp.MustCompile(`\b:\(\)\{\s*:\|\:&\s*\};:`),         // fork bomb
	regexp.MustCompile(`\bchmod\s+-R\s+0\b`),               // chmod -R 0
}

// cautionPatterns are commands that modify state but are reversible.
var cautionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bdocker\s+(stop|restart)\b`),
	regexp.MustCompile(`\bkill\b`),
	regexp.MustCompile(`\bpkill\b`),
	regexp.MustCompile(`\biptables\b`),
	regexp.MustCompile(`\bmount\b`),
	regexp.MustCompile(`\bumount\b`),
}

func isTimeout(err error) bool {
	return err != nil && strings.Contains(err.Error(), "timed out")
}

func isTransport(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "host unreachable") ||
		strings.Contains(msg, "authentication failed") ||
		strings.Contains(msg, "host key changed") ||
		strings.Contains(msg, "credential not configured")
}

func truncateOutput(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...[truncated]"
}
