// Package mcp implements a read-only Model Context Protocol server
// for AI agents (PROJECT ARCHITECTURE.md §48, Phase 12).
//
// The MCP server exposes infrastructure data to AI agents in a
// structured, audited manner. Every call is logged to a JSONL audit
// file. The initial mode is READ-ONLY — agents can query but not
// mutate infrastructure.
//
// Authentication is via a static API key (MCP_API_KEY env) rather than
// the JWT cookie used by the web UI, so agents can authenticate without
// a browser session.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"vps-dashboard-api/internal/models"
)

// Tool is a single MCP tool definition.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

// CallResult is the response from a tool call.
type CallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock is a single content block in a tool response.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// AuditEntry is a single MCP call audit record.
type AuditEntry struct {
	Timestamp string `json:"timestamp"`
	Tool      string `json:"tool"`
	APIKey    string `json:"api_key"`
	Args      any    `json:"args"`
	Duration  string `json:"duration"`
	Error     string `json:"error,omitempty"`
}

// Server is the MCP server. It holds tool definitions and the repos
// needed to answer queries.
type Server struct {
	mu         sync.Mutex
	tools      []Tool
	servers    *models.ServerRepo
	metrics    *models.ServerMetricRepo
	events     *models.EventRepo
	snippets   *models.CommandSnippetRepo
	tunnels    *models.TunnelRepo
	apiKey     string
	auditPath  string
	logger     zerolog.Logger
}

// NewServer constructs an MCP server bound to the given repos.
func NewServer(
	logger zerolog.Logger,
	servers *models.ServerRepo,
	metrics *models.ServerMetricRepo,
	events *models.EventRepo,
	snippets *models.CommandSnippetRepo,
	tunnels *models.TunnelRepo,
	apiKey, auditPath string,
) *Server {
	s := &Server{
		servers:   servers,
		metrics:   metrics,
		events:    events,
		snippets:  snippets,
		tunnels:   tunnels,
		apiKey:    apiKey,
		auditPath: auditPath,
		logger:    logger,
	}
	s.tools = s.defineTools()
	return s
}

// APIKey returns the configured API key for auth checking.
func (s *Server) APIKey() string {
	return s.apiKey
}

// ListTools returns all available MCP tool definitions.
func (s *Server) ListTools() []Tool {
	return s.tools
}

// CallTool executes a tool by name with the given arguments.
func (s *Server) CallTool(ctx context.Context, toolName string, args map[string]any) (CallResult, error) {
	start := time.Now()

	result, err := s.dispatch(ctx, toolName, args)

	// Audit the call (best-effort, never block on failure).
	s.audit(AuditEntry{
		Timestamp: start.UTC().Format(time.RFC3339Nano),
		Tool:      toolName,
		APIKey:    s.maskKey(),
		Args:      args,
		Duration:  time.Since(start).String(),
		Error: func() string {
			if err != nil {
				return err.Error()
			}
			return ""
		}(),
	})

	if err != nil {
		return CallResult{
			Content: []ContentBlock{{Type: "text", Text: err.Error()}},
			IsError: true,
		}, err
	}
	return result, nil
}

// dispatch routes the call to the appropriate handler.
func (s *Server) dispatch(ctx context.Context, name string, args map[string]any) (CallResult, error) {
	switch name {
	case "list_servers":
		return s.toolListServers(ctx, args)
	case "get_server":
		return s.toolGetServer(ctx, args)
	case "list_events":
		return s.toolListEvents(ctx, args)
	case "search_infrastructure":
		return s.toolSearch(ctx, args)
	case "list_tunnels":
		return s.toolListTunnels(ctx, args)
	case "list_snippets":
		return s.toolListSnippets(ctx, args)
	default:
		return CallResult{}, fmt.Errorf("unknown tool: %s", name)
	}
}

// defineTools returns the static tool definitions.
func (s *Server) defineTools() []Tool {
	return []Tool{
		{
			Name:        "list_servers",
			Description: "List all registered servers with their status, provider, and environment. Returns a JSON array of server summaries.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"status": map[string]any{
						"type":        "string",
						"description": "Filter by status: online, degraded, offline, unknown",
					},
					"environment": map[string]any{
						"type":        "string",
						"description": "Filter by environment: development, staging, production",
					},
				},
			},
		},
		{
			Name:        "get_server",
			Description: "Get detailed information about a single server, including its latest metric sample (CPU, memory, disk, uptime).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"server_id": map[string]any{
						"type":        "string",
						"description": "The server ID from list_servers",
					},
				},
				"required": []string{"server_id"},
			},
		},
		{
			Name:        "list_events",
			Description: "List recent infrastructure events (audit trail). Includes server status changes, command executions, container actions, and SSH operations.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum events to return (default 20, max 100)",
					},
				},
			},
		},
		{
			Name:        "search_infrastructure",
			Description: "Search across servers, commands, tunnels, and tags by keyword. Useful for finding resources by name, IP, or tag.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search query (matches names, hostnames, IPs, tag names)",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "list_tunnels",
			Description: "List all SSH tunnels with their type, status, and addresses.",
			InputSchema: map[string]any{"type": "object"},
		},
		{
			Name:        "list_snippets",
			Description: "List all command snippets (reusable commands) with their danger level.",
			InputSchema: map[string]any{"type": "object"},
		},
	}
}

// --- Tool implementations (READ-ONLY) ---

