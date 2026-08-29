// Package notifier dispatches alert messages over user-configured
// channels. Today only Telegram is supported, but the Sender interface
// is intended to be extended to other transports without affecting
// the calling code.
package notifier

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog"
	"golang.org/x/time/rate"

	"vps-dashboard-api/internal/models"
)

// Message is a single notification delivered to one or more channels.
type Message struct {
	Title    string
	Text     string
	Severity string
	ProjectID string
	Data     map[string]any
	// Channels, when non-empty, narrows delivery to a subset of channel
	// IDs. Service.Notify expects this list to come from the alert rule.
	Channels []string
}

// Sender is the per-channel-type implementation responsible for
// transmitting the rendered message to the remote service.
type Sender interface {
	Send(ctx context.Context, ch *models.Channel, m Message) error
}

// Service routes Notify calls to the matching sender, applying a
// per-channel rate limit token bucket along the way.
type Service struct {
	Logger   zerolog.Logger
	Senders  map[string]Sender
	Channels *models.ChannelRepo

	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

// NewService constructs a Service. senders is keyed by channel type
// (e.g. "telegram"). The map must not be nil.
func NewService(l zerolog.Logger, repo *models.ChannelRepo, senders map[string]Sender) *Service {
	return &Service{
		Logger:   l,
		Senders:  senders,
		Channels: repo,
		limiters: map[string]*rate.Limiter{},
	}
}

// Notify resolves each channel id, applies rate limiting, and forwards
// the message to the matching sender. Disabled channels are skipped
// with a warning log entry. Errors are returned per-channel so callers
// can surface partial failures without aborting the rest.
func (s *Service) Notify(ctx context.Context, channelIDs []string, m Message) (delivered []string, errs map[string]error) {
	delivered = make([]string, 0, len(channelIDs))
	errs = make(map[string]error)

	if len(channelIDs) == 0 {
		return delivered, errs
	}

	for _, id := range channelIDs {
		ch, err := s.Channels.Get(ctx, id)
		if err != nil {
			errs[id] = fmt.Errorf("notifier: load channel %s: %w", id, err)
			continue
		}
		if !ch.Enabled {
			s.Logger.Warn().
				Str("channel_id", ch.ID).
				Str("channel_name", ch.Name).
				Msg("notifier.channel_disabled")
			errs[id] = fmt.Errorf("notifier: channel %s disabled", ch.ID)
			continue
		}
		sender, ok := s.Senders[ch.Type]
		if !ok {
			errs[id] = fmt.Errorf("notifier: no sender for type %q", ch.Type)
			continue
		}

		if err := s.waitLimiter(ctx, ch.ID); err != nil {
			errs[id] = fmt.Errorf("notifier: rate limit %s: %w", ch.ID, err)
			continue
		}

		if err := sender.Send(ctx, &ch, m); err != nil {
			errs[id] = fmt.Errorf("notifier: send to %s: %w", ch.ID, err)
			continue
		}
		delivered = append(delivered, ch.ID)
	}
	return delivered, errs
}

// limiterFor returns the channel's token bucket, lazily creating one
// the first time it is requested.
func (s *Service) limiterFor(channelID string) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.limiters[channelID]
	if ok {
		return l
	}
	// 1 message/sec sustained, burst 5.
	l = rate.NewLimiter(rate.Limit(1), 5)
	s.limiters[channelID] = l
	return l
}

func (s *Service) waitLimiter(ctx context.Context, channelID string) error {
	return s.limiterFor(channelID).Wait(ctx)
}
