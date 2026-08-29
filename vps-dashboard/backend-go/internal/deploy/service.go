package deploy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/models"
)

// Service runs deploy commands and persists each invocation. It is
// safe for concurrent use across projects but only one deployment may
// run per project at any time.
type Service struct {
	Logger   zerolog.Logger
	Projects *models.ProjectRepo
	Repo     *DeploymentRepo
	Events   *models.EventRepo

	mu     sync.Mutex
	locks  map[string]*projectLock
	doneCh map[string]chan struct{}
}

type projectLock struct {
	mu sync.Mutex
}

// NewService constructs a Service.
func NewService(logger zerolog.Logger, projects *models.ProjectRepo, repo *DeploymentRepo, events *models.EventRepo) *Service {
	return &Service{
		Logger:   logger,
		Projects: projects,
		Repo:     repo,
		Events:   events,
		locks:    map[string]*projectLock{},
		doneCh:   map[string]chan struct{}{},
	}
}

func (s *Service) lockFor(projectID string) *projectLock {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.locks[projectID]; ok {
		return l
	}
	l := &projectLock{}
	s.locks[projectID] = l
	return l
}

// tryAcquire returns ok=true if the per-project lock was successfully
// taken. The caller must call release when the deployment finishes.
func (s *Service) tryAcquire(projectID string) bool {
	l := s.lockFor(projectID)
	return l.mu.TryLock()
}

func (s *Service) release(projectID string) {
	s.mu.Lock()
	l := s.locks[projectID]
	ch := s.doneCh[projectID]
	delete(s.doneCh, projectID)
	s.mu.Unlock()
	if l != nil {
		l.mu.Unlock()
	}
	if ch != nil {
		close(ch)
	}
}

// Trigger validates a project's deploy configuration, persists a
// pending deployment row and runs the command in the background. The
// returned Deployment captures the row at insertion time; subsequent
// state lives in the database. Use WaitFor to block until terminal.
func (s *Service) Trigger(ctx context.Context, project models.Project, triggeredBy, remoteRef string) (Deployment, error) {
	if !project.DeployEnabled || project.DeployCommand == "" {
		return Deployment{}, ErrNotConfigured
	}
	if !s.tryAcquire(project.ID) {
		return Deployment{}, ErrAlreadyRunning
	}

	d := Deployment{
		ID:          uuid.NewString(),
		ProjectID:   project.ID,
		TriggeredBy: triggeredBy,
		TriggeredAt: time.Now().UTC(),
		Status:      StatusPending,
		ExitCode:    -1,
		RemoteRef:   remoteRef,
	}

	created, err := s.Repo.Create(ctx, d)
	if err != nil {
		s.release(project.ID)
		return Deployment{}, err
	}

	// Register a doneCh BEFORE returning so WaitFor sees the channel.
	s.mu.Lock()
	doneCh := make(chan struct{})
	s.doneCh[project.ID] = doneCh
	s.mu.Unlock()

	if s.Events != nil {
		_, _ = s.Events.Append(ctx, models.Event{
			Category:  "deploy",
			Severity:  models.SeverityInfo,
			Source:    "deploy:" + project.ID,
			ProjectID: project.ID,
			Message:   fmt.Sprintf("Deploy started for %s (%s)", project.Name, triggeredBy),
			Data: map[string]any{
				"deployment_id": created.ID,
				"triggered_by":  triggeredBy,
				"remote_ref":    remoteRef,
			},
		})
	}

	go s.run(project, created)
	return created, nil
}

