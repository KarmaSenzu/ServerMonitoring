package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/docker"
	"vps-dashboard-api/internal/safeexec"
)

// DockerHandler exposes the /docker/* endpoints. Read-only routes are
// safe for any authenticated user; mutating routes are gated behind
// RequireRole("admin") by the caller.
type DockerHandler struct {
	App *app.App
	Svc *docker.Service
}

// NewDockerHandler builds a DockerHandler with a service that uses the
// app's logger.
func NewDockerHandler(a *app.App) *DockerHandler {
	return &DockerHandler{App: a, Svc: docker.NewService(a.Logger)}
}

// dockerListTimeout caps a `docker ps` invocation.
const dockerListTimeout = 8 * time.Second

// dockerLogsTimeout caps a `docker logs --tail N` snapshot.
const dockerLogsTimeout = 10 * time.Second

// dockerStreamCap caps the total time an SSE log stream may run.
const dockerStreamCap = 60 * time.Second

// dockerMutateTimeout caps start/stop/restart, which can wait on
// container teardown.
const dockerMutateTimeout = 30 * time.Second

// RegisterReads mounts read-only routes. The caller is expected to
// supply auth middleware on rg.
func (h *DockerHandler) RegisterReads(rg *gin.RouterGroup) {
	rg.GET("/docker/containers", h.list)
	rg.GET("/docker/status", h.list) // back-compat alias
	rg.GET("/docker/containers/:name/logs", h.logsSnapshot)
	rg.GET("/docker/containers/:name/logs/stream", h.logsStream)
}

// RegisterWrites mounts mutating routes. The caller is expected to
// supply auth + admin-role middleware on rg.
func (h *DockerHandler) RegisterWrites(rg *gin.RouterGroup) {
	rg.POST("/docker/containers/:name/start", h.startByName)
	rg.POST("/docker/containers/:name/stop", h.stopByName)
	rg.POST("/docker/containers/:name/restart", h.restartByName)

	// Back-compat aliases for the legacy frontend payload shape.
	rg.POST("/docker/start", h.startBody)
	rg.POST("/docker/stop", h.stopBody)
	rg.POST("/docker/restart", h.restartBody)
}

// list handles GET /docker/containers (and the /docker/status alias).
func (h *DockerHandler) list(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), dockerListTimeout)
	defer cancel()

	containers, err := h.Svc.List(ctx)
	if err != nil {
		h.respondDockerError(c, "docker.list", err, "")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": containers})
}

func (h *DockerHandler) startByName(c *gin.Context) {
	name := c.Param("name")
	h.runStart(c, name)
}

func (h *DockerHandler) stopByName(c *gin.Context) {
	name := c.Param("name")
	t := readTimeoutSeconds(c)
	h.runStop(c, name, t)
}

func (h *DockerHandler) restartByName(c *gin.Context) {
	name := c.Param("name")
	t := readTimeoutSeconds(c)
	h.runRestart(c, name, t)
}

type startBodyReq struct {
	Name string `json:"name"`
}

