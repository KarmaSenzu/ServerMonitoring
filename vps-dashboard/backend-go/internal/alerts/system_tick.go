package alerts

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/sysinfo"
)

// SystemTickConfig configures the cadence and source of system signals.
type SystemTickConfig struct {
	Interval time.Duration
	// MaxSampleAge bounds how stale a recorder sample may be before we
	// stop forwarding it. Defaults to 2 * Interval.
	MaxSampleAge time.Duration
}

// RunSystemTick polls the latest sample from the recorder on Interval
// and emits CPU/memory/disk signals to the evaluator. The function
// returns when ctx is cancelled.
func RunSystemTick(ctx context.Context, logger zerolog.Logger, recorder *sysinfo.Recorder, evaluator *Evaluator, cfg SystemTickConfig) {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.MaxSampleAge <= 0 {
		cfg.MaxSampleAge = 2 * cfg.Interval
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("alerts.system_tick.stopped")
			return
		case <-ticker.C:
			emitFromRecorder(ctx, logger, recorder, evaluator, cfg.MaxSampleAge)
		}
	}
}

func emitFromRecorder(ctx context.Context, logger zerolog.Logger, recorder *sysinfo.Recorder, evaluator *Evaluator, maxAge time.Duration) {
	if recorder == nil || evaluator == nil {
		return
	}
	hist := recorder.Snapshot(0)
	if hist == nil {
		return
	}
	now := time.Now().UTC()

	if n := len(hist.CPU); n > 0 {
		s := hist.CPU[n-1]
		if !s.Timestamp.IsZero() && now.Sub(s.Timestamp) <= maxAge {
			evaluator.Evaluate(ctx, Signal{
				Type:      "system_cpu",
				Value:     s.UsagePercent,
				Timestamp: s.Timestamp,
			})
		}
	}
	if n := len(hist.Memory); n > 0 {
		s := hist.Memory[n-1]
		if !s.Timestamp.IsZero() && now.Sub(s.Timestamp) <= maxAge {
			evaluator.Evaluate(ctx, Signal{
				Type:      "system_memory",
				Value:     s.UsedPercent,
				Timestamp: s.Timestamp,
			})
		}
	}
	if n := len(hist.Disk); n > 0 {
		s := hist.Disk[n-1]
		if !s.Timestamp.IsZero() && now.Sub(s.Timestamp) <= maxAge {
			evaluator.Evaluate(ctx, Signal{
				Type:      "system_disk",
				Value:     s.UsedPercent,
				Timestamp: s.Timestamp,
			})
		}
	}
	logger.Debug().Msg("alerts.system_tick.emitted")
}