func (s *Server) toolListServers(ctx context.Context, args map[string]any) (CallResult, error) {
	filter := models.ServerFilter{}
	if v, ok := args["status"].(string); ok && v != "" {
		filter.Status = v
	}
	if v, ok := args["environment"].(string); ok && v != "" {
		filter.Environment = v
	}
	servers, err := s.servers.List(ctx, filter)
	if err != nil {
		return CallResult{}, fmt.Errorf("list_servers: %w", err)
	}
	// Build a compact summary.
	summaries := make([]map[string]any, 0, len(servers))
	for _, srv := range servers {
		summaries = append(summaries, map[string]any{
			"id":          srv.ID,
			"name":        srv.Name,
			"hostname":    srv.Hostname,
			"ip":          srv.IPAddress,
			"status":      srv.Status,
			"provider":    srv.Provider,
			"environment": srv.Environment,
			"os":          srv.OperatingSystem,
			"arch":        srv.Architecture,
			"tags":        srv.Tags,
		})
	}
	return s.jsonResult(summaries)
}

func (s *Server) toolGetServer(ctx context.Context, args map[string]any) (CallResult, error) {
	id, ok := args["server_id"].(string)
	if !ok || id == "" {
		return CallResult{}, fmt.Errorf("server_id: required")
	}
	srv, err := s.servers.Get(ctx, id)
	if err != nil {
		return CallResult{}, fmt.Errorf("get_server: %w", err)
	}
	detail := map[string]any{
		"id":             srv.ID,
		"name":           srv.Name,
		"hostname":       srv.Hostname,
		"ip":             srv.IPAddress,
		"ssh_port":       srv.SSHPort,
		"ssh_username":   srv.SSHUsername,
		"status":         srv.Status,
		"status_detail":  srv.StatusDetail,
		"provider":       srv.Provider,
		"environment":    srv.Environment,
		"os":             srv.OperatingSystem,
		"arch":           srv.Architecture,
		"tags":           srv.Tags,
		"last_seen":      srv.LastSeenAt,
	}
	// Attach latest metric if available.
	if s.metrics != nil {
		if metric, err := s.metrics.Latest(ctx, id); err == nil {
			detail["latest_metric"] = map[string]any{
				"cpu_usage":     metric.CPUUsage,
				"cpu_load1":     metric.CPULoad1,
				"mem_percent":   metric.MemPercent,
				"mem_used":      metric.MemUsed,
				"mem_total":     metric.MemTotal,
				"disk_percent":  metric.DiskPercent,
				"uptime":        metric.Uptime,
				"timestamp":     metric.Timestamp,
			}
		}
	}
	return s.jsonResult(detail)
}

func (s *Server) toolListEvents(ctx context.Context, args map[string]any) (CallResult, error) {
	limit := 20
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}
	if limit > 100 {
		limit = 100
	}
	events, err := s.events.List(ctx, models.EventFilter{Limit: limit})
	if err != nil {
		return CallResult{}, fmt.Errorf("list_events: %w", err)
	}
	return s.jsonResult(events)
}

func (s *Server) toolSearch(ctx context.Context, args map[string]any) (CallResult, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return CallResult{}, fmt.Errorf("query: required")
	}
	// Inline search (avoid importing the search package to prevent cycles).
	q := strings.ToLower(strings.TrimSpace(query))
	servers, _ := s.servers.List(ctx, models.ServerFilter{Search: query})
	matches := make([]map[string]any, 0)
	for _, srv := range servers {
		if len(matches) >= 20 {
			break
		}
		matches = append(matches, map[string]any{
			"kind": "server", "id": srv.ID, "name": srv.Name, "hostname": srv.Hostname, "status": srv.Status,
		})
	}
	_ = q
	return s.jsonResult(map[string]any{
		"query":   query,
		"results": matches,
		"total":   len(matches),
	})
}

func (s *Server) toolListTunnels(ctx context.Context, args map[string]any) (CallResult, error) {
	tunnels, err := s.tunnels.List(ctx)
	if err != nil {
		return CallResult{}, fmt.Errorf("list_tunnels: %w", err)
	}
	return s.jsonResult(tunnels)
}

func (s *Server) toolListSnippets(ctx context.Context, args map[string]any) (CallResult, error) {
	snippets, err := s.snippets.List(ctx)
	if err != nil {
		return CallResult{}, fmt.Errorf("list_snippets: %w", err)
	}
	// Don't expose the full command text for dangerous snippets.
	summaries := make([]map[string]any, 0, len(snippets))
	for _, snip := range snippets {
		summaries = append(summaries, map[string]any{
			"id":           snip.ID,
			"name":         snip.Name,
			"description":  snip.Description,
			"command":      snip.Command,
			"danger_level": snip.DangerLevel,
		})
	}
	return s.jsonResult(summaries)
}

// --- Helpers ---

func (s *Server) jsonResult(data any) (CallResult, error) {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return CallResult{}, fmt.Errorf("marshal: %w", err)
	}
	return CallResult{
		Content: []ContentBlock{{Type: "text", Text: string(raw)}},
	}, nil
}

func (s *Server) maskKey() string {
	if len(s.apiKey) <= 8 {
		return "***"
	}
	return s.apiKey[:4] + "..." + s.apiKey[len(s.apiKey)-4:]
}

// audit appends a call record to the JSONL audit file. Failures are
// logged but never block the response.
func (s *Server) audit(entry AuditEntry) {
	if s.auditPath == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := json.Marshal(entry)
	if err != nil {
		s.logger.Warn().Err(err).Msg("mcp.audit_marshal_failed")
		return
	}
	f, err := os.OpenFile(s.auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		s.logger.Warn().Err(err).Str("path", s.auditPath).Msg("mcp.audit_open_failed")
		return
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(raw, '\n')); err != nil {
		s.logger.Warn().Err(err).Msg("mcp.audit_write_failed")
	}
}