type stopBodyReq struct {
	Name           string `json:"name"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func (h *DockerHandler) startBody(c *gin.Context) {
	var req startBodyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "details": err.Error()})
		return
	}
	h.runStart(c, req.Name)
}

func (h *DockerHandler) stopBody(c *gin.Context) {
	var req stopBodyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "details": err.Error()})
		return
	}
	h.runStop(c, req.Name, req.TimeoutSeconds)
}

func (h *DockerHandler) restartBody(c *gin.Context) {
	var req stopBodyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body", "details": err.Error()})
		return
	}
	h.runRestart(c, req.Name, req.TimeoutSeconds)
}

func (h *DockerHandler) runStart(c *gin.Context, name string) {
	if err := safeexec.ValidateContainerName(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_container_name"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), dockerMutateTimeout)
	defer cancel()
	if err := h.Svc.Start(ctx, name); err != nil {
		h.respondDockerError(c, "docker.start", err, name)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"name": name, "action": "start"}})
}

func (h *DockerHandler) runStop(c *gin.Context, name string, timeoutSec int) {
	if err := safeexec.ValidateContainerName(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_container_name"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), dockerMutateTimeout)
	defer cancel()
	if err := h.Svc.Stop(ctx, name, timeoutSec); err != nil {
		h.respondDockerError(c, "docker.stop", err, name)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"name": name, "action": "stop"}})
}

func (h *DockerHandler) runRestart(c *gin.Context, name string, timeoutSec int) {
	if err := safeexec.ValidateContainerName(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_container_name"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), dockerMutateTimeout)
	defer cancel()
	if err := h.Svc.Restart(ctx, name, timeoutSec); err != nil {
		h.respondDockerError(c, "docker.restart", err, name)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"name": name, "action": "restart"}})
}

// readTimeoutSeconds best-effort parses {"timeout_seconds": int} from
// the request body without consuming binding state. It returns 0 when
// nothing is supplied; the service layer treats 0 as "use default".
func readTimeoutSeconds(c *gin.Context) int {
	if c.Request.Body == nil {
		return 0
	}
	if c.Request.ContentLength == 0 {
		return 0
	}
	var body struct {
		TimeoutSeconds int `json:"timeout_seconds"`
	}
	if err := json.NewDecoder(c.Request.Body).Decode(&body); err != nil {
		return 0
	}
	return body.TimeoutSeconds
}

func (h *DockerHandler) respondDockerError(c *gin.Context, op string, err error, name string) {
	if errors.Is(err, docker.ErrDockerUnavailable) {
		h.App.Logger.Warn().Str("op", op).Str("name", name).Msg("docker_unavailable")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker_unavailable"})
		return
	}
	h.App.Logger.Error().Err(err).Str("op", op).Str("name", name).Msg("docker_command_failed")
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":  "docker_command_failed",
		"detail": err.Error(),
	})
}

// logsSnapshot handles GET /docker/containers/:name/logs. It returns
// the most recent stdout/stderr separately, with a truncated flag set
// when either stream exceeded the 1 MiB-per-stream budget.
func (h *DockerHandler) logsSnapshot(c *gin.Context) {
	name := c.Param("name")
	if err := safeexec.ValidateContainerName(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_container_name"})
		return
	}

	tail := 200
	if v := strings.TrimSpace(c.Query("tail")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 5000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_tail"})
			return
		}
		tail = n
	}

	since := strings.TrimSpace(c.Query("since"))
	if since != "" {
		if err := safeexec.ValidateDuration(since); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_since"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), dockerLogsTimeout)
	defer cancel()

	stdout, stderr, truncated, err := h.Svc.Logs(ctx, name, tail, since)
	if err != nil {
		h.respondDockerError(c, "docker.logs", err, name)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"name":      name,
		"tail":      tail,
		"stdout":    stdout,
		"stderr":    stderr,
		"truncated": truncated,
	}})
}

// logsStream handles GET /docker/containers/:name/logs/stream.
// It multiplexes stdout/stderr lines from `docker logs -f` over an
// SSE channel and stops when the client disconnects, the underlying
// process exits, or dockerStreamCap is reached.
func (h *DockerHandler) logsStream(c *gin.Context) {
	name := c.Param("name")
	if err := safeexec.ValidateContainerName(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_container_name"})
		return
	}

	tail := 100
	if v := strings.TrimSpace(c.Query("tail")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 5000 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_tail"})
			return
		}
		tail = n
	}

	// Bound the total stream duration; the goroutines exit when the
	// caller cancels this context or the cmd finishes on its own.
	streamCtx, cancel := context.WithTimeout(c.Request.Context(), dockerStreamCap)
	defer cancel()

	stdout, stderr, cmd, err := h.Svc.StreamLogs(streamCtx, name, tail, true)
	if err != nil {
		// Surface as JSON 4xx/5xx instead of opening an SSE stream so the
		// client gets a normal error envelope.
		h.respondDockerError(c, "docker.logs.stream", err, name)
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()

	events := make(chan sseEvent, 32)
	done := make(chan struct{})
	var doneOnce sync.Once
	closeDone := func() { doneOnce.Do(func() { close(done) }) }

	go pumpLogLines(stdout, "stdout", events, done)
	go pumpLogLines(stderr, "stderr", events, done)

	// Wait for the process in a goroutine so we can react to exit
	// alongside client-disconnect/timeout.
	exitCh := make(chan exitInfo, 1)
	go func() {
		err := cmd.Wait()
		exitCh <- exitInfo{Err: err, ExitCode: extractExitCode(err)}
	}()

	cleanup := func(reason string, exitCode int) {
		// Signal pumpLogLines to stop and best-effort kill the child.
		closeDone()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		// Drain Wait so the process is reaped before we return.
		select {
		case <-exitCh:
		case <-time.After(500 * time.Millisecond):
		}
		writeSSE(c.Writer, "end", fmt.Sprintf(`{"reason":%q,"exit_code":%d}`, reason, exitCode))
	}

	clientGone := c.Request.Context().Done()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-clientGone:
			cleanup("close", -1)
			return false
		case <-streamCtx.Done():
			if errors.Is(streamCtx.Err(), context.DeadlineExceeded) {
				cleanup("timeout", -1)
			} else {
				cleanup("close", -1)
			}
			return false
		case info := <-exitCh:
			reason := "exit"
			if info.Err != nil && info.ExitCode == -1 {
				reason = "error"
			}
			drainSSEEvents(c.Writer, events)
			writeSSE(c.Writer, "end", fmt.Sprintf(`{"reason":%q,"exit_code":%d}`, reason, info.ExitCode))
			closeDone()
			return false
		case ev := <-events:
			writeSSE(c.Writer, ev.Name, ev.Data)
			return true
		}
	})
}

// sseEvent is a single named event ready to be flushed over the wire.
type sseEvent struct {
	Name string
	Data string
}

// pumpLogLines reads line-delimited input from r and forwards each line
// (capped at 8 KiB) onto events, until r is closed or done is signalled.
// It always closes r on return.
func pumpLogLines(r io.ReadCloser, label string, events chan<- sseEvent, done <-chan struct{}) {
	defer func() { _ = r.Close() }()
	scanner := bufio.NewScanner(r)
	const maxLine = 8 * 1024
	scanner.Buffer(make([]byte, 0, 4*1024), maxLine)
	for scanner.Scan() {
		line := scanner.Text()
		payload, _ := json.Marshal(map[string]string{"line": line})
		select {
		case events <- sseEvent{Name: label, Data: string(payload)}:
		case <-done:
			return
		}
	}
}

type exitInfo struct {
	Err      error
	ExitCode int
}

// extractExitCode pulls a process exit code from an error returned by
// cmd.Wait(). Non-exit errors yield -1.
func extractExitCode(err error) int {
	if err == nil {
		return 0
	}
	type exitCoder interface{ ExitCode() int }
	if ec, ok := err.(exitCoder); ok {
		return ec.ExitCode()
	}
	return -1
}

// writeSSE formats a single SSE frame and flushes it.
func writeSSE(w gin.ResponseWriter, event, data string) {
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
	w.Flush()
}

// drainSSEEvents flushes any buffered events on the channel before the
// final end frame. It does not block waiting for new events.
func drainSSEEvents(w gin.ResponseWriter, events <-chan sseEvent) {
	for {
		select {
		case ev := <-events:
			writeSSE(w, ev.Name, ev.Data)
		default:
			return
		}
	}
}
