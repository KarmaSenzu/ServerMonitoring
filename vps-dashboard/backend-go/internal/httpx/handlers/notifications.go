package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/alerts"
	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/notifier"
)

// notificationsTimeout caps every /notifications request and the
// /alerts/rules subset routed by this handler.
const notificationsTimeout = 8 * time.Second

// NotificationsHandler exposes the notification channels and alert
// rules CRUD plus the per-rule and per-channel test endpoints.
type NotificationsHandler struct {
	App      *app.App
	Channels *models.ChannelRepo
	Rules    *models.AlertRepo
	Notifier *notifier.Service
	Eval     *alerts.Evaluator
}

// NewNotificationsHandler constructs a handler. Repos default to the
// values stored on a, falling back to fresh repos built from a.DB so
// older test paths that bypass the wiring keep working.
func NewNotificationsHandler(a *app.App) *NotificationsHandler {
	channels := a.Channels
	if channels == nil {
		channels = models.NewChannelRepo(a.DB)
	}
	rules := a.Alerts
	if rules == nil {
		rules = models.NewAlertRepo(a.DB)
	}
	return &NotificationsHandler{
		App:      a,
		Channels: channels,
		Rules:    rules,
		Notifier: a.Notifier,
		Eval:     a.AlertEvaluator,
	}
}

// RegisterChannelsReads mounts /notifications/channels list and get.
// Both admin and viewer roles are allowed by the caller.
func (h *NotificationsHandler) RegisterChannelsReads(rg *gin.RouterGroup) {
	rg.GET("/notifications/channels", h.listChannels)
	rg.GET("/notifications/channels/:id", h.getChannel)
}

// RegisterChannelsWrites mounts the admin-only channel mutators and
// the per-channel test endpoint.
func (h *NotificationsHandler) RegisterChannelsWrites(rg *gin.RouterGroup) {
	rg.POST("/notifications/channels", h.createChannel)
	rg.PATCH("/notifications/channels/:id", h.patchChannel)
	rg.DELETE("/notifications/channels/:id", h.deleteChannel)
	rg.POST("/notifications/channels/:id/test", h.testChannel)
}

// RegisterRulesReads mounts /alerts/rules list and get.
func (h *NotificationsHandler) RegisterRulesReads(rg *gin.RouterGroup) {
	rg.GET("/alerts/rules", h.listRules)
	rg.GET("/alerts/rules/:id", h.getRule)
}

// RegisterRulesWrites mounts the admin-only rule mutators and the
// per-rule test endpoint.
func (h *NotificationsHandler) RegisterRulesWrites(rg *gin.RouterGroup) {
	rg.POST("/alerts/rules", h.createRule)
	rg.PATCH("/alerts/rules/:id", h.patchRule)
	rg.DELETE("/alerts/rules/:id", h.deleteRule)
	rg.POST("/alerts/rules/:id/test", h.testRule)
}

