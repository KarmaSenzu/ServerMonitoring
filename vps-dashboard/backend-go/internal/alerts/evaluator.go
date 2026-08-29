// Package alerts evaluates incoming Signal values against configured
// AlertRule rows and dispatches notifications via the notifier service.
package alerts

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/notifier"
)

// Signal carries the metric or state-change observation that the
// evaluator needs to assess.
type Signal struct {
	Type          string
	Value         float64
	State         string // "up" | "down" for *_state and project_health
	ProjectID     string
	Container     string
	TunnelService string
	Timestamp     time.Time
}

// State tracks transient evaluator memory: when each rule first crossed
// its threshold so a sustained-duration check can fire correctly.
type State struct {
	mu          sync.Mutex
	firstBreach map[string]time.Time
}

// NewState returns a zero-valued State suitable for evaluator use.
func NewState() *State {
	return &State{firstBreach: map[string]time.Time{}}
}

// Evaluator wires the alert pipeline together.
type Evaluator struct {
	Logger   zerolog.Logger
	Rules    *models.AlertRepo
	Channels *models.ChannelRepo
	Notifier *notifier.Service
	Events   *models.EventRepo
	State    *State
	EnvFloor *EnvFloor
}

// NewEvaluator constructs an Evaluator with a fresh State.
func NewEvaluator(l zerolog.Logger, rules *models.AlertRepo, channels *models.ChannelRepo, n *notifier.Service, events *models.EventRepo) *Evaluator {
	return &Evaluator{
		Logger:   l,
		Rules:    rules,
		Channels: channels,
		Notifier: n,
		Events:   events,
		State:    NewState(),
	}
}

// Evaluate inspects every enabled rule whose Type matches sig.Type and
// fires a notification when the rule's conditions are met. Failures
// inside one rule never affect the others.
func (e *Evaluator) Evaluate(ctx context.Context, sig Signal) {
	if sig.Timestamp.IsZero() {
		sig.Timestamp = time.Now().UTC()
	}
	rules, err := e.Rules.List(ctx)
	if err != nil {
		e.Logger.Warn().Err(err).Msg("alerts.evaluate.rules_list_failed")
		return
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if rule.Type != sig.Type {
			continue
		}
		if !scopeMatches(rule.Scope, sig) {
			continue
		}
		// Per-environment severity floor: when a project is in scope,
		// drop rules whose severity is below that environment's floor.
		if e.EnvFloor != nil && sig.ProjectID != "" {
			if !e.EnvFloor.Allow(ctx, sig.ProjectID, rule.Severity) {
				e.Logger.Debug().
					Str("rule_id", rule.ID).
					Str("project_id", sig.ProjectID).
					Str("rule_severity", rule.Severity).
					Msg("alerts.evaluate.suppressed_by_env_floor")
				continue
			}
		}
		e.evaluateRule(ctx, rule, sig, false)
	}
}

// Force runs evaluation for a single rule with cooldown/sustained-time
// gates bypassed. It is intended for the "test rule" endpoint and never
// updates the rule's first-breach state.
func (e *Evaluator) Force(ctx context.Context, rule models.AlertRule, sig Signal) (delivered []string, errs map[string]error) {
	if sig.Timestamp.IsZero() {
		sig.Timestamp = time.Now().UTC()
	}
	msg := buildMessage(rule, sig)
	delivered, errs = e.Notifier.Notify(ctx, rule.Channels, msg)
	e.appendFireEvent(ctx, rule, sig, msg, true)
	return delivered, errs
}

// evaluateRule applies a single rule to a signal, optionally forcing
// the fire path.
func (e *Evaluator) evaluateRule(ctx context.Context, rule models.AlertRule, sig Signal, force bool) {
	now := sig.Timestamp.UTC()
	key := stateKey(rule, sig)

	breached := false
	switch rule.Type {
	case models.AlertTypeSystemCPU, models.AlertTypeSystemMemory, models.AlertTypeSystemDisk:
		breached = compareNumeric(sig.Value, rule.Threshold, rule.Comparator)
	case models.AlertTypeProjectHealth, models.AlertTypeContainerState, models.AlertTypeTunnelState:
		// State rules fire when the observed state is "down".
		breached = strings.EqualFold(sig.State, "down")
	default:
		return
	}

	if !breached {
		e.State.clear(key)
		return
	}

	first := e.State.firstOrSet(key, now)

	if !force {
		// Sustained-duration gate (only meaningful for numeric rules; for
		// state rules ForSeconds=0 is the typical config).
		if rule.ForSeconds > 0 {
			if now.Sub(first) < time.Duration(rule.ForSeconds)*time.Second {
				return
			}
		}
		// Cooldown gate.
		if rule.CooldownSeconds > 0 && !rule.LastTriggeredAt.IsZero() {
			if now.Sub(rule.LastTriggeredAt) < time.Duration(rule.CooldownSeconds)*time.Second {
				return
			}
		}
	}

	msg := buildMessage(rule, sig)
	delivered, errs := e.Notifier.Notify(ctx, rule.Channels, msg)
	e.appendFireEvent(ctx, rule, sig, msg, force)

	if !force {
		if err := e.Rules.UpdateLastTriggered(ctx, rule.ID, now); err != nil {
			e.Logger.Warn().Err(err).Str("rule_id", rule.ID).Msg("alerts.update_last_triggered_failed")
		}
		// Clear breach so we re-arm only after the metric recovers.
		e.State.clear(key)
	}

	if len(errs) > 0 {
		for id, err := range errs {
			e.Logger.Warn().Err(err).Str("channel_id", id).Msg("alerts.notify_partial_error")
		}
	}
	e.Logger.Info().
		Str("rule_id", rule.ID).
		Str("rule_name", rule.Name).
		Int("delivered", len(delivered)).
		Int("errors", len(errs)).
		Msg("alerts.fired")
}

