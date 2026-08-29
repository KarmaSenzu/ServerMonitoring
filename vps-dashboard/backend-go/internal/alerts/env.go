package alerts

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/models"
)

// envOverrideRefresh mirrors the cadence used by the healthcheck engine.
const envOverrideRefresh = 5 * time.Minute

// EnvFloor caches per-environment severity floors. The evaluator
// consults the cache when a signal carries a project_id; rules with a
// severity below the project's environment floor are suppressed.
type EnvFloor struct {
	Logger zerolog.Logger

	Projects     *models.ProjectRepo
	EnvOverrides *models.EnvOverrideRepo

	mu              sync.Mutex
	envFloors       map[string]string // env -> severity floor
	projectEnvCache map[string]string // project_id -> environment
	loadedAt        time.Time
}

// NewEnvFloor builds an EnvFloor wired to the given repos. Either repo
// may be nil; the floor will fall back to "info" (no suppression).
func NewEnvFloor(logger zerolog.Logger, projects *models.ProjectRepo, env *models.EnvOverrideRepo) *EnvFloor {
	return &EnvFloor{
		Logger:          logger,
		Projects:        projects,
		EnvOverrides:    env,
		envFloors:       map[string]string{},
		projectEnvCache: map[string]string{},
	}
}

// Allow reports whether a rule of severity should be allowed to fire
// for the given project. When projectID is empty the floor cannot be
// applied (system-wide rule) and Allow returns true.
func (f *EnvFloor) Allow(ctx context.Context, projectID, severity string) bool {
	if f == nil || projectID == "" {
		return true
	}
	env := f.lookupEnv(ctx, projectID)
	if env == "" {
		return true
	}
	floor := f.lookupFloor(ctx, env)
	if floor == "" {
		return true
	}
	return severityAtLeast(severity, floor)
}

func (f *EnvFloor) lookupEnv(ctx context.Context, projectID string) string {
	f.mu.Lock()
	if v, ok := f.projectEnvCache[projectID]; ok {
		f.mu.Unlock()
		return v
	}
	f.mu.Unlock()

	if f.Projects == nil {
		return ""
	}
	project, err := f.Projects.Get(ctx, projectID)
	if err != nil {
		return ""
	}

	f.mu.Lock()
	f.projectEnvCache[projectID] = project.Environment
	f.mu.Unlock()
	return project.Environment
}

func (f *EnvFloor) lookupFloor(ctx context.Context, env string) string {
	f.mu.Lock()
	stale := time.Since(f.loadedAt) > envOverrideRefresh || len(f.envFloors) == 0
	f.mu.Unlock()
	if stale {
		f.refresh(ctx)
	}
	f.mu.Lock()
	v := f.envFloors[env]
	f.mu.Unlock()
	if v != "" {
		return v
	}
	d := models.DefaultEnvOverrideFor(env).Config
	if s, ok := d["alert_severity_floor"].(string); ok {
		return s
	}
	return ""
}

func (f *EnvFloor) refresh(ctx context.Context) {
	if f.EnvOverrides == nil {
		return
	}
	rows, err := f.EnvOverrides.List(ctx)
	if err != nil {
		f.Logger.Warn().Err(err).Msg("alerts.env_floor.list_failed")
		return
	}
	floors := map[string]string{}
	for _, r := range rows {
		if s, ok := r.Config["alert_severity_floor"].(string); ok && s != "" {
			floors[r.Environment] = s
		}
	}
	f.mu.Lock()
	f.envFloors = floors
	f.loadedAt = time.Now()
	f.mu.Unlock()
}

// severityRank orders severities from lowest (info) to highest
// (critical). Unknown values rank at the bottom so they never block
// known severities.
func severityRank(s string) int {
	switch s {
	case models.SeverityInfo:
		return 1
	case models.SeverityWarning:
		return 2
	case models.SeverityError:
		return 3
	case models.SeverityCritical:
		return 4
	}
	return 0
}

// severityAtLeast returns true when have is at least as severe as floor.
func severityAtLeast(have, floor string) bool {
	return severityRank(have) >= severityRank(floor)
}
