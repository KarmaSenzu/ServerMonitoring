package remote

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/models"
)

// EngineConfig tunes the remote monitoring loop.
type EngineConfig struct {
	// Interval between full sweeps of the server registry.
	Interval time.Duration

	// MaxParallel caps the number of concurrent SSH metric collections.
	// PROJECT ARCHITECTURE.md §46: "Never create unbounded concurrency".
	MaxParallel int

	// CommandTimeout bounds each SSH metrics command (applied via the
	// SSH engine's context).
	CommandTimeout time.Duration

	// Retention controls how old metrics are purged.
	Retention time.Duration
}

// Engine polls registered servers on a fixed cadence, collects metrics
// via the SSH collector, persists them, and updates server status.
type Engine struct {
	Logger  zerolog.Logger
	Servers *models.ServerRepo
	Metrics *models.ServerMetricRepo
	Events  *models.EventRepo

	Collector *Collector

	cfg     EngineConfig

	// Anti-flapping: track consecutive failures per server.
	// Only mark offline after multiple consecutive failures.
	mu        sync.Mutex
	failCount map[string]int // server ID → consecutive failure count
}

// NewEngine constructs a remote monitoring engine.
func NewEngine(
	logger zerolog.Logger,
	servers *models.ServerRepo,
	metrics *models.ServerMetricRepo,
	events *models.EventRepo,
	collector *Collector,
	cfg EngineConfig,
) *Engine {
	if cfg.Interval <= 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.MaxParallel <= 0 {
		cfg.MaxParallel = 4
	}
	if cfg.CommandTimeout <= 0 {
		cfg.CommandTimeout = 15 * time.Second
	}
	if cfg.Retention <= 0 {
		cfg.Retention = 24 * time.Hour
	}
	return &Engine{
		Logger:    logger,
		Servers:   servers,
		Metrics:   metrics,
		Events:    events,
		Collector: collector,
		cfg:       cfg,
		failCount: make(map[string]int),
	}
}

// Run starts the polling loop and blocks until ctx is cancelled. It
// performs an immediate sweep on start so metrics are available right
// away, then ticks on cfg.Interval.
func (e *Engine) Run(ctx context.Context) {
	e.Logger.Info().
		Dur("interval", e.cfg.Interval).
		Int("max_parallel", e.cfg.MaxParallel).
		Dur("retention", e.cfg.Retention).
		Msg("remote.monitoring.started")

	// Immediate first sweep.
	e.sweep(ctx)

	ticker := time.NewTicker(e.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			e.Logger.Info().Msg("remote.monitoring.stopped")
			return
		case <-ticker.C:
			e.sweep(ctx)
		}
	}
}

// sweep collects metrics from every enabled server in the registry
// using a bounded worker pool.
func (e *Engine) sweep(ctx context.Context) {
	servers, err := e.Servers.List(ctx, models.ServerFilter{EnabledOnly: true})
	if err != nil {
		e.Logger.Error().Err(err).Msg("remote.monitoring.list_failed")
		return
	}
	if len(servers) == 0 {
		return
	}

	e.Logger.Debug().Int("servers", len(servers)).Msg("remote.monitoring.sweep_start")

	// Bounded concurrency: a semaphore channel of size MaxParallel.
	sem := make(chan struct{}, e.cfg.MaxParallel)
	var wg sync.WaitGroup

	for _, srv := range servers {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		default:
		}

		wg.Add(1)
		go func(s models.Server) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			e.collectOne(ctx, s)
		}(srv)
	}

	wg.Wait()
	e.Logger.Debug().Int("servers", len(servers)).Msg("remote.monitoring.sweep_done")
}

