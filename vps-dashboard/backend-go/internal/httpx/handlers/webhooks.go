package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/deploy"
	"vps-dashboard-api/internal/models"
)

// webhookMaxBody caps the inbound webhook payload size to keep memory
// usage bounded. GitHub push payloads are well under this.
const webhookMaxBody = 1 << 20 // 1 MiB

// webhookTimeout caps the synchronous portion of the webhook handler.
const webhookTimeout = 8 * time.Second

// WebhooksHandler exposes the public per-project webhook receiver. The
// handler is mounted on the router's PUBLIC group (no auth middleware)
// because HMAC over the request body is the authentication mechanism.
type WebhooksHandler struct {
	App      *app.App
	Projects *models.ProjectRepo
	Deploy   *deploy.Service
}

// NewWebhooksHandler builds a WebhooksHandler bound to a.
func NewWebhooksHandler(a *app.App) *WebhooksHandler {
	repo := models.NewProjectRepo(a.DB)
	return &WebhooksHandler{App: a, Projects: repo, Deploy: a.DeployService}
}

// RegisterPublic mounts the webhook routes. The supplied group must
// NOT carry the RequireAuth middleware.
func (h *WebhooksHandler) RegisterPublic(rg *gin.RouterGroup) {
	rg.POST("/webhooks/deploy/:project_id", h.deploy)
}

// deploy handles POST /webhooks/deploy/:project_id.
//
// Authentication: the X-Hub-Signature-256 header MUST contain
// "sha256=<hex_lowercase>" of HMAC-SHA256(webhook_secret, raw_body).
// Comparison uses constant-time equality.
//
// Behaviour:
//   - 413 if the body exceeds webhookMaxBody
//   - 404 if the project is missing
//   - 403 if deploy_enabled=false
//   - 412 precondition_failed if webhook_secret is empty
//   - 401 invalid_signature when HMAC fails
//   - 200 with status=pong on GitHub ping payloads ({"zen": ...} or X-Event=ping)
//   - 200 with status=ignored when X-Event is set and is not push|ping
//   - 429 already_running when a deploy is in flight
//   - 202 with the new deployment_id on success
func (h *WebhooksHandler) deploy(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), webhookTimeout)
	defer cancel()

	projectID := strings.TrimSpace(c.Param("project_id"))
	if projectID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_project_id"})
		return
	}

	body, err := readLimited(c.Request.Body, webhookMaxBody)
	if err != nil {
		if errors.Is(err, errBodyTooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "body_too_large"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "read_body_failed", "detail": err.Error()})
		return
	}

	// Detect GitHub ping early. A ping is a benign liveness check that
	// must NOT trigger a deploy, but should respond 200 so the sender
	// reports green.
	event := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Event")))
	if event == "" {
		event = strings.ToLower(strings.TrimSpace(c.GetHeader("X-GitHub-Event")))
	}
	if event == "ping" || isGitHubPing(body) {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "pong"}})
		return
	}

	project, err := h.Projects.Get(ctx, projectID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.App.Logger.Error().Err(err).Str("project_id", projectID).Msg("webhook.lookup_failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
		return
	}
	if !project.DeployEnabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "deploy_disabled"})
		return
	}
	if strings.TrimSpace(project.WebhookSecret) == "" {
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "precondition_failed", "detail": "webhook_secret_not_set"})
		return
	}

	if !verifySignature(c.GetHeader("X-Hub-Signature-256"), project.WebhookSecret, body) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_signature"})
		return
	}

	if event != "" && event != "push" && event != "ping" {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": "ignored", "reason": "event_type"}})
		return
	}

	remoteRef := extractRemoteRef(body)

	d, err := h.Deploy.Trigger(ctx, project, "webhook", remoteRef)
	if err != nil {
		if errors.Is(err, deploy.ErrAlreadyRunning) {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "already_running"})
			return
		}
		if errors.Is(err, deploy.ErrNotConfigured) {
			c.JSON(http.StatusForbidden, gin.H{"error": "deploy_disabled"})
			return
		}
		h.App.Logger.Error().Err(err).Str("project_id", project.ID).Msg("webhook.trigger_failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"data": gin.H{
		"deployment_id": d.ID,
		"status":        d.Status,
		"remote_ref":    remoteRef,
	}})
}

// errBodyTooLarge is returned by readLimited when the request body
// would exceed the cap.
var errBodyTooLarge = errors.New("webhook: body too large")

// readLimited reads up to max+1 bytes; if max is reached it returns
// errBodyTooLarge so the caller can answer 413.
func readLimited(r io.ReadCloser, max int) ([]byte, error) {
	defer func() { _ = r.Close() }()
	buf := bytes.NewBuffer(make([]byte, 0, 4096))
	n, err := io.Copy(buf, io.LimitReader(r, int64(max)+1))
	if err != nil {
		return nil, err
	}
	if n > int64(max) {
		return nil, errBodyTooLarge
	}
	return buf.Bytes(), nil
}

// verifySignature checks header against HMAC-SHA256(secret, body).
// Header must be "sha256=<hex>" with lowercase hex digits.
func verifySignature(header, secret string, body []byte) bool {
	const prefix = "sha256="
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	got, err := hex.DecodeString(header[len(prefix):])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := mac.Sum(nil)
	return hmac.Equal(got, want)
}

// isGitHubPing reports whether body looks like a GitHub ping payload
// (i.e. contains a top-level "zen" string). The probe parses only the
// first few KiB to avoid spending CPU on large unsigned payloads.
func isGitHubPing(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	probe := body
	if len(probe) > 4096 {
		probe = probe[:4096]
	}
	var pl struct {
		Zen string `json:"zen"`
	}
	if err := json.Unmarshal(probe, &pl); err != nil {
		return false
	}
	return strings.TrimSpace(pl.Zen) != ""
}

// extractRemoteRef pulls "after" (preferred) or "ref" from a JSON
// payload to populate deployments.remote_ref. Failures are silent
// because remote_ref is purely an audit field.
func extractRemoteRef(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var pl struct {
		Ref   string `json:"ref"`
		After string `json:"after"`
	}
	if err := json.Unmarshal(body, &pl); err != nil {
		return ""
	}
	if v := strings.TrimSpace(pl.After); v != "" {
		return v
	}
	return strings.TrimSpace(pl.Ref)
}
