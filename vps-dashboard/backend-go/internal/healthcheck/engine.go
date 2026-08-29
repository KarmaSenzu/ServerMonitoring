// Package healthcheck runs scheduled HTTP health probes against every
// project that has a non-empty health_url and an enabled flag. The
// engine persists each result, appends an Event when state transitions,
// and forwards the latest observation to the alert evaluator.
package healthcheck

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"

	"vps-dashboard-api/internal/alerts"
	"vps-dashboard-api/internal/models"
)

// defaultProbeTimeout caps a single HTTP round-trip.
const defaultProbeTimeout = 8 * time.Second

// concurrency caps how many projects are probed in parallel per tick.
const concurrency = 8

// envOverrideRefresh is how often the engine re-reads environment
// overrides. The cost is one tiny SELECT per refresh.
const envOverrideRefresh = 5 * time.Minute

// Engine ticks every Interval and probes every project with a health_url.
type Engine struct {
	Logger       zerolog.Logger
	Projects     *models.ProjectRepo
	Health       *models.HealthRepo
	Events       *models.EventRepo
	Alerts       *alerts.Evaluator
	EnvOverrides *models.EnvOverrideRepo
	Interval     time.Duration
	HTTP         *http.Client

	mu              sync.Mutex
	lastTick        map[string]time.Time
	envCache        map[string]float64
	envCacheLoadedAt time.Time
}

