package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/httpx/middleware"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/ssh"
)

// sshKeysTimeout caps key management requests.
const sshKeysTimeout = 8 * time.Second

// sshTestTimeout bounds the outbound SSH connectivity probe
// (connect + handshake). The engine applies its own 10s default; the
// HTTP deadline is slightly larger to leave room for classification.
const sshTestTimeout = 15 * time.Second

// sshCommandTimeout bounds the outbound command execution. The engine
// caps commands at 30s by default; the HTTP deadline adds slack.
const sshCommandTimeout = 35 * time.Second

// SSHHandler exposes the SSH engine endpoints: key management (admin
// only), connectivity tests and command execution against registered
// servers (admin only — these are infrastructure-changing surfaces).
type SSHHandler struct {
	App    *app.App
	Repo   *models.ServerRepo
	Keys   *ssh.KeyStore
	Engine *ssh.Service
}

// NewSSHHandler constructs an SSHHandler. When a.SSHService is wired
// by main the shared engine is reused; otherwise a fresh one is built
// from a temp key store so tests that bypass main wiring still work.
func NewSSHHandler(a *app.App) *SSHHandler {
	keys := a.SSHKeys
	if keys == nil {
		// A KeyStore cannot fail except on an unwritable dir; fall
		// back to the OS temp dir so the handler degrades instead of
		// panicking.
		dir := a.Cfg.SSHKeysDir
		if dir == "" {
			dir = filepath.Join(os.TempDir(), "vpsd-ssh-keys")
		}
		ks, err := ssh.NewKeyStore(dir)
		if err != nil {
			ks, _ = ssh.NewKeyStore(filepath.Join(os.TempDir(), fmt.Sprintf("vpsd-ssh-keys-%d", time.Now().UnixNano())))
		}
		keys = ks
	}
	engine := a.SSHService
	if engine == nil {
		engine = ssh.NewService(keys)
	}
	return &SSHHandler{
		App:    a,
		Repo:   models.NewServerRepo(a.DB),
		Keys:   keys,
		Engine: engine,
	}
}

// RegisterReads mounts the read-only SSH routes (key listing).
func (h *SSHHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/ssh/keys", h.listKeys)
}

// RegisterWrites mounts the admin-only SSH routes (key management).
// Caller is responsible for adding admin-role middleware.
// SSH test/command are operator-level — see RegisterOperatorWrites.
func (h *SSHHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/ssh/keys", h.addKey)
	rg.POST("/ssh/keys/generate", h.generateKey)
	rg.GET("/ssh/keys/:name/public", h.getPublicKey)
	rg.DELETE("/ssh/keys/:name", h.deleteKey)
}

// RegisterOperatorWrites mounts SSH operations that operators can
// access (test + command execution). Key management stays admin-only.
// Must be called on a group with RequireRole("admin", "operator").
func (h *SSHHandler) RegisterOperatorWrites(rg *gin.RouterGroup) {
	rg.POST("/ssh/test/:id", h.testServer)
	rg.POST("/ssh/command/:id", h.runCommand)
}

// listKeys returns the key catalogue (metadata only — no private
// material ever leaves the store).
func (h *SSHHandler) listKeys(c *gin.Context) {
	keys, err := h.Keys.List()
	if err != nil {
		h.serverError(c, "ssh.keys.list", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": keys})
}

// addKeyBody is the payload for uploading an existing private key.
type addKeyBody struct {
	Name    string `json:"name" binding:"required"`
	Private string `json:"private_key" binding:"required"`
	Comment string `json:"comment"`
}

// addKey stores a supplied private key. The PEM payload is accepted
// exactly once and never echoed back. The comment is taken from the
// key blob when present.
func (h *SSHHandler) addKey(c *gin.Context) {
	var body addKeyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	meta, err := h.Keys.Add(strings.TrimSpace(body.Name), strings.TrimSpace(body.Private))
	if err != nil {
		switch {
		case errors.Is(err, ssh.ErrKeyExists):
			c.JSON(http.StatusConflict, gin.H{"error": "key_exists"})
		case errors.Is(err, ssh.ErrInvalidKey):
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_key", "detail": err.Error()})
		default:
			h.serverError(c, "ssh.keys.add", err)
		}
		return
	}

	h.auditSSH(c, "ssh_key_add", "", meta.Name, map[string]any{
		"key_type":    meta.Type,
		"fingerprint": meta.Fingerprint,
	})
	c.JSON(http.StatusCreated, gin.H{"data": meta})
}

// generateKeyBody is the payload for generating a fresh key pair.
type generateKeyBody struct {
	Name    string `json:"name" binding:"required"`
	Comment string `json:"comment"`
}

// generateKey creates a new Ed25519 key and returns the metadata plus
// the public key line (safe to distribute to authorized_keys).
func (h *SSHHandler) generateKey(c *gin.Context) {
	var body generateKeyBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}

	meta, public, err := h.Keys.Generate(strings.TrimSpace(body.Name), body.Comment)
	if err != nil {
		if errors.Is(err, ssh.ErrKeyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": "key_exists"})
			return
		}
		h.serverError(c, "ssh.keys.generate", err)
		return
	}

	h.auditSSH(c, "ssh_key_generate", "", meta.Name, map[string]any{
		"key_type":    meta.Type,
		"fingerprint": meta.Fingerprint,
	})
	c.JSON(http.StatusCreated, gin.H{"data": gin.H{
		"meta":       meta,
		"public_key": public,
	}})
}

// getPublicKey returns the OpenSSH-format public line of a stored key.
func (h *SSHHandler) getPublicKey(c *gin.Context) {
	name := c.Param("name")
	public, err := h.Keys.GetPublic(name)
	if err != nil {
		if errors.Is(err, ssh.ErrKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "ssh.keys.public", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"name": name, "public_key": public}})
}