// run executes the deploy command. It is invoked in its own goroutine
// from Trigger and never returns to the caller.
func (s *Service) run(project models.Project, d Deployment) {
	defer s.release(project.ID)

	// Build a fresh context decoupled from the HTTP request that
	// triggered the deploy: a webhook caller closing the connection
	// must not abort an in-flight deploy.
	timeout := time.Duration(project.DeployTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := s.Repo.MarkStatus(ctx, d.ID, StatusRunning); err != nil {
		s.Logger.Warn().Err(err).Str("deployment_id", d.ID).Msg("deploy.mark_running_failed")
	}

	// Deploy commands run via `bash -c` because real-world flows chain
	// steps. Validation in models/project_validate.go ensures the
	// command does not contain dangerous metacharacters.
	cmd := exec.CommandContext(ctx, "bash", "-c", project.DeployCommand)
	if project.DeployWorkingDir != "" {
		cmd.Dir = project.DeployWorkingDir
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		s.finalize(ctx, d.ID, StatusFailed, -1, fmt.Sprintf("stdout pipe: %v", err))
		s.fireEvent(ctx, project, d.ID, StatusFailed, "stdout pipe failed")
		return
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		s.finalize(ctx, d.ID, StatusFailed, -1, fmt.Sprintf("stderr pipe: %v", err))
		s.fireEvent(ctx, project, d.ID, StatusFailed, "stderr pipe failed")
		return
	}

	if err := cmd.Start(); err != nil {
		s.finalize(ctx, d.ID, StatusFailed, -1, fmt.Sprintf("start: %v", err))
		s.fireEvent(ctx, project, d.ID, StatusFailed, "start failed")
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s.streamInto(ctx, stdoutPipe, d.ID, true)
	}()
	go func() {
		defer wg.Done()
		s.streamInto(ctx, stderrPipe, d.ID, false)
	}()
	wg.Wait()

	runErr := cmd.Wait()
	exitCode := 0
	status := StatusSuccess
	errMsg := ""

	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
		status = StatusFailed
		errMsg = runErr.Error()
		if ctx.Err() == context.DeadlineExceeded {
			status = StatusTimeout
			errMsg = "deploy timeout"
		}
	}

	if err := s.finalize(context.Background(), d.ID, status, exitCode, errMsg); err != nil {
		s.Logger.Warn().Err(err).Str("deployment_id", d.ID).Msg("deploy.finalize_failed")
	}
	s.fireEvent(context.Background(), project, d.ID, status, errMsg)
}

func (s *Service) streamInto(ctx context.Context, r io.ReadCloser, deploymentID string, stdout bool) {
	defer func() { _ = r.Close() }()
	scanner := bufio.NewScanner(r)
	// Allow long lines (default 64KiB token cap is fine; do not raise).
	for scanner.Scan() {
		line := scanner.Text() + "\n"
		var err error
		if stdout {
			err = s.Repo.AppendStdout(ctx, deploymentID, line)
		} else {
			err = s.Repo.AppendStderr(ctx, deploymentID, line)
		}
		if err != nil {
			s.Logger.Warn().Err(err).Str("deployment_id", deploymentID).Msg("deploy.stream_append_failed")
			return
		}
	}
	if err := scanner.Err(); err != nil {
		s.Logger.Warn().Err(err).Str("deployment_id", deploymentID).Msg("deploy.stream_scan_failed")
	}
}

func (s *Service) finalize(ctx context.Context, id, status string, exitCode int, errMsg string) error {
	return s.Repo.Finish(ctx, id, status, exitCode, errMsg)
}

func (s *Service) fireEvent(ctx context.Context, project models.Project, deploymentID, status, detail string) {
	if s.Events == nil {
		return
	}
	severity := models.SeverityInfo
	switch status {
	case StatusFailed, StatusTimeout:
		severity = models.SeverityError
	}
	msg := fmt.Sprintf("Deploy %s for %s", status, project.Name)
	if detail != "" {
		msg = fmt.Sprintf("%s: %s", msg, detail)
	}
	if _, err := s.Events.Append(ctx, models.Event{
		Category:  "deploy",
		Severity:  severity,
		Source:    "deploy:" + project.ID,
		ProjectID: project.ID,
		Message:   msg,
		Data: map[string]any{
			"deployment_id": deploymentID,
			"status":        status,
		},
	}); err != nil {
		s.Logger.Warn().Err(err).Str("deployment_id", deploymentID).Msg("deploy.event_append_failed")
	}
}

// WaitFor blocks until the deployment with id reaches a terminal
// status or timeout elapses. The polling interval is short so manual
// triggers feel responsive while not stressing SQLite.
func (s *Service) WaitFor(ctx context.Context, id string, timeout time.Duration) (Deployment, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		d, err := s.Repo.Get(ctx, id)
		if err != nil {
			return Deployment{}, err
		}
		if IsTerminal(d.Status) {
			return d, nil
		}
		if time.Now().After(deadline) {
			return d, fmt.Errorf("deploy: wait timeout after %s", timeout)
		}
		select {
		case <-ctx.Done():
			return d, ctx.Err()
		case <-ticker.C:
		}
	}
}