func (e *Evaluator) appendFireEvent(ctx context.Context, rule models.AlertRule, sig Signal, msg notifier.Message, forced bool) {
	data := map[string]any{
		"rule_id":   rule.ID,
		"rule_name": rule.Name,
		"type":      rule.Type,
		"value":     sig.Value,
		"state":     sig.State,
	}
	if forced {
		data["forced"] = true
	}
	source := "alert:" + rule.Name
	if _, err := e.Events.Append(ctx, models.Event{
		Category:  models.EventCategoryAlert,
		Severity:  rule.Severity,
		Source:    source,
		ProjectID: sig.ProjectID,
		Message:   msg.Title + ": " + msg.Text,
		Data:      data,
		Timestamp: sig.Timestamp,
	}); err != nil {
		e.Logger.Warn().Err(err).Str("rule_id", rule.ID).Msg("alerts.event_append_failed")
	}
}

// stateKey identifies a rule+scope pair so two scopes against the same
// rule track their breach windows independently.
func stateKey(rule models.AlertRule, sig Signal) string {
	parts := []string{rule.ID}
	if sig.ProjectID != "" {
		parts = append(parts, "p="+sig.ProjectID)
	}
	if sig.Container != "" {
		parts = append(parts, "c="+sig.Container)
	}
	if sig.TunnelService != "" {
		parts = append(parts, "t="+sig.TunnelService)
	}
	return strings.Join(parts, ":")
}

// scopeMatches checks the rule's Scope against the signal's metadata.
// An empty scope matches any signal.
func scopeMatches(scope map[string]any, sig Signal) bool {
	if len(scope) == 0 {
		return true
	}
	if v, ok := scope["project_id"].(string); ok && v != "" {
		if v != sig.ProjectID {
			return false
		}
	}
	if v, ok := scope["container"].(string); ok && v != "" {
		if v != sig.Container {
			return false
		}
	}
	if v, ok := scope["tunnel_service"].(string); ok && v != "" {
		if v != sig.TunnelService {
			return false
		}
	}
	return true
}

func compareNumeric(value, threshold float64, comparator string) bool {
	switch comparator {
	case models.ComparatorGTE:
		return value >= threshold
	case models.ComparatorLTE:
		return value <= threshold
	case models.ComparatorEQ:
		return value == threshold
	case models.ComparatorNEQ:
		return value != threshold
	}
	return false
}

func buildMessage(rule models.AlertRule, sig Signal) notifier.Message {
	title := "Alert: " + rule.Name
	body := describeSignal(rule, sig)
	return notifier.Message{
		Title:     title,
		Text:      body,
		Severity:  rule.Severity,
		ProjectID: sig.ProjectID,
		Channels:  rule.Channels,
		Data: map[string]any{
			"rule_id":     rule.ID,
			"rule_type":   rule.Type,
			"value":       sig.Value,
			"state":       sig.State,
			"project_id":  sig.ProjectID,
			"container":   sig.Container,
			"tunnel":      sig.TunnelService,
		},
	}
}

func describeSignal(rule models.AlertRule, sig Signal) string {
	switch rule.Type {
	case models.AlertTypeSystemCPU:
		return fmt.Sprintf("CPU %.1f%% %s %.1f%%", sig.Value, prettyComparator(rule.Comparator), rule.Threshold)
	case models.AlertTypeSystemMemory:
		return fmt.Sprintf("Memory %.1f%% %s %.1f%%", sig.Value, prettyComparator(rule.Comparator), rule.Threshold)
	case models.AlertTypeSystemDisk:
		return fmt.Sprintf("Disk %.1f%% %s %.1f%%", sig.Value, prettyComparator(rule.Comparator), rule.Threshold)
	case models.AlertTypeProjectHealth:
		return fmt.Sprintf("Project %s state=%s", sig.ProjectID, sig.State)
	case models.AlertTypeContainerState:
		return fmt.Sprintf("Container %s state=%s", sig.Container, sig.State)
	case models.AlertTypeTunnelState:
		return fmt.Sprintf("Tunnel %s state=%s", sig.TunnelService, sig.State)
	}
	return fmt.Sprintf("type=%s value=%.2f state=%s", rule.Type, sig.Value, sig.State)
}

func prettyComparator(c string) string {
	switch c {
	case models.ComparatorGTE:
		return ">="
	case models.ComparatorLTE:
		return "<="
	case models.ComparatorEQ:
		return "=="
	case models.ComparatorNEQ:
		return "!="
	}
	return c
}

// State helpers --------------------------------------------------------

func (s *State) firstOrSet(key string, now time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.firstBreach[key]; ok {
		return t
	}
	s.firstBreach[key] = now
	return now
}

func (s *State) clear(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.firstBreach, key)
}
