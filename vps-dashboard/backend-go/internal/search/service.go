// Package search implements cross-resource infrastructure search
// (PROJECT ARCHITECTURE.md §26, Phase 10): a single query that
// searches across servers, commands, tunnels, and tags, returning
// ranked results grouped by resource type.
//
// The search is inspired by Purple's infrastructure-oriented search
// model: one box, everything searchable, results grouped by kind.
package search

import (
	"context"
	"strings"

	"vps-dashboard-api/internal/models"
)

// ResultKind enumerates the searchable resource types.
const (
	KindServer  = "server"
	KindCommand = "command"
	KindTunnel  = "tunnel"
	KindTag     = "tag"
)

// Result is a single search hit.
type Result struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Detail  string `json:"detail,omitempty"`
	Link    string `json:"link,omitempty"` // frontend route
}

// Results is the aggregate search response, grouped by kind.
type Results struct {
	Query   string   `json:"query"`
	Total   int      `json:"total"`
	Servers []Result `json:"servers"`
	Commands []Result `json:"commands"`
	Tunnels  []Result `json:"tunnels"`
	Tags     []Result `json:"tags"`
}

// Service searches across registered repositories.
type Service struct {
	Servers  *models.ServerRepo
	Snippets *models.CommandSnippetRepo
	Tunnels  *models.TunnelRepo
}

// NewService constructs a search Service.
func NewService(servers *models.ServerRepo, snippets *models.CommandSnippetRepo, tunnels *models.TunnelRepo) *Service {
	return &Service{Servers: servers, Snippets: snippets, Tunnels: tunnels}
}

// Search executes a cross-resource query. The query matches
// case-insensitively against names, hostnames, commands, and tag
// names. Results are capped at 20 per kind to keep responses snappy.
func (s *Service) Search(ctx context.Context, query string) Results {
	q := strings.TrimSpace(strings.ToLower(query))
	out := Results{Query: query}
	if q == "" {
		return out
	}

	// Servers: search name, hostname, ip_address, provider.
	allServers := []models.Server{}
	if s.Servers != nil {
		filtered, _ := s.Servers.List(ctx, models.ServerFilter{Search: query})
		for _, srv := range filtered {
			if len(out.Servers) >= 20 {
				break
			}
			detail := srv.Hostname
			if srv.IPAddress != "" {
				detail += " / " + srv.IPAddress
			}
			if srv.Provider != "" {
				detail += " (" + srv.Provider + ")"
			}
			out.Servers = append(out.Servers, Result{
				Kind:   KindServer,
				ID:     srv.ID,
				Name:   srv.Name,
				Detail: detail,
				Link:   "/servers",
			})
		}
		// Also load all servers for tag search (unfiltered).
		allServers, _ = s.Servers.List(ctx, models.ServerFilter{})
	}

	// Commands: search name, description, command text.
	if s.Snippets != nil {
		snippets, _ := s.Snippets.List(ctx)
		for _, snip := range snippets {
			if len(out.Commands) >= 20 {
				break
			}
			haystack := strings.ToLower(snip.Name + " " + snip.Description + " " + snip.Command)
			if !strings.Contains(haystack, q) {
				continue
			}
			out.Commands = append(out.Commands, Result{
				Kind:   KindCommand,
				ID:     snip.ID,
				Name:   snip.Name,
				Detail: snip.Command,
				Link:   "/commands",
			})
		}
	}

	// Tunnels: search name, local_addr, remote_addr.
	if s.Tunnels != nil {
		tunnels, _ := s.Tunnels.List(ctx)
		for _, t := range tunnels {
			if len(out.Tunnels) >= 20 {
				break
			}
			haystack := strings.ToLower(t.Name + " " + t.LocalAddr + " " + t.RemoteAddr)
			if !strings.Contains(haystack, q) {
				continue
			}
			detail := t.Type + " " + t.LocalAddr
			if t.RemoteAddr != "" {
				detail += " → " + t.RemoteAddr
			}
			out.Tunnels = append(out.Tunnels, Result{
				Kind:   KindTunnel,
				ID:     t.ID,
				Name:   t.Name,
				Detail: detail,
				Link:   "/tunnels",
			})
		}
	}

	// Tags: collect unique tags from all servers that match the query.
	seen := make(map[string]bool)
	for _, srv := range allServers {
		for _, tag := range srv.Tags {
			tagLower := strings.ToLower(tag)
			if !strings.Contains(tagLower, q) {
				continue
			}
			if seen[tag] {
				continue
			}
			seen[tag] = true
			if len(out.Tags) >= 20 {
				break
			}
			out.Tags = append(out.Tags, Result{
				Kind:  KindTag,
				ID:    tag,
				Name:  tag,
				Link:  "/servers?tag=" + tag,
			})
		}
	}

	out.Total = len(out.Servers) + len(out.Commands) + len(out.Tunnels) + len(out.Tags)
	return out
}
