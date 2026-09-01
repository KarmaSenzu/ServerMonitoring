package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/httpx/middleware"
	"vps-dashboard-api/internal/models"
	"vps-dashboard-api/internal/ssh"
)

// TerminalHandler exposes the WebSocket endpoint for interactive SSH
// terminals (Phase 5: React → WebSocket → Go → SSH PTY).
type TerminalHandler struct {
	App  *app.App
	Repo *models.ServerRepo
}

// NewTerminalHandler constructs a TerminalHandler.
func NewTerminalHandler(a *app.App) *TerminalHandler {
	return &TerminalHandler{
		App:  a,
		Repo: models.NewServerRepo(a.DB),
	}
}

// upgrader allows connections from any origin. CORS is enforced at the
// HTTP layer; the WebSocket handshake itself is safe within the same
// origin or via the reverse proxy.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// TerminalMessage is the JSON envelope exchanged over the WebSocket.
// The frontend sends {type:"input", data:"..."} for keystrokes and
// {type:"resize", rows:N, cols:N} for window changes. The backend
// sends {type:"output", data:"..."} for PTY output and
// {type:"error", data:"..."} for fatal errors.
type TerminalMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Rows int    `json:"rows,omitempty"`
	Cols int    `json:"cols,omitempty"`
}

// terminalReadDeadline bounds idle time before the server pings.
const terminalReadDeadline = 60 * time.Second

// terminalWriteDeadline bounds a single WebSocket write.
const terminalWriteDeadline = 10 * time.Second

// terminalDialTimeout bounds the SSH dial for the PTY session.
const terminalDialTimeout = 15 * time.Second

// Register mounts the terminal WebSocket endpoint. Must be on an
// admin-only group (interactive terminals are infrastructure-changing
// surfaces).
func (h *TerminalHandler) Register(rg *gin.RouterGroup) {
	rg.GET("/servers/:id/terminal", h.connect)
}

// connect upgrades the HTTP request to a WebSocket, opens an SSH PTY
// to the requested server, and bidirectionally pipes data.
func (h *TerminalHandler) connect(c *gin.Context) {
	// WebSocket upgrade must happen first; the underlying connection
	// is hijacked so normal middleware (timeout) no longer applies.
	ws, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "websocket_upgrade_failed", "detail": err.Error()})
		return
	}
	defer func() { _ = ws.Close() }()

	// Look up the server.
	id := c.Param("id")
	lookupCtx, lookupCancel := context.WithTimeout(c.Request.Context(), 8*time.Second)
	srv, err := h.Repo.Get(lookupCtx, id)
	lookupCancel()
	if err != nil {
		_ = writeTerminalError(ws, "Server not found: "+err.Error())
		return
	}

	// Open the SSH PTY session.
	dialCtx, dialCancel := context.WithTimeout(context.Background(), terminalDialTimeout)
	session, err := h.App.SSHService.NewTerminalSession(dialCtx, srv, ssh.PTYSize{
		Rows: 24,
		Cols: 80,
	})
	dialCancel()
	if err != nil {
		_ = writeTerminalError(ws, classifyTerminalError(err))
		return
	}
	defer func() { _ = session.Close() }()

	// Audit: record the terminal session start.
	userID, _ := middleware.CurrentUserID(c)
	h.appendTerminalEvent(c, srv, "terminal_open", userID)

	// Bidirectional pipe:
	//   ws → session.Write   (keystrokes, resize)
	//   session.Read → ws    (PTY output)
	//
	// Either side closing ends the session.

	done := make(chan struct{})

	// PTY → WebSocket pump.
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := session.Read(buf)
			if n > 0 {
				_ = ws.SetWriteDeadline(time.Now().Add(terminalWriteDeadline))
				msg, _ := json.Marshal(TerminalMessage{Type: "output", Data: string(buf[:n])})
				if err := ws.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// WebSocket → PTY pump (blocking read loop).
	go func() {
		for {
			_ = ws.SetReadDeadline(time.Now().Add(terminalReadDeadline))
			_, payload, err := ws.ReadMessage()
			if err != nil {
				// Connection closed or timed out → kill the session.
				_ = session.Close()
				return
			}

			var msg TerminalMessage
			if err := json.Unmarshal(payload, &msg); err != nil {
				continue
			}

			switch msg.Type {
			case "input":
				_, _ = session.Write([]byte(msg.Data))
			case "resize":
				_ = session.Resize(msg.Rows, msg.Cols)
			case "close":
				_ = session.Close()
				return
			}
		}
	}()

	// Wait for the PTY pump to finish (remote shell exited).
	select {
	case <-done:
	case <-session.Done():
	}

	_ = writeTerminalError(ws, "Session closed")
}

func writeTerminalError(ws *websocket.Conn, message string) error {
	msg, _ := json.Marshal(TerminalMessage{Type: "error", Data: message})
	_ = ws.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return ws.WriteMessage(websocket.TextMessage, msg)
}

func classifyTerminalError(err error) string {
	switch {
	case errors.Is(err, ssh.ErrHostUnreachable):
		return "SSH host unreachable: " + err.Error()
	case errors.Is(err, ssh.ErrAuthFailed):
		return "SSH authentication failed: " + err.Error()
	case errors.Is(err, ssh.ErrHostKeyChanged):
		return "SSH host key changed (possible MITM): " + err.Error()
	case errors.Is(err, ssh.ErrCredentialNotConfigured):
		return "SSH credential not configured: " + err.Error()
	default:
		return "Terminal error: " + err.Error()
	}
}

// appendTerminalEvent records an audit event for terminal access.
func (h *TerminalHandler) appendTerminalEvent(c *gin.Context, srv models.Server, action, userID string) {
	if h.App.Events == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = h.App.Events.Append(ctx, models.Event{
		Category: models.EventCategoryAuth,
		Severity: models.SeverityInfo,
		Source:   "terminal:" + srv.Name,
		Message:  "Terminal opened on " + srv.Name,
		Data: map[string]any{
			"action":      action,
			"server_id":   srv.ID,
			"server_name": srv.Name,
			"by_user_id":  userID,
		},
	})
}