// NewEngine constructs an Engine with sane defaults.
func NewEngine(logger zerolog.Logger, projects *models.ProjectRepo, health *models.HealthRepo, events *models.EventRepo, ev *alerts.Evaluator, interval time.Duration) *Engine {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Engine{
		Logger:   logger,
		Projects: projects,
		Health:   health,
		Events:   events,
		Alerts:   ev,
		Interval: interval,
		HTTP: &http.Client{
			Timeout: defaultProbeTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
		lastTick: map[string]time.Time{},
		envCache: map[string]float64{},
	}
}

// Run blocks until ctx is cancelled, ticking once per Interval.
func (e *Engine) Run(ctx context.Context) {
	ticker := time.NewTicker(e.Interval)
	defer ticker.Stop()

	// First tick is delayed by Interval to let the server finish booting.
	for {
		select {
		case <-ctx.Done():
			e.Logger.Info().Msg("healthcheck.engine.stopped")
			return
		case <-ticker.C:
			e.runOnce(ctx)
		}
	}
}

// envMultiplier returns the per-environment healthcheck multiplier,
// caching the lookup for envOverrideRefresh between reloads. Missing
// rows fall back to the canonical defaults from models.
func (e *Engine) envMultiplier(ctx context.Context, env string) float64 {
	e.mu.Lock()
	stale := time.Since(e.envCacheLoadedAt) > envOverrideRefresh || e.envCache == nil
	e.mu.Unlock()

	if stale {
		e.refreshEnvCache(ctx)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if v, ok := e.envCache[env]; ok && v > 0 {
		return v
	}
	// Fall back to documented defaults.
	d := models.DefaultEnvOverrideFor(env).Config
	if v, ok := d["healthcheck_multiplier"].(float64); ok && v > 0 {
		return v
	}
	return 1.0
}

func (e *Engine) refreshEnvCache(ctx context.Context) {
	if e.EnvOverrides == nil {
		return
	}
	rows, err := e.EnvOverrides.List(ctx)
	if err != nil {
		e.Logger.Warn().Err(err).Msg("healthcheck.env_overrides.list_failed")
		return
	}
	cache := map[string]float64{}
	for _, r := range rows {
		if v, ok := r.Config["healthcheck_multiplier"].(float64); ok && v > 0 {
			cache[r.Environment] = v
		}
	}
	e.mu.Lock()
	e.envCache = cache
	e.envCacheLoadedAt = time.Now()
	e.mu.Unlock()
}

// shouldProbe returns true when the per-project effective interval has
// elapsed since the last probe. The lastTick map is updated to the
// current instant when true is returned so concurrent ticks do not
// double-probe.
func (e *Engine) shouldProbe(projectID string, effective time.Duration, now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	last, ok := e.lastTick[projectID]
	if ok && now.Sub(last) < effective {
		return false
	}
	e.lastTick[projectID] = now
	return true
}

// runOnce probes every eligible project once. Errors in any single
// probe are logged but never abort the rest of the batch.
func (e *Engine) runOnce(ctx context.Context) {
	projects, err := e.Projects.List(ctx, models.ProjectFilter{EnabledOnly: true})
	if err != nil {
		e.Logger.Warn().Err(err).Msg("healthcheck.list_projects_failed")
		return
	}
	if len(projects) == 0 {
		return
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	now := time.Now()
	for _, p := range projects {
		p := p
		if strings.TrimSpace(p.HealthURL) == "" {
			continue
		}
		if !validHealthURL(p.HealthURL) {
			e.Logger.Warn().
				Str("project_id", p.ID).
				Str("health_url", p.HealthURL).
				Msg("healthcheck.invalid_url_skipped")
			continue
		}

		multiplier := e.envMultiplier(ctx, p.Environment)
		effective := time.Duration(float64(e.Interval) * multiplier)
		if effective < e.Interval {
			effective = e.Interval
		}
		if !e.shouldProbe(p.ID, effective, now) {
			continue
		}

		g.Go(func() error {
			e.probeAndPersist(gctx, p)
			return nil
		})
	}
	_ = g.Wait()
}

// probeAndPersist performs a single probe, persists the result, and
// emits state-change events when the OK flag flips.
func (e *Engine) probeAndPersist(ctx context.Context, p models.Project) {
	previous, prevErr := e.Health.LatestByProject(ctx, p.ID)

	res := e.probe(ctx, p)
	if err := e.Health.Append(ctx, res); err != nil {
		e.Logger.Warn().Err(err).Str("project_id", p.ID).Msg("healthcheck.append_failed")
	}

	state := "up"
	if !res.OK {
		state = "down"
	}

	transition := false
	if prevErr == nil {
		if previous.OK != res.OK {
			transition = true
		}
	} else {
		// First observation. Only log a transition event when the first
		// observation is "down" so we do not flood events on boot.
		if !res.OK {
			transition = true
		}
	}

	if transition {
		severity := models.SeverityInfo
		message := fmt.Sprintf("%s recovered (HTTP %d)", p.Name, res.StatusCode)
		if !res.OK {
			severity = models.SeverityWarning
			message = fmt.Sprintf("%s went DOWN (HTTP %d)", p.Name, res.StatusCode)
			if res.Error != "" {
				message = fmt.Sprintf("%s went DOWN: %s", p.Name, res.Error)
			}
		}
		if _, err := e.Events.Append(ctx, models.Event{
			Category:  models.EventCategoryHealth,
			Severity:  severity,
			Source:    "project:" + p.ID,
			ProjectID: p.ID,
			Message:   message,
			Data: map[string]any{
				"status_code": res.StatusCode,
				"latency_ms":  res.LatencyMs,
				"ok":          res.OK,
				"health_url":  p.HealthURL,
			},
			Timestamp: res.Timestamp,
		}); err != nil {
			e.Logger.Warn().Err(err).Str("project_id", p.ID).Msg("healthcheck.event_append_failed")
		}
	}

	// Forward a project_health signal to the alert evaluator on every
	// observation; the evaluator decides whether to fire.
	if e.Alerts != nil {
		e.Alerts.Evaluate(ctx, alerts.Signal{
			Type:      models.AlertTypeProjectHealth,
			Value:     float64(res.LatencyMs),
			State:     state,
			ProjectID: p.ID,
			Timestamp: res.Timestamp,
		})
	}
}

// probe performs a single GET against p.HealthURL, returning a
// ready-to-persist HealthResult.
func (e *Engine) probe(ctx context.Context, p models.Project) models.HealthResult {
	out := models.HealthResult{
		ProjectID: p.ID,
		Timestamp: time.Now().UTC(),
		LatencyMs: -1,
	}

	probeCtx, cancel := context.WithTimeout(ctx, defaultProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, p.HealthURL, nil)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	start := time.Now()
	resp, err := e.HTTP.Do(req)
	out.LatencyMs = int(time.Since(start).Milliseconds())
	if err != nil {
		out.Error = err.Error()
		e.Logger.Warn().Err(err).Str("project", p.ID).Msg("healthcheck.health_check_failed")
		return out
	}
	defer func() { _ = resp.Body.Close() }()
	out.StatusCode = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		out.OK = true
	}
	return out
}

func validHealthURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
