// Package deploy runs validated per-project deploy commands and persists
// each invocation to the deployments audit table.
//
// Security trade-offs (see also models/project_validate.go):
//   - Deploy commands are exec'd via `bash -c <cmd>` because real
//     deploy flows chain steps (git pull && pm2 restart). The validator
//     bans common shell-injection patterns and configuration is
//     admin-only.
//   - Commands run as the dashboard process user (NOT root) and inside
//     the project-scoped DeployWorkingDir.
//   - Every invocation is bounded by a hard timeout and lands in the
//     deployments table for auditability.
package deploy

import (
	"errors"
	"time"
)

// outputCap caps the per-stream stdout/stderr storage to keep audit
// rows from ballooning. The first N bytes are kept and a truncation
// marker is appended.
const outputCap = 64 * 1024

// Deployment is one row from the deployments audit table.
type Deployment struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	TriggeredBy string    `json:"triggered_by"`
	TriggeredAt time.Time `json:"triggered_at"`
	FinishedAt  time.Time `json:"finished_at"`
	Status      string    `json:"status"`
	ExitCode    int       `json:"exit_code"`
	Stdout      string    `json:"stdout"`
	Stderr      string    `json:"stderr"`
	RemoteRef   string    `json:"remote_ref"`
	Error       string    `json:"error"`
}

// Deployment statuses persisted in the deployments.status column.
const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusTimeout = "timeout"
)

// IsTerminal reports whether status indicates the deployment has
// finished (regardless of outcome).
func IsTerminal(status string) bool {
	switch status {
	case StatusSuccess, StatusFailed, StatusTimeout:
		return true
	}
	return false
}

// ErrAlreadyRunning is returned by Service.Trigger when another
// deployment for the same project is still running.
var ErrAlreadyRunning = errors.New("deploy: already running for this project")

// ErrNotConfigured is returned when a project has DeployEnabled=false
// or no DeployCommand.
var ErrNotConfigured = errors.New("deploy: not configured")

// ErrNotFound is returned when a deployment id has no row.
var ErrNotFound = errors.New("deploy: not found")