// collectOne gathers metrics for a single server, persists them, and
// updates the server's status row. On successful SSH connection, it
// also auto-populates system metadata (OS, architecture) that was
// detected from the remote host — so users don't have to fill these
// manually when registering a server.
//
// STATUS LOGIC (anti-flapping):
// - A single failed poll does NOT immediately mark a server offline.
// - We track consecutive failures per server and only mark offline
//   after maxFailures (default 3) consecutive failures.
// - A single successful poll immediately marks online.
// - This prevents transient network blips from causing flapping.
func (e *Engine) collectOne(ctx context.Context, server models.Server) {
	collectCtx, cancel := context.WithTimeout(ctx, e.cfg.CommandTimeout)
	defer cancel()

	metric := e.Collector.Collect(collectCtx, server)

	// Persist the metric (even on failure — the Error field records
	// what happened so the history endpoint can surface it).
	if _, err := e.Metrics.Append(ctx, metric); err != nil {
		e.Logger.Warn().Err(err).Str("server", server.Name).Msg("remote.monitoring.persist_failed")
	}

	// Auto-populate system metadata on successful SSH connection.
	if metric.Error == "" && (server.OperatingSystem == "" || server.Architecture == "") {
		sysInfo := ParseSystemInfo(metric.RawStdout)
		if sysInfo.OperatingSystem != "" || sysInfo.Architecture != "" {
			if err := e.Servers.UpdateSystemInfo(ctx, server.ID, sysInfo.OperatingSystem, sysInfo.Architecture); err != nil {
				e.Logger.Warn().Err(err).Str("server", server.Name).Msg("remote.monitoring.sysinfo_update_failed")
			}
		}
	}

	// Determine new status using anti-flapping logic.
	// Key insight: don't mark offline on first failure. Only after
	// consecutive failures do we consider the server truly offline.
	maxFailures := 3
	newStatus := models.ServerStatusOnline
	detail := ""

	if metric.Error != "" {
		// This poll failed. Increment consecutive failure count.
		e.mu.Lock()
		e.failCount[server.ID]++
		failures := e.failCount[server.ID]
		e.mu.Unlock()

		if failures >= maxFailures {
			// Only mark offline after multiple consecutive failures
			newStatus = models.ServerStatusOffline
			detail = metric.Error
			if len(detail) > 256 {
				detail = detail[:256]
			}
		} else {
			// Keep current status (don't flap). If was online, stay online
			// but note the transient failure in detail.
			newStatus = server.Status
			if server.Status == models.ServerStatusOnline {
				newStatus = models.ServerStatusDegraded
			}
			detail = fmt.Sprintf("transient failure (%d/%d): %s", failures, maxFailures, metric.Error)
			if len(detail) > 256 {
				detail = detail[:256]
			}
		}
	} else {
		// Success! Reset failure count.
		e.mu.Lock()
		e.failCount[server.ID] = 0
		e.mu.Unlock()
	}

	// Update server status.
	if err := e.Servers.SetStatus(ctx, server.ID, newStatus, detail, metric.Timestamp); err != nil {
		e.Logger.Warn().Err(err).Str("server", server.Name).Msg("remote.monitoring.status_update_failed")
	}

	// Emit events on status transitions.
	if newStatus == models.ServerStatusOffline && server.Status != models.ServerStatusOffline {
		e.emitEvent(ctx, server, models.SeverityWarning, "server_offline",
			"Server "+server.Name+" is unreachable after "+fmt.Sprintf("%d", maxFailures)+" consecutive failures: "+detail)
	} else if newStatus == models.ServerStatusOnline && server.Status != models.ServerStatusOnline {
		e.emitEvent(ctx, server, models.SeverityInfo, "server_recovered",
			"Server "+server.Name+" is back online")
	}
}

// emitEvent records an infrastructure event. Failures are logged but
// never block the monitoring loop.
func (e *Engine) emitEvent(ctx context.Context, server models.Server, severity, action, message string) {
	if e.Events == nil {
		return
	}
	evCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = e.Events.Append(evCtx, models.Event{
		Category: models.EventCategorySystem,
		Severity: severity,
		Source:   "remote-monitoring:" + server.Name,
		Message:  message,
		Data: map[string]any{
			"action":      action,
			"server_id":   server.ID,
			"server_name": server.Name,
		},
	})
}

// Purge removes metrics older than the configured retention. Call
// periodically (e.g. from the maintenance purger).
func (e *Engine) Purge(ctx context.Context) (int, error) {
	olderThan := time.Now().UTC().Add(-e.cfg.Retention)
	n, err := e.Metrics.Purge(ctx, olderThan)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		e.Logger.Info().Int("purged", n).Msg("remote.monitoring.purged")
	}
	return n, nil
}