// channelView is the JSON shape of a channel returned to clients. Note
// that bot_token (and any other secret-looking value in config) are
// NEVER included; instead the boolean bot_token_present surfaces
// whether the secret is configured. See sanitizeChannel for the
// stripping rules.
type channelView struct {
	ID              string         `json:"id"`
	Type            string         `json:"type"`
	Name            string         `json:"name"`
	Enabled         bool           `json:"enabled"`
	Config          map[string]any `json:"config"`
	BotTokenPresent bool           `json:"bot_token_present"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// sanitizeChannel converts a stored Channel into a safe view by
// stripping every config key that contains "token" or "secret"
// (case-insensitive). The bot_token's presence is exposed as the
// boolean bot_token_present so admins know whether to re-enter it.
func sanitizeChannel(ch models.Channel) channelView {
	cfg := map[string]any{}
	tokenPresent := false
	for k, v := range ch.Config {
		lk := strings.ToLower(k)
		if k == "bot_token" {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				tokenPresent = true
			}
			continue
		}
		if strings.Contains(lk, "token") || strings.Contains(lk, "secret") {
			// Other secret-looking keys are dropped silently. Their
			// presence is intentionally NOT surfaced via *_present
			// helpers; only bot_token has that affordance because it
			// is the one supported channel field today.
			continue
		}
		cfg[k] = v
	}
	return channelView{
		ID:              ch.ID,
		Type:            ch.Type,
		Name:            ch.Name,
		Enabled:         ch.Enabled,
		Config:          cfg,
		BotTokenPresent: tokenPresent,
		CreatedAt:       ch.CreatedAt,
		UpdatedAt:       ch.UpdatedAt,
	}
}

func (h *NotificationsHandler) listChannels(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), notificationsTimeout)
	defer cancel()

	rows, err := h.Channels.List(ctx)
	if err != nil {
		h.serverError(c, "channels.list", err)
		return
	}
	out := make([]channelView, 0, len(rows))
	for _, r := range rows {
		out = append(out, sanitizeChannel(r))
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *NotificationsHandler) getChannel(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), notificationsTimeout)
	defer cancel()

	ch, err := h.Channels.Get(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, models.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "channels.get", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sanitizeChannel(ch)})
}

// channelDTO is the JSON shape accepted on POST. config.bot_token is
// the only place where the secret can be supplied.
type channelDTO struct {
	Type    string         `json:"type"`
	Name    string         `json:"name"`
	Enabled *bool          `json:"enabled"`
	Config  map[string]any `json:"config"`
}

func (h *NotificationsHandler) createChannel(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), notificationsTimeout)
	defer cancel()

	var dto channelDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}
	enabled := true
	if dto.Enabled != nil {
		enabled = *dto.Enabled
	}
	cfg := dto.Config
	if cfg == nil {
		cfg = map[string]any{}
	}
	ch := models.Channel{
		Type:    strings.TrimSpace(dto.Type),
		Name:    strings.TrimSpace(dto.Name),
		Enabled: enabled,
		Config:  cfg,
	}

	created, err := h.Channels.Create(ctx, ch)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateChannelName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		// validateChannel returns plain errors; map them to 400.
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
			return
		}
		h.serverError(c, "channels.create", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": sanitizeChannel(created)})
}

// channelPatchDTO uses pointers so the handler can distinguish "field
// not present" from "field set to empty". config is merged shallowly
// into the existing config map: keys present in the incoming map
// overwrite, keys absent are kept. To clear a key explicitly set its
// value to JSON null.
type channelPatchDTO struct {
	Type    *string        `json:"type"`
	Name    *string        `json:"name"`
	Enabled *bool          `json:"enabled"`
	Config  map[string]any `json:"config"`
}

func (h *NotificationsHandler) patchChannel(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), notificationsTimeout)
	defer cancel()

	id := c.Param("id")
	existing, err := h.Channels.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "channels.patch_lookup", err)
		return
	}

	var dto channelPatchDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	if dto.Type != nil {
		existing.Type = strings.TrimSpace(*dto.Type)
	}
	if dto.Name != nil {
		existing.Name = strings.TrimSpace(*dto.Name)
	}
	if dto.Enabled != nil {
		existing.Enabled = *dto.Enabled
	}
	if dto.Config != nil {
		mergedCfg := mergeConfig(existing.Config, dto.Config)
		existing.Config = mergedCfg
	}

	updated, err := h.Channels.Update(ctx, existing)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateChannelName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		if errors.Is(err, models.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
			return
		}
		h.serverError(c, "channels.patch", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": sanitizeChannel(updated)})
}

// mergeConfig overlays patch onto base. Special rules:
//   - When a key is present in patch with a JSON null value, it is
//     removed from the merged map.
//   - When bot_token is present in patch but with an empty string,
//     it is treated as "not set" (the existing token is kept). This
//     mirrors the documented contract that empty strings are NOT
//     accepted as a way to clear the secret; explicit null is.
func mergeConfig(base, patch map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range patch {
		if v == nil {
			delete(out, k)
			continue
		}
		if k == "bot_token" {
			if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
				continue
			}
		}
		out[k] = v
	}
	return out
}

func (h *NotificationsHandler) deleteChannel(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), notificationsTimeout)
	defer cancel()

	if err := h.Channels.Delete(ctx, c.Param("id")); err != nil {
		if errors.Is(err, models.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "channels.delete", err)
		return
	}
	c.Status(http.StatusNoContent)
}

// testChannel sends a fixed test message to the channel. The HTTP
// status is 200 even on delivery failure; the per-channel error is
// returned in the body so the UI can surface it without retrying.
func (h *NotificationsHandler) testChannel(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), notificationsTimeout)
	defer cancel()

	id := c.Param("id")
	if h.Notifier == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "notifier_unavailable"})
		return
	}
	if _, err := h.Channels.Get(ctx, id); err != nil {
		if errors.Is(err, models.ErrChannelNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "channels.test_lookup", err)
		return
	}

	msg := notifier.Message{
		Title:    "VPS Dashboard test",
		Text:     "Hello from VPS Dashboard. If you can read this, the channel works.",
		Severity: models.SeverityInfo,
	}
	delivered, errs := h.Notifier.Notify(ctx, []string{id}, msg)
	resp := gin.H{
		"channel_id": id,
		"delivered":  len(delivered) == 1,
	}
	if e, ok := errs[id]; ok && e != nil {
		resp["error"] = e.Error()
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

// ruleDTO is the JSON shape accepted on POST/PATCH rules.
type ruleDTO struct {
	Name            string         `json:"name"`
	Enabled         *bool          `json:"enabled"`
	Type            string         `json:"type"`
	Threshold       float64        `json:"threshold"`
	Comparator      string         `json:"comparator"`
	ForSeconds      int            `json:"for_seconds"`
	CooldownSeconds int            `json:"cooldown_seconds"`
	Severity        string         `json:"severity"`
	Channels        []string       `json:"channels"`
	Scope           map[string]any `json:"scope"`
}

func (d ruleDTO) toRule() models.AlertRule {
	enabled := true
	if d.Enabled != nil {
		enabled = *d.Enabled
	}
	channels := d.Channels
	if channels == nil {
		channels = []string{}
	}
	scope := d.Scope
	if scope == nil {
		scope = map[string]any{}
	}
	return models.AlertRule{
		Name:            strings.TrimSpace(d.Name),
		Enabled:         enabled,
		Type:            d.Type,
		Threshold:       d.Threshold,
		Comparator:      d.Comparator,
		ForSeconds:      d.ForSeconds,
		CooldownSeconds: d.CooldownSeconds,
		Severity:        d.Severity,
		Channels:        channels,
		Scope:           scope,
	}
}

func (h *NotificationsHandler) listRules(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), notificationsTimeout)
	defer cancel()

	rows, err := h.Rules.List(ctx)
	if err != nil {
		h.serverError(c, "rules.list", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows})
}

func (h *NotificationsHandler) getRule(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), notificationsTimeout)
	defer cancel()

	r, err := h.Rules.Get(ctx, c.Param("id"))
	if err != nil {
		if errors.Is(err, models.ErrAlertRuleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "rules.get", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": r})
}

func (h *NotificationsHandler) createRule(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), notificationsTimeout)
	defer cancel()

	var dto ruleDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}
	rule := dto.toRule()

	created, err := h.Rules.Create(ctx, rule)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateAlertRuleName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
			return
		}
		h.serverError(c, "rules.create", err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": created})
}

// rulePatchDTO uses pointers everywhere so missing fields keep the
// existing value and explicit zero-valued fields are honored.
type rulePatchDTO struct {
	Name            *string        `json:"name"`
	Enabled         *bool          `json:"enabled"`
	Type            *string        `json:"type"`
	Threshold       *float64       `json:"threshold"`
	Comparator      *string        `json:"comparator"`
	ForSeconds      *int           `json:"for_seconds"`
	CooldownSeconds *int           `json:"cooldown_seconds"`
	Severity        *string        `json:"severity"`
	Channels        *[]string      `json:"channels"`
	Scope           map[string]any `json:"scope"`
}

func (h *NotificationsHandler) patchRule(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), notificationsTimeout)
	defer cancel()

	id := c.Param("id")
	existing, err := h.Rules.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrAlertRuleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "rules.patch_lookup", err)
		return
	}

	var dto rulePatchDTO
	if err := c.ShouldBindJSON(&dto); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	if dto.Name != nil {
		existing.Name = strings.TrimSpace(*dto.Name)
	}
	if dto.Enabled != nil {
		existing.Enabled = *dto.Enabled
	}
	if dto.Type != nil {
		existing.Type = *dto.Type
	}
	if dto.Threshold != nil {
		existing.Threshold = *dto.Threshold
	}
	if dto.Comparator != nil {
		existing.Comparator = *dto.Comparator
	}
	if dto.ForSeconds != nil {
		existing.ForSeconds = *dto.ForSeconds
	}
	if dto.CooldownSeconds != nil {
		existing.CooldownSeconds = *dto.CooldownSeconds
	}
	if dto.Severity != nil {
		existing.Severity = *dto.Severity
	}
	if dto.Channels != nil {
		existing.Channels = *dto.Channels
	}
	if dto.Scope != nil {
		existing.Scope = dto.Scope
	}

	updated, err := h.Rules.Update(ctx, existing)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateAlertRuleName) {
			c.JSON(http.StatusConflict, gin.H{"error": "duplicate_name"})
			return
		}
		if errors.Is(err, models.ErrAlertRuleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
			return
		}
		h.serverError(c, "rules.patch", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": updated})
}

func (h *NotificationsHandler) deleteRule(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), notificationsTimeout)
	defer cancel()

	if err := h.Rules.Delete(ctx, c.Param("id")); err != nil {
		if errors.Is(err, models.ErrAlertRuleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "rules.delete", err)
		return
	}
	c.Status(http.StatusNoContent)
}

// testRule synthesizes a Signal that satisfies the rule, then forces
// evaluation through the evaluator (which bypasses cooldown and the
// sustained-duration gate but does not touch state). The HTTP status
// is 200 even when partial channels fail.
func (h *NotificationsHandler) testRule(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), notificationsTimeout)
	defer cancel()

	id := c.Param("id")
	if h.Eval == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "evaluator_unavailable"})
		return
	}
	rule, err := h.Rules.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrAlertRuleNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "rules.test_lookup", err)
		return
	}

	sig := buildTestSignal(rule)
	delivered, errs := h.Eval.Force(ctx, rule, sig)
	errOut := map[string]string{}
	for k, v := range errs {
		if v != nil {
			errOut[k] = v.Error()
		}
	}
	if delivered == nil {
		delivered = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"delivered": delivered,
		"errors":    errOut,
	}})
}

// buildTestSignal constructs a Signal that satisfies rule. For numeric
// rules the value is offset from the threshold to land on the
// breaching side of the comparator; for state-typed rules a "down"
// state is emitted with the rule's scope copied over so scopeMatches
// passes.
func buildTestSignal(rule models.AlertRule) alerts.Signal {
	now := time.Now().UTC()
	switch rule.Type {
	case models.AlertTypeSystemCPU, models.AlertTypeSystemMemory, models.AlertTypeSystemDisk:
		offset := 0.0
		switch rule.Comparator {
		case models.ComparatorGTE, models.ComparatorEQ:
			offset = 1
		case models.ComparatorLTE:
			offset = -1
		case models.ComparatorNEQ:
			offset = 1
		}
		return alerts.Signal{
			Type:      rule.Type,
			Value:     rule.Threshold + offset,
			Timestamp: now,
		}
	case models.AlertTypeProjectHealth, models.AlertTypeContainerState, models.AlertTypeTunnelState:
		sig := alerts.Signal{
			Type:      rule.Type,
			State:     "down",
			Timestamp: now,
		}
		if v, ok := rule.Scope["project_id"].(string); ok {
			sig.ProjectID = v
		}
		if v, ok := rule.Scope["container"].(string); ok {
			sig.Container = v
		}
		if v, ok := rule.Scope["tunnel_service"].(string); ok {
			sig.TunnelService = v
		}
		return sig
	}
	return alerts.Signal{Type: rule.Type, Timestamp: now}
}

func (h *NotificationsHandler) serverError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("notifications_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}

// isValidationError treats any error whose message starts with one of
// the model-layer prefixes as a 400-class user input issue rather than
// a 500-class server fault. The repos return such errors from
// Validate() and validateChannel() before any SQL is issued.
func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "channel:"):
		return true
	case strings.HasPrefix(msg, "alert_rule:"):
		return true
	}
	return false
}