// deleteKey removes a key from the store.
func (h *SSHHandler) deleteKey(c *gin.Context) {
	name := c.Param("name")
	if err := h.Keys.Remove(name); err != nil {
		if errors.Is(err, ssh.ErrKeyNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		h.serverError(c, "ssh.keys.delete", err)
		return
	}

	h.auditSSH(c, "ssh_key_delete", "", name, nil)
	c.Status(http.StatusNoContent)
}

// testServer performs a connectivity + authentication probe against a
// registered server and records the outcome on the server row
// (status/last_seen) plus an event. It is also the Phase 3 hook for
// the monitoring pipeline.
func (h *SSHHandler) testServer(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sshTestTimeout)
	defer cancel()

	s, err := h.lookupServer(c, ctx)
	if err != nil {
		return // lookupServer already responded.
	}

	res, testErr := h.Engine.Test(ctx, s)

	// Record the outcome on the registry row (best effort).
	status := models.ServerStatusOnline
	detail := ""
	if testErr != nil {
		status = models.ServerStatusOffline
		detail = testErr.Error()
		if len(detail) > 256 {
			detail = detail[:256]
		}
	}
	updateCtx, updateCancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = h.Repo.SetStatus(updateCtx, s.ID, status, detail, res.CheckedAt)
	updateCancel()

	if testErr != nil {
		severity := models.SeverityWarning
		if errors.Is(testErr, ssh.ErrHostKeyChanged) {
			severity = models.SeverityCritical
		}
		h.appendEvent(c, models.Event{
			Category: models.EventCategorySystem,
			Severity: severity,
			Source:   "ssh:" + s.Name,
			Message:  "SSH test failed for " + s.Name + ": " + classifySSHError(testErr),
			Data: map[string]any{
				"server_id":   s.ID,
				"server_name": s.Name,
				"error":       testErr.Error(),
			},
		})
		h.respondSSHError(c, testErr)
		return
	}

	h.appendEvent(c, models.Event{
		Category: models.EventCategorySystem,
		Severity: models.SeverityInfo,
		Source:   "ssh:" + s.Name,
		Message:  "SSH test succeeded for " + s.Name,
		Data: map[string]any{
			"server_id":   s.ID,
			"server_name": s.Name,
			"latency_ms":  res.LatencyMs,
			"fingerprint": res.Fingerprint,
		},
	})
	c.JSON(http.StatusOK, gin.H{"data": res})
}

// runCommandBody is the payload for remote command execution.
type runCommandBody struct {
	Command string `json:"command" binding:"required"`
	Timeout int    `json:"timeout_seconds"`
}

// maxSSHCommandBytes caps the length of a submitted command.
const maxSSHCommandBytes = 8192

