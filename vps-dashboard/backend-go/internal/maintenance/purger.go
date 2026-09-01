// Package maintenance houses background housekeeping workers.
package maintenance

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/models"
)

// Default retention windows applied when zero values are supplied.
const (
	defaultKeepEventsFor  = 30 * 24 * time.Hour // 30 days
	defaultKeepHealthFor  = 14 * 24 * time.Hour // 14 days
	defaultKeepMetricsFor = 24 * time.Hour      // 24 hours
	purgeInterval         = time.Hour
)

// Purger periodically deletes old rows from the events and
// health_results tables. It is launched once from main as a goroutine
// and stops when ctx is cancelled.
type Purger struct {
	Logger        zerolog.Logger
	Events        *models.EventRepo
	Health        *models.HealthRepo
	Metrics       *models.ServerMetricRepo
	KeepEventsFor time.Duration
	KeepHealthFor time.Duration
	KeepMetricsFor time.Duration
}

// Run blocks until ctx is cancelled, ticking once an hour. The first
// purge is intentionally deferred by purgeInterval so server boot is
// not delayed.
func (p *Purger) Run(ctx context.Context) {
	keepEvents := p.KeepEventsFor
	if keepEvents <= 0 {
		keepEvents = defaultKeepEventsFor
	}
	keepHealth := p.KeepHealthFor
	if keepHealth <= 0 {
		keepHealth = defaultKeepHealthFor
	}
	keepMetrics := p.KeepMetricsFor
	if keepMetrics <= 0 {
		keepMetrics = defaultKeepMetricsFor
	}

	ticker := time.NewTicker(purgeInterval)
	defer ticker.Stop()

	p.Logger.Info().
		Dur("keep_events_for", keepEvents).
		Dur("keep_health_for", keepHealth).
		Dur("keep_metrics_for", keepMetrics).
		Dur("interval", purgeInterval).
		Msg("maintenance.purger.started")

	for {
		select {
		case <-ctx.Done():
			p.Logger.Info().Msg("maintenance.purger.stopped")
			return
		case <-ticker.C:
			p.tick(ctx, keepEvents, keepHealth, keepMetrics)
		}
	}
}

// tick runs one purge cycle. Errors are logged but never propagated.
func (p *Purger) tick(ctx context.Context, keepEvents, keepHealth, keepMetrics time.Duration) {
	now := time.Now().UTC()

	if p.Events != nil {
		cutoff := now.Add(-keepEvents)
		n, err := p.Events.Purge(ctx, cutoff)
		if err != nil {
			p.Logger.Warn().Err(err).Msg("maintenance.purger.events_failed")
		} else {
			p.Logger.Info().
				Int("deleted", n).
				Time("cutoff", cutoff).
				Msg("maintenance.purger.events")
		}
	}

	if p.Health != nil {
		cutoff := now.Add(-keepHealth)
		n, err := p.Health.Purge(ctx, cutoff)
		if err != nil {
			p.Logger.Warn().Err(err).Msg("maintenance.purger.health_failed")
		} else {
			p.Logger.Info().
				Int("deleted", n).
				Time("cutoff", cutoff).
				Msg("maintenance.purger.health")
		}
	}

	if p.Metrics != nil {
		cutoff := now.Add(-keepMetrics)
		n, err := p.Metrics.Purge(ctx, cutoff)
		if err != nil {
			p.Logger.Warn().Err(err).Msg("maintenance.purger.metrics_failed")
		} else if n > 0 {
			p.Logger.Info().
				Int("deleted", n).
				Time("cutoff", cutoff).
				Msg("maintenance.purger.metrics")
		}
	}
}
