package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"vps-dashboard-api/internal/app"
	"vps-dashboard-api/internal/mcp"
)

// MCPHandler exposes the MCP protocol endpoints for AI agents.
// Authentication is via a static API key (Authorization: Bearer <key>).
type MCPHandler struct {
	App *app.App
	Svc *mcp.Server
}

func NewMCPHandler(a *app.App) *MCPHandler {
	if a.MCP == nil {
		return &MCPHandler{App: a, Svc: nil}
	}
	return &MCPHandler{App: a, Svc: a.MCP}
}

// Register mounts MCP routes. These are public (API key auth is inline).
func (h *MCPHandler) Register(rg *gin.RouterGroup) {
	rg.POST("/mcp", h.handle)
	rg.GET("/mcp/tools", h.listTools)
}

// mcpRequest is the JSON-RPC 2.0 envelope.
type mcpRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
	ID      any    `json:"id"`
}

// mcpResponse is the JSON-RPC 2.0 response.
type mcpResponse struct {
	JSONRPC string `json:"jsonrpc"`
	Result  any    `json:"result,omitempty"`
	Error   *mcpError `json:"error,omitempty"`
	ID      any    `json:"id"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// handle is the main MCP JSON-RPC endpoint.
func (h *MCPHandler) handle(c *gin.Context) {
	if h.Svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mcp_not_configured"})
		return
	}

	// API key auth: Authorization: Bearer <key> or X-API-Key header.
	if !h.checkAPIKey(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_api_key"})
		return
	}

	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_body"})
		return
	}

	// Try to parse as a single request or a batch.
	var single mcpRequest
	if err := jsonUnmarshal(body, &single); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_json", "detail": err.Error()})
		return
	}

	resp := h.processRequest(c, single)
	c.JSON(http.StatusOK, resp)
}

// listTools is a convenience REST endpoint (non-JSON-RPC) for
// discovering available tools.
func (h *MCPHandler) listTools(c *gin.Context) {
	if h.Svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "mcp_not_configured"})
		return
	}
	if !h.checkAPIKey(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_api_key"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tools": h.Svc.ListTools()})
}

// processRequest dispatches a single JSON-RPC request.
func (h *MCPHandler) processRequest(c *gin.Context, req mcpRequest) mcpResponse {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	switch req.Method {
	case "tools/list":
		return mcpResponse{
			JSONRPC: "2.0",
			Result:  gin.H{"tools": h.Svc.ListTools()},
			ID:      req.ID,
		}

	case "tools/call":
		return h.processToolCall(ctx, req)

	case "initialize":
		return mcpResponse{
			JSONRPC: "2.0",
			Result: gin.H{
				"protocolVersion": "2024-11-05",
				"capabilities": gin.H{
					"tools": gin.H{"listChanged": false},
				},
				"serverInfo": gin.H{
					"name":    "vps-dashboard-mcp",
					"version": "1.0.0",
				},
			},
			ID: req.ID,
		}

	default:
		return mcpResponse{
			JSONRPC: "2.0",
			Error:   &mcpError{Code: -32601, Message: "method not found: " + req.Method},
			ID:      req.ID,
		}
	}
}

// processToolCall handles a tools/call JSON-RPC request.
func (h *MCPHandler) processToolCall(ctx context.Context, req mcpRequest) mcpResponse {
	// Extract tool name and arguments from params.
	params, ok := req.Params.(map[string]any)
	if !ok {
		return mcpResponse{
			JSONRPC: "2.0",
			Error:   &mcpError{Code: -32602, Message: "invalid params"},
			ID:      req.ID,
		}
	}
	toolName, _ := params["name"].(string)
	if toolName == "" {
		return mcpResponse{
			JSONRPC: "2.0",
			Error:   &mcpError{Code: -32602, Message: "missing tool name"},
			ID:      req.ID,
		}
	}
	args, _ := params["arguments"].(map[string]any)

	result, err := h.Svc.CallTool(ctx, toolName, args)
	if err != nil {
		return mcpResponse{
			JSONRPC: "2.0",
			Result:  result,
			ID:      req.ID,
		}
	}
	return mcpResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      req.ID,
	}
}

// checkAPIKey validates the API key from Authorization header or
// X-API-Key header using constant-time comparison to prevent timing attacks.
func (h *MCPHandler) checkAPIKey(c *gin.Context) bool {
	key := h.Svc.APIKey()
	if key == "" {
		// No API key configured = deny all.
		return false
	}
	// Check X-API-Key header first (constant-time compare).
	if provided := c.GetHeader("X-API-Key"); provided != "" {
		if subtle.ConstantTimeCompare([]byte(provided), []byte(key)) == 1 {
			return true
		}
	}
	// Check Authorization: Bearer <key>.
	auth := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if len(auth) > len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
		provided := strings.TrimSpace(auth[len(prefix):])
		if provided != "" {
			if subtle.ConstantTimeCompare([]byte(provided), []byte(key)) == 1 {
				return true
			}
		}
	}
	return false
}

// jsonUnmarshal wraps encoding/json.Unmarshal.
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