// runCommand executes a single command on a registered server.
// Command execution is admin-only and always audited.
func (h *SSHHandler) runCommand(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), sshCommandTimeout)
	defer cancel()

	s, err := h.lookupServer(c, ctx)
	if err != nil {
		return
	}

	var body runCommandBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": err.Error()})
		return
	}
	command := strings.TrimSpace(body.Command)
	if command == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": "command: required"})
		return
	}
	if len(command) > maxSSHCommandBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "detail": "command: too long"})
		return
	}

	res, runErr := h.Engine.RunCommand(ctx, s, command)

	// Audit every execution attempt (success, failure, transport
	// error — all of them).
	severity := models.SeverityInfo
	if runErr != nil {
		severity = models.SeverityError
	}
	userID, _ := middleware.CurrentUserID(c)
	h.appendEvent(c, models.Event{
		Category: models.EventCategorySystem,
		Severity: severity,
		Source:   "ssh:" + s.Name,
		Message:  "Command executed on " + s.Name + ": " + truncate(command, 120),
		Data: map[string]any{
			"server_id":   s.ID,
			"server_name": s.Name,
			"command":     command,
			"exit_code":   res.ExitCode,
			"error":       res.Err,
			"duration_ms": res.DurationMs,
			"by_user_id":  userID,
		},
	})

	if runErr != nil {
		// A completed command with a non-zero exit status is a *result*,
		// not a transport error — RunCommand returns nil error for that
		// case. Reaching here means the transport itself failed.
		h.respondSSHError(c, runErr)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": res})
}

// lookupServer resolves the :id parameter into a Server row and
// writes the HTTP error response when it fails.
func (h *SSHHandler) lookupServer(c *gin.Context, ctx context.Context) (models.Server, error) {
	id := c.Param("id")
	s, err := h.Repo.Get(ctx, id)
	if err != nil {
		if errors.Is(err, models.ErrServerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return models.Server{}, err
		}
		h.serverError(c, "ssh.lookup", err)
		return models.Server{}, err
	}
	return s, nil
}

// respondSSHError maps engine errors to HTTP responses.
func (h *SSHHandler) respondSSHError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ssh.ErrHostUnreachable):
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "ssh_host_unreachable", "detail": err.Error()})
	case errors.Is(err, ssh.ErrAuthFailed):
		c.JSON(http.StatusUnauthorized, gin.H{"error": "ssh_auth_failed", "detail": err.Error()})
	case errors.Is(err, ssh.ErrHostKeyChanged):
		c.JSON(http.StatusConflict, gin.H{"error": "ssh_host_key_changed", "detail": err.Error()})
	case errors.Is(err, ssh.ErrCommandTimeout):
		c.JSON(http.StatusGatewayTimeout, gin.H{"error": "ssh_command_timeout", "detail": err.Error()})
	case errors.Is(err, ssh.ErrCredentialNotConfigured):
		c.JSON(http.StatusBadRequest, gin.H{"error": "ssh_credential_not_configured", "detail": err.Error()})
	default:
		c.JSON(http.StatusBadGateway, gin.H{"error": "ssh_error", "detail": err.Error()})
	}
}

// classifySSHError produces a short human phrase for events.
func classifySSHError(err error) string {
	switch {
	case errors.Is(err, ssh.ErrHostUnreachable):
		return "host unreachable"
	case errors.Is(err, ssh.ErrAuthFailed):
		return "authentication failed"
	case errors.Is(err, ssh.ErrHostKeyChanged):
		return "host key changed (possible MITM)"
	case errors.Is(err, ssh.ErrCredentialNotConfigured):
		return "credential not configured"
	case errors.Is(err, ssh.ErrCommandTimeout):
		return "command timed out"
	default:
		return "unknown error"
	}
}

// appendEvent records an infrastructure event. Failures are logged
// but never block the HTTP response.
func (h *SSHHandler) appendEvent(c *gin.Context, ev models.Event) {
	if h.App.Events == nil {
		return
	}
	if _, ok := ev.Data["by_user_id"]; !ok {
		userID, _ := middleware.CurrentUserID(c)
		ev.Data["by_user_id"] = userID
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := h.App.Events.Append(ctx, ev); err != nil {
		h.App.Logger.Warn().Err(err).Str("source", ev.Source).Msg("ssh.event_append_failed")
	}
}

// auditSSH is a thin alias for key-management audit events.
func (h *SSHHandler) auditSSH(c *gin.Context, action, serverID, keyName string, extra map[string]any) {
	data := map[string]any{"key_name": keyName}
	for k, v := range extra {
		data[k] = v
	}
	h.appendEvent(c, models.Event{
		Category: models.EventCategorySystem,
		Severity: models.SeverityInfo,
		Source:   "ssh-registry",
		Message:  action + " " + keyName,
		Data:     data,
	})
}

func (h *SSHHandler) serverError(c *gin.Context, op string, err error) {
	h.App.Logger.Error().Err(err).Str("op", op).Msg("ssh_handler_error")
	c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "detail": err.Error()})
}

// truncate shortens s to n runes with an ellipsis.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
